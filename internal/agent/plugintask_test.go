package agent

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

const (
	tutorial   = clientcfg.TutorialIslandClass
	tutorialID = "job_00000000000000a1"
	tutorialUp = "varp:281>=1000"
)

func tutorialParams(mod func(*TaskRunParams)) TaskRunParams {
	p := TaskRunParams{
		JobID: tutorialID, Label: "Tutorial Island", Mode: "regular", Account: "acc_appendonly",
		Task: execspec.Task{Type: execspec.TypePlugin, Plugin: &execspec.Plugin{
			Plugin: tutorial, Name: "Tutorial Island", Until: tutorialUp,
			Config: []clientcfg.Setting{{Group: "MicroTutIsland", Key: "toggleMusic", Value: "false"}},
		}},
	}
	if mod != nil {
		mod(&p)
	}
	return p
}

func TestValidatePluginTaskRun(t *testing.T) {
	if _, err := ValidateParams(CmdTaskRun, raw(t, tutorialParams(nil))); err != nil {
		t.Fatalf("a plugin task refused: %v", err)
	}
	for _, tc := range []struct {
		name string
		mod  func(*TaskRunParams)
		want string
	}{
		{"no job", func(p *TaskRunParams) { p.JobID = "" }, "needs its job"},
		{"a plan key", func(p *TaskRunParams) { p.Key = "task:0#0" }, "no plan key"},
		{"no account", func(p *TaskRunParams) { p.Account = "" }, "needs the account"},
		{"short timeout", func(p *TaskRunParams) { p.TimeoutS = 30 }, "timeoutS between 60"},
		{"protected plugin", func(p *TaskRunParams) { p.Task.Plugin.Plugin = ChronicleLinkClass }, "not controlled"},
		{"a setting the plugin does not take", func(p *TaskRunParams) { p.Task.Plugin.Config[0].Key = "ironman" }, "not a setting"},
		{"a value it does not take", func(p *TaskRunParams) { p.Task.Plugin.Config[0].Value = "maybe" }, "true, false"},
		{"another group", func(p *TaskRunParams) { p.Task.Plugin.Config[0].Group = "runelite" }, "not edited"},
		{"no until", func(p *TaskRunParams) { p.Task.Plugin.Until = "" }, "until"},
		{"a loose until", func(p *TaskRunParams) { p.Task.Plugin.Until = "varp:281>=01000" }, "until"},
	} {
		if _, err := ValidateParams(CmdTaskRun, raw(t, tutorialParams(tc.mod))); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}

func newTutorialExecutor() (*Executor, *runGame) {
	g := newRunGame("")
	g.active = map[string]bool{tutorial: false}
	return &Executor{Game: g, Bridge: &taskBridge{version: "1.15.0"}, RunPoll: time.Millisecond}, g
}

func posts(g *runGame) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []string
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			out = append(out, q)
		}
	}
	return out
}

// The plugin goes on with its settings, stays on while the server has not
// proven the goal, and goes off on the proof (a var label, not a key).
func TestPluginTaskRunsUntilTheServerProvesTheGoal(t *testing.T) {
	e, g := newTutorialExecutor()
	started := startTask(t, e, tutorialParams(nil))
	if started.ClassName != tutorial || started.Task == nil || started.Task.Type != "plugin" || started.Task.JobID != tutorialID ||
		started.Until == nil || started.Until.Objective != tutorialUp || started.Until.Account != "acc_appendonly" {
		t.Fatalf("started = %+v", started)
	}
	waitTrail(t, e, "iniciado")
	time.Sleep(20 * time.Millisecond)
	g.mu.Lock()
	on := g.active[tutorial] && g.settings["MicroTutIsland.toggleMusic"] == "false"
	g.mu.Unlock()
	if !on {
		t.Fatalf("the plugin is not on with its settings: %v", g.settings)
	}
	if out, _ := e.Execute(context.Background(), CmdRunStatus, nil); out.(RunStatus).Run.Final() {
		t.Fatal("the run ended before the server's proof")
	}
	if _, err := e.Execute(context.Background(), CmdRunUntil, raw(t, RunUntilParams{UID: started.UID, Objective: tutorialUp, Evidence: "observado 1000"})); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "feito: "+tutorialUp+" provado pelo ingest") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	still := g.active[tutorial]
	g.mu.Unlock()
	if still {
		t.Fatal("the plugin is still on")
	}
	want := []string{"POST /settings/plugin ", "POST /scripts/start " + tutorial, "POST /scripts/stop " + tutorial}
	if got := posts(g); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

func TestPluginTaskCancelStopsThePlugin(t *testing.T) {
	e, g := newTutorialExecutor()
	startTask(t, e, tutorialParams(nil))
	waitTrail(t, e, "iniciado")
	if _, err := e.Execute(context.Background(), CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, e); r.State != RunCanceled {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[tutorial] {
		t.Fatal("the cancel left the plugin on")
	}
}

func TestPluginTaskTimeoutAndRefusals(t *testing.T) {
	e, g := newTutorialExecutor()
	e.taskTimeout = 30 * time.Millisecond
	startTask(t, e, tutorialParams(nil))
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "did not reach "+tutorialUp) {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	if g.active[tutorial] {
		t.Fatal("the timeout left the plugin on")
	}
	g.mu.Unlock()

	for name, setup := range map[string]func(*runGame){
		"not installed": func(g *runGame) { delete(g.active, tutorial) },
		"already on":    func(g *runGame) { g.active[tutorial] = true },
	} {
		e, g := newTutorialExecutor()
		g.mu.Lock()
		setup(g)
		g.mu.Unlock()
		startTask(t, e, tutorialParams(nil))
		want := strings.ReplaceAll(name, "already on", "already on")
		if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, want[:min(len(want), 10)]) || len(posts(g)) != 0 {
			t.Errorf("%s: run = %+v, posts %v", name, r, posts(g))
		}
	}
}

// An agent without the feature is told apart by what it declares in its hello.
func TestPluginTaskIsAnAgentFeature(t *testing.T) {
	if !slices.Contains(AgentFeatures, AgentFeaturePlugin) {
		t.Fatalf("the agent does not declare %q: %v", AgentFeaturePlugin, AgentFeatures)
	}
}

// Microbot's message window, with every script paused, ends the run with its
// text instead of the timeout: the agent closes it and stops the plugin
// (FEL-134).
func TestPluginTaskEndsOnAClientMessage(t *testing.T) {
	e, g := newTutorialExecutor()
	b := e.Bridge.(*taskBridge)
	startTask(t, e, tutorialParams(nil))
	waitTrail(t, e, "iniciado")
	b.mu.Lock()
	b.messages = []string{"Your bot tried to interact with an NPC for 3 times but failed."}
	b.mu.Unlock()
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "tried to interact with an NPC") {
		t.Fatalf("run = %+v", r)
	}
	b.mu.Lock()
	closes := b.closes
	b.mu.Unlock()
	g.mu.Lock()
	on := g.active[tutorial]
	g.mu.Unlock()
	if closes != 1 || on {
		t.Fatalf("closes %d, plugin on %v", closes, on)
	}
	if !slices.ContainsFunc(r.Events, func(ev RunEvent) bool { return strings.Contains(ev.Text, "(janela fechada)") }) {
		t.Fatalf("trail = %+v", r.Events)
	}
}

// dialogueGame is a runGame with a dialogue open at the start: lines to click
// through, or options to choose when options is set.
type dialogueGame struct {
	*runGame
	lines   int
	options bool
}

func (g *dialogueGame) JSON(ctx context.Context, method, path string, q url.Values, body any, t time.Duration) (json.RawMessage, error) {
	switch method + " " + path {
	case "GET /dialogue":
		g.mu.Lock()
		defer g.mu.Unlock()
		open := g.options || g.lines > 0
		b, _ := json.Marshal(map[string]any{"inDialogue": open, "hasContinue": open && !g.options, "hasOptions": g.options})
		return b, nil
	case "POST /dialogue/continue":
		g.mu.Lock()
		defer g.mu.Unlock()
		g.lines--
		g.requests = append(g.requests, "POST /dialogue/continue ")
		return json.RawMessage(`{"success":true}`), nil
	}
	return g.runGame.JSON(ctx, method, path, q, body, t)
}

func newDialogueExecutor(lines int, options bool) (*Executor, *dialogueGame) {
	e, g := newTutorialExecutor()
	d := &dialogueGame{runGame: g, lines: lines, options: options}
	e.Game = d
	return e, d
}

// A job's plugin handles the dialogue it meets: the lines that only continue are clicked
// through before the start instead of holding it (FEL-130).
func TestPluginTaskContinuesADialogueBeforeTheStart(t *testing.T) {
	e, g := newDialogueExecutor(3, false)
	startTask(t, e, tutorialParams(nil))
	waitTrail(t, e, "iniciado")
	want := []string{"POST /dialogue/continue ", "POST /dialogue/continue ", "POST /dialogue/continue ", "POST /settings/plugin ", "POST /scripts/start " + tutorial}
	if got := posts(g.runGame); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	out, _ := e.Execute(context.Background(), CmdRunStatus, nil)
	var trail []string
	for _, ev := range out.(RunStatus).Run.Events {
		trail = append(trail, ev.Text)
	}
	if !slices.Contains(trail, "avançando o diálogo") || slices.Contains(trail, "esperando: "+whyDialogue) {
		t.Fatalf("trail = %q", trail)
	}
}

// A dialogue with options is a choice the plugin did not make: the run waits,
// and gives up at the safe point's limit without touching the game.
func TestPluginTaskWaitsForADialogueWithOptions(t *testing.T) {
	e, g := newDialogueExecutor(0, true)
	e.SafeWait = 30 * time.Millisecond
	startTask(t, e, tutorialParams(nil))
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, whyDialogue) {
		t.Fatalf("run = %+v", r)
	}
	if got := posts(g.runGame); len(got) != 0 {
		t.Fatalf("touched the game in a dialogue with options: %v", got)
	}
}
