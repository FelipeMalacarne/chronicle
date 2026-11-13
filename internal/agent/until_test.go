package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const trainer = "a.b.FishingTrainerPlugin"

// startUntil starts a run of the trainer until fishing 76 and returns its uid.
func startUntil(t *testing.T, e *Executor, extra string) string {
	t.Helper()
	out, err := e.Execute(context.Background(), CmdRunStart, json.RawMessage(
		`{"className":"`+trainer+`","stop":["`+fisher+`"],"until":{"objective":"skill:fishing>=76","account":"acc_zezima"}`+extra+`}`))
	if err != nil {
		t.Fatal(err)
	}
	r := out.(RunStatus).Run
	if r.Until == nil || r.Until.Objective != "skill:fishing>=76" || r.Until.Account != "acc_zezima" {
		t.Fatalf("run until = %+v", r.Until)
	}
	return r.UID
}

// waitTrail waits for a trail line containing text.
func waitTrail(t *testing.T, e *Executor, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, _ := e.Execute(context.Background(), CmdRunStatus, nil)
		if r := out.(RunStatus).Run; r != nil {
			for _, ev := range r.Events {
				if strings.Contains(ev.Text, text) {
					return
				}
			}
			if r.Final() {
				t.Fatalf("the run ended without %q: %+v", text, r)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %q on the trail: %+v", text, out.(RunStatus).Run)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func proveUntil(t *testing.T, e *Executor, uid string) {
	t.Helper()
	p := RunUntilParams{UID: uid, Objective: "skill:fishing>=76", Evidence: "nível 76"}
	if _, err := e.Execute(context.Background(), CmdRunUntil, raw(t, p)); err != nil {
		t.Fatal(err)
	}
}

func TestRunUntilStopsThePluginWhenTheServerProvesIt(t *testing.T) {
	g := newRunGame("")
	g.levels = map[string]int{"Fishing": 75}
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	uid := startUntil(t, e, "")
	waitTrail(t, e, "Fishing: nível 75")
	// The trainer never stops by itself; the run goes on until the proof.
	time.Sleep(20 * time.Millisecond)
	if out, _ := e.Execute(context.Background(), CmdRunStatus, nil); out.(RunStatus).Run.Final() {
		t.Fatalf("the run ended without the proof: %+v", out.(RunStatus).Run)
	}
	g.mu.Lock()
	g.pos = WalkParams{X: 2611, Y: 3410} // the trainer walked to the spot
	g.mu.Unlock()
	proveUntil(t, e, uid)
	proveUntil(t, e, uid) // the same proof twice changes nothing
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "skill:fishing>=76 provado pelo ingest") {
		t.Fatalf("run = %+v", r)
	}
	proven := 0
	for _, ev := range r.Events {
		if strings.Contains(ev.Text, "objetivo cumprido, provado pelo ingest: skill:fishing>=76 (nível 76)") {
			proven++
		}
	}
	if proven != 1 {
		t.Fatalf("proof lines = %d: %+v", proven, r.Events)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[trainer] || !g.active[fisher] {
		t.Fatalf("after the run: %v", g.active)
	}
	var order []string
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			order = append(order, q)
		}
	}
	want := []string{"POST /scripts/stop " + fisher, "POST /scripts/start " + trainer, "POST /scripts/stop " + trainer, "POST /walk ", "POST /scripts/start " + fisher}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %v, want %v", order, want)
	}
}

func TestRunUntilAlreadyProvenTouchesNothing(t *testing.T) {
	g := newRunGame("")
	g.fight = true // the run waits for a safe point
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	uid := startUntil(t, e, "")
	waitTrail(t, e, "em combate")
	proveUntil(t, e, uid)
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "já estava cumprido") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			t.Fatalf("the run acted: %v", g.requests)
		}
	}
}

func TestRunUntilPluginThatStopsWithoutTheProofFails(t *testing.T) {
	g := newRunGame("")
	e := &Executor{Game: g, RunPoll: time.Millisecond, untilGrace: 30 * time.Millisecond}
	startUntil(t, e, "")
	waitTrail(t, e, "iniciado")
	g.mu.Lock()
	g.active[trainer] = false // it stopped by itself
	g.mu.Unlock()
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "did not prove skill:fishing>=76") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[fisher] {
		t.Fatal("the main activity was restarted after a failed run")
	}
}

func TestRunUntilProofAfterThePluginStopped(t *testing.T) {
	g := newRunGame("")
	e := &Executor{Game: g, RunPoll: time.Millisecond, untilGrace: time.Minute}
	uid := startUntil(t, e, `,"stay":true`)
	waitTrail(t, e, "iniciado")
	g.mu.Lock()
	g.active[trainer] = false
	g.mu.Unlock()
	waitTrail(t, e, "parou sozinho")
	proveUntil(t, e, uid)
	r := waitRun(t, e)
	if r.State != RunDone {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	stops := 0
	for _, q := range g.requests {
		if q == "POST /scripts/stop "+trainer {
			stops++
		}
		if strings.HasPrefix(q, "POST /walk") {
			t.Fatal("a stay run walked back")
		}
	}
	if stops != 0 || !g.active[fisher] {
		t.Fatalf("stops of the stopped trainer = %d, active %v", stops, g.active)
	}
}

func TestRunUntilFailedOutcomeAndCancel(t *testing.T) {
	g := newRunGame("")
	e := &Executor{Game: g, RunPoll: time.Millisecond, untilGrace: time.Minute}
	startUntil(t, e, "")
	waitTrail(t, e, "iniciado")
	g.mu.Lock()
	g.results = append(g.results, json.RawMessage(`{"outcome":"failed","reason":"no bait"}`))
	g.active[trainer] = false
	g.mu.Unlock()
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "no bait") {
		t.Fatalf("run = %+v", r)
	}

	g2 := newRunGame("")
	e2 := &Executor{Game: g2, RunPoll: time.Millisecond}
	startUntil(t, e2, "")
	waitTrail(t, e2, "iniciado")
	if _, err := e2.Execute(context.Background(), CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, e2); r.State != RunCanceled {
		t.Fatalf("run = %+v", r)
	}
	g2.mu.Lock()
	defer g2.mu.Unlock()
	if g2.active[trainer] {
		t.Fatal("a canceled run left the trainer on")
	}
}

func TestRunUntilRefusals(t *testing.T) {
	g := newRunGame("")
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	uid := startUntil(t, e, "")
	for _, c := range []struct {
		p    RunUntilParams
		want string
	}{
		{RunUntilParams{UID: "0123456789abcdef", Objective: "skill:fishing>=76"}, "no such run"},
		{RunUntilParams{UID: uid, Objective: "skill:fishing>=77"}, "does not wait"},
		{RunUntilParams{UID: "x", Objective: "skill:fishing>=76"}, "bad run uid"},
		{RunUntilParams{UID: uid, Objective: "skill:fishing>=100"}, "skills take"},
	} {
		if _, err := e.Execute(context.Background(), CmdRunUntil, raw(t, c.p)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v, want %q", c.p, err, c.want)
		}
	}
	// The web never sends it.
	if _, err := ValidateParams(CmdRunUntil, raw(t, RunUntilParams{UID: uid, Objective: "skill:fishing>=76"})); err == nil {
		t.Error("run_until passed the web's validation")
	}
	if _, err := e.Execute(context.Background(), CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if _, err := e.Execute(context.Background(), CmdRunUntil, raw(t, RunUntilParams{UID: r.UID, Objective: "skill:fishing>=76"})); err == nil || !strings.Contains(err.Error(), "already ended") {
		t.Errorf("proof after the end: %v", err)
	}
}

func TestRunUntilParams(t *testing.T) {
	for _, c := range []struct{ params, bad string }{
		{`{"className":"a.b.T","until":{"objective":"skill:fishing>=76"},"timeoutS":43200}`, ""},
		{`{"className":"a.b.T","until":{"objective":"quest:cooks_assistant@finished","account":"acc_ze_ma"}}`, ""},
		{`{"className":"a.b.T","until":{"objective":"skill:fishing>=76"},"timeoutS":43201}`, "timeoutS"},
		{`{"className":"a.b.T","timeoutS":43200}`, "timeoutS"},
		{`{"className":"a.b.T","until":{"objective":"skill:fishing >= 76"}}`, "until"},
		{`{"className":"a.b.T","until":{"objective":"fishing"}}`, "until"},
		{`{"className":"a.b.T","until":{"objective":"skill:fishing>=76","account":"zezima"}}`, "bad account"},
		{`{"className":"a.b.T","until":{"objective":"skill:fishing>=76","extra":1}}`, "unknown field"},
	} {
		_, err := ValidateParams(CmdRunStart, json.RawMessage(c.params))
		if (c.bad == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.bad)) {
			t.Errorf("%s: %v, want %q", c.params, err, c.bad)
		}
	}
	// A reported run's until passes the same checks.
	r := Run{ID: "run-1", UID: "0123456789abcdef", ClassName: "a.b.T", State: RunRunning, StartedAt: time.Now(), Until: &RunUntil{Objective: "skill:fishing>=76"}}
	if err := ValidateReport(r); err != nil {
		t.Fatal(err)
	}
	r.Until.Objective = "skill:fishing"
	if ValidateReport(r) == nil {
		t.Fatal("a bad until passed the report check")
	}
}

func TestRoutinesForLeavesUntilOutForOlderAgents(t *testing.T) {
	plain := RunParams{ClassName: "a.b.CatSwapPlugin"}
	until := RunParams{ClassName: "a.b.T", Until: &RunUntil{Objective: "skill:fishing>=76", Account: "acc_zezima"}}
	ts := []Trigger{{Event: EventKittenGrown, Enabled: true, Run: until}}
	rs := []Recurring{{Name: "plain", Run: plain}, {Name: "until", Run: until}}
	old := RoutinesFor(Info{Commands: []string{CmdRoutinesSync, CmdRecurringGet}}, ts, rs)
	if len(old.Triggers) != 0 || len(old.Recurring) != 1 || old.Recurring[0].Name != "plain" {
		t.Fatalf("older agent got %+v", old)
	}
	cur := RoutinesFor(Info{Commands: []string{CmdRoutinesSync, CmdRecurringGet, CmdRunUntil}}, ts, rs)
	if len(cur.Triggers) != 1 || len(cur.Recurring) != 2 {
		t.Fatalf("current agent got %+v", cur)
	}
	if b, _ := json.Marshal(RoutinesFor(Info{}, nil, nil)); string(b) != `{"triggers":[]}` {
		t.Fatalf("empty = %s", b)
	}
}
