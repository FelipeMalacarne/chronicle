package progress

import (
	"fmt"
	"slices"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

func pathKeys(g Graph) []string {
	var out []string
	for _, s := range g.Critical {
		out = append(out, s.Key)
	}
	return out
}

func graphKeys(g Graph) []string {
	var out []string
	for _, n := range g.Nodes {
		out = append(out, n.Key)
	}
	return out
}

func TestBuildGraphDepthAndCriticalPath(t *testing.T) {
	in := goalInput(t, "quest:shilo_village@finished")
	full := BuildGraph(in, 3)
	if full.Focus != "quest:shilo_village@finished" || full.Label != "Shilo Village" || full.Kind != "quest" {
		t.Fatalf("focus = %q %q %q", full.Focus, full.Label, full.Kind)
	}
	if full.Depth != 3 || full.MaxDepth != 2 || full.Hidden != 0 || len(full.Nodes) != 6 {
		t.Fatalf("depth %d max %d hidden %d nodes %v", full.Depth, full.MaxDepth, full.Hidden, graphKeys(full))
	}
	// The counts are the goal plan's, over the whole closure.
	if !full.Requirements || full.Met != 2 || full.Presumed != 1 || full.Total != 5 {
		t.Fatalf("counts = %+v", full)
	}
	// Jungle Potion (unknown) and 32 Agility (blocked) are both one node deep
	// and open; the plan orders Jungle Potion first, so the tie goes to it.
	// Druidic Ritual is presumed (its step is marked done): not on the path.
	if fmt.Sprint(pathKeys(full)) != "[quest:shilo_village@finished quest:jungle_potion@finished]" || full.Critical[1].Label != "Jungle Potion" {
		t.Fatalf("critical = %v", full.Critical)
	}
	byKey := map[string]GraphNode{}
	for _, n := range full.Nodes {
		byKey[n.Key] = n
	}
	if n := byKey["skill:agility>=32"]; n.Kind != "skill" || n.Have == nil || *n.Have != 20 || n.Critical {
		t.Fatalf("agility = %+v", n)
	}
	if n := byKey["quest:jungle_potion@finished"]; !n.Critical || n.Have != nil || fmt.Sprint(n.For) != "[quest:shilo_village@finished]" {
		t.Fatalf("jungle potion = %+v", n)
	}
	if n := byKey["quest:druidic_ritual@finished"]; n.Group != GroupPresumed || n.Critical {
		t.Fatalf("druidic ritual = %+v", n)
	}

	shallow := BuildGraph(in, 1)
	if shallow.Depth != 1 || shallow.MaxDepth != 2 || shallow.Hidden != 2 || len(shallow.Nodes) != 4 {
		t.Fatalf("depth 1: hidden %d nodes %v", shallow.Hidden, graphKeys(shallow))
	}
	if slices.Contains(graphKeys(shallow), "skill:herblore>=3") {
		t.Fatalf("depth 1 kept a depth 2 node: %v", graphKeys(shallow))
	}
	if fmt.Sprint(shallow.Critical) != fmt.Sprint(full.Critical) {
		t.Fatalf("the critical path depends on the depth shown: %v", shallow.Critical)
	}
	for _, d := range []int{0, -1, MaxGraphDepth + 1} {
		if g := BuildGraph(in, d); g.Depth != DefaultGraphDepth {
			t.Fatalf("depth %d = %d, want the default", d, g.Depth)
		}
	}
}

func TestBuildGraphMetFocusHasNoCriticalPath(t *testing.T) {
	in := goalInput(t, "skill:crafting>=20")
	g := BuildGraph(in, 3)
	if len(g.Critical) != 0 || g.Requirements || len(g.Nodes) != 1 || g.Nodes[0].Group != GroupDone {
		t.Fatalf("met level = critical %v nodes %+v", g.Critical, g.Nodes)
	}
	if g.Nodes[0].Have == nil || *g.Nodes[0].Have != 30 {
		t.Fatalf("have = %v", g.Nodes[0].Have)
	}
}

func TestBuildGraphUnlocks(t *testing.T) {
	in := goalInput(t, "quest:jungle_potion@finished")
	g := BuildGraph(in, 3)
	var shilo *GraphLink
	for i := range g.Unlocks {
		if g.Unlocks[i].Key == "quest:shilo_village@finished" {
			shilo = &g.Unlocks[i]
		}
	}
	if shilo == nil || g.UnlocksTotal < 1 {
		t.Fatalf("Jungle Potion does not unlock Shilo Village: %+v", g.Unlocks)
	}
	// Shilo needs Jungle Potion (unknown) and 32 Agility (20): two missing.
	if shilo.Label != "Shilo Village" || shilo.Kind != "quest" || shilo.Verdict != rules.Unknown || shilo.Missing != 2 {
		t.Fatalf("shilo = %+v", *shilo)
	}
	for i := 1; i < len(g.Unlocks); i++ {
		a, b := g.Unlocks[i-1], g.Unlocks[i]
		if a.Verdict != rules.Met && b.Verdict != rules.Met && a.Missing > b.Missing {
			t.Fatalf("unlocks out of order: %+v before %+v", a, b)
		}
	}

	// A level unlocks every quest whose requirement on that skill it reaches.
	lvl := BuildGraph(goalInput(t, "skill:agility>=99"), 3)
	if len(lvl.Unlocks) != min(lvl.UnlocksTotal, maxGraphUnlocks) || lvl.UnlocksTotal < 10 {
		t.Fatalf("99 Agility unlocks %d of %d", len(lvl.Unlocks), lvl.UnlocksTotal)
	}

	// Without a catalog there are no requirements and nothing to unlock.
	in.Registry = nil
	if g := BuildGraph(in, 3); len(g.Unlocks) != 0 || g.UnlocksTotal != 0 || g.Label != "jungle_potion" {
		t.Fatalf("no catalog = %+v", g)
	}
}

func TestCriticalPathCycle(t *testing.T) {
	nodes := []GoalNode{
		{Key: "a", Group: GroupBlocked, Requires: []string{"b"}},
		{Key: "b", Group: GroupBlocked, Requires: []string{"c"}},
		{Key: "c", Group: GroupBlocked, Requires: []string{"b"}},
	}
	if got := criticalPath(nodes, "a"); fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("cycle = %v", got)
	}
	if got := criticalPath(nodes, "missing"); len(got) != 0 {
		t.Fatalf("unknown focus = %v", got)
	}
}

// A quest without catalog requirements that is not done can be started now:
// available, never blocked (blocked is for levels and quest points to train).
func TestRequirementFreeQuestIsAvailable(t *testing.T) {
	in := goalInput(t, "quest:jungle_potion@finished")
	in.State.Quests = map[string]string{"Druidic Ritual": "NOT_STARTED"}
	g := BuildGraph(in, 3)
	for _, n := range g.Nodes {
		switch n.Key {
		case "quest:druidic_ritual@finished":
			if n.Verdict != rules.Unmet || n.Group != GroupAvailable {
				t.Fatalf("druidic ritual = %s %s, want unmet available", n.Verdict, n.Group)
			}
		case "skill:herblore>=3":
			if n.Group != GroupDone {
				t.Fatalf("herblore = %s", n.Group)
			}
		}
	}
	in.State.Skills["herblore"] = rules.SkillState{Level: 1}
	for _, n := range BuildGraph(in, 3).Nodes {
		if n.Key == "skill:herblore>=3" && n.Group != GroupBlocked {
			t.Fatalf("an unmet level = %s, want blocked", n.Group)
		}
	}
}
