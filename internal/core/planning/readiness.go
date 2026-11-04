// Package planning evaluates route-level questions over a guide and the
// observed account state. It is pure: no IO and no clock.
//
// Three semantics are kept apart on purpose:
//   - ownership: the account holds an item somewhere (rules.Owns);
//   - readiness: the current inventory/equipment matches a segment loadout;
//   - bank actions: what the guide tells the player to do at the bank. They
//     are instructions, returned verbatim and never marked done.
package planning

import (
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// InventorySlots is the size of the player inventory.
const InventorySlots = 28

// Status summarizes a segment's readiness.
type Status string

const (
	// Ready: the loadout is fully specified and every required item and the
	// free-slot count are verified in live containers.
	Ready Status = "ready"
	// ListedReady: the loadout is partial; every listed item is verified but
	// the guide does not state the whole setup.
	ListedReady Status = "listed_ready"
	// NotReady: at least one required item or the free-slot count is
	// verifiably missing.
	NotReady Status = "not_ready"
	// Unknown: the observed state cannot decide (no live inventory, unresolved
	// item names, deltas without a baseline...).
	Unknown Status = "unknown"
	// NoLoadout: the guide does not specify a setup for this segment.
	NoLoadout Status = "no_loadout"
)

// Slot names of a loadout item.
const (
	SlotInventory = "inventory"
	SlotEquipment = "equipment"
)

// Segment is the readiness of one guide segment.
type Segment struct {
	GuideID       string               `json:"guide_id"`
	SegmentID     string               `json:"segment_id"`
	SectionID     string               `json:"section_id"`
	Kind          string               `json:"kind"`
	Title         string               `json:"title,omitempty"`
	FirstStepID   string               `json:"first_step_id,omitempty"`
	Status        Status               `json:"status"`
	LoadoutStatus string               `json:"loadout_status"`
	Items         []Item               `json:"items"`
	EmptySlots    *Slots               `json:"empty_slots,omitempty"`
	BankActions   []content.BankAction `json:"bank_actions"`
	Freshness     map[string]Freshness `json:"freshness"`
	Source        *content.Provenance  `json:"loadout_source,omitempty"`
	Reason        string               `json:"reason,omitempty"`
}

// Item is the readiness of one loadout item. Carried is about the live
// inventory/equipment; Owned is ownership anywhere, so the UI can say "it is
// in the bank" without implying that the player is ready.
type Item struct {
	Name       string           `json:"name"`
	ItemID     *int             `json:"item_id,omitempty"`
	Slot       string           `json:"slot"`
	Required   int64            `json:"required"`
	QtyStated  bool             `json:"qty_stated"`
	Optional   bool             `json:"optional,omitempty"`
	SourceText string           `json:"source_text,omitempty"`
	Verdict    rules.Verdict    `json:"verdict"`
	Carried    int64            `json:"carried"`
	Owned      *rules.Ownership `json:"owned,omitempty"`
	Reason     string           `json:"reason,omitempty"`
}

// Slots is the free-inventory-slot requirement. Free is bounded because
// quantities do not say how many slots unstackable items use when their
// stackability is not in the catalog.
type Slots struct {
	Required int           `json:"required"`
	FreeMin  *int          `json:"free_min,omitempty"`
	FreeMax  *int          `json:"free_max,omitempty"`
	Verdict  rules.Verdict `json:"verdict"`
	Reason   string        `json:"reason,omitempty"`
}

// Freshness describes one container as used by the evaluation.
type Freshness struct {
	State    ingest.ItemState `json:"state,omitempty"`
	Observed bool             `json:"observed"`
	Complete bool             `json:"complete"`
	// Live reports whether the container can prove a current loadout.
	Live bool `json:"live"`
	// ObservedAt is omitted for never-observed containers.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

// EvaluateSegment computes the readiness of seg (which must belong to g)
// against the account's item holdings.
func EvaluateSegment(g *content.Guide, seg *content.Segment, h rules.ItemHoldings, catalog *entities.Registry) Segment {
	out := Segment{
		GuideID: g.ID, SegmentID: seg.ID, SectionID: seg.SectionID, Kind: seg.Kind, Title: seg.Title,
		Items: []Item{}, BankActions: append([]content.BankAction{}, seg.BankActions...),
		Freshness: freshness(h),
	}
	if len(seg.Steps) > 0 {
		out.FirstStepID = seg.Steps[0].ID
	}
	lo := seg.Loadout
	if lo == nil || lo.Status == content.LoadoutUnknown {
		out.LoadoutStatus = content.LoadoutUnknown
		out.Status = NoLoadout
		out.Reason = "the guide does not specify a loadout for this segment"
		return out
	}
	out.LoadoutStatus = lo.Status
	out.Source = lo.Source

	var verdicts []rules.Verdict
	for _, li := range lo.Equipment {
		it := evaluateItem(li, SlotEquipment, h, catalog)
		out.Items = append(out.Items, it)
		if !it.Optional {
			verdicts = append(verdicts, it.Verdict)
		}
	}
	for _, li := range lo.Inventory {
		it := evaluateItem(li, SlotInventory, h, catalog)
		out.Items = append(out.Items, it)
		if !it.Optional {
			verdicts = append(verdicts, it.Verdict)
		}
	}
	if lo.EmptySlots != nil {
		out.EmptySlots = evaluateSlots(*lo.EmptySlots, h, catalog)
		verdicts = append(verdicts, out.EmptySlots.Verdict)
	}

	if len(verdicts) == 0 {
		out.Status = NoLoadout
		out.Reason = "the loadout lists no required item"
		return out
	}
	switch combine(verdicts) {
	case rules.Unmet:
		out.Status = NotReady
	case rules.Unknown:
		out.Status = Unknown
		out.Reason = "some requirements cannot be verified from live containers"
	default:
		if lo.Status == content.LoadoutKnown {
			out.Status = Ready
		} else {
			out.Status = ListedReady
			out.Reason = "every listed item is carried, but the guide lists the setup only partially"
		}
	}
	return out
}

func evaluateItem(li content.LoadoutItem, slot string, h rules.ItemHoldings, catalog *entities.Registry) Item {
	it := Item{
		Name: li.Name, ItemID: li.ItemID, Slot: slot, Required: 1, QtyStated: li.Qty != nil,
		Optional: li.Optional, SourceText: li.SourceText, Verdict: rules.Unknown,
	}
	if li.Qty != nil && *li.Qty > 0 {
		it.Required = int64(*li.Qty)
	}
	if li.ItemID == nil {
		it.Reason = "item name not resolved to an item id"
		return it
	}
	id := *li.ItemID
	owned := rules.Owns(h, catalog, id, it.Required, nil)
	it.Owned = &owned

	// An inventory item may also be wielded (axes, pickaxes); an equipment
	// item must be worn. Ids compare exactly: a noted stack is not the
	// unnoted item it notes, and doses are distinct items.
	carriers := []string{rules.ContainerEquipment}
	if slot == SlotInventory {
		carriers = []string{rules.ContainerInventory, rules.ContainerEquipment}
	}
	provesAbsence := true
	for _, name := range carriers {
		c, ok := h[name]
		if !ok || c.State != ingest.ItemLive {
			provesAbsence = false
			continue
		}
		it.Carried += c.Quantities[id]
		if !c.Complete {
			provesAbsence = false
		}
	}
	switch {
	case it.Carried >= it.Required:
		it.Verdict = rules.Met
	case provesAbsence:
		it.Verdict = rules.Unmet
	default:
		it.Reason = carrierReason(h, carriers)
	}
	return it
}

func carrierReason(h rules.ItemHoldings, carriers []string) string {
	for _, name := range carriers {
		c, ok := h[name]
		switch {
		case !ok:
			return name + " not observed"
		case c.State == ingest.ItemRestored:
			return name + " is a restored mirror and cannot prove a live loadout"
		case c.State != ingest.ItemLive:
			return name + " unavailable"
		case !c.Complete:
			return name + " has no full baseline"
		}
	}
	return ""
}

func evaluateSlots(required int, h rules.ItemHoldings, catalog *entities.Registry) *Slots {
	s := &Slots{Required: required, Verdict: rules.Unknown}
	inv, ok := h[rules.ContainerInventory]
	switch {
	case !ok || inv.State != ingest.ItemLive:
		s.Reason = "inventory not observed live"
		return s
	case !inv.Complete:
		s.Reason = "inventory has no full baseline"
		return s
	}
	// Catalog items occupy an exact number of slots; an item outside the
	// catalog occupies between one slot and one per unit.
	var usedMin, usedMax int
	for id, qty := range inv.Quantities {
		if qty <= 0 {
			continue
		}
		slots := int(min(qty, InventorySlots))
		form, known := itemForm(catalog, id)
		switch {
		case known && form.Stackable:
			usedMin++
			usedMax++
		case known:
			usedMin += slots
			usedMax += slots
		default:
			usedMin++
			usedMax += slots
		}
	}
	freeMin := max(InventorySlots-usedMax, 0)
	freeMax := max(InventorySlots-usedMin, 0)
	s.FreeMin, s.FreeMax = &freeMin, &freeMax
	switch {
	case freeMin >= required:
		s.Verdict = rules.Met
	case freeMax < required:
		s.Verdict = rules.Unmet
	default:
		s.Reason = "stackability of some carried items is unknown"
	}
	return s
}

func itemForm(catalog *entities.Registry, id int) (entities.ItemForm, bool) {
	if catalog == nil {
		return entities.ItemForm{}, false
	}
	return catalog.Item(id)
}

func freshness(h rules.ItemHoldings) map[string]Freshness {
	out := make(map[string]Freshness, len(rules.DefaultItemContainers))
	for _, name := range rules.DefaultItemContainers {
		c, ok := h[name]
		f := Freshness{State: c.State, Observed: ok, Complete: c.Complete, Live: ok && c.State == ingest.ItemLive}
		if ok {
			at := c.ObservedAt
			f.ObservedAt = &at
		}
		out[name] = f
	}
	return out
}

// combine is AND over three-valued verdicts: any unmet wins, then unknown.
func combine(vs []rules.Verdict) rules.Verdict {
	out := rules.Met
	for _, v := range vs {
		switch v {
		case rules.Unmet:
			return rules.Unmet
		case rules.Unknown:
			out = rules.Unknown
		}
	}
	return out
}
