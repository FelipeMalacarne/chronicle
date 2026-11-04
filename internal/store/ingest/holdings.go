package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// ItemHoldings returns the merged item containers of an account for the
// readiness evaluator. Containers the plugin never reported are absent; item
// facts that do not belong to the container's current state and live epoch
// are dropped, so a leftover from an earlier session never proves presence.
func (r *Repository) ItemHoldings(ctx context.Context, accountID string) (rules.ItemHoldings, error) {
	if _, err := r.queries.GetAccount(ctx, accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	rows, err := r.queries.ListItemContainerFreshness(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list item container freshness: %w", err)
	}
	facts, err := r.queries.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: "item"})
	if err != nil {
		return nil, fmt.Errorf("list item facts: %w", err)
	}
	return mergeHoldings(rows, facts), nil
}

type itemFact struct {
	Qty            int64                `json:"qty"`
	ContainerState coreingest.ItemState `json:"container_state"`
	LiveEpoch      string               `json:"live_epoch"`
}

func mergeHoldings(rows []storedb.ListItemContainerFreshnessRow, facts []storedb.PlayerState) rules.ItemHoldings {
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
