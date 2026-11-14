package planning

import (
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func TestOwnerMarksOverlayReadiness(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	spade := item("Spade", ip(hammer), nil) // any resolved id will do
	axe := content.ItemRequirement{Name: "Axe", AnyOf: []int{ironAxe, bronzeAxe}}
	lo := &content.Loadout{Status: content.LoadoutPartial, Inventory: []content.ItemRequirement{
		spade, axe, item("Mystery", nil, nil), item("Logs", ip(logs), ip(5)),
	}}
	g, seg := guideWith("g", lo)
	// A live, complete inventory without any of them: every item is unmet,
	// except the unresolved one, which stays unknown.
	h := holdings(map[int]int64{}, map[int]int64{})

	base := EvaluateSegment(g, seg, h, catalog, nil)
	if base.Status != NotReady {
		t.Fatalf("base status = %s", base.Status)
	}
	for _, it := range base.Items {
		if it.OwnerMarked || it.Key == "" {
			t.Fatalf("unmarked item %+v", it)
		}
	}
	if base.Items[2].Key != "name:mystery" || base.Items[0].Key != "id:2347" || base.Items[1].Key != "any:1349|1351" {
		t.Fatalf("keys = %q %q %q", base.Items[0].Key, base.Items[1].Key, base.Items[2].Key)
	}

	marks := ItemMarks{"id:2347": true, "any:1349|1351": true, "name:mystery": true, "id:1511": true}
	got := EvaluateSegment(g, seg, h, catalog, marks)
	if got.Status != ListedReady {
		t.Fatalf("marked status = %s (%+v)", got.Status, got.Items)
	}
	for _, it := range got.Items {
		if !it.OwnerMarked || it.Verdict != rules.Met {
			t.Fatalf("marked item = %+v", it)
		}
		// The facts are left as they were: nothing carried, ownership as proven.
		if it.Carried != 0 {
			t.Fatalf("a mark rewrote Carried: %+v", it)
		}
	}
	if got.Items[0].Owned == nil || got.Items[0].Owned.Verdict == rules.Met {
		t.Fatalf("a mark rewrote ownership: %+v", got.Items[0].Owned)
	}

	// A partial mark leaves the rest as the facts say; unknown stays unknown.
	only := EvaluateSegment(g, seg, h, catalog, ItemMarks{"id:2347": true})
	if only.Status != NotReady || !only.Items[0].OwnerMarked || only.Items[1].OwnerMarked || only.Items[1].Verdict != rules.Unmet {
		t.Fatalf("partial mark = %s %+v", only.Status, only.Items)
	}

	// A mark is harmless when the plugin proves the item is carried.
	carried := EvaluateSegment(g, seg, holdings(map[int]int64{hammer: 1}, nil), catalog, ItemMarks{"id:2347": true})
	if !carried.Items[0].OwnerMarked || carried.Items[0].Verdict != rules.Met || carried.Items[0].Carried != 1 {
		t.Fatalf("marked and carried = %+v", carried.Items[0])
	}
}

func TestOwnerMarksUnknownStaysUnknown(t *testing.T) {
	// Without a live inventory the verdict is unknown, never unmet; marking
	// resolves it to met, and an unmarked sibling stays unknown.
	g, seg := guideWith("g", &content.Loadout{Status: content.LoadoutKnown, Inventory: []content.ItemRequirement{
		item("Hammer", ip(hammer), nil), item("Logs", ip(logs), nil),
	}})
	h := rules.ItemHoldings{}
	got := EvaluateSegment(g, seg, h, nil, ItemMarks{"id:2347": true})
	if got.Status != Unknown || got.Items[0].Verdict != rules.Met || got.Items[1].Verdict != rules.Unknown {
		t.Fatalf("got %s %+v", got.Status, got.Items)
	}
}

func TestOwnerMarksScopeAndLookAhead(t *testing.T) {
	m := OwnerMarks{}
	m.Add("g1", "seg-a", "id:2347")
	if !m.For("g1", "seg-a")["id:2347"] || m.For("g1", "seg-b") != nil || m.For("g2", "seg-a") != nil {
		t.Fatalf("scope leaked: %+v", m)
	}
	h := holdings(map[int]int64{}, map[int]int64{})
	a := LookAhead(aheadLeg(), "1", h, nil, m.For("g1", "x"))
	if a == nil {
		t.Fatal("no ahead")
	}
	for _, g := range a.Groups {
		for _, it := range g.Items {
			if it.OwnerMarked {
				t.Fatalf("mark of another segment applied: %+v", it)
			}
		}
	}
	a = LookAhead(aheadLeg(), "1", h, nil, ItemMarks{"id:2347": true})
	marked := 0
	for _, g := range a.Groups {
		for _, it := range g.Items {
			if it.OwnerMarked {
				marked++
				if it.Verdict != rules.Met {
					t.Fatalf("marked ahead item = %+v", it)
				}
			}
		}
	}
	if marked != 1 {
		t.Fatalf("marked ahead items = %d", marked)
	}
	keys := SegmentRequirementKeys(aheadLeg())
	if !keys["id:2347"] || keys["id:1"] {
		t.Fatalf("keys = %v", keys)
	}
}
