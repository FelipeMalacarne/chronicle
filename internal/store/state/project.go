package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/facts"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// Project rewrites player_state for the given facets of one account as the
// merge of their observations (docs/10 section 5, decision E1). Call it in the
// writer's transaction, after the account is locked and the observations are
// written: player_state is never written any other way.
func Project(ctx context.Context, q *storedb.Queries, accountID string, facets ...string) error {
	seen := map[string]bool{}
	sort.Strings(facets)
	for _, facet := range facets {
		if seen[facet] {
			continue
		}
		seen[facet] = true
		rows, err := q.ListObservationsByFacet(ctx, storedb.ListObservationsByFacetParams{AccountID: accountID, Facet: facet})
		if err != nil {
			return fmt.Errorf("list %s observations: %w", facet, err)
		}
		obs := make([]facts.Observation, len(rows))
		for i, r := range rows {
			obs[i] = facts.Observation{Key: r.Key, Source: r.Source, Value: r.Value, SnapshotID: r.SnapshotID, ObservedAt: r.ObservedAt.Time}
		}
		merged := facts.Merge(facet, obs)
		if err := q.DeletePlayerStateFacetProjection(ctx, storedb.DeletePlayerStateFacetProjectionParams{AccountID: accountID, Facet: facet}); err != nil {
			return fmt.Errorf("clear %s projection: %w", facet, err)
		}
		if len(merged) == 0 {
			continue
		}
		out := make([]storedb.InsertPlayerStateProjectionParams, len(merged))
		for i, f := range merged {
			out[i] = storedb.InsertPlayerStateProjectionParams{
				AccountID: accountID, Facet: facet, Key: f.Key, Value: f.Value, Source: f.Source,
				SnapshotID: f.SnapshotID, ObservedAt: pgtype.Timestamptz{Time: f.ObservedAt, Valid: true},
			}
		}
		if _, err := q.InsertPlayerStateProjection(ctx, out); err != nil {
			return fmt.Errorf("write %s projection: %w", facet, err)
		}
	}
	return nil
}

// ProjectAll rewrites the whole player_state of one account (boot): every
// facet that has observations or projected rows.
func ProjectAll(ctx context.Context, q *storedb.Queries, accountID string) error {
	facets, err := q.ListObservedFacets(ctx, accountID)
	if err != nil {
		return fmt.Errorf("list observed facets: %w", err)
	}
	return Project(ctx, q, accountID, facets...)
}
