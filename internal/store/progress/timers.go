package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// TimerView is one recurring clock of the account (decision 31 of doc 01):
// a Time Tracking tab of the plugin with its state and, while growing or
// done, when it is ready (epoch seconds of the game machine's clock).
type TimerView struct {
	Key     string `json:"key"`
	Label   string `json:"label,omitempty"`
	State   string `json:"state"`
	ReadyAt *int64 `json:"ready_at,omitempty"`
	FactSource
}

// TimersView is GET /api/account/timers: the Agora's Recorrentes panel.
type TimersView struct {
	Timers []TimerView `json:"timers"`
}

// Timers reads the account's clocks; empty until the plugin sends them.
func (r *Repository) Timers(ctx context.Context, accountID string) (TimersView, error) {
	if _, err := r.queries.GetAccount(ctx, accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimersView{}, ErrAccountNotFound
		}
		return TimersView{}, fmt.Errorf("get account: %w", err)
	}
	rows, err := r.queries.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: coreingest.FacetTimer})
	if err != nil {
		return TimersView{}, fmt.Errorf("list timers: %w", err)
	}
	out := TimersView{Timers: []TimerView{}}
	for _, row := range rows {
		var v struct {
			State   string `json:"state"`
			ReadyAt *int64 `json:"ready_at"`
			Label   string `json:"label"`
		}
		if json.Unmarshal(row.Value, &v) != nil {
			continue
		}
		out.Timers = append(out.Timers, TimerView{
			Key: row.Key, Label: v.Label, State: v.State, ReadyAt: v.ReadyAt,
			FactSource: FactSource{Source: row.Source, ObservedAt: row.ObservedAt.Time},
		})
	}
	return out, nil
}
