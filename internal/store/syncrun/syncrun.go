// Package syncrun runs the polling sources for an account (today the
// hiscores; docs/04 section 6): it records each attempt in sync_runs, stores
// the snapshot and its facts, reevaluates progress in the same transaction and
// backs off after failures. Scheduler calls it on a cadence; POST /api/sync on
// demand.
package syncrun

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// Source is the only polling source so far.
const Source = "hiscores"

// Triggers (sync_runs.trigger_kind).
const (
	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
)

// Debounce is the minimum time between two manual runs of an account.
const Debounce = 60 * time.Second

// Backoff after consecutive failures: 2^(n-1) × 5 min, at most 6 h (docs/04
// section 6.2).
const (
	backoffBase = 5 * time.Minute
	backoffMax  = 6 * time.Hour
	// notFoundAlert consecutive 404s mean a renamed or unranked account.
	notFoundAlert = 3
)

// Errors.
var (
	ErrAccountNotFound = errors.New("account not found")
	ErrNoName          = errors.New("account has no RuneScape name")
)

// TooSoonError refuses a manual run during the debounce or a backoff.
type TooSoonError struct {
	RetryAt time.Time
	Last    RunView
}

func (e *TooSoonError) Error() string {
	return fmt.Sprintf("sync too soon; retry at %s", e.RetryAt.Format(time.RFC3339))
}

// Fetcher reads the hiscores (the real client or a fake in tests).
type Fetcher interface {
	Fetch(ctx context.Context, rsn, mode string) (hiscores.Result, error)
}

// Reevaluator reevaluates progress inside the sync transaction.
type Reevaluator interface {
	Reevaluate(ctx context.Context, tx pgx.Tx, accountID, snapshotID string) (int, error)
}

// RunView is one sync_runs row.
type RunView struct {
	ID            string     `json:"id"`
	Source        string     `json:"source"`
	Trigger       string     `json:"trigger"`
	Status        string     `json:"status"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	HTTPStatus    int        `json:"http_status,omitempty"`
	Error         string     `json:"error,omitempty"`
	SnapshotID    string     `json:"snapshot_id,omitempty"`
	StepsChanged  int        `json:"steps_changed"`
	NextAllowedAt *time.Time `json:"next_allowed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Repository runs syncs over PostgreSQL.
type Repository struct {
	pool        *pgxpool.Pool
	queries     *storedb.Queries
	fetcher     Fetcher
	reevaluator Reevaluator
	now         func() time.Time
}

// New returns a repository; reevaluator may be nil (no progress).
func New(pool *pgxpool.Pool, fetcher Fetcher, reevaluator Reevaluator) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), fetcher: fetcher, reevaluator: reevaluator, now: time.Now}
}

// Runs returns the newest runs of the account.
func (r *Repository) Runs(ctx context.Context, accountID string, limit int) ([]RunView, error) {
	if _, err := r.account(ctx, accountID); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListSyncRuns(ctx, storedb.ListSyncRunsParams{AccountID: text(accountID), Source: Source, Limit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("list sync runs: %w", err)
	}
	out := make([]RunView, 0, len(rows))
	for _, row := range rows {
		out = append(out, view(row))
	}
	return out, nil
}

func (r *Repository) account(ctx context.Context, accountID string) (storedb.Account, error) {
	acc, err := r.queries.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return acc, ErrAccountNotFound
		}
		return acc, fmt.Errorf("get account: %w", err)
	}
	if !acc.Active || acc.Rsn == "" {
		return acc, ErrNoName
	}
	return acc, nil
}

// Run syncs the hiscores of one account. A failure of the source is not an
// error of Run: it is recorded in the returned run (status error or
// rate_limited, with the backoff in next_allowed_at).
func (r *Repository) Run(ctx context.Context, accountID, trigger string) (RunView, error) {
	acc, err := r.account(ctx, accountID)
	if err != nil {
		return RunView{}, err
	}
	recent, err := r.queries.ListSyncRuns(ctx, storedb.ListSyncRunsParams{AccountID: text(accountID), Source: Source, Limit: 10})
	if err != nil {
		return RunView{}, fmt.Errorf("list sync runs: %w", err)
	}
	now := r.now()
	if trigger == TriggerManual && len(recent) > 0 {
		last := recent[0]
		if retry := last.CreatedAt.Time.Add(Debounce); now.Before(retry) {
			return RunView{}, &TooSoonError{RetryAt: retry, Last: view(last)}
		}
		if last.NextAllowedAt.Valid && now.Before(last.NextAllowedAt.Time) {
			return RunView{}, &TooSoonError{RetryAt: last.NextAllowedAt.Time, Last: view(last)}
		}
	}
	run, err := r.queries.InsertSyncRun(ctx, storedb.InsertSyncRunParams{ID: newID("sync_"), AccountID: text(accountID), Source: Source, TriggerKind: trigger})
	if err != nil {
		return RunView{}, fmt.Errorf("insert sync run: %w", err)
	}
	res, err := r.fetcher.Fetch(ctx, acc.Rsn, acc.Mode)
	if err != nil {
		return r.fail(ctx, run.ID, recent, err, now)
	}
	snap, err := hiscores.Parse(res.Raw)
	if err != nil {
		return r.fail(ctx, run.ID, recent, err, now)
	}
	var out RunView
	err = pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		finished, txErr := r.store(ctx, tx, accountID, run.ID, res.Raw, hiscores.Normalize(snap), now)
		out = finished
		return txErr
	})
	if err != nil {
		return r.fail(ctx, run.ID, recent, err, now)
	}
	return out, nil
}

// store writes the snapshot and its facts unless the facts did not change,
// reevaluates and closes the run, all in tx.
func (r *Repository) store(ctx context.Context, tx pgx.Tx, accountID, runID string, raw []byte, facts []hiscores.Fact, now time.Time) (RunView, error) {
	q := r.queries.WithTx(tx)
	hash := factsHash(facts)
	_, findErr := q.GetSnapshotByHash(ctx, storedb.GetSnapshotByHashParams{AccountID: accountID, Source: Source, RawHash: hash})
	switch {
	case findErr == nil:
		row, err := q.FinishSyncRun(ctx, storedb.FinishSyncRunParams{ID: runID, Status: "unchanged", HttpStatus: pgtype.Int4{Int32: 200, Valid: true}})
		return view(row), err
	case !errors.Is(findErr, pgx.ErrNoRows):
		return RunView{}, fmt.Errorf("find snapshot: %w", findErr)
	}
	snapshotID := newID("snap_")
	if _, err := q.InsertSnapshot(ctx, storedb.InsertSnapshotParams{
		ID: snapshotID, AccountID: accountID, Source: Source, Kind: "full",
		FetchedAt: pgtype.Timestamptz{Time: now, Valid: true}, Raw: json.RawMessage(raw), RawHash: hash,
		AdapterVersion: hiscores.AdapterVersion,
	}); err != nil {
		return RunView{}, fmt.Errorf("insert snapshot: %w", err)
	}
	if err := writeFacts(ctx, q, accountID, snapshotID, facts, now); err != nil {
		return RunView{}, err
	}
	changed := 0
	if r.reevaluator != nil {
		n, err := r.reevaluator.Reevaluate(ctx, tx, accountID, snapshotID)
		if err != nil {
			return RunView{}, fmt.Errorf("reevaluate progress: %w", err)
		}
		changed = n
	}
	row, err := q.FinishSyncRun(ctx, storedb.FinishSyncRunParams{
		ID: runID, Status: "ok", HttpStatus: pgtype.Int4{Int32: 200, Valid: true},
		SnapshotID: text(snapshotID), StepsChanged: int32(changed),
	})
	if err != nil {
		return RunView{}, fmt.Errorf("finish sync run: %w", err)
	}
	return view(row), nil
}

func writeFacts(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, facts []hiscores.Fact, now time.Time) error {
	at := pgtype.Timestamptz{Time: now, Valid: true}
	var skills []storedb.InsertSnapshotSkillsParams
	var activities []storedb.InsertSnapshotActivitiesParams
	for _, f := range facts {
		value, err := json.Marshal(f.Value)
		if err != nil {
			return fmt.Errorf("encode %s %s: %w", f.Facet, f.Key, err)
		}
		switch f.Facet {
		case "skill":
			// Merge: a plugin's "boosted" survives a hiscores level/xp update.
			err = q.UpsertPlayerStateJSONPatch(ctx, storedb.UpsertPlayerStateJSONPatchParams{
				AccountID: accountID, Facet: f.Facet, Key: f.Key, Value: value, Source: Source, SnapshotID: snapshotID, ObservedAt: at,
			})
			skills = append(skills, storedb.InsertSnapshotSkillsParams{
				SnapshotID: snapshotID, Skill: f.Key, Level: int32(f.Value["level"]), Xp: pgtype.Int8{Int64: f.Value["xp"], Valid: true},
			})
		default:
			err = q.UpsertPlayerStateFact(ctx, storedb.UpsertPlayerStateFactParams{
				AccountID: accountID, Facet: f.Facet, Key: f.Key, Value: value, Source: Source, SnapshotID: snapshotID, ObservedAt: at,
			})
			activities = append(activities, storedb.InsertSnapshotActivitiesParams{
				SnapshotID: snapshotID, Activity: f.Key, Score: f.Value["score"], Rank: pgtype.Int8{Int64: f.Value["rank"], Valid: true},
			})
		}
		if err != nil {
			return fmt.Errorf("merge %s %s: %w", f.Facet, f.Key, err)
		}
	}
	if len(skills) > 0 {
		if _, err := q.InsertSnapshotSkills(ctx, skills); err != nil {
			return fmt.Errorf("insert snapshot skills: %w", err)
		}
	}
	if len(activities) > 0 {
		if _, err := q.InsertSnapshotActivities(ctx, activities); err != nil {
			return fmt.Errorf("insert snapshot activities: %w", err)
		}
	}
	return nil
}

// fail closes the run as error or rate_limited with the backoff of the
// consecutive failures (including this one).
func (r *Repository) fail(ctx context.Context, runID string, recent []storedb.SyncRun, cause error, now time.Time) (RunView, error) {
	failures, notFound := 1, 0
	if errors.Is(cause, hiscores.ErrNotFound) {
		notFound = 1
	}
	for _, run := range recent {
		if run.Status != "error" && run.Status != "rate_limited" {
			break
		}
		failures++
		if notFound > 0 && run.HttpStatus.Int32 == 404 {
			notFound++
		}
	}
	status := "error"
	if errors.Is(cause, hiscores.ErrRateLimited) {
		status = "rate_limited"
	}
	msg := cause.Error()
	if notFound >= notFoundAlert {
		msg = fmt.Sprintf("conta não encontrada no hiscores %d vezes seguidas; renomeou ou ainda não ranqueia?", notFound)
	}
	params := storedb.FinishSyncRunParams{
		ID: runID, Status: status, Error: text(msg),
		NextAllowedAt: pgtype.Timestamptz{Time: now.Add(Backoff(failures)), Valid: true},
	}
	var se *hiscores.StatusError
	if errors.As(cause, &se) {
		params.HttpStatus = pgtype.Int4{Int32: int32(se.Status), Valid: true}
	}
	row, err := r.queries.FinishSyncRun(ctx, params)
	if err != nil {
		return RunView{}, fmt.Errorf("finish failed sync run: %w", err)
	}
	return view(row), nil
}

// Backoff is the wait after n consecutive failures.
func Backoff(n int) time.Duration {
	if n < 1 {
		return 0
	}
	d := backoffBase
	for i := 1; i < n; i++ {
		d *= 2
		if d >= backoffMax {
			return backoffMax
		}
	}
	return d
}

// factsHash identifies what the snapshot says, without the ranks (they move
// as other players progress), so an unchanged account gives "unchanged".
func factsHash(facts []hiscores.Fact) string {
	lines := make([]string, 0, len(facts))
	for _, f := range facts {
		switch f.Facet {
		case "skill":
			lines = append(lines, fmt.Sprintf("%s/%s=%d/%d", f.Facet, f.Key, f.Value["level"], f.Value["xp"]))
		default:
			lines = append(lines, fmt.Sprintf("%s/%s=%d", f.Facet, f.Key, f.Value["score"]))
		}
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func view(row storedb.SyncRun) RunView {
	return RunView{
		ID: row.ID, Source: row.Source, Trigger: row.TriggerKind, Status: row.Status,
		StartedAt: timePtr(row.StartedAt), FinishedAt: timePtr(row.FinishedAt), HTTPStatus: int(row.HttpStatus.Int32),
		Error: row.Error.String, SnapshotID: row.SnapshotID.String, StepsChanged: int(row.StepsChanged),
		NextAllowedAt: timePtr(row.NextAllowedAt), CreatedAt: row.CreatedAt.Time,
	}
}

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
