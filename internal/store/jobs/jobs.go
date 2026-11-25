// Package jobs stores the jobs of docs/12 section 13 (migration 0031) and
// moves them through the state machine of internal/core/job. Every change
// writes its event in the same transaction (internal/store/eventlog) and wakes
// the log's consumers after the commit. Only Succeed ends a job as proven, and
// it takes the proof the rule engine's evaluation gave.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
)

// Errors returned to the callers.
var (
	// ErrNotFound: no job with that id in the account.
	ErrNotFound = errors.New("jobs: not found")
	// ErrState: the job is not in the state the change needs (another writer
	// moved it first, or the move is not in the state machine).
	ErrState = errors.New("jobs: the job is not in a state that allows it")
	// ErrNoProof: Succeed without the proof Result.Prove gives.
	ErrNoProof = errors.New("jobs: a job succeeds only on a proof of its goal")
)

// Repository reads and writes jobs.
type Repository struct {
	pool      *pgxpool.Pool
	queries   *storedb.Queries
	publisher events.Publisher
	// Now is the clock of started_at and finished_at; nil is time.Now.
	Now func() time.Time
}

// New returns a repository over pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool)}
}

// WithPublisher wakes the event log's consumers after every commit.
func (r *Repository) WithPublisher(p events.Publisher) *Repository {
	r.publisher = p
	return r
}

func (r *Repository) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Wake tells the log's consumers there is something new for an account.
func Wake(p events.Publisher, account string) {
	if p != nil {
		p.Publish(events.Event{Type: events.TypeLog, Account: account})
	}
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

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "job_" + hex.EncodeToString(b)
}

// Insert writes a new job (in the queue) and its job.queued event through q,
// which is bound to the caller's transaction, and returns its id. The caller
// wakes the consumers after the commit.
func Insert(ctx context.Context, q *storedb.Queries, j job.Job) (string, error) {
	j.ID = newID()
	var metric json.RawMessage
	if j.Metric != nil {
		metric, _ = json.Marshal(j.Metric)
	}
	err := q.InsertJob(ctx, storedb.InsertJobParams{
		ID: j.ID, AccountID: j.Account, Kind: string(j.Kind), Spec: j.Spec, Goal: j.Goal.JSON(), Metric: metric,
		StallAfterS: int32(j.StallAfter / time.Second), //nolint:gosec // bounded by job.New
		Source:      string(j.Source), PlanID: text(j.Plan), TaskKey: text(j.TaskKey),
	})
	if err != nil {
		return "", fmt.Errorf("insert job: %w", err)
	}
	if _, err := eventlog.Append(ctx, q, j.Account, eventlog.TypeJobQueued, j.ID, j.ID, jobPayload(j, "")); err != nil {
		return "", err
	}
	return j.ID, nil
}

// Create queues a job.
func (r *Repository) Create(ctx context.Context, j job.Job) (job.Job, error) {
	var id string
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		var err error
		id, err = Insert(ctx, q, j)
		return err
	})
	if err != nil {
		return job.Job{}, err
	}
	Wake(r.publisher, j.Account)
	return r.Get(ctx, j.Account, id)
}

// Get reads a job of the account.
func (r *Repository) Get(ctx context.Context, account, id string) (job.Job, error) {
	row, err := r.queries.GetJob(ctx, storedb.GetJobParams{ID: id, AccountID: account})
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, ErrNotFound
	}
	if err != nil {
		return job.Job{}, fmt.Errorf("read job: %w", err)
	}
	return toJob(row)
}

// List returns the account's jobs, newest first; state "" takes every state.
func (r *Repository) List(ctx context.Context, account, state string, limit int) ([]job.Job, error) {
	rows, err := r.queries.ListJobs(ctx, storedb.ListJobsParams{
		AccountID: account, State: text(state), Limit: int32(limit), //nolint:gosec // a small page size
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return toJobs(rows)
}

// Active returns the account's jobs that can still change, oldest first.
func (r *Repository) Active(ctx context.Context, account string) ([]job.Job, error) {
	rows, err := r.queries.ListActiveJobs(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("list active jobs: %w", err)
	}
	return toJobs(rows)
}

// ActiveAccounts lists the accounts with a running or stalled job.
func (r *Repository) ActiveAccounts(ctx context.Context) ([]string, error) {
	out, err := r.queries.ListActiveJobAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts with jobs: %w", err)
	}
	return out, nil
}

// Due returns the running jobs whose deadline passed.
func (r *Repository) Due(ctx context.Context, now time.Time) ([]job.Job, error) {
	rows, err := r.queries.ListDueJobs(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list due jobs: %w", err)
	}
	return toJobs(rows)
}

// QueuedWithActiveRun returns the queued jobs whose run is going: the ones
// whose run.started the tracker did not act on.
func (r *Repository) QueuedWithActiveRun(ctx context.Context) ([]job.Job, error) {
	rows, err := r.queries.ListQueuedJobsWithActiveRun(ctx)
	if err != nil {
		return nil, fmt.Errorf("list queued jobs with a run: %w", err)
	}
	return toJobs(rows)
}

// StalledBefore returns the stalled jobs with no run going whose last run
// ended before the cutoff.
func (r *Repository) StalledBefore(ctx context.Context, cutoff time.Time) ([]job.Job, error) {
	rows, err := r.queries.ListStalledJobsPastCutoff(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list stalled jobs: %w", err)
	}
	return toJobs(rows)
}

// LastRunState is the state of the job's newest run; false when it has none.
func (r *Repository) LastRunState(ctx context.Context, id string) (string, bool, error) {
	state, err := r.queries.LastRunStateOfJob(ctx, text(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read last run: %w", err)
	}
	return state, true, nil
}

// NextDeadline is when the first running job stalls; false when none can.
func (r *Repository) NextDeadline(ctx context.Context) (time.Time, bool, error) {
	at, err := r.queries.NextJobDeadline(ctx)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("next deadline: %w", err)
	}
	return at.Time, at.Valid, nil
}

// ProvenSales returns the task keys of a plan whose Grand Exchange sale was
// proven, with the job that proved it (plan.Input.SalesProven): the newest
// job of each task that was not canceled, when it succeeded.
func (r *Repository) ProvenSales(ctx context.Context, account, planID string) (map[string]string, error) {
	rows, err := r.queries.LatestPlanJobs(ctx, storedb.LatestPlanJobsParams{AccountID: account, PlanID: text(planID)})
	if err != nil {
		return nil, fmt.Errorf("read sale jobs: %w", err)
	}
	out := map[string]string{}
	for _, row := range rows {
		if job.State(row.State) == job.Succeeded {
			out[row.TaskKey.String] = row.ID
		}
	}
	return out, nil
}

// RunToStop is a run still going whose job succeeded.
type RunToStop struct {
	RunUID  string
	Account string
	JobID   string
	Goal    job.Goal
}

// RunsToStop lists the runs still going whose job the tracker proved.
func (r *Repository) RunsToStop(ctx context.Context) ([]RunToStop, error) {
	rows, err := r.queries.ListRunsToStopForSucceededJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list runs to stop: %w", err)
	}
	out := make([]RunToStop, 0, len(rows))
	for _, row := range rows {
		g, err := job.ParseGoal(row.Goal)
		if err != nil {
			continue // a goal the code no longer reads: nothing to tell the agent
		}
		out = append(out, RunToStop{RunUID: row.RunUid, Account: row.AccountID, JobID: row.JobID, Goal: g})
	}
	return out, nil
}

// ActiveRuns lists the uids of a job's runs still going.
func (r *Repository) ActiveRuns(ctx context.Context, id string) ([]string, error) {
	out, err := r.queries.ListActiveRunUIDsOfJob(ctx, text(id))
	if err != nil {
		return nil, fmt.Errorf("list active runs: %w", err)
	}
	return out, nil
}

// change is one move of the state machine.
type change struct {
	to       job.State
	reason   string
	progress *job.Progress
	deadline *time.Time
	event    string
}

// Start moves a queued job, or a stalled one that got a new run, to running;
// progress is the metric's value as the run begins (nil when unread) and
// deadline when the job stalls if nothing changes.
func (r *Repository) Start(ctx context.Context, id, reason string, progress *job.Progress, deadline *time.Time) (job.Job, error) {
	return r.move(ctx, id, change{to: job.Running, reason: reason, progress: progress, deadline: deadline, event: eventlog.TypeJobStarted})
}

// Stall moves a running job to stalled.
func (r *Repository) Stall(ctx context.Context, id, reason string) (job.Job, error) {
	return r.move(ctx, id, change{to: job.Stalled, reason: reason, event: eventlog.TypeJobStalled})
}

// Fail moves a running or stalled job to failed.
func (r *Repository) Fail(ctx context.Context, id, reason string) (job.Job, error) {
	return r.move(ctx, id, change{to: job.Failed, reason: reason, event: eventlog.TypeJobFailed})
}

// Cancel moves an active job of the account to canceled.
func (r *Repository) Cancel(ctx context.Context, account, id, reason string) (job.Job, error) {
	if _, err := r.Get(ctx, account, id); err != nil {
		return job.Job{}, err
	}
	return r.move(ctx, id, change{to: job.Canceled, reason: reason, event: eventlog.TypeJobCanceled})
}

// Succeed ends a running or stalled job as proven. It takes the proof the
// goal's evaluation gave (job.Result.Prove): nothing else makes a job succeed.
func (r *Repository) Succeed(ctx context.Context, id string, proof job.Proof) (job.Job, error) {
	if !proof.Valid() {
		return job.Job{}, ErrNoProof
	}
	reason := "goal met"
	if e := proof.Evidence(); e != "" {
		reason += " (" + e + ")"
	}
	return r.move(ctx, id, change{to: job.Succeeded, reason: reason, event: eventlog.TypeJobSucceeded})
}

// Progress records a running job's new progress and moves its deadline.
func (r *Repository) Progress(ctx context.Context, id string, p job.Progress, deadline *time.Time) error {
	var account string
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		row, err := q.GetJobForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock job: %w", err)
		}
		if job.State(row.State) != job.Running {
			return ErrState
		}
		raw, _ := json.Marshal(p)
		if _, werr := q.UpdateJobProgress(ctx, storedb.UpdateJobProgressParams{ID: id, Progress: raw, DeadlineAt: timestamptz(deadline)}); werr != nil {
			return fmt.Errorf("write progress: %w", werr)
		}
		account = row.AccountID
		_, err = eventlog.Append(ctx, q, row.AccountID, eventlog.TypeJobProgressed, id, id, map[string]any{"job": id, "progress": p})
		return err
	})
	if err != nil {
		return err
	}
	Wake(r.publisher, account)
	return nil
}

// move applies one change in a transaction: the state machine's check, the
// update and the event.
func (r *Repository) move(ctx context.Context, id string, c change) (job.Job, error) {
	var out job.Job
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		row, err := q.GetJobForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock job: %w", err)
		}
		from := job.State(row.State)
		if cerr := job.Check(from, c.to); cerr != nil {
			return fmt.Errorf("%w: %w", ErrState, cerr)
		}
		now := r.now()
		p := storedb.TransitionJobParams{
			ToState: string(c.to), Reason: c.reason, ID: id, FromState: string(from),
			DeadlineAt: timestamptz(c.deadline),
		}
		if c.progress != nil {
			p.Progress, _ = json.Marshal(c.progress)
		}
		if c.to == job.Running {
			p.StartedAt = timestamptz(&now)
		}
		if c.to.Final() {
			p.FinishedAt = timestamptz(&now)
		}
		n, err := q.TransitionJob(ctx, p)
		if err != nil {
			return fmt.Errorf("move job: %w", err)
		}
		if n == 0 {
			return ErrState
		}
		moved, err := q.GetJobForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("read job: %w", err)
		}
		if out, err = toJob(moved); err != nil {
			return err
		}
		_, err = eventlog.Append(ctx, q, row.AccountID, c.event, id, id, jobPayload(out, string(from)))
		return err
	})
	if err != nil {
		return job.Job{}, err
	}
	Wake(r.publisher, out.Account)
	return out, nil
}

// jobPayload is the payload of a job's events: the job, its state, why and
// its progress.
func jobPayload(j job.Job, from string) map[string]any {
	p := map[string]any{"job": j.ID, "kind": j.Kind, "state": j.State, "reason": j.Reason, "progress": j.Progress}
	if from != "" {
		p["from"] = from
	}
	return p
}

// toJobs skips a row the code can no longer read (a goal whose rule type was
// removed, say), so one such job never stops the others from being tracked.
func toJobs(rows []storedb.Job) ([]job.Job, error) {
	out := make([]job.Job, 0, len(rows))
	for _, row := range rows {
		j, err := toJob(row)
		if err != nil {
			slog.Warn("jobs: skipping an unreadable job", "account", row.AccountID, "err", err)
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func toJob(row storedb.Job) (job.Job, error) {
	goal, err := job.ParseGoal(row.Goal)
	if err != nil {
		return job.Job{}, fmt.Errorf("job %s: %w", row.ID, err)
	}
	j := job.Job{
		ID: row.ID, Account: row.AccountID, Kind: job.Kind(row.Kind), Spec: row.Spec, Goal: goal,
		State: job.State(row.State), Reason: row.Reason, Source: job.Source(row.Source),
		StallAfter: time.Duration(row.StallAfterS) * time.Second, Plan: row.PlanID.String, TaskKey: row.TaskKey.String,
		CreatedAt: row.CreatedAt.Time.UTC(), StartedAt: timePtr(row.StartedAt), FinishedAt: timePtr(row.FinishedAt),
		Deadline: timePtr(row.DeadlineAt),
	}
	if len(row.Metric) > 0 {
		var m job.Metric
		if err := json.Unmarshal(row.Metric, &m); err != nil {
			return job.Job{}, fmt.Errorf("job %s metric: %w", row.ID, err)
		}
		j.Metric = &m
	}
	if len(row.Progress) > 0 {
		var p job.Progress
		if err := json.Unmarshal(row.Progress, &p); err != nil {
			return job.Job{}, fmt.Errorf("job %s progress: %w", row.ID, err)
		}
		j.Progress = &p
	}
	return j, nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func timestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}
