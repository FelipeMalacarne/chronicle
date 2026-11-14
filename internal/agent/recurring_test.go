package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// clockBridge plays the bridge's /bridge/timers: the Time Tracking clocks.
type clockBridge struct {
	mu      sync.Mutex
	enabled bool
	timers  map[string]Timer
}

func newClocks() *clockBridge { return &clockBridge{enabled: true, timers: map[string]Timer{}} }

// set puts one clock; readyIn is from now (0: no readyAt).
func (b *clockBridge) set(key, state string, readyIn time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := Timer{Key: key, Label: map[string]string{"HERB": "Herb Patches", "BIRD_HOUSE": "Bird Houses"}[key], State: state}
	if readyIn != 0 {
		at := time.Now().Add(readyIn).Unix()
		t.ReadyAt = &at
	}
	b.timers[key] = t
}

func (b *clockBridge) JSON(_ context.Context, _, path string, _ url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	if path != "/bridge/timers" {
		return nil, fmt.Errorf("unexpected %s", path)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := Timers{Now: time.Now().Unix(), Enabled: b.enabled, Timers: []Timer{}}
	if b.enabled {
		for _, t := range b.timers {
			out.Timers = append(out.Timers, t)
		}
	}
	return json.Marshal(out)
}

func routine(id, name, clock string, interval, priority int) Recurring {
	saved := time.Now().Add(-2 * time.Hour).UTC()
	return Recurring{
		ID: id, Name: name, Enabled: true, Clock: clock, IntervalMin: interval, Priority: priority,
		Run: RunParams{ClassName: catSwap, Stop: []string{fisher}}, UpdatedAt: &saved,
	}
}

const (
	herbID  = "rtn_000000000001"
	birdID  = "rtn_000000000002"
	treeID  = "rtn_000000000003"
	hourAgo = -time.Hour
)

func syncRecurring(t *testing.T, e *Executor, list ...Recurring) {
	t.Helper()
	raw, _ := json.Marshal(RoutinesParams{Triggers: []Trigger{}, Recurring: list})
	if _, err := e.Execute(context.Background(), CmdRoutinesSync, raw); err != nil {
		t.Fatal(err)
	}
}

func recurring(t *testing.T, e *Executor) RecurringStatus {
	t.Helper()
	out, err := e.Execute(context.Background(), CmdRecurringGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out.(RecurringStatus)
}

func stateOf(t *testing.T, e *Executor, id string) RecurringState {
	t.Helper()
	for _, s := range recurring(t, e).Recurring {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no routine %s", id)
	return RecurringState{}
}

func currentRun(e *Executor) *Run {
	out, _ := e.Execute(context.Background(), CmdRunStatus, nil)
	return out.(RunStatus).Run
}

// The clock fires the routine when it is ready, once: the run carries the
// routine and says why, and a clock that still reads ready after the run
// fires nothing until it was seen growing again.
func TestRecurringFiresWhenItsClockIsReady(t *testing.T) {
	ctx := context.Background()
	clocks := newClocks()
	e := &Executor{Game: newRunGame("done"), Bridge: clocks, RunPoll: time.Millisecond}
	syncRecurring(t, e, routine(herbID, "Herb run", "HERB", 0, 0))

	clocks.set("HERB", "IN_PROGRESS", 20*time.Minute)
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonClockWaiting || s.NextAt == nil || time.Until(*s.NextAt) < 19*time.Minute {
		t.Fatalf("growing = %+v", s)
	}
	if currentRun(e) != nil {
		t.Fatal("a growing clock started a run")
	}

	clocks.set("HERB", "COMPLETED", 0)
	e.recurringOnce(ctx)
	r := waitRun(t, e)
	if r.State != RunDone || r.Routine != herbID || r.Trigger != "" || !strings.Contains(r.Events[0].Text, "relógio Herb Patches pronto") {
		t.Fatalf("run = %+v", r)
	}
	if l := recurring(t, e).Last; l == nil || l.RunID != r.ID || l.Routine != herbID || l.Skipped != "" {
		t.Fatalf("last = %+v", l)
	}

	// Still ready after the run (it did not replant): nothing until it changes.
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonClockStale || currentRun(e).ID != r.ID {
		t.Fatalf("stale clock = %+v, run %s", s, currentRun(e).ID)
	}
	clocks.set("HERB", "IN_PROGRESS", time.Hour)
	e.recurringOnce(ctx)
	clocks.set("HERB", "COMPLETED", 0)
	e.recurringOnce(ctx)
	if r2 := waitRun(t, e); r2.ID == r.ID || r2.Routine != herbID {
		t.Fatalf("the clock ready again did not fire: %+v", r2)
	}
	// A clock past its readyAt counts as ready too.
	if !clockReady(t, Timer{Key: "HERB", State: "IN_PROGRESS"}, -time.Minute) || clockReady(t, Timer{Key: "HERB", State: "OCCUPIED"}, 0) {
		t.Fatal("readiness of IN_PROGRESS past readyAt or OCCUPIED")
	}
}

func clockReady(t *testing.T, c Timer, readyIn time.Duration) bool {
	t.Helper()
	now := time.Now()
	if readyIn != 0 {
		at := now.Add(readyIn).Unix()
		c.ReadyAt = &at
	}
	st, _, _ := assess(routine(herbID, "Herb run", "HERB", 0, 0), recurringLook{now: now, clocks: map[string]Timer{"HERB": c}, clockNow: now.Unix()}, true)
	return st.Due
}

// Off, an unknown clock without an interval, the Time Tracking off: nothing
// starts.
func TestRecurringNotDue(t *testing.T) {
	ctx := context.Background()
	clocks := newClocks()
	e := &Executor{Game: newRunGame("done"), Bridge: clocks, RunPoll: time.Millisecond}
	off := routine(herbID, "Herb run", "HERB", 0, 0)
	off.Enabled = false
	syncRecurring(t, e, off, routine(birdID, "Birdhouses", "BIRD_HOUSE", 0, 0))
	clocks.set("HERB", "COMPLETED", 0)
	clocks.set("BIRD_HOUSE", "UNKNOWN", 0)
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonOff {
		t.Fatalf("off = %+v", s)
	}
	if s := stateOf(t, e, birdID); s.Due || s.Reason != ReasonNoClock {
		t.Fatalf("unknown clock = %+v", s)
	}
	clocks.mu.Lock()
	clocks.enabled = false
	clocks.mu.Unlock()
	clocks.set("BIRD_HOUSE", "COMPLETED", 0)
	e.recurringOnce(ctx)
	if st := recurring(t, e); !strings.Contains(st.ClockError, "Time Tracking") || st.Recurring[1].Reason != ReasonNoClock {
		t.Fatalf("Time Tracking off = %+v", st)
	}
	// No bridge at all: the clocks are not read, the error says why.
	e2 := &Executor{Game: newRunGame("done"), RunPoll: time.Millisecond}
	syncRecurring(t, e2, routine(birdID, "Birdhouses", "BIRD_HOUSE", 0, 0))
	e2.recurringOnce(ctx)
	if currentRun(e) != nil || currentRun(e2) != nil || recurring(t, e2).ClockError == "" {
		t.Fatalf("something started: %+v %+v", currentRun(e), recurring(t, e2))
	}
}

// Several due: the highest priority first, one run at a time, nothing while
// a run goes; a canceled run holds every routine until the owner saves one
// again.
func TestRecurringPriorityAndOneAtATime(t *testing.T) {
	ctx := context.Background()
	clocks := newClocks()
	g := newRunGame("") // the run plugin never ends by itself
	e := &Executor{Game: g, Bridge: clocks, RunPoll: time.Millisecond}
	herb, bird := routine(herbID, "Herb run", "HERB", 0, 10), routine(birdID, "Birdhouses", "BIRD_HOUSE", 0, 50)
	syncRecurring(t, e, herb, bird)
	clocks.set("HERB", "COMPLETED", 0)
	clocks.set("BIRD_HOUSE", "COMPLETED", 0)

	e.recurringOnce(ctx)
	first := currentRun(e)
	if first == nil || first.Routine != birdID || first.Final() {
		t.Fatalf("first run = %+v", first)
	}
	// While it goes, nothing else starts.
	e.recurringOnce(ctx)
	if st := recurring(t, e); st.Waiting != "um run em andamento" || currentRun(e).ID != first.ID {
		t.Fatalf("during a run = %+v, run %+v", st, currentRun(e))
	}
	if _, err := e.Execute(ctx, CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, e); r.State != RunCanceled {
		t.Fatalf("canceled = %+v", r)
	}
	// The cancel left the main activity stopped: everything waits for the owner.
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonHeldAgent {
		t.Fatalf("herb after the cancel = %+v", s)
	}
	if s := stateOf(t, e, birdID); s.Reason != ReasonHeld {
		t.Fatalf("bird after its cancel = %+v", s)
	}
	if currentRun(e).ID != first.ID {
		t.Fatal("a run started after a cancel")
	}
	// The owner saves the herb run again: it starts; the birdhouses stay held.
	saved := time.Now().Add(time.Millisecond).UTC()
	herb.UpdatedAt = &saved
	syncRecurring(t, e, herb, bird)
	e.recurringOnce(ctx)
	second := currentRun(e)
	if second.ID == first.ID || second.Routine != herbID {
		t.Fatalf("second run = %+v", second)
	}
	if s := stateOf(t, e, birdID); s.Due || s.Reason != ReasonHeld {
		t.Fatalf("bird = %+v", s)
	}
	_, _ = e.Execute(ctx, CmdRunCancel, nil)
	waitRun(t, e)
}

// The interval counts from the end of the routine's last run: the server's
// (a pod that lost its history) or the agent's own, the newer.
func TestRecurringIntervalSinceLastEnd(t *testing.T) {
	ctx := context.Background()
	e := &Executor{Game: newRunGame("done"), RunPoll: time.Millisecond}
	rc := routine(herbID, "Herb run", "", 80, 0)
	end := time.Now().Add(-79 * time.Minute).UTC()
	rc.LastRun = &RecurringRun{State: RunDone, StartedAt: end.Add(-10 * time.Minute), FinishedAt: &end}
	syncRecurring(t, e, rc)

	e.recurringOnce(ctx)
	s := stateOf(t, e, herbID)
	if s.Due || s.Reason != ReasonIntervalWaiting || s.NextAt == nil || !s.NextAt.Equal(end.Add(80*time.Minute)) {
		t.Fatalf("79 min after = %+v", s)
	}
	e.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	e.recurringOnce(ctx)
	r := waitRun(t, e)
	if r.Routine != herbID || !strings.Contains(r.Events[0].Text, "80 min desde o fim do último run") {
		t.Fatalf("run = %+v", r)
	}
	// Now its own run is the last: 80 minutes from its end.
	e.recurringOnce(ctx)
	if s = stateOf(t, e, herbID); s.Due || s.Reason != ReasonIntervalWaiting || !s.NextAt.Equal(r.FinishedAt.Add(80*time.Minute)) {
		t.Fatalf("after its own run = %+v (ended %v)", s, r.FinishedAt)
	}
	e.now = func() time.Time { return time.Now().Add(81 * time.Minute) }
	e.recurringOnce(ctx)
	if r2 := waitRun(t, e); r2.ID == r.ID || r2.Routine != herbID {
		t.Fatalf("second run = %+v", r2)
	}
}

// A clock the Time Tracking does not know falls back to the interval; a
// routine that never ran is due at once.
func TestRecurringIntervalFallbackAndFirst(t *testing.T) {
	look := recurringLook{now: time.Now(), clocks: map[string]Timer{"TREE": {Key: "TREE", State: "UNKNOWN"}}}
	rc := routine(treeID, "Tree run", "TREE", 300, 0)
	if st, byClock, _ := assess(rc, look, true); !st.Due || st.Reason != ReasonFirst || byClock {
		t.Fatalf("never ran = %+v", st)
	}
	end := time.Now().Add(-time.Hour).UTC()
	rc.LastRun = &RecurringRun{State: RunDone, StartedAt: end, FinishedAt: &end}
	if st, _, _ := assess(rc, look, true); st.Due || st.Reason != ReasonIntervalWaiting {
		t.Fatalf("an hour after = %+v", st)
	}
	// A run the server saw start but never end (the pod went away) counts as
	// failed when it started: its one retry, 15 minutes later, is due.
	rc.LastRun = &RecurringRun{State: RunRunning, StartedAt: end}
	if st, _, _ := assess(rc, look, true); !st.Due || st.Reason != ReasonRetry || !st.NextAt.Equal(end.Add(RecurringRetryAfter)) {
		t.Fatalf("unfinished run = %+v", st)
	}
	// Saved again after it: the interval counts from when it started.
	saved := time.Now().UTC()
	rc.UpdatedAt = &saved
	if st, _, _ := assess(rc, look, true); st.Due || st.Reason != ReasonIntervalWaiting || !st.NextAt.Equal(end.Add(300*time.Minute)) {
		t.Fatalf("saved after the unfinished run = %+v", st)
	}
}

func (g *runGame) setOutcome(outcome string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.outcome = outcome
}

// A failed run retries once, 15 minutes after the failure, whatever the
// clock says; a retry that fails too holds the routine until the owner saves
// it again.
func TestRecurringFailedRunRetriesOnceThenHolds(t *testing.T) {
	ctx := context.Background()
	clocks := newClocks()
	g := newRunGame("failed")
	e := &Executor{Game: g, Bridge: clocks, RunPoll: time.Millisecond}
	herb := routine(herbID, "Herb run", "HERB", 0, 0)
	syncRecurring(t, e, herb)
	clocks.set("HERB", "COMPLETED", 0)
	e.recurringOnce(ctx)
	first := waitRun(t, e)
	if first.State != RunFailed {
		t.Fatalf("run = %+v", first)
	}
	// Before the 15 minutes it waits, even with the clock seen growing and
	// ready again; its own failure, the agent's last run, does not hold it.
	clocks.set("HERB", "IN_PROGRESS", time.Hour)
	e.recurringOnce(ctx)
	clocks.set("HERB", "COMPLETED", 0)
	e.recurringOnce(ctx)
	s := stateOf(t, e, herbID)
	if s.Due || s.Reason != ReasonRetryWaiting || s.NextAt == nil || !s.NextAt.Equal(first.FinishedAt.Add(RecurringRetryAfter)) ||
		s.LastRun == nil || s.LastRun.State != RunFailed || currentRun(e).ID != first.ID {
		t.Fatalf("after the failure = %+v", s)
	}
	// 15 minutes later: the retry, whose trail says so.
	e.now = func() time.Time { return first.FinishedAt.Add(RecurringRetryAfter + time.Second) }
	e.recurringOnce(ctx)
	retry := waitRun(t, e)
	if retry.ID == first.ID || retry.Routine != herbID || retry.State != RunFailed ||
		!strings.Contains(retry.Events[0].Text, "rotina Herb run: tenta de novo, 15 min depois do run que falhou") {
		t.Fatalf("retry = %+v", retry)
	}
	// It failed too: held, however long after.
	for _, after := range []time.Duration{time.Second, RecurringRetryAfter + time.Second, 3 * time.Hour} {
		e.now = func() time.Time { return retry.FinishedAt.Add(after) }
		e.recurringOnce(ctx)
		if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonHeld || s.PrevRun == nil || s.PrevRun.State != RunFailed ||
			currentRun(e).ID != retry.ID {
			t.Fatalf("%v after the retry failed = %+v", after, s)
		}
	}
	// The owner saves it again ("Liberar"): it runs by its clock.
	saved := time.Now().Add(time.Millisecond).UTC()
	herb.UpdatedAt = &saved
	syncRecurring(t, e, herb)
	g.setOutcome("done")
	e.recurringOnce(ctx)
	if r := waitRun(t, e); r.ID == retry.ID || r.State != RunDone || !strings.Contains(r.Events[0].Text, "relógio Herb Patches pronto") {
		t.Fatalf("saved again = %+v", r)
	}
}

// A run the owner canceled never retries: the routine stays held until the
// owner saves it again.
func TestRecurringCanceledRunNeverRetries(t *testing.T) {
	ctx := context.Background()
	e := &Executor{Game: newRunGame(""), RunPoll: time.Millisecond} // the run plugin never ends by itself
	syncRecurring(t, e, routine(herbID, "Herb run", "", 60, 0))
	e.recurringOnce(ctx)
	if r := currentRun(e); r == nil || r.Routine != herbID {
		t.Fatalf("run = %+v", r)
	}
	if _, err := e.Execute(ctx, CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	canceled := waitRun(t, e)
	if canceled.State != RunCanceled {
		t.Fatalf("canceled = %+v", canceled)
	}
	for _, after := range []time.Duration{time.Second, RecurringRetryAfter + time.Second, 3 * time.Hour} {
		e.now = func() time.Time { return canceled.FinishedAt.Add(after) }
		e.recurringOnce(ctx)
		if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonHeld || s.NextAt != nil || currentRun(e).ID != canceled.ID {
			t.Fatalf("%v after the cancel = %+v", after, s)
		}
	}
}

// A routine's first failure holds the other routines (the agent's last run
// failed), but not itself: its retry runs first, and a retry that ends done
// frees the others.
func TestRecurringRetryHoldsTheOthersUntilItEnds(t *testing.T) {
	ctx := context.Background()
	g := newRunGame("failed")
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	syncRecurring(t, e, routine(herbID, "Herb run", "", 60, 50), routine(birdID, "Birdhouses", "", 60, 10))
	e.recurringOnce(ctx)
	first := waitRun(t, e)
	if first.Routine != herbID || first.State != RunFailed {
		t.Fatalf("first = %+v", first)
	}
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Reason != ReasonRetryWaiting {
		t.Fatalf("herb = %+v", s)
	}
	if s := stateOf(t, e, birdID); s.Due || s.Reason != ReasonHeldAgent || currentRun(e).ID != first.ID {
		t.Fatalf("bird while herb waits for its retry = %+v", s)
	}
	g.setOutcome("done")
	e.now = func() time.Time { return first.FinishedAt.Add(RecurringRetryAfter + time.Second) }
	e.recurringOnce(ctx)
	retry := waitRun(t, e)
	if retry.Routine != herbID || retry.State != RunDone {
		t.Fatalf("retry = %+v", retry)
	}
	e.recurringOnce(ctx)
	if r := waitRun(t, e); r.Routine != birdID || r.State != RunDone {
		t.Fatalf("bird after the retry = %+v", r)
	}
	if s := stateOf(t, e, herbID); s.Reason != ReasonIntervalWaiting || !s.NextAt.Equal(retry.FinishedAt.Add(60*time.Minute)) {
		t.Fatalf("herb after its retry = %+v", s)
	}
}

// A failed run that is not a recurring routine's (the owner's "Fazer
// agora") holds every routine, its retry too, until the owner saves any one.
func TestRecurringManualFailureHoldsEveryRoutine(t *testing.T) {
	ctx := context.Background()
	e := &Executor{Game: newRunGame("failed"), RunPoll: time.Millisecond}
	syncRecurring(t, e, routine(herbID, "Herb run", "", 60, 0))
	raw, _ := json.Marshal(RunParams{ClassName: catSwap, Stop: []string{fisher}})
	if _, err := e.Execute(ctx, CmdRunStart, raw); err != nil {
		t.Fatal(err)
	}
	manual := waitRun(t, e)
	if manual.Routine != "" || manual.State != RunFailed {
		t.Fatalf("manual = %+v", manual)
	}
	e.now = func() time.Time { return manual.FinishedAt.Add(3 * time.Hour) }
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonHeldAgent || currentRun(e).ID != manual.ID {
		t.Fatalf("herb after a failed manual run = %+v", s)
	}
}

// The hold and retry rules, one look at a time.
func TestAssessRetryAndHolds(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return now.Add(d) }
	rr := func(state string, start, end time.Duration) *RecurringRun {
		r := &RecurringRun{State: state, StartedAt: at(start)}
		if end != 0 {
			f := at(end)
			r.FinishedAt = &f
		}
		return r
	}
	run := func(routine, state string, start, end time.Duration) Run {
		f := at(end)
		return Run{ID: "run-x", Routine: routine, State: state, StartedAt: at(start), FinishedAt: &f}
	}
	ptr := func(r Run) *Run { return &r }
	dur := func(d time.Duration) *time.Duration { return &d }
	const saved = -3 * time.Hour
	cases := []struct {
		name       string
		clock      *Timer // the routine follows HERB when set; otherwise every 80 min
		disarmed   bool
		updated    *time.Duration // the routine's save; default saved
		anySaved   *time.Duration // the latest save of any routine; default its own
		last, prev *RecurringRun  // the database's
		local      []Run          // the agent's own, newest first
		latest     *Run           // the agent's last run of any kind
		reason     string
		due        bool
		next       *time.Duration
		byClock    bool
	}{
		{name: "a done run: its interval", last: rr(RunDone, -15*time.Minute, -10*time.Minute),
			reason: ReasonIntervalWaiting, next: dur(70 * time.Minute)},
		{name: "first failure, before the 15 minutes", last: rr(RunFailed, -12*time.Minute, -10*time.Minute),
			reason: ReasonRetryWaiting, next: dur(5 * time.Minute)},
		{name: "first failure, 15 minutes passed: the retry, not the 80 min interval", last: rr(RunFailed, -25*time.Minute, -20*time.Minute),
			reason: ReasonRetry, due: true, next: dur(-5 * time.Minute)},
		{name: "a clock still ready after the failed run it fired: the retry, which disarms it again",
			clock: &Timer{Key: "HERB", State: "COMPLETED"}, disarmed: true, last: rr(RunFailed, -25*time.Minute, -20*time.Minute),
			reason: ReasonRetry, due: true, next: dur(-5 * time.Minute), byClock: true},
		{name: "a clock growing after the failure: the retry anyway",
			clock: &Timer{Key: "HERB", State: "IN_PROGRESS"}, last: rr(RunFailed, -25*time.Minute, -20*time.Minute),
			reason: ReasonRetry, due: true, next: dur(-5 * time.Minute)},
		{name: "the retry failed too: held", last: rr(RunFailed, -8*time.Minute, -5*time.Minute), prev: rr(RunFailed, -25*time.Minute, -20*time.Minute),
			reason: ReasonHeld},
		{name: "the agent's own two failures: held", last: rr(RunDone, -2*time.Hour, -2*time.Hour+time.Minute),
			local:  []Run{run(herbID, RunFailed, -8*time.Minute, -5*time.Minute), run(herbID, RunFailed, -25*time.Minute, -20*time.Minute)},
			reason: ReasonHeld},
		{name: "a failure before the save does not count", last: rr(RunFailed, -8*time.Minute, -5*time.Minute), prev: rr(RunFailed, -4*time.Hour, -4*time.Hour+time.Minute),
			reason: ReasonRetryWaiting, next: dur(10 * time.Minute)},
		{name: "canceled: held, never retries", last: rr(RunCanceled, -2*time.Hour, -2*time.Hour+time.Minute),
			reason: ReasonHeld},
		{name: "canceled after a done run: held", last: rr(RunCanceled, -8*time.Minute, -5*time.Minute), prev: rr(RunDone, -2*time.Hour, -2*time.Hour+time.Minute),
			reason: ReasonHeld},
		{name: "a run that never finished (the pod went away) counts as failed when it started", last: rr(RunRunning, -20*time.Minute, 0),
			reason: ReasonRetry, due: true, next: dur(-5 * time.Minute)},
		{name: "a failure after a run that never finished: held", last: rr(RunFailed, -12*time.Minute, -10*time.Minute), prev: rr(RunRunning, -50*time.Minute, 0),
			reason: ReasonHeld},
		{name: "the database's copy of the agent's run is the same run",
			local: []Run{func() Run {
				r := run(herbID, RunFailed, -12*time.Minute, -10*time.Minute)
				r.StartedAt = r.StartedAt.Add(789 * time.Nanosecond)
				return r
			}()},
			last: rr(RunFailed, -12*time.Minute, -10*time.Minute), prev: rr(RunDone, -2*time.Hour, -2*time.Hour+time.Minute),
			reason: ReasonRetryWaiting, next: dur(5 * time.Minute)},
		{name: "saved again after the failure (Liberar): its interval", updated: dur(-5 * time.Minute),
			last: rr(RunFailed, -12*time.Minute, -10*time.Minute), reason: ReasonIntervalWaiting, next: dur(70 * time.Minute)},
		{name: "a done retry: its interval from the retry", last: rr(RunDone, -8*time.Minute, -5*time.Minute), prev: rr(RunFailed, -25*time.Minute, -20*time.Minute),
			reason: ReasonIntervalWaiting, next: dur(75 * time.Minute)},
		{name: "its own first failure is the agent's last run: not held by it",
			local: []Run{run(herbID, RunFailed, -12*time.Minute, -10*time.Minute)}, latest: ptr(run(herbID, RunFailed, -12*time.Minute, -10*time.Minute)),
			reason: ReasonRetryWaiting, next: dur(5 * time.Minute)},
		{name: "another routine's failure holds it until that one retries", latest: ptr(run(birdID, RunFailed, -12*time.Minute, -10*time.Minute)),
			last: rr(RunDone, -2*time.Hour, -2*time.Hour+time.Minute), reason: ReasonHeldAgent},
		{name: "a failed manual run holds it", latest: ptr(run("", RunFailed, -12*time.Minute, -10*time.Minute)),
			last: rr(RunDone, -2*time.Hour, -2*time.Hour+time.Minute), reason: ReasonHeldAgent},
		{name: "a canceled trigger run holds it", latest: func() *Run {
			r := run("", RunCanceled, -12*time.Minute, -10*time.Minute)
			r.Trigger = EventKittenGrown
			return &r
		}(), reason: ReasonHeldAgent},
		{name: "a failed manual run after its first failure: no retry while that holds",
			last: rr(RunFailed, -40*time.Minute, -30*time.Minute), latest: ptr(run("", RunFailed, -12*time.Minute, -10*time.Minute)),
			reason: ReasonHeldAgent},
		{name: "saving any routine frees the agent's hold, and its own retry is due",
			anySaved: dur(-5 * time.Minute), last: rr(RunFailed, -40*time.Minute, -30*time.Minute), latest: ptr(run("", RunFailed, -12*time.Minute, -10*time.Minute)),
			reason: ReasonRetry, due: true, next: dur(-15 * time.Minute)},
		{name: "a done run of any kind after the failure: its retry still waits", last: rr(RunFailed, -12*time.Minute, -10*time.Minute),
			latest: ptr(run("", RunDone, -9*time.Minute, -8*time.Minute)), reason: ReasonRetryWaiting, next: dur(5 * time.Minute)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := routine(herbID, "Herb run", "", 80, 0)
			look := recurringLook{now: now, lastOf: map[string][]Run{herbID: tc.local}, latest: tc.latest}
			if tc.clock != nil {
				rc.Clock = "HERB"
				look.clocks, look.clockNow = map[string]Timer{"HERB": *tc.clock}, now.Unix()
			}
			up := at(saved)
			if tc.updated != nil {
				up = at(*tc.updated)
			}
			rc.UpdatedAt, rc.LastRun, rc.PrevRun = &up, tc.last, tc.prev
			look.saved = up
			if tc.anySaved != nil {
				look.saved = at(*tc.anySaved)
			}
			st, byClock, _ := assess(rc, look, !tc.disarmed)
			if st.Reason != tc.reason || st.Due != tc.due || byClock != tc.byClock {
				t.Fatalf("got %s due=%v byClock=%v, want %s due=%v byClock=%v", st.Reason, st.Due, byClock, tc.reason, tc.due, tc.byClock)
			}
			if tc.next != nil && (st.NextAt == nil || !st.NextAt.Equal(at(*tc.next))) {
				t.Fatalf("nextAt = %v, want %v", st.NextAt, at(*tc.next))
			}
			if tc.next == nil && st.NextAt != nil {
				t.Fatalf("nextAt = %v, want none", st.NextAt)
			}
		})
	}
}

// awayGame is a game whose character is logged out (a break).
type awayGame struct{ *runGame }

func (awayGame) State(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"loggedIn":false,"player":{}}`), nil
}

// Due but away from a safe point (a break, logged out): it waits and says so.
func TestRecurringWaitsForASafePoint(t *testing.T) {
	ctx := context.Background()
	e := &Executor{Game: awayGame{newRunGame("done")}, RunPoll: time.Millisecond}
	syncRecurring(t, e, routine(herbID, "Herb run", "", 60, 0))
	e.recurringOnce(ctx)
	st := recurring(t, e)
	if !st.Recurring[0].Due || !strings.Contains(st.Waiting, "fora do jogo") || st.Last != nil || currentRun(e) != nil {
		t.Fatalf("away = %+v, run %+v", st, currentRun(e))
	}
}

// routines_sync replaces the recurring routines and keeps them in the file
// for an agent that restarts with the server away; a bad list changes
// nothing.
func TestRecurringSyncAndFile(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "agent-recurring.json")
	e := &Executor{Game: &fakeGame{}, RecurringFile: file}
	syncRecurring(t, e, routine(herbID, "Herb run", "HERB", 80, 5))
	again := &Executor{Game: &fakeGame{}, RecurringFile: file}
	if got := recurring(t, again).Recurring; len(got) != 1 || got[0].ID != herbID || got[0].IntervalMin != 80 {
		t.Fatalf("loaded = %+v", got)
	}
	bad := map[string][]Recurring{
		"no id": {routine("", "x", "HERB", 0, 0)},
		"twice": {routine(herbID, "a", "HERB", 0, 0), routine(herbID, "b", "TREE", 0, 0)},
		"clock": {routine(herbID, "a", "MOON", 0, 0)},
		"too many": func() []Recurring {
			var out []Recurring
			for i := range RecurringMax + 1 {
				out = append(out, routine(fmt.Sprintf("rtn_%08x", i), fmt.Sprint(i), "HERB", 0, 0))
			}
			return out
		}(),
	}
	for name, list := range bad {
		raw, _ := json.Marshal(RoutinesParams{Recurring: list})
		if _, err := e.Execute(ctx, CmdRoutinesSync, raw); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if got := recurring(t, e).Recurring; len(got) != 1 {
		t.Fatalf("a refused sync changed them: %+v", got)
	}
	// A sync without recurring routines (a server from before them) clears them.
	if _, err := e.Execute(ctx, CmdRoutinesSync, json.RawMessage(`{"triggers":[]}`)); err != nil {
		t.Fatal(err)
	}
	if got := recurring(t, e).Recurring; len(got) != 0 {
		t.Fatalf("cleared = %+v", got)
	}
	if _, err := ValidateParams(CmdRecurringGet, nil); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRecurring(t *testing.T) {
	ok := routine("", "  Herb run ", "HERB", 80, 10)
	got, err := ValidateRecurring(ok)
	if err != nil || got.Name != "Herb run" {
		t.Fatalf("valid = %+v, %v", got, err)
	}
	for name, mut := range map[string]func(*Recurring){
		"bad id":         func(r *Recurring) { r.ID = "rtn_../x" },
		"no name":        func(r *Recurring) { r.Name = " " },
		"long name":      func(r *Recurring) { r.Name = strings.Repeat("x", 81) },
		"two lines":      func(r *Recurring) { r.Name = "a\nb" },
		"unknown clock":  func(r *Recurring) { r.Clock = "MOON" },
		"short interval": func(r *Recurring) { r.IntervalMin = 5 },
		"long interval":  func(r *Recurring) { r.IntervalMin = RecurringIntervalMax + 1 },
		"neither":        func(r *Recurring) { r.Clock, r.IntervalMin = "", 0 },
		"priority":       func(r *Recurring) { r.Priority = 101 },
		"negative":       func(r *Recurring) { r.Priority = -1 },
		"bad run":        func(r *Recurring) { r.Run.ClassName = "Herbrun" },
		"protected run":  func(r *Recurring) { r.Run.ClassName = "net.runelite.client.plugins.microbot.MicrobotPlugin" },
		"bad last run":   func(r *Recurring) { r.LastRun = &RecurringRun{State: "exploded", StartedAt: time.Now()} },
	} {
		r := ok
		mut(&r)
		if _, err := ValidateRecurring(r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A report naming a routine: an id, and never with a trigger.
	now := time.Now().UTC()
	run := Run{ID: "run-1", UID: "0123456789abcdef", ClassName: catSwap, Routine: herbID, State: RunDone, StartedAt: now}
	if err := ValidateReport(run); err != nil {
		t.Fatal(err)
	}
	for _, mut := range []func(*Run){func(r *Run) { r.Routine = "herb" }, func(r *Run) { r.Trigger = EventKittenGrown }} {
		bad := run
		mut(&bad)
		if ValidateReport(bad) == nil {
			t.Errorf("report %+v accepted", bad)
		}
	}
}

// Only an agent that declares recurring_get gets the recurring routines in
// routines_sync: an older one refuses the unknown field.
func TestRoutinesFor(t *testing.T) {
	rs := []Recurring{routine(herbID, "Herb run", "HERB", 0, 0)}
	if p := RoutinesFor(Info{Commands: []string{CmdRoutinesSync}}, nil, rs); p.Recurring != nil || p.Triggers == nil {
		t.Fatalf("older agent = %+v", p)
	}
	if p := RoutinesFor(Info{Commands: Commands}, nil, rs); len(p.Recurring) != 1 {
		t.Fatalf("agent = %+v", p)
	}
	raw, _ := json.Marshal(RoutinesFor(Info{Commands: []string{CmdRoutinesSync}}, []Trigger{cat(true)}, rs))
	if strings.Contains(string(raw), "recurring") {
		t.Fatalf("older agent's params = %s", raw)
	}
	// The previous run goes only to an agent that retries: an agent that runs
	// recurring routines from before the retry refuses the field.
	end := time.Now().UTC()
	rs[0].LastRun = &RecurringRun{State: RunFailed, StartedAt: end.Add(-time.Minute), FinishedAt: &end}
	rs[0].PrevRun = &RecurringRun{State: RunDone, StartedAt: end.Add(-time.Hour), FinishedAt: &end}
	if p := RoutinesFor(Info{Commands: Commands}, nil, rs); p.Recurring[0].PrevRun != nil || p.Recurring[0].LastRun == nil || rs[0].PrevRun == nil {
		t.Fatalf("agent before the retry = %+v", p.Recurring[0])
	}
	if p := RoutinesFor(Info{Commands: Commands, Features: AgentFeatures}, nil, rs); p.Recurring[0].PrevRun == nil {
		t.Fatalf("agent = %+v", p.Recurring[0])
	}
	raw, _ = json.Marshal(RoutinesFor(Info{Commands: Commands}, nil, rs))
	if strings.Contains(string(raw), "prevRun") {
		t.Fatalf("params of an agent before the retry = %s", raw)
	}
	raw, _ = json.Marshal(RoutinesFor(Info{Commands: Commands, Features: AgentFeatures}, nil, rs))
	if _, err := validateRoutines(raw); err != nil {
		t.Fatalf("the agent refuses its own params: %v", err)
	}
}

// WatchRecurring wakes at once when the server sends new routines.
func TestWatchRecurringWakesOnSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &Executor{Game: newRunGame("done"), RunPoll: time.Millisecond, RecurringPoll: time.Hour}
	go e.WatchRecurring(ctx)
	waitFor(t, func() bool { return recurring(t, e).CheckedAt != nil })
	syncRecurring(t, e, routine(herbID, "Herb run", "", 60, 0))
	waitFor(t, func() bool { r := currentRun(e); return r != nil && r.Routine == herbID && r.Final() })
}

// breakBridge adds the bridge's /bridge/break to the clocks.
type breakBridge struct {
	*clockBridge
	brk string // the /bridge/break answer; "" answers 404
}

func (b *breakBridge) JSON(ctx context.Context, method, path string, q url.Values, body any, timeout time.Duration) (json.RawMessage, error) {
	if path == "/bridge/break" {
		if b.brk == "" {
			return nil, fmt.Errorf("404")
		}
		return json.RawMessage(b.brk), nil
	}
	return b.clockBridge.JSON(ctx, method, path, q, body, timeout)
}

func TestBreakNear(t *testing.T) {
	cases := []struct {
		name, brk string
		timeoutS  int
		want      string
	}{
		{"no break from the bridge", "", 0, ""},
		{"a break going on", `{"active":true,"left_s":120}`, 0, "a pausa do Break Handler"},
		{"V2 does not say when", `{"active":false}`, 0, ""},
		{"closer than the margin", `{"active":false,"in_s":300}`, 0, "a pausa do Break Handler começa em 5 min"},
		{"past the margin", `{"active":false,"in_s":1200}`, 0, ""},
		{"the run's timeout counts", `{"active":false,"in_s":720}`, 600, ""},
		{"closer than the run's timeout", `{"active":false,"in_s":480}`, 600, "a pausa do Break Handler começa em 8 min"},
		{"a long timeout is capped", `{"active":false,"in_s":2100}`, 7200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Executor{Game: newRunGame("done"), Bridge: &breakBridge{clockBridge: newClocks(), brk: tc.brk}}
			if got := e.breakNear(context.Background(), RunParams{ClassName: catSwap, TimeoutS: tc.timeoutS}); got != tc.want {
				t.Errorf("breakNear = %q, want %q", got, tc.want)
			}
		})
	}
}

// A due routine waits for the Break Handler's next break when it is near,
// and starts once the break is far again.
func TestRecurringWaitsForANearBreak(t *testing.T) {
	ctx := context.Background()
	bridge := &breakBridge{clockBridge: newClocks(), brk: `{"active":false,"in_s":240}`}
	e := &Executor{Game: newRunGame("done"), Bridge: bridge, RunPoll: time.Millisecond}
	syncRecurring(t, e, routine(herbID, "Herb run", "HERB", 0, 0))
	bridge.set("HERB", "COMPLETED", 0)

	e.recurringOnce(ctx)
	if st := recurring(t, e); currentRun(e) != nil || st.Waiting != "esperando: a pausa do Break Handler começa em 4 min" {
		t.Fatalf("near break: waiting %q, run %+v", st.Waiting, currentRun(e))
	}
	if s := stateOf(t, e, herbID); !s.Due {
		t.Fatalf("the routine stays due while it waits: %+v", s)
	}

	bridge.brk = `{"active":false,"in_s":3600}`
	e.recurringOnce(ctx)
	if r := waitRun(t, e); r.Routine != herbID {
		t.Fatalf("run = %+v", r)
	}
}
