package agent

import (
	"context"
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
