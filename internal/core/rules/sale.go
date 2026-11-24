package rules

import "github.com/FelipeMalacarne/chronicle/internal/core/entities"

// A Grand Exchange sale (docs/12 section 6) is proven on the live
// inventory, where the GE takes the items from and the agent collects the
// coins into: the items left it, and the coins it holds reached what it held
// before plus the least the sale pays. Only a live inventory with a full
// baseline proves either (CLAUDE.md rule 8).

// Sold says whether the live inventory holds none of itemID, noted or not.
// Unknown, with the reason, when the inventory is not live with a full
// baseline.
func Sold(h ItemHoldings, catalog *entities.Registry, itemID int) (Verdict, string) {
	if !h[ContainerInventory].ProvesAbsence() {
		return Unknown, "inventário não compartilhado ao vivo"
	}
	if h.Count(ContainerInventory, itemID, entities.CompareIgnoreNoted, catalog) > 0 {
		return Unmet, ""
	}
	return Met, ""
}

// SaleProven says whether a sale whose item left the inventory (Sold met)
// also paid: the live inventory holds at least coinsAtLeast coins.
func SaleProven(h ItemHoldings, catalog *entities.Registry, itemID int, coinsAtLeast int64) (Verdict, string) {
	v, why := Sold(h, catalog, itemID)
	if v != Met {
		return v, why
	}
	if h.Count(ContainerInventory, entities.CoinsID, entities.CompareExact, catalog) < coinsAtLeast {
		return Unmet, "as moedas da venda ainda não estão no inventário"
	}
	return Met, ""
}
