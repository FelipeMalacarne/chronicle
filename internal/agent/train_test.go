package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

func trainParams(mode string, risks []string, supervised bool, tr *execspec.Train) TaskRunParams {
	return TaskRunParams{
		Key: "task:0#0", Plan: "pln_abcdefgh", Label: "Treinar " + tr.SkillName, Mode: mode, Risks: risks,
		Supervised: supervised, Account: "acc_zezima", Task: execspec.Task{Type: execspec.TypeTrain, Train: tr},
	}
}

func fishingTrain() *execspec.Train {
	return &execspec.Train{Skill: "skill:fishing", SkillName: "Fishing", Level: 76, Plugin: trainer,
		Config: []clientcfg.Setting{{Group: "fishingtrainer", Key: "spot", Value: "BARBARIAN"}}}
}

func fightingTrain() *execspec.Train {
	return &execspec.Train{Skill: "skill:attack", SkillName: "Attack", Level: 40, Plugin: "a.b.Fighter", Combat: true}
}

func TestValidateTrainRun(t *testing.T) {
	ok := []TaskRunParams{
		trainParams("ironman", nil, false, fishingTrain()),
		trainParams("hcim", nil, false, fishingTrain()),
		trainParams("ironman", []string{"combat"}, false, fightingTrain()),
		trainParams("hcim", []string{"combat"}, true, fightingTrain()),
		func() TaskRunParams {
			p := trainParams("ironman", nil, false, fishingTrain())
			p.TimeoutS = 43200
			return p
		}(),
	}
	for i, p := range ok {
		if _, err := ValidateParams(CmdTaskRun, raw(t, p)); err != nil {
			t.Errorf("ok %d refused: %v", i, err)
		}
	}
	mod := func(f func(*TaskRunParams)) TaskRunParams {
		p := trainParams("ironman", nil, false, fishingTrain())
		f(&p)
		return p
	}
	for _, tc := range []struct {
		name string
		p    TaskRunParams
		want string
	}{
		{"no account", mod(func(p *TaskRunParams) { p.Account = "" }), "needs the account"},
		{"bad account", mod(func(p *TaskRunParams) { p.Account = "zezima" }), "bad account"},
		{"timeout", mod(func(p *TaskRunParams) { p.TimeoutS = 43201 }), "timeoutS between 60 and 43200"},
		{"combat undeclared", trainParams("ironman", nil, false, fightingTrain()), "not declared"},
		{"hcim fighting alone", trainParams("hcim", []string{"combat"}, false, fightingTrain()), "supervised"},
		{"chronicle link", mod(func(p *TaskRunParams) { p.Task.Train.Plugin = ChronicleLinkClass }), "not controlled"},
		{"secret setting", mod(func(p *TaskRunParams) { p.Task.Train.Config[0].Key = "token" }), "secret"},
	} {
		if _, err := ValidateParams(CmdTaskRun, raw(t, tc.p)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}

// newTrainExecutor is a client with the trainer installed and off, and
// Chronicle Link's bridge answering.
func newTrainExecutor(version string) (*Executor, *runGame) {
	g := newRunGame("")
	g.active = map[string]bool{trainer: false}
	g.levels = map[string]int{"Fishing": 74}
	return &Executor{Game: g, Bridge: &taskBridge{version: version}, RunPoll: time.Millisecond}, g
}

func TestTrainRunStopsThePluginOnTheServersProof(t *testing.T) {
	e, g := newTrainExecutor("1.15.0")
	started := startTask(t, e, trainParams("ironman", nil, false, fishingTrain()))
	if started.ClassName != trainer || started.Task == nil || started.Task.Type != "train" ||
		started.Until == nil || started.Until.Objective != "skill:fishing>=76" || started.Until.Account != "acc_zezima" {
		t.Fatalf("started = %+v", started)
	}
	waitTrail(t, e, "Fishing: nível 74")
	g.mu.Lock()
	if g.settings["fishingtrainer.spot"] != "BARBARIAN" || !g.active[trainer] {
		t.Fatalf("settings %v, active %v", g.settings, g.active)
	}
	g.levels["Fishing"] = 76
	g.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	if out, _ := e.Execute(context.Background(), CmdRunStatus, nil); out.(RunStatus).Run.Final() {
		t.Fatal("the run ended on the client's level, not the server's proof")
	}
	proveUntil(t, e, started.UID)
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "treino feito: skill:fishing>=76 provado pelo ingest") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[trainer] {
		t.Fatal("the trainer is still on")
	}
	// Set, start, stop: nothing else acted (no walk, no other plugin).
	var posts []string
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			posts = append(posts, q)
		}
	}
	want := []string{"POST /settings/plugin ", "POST /scripts/start " + trainer, "POST /scripts/stop " + trainer}
	if strings.Join(posts, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %v, want %v", posts, want)
	}
}

func TestTrainRunRefusals(t *testing.T) {
	for _, c := range []struct {
		name    string
		version string
		setup   func(*runGame)
		params  TaskRunParams
		want    string
	}{
		{"not installed", "1.15.0", func(g *runGame) { delete(g.active, trainer) }, trainParams("ironman", nil, false, fishingTrain()), "not installed"},
		{"already on", "1.15.0", func(g *runGame) { g.active[trainer] = true }, trainParams("ironman", nil, false, fishingTrain()), "already on"},
		{"no tracking", "", func(*runGame) {}, trainParams("ironman", nil, false, fishingTrain()), "bridge did not answer"},
		{"hcim in the wilderness", "1.15.0", func(g *runGame) { g.pos = WalkParams{X: 3090, Y: 3956} }, trainParams("hcim", nil, false, fishingTrain()), "wilderness"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, g := newTrainExecutor(c.version)
			g.mu.Lock()
			c.setup(g)
			g.mu.Unlock()
			startTask(t, e, c.params)
			r := waitRun(t, e)
			if r.State != RunFailed || !strings.Contains(r.Error, c.want) {
				t.Fatalf("run = %+v", r)
			}
			g.mu.Lock()
			defer g.mu.Unlock()
			for _, q := range g.requests {
				if strings.HasPrefix(q, "POST") {
					t.Fatalf("a refused train acted: %v", g.requests)
				}
			}
		})
	}
}

func TestTrainRunTimeoutAndCancelStopThePlugin(t *testing.T) {
	e, g := newTrainExecutor("1.15.0")
	e.taskTimeout = 30 * time.Millisecond
	startTask(t, e, trainParams("ironman", nil, false, fishingTrain()))
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "did not reach skill:fishing>=76") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	if g.active[trainer] {
		t.Fatal("the timeout left the trainer on")
	}
	g.mu.Unlock()

	e2, g2 := newTrainExecutor("1.15.0")
	startTask(t, e2, trainParams("ironman", nil, false, fishingTrain()))
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
		t.Fatal("the cancel left the trainer on")
	}
}

func TestTrainRunAlreadyProvenStartsNothing(t *testing.T) {
	e, g := newTrainExecutor("1.15.0")
	g.mu.Lock()
	g.fight = true
	g.mu.Unlock()
	started := startTask(t, e, trainParams("ironman", nil, false, fishingTrain()))
	waitTrail(t, e, "em combate")
	proveUntil(t, e, started.UID)
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "já prova skill:fishing>=76") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			t.Fatalf("acted: %v", g.requests)
		}
	}
}
