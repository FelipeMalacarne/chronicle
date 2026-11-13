package content

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// executionFixture is a content directory with one guide whose annotation
// gives its steps tasks, and three recipes (docs/10 sections 7 and 8).
var executionFixture = filepath.Join("testdata", "execution", "content")

// copyFixture copies the execution fixture into a temporary content dir.
func copyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(executionFixture, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(executionFixture, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestExecutionFixtureLoads(t *testing.T) {
	c, err := Load(executionFixture)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Warnings) != 0 {
		t.Fatalf("warnings: %v", c.Warnings)
	}
	types := func(ts []Task) string {
		var out []string
		for _, x := range ts {
			out = append(out, x.Type)
		}
		return strings.Join(out, ",")
	}
	want := map[string]string{"a": "travel,talk", "e": "", "b": "bank,buy", "c": "train", "d": "manual"}
	for id, w := range want {
		st, _ := c.StepInGuide("t", id)
		if got := types(st.Tasks); got != w {
			t.Errorf("step %s tasks = %s, want %s", id, got, w)
		}
	}
	if c.TaskCount() != 6 || len(c.Recipes) != 3 {
		t.Errorf("tasks %d, recipes %d", c.TaskCount(), len(c.Recipes))
	}
	a, _ := c.StepInGuide("t", "a")
	if talk := a.Tasks[1]; talk.NPC.String() != "npc:cook_lumbridge" || len(talk.Options) != 2 || talk.Until.State != UntilGoalMet {
		t.Errorf("a talk = %+v", talk)
	}
	// Defaults: travel ends in the area, buy when the item is owned.
	if u, _ := a.Tasks[0].Ends(); u.State != UntilAtLocation {
		t.Errorf("travel ends on %v", u)
	}
	b, _ := c.StepInGuide("t", "b")
	if u, _ := b.Tasks[1].Ends(); u.Objective == nil || u.Objective.String() != "item:1351x1" {
		t.Errorf("buy ends on %v", u)
	}

	// A step inherits the tasks of the recipe of the objective it completes.
	e, _ := c.StepInGuide("t", "e")
	if got := types(c.TasksOf(e, ModeIronman)); got != "travel,talk,interact,manual,talk" {
		t.Errorf("TasksOf(e) = %s", got)
	}
	cStep, _ := c.StepInGuide("t", "c")
	if got := types(c.TasksOf(cStep, ModeIronman)); got != "train" {
		t.Errorf("TasksOf(c) = %s (no recipe for woodcutting)", got)
	}

	// E9: in HCIM, nothing with a death risk runs unsupervised.
	attack, ok := c.Recipe(entities.MustParseObjectiveKey("skill:attack>=10"))
	if !ok {
		t.Fatal("skill:attack>=10 recipe not loaded")
	}
	hc, _ := attack.VariantFor(ModeHCIM)
	main, _ := attack.VariantFor(ModeIronman)
	if hc.ID != "hcim" || hc.Unsupervised(ModeHCIM) || !main.Unsupervised(ModeIronman) {
		t.Errorf("variants: hcim %q unsupervised %v; ironman %q unsupervised %v", hc.ID, hc.Unsupervised(ModeHCIM), main.ID, main.Unsupervised(ModeIronman))
	}
	axe, _ := c.Recipe(entities.MustParseObjectiveKey("item:1351x1"))
	if v, _ := axe.VariantFor(ModeHCIM); !v.Unsupervised(ModeHCIM) {
		t.Error("a recipe without risks runs unsupervised in HCIM")
	}
}

type invalidCase struct {
	Name   string          `json:"name"`
	Step   string          `json:"step"`
	Task   json.RawMessage `json:"task"`
	File   string          `json:"file"`
	Recipe json.RawMessage `json:"recipe"`
	Want   string          `json:"want"`
}

func readCases(t *testing.T, name string) []invalidCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "execution", name))
	if err != nil {
		t.Fatal(err)
	}
	var cases []invalidCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s has no cases", name)
	}
	return cases
}

// TestInvalidStepTasks replaces the tasks of one step of the fixture with
// each invalid task of testdata/execution/invalid_tasks.json and expects the
// loader (schema and verifier) to refuse it.
func TestInvalidStepTasks(t *testing.T) {
	for _, tc := range readCases(t, "invalid_tasks.json") {
		t.Run(tc.Name, func(t *testing.T) {
			dir := copyFixture(t)
			path := filepath.Join(dir, GuidesSubdir, "t", AnnotationsSubdir, "1.1.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err = json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			var task any
			if err = json.Unmarshal(tc.Task, &task); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, s := range doc["steps"].([]any) {
				if st := s.(map[string]any); st["step_id"] == tc.Step {
					st["tasks"] = []any{task}
					found = true
				}
			}
			if !found {
				t.Fatalf("no step %q in the fixture", tc.Step)
			}
			out, _ := json.Marshal(doc)
			if err = os.WriteFile(path, out, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err = Load(dir)
			if err == nil || !strings.Contains(err.Error(), tc.Want) {
				t.Fatalf("Load error = %v, want %q", err, tc.Want)
			}
		})
	}
}

// TestInvalidRecipes writes each recipe of testdata/execution/invalid_recipes.json
// into the fixture's recipes and expects the loader to refuse it.
func TestInvalidRecipes(t *testing.T) {
	for _, tc := range readCases(t, "invalid_recipes.json") {
		t.Run(tc.Name, func(t *testing.T) {
			dir := copyFixture(t)
			if err := os.WriteFile(filepath.Join(dir, RecipesSubdir, tc.File), tc.Recipe, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), tc.Want) {
				t.Fatalf("Load error = %v, want %q", err, tc.Want)
			}
			if !strings.Contains(err.Error(), tc.File) {
				t.Errorf("the error does not name the file %s: %v", tc.File, err)
			}
		})
	}
}

func ref(s string) *entities.Ref {
	r := entities.MustParseRef(s)
	return &r
}

func until(s string) *TaskUntil {
	if k, err := entities.ParseObjectiveKey(s); err == nil {
		return &TaskUntil{Objective: &k}
	}
	return &TaskUntil{State: s}
}

// TestTaskCheck is the verifier's table (docs/10 section 7, rule 4), on tasks
// built in code: what the schema already refuses in files is checked here
// too, because the extraction tooling builds tasks without a file.
func TestTaskCheck(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	goal := []entities.ObjectiveKey{entities.MustParseObjectiveKey("quest:cooks_assistant@started")}
	step := TaskContext{Goal: goal}
	bare := TaskContext{}
	recipe := TaskContext{Goal: []entities.ObjectiveKey{entities.MustParseObjectiveKey("item:1351x1")}, Recipe: true}
	hammer := 2347
	cases := []struct {
		name string
		task Task
		ctx  TaskContext
		want string // "" when valid
	}{
		{"travel", Task{Type: TaskTravel, Location: ref("location:lumbridge"), Via: ref("item:8007")}, bare, ""},
		{"travel via an unlock", Task{Type: TaskTravel, Location: ref("location:lumbridge"), Via: ref("unlock:fairy_rings")}, bare, ""},
		{"travel needs a location", Task{Type: TaskTravel}, bare, "travel needs location"},
		{"travel to an npc", Task{Type: TaskTravel, Location: ref("npc:hans")}, bare, "is not of kind location"},
		{"travel via a quest", Task{Type: TaskTravel, Location: ref("location:lumbridge"), Via: ref("quest:cooks_assistant")}, bare, "is not of kind unlock or item"},
		{"travel without area", Task{Type: TaskTravel, Location: ref("location:wilderness")}, bare, "has no area"},
		{"bank with the segment loadout", Task{Type: TaskBank}, TaskContext{Loadout: true}, ""},
		{"bank with items", Task{Type: TaskBank, Items: []ItemRequirement{{Name: "Hammer", ItemID: &hammer}}}, recipe, ""},
		{"bank with an unresolved item", Task{Type: TaskBank, Items: []ItemRequirement{{Name: "Some food"}}}, bare, "must be resolved"},
		{"bank in a recipe without items", Task{Type: TaskBank}, recipe, "bank in a recipe needs items"},
		{"buy", Task{Type: TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 2}, bare, ""},
		{"buy qty 0", Task{Type: TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351")}, bare, "buy needs qty >= 1"},
		{"buy at an unknown shop", Task{Type: TaskBuy, Shop: ref("shop:not_a_shop"), Item: ref("item:1351"), Qty: 1}, bare, "unknown entity shop:not_a_shop"},
		{"buy what the shop does not sell", Task{Type: TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1511"), Qty: 1}, bare, "does not sell"},
		{"talk", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Until: until(UntilGoalMet)}, step, ""},
		{"talk until an objective", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Until: until("quest:cooks_assistant@started")}, bare, ""},
		{"talk without until", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge")}, step, "talk needs until"},
		{"talk to an object", Task{Type: TaskTalk, NPC: ref("object:bank_booth"), Until: until(UntilGoalMet)}, step, "is not of kind npc"},
		{"talk with an empty option", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Options: []string{" "}, Until: until(UntilGoalMet)}, step, "not empty"},
		{"talk goal_met without goal", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Until: until(UntilGoalMet)}, bare, "completes no objective"},
		{"talk until owner_mark", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Until: until(UntilOwnerMark)}, step, "cannot end on owner_mark"},
		{"interact", Task{Type: TaskInteract, Object: ref("object:bank_booth"), Action: "Bank", Until: until(UntilGoalMet)}, step, ""},
		{"interact without action", Task{Type: TaskInteract, Object: ref("object:bank_booth"), Until: until(UntilGoalMet)}, step, "needs action"},
		{"train", Task{Type: TaskTrain, Skill: ref("skill:magic"), NPC: ref("npc:chicken"), Until: until("skill:magic>=13")}, bare, ""},
		{"train on npc and object", Task{Type: TaskTrain, Skill: ref("skill:magic"), NPC: ref("npc:chicken"), Object: ref("object:tree"), Until: until("skill:magic>=13")}, bare, "not both"},
		{"train until another skill", Task{Type: TaskTrain, Skill: ref("skill:magic"), Until: until("skill:attack>=13")}, bare, "ends on skill:magic>=N"},
		{"train until a state", Task{Type: TaskTrain, Skill: ref("skill:magic"), Until: until(UntilGoalMet)}, step, "cannot end on goal_met"},
		{"train delegated with config", Task{Type: TaskTrain, Skill: ref("skill:fishing"), Plugin: "a.b.TickFisherPlugin",
			Config: []TaskSetting{{Group: "tickfisher", Key: "mode", Value: "BARBARIAN"}}, Until: until("skill:fishing>=76")}, recipe, ""},
		{"train config without plugin", Task{Type: TaskTrain, Skill: ref("skill:fishing"),
			Config: []TaskSetting{{Group: "tickfisher", Key: "mode", Value: "x"}}, Until: until("skill:fishing>=76")}, bare, "needs plugin"},
		{"train on a bare class", Task{Type: TaskTrain, Skill: ref("skill:fishing"), Plugin: "TickFisherPlugin", Until: until("skill:fishing>=76")}, bare, "fully qualified"},
		{"train on chronicle link", Task{Type: TaskTrain, Skill: ref("skill:fishing"),
			Plugin: "net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin", Until: until("skill:fishing>=76")}, bare, "not controlled"},
		{"train config of a protected group", Task{Type: TaskTrain, Skill: ref("skill:fishing"), Plugin: "a.b.T",
			Config: []TaskSetting{{Group: "agentServer", Key: "port", Value: "1"}}, Until: until("skill:fishing>=76")}, bare, "not edited"},
		{"train config of a secret", Task{Type: TaskTrain, Skill: ref("skill:fishing"), Plugin: "a.b.T",
			Config: []TaskSetting{{Group: "t", Key: "bankPin", Value: "1"}}, Until: until("skill:fishing>=76")}, bare, "looks like a secret"},
		{"train config twice", Task{Type: TaskTrain, Skill: ref("skill:fishing"), Plugin: "a.b.T",
			Config: []TaskSetting{{Group: "t", Key: "k", Value: "1"}, {Group: "t", Key: "k", Value: "2"}}, Until: until("skill:fishing>=76")}, bare, "twice"},
		{"a plugin on a travel", Task{Type: TaskTravel, Location: ref("location:lumbridge"), Plugin: "a.b.T"}, bare, "travel takes no plugin"},
		{"manual", Task{Type: TaskManual, Until: until(UntilOwnerMark)}, bare, ""},
		{"manual in a recipe", Task{Type: TaskManual, Note: "Buy it.", Until: until(UntilGoalMet)}, recipe, ""},
		{"manual in a recipe without note", Task{Type: TaskManual, Until: until(UntilGoalMet)}, recipe, "needs a note"},
		{"manual owner_mark in a recipe", Task{Type: TaskManual, Note: "x", Until: until(UntilOwnerMark)}, recipe, "no step for the owner to mark"},
		{"quest is planned", Task{Type: TaskQuest}, step, "not an executor primitive yet"},
		{"unknown type", Task{Type: "teleport"}, step, "unknown task type"},
		{"a parameter of another type", Task{Type: TaskTravel, Location: ref("location:lumbridge"), Shop: ref("shop:bobs_brilliant_axes")}, bare, "travel takes no shop"},
		{"until with both", Task{Type: TaskManual, Until: &TaskUntil{State: UntilOwnerMark, Objective: &goal[0]}}, step, "exactly one of objective and state"},
		{"until an unknown state", Task{Type: TaskManual, Until: until("arrived")}, step, "unknown state"},
		{"until an unknown objective", Task{Type: TaskManual, Until: until("quest:not_a_quest@started")}, step, "unknown entity quest:not_a_quest"},
	}
	for _, c := range cases {
		msgs := c.task.Check(reg, c.ctx)
		got := strings.Join(msgs, "; ")
		switch {
		case c.want == "" && len(msgs) > 0:
			t.Errorf("%s: unexpected %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRecipeFileName(t *testing.T) {
	cases := map[string]string{
		"quest:cooks_assistant@finished":        "quest-cooks_assistant-finished.json",
		"skill:agility>=70":                     "skill-agility-70.json",
		"item:960x120":                          "item-960x120.json",
		"item:any(1355x2|1357x1|1359x1)":        "item-any-1355x2-1357x1-1359x1.json",
		"item:family/graceful_hoodx1":           "item-family-graceful_hoodx1.json",
		"diary:kourend_and_kebos:easy@complete": "diary-kourend_and_kebos-easy-complete.json",
		"qp>=107":                               "qp-107.json",
	}
	for key, want := range cases {
		if got := RecipeFileName(entities.MustParseObjectiveKey(key)); got != want {
			t.Errorf("RecipeFileName(%s) = %s, want %s", key, got, want)
		}
	}
}

func TestTaskJSONRoundTrip(t *testing.T) {
	src := `{"type":"buy","shop":"shop:bobs_brilliant_axes","item":"item:1351","qty":2,"until":{"objective":"item:1351x2"}}`
	var task Task
	if err := json.Unmarshal([]byte(src), &task); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != src {
		t.Errorf("round trip = %s", out)
	}
	if err := json.Unmarshal([]byte(`{"type":"talk","npc":"npc:Not A Slug"}`), &task); err == nil {
		t.Error("an invalid ref decoded")
	}
}
