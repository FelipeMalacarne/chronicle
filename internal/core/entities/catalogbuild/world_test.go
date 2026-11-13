package catalogbuild

import (
	"reflect"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func TestMapPoint(t *testing.T) {
	cases := []struct {
		tpl         string
		x, y, plane int
		area        entities.Area
		reason      string
	}{
		{tpl: "{{Map|name=Al Kharid|x=3290|y=3185|height=300|width=300|zoom=2}}", x: 3290, y: 3185,
			area: entities.PointArea(3290, 3185, 0)},
		{tpl: "{{Map|name=Observatory|x=2441|y=3162|r=8|mtype=square}}", x: 2441, y: 3162,
			area: entities.Area{MinX: 2433, MinY: 3154, MaxX: 2449, MaxY: 3170, From: entities.AreaSquare}},
		{tpl: "{{Map|name=Varrock Museum|3260,3449|height=250|zoom=2|mtype=rectangle|rectX=15|rectY=14}}", x: 3260, y: 3449,
			area: entities.Area{MinX: 3253, MinY: 3442, MaxX: 3267, MaxY: 3456, From: entities.AreaRectangle}},
		// The point is the center of every point (polygon vertices and x/y),
		// rounded half to even; the area is the polygon.
		{tpl: "{{Map|name=Box|x=10|y=10|mtype=polygon|100,100|103,100|103,104|100,104}}", x: 83, y: 84,
			area: entities.Area{MinX: 100, MinY: 100, MaxX: 103, MaxY: 104, From: entities.AreaPolygon,
				Polygon: [][2]int{{100, 100}, {103, 100}, {103, 104}, {100, 104}}}},
		{tpl: "{{Map|mtype=focusarea|3137,9663|3199,9663|3199,9601|3137,9601|mapID=-1|zoom=2}}", x: 3168, y: 9632,
			area: entities.Area{MinX: 3137, MinY: 9601, MaxX: 3199, MaxY: 9663, From: entities.AreaPolygon,
				Polygon: [][2]int{{3137, 9663}, {3199, 9663}, {3199, 9601}, {3137, 9601}}}},
		// A polygon of two points, a square without r: the default box.
		{tpl: "{{Map|1000,2000|1002,2000|mtype=polygon|plane=1}}", x: 1001, y: 2000, plane: 1,
			area: entities.PointArea(1001, 2000, 1)},
		{tpl: "{{Map|x=1000|y=2000|mtype=square}}", x: 1000, y: 2000, area: entities.PointArea(1000, 2000, 0)},
		{tpl: "{{Map|name=Zanaris|zoom=1|x=2430|y=4428|mapID=28}}", reason: "map id 28"},
		{tpl: "{{Map|name=Goblin Village|mtype=polygon|2941:3525,2975:3525}}", reason: "map without a point"},
		{tpl: "{{Map|x=abc|y=1}}", reason: "map x/y not numbers"},
	}
	for _, c := range cases {
		x, y, plane, area, reason := mapPoint(c.tpl)
		if reason != c.reason {
			t.Errorf("%s: reason %q, want %q", c.tpl, reason, c.reason)
			continue
		}
		if reason != "" {
			continue
		}
		if x != c.x || y != c.y || plane != c.plane {
			t.Errorf("%s: point (%d, %d, %d), want (%d, %d, %d)", c.tpl, x, y, plane, c.x, c.y, c.plane)
		}
		c.area.Plane = c.plane
		if !reflect.DeepEqual(*area, c.area) {
			t.Errorf("%s: area %+v, want %+v", c.tpl, *area, c.area)
		}
	}
}

func TestJunkPage(t *testing.T) {
	cases := map[string]bool{
		"Aggie":                                false,
		"Cook (Lumbridge)":                     false,
		"Genie (random event)":                 false,
		"Archeio (Library Historical Archive)": false,
		"Aggie (2025 Christmas event)":         true,
		"Ak-Haranu (2014 April Fools)":         true,
		"Banker (Deadman Mode)":                true,
		"Rocks (iron, historical)":             true,
		"Guard (Raging Echoes League)":         true,
		"Hans (RuneScape 2 Beta)":              true,
		"Boat (Sailing beta)":                  true,
		"Ghost (?)":                            false,
	}
	for title, want := range cases {
		if got := junkPage(title); got != want {
			t.Errorf("junkPage(%q) = %v, want %v", title, got, want)
		}
	}
}

func TestPageIDs(t *testing.T) {
	ids, err := pageIDs("npc", []string{"Crab", "'Crab'", "Cook (Lumbridge)"}, []idOverride{{Page: "'Crab'", ID: "crab_16485"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Crab": "crab", "'Crab'": "crab_16485", "Cook (Lumbridge)": "cook_lumbridge"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v", ids)
	}
	for _, c := range []struct {
		pages []string
		over  []idOverride
		err   string
	}{
		{[]string{"Crab", "'Crab'"}, nil, `share id "crab"`},
		{[]string{"? ? ?"}, nil, "has no slug"},
		{[]string{"Crab"}, []idOverride{{Page: "Gone", ID: "gone"}}, "unknown page"},
		{[]string{"Crab"}, []idOverride{{Page: "Crab", ID: "Not A Slug"}}, "invalid npc id"},
	} {
		if _, err := pageIDs("npc", c.pages, c.over); err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("pageIDs(%v, %v) = %v, want %q", c.pages, c.over, err, c.err)
		}
	}
}

func TestPickNameAndClean(t *testing.T) {
	name, aliases := pickName("Skeleton (mage)", []string{"Skeleton", "", "Skeleton"})
	if name != "Skeleton" || aliases != nil {
		t.Errorf("pickName = %q %v", name, aliases)
	}
	name, aliases = pickName("Beach Cocktails", []string{"Party Cocktails", "Beach Cocktails"})
	if name != "Beach Cocktails" || !reflect.DeepEqual(aliases, []string{"Party Cocktails"}) {
		t.Errorf("pickName = %q %v", name, aliases)
	}
	for in, want := range map[string]string{
		`null <sup><span style="cursor:help">[?]</span></sup>`: "",
		`Racoon<sup class="noprint">x</sup>`:                   "Racoon",
		"  Hans ":                                              "Hans",
	} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
	got := links("[[Lumbridge Castle]] kitchen, [[File:x.png]] [[lumbridge_Castle|the castle]] [[Varrock#Bank]]", nil)
	if !reflect.DeepEqual(got, []entities.WikiKey{"Lumbridge Castle", "Varrock"}) {
		t.Errorf("links = %v", got)
	}
}

func TestBuildShops(t *testing.T) {
	npcs := &entities.NPCCatalog{NPCs: []entities.NPCEntry{{ID: "bob", Name: "Bob", WikiKey: "Bob", GameIDs: []int{1}}}}
	var raw rawShops
	raw.Shops = append(raw.Shops,
		struct {
			Page     string `json:"page"`
			Version  string `json:"version"`
			Name     string `json:"name"`
			Owner    string `json:"owner"`
			Location string `json:"location"`
		}{Page: "Bob's Axes", Name: "Bob's Axes", Owner: "[[Bob]] and [[Not An NPC]]", Location: "[[Lumbridge]]"})
	type line = struct {
		SoldBy   string `json:"sold_by"`
		Item     string `json:"item"`
		Price    string `json:"price"`
		Stock    string `json:"stock"`
		Currency string `json:"currency"`
	}
	raw.Stock = []line{
		{SoldBy: "Bob's Axes", Item: "Bronze axe", Price: "16", Stock: "10", Currency: "Coins"},
		{SoldBy: "Bob's Axes", Item: "Bronze axe", Price: "16", Stock: "10", Currency: "Coins"}, // repeated
		{SoldBy: "Bob's Axes", Item: "Rope", Price: "N/A", Stock: "∞", Currency: ""},
		{SoldBy: "Bob's Axes", Item: "Obsidian cape", Price: "90,000", Stock: "1", Currency: "Tokkul"},
		{SoldBy: "Bob's Axes", Item: "Axe", Price: "1", Stock: "1", Currency: "Coins"},
		{SoldBy: "Bob's Axes", Item: "Mystery", Price: "1", Stock: "1", Currency: "Coins"},
		{SoldBy: "Bob's Axes", Item: "Rope", Price: "5", Stock: "Unknown", Currency: "Points"},
		{SoldBy: "Some NPC", Item: "Rope", Price: "1", Stock: "1", Currency: "Coins"},
	}
	ids := map[string]int{"Bronze axe": 1351, "Rope": 954, "Obsidian cape": 6568, "Tokkul": 6529}
	resolve := func(name string) (int, string) {
		if name == "Axe" {
			return 0, entities.StockItemAmbiguous
		}
		if id, ok := ids[name]; ok {
			return id, ""
		}
		return 0, entities.StockItemNotInCatalog
	}
	sc, elsewhere, err := buildShops(raw, npcs, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if elsewhere != 1 || len(sc.Shops) != 1 {
		t.Fatalf("elsewhere %d, shops %+v", elsewhere, sc.Shops)
	}
	s := sc.Shops[0]
	if s.ID != "bobs_axes" || !reflect.DeepEqual(s.Owners, []string{"bob"}) || !reflect.DeepEqual(s.LocationKeys, []entities.WikiKey{"Lumbridge"}) {
		t.Errorf("shop = %+v", s)
	}
	ten, sixteen, price := 10, 16, 90000
	want := []entities.ShopStock{
		{Item: 954, Infinite: true},
		{Item: 1351, Stock: &ten, Price: &sixteen},
		{Item: 6568, Stock: ptr(1), Price: &price, Currency: 6529},
	}
	if !reflect.DeepEqual(s.Stock, want) {
		t.Errorf("stock = %+v", s.Stock)
	}
	wantU := []entities.ShopUnresolved{
		{Name: "Axe", Reason: entities.StockItemAmbiguous},
		{Name: "Mystery", Reason: entities.StockItemNotInCatalog},
		{Name: "Rope (Points)", Reason: entities.StockCurrencyNotAnItem},
	}
	if !reflect.DeepEqual(s.Unresolved, wantU) {
		t.Errorf("unresolved = %+v", s.Unresolved)
	}
}

func ptr(n int) *int { return &n }
