package rules

import (
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
)

// TestSaleProven: a sale is proven only on a live inventory with a full
// baseline: the item (noted or not) gone and the coins arrived. The task's
// termination (Sold) is never met by the item's absence alone: only by a
// run's recorded proof, and never while the inventory holds the item.
func TestSaleProven(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	inv := func(state ingest.ItemState, complete bool, q map[int]int64) ItemHoldings {
		return ItemHoldings{ContainerInventory: {State: state, Complete: complete, Quantities: q}}
	}
	const glass, noted, want = 1775, 1776, 170_000
	for _, tc := range []struct {
		name string
		h    ItemHoldings
		// sold is Sold without a recorded proof, proven with one.
		sold, proven, paid Verdict
	}{
		{"sold and paid", inv(ingest.ItemLive, true, map[int]int64{entities.CoinsID: want}), Unknown, Met, Met},
		{"items left, coins not yet", inv(ingest.ItemLive, true, map[int]int64{entities.CoinsID: want - 1}), Unknown, Met, Unmet},
		{"noted stack still there", inv(ingest.ItemLive, true, map[int]int64{noted: 3, entities.CoinsID: want}), Unmet, Unmet, Unmet},
		{"unnoted still there", inv(ingest.ItemLive, true, map[int]int64{glass: 1}), Unmet, Unmet, Unmet},
		{"held on deltas only is still something to sell", inv(ingest.ItemLive, false, map[int]int64{glass: 1}), Unmet, Unmet, Unknown},
		{"restored inventory proves nothing", inv(ingest.ItemRestored, true, map[int]int64{entities.CoinsID: want}), Unknown, Met, Unknown},
		{"deltas only prove nothing", inv(ingest.ItemLive, false, map[int]int64{entities.CoinsID: want}), Unknown, Met, Unknown},
		{"the item in the bank is not a sale", ItemHoldings{
			ContainerInventory: {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{entities.CoinsID: 5}},
			ContainerBank:      {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{glass: 100}},
		}, Unknown, Met, Unmet},
		{"coins in the bank do not count", ItemHoldings{
			ContainerInventory: {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{}},
			ContainerBank:      {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{entities.CoinsID: want}},
		}, Unknown, Met, Unmet},
		{"never observed", nil, Unknown, Met, Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v, _ := Sold(tc.h, reg, glass, false); v != tc.sold {
				t.Errorf("Sold = %s, want %s", v, tc.sold)
			}
			if v, _ := Sold(tc.h, reg, glass, true); v != tc.proven {
				t.Errorf("Sold proven = %s, want %s", v, tc.proven)
			}
			if v, _ := SaleProven(tc.h, reg, glass, want); v != tc.paid {
				t.Errorf("SaleProven = %s, want %s", v, tc.paid)
			}
		})
	}
}
