// Package rhythm reads an account's rhythm (internal/core/rhythm; the v7's
// slice V5) from the snapshots that already exist: the current facts, the
// XP at the start of the day and of the session, the last hour of XP
// points, the GP items' observations and the session's item observations.
// Read-only, every query bounded in time.
package rhythm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	corerhythm "github.com/FelipeMalacarne/chronicle/internal/core/rhythm"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/state"
)

// ErrNotFound: the account does not exist.
var ErrNotFound = errors.New("rhythm: account not found")

const (
	// sessionLookback bounds the search for the latest session's start.
	sessionLookback = 24 * time.Hour
	// xpLookback bounds the search for a skill's XP at an instant: every
	// sync (the hiscores every 30 minutes, a plugin login) observes all
	// skills, so a skill not seen in this window did not change.
	xpLookback = 48 * time.Hour
)

// Repository reads the rhythm.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
	catalog *entities.Registry
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool, catalog *entities.Registry) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), catalog: catalog}
}

// Rhythm is the account's rhythm at now, with today starting at since.
func (r *Repository) Rhythm(ctx context.Context, account string, now, since time.Time) (corerhythm.Rhythm, error) {
	var out corerhythm.Rhythm
	err := r.readTx(ctx, func(q *storedb.Queries) error {
		if _, err := q.GetAccount(ctx, account); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return fmt.Errorf("read account: %w", err)
		}
		in, err := r.input(ctx, q, account, now, since)
		if err != nil {
			return err
		}
		out = corerhythm.Compute(in)
		r.name(out.Session)
		return nil
	})
	return out, err
}

func (r *Repository) input(ctx context.Context, q *storedb.Queries, account string, now, since time.Time) (corerhythm.Input, error) {
	ps, _, err := state.Load(ctx, q, account, r.catalog)
	if err != nil {
		return corerhythm.Input{}, fmt.Errorf("load player state: %w", err)
	}
	in := corerhythm.Input{Now: now, Since: since, Current: map[string]int{}, GPNow: corerhythm.GPOf(ps.Items), Base: r.base}
	for skill, s := range ps.Skills {
		in.Current[skill] = int(s.XP)
	}
	sess, err := q.SessionStart(ctx, storedb.SessionStartParams{
		AccountID: account, FromAt: ts(now.Add(-sessionLookback)), GapS: int32(corerhythm.SessionGap / time.Second),
	})
	if err != nil {
		return in, fmt.Errorf("session: %w", err)
	}
	if sess.StartedAt.Valid && sess.LastAt.Valid && now.Sub(sess.LastAt.Time) <= corerhythm.SessionGap {
		start := sess.StartedAt.Time.UTC()
		in.SessionStart = &start
	}
	if in.AtSince, err = xpAt(ctx, q, account, since); err != nil {
		return in, err
	}
	if in.SessionStart != nil {
		if in.AtSession, err = xpAt(ctx, q, account, *in.SessionStart); err != nil {
			return in, err
		}
		if in.Items, err = itemObservations(ctx, q, account, *in.SessionStart); err != nil {
			return in, err
		}
	}
	points, err := q.SkillPoints(ctx, storedb.SkillPointsParams{AccountID: account, FromAt: ts(now.Add(-corerhythm.RateWindow))})
	if err != nil {
		return in, fmt.Errorf("xp points: %w", err)
	}
	for _, p := range points {
		in.Recent = append(in.Recent, corerhythm.Point{At: p.FetchedAt.Time.UTC(), Skill: p.Skill, XP: int(p.Xp)})
	}
	coins, err := q.CoinObservations(ctx, storedb.CoinObservationsParams{AccountID: account, BeforeAt: ts(since)})
	if err != nil {
		return in, fmt.Errorf("coin observations: %w", err)
	}
	for _, c := range coins {
		in.Coins = append(in.Coins, corerhythm.CoinObs{
			At: c.FetchedAt.Time.UTC(), Full: c.Full, Container: c.Container, Coins: listed(c.Coins), Platinum: listed(c.Platinum),
		})
	}
	return in, nil
}

// xpAt is each skill's XP at an instant: its last observation at or before
// it (within xpLookback), else its first after it.
func xpAt(ctx context.Context, q *storedb.Queries, account string, at time.Time) (map[string]int, error) {
	out := map[string]int{}
	before, err := q.SkillXPLastBefore(ctx, storedb.SkillXPLastBeforeParams{AccountID: account, FromAt: ts(at.Add(-xpLookback)), ToAt: ts(at)})
	if err != nil {
		return nil, fmt.Errorf("xp before: %w", err)
	}
	for _, row := range before {
		out[row.Skill] = int(row.Xp)
	}
	after, err := q.SkillXPFirstAfter(ctx, storedb.SkillXPFirstAfterParams{AccountID: account, FromAt: ts(at)})
	if err != nil {
		return nil, fmt.Errorf("xp after: %w", err)
	}
	for _, row := range after {
		if _, ok := out[row.Skill]; !ok {
			out[row.Skill] = int(row.Xp)
		}
	}
	return out, nil
}

// itemObservations are the item containers' observations from the last
// full one of each at or before at.
func itemObservations(ctx context.Context, q *storedb.Queries, account string, at time.Time) ([]corerhythm.ItemObs, error) {
	rows, err := q.ItemObservations(ctx, storedb.ItemObservationsParams{AccountID: account, BeforeAt: ts(at)})
	if err != nil {
		return nil, fmt.Errorf("item observations: %w", err)
	}
	out := make([]corerhythm.ItemObs, 0, len(rows))
	for _, row := range rows {
		var raw map[string]int64
		if err := json.Unmarshal(row.Items, &raw); err != nil {
			return nil, fmt.Errorf("item observations: %w", err)
		}
		items := make(map[int]int64, len(raw))
		for k, v := range raw {
			id, err := strconv.Atoi(k)
			if err != nil {
				continue // the ingest only stores numeric ids
			}
			items[id] = v
		}
		out = append(out, corerhythm.ItemObs{At: row.FetchedAt.Time.UTC(), Full: row.Full, Container: row.Container, Items: items})
	}
	return out, nil
}

// base adds a noted form up under its unnoted id.
func (r *Repository) base(id int) int {
	if it, ok := r.catalog.Item(id); ok {
		return it.BaseID
	}
	return id
}

// name fills in the session's item names from the catalog.
func (r *Repository) name(s *corerhythm.Session) {
	if s == nil {
		return
	}
	for i := range s.Gained {
		if it, ok := r.catalog.Item(s.Gained[i].Item); ok {
			s.Gained[i].Name = it.Name
		}
	}
	for i := range s.RunningOut {
		if it, ok := r.catalog.Item(s.RunningOut[i].Item); ok {
			s.RunningOut[i].Name = it.Name
		}
	}
}

// readTx runs fn in one read-only snapshot of the database.
func (r *Repository) readTx(ctx context.Context, fn func(q *storedb.Queries) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return fn(r.queries.WithTx(tx))
}

// listed is a coin column (-1: the snapshot does not list the item).
func listed(v int64) *int64 {
	if v < 0 {
		return nil
	}
	return &v
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
