// Package state reads an account's merged player_state and item-container
// freshness into the engine's rules.PlayerState, with the provenance of every
// fact. It is shared by the readiness routes and the progress reevaluation.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// Load returns the merged state of an account and the provenance of each
// fact. Facets the account never reported stay nil (unknown to the engine).
func Load(ctx context.Context, q *storedb.Queries, accountID string, catalog *entities.Registry) (rules.PlayerState, map[progress.Fact]progress.FactObservation, error) {
	ps := rules.PlayerState{Catalog: catalog}
	obs := map[progress.Fact]progress.FactObservation{}
	facts, err := q.ListPlayerState(ctx, accountID)
	if err != nil {
		return ps, nil, fmt.Errorf("list player state: %w", err)
	}
	var items []storedb.PlayerState
	for _, f := range facts {
		key := progress.Fact{Facet: f.Facet, Key: f.Key}
		switch f.Facet {
		case "skill":
			var v rules.SkillState
			if json.Unmarshal(f.Value, &v) != nil {
				continue
			}
			if ps.Skills == nil {
				ps.Skills = map[string]rules.SkillState{}
			}
			ps.Skills[f.Key] = v
		case "quest":
			var v struct {
				State string `json:"state"`
			}
			if json.Unmarshal(f.Value, &v) != nil || v.State == "" {
				continue
			}
			if ps.Quests == nil {
				ps.Quests = map[string]string{}
			}
			ps.Quests[f.Key] = v.State
		case "activity":
			var v struct {
				Score int `json:"score"`
			}
			if json.Unmarshal(f.Value, &v) != nil || v.Score < 0 {
				continue
			}
			if ps.BossKC == nil {
				ps.BossKC = map[string]int{}
			}
			ps.BossKC[f.Key] = v.Score
		case "diary":
			area, tier, ok := strings.Cut(f.Key, ":")
			var v struct {
				Complete *bool `json:"complete"`
			}
			if !ok || json.Unmarshal(f.Value, &v) != nil || v.Complete == nil {
				continue
			}
			if ps.Diaries == nil {
				ps.Diaries = map[string]map[string]bool{}
			}
			if ps.Diaries[area] == nil {
				ps.Diaries[area] = map[string]bool{}
			}
			ps.Diaries[area][tier] = *v.Complete
		case "varbit", "varp":
			id, convErr := strconv.Atoi(f.Key)
			var v struct {
				Value *int64 `json:"value"`
			}
			if convErr != nil || json.Unmarshal(f.Value, &v) != nil || v.Value == nil {
				continue
			}
			target := &ps.Varbits
			if f.Facet == "varp" {
				target = &ps.Varps
			}
			if *target == nil {
				*target = map[int]int64{}
			}
			(*target)[id] = *v.Value
		case "item":
			items = append(items, f)
			continue
		default:
			continue
		}
		obs[key] = progress.FactObservation{Source: f.Source, SnapshotID: f.SnapshotID, ObservedAt: f.ObservedAt.Time}
	}
	rows, err := q.ListItemContainerFreshness(ctx, accountID)
	if err != nil {
		return ps, nil, fmt.Errorf("list item container freshness: %w", err)
	}
	if len(rows) > 0 {
		ps.Items = MergeHoldings(rows, items)
		for _, row := range rows {
			obs[progress.Fact{Facet: progress.FactItemContainer, Key: row.Container}] = progress.FactObservation{
				Source: "plugin", SnapshotID: row.SnapshotID, ObservedAt: row.ObservedAt.Time,
			}
		}
	}
	return ps, obs, nil
}

type itemFact struct {
	Qty            int64                `json:"qty"`
	ContainerState coreingest.ItemState `json:"container_state"`
	LiveEpoch      string               `json:"live_epoch"`
}

// MergeHoldings builds the item containers from their latest accepted
// observation and the item facts. Containers the plugin never reported are
// absent; item facts that do not belong to the container's current state and
// live epoch are dropped, so a leftover from an earlier session never proves
// presence.
func MergeHoldings(rows []storedb.ListItemContainerFreshnessRow, facts []storedb.PlayerState) rules.ItemHoldings {
	out := make(rules.ItemHoldings, len(rows))
	epochs := make(map[string]string, len(rows))
	for _, row := range rows {
		c := rules.ContainerItems{
			State:      coreingest.ItemState(row.State),
			ObservedAt: row.ObservedAt.Time,
			Complete:   row.HasBaseline,
			Quantities: map[int]int64{},
		}
		out[row.Container] = c
		epochs[row.Container] = row.LiveEpoch.String
	}
	for _, fact := range facts {
		container, rawID, ok := strings.Cut(fact.Key, ":")
		if !ok {
			continue
		}
		c, observed := out[container]
		if !observed || !c.Readable() {
			continue
		}
		id, err := strconv.Atoi(rawID)
		if err != nil {
			continue
		}
		var v itemFact
		if err := json.Unmarshal(fact.Value, &v); err != nil || v.Qty <= 0 {
			continue
		}
		if v.ContainerState != c.State || (c.State == coreingest.ItemLive && v.LiveEpoch != epochs[container]) {
			continue
		}
		c.Quantities[id] = v.Qty
	}
	return out
}
