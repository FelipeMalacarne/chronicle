package progress

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

const (
	slayer48   = "skill:slayer>=48"
	agility62  = "skill:agility>=62"
	twilights  = "quest:twilights_promise@finished"
	perilous   = "quest:perilous_moons@finished"
	floatGuide = `{
  "id": "f", "title": "Floating", "default_variant": "main",
  "variants": [{ "id": "main", "title": "Main", "sections": [{ "id": "s1", "slug": "s1", "title": "S1", "steps": [
    { "id": "f1", "text": "Complete Perilous Moons when you reach 48 Slayer",
      "objectives": [
        { "key": "quest:perilous_moons@finished", "role": "completes", "source": { "method": "llm", "confidence": "high" } },
        { "key": "quest:twilights_promise@finished", "role": "requires", "source": { "method": "llm", "confidence": "high" } }
      ] },
    { "id": "f2", "text": "Run a course",
      "objectives": [ { "key": "skill:agility>=62", "role": "requires", "source": { "method": "llm", "confidence": "high" } } ] },
    { "id": "f3", "text": "Train slayer to 48 and do Perilous Moons",
      "objectives": [
        { "key": "skill:slayer>=48", "role": "completes", "source": { "method": "llm", "confidence": "high" } },
        { "key": "quest:perilous_moons@finished", "role": "completes", "source": { "method": "llm", "confidence": "high" } }
      ] },
    { "id": "f4", "text": "bank" }
  ] }] }]
}`
)

func floatingGuide(t *testing.T) *content.Guide {
	t.Helper()
	var g content.Guide
	if err := json.Unmarshal([]byte(floatGuide), &g); err != nil {
		t.Fatal(err)
	}
	g.Index()
	return &g
}

func TestCatalogRequires(t *testing.T) {
	g, reg := floatingGuide(t), registry(t)
	f1, _ := g.Step("f1")
	got := CatalogRequires(f1, reg)
	for _, want := range []string{slayer48, twilights} {
		if !contains(got, want) {
			t.Errorf("f1 catalog requires %v miss %s", got, want)
		}
	}
	f3, _ := g.Step("f3")
	if contains(CatalogRequires(f3, reg), slayer48) {
		t.Errorf("f3 completes %s itself, it is not a requirement", slayer48)
	}
	f2, _ := g.Step("f2")
	if got := CatalogRequires(f2, reg); len(got) != 0 {
		t.Errorf("f2 completes no quest, got %v", got)
	}
	if got := CatalogRequires(f1, nil); got != nil {
		t.Errorf("no catalog, got %v", got)
	}
}

func TestBlocked(t *testing.T) {
	g, reg := floatingGuide(t), registry(t)
	f1, _ := g.Step("f1")
	f2, _ := g.Step("f2")
	cases := []struct {
		name     string
		st       *content.Step
		verdicts Verdicts
		want     []string
	}{
		{"unknown never blocks", f1, Verdicts{}, nil},
		{"catalog requirement unmet", f1, Verdicts{slayer48: rules.Unmet}, []string{slayer48}},
		{"explicit and catalog, sorted", f1, Verdicts{slayer48: rules.Unmet, twilights: rules.Unmet}, []string{twilights, slayer48}},
		{"met does not block", f1, Verdicts{slayer48: rules.Met}, nil},
		{"explicit requires unmet", f2, Verdicts{agility62: rules.Unmet}, []string{agility62}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Blocked(tc.st, tc.verdicts, reg); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("blocked = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlaceCursorPassesBlockedSteps(t *testing.T) {
	g, reg := floatingGuide(t), registry(t)
	unmet := Verdicts{slayer48: rules.Unmet, agility62: rules.Unmet}
	cases := []struct {
		name     string
		status   map[string]Status
		verdicts Verdicts
		cur      CursorRecord
		want     CursorRecord
	}{
		{"unknown holds the cursor", map[string]Status{}, Verdicts{}, CursorRecord{}, CursorRecord{"f1", CursorAuto}},
		{"blocked steps are passed", map[string]Status{}, unmet, CursorRecord{}, CursorRecord{"f3", CursorAuto}},
		{"met pulls the cursor back", map[string]Status{"f3": ManualDone}, Verdicts{slayer48: rules.Met, agility62: rules.Unmet}, CursorRecord{"f4", CursorAuto}, CursorRecord{"f1", CursorAuto}},
		{"pinned_open holds a blocked step", map[string]Status{"f1": PinnedOpen}, unmet, CursorRecord{}, CursorRecord{"f1", CursorAuto}},
		{"pin holds a blocked step", map[string]Status{}, unmet, CursorRecord{"f2", CursorPin}, CursorRecord{"f2", CursorPin}},
		{"only blocked steps left", map[string]Status{"f3": AutoDone, "f4": ManualDone}, unmet, CursorRecord{}, CursorRecord{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlaceCursor(g, tc.status, tc.verdicts, reg, Account{}, tc.cur); got != tc.want {
				t.Fatalf("cursor = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReevaluateBlocksOnCatalogRequirements(t *testing.T) {
	g, reg := floatingGuide(t), registry(t)
	in := Input{
		Guides:       []*content.Guide{g},
		Registry:     reg,
		State:        rules.PlayerState{Skills: skills(map[string]int{"slayer": 40, "agility": 62})},
		Observations: obs(t0, "snap-1", Fact{"skill", "slayer"}, Fact{"skill", "agility"}),
		Account:      Account{ActiveGuideID: "f"},
		Now:          t1,
	}
	plan := Reevaluate(in)
	got := objectivesAfter(nil, plan)
	if got[slayer48].Verdict != rules.Unmet || got[agility62].Verdict != rules.Met {
		t.Fatalf("objectives = %+v", got)
	}
	// f1 is blocked by Slayer 48 (catalog), f2's requirement is met.
	if cur := cursorsAfter(nil, plan)["f"]; cur != (CursorRecord{"f2", CursorAuto}) {
		t.Fatalf("cursor = %+v", cur)
	}
}

func TestBuildBoardBlocked(t *testing.T) {
	g, reg := floatingGuide(t), registry(t)
	in := GuideInput{
		Guide: g, Verdicts: Verdicts{slayer48: rules.Unmet, agility62: rules.Unmet}, Registry: reg,
		Marks: map[string]Mark{"f2": {Kind: MarkSkipped}}, LastStatus: map[string]Status{},
	}
	b := BuildBoard(in, nil)
	if b.Counts.Blocked != 1 || b.Counts.Remaining != 3 {
		t.Fatalf("counts = %+v", b.Counts)
	}
	if !reflect.DeepEqual(b.Steps[0].BlockedBy, []string{slayer48}) || b.Steps[1].BlockedBy != nil {
		t.Fatalf("blocked_by = %v / %v", b.Steps[0].BlockedBy, b.Steps[1].BlockedBy)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
