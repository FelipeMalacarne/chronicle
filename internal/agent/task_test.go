package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

// taskGame is the Agent Server during a task run: a logged-in character
// standing at pos, out of combat and dialogue.
type taskGame struct {
	fakeGame
	mu  sync.Mutex
	pos WalkParams
}

func (g *taskGame) State(context.Context) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"loggedIn": true, "player": map[string]any{"position": g.pos}})
	return b, nil
}

// taskBridge is Chronicle Link's bridge running the plugin's machine: it
// takes a start, then answers its state through the script: each poll
// takes the next step (events to add, a final state).
type taskBridge struct {
	mu       sync.Mutex
	version  string
	started  map[string]any // the start body
	id       string
	state    string
	reason   string
	events   []map[string]any
	script   []taskStep
	canceled []string
	refuse   error // the start's answer
	lost     bool  // answer another task's id
	down     bool  // stop answering /bridge/task
}

type taskStep struct {
	text  string
	state string
}

func (b *taskBridge) JSON(_ context.Context, method, path string, _ url.Values, body any, _ time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch method + " " + path {
	case "GET /bridge/version":
		return json.RawMessage(`{"version":"` + b.version + `"}`), nil
	case "GET /dialogue":
		return json.RawMessage(`{"inDialogue":false}`), nil
	case "POST /bridge/task/start":
		if b.refuse != nil {
			return nil, b.refuse
		}
		raw, _ := json.Marshal(body)
		_ = json.Unmarshal(raw, &b.started)
		b.id, _ = b.started["id"].(string)
		b.state = "running"
		return json.RawMessage(`{"success":true}`), nil
	case "GET /bridge/task":
		if b.down {
			return nil, errors.New("connection refused")
		}
		if len(b.script) > 0 && b.state == "running" {
			s := b.script[0]
			b.script = b.script[1:]
			if s.text != "" {
				b.events = append(b.events, map[string]any{"seq": len(b.events) + 1, "text": s.text})
			}
			if s.state != "" {
				b.state = s.state
				b.reason = s.text
			}
		}
		id := b.id
		if b.lost {
			id = "0000000000000000"
		}
		out, _ := json.Marshal(map[string]any{"id": id, "type": "travel", "state": b.state, "reason": b.reason, "events": b.events})
		return out, nil
	case "POST /bridge/task/cancel":
		m, _ := body.(map[string]any)
		id, _ := m["id"].(string)
		b.canceled = append(b.canceled, id)
		if id == b.id && b.state == "running" {
			b.state = "canceled"
		}
		return json.RawMessage(`{"success":true}`), nil
	}
	return nil, errors.New("HTTP 404")
}

func travelParams(mode string, risks []string, supervised bool, travel *execspec.Travel) TaskRunParams {
	return TaskRunParams{
		Key: "step:oziris-main::1.1.3#0", Plan: "route:oziris-main", Label: "Ir até " + travel.Name, Mode: mode,
		Risks: risks, Supervised: supervised, Task: execspec.Task{Type: execspec.TypeTravel, Travel: travel},
	}
}

func lumbridge() *execspec.Travel {
	return &execspec.Travel{Location: "location:lumbridge", Name: "Lumbridge", Target: execspec.Tile{X: 3188, Y: 3220},
		Area: execspec.Area{MinX: 3178, MinY: 3210, MaxX: 3198, MaxY: 3230}}
}

func mageBankTravel() *execspec.Travel {
	return &execspec.Travel{Location: "location:mage_bank", Name: "Mage Bank", Target: execspec.Tile{X: 3090, Y: 3956},
		Area: execspec.Area{MinX: 3087, MinY: 3953, MaxX: 3093, MaxY: 3959}, Wilderness: true}
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateTaskRun(t *testing.T) {
	ok := []TaskRunParams{
		travelParams("ironman", nil, false, lumbridge()),
		travelParams("ironman", []string{"wilderness"}, false, mageBankTravel()),
		travelParams("hcim", []string{"wilderness"}, true, mageBankTravel()),
		travelParams("hcim", []string{"item_loss"}, false, lumbridge()),
	}
	for i, p := range ok {
		if _, err := ValidateParams(CmdTaskRun, raw(t, p)); err != nil {
			t.Errorf("ok %d refused: %v", i, err)
		}
	}
	mod := func(f func(*TaskRunParams)) TaskRunParams {
		p := travelParams("ironman", nil, false, lumbridge())
		f(&p)
		return p
	}
	for _, tc := range []struct {
		name string
		p    any
		want string
	}{
		{"talk", mod(func(p *TaskRunParams) { p.Task = execspec.Task{Type: "talk"} }), "not executed yet"},
		{"train", mod(func(p *TaskRunParams) { p.Task = execspec.Task{Type: "train"}; p.Account = "acc_zezima" }), "only train"},
		{"hcim in the wilderness alone", travelParams("hcim", []string{"wilderness"}, false, mageBankTravel()), "supervised"},
		{"hcim and a recipe's combat alone", travelParams("hcim", []string{"combat"}, false, lumbridge()), "supervised"},
		{"the wilderness undeclared", travelParams("ironman", nil, false, mageBankTravel()), "not declared"},
		{"mode", mod(func(p *TaskRunParams) { p.Mode = "regular" }), "mode"},
		{"risk", mod(func(p *TaskRunParams) { p.Risks = []string{"pking"} }), "risk"},
		{"repeated risk", mod(func(p *TaskRunParams) { p.Risks = []string{"combat", "combat"} }), "repeated"},
		{"key", mod(func(p *TaskRunParams) { p.Key = "1.1.3" }), "bad key"},
		{"key with space", mod(func(p *TaskRunParams) { p.Key = "step:a b#0" }), "bad key"},
		{"plan", mod(func(p *TaskRunParams) { p.Plan = "../etc" }), "bad plan"},
		{"label", mod(func(p *TaskRunParams) { p.Label = "a\nb" }), "label"},
		{"timeout", mod(func(p *TaskRunParams) { p.TimeoutS = 10 }), "timeoutS"},
		{"target outside", mod(func(p *TaskRunParams) { p.Task.Travel.Target = execspec.Tile{X: 0, Y: 0} }), "not inside"},
		{"unknown field", map[string]any{"key": "task:0#0", "plan": "pln_abcdefgh", "label": "x", "mode": "ironman", "click": true}, "unknown field"},
	} {
		if _, err := ValidateParams(CmdTaskRun, raw(t, tc.p)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}

func newTaskExecutor(b *taskBridge, pos WalkParams) (*Executor, *taskGame) {
	g := &taskGame{pos: pos}
	return &Executor{Game: g, Bridge: b, RunPoll: time.Millisecond}, g
}

func startTask(t *testing.T, e *Executor, p TaskRunParams) *Run {
	t.Helper()
	out, err := e.Execute(context.Background(), CmdTaskRun, raw(t, p))
	if err != nil {
		t.Fatal(err)
	}
	return out.(RunStatus).Run
}

func TestTaskRunEndsWhenTheMachineDoesAndLeavesTheProofToTheIngest(t *testing.T) {
	b := &taskBridge{version: "1.15.0", script: []taskStep{{text: "andando até Lumbridge"}, {}, {text: "na área de Lumbridge", state: "done"}}}
	e, _ := newTaskExecutor(b, WalkParams{X: 3103, Y: 3265})
	var reports []Run
	var rmu sync.Mutex
	e.OnRun = func(r Run) { rmu.Lock(); reports = append(reports, r); rmu.Unlock() }
	started := startTask(t, e, travelParams("ironman", nil, false, lumbridge()))
	if started.Task == nil || started.Task.Type != "travel" || started.ClassName != ChronicleLinkClass || started.State != RunWaiting {
		t.Fatalf("started = %+v", started)
	}
	r := waitRun(t, e)
	if r.State != RunDone || r.Task == nil || r.Task.Key != "step:oziris-main::1.1.3#0" {
		t.Fatalf("run = %+v", r)
	}
	var trail []string
	for _, ev := range r.Events {
		trail = append(trail, ev.Text)
	}
	joined := strings.Join(trail, " | ")
	for _, want := range []string{"tarefa: Ir até Lumbridge", "em 3103, 3265, 0", "plugins ligados durante a tarefa: QoLPlugin", "andando até Lumbridge", "na área de Lumbridge", "provado pelo ingest"} {
		if !strings.Contains(joined, want) {
			t.Errorf("trail %q lacks %q", joined, want)
		}
	}
	b.mu.Lock()
	if b.id != r.UID || b.started["task"].(map[string]any)["type"] != "travel" {
		t.Errorf("start body = %v (uid %s)", b.started, r.UID)
	}
	b.mu.Unlock()
	if err := ValidateReport(*r); err != nil {
		t.Errorf("the run's report is refused: %v", err)
	}
	rmu.Lock()
	defer rmu.Unlock()
	if len(reports) == 0 || reports[len(reports)-1].Task == nil {
		t.Errorf("reports = %d", len(reports))
	}
}

func TestTaskRunFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bridge   *taskBridge
		pos      WalkParams
		p        TaskRunParams
		timeout  time.Duration
		want     string
		canceled bool // the bridge saw a cancel
		started  bool
	}{
		{
			name: "the machine fails", started: true,
			bridge: &taskBridge{version: "1.15.0", script: []taskStep{{text: "o webwalker não achou caminho", state: "failed"}}},
			pos:    WalkParams{X: 3103, Y: 3265}, p: travelParams("ironman", nil, false, lumbridge()), want: "não achou caminho",
		},
		{
			name: "an old plugin", bridge: &taskBridge{version: "1.14.0"},
			pos: WalkParams{X: 3103, Y: 3265}, p: travelParams("ironman", nil, false, lumbridge()), want: "1.15.0 or newer",
		},
		{
			name: "the bridge refuses the start", bridge: &taskBridge{version: "1.15.1", refuse: errors.New("HTTP 409: a task is running")},
			pos: WalkParams{X: 3103, Y: 3265}, p: travelParams("ironman", nil, false, lumbridge()), want: "did not start",
		},
		{
			name: "an HCIM standing in the wilderness, alone", bridge: &taskBridge{version: "1.15.0"},
			pos: WalkParams{X: 3093, Y: 3957}, p: travelParams("hcim", nil, false, lumbridge()), want: "in the wilderness",
		},
		{
			name: "the bridge loses the task", started: true,
			bridge: &taskBridge{version: "1.15.0", lost: true},
			pos:    WalkParams{X: 3103, Y: 3265}, p: travelParams("ironman", nil, false, lumbridge()), want: "lost the task",
		},
		{
			name: "the bridge goes away", started: true, canceled: true,
			bridge: &taskBridge{version: "1.15.0", down: true},
			pos:    WalkParams{X: 3103, Y: 3265}, p: travelParams("ironman", nil, false, lumbridge()), want: "stopped answering",
		},
		{
			name: "the timeout cancels the machine", started: true, canceled: true, timeout: 20 * time.Millisecond,
			bridge: &taskBridge{version: "1.15.0"},
			pos:    WalkParams{X: 3103, Y: 3265}, p: travelParams("ironman", nil, false, lumbridge()), want: "did not finish",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newTaskExecutor(tc.bridge, tc.pos)
			e.taskTimeout = tc.timeout
			startTask(t, e, tc.p)
			r := waitRun(t, e)
			if r.State != RunFailed || !strings.Contains(r.Error, tc.want) {
				t.Fatalf("run = %s %q, want failed with %q", r.State, r.Error, tc.want)
			}
			tc.bridge.mu.Lock()
			defer tc.bridge.mu.Unlock()
			if (tc.bridge.id != "") != tc.started {
				t.Errorf("started on the bridge = %v", tc.bridge.id != "")
			}
			if (len(tc.bridge.canceled) > 0) != tc.canceled {
				t.Errorf("canceled on the bridge = %v", tc.bridge.canceled)
			}
		})
	}
}

func TestTaskRunCancelStopsTheMachine(t *testing.T) {
	b := &taskBridge{version: "1.15.0"}
	e, _ := newTaskExecutor(b, WalkParams{X: 3103, Y: 3265})
	startTask(t, e, travelParams("ironman", nil, false, lumbridge()))
	// While it runs, actions are refused and reads go through.
	deadline := time.Now().Add(5 * time.Second)
	for {
		b.mu.Lock()
		started := b.id != ""
		b.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never started")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := e.Execute(context.Background(), CmdWalk, json.RawMessage(`{"x":3200,"y":3200}`)); !errors.Is(err, ErrRunGoing) {
		t.Errorf("walk during a task = %v", err)
	}
	if _, err := e.Execute(context.Background(), CmdTaskRun, raw(t, travelParams("ironman", nil, false, lumbridge()))); !errors.Is(err, ErrRunGoing) {
		t.Errorf("a second task = %v", err)
	}
	if _, err := e.Execute(context.Background(), CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunCanceled {
		t.Fatalf("run = %+v", r)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.canceled) == 0 || b.canceled[0] != r.UID {
		t.Errorf("bridge cancels = %v", b.canceled)
	}
}

func TestTaskRunNeedsTheBridge(t *testing.T) {
	e := &Executor{Game: &taskGame{}, RunPoll: time.Millisecond}
	if _, err := e.Execute(context.Background(), CmdTaskRun, raw(t, travelParams("ironman", nil, false, lumbridge()))); err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Fatalf("no bridge = %v", err)
	}
}

func TestRunReportWithATask(t *testing.T) {
	r := Run{ID: "run-1", UID: "0123456789abcdef", ClassName: ChronicleLinkClass, State: RunDone, StartedAt: time.Now(),
		Task: &RunTask{Key: "task:0#0", Plan: "pln_abcdefgh", Type: "travel", Label: "Ir até Lumbridge"}}
	if err := ValidateReport(r); err != nil {
		t.Fatal(err)
	}
	r.Task.Key = "nope"
	if err := ValidateReport(r); err == nil {
		t.Fatal("a bad task key passes")
	}
}

func TestVersionAtLeast(t *testing.T) {
	for _, tc := range []struct {
		v    string
		want bool
	}{
		{"1.15.0", true}, {"1.15.1", true}, {"1.16", true}, {"2.0.0", true}, {"1.15", true},
		{"1.14.9", false}, {"1.9.0", false}, {"", false}, {"dev", false}, {"1.15.0-rc1", false},
	} {
		if got := versionAtLeast(tc.v, TaskBridgeVersion); got != tc.want {
			t.Errorf("versionAtLeast(%q) = %v", tc.v, got)
		}
	}
}
