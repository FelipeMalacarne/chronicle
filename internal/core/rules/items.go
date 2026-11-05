package rules

import (
	"sort"
	"strings"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

// Item containers reported by the plugin. "poh" is valid in item_owned rules
// but no source observes it yet, so it always contributes unknown.
const (
	ContainerBank      = "bank"
	ContainerInventory = "inventory"
	ContainerEquipment = "equipment"
	ContainerPOH       = "poh"
)

// DefaultItemContainers is the item_owned default when a rule lists none.
var DefaultItemContainers = []string{ContainerBank, ContainerInventory, ContainerEquipment}

// ContainerItems is the merged server-side view of one item container.
type ContainerItems struct {
	// State is the freshness of the latest accepted observation; empty means
	// the container was never observed (missing scope or local opt-in).
	State      ingest.ItemState
	ObservedAt time.Time
	// Complete is true when a full image (a full push with a readable
	// observation) is the baseline of Quantities, so a missing id means the
	// container did not hold it. Deltas alone only prove presence.
	Complete bool
	// Quantities maps item id to quantity; zero quantities are never stored.
	Quantities map[int]int64
}

// Readable reports whether the container contents are usable as evidence.
func (c ContainerItems) Readable() bool {
	return c.State == ingest.ItemLive || c.State == ingest.ItemRestored
}

// ProvesAbsence reports whether a missing id is evidence of not holding it:
// only a live container with a full baseline can say so. A restored mirror is
// complete but may be outdated.
func (c ContainerItems) ProvesAbsence() bool {
	return c.State == ingest.ItemLive && c.Complete
}

// ItemHoldings maps container name to its merged view. A container missing
// from the map was never observed.
type ItemHoldings map[string]ContainerItems

// Count returns the quantity of itemID held in one container under cmp. A nil
// catalog compares ids exactly.
func (h ItemHoldings) Count(container string, itemID int, cmp entities.ItemComparison, catalog *entities.Registry) int64 {
	c, ok := h[container]
	if !ok || !c.Readable() {
		return 0
	}
	if catalog == nil || cmp == entities.CompareExact {
		return c.Quantities[itemID]
	}
	var n int64
	for id, qty := range c.Quantities {
		if catalog.SameItem(id, itemID, cmp) {
			n += qty
		}
	}
	return n
}

// ContainerEvidence is what one container contributed to an item verdict.
type ContainerEvidence struct {
	State    ingest.ItemState `json:"state,omitempty"`
	Observed bool             `json:"observed"`
	Complete bool             `json:"complete"`
	Qty      int64            `json:"qty"`
	// ObservedAt is omitted for never-observed containers.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

// Ownership is the outcome of "does the account own Qty of this item in any
// of these containers". Stale is true when a met verdict relies on a restored
// container, i.e. the item was there when the mirror was saved.
type Ownership struct {
	Verdict    Verdict                      `json:"verdict"`
	Stale      bool                         `json:"stale,omitempty"`
	Held       int64                        `json:"held"`
	Required   int64                        `json:"required"`
	Containers map[string]ContainerEvidence `json:"containers"`
	Reason     string                       `json:"reason,omitempty"`
}

// Owns evaluates ownership of qty of itemID across containers. Noted and
// unnoted forms count as the same item; variants (doses, charges) do not.
func Owns(h ItemHoldings, catalog *entities.Registry, itemID int, qty int64, containers []string) Ownership {
	return owns(h, qty, containers, func(name string) int64 {
		return h.Count(name, itemID, entities.CompareIgnoreNoted, catalog)
	})
}

// OwnsFamily evaluates ownership of qty units of any variant of an item
// family (docs/09 section 3.4): every dose or charge, noted or not, adds up.
// An unknown family is never met.
func OwnsFamily(h ItemHoldings, catalog *entities.Registry, family string, qty int64, containers []string) Ownership {
	var ids []int
	if catalog != nil {
		ids, _ = catalog.Family(family)
	}
	if len(ids) == 0 {
		return Ownership{Verdict: Unknown, Required: max(qty, 1), Containers: map[string]ContainerEvidence{}, Reason: "unknown item family " + family}
	}
	return owns(h, qty, containers, func(name string) int64 {
		return h.Count(name, ids[0], entities.CompareFamily, catalog)
	})
}

// OwnsAnyOf evaluates "qty of any one of ids" (an any_of requirement): one id
// must reach qty on its own. It returns the best outcome (met, then met only
// through a restored container, then unknown, then unmet) and the id that
// produced it, preferring earlier ids on ties.
func OwnsAnyOf(h ItemHoldings, catalog *entities.Registry, ids []int, qty int64, containers []string) (Ownership, int) {
	alts := make([]entities.ItemQty, len(ids))
	for i, id := range ids {
		alts[i] = entities.ItemQty{ID: id, N: int(qty)}
	}
	return OwnsAlternatives(h, catalog, alts, containers)
}

// OwnsAlternatives is OwnsAnyOf with a quantity per alternative (an
// item:any objective): one alternative must reach its own quantity alone.
// It returns the best outcome, ranked as OwnsAnyOf does, and the id that
// produced it; earlier alternatives win ties.
func OwnsAlternatives(h ItemHoldings, catalog *entities.Registry, alts []entities.ItemQty, containers []string) (Ownership, int) {
	rank := func(o Ownership) int {
		switch {
		case o.Verdict == Met && !o.Stale:
			return 3
		case o.Verdict == Met:
			return 2
		case o.Verdict == Unknown:
			return 1
		}
		return 0
	}
	var (
		best   Ownership
		bestID int
	)
	for i, a := range alts {
		o := Owns(h, catalog, a.ID, int64(a.N), containers)
		if i == 0 || rank(o) > rank(best) {
			best, bestID = o, a.ID
		}
	}
	return best, bestID
}

func owns(h ItemHoldings, qty int64, containers []string, count func(string) int64) Ownership {
	if qty < 1 {
		qty = 1
	}
	if len(containers) == 0 {
		containers = DefaultItemContainers
	}
	out := Ownership{Required: qty, Containers: make(map[string]ContainerEvidence, len(containers))}
	var live int64
	var blind []string
	for _, name := range containers {
		c, observed := h[name]
		ev := ContainerEvidence{State: c.State, Observed: observed, Complete: c.Complete}
		if observed {
			at := c.ObservedAt
			ev.ObservedAt = &at
		}
		if observed && c.Readable() {
			ev.Qty = count(name)
			out.Held += ev.Qty
			if c.State == ingest.ItemLive {
				live += ev.Qty
			}
		}
		if !c.ProvesAbsence() {
			blind = append(blind, name+" "+blindReason(c, observed))
		}
		out.Containers[name] = ev
	}
	switch {
	case live >= qty:
		out.Verdict = Met
	case out.Held >= qty:
		out.Verdict = Met
		out.Stale = true
		out.Reason = "met only through a restored container"
	case len(blind) == 0:
		out.Verdict = Unmet
	default:
		out.Verdict = Unknown
		sort.Strings(blind)
		out.Reason = "cannot prove absence: " + strings.Join(blind, ", ")
	}
	return out
}

func blindReason(c ContainerItems, observed bool) string {
	switch {
	case !observed:
		return "not observed"
	case c.State == ingest.ItemUnavailable:
		return "unavailable"
	case c.State == ingest.ItemRestored:
		return "restored"
	default:
		return "has no full baseline"
	}
}

func itemOwned(r content.ItemOwnedRule, ps PlayerState, ev *Evidence) {
	if r.ItemID == nil {
		ev.Reason = "item_owned without item_id"
		return
	}
	if ps.Items == nil {
		ev.Reason = "items not observed"
		return
	}
	o := Owns(ps.Items, ps.Catalog, *r.ItemID, int64(r.Qty), r.Containers)
	ev.Verdict = o.Verdict
	ev.Stale = o.Stale
	ev.Observed = o
	ev.Reason = o.Reason
}
