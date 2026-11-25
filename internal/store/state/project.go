package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FelipeMalacarne/chronicle/internal/core/facts"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
)

// quietFacets are projected but never logged as changed: the position and the
// timers move with every push, and no goal reads them from an event (the job
// tracker evaluates a task's position when its run ends and on its own tick).
var quietFacets = map[string]bool{"position": true, "timer": true}

// maxChangedKeys bounds the keys a fact.changed payload lists per facet.
const maxChangedKeys = 100

// Project rewrites player_state for the given facets of one account as the
// merge of their observations (docs/10 section 5, decision E1). Call it in the
// writer's transaction, after the account is locked and the observations are
// written: player_state is never written any other way. When a fact changed
// it appends a fact.changed event in the same transaction (docs/12 section
// 13.4); the writer wakes the log's consumers after the commit.
func Project(ctx context.Context, q *storedb.Queries, accountID string, facets ...string) error {
	seen := map[string]bool{}
	changed := map[string][]string{}
	truncated := false
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
		if !quietFacets[facet] {
			keys, err := changedKeys(ctx, q, accountID, facet, merged)
			if err != nil {
				return err
			}
			if len(keys) > maxChangedKeys {
				keys, truncated = keys[:maxChangedKeys], true
			}
			if len(keys) > 0 {
				changed[facet] = keys
			}
		}
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
	if len(changed) == 0 {
		return nil
	}
	_, err := eventlog.Append(ctx, q, accountID, eventlog.TypeFactChanged, "", "", map[string]any{"facts": changed, "truncated": truncated})
	return err
}

// changedKeys lists the keys of a facet whose fact the merge adds, removes or
// changes (its value or its source) against the stored projection.
func changedKeys(ctx context.Context, q *storedb.Queries, accountID, facet string, merged []facts.Fact) ([]string, error) {
	stored, err := q.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: facet})
	if err != nil {
		return nil, fmt.Errorf("list %s facts: %w", facet, err)
	}
	have := make(map[string]storedb.PlayerState, len(stored))
	for _, row := range stored {
		have[row.Key] = row
	}
	var keys []string
	for _, f := range merged {
		old, ok := have[f.Key]
		if !ok || old.Source != f.Source || !sameJSON(old.Value, f.Value) {
			keys = append(keys, f.Key)
		}
		delete(have, f.Key)
	}
	for key := range have {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
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
// writing (docs/10 section 6, `chronicle progress check`). It returns one line
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
		return bytes.Equal(a, b)
	}
	ca, _ := json.Marshal(x)
	cb, _ := json.Marshal(y)
	return bytes.Equal(ca, cb)
}
