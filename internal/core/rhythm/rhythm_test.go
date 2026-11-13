package rhythm

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

var (
	now   = time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	since = time.Date(2026, 11, 13, 3, 0, 0, 0, time.UTC) // 00:00 in São Paulo
)

func at(minAgo int) time.Time { return now.Add(-time.Duration(minAgo) * time.Minute) }

func i64(v int64) *int64 { return &v }

func TestTraining(t *testing.T) {
	fishing76 := entities.XPForLevel(76) + 1000
	cases := []struct {
		name   string
		recent []Point
		want   *Training
	}{
		{"nothing gained", []Point{{at(50), "fishing", fishing76}, {at(5), "fishing", fishing76}}, nil},
		{
			// 20 000 XP over the 50 minutes from the first gain to the last.
			"fishing, idle before it began", []Point{
				{at(58), "fishing", fishing76 - 100}, {at(52), "fishing", fishing76 - 100},
				{at(51), "fishing", fishing76}, {at(26), "fishing", fishing76 + 10000}, {at(1), "fishing", fishing76 + 20000},
			},
			&Training{Skill: "fishing", Level: 76, XP: fishing76 + 20000, XPPerHour: 24000, LastGainAt: at(1)},
		},
		{
			"the skill that gained the most in the last 30 minutes", []Point{
				{at(40), "cooking", 10000}, {at(35), "cooking", 50000}, {at(20), "cooking", 50000},
				{at(25), "fishing", fishing76}, {at(2), "fishing", fishing76 + 3000},
			},
			// One gain: the rate runs from the point before it, 3 000 XP in 23 minutes.
			&Training{Skill: "fishing", Level: 76, XP: fishing76 + 3000, XPPerHour: 7826, LastGainAt: at(2)},
		},
		{"gained, but not for 12 minutes", []Point{{at(40), "fishing", fishing76}, {at(12), "fishing", fishing76 + 9000}, {at(3), "fishing", fishing76 + 9000}}, nil},
		{
			// The hiscores lag the plugin: their lower XP 5 minutes ago is stale,
			// not a loss, and the rate runs on the plugin's points.
			"a stale lower point", []Point{
				{at(31), "fishing", fishing76}, {at(16), "fishing", fishing76 + 6000},
				{at(5), "fishing", fishing76 + 1000}, {at(1), "fishing", fishing76 + 12000},
			},
			&Training{Skill: "fishing", Level: 76, XP: fishing76 + 12000, XPPerHour: 24000, LastGainAt: at(1)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(Input{Now: now, Since: since, Recent: tc.recent}).Training
			if tc.want == nil || got == nil {
				if tc.want != got {
					t.Fatalf("training = %+v, want %+v", got, tc.want)
				}
				return
			}
			want := *tc.want
			want.NextLevelInS = got.NextLevelInS
			if !reflect.DeepEqual(*got, want) {
				t.Fatalf("training = %+v, want %+v", *got, want)
			}
		})
	}
	// The next level's time comes from the rate.
	got := Compute(Input{Now: now, Since: since, Recent: []Point{{at(31), "fishing", fishing76}, {at(1), "fishing", fishing76 + 12000}}}).Training
	need := entities.XPForLevel(77) - (fishing76 + 12000)
	if got == nil || got.XPPerHour != 24000 || got.NextLevelInS == nil || *got.NextLevelInS != int(float64(need)/24000*3600) {
		t.Fatalf("rate and next level = %+v", got)
	}
	// At 99 there is no next level.
	top := entities.XPForLevel(99) + 5000
	if got = Compute(Input{Now: now, Recent: []Point{{at(31), "fishing", top}, {at(1), "fishing", top + 9000}}}).Training; got == nil || got.Level != 99 || got.NextLevelInS != nil {
		t.Fatalf("at 99 = %+v", got)
	}
}

func TestTodayAndSession(t *testing.T) {
	start := at(45)
	in := Input{
		Now: now, Since: since, SessionStart: &start,
		Current:   map[string]int{"fishing": entities.XPForLevel(77) + 10, "cooking": 5000, "magic": 900},
		AtSince:   map[string]int{"fishing": entities.XPForLevel(76) - 10, "cooking": 5000, "magic": 1000},
		AtSession: map[string]int{"fishing": entities.XPForLevel(77) - 990, "cooking": 4000},
	}
	r := Compute(in)
	fishing := entities.XPForLevel(77) + 10 - (entities.XPForLevel(76) - 10)
	want := Today{Since: since, XP: fishing, Levels: 2, Skills: []SkillDay{{Skill: "fishing", XP: fishing, Levels: 2, Level: 77}}}
	if !reflect.DeepEqual(r.Today, want) {
		t.Fatalf("today = %+v, want %+v", r.Today, want)
	}
	if r.Session == nil || r.Session.XP != 1000+1000 || !r.Session.StartedAt.Equal(start) {
		t.Fatalf("session = %+v", r.Session)
	}
	if r = Compute(Input{Now: now, Since: since}); r.Session != nil || r.Today.XP != 0 || r.Today.Skills == nil || r.GP != nil {
		t.Fatalf("empty = %+v", r)
	}
}

func TestGP(t *testing.T) {
	live := func(q map[int]int64) rules.ContainerItems {
		return rules.ContainerItems{State: ingest.ItemLive, Complete: true, Quantities: q}
	}
	h := rules.ItemHoldings{
		rules.ContainerBank:      live(map[int]int64{CoinsID: 1_000_000, PlatinumID: 50, 4151: 1}),
		rules.ContainerInventory: live(map[int]int64{CoinsID: 2500}),
	}
	if gp := GPOf(h); gp == nil || *gp != 1_052_500 {
		t.Fatalf("GPOf = %v", gp)
	}
	if gp := GPOf(rules.ItemHoldings{rules.ContainerInventory: live(map[int]int64{CoinsID: 9})}); gp != nil {
		t.Fatalf("GP without the bank = %v", *gp)
	}

	cases := []struct {
		name  string
		coins []CoinObs
		want  *int64
	}{
		{"no observation", nil, nil},
		{
			"full before Since, deltas after", []CoinObs{
				{At: since.Add(-time.Hour), Full: true, Container: "bank", Coins: i64(100_000)},
				{At: since.Add(-time.Hour), Full: true, Container: "inventory"}, // no coins: 0
				{At: since.Add(time.Hour), Container: "inventory", Coins: i64(3000)},
				{At: since.Add(2 * time.Hour), Container: "bank", Platinum: i64(10)},
			},
			i64(3000 + 10_000),
		},
		{
			// A later full image replaces the container: the platinum it no longer lists is gone.
			"full after Since replaces", []CoinObs{
				{At: since.Add(-time.Hour), Full: true, Container: "bank", Coins: i64(100_000), Platinum: i64(5)},
				{At: since.Add(time.Hour), Full: true, Container: "bank", Coins: i64(90_000)},
			},
			i64(-10_000 - 5000),
		},
		{
			// First seen today: what it held then counts as what it held at Since.
			"first seen after Since", []CoinObs{
				{At: since.Add(time.Hour), Container: "bank", Coins: i64(7)}, // a delta before any full: skipped
				{At: since.Add(2 * time.Hour), Full: true, Container: "bank", Coins: i64(500)},
				{At: since.Add(3 * time.Hour), Container: "bank", Coins: i64(800)},
			},
			i64(300),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute(Input{Now: now, Since: since, Coins: tc.coins, GPNow: i64(1)})
			if !reflect.DeepEqual(r.GP.Today, tc.want) {
				t.Fatalf("gp today = %v, want %v", deref(r.GP.Today), deref(tc.want))
			}
		})
	}
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestSessionItems(t *testing.T) {
	const (
		sturgeon, notedSturgeon = 383, 384
		feather, lobsterPot     = 314, 301
		harpoon                 = 311
	)
	start := at(60)
	full := func(ago int, c string, items map[int]int64) ItemObs {
		return ItemObs{At: at(ago), Full: true, Container: c, Items: items}
	}
	delta := func(ago int, c string, items map[int]int64) ItemObs {
		return ItemObs{At: at(ago), Container: c, Items: items}
	}
	login := []ItemObs{
		full(60, rules.ContainerBank, map[int]int64{sturgeon: 100, feather: 50}),
		full(60, rules.ContainerInventory, map[int]int64{feather: 1000, harpoon: 1}),
		full(60, rules.ContainerEquipment, map[int]int64{}),
	}
	base := func(id int) int {
		if id == notedSturgeon {
			return sturgeon
		}
		return id
	}
	cases := []struct {
		name    string
		items   []ItemObs
		gained  []ItemCount
		running []ItemRunOut
	}{
		{
			// Fishing an hour: 26 sturgeon in the inventory, 300 banked (one
			// trip banked as notes), 400 feathers used, the harpoon wielded.
			"gains add up over the containers, moves are not gains",
			append(slices.Clone(login),
				delta(40, rules.ContainerInventory, map[int]int64{sturgeon: 26, feather: 600}),
				delta(30, rules.ContainerBank, map[int]int64{sturgeon: 350, notedSturgeon: 50}),
				delta(30, rules.ContainerInventory, map[int]int64{sturgeon: 0}),
				delta(5, rules.ContainerInventory, map[int]int64{sturgeon: 26, harpoon: 0}),
				delta(5, rules.ContainerEquipment, map[int]int64{harpoon: 1}),
			),
			[]ItemCount{{Item: sturgeon, Qty: 326}},
			// 400 of 1 050 feathers used in an hour: the 650 left last 97.5 min.
			[]ItemRunOut{{Item: feather, Used: 400, Left: 650, InS: 5850}},
		},
		{
			"a full image replaces the container",
			append(slices.Clone(login),
				full(10, rules.ContainerInventory, map[int]int64{lobsterPot: 1}),
			),
			[]ItemCount{{Item: lobsterPot, Qty: 1}},
			// 1 000 of the 1 050 feathers left in an hour; the 50 in the bank last 3 min.
			[]ItemRunOut{{Item: feather, Used: 1000, Left: 50, InS: 180}},
		},
		{
			"a delta before the full image is ignored",
			append([]ItemObs{delta(61, rules.ContainerInventory, map[int]int64{sturgeon: 99})}, login...),
			[]ItemCount{}, []ItemRunOut{},
		},
		{
			"the equipment counts only when known at the start",
			[]ItemObs{
				login[0], login[1],
				delta(5, rules.ContainerInventory, map[int]int64{harpoon: 0}),
				full(5, rules.ContainerEquipment, map[int]int64{harpoon: 1}),
			},
			[]ItemCount{}, []ItemRunOut{},
		},
		{
			"without the bank at the start, nothing",
			[]ItemObs{login[1], full(30, rules.ContainerBank, map[int]int64{sturgeon: 400})},
			nil, nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(Input{Now: now, Since: since, SessionStart: &start, Items: tc.items, Base: base})
			if got.Session == nil {
				t.Fatal("no session")
			}
			if !reflect.DeepEqual(got.Session.Gained, tc.gained) {
				t.Errorf("gained = %#v, want %#v", got.Session.Gained, tc.gained)
			}
			if !reflect.DeepEqual(got.Session.RunningOut, tc.running) {
				t.Errorf("running out = %#v, want %#v", got.Session.RunningOut, tc.running)
			}
		})
	}
}

func TestSessionItemsBounds(t *testing.T) {
	start := at(5)
	var bank, inv = map[int]int64{}, map[int]int64{}
	for id := 1; id <= 20; id++ {
		inv[id] = int64(id)
	}
	in := Input{Now: now, Since: since, SessionStart: &start, Items: []ItemObs{
		{At: start, Full: true, Container: rules.ContainerBank, Items: bank},
		{At: start, Full: true, Container: rules.ContainerInventory, Items: inv},
		{At: at(1), Full: true, Container: rules.ContainerInventory, Items: map[int]int64{100: 5}},
		{At: at(1), Full: true, Container: rules.ContainerBank, Items: func() map[int]int64 {
			out := map[int]int64{}
			for id := 200; id < 215; id++ {
				out[id] = int64(id)
			}
			return out
		}()},
	}}
	got := Compute(in).Session
	if len(got.Gained) != MaxGained || got.Gained[0].Item != 214 {
		t.Errorf("gained = %v, want the %d largest, 214 first", got.Gained, MaxGained)
	}
	// Five minutes is too short to tell a pace.
	if len(got.RunningOut) != 0 {
		t.Errorf("running out = %v, want none before Idle", got.RunningOut)
	}
}
