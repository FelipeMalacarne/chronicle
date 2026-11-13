package plan

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// The execution fixture of internal/core/content: guide "t" with tasks on
// its steps (a: travel, talk; b: bank, buy; c: train; d: manual) and three
// recipes (Cook's Assistant, a bronze axe, 10 Attack with an HCIM variant).
var (
	fixtureOnce sync.Once
	fixture     *content.Content
	fixtureErr  error
)

func load(t *testing.T) (*content.Content, *entities.Registry) {
	t.Helper()
	fixtureOnce.Do(func() {
		fixture, fixtureErr = content.Load(filepath.Join("..", "content", "testdata", "execution", "content"))
	})
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	return fixture, reg
}

var now = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// steps is the board of guide t: every step pending unless status says
// otherwise, the objectives of each step with verdicts.
func steps(c *content.Content, status map[string]progress.Status, verdicts map[string]rules.Verdict, blocked map[string][]string) []progress.StepState {
	g, _ := c.Guide("t")
	var out []progress.StepState
	for _, st := range g.Steps() {
		s := progress.StepState{ID: st.ID, Ref: st.ID, Title: progress.StepTitle(st.Text), SectionID: st.SectionID, Status: progress.Pending, Main: true, Visible: true, BlockedBy: blocked[st.ID]}
		if v, ok := status[st.ID]; ok {
			s.Status = v
			if v != progress.AutoDone {
				s.Mark = &progress.Mark{Kind: progress.MarkKind(v)}
			}
		}
		for _, o := range st.Objectives {
			v := verdicts[o.Key.String()]
			if v == "" {
				v = rules.Unknown
			}
			s.Objectives = append(s.Objectives, progress.ObjectiveState{Key: o.Key.String(), Role: o.Role, Verdict: v})
		}
		out = append(out, s)
	}
	return out
}

func live(q map[int]int64) rules.ContainerItems {
	return rules.ContainerItems{State: ingest.ItemLive, Complete: true, ObservedAt: now, Quantities: q}
}

// at is a fresh position.
func at(x, y int) *panel.Position {
	return &panel.Position{X: x, Y: y, ObservedAt: now.Add(-time.Minute)}
}

func key(s string) *entities.ObjectiveKey {
	k := entities.MustParseObjectiveKey(s)
	return &k
}

func ref(s string) *entities.Ref {
	r := entities.MustParseRef(s)
	return &r
}

type expect struct {
	status   string
	item     string // the current item's key
	task     string // the next task's key
	typ      string
	state    rules.Verdict
	skipped  []string
	risks    []string
	super    bool
	exec     bool
	notExec  string // substring of NotExecutable
	reason   string // substring of Reason
	recipe   string
	forKey   string
	label    string // substring of the task's label
	position int
}

func TestNext(t *testing.T) {
	c, reg := load(t)
	route := Plan{ID: RouteID("t"), Kind: KindRoute, Title: "Execution fixture", Guide: "t"}
	hammer := map[string]rules.ContainerItems{"inventory": live(map[int]int64{2347: 1}), "equipment": live(map[int]int64{})}
	axeBanked := map[string]rules.ContainerItems{
		"inventory": live(map[int]int64{2347: 1}), "equipment": live(map[int]int64{}), "bank": live(map[int]int64{1351: 1}),
	}
	custom := func(items ...Item) Plan {
		return Plan{ID: "pln_test0001", Kind: KindCustom, Title: "Logística", Items: items}
	}

	for _, tc := range []struct {
		name     string
		plan     Plan
		cursor   string
		status   map[string]progress.Status
		verdicts map[string]rules.Verdict
		blocked  map[string][]string
		holdings rules.ItemHoldings
		mode     string
		pos      *panel.Position
		want     expect
	}{
		{
			name: "route: the cursor step's first task, unknown without a position",
			plan: route, cursor: "a",
			want: expect{status: StatusTask, item: "step:t::a", task: "step:t::a#0", typ: "travel", state: rules.Unknown, exec: true, label: "Lumbridge", risks: []string{}},
		},
		{
			name: "route: a travel already met is skipped (idempotence)",
			plan: route, cursor: "a", pos: at(3188, 3220),
			want: expect{status: StatusTask, item: "step:t::a", task: "step:t::a#1", typ: "talk", state: rules.Unknown, skipped: []string{"step:t::a#0"}, notExec: "só roda travel, bank e buy", risks: []string{}},
		},
		{
			name: "route: away from the area the travel is unmet",
			plan: route, cursor: "a", pos: at(3103, 3265),
			want: expect{status: StatusTask, task: "step:t::a#0", typ: "travel", state: rules.Unmet, exec: true, risks: []string{}},
		},
		{
			name: "route: every task met but the step not done waits",
			plan: route, cursor: "a", pos: at(3188, 3220), verdicts: map[string]rules.Verdict{"quest:cooks_assistant@started": rules.Met},
			want: expect{status: StatusWaiting, item: "step:t::a", skipped: []string{"step:t::a#0", "step:t::a#1"}, reason: "falta o passo concluir"},
		},
		{
			name: "route: a step inherits the recipe of the objective it completes",
			plan: route, cursor: "e",
			want: expect{status: StatusTask, item: "step:t::e", task: "step:t::e#0", typ: "travel", state: rules.Unknown, exec: true, recipe: "quest:cooks_assistant@finished", risks: []string{}},
		},
		{
			name: "route: bank for the segment's loadout, unmet without the hammer",
			plan: route, cursor: "b", holdings: map[string]rules.ContainerItems{"inventory": live(map[int]int64{}), "equipment": live(map[int]int64{})},
			want: expect{status: StatusTask, task: "step:t::b#0", typ: "bank", state: rules.Unmet, exec: true, risks: []string{}},
		},
		{
			name: "route: with the hammer carried the bank is skipped and buy is next",
			plan: route, cursor: "b", holdings: hammer,
			want: expect{status: StatusTask, task: "step:t::b#1", typ: "buy", state: rules.Unknown, skipped: []string{"step:t::b#0"}, exec: true, risks: []string{}},
		},
		{
			name: "route: the axe owned too: the step waits for the owner",
			plan: route, cursor: "b", holdings: axeBanked,
			want: expect{status: StatusWaiting, skipped: []string{"step:t::b#0", "step:t::b#1"}},
		},
		{
			name: "route: an Ultimate Ironman does not bank",
			plan: route, cursor: "b", mode: content.ModeUIM,
			want: expect{status: StatusTask, task: "step:t::b#0", typ: "bank", notExec: "Ultimate Ironman", risks: []string{}},
		},
		{
			name: "route: train is not executed in phase 3",
			plan: route, cursor: "c",
			want: expect{status: StatusTask, task: "step:t::c#0", typ: "train", notExec: "só roda travel, bank e buy", risks: []string{}},
		},
		{
			name: "route: manual waits for the owner's mark",
			plan: route, cursor: "d",
			want: expect{status: StatusTask, task: "step:t::d#0", typ: "manual", state: rules.Unmet, notExec: "só roda travel, bank e buy", risks: []string{}},
		},
		{
			name: "route: the owner's mark meets the manual task",
			plan: route, cursor: "d", status: map[string]progress.Status{"d": progress.ManualDone},
			want: expect{status: StatusWaiting, skipped: []string{"step:t::d#0"}},
		},
		{
			name: "route: finished",
			plan: route, cursor: "",
			want: expect{status: StatusDone, reason: "a rota terminou"},
		},
		{
			name: "route: an unmet requirement with a recipe inserts the recipe's tasks first",
			plan: route, cursor: "c", blocked: map[string][]string{"c": {"skill:attack>=10"}},
			want: expect{status: StatusTask, item: "step:t::c", task: "recipe:skill:attack>=10#0", typ: "bank", forKey: "skill:attack>=10", recipe: "skill:attack>=10", risks: []string{"combat"}, exec: true},
		},
		{
			name: "route: in HCIM the inserted recipe's death risk needs supervision",
			plan: route, cursor: "c", blocked: map[string][]string{"c": {"skill:attack>=10"}}, mode: content.ModeHCIM,
			want: expect{status: StatusTask, task: "recipe:skill:attack>=10#0", typ: "travel", forKey: "skill:attack>=10", recipe: "skill:attack>=10", risks: []string{"combat"}, super: true, exec: true},
		},
		{
			name: "route: an unmet requirement without a recipe is the owner's",
			plan: route, cursor: "c", blocked: map[string][]string{"c": {"skill:mining>=15"}},
			want: expect{status: StatusManual, item: "step:t::c", reason: "15 Mining"},
		},
		{
			name:   "custom: done steps pass, a recipe item gives its first task",
			plan:   custom(Item{Kind: ItemStep, Guide: "t", Step: "a"}, Item{Kind: ItemRecipe, Recipe: key("item:1351x1")}),
			status: map[string]progress.Status{"a": progress.AutoDone},
			want:   expect{status: StatusTask, item: "recipe:item:1351x1", task: "recipe:item:1351x1#0", typ: "travel", recipe: "item:1351x1", exec: true, position: 1, risks: []string{}},
		},
		{
			name:     "custom: a met recipe passes; a travel into the wilderness is a risk",
			plan:     custom(Item{Kind: ItemRecipe, Recipe: key("item:1351x1")}, Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskTravel, Location: ref("location:mage_bank")}}),
			holdings: axeBanked,
			want:     expect{status: StatusTask, item: "task:1", task: "task:1#0", typ: "travel", risks: []string{"wilderness"}, exec: true, position: 1},
		},
		{
			name: "custom: in HCIM the wilderness needs supervision",
			plan: custom(Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskTravel, Location: ref("location:mage_bank")}}),
			mode: content.ModeHCIM,
			want: expect{status: StatusTask, task: "task:0#0", risks: []string{"wilderness"}, super: true, exec: true},
		},
		{
			name: "custom: a met task item is skipped",
			plan: custom(
				Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskTravel, Location: ref("location:lumbridge")}},
				Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskTravel, Location: ref("location:draynor_village"), Via: ref("item:8007")}},
			),
			pos:  at(3188, 3220),
			want: expect{status: StatusTask, task: "task:1#0", skipped: []string{"task:0#0"}, state: rules.Unmet, notExec: "via", position: 1, risks: []string{}},
		},
		{
			name: "custom: a step that left the guide stops the plan",
			plan: custom(Item{Kind: ItemStep, Guide: "t", Step: "gone"}),
			want: expect{status: StatusManual, item: "step:t::gone", reason: "não existe mais"},
		},
		{
			name:   "custom: everything done",
			plan:   custom(Item{Kind: ItemStep, Guide: "t", Step: "a"}),
			status: map[string]progress.Status{"a": progress.Skipped},
			want:   expect{status: StatusDone},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode := tc.mode
			if mode == "" {
				mode = content.ModeIronman
			}
			ps := rules.PlayerState{Items: tc.holdings, Catalog: reg}
			in := Input{
				Plan: tc.plan, Content: c, Registry: reg,
				Boards:   map[string]Board{"t": {Steps: steps(c, tc.status, tc.verdicts, tc.blocked), Cursor: tc.cursor}},
				Holdings: tc.holdings, Facts: panel.Facts{Mode: mode, State: &ps, Position: tc.pos}, Now: now,
			}
			got := Next(in)
			w := tc.want
			if got.Status != w.status {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Reason, w.status)
			}
			if w.reason != "" && !strings.Contains(got.Reason, w.reason) {
				t.Errorf("reason = %q, want %q in it", got.Reason, w.reason)
			}
			if w.item != "" && (got.Item == nil || got.Item.Key != w.item) {
				t.Errorf("item = %+v, want %s", got.Item, w.item)
			}
			if got.Item != nil && got.Item.Position != w.position && tc.plan.Kind != KindRoute {
				t.Errorf("position = %d, want %d", got.Item.Position, w.position)
			}
			var skipped []string
			for _, s := range got.Skipped {
				if s.State != rules.Met {
					t.Errorf("skipped %s is %s, not met", s.Key, s.State)
				}
				skipped = append(skipped, s.Key)
			}
			if !slices.Equal(skipped, w.skipped) {
				t.Errorf("skipped = %v, want %v", skipped, w.skipped)
			}
			if w.status != StatusTask {
				if got.Task != nil {
					t.Errorf("task = %+v on status %s", got.Task, got.Status)
				}
				return
			}
			task := got.Task
			if task == nil {
				t.Fatal("no task")
			}
			if task.Key != w.task || (w.typ != "" && task.Type != w.typ) {
				t.Errorf("task = %s %s, want %s %s", task.Key, task.Type, w.task, w.typ)
			}
			if w.state != "" && task.State != w.state {
				t.Errorf("state = %s (%s), want %s", task.State, task.Reason, w.state)
			}
			if task.State == rules.Met {
				t.Errorf("the next task is never met")
			}
			if !slices.Equal(task.Risks, w.risks) || task.Supervised != w.super {
				t.Errorf("risks = %v supervised %v, want %v %v", task.Risks, task.Supervised, w.risks, w.super)
			}
			if task.Executable != (w.notExec == "" && w.exec) {
				t.Errorf("executable = %v (%s)", task.Executable, task.NotExecutable)
			}
			if w.notExec != "" && !strings.Contains(task.NotExecutable, w.notExec) {
				t.Errorf("not executable = %q, want %q in it", task.NotExecutable, w.notExec)
			}
			if task.Recipe != w.recipe || task.For != w.forKey {
				t.Errorf("recipe %q for %q, want %q for %q", task.Recipe, task.For, w.recipe, w.forKey)
			}
			if w.label != "" && !strings.Contains(task.Label, w.label) {
				t.Errorf("label = %q, want %q in it", task.Label, w.label)
			}
			// Find gives the same task by its key.
			again, ok := Find(in, task.Key)
			if !ok || again.Key != task.Key || again.Type != task.Type || again.State != task.State {
				t.Errorf("Find(%s) = %+v, %v", task.Key, again, ok)
			}
		})
	}
}

func TestRouteItemCarriesTheGuideAndItsCredit(t *testing.T) {
	c, reg := load(t)
	ps := rules.PlayerState{Catalog: reg}
	got := Next(Input{
		Plan: Plan{ID: RouteID("t"), Kind: KindRoute, Guide: "t"}, Content: c, Registry: reg,
		Boards: map[string]Board{"t": {Steps: steps(c, nil, nil, nil), Cursor: "b"}},
		Facts:  panel.Facts{Mode: content.ModeIronman, State: &ps}, Now: now,
	})
	it := got.Item
	if it == nil || it.Guide != "t" || it.Step != "b" || it.Position != 2 || it.Status != progress.Pending || it.Attribution == "" || it.Title == "" {
		t.Fatalf("item = %+v", it)
	}
}

func TestFindRefusesWhatIsNotAPlanTask(t *testing.T) {
	c, reg := load(t)
	ps := rules.PlayerState{Catalog: reg}
	in := Input{
		Plan: Plan{ID: "pln_x", Kind: KindCustom, Items: []Item{{Kind: ItemStep, Guide: "t", Step: "a"}}}, Content: c, Registry: reg,
		Boards: map[string]Board{"t": {Steps: steps(c, nil, nil, nil)}}, Facts: panel.Facts{Mode: content.ModeIronman, State: &ps}, Now: now,
	}
	for _, k := range []string{
		"", "step:t::a", "step:t::a#9", "step:t::zz#0", "step:x::a#0", "recipe:item:1351x1#5", "recipe:quest:nope@finished#0",
		"task:0#0", "task:5#0", "task:0#1", "weird#0", "step:t::a#-1",
	} {
		if v, ok := Find(in, k); ok {
			t.Errorf("Find(%q) = %+v", k, v)
		}
	}
	if v, ok := Find(in, "recipe:skill:attack>=10#0"); !ok || v.Type != content.TaskBank || !slices.Equal(v.Risks, []string{"combat"}) {
		t.Errorf("recipe task = %+v, %v", v, ok)
	}
}

func TestCheckItems(t *testing.T) {
	c, reg := load(t)
	ok := []Item{
		{Kind: ItemStep, Guide: "t", Step: "a"},
		{Kind: ItemRecipe, Recipe: key("item:1351x1")},
		{Kind: ItemTask, Task: &content.Task{Type: content.TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 2}},
	}
	if msgs := CheckItems(ok, c, reg); len(msgs) > 0 {
		t.Fatalf("valid items refused: %v", msgs)
	}
	for _, tc := range []struct {
		item Item
		want string
	}{
		{Item{Kind: ItemStep, Guide: "t"}, "guide and step only"},
		{Item{Kind: ItemStep, Guide: "nope", Step: "a"}, "unknown guide"},
		{Item{Kind: ItemStep, Guide: "t", Step: "zz"}, "has no step"},
		{Item{Kind: ItemStep, Guide: "t", Step: "a", Recipe: key("item:1351x1")}, "guide and step only"},
		{Item{Kind: ItemRecipe, Recipe: key("skill:mining>=15")}, "no recipe"},
		{Item{Kind: ItemTask}, "task only"},
		{Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskBank}}, "bank in a recipe needs items"},
		{Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskManual, Note: "x", Until: &content.TaskUntil{State: content.UntilOwnerMark}}}, "owner_mark"},
		{Item{Kind: ItemTask, Task: &content.Task{Type: content.TaskTalk, NPC: ref("npc:hans"), Until: &content.TaskUntil{State: content.UntilGoalMet}}}, "goal_met"},
		{Item{Kind: "chapter"}, "unknown item kind"},
	} {
		msgs := CheckItems([]Item{tc.item}, c, reg)
		if len(msgs) == 0 || !strings.Contains(strings.Join(msgs, "; "), tc.want) {
			t.Errorf("%+v: %v, want %q", tc.item, msgs, tc.want)
		}
	}
	if msgs := CheckItems(make([]Item, MaxItems+1), c, reg); len(msgs) != 1 {
		t.Errorf("too many items: %v", msgs)
	}
}

func TestWilderness(t *testing.T) {
	for _, tc := range []struct {
		x, y int
		want bool
	}{
		{3093, 3957, true},   // Mage Bank
		{3080, 3492, false},  // Edgeville
		{3080, 3525, true},   // the ditch's north side
		{3080, 3524, false},  // south of it
		{2943, 3600, false},  // west of it
		{3391, 3972, true},   // its corner
		{3392, 3700, false},  // east of it
		{3045, 10340, true},  // under the wilderness
		{3200, 9900, false},  // under Varrock
		{3461, 10375, true},  // the underground's corner
		{3188, 3220, false},  // Lumbridge
		{3210, 3448, false},  // Varrock
		{3135, 3630, true},   // Ferox Enclave counts: the box errs on the side of risk
		{3100, 9918, true},   // underground south edge
		{2944, 3525, true},   // the corner
		{3000, 10376, false}, // past the underground
	} {
		if got := execspec.InWilderness(tc.x, tc.y); got != tc.want {
			t.Errorf("InWilderness(%d, %d) = %v", tc.x, tc.y, got)
		}
	}
	if !AreaInWilderness(entities.Area{MinX: 3070, MinY: 3515, MaxX: 3090, MaxY: 3535}) {
		t.Error("an area across the ditch touches the wilderness")
	}
	if AreaInWilderness(entities.Area{MinX: 3070, MinY: 3482, MaxX: 3090, MaxY: 3502}) {
		t.Error("Edgeville is not in the wilderness")
	}
	if !NeedsSupervision(content.ModeHCIM, []string{"wilderness"}) || NeedsSupervision(content.ModeIronman, []string{"wilderness"}) ||
		NeedsSupervision(content.ModeHCIM, []string{"item_loss"}) || !NeedsSupervision(content.ModeHCIM, []string{"item_loss", "hazard"}) {
		t.Error("E9: HCIM and a death risk, only")
	}
}

func TestKeys(t *testing.T) {
	if k := TaskKey(StepKey("oziris-main", "1.1.3"), 2); k != "step:oziris-main::1.1.3#2" {
		t.Errorf("task key = %s", k)
	}
	item, i, ok := SplitTaskKey("recipe:skill:attack>=10#1")
	if !ok || item != "recipe:skill:attack>=10" || i != 1 {
		t.Errorf("split = %s %d %v", item, i, ok)
	}
	if g, ok := RouteGuide(RouteID("bruhsailer-main")); !ok || g != "bruhsailer-main" {
		t.Errorf("route guide = %s %v", g, ok)
	}
	if _, ok := RouteGuide("pln_abc"); ok {
		t.Error("a stored plan is not a route")
	}
	if CheckTitle("Logística de Lumbridge") != nil || CheckTitle(" ") == nil || CheckTitle(strings.Repeat("x", MaxTitle+1)) == nil || CheckTitle("a\nb") == nil {
		t.Error("titles")
	}
}

func TestShopKeepers(t *testing.T) {
	_, reg := load(t)
	ids := ShopKeepers(content.Task{Type: content.TaskBuy, Shop: ref("shop:bobs_brilliant_axes")}, reg)
	if len(ids) == 0 {
		t.Fatal("Bob's Brilliant Axes has a shopkeeper")
	}
	if ShopKeepers(content.Task{Type: content.TaskBuy}, reg) != nil {
		t.Error("no shop, no keepers")
	}
}
