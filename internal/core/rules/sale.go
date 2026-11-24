package rules

import (
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
)

// A Grand Exchange sale (docs/12 section 6) is proven on the live
// inventory, where the GE takes the items from and the agent collects the
// coins into: the items left it, and the coins it holds reached what it held
// before plus the least the sale pays. Only a live inventory with a full
// baseline proves either (CLAUDE.md rule 8). The item merely absent from the
// inventory proves nothing (it may be in the bank, or never was there): only
// a run's proof does, which the server records once it sees it.

// Sold is a ge_sell's termination: unmet while the live inventory holds the
// item, noted or not (there is something to sell), met only when a run of
// the task proved the sale (proven: SaleProven, recorded by the server), and
// unknown otherwise, with the reason.
func Sold(h ItemHoldings, catalog *entities.Registry, itemID int, proven bool) (Verdict, string) {
	inv := h[ContainerInventory]
	if inv.State == ingest.ItemLive && h.Count(ContainerInventory, itemID, entities.CompareIgnoreNoted, catalog) > 0 {
		return Unmet, ""
	}
	switch {
	case proven:
		return Met, ""
	case !inv.ProvesAbsence():
		return Unknown, "inventário não compartilhado ao vivo"
	}
	return Unknown, "o inventário não tem o item e nenhum run provou a venda: nada a vender (um bank antes o traz)"
}

// SaleProven says whether a sale's run paid: the live inventory, with a
// full baseline, holds none of the item (noted or not) and at least
// coinsAtLeast coins. Coins anywhere else (the bank, where an aborted offer
// returns them) never count.
func SaleProven(h ItemHoldings, catalog *entities.Registry, itemID int, coinsAtLeast int64) (Verdict, string) {
	if !h[ContainerInventory].ProvesAbsence() {
		return Unknown, "inventário não compartilhado ao vivo"
	}
	if h.Count(ContainerInventory, itemID, entities.CompareIgnoreNoted, catalog) > 0 {
		return Unmet, "o item ainda está no inventário"
	}
	if h.Count(ContainerInventory, entities.CoinsID, entities.CompareExact, catalog) < coinsAtLeast {
		return Unmet, "as moedas da venda ainda não estão no inventário"
	}
	return Met, ""
}
