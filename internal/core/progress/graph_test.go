package progress

import (
	"fmt"
	"testing"
)

func columnKeys(c GraphColumn) string {
	var open, done []string
	for _, n := range c.Nodes {
		open = append(open, n.Key)
	}
	for _, n := range c.Done {
		done = append(done, n.Key)
	}
	return fmt.Sprintf("%d open %v done %v", c.Depth, open, done)
}

// shiloUnlocks are the catalog quests that require Shilo Village (wiki
// quest ids keep the game's British spelling).
const shiloUnlocks = "[quest:legends_quest@finished quest:lunar_diplomacy@finished quest:one_small_favour@finished quest:recipe_for_disaster_freeing_sir_amik_varze@finished]" //nolint:misspell // quest id

func TestBuildGraph(t *testing.T) {
	cases := []struct {
		name     string
		focus    string
		depth    int
		columns  []string
		critical string
		counts   string
		// unlocks come from the real quest catalog
		unlocks string
	}{
		{
			name: "shilo at depth 2", focus: "quest:shilo_village@finished", depth: 2,
			columns: []string{
				"1 open [quest:jungle_potion@finished levels:quest:shilo_village@finished] done []",
				"2 open [quest:druidic_ritual@finished] done [levels:quest:jungle_potion@finished]",
			},
			critical: "[quest:jungle_potion@finished]", counts: "4/1/2", unlocks: shiloUnlocks,
		},
		{
			name: "shilo at depth 1", focus: "quest:shilo_village@finished", depth: 1,
			columns:  []string{"1 open [quest:jungle_potion@finished levels:quest:shilo_village@finished] done []"},
			critical: "[quest:jungle_potion@finished]", counts: "2/0/2", unlocks: shiloUnlocks,
		},
		{
			name: "out of range depth is the default", focus: "quest:shilo_village@finished", depth: 9,
			columns: []string{
				"1 open [quest:jungle_potion@finished levels:quest:shilo_village@finished] done []",
				"2 open [quest:druidic_ritual@finished] done [levels:quest:jungle_potion@finished]",
			},
			critical: "[quest:jungle_potion@finished]", counts: "4/1/2", unlocks: shiloUnlocks,
		},
		{
			name: "druidic ritual unlocks jungle potion", focus: "quest:druidic_ritual@finished", depth: 2,
			critical: "[]", counts: "0/0/0", unlocks: "[quest:eadgars_ruse@finished quest:jungle_potion@finished quest:recruitment_drive@finished quest:song_of_the_elves@finished]",
		},
		{
			name: "a level alone has no requirements", focus: "skill:agility>=32", depth: 3,
			critical: "[]", counts: "0/0/0", unlocks: "[quest:death_on_the_isle@finished quest:shilo_village@finished]",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := BuildGraph(GraphInput{GoalPlanInput: goalInput(t, c.focus), Depth: c.depth})
			var cols []string
			for _, col := range g.Columns {
				cols = append(cols, columnKeys(col))
			}
			if fmt.Sprint(cols) != fmt.Sprint(c.columns) {
				t.Errorf("columns:\n%v\nwant:\n%v", cols, c.columns)
			}
			if fmt.Sprint(g.Critical) != c.critical {
				t.Errorf("critical = %v", g.Critical)
			}
			if got := fmt.Sprintf("%d/%d/%d", g.Nodes, g.DoneNodes, g.MaxDepth); got != c.counts {
				t.Errorf("nodes/done/max depth = %s", got)
			}
			var unlocks []string
			for _, u := range g.Unlocks {
				unlocks = append(unlocks, u.Key)
			}
			if fmt.Sprint(unlocks) != c.unlocks {
				t.Errorf("unlocks = %v", unlocks)
			}
			if g.Focus.Key != c.focus {
				t.Errorf("focus = %+v", g.Focus)
			}
		})
	}
}

func TestBuildGraphCards(t *testing.T) {
	g := BuildGraph(GraphInput{GoalPlanInput: goalInput(t, "quest:shilo_village@finished"), Depth: 2})
	levels := g.Columns[0].Nodes[1]
	if levels.Kind != "levels" || levels.Label != "32 Agility · 20 Crafting" || levels.Group != GroupBlocked ||
		!levels.Critical || fmt.Sprint(levels.For) != "[Shilo Village]" {
		t.Fatalf("levels card = %+v", levels)
	}
	if a := levels.Levels[0]; a.Skill != "Agility" || a.Target != 32 || a.Level == nil || *a.Level != 20 || a.Verdict != "unmet" {
		t.Fatalf("agility member = %+v", a)
	}
	jungle := g.Columns[0].Nodes[0]
	if jungle.Label != "Jungle Potion" || !jungle.Critical || fmt.Sprint(jungle.For) != "[Shilo Village]" ||
		jungle.Step == nil || jungle.Step.StepID != "x3" || jungle.Group != GroupUnknown {
		t.Fatalf("jungle potion card = %+v", jungle)
	}
	druidic := g.Columns[1].Nodes[0]
	if druidic.Group != GroupPresumed || fmt.Sprint(druidic.For) != "[Jungle Potion]" || druidic.Critical {
		t.Fatalf("druidic card = %+v", druidic)
	}
	if g.Focus.Label != "Shilo Village" || g.Focus.Kind != "quest" || len(g.Focus.Missing) != 2 || len(g.Routes) != 2 {
		t.Fatalf("focus = %+v routes %d", g.Focus, len(g.Routes))
	}
}

// A quest with nothing missing before it is available, also when it has no
// requirements at all; a level never is (there is only training).
func TestGraphAvailableQuest(t *testing.T) {
	in := goalInput(t, "quest:jungle_potion@finished")
	in.State.Quests = map[string]string{"Jungle Potion": "NOT_STARTED", "Druidic Ritual": "NOT_STARTED"}
	for i := range in.Guides {
		in.Guides[i].Marks = map[string]Mark{}
	}
	g := BuildGraph(GraphInput{GoalPlanInput: in, Depth: 1})
	byKey := map[string]GraphNode{}
	for _, c := range g.Columns {
		for _, n := range append(c.Nodes, c.Done...) {
			byKey[n.Key] = n
		}
	}
	if n := byKey["quest:druidic_ritual@finished"]; n.Verdict != "unmet" || n.Group != GroupAvailable {
		t.Fatalf("druidic ritual = %+v", n)
	}
	if g.Focus.Group != GroupBlocked || fmt.Sprint(g.Focus.Missing) != "[Druidic Ritual]" {
		t.Fatalf("focus = %+v", g.Focus)
	}
}
