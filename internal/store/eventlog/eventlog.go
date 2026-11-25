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
	"github.com/jackc/pgx/v5/pgtype"
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
	ID      int64  `json:"id"`
	Account string `json:"account"`
	Type    string `json:"type"`
	Subject string `json:"subject,omitempty"`
	// JobID is the job the event is about, when one is.
	JobID   string          `json:"job_id,omitempty"`
	Payload json.RawMessage `json:"payload"`
	At      time.Time       `json:"at"`
}

// Append writes an event through q, which must be bound to the transaction
// of the change that causes it. jobID is the job it is about, "" for none.
//
// Ids come out of a sequence in the order the inserts run, not the order the
// transactions commit, so a reader that moves a cursor over ids could pass an
// event that commits late. The writer therefore holds the log's shared
// advisory lock from the insert to its commit, and Repository.After takes it
// exclusively before it reads: it never reads while an event is uncommitted.
func Append(ctx context.Context, q *storedb.Queries, account, typ, subject, jobID string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode %s payload: %w", typ, err)
	}
	if string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	if err = q.LockEventLogShared(ctx); err != nil {
		return Event{}, fmt.Errorf("lock the event log: %w", err)
	}
	row, err := q.InsertEvent(ctx, storedb.InsertEventParams{
		AccountID: account, Type: typ, Subject: subject, JobID: pgtype.Text{String: jobID, Valid: jobID != ""}, Payload: raw,
	})
	if err != nil {
		return Event{}, fmt.Errorf("append %s: %w", typ, err)
	}
	return Event{ID: row.ID, Account: account, Type: typ, Subject: subject, JobID: jobID, Payload: raw, At: row.At.Time.UTC()}, nil
}

// Repository reads the log and keeps the consumers' cursors.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

// ErrBusy: writers kept the log's lock for the whole wait; the reader asks
// again at its next pass.
var ErrBusy = errors.New("eventlog: a writer holds the log")

const (
	lockTries = 50
	lockWait  = 20 * time.Millisecond
)

// Write appends an event that no other change causes (the tracker's
// membership.low), as its own statement.
func (r *Repository) Write(ctx context.Context, account, typ, subject string, payload any) (Event, error) {
	var ev Event
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		var err error
		ev, err = Append(ctx, q, account, typ, subject, "", payload)
		return err
	})
	return ev, err
}

func (r *Repository) inTx(ctx context.Context, fn func(q *storedb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(r.queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// After returns up to limit events with an id greater than after, oldest
// first, read while no writer holds an uncommitted event: every id below the
// last one returned is in the result or never will be. It takes the lock with
// try-lock, never queued behind writers, which could deadlock one that waits
// on another writer's row.
func (r *Repository) After(ctx context.Context, after int64, limit int) ([]Event, error) {
	var rows []storedb.Event
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		for try := 0; ; try++ {
			locked, err := q.TryLockEventLog(ctx)
			if err != nil {
				return fmt.Errorf("lock the event log: %w", err)
			}
			if locked {
				break
			}
			if try == lockTries {
				return ErrBusy
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(lockWait):
			}
		}
		var err error
		rows, err = q.ListEventsAfter(ctx, storedb.ListEventsAfterParams{ID: after, Limit: int32(limit)}) //nolint:gosec // a small batch size
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fromRows(rows), nil
}

// OfJob returns a job's events and its runs', oldest first.
func (r *Repository) OfJob(ctx context.Context, account, jobID string, limit int) ([]Event, error) {
	rows, err := r.queries.ListJobEvents(ctx, storedb.ListJobEventsParams{AccountID: account, JobID: pgtype.Text{String: jobID, Valid: true}, Limit: int32(limit)}) //nolint:gosec // a small page size
	if err != nil {
		return nil, fmt.Errorf("list job events: %w", err)
	}
	return fromRows(rows), nil
}

// LastOf is the account's newest event of a type; false when it has none.
func (r *Repository) LastOf(ctx context.Context, account, typ string) (Event, bool, error) {
	row, err := r.queries.LastEventOfType(ctx, storedb.LastEventOfTypeParams{AccountID: account, Type: typ})
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, fmt.Errorf("read last %s: %w", typ, err)
	}
	return fromRows([]storedb.Event{row})[0], true, nil
}

// Cursor is the id of the last event the consumer handled; 0 for one that
// never ran.
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

// PruneFactChanged deletes the fact.changed events older than before that
// every consumer's cursor has passed; it returns how many.
func (r *Repository) PruneFactChanged(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.queries.PruneFactChangedEvents(ctx, pgtype.Timestamptz{Time: before, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("prune fact.changed: %w", err)
	}
	return n, nil
}

func fromRows(rows []storedb.Event) []Event {
	out := make([]Event, len(rows))
	for i, e := range rows {
		out[i] = Event{ID: e.ID, Account: e.AccountID, Type: e.Type, Subject: e.Subject, JobID: e.JobID.String, Payload: e.Payload, At: e.At.Time.UTC()}
	}
	return out
}
