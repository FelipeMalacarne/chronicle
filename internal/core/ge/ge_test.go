package ge

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func TestOffers(t *testing.T) {
	fresh := now.Add(-time.Hour)
	for _, tc := range []struct {
		name      string
		p         Price
		buy, sell int64
		noBuy     bool
		noSell    bool
	}{
		{name: "margin rounds the buy up and the sale down", p: Price{High: 101, HighAt: fresh, Low: 99, LowAt: fresh}, buy: 107, sell: 94},
		{name: "a 1 gp item still sells for 1", p: Price{High: 1, HighAt: fresh, Low: 1, LowAt: fresh}, buy: 2, sell: 1},
		{name: "above int32 cannot be one offer", p: Price{High: 7_949_999_997, HighAt: fresh, Low: 2_100_000_000, LowAt: fresh}, noBuy: true, sell: 1_995_000_000},
		{name: "never bought", p: Price{Low: 50, LowAt: fresh}, noBuy: true, sell: 47},
		{name: "never sold", p: Price{High: 50, HighAt: fresh}, buy: 53, noSell: true},
		{name: "older than a day", p: Price{High: 50, HighAt: now.Add(-MaxAge - time.Second), Low: 50, LowAt: now.Add(-MaxAge)}, noBuy: true, sell: 47},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buy, err := BuyOffer(tc.p, now)
			if tc.noBuy != (err != nil) || !tc.noBuy && buy != tc.buy {
				t.Errorf("buy = %d, %v; want %d", buy, err, tc.buy)
			}
			sell, err := SellOffer(tc.p, now)
			if tc.noSell != (err != nil) || !tc.noSell && sell != tc.sell {
				t.Errorf("sell = %d, %v; want %d", sell, err, tc.sell)
			}
			if err != nil && tc.noSell && !errors.Is(err, ErrNoPrice) {
				t.Errorf("sell error %v is not ErrNoPrice", err)
			}
		})
	}
}

// TestLeastProceeds follows the wiki's examples: 2% rounded down, nothing
// under 50 gp, 5M at most per item.
func TestLeastProceeds(t *testing.T) {
	for _, tc := range []struct{ price, qty, want int64 }{
		{49, 10, 490},
		{50, 1, 49},
		{100, 3, 294},
		{1_000_000_000, 1, 995_000_000},
	} {
		if got := LeastProceeds(tc.price, tc.qty); got != tc.want {
			t.Errorf("LeastProceeds(%d, %d) = %d, want %d", tc.price, tc.qty, got, tc.want)
		}
	}
}
