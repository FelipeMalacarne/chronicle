// Package eventlog is the durable event log (migration 0031, docs/12 section
// 13.4): every change the server's pieces react to is appended here, in the
// transaction of the change that causes it, and each consumer reads from its
// own cursor. An event says "changed"; the consumer reads the state. Append
// is the only writer of the events table.
package eventlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// Event types (docs/12 section 13.4).
const (
	TypeFactChanged   = "fact.changed"
	TypeRunStarted    = "run.started"
	TypeRunEnded      = "run.ended"
	TypeJobQueued     = "job.queued"
	TypeJobStarted    = "job.started"
	TypeJobProgressed = "job.progressed"
	TypeJobStalled    = "job.stalled"
	TypeJobSucceeded  = "job.succeeded"
	TypeJobFailed     = "job.failed"
	TypeJobCanceled   = "job.canceled"
	TypeMembershipLow = "membership.low"
)

// Event is one row of the log.
type Event struct {
	ID      int64           `json:"id"`
	Account string          `json:"account"`
	Type    string          `json:"type"`
	Subject string          `json:"subject,omitempty"`
	Payload json.RawMessage `json:"payload"`
	At      time.Time       `json:"at"`
}

// Append writes an event through q, which must be bound to the transaction
// of the change that causes it.
func Append(ctx context.Context, q *storedb.Queries, account, typ, subject string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode %s payload: %w", typ, err)
	}
	if string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	row, err := q.InsertEvent(ctx, storedb.InsertEventParams{AccountID: account, Type: typ, Subject: subject, Payload: raw})
	if err != nil {
		return Event{}, fmt.Errorf("append %s: %w", typ, err)
	}
	return Event{ID: row.ID, Account: account, Type: typ, Subject: subject, Payload: raw, At: row.At.Time.UTC()}, nil
}

// Repository reads the log and keeps the consumers' cursors.
type Repository struct {
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: storedb.New(pool)} }

// Write appends an event that no other change causes (the tracker's
// membership.low), as its own statement.
func (r *Repository) Write(ctx context.Context, account, typ, subject string, payload any) (Event, error) {
	return Append(ctx, r.queries, account, typ, subject, payload)
}

// After returns up to limit events with an id greater than after, oldest first.
func (r *Repository) After(ctx context.Context, after int64, limit int) ([]Event, error) {
	rows, err := r.queries.ListEventsAfter(ctx, storedb.ListEventsAfterParams{ID: after, Limit: int32(limit)}) //nolint:gosec // a small batch size
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return fromRows(rows), nil
}

// OfJob returns a job's events and its runs', oldest first.
func (r *Repository) OfJob(ctx context.Context, account, jobID string, limit int) ([]Event, error) {
	rows, err := r.queries.ListJobEvents(ctx, storedb.ListJobEventsParams{AccountID: account, JobID: jobID, MaxRows: int32(limit)}) //nolint:gosec // a small page size
	if err != nil {
		return nil, fmt.Errorf("list job events: %w", err)
	}
	return fromRows(rows), nil
}

// Cursor is the last event the consumer handled; 0 for one that never ran.
func (r *Repository) Cursor(ctx context.Context, consumer string) (int64, error) {
	id, err := r.queries.GetEventCursor(ctx, consumer)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read cursor %s: %w", consumer, err)
	}
	return id, nil
}

// SetCursor records that the consumer handled everything up to id. A cursor
// never moves back.
func (r *Repository) SetCursor(ctx context.Context, consumer string, id int64) error {
	if err := r.queries.SetEventCursor(ctx, storedb.SetEventCursorParams{Consumer: consumer, LastID: id}); err != nil {
		return fmt.Errorf("write cursor %s: %w", consumer, err)
	}
	return nil
}

func fromRows(rows []storedb.Event) []Event {
	out := make([]Event, len(rows))
	for i, e := range rows {
		out[i] = Event{ID: e.ID, Account: e.AccountID, Type: e.Type, Subject: e.Subject, Payload: e.Payload, At: e.At.Time.UTC()}
	}
	return out
}
