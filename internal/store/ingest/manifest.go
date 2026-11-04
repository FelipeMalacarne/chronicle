package ingest

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

// Definition is the token-independent part of an ingest manifest.
type Definition struct {
	Version         int64
	Varbits         []int64
	Varps           []int64
	Collections     []int64
	OnlyItemIDs     []int64
	IntervalSeconds int
}

// BuildDefinition derives the v1 manifest from all loaded guide rules. The
// version is a stable positive 31-bit truncation of the canonical definition,
// which also fits the existing PostgreSQL INTEGER column.
func BuildDefinition(c *content.Content) Definition {
	varbits := map[int64]struct{}{}
	varps := map[int64]struct{}{}
	collections := map[int64]struct{}{}
	items := map[int64]struct{}{}
	if c != nil {
		for _, guide := range c.Guides {
			for _, section := range guide.Sections {
				for _, step := range section.Steps {
					for _, rule := range step.CompleteWhen {
						collectRule(rule, varbits, varps, collections, items)
					}
					for _, rule := range step.Prerequisites {
						collectRule(rule, varbits, varps, collections, items)
					}
				}
			}
		}
	}
	d := Definition{
		Varbits:         sortedIDs(varbits),
		Varps:           sortedIDs(varps),
		Collections:     sortedIDs(collections),
		OnlyItemIDs:     sortedIDs(items),
		IntervalSeconds: 10,
	}
	canonical, _ := json.Marshal(struct {
		Varbits     []int64 `json:"varbits"`
		Varps       []int64 `json:"varps"`
		Collections []int64 `json:"collections"`
		OnlyItemIDs []int64 `json:"only_item_ids"`
	}{d.Varbits, d.Varps, d.Collections, d.OnlyItemIDs})
	hash := sha256.Sum256(canonical)
	d.Version = int64(binary.BigEndian.Uint32(hash[:4]) & math.MaxInt32)
	if d.Version == 0 {
		d.Version = 1
	}
	return d
}

func collectRule(rule content.Rule, varbits, varps, collections, items map[int64]struct{}) {
	switch rule := rule.(type) {
	case content.AllRule:
		for _, child := range rule.All {
			collectRule(child, varbits, varps, collections, items)
		}
	case content.AnyRule:
		for _, child := range rule.Any {
			collectRule(child, varbits, varps, collections, items)
		}
	case content.NotRule:
		collectRule(rule.Not, varbits, varps, collections, items)
	case content.VarbitRule:
		varbits[int64(rule.ID)] = struct{}{}
	case content.VarpRule:
		varps[int64(rule.ID)] = struct{}{}
	case content.CollectionLogItemRule:
		if rule.ItemID != nil {
			collections[int64(*rule.ItemID)] = struct{}{}
		}
	case content.ItemOwnedRule:
		if rule.ItemID != nil {
			items[int64(*rule.ItemID)] = struct{}{}
		}
	}
}

func sortedIDs(set map[int64]struct{}) []int64 {
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (d Definition) manifest(scopes []coreingest.TokenScope, resync bool) coreingest.Manifest {
	has := make(map[coreingest.TokenScope]bool, len(scopes))
	for _, scope := range scopes {
		has[scope] = true
	}
	interval := d.IntervalSeconds
	if interval < 5 {
		interval = 10
	}
	return coreingest.Manifest{
		ContractVersion: coreingest.ContractVersion,
		ManifestVersion: d.Version,
		TokenScopes:     append([]coreingest.TokenScope(nil), scopes...),
		Varbits:         append([]int64{}, d.Varbits...),
		Varps:           append([]int64{}, d.Varps...),
		Collections:     append([]int64{}, d.Collections...),
		Skills:          true,
		Items: coreingest.ItemSelection{
			Bank:        has[coreingest.ScopeItemsBank],
			Inventory:   has[coreingest.ScopeItemsInventory],
			Equipment:   has[coreingest.ScopeItemsEquipment],
			OnlyItemIDs: append([]int64(nil), d.OnlyItemIDs...),
		},
		IntervalSeconds: interval,
		Resync:          resync,
	}
}
