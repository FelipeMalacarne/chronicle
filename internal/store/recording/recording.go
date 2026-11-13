// Package recording reads and deletes an account's demonstration recording
// (docs/10 section 11, phase 2): the events the Chronicle Link plugin
// recorded while the owner played with "Record demonstration" on. The ingest
// (internal/store/ingest) is the only writer; this package serves the
// owner's GET and DELETE /api/account/recording.
package recording

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	corerecording "github.com/FelipeMalacarne/chronicle/internal/core/recording"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// ErrNotFound: the account does not exist.
var ErrNotFound = errors.New("recording: account not found")

// Repository reads and deletes recorded events.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

// Events returns the account's events recorded from since (inclusive) to
// until (exclusive), in the order they happened, at most limit of them.
func (r *Repository) Events(ctx context.Context, account string, since, until time.Time, limit int) ([]corerecording.Event, error) {
	if _, err := r.queries.GetAccount(ctx, account); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("read account: %w", err)
	}
	rows, err := r.queries.ListRecordedEvents(ctx, storedb.ListRecordedEventsParams{
		AccountID: account, Since: ts(since), Until: ts(until), MaxRows: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded events: %w", err)
	}
	out := make([]corerecording.Event, 0, len(rows))
	for _, raw := range rows {
		var e corerecording.Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("decode recorded event: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}

// Delete removes the account's events: every one, or those of one recording
// when recordingID is not empty. It returns how many were removed.
func (r *Repository) Delete(ctx context.Context, account, recordingID string) (int64, error) {
	if _, err := r.queries.GetAccount(ctx, account); errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, fmt.Errorf("read account: %w", err)
	}
	n, err := r.queries.DeleteRecordedEvents(ctx, storedb.DeleteRecordedEventsParams{
		AccountID: account, Recording: pgtype.Text{String: recordingID, Valid: recordingID != ""},
	})
	if err != nil {
		return 0, fmt.Errorf("delete recorded events: %w", err)
	}
	return n, nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
