package progress

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Shilo Village in the catalog: Jungle Potion (Druidic Ritual, 3 Herblore
// boostable), 20 Crafting and 32 Agility.
const goalGuides = `[
 {"id": "act", "title": "Active", "sections": [{"id": "s1", "slug": "s1", "title": "S1", "steps": [
   {"id": "x1", "text": "Druidic Ritual", "objectives": [{"key": "quest:druidic_ritual@finished", "role": "completes", "source": {"method": "explicit", "confidence": "high"}}]},
   {"id": "x2", "text": "Agility to 30", "objectives": [{"key": "skill:agility>=30", "role": "completes", "source": {"method": "explicit", "confidence": "high"}}]},
   {"id": "x3", "text": "Jungle Potion", "objectives": [{"key": "quest:jungle_potion@finished", "role": "completes", "source": {"method": "explicit", "confidence": "high"}}]},
   {"id": "x4", "text": "Crafting to 25", "objectives": [{"key": "skill:crafting>=25", "role": "completes", "source": {"method": "explicit", "confidence": "high"}}]},
   {"id": "x5", "text": "Shilo Village", "objectives": [{"key": "quest:shilo_village@finished", "role": "completes", "source": {"method": "explicit", "confidence": "high"}}]}
 ]}]},
 {"id": "other", "title": "Other", "sections": [{"id": "o1", "slug": "o1", "title": "O1", "steps": [
   {"id": "y0", "text": "walk"},
   {"id": "y1", "text": "Start Shilo", "objectives": [{"key": "quest:shilo_village@started", "role": "starts", "source": {"method": "explicit", "confidence": "high"}}]}
 ]}]}
]`

func goalInput(t *testing.T, goal string) GoalPlanInput {
	t.Helper()
	var gs []*content.Guide
	if err := json.Unmarshal([]byte(goalGuides), &gs); err != nil {
		t.Fatal(err)
	}
	in := GoalPlanInput{
		Goal:     entities.MustParseObjectiveKey(goal),
		Registry: registry(t),
		State:    rules.PlayerState{Skills: skills(map[string]int{"agility": 20, "crafting": 30, "herblore": 5})},
		Verdicts: Verdicts{},
	}
	for _, g := range gs {
		g.Index()
		gi := GuideInput{Guide: g, Verdicts: Verdicts{}, Marks: map[string]Mark{}, LastStatus: map[string]Status{}, Account: Account{ActiveGuideID: "act"}}
		if g.ID == "act" {
			gi.Marks["x1"] = Mark{Kind: MarkManualDone}
		}
		in.Guides = append(in.Guides, gi)
	}
	return in
}

func TestBuildGoalPlanQuest(t *testing.T) {
	p := BuildGoalPlan(goalInput(t, "quest:shilo_village@finished"))
	if !p.Requirements || p.Met != 2 || p.Presumed != 1 || p.Total != 5 {
		t.Fatalf("counts = req %v met %d presumed %d total %d", p.Requirements, p.Met, p.Presumed, p.Total)
	}
	var order []string
	byKey := map[string]GoalNode{}
	for _, n := range p.Nodes {
		order = append(order, n.Key)
		byKey[n.Key] = n
	}
	want := "[quest:druidic_ritual@finished skill:crafting>=20 skill:herblore>=3 quest:jungle_potion@finished skill:agility>=32 quest:shilo_village@finished]"
	if fmt.Sprint(order) != want {
		t.Fatalf("order = %v", order)
	}
	cases := []struct {
		key     string
		group   GoalGroup
		depth   int
		missing string
	}{
		{"quest:druidic_ritual@finished", GroupPresumed, 2, "[]"},
		{"skill:herblore>=3", GroupDone, 2, "[]"},
		{"quest:jungle_potion@finished", GroupUnknown, 1, "[]"},
		{"skill:agility>=32", GroupBlocked, 1, "[]"},
		{"quest:shilo_village@finished", GroupUnknown, 0, "[quest:jungle_potion@finished skill:agility>=32]"},
	}
	for _, c := range cases {
		n := byKey[c.key]
		if n.Group != c.group || n.Depth != c.depth || fmt.Sprint(n.Missing) != c.missing {
			t.Errorf("%s = group %s depth %d missing %v", c.key, n.Group, n.Depth, n.Missing)
		}
	}
	if n := byKey["skill:herblore>=3"]; !n.Boostable || fmt.Sprint(n.For) != "[quest:jungle_potion@finished]" {
		t.Errorf("herblore = %+v", n)
	}
	if n := byKey["skill:crafting>=20"]; n.Step == nil || n.Step.StepID != "x4" {
		t.Errorf("crafting step = %+v", n.Step)
	}

	var bn []string
	for _, b := range p.Bottlenecks {
		bn = append(bn, fmt.Sprintf("%s:%d/%d:%v", b.Skill, *b.Level, b.Target, b.Met))
	}
	if fmt.Sprint(bn) != "[agility:20/32:false herblore:5/3:true crafting:30/20:true]" {
		t.Fatalf("bottlenecks = %v", bn)
	}
	if a := p.Bottlenecks[0]; a.Step != nil || a.Reach != 30 || a.ReachStep == nil || a.ReachStep.StepID != "x2" {
		t.Fatalf("agility reach = %+v", a)
	}

	var routes []string
	for _, r := range p.Routes {
		routes = append(routes, fmt.Sprintf("%s/%s/%d/%v", r.GuideID, r.StepID, r.SectionPos, r.Goal))
	}
	if fmt.Sprint(routes) != "[act/x5/5/true other/y1/2/false]" {
		t.Fatalf("routes = %v", routes)
	}
}

func TestBuildGoalPlanLatchAndItem(t *testing.T) {
	in := goalInput(t, "quest:shilo_village@finished")
	// A latched met verdict wins over the live facts (agility is 20 now).
	in.Verdicts["skill:agility>=32"] = rules.Met
	p := BuildGoalPlan(in)
	if p.Met != 3 || p.Bottlenecks[0].Skill != "agility" || !p.Bottlenecks[0].Met {
		t.Fatalf("latched = met %d bottlenecks %+v", p.Met, p.Bottlenecks)
	}

	item := BuildGoalPlan(goalInput(t, "item:7462x1"))
	if item.Requirements || item.Total != 0 || len(item.Nodes) != 1 || len(item.Routes) != 0 {
		t.Fatalf("item goal = %+v", item)
	}
}

// An item goal has no requirements in the catalog: the plan's lists are empty,
// never null, so the web can read them without a guard.
func TestBuildGoalPlanItemListsAreEmpty(t *testing.T) {
	p := BuildGoalPlan(goalInput(t, "item:7462x1"))
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"bottlenecks":[]`, `"nodes":[`} {
		if !strings.Contains(string(b), field) {
			t.Errorf("plan %s lacks %s", b, field)
		}
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("plan has a null: %s", b)
	}
}
