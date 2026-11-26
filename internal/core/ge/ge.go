// Package ge is the Grand Exchange of a regular account (docs/12 section
// 6): an item's market price as the server stores it, the offer price a
// typed ge_buy or ge_sell carries, and the least a sale pays after the GE
// tax. Pure: the prices come from internal/sources/wikiprices through the
// store, and plan.Resolve applies the policy.
package ge

import (
	"errors"
	"fmt"
	"time"
)

// SourceWiki is the OSRS Wiki real-time prices API.
const SourceWiki = "wiki"

// Price is the last market price of one item. High is the last instant-buy
// price and Low the last instant-sell price, each with the time of that
// trade (zero, with a zero time, when the source never saw one). Limit is
// the GE's 4-hour buy limit and Members whether only members may buy it,
// both nil when the source does not say.
type Price struct {
	ItemID    int
	High      int64
	HighAt    time.Time
	Low       int64
	LowAt     time.Time
	Limit     *int
	Members   *bool
	Source    string
	FetchedAt time.Time
}

// Policy of the offer price.
const (
	// MaxAge is how old the trade behind a price may be: an older one is
	// not the market any more.
	MaxAge = 24 * time.Hour
	// MarginPercent is paid over the last instant-buy price on a buy, and
	// given up under the last instant-sell price on a sale, so the offer
	// fills now instead of waiting for the market.
	MarginPercent = 5
	// MaxPrice is the most an offer may ask per item (the GE's coin stack).
	MaxPrice = 2_147_483_647
)

// Tax of a sale (OSRS Wiki, Grand Exchange, "Convenience fee and item
// sink"): 2% of each item's price, rounded down, capped per item. Some items
// are exempt; counting them taxed only lowers the bound.
const (
	taxDivisor = 50 // 2%
	TaxCap     = 5_000_000
)

// ErrNoPrice: there is no usable price for the item.
var ErrNoPrice = errors.New("ge: no recent price")

// BuyOffer is the price per item a buy offers: the last instant-buy price
// plus the margin, rounded up. It refuses a price the source never saw or
// one older than MaxAge at now.
func BuyOffer(p Price, now time.Time) (int64, error) {
	if p.High < 1 || p.HighAt.IsZero() {
		return 0, fmt.Errorf("%w: the wiki never saw item %d bought", ErrNoPrice, p.ItemID)
	}
	if now.Sub(p.HighAt) > MaxAge {
		return 0, fmt.Errorf("%w: item %d was last bought at %s", ErrNoPrice, p.ItemID, p.HighAt.UTC().Format(time.RFC3339))
	}
	offer := (p.High*(100+MarginPercent) + 99) / 100
	if offer > MaxPrice {
		return 0, fmt.Errorf("ge: item %d costs more than one offer can pay", p.ItemID)
	}
	return offer, nil
}

// SellOffer is the price per item a sale asks: the last instant-sell price
// less the margin, rounded down, at least 1. It refuses a price the source
// never saw or one older than MaxAge at now.
func SellOffer(p Price, now time.Time) (int64, error) {
	if p.Low < 1 || p.LowAt.IsZero() {
		return 0, fmt.Errorf("%w: the wiki never saw item %d sold", ErrNoPrice, p.ItemID)
	}
	if now.Sub(p.LowAt) > MaxAge {
		return 0, fmt.Errorf("%w: item %d was last sold at %s", ErrNoPrice, p.ItemID, p.LowAt.UTC().Format(time.RFC3339))
	}
	return min(max(p.Low*(100-MarginPercent)/100, 1), MaxPrice), nil
}

// Tax is the GE tax on one item sold at price.
func Tax(price int64) int64 { return min(price/taxDivisor, TaxCap) }

// LeastProceeds is the least a sale of qty items at an asking price pays:
// a sell offer fills at its price or better, less the tax of each item.
func LeastProceeds(price, qty int64) int64 { return (price - Tax(price)) * qty }
