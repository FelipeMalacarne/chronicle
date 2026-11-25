package jobtracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	"github.com/FelipeMalacarne/chronicle/internal/routinesync"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

// A run.started whose transaction commits after a later event is still read:
// the reader never moves past an event a writer has not committed.
func TestLateCommittedEventIsNotSkippedPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.level("fishing", 70)
	w.step()
	j := w.create(job.KindTrain, `"skill:fishing>=76"`, 0)
	w.step()

	// The run.started of the job, not committed yet.
	txA, err := w.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventlog.Append(ctx, w.q.WithTx(txA), account, eventlog.TypeRunStarted, "u1", j.ID, map[string]any{"run": "u1", "job": j.ID}); err != nil {
		t.Fatal(err)
	}
	// Another event commits first; the reader must wait for A, not pass it.
	w.level("fishing", 71)
	w.tracker.Step(ctx, false)
	if err := txA.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	w.level("fishing", 76)
	w.step()
	if got := w.get(j.ID); got.State != job.Succeeded {
		t.Fatalf("state = %s (%s): run.started was skipped", got.State, got.Reason)
	}
}

// failingStart fails the first Start.
type failingStart struct {
	Jobs
	fail atomic.Bool
}

func (f *failingStart) Start(ctx context.Context, id, reason string, p *job.Progress, d *time.Time) (job.Job, error) {
	if f.fail.CompareAndSwap(true, false) {
		return job.Job{}, errors.New("database away")
	}
	return f.Jobs.Start(ctx, id, reason, p, d)
}

// An event whose handling failed is read again: the cursor does not pass it.
func TestFailedHandlingKeepsTheCursorPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.varp(281, 10)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, 0)
	w.step()
	f := &failingStart{Jobs: w.jobs}
	f.fail.Store(true)
	w.tracker.Jobs = f
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	w.step()
	if got := w.get(j.ID); got.State != job.Queued {
		t.Fatalf("state = %s after a failed start", got.State)
	}
	w.step()
	if got := w.get(j.ID); got.State != job.Running {
		t.Fatalf("state = %s: the event was not handled again", got.State)
	}
	if cursor, _ := w.log.Cursor(ctx, Consumer); cursor == 0 {
		t.Fatal("the cursor never moved")
	}
}

// A queued job whose run is going starts on the safety tick even when its
// run.started was never acted on.
func TestMissedRunStartedPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.varp(281, 10)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, 0)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	last, err := w.log.After(ctx, 0, 1000) // the tracker never looks at them
	if err != nil {
		t.Fatal(err)
	}
	if err := w.log.SetCursor(ctx, Consumer, last[len(last)-1].ID); err != nil {
		t.Fatal(err)
	}
	w.step()
	if got := w.get(j.ID); got.State != job.Queued {
		t.Fatalf("state = %s", got.State)
	}
	w.tracker.Step(ctx, true)
	if got := w.get(j.ID); got.State != job.Running || got.Progress == nil {
		t.Fatalf("after the safety tick = %+v", got)
	}
}

// applyMigration runs the up part of a migration file.
func applyMigration(ctx context.Context, t *testing.T, w *world, file string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", file))
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(raw), "-- +goose Down")
	if _, err := w.pool.Exec(ctx, up); err != nil {
		t.Fatalf("apply %s: %v", file, err)
	}
}

// A sale proof 0030 holds becomes a ge_sell job a listing reads: its goal is
// the sale's rule, not a placeholder.
func TestSaleProofsMigrateToValidJobsPostgres(t *testing.T) {
	ctx := context.Background()
	w := &world{t: t, pool: testPoolTo(ctx, t, 30)}
	if _, err := w.pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES ('acc_jobs', 'u1', 'Jobs', 'jobs', 'regular', NULL, 'enhanced');
		INSERT INTO sale_proofs (account_id, plan_id, task_key, run_uid, item_id, coins_at_least, coins_seen)
		VALUES ('acc_jobs', 'pln_abcdefgh', 'task:0#0', '0123456789abcdef', 1775, 12100, 13100);
	`); err != nil {
		t.Fatal(err)
	}
	applyMigration(ctx, t, w, "0031_jobs_events.sql")
	jobs := storejobs.New(w.pool)
	list, err := jobs.List(ctx, account, "", 50)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	want, _ := job.SaleGoal(1775, 12100)
	var a, b any
	if err := json.Unmarshal(list[0].Goal.JSON(), &a); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(want.JSON(), &b)
	if wa, _ := json.Marshal(a); !bytes.Equal(wa, mustJSON(b)) {
		t.Fatalf("goal = %s, want %s", wa, mustJSON(b))
	}
	if list[0].Kind != job.KindGESell || list[0].State != job.Succeeded {
		t.Fatalf("job = %+v", list[0])
	}
	if m, err := jobs.ProvenSales(ctx, account, "pln_abcdefgh"); err != nil || m["task:0#0"] != list[0].ID {
		t.Fatalf("proven = %v, %v", m, err)
	}
}

func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }

// fixedItems replaces the inventory of the facts the tracker reads.
type fixedItems struct {
	dbFacts
	items rules.ItemHoldings
}

func (f fixedItems) TaskFacts(ctx context.Context, acc string) (panel.Facts, error) {
	pf, err := f.dbFacts.TaskFacts(ctx, acc)
	if err == nil && pf.State != nil {
		pf.State.Items = f.items
	}
	return pf, err
}

// saleWorld queues a ge_sell of the glass with its plan task and an inventory
// the test sets.
func saleWorld(t *testing.T) (*world, job.Job, func(map[int]int64)) {
	t.Helper()
	w := setup(t)
	g, err := job.SaleGoal(1775, 1000)
	if err != nil {
		t.Fatal(err)
	}
	nj, _ := job.New(account, job.KindGESell, nil, g, nil, 0, job.SourceOwner)
	nj.Plan, nj.TaskKey = "pln_abcdefgh", "task:0#0"
	j, err := w.jobs.Create(context.Background(), nj)
	if err != nil {
		t.Fatal(err)
	}
	inv := func(q map[int]int64) {
		w.tracker.Facts = fixedItems{dbFacts{w}, rules.ItemHoldings{"inventory": rules.ContainerItems{State: coreingest.ItemLive, Complete: true, Quantities: q}}}
	}
	return w, j, inv
}

// A sale whose run failed or was canceled is a failed job, and what the
// inventory shows afterwards (the glass banked, coins withdrawn) proves nothing.
func TestSaleOfAFailedRunIsNotProvenLaterPostgres(t *testing.T) {
	for _, state := range []string{agent.RunFailed, agent.RunCanceled} {
		w, j, inv := saleWorld(t)
		inv(map[int]int64{1775: 100, 995: 10})
		w.run("0123abcd", agent.RunRunning, j.ID, nil)
		w.step()
		w.run("0123abcd", state, j.ID, nil)
		w.step()
		if got := w.get(j.ID); got.State != job.Failed || got.FinishedAt == nil {
			t.Fatalf("%s: after the run = %+v", state, got)
		}
		inv(map[int]int64{995: 5000})
		w.tracker.Step(context.Background(), true)
		if got := w.get(j.ID); got.State != job.Failed {
			t.Fatalf("%s: later = %+v", state, got)
		}
		if m, _ := w.jobs.ProvenSales(context.Background(), account, "pln_abcdefgh"); len(m) != 0 {
			t.Fatalf("%s: proven = %v", state, m)
		}
	}
}

// A sale whose run ended done before the coins showed is proven when they do,
// while that run is the job's last; with no proof for 13 hours the job fails
// and is not looked at again.
func TestSaleOfADoneRunWaitsForItsCoinsPostgres(t *testing.T) {
	w, j, inv := saleWorld(t)
	ctx := context.Background()
	inv(map[int]int64{1775: 100, 995: 10})
	w.run("0123abcd", agent.RunRunning, j.ID, nil)
	w.step()
	w.run("0123abcd", agent.RunDone, j.ID, nil)
	w.step()
	if got := w.get(j.ID); got.State != job.Stalled {
		t.Fatalf("after the run = %+v", got)
	}
	inv(map[int]int64{995: 1010})
	w.tracker.Step(ctx, true)
	if got := w.get(j.ID); got.State != job.Succeeded {
		t.Fatalf("coins arrived = %+v", got)
	}

	// Another sale whose coins never come.
	w2, j2, inv2 := saleWorld(t)
	inv2(map[int]int64{1775: 100, 995: 10})
	w2.run("0123abce", agent.RunRunning, j2.ID, nil)
	w2.step()
	w2.run("0123abce", agent.RunDone, j2.ID, nil)
	w2.step()
	w2.advance(12 * time.Hour)
	w2.tracker.Step(ctx, true)
	if got := w2.get(j2.ID); got.State != job.Stalled {
		t.Fatalf("12 hours later = %+v", got)
	}
	w2.advance(2 * time.Hour)
	w2.tracker.Step(ctx, true)
	if got := w2.get(j2.ID); got.State != job.Failed || !strings.Contains(got.Reason, "not proven") {
		t.Fatalf("14 hours later = %+v", got)
	}
	inv2(map[int]int64{995: 5000})
	w2.tracker.Step(ctx, true)
	if got := w2.get(j2.ID); got.State != job.Failed {
		t.Fatalf("a late fact revived it: %+v", got)
	}
}

// membership.low is written once when the membership crosses the threshold,
// not at every change below it nor on a redelivery.
func TestMembershipLowOnlyOnCrossingPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	lows := func() int {
		evs, _ := w.log.After(ctx, 0, 1000)
		n := 0
		for _, e := range evs {
			if e.Type == eventlog.TypeMembershipLow {
				n++
			}
		}
		return n
	}
	w.observe("membership", "membership", map[string]any{"days": 8, "member": true})
	w.step()
	w.observe("membership", "membership", map[string]any{"days": 7, "member": true})
	w.step()
	w.observe("membership", "membership", map[string]any{"days": 6, "member": true})
	w.step()
	w.observe("membership", "membership", map[string]any{"days": 5, "member": true})
	w.step()
	if n := lows(); n != 1 {
		t.Fatalf("membership.low written %d times", n)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE event_cursors SET last_id = 0 WHERE consumer = $1`, Consumer); err != nil {
		t.Fatal(err)
	}
	w.step()
	if n := lows(); n != 1 {
		t.Fatalf("membership.low written %d times after a redelivery", n)
	}
	// Renewed and low again, a month later.
	w.advance(30 * 24 * time.Hour)
	w.observe("membership", "membership", map[string]any{"days": 6, "member": true})
	w.step()
	if n := lows(); n != 2 {
		t.Fatalf("membership.low written %d times for the next period", n)
	}
}

// The run_until goes out beside the loop: a slow agent does not hold the pass,
// and the run is asked once.
func TestSlowAgentDoesNotHoldTheLoopPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.hub.agents[account] = agent.Info{ID: "c1", Account: account, Commands: []string{agent.CmdRunUntil}}
	w.hub.block = make(chan struct{})
	w.level("fishing", 76)
	w.run("00000000000000a1", agent.RunRunning, "", &agent.RunUntil{Objective: "skill:fishing>=76", Account: account})
	done := make(chan struct{})
	go func() {
		w.tracker.Step(ctx, false)
		w.tracker.Step(ctx, true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the pass waited for the agent")
	}
	close(w.hub.block)
	w.tracker.inflight.Wait()
	w.tracker.Step(ctx, true)
	w.tracker.inflight.Wait()
	if w.hub.calls != 1 || w.hub.count() != 1 {
		t.Fatalf("calls = %d, sent = %d", w.hub.calls, w.hub.count())
	}
}

// An agent that does not carry the job id in its reports still has its run
// linked to the job the server queued for the task, and no second job is
// adopted for the run.
func TestRunWithoutJobIDLinksTheQueuedJobPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.level("fishing", 70)
	g, _ := job.ParseGoal([]byte(`"skill:fishing>=76"`))
	nj, _ := job.New(account, job.KindTrain, nil, g, nil, 0, job.SourceOwner)
	nj.Plan, nj.TaskKey = "pln_abcdefgh", "task:0#0"
	j, err := w.jobs.Create(ctx, nj)
	if err != nil {
		t.Fatal(err)
	}
	r := agent.Run{
		ID: "run-1", UID: "00000000000000a1", ClassName: "a.b.Trainer", Stop: []string{}, State: agent.RunRunning, StartedAt: w.now(),
		Events: []agent.RunEvent{}, Until: &agent.RunUntil{Objective: "skill:fishing>=76", Account: account},
		Task: &agent.RunTask{Key: "task:0#0", Plan: "pln_abcdefgh", Type: "train", Label: "Pescar"},
	}
	if _, _, err = w.runs.RecordRun(ctx, account, "zaros", r); err != nil {
		t.Fatal(err)
	}
	w.step()
	jobs, err := w.jobs.Active(ctx, account)
	if err != nil || len(jobs) != 1 || jobs[0].ID != j.ID || jobs[0].State != job.Running {
		t.Fatalf("jobs = %+v, %v", jobs, err)
	}
}

// An agent that declares no account is taken at the run's word when the
// server routes that account to it: its run waiting for a key is adopted and
// proven, and a sale's run is recorded under its job's account. A run naming
// an account served by another agent is dropped.
func TestUndeclaredAgentRunIsTrackedPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	attached := agent.Info{ID: "c2", Commands: []string{agent.CmdRunUntil}}
	w.hub.agents[account] = attached
	sync := &routinesync.Sync{Store: w.runs, Hub: w.hub, Log: w.tracker.log()}
	w.level("fishing", 80)

	until := agent.Run{ID: "run-1", UID: "00000000000000b1", ClassName: "a.b.T", Stop: []string{}, State: agent.RunWaiting, StartedAt: w.now(),
		Events: []agent.RunEvent{}, Until: &agent.RunUntil{Objective: "skill:fishing>=76", Account: account}}
	sync.OnEvent(attached, agent.Event{Run: &until})
	w.step()
	if w.hub.count() != 1 || w.hub.sent[0].Evidence != "nível 80" {
		t.Fatalf("sent %+v", w.hub.sent)
	}

	g, _ := job.SaleGoal(1775, 1000)
	nj, _ := job.New(account, job.KindGESell, nil, g, nil, 0, job.SourceOwner)
	sale, err := w.jobs.Create(ctx, nj)
	if err != nil {
		t.Fatal(err)
	}
	saleRun := agent.Run{ID: "run-2", UID: "00000000000000b2", ClassName: "a.b.T", Stop: []string{}, State: agent.RunRunning, StartedAt: w.now(),
		Events: []agent.RunEvent{}, Task: &agent.RunTask{Key: "task:0#0", Plan: "pln_abcdefgh", Type: "ge_sell", Label: "Vender", JobID: sale.ID}}
	sync.OnEvent(attached, agent.Event{Run: &saleRun})
	w.step()
	if got := w.get(sale.ID); got.State != job.Running {
		t.Fatalf("the sale's job = %+v", got)
	}

	// Another agent serves the account: this one's report is not taken.
	w.hub.agents[account] = agent.Info{ID: "c9", Account: account, Commands: []string{agent.CmdRunUntil}}
	foreign := until
	foreign.UID = "00000000000000b3"
	sync.OnEvent(attached, agent.Event{Run: &foreign})
	if runs, _ := w.runs.Runs(ctx, account, 10); len(runs) != 2 {
		t.Fatalf("runs = %d", len(runs))
	}
}

// fact.changed events older than a week that the cursor passed are pruned by
// the safety tick; the ones it did not pass, and the rest of the log, stay.
func TestFactChangedIsPrunedPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.varp(281, 10)
	w.step()
	w.varp(281, 11) // after the cursor
	if n, err := w.log.PruneFactChanged(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("pruned %d, %v: the passed event is the only one to go", n, err)
	}
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, 0)
	w.step()
	facts := func() int {
		evs, _ := w.log.After(ctx, 0, 1000)
		n := 0
		for _, e := range evs {
			if e.Type == eventlog.TypeFactChanged {
				n++
			}
		}
		return n
	}
	if facts() != 1 {
		t.Fatalf("fact.changed events = %d", facts())
	}
	w.advance(8 * 24 * time.Hour)
	w.tracker.Step(ctx, true)
	if facts() != 0 || len(w.types(j.ID)) == 0 {
		t.Fatalf("after a week: fact.changed %d, the job's events %v", facts(), w.types(j.ID))
	}
}

// An unreadable job (here a rule type the code no longer has) is skipped: the
// cursor moves on and the account's other jobs are still proven.
func TestUnreadableJobDoesNotStopTheTrackerPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.level("fishing", 70)
	j := w.create(job.KindTrain, `"skill:fishing>=76"`, 0)
	if _, err := w.pool.Exec(ctx, `INSERT INTO jobs (id, account_id, kind, goal, stall_after_s, source)
		VALUES ('job_00000000000000bb', $1, 'plugin', '{"type": "removed_rule"}', 1800, 'test')`, account); err != nil {
		t.Fatal(err)
	}
	_ = w.run("0123abce", agent.RunRunning, j.ID, nil)
	w.level("fishing", 76)
	w.step()
	if got := w.get(j.ID).State; got != job.Succeeded {
		t.Fatalf("train job = %s, want succeeded", got)
	}
	if cur, err := w.log.Cursor(ctx, Consumer); err != nil || cur == 0 {
		t.Fatalf("cursor = %d, %v: the tracker stopped on the unreadable job", cur, err)
	}
}
