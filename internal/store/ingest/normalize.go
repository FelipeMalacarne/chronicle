package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/state"
)

// facts accumulates the snapshot_facts rows of one push; they are copied in
// one batch after the player_state merge.
type facts []storedb.InsertSnapshotFactsParams

func (f *facts) add(snapshotID, facet, key string, value json.RawMessage) {
	*f = append(*f, storedb.InsertSnapshotFactsParams{SnapshotID: snapshotID, Facet: facet, Key: key, Value: value})
}

func (r *Repository) persistNormalized(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, push coreingest.Push) error {
	if push.Kind == coreingest.PushFull {
		// A full push carries complete var and level maps, so every fact
		// derived from them is replaced. Quests are replaced only when the
		// push has data.quest: a plugin before 1.1.0 does not send it.
		replaced := []string{"varbit", "varp", "skill", coreingest.FacetDiary}
		if push.Data.Quests != nil {
			replaced = append(replaced, coreingest.FacetQuest)
		}
		for _, facet := range replaced {
			if err := q.DeleteObservationFacet(ctx, storedb.DeleteObservationFacetParams{AccountID: accountID, Facet: facet, Source: pluginSource}); err != nil {
				return fmt.Errorf("replace %s state: %w", facet, err)
			}
		}
	}
	var rows facts
	if err := persistVars(ctx, q, &rows, accountID, snapshotID, "varbit", push.Data.Varbits, push.SentAt); err != nil {
		return err
	}
	if err := persistVars(ctx, q, &rows, accountID, snapshotID, "varp", push.Data.Varps, push.SentAt); err != nil {
		return err
	}
	if err := persistDerived(ctx, q, &rows, accountID, snapshotID, coreingest.DiaryFacts(r.definition.Catalog, push.Data.Varbits, push.Data.Varps), push.SentAt); err != nil {
		return err
	}
	if push.Data.Quests != nil {
		// Names outside the catalog are dropped (never guessed); the raw
		// snapshot keeps them for a later re-normalization.
		quests, _ := coreingest.QuestFacts(r.definition.Catalog, *push.Data.Quests)
		if err := persistDerived(ctx, q, &rows, accountID, snapshotID, quests, push.SentAt); err != nil {
			return err
		}
	}
	if err := persistSkills(ctx, q, &rows, accountID, snapshotID, push); err != nil {
		return err
	}
	if err := r.persistCollectionLog(ctx, q, &rows, accountID, snapshotID, push); err != nil {
		return err
	}
	if err := persistItems(ctx, q, &rows, accountID, snapshotID, push); err != nil {
		return err
	}
	if len(rows) > 0 {
		if _, err := q.InsertSnapshotFacts(ctx, rows); err != nil {
			return fmt.Errorf("insert snapshot facts: %w", err)
		}
	}
	return state.Project(ctx, q, accountID, touchedFacets(push)...)
}

// pluginSource is the source of every observation the ingest writes.
const pluginSource = "plugin"

// touchedFacets lists the facets a push may have changed, which the
// projection rewrites (docs/10 section 5).
func touchedFacets(push coreingest.Push) []string {
	var out []string
	d := push.Data
	if push.Kind == coreingest.PushFull {
		out = append(out, "varbit", "varp", "skill", coreingest.FacetDiary)
	}
	if d.Varbits != nil {
		out = append(out, "varbit", coreingest.FacetDiary)
	}
	if d.Varps != nil {
		out = append(out, "varp", coreingest.FacetDiary)
	}
	if d.Quests != nil {
		out = append(out, coreingest.FacetQuest)
	}
	if d.Levels != nil || d.XP != nil || d.Boosted != nil {
		out = append(out, "skill")
	}
	if d.CollectionLogSlots != nil {
		out = append(out, "collection_log")
	}
	if d.Items != nil {
		out = append(out, "item")
	}
	return out
}

func persistVars(ctx context.Context, q *storedb.Queries, rows *facts, accountID, snapshotID, kind string, values *coreingest.IntMap, observedAt time.Time) error {
	if values == nil {
		return nil
	}
	for rawID, value := range *values {
		if _, err := parseInt32(rawID); err != nil || value < math.MinInt32 || value > math.MaxInt32 {
			return fmt.Errorf("%w: data.%s[%q] is outside PostgreSQL integer range", ErrInvalidPayload, kind, rawID)
		}
		raw, err := upsertFact(ctx, q, accountID, snapshotID, kind, rawID, map[string]int64{"value": value}, observedAt)
		if err != nil {
			return err
		}
		rows.add(snapshotID, kind, rawID, raw)
	}
	return nil
}

// persistDerived merges facts the core derived from the push (quests resolved
// to catalog names, diary tiers from the reference varbits).
func persistDerived(ctx context.Context, q *storedb.Queries, rows *facts, accountID, snapshotID string, derived []coreingest.Fact, observedAt time.Time) error {
	for _, f := range derived {
		raw, err := upsertFact(ctx, q, accountID, snapshotID, f.Facet, f.Key, f.Value, observedAt)
		if err != nil {
			return err
		}
		rows.add(snapshotID, f.Facet, f.Key, raw)
	}
	return nil
}

func persistSkills(ctx context.Context, q *storedb.Queries, rows *facts, accountID, snapshotID string, push coreingest.Push) error {
	keys := map[string]struct{}{}
	for _, values := range []*coreingest.IntMap{push.Data.Levels, push.Data.XP, push.Data.Boosted} {
		if values != nil {
			for key := range *values {
				keys[key] = struct{}{}
			}
		}
	}
	for skill := range keys {
		value := map[string]int64{}
		if level, ok := mapValue(push.Data.Levels, skill); ok {
			if level < math.MinInt32 || level > math.MaxInt32 {
				return fmt.Errorf("%w: level[%q] is outside PostgreSQL integer range", ErrInvalidPayload, skill)
			}
			value["level"] = level
		}
		if xp, ok := mapValue(push.Data.XP, skill); ok {
			value["xp"] = xp
		}
		if boosted, ok := mapValue(push.Data.Boosted, skill); ok {
			if boosted < math.MinInt32 || boosted > math.MaxInt32 {
				return fmt.Errorf("%w: boosted[%q] is outside PostgreSQL integer range", ErrInvalidPayload, skill)
			}
			value["boosted"] = boosted
		}
		rawValue, _ := json.Marshal(value)
		if err := q.PatchObservation(ctx, storedb.PatchObservationParams{
			AccountID: accountID, Facet: "skill", Key: skill, Value: rawValue,
			Source: pluginSource, SnapshotID: snapshotID, ObservedAt: timestamptz(push.SentAt),
		}); err != nil {
			return fmt.Errorf("merge skill %q: %w", skill, err)
		}
		rows.add(snapshotID, "skill", skill, rawValue)
	}
	return nil
}

func (r *Repository) persistCollectionLog(ctx context.Context, q *storedb.Queries, rows *facts, accountID, snapshotID string, push coreingest.Push) error {
	if push.Data.CollectionLogSlots == nil {
		return nil
	}
	// collectionLogSlots is always a complete bitset, including on delta pushes.
	if err := q.DeleteObservationFacet(ctx, storedb.DeleteObservationFacetParams{AccountID: accountID, Facet: "collection_log", Source: pluginSource}); err != nil {
		return fmt.Errorf("replace collection log state: %w", err)
	}
	bits, err := base64.StdEncoding.DecodeString(*push.Data.CollectionLogSlots)
	if err != nil {
		bits, err = base64.RawStdEncoding.DecodeString(*push.Data.CollectionLogSlots)
	}
	if err != nil {
		return fmt.Errorf("%w: collectionLogSlots is not base64", ErrInvalidPayload)
	}
	for index, itemID := range r.definition.Collections {
		if index/8 >= len(bits) || bits[index/8]&(1<<uint(index%8)) == 0 {
			continue
		}
		if itemID < 1 || itemID > math.MaxInt32 {
			return fmt.Errorf("%w: collection item id %d is outside PostgreSQL integer range", ErrInvalidPayload, itemID)
		}
		key := strconv.FormatInt(itemID, 10)
		raw, err := upsertFact(ctx, q, accountID, snapshotID, "collection_log", key, map[string]int{"qty": 1}, push.SentAt)
		if err != nil {
			return err
		}
		rows.add(snapshotID, "collection_log", key, raw)
	}
	return nil
}

func persistItems(ctx context.Context, q *storedb.Queries, rows *facts, accountID, snapshotID string, push coreingest.Push) error {
	if push.Data.Items == nil {
		return nil
	}
	containers := []struct {
		name string
		obs  *coreingest.ItemObservation
	}{
		{name: "bank", obs: push.Data.Items.Bank},
		{name: "inventory", obs: push.Data.Items.Inventory},
		{name: "equipment", obs: push.Data.Items.Equipment},
	}
	for _, container := range containers {
		if container.obs == nil {
			continue
		}
		if err := persistItemObservation(ctx, q, rows, accountID, snapshotID, container.name, container.obs, push.Kind); err != nil {
			return err
		}
	}
	return nil
}

func persistItemObservation(ctx context.Context, q *storedb.Queries, rows *facts, accountID, snapshotID, container string, obs *coreingest.ItemObservation, kind coreingest.PushKind) error {
	revision := pgtype.Int8{}
	if obs.Revision != nil {
		revision = pgtype.Int8{Int64: *obs.Revision, Valid: true}
	}
	liveEpoch := pgtype.Text{String: obs.LiveEpoch, Valid: obs.LiveEpoch != ""}
	if err := q.InsertSnapshotItemContainer(ctx, storedb.InsertSnapshotItemContainerParams{
		SnapshotID: snapshotID, Container: container, State: string(obs.State),
		ObservedAt: timestamptz(obs.ObservedAt), Revision: revision, LiveEpoch: liveEpoch,
	}); err != nil {
		return fmt.Errorf("insert %s container observation: %w", container, err)
	}
	if obs.Quantities != nil {
		for rawID, qty := range *obs.Quantities {
			if _, err := parseInt32(rawID); err != nil || qty < 0 || qty > math.MaxInt32 {
				return fmt.Errorf("%w: %s item %q is outside PostgreSQL integer range", ErrInvalidPayload, container, rawID)
			}
			// The snapshot keeps the observation as sent, tombstones (0)
			// included; the container provenance is snapshot_item_containers.
			raw, _ := json.Marshal(map[string]int64{"qty": qty})
			rows.add(snapshotID, "item", container+":"+rawID, raw)
		}
	}

	_, err := q.UpsertIngestContainerFreshness(ctx, storedb.UpsertIngestContainerFreshnessParams{
		AccountID: accountID, Container: container, State: string(obs.State),
		ObservedAt: timestamptz(obs.ObservedAt), Revision: revision, LiveEpoch: liveEpoch,
		SnapshotID: snapshotID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("merge %s freshness: %w", container, err)
	}
	if obs.State == coreingest.ItemUnavailable {
		return nil
	}
	if kind == coreingest.PushFull {
		if err := q.DeleteObservationItemsByContainer(ctx, storedb.DeleteObservationItemsByContainerParams{
			AccountID: accountID, Source: pluginSource, Container: container,
		}); err != nil {
			return fmt.Errorf("replace %s state: %w", container, err)
		}
	}
	for rawID, qty := range *obs.Quantities {
		key := container + ":" + rawID
		if qty == 0 {
			if err := q.DeleteObservation(ctx, storedb.DeleteObservationParams{AccountID: accountID, Facet: "item", Key: key, Source: pluginSource}); err != nil {
				return fmt.Errorf("delete %s tombstone: %w", key, err)
			}
			continue
		}
		value := map[string]any{
			"qty": qty, "container_state": obs.State, "revision": *obs.Revision,
		}
		if obs.State == coreingest.ItemLive {
			value["live_epoch"] = obs.LiveEpoch
		}
		if _, err := upsertFact(ctx, q, accountID, snapshotID, "item", key, value, obs.ObservedAt); err != nil {
			return err
		}
	}
	return nil
}

// upsertFact records one plugin observation (player_state is its projection)
// and returns its JSON value for the snapshot_facts row.
func upsertFact(ctx context.Context, q *storedb.Queries, accountID, snapshotID, facet, key string, value any, observedAt time.Time) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if err := q.UpsertObservation(ctx, storedb.UpsertObservationParams{
		AccountID: accountID, Facet: facet, Key: key, Value: raw,
		Source: pluginSource, SnapshotID: snapshotID, ObservedAt: timestamptz(observedAt),
	}); err != nil {
		return nil, fmt.Errorf("merge %s %q: %w", facet, key, err)
	}
	return raw, nil
}

func mapValue(values *coreingest.IntMap, key string) (int64, bool) {
	if values == nil {
		return 0, false
	}
	value, ok := (*values)[key]
	return value, ok
}

func parseInt32(value string) (int32, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err
}
