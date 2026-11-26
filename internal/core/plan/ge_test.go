package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/ge"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

const (
	glass      = 1775 // Molten glass
	glassNoted = 1776
	coins      = 995
)

func glassPrice() map[int]ge.Price {
	limit := 10_000
	return map[int]ge.Price{glass: {
		ItemID: glass, High: 140, HighAt: now.Add(-time.Hour), Low: 130, LowAt: now.Add(-time.Hour),
		Limit: &limit, Source: ge.SourceWiki, FetchedAt: now,
	}}
}

// TestResolveGE: a regular account's Grand Exchange tasks resolve to
// offers priced from the stored price and backed by the live inventory;
// every other mode, a missing or old price and an inventory that cannot
// back the offer are refused.
func TestResolveGE(t *testing.T) {
	c, reg := load(t)
	buy := content.Task{Type: content.TaskGEBuy, Item: ref("item:1775"), Qty: 1000}
	sell := content.Task{Type: content.TaskGESell, Item: ref("item:1775")}
	input := func(task content.Task, mode string, h rules.ItemHoldings, prices map[int]ge.Price) Input {
		ps := rules.PlayerState{Items: h, Catalog: reg}
		return Input{
			Plan:    Plan{ID: "pln_test0001", Kind: KindCustom, Items: []Item{{Kind: ItemTask, Task: &task}}},
			Content: c, Registry: reg, Holdings: h, Now: now, Prices: prices,
			Facts: panel.Facts{Mode: mode, State: &ps},
		}
	}
	inv := func(q map[int]int64) rules.ItemHoldings { return rules.ItemHoldings{rules.ContainerInventory: live(q)} }

	t.Run("buy what is missing at the instant-buy price plus the margin", func(t *testing.T) {
		h := rules.ItemHoldings{rules.ContainerInventory: live(map[int]int64{coins: 200_000}), rules.ContainerBank: live(map[int]int64{glass: 400})}
		spec, v, err := Resolve(input(buy, content.ModeRegular, h, glassPrice()), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		want := execspec.GEOffer{Item: glass, ItemName: "Molten glass", Qty: 600, Price: 147}
		if spec.Type != execspec.TypeGEBuy || spec.GEBuy == nil || *spec.GEBuy != want || v.Until != "1000 Molten glass" {
			t.Fatalf("buy = %+v %+v", spec.GEBuy, v)
		}
	})
	t.Run("a buy stops at the GE's 4-hour limit", func(t *testing.T) {
		p := glassPrice()
		g := p[glass]
		g.Limit = new(int)
		*g.Limit = 100
		p[glass] = g
		spec, _, err := Resolve(input(buy, content.ModeRegular, inv(map[int]int64{coins: 200_000}), p), "task:0#0")
		if err != nil || spec.GEBuy.Qty != 100 {
			t.Fatalf("buy = %+v %v", spec.GEBuy, err)
		}
	})
	t.Run("sell the inventory's stack, with the coins the proof waits for", func(t *testing.T) {
		spec, v, err := Resolve(input(sell, content.ModeRegular, inv(map[int]int64{glassNoted: 1400, coins: 1000}), glassPrice()), "task:0#0")
		if err != nil {
			t.Fatal(err)
		}
		// 130 less 5% is 123; the tax takes 2 a glass, so the sale pays at least 121 each.
		want := execspec.GESell{GEOffer: execspec.GEOffer{Item: glass, ItemName: "Molten glass", Qty: 1400, Price: 123}, CoinsAtLeast: 1000 + 1400*121}
		if spec.Type != execspec.TypeGESell || spec.GESell == nil || *spec.GESell != want || v.State != rules.Unmet {
			t.Fatalf("sell = %+v %+v", spec.GESell, v)
		}
	})
	for _, tc := range []struct {
		name string
		in   Input
		want string
	}{
		{"ironman buy", input(buy, content.ModeIronman, inv(map[int]int64{coins: 200_000}), glassPrice()), "só do modo regular"},
		{"hcim sell", input(sell, content.ModeHCIM, inv(map[int]int64{glass: 5}), glassPrice()), "só do modo regular"},
		{"no price", input(buy, content.ModeRegular, inv(map[int]int64{coins: 200_000}), nil), "não tem preço guardado"},
		{"old price", input(buy, content.ModeRegular, inv(map[int]int64{coins: 200_000}), func() map[int]ge.Price {
			p := glassPrice()
			g := p[glass]
			g.HighAt = now.Add(-25 * time.Hour)
			p[glass] = g
			return p
		}()), "no recent price"},
		{"not enough coins", input(buy, content.ModeRegular, inv(map[int]int64{coins: 1000}), glassPrice()), "faltam moedas no inventário"},
		{"inventory not live", input(buy, content.ModeRegular, rules.ItemHoldings{rules.ContainerInventory: {State: ingest.ItemRestored, Complete: true, Quantities: map[int]int64{coins: 1 << 30}}}, glassPrice()), "não é compartilhado ao vivo"},
		{"bought already", input(buy, content.ModeRegular, rules.ItemHoldings{rules.ContainerBank: live(map[int]int64{glass: 1000})}, glassPrice()), "já está cumprida"},
		{"nothing to sell", input(sell, content.ModeRegular, inv(map[int]int64{coins: 5}), glassPrice()), "nada a vender"},
		{"sold already", func() Input {
			in := input(sell, content.ModeRegular, inv(map[int]int64{coins: 5}), glassPrice())
			in.TasksProven = map[string]string{"task:0#0": "0123abcd"}
			return in
		}(), "já está cumprida"},
		{"two forms", input(sell, content.ModeRegular, inv(map[int]int64{glass: 5, glassNoted: 100}), glassPrice()), "noted e unnoted"},
		{"sell blind", input(sell, content.ModeRegular, nil, glassPrice()), "não é compartilhado ao vivo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Resolve(tc.in, "task:0#0"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestCheckItemsGE: a plan's Grand Exchange task is accepted for a regular
// account only.
func TestCheckItemsGE(t *testing.T) {
	c, reg := load(t)
	items := []Item{{Kind: ItemTask, Task: &content.Task{Type: content.TaskGEBuy, Item: ref("item:1775"), Qty: 10}}}
	if msgs := CheckItems(items, content.ModeRegular, c, reg); len(msgs) > 0 {
		t.Fatalf("regular: %v", msgs)
	}
	if msgs := CheckItems(items, content.ModeGIM, c, reg); len(msgs) != 1 || !strings.Contains(msgs[0], "regular accounts only") {
		t.Fatalf("gim: %v", msgs)
	}
}

// TestNextGESale: a ge_sell is met only by a run's recorded proof, never by
// the item merely missing from the inventory (in the bank, or never there),
// and is to do again once the inventory holds the item.
func TestNextGESale(t *testing.T) {
	c, reg := load(t)
	sell := content.Task{Type: content.TaskGESell, Item: ref("item:1775")}
	inBank := rules.ItemHoldings{
		rules.ContainerInventory: live(map[int]int64{coins: 5}),
		rules.ContainerBank:      live(map[int]int64{glass: 1400}),
	}
	inInventory := rules.ItemHoldings{rules.ContainerInventory: live(map[int]int64{glassNoted: 1400})}
	proven := map[string]string{"task:0#0": "0123abcd"}
	for _, tc := range []struct {
		name    string
		h       rules.ItemHoldings
		proven  map[string]string
		status  string
		state   rules.Verdict
		reason  string
		skipped int
	}{
		{"the item in the bank is no sale", inBank, nil, StatusTask, rules.Unknown, "nenhum run provou a venda", 0},
		{"another task's proof does not count", inBank, map[string]string{"task:1#0": "0123abcd"}, StatusTask, rules.Unknown, "nenhum run provou a venda", 0},
		{"a blind inventory proves nothing", nil, nil, StatusTask, rules.Unknown, "não compartilhado ao vivo", 0},
		{"the item in the inventory is to sell", inInventory, nil, StatusTask, rules.Unmet, "", 0},
		{"a proven sale is met", inBank, proven, StatusDone, "", "", 1},
		{"a proven sale stays met without inventory", nil, proven, StatusDone, "", "", 1},
		{"the item back in the inventory is to sell again", inInventory, proven, StatusTask, rules.Unmet, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := rules.PlayerState{Items: tc.h, Catalog: reg}
			got := Next(Input{
				Plan:    Plan{ID: "pln_test0001", Kind: KindCustom, Items: []Item{{Kind: ItemTask, Task: &sell}}},
				Content: c, Registry: reg, Holdings: tc.h, Now: now, TasksProven: tc.proven,
				Facts: panel.Facts{Mode: content.ModeRegular, State: &ps},
			})
			if got.Status != tc.status || len(got.Skipped) != tc.skipped {
				t.Fatalf("next = %s (skipped %d), want %s (skipped %d): %+v", got.Status, len(got.Skipped), tc.status, tc.skipped, got)
			}
			if tc.status != StatusTask {
				return
			}
			if got.Task.State != tc.state || !strings.Contains(got.Task.Reason, tc.reason) {
				t.Fatalf("task = %s %q, want %s %q", got.Task.State, got.Task.Reason, tc.state, tc.reason)
			}
		})
	}
}
