// Package routines persists what the Microbot workers run by themselves
// (docs/11 section 4.4, phase F6; migrations 0014 and 0015): each account's
// triggers and recurring routines, which the agent gets on every connection,
// and the runs' history with its trail, which the agent sends as each run
// changes.
package routines

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// Errors returned to the callers.
var (
	// ErrNotFound: the account does not exist.
	ErrNotFound = errors.New("routines: account not found")
	// ErrForeignRun: the run is stored under another account.
	ErrForeignRun = errors.New("routines: the run belongs to another account")
	// ErrRoutineNotFound: no recurring routine with that id in the account.
	ErrRoutineNotFound = errors.New("routines: recurring routine not found")
	// ErrNameTaken: the account has another recurring routine of that name.
	ErrNameTaken = errors.New("routines: another recurring routine has that name")
	// ErrTooMany: the account has agent.RecurringMax recurring routines.
	ErrTooMany = fmt.Errorf("routines: at most %d recurring routines per account", agent.RecurringMax)
)

// Repository reads and writes routines, routine_runs and run_events.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

func (r *Repository) inTx(ctx context.Context, fn func(q *storedb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if fnErr := fn(r.queries.WithTx(tx)); fnErr != nil {
		return fnErr
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Triggers returns the account's triggers by event.
func (r *Repository) Triggers(ctx context.Context, account string) ([]agent.Trigger, error) {
	return listTriggers(ctx, r.queries, account)
}

// PutTrigger saves one trigger (its event's, replacing it) and returns the
// account's triggers.
func (r *Repository) PutTrigger(ctx context.Context, account string, t agent.Trigger) ([]agent.Trigger, error) {
	var out []agent.Trigger
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		if err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		if err := upsertTrigger(ctx, q, account, t); err != nil {
			return err
		}
		var err error
		out, err = listTriggers(ctx, q, account)
		return err
	})
	return out, err
}

// ImportTriggers seeds an account that has no triggers yet with ts (an
// attached agent's file from before the server kept them); it reports
// whether it did.
func (r *Repository) ImportTriggers(ctx context.Context, account string, ts []agent.Trigger) (bool, error) {
	imported := false
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		if err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		have, err := listTriggers(ctx, q, account)
		if err != nil || len(have) > 0 || len(ts) == 0 {
			return err
		}
		for _, t := range ts {
			if err := upsertTrigger(ctx, q, account, t); err != nil {
				return err
			}
		}
		imported = true
		return nil
	})
	return imported, err
}

// RecordRun stores a run as an agent reported it (agent.ValidateReport
// first): the run and the trail lines it did not have. It reports whether
// the run is new and whether it finished with this report. An older report
// (a shorter trail) changes nothing.
func (r *Repository) RecordRun(ctx context.Context, account, agentName string, run agent.Run) (newRun, finished bool, err error) {
	if verr := agent.ValidateReport(run); verr != nil {
		return false, false, verr
	}
	err = r.inTx(ctx, func(q *storedb.Queries) error {
		prev, readErr := q.GetRoutineRunForUpdate(ctx, run.UID)
		found := readErr == nil
		switch {
		case errors.Is(readErr, pgx.ErrNoRows):
		case readErr != nil:
			return fmt.Errorf("read run: %w", readErr)
		case prev.AccountID != account:
			return ErrForeignRun
		}
		var routineID pgtype.Text
		switch {
		case run.Trigger != "":
			t, trigErr := q.GetTrigger(ctx, storedb.GetTriggerParams{AccountID: account, Event: text(run.Trigger)})
			switch {
			case trigErr == nil:
				routineID = text(t.ID)
			case !errors.Is(trigErr, pgx.ErrNoRows):
				return fmt.Errorf("read trigger: %w", trigErr)
			}
		case run.Routine != "":
			// A recurring routine of this account; one deleted since leaves
			// the run without it.
			id, recErr := q.GetRecurringID(ctx, storedb.GetRecurringIDParams{ID: run.Routine, AccountID: account})
			switch {
			case recErr == nil:
				routineID = text(id)
			case !errors.Is(recErr, pgx.ErrNoRows):
				return fmt.Errorf("read recurring routine: %w", recErr)
			}
		}
		stop, _ := json.Marshal(nonNil(run.Stop))
		var from, task json.RawMessage
		if run.From != nil {
			from, _ = json.Marshal(run.From)
		}
		if run.Task != nil {
			task, _ = json.Marshal(run.Task)
		}
		n, upErr := q.UpsertRoutineRun(ctx, storedb.UpsertRoutineRunParams{
			ID: run.UID, AccountID: account, RoutineID: routineID, Agent: agentName, AgentRunID: run.ID,
			ClassName: run.ClassName, Stop: stop, Trigger: run.Trigger, State: run.State,
			StartedAt: timestamptz(&run.StartedAt), FinishedAt: timestamptz(run.FinishedAt),
			FromPos: from, Result: run.Result, Error: run.Error, TrailLen: int32(len(run.Events)), //nolint:gosec // at most 500 (ValidateReport)
			Task: task,
		})
		if upErr != nil {
			return fmt.Errorf("write run: %w", accountMissing(upErr))
		}
		if n == 0 {
			return nil // an older report
		}
		start := 0
		if found {
			start = int(prev.TrailLen)
		}
		for seq := start; seq < len(run.Events); seq++ {
			ev := run.Events[seq]
			if evErr := q.InsertRunEvent(ctx, storedb.InsertRunEventParams{
				RunID: run.UID, Seq: int32(seq), At: timestamptz(&ev.At), State: ev.State, Text: ev.Text, //nolint:gosec // at most 500
			}); evErr != nil {
				return fmt.Errorf("write trail: %w", evErr)
			}
		}
		newRun = !found
		finished = run.Final() && (!found || !final(prev.State))
		return nil
	})
	return newRun, finished, err
}

// Runs returns the account's last runs, newest first, with their trails.
func (r *Repository) Runs(ctx context.Context, account string, limit int) ([]agent.Run, error) {
	rows, err := r.queries.ListRoutineRuns(ctx, storedb.ListRoutineRunsParams{AccountID: account, Limit: int32(limit)}) //nolint:gosec // a small page size
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	events, err := r.queries.ListRunEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list trails: %w", err)
	}
	trails := map[string][]agent.RunEvent{}
	for _, ev := range events {
		trails[ev.RunID] = append(trails[ev.RunID], agent.RunEvent{At: ev.At.Time.UTC(), State: ev.State, Text: ev.Text})
	}
	out := make([]agent.Run, 0, len(rows))
	for _, row := range rows {
		run := agent.Run{
			ID: row.AgentRunID, UID: row.ID, ClassName: row.ClassName, Trigger: row.Trigger, State: row.State,
			StartedAt: row.StartedAt.Time.UTC(), Error: row.Error, Events: trails[row.ID], Stop: []string{},
		}
		if run.Events == nil {
			run.Events = []agent.RunEvent{}
		}
		if row.Trigger == "" && row.RoutineID.Valid {
			// No trigger and a routine: a recurring routine's run.
			run.Routine = row.RoutineID.String
		}
		_ = json.Unmarshal(row.Stop, &run.Stop)
		if row.FinishedAt.Valid {
			t := row.FinishedAt.Time.UTC()
			run.FinishedAt = &t
		}
		if len(row.FromPos) > 0 {
			var from agent.WalkParams
			if json.Unmarshal(row.FromPos, &from) == nil {
				run.From = &from
			}
		}
		if len(row.Result) > 0 {
			run.Result = row.Result
		}
		if len(row.Task) > 0 {
			var task agent.RunTask
			if json.Unmarshal(row.Task, &task) == nil {
				run.Task = &task
			}
		}
		out = append(out, run)
	}
	return out, nil
}

func listTriggers(ctx context.Context, q *storedb.Queries, account string) ([]agent.Trigger, error) {
	rows, err := q.ListTriggers(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	out := make([]agent.Trigger, 0, len(rows))
	for _, row := range rows {
		t := agent.Trigger{Event: row.Event.String, Enabled: row.Enabled}
		if err := json.Unmarshal(row.Spec, &t.Run); err != nil {
			return nil, fmt.Errorf("trigger %s: %w", row.Event.String, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func upsertTrigger(ctx context.Context, q *storedb.Queries, account string, t agent.Trigger) error {
	spec, err := json.Marshal(t.Run)
	if err != nil {
		return err
	}
	if err := q.UpsertTrigger(ctx, storedb.UpsertTriggerParams{
		ID: newID("rtn_"), AccountID: account, Event: text(t.Event), Enabled: t.Enabled, Spec: spec,
	}); err != nil {
		return fmt.Errorf("write trigger: %w", err)
	}
	return nil
}

// Recurring returns the account's recurring routines, the highest priority
// first, each with its last run.
func (r *Repository) Recurring(ctx context.Context, account string) ([]agent.Recurring, error) {
	return listRecurring(ctx, r.queries, account)
}

// PutRecurring saves one recurring routine (agent.ValidateRecurring first):
// a new one when rc.ID is empty, else the account's routine of that id. It
// returns the routine's id and the account's recurring routines.
func (r *Repository) PutRecurring(ctx context.Context, account string, rc agent.Recurring) (string, []agent.Recurring, error) {
	rc, err := agent.ValidateRecurring(rc)
	if err != nil {
		return "", nil, err
	}
	spec, err := json.Marshal(rc.Run)
	if err != nil {
		return "", nil, err
	}
	clock := pgtype.Text{String: rc.Clock, Valid: rc.Clock != ""}
	interval := pgtype.Int4{Int32: int32(rc.IntervalMin), Valid: rc.IntervalMin != 0} //nolint:gosec // at most a week (ValidateRecurring)
	priority := int32(rc.Priority)                                                    //nolint:gosec // 0 to 100 (ValidateRecurring)
	var out []agent.Recurring
	err = r.inTx(ctx, func(q *storedb.Queries) error {
		if lerr := lockAccount(ctx, q, account); lerr != nil {
			return lerr
		}
		if rc.ID == "" {
			n, cerr := q.CountRecurring(ctx, account)
			if cerr != nil {
				return fmt.Errorf("count recurring routines: %w", cerr)
			}
			if n >= agent.RecurringMax {
				return ErrTooMany
			}
			rc.ID = newID("rtn_")
			if ierr := q.InsertRecurring(ctx, storedb.InsertRecurringParams{
				ID: rc.ID, AccountID: account, Name: text(rc.Name), Enabled: rc.Enabled, Clock: clock,
				IntervalMin: interval, Priority: priority, Spec: spec,
			}); ierr != nil {
				return fmt.Errorf("write recurring routine: %w", nameTaken(ierr))
			}
		} else {
			n, uerr := q.UpdateRecurring(ctx, storedb.UpdateRecurringParams{
				ID: rc.ID, AccountID: account, Name: text(rc.Name), Enabled: rc.Enabled, Clock: clock,
				IntervalMin: interval, Priority: priority, Spec: spec,
			})
			if uerr != nil {
				return fmt.Errorf("write recurring routine: %w", nameTaken(uerr))
			}
			if n == 0 {
				return ErrRoutineNotFound
			}
		}
		var lerr error
		out, lerr = listRecurring(ctx, q, account)
		return lerr
	})
	if err != nil {
		return "", nil, err
	}
	return rc.ID, out, nil
}

// DeleteRecurring removes one of the account's recurring routines (its runs
// stay, without it) and returns the ones left.
func (r *Repository) DeleteRecurring(ctx context.Context, account, id string) ([]agent.Recurring, error) {
	var out []agent.Recurring
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		if err := lockAccount(ctx, q, account); err != nil {
			return err
		}
		n, err := q.DeleteRecurring(ctx, storedb.DeleteRecurringParams{ID: id, AccountID: account})
		if err != nil {
			return fmt.Errorf("delete recurring routine: %w", err)
		}
		if n == 0 {
			return ErrRoutineNotFound
		}
		out, err = listRecurring(ctx, q, account)
		return err
	})
	return out, err
}

func listRecurring(ctx context.Context, q *storedb.Queries, account string) ([]agent.Recurring, error) {
	rows, err := q.ListRecurring(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("list recurring routines: %w", err)
	}
	out := make([]agent.Recurring, 0, len(rows))
	for _, row := range rows {
		rc := agent.Recurring{
			ID: row.ID, Name: row.Name.String, Enabled: row.Enabled, Clock: row.Clock.String,
			IntervalMin: int(row.IntervalMin.Int32), Priority: int(row.Priority),
		}
		if err := json.Unmarshal(row.Spec, &rc.Run); err != nil {
			return nil, fmt.Errorf("recurring routine %s: %w", row.ID, err)
		}
		if row.UpdatedAt.Valid {
			t := row.UpdatedAt.Time.UTC()
			rc.UpdatedAt = &t
		}
		if row.LastState != "" && row.LastStartedAt.Valid {
			rc.LastRun = &agent.RecurringRun{State: row.LastState, StartedAt: row.LastStartedAt.Time.UTC()}
			if row.LastFinishedAt.Valid {
				t := row.LastFinishedAt.Time.UTC()
				rc.LastRun.FinishedAt = &t
			}
		}
		out = append(out, rc)
	}
	return out, nil
}

// nameTaken maps the unique violation of a recurring routine's name to
// ErrNameTaken.
func nameTaken(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "routines_recurring_name_idx" {
		return ErrNameTaken
	}
	return err
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func lockAccount(ctx context.Context, q *storedb.Queries, account string) error {
	_, err := q.LockAccountForWorker(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock account: %w", err)
	}
	return nil
}

// accountMissing maps the foreign key violation of an unknown account to
// ErrNotFound.
func accountMissing(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23503" && pg.ConstraintName == "routine_runs_account_id_fkey" {
		return ErrNotFound
	}
	return err
}

func final(state string) bool {
	return state == agent.RunDone || state == agent.RunFailed || state == agent.RunCanceled
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func timestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
