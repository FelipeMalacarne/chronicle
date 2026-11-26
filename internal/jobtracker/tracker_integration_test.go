package jobtracker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
	"github.com/FelipeMalacarne/chronicle/internal/store/state"
)

const account = "acc_jobs"

// world is a throwaway database with the tracker's stores around it, a clock
// the test moves and the agents' link faked.
type world struct {
	t        *testing.T
	pool     *pgxpool.Pool
	q        *storedb.Queries
	jobs     *storejobs.Repository
	log      *eventlog.Repository
	runs     *storeroutines.Repository
	tracker  *Tracker
	hub      *fakeHub
	audit    *fakeAudit
	reg      *entities.Registry
	mu       sync.Mutex
	clock    time.Time
	snapshot int
}

func setup(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_jobs', 'u1', 'Jobs', 'jobs', 'regular', NULL, 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{
		t: t, pool: pool, q: storedb.New(pool), reg: reg, hub: &fakeHub{agents: map[string]agent.Info{}}, audit: &fakeAudit{},
		clock: time.Now().UTC().Truncate(time.Second),
	}
	w.jobs = storejobs.New(pool)
	w.jobs.Now = w.now
	w.log = eventlog.New(pool)
	w.runs = storeroutines.New(pool)
	w.tracker = &Tracker{Jobs: w.jobs, Log: w.log, Facts: dbFacts{w}, Registry: reg, Hub: w.hub, Audit: w.audit, Now: w.now}
	return w
}

func (w *world) now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.clock
}

func (w *world) advance(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clock = w.clock.Add(d)
}

// observe is what an ingest push does to one fact: the observation, its
// projection into player_state and the fact.changed event, in one transaction.
func (w *world) observe(facet, key string, value any) {
	w.t.Helper()
	ctx := context.Background()
	raw, _ := json.Marshal(value)
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		w.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := w.q.WithTx(tx)
	w.snapshot++
	id := fmt.Sprintf("snap_%d", w.snapshot)
	if _, err := tx.Exec(ctx, `INSERT INTO snapshots (id, account_id, source, fetched_at, raw, raw_hash, adapter_version) VALUES ($1, $2, 'plugin', now(), '{}', $1, 1)`, id, account); err != nil {
		w.t.Fatal(err)
	}
	if err := q.UpsertObservation(ctx, storedb.UpsertObservationParams{
		AccountID: account, Facet: facet, Key: key, Value: raw, Source: "plugin", SnapshotID: id,
		ObservedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}); err != nil {
		w.t.Fatal(err)
	}
	if err := state.Project(ctx, q, account, facet); err != nil {
		w.t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) varp(id int, v int64) {
	w.t.Helper()
	w.observe("varp", fmt.Sprint(id), map[string]int64{"value": v})
}

func (w *world) level(skill string, level int) {
	w.t.Helper()
	w.observe("skill", skill, map[string]int64{"level": int64(level), "xp": int64(entities.XPForLevel(level))})
}

// create queues a job of the account.
func (w *world) create(kind job.Kind, goal string, stall time.Duration) job.Job {
	w.t.Helper()
	g, err := job.ParseGoal([]byte(goal))
	if err != nil {
		w.t.Fatal(err)
	}
	j, err := job.New(account, kind, nil, g, nil, stall, job.SourceTest)
	if err != nil {
		w.t.Fatal(err)
	}
	created, err := w.jobs.Create(context.Background(), j)
	if err != nil {
		w.t.Fatal(err)
	}
	return created
}

// run is what the agent reports of a run, stored as the server does.
func (w *world) run(uid, state, jobID string, until *agent.RunUntil) agent.Run {
	w.t.Helper()
	r := agent.Run{ID: "run-1", UID: uid, ClassName: "a.b.Trainer", Stop: []string{}, State: state, StartedAt: w.now(), Events: []agent.RunEvent{}, Until: until}
	if jobID != "" {
		r.Task = &agent.RunTask{Key: "task:0#0", Plan: "pln_abcdefgh", Type: "travel", Label: "Ir", JobID: jobID}
	}
	if r.Final() {
		end := w.now()
		r.FinishedAt = &end
	}
	if _, _, err := w.runs.RecordRun(context.Background(), account, "zaros", r); err != nil {
		w.t.Fatal(err)
	}
	return r
}

func (w *world) step() {
	w.t.Helper()
	w.tracker.Step(context.Background(), false)
	w.tracker.inflight.Wait()
}

func (w *world) get(id string) job.Job {
	w.t.Helper()
	j, err := w.jobs.Get(context.Background(), account, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return j
}

// types lists the events of a job (its own and its runs'), oldest first.
func (w *world) types(id string) []string {
	w.t.Helper()
	evs, err := w.log.OfJob(context.Background(), account, id, 100)
	if err != nil {
		w.t.Fatal(err)
	}
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

// changed reports whether a fact.changed event lists the key of the facet.
func changed(e eventlog.Event, facet, key string) bool {
	var p struct {
		Facts map[string][]string `json:"facts"`
	}
	return json.Unmarshal(e.Payload, &p) == nil && slices.Contains(p.Facts[facet], key)
}

func count(types []string, want string) int {
	n := 0
	for _, t := range types {
		if t == want {
			n++
		}
	}
	return n
}

// A job made by a test goes queued, running and succeeded from the ingest's
// facts, records its progress and writes one event per transition.
func TestJobLifecyclePostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.varp(281, 10)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, 0)
	if j.State != job.Queued || j.Metric == nil || j.Metric.Target != 1000 {
		t.Fatalf("created = %+v", j)
	}
	w.step()
	if w.get(j.ID).State != job.Queued {
		t.Fatal("a job nobody attempts left the queue")
	}

	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	w.step()
	got := w.get(j.ID)
	if got.State != job.Running || got.StartedAt == nil || got.Progress == nil || got.Progress.From != 10 || got.Progress.Current != 10 || got.Progress.Target != 1000 || got.Deadline == nil {
		t.Fatalf("running = %+v progress %+v", got, got.Progress)
	}

	w.advance(10 * time.Second)
	w.varp(281, 500)
	w.step()
	got = w.get(j.ID)
	if got.Progress.Current != 500 || got.Progress.From != 10 || !got.Progress.At.Equal(w.now()) || !got.Deadline.Equal(w.now().Add(got.StallAfter)) {
		t.Fatalf("progress = %+v deadline %v", got.Progress, got.Deadline)
	}

	w.varp(281, 1000)
	w.step()
	got = w.get(j.ID)
	if got.State != job.Succeeded || got.FinishedAt == nil || got.Deadline != nil || !strings.HasPrefix(got.Reason, "goal met") {
		t.Fatalf("succeeded = %+v", got)
	}
	types := w.types(j.ID)
	want := []string{"job.queued", "run.started", "job.started", "job.progressed", "job.succeeded"}
	if !slices.Equal(types, want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
	// One run, linked to the job.
	runs, err := w.runs.RunsOfJob(ctx, account, j.ID)
	if err != nil || len(runs) != 1 || runs[0].Task.JobID != j.ID {
		t.Fatalf("runs = %+v, %v", runs, err)
	}

	// Final: nothing moves it, and a success needs a proof.
	if _, err := w.jobs.Stall(ctx, j.ID, "late"); err == nil {
		t.Fatal("a succeeded job stalled")
	}
	if _, err := w.jobs.Succeed(ctx, j.ID, job.Proof{}, nil); err == nil {
		t.Fatal("a job succeeded without a proof")
	}
	if _, err := w.jobs.Cancel(ctx, account, j.ID, "x"); err == nil {
		t.Fatal("a succeeded job was canceled")
	}
	if _, err := w.jobs.Get(ctx, "acc_other", j.ID); err == nil {
		t.Fatal("another account's job was read")
	}
}

// A running job with a metric stalls when the metric does not change for
// stall_after, by the deadline, with no fact to wake it; a changing metric
// keeps it going; a new run takes it back to running.
func TestStallByDeadlinePostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.varp(281, 10)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, time.Minute)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	w.step()

	w.advance(40 * time.Second)
	w.varp(281, 20) // progress: the deadline moves
	w.step()
	w.advance(40 * time.Second)
	w.step()
	if got := w.get(j.ID); got.State != job.Running {
		t.Fatalf("stalled with a changing metric: %+v", got)
	}
	if at, ok, err := w.jobs.NextDeadline(ctx); err != nil || !ok || !at.Equal(w.now().Add(20*time.Second)) {
		t.Fatalf("next deadline = %v %v %v", at, ok, err)
	}

	w.advance(21 * time.Second) // a minute since the last change
	w.step()
	got := w.get(j.ID)
	if got.State != job.Stalled || got.Deadline != nil || !strings.Contains(got.Reason, "no progress") {
		t.Fatalf("after the deadline = %+v", got)
	}
	if n := count(w.types(j.ID), "job.stalled"); n != 1 {
		t.Fatalf("job.stalled written %d times", n)
	}
	w.step() // nothing more to say
	if n := count(w.types(j.ID), "job.stalled"); n != 1 {
		t.Fatalf("job.stalled written again: %v", w.types(j.ID))
	}

	// Another run continues the same job.
	w.run("aa11bb22cc33dd45", agent.RunRunning, j.ID, nil)
	w.step()
	if got = w.get(j.ID); got.State != job.Running || got.Deadline == nil {
		t.Fatalf("second run = %+v", got)
	}
	w.varp(281, 1000)
	w.step()
	if got = w.get(j.ID); got.State != job.Succeeded {
		t.Fatalf("after the goal = %+v", got)
	}
	runs, _ := w.runs.RunsOfJob(ctx, account, j.ID)
	if len(runs) != 2 {
		t.Fatalf("the job's runs = %d", len(runs))
	}
}

// A job stalled by its deadline whose run still goes is running again at the
// metric's next change (FEL-133); once the run ended, a change leaves it
// stalled, waiting for another run.
func TestStalledJobResumesWhileItsRunGoesPostgres(t *testing.T) {
	w := setup(t)
	w.varp(281, 3)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, time.Minute)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	w.step()
	w.advance(61 * time.Second)
	w.step()
	if got := w.get(j.ID); got.State != job.Stalled {
		t.Fatalf("after the deadline = %+v", got)
	}

	w.varp(281, 650)
	w.step()
	got := w.get(j.ID)
	if got.State != job.Running || got.Deadline == nil || got.Progress == nil || got.Progress.Current != 650 {
		t.Fatalf("after progress with the run going = %+v", got)
	}
	if n := count(w.types(j.ID), "job.started"); n != 2 {
		t.Fatalf("job.started written %d times: %v", n, w.types(j.ID))
	}

	w.advance(61 * time.Second)
	w.step()
	w.run("aa11bb22cc33dd44", agent.RunFailed, j.ID, nil)
	w.step()
	if got = w.get(j.ID); got.State != job.Stalled {
		t.Fatalf("after the run failed = %+v", got)
	}
	w.varp(281, 700)
	w.step()
	if got = w.get(j.ID); got.State != job.Stalled || got.Progress.Current != 650 {
		t.Fatalf("a stalled job without a run moved = %+v", got)
	}
}

// A run that ends before the goal is on the facts stalls the job; the fact
// that proves it can still come after.
func TestRunEndedBeforeTheProofPostgres(t *testing.T) {
	w := setup(t)
	w.varp(281, 10)
	j := w.create(job.KindTravel, `{"type":"varp","id":281,"min":1000}`, 0)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	w.step()
	w.advance(time.Second)
	w.run("aa11bb22cc33dd44", agent.RunDone, j.ID, nil)
	w.step()
	if got := w.get(j.ID); got.State != job.Stalled || got.Reason != "run ended: done" {
		t.Fatalf("after the run = %+v", got)
	}
	w.varp(281, 1500)
	w.step()
	if got := w.get(j.ID); got.State != job.Succeeded {
		t.Fatalf("after the late proof = %+v", got)
	}
	if types := w.types(j.ID); !slices.Equal(types, []string{"job.queued", "run.started", "job.started", "run.ended", "job.stalled", "job.succeeded"}) {
		t.Fatalf("events = %v", types)
	}
}

// A run that the facts already prove at its start ends the job at once.
func TestProvenAtOncePostgres(t *testing.T) {
	w := setup(t)
	w.level("fishing", 80)
	j := w.create(job.KindTrain, `"skill:fishing>=76"`, 0)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, &agent.RunUntil{Objective: "skill:fishing>=76", Account: account})
	w.step()
	if got := w.get(j.ID); got.State != job.Succeeded {
		t.Fatalf("job = %+v", got)
	}
}

// The tracker resumes from its cursor after a restart and handles a
// redelivered event without changing anything.
func TestCursorAndRedeliveryPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.varp(281, 10)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, 0)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	w.varp(281, 400)
	w.step()
	before := w.types(j.ID)
	cursor, err := w.log.Cursor(ctx, Consumer)
	if err != nil || cursor == 0 {
		t.Fatalf("cursor = %d, %v", cursor, err)
	}

	// A new tracker (a restart) has nothing left to read.
	again := &Tracker{Jobs: w.jobs, Log: w.log, Facts: dbFacts{w}, Registry: w.reg, Hub: w.hub, Now: w.now}
	again.Step(ctx, false)
	if after := w.types(j.ID); !slices.Equal(before, after) {
		t.Fatalf("a restart changed the job's events: %v -> %v", before, after)
	}

	// An event delivered again (a crash between the effect and the cursor).
	if _, err := w.pool.Exec(ctx, `UPDATE event_cursors SET last_id = 0 WHERE consumer = $1`, Consumer); err != nil {
		t.Fatal(err)
	}
	again.Step(ctx, false)
	if after := w.types(j.ID); !slices.Equal(before, after) {
		t.Fatalf("redelivery changed the job's events: %v -> %v", before, after)
	}
	if got := w.get(j.ID); got.State != job.Running || got.Progress.Current != 400 {
		t.Fatalf("after redelivery = %+v", got)
	}
	if now, _ := w.log.Cursor(ctx, Consumer); now < cursor {
		t.Fatalf("the cursor moved back: %d -> %d", cursor, now)
	}
}

// fact.changed is written by the projection only when a fact changed, and
// never for the position and the timers.
func TestFactChangedPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	facts := func() []eventlog.Event {
		evs, err := w.log.After(ctx, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		var out []eventlog.Event
		for _, e := range evs {
			if e.Type == eventlog.TypeFactChanged {
				out = append(out, e)
			}
		}
		return out
	}
	w.varp(281, 10)
	w.varp(281, 10) // the same value: no change
	if got := facts(); len(got) != 1 || !changed(got[0], "varp", "281") {
		t.Fatalf("after the same value twice = %+v", got)
	}
	w.varp(281, 11)
	w.level("mining", 5)
	if got := facts(); len(got) != 3 || !changed(got[2], "skill", "mining") {
		t.Fatalf("after two more = %+v", got)
	}
}

// The tracker tells the agent to stop a run whose job it proved, once; an
// agent that is away is asked again; one that refuses is not.
func TestRunUntilToTheAgentPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.hub.agents[account] = agent.Info{ID: "c1", Account: account, Commands: []string{agent.CmdRunUntil}}
	w.level("fishing", 75)
	until := &agent.RunUntil{Objective: "skill:fishing>=76", Account: account}
	// A run that waits for an objective is adopted as a job.
	w.run("00000000000000a1", agent.RunRunning, "", until)
	w.step()
	jobs, err := w.jobs.Active(ctx, account)
	if err != nil || len(jobs) != 1 || jobs[0].State != job.Running || jobs[0].Source != job.SourceOwner || jobs[0].Kind != job.KindPlugin {
		t.Fatalf("adopted = %+v, %v", jobs, err)
	}
	if w.hub.count() != 0 {
		t.Fatal("proven at level 75")
	}

	w.level("fishing", 76)
	w.step()
	if got := w.get(jobs[0].ID); got.State != job.Succeeded {
		t.Fatalf("job = %+v", got)
	}
	if w.hub.count() != 1 {
		t.Fatalf("sent %d", w.hub.count())
	}
	if p := w.hub.sent[0]; p.UID != "00000000000000a1" || p.Objective != "skill:fishing>=76" || p.Evidence != "nível 76" {
		t.Fatalf("sent %+v", p)
	}
	if e := w.audit.entries[0]; e.Actor != Actor || e.Origin != "server" || e.Action != agent.CmdRunUntil || !e.OK || e.AccountID != account {
		t.Fatalf("audit %+v", e)
	}
	// A report in between does not tell it again; the run's end clears it.
	w.run("00000000000000a1", agent.RunRunning, "", until)
	w.step()
	w.tracker.Step(ctx, true)
	w.tracker.inflight.Wait()
	if w.hub.count() != 1 {
		t.Fatalf("sent again: %d", w.hub.count())
	}
	w.run("00000000000000a1", agent.RunDone, "", until)
	w.step()
	if len(w.tracker.sent) != 0 || w.hub.count() != 1 {
		t.Fatalf("after the end: sent %v, calls %d", w.tracker.sent, w.hub.count())
	}
}

// The Tutorial Island job (docs/12 section 13.7): a plugin job on varp 281,
// whose run the agent reports with the job's id and the var label it waits
// for. The job goes queued, running, progress on the varp, then succeeded at
// 1000, and the agent is told to stop the run once, by that label.
func TestPluginJobOnVarpPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	w.hub.agents[account] = agent.Info{ID: "c1", Account: account, Commands: []string{agent.CmdRunUntil}}
	w.varp(281, 20)
	spec := json.RawMessage(`{"type":"plugin","plugin":{"plugin":"` + clientcfg.TutorialIslandClass + `","name":"Tutorial Island","until":"varp:281>=1000"}}`)
	g, err := job.ParseGoal([]byte(`{"type":"varp","id":281,"min":1000}`))
	if err != nil {
		t.Fatal(err)
	}
	j, err := job.New(account, job.KindPlugin, spec, g, nil, 0, job.SourceOwner)
	if err != nil {
		t.Fatal(err)
	}
	if j, err = w.jobs.Create(ctx, j); err != nil {
		t.Fatal(err)
	}

	report := func(state string) {
		r := agent.Run{
			ID: "run-1", UID: "aa11bb22cc33dd44", ClassName: clientcfg.TutorialIslandClass, Stop: []string{}, State: state,
			StartedAt: w.now(), Events: []agent.RunEvent{}, Until: &agent.RunUntil{Objective: "varp:281>=1000", Account: account},
			Task: &agent.RunTask{Type: "plugin", Label: "Tutorial Island", JobID: j.ID},
		}
		if r.Final() {
			end := w.now()
			r.FinishedAt = &end
		}
		if _, _, err := w.runs.RecordRun(ctx, account, "zaros", r); err != nil {
			t.Fatal(err)
		}
		w.step()
	}
	report(agent.RunRunning)
	got := w.get(j.ID)
	if got.State != job.Running || got.Progress == nil || got.Progress.From != 20 || got.Progress.Target != 1000 {
		t.Fatalf("running = %+v progress %+v", got, got.Progress)
	}

	w.advance(time.Minute)
	w.varp(281, 640)
	w.step()
	if got = w.get(j.ID); got.State != job.Running || got.Progress.Current != 640 {
		t.Fatalf("progress = %+v, state %s", got.Progress, got.State)
	}
	if w.hub.count() != 0 {
		t.Fatal("told the agent to stop before the proof")
	}

	w.varp(281, 1000)
	w.step()
	if got = w.get(j.ID); got.State != job.Succeeded || got.Progress.Current != 1000 {
		t.Fatalf("job = %+v progress %+v", got, got.Progress)
	}
	if w.hub.count() != 1 {
		t.Fatalf("run_until sent %d times", w.hub.count())
	}
	if p := w.hub.sent[0]; p.UID != "aa11bb22cc33dd44" || p.Objective != "varp:281>=1000" || p.Evidence != `observado {"value":1000}` {
		t.Fatalf("sent %+v", p)
	}
	report(agent.RunRunning)
	w.tracker.Step(ctx, true)
	w.tracker.inflight.Wait()
	if w.hub.count() != 1 {
		t.Fatalf("run_until sent again: %d", w.hub.count())
	}
	if want := []string{"job.queued", "run.started", "job.started", "job.progressed", "job.succeeded"}; !slices.Equal(w.types(j.ID), want) {
		t.Fatalf("events = %v, want %v", w.types(j.ID), want)
	}
}

func TestRunUntilAgentAwayOrRefusingPostgres(t *testing.T) {
	w := setup(t)
	w.level("fishing", 76)
	until := &agent.RunUntil{Objective: "skill:fishing>=76", Account: account}
	// No agent serves the account: the run waits for it.
	w.run("00000000000000b1", agent.RunRunning, "", until)
	w.step()
	if w.hub.count() != 0 {
		t.Fatal("sent with no agent")
	}
	w.hub.agents[account] = agent.Info{ID: "c1", Account: account, Commands: []string{agent.CmdRunUntil}}
	w.step()
	if w.hub.count() != 1 {
		t.Fatalf("once the agent is back, sent %d", w.hub.count())
	}

	// An agent that has no such run waiting is not asked again.
	w.hub.refuse = true
	w.run("00000000000000b2", agent.RunRunning, "", &agent.RunUntil{Objective: "skill:fishing>=70", Account: account})
	w.step()
	w.step()
	if w.hub.calls != 2 {
		t.Fatalf("calls = %d, want the refused one once", w.hub.calls)
	}
}

// A run with neither a job nor an objective has no job.
func TestPlainRunHasNoJobPostgres(t *testing.T) {
	w := setup(t)
	w.run("00000000000000c1", agent.RunRunning, "", nil)
	w.step()
	if jobs, err := w.jobs.Active(context.Background(), account); err != nil || len(jobs) != 0 {
		t.Fatalf("jobs = %+v, %v", jobs, err)
	}
	// A job id the account does not have links nothing.
	w.run("00000000000000c2", agent.RunRunning, "job_0000000000000000", nil)
	w.step()
	if jobs, _ := w.jobs.Active(context.Background(), account); len(jobs) != 0 {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestMembershipLowPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	lows := func() []string {
		evs, _ := w.log.After(ctx, 0, 100)
		var out []string
		for _, e := range evs {
			if e.Type == eventlog.TypeMembershipLow {
				out = append(out, string(e.Payload))
			}
		}
		return out
	}
	w.observe("membership", "membership", map[string]any{"days": 30, "member": true})
	w.step()
	if len(lows()) != 0 {
		t.Fatalf("30 days is not low: %v", lows())
	}
	w.observe("membership", "membership", map[string]any{"days": 5, "member": true})
	w.step()
	if got := lows(); len(got) != 1 || !strings.Contains(got[0], `5`) {
		t.Fatalf("5 days = %v", got)
	}
	w.observe("membership", "membership", map[string]any{"days": 0, "member": false})
	w.step()
	if len(lows()) != 1 {
		t.Fatalf("a lapsed membership is not low: %v", lows())
	}
}

// A sale is proven by its job: the newest job of a task that was not
// canceled, when it succeeded.
func TestProvenTasksPostgres(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	sale := func() job.Job {
		g, err := job.SaleGoal(1775, 1000)
		if err != nil {
			t.Fatal(err)
		}
		j, err := job.New(account, job.KindGESell, nil, g, nil, 0, job.SourceOwner)
		if err != nil {
			t.Fatal(err)
		}
		j.Plan, j.TaskKey = "pln_abcdefgh", "task:0#0"
		created, err := w.jobs.Create(ctx, j)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	proven := func() map[string]string {
		m, err := w.jobs.ProvenTasks(ctx, account, "pln_abcdefgh")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	first := sale()
	if len(proven()) != 0 {
		t.Fatalf("a queued sale is proven: %v", proven())
	}
	// Forced to succeeded by SQL: only the tracker's proof does it in code.
	if _, err := w.pool.Exec(ctx, `UPDATE jobs SET state = 'succeeded', finished_at = now() WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if got := proven(); got["task:0#0"] != first.ID {
		t.Fatalf("proven = %v", got)
	}
	second := sale() // a new run of the task: the first proof no longer speaks for it
	if len(proven()) != 0 {
		t.Fatalf("an earlier proof counts for the new sale: %v", proven())
	}
	if _, err := w.jobs.Cancel(ctx, account, second.ID, "the run did not start"); err != nil {
		t.Fatal(err)
	}
	if got := proven(); got["task:0#0"] != first.ID {
		t.Fatalf("a canceled attempt hid the proof: %v", got)
	}
}

// Run wakes on the hub and proves a job without a Step from the test.
func TestRunWakesOnTheHubPostgres(t *testing.T) {
	w := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := events.NewHub(nil)
	w.tracker.Events, w.tracker.Interval = hub, time.Hour
	w.tracker.Now = nil // the real clock
	w.varp(281, 10)
	j := w.create(job.KindPlugin, `{"type":"varp","id":281,"min":1000}`, 0)
	w.run("aa11bb22cc33dd44", agent.RunRunning, j.ID, nil)
	go w.tracker.Run(ctx)
	waitFor(t, "running", func() bool { return w.get(j.ID).State == job.Running })
	w.varp(281, 1000)
	hub.Publish(events.Event{Type: events.TypeLog, Account: account})
	waitFor(t, "succeeded", func() bool { return w.get(j.ID).State == job.Succeeded })
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
	}
}

// dbFacts reads the facts as the progress repository does.
type dbFacts struct{ w *world }

func (f dbFacts) TaskFacts(ctx context.Context, acc string) (panel.Facts, error) {
	ps, _, err := state.Load(ctx, f.w.q, acc, f.w.reg)
	if err != nil {
		return panel.Facts{}, err
	}
	return panel.Facts{Mode: "regular", State: &ps}, nil
}

func (f dbFacts) Membership(ctx context.Context, acc string) (int, bool, bool, error) {
	rows, err := f.w.q.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: acc, Facet: "membership"})
	if err != nil || len(rows) == 0 {
		return 0, false, false, err
	}
	var v struct {
		Days   int  `json:"days"`
		Member bool `json:"member"`
	}
	err = json.Unmarshal(rows[0].Value, &v)
	return v.Days, v.Member, err == nil, err
}

// fakeHub is the agents' link: one agent per account and what was sent.
type fakeHub struct {
	mu     sync.Mutex
	agents map[string]agent.Info
	sent   []agent.RunUntilParams
	calls  int
	refuse bool
	// block, when set, holds every call until it closes.
	block chan struct{}
}

func (h *fakeHub) Route(acc string) (agent.Info, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a, ok := h.agents[acc]; ok {
		return a, nil
	}
	return agent.Info{}, agent.ErrNoAgent
}

func (h *fakeHub) CallAgent(_ context.Context, _, command string, params, _ any) error {
	h.mu.Lock()
	block := h.block
	h.calls++
	h.mu.Unlock()
	if block != nil {
		<-block
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if command != agent.CmdRunUntil {
		return agent.ErrUnknownCommand
	}
	if h.refuse {
		return &agent.CommandError{Command: command, Message: "run_until: no such run"}
	}
	h.sent = append(h.sent, params.(agent.RunUntilParams))
	return nil
}

func (h *fakeHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.sent)
}

type fakeAudit struct{ entries []storemicrobot.Entry }

func (a *fakeAudit) Append(_ context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error) {
	a.entries = append(a.entries, e)
	return e, nil
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testPoolTo(ctx, t, 0)
}

// testPoolTo applies the migrations up to number upTo (0 is all of them).
func testPoolTo(ctx context.Context, t *testing.T, upTo int) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL job tracker integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("jobs_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
		if n, _ := strconv.Atoi(filepath.Base(f)[:4]); upTo > 0 && n > upTo {
			break
		}
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
