package entities

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWorldRefs(t *testing.T) {
	for _, s := range []string{"npc:duke_horacio", "shop:bobs_brilliant_axes", "object:bank_booth"} {
		if _, err := ParseRef(s); err != nil {
			t.Errorf("ParseRef(%q): %v", s, err)
		}
	}
	for _, s := range []string{"npc:Duke Horacio", "shop:", "npc:cook-lumbridge", "object:Bank_booth"} {
		if r, err := ParseRef(s); err == nil {
			t.Errorf("ParseRef(%q) = %v, accepted", s, r)
		}
	}
}

func TestResolveWorld(t *testing.T) {
	r := registry(t)
	cases := []struct {
		kind    Kind
		name    string
		status  Status
		want    string // ref when resolved
		hasCand string // a candidate when ambiguous
	}{
		{KindNPC, "Duke Horacio", Resolved, "npc:duke_horacio", ""},
		{KindNPC, "duke  horacio", Resolved, "npc:duke_horacio", ""},
		{KindNPC, "King Narnode Shareen", Resolved, "npc:king_narnode_shareen", ""},
		// Several pages carry the name; the page titled with it wins.
		{KindNPC, "Banker", Resolved, "npc:banker", ""},
		{KindNPC, "Bob", Resolved, "npc:bob", ""},
		// No page is titled "Cook": ambiguous, never guessed.
		{KindNPC, "Cook", Ambiguous, "", "npc:cook_lumbridge"},
		{KindNPC, "Shop keeper", Ambiguous, "", "npc:shop_keeper_lumbridge"},
		// The page title names one page.
		{KindNPC, "Cook (Lumbridge)", Resolved, "npc:cook_lumbridge", ""},
		{KindNPC, "Nobody McNoone", Unresolved, "", ""},
		{KindShop, "Bob's Brilliant Axes.", Resolved, "shop:bobs_brilliant_axes", ""},
		{KindShop, "Lumbridge General Store", Resolved, "shop:lumbridge_general_store", ""},
		{KindShop, "Crossbow Shop", Ambiguous, "", "shop:crossbow_shop_keldagrim"},
		{KindObject, "Bank booth", Resolved, "object:bank_booth", ""},
		{KindObject, "Furnace", Resolved, "object:furnace", ""},
		{KindObject, "Cooking range", Ambiguous, "", "object:cooking_range_lumbridge_castle"},
		{KindObject, "Cooking range (Lumbridge Castle)", Resolved, "object:cooking_range_lumbridge_castle", ""},
	}
	for _, c := range cases {
		var res Resolution
		switch c.kind {
		case KindNPC:
			res = r.ResolveNPC(c.name)
		case KindShop:
			res = r.ResolveShop(c.name)
		case KindObject:
			res = r.ResolveObject(c.name)
		}
		if res.Status != c.status {
			t.Errorf("%s %q: status %s (%+v), want %s", c.kind, c.name, res.Status, res, c.status)
			continue
		}
		if c.want != "" && res.Ref.String() != c.want {
			t.Errorf("%s %q = %s, want %s", c.kind, c.name, res.Ref, c.want)
		}
		if c.hasCand != "" && !slices.ContainsFunc(res.Candidates, func(x Ref) bool { return x.String() == c.hasCand }) {
			t.Errorf("%s %q: candidates %v lack %s", c.kind, c.name, res.Candidates, c.hasCand)
		}
	}
}

func TestWorldCatalogs(t *testing.T) {
	r := registry(t)
	n := r.WorldCounts()
	if n[KindNPC] < 4000 || n[KindShop] < 400 || n[KindObject] < 4000 {
		t.Fatalf("world counts = %v", n)
	}
	for _, s := range []string{"npc:duke_horacio", "shop:bobs_brilliant_axes", "object:bank_booth"} {
		ref := MustParseRef(s)
		if !r.HasCatalog(ref.Kind) || !r.Known(ref) {
			t.Errorf("%s is not known", s)
		}
		if _, ok := r.Display(ref); !ok {
			t.Errorf("%s has no display name", s)
		}
		if u, ok := r.WikiURL(ref); !ok || !strings.HasPrefix(u, WikiBaseURL) {
			t.Errorf("%s wiki url = %q", s, u)
		}
	}
	if r.Known(MustParseRef("npc:not_an_npc_at_all")) {
		t.Error("unknown npc is known")
	}
	duke, _ := r.NPC("duke_horacio")
	if !slices.Contains(duke.GameIDs, 815) {
		t.Errorf("duke_horacio game ids = %v", duke.GameIDs)
	}
	// Bob's axes: owner Bob, in Lumbridge, sells a bronze pickaxe (1265).
	shop, _ := r.Shop("bobs_brilliant_axes")
	if !slices.Contains(shop.Owners, "bob") {
		t.Errorf("owners = %v", shop.Owners)
	}
	st, ok := r.ShopStockOf("bobs_brilliant_axes", 1265)
	if !ok || st.Stock == nil || *st.Stock < 1 || st.Price == nil || st.CurrencyID() != CoinsID {
		t.Errorf("bronze pickaxe stock = %+v, %v", st, ok)
	}
	if _, ok := r.ShopStockOf("bobs_brilliant_axes", 1511); ok {
		t.Error("logs are not sold at Bob's")
	}
	if locs := r.LocationsOf(MustParseRef("shop:bobs_brilliant_axes")); !slices.Contains(locs, MustParseRef("location:lumbridge")) {
		t.Errorf("shop locations = %v", locs)
	}
	if locs := r.LocationsOf(MustParseRef("npc:hans")); !slices.Contains(locs, MustParseRef("location:lumbridge")) {
		t.Errorf("hans locations = %v", locs)
	}
	// The world kinds are kept out of the wiki-key index the importers
	// resolve guide links with: adding them changes no guide mention.
	if refs := r.ByWikiKey("Duke Horacio"); len(refs) != 0 {
		t.Errorf("ByWikiKey(Duke Horacio) = %v", refs)
	}
}

func TestWorldCatalogsOptionalAndChecked(t *testing.T) {
	fsys := catalogFS(t)
	r, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if n := r.WorldCounts(); n[KindNPC] != 0 || r.Known(MustParseRef("npc:duke_horacio")) {
		t.Fatalf("without world files: %v", n)
	}
	fsys[NPCsFile] = &fstest.MapFile{Data: []byte(`{"catalog":"npcs","version":"t","source":"t","npcs":[
		{"id":"a","name":"A","wiki_key":"A","game_ids":[1]},
		{"id":"a","name":"A2","wiki_key":"A2","game_ids":[2]},
		{"id":"b","name":"B","wiki_key":"A","game_ids":[3]},
		{"id":"c","name":"C","wiki_key":"C","game_ids":[]}]}`)}
	fsys[ShopsFile] = &fstest.MapFile{Data: []byte(`{"catalog":"shops","version":"t","source":"t","shops":[
		{"id":"s","name":"S","wiki_key":"S","owners":["nobody"],"stock":[{"item":999999},{"item":1265,"stock":-1}]}]}`)}
	_, err = Load(fsys)
	for _, want := range []string{`duplicate npc id "a"`, `share wiki page "A"`, `npc "c" has no game ids`, `unknown owner npc "nobody"`, "unknown item 999999", "stock is a count or infinite"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load error %v lacks %q", err, want)
		}
	}
}

func TestAreaContains(t *testing.T) {
	square := Area{Plane: 0, MinX: 10, MinY: 10, MaxX: 20, MaxY: 20, From: AreaSquare}
	// An L-shaped polygon: the box corner (19, 19) is outside it.
	ell := Area{Plane: 1, MinX: 0, MinY: 0, MaxX: 10, MaxY: 10, From: AreaPolygon,
		Polygon: [][2]int{{0, 0}, {10, 0}, {10, 5}, {5, 5}, {5, 10}, {0, 10}}}
	cases := []struct {
		area    Area
		x, y, p int
		want    bool
	}{
		{square, 15, 15, 0, true},
		{square, 10, 20, 0, true}, // edges are inside
		{square, 21, 15, 0, false},
		{square, 15, 15, 1, false}, // another plane
		{ell, 2, 2, 1, true},
		{ell, 8, 2, 1, true},
		{ell, 2, 8, 1, true},
		{ell, 8, 8, 1, false}, // in the box, not in the L
		{ell, 5, 7, 1, true},  // on an edge
		{ell, 7, 5, 1, true},  // on an edge
		{ell, 0, 0, 1, true},  // a vertex
		{ell, 2, 2, 0, false},
		{PointArea(100, 100, 0), 110, 90, 0, true},
		{PointArea(100, 100, 0), 111, 100, 0, false},
	}
	for _, c := range cases {
		if got := c.area.Contains(c.x, c.y, c.p); got != c.want {
			t.Errorf("%s area Contains(%d, %d, %d) = %v, want %v", c.area.From, c.x, c.y, c.p, got, c.want)
		}
	}
	bad := []Area{
		{From: "blob"},
		{MinX: 5, MaxX: 4, From: AreaPoint},
		{MaxX: 9, MaxY: 9, From: AreaPolygon, Polygon: [][2]int{{0, 0}, {1, 1}}},
		{MaxX: 9, MaxY: 9, From: AreaPolygon, Polygon: [][2]int{{0, 0}, {1, 1}, {20, 0}}},
		{MaxX: 9, MaxY: 9, From: AreaSquare, Polygon: [][2]int{{0, 0}, {1, 1}, {2, 0}}},
	}
	for _, a := range bad {
		if a.check() == "" {
			t.Errorf("area %+v passed the check", a)
		}
	}
}

func TestLocationAreas(t *testing.T) {
	r := registry(t)
	cases := []struct {
		id   string
		from string
		in   [3]int // a tile inside
		out  [3]int // a tile outside
	}{
		// Polygon from the wiki map: the castle, not the town around it.
		{"lumbridge_castle", AreaPolygon, [3]int{3210, 3220, 0}, [3]int{3222, 3240, 0}},
		{"varrock_museum", AreaRectangle, [3]int{3260, 3449, 0}, [3]int{3270, 3449, 0}},
		{"rogues_den", AreaSquare, [3]int{2906, 3537, 0}, [3]int{2910, 3537, 0}},
		// Only a point: the default box around it.
		{"al_kharid", AreaPoint, [3]int{3290 + AreaRadius, 3185, 0}, [3]int{3290 + AreaRadius + 1, 3185, 0}},
	}
	for _, c := range cases {
		a, ok := r.LocationArea(c.id)
		if !ok {
			t.Errorf("%s has no area", c.id)
			continue
		}
		if a.From != c.from {
			t.Errorf("%s area from %s, want %s", c.id, a.From, c.from)
		}
		if !a.Contains(c.in[0], c.in[1], c.in[2]) || a.Contains(c.out[0], c.out[1], c.out[2]) {
			t.Errorf("%s area %+v: in %v / out %v", c.id, a, c.in, c.out)
		}
		m, _ := r.LocationMap(c.id)
		if a.From != AreaPolygon && !a.Contains(m.X, m.Y, m.Plane) {
			t.Errorf("%s: area %+v leaves out its own point", c.id, a)
		}
	}
	if _, ok := r.LocationArea("wilderness"); ok {
		t.Error("an unmapped location has no area")
	}
}
