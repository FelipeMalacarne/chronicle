// Package microbot persists the audit of the Microbot control module
// (docs/08, Segurança item 9).
package microbot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// Entry is one audited action.
type Entry struct {
	ID     int64           `json:"id"`
	At     time.Time       `json:"at"`
	Actor  string          `json:"actor"`
	Origin string          `json:"origin"`
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	// AccountID and AgentID say which account and which agent the action
	// went to (docs/11); empty for actions from before migration 0012.
	AccountID string `json:"account_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
}

// Repository writes and reads microbot_audit.
type Repository struct {
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: storedb.New(pool)}
}

// Append records one action.
func (r *Repository) Append(ctx context.Context, e Entry) (Entry, error) {
	if len(e.Params) == 0 {
		e.Params = json.RawMessage("{}")
	}
	row, err := r.queries.AppendMicrobotAudit(ctx, storedb.AppendMicrobotAuditParams{
		Actor: e.Actor, Origin: e.Origin, Action: e.Action, Params: e.Params, Ok: e.OK, Error: e.Error,
	})
	if err != nil {
		return Entry{}, fmt.Errorf("append microbot audit: %w", err)
	}
	return entryOf(row), nil
}

// List returns the latest limit entries, newest first.
func (r *Repository) List(ctx context.Context, limit int) ([]Entry, error) {
	rows, err := r.queries.ListMicrobotAudit(ctx, int32(min(max(limit, 1), 500))) //nolint:gosec // bounded above
	if err != nil {
		return nil, fmt.Errorf("list microbot audit: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for _, row := range rows {
		out = append(out, entryOf(row))
	}
	return out, nil
}

func entryOf(row storedb.MicrobotAudit) Entry {
	return Entry{
		ID: row.ID, At: row.At.Time, Actor: row.Actor, Origin: row.Origin, Action: row.Action,
		Params: row.Params, OK: row.Ok, Error: row.Error,
	}
}
