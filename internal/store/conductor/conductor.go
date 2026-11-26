// Package conductor stores the conductor's state per account (conductor_state,
// docs/12 section 12.5): the owner turns it on or off, internal/conduct moves
// it on the job events. An account without a row is off.
package conductor

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/conductor"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

// ErrAccountNotFound is a state for an account that does not exist.
var ErrAccountNotFound = errors.New("account not found")

// Repository reads and writes conductor_state.
type Repository struct {
	queries   *storedb.Queries
	publisher events.Publisher
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: storedb.New(pool)}
}

// WithPublisher wakes the conductor when the owner turns it on or off.
func (r *Repository) WithPublisher(p events.Publisher) *Repository {
	r.publisher = p
	return r
}

// Get returns the account's state; Off without a row.
func (r *Repository) Get(ctx context.Context, account string) (conductor.State, error) {
	row, err := r.queries.GetConductorState(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return conductor.State{Mode: conductor.Off}, nil
	}
	if err != nil {
		return conductor.State{}, fmt.Errorf("read the conductor state: %w", err)
	}
	return conductor.State{Mode: conductor.Mode(row.State), Job: row.JobID.String, Reason: row.Reason}, nil
}

// Accounts lists the accounts the conductor acts on (idle or running).
func (r *Repository) Accounts(ctx context.Context) ([]string, error) {
	out, err := r.queries.ListConductorAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the conductor's accounts: %w", err)
	}
	return out, nil
}

// TurnOn puts the account's conductor in idle, whatever it was doing (a
// blocked account starts again); TurnOff puts it off. The job it dispatched
// last is kept, for the record.
func (r *Repository) TurnOn(ctx context.Context, account string) error {
	return r.set(ctx, account, conductor.Idle)
}

// TurnOff stops the conductor for the account; a job it dispatched goes on.
func (r *Repository) TurnOff(ctx context.Context, account string) error {
	return r.set(ctx, account, conductor.Off)
}

func (r *Repository) set(ctx context.Context, account string, mode conductor.Mode) error {
	cur, err := r.Get(ctx, account)
	if err != nil {
		return err
	}
	err = r.queries.SetConductorState(ctx, storedb.SetConductorStateParams{
		AccountID: account, State: string(mode), JobID: text(cur.Job),
	})
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			return ErrAccountNotFound
		}
		return fmt.Errorf("write the conductor state: %w", err)
	}
	storejobs.Wake(r.publisher, account)
	return nil
}

// Move writes to only if the account is still at from: false when the owner
// turned it on or off in between, and the conductor's step is dropped.
func (r *Repository) Move(ctx context.Context, account string, from, to conductor.State) (bool, error) {
	n, err := r.queries.MoveConductorState(ctx, storedb.MoveConductorStateParams{
		AccountID: account, FromState: string(from.Mode), FromJobID: text(from.Job),
		State: string(to.Mode), JobID: text(to.Job), Reason: to.Reason,
	})
	if err != nil {
		return false, fmt.Errorf("move the conductor state: %w", err)
	}
	return n == 1, nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
