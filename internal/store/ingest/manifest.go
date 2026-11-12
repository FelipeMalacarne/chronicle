package ingest

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
)

// Definition is the token-independent part of an ingest manifest.
type Definition struct {
	Version     int64
	Varbits     []int64
	Varps       []int64
	Collections []int64
	// OnlyItemIDs stays empty in the generated manifest: the plugin then sends
	// whole containers (already double opt-in: token scope and client toggle).
	// An allowlist built from content rules alone would drop the items of
	// sidecar objectives, loadouts, noted forms, any_of alternatives and
	// families, which posse and readiness need. A minimized allowlist needs
	// all of those and is an owner decision (docs/04 section 6.4).
	OnlyItemIDs     []int64
	IntervalSeconds int
	// Catalog resolves data.quest names and derives the diary facet from the
	// reference varbits. Without it the manifest does not offer quests.
	Catalog *entities.Registry
}

// BuildDefinition derives the v1 manifest from the reference vars of the
// catalog (content/reference/varbits.json: diary tiers, quest points and the
// counts of the account screen; the proof of every unlock in unlocks.json and
// of every combat achievement; docs/09 section 3.2) plus every
// var and item the loaded guide rules name. The plugin reads whatever ids the
// manifest lists, so a new proof needs only catalog work, never a plugin change.
// The version is a stable positive 31-bit truncation of the canonical
// definition, which also fits the existing PostgreSQL INTEGER column.
func BuildDefinition(c *content.Content, reg *entities.Registry) Definition {
	varbits := map[int64]struct{}{}
	varps := map[int64]struct{}{}
	collections := map[int64]struct{}{}
	if reg != nil {
		tests := reg.ProofVars()
		for _, v := range reg.Vars() {
			tests = append(tests, entities.VarTest{Var: v.Var, ID: v.ID})
		}
		for _, c := range reg.CombatAchievements() {
			if c.Proof != nil {
				tests = append(tests, *c.Proof)
			}
		}
		for _, t := range tests {
			switch t.Var {
			case entities.Varbit:
				varbits[int64(t.ID)] = struct{}{}
			case entities.Varp:
				varps[int64(t.ID)] = struct{}{}
			}
		}
	}
	if c != nil {
		for _, guide := range c.Guides {
			for _, section := range guide.Sections {
				for _, step := range section.Steps {
					for _, rule := range step.CompleteWhen {
						collectRule(rule, varbits, varps, collections)
					}
					for _, rule := range step.Prerequisites {
						collectRule(rule, varbits, varps, collections)
					}
				}
			}
		}
	}
	d := Definition{
		Varbits:         sortedIDs(varbits),
		Varps:           sortedIDs(varps),
		Collections:     sortedIDs(collections),
		IntervalSeconds: 10,
		Catalog:         reg,
	}
	canonical, _ := json.Marshal(struct {
		Varbits     []int64 `json:"varbits"`
		Varps       []int64 `json:"varps"`
		Collections []int64 `json:"collections"`
		OnlyItemIDs []int64 `json:"only_item_ids"`
		Quests      bool    `json:"quests"`
		// The clocks and the position, offered since 2026-11-10.
		Timers bool `json:"timers_position"`
	}{d.Varbits, d.Varps, d.Collections, d.OnlyItemIDs, reg != nil, true})
	hash := sha256.Sum256(canonical)
	d.Version = int64(binary.BigEndian.Uint32(hash[:4]) & math.MaxInt32)
	if d.Version == 0 {
		d.Version = 1
	}
	return d
}

func collectRule(rule content.Rule, varbits, varps, collections map[int64]struct{}) {
	switch rule := rule.(type) {
	case content.AllRule:
		for _, child := range rule.All {
			collectRule(child, varbits, varps, collections)
		}
	case content.AnyRule:
		for _, child := range rule.Any {
			collectRule(child, varbits, varps, collections)
		}
	case content.NotRule:
		collectRule(rule.Not, varbits, varps, collections)
	case content.VarbitRule:
		varbits[int64(rule.ID)] = struct{}{}
	case content.VarpRule:
		varps[int64(rule.ID)] = struct{}{}
	case content.CollectionLogItemRule:
		if rule.ItemID != nil {
			collections[int64(*rule.ItemID)] = struct{}{}
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
		Quests:          d.Catalog != nil,
		Timers:          true,
		Position:        true,
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
