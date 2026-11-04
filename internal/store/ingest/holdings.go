package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/state"
)

// ItemHoldings returns the merged item containers of an account for the
// readiness evaluator (state.MergeHoldings).
func (r *Repository) ItemHoldings(ctx context.Context, accountID string) (rules.ItemHoldings, error) {
	if _, err := r.queries.GetAccount(ctx, accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	rows, err := r.queries.ListItemContainerFreshness(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list item container freshness: %w", err)
	}
	facts, err := r.queries.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: "item"})
	if err != nil {
		return nil, fmt.Errorf("list item facts: %w", err)
	}
	return state.MergeHoldings(rows, facts), nil
}
