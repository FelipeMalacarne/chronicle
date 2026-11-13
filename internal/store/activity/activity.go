// Package activity reads what an account did (internal/core/activity; the
// v7's Histórico and Atividade) from the snapshots that already exist: for
// now, the XP observations behind the level-ups. Read-only, every query
// bounded in time.
package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	coreactivity "github.com/FelipeMalacarne/chronicle/internal/core/activity"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// ErrNotFound: the account does not exist.
var ErrNotFound = errors.New("activity: account not found")

// BaselineLookback bounds the search for each skill's XP before the window.
// A hiscores sync observes every ranked skill and a plugin login pushes them
// all, so an account seen in that week has a baseline; a skill without one
// starts at its first observation in the window, which is never an event.
const BaselineLookback = 7 * 24 * time.Hour

// Repository reads the activity.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

// Levels is the account's level-ups and skill gains from since on.
func (r *Repository) Levels(ctx context.Context, account string, since time.Time) (coreactivity.Levels, error) {
	var out coreactivity.Levels
	err := r.readTx(ctx, func(q *storedb.Queries) error {
		if _, err := q.GetAccount(ctx, account); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return fmt.Errorf("read account: %w", err)
		}
		before, err := q.LevelXPBaseline(ctx, storedb.LevelXPBaselineParams{
			AccountID: account, LookbackAt: ts(since.Add(-BaselineLookback)), FromAt: ts(since),
		})
		if err != nil {
			return fmt.Errorf("xp baseline: %w", err)
		}
		baseline := make(map[string]int, len(before))
		for _, row := range before {
			baseline[row.Skill] = int(row.Xp)
		}
		rows, err := q.LevelXPObservations(ctx, storedb.LevelXPObservationsParams{AccountID: account, FromAt: ts(since)})
		if err != nil {
			return fmt.Errorf("xp observations: %w", err)
		}
		window := make([]coreactivity.Observation, 0, len(rows))
		for _, row := range rows {
			window = append(window, coreactivity.Observation{At: row.FetchedAt.Time.UTC(), Skill: row.Skill, XP: int(row.Xp)})
		}
		out = coreactivity.Compute(baseline, window)
		return nil
	})
	return out, err
}

// readTx runs fn in one read-only snapshot of the database, so the baseline
// and the window see the same pushes.
func (r *Repository) readTx(ctx context.Context, fn func(q *storedb.Queries) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return fn(r.queries.WithTx(tx))
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
