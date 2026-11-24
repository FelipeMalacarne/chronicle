package execspec

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
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

func fishing() *Train {
	return &Train{Skill: "skill:fishing", SkillName: "Fishing", Level: 76, Plugin: "a.b.TickFisherPlugin",
		Config: []clientcfg.Setting{{Group: "tickfisher", Key: "mode", Value: "BARBARIAN"}}}
}

func cooks() *Quest {
	return &Quest{Quest: "quest:cooks_assistant", Name: "Cook's Assistant", Helper: "Cook's Assistant"}
}

func moltenGlass() *GEOffer {
	return &GEOffer{Item: 1775, ItemName: "Molten glass", Qty: 1400, Price: 120}
}

func sellGlass() *GESell {
	return &GESell{GEOffer: *moltenGlass(), CoinsAtLeast: 1000 + 1400*118}
}

func TestCheck(t *testing.T) {
	for _, ok := range []Task{
		{Type: TypeTravel, Travel: castle()},
		{Type: TypeTravel, Travel: mageBank()},
		{Type: TypeBank, Bank: bank()},
		{Type: TypeBuy, Buy: buy()},
		{Type: TypeTrain, Train: fishing()},
		{Type: TypeTrain, Train: &Train{Skill: "skill:attack", SkillName: "Attack", Level: 40, Plugin: "a.b.Fighter", Combat: true}},
		{Type: TypeQuest, Quest: cooks()},
		{Type: TypeGEBuy, GEBuy: moltenGlass()},
		{Type: TypeGESell, GESell: sellGlass()},
	} {
		if err := ok.Check(); err != nil {
			t.Errorf("%s refused: %v", ok.Type, err)
		}
	}
	tr := func(f func(*Travel)) Task { v := castle(); f(v); return Task{Type: TypeTravel, Travel: v} }
	bk := func(f func(*Bank)) Task { v := bank(); f(v); return Task{Type: TypeBank, Bank: v} }
	by := func(f func(*Buy)) Task { v := buy(); f(v); return Task{Type: TypeBuy, Buy: v} }
	tn := func(f func(*Train)) Task { v := fishing(); f(v); return Task{Type: TypeTrain, Train: v} }
	qs := func(f func(*Quest)) Task { v := cooks(); f(v); return Task{Type: TypeQuest, Quest: v} }
	for _, tc := range []struct {
		name string
		task Task
		want string
	}{
		{"talk", Task{Type: "talk"}, "not executed yet"},
		{"train without train", Task{Type: "train"}, "only train"},
		{"skill ref", tn(func(v *Train) { v.Skill = "fishing" }), "skill ref"},
		{"level", tn(func(v *Train) { v.Level = 100 }), "level 1 to 99"},
		{"bare class", tn(func(v *Train) { v.Plugin = "TickFisherPlugin" }), "fully qualified"},
		{"chronicle link", tn(func(v *Train) { v.Plugin = "net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin" }), "not controlled"},
		{"secret setting", tn(func(v *Train) { v.Config[0].Key = "password" }), "secret"},
		{"protected group", tn(func(v *Train) { v.Config[0].Group = "microbot" }), "not edited"},
		{"fighting without combat", tn(func(v *Train) { v.Skill, v.SkillName = "skill:strength", "Strength" }), "combat must be set"},
		{"train and travel", Task{Type: TypeTrain, Train: fishing(), Travel: castle()}, "only train"},
		{"quest without quest", Task{Type: "quest"}, "only quest"},
		{"quest and train", Task{Type: TypeQuest, Quest: cooks(), Train: fishing()}, "only quest"},
		{"quest ref", qs(func(v *Quest) { v.Quest = "skill:cooking" }), "quest ref"},
		{"quest slug", qs(func(v *Quest) { v.Quest = "quest:Cooks Assistant" }), "quest ref"},
		{"quest name", qs(func(v *Quest) { v.Name = "" }), "name is required"},
		{"quest helper", qs(func(v *Quest) { v.Helper = "Cook's\nAssistant" }), "one line"},
		{"manual", Task{Type: "manual"}, "not executed yet"},
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
		{"two points", tr(func(v *Travel) { v.Area.Polygon = v.Area.Polygon[:2] }), "3 to 256"},
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
	if r := (Task{Type: TypeTrain, Train: fishing()}).Risks(); len(r) != 0 {
		t.Errorf("fishing risks = %v", r)
	}
	if r := (Task{Type: TypeTrain, Train: &Train{Combat: true}}).Risks(); len(r) != 1 || r[0] != "combat" {
		t.Errorf("fighting risks = %v", r)
	}
	if u := fishing().Until(); u != "skill:fishing>=76" {
		t.Errorf("until = %s", u)
	}
	if r := (Task{Type: TypeQuest, Quest: cooks()}).Risks(); strings.Join(r, ",") != "combat,hazard" {
		t.Errorf("quest risks = %v", r)
	}
	safe := cooks()
	safe.Safe = true
	if r := (Task{Type: TypeQuest, Quest: safe}).Risks(); len(r) != 0 {
		t.Errorf("safe quest risks = %v", r)
	}
	if u := cooks().Until(); u != "quest:cooks_assistant@finished" {
		t.Errorf("quest until = %s", u)
	}
	if !NeedsSupervision("hcim", []string{"wilderness"}) || NeedsSupervision("ironman", []string{"combat"}) ||
		NeedsSupervision("hcim", []string{"item_loss"}) || !NeedsSupervision("hcim", []string{"hazard"}) {
		t.Error("E9: HCIM and a death risk, only")
	}
	for _, mode := range []string{"regular", "uim", "gim"} {
		if NeedsSupervision(mode, []string{"combat", "wilderness", "hazard"}) {
			t.Errorf("E9 is HCIM only, %s supervised", mode)
		}
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
// the agent posts to the bridge, the wilderness probes, the types the
// plugin must refuse too, and the tasks the agent delegates elsewhere
// (valid here, refused by the plugin). Chronicle Link's TaskSpecTest reads
// the file.
func TestPluginWire(t *testing.T) {
	type start struct {
		ID   string `json:"id"`
		Task Task   `json:"task"`
	}
	wire := struct {
		Starts     []start `json:"starts"`
		Wilderness any     `json:"wilderness"`
		Refused    []Task  `json:"refused"`
		Delegated  []Task  `json:"delegated"`
	}{
		Starts: []start{
			{ID: "5f2a9c0d11e84b7a", Task: Task{Type: TypeTravel, Travel: castle()}},
			{ID: "5f2a9c0d11e84b7b", Task: Task{Type: TypeTravel, Travel: mageBank()}},
			{ID: "5f2a9c0d11e84b7c", Task: Task{Type: TypeBank, Bank: bank()}},
			{ID: "5f2a9c0d11e84b7d", Task: Task{Type: TypeBuy, Buy: buy()}},
		},
		Wilderness: probes,
		Refused: []Task{{Type: "talk"}, {Type: "train"}, {Type: TypeTravel}, {Type: TypeBank, Bank: &Bank{}},
			{Type: TypeGEBuy}, {Type: TypeGESell}},
		Delegated: []Task{{Type: TypeTrain, Train: fishing()}, {Type: TypeQuest, Quest: cooks()},
			{Type: TypeGEBuy, GEBuy: moltenGlass()}, {Type: TypeGESell, GESell: sellGlass()}},
	}
	for _, d := range wire.Delegated {
		if err := d.Check(); err != nil {
			t.Fatalf("delegated %s: %v", d.Type, err)
		}
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

// TestGE: an offer is bounded (an id, a quantity, a price whose total fits
// one coin stack, the coins a sale's proof waits for) and the Grand Exchange
// runs for a regular account only.
func TestGE(t *testing.T) {
	offer := func(f func(*GEOffer)) Task { o := moltenGlass(); f(o); return Task{Type: TypeGEBuy, GEBuy: o} }
	sell := func(f func(*GESell)) Task { s := sellGlass(); f(s); return Task{Type: TypeGESell, GESell: s} }
	for _, tc := range []struct {
		name string
		task Task
		want string
	}{
		{"no item name", offer(func(o *GEOffer) { o.ItemName = "" }), "itemName is required"},
		{"bad id", offer(func(o *GEOffer) { o.Item = 0 }), "bad item id"},
		{"qty 0", offer(func(o *GEOffer) { o.Qty = 0 }), "qty 1 to"},
		{"price 0", offer(func(o *GEOffer) { o.Price = 0 }), "price of at least 1"},
		{"over one coin stack", offer(func(o *GEOffer) { o.Qty, o.Price = 2, MaxCoins/2+1 }), "at most 2147483647"},
		{"sale without its proof", sell(func(s *GESell) { s.CoinsAtLeast = 0 }), "coinsAtLeast 1 to"},
		{"buy with a sell", Task{Type: TypeGEBuy, GEBuy: moltenGlass(), GESell: sellGlass()}, "only geBuy"},
		{"sell as a buy", Task{Type: TypeGESell, GEBuy: moltenGlass()}, "only geSell"},
	} {
		if err := tc.task.Check(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
	for _, task := range []Task{{Type: TypeGEBuy, GEBuy: moltenGlass()}, {Type: TypeGESell, GESell: sellGlass()}} {
		if err := task.CheckMode(content.ModeRegular); err != nil {
			t.Errorf("%s regular: %v", task.Type, err)
		}
		for _, m := range []string{content.ModeIronman, content.ModeHCIM, content.ModeUIM, content.ModeGIM} {
			if err := task.CheckMode(m); err == nil || !strings.Contains(err.Error(), "regular accounts only") {
				t.Errorf("%s %s: %v", task.Type, m, err)
			}
		}
	}
	if err := (Task{Type: TypeBuy, Buy: buy()}).CheckMode(content.ModeIronman); err != nil {
		t.Errorf("a shop buy is any mode's: %v", err)
	}
}
