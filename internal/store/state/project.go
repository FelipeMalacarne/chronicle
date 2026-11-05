package state

import (
	"context"
	"encoding/json"
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

// Diff compares player_state with the merge of the observations, without
// writing (docs/10 section 6, `ironman progress check`). It returns one line
// per fact out of step; none means player_state is exactly the projection.
func Diff(ctx context.Context, q *storedb.Queries, accountID string) ([]string, error) {
	facets, err := q.ListObservedFacets(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list observed facets: %w", err)
	}
	sort.Strings(facets)
	var out []string
	for _, facet := range facets {
		rows, err := q.ListObservationsByFacet(ctx, storedb.ListObservationsByFacetParams{AccountID: accountID, Facet: facet})
		if err != nil {
			return nil, fmt.Errorf("list %s observations: %w", facet, err)
		}
		obs := make([]facts.Observation, len(rows))
		for i, r := range rows {
			obs[i] = facts.Observation{Key: r.Key, Source: r.Source, Value: r.Value, SnapshotID: r.SnapshotID, ObservedAt: r.ObservedAt.Time}
		}
		want := map[string]facts.Fact{}
		for _, f := range facts.Merge(facet, obs) {
			want[f.Key] = f
		}
		stored, err := q.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: facet})
		if err != nil {
			return nil, fmt.Errorf("list %s facts: %w", facet, err)
		}
		have := map[string]storedb.PlayerState{}
		for _, row := range stored {
			have[row.Key] = row
		}
		keys := map[string]bool{}
		for k := range want {
			keys[k] = true
		}
		for k := range have {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			w, inMerge := want[k]
			h, inTable := have[k]
			switch {
			case !inMerge:
				out = append(out, fmt.Sprintf("%s %s: in player_state, but no observation gives it", facet, k))
			case !inTable:
				out = append(out, fmt.Sprintf("%s %s: the merge gives %s, player_state has none", facet, k, w.Value))
			case !sameJSON(w.Value, h.Value) || w.Source != h.Source || w.SnapshotID != h.SnapshotID:
				out = append(out, fmt.Sprintf("%s %s: the merge gives %s from %s, player_state has %s from %s", facet, k, w.Value, w.Source, h.Value, h.Source))
			}
		}
	}
	return out, nil
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	ca, _ := json.Marshal(x)
	cb, _ := json.Marshal(y)
	return string(ca) == string(cb)
}
