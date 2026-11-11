package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
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

// PositionView is the character's last position the plugin sent (opt-in).
type PositionView struct {
	World  int `json:"world"`
	X      int `json:"x"`
	Y      int `json:"y"`
	Plane  int `json:"plane"`
	Region int `json:"region"`
	FactSource
}

// NearbyView is GET /api/guides/{guide}/nearby: the open steps of the route
// near the character. Position is nil until the plugin sends one.
type NearbyView struct {
	Position *PositionView           `json:"position"`
	Radius   int                     `json:"radius"`
	Steps    []coreprogress.NearStep `json:"steps"`
}

// Nearby lists the open steps of the guide near the account's last position.
func (r *Repository) Nearby(ctx context.Context, accountID, guideID string, limit int) (NearbyView, error) {
	out := NearbyView{Radius: coreprogress.NearRadius, Steps: []coreprogress.NearStep{}}
	g, ok := r.content.Guide(guideID)
	if !ok {
		return out, ErrGuideNotFound
	}
	board, err := r.Board(ctx, accountID, guideID)
	if err != nil {
		return out, err
	}
	rows, err := r.queries.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: coreingest.FacetPosition})
	if err != nil {
		return out, fmt.Errorf("list position: %w", err)
	}
	for _, row := range rows {
		var p PositionView
		if json.Unmarshal(row.Value, &p) != nil {
			continue
		}
		p.FactSource = FactSource{Source: row.Source, ObservedAt: row.ObservedAt.Time}
		out.Position = &p
	}
	if out.Position == nil {
		return out, nil
	}
	at := coreprogress.Point{X: out.Position.X, Y: out.Position.Y, Plane: out.Position.Plane}
	out.Steps = coreprogress.Nearby(board.Board, g, r.catalog, at, coreprogress.NearRadius, limit)
	return out, nil
}
