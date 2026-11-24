// Package prices stores the Grand Exchange prices (docs/12 section 6): the
// last price of each item, written by the serve-side scheduler and read by
// whatever prices a typed offer. They belong to no account.
package prices

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/ge"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// Repository reads and writes the prices.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

// Replace upserts the batch in one transaction. Items the batch lacks stay,
// and a price never goes back in time: a side whose trade is older than the
// stored one keeps the stored one. A limit or membership the batch does not
// know keeps the stored value.
func (r *Repository) Replace(ctx context.Context, batch []ge.Price) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	for _, p := range batch {
		if p.ItemID <= 0 || (p.High <= 0 && p.Low <= 0) {
			return fmt.Errorf("prices: item %d has no price", p.ItemID)
		}
		params := storedb.UpsertItemPriceParams{
			ItemID:    int32(p.ItemID), //nolint:gosec // item ids are far below int32
			Source:    p.Source,
			FetchedAt: ts(p.FetchedAt),
		}
		if p.High > 0 {
			params.High, params.HighAt = pgtype.Int8{Int64: p.High, Valid: true}, ts(p.HighAt)
		}
		if p.Low > 0 {
			params.Low, params.LowAt = pgtype.Int8{Int64: p.Low, Valid: true}, ts(p.LowAt)
		}
		if p.Limit != nil {
			params.BuyLimit = pgtype.Int4{Int32: int32(*p.Limit), Valid: true} //nolint:gosec // GE limits are small
		}
		if p.Members != nil {
			params.Members = pgtype.Bool{Bool: *p.Members, Valid: true}
		}
		if err := q.UpsertItemPrice(ctx, params); err != nil {
			return fmt.Errorf("upsert item %d: %w", p.ItemID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Get returns the stored price of each id that has one; an id without a
// price is absent from the map.
func (r *Repository) Get(ctx context.Context, ids ...int) (map[int]ge.Price, error) {
	out := make(map[int]ge.Price, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	arg := make([]int32, len(ids))
	for i, id := range ids {
		arg[i] = int32(id) //nolint:gosec // item ids are far below int32
	}
	rows, err := r.queries.GetItemPrices(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("read prices: %w", err)
	}
	for _, row := range rows {
		p := ge.Price{ItemID: int(row.ItemID), Source: row.Source, FetchedAt: row.FetchedAt.Time.UTC()}
		if row.High.Valid {
			p.High, p.HighAt = row.High.Int64, row.HighAt.Time.UTC()
		}
		if row.Low.Valid {
			p.Low, p.LowAt = row.Low.Int64, row.LowAt.Time.UTC()
		}
		if row.BuyLimit.Valid {
			v := int(row.BuyLimit.Int32)
			p.Limit = &v
		}
		if row.Members.Valid {
			v := row.Members.Bool
			p.Members = &v
		}
		out[p.ItemID] = p
	}
	return out, nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
