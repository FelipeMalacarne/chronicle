package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func cat(enabled bool) Trigger {
	return Trigger{Event: EventKittenGrown, Enabled: enabled, Run: RunParams{ClassName: catSwap, Stop: []string{fisher}}}
}

// The server's triggers replace the agent's, and the file keeps them for an
// attached agent; a bad list changes nothing; the web never sends it.
func TestRoutinesSyncReplacesTheTriggers(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agent-triggers.json")
	e := &Executor{Game: &fakeGame{}, TriggerFile: file}
	ctx := context.Background()
	raw, _ := json.Marshal(cat(false))
	if _, err := e.Execute(ctx, CmdTriggerSet, raw); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(RoutinesParams{Triggers: []Trigger{cat(true)}})
	out, err := e.Execute(ctx, CmdRoutinesSync, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.(TriggerStatus).Triggers; len(got) != 1 || !got[0].Enabled {
		t.Fatalf("synced triggers = %+v", got)
	}
	var saved []Trigger
	if b, _ := os.ReadFile(file); json.Unmarshal(b, &saved) != nil || len(saved) != 1 || !saved[0].Enabled {
		t.Fatalf("file = %+v", saved)
	}
	for name, p := range map[string]RoutinesParams{
		"twice":         {Triggers: []Trigger{cat(true), cat(false)}},
		"unknown event": {Triggers: []Trigger{{Event: "level_up", Run: RunParams{ClassName: catSwap}}}},
		"protected run": {Triggers: []Trigger{{Event: EventKittenGrown, Run: RunParams{ClassName: "net.runelite.client.plugins.microbot.MicrobotPlugin"}}}},
	} {
		raw, _ = json.Marshal(p)
		if _, err := e.Execute(ctx, CmdRoutinesSync, raw); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	out, _ = e.Execute(ctx, CmdTriggerGet, nil)
	if got := out.(TriggerStatus).Triggers; len(got) != 1 || !got[0].Enabled {
		t.Fatalf("a refused sync changed the triggers: %+v", got)
	}
	// An empty list clears them.
	if _, err := e.Execute(ctx, CmdRoutinesSync, json.RawMessage(`{"triggers":[]}`)); err != nil {
		t.Fatal(err)
	}
	if out, _ = e.Execute(ctx, CmdTriggerGet, nil); len(out.(TriggerStatus).Triggers) != 0 {
		t.Fatalf("cleared triggers = %+v", out)
	}
	if _, err := ValidateParams(CmdRoutinesSync, raw); err == nil {
		t.Fatal("ValidateParams took routines_sync from the web")
	}
}

func TestOutboxKeepsTheNewestState(t *testing.T) {
	var o outbox
	o.init()
	ev := func(n int) []RunEvent { return make([]RunEvent, n) }
	all := func(string) bool { return true }
	o.put("acc_a", Run{UID: "aa", Events: ev(2), State: RunRunning})
	o.put("acc_a", Run{UID: "bb", Events: ev(1)})
	o.put("acc_a", Run{UID: "aa", Events: ev(1), State: RunWaiting}) // a stale one put back
	o.put("acc_a", Run{UID: "aa", Events: ev(3), State: RunDone})
	o.put("acc_a", Run{Events: ev(9)}) // no uid: nothing to key it by
	got := o.take(all)
	if len(got) != 2 || got[0].run.UID != "aa" || got[0].run.State != RunDone || got[1].run.UID != "bb" || got[0].account != "acc_a" {
		t.Fatalf("outbox = %+v", got)
	}
	if len(o.take(all)) != 0 {
		t.Fatal("take did not empty the outbox")
	}
}

// The runs of a client the link does not serve stay queued, in order, for
// a link that does; the same uid on two clients is two runs.
func TestOutboxKeepsTheRunsOfClientsNotLinked(t *testing.T) {
	var o outbox
	o.init()
	o.put("acc_a", Run{UID: "aa"})
	o.put("acc_b", Run{UID: "aa", State: RunDone})
	o.put("acc_b", Run{UID: "bb"})
	o.put("acc_a", Run{UID: "cc"})
	onlyA := func(account string) bool { return account == "acc_a" }
	if got := o.take(onlyA); len(got) != 2 || got[0].run.UID != "aa" || got[1].run.UID != "cc" {
		t.Fatalf("first link = %+v", got)
	}
	got := o.take(func(string) bool { return true })
	if len(got) != 2 || got[0].account != "acc_b" || got[0].run.State != RunDone || got[1].run.UID != "bb" {
		t.Fatalf("second link = %+v", got)
	}
}

func TestValidateReport(t *testing.T) {
	now := time.Now().UTC()
	ok := Run{
		ID: "run-3", UID: "0123456789abcdef", ClassName: catSwap, Stop: []string{fisher}, Trigger: EventKittenGrown,
		State: RunDone, StartedAt: now, FinishedAt: &now, Result: json.RawMessage(`{"outcome":"done"}`),
		Events: []RunEvent{{At: now, State: RunWaiting, Text: "esperando"}, {At: now, State: RunDone, Text: "feito"}},
	}
	if err := ValidateReport(ok); err != nil {
		t.Fatalf("valid report: %v", err)
	}
	for name, mut := range map[string]func(*Run){
		"no uid":        func(r *Run) { r.UID = "" },
		"uid not hex":   func(r *Run) { r.UID = "../../etc" },
		"no id":         func(r *Run) { r.ID = "" },
		"bad class":     func(r *Run) { r.ClassName = "rm -rf" },
		"bad stop":      func(r *Run) { r.Stop = []string{"x"} },
		"trigger":       func(r *Run) { r.Trigger = "level_up" },
		"state":         func(r *Run) { r.State = "exploded" },
		"no start":      func(r *Run) { r.StartedAt = time.Time{} },
		"long trail":    func(r *Run) { r.Events = make([]RunEvent, 501) },
		"trail state":   func(r *Run) { r.Events = []RunEvent{{At: now, State: "x"}} },
		"long line":     func(r *Run) { r.Events = []RunEvent{{At: now, State: RunDone, Text: strings.Repeat("x", 1001)}} },
		"result":        func(r *Run) { r.Result = json.RawMessage(`{`) },
		"long error":    func(r *Run) { r.Error = strings.Repeat("x", 2001) },
		"too many stop": func(r *Run) { r.Stop = slices.Repeat([]string{fisher}, 9) },
	} {
		r := ok
		r.Events = slices.Clone(ok.Events)
		mut(&r)
		if ValidateReport(r) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Every change of a run reaches OnRun, under one uid, with a growing trail,
// and the last is the final state.
func TestRunsReachOnRun(t *testing.T) {
	g := newRunGame("done")
	var (
		mu   sync.Mutex
		seen []Run
	)
	e := &Executor{Game: g, RunPoll: time.Millisecond, OnRun: func(r Run) { mu.Lock(); seen = append(seen, r); mu.Unlock() }}
	if _, err := e.Execute(context.Background(), CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`)); err != nil {
		t.Fatal(err)
	}
	final := waitRun(t, e)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 3 || final.UID == "" || !reRunUID.MatchString(final.UID) {
		t.Fatalf("seen %d reports, final %+v", len(seen), final)
	}
	for i, r := range seen {
		if r.UID != final.UID || (i > 0 && len(r.Events) < len(seen[i-1].Events)) {
			t.Fatalf("report %d = %+v", i, r)
		}
	}
	if last := seen[len(seen)-1]; !last.Final() || len(last.Events) != len(final.Events) {
		t.Fatalf("last report = %+v", last)
	}
}

// A history from before the uids gets one per run, kept in the file.
func TestOldHistoryGetsStableUIDs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agent-runs.json")
	now := time.Now().UTC()
	old := []Run{
		{ID: "run-1", ClassName: catSwap, State: RunDone, StartedAt: now, FinishedAt: &now, Events: []RunEvent{}},
		{ID: "run-2", ClassName: catSwap, State: RunFailed, StartedAt: now, FinishedAt: &now, Events: []RunEvent{}},
	}
	if err := writeJSONFile(file, old); err != nil {
		t.Fatal(err)
	}
	uids := func() []string {
		e := &Executor{Game: &fakeGame{}, RunFile: file}
		var out []string
		for _, r := range e.runHistory().Runs {
			if !reRunUID.MatchString(r.UID) {
				t.Fatalf("run without uid: %+v", r)
			}
			out = append(out, r.UID)
		}
		return out
	}
	first := uids()
	if len(first) != 2 || first[0] == first[1] || !reflect.DeepEqual(first, uids()) {
		t.Fatalf("uids = %v, then %v", first, uids())
	}
}

// End to end over the link: the agent declares routines_sync, sends its
// history when it links, then every change of a run as it happens.
func TestAgentReportsRunsToTheServer(t *testing.T) {
	link, url := hubServer(t)
	var (
		mu   sync.Mutex
		runs = map[string]Run{}
	)
	link.OnEvent = func(_ Info, ev Event) {
		mu.Lock()
		defer mu.Unlock()
		if ev.Run != nil {
			if cur, ok := runs[ev.Run.UID]; !ok || len(ev.Run.Events) >= len(cur.Events) {
				runs[ev.Run.UID] = *ev.Run
			}
		}
	}
	file := filepath.Join(t.TempDir(), "agent-runs.json")
	now := time.Now().UTC()
	if err := writeJSONFile(file, []Run{{ID: "run-1", UID: "00000000000000aa", ClassName: catSwap, State: RunDone, StartedAt: now, FinishedAt: &now, Events: []RunEvent{}}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := &Agent{URL: url, Token: StaticToken("zaros"), Log: quiet, Hello: Hello{Host: "zaros", Account: "acc_a"},
		Executor: &Executor{Game: newRunGame("done"), RunPoll: time.Millisecond, RunFile: file}}
	go func() { _ = a.Run(ctx) }()
	if info := waitAgents(t, link, 1)[0]; !info.Supports(CmdRoutinesSync) {
		t.Fatalf("hello without routines_sync: %+v", info.Commands)
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return runs["00000000000000aa"].State == RunDone })

	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	defer ccancel()
	var started RunStatus
	if err := link.Call(cctx, "acc_a", CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`), &started); err != nil {
		t.Fatal(err)
	}
	uid := started.Run.UID
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); r := runs[uid]; return r.Final() && len(r.Events) > 1 })
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition never held")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
