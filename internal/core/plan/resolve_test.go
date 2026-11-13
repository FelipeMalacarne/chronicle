package plan

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func intp(n int) *int { return &n }

func TestResolve(t *testing.T) {
	c, reg := load(t)
	input := func(p Plan, mode string, h rules.ItemHoldings, pos *panel.Position) Input {
		ps := rules.PlayerState{Items: h, Catalog: reg}
		return Input{
			Plan: p, Content: c, Registry: reg, Holdings: h, Now: now,
			Boards: map[string]Board{"t": {Steps: steps(c, nil, nil, nil), Cursor: "a"}},
			Facts:  panel.Facts{Mode: mode, State: &ps, Position: pos},
		}
	}
	route := Plan{ID: RouteID("t"), Kind: KindRoute, Guide: "t"}
	custom := func(tasks ...content.Task) Plan {
		p := Plan{ID: "pln_test0001", Kind: KindCustom}
		for i := range tasks {
			p.Items = append(p.Items, Item{Kind: ItemTask, Task: &tasks[i]})
		}
		return p
	}

	t.Run("travel to a point area: the map point", func(t *testing.T) {
		spec, v, err := Resolve(input(route, content.ModeIronman, nil, nil), "step:t::a#0")
		if err != nil {
			t.Fatal(err)
		}
		tr := spec.Travel
		if spec.Type != execspec.TypeTravel || tr == nil || tr.Location != "location:lumbridge" || tr.Name != "Lumbridge" ||
			tr.Target != (execspec.Tile{X: 3188, Y: 3220}) || tr.Wilderness || v.Key != "step:t::a#0" {
			t.Fatalf("travel = %+v %+v", tr, v)
		}
	})
	t.Run("travel to a polygon", func(t *testing.T) {
		spec, _, err := Resolve(input(custom(content.Task{Type: content.TaskTravel, Location: ref("location:lumbridge_castle")}), content.ModeIronman, nil, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		a := spec.Travel.Area
		if len(a.Polygon) < 3 || !a.Contains(spec.Travel.Target.X, spec.Travel.Target.Y, spec.Travel.Target.Plane) {
			t.Fatalf("castle = %+v", spec.Travel)
		}
	})
	t.Run("travel into the wilderness carries the flag", func(t *testing.T) {
		spec, v, err := Resolve(input(custom(content.Task{Type: content.TaskTravel, Location: ref("location:mage_bank")}), content.ModeHCIM, nil, nil), "task:0#0")
		if err != nil || !spec.Travel.Wilderness || !v.Supervised || !slices.Equal(spec.Risks(), []string{"wilderness"}) {
			t.Fatalf("mage bank = %+v %+v %v", spec.Travel, v, err)
		}
	})
	t.Run("bank for the segment's loadout", func(t *testing.T) {
		spec, _, err := Resolve(input(route, content.ModeIronman, nil, nil), "step:t::b#0")
		if err != nil {
			t.Fatal(err)
		}
		if spec.Bank == nil || len(spec.Bank.Items) != 1 || spec.Bank.Items[0].Name != "Hammer" || !slices.Equal(spec.Bank.Items[0].IDs, []int{2347}) || spec.Bank.Items[0].Qty != 1 {
			t.Fatalf("bank = %+v", spec.Bank)
		}
	})
	t.Run("bank items: any_of, family and noted", func(t *testing.T) {
		task := content.Task{Type: content.TaskBank, Items: []content.ItemRequirement{
			{Name: "Axe", AnyOf: []int{1359, 1351}},
			{Name: "Prayer potion", Family: "prayer_potion", Qty: intp(2), Optional: true},
			{Name: "Raw shrimps", ItemID: intp(317), Qty: intp(50), Noted: true},
		}}
		spec, _, err := Resolve(input(custom(task), content.ModeIronman, nil, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		it := spec.Bank.Items
		if len(it) != 3 || !slices.Equal(it[0].IDs, []int{1359, 1351}) || len(it[1].IDs) < 2 || !it[1].Optional || it[1].Qty != 2 || !it[2].Noted || it[2].Qty != 50 {
			t.Fatalf("items = %+v", it)
		}
	})
	t.Run("buy what is missing", func(t *testing.T) {
		task := content.Task{Type: content.TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 5}
		h := rules.ItemHoldings{"inventory": live(map[int]int64{1351: 2}), "bank": live(map[int]int64{}), "equipment": live(map[int]int64{})}
		spec, _, err := Resolve(input(custom(task), content.ModeIronman, h, nil), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		b := spec.Buy
		if b.Qty != 3 || b.Item != 1351 || b.ItemName != "Bronze axe" || len(b.NPCs) == 0 || b.Shop != "shop:bobs_brilliant_axes" || strings.HasSuffix(b.ShopName, ".") {
			t.Fatalf("buy = %+v", b)
		}
	})
	for _, tc := range []struct {
		name string
		in   Input
		key  string
		want string
	}{
		{"met", input(route, content.ModeIronman, nil, at(3188, 3220)), "step:t::a#0", "já está cumprida"},
		{"talk", input(route, content.ModeIronman, nil, nil), "step:t::a#1", "só roda travel, bank e buy"},
		{"uim bank", input(route, content.ModeUIM, nil, nil), "step:t::b#0", "Ultimate Ironman"},
		{"via", input(custom(content.Task{Type: content.TaskTravel, Location: ref("location:lumbridge"), Via: ref("item:8007")}), content.ModeIronman, nil, nil), "task:0#0", "via"},
		{"buy owned", input(route, content.ModeIronman, rules.ItemHoldings{"bank": live(map[int]int64{1351: 1})}, nil), "step:t::b#1", "já está cumprida"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Resolve(tc.in, tc.key); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	if _, _, err := Resolve(input(route, content.ModeIronman, nil, nil), "step:t::zz#0"); !errors.Is(err, ErrNotATask) {
		t.Errorf("unknown key: %v", err)
	}
}

func TestTarget(t *testing.T) {
	box := execspec.Area{MinX: 10, MinY: 10, MaxX: 20, MaxY: 20}
	if got, ok := Target(box, execspec.Tile{X: 12, Y: 18}); !ok || got != (execspec.Tile{X: 12, Y: 18}) {
		t.Errorf("point inside = %+v", got)
	}
	if got, ok := Target(box, execspec.Tile{X: 0, Y: 0}); !ok || got != (execspec.Tile{X: 15, Y: 15}) {
		t.Errorf("point outside = %+v", got)
	}
	// An L-shaped polygon whose box center is outside it.
	l := execspec.Area{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10, Polygon: [][2]int{{0, 0}, {10, 0}, {10, 2}, {2, 2}, {2, 10}, {0, 10}}}
	got, ok := Target(l, execspec.Tile{X: 5, Y: 5})
	if !ok || !l.Contains(got.X, got.Y, 0) {
		t.Errorf("L = %+v %v", got, ok)
	}
	if _, ok := Target(execspec.Area{Plane: 1, MinX: 0, MinY: 0, MaxX: 2, MaxY: 2, Polygon: [][2]int{{0, 0}, {0, 0}, {0, 0}}}, execspec.Tile{}); !ok {
		t.Error("a degenerate polygon still has its point")
	}
}
