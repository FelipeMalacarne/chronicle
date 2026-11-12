package planning

import (
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func obtains(key string) content.Objective {
	return content.Objective{Key: entities.MustParseObjectiveKey(key), Role: content.RoleCompletes}
}

func aheadLeg() *content.Segment {
	return &content.Segment{ID: "leg", Kind: content.SegmentBankLeg, Steps: []*content.Step{
		{ID: "1", Text: "bank"},
		{ID: "2", Text: "build", Requirements: []content.ItemRequirement{item("Hammer", ip(hammer), nil), item("Planks", ip(plank), ip(3))}},
		{ID: "3", Text: "buy logs", Objectives: []content.Objective{obtains("item:1511x5")}},
		{ID: "4", Text: "burn", Requirements: []content.ItemRequirement{item("Logs", ip(logs), ip(5)), item("Hammer", ip(hammer), nil)}},
		{ID: "5", Text: "chop", Requirements: []content.ItemRequirement{{Name: "Axe", AnyOf: []int{ironAxe, bronzeAxe}, SourceText: "Axe"}, {Name: "cat", SourceText: "cat"}}},
		{ID: "6", Text: "chop again", Requirements: []content.ItemRequirement{{Name: "Axe", AnyOf: []int{bronzeAxe, ironAxe}}, {Name: "Cat"}}},
	}}
}

func TestLookAhead(t *testing.T) {
	h := holdings(map[int]int64{hammer: 1}, map[int]int64{})
	a := LookAhead(aheadLeg(), "2", h, nil)
	if a == nil {
		t.Fatal("LookAhead = nil")
	}
	if a.FromStepID != "2" || a.UntilStepID != "6" || a.Steps != 4 {
		t.Fatalf("window = %s..%s (%d)", a.FromStepID, a.UntilStepID, a.Steps)
	}
	type row struct {
		step   string
		offset int
		items  []string
	}
	want := []row{{"2", 0, []string{"Hammer", "Planks"}}, {"5", 3, []string{"Axe", "cat"}}}
	if len(a.Groups) != len(want) {
		t.Fatalf("groups = %+v", a.Groups)
	}
	for i, w := range want {
		g := a.Groups[i]
		if g.StepID != w.step || g.Offset != w.offset || len(g.Items) != len(w.items) {
			t.Fatalf("group %d = %+v, want %+v", i, g, w)
		}
		for j, name := range w.items {
			if g.Items[j].Name != name {
				t.Errorf("group %d item %d = %q, want %q", i, j, g.Items[j].Name, name)
			}
		}
	}
	hammerItem := a.Groups[0].Items[0]
	if hammerItem.Verdict != rules.Met || len(hammerItem.AlsoAt) != 1 || hammerItem.AlsoAt[0] != "4" {
		t.Errorf("hammer = %+v, want carried and also at 4", hammerItem)
	}
	if planks := a.Groups[0].Items[1]; planks.Verdict != rules.Unmet || planks.Required != 3 {
		t.Errorf("planks = %+v, want 3 missing", planks)
	}
	// Any-of ids in another order and names in another case are the same item.
	if axe := a.Groups[1].Items[0]; len(axe.AlsoAt) != 1 || axe.AlsoAt[0] != "6" {
		t.Errorf("axe also at = %v, want [6]", axe.AlsoAt)
	}
	if cat := a.Groups[1].Items[1]; cat.Verdict != rules.Unknown || len(cat.AlsoAt) != 1 {
		t.Errorf("cat = %+v, want unresolved and also at 6", cat)
	}
	if len(a.Obtained) != 1 {
		t.Fatalf("obtained = %+v", a.Obtained)
	}
	if o := a.Obtained[0]; o.ItemID != logs || o.Qty != 5 || o.StepID != "3" || o.UseStepID != "4" || o.UseOffset != 2 {
		t.Errorf("obtained = %+v", o)
	}
}

func TestLookAheadEdges(t *testing.T) {
	h := holdings(nil, nil)
	if a := LookAhead(aheadLeg(), "missing", h, nil); a != nil {
		t.Errorf("unknown step: %+v, want nil", a)
	}
	if a := LookAhead(aheadLeg(), "6", h, nil); a == nil || a.Steps != 0 || len(a.Groups) != 1 {
		t.Errorf("last step: %+v, want one group", a)
	}
	if a := LookAhead(aheadLeg(), "", h, nil); a == nil || a.FromStepID != "1" {
		t.Errorf("empty step: %+v, want the leg start", a)
	}
	// A step that obtains what it also uses still lists it.
	seg := &content.Segment{Steps: []*content.Step{{ID: "1", Requirements: []content.ItemRequirement{item("Logs", ip(logs), nil)}, Objectives: []content.Objective{obtains("item:1511x1")}}}}
	if a := LookAhead(seg, "1", h, nil); a == nil || len(a.Groups) != 1 || len(a.Obtained) != 0 {
		t.Errorf("same-step acquisition: %+v", a)
	}
	empty := &content.Segment{Steps: []*content.Step{{ID: "1"}}}
	if a := LookAhead(empty, "1", h, nil); a != nil {
		t.Errorf("no requirements: %+v, want nil", a)
	}
}
