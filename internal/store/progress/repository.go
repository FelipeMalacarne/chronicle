// Package progress persists account progress over objectives (docs/09 section
// 5): it loads what internal/core/progress needs, runs it inside a
// transaction and writes the plan back. It also indexes the loaded content at
// boot and applies step id aliases.
package progress

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	"github.com/FelipeMalacarne/ironman-planner/internal/events"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/state"
)

// Errors returned to the transport.
var (
	ErrAccountNotFound = errors.New("account not found")
	ErrGuideNotFound   = errors.New("guide not found")
)

// Repository implements progress persistence over PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
	content *content.Content
	catalog *entities.Registry
	// publisher receives an event after each committed owner write; nil
	// disables it. Reevaluate does not publish: it runs inside its caller's
	// transaction, and the caller publishes after committing.
	publisher events.Publisher
	now       func() time.Time
}

// New returns a repository over the loaded content.
func New(pool *pgxpool.Pool, cnt *content.Content, catalog *entities.Registry) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), content: cnt, catalog: catalog, now: time.Now}
}

// WithPublisher makes every committed owner write (marks, cursor, active
// guide, goals) publish a progress event (GET /api/events). nil disables it.
func (r *Repository) WithPublisher(p events.Publisher) *Repository {
	r.publisher = p
	return r
}

// publishOwner signals an owner write after its commit; no guides means every
// guide (the hub fills them).
func (r *Repository) publishOwner(accountID string, stepsChanged int, guides ...string) {
	if r.publisher == nil {
		return
	}
	r.publisher.Publish(events.Event{
		Type: events.TypeProgress, Account: accountID, Source: events.SourceOwner, Guides: guides, StepsChanged: stepsChanged,
	})
}

// Reevaluate reevaluates one account inside tx (docs/09 section 5) and
// returns how many steps changed status because of the engine, in every
// guide. snapshotID is the snapshot that triggered it, or empty (boot).
func (r *Repository) Reevaluate(ctx context.Context, tx pgx.Tx, accountID, snapshotID string) (int, error) {
	n, _, err := r.ReevaluateRoute(ctx, tx, accountID, snapshotID)
	return n, err
}

// ReevaluateRoute is Reevaluate plus what changed in the account's active
// route: its cursor after the reevaluation and the steps whose status
// changed (the ingest response's route, read by the plugin panel).
func (r *Repository) ReevaluateRoute(ctx context.Context, tx pgx.Tx, accountID, snapshotID string) (int, coreingest.RouteUpdate, error) {
	q := r.queries.WithTx(tx)
	acc, err := lockAccount(ctx, q, accountID)
	if err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	ps, obs, err := state.Load(ctx, q, accountID, r.catalog)
	if err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	in := coreprogress.Input{
		Guides: r.content.Guides, Registry: r.catalog, State: ps, Observations: obs,
		Account: acc, Now: r.now(),
	}
	if in.Objectives, err = loadObjectives(ctx, q, accountID); err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	if in.Marks, err = loadMarks(ctx, q, accountID); err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	if in.LastStatus, err = loadLastStatus(ctx, q, accountID); err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	if in.Cursors, err = loadCursors(ctx, q, accountID); err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	goals, err := q.ListGoalsByAccount(ctx, accountID)
	if err != nil {
		return 0, coreingest.RouteUpdate{}, fmt.Errorf("list goals: %w", err)
	}
	for _, g := range goals {
		if g.Status == "active" {
			in.Goals = append(in.Goals, g.ObjectiveKey)
		}
	}
	plan := coreprogress.Reevaluate(in)
	if err := writePlan(ctx, q, accountID, snapshotID, in.Now, plan); err != nil {
		return 0, coreingest.RouteUpdate{}, err
	}
	var route coreingest.RouteUpdate
	if _, ok := r.content.Guide(acc.ActiveGuideID); ok {
		rc := plan.Route(acc.ActiveGuideID, in.Cursors[acc.ActiveGuideID])
		route = coreingest.RouteUpdate{Guide: rc.GuideID, Cursor: rc.Cursor.StepID, CursorCause: rc.Cursor.Cause, Changed: rc.Changed}
	}
	return plan.StepsChanged, route, nil
}

// ReevaluateAll reevaluates every active account, each in its own
// transaction (boot and content reload).
func (r *Repository) ReevaluateAll(ctx context.Context) (int, error) {
	accounts, err := r.queries.ListActiveAccounts(ctx)
	if err != nil {
		return 0, fmt.Errorf("list active accounts: %w", err)
	}
	for _, acc := range accounts {
		if err := r.inTx(ctx, func(tx pgx.Tx) error {
			_, err := r.Reevaluate(ctx, tx, acc.ID, "")
			return err
		}); err != nil {
			return 0, fmt.Errorf("reevaluate %s: %w", acc.ID, err)
		}
	}
	return len(accounts), nil
}

func (r *Repository) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin progress transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit progress transaction: %w", err)
	}
	return nil
}

func lockAccount(ctx context.Context, q *storedb.Queries, accountID string) (coreprogress.Account, error) {
	row, err := q.LockAccountProgress(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreprogress.Account{}, ErrAccountNotFound
		}
		return coreprogress.Account{}, fmt.Errorf("lock account: %w", err)
	}
	return coreprogress.Account{ActiveGuideID: row.GuideID, Layer: row.Layer, Mode: row.Mode}, nil
}

func loadObjectives(ctx context.Context, q *storedb.Queries, accountID string) (map[string]coreprogress.ObjectiveRecord, error) {
	rows, err := q.ListObjectiveProgress(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list objective progress: %w", err)
	}
	out := make(map[string]coreprogress.ObjectiveRecord, len(rows))
	for _, row := range rows {
		out[row.ObjectiveKey] = coreprogress.ObjectiveRecord{
			Exists: true, Verdict: rules.Verdict(row.Verdict), MetInGuideID: row.MetInGuideID.String,
			FirstMetAt: timePtr(row.FirstMetAt), RegressedAt: timePtr(row.RegressedAt),
		}
	}
	return out, nil
}

func loadMarks(ctx context.Context, q *storedb.Queries, accountID string) (map[coreprogress.StepRef]coreprogress.Mark, error) {
	rows, err := q.ListStepMarksByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list step marks: %w", err)
	}
	out := make(map[coreprogress.StepRef]coreprogress.Mark, len(rows))
	for _, row := range rows {
		out[coreprogress.StepRef{GuideID: row.GuideID, StepID: row.StepID}] = markOf(row)
	}
	return out, nil
}

func loadLastStatus(ctx context.Context, q *storedb.Queries, accountID string) (map[coreprogress.StepRef]coreprogress.Status, error) {
	rows, err := q.ListLastStepStatuses(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list last step statuses: %w", err)
	}
	out := make(map[coreprogress.StepRef]coreprogress.Status, len(rows))
	for _, row := range rows {
		out[coreprogress.StepRef{GuideID: row.GuideID, StepID: row.StepID}] = coreprogress.Status(row.ToStatus)
	}
	return out, nil
}

func loadCursors(ctx context.Context, q *storedb.Queries, accountID string) (map[string]coreprogress.CursorRecord, error) {
	rows, err := q.ListRouteCursors(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list route cursors: %w", err)
	}
	out := make(map[string]coreprogress.CursorRecord, len(rows))
	for _, row := range rows {
		out[row.GuideID] = coreprogress.CursorRecord{StepID: row.StepID, Cause: row.Cause}
	}
	return out, nil
}

func markOf(row storedb.StepMark) coreprogress.Mark {
	return coreprogress.Mark{Kind: coreprogress.MarkKind(row.Kind), ImpliedBy: row.ImpliedByStepID.String, Note: row.Note.String}
}

func writePlan(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, now time.Time, plan coreprogress.Plan) error {
	for _, u := range plan.Objectives {
		if !u.ReplaceEvidence {
			if err := q.SetObjectiveRegressed(ctx, storedb.SetObjectiveRegressedParams{
				AccountID: accountID, ObjectiveKey: u.Key, RegressedAt: timestamptz(u.RegressedAt),
			}); err != nil {
				return fmt.Errorf("update regression of %s: %w", u.Key, err)
			}
			continue
		}
		evidence, err := json.Marshal(u.Evidence)
		if err != nil {
			return fmt.Errorf("encode evidence of %s: %w", u.Key, err)
		}
		if err := q.UpsertObjectiveProgress(ctx, storedb.UpsertObjectiveProgressParams{
			AccountID: accountID, ObjectiveKey: u.Key, Verdict: string(u.Verdict),
			EvidenceSnapshotID: text(u.EvidenceSnapshotID), Evidence: evidence,
			FirstMetAt: timestamptz(u.FirstMetAt), MetInGuideID: text(u.MetInGuideID), RegressedAt: timestamptz(u.RegressedAt),
		}); err != nil {
			return fmt.Errorf("upsert objective %s: %w", u.Key, err)
		}
	}
	evidenceSnapshot := map[string]string{}
	for _, u := range plan.Objectives {
		evidenceSnapshot[u.Key] = u.EvidenceSnapshotID
	}
	for _, c := range plan.ObjectiveEvents {
		snap := snapshotID
		if snap == "" {
			snap = evidenceSnapshot[c.Key]
		}
		if err := q.InsertObjectiveProgressEvent(ctx, storedb.InsertObjectiveProgressEventParams{
			ID: newID("ope_"), AccountID: accountID, ObjectiveKey: c.Key,
			FromVerdict: text(string(c.From)), ToVerdict: string(c.To), SnapshotID: text(snap),
		}); err != nil {
			return fmt.Errorf("insert objective event %s: %w", c.Key, err)
		}
	}
	var achieved []string
	for _, c := range plan.ObjectiveEvents {
		if c.To == rules.Met {
			achieved = append(achieved, c.Key)
		}
	}
	if len(achieved) > 0 {
		if err := q.AchieveGoals(ctx, storedb.AchieveGoalsParams{AccountID: accountID, Keys: achieved}); err != nil {
			return fmt.Errorf("achieve goals: %w", err)
		}
	}
	for _, w := range plan.Marks {
		if err := upsertMark(ctx, q, accountID, w.Ref.GuideID, w.Ref.StepID, w.Mark, now); err != nil {
			return err
		}
	}
	if err := insertEvents(ctx, q, accountID, snapshotID, plan.StepEvents); err != nil {
		return err
	}
	for _, c := range plan.Cursors {
		if err := writeCursor(ctx, q, accountID, c.GuideID, c.Cursor); err != nil {
			return err
		}
	}
	return nil
}

func upsertMark(ctx context.Context, q *storedb.Queries, accountID, guideID, stepID string, m coreprogress.Mark, now time.Time) error {
	params := storedb.UpsertStepMarkParams{
		AccountID: accountID, GuideID: guideID, StepID: stepID, Kind: string(m.Kind), Note: text(m.Note),
	}
	if m.Kind == coreprogress.MarkImplied {
		params.ImpliedByStepID = text(m.ImpliedBy)
		params.ImpliedAt = timestamptz(&now)
	}
	if err := q.UpsertStepMark(ctx, params); err != nil {
		return fmt.Errorf("upsert mark %s/%s: %w", guideID, stepID, err)
	}
	return nil
}

func insertEvents(ctx context.Context, q *storedb.Queries, accountID, snapshotID string, events []coreprogress.StepEvent) error {
	for _, e := range events {
		if err := q.InsertStepStatusEvent(ctx, storedb.InsertStepStatusEventParams{
			ID: newID("sse_"), AccountID: accountID, GuideID: e.Ref.GuideID, StepID: e.Ref.StepID,
			FromStatus: text(string(e.From)), ToStatus: string(e.To), Cause: string(e.Cause), SnapshotID: text(snapshotID),
		}); err != nil {
			return fmt.Errorf("insert step event %s/%s: %w", e.Ref.GuideID, e.Ref.StepID, err)
		}
	}
	return nil
}

func writeCursor(ctx context.Context, q *storedb.Queries, accountID, guideID string, c coreprogress.CursorRecord) error {
	if c.StepID == "" {
		if err := q.DeleteRouteCursor(ctx, storedb.DeleteRouteCursorParams{AccountID: accountID, GuideID: guideID}); err != nil {
			return fmt.Errorf("delete cursor %s: %w", guideID, err)
		}
		return nil
	}
	if err := q.UpsertRouteCursor(ctx, storedb.UpsertRouteCursorParams{AccountID: accountID, GuideID: guideID, StepID: c.StepID, Cause: c.Cause}); err != nil {
		return fmt.Errorf("upsert cursor %s: %w", guideID, err)
	}
	return nil
}

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
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
	v := t.Time
	return &v
}
