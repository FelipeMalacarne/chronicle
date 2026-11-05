package rules

import (
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

// Catalog ids used below: Plank 960 (noted 961), Prayer potion(4) 2434,
// Prayer potion(3) 139.
const (
	plank       = 960
	notedPlank  = 961
	prayerPot4  = 2434
	prayerPot3  = 139
	unknownItem = 999_999
)

var observedAt = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func live(complete bool, q map[int]int64) ContainerItems {
	return ContainerItems{State: ingest.ItemLive, ObservedAt: observedAt, Complete: complete, Quantities: q}
}

func restored(q map[int]int64) ContainerItems {
	return ContainerItems{State: ingest.ItemRestored, ObservedAt: observedAt, Complete: true, Quantities: q}
}

func unavailable() ContainerItems {
	return ContainerItems{State: ingest.ItemUnavailable, ObservedAt: observedAt}
}

func owned(id, qty int, containers ...string) content.Rule {
	return content.ItemOwnedRule{RuleHeader: hdr(content.TypeItemOwned), ItemID: intp(id), Qty: qty, Containers: containers}
}

func TestItemOwned(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	allLive := ItemHoldings{
		ContainerBank:      live(true, map[int]int64{notedPlank: 100}),
		ContainerInventory: live(true, map[int]int64{plank: 20, prayerPot3: 2}),
		ContainerEquipment: live(true, map[int]int64{}),
	}
	cases := []struct {
		name    string
		rule    content.Rule
		items   ItemHoldings
		catalog *entities.Registry
		want    Verdict
		stale   bool
	}{
		{"never observed", owned(plank, 1), nil, catalog, Unknown, false},
		{"missing scopes: no container observed", owned(plank, 1), ItemHoldings{}, catalog, Unknown, false},
		{"noted and unnoted forms add up", owned(plank, 120), allLive, catalog, Met, false},
		{"quantity above holdings is unmet with live full baselines", owned(plank, 121), allLive, catalog, Unmet, false},
		{"without a catalog noted planks are a different id", owned(plank, 120), allLive, nil, Unmet, false},
		{"noted requirement satisfied by unnoted stack", owned(notedPlank, 120), allLive, catalog, Met, false},
		{"other dose is not the same item", owned(prayerPot4, 1), allLive, catalog, Unmet, false},
		{"unknown id never matches by equivalence", owned(unknownItem, 1), allLive, catalog, Unmet, false},
		{"restricted to inventory", owned(plank, 21, ContainerInventory), allLive, catalog, Unmet, false},
		{"restricted to inventory met", owned(plank, 20, ContainerInventory), allLive, catalog, Met, false},
		{"bank scope missing cannot prove absence", owned(prayerPot4, 1), ItemHoldings{
			ContainerInventory: live(true, nil), ContainerEquipment: live(true, nil),
		}, catalog, Unknown, false},
		{"restored bank proves ownership but is stale", owned(plank, 50), ItemHoldings{
			ContainerBank: restored(map[int]int64{plank: 50}), ContainerInventory: live(true, nil), ContainerEquipment: live(true, nil),
		}, catalog, Met, true},
		{"live part suffices so not stale", owned(plank, 5), ItemHoldings{
			ContainerBank: restored(map[int]int64{plank: 50}), ContainerInventory: live(true, map[int]int64{plank: 5}), ContainerEquipment: live(true, nil),
		}, catalog, Met, false},
		{"restored bank cannot prove absence", owned(prayerPot4, 1), ItemHoldings{
			ContainerBank: restored(map[int]int64{}), ContainerInventory: live(true, nil), ContainerEquipment: live(true, nil),
		}, catalog, Unknown, false},
		{"unavailable bank cannot prove absence", owned(prayerPot4, 1), ItemHoldings{
			ContainerBank: unavailable(), ContainerInventory: live(true, nil), ContainerEquipment: live(true, nil),
		}, catalog, Unknown, false},
		{"unavailable contents are ignored", owned(plank, 1, ContainerBank), ItemHoldings{
			ContainerBank: {State: ingest.ItemUnavailable, Quantities: map[int]int64{plank: 9}},
		}, catalog, Unknown, false},
		{"delta without baseline proves presence", owned(plank, 3, ContainerBank), ItemHoldings{
			ContainerBank: live(false, map[int]int64{plank: 3}),
		}, catalog, Met, false},
		{"delta without baseline cannot prove absence", owned(plank, 4, ContainerBank), ItemHoldings{
			ContainerBank: live(false, map[int]int64{plank: 3}),
		}, catalog, Unknown, false},
		{"poh is never observed", owned(plank, 1, ContainerPOH), allLive, catalog, Unknown, false},
		{"qty defaults to one", owned(plank, 0), allLive, catalog, Met, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Evaluate([]content.Rule{tc.rule}, PlayerState{Items: tc.items, Catalog: tc.catalog})
			if res.Verdict != tc.want {
				t.Fatalf("verdict = %s, want %s (%+v)", res.Verdict, tc.want, res.Evidence)
			}
			if got := res.Evidence[0].Stale; got != tc.stale {
				t.Fatalf("stale = %v, want %v", got, tc.stale)
			}
			if res.Monotonic {
				t.Fatal("item_owned must not be monotonic")
			}
			if tc.want == Unknown && res.Evidence[0].Reason == "" {
				t.Fatal("unknown verdict without a reason")
			}
		})
	}
}

func TestOwnsReportsEveryRequestedContainer(t *testing.T) {
	o := Owns(ItemHoldings{ContainerInventory: live(true, map[int]int64{plank: 2})}, nil, plank, 1, nil)
	if o.Verdict != Met || o.Held != 2 {
		t.Fatalf("got %+v", o)
	}
	for _, name := range DefaultItemContainers {
		if _, ok := o.Containers[name]; !ok {
			t.Fatalf("container %s missing from evidence", name)
		}
	}
	if o.Containers[ContainerBank].Observed || o.Containers[ContainerBank].ObservedAt != nil {
		t.Fatal("unobserved bank reported as observed")
	}
}

func TestOwnsFamilyAndAnyOf(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	notedPot3, ok := catalog.NotedID(prayerPot3)
	if !ok {
		t.Fatal("prayer potion(3) has no noted form in the catalog")
	}
	h := ItemHoldings{
		ContainerBank:      live(true, map[int]int64{notedPot3: 5}),
		ContainerInventory: live(true, map[int]int64{prayerPot4: 1}),
		ContainerEquipment: live(true, nil),
	}
	if o := OwnsFamily(h, catalog, "prayer_potion", 6, nil); o.Verdict != Met || o.Held != 6 {
		t.Errorf("family across doses and noted forms = %+v", o)
	}
	if o := OwnsFamily(h, catalog, "prayer_potion", 7, nil); o.Verdict != Unmet {
		t.Errorf("family short = %+v", o)
	}
	if o := OwnsFamily(h, catalog, "not_a_family", 1, nil); o.Verdict != Unknown {
		t.Errorf("unknown family = %+v", o)
	}
	if o, id := OwnsAnyOf(h, catalog, []int{plank, prayerPot4}, 1, nil); o.Verdict != Met || id != prayerPot4 {
		t.Errorf("any_of = %+v %d", o, id)
	}
	if o, id := OwnsAnyOf(h, catalog, []int{plank, prayerPot4}, 2, nil); o.Verdict != Unmet || id != plank {
		t.Errorf("any_of does not sum ids and keeps the first on ties: %+v %d", o, id)
	}
	restored := ItemHoldings{ContainerBank: {State: ingest.ItemRestored, Complete: true, Quantities: map[int]int64{plank: 1}}}
	if o, id := OwnsAnyOf(restored, catalog, []int{prayerPot4, plank}, 1, []string{ContainerBank}); o.Verdict != Met || !o.Stale || id != plank {
		t.Errorf("any_of through a restored mirror = %+v %d", o, id)
	}
}

func TestOwnsAlternatives(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	full := func(q map[int]int64) ItemHoldings {
		return ItemHoldings{ContainerBank: live(true, q), ContainerInventory: live(true, nil), ContainerEquipment: live(true, nil)}
	}
	alts := []entities.ItemQty{{ID: plank, N: 2}, {ID: prayerPot4, N: 1}}
	cases := []struct {
		name   string
		h      ItemHoldings
		want   Verdict
		stale  bool
		wantID int
	}{
		{"second alternative reaches its own quantity", full(map[int]int64{plank: 1, prayerPot4: 1}), Met, false, prayerPot4},
		{"first alternative reaches its own quantity", full(map[int]int64{plank: 2}), Met, false, plank},
		{"noted forms count for their alternative", full(map[int]int64{notedPlank: 2}), Met, false, plank},
		{"alternatives are never summed", full(map[int]int64{plank: 1}), Unmet, false, plank},
		{"other variants do not count", full(map[int]int64{prayerPot3: 5, plank: 1}), Unmet, false, plank},
		{"live beats a restored mirror", ItemHoldings{
			ContainerBank:      restored(map[int]int64{plank: 2}),
			ContainerInventory: live(true, map[int]int64{prayerPot4: 1}),
		}, Met, false, prayerPot4},
		{"met only through a restored mirror stays stale", ItemHoldings{ContainerBank: restored(map[int]int64{prayerPot4: 1})}, Met, true, prayerPot4},
		{"restored absence is unknown, never unmet", ItemHoldings{ContainerBank: restored(nil)}, Unknown, false, plank},
		{"nothing observed", ItemHoldings{}, Unknown, false, plank},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, id := OwnsAlternatives(tc.h, catalog, alts, nil)
			if o.Verdict != tc.want || o.Stale != tc.stale || id != tc.wantID {
				t.Errorf("got %s stale=%v id=%d (%+v), want %s stale=%v id=%d", o.Verdict, o.Stale, id, o, tc.want, tc.stale, tc.wantID)
			}
		})
	}
}
