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
)

func (r *Repository) persistNormalized(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, push coreingest.Push) error {
	if push.Kind == coreingest.PushFull {
		for _, facet := range []string{"varbit", "varp", "skill"} {
			if err := q.DeletePlayerStateFacet(ctx, storedb.DeletePlayerStateFacetParams{AccountID: accountID, Facet: facet}); err != nil {
				return fmt.Errorf("replace %s state: %w", facet, err)
			}
		}
	}
	if err := persistVars(ctx, q, accountID, snapshotID, "varbit", push.Data.Varbits, push.SentAt); err != nil {
		return err
	}
	if err := persistVars(ctx, q, accountID, snapshotID, "varp", push.Data.Varps, push.SentAt); err != nil {
		return err
	}
	if err := persistSkills(ctx, q, accountID, snapshotID, push); err != nil {
		return err
	}
	if err := r.persistCollectionLog(ctx, q, accountID, snapshotID, push); err != nil {
		return err
	}
	return persistItems(ctx, q, accountID, snapshotID, push)
}

func persistVars(ctx context.Context, q *storedb.Queries, accountID, snapshotID, kind string, values *coreingest.IntMap, observedAt time.Time) error {
	if values == nil {
		return nil
	}
	rows := make([]storedb.InsertSnapshotVarsParams, 0, len(*values))
	for rawID, value := range *values {
		id, err := parseInt32(rawID)
		if err != nil || value < math.MinInt32 || value > math.MaxInt32 {
			return fmt.Errorf("%w: data.%s[%q] is outside PostgreSQL integer range", ErrInvalidPayload, kind, rawID)
		}
		rows = append(rows, storedb.InsertSnapshotVarsParams{SnapshotID: snapshotID, Kind: kind, VarID: id, Value: int32(value)})
		if err := upsertFact(ctx, q, accountID, snapshotID, kind, rawID, map[string]int64{"value": value}, observedAt); err != nil {
			return err
		}
	}
	if len(rows) > 0 {
		if _, err := q.InsertSnapshotVars(ctx, rows); err != nil {
			return fmt.Errorf("insert normalized %s rows: %w", kind, err)
		}
	}
	return nil
}

func persistSkills(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, push coreingest.Push) error {
	keys := map[string]struct{}{}
	for _, values := range []*coreingest.IntMap{push.Data.Levels, push.Data.XP, push.Data.Boosted} {
		if values != nil {
			for key := range *values {
				keys[key] = struct{}{}
			}
		}
	}
	rows := make([]storedb.InsertSnapshotSkillsParams, 0, len(keys))
	for skill := range keys {
		value := map[string]int64{}
		level, hasLevel := mapValue(push.Data.Levels, skill)
		xp, hasXP := mapValue(push.Data.XP, skill)
		boosted, hasBoosted := mapValue(push.Data.Boosted, skill)
		if hasLevel {
			if level < math.MinInt32 || level > math.MaxInt32 {
				return fmt.Errorf("%w: level[%q] is outside PostgreSQL integer range", ErrInvalidPayload, skill)
			}
			value["level"] = level
		}
		if hasXP {
			value["xp"] = xp
		}
		if hasBoosted {
			if boosted < math.MinInt32 || boosted > math.MaxInt32 {
				return fmt.Errorf("%w: boosted[%q] is outside PostgreSQL integer range", ErrInvalidPayload, skill)
			}
			value["boosted"] = boosted
		}
		if hasLevel {
			rows = append(rows, storedb.InsertSnapshotSkillsParams{
				SnapshotID: snapshotID,
				Skill:      skill,
				Level:      int32(level),
				Xp:         pgtype.Int8{Int64: xp, Valid: hasXP},
				Boosted:    pgtype.Int4{Int32: int32(boosted), Valid: hasBoosted},
			})
		}
		rawValue, _ := json.Marshal(value)
		if err := q.UpsertPlayerStateJSONPatch(ctx, storedb.UpsertPlayerStateJSONPatchParams{
			AccountID: accountID, Facet: "skill", Key: skill, Value: rawValue,
			Source: "plugin", SnapshotID: snapshotID, ObservedAt: timestamptz(push.SentAt),
		}); err != nil {
			return fmt.Errorf("merge skill %q: %w", skill, err)
		}
	}
	if len(rows) > 0 {
		if _, err := q.InsertSnapshotSkills(ctx, rows); err != nil {
			return fmt.Errorf("insert normalized skills: %w", err)
		}
	}
	return nil
}

func (r *Repository) persistCollectionLog(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, push coreingest.Push) error {
	if push.Data.CollectionLogSlots == nil {
		return nil
	}
	// collectionLogSlots is always a complete bitset, including on delta pushes.
	if err := q.DeletePlayerStateFacet(ctx, storedb.DeletePlayerStateFacetParams{AccountID: accountID, Facet: "collection_log"}); err != nil {
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
		if err := q.InsertSnapshotCollectionLogItem(ctx, storedb.InsertSnapshotCollectionLogItemParams{
			SnapshotID: snapshotID, ItemID: int32(itemID), Qty: 1,
		}); err != nil {
			return fmt.Errorf("insert collection log item %d: %w", itemID, err)
		}
		if err := upsertFact(ctx, q, accountID, snapshotID, "collection_log", strconv.FormatInt(itemID, 10), map[string]int{"qty": 1}, push.SentAt); err != nil {
			return err
		}
	}
	return nil
}

func persistItems(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, push coreingest.Push) error {
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
		if err := persistItemObservation(ctx, q, accountID, snapshotID, container.name, container.obs, push.Kind); err != nil {
			return err
		}
	}
	return nil
}

func persistItemObservation(ctx context.Context, q *storedb.Queries, accountID, snapshotID, container string, obs *coreingest.ItemObservation, kind coreingest.PushKind) error {
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
			itemID, err := parseInt32(rawID)
			if err != nil || qty < 0 || qty > math.MaxInt32 {
				return fmt.Errorf("%w: %s item %q is outside PostgreSQL integer range", ErrInvalidPayload, container, rawID)
			}
			if err := q.InsertSnapshotItem(ctx, storedb.InsertSnapshotItemParams{
				SnapshotID: snapshotID, Container: container, ItemID: itemID, Qty: int32(qty),
			}); err != nil {
				return fmt.Errorf("insert normalized %s item %s: %w", container, rawID, err)
			}
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
		if err := q.DeletePlayerStateItemsByContainer(ctx, storedb.DeletePlayerStateItemsByContainerParams{
			AccountID: accountID, Container: container,
		}); err != nil {
			return fmt.Errorf("replace %s state: %w", container, err)
		}
	}
	for rawID, qty := range *obs.Quantities {
		key := container + ":" + rawID
		if qty == 0 {
			if err := q.DeletePlayerStateFact(ctx, storedb.DeletePlayerStateFactParams{AccountID: accountID, Facet: "item", Key: key}); err != nil {
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
		if err := upsertFact(ctx, q, accountID, snapshotID, "item", key, value, obs.ObservedAt); err != nil {
			return err
		}
	}
	return nil
}

func upsertFact(ctx context.Context, q *storedb.Queries, accountID, snapshotID, facet, key string, value any, observedAt time.Time) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := q.UpsertPlayerStateFact(ctx, storedb.UpsertPlayerStateFactParams{
		AccountID: accountID, Facet: facet, Key: key, Value: raw,
		Source: "plugin", SnapshotID: snapshotID, ObservedAt: timestamptz(observedAt),
	}); err != nil {
		return fmt.Errorf("merge %s %q: %w", facet, key, err)
	}
	return nil
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
