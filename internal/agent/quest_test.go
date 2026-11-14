package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/microbot"
)

// questGame is a logged-in character in a client whose Quest Helper the
// test plays, as the pinned client's Agent Server answers it
// (QuestHelperHandler): /quest-helper/status, /list, /start and /stop, and
// the settings the run writes; the rest is a runGame's.
type questGame struct {
	*runGame
	qmu       sync.Mutex
	missing   bool   // an Agent Server without /quest-helper (404)
	active    bool   // the Quest Helper plugin is on
	turnOn    string // "Enable QuestHelper" as stored; "" is unset (on)
	selected  string // the quest the helper follows
	completed bool
	step      string
	listed    []string
	keepNot   bool     // the client does not keep a setting written
	posts     []string // what the run did, in order
}

func newQuestGame() *questGame {
	g := newRunGame("")
	g.active = map[string]bool{clientcfg.QuestHelperClass: true}
	return &questGame{runGame: g, active: true, listed: []string{"Cook's Assistant", "Sheep Shearer", "X Marks the Spot"}}
}

func (g *questGame) JSON(ctx context.Context, method, path string, q url.Values, body any, d time.Duration) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/quest-helper") && method+" "+path != "POST /settings/plugin" {
		return g.runGame.JSON(ctx, method, path, q, body, d)
	}
	g.qmu.Lock()
	defer g.qmu.Unlock()
	if g.missing && strings.HasPrefix(path, "/quest-helper") {
		return nil, &microbot.APIError{Path: path, Status: http.StatusNotFound, Message: "No context found for request"}
	}
	m, _ := body.(map[string]any)
	switch method + " " + path {
	case "GET /quest-helper/status":
		st := map[string]any{"active": g.active, "enabled": g.active, "selected": g.selected != ""}
		if g.turnOn != "" {
			st["turnOn"] = g.turnOn
		}
		if g.selected != "" {
			st["questName"], st["completed"], st["stepText"], st["stepClass"] = g.selected, g.completed, g.step, "NpcStep"
		}
		return json.Marshal(st)
	case "GET /quest-helper/list":
		var qs []map[string]any
		for _, n := range g.listed {
			qs = append(qs, map[string]any{"enum": strings.ToUpper(n), "displayName": n})
		}
		return json.Marshal(map[string]any{"count": len(qs), "quests": qs})
	case "POST /quest-helper/start":
		name, _ := m["name"].(string)
		g.posts = append(g.posts, "start "+name)
		if !g.active {
			return nil, &microbot.APIError{Path: path, Status: http.StatusConflict, Message: "Quest Helper plugin is not active"}
		}
		if !slices.Contains(g.listed, name) {
			return nil, &microbot.APIError{Path: path, Status: http.StatusNotFound, Message: "Quest helper not found or not loaded: " + name}
		}
		g.selected, g.completed = name, false
		return json.Marshal(map[string]any{"success": true, "questName": name})
	case "POST /quest-helper/stop":
		g.posts = append(g.posts, "stop")
		g.selected = ""
		return json.RawMessage(`{"success":true}`), nil
	case "POST /settings/plugin":
		group, _ := m["group"].(string)
		key, _ := m["key"].(string)
		value := ""
		switch v := m["value"].(type) {
		case string:
			value = v
		case *string:
			value = *v
		}
		g.posts = append(g.posts, "set "+group+"."+key+"="+value)
		if g.keepNot {
			value = "ASK"
		} else if group == clientcfg.QuestHelperGroup && key == clientcfg.QuestTurnOn {
			g.turnOn = value
		}
		return json.Marshal(map[string]any{"group": group, "key": key, "value": value, "success": true})
	}
	return nil, &microbot.APIError{Path: path, Status: http.StatusNotFound, Message: "Unknown endpoint"}
}

func (g *questGame) set(f func(*questGame)) {
	g.qmu.Lock()
	defer g.qmu.Unlock()
	f(g)
}

func (g *questGame) actions() []string {
	g.qmu.Lock()
	defer g.qmu.Unlock()
	return slices.Clone(g.posts)
}

// acted lists every POST the run sent: the Quest Helper's and the rest.
func (g *questGame) acted() []string {
	out := g.actions()
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			out = append(out, q)
		}
	}
	return out
}

func cooksQuest() *execspec.Quest {
	return &execspec.Quest{Quest: "quest:cooks_assistant", Name: "Cook's Assistant", Helper: "Cook's Assistant"}
}

func questParams(mode string, risks []string, supervised bool, q *execspec.Quest) TaskRunParams {
	return TaskRunParams{
		Key: "recipe:quest:cooks_assistant@finished#0", Plan: "pln_abcdefgh", Label: "Fazer a quest " + q.Name, Mode: mode, Risks: risks,
		Supervised: supervised, Account: "acc_zezima", Task: execspec.Task{Type: execspec.TypeQuest, Quest: q},
	}
}

var questRisks = []string{"combat", "hazard"}

func newQuestExecutor(version string) (*Executor, *questGame) {
	g := newQuestGame()
	return &Executor{Game: g, Bridge: &taskBridge{version: version}, RunPoll: time.Millisecond}, g
}

func proveQuest(t *testing.T, e *Executor, uid string) {
	t.Helper()
	p := RunUntilParams{UID: uid, Objective: "quest:cooks_assistant@finished", Evidence: "observado FINISHED"}
	if _, err := e.Execute(context.Background(), CmdRunUntil, raw(t, p)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateQuestRun(t *testing.T) {
	safe := cooksQuest()
	safe.Safe = true
	for i, p := range []TaskRunParams{
		questParams("ironman", questRisks, false, cooksQuest()),
		questParams("hcim", questRisks, true, cooksQuest()),
		questParams("hcim", nil, false, safe),
		func() TaskRunParams {
			p := questParams("gim", questRisks, false, cooksQuest())
			p.TimeoutS = 43200
			return p
		}(),
	} {
		if _, err := ValidateParams(CmdTaskRun, raw(t, p)); err != nil {
			t.Errorf("ok %d refused: %v", i, err)
		}
	}
	mod := func(f func(*TaskRunParams)) TaskRunParams {
		p := questParams("ironman", questRisks, false, cooksQuest())
		f(&p)
		return p
	}
	for _, tc := range []struct {
		name string
		p    TaskRunParams
		want string
	}{
		{"no account", mod(func(p *TaskRunParams) { p.Account = "" }), "a quest needs the account"},
		{"timeout", mod(func(p *TaskRunParams) { p.TimeoutS = 43201 }), "quest's timeoutS between 60 and 43200"},
		{"risks undeclared", questParams("ironman", nil, false, cooksQuest()), "risk of the task is not declared"},
		{"hcim alone", questParams("hcim", questRisks, false, cooksQuest()), "supervised"},
		{"no quest", mod(func(p *TaskRunParams) { p.Task.Quest = nil }), "only quest"},
		{"bad ref", mod(func(p *TaskRunParams) { p.Task.Quest.Quest = "quest:Cook" }), "quest ref"},
		{"multi-line name", mod(func(p *TaskRunParams) { p.Task.Quest.Helper = "Cook's\nAssistant" }), "one line"},
	} {
		if _, err := ValidateParams(CmdTaskRun, raw(t, tc.p)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}

// TestQuestRunStopsTheBotOnTheServersProof: the run sets "Obtain Missing
// Items" to No, selects the quest and follows the helper's steps; the
// client calling the quest completed ends nothing, the server's proof does,
// and stops the helper.
func TestQuestRunStopsTheBotOnTheServersProof(t *testing.T) {
	e, g := newQuestExecutor("1.15.0")
	started := startTask(t, e, questParams("ironman", questRisks, false, cooksQuest()))
	if started.ClassName != clientcfg.QuestHelperClass || started.Task == nil || started.Task.Type != "quest" ||
		started.Until == nil || started.Until.Objective != "quest:cooks_assistant@finished" || started.Until.Account != "acc_zezima" {
		t.Fatalf("started = %+v", started)
	}
	waitTrail(t, e, "iniciado: Cook's Assistant no Quest Helper")
	if got := g.actions(); strings.Join(got, "|") != "set questhelper.obtainMissingItems=NO|start Cook's Assistant" {
		t.Fatalf("actions = %v, want the setting before the start", got)
	}
	g.set(func(g *questGame) { g.step = "Talk to the Cook in\nLumbridge Castle." })
	waitTrail(t, e, "passo do Quest Helper: Talk to the Cook in Lumbridge Castle.")
	time.Sleep(20 * time.Millisecond)
	if out, _ := e.Execute(context.Background(), CmdRunStatus, nil); out.(RunStatus).Run.Final() {
		t.Fatal("the run ended without the server's proof")
	}
	// Reads go on during the run; actions wait for it.
	if _, err := e.Execute(context.Background(), CmdQuestHelper, nil); err != nil {
		t.Fatalf("quest_helper during the run: %v", err)
	}
	if _, err := e.Execute(context.Background(), CmdScriptStart, raw(t, ScriptParams{ClassName: fisher})); err == nil {
		t.Fatal("an action ran during the quest")
	}
	proveQuest(t, e, started.UID)
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "quest feita: quest:cooks_assistant@finished provado pelo ingest") {
		t.Fatalf("run = %+v", r)
	}
	want := []string{"set questhelper.obtainMissingItems=NO", "start Cook's Assistant", "stop"}
	if got := g.acted(); !slices.Equal(got, want) {
		t.Fatalf("acted = %v, want %v (nothing else: no walk, no plugin)", got, want)
	}
	var trail []string
	for _, ev := range r.Events {
		trail = append(trail, ev.Text)
	}
	if s := strings.Join(trail, "\n"); strings.Contains(s, "plugins ligados") || !strings.Contains(s, "obtainMissingItems = NO") {
		t.Fatalf("trail:\n%s", s)
	}
}

// TestQuestRunSwitchesTheBotOnAndBack: after the bot gave up once ("Enable
// QuestHelper" off), the run switches it on after selecting the quest and
// off again when it ends (here, the owner's cancel).
func TestQuestRunSwitchesTheBotOnAndBack(t *testing.T) {
	e, g := newQuestExecutor("1.15.0")
	g.set(func(g *questGame) { g.turnOn, g.selected = "false", "Sheep Shearer" })
	startTask(t, e, questParams("ironman", questRisks, false, cooksQuest()))
	waitTrail(t, e, "iniciado")
	if _, err := e.Execute(context.Background(), CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, e); r.State != RunCanceled {
		t.Fatalf("run = %+v", r)
	}
	want := []string{"set questhelper.obtainMissingItems=NO", "start Cook's Assistant", "set questhelper.TurnOn=true", "stop", "set questhelper.TurnOn=false"}
	if got := g.actions(); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

func TestQuestRunRefusals(t *testing.T) {
	for _, c := range []struct {
		name    string
		version string
		setup   func(*questGame)
		params  TaskRunParams
		want    string
	}{
		{"an older client", "1.15.0", func(g *questGame) { g.missing = true }, questParams("ironman", questRisks, false, cooksQuest()), "has no Quest Helper"},
		{"helper off", "1.15.0", func(g *questGame) { g.active = false }, questParams("ironman", questRisks, false, cooksQuest()), "off in the client"},
		{"bot busy", "1.15.0", func(g *questGame) { g.selected = "Sheep Shearer" }, questParams("ironman", questRisks, false, cooksQuest()), "already doing Sheep Shearer"},
		{"not listed", "1.15.0", func(g *questGame) { g.listed = g.listed[1:] }, questParams("ironman", questRisks, false, cooksQuest()), "does not list"},
		{"no tracking", "", func(*questGame) {}, questParams("ironman", questRisks, false, cooksQuest()), "bridge did not answer"},
		{"hcim in the wilderness", "1.15.0", func(g *questGame) {
			g.mu.Lock()
			g.pos = WalkParams{X: 3090, Y: 3956}
			g.mu.Unlock()
		}, questParams("hcim", nil, false, func() *execspec.Quest { q := cooksQuest(); q.Safe = true; return q }()), "wilderness"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, g := newQuestExecutor(c.version)
			g.set(c.setup)
			startTask(t, e, c.params)
			r := waitRun(t, e)
			if r.State != RunFailed || !strings.Contains(r.Error, c.want) {
				t.Fatalf("run = %+v", r)
			}
			if got := g.acted(); len(got) > 0 {
				t.Fatalf("a refused quest acted: %v", got)
			}
		})
	}
	// The client not keeping "No" stops it before the quest is selected.
	e, g := newQuestExecutor("1.15.0")
	g.set(func(g *questGame) { g.keepNot = true })
	startTask(t, e, questParams("ironman", questRisks, false, cooksQuest()))
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "did not keep questhelper.obtainMissingItems = NO") {
		t.Fatalf("run = %+v", r)
	}
	if got := g.actions(); !slices.Equal(got, []string{"set questhelper.obtainMissingItems=NO"}) {
		t.Fatalf("actions = %v", got)
	}
}

func TestQuestRunTimeoutStopsTheBot(t *testing.T) {
	e, g := newQuestExecutor("1.15.0")
	e.taskTimeout = 30 * time.Millisecond
	startTask(t, e, questParams("ironman", questRisks, false, cooksQuest()))
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "did not reach quest:cooks_assistant@finished") {
		t.Fatalf("run = %+v", r)
	}
	if got := g.actions(); len(got) == 0 || got[len(got)-1] != "stop" {
		t.Fatalf("actions = %v, want the stop last", got)
	}
}

// TestQuestRunBotStoppingByItself: the bot giving up fails the run after
// the grace period without the proof; the helper calling the quest
// completed waits for the proof, which ends it.
func TestQuestRunBotStoppingByItself(t *testing.T) {
	e, g := newQuestExecutor("1.15.0")
	e.untilGrace = 20 * time.Millisecond
	startTask(t, e, questParams("ironman", questRisks, false, cooksQuest()))
	waitTrail(t, e, "iniciado")
	g.set(func(g *questGame) { g.turnOn = "false" })
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "QuestHelperPlugin stopped and the ingest did not prove quest:cooks_assistant@finished") {
		t.Fatalf("run = %+v", r)
	}
	var trail []string
	for _, ev := range r.Events {
		trail = append(trail, ev.Text)
	}
	if s := strings.Join(trail, "\n"); !strings.Contains(s, "desligou \"Enable QuestHelper\"") {
		t.Fatalf("trail:\n%s", s)
	}
	if got := g.actions(); got[len(got)-1] != "stop" {
		t.Fatalf("actions = %v, want the stop last", got)
	}

	e2, g2 := newQuestExecutor("1.15.0")
	e2.untilGrace = 5 * time.Second
	started := startTask(t, e2, questParams("ironman", questRisks, false, cooksQuest()))
	waitTrail(t, e2, "iniciado")
	g2.set(func(g *questGame) { g.completed = true })
	waitTrail(t, e2, "diz que a quest terminou")
	proveQuest(t, e2, started.UID)
	if r := waitRun(t, e2); r.State != RunDone {
		t.Fatalf("run = %+v", r)
	}
	if got := g2.actions(); got[len(got)-1] != "stop" {
		t.Fatalf("actions = %v, want the stop last", got)
	}
}

func TestQuestRunAlreadyProvenStartsNothing(t *testing.T) {
	e, g := newQuestExecutor("1.15.0")
	g.mu.Lock()
	g.fight = true
	g.mu.Unlock()
	started := startTask(t, e, questParams("ironman", questRisks, false, cooksQuest()))
	waitTrail(t, e, "em combate")
	proveQuest(t, e, started.UID)
	r := waitRun(t, e)
	if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "já prova quest:cooks_assistant@finished") {
		t.Fatalf("run = %+v", r)
	}
	if got := g.acted(); len(got) > 0 {
		t.Fatalf("acted: %v", got)
	}
}

func TestQuestHelperCommand(t *testing.T) {
	e, g := newQuestExecutor("1.15.0")
	out, err := e.Execute(context.Background(), CmdQuestHelper, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info := out.(QuestHelperInfo); !info.Active || info.Running || info.Selected != "" || !slices.Contains(info.Quests, "Cook's Assistant") {
		t.Fatalf("info = %+v", info)
	}
	g.set(func(g *questGame) { g.selected = "Sheep Shearer" })
	if out, _ = e.Execute(context.Background(), CmdQuestHelper, nil); !out.(QuestHelperInfo).Running || out.(QuestHelperInfo).Selected != "Sheep Shearer" {
		t.Fatalf("info = %+v", out)
	}
	g.set(func(g *questGame) { g.turnOn = "false" })
	if out, _ = e.Execute(context.Background(), CmdQuestHelper, nil); out.(QuestHelperInfo).Running {
		t.Fatalf("a selected quest with the bot off runs: %+v", out)
	}
	g.set(func(g *questGame) { g.missing = true })
	if _, err = e.Execute(context.Background(), CmdQuestHelper, nil); err == nil || !strings.Contains(err.Error(), "has no Quest Helper") {
		t.Fatalf("missing = %v", err)
	}
	if got := g.acted(); len(got) > 0 {
		t.Fatalf("a read acted: %v", got)
	}
}

// TestQuestBotNeverToTheGrandExchange: no command of the planner sets the
// quest bot to fetch items (clientcfg.PinnedSettings).
func TestQuestBotNeverToTheGrandExchange(t *testing.T) {
	set := func(v string) json.RawMessage {
		return raw(t, ConfigSetParams{Group: clientcfg.QuestHelperGroup, Key: clientcfg.QuestObtainMissingItems, Value: &v})
	}
	if _, err := ValidateParams(CmdConfigSet, set("YES")); err == nil || !strings.Contains(err.Error(), "Grand Exchange") {
		t.Fatalf("config_set YES = %v", err)
	}
	if _, err := ValidateParams(CmdConfigSet, set("NO")); err != nil {
		t.Fatalf("config_set NO = %v", err)
	}
	if err := ValidateSetting(ProfileSetting{Group: clientcfg.QuestHelperGroup, Key: clientcfg.QuestObtainMissingItems, Value: "ASK"}); err == nil {
		t.Fatal("a profile asks the quest bot to fetch items")
	}
	if err := ValidateSetting(ProfileSetting{Group: clientcfg.QuestHelperGroup, Key: clientcfg.QuestObtainMissingItems}); err != nil {
		t.Fatalf("the key alone = %v", err)
	}
}
