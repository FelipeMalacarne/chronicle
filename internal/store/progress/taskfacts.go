package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/state"
)

// TaskFacts reads what the panel's assisted mode evaluates the cursor
// step's tasks on (docs/10 phase 2): the account's mode, its merged facts
// and the character's last position, when the plugin sends it.
func (r *Repository) TaskFacts(ctx context.Context, accountID string) (panel.Facts, error) {
	acc, err := r.queries.GetAccount(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return panel.Facts{}, ErrAccountNotFound
	}
	if err != nil {
		return panel.Facts{}, fmt.Errorf("get account: %w", err)
	}
	ps, _, err := state.Load(ctx, r.queries, accountID, r.catalog)
	if err != nil {
		return panel.Facts{}, err
	}
	out := panel.Facts{Mode: acc.Mode, State: &ps}
	rows, err := r.queries.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: coreingest.FacetPosition})
	if err != nil {
		return panel.Facts{}, fmt.Errorf("list position: %w", err)
	}
	for _, row := range rows {
		var p PositionView
		if json.Unmarshal(row.Value, &p) == nil {
			out.Position = &panel.Position{X: p.X, Y: p.Y, Plane: p.Plane, ObservedAt: row.ObservedAt.Time}
		}
	}
	return out, nil
}
