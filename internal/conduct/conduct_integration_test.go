package conduct

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/conductor"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	storeconductor "github.com/FelipeMalacarne/chronicle/internal/store/conductor"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

const account = "acc_auto"

type world struct {
	t      *testing.T
	jobs   *storejobs.Repository
	states *storeconductor.Repository
	log    *eventlog.Repository
	clock  time.Time
	d      *fakeDispatch
	c      *Conductor
}

// fakeDispatch answers DispatchNext from a queue; a Dispatched without a job
// creates one of the conductor and starts it, as the agent's run would.
type fakeDispatch struct {
	w      *world
	outs   []conductor.Outcome
	calls  int
	during func()
}

func (f *fakeDispatch) DispatchNext(ctx context.Context, acc string) conductor.Outcome {
	f.calls++
	if f.during != nil {
		f.during()
	}
	out := conductor.Outcome{Kind: conductor.Dispatched}
	if len(f.outs) > 0 {
		out, f.outs = f.outs[0], f.outs[1:]
	}
	if out.Kind == conductor.Dispatched && out.Job == "" {
		out.Job = f.w.job(job.SourceConductor).ID
	}
	return out
}

func setup(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_auto', 'u1', 'Auto', 'auto', 'regular', NULL, 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, jobs: storejobs.New(pool), states: storeconductor.New(pool), log: eventlog.New(pool),
		clock: time.Now().UTC().Truncate(time.Second)}
	w.d = &fakeDispatch{w: w}
	w.c = w.conductor()
	return w
}

// conductor is a fresh one, as after a restart: only the database carries over.
func (w *world) conductor() *Conductor {
	return &Conductor{States: w.states, Jobs: w.jobs, Log: w.log, Dispatch: w.d, Now: func() time.Time { return w.clock }}
}

func (w *world) step(all bool) {
	w.c.Step(context.Background(), all, map[string]bool{})
}

// job creates a running job of the account.
func (w *world) job(source job.Source) job.Job {
	w.t.Helper()
	ctx := context.Background()
	g, err := job.ParseGoal([]byte(`{"type":"varp","id":281,"min":1000}`))
	if err != nil {
		w.t.Fatal(err)
	}
	j, err := job.New(account, job.KindPlugin, nil, g, nil, 0, source)
	if err != nil {
		w.t.Fatal(err)
	}
	created, err := w.jobs.Create(ctx, j)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.jobs.Start(ctx, created.ID, "run started", nil, nil); err != nil {
		w.t.Fatal(err)
	}
	return created
}

func (w *world) succeed(id string) {
	w.t.Helper()
	proof, _ := job.Result{Verdict: rules.Met, Evidence: "observed 1000"}.Prove()
	if _, err := w.jobs.Succeed(context.Background(), id, proof, nil); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) state() conductor.State {
	w.t.Helper()
	st, err := w.states.Get(context.Background(), account)
	if err != nil {
		w.t.Fatal(err)
	}
	return st
}

func (w *world) turnOn() {
	w.t.Helper()
	if err := w.states.TurnOn(context.Background(), account); err != nil {
		w.t.Fatal(err)
	}
}

// The conductor walks the plan job by job, resumes from the database after a
// restart without dispatching again, and stops on a failed job until the
// owner turns it on again.
func TestConductorWalksThePlanPostgres(t *testing.T) {
	w := setup(t)
	w.step(true)
	if w.d.calls != 0 || w.state().Mode != conductor.Off {
		t.Fatalf("an account the owner did not turn on: calls %d, %+v", w.d.calls, w.state())
	}

	w.turnOn()
	w.c.Step(context.Background(), false, map[string]bool{account: true})
	first := w.state()
	if w.d.calls != 1 || first.Mode != conductor.Running || first.Job == "" {
		t.Fatalf("after turning on: calls %d, %+v", w.d.calls, first)
	}
	w.step(true)
	if w.d.calls != 1 {
		t.Fatalf("dispatched again with its job running: %d", w.d.calls)
	}

	w.succeed(first.Job)
	w.step(false) // the job.succeeded in the log names the account
	second := w.state()
	if w.d.calls != 2 || second.Mode != conductor.Running || second.Job == first.Job {
		t.Fatalf("after the first job succeeded: calls %d, %+v", w.d.calls, second)
	}

	// A restart: the event is behind the cursor, the job still runs.
	w.c = w.conductor()
	w.step(true)
	if w.d.calls != 2 || w.state() != second {
		t.Fatalf("a restart dispatched again: calls %d, %+v", w.d.calls, w.state())
	}

	if _, err := w.jobs.Fail(context.Background(), second.Job, "run ended: failed"); err != nil {
		t.Fatal(err)
	}
	w.step(false)
	blocked := w.state()
	if blocked.Mode != conductor.Blocked || !strings.Contains(blocked.Reason, "terminou failed: run ended: failed") || w.d.calls != 2 {
		t.Fatalf("after a failed job: calls %d, %+v", w.d.calls, blocked)
	}
	w.step(true)
	if w.d.calls != 2 {
		t.Fatal("a blocked account dispatched")
	}

	w.turnOn()
	w.c.Step(context.Background(), false, map[string]bool{account: true})
	if st := w.state(); w.d.calls != 3 || st.Mode != conductor.Running {
		t.Fatalf("turned on again: calls %d, %+v", w.d.calls, st)
	}
}

// A dispatch that has to wait keeps the account idle with why, and asks again
// only after retryEvery; a refused one blocks it.
func TestConductorWaitsThenIsRefusedPostgres(t *testing.T) {
	w := setup(t)
	w.d.outs = []conductor.Outcome{
		{Kind: conductor.Wait, Reason: "nenhum agent conectado para a conta"},
		{Kind: conductor.Refused, Reason: "o plano não tem tarefa a executar agora (done)"},
	}
	w.turnOn()
	w.step(true)
	if st := w.state(); st.Mode != conductor.Idle || st.Reason != "nenhum agent conectado para a conta" {
		t.Fatalf("after a wait: %+v", st)
	}
	w.step(true)
	if w.d.calls != 1 {
		t.Fatalf("asked again before retryEvery: %d", w.d.calls)
	}
	w.clock = w.clock.Add(retryEvery + time.Second)
	w.step(true)
	if st := w.state(); w.d.calls != 2 || st.Mode != conductor.Blocked || !strings.Contains(st.Reason, "(done)") {
		t.Fatalf("after a refusal: calls %d, %+v", w.d.calls, st)
	}
}

// The conductor never runs alongside the owner's job, and the owner turning
// it off while a task is sent wins.
func TestConductorYieldsToTheOwnerPostgres(t *testing.T) {
	w := setup(t)
	owners := w.job(job.SourceOwner)
	w.turnOn()
	w.step(true)
	if w.d.calls != 0 || w.state().Mode != conductor.Idle {
		t.Fatalf("dispatched beside the owner's job: calls %d, %+v", w.d.calls, w.state())
	}
	w.succeed(owners.ID)
	w.d.during = func() {
		if err := w.states.TurnOff(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}
	w.step(false)
	if st := w.state(); w.d.calls != 1 || st.Mode != conductor.Off {
		t.Fatalf("the owner's turn off lost: calls %d, %+v", w.d.calls, st)
	}
}

func TestTurnOnAnUnknownAccountPostgres(t *testing.T) {
	w := setup(t)
	if err := w.states.TurnOn(context.Background(), "acc_nobody"); err != storeconductor.ErrAccountNotFound {
		t.Fatalf("err = %v", err)
	}
}

func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL conductor integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("conduct_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, dropErr := admin.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE"); dropErr != nil {
			t.Errorf("drop test schema: %v", dropErr)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if pool, err = pgxpool.NewWithConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		up, _, found := strings.Cut(string(raw), "-- +goose Down")
		if !found {
			t.Fatalf("%s has no goose Down marker", f)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	return pool
}
