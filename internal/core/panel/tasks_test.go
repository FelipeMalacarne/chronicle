package panel

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// updatePluginFixture rewrites the plugin's fixture of the assisted mode:
//
//	go test ./internal/core/panel -run TestAssistedModeWire -update
var updatePluginFixture = flag.Bool("update", false, "rewrite the Chronicle Link fixture of the panel's tasks")

// pluginFixture is the window with tasks that Chronicle Link's tests parse
// and render (RouteWindowTest, RoutePanelRenderTest): the contract of the
// panel route's tasks on both sides.
const pluginFixture = "../../../microbot/src/test/resources/net/runelite/client/plugins/microbot/fm/chroniclelink/route-tasks.json"

func ref(s string) *entities.Ref {
	r := entities.MustParseRef(s)
	return &r
}

// tasksFixture is the test guide with tasks on step 1 and a recipe for the
// objective it completes, so the cursor step inherits the recipe's task.
func tasksFixture(t *testing.T) (*content.Guide, *content.Content) {
	t.Helper()
	g := testGuide()
	st, _ := g.Step("1")
	axe := entities.MustParseObjectiveKey("item:1351x1")
	st.Tasks = []content.Task{
		{Type: content.TaskTravel, Location: ref("location:lumbridge_castle")},
		{Type: content.TaskBank},
		{Type: content.TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 1},
		{Type: content.TaskTalk, NPC: ref("npc:hans"), Options: []string{"Who are you?"}, Until: &content.TaskUntil{State: content.UntilGoalMet}},
		{Type: content.TaskInteract, Object: ref("object:bank_booth"), Action: "Bank", Until: &content.TaskUntil{Objective: &axe}},
		{Type: content.TaskManual, Until: &content.TaskUntil{State: content.UntilOwnerMark}},
	}
	agility := entities.MustParseObjectiveKey(agility10)
	recipe := &content.Recipe{Objective: agility, Variants: []content.RecipeVariant{{ID: "main", Modes: []string{"ironman"}, Tasks: []content.Task{
		{Type: content.TaskTrain, Skill: ref("skill:agility"), Until: &content.TaskUntil{Objective: &agility}},
	}}}}
	return g, &content.Content{Recipes: map[string]*content.Recipe{agility10: recipe}}
}

func TestAssistedModeTasks(t *testing.T) {
	reg := registry(t)
	g, c := tasksFixture(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	b := board(g, reg, progress.Verdicts{agility10: rules.Unmet}, nil)
	cursor := progress.CursorRecord{StepID: "1", Cause: progress.CursorAuto}
	inCastle := &Position{X: 3222, Y: 3218, ObservedAt: now.Add(-time.Minute)}
	state := &rules.PlayerState{Items: liveInventory(map[int]int64{hammer: 1})}
	w := Build(Input{Guide: g, Board: b, Cursor: cursor, Holdings: liveInventory(map[int]int64{hammer: 1}), Registry: reg,
		Content: c, Facts: Facts{Mode: "ironman", State: state, Position: inCastle}, Now: now})
	got := w.Steps[0].Tasks
	want := []Task{
		{Type: "train", Label: "Treinar Agility até 10", Until: "10 Agility", State: rules.Unmet, Recipe: true},
		{Type: "travel", Label: "Ir até Lumbridge Castle", Until: "na área do local", State: rules.Met},
		{Type: "bank", Label: "Banco: montar o setup do trecho", Until: "inventário e equipamento prontos", State: rules.Met},
		{Type: "buy", Label: "Comprar Bronze axe em Bob's Brilliant Axes.", Until: "Bronze axe", State: rules.Unmet},
		{Type: "talk", Label: "Falar com Hans: “Who are you?”", Until: "objetivos do passo", State: rules.Unmet},
		{Type: "interact", Label: "“Bank” em Bank booth", Until: "Bronze axe", State: rules.Unmet},
		{Type: "manual", Label: "À mão: siga o texto do passo", Until: "sua marca no passo", State: rules.Unmet},
	}
	if len(got) != len(want) {
		t.Fatalf("tasks = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("task %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
	for _, s := range w.Steps[1:] {
		if len(s.Tasks) > 0 {
			t.Errorf("step %s has tasks: only the cursor step shows them", s.ID)
		}
	}

	// What the server cannot tell is unknown, with the reason, never unmet.
	unknown := Build(Input{Guide: g, Board: b, Cursor: cursor, Registry: reg, Content: c, Now: now,
		Facts: Facts{Mode: "ironman", Position: &Position{X: 3222, Y: 3218, ObservedAt: now.Add(-time.Hour)}}}).Steps[0].Tasks
	for i, wantReason := range map[int]string{1: "posição velha", 2: "inventário ou equipamento não compartilhados ao vivo", 3: "sem fatos da conta"} {
		if unknown[i].State != rules.Unknown || unknown[i].Reason != wantReason {
			t.Errorf("task %d = %+v, want unknown (%s)", i, unknown[i], wantReason)
		}
	}
	if p := Build(Input{Guide: g, Board: b, Cursor: cursor, Registry: reg, Content: c, Now: now, Facts: Facts{Mode: "ironman"}}).Steps[0].Tasks[1]; p.Reason != "sem posição: ligue Send position" {
		t.Errorf("no position = %+v", p)
	}

	// The step met and marked: goal and mark tasks follow.
	done := board(g, reg, progress.Verdicts{agility10: rules.Met}, map[string]progress.Mark{"1": {Kind: progress.MarkManualDone}})
	doneTasks := Build(Input{Guide: g, Board: done, Cursor: cursor, Registry: reg, Content: c, Now: now, Facts: Facts{Mode: "ironman"}}).Steps[0].Tasks
	if doneTasks[0].State != rules.Met || doneTasks[4].State != rules.Met || doneTasks[6].State != rules.Met {
		t.Errorf("met step: %+v", doneTasks)
	}

	// Another mode has no variant of the recipe; without content, no list.
	if hc := Build(Input{Guide: g, Board: b, Cursor: cursor, Registry: reg, Content: c, Now: now, Facts: Facts{Mode: "hcim"}}).Steps[0].Tasks; len(hc) != 6 || hc[0].Recipe {
		t.Errorf("hcim tasks = %+v", hc)
	}
	if none := Build(Input{Guide: g, Board: b, Cursor: cursor, Registry: reg, Now: now}).Steps[0].Tasks; none != nil {
		t.Errorf("without content: %+v", none)
	}
}

// TestAssistedModeWire pins the JSON of a window with tasks that Chronicle
// Link reads: the plugin's tests parse the same file.
func TestAssistedModeWire(t *testing.T) {
	reg := registry(t)
	g, c := tasksFixture(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	b := board(g, reg, progress.Verdicts{agility10: rules.Unmet}, nil)
	w := Build(Input{Guide: g, Board: b, Cursor: progress.CursorRecord{StepID: "1", Cause: progress.CursorAuto}, Limit: 1,
		Holdings: liveInventory(map[int]int64{hammer: 1}), Registry: reg, Content: c, Now: now,
		Facts: Facts{Mode: "ironman", State: &rules.PlayerState{Items: liveInventory(map[int]int64{})}, Position: &Position{X: 3222, Y: 3218, ObservedAt: now}}})
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(w); err != nil {
		t.Fatal(err)
	}
	path := filepath.FromSlash(pluginFixture)
	if *updatePluginFixture {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (rewrite it with -update)", err)
	}
	if !bytes.Equal(want, buf.Bytes()) {
		t.Fatalf("the panel's JSON changed: rewrite %s with -update and check the plugin's tests\n%s", path, buf.String())
	}
}
