package planning

import (
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Catalog ids: Plank 960 (noted 961), Hammer 2347, Bronze axe 1351, Coins 995
// (stackable), Logs 1511 (noted 1512).
const (
	plank      = 960
	notedPlank = 961
	hammer     = 2347
	bronzeAxe  = 1351
	coins      = 995
	logs       = 1511
	notedLogs  = 1512
	notCatalog = 999_999
	ironAxe    = 1349
	prayerPot4 = 2434
	prayerPot3 = 139
)

var at = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func ip(i int) *int { return &i }

func item(name string, id *int, qty *int) content.ItemRequirement {
	return content.ItemRequirement{Name: name, ItemID: id, Qty: qty, SourceText: name}
}

func live(complete bool, q map[int]int64) rules.ContainerItems {
	return rules.ContainerItems{State: ingest.ItemLive, ObservedAt: at, Complete: complete, Quantities: q}
}

func holdings(inv, equip map[int]int64) rules.ItemHoldings {
	return rules.ItemHoldings{
		rules.ContainerBank:      live(true, map[int]int64{}),
		rules.ContainerInventory: live(true, inv),
		rules.ContainerEquipment: live(true, equip),
	}
}

func guideWith(id string, lo *content.Loadout) (*content.Guide, *content.Segment) {
	seg := &content.Segment{
		ID: "seg-a", Kind: content.SegmentBankLeg, Loadout: lo,
		BankActions: []content.BankAction{{Type: content.BankWithdraw, Position: content.PositionStart, Text: "withdraw a hammer"}},
		Steps:       []*content.Step{{ID: "1.1", Text: "bank"}},
	}
	g := &content.Guide{ID: id, Sections: []*content.Section{{ID: "s1", Slug: "s1", Segments: []*content.Segment{seg}}}}
	g.Index()
	return g, seg
}

func TestEvaluateSegment(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	known := func(inv []content.ItemRequirement, equip []content.ItemRequirement, empty *int) *content.Loadout {
		return &content.Loadout{Status: content.LoadoutKnown, Inventory: inv, Equipment: equip, EmptySlots: empty}
	}
	partial := func(inv ...content.ItemRequirement) *content.Loadout {
		return &content.Loadout{Status: content.LoadoutPartial, Inventory: inv}
	}
	cases := []struct {
		name  string
		lo    *content.Loadout
		h     rules.ItemHoldings
		want  Status
		check func(t *testing.T, s Segment)
	}{
		{"no loadout", nil, holdings(nil, nil), NoLoadout, nil},
		{"unknown loadout", &content.Loadout{Status: content.LoadoutUnknown}, holdings(nil, nil), NoLoadout, nil},
		{"known loadout carried", known([]content.ItemRequirement{item("Hammer", ip(hammer), nil)}, nil, nil),
			holdings(map[int]int64{hammer: 1}, nil), Ready, nil},
		{"partial loadout carried is only listed_ready", partial(item("Hammer", ip(hammer), nil)),
			holdings(map[int]int64{hammer: 1}, nil), ListedReady, nil},
		{"missing item with live full inventory", partial(item("Hammer", ip(hammer), nil)),
			holdings(map[int]int64{}, nil), NotReady, nil},
		{"item in bank is owned but not carried", partial(item("Hammer", ip(hammer), nil)),
			rules.ItemHoldings{
				rules.ContainerBank:      live(true, map[int]int64{hammer: 1}),
				rules.ContainerInventory: live(true, map[int]int64{}),
				rules.ContainerEquipment: live(true, map[int]int64{}),
			}, NotReady, func(t *testing.T, s Segment) {
				it := s.Items[0]
				if it.Verdict != rules.Unmet || it.Owned == nil || it.Owned.Verdict != rules.Met {
					t.Fatalf("want carried=unmet owned=met, got %+v owned=%+v", it, it.Owned)
				}
			}},
		{"missing scopes: inventory never observed", partial(item("Hammer", ip(hammer), nil)),
			rules.ItemHoldings{}, Unknown, func(t *testing.T, s Segment) {
				if s.Freshness[rules.ContainerInventory].Observed {
					t.Fatal("inventory reported as observed")
				}
			}},
		{"restored inventory cannot prove a live loadout", partial(item("Hammer", ip(hammer), nil)),
			rules.ItemHoldings{
				rules.ContainerInventory: {State: ingest.ItemRestored, ObservedAt: at, Complete: true, Quantities: map[int]int64{hammer: 1}},
				rules.ContainerEquipment: live(true, nil),
			}, Unknown, nil},
		{"restored bank: ownership stale, readiness from live inventory", partial(item("Hammer", ip(hammer), nil)),
			rules.ItemHoldings{
				rules.ContainerBank:      {State: ingest.ItemRestored, ObservedAt: at, Complete: true, Quantities: map[int]int64{hammer: 1}},
				rules.ContainerInventory: live(true, map[int]int64{}),
				rules.ContainerEquipment: live(true, map[int]int64{}),
			}, NotReady, func(t *testing.T, s Segment) {
				if o := s.Items[0].Owned; o == nil || o.Verdict != rules.Met || !o.Stale {
					t.Fatalf("want stale ownership, got %+v", o)
				}
				if s.Freshness[rules.ContainerBank].Live {
					t.Fatal("restored bank reported live")
				}
			}},
		{"delta without baseline proves presence", partial(item("Hammer", ip(hammer), nil)),
			rules.ItemHoldings{rules.ContainerInventory: live(false, map[int]int64{hammer: 1})}, ListedReady, nil},
		{"delta without baseline cannot prove absence", partial(item("Hammer", ip(hammer), nil)),
			rules.ItemHoldings{rules.ContainerInventory: live(false, map[int]int64{}), rules.ContainerEquipment: live(true, nil)}, Unknown, nil},
		{"noted requirement not satisfied by unnoted planks", partial(item("noted planks", ip(notedPlank), ip(120))),
			holdings(map[int]int64{plank: 120}, nil), NotReady, func(t *testing.T, s Segment) {
				if s.Items[0].Owned.Verdict != rules.Met {
					t.Fatal("unnoted planks should still count as owning planks")
				}
			}},
		{"noted requirement satisfied", partial(item("noted planks", ip(notedPlank), ip(120))),
			holdings(map[int]int64{notedPlank: 120}, nil), ListedReady, nil},
		{"quantity short", partial(item("noted planks", ip(notedPlank), ip(120))),
			holdings(map[int]int64{notedPlank: 119}, nil), NotReady, nil},
		{"unresolved name is unknown", partial(item("1k arrow shafts", nil, nil)),
			holdings(map[int]int64{}, nil), Unknown, nil},
		{"optional item does not block", &content.Loadout{Status: content.LoadoutKnown, Inventory: []content.ItemRequirement{
			item("Hammer", ip(hammer), nil), {Name: "Logs", ItemID: ip(logs), Optional: true},
		}}, holdings(map[int]int64{hammer: 1}, nil), Ready, nil},
		{"inventory tool may be wielded", partial(item("Bronze axe", ip(bronzeAxe), nil)),
			holdings(map[int]int64{}, map[int]int64{bronzeAxe: 1}), ListedReady, nil},
		{"equipment item in inventory is not worn", known(nil, []content.ItemRequirement{item("Bronze axe", ip(bronzeAxe), nil)}, nil),
			holdings(map[int]int64{bronzeAxe: 1}, map[int]int64{}), NotReady, nil},
		{"empty slots met with stackables", known([]content.ItemRequirement{item("Coins", ip(coins), nil)}, nil, ip(27)),
			holdings(map[int]int64{coins: 50_000}, nil), Ready, nil},
		{"empty slots unmet", known(nil, nil, ip(20)),
			holdings(map[int]int64{hammer: 1, logs: 1, bronzeAxe: 1, notedLogs: 1, coins: 1, plank: 4, notCatalog: 1}, nil), NotReady,
			func(t *testing.T, s Segment) {
				// 10 slots used for sure (plank is unstackable in the catalog).
				if *s.EmptySlots.FreeMin != 18 || *s.EmptySlots.FreeMax != 18 {
					t.Fatalf("free = [%d,%d]", *s.EmptySlots.FreeMin, *s.EmptySlots.FreeMax)
				}
			}},
		{"empty slots unknown when stackability is unknown", known(nil, nil, ip(20)),
			holdings(map[int]int64{notCatalog: 10}, nil), Unknown, func(t *testing.T, s Segment) {
				if *s.EmptySlots.FreeMin != 18 || *s.EmptySlots.FreeMax != 27 {
					t.Fatalf("free = [%d,%d]", *s.EmptySlots.FreeMin, *s.EmptySlots.FreeMax)
				}
			}},
		{"empty slots need a live baseline", known(nil, nil, ip(1)),
			rules.ItemHoldings{rules.ContainerInventory: live(false, nil)}, Unknown, nil},
		{"partial loadout without items", &content.Loadout{Status: content.LoadoutPartial}, holdings(nil, nil), NoLoadout, nil},
		{"any_of met by a later id", partial(content.ItemRequirement{Name: "Axe", AnyOf: []int{ironAxe, bronzeAxe}}),
			holdings(map[int]int64{bronzeAxe: 1}, nil), ListedReady, func(t *testing.T, s Segment) {
				if it := s.Items[0]; it.Variant != content.RequireAnyOf || it.MatchedID == nil || *it.MatchedID != bronzeAxe {
					t.Fatalf("item = %+v", it)
				}
			}},
		{"any_of missing everywhere", partial(content.ItemRequirement{Name: "Axe", AnyOf: []int{ironAxe, bronzeAxe}}),
			holdings(map[int]int64{}, nil), NotReady, nil},
		{"any_of quantities are not summed across ids", partial(content.ItemRequirement{Name: "Axe", AnyOf: []int{ironAxe, bronzeAxe}, Qty: ip(2)}),
			holdings(map[int]int64{ironAxe: 1, bronzeAxe: 1}, nil), NotReady, nil},
		{"any_of without a live inventory", partial(content.ItemRequirement{Name: "Axe", AnyOf: []int{ironAxe, bronzeAxe}}),
			rules.ItemHoldings{}, Unknown, nil},
		{"family adds up doses", partial(content.ItemRequirement{Name: "prayer potions", Family: "prayer_potion", Qty: ip(2)}),
			holdings(map[int]int64{prayerPot4: 1, prayerPot3: 1}, nil), ListedReady, func(t *testing.T, s Segment) {
				if it := s.Items[0]; it.Carried != 2 || it.Owned == nil || it.Owned.Verdict != rules.Met {
					t.Fatalf("item = %+v owned %+v", it, it.Owned)
				}
			}},
		{"family short", partial(content.ItemRequirement{Name: "prayer potions", Family: "prayer_potion", Qty: ip(3)}),
			holdings(map[int]int64{prayerPot4: 1, hammer: 2}, nil), NotReady, nil},
		{"unknown family is unknown", partial(content.ItemRequirement{Name: "x", Family: "not_a_family"}),
			holdings(map[int]int64{}, nil), Unknown, nil},
		{"noted requirement needs the noted stack", partial(content.ItemRequirement{Name: "planks", ItemID: ip(plank), Qty: ip(120), Noted: true}),
			holdings(map[int]int64{notedPlank: 120}, nil), ListedReady, nil},
		{"unnoted stack does not satisfy noted", partial(content.ItemRequirement{Name: "planks", ItemID: ip(plank), Qty: ip(2), Noted: true}),
			holdings(map[int]int64{plank: 2}, nil), NotReady, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, seg := guideWith("g", tc.lo)
			got := EvaluateSegment(g, seg, tc.h, catalog)
			if got.Status != tc.want {
				t.Fatalf("status = %s, want %s (%+v)", got.Status, tc.want, got)
			}
			if len(got.BankActions) != 1 || got.BankActions[0].Type != content.BankWithdraw {
				t.Fatalf("bank actions must be passed through untouched: %+v", got.BankActions)
			}
			if got.GuideID != "g" || got.SegmentID != "seg-a" || got.FirstStepID != "1.1" {
				t.Fatalf("identity = %s/%s/%s", got.GuideID, got.SegmentID, got.FirstStepID)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestEvaluateSegmentGuideIsolation(t *testing.T) {
	// Two routes with the same local segment and step ids but different
	// loadouts: each guide is evaluated against its own setup only.
	oz, ozSeg := guideWith("oziris-main", &content.Loadout{Status: content.LoadoutPartial, Inventory: []content.ItemRequirement{item("Hammer", ip(hammer), nil)}})
	bruh, bruhSeg := guideWith("bruhsailer-main", &content.Loadout{Status: content.LoadoutPartial, Inventory: []content.ItemRequirement{item("Logs", ip(logs), nil)}})
	h := holdings(map[int]int64{hammer: 1}, nil)

	if got, ok := oz.Segment("seg-a"); !ok || got != ozSeg {
		t.Fatal("oziris segment lookup")
	}
	if got, ok := bruh.Segment("seg-a"); !ok || got != bruhSeg {
		t.Fatal("bruhsailer segment lookup")
	}
	a := EvaluateSegment(oz, ozSeg, h, nil)
	b := EvaluateSegment(bruh, bruhSeg, h, nil)
	if a.Status != ListedReady || a.GuideID != "oziris-main" {
		t.Fatalf("oziris = %s/%s", a.GuideID, a.Status)
	}
	if b.Status != NotReady || b.GuideID != "bruhsailer-main" {
		t.Fatalf("bruhsailer = %s/%s", b.GuideID, b.Status)
	}
}

func TestLegs(t *testing.T) {
	seq := func(id string, steps ...string) *content.Segment {
		s := &content.Segment{ID: id, Kind: content.SegmentSequence}
		for _, st := range steps {
			s.Steps = append(s.Steps, &content.Step{ID: st})
		}
		return s
	}
	leg := func(id string, steps ...string) *content.Segment {
		s := seq(id, steps...)
		s.Kind = content.SegmentBankLeg
		return s
	}
	g := &content.Guide{ID: "g", Variants: []*content.Variant{
		{ID: "main", Sections: []*content.Section{
			{ID: "a", Slug: "a", Segments: []*content.Segment{seq("s1", "1", "2"), seq("s2", "3"), leg("s3", "4")}},
			{ID: "t", Slug: "t", Kind: content.KindTrack, Segments: []*content.Segment{leg("t1", "x")}},
			{ID: "b", Slug: "b", Segments: []*content.Segment{leg("s4", "5", "6")}},
		}},
		{ID: "alt", Sections: []*content.Section{{ID: "c", Slug: "c", Segments: []*content.Segment{leg("s5", "7")}}}},
	}}
	g.Index()
	cases := []struct {
		step, current, next string
	}{
		{"", "s1", "s3"},
		{"2", "s1", "s3"},
		{"4", "s3", "s4"},
		{"5", "s4", ""},
		{"x", "", ""}, // tracks have no "next leg" on the main route
		{"missing", "", ""},
		{"7", "s5", ""}, // never crosses into another variant
	}
	for _, tc := range cases {
		cur, next := Legs(g, tc.step)
		if id(cur) != tc.current || id(next) != tc.next {
			t.Errorf("Legs(%q) = %q, %q; want %q, %q", tc.step, id(cur), id(next), tc.current, tc.next)
		}
	}
}

func id(s *content.Segment) string {
	if s == nil {
		return ""
	}
	return s.ID
}
