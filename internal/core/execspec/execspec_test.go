package execspec

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updatePluginFixture rewrites the plugin's fixture of the task wire:
//
//	go test ./internal/core/execspec -run TestPluginWire -update
var updatePluginFixture = flag.Bool("update", false, "rewrite the Chronicle Link fixture of the executor's tasks")

// pluginFixture is what Chronicle Link's TaskSpecTest parses: the bodies
// the agent posts to /bridge/task/start and the wilderness bounds, the
// contract of the executor on both sides.
const pluginFixture = "../../../microbot/src/test/resources/net/runelite/client/plugins/microbot/fm/chroniclelink/task-wire.json"

// castle is Lumbridge Castle's area as the catalog has it (a polygon).
func castle() *Travel {
	return &Travel{
		Location: "location:lumbridge_castle", Name: "Lumbridge Castle", Target: Tile{X: 3217, Y: 3219},
		Area: Area{MinX: 3199, MinY: 3201, MaxX: 3231, MaxY: 3237, Polygon: [][2]int{
			{3204, 3237}, {3201, 3234}, {3201, 3220}, {3199, 3219}, {3201, 3217}, {3201, 3204}, {3204, 3201},
			{3226, 3201}, {3231, 3206}, {3231, 3232}, {3226, 3237},
		}},
	}
}

func mageBank() *Travel {
	return &Travel{
		Location: "location:mage_bank", Name: "Mage Bank", Target: Tile{X: 3090, Y: 3956},
		Area: Area{MinX: 3087, MinY: 3953, MaxX: 3093, MaxY: 3959}, Wilderness: true,
	}
}

func bank() *Bank {
	return &Bank{Items: []BankItem{
		{Name: "Hammer", IDs: []int{2347}, Qty: 1},
		{Name: "Coins", IDs: []int{995}, Qty: 2500},
		{Name: "Raw shrimps", IDs: []int{317}, Qty: 50, Noted: true},
		{Name: "Axe", IDs: []int{1359, 1357, 1355, 1353, 1349, 1351}, Qty: 1, Equip: true},
		{Name: "Prayer potion", IDs: []int{2434, 139, 141, 143}, Qty: 2, Optional: true},
	}}
}

func buy() *Buy {
	return &Buy{Shop: "shop:bobs_brilliant_axes", ShopName: "Bob's Brilliant Axes", NPCs: []int{2812, 10619}, Item: 1351, ItemName: "Bronze axe", Qty: 1}
}

func TestCheck(t *testing.T) {
	for _, ok := range []Task{
		{Type: TypeTravel, Travel: castle()},
		{Type: TypeTravel, Travel: mageBank()},
		{Type: TypeBank, Bank: bank()},
		{Type: TypeBuy, Buy: buy()},
	} {
		if err := ok.Check(); err != nil {
			t.Errorf("%s refused: %v", ok.Type, err)
		}
	}
	tr := func(f func(*Travel)) Task { v := castle(); f(v); return Task{Type: TypeTravel, Travel: v} }
	bk := func(f func(*Bank)) Task { v := bank(); f(v); return Task{Type: TypeBank, Bank: v} }
	by := func(f func(*Buy)) Task { v := buy(); f(v); return Task{Type: TypeBuy, Buy: v} }
	for _, tc := range []struct {
		name string
		task Task
		want string
	}{
		{"talk", Task{Type: "talk"}, "not executed yet"},
		{"train", Task{Type: "train"}, "not executed yet"},
		{"quest", Task{Type: "quest"}, "not executed yet"},
		{"unknown", Task{Type: "teleport"}, "unknown type"},
		{"two params", Task{Type: TypeTravel, Travel: castle(), Buy: buy()}, "only travel"},
		{"no params", Task{Type: TypeBank}, "only bank"},
		{"location ref", tr(func(v *Travel) { v.Location = "npc:hans" }), "location ref"},
		{"name", tr(func(v *Travel) { v.Name = "a\nb" }), "one line"},
		{"plane", tr(func(v *Travel) { v.Area.Plane = 4; v.Target.Plane = 4 }), "plane"},
		{"region", tr(func(v *Travel) { v.Area.MinX = 2900; v.Area.Polygon = nil }), "at most 256"},
		{"empty box", tr(func(v *Travel) { v.Area.MinX = v.Area.MaxX + 1 }), "empty"},
		{"outside", tr(func(v *Travel) { v.Target = Tile{X: 3199, Y: 3236} }), "not inside"},
		{"polygon point", tr(func(v *Travel) { v.Area.Polygon[0] = [2]int{3000, 3000} }), "outside the box"},
		{"two points", tr(func(v *Travel) { v.Area.Polygon = v.Area.Polygon[:2] }), "3 to 64"},
		{"hidden wilderness", Task{Type: TypeTravel, Travel: func() *Travel { v := mageBank(); v.Wilderness = false; return v }()}, "wilderness flag"},
		{"no items", bk(func(v *Bank) { v.Items = nil }), "1 to 39"},
		{"no ids", bk(func(v *Bank) { v.Items[0].IDs = nil }), "1 to 16 ids"},
		{"repeated id", bk(func(v *Bank) { v.Items[3].IDs = []int{1351, 1351} }), "repeated"},
		{"qty", bk(func(v *Bank) { v.Items[0].Qty = 0 }), "qty"},
		{"noted worn", bk(func(v *Bank) { v.Items[3].Noted = true }), "not noted"},
		{"all optional", bk(func(v *Bank) {
			for i := range v.Items {
				v.Items[i].Optional = true
			}
		}), "at least one"},
		{"shop ref", by(func(v *Buy) { v.Shop = "location:x" }), "shop ref"},
		{"no keepers", by(func(v *Buy) { v.NPCs = nil }), "shopkeeper"},
		{"item", by(func(v *Buy) { v.Item = 0 }), "item id"},
		{"too many", by(func(v *Buy) { v.Qty = MaxBuy + 1 }), "qty"},
	} {
		err := tc.task.Check()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestRisks(t *testing.T) {
	if r := (Task{Type: TypeTravel, Travel: mageBank()}).Risks(); len(r) != 1 || r[0] != "wilderness" {
		t.Errorf("mage bank risks = %v", r)
	}
	if r := (Task{Type: TypeTravel, Travel: castle()}).Risks(); len(r) != 0 {
		t.Errorf("castle risks = %v", r)
	}
	if !NeedsSupervision("hcim", []string{"wilderness"}) || NeedsSupervision("ironman", []string{"combat"}) ||
		NeedsSupervision("hcim", []string{"item_loss"}) || !NeedsSupervision("hcim", []string{"hazard"}) {
		t.Error("E9: HCIM and a death risk, only")
	}
	if !KnownRisk("combat") || KnownRisk("pking") {
		t.Error("closed risk list")
	}
}

// probes are tiles around the wilderness's edges, with whether they are in.
var probes = []struct {
	X  int  `json:"x"`
	Y  int  `json:"y"`
	In bool `json:"in"`
}{
	{3093, 3957, true}, {3080, 3492, false}, {3080, 3525, true}, {3080, 3524, false}, {2943, 3600, false},
	{3391, 3972, true}, {3392, 3700, false}, {3391, 3973, false}, {3045, 10340, true}, {3200, 9900, false},
	{3461, 10375, true}, {3462, 10000, false}, {2944, 9918, true}, {3188, 3220, false},
}

func TestWildernessProbes(t *testing.T) {
	for _, p := range probes {
		if got := InWilderness(p.X, p.Y); got != p.In {
			t.Errorf("InWilderness(%d, %d) = %v", p.X, p.Y, got)
		}
	}
}

// TestPluginWire pins the executor's JSON on both sides: the start bodies
// the agent posts to the bridge, the wilderness probes, and the types the
// plugin must refuse too. Chronicle Link's TaskSpecTest reads the file.
func TestPluginWire(t *testing.T) {
	type start struct {
		ID   string `json:"id"`
		Task Task   `json:"task"`
	}
	wire := struct {
		Starts     []start `json:"starts"`
		Wilderness any     `json:"wilderness"`
		Refused    []Task  `json:"refused"`
	}{
		Starts: []start{
			{ID: "5f2a9c0d11e84b7a", Task: Task{Type: TypeTravel, Travel: castle()}},
			{ID: "5f2a9c0d11e84b7b", Task: Task{Type: TypeTravel, Travel: mageBank()}},
			{ID: "5f2a9c0d11e84b7c", Task: Task{Type: TypeBank, Bank: bank()}},
			{ID: "5f2a9c0d11e84b7d", Task: Task{Type: TypeBuy, Buy: buy()}},
		},
		Wilderness: probes,
		Refused:    []Task{{Type: "talk"}, {Type: "train"}, {Type: TypeTravel}, {Type: TypeBank, Bank: &Bank{}}},
	}
	for _, s := range wire.Starts {
		if err := s.Task.Check(); err != nil {
			t.Fatalf("%s: %v", s.Task.Type, err)
		}
	}
	for _, r := range wire.Refused {
		if r.Check() == nil {
			t.Fatalf("%+v passes", r)
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(wire); err != nil {
		t.Fatal(err)
	}
	path := filepath.FromSlash(pluginFixture)
	if *updatePluginFixture {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (rewrite it with -update)", err)
	}
	if !bytes.Equal(want, buf.Bytes()) {
		t.Fatalf("the executor's JSON changed: rewrite %s with -update and check the plugin's tests\n%s", path, buf.String())
	}
}
