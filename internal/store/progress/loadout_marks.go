package progress

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/chronicle/internal/core/planning"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// ErrInvalidLoadoutItem: the guide, segment or item to mark does not exist, or
// the item is not a requirement of that segment.
var ErrInvalidLoadoutItem = errors.New("invalid loadout item")

// LoadoutMarkResult is what a loadout mark or unmark did.
type LoadoutMarkResult struct {
	Guide   string `json:"guide"`
	Segment string `json:"segment"`
	// Item is the requirement key (planning.RequirementKey).
	Item string `json:"item"`
	// Marked: the item is marked after the command.
	Marked bool `json:"marked"`
}

// MarkLoadoutItem records the owner's mark "I already have this item for this
// leg" (docs/09 section 5, owner decision of 2026-11-14). It is scoped to one
// (guide, segment) and satisfies the whole requirement there; another leg or
// guide that lists the same item is not marked.
//
// The mark is an input overlaid when readiness is computed, never evidence:
// item holdings, snapshots, player_state and objective_progress are untouched
// (CLAUDE.md rules 2 and 8). If the plugin later proves the item held the mark
// is harmless; if it proves the item absent the mark still wins until
// UnmarkLoadoutItem. It is not appended to mark_commands (that log replays
// step_marks); the row (note, marked_at) is its record.
//
// item must be a RequirementKey of the segment's loadout or, for a leg
// without a loadout, of its steps' requirements.
func (r *Repository) MarkLoadoutItem(ctx context.Context, accountID, guideID, segmentID, item, note string) (LoadoutMarkResult, error) {
	guideID, segmentID, item = strings.TrimSpace(guideID), strings.TrimSpace(segmentID), strings.TrimSpace(item)
	g, ok := r.content.Guide(guideID)
	if !ok {
		return LoadoutMarkResult{}, fmt.Errorf("%w: guide %q not found", ErrInvalidLoadoutItem, guideID)
	}
	seg, ok := g.Segment(segmentID)
	if !ok {
		return LoadoutMarkResult{}, fmt.Errorf("%w: segment %q not found in %s", ErrInvalidLoadoutItem, segmentID, guideID)
	}
	if !planning.SegmentRequirementKeys(seg)[item] {
		return LoadoutMarkResult{}, fmt.Errorf("%w: %q is not an item of segment %s", ErrInvalidLoadoutItem, item, segmentID)
	}
	return r.writeLoadoutMark(ctx, accountID, guideID, segmentID, item, true, func(q *storedb.Queries) error {
		if err := q.UpsertLoadoutMark(ctx, storedb.UpsertLoadoutMarkParams{
			AccountID: accountID, GuideID: guideID, SegmentID: segmentID, ItemKey: item, Note: note,
		}); err != nil {
			return fmt.Errorf("upsert loadout mark %s/%s/%s: %w", guideID, segmentID, item, err)
		}
		return nil
	})
}

// UnmarkLoadoutItem takes the owner's mark back; the item reads as the
// observed facts say again. Unmarking an item that is not marked changes
// nothing, and an item the content no longer lists can still be unmarked.
func (r *Repository) UnmarkLoadoutItem(ctx context.Context, accountID, guideID, segmentID, item string) (LoadoutMarkResult, error) {
	guideID, segmentID, item = strings.TrimSpace(guideID), strings.TrimSpace(segmentID), strings.TrimSpace(item)
	return r.writeLoadoutMark(ctx, accountID, guideID, segmentID, item, false, func(q *storedb.Queries) error {
		if _, err := q.DeleteLoadoutMark(ctx, storedb.DeleteLoadoutMarkParams{
			AccountID: accountID, GuideID: guideID, SegmentID: segmentID, ItemKey: item,
		}); err != nil {
			return fmt.Errorf("delete loadout mark %s/%s/%s: %w", guideID, segmentID, item, err)
		}
		return nil
	})
}

// writeLoadoutMark runs write in a transaction that locks the account, so it
// serializes with the account's other owner writes.
func (r *Repository) writeLoadoutMark(ctx context.Context, accountID, guideID, segmentID, item string, marked bool, write func(*storedb.Queries) error) (LoadoutMarkResult, error) {
	out := LoadoutMarkResult{Guide: guideID, Segment: segmentID, Item: item, Marked: marked}
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		if _, err := lockAccount(ctx, q, accountID); err != nil {
			return err
		}
		return write(q)
	})
	if err == nil {
		r.publishOwner(accountID, 0, guideID)
	}
	return out, err
}

// LoadoutMarks reads the account's marks on the items of its legs.
func (r *Repository) LoadoutMarks(ctx context.Context, accountID string) (planning.OwnerMarks, error) {
	rows, err := r.queries.ListLoadoutMarks(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list loadout marks: %w", err)
	}
	out := planning.OwnerMarks{}
	for _, row := range rows {
		out.Add(row.GuideID, row.SegmentID, row.ItemKey)
	}
	return out, nil
}
