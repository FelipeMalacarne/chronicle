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
	// A run the server saw start but never end (the pod went away) holds it.
	rc.LastRun = &RecurringRun{State: RunRunning, StartedAt: end}
	if st, _, _ := assess(rc, look, true); st.Due || st.Reason != ReasonHeld {
		t.Fatalf("unfinished run = %+v", st)
	}
	// Saved again after it: the interval counts from when it started.
	saved := time.Now().UTC()
	rc.UpdatedAt = &saved
	if st, _, _ := assess(rc, look, true); st.Due || st.Reason != ReasonIntervalWaiting || !st.NextAt.Equal(end.Add(300*time.Minute)) {
		t.Fatalf("saved after the unfinished run = %+v", st)
	}
}

// A failed run holds its routine until the owner saves it again.
func TestRecurringFailedRunHolds(t *testing.T) {
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
	clocks.set("HERB", "IN_PROGRESS", time.Hour) // even with the clock seen growing
	e.recurringOnce(ctx)
	clocks.set("HERB", "COMPLETED", 0)
	e.recurringOnce(ctx)
	if s := stateOf(t, e, herbID); s.Due || s.Reason != ReasonHeld || currentRun(e).ID != first.ID {
		t.Fatalf("after the failure = %+v", s)
	}
	saved := time.Now().Add(time.Millisecond).UTC()
	herb.UpdatedAt = &saved
	syncRecurring(t, e, herb)
	g.mu.Lock()
	g.outcome = "done"
	g.mu.Unlock()
	e.recurringOnce(ctx)
	if r := waitRun(t, e); r.ID == first.ID || r.State != RunDone {
		t.Fatalf("saved again = %+v", r)
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
