package plan

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func intp(n int) *int { return &n }

func TestResolve(t *testing.T) {
	c, reg := load(t)
	input := func(p Plan, mode string, h rules.ItemHoldings, pos *panel.Position) Input {
		ps := rules.PlayerState{Items: h, Catalog: reg}
		return Input{
			Plan: p, Content: c, Registry: reg, Holdings: h, Now: now,
			Boards: map[string]Board{"t": {Steps: steps(c, nil, nil, nil), Cursor: "a"}},
			Facts:  panel.Facts{Mode: mode, State: &ps, Position: pos},
		}
	}
	route := Plan{ID: RouteID("t"), Kind: KindRoute, Guide: "t"}
	custom := func(tasks ...content.Task) Plan {
		p := Plan{ID: "pln_test0001", Kind: KindCustom}
		for i := range tasks {
			p.Items = append(p.Items, Item{Kind: ItemTask, Task: &tasks[i]})
		}
		return p
	}

	t.Run("travel to a point area: the map point", func(t *testing.T) {
		spec, v, err := Resolve(input(route, content.ModeIronman, nil, nil), "step:t::a#0")
		if err != nil {
			t.Fatal(err)
		}
		tr := spec.Travel
		if spec.Type != execspec.TypeTravel || tr == nil || tr.Location != "location:lumbridge" || tr.Name != "Lumbridge" ||
			tr.Target != (execspec.Tile{X: 3188, Y: 3220}) || tr.Wilderness || v.Key != "step:t::a#0" {
			t.Fatalf("travel = %+v %+v", tr, v)
		}
	})
	t.Run("travel to a polygon", func(t *testing.T) {
		spec, _, err := Resolve(input(custom(content.Task{Type: content.TaskTravel, Location: ref("location:lumbridge_castle")}), content.ModeIronman, nil, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		a := spec.Travel.Area
		if len(a.Polygon) < 3 || !a.Contains(spec.Travel.Target.X, spec.Travel.Target.Y, spec.Travel.Target.Plane) {
			t.Fatalf("castle = %+v", spec.Travel)
		}
	})
	t.Run("travel into the wilderness carries the flag", func(t *testing.T) {
		spec, v, err := Resolve(input(custom(content.Task{Type: content.TaskTravel, Location: ref("location:mage_bank")}), content.ModeHCIM, nil, nil), "task:0#0")
		if err != nil || !spec.Travel.Wilderness || !v.Supervised || !slices.Equal(spec.Risks(), []string{"wilderness"}) {
			t.Fatalf("mage bank = %+v %+v %v", spec.Travel, v, err)
		}
	})
	t.Run("bank for the segment's loadout", func(t *testing.T) {
		spec, _, err := Resolve(input(route, content.ModeIronman, nil, nil), "step:t::b#0")
		if err != nil {
			t.Fatal(err)
		}
		if spec.Bank == nil || len(spec.Bank.Items) != 1 || spec.Bank.Items[0].Name != "Hammer" || !slices.Equal(spec.Bank.Items[0].IDs, []int{2347}) || spec.Bank.Items[0].Qty != 1 {
			t.Fatalf("bank = %+v", spec.Bank)
		}
	})
	t.Run("bank items: any_of, family and noted", func(t *testing.T) {
		task := content.Task{Type: content.TaskBank, Items: []content.ItemRequirement{
			{Name: "Axe", AnyOf: []int{1359, 1351}},
			{Name: "Prayer potion", Family: "prayer_potion", Qty: intp(2), Optional: true},
			{Name: "Raw shrimps", ItemID: intp(317), Qty: intp(50), Noted: true},
		}}
		spec, _, err := Resolve(input(custom(task), content.ModeIronman, nil, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		it := spec.Bank.Items
		if len(it) != 3 || !slices.Equal(it[0].IDs, []int{1359, 1351}) || len(it[1].IDs) < 2 || !it[1].Optional || it[1].Qty != 2 || !it[2].Noted || it[2].Qty != 50 {
			t.Fatalf("items = %+v", it)
		}
	})
	t.Run("buy what is missing", func(t *testing.T) {
		task := content.Task{Type: content.TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 5}
		h := rules.ItemHoldings{"inventory": live(map[int]int64{1351: 2}), "bank": live(map[int]int64{}), "equipment": live(map[int]int64{})}
		spec, _, err := Resolve(input(custom(task), content.ModeIronman, h, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		b := spec.Buy
		if b.Qty != 3 || b.Item != 1351 || b.ItemName != "Bronze axe" || len(b.NPCs) == 0 || b.Shop != "shop:bobs_brilliant_axes" || strings.HasSuffix(b.ShopName, ".") {
			t.Fatalf("buy = %+v", b)
		}
	})
	fisher := content.Task{Type: content.TaskTrain, Skill: ref("skill:fishing"), Location: ref("location:lumbridge"),
		Plugin: "a.b.TickFisherPlugin", Config: []content.TaskSetting{{Group: "tickfisher", Key: "mode", Value: "SHRIMP"}},
		Until: &content.TaskUntil{Objective: key("skill:fishing>=76")}}
	withFishing := func(in Input, level int) Input {
		ps := *in.Facts.State
		ps.Skills = map[string]rules.SkillState{"fishing": {Level: level}}
		in.Facts.State = &ps
		return in
	}
	t.Run("train delegated to its plugin", func(t *testing.T) {
		spec, v, err := Resolve(withFishing(input(custom(fisher), content.ModeHCIM, nil, nil), 70), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		tr := spec.Train
		if spec.Type != execspec.TypeTrain || tr == nil || tr.Skill != "skill:fishing" || tr.SkillName != "Fishing" || tr.Level != 76 ||
			tr.Plugin != "a.b.TickFisherPlugin" || len(tr.Config) != 1 || tr.Combat || tr.Until() != "skill:fishing>=76" {
			t.Fatalf("train = %+v", tr)
		}
		if !v.Executable || len(v.Risks) != 0 || v.Supervised || v.State != rules.Unmet || !strings.Contains(v.Label, "(plugin TickFisherPlugin)") {
			t.Fatalf("view = %+v", v)
		}
	})
	t.Run("train on a regular account runs unsupervised even when it fights", func(t *testing.T) {
		goblins := content.Task{Type: content.TaskTrain, Skill: ref("skill:magic"), NPC: ref("npc:goblin"), Plugin: "a.b.Splasher",
			Until: &content.TaskUntil{Objective: key("skill:magic>=33")}}
		spec, v, err := Resolve(input(custom(goblins), content.ModeRegular, nil, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		if spec.Type != execspec.TypeTrain || !v.Executable || v.Supervised || !slices.Equal(v.Risks, []string{"combat"}) {
			t.Fatalf("regular goblins = %+v %+v", spec.Train, v)
		}
		bank := input(route, content.ModeRegular, nil, nil)
		if _, v, err = Resolve(bank, "step:t::b#0"); err != nil || !v.Executable {
			t.Fatalf("regular bank = %+v %v", v, err)
		}
	})
	t.Run("train on a monster fights: combat, HCIM supervised", func(t *testing.T) {
		goblins := content.Task{Type: content.TaskTrain, Skill: ref("skill:magic"), NPC: ref("npc:goblin"), Plugin: "a.b.Splasher",
			Until: &content.TaskUntil{Objective: key("skill:magic>=33")}}
		spec, v, err := Resolve(input(custom(goblins), content.ModeHCIM, nil, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		if !spec.Train.Combat || !slices.Equal(spec.Risks(), []string{"combat"}) || !slices.Equal(v.Risks, []string{"combat"}) || !v.Supervised {
			t.Fatalf("goblins = %+v %+v", spec.Train, v)
		}
		strength := content.Task{Type: content.TaskTrain, Skill: ref("skill:strength"), Plugin: "a.b.Fighter",
			Until: &content.TaskUntil{Objective: key("skill:strength>=40")}}
		if spec, _, err = Resolve(input(custom(strength), content.ModeIronman, nil, nil), "task:0#0"); err != nil || !spec.Train.Combat {
			t.Fatalf("strength = %+v %v", spec.Train, err)
		}
	})
	cooks := content.Task{Type: content.TaskQuest, Quest: ref("quest:cooks_assistant")}
	withQuests := func(in Input, quests map[string]string, skills map[string]rules.SkillState) Input {
		ps := *in.Facts.State
		ps.Quests, ps.Skills = quests, skills
		in.Facts.State = &ps
		return in
	}
	t.Run("quest delegated to the Quest Helper: a death risk, HCIM supervised", func(t *testing.T) {
		spec, v, err := Resolve(withQuests(input(custom(cooks), content.ModeHCIM, nil, nil), map[string]string{"Cook's Assistant": "IN_PROGRESS"}, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		q := spec.Quest
		if spec.Type != execspec.TypeQuest || q == nil || q.Quest != "quest:cooks_assistant" || q.Name != "Cook's Assistant" ||
			q.Helper != "Cook's Assistant" || q.Safe || q.Until() != "quest:cooks_assistant@finished" {
			t.Fatalf("quest = %+v", q)
		}
		if !v.Executable || !slices.Equal(v.Risks, []string{"combat", "hazard"}) || !slices.Equal(spec.Risks(), v.Risks) || !v.Supervised ||
			v.State != rules.Unmet || v.Label != "Fazer a quest Cook's Assistant (Quest Helper)" || v.Until == "" {
			t.Fatalf("view = %+v", v)
		}
	})
	t.Run("a quest the plan says is safe", func(t *testing.T) {
		safe := cooks
		safe.Safe = true
		spec, v, err := Resolve(input(custom(safe), content.ModeHCIM, nil, nil), "task:0#0")
		if err != nil || !spec.Quest.Safe || len(spec.Risks()) != 0 || len(v.Risks) != 0 || v.Supervised {
			t.Fatalf("safe = %+v %+v %v", spec.Quest, v, err)
		}
	})
	chompy := content.Task{Type: content.TaskQuest, Quest: ref("quest:big_chompy_bird_hunting")}
	ready := map[string]rules.SkillState{"fletching": {Level: 5}, "cooking": {Level: 30}, "ranged": {Level: 30}}
	t.Run("a quest whose requirements are proven", func(t *testing.T) {
		spec, _, err := Resolve(withQuests(input(custom(chompy), content.ModeIronman, nil, nil), map[string]string{}, ready), "task:0#0")
		if err != nil || spec.Quest.Helper != "Big Chompy Bird Hunting" {
			t.Fatalf("chompy = %+v %v", spec.Quest, err)
		}
	})
	noPlugin := fisher
	noPlugin.Plugin, noPlugin.Config = "", nil
	for _, tc := range []struct {
		name string
		in   Input
		key  string
		want string
	}{
		{"train met", withFishing(input(custom(fisher), content.ModeIronman, nil, nil), 76), "task:0#0", "já está cumprida"},
		{"train without plugin", input(custom(noPlugin), content.ModeIronman, nil, nil), "task:0#0", "não diz qual plugin"},
		{"quest finished", withQuests(input(custom(cooks), content.ModeIronman, nil, nil), map[string]string{"Cook's Assistant": "FINISHED"}, nil), "task:0#0", "já está cumprida"},
		{"quest lacking a level", withQuests(input(custom(chompy), content.ModeIronman, nil, nil), map[string]string{},
			map[string]rules.SkillState{"fletching": {Level: 5}, "cooking": {Level: 30}, "ranged": {Level: 29}}), "task:0#0", "faltam requisitos da quest: 30 Ranged"},
		{"quest with an unproven level", withQuests(input(custom(chompy), content.ModeIronman, nil, nil), map[string]string{},
			map[string]rules.SkillState{"fletching": {Level: 5}}), "task:0#0", "não prova os requisitos"},
		{"met", input(route, content.ModeIronman, nil, at(3188, 3220)), "step:t::a#0", "já está cumprida"},
		{"talk", input(route, content.ModeIronman, nil, nil), "step:t::a#1", "só roda travel, bank, buy, train, quest, ge_buy e ge_sell"},
		{"uim bank", input(route, content.ModeUIM, nil, nil), "step:t::b#0", "Ultimate Ironman"},
		{"via", input(custom(content.Task{Type: content.TaskTravel, Location: ref("location:lumbridge"), Via: ref("item:8007")}), content.ModeIronman, nil, nil), "task:0#0", "via"},
		{"buy owned", input(route, content.ModeIronman, rules.ItemHoldings{"bank": live(map[int]int64{1351: 1})}, nil), "step:t::b#1", "já está cumprida"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Resolve(tc.in, tc.key); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	if _, _, err := Resolve(input(route, content.ModeIronman, nil, nil), "step:t::zz#0"); !errors.Is(err, ErrNotATask) {
		t.Errorf("unknown key: %v", err)
	}
}

// TestResolveEveryMappedLocation: a travel to any location the catalog
// maps resolves to a task the agent and the plugin accept (the area's size
// and polygon within bounds, a target inside it, the wilderness flag).
func TestResolveEveryMappedLocation(t *testing.T) {
	c, reg := load(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "content", "reference", "location_maps.json"))
	if err != nil {
		t.Fatal(err)
	}
	var maps struct {
		Maps []struct {
			ID   string          `json:"id"`
			Area json.RawMessage `json:"area"`
		} `json:"maps"`
	}
	if err := json.Unmarshal(raw, &maps); err != nil {
		t.Fatal(err)
	}
	ps := rules.PlayerState{Catalog: reg}
	n := 0
	for _, m := range maps.Maps {
		if len(m.Area) == 0 {
			continue
		}
		task := content.Task{Type: content.TaskTravel, Location: ref("location:" + m.ID)}
		in := Input{
			Plan: Plan{ID: "pln_test0001", Kind: KindCustom, Items: []Item{{Kind: ItemTask, Task: &task}}}, Content: c, Registry: reg,
			Facts: panel.Facts{Mode: content.ModeIronman, State: &ps}, Now: now,
		}
		spec, _, err := Resolve(in, "task:0#0")
		if err != nil {
			t.Errorf("%s: %v", m.ID, err)
			continue
		}
		if spec.Travel.Wilderness != slices.Contains(spec.Risks(), "wilderness") {
			t.Errorf("%s: wilderness flag and risks disagree", m.ID)
		}
		n++
	}
	if n < 80 {
		t.Fatalf("only %d mapped locations resolved", n)
	}
}

func TestTarget(t *testing.T) {
	box := execspec.Area{MinX: 10, MinY: 10, MaxX: 20, MaxY: 20}
	if got, ok := Target(box, execspec.Tile{X: 12, Y: 18}); !ok || got != (execspec.Tile{X: 12, Y: 18}) {
		t.Errorf("point inside = %+v", got)
	}
	if got, ok := Target(box, execspec.Tile{X: 0, Y: 0}); !ok || got != (execspec.Tile{X: 15, Y: 15}) {
		t.Errorf("point outside = %+v", got)
	}
	// An L-shaped polygon whose box center is outside it.
	l := execspec.Area{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10, Polygon: [][2]int{{0, 0}, {10, 0}, {10, 2}, {2, 2}, {2, 10}, {0, 10}}}
	got, ok := Target(l, execspec.Tile{X: 5, Y: 5})
	if !ok || !l.Contains(got.X, got.Y, 0) {
		t.Errorf("L = %+v %v", got, ok)
	}
	if _, ok := Target(execspec.Area{Plane: 1, MinX: 0, MinY: 0, MaxX: 2, MaxY: 2, Polygon: [][2]int{{0, 0}, {0, 0}, {0, 0}}}, execspec.Tile{}); !ok {
		t.Error("a degenerate polygon still has its point")
	}
}

// TestQuestHelperName: the name the client's Quest Helper lists a quest by,
// matched through the catalog as the ingest matches the client's quest
// names; never a guess.
func TestQuestHelperName(t *testing.T) {
	_, reg := load(t)
	listed := []string{"Cook's Assistant", "Sheep Shearer", "The Restless Ghost", "Ardougne Easy", "Alfred Grimhand's Barcrawl"}
	for _, tc := range []struct {
		name   string
		listed []string
		quest  string
		want   string
		err    string
	}{
		{"the catalog's name", listed, "quest:cooks_assistant", "Cook's Assistant", ""},
		{"with its article", listed, "quest:the_restless_ghost", "The Restless Ghost", ""},
		{"article-less, the only one", []string{"Restless Ghost"}, "quest:the_restless_ghost", "Restless Ghost", ""},
		{"the exact one of two", []string{"Restless Ghost", "The Restless Ghost"}, "quest:the_restless_ghost", "The Restless Ghost", ""},
		{"not listed", listed, "quest:x_marks_the_spot", "", "não lista X Marks the Spot"},
		{"two names, none exact", []string{"Restless Ghost", "restless ghost"}, "quest:the_restless_ghost", "", "mais de um nome"},
		{"an empty list", nil, "quest:cooks_assistant", "", "não lista"},
	} {
		got, err := QuestHelperName(tc.listed, *ref(tc.quest), reg)
		switch {
		case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.err)
		case tc.err == "" && (err != nil || got != tc.want):
			t.Errorf("%s: %q %v, want %q", tc.name, got, err, tc.want)
		}
	}
}
