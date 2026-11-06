package progress

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// Cursor is the stored cursor of a guide.
type Cursor struct {
	StepID string `json:"step_id"`
	Cause  string `json:"cause"`
}

// Orphan is a mark whose step is not in the loaded content.
type Orphan struct {
	StepID string            `json:"step_id"`
	Mark   coreprogress.Mark `json:"mark"`
}

// BoardView is the account's progress in one guide.
type BoardView struct {
	Guide   string `json:"guide"`
	Account string `json:"account"`
	// Active: this guide is the route the account follows.
	Active  bool     `json:"active"`
	Cursor  *Cursor  `json:"cursor"`
	Orphans []Orphan `json:"orphans"`
	// Marked lists the steps a batch marked (MarkBefore); Undo is what
	// RestoreMarks needs to put each one back.
	Marked []string      `json:"marked,omitempty"`
	Undo   []MarkRestore `json:"undo,omitempty"`
	// CommandID is the mark command a write logged; an undo names it in
	// revokes (docs/10 section 6).
	CommandID string `json:"command_id,omitempty"`
	coreprogress.Board
}

// ObjectiveView is one stored verdict.
type ObjectiveView struct {
	Key          string     `json:"key"`
	Verdict      string     `json:"verdict"`
	FirstMetAt   *time.Time `json:"first_met_at,omitempty"`
	MetInGuideID string     `json:"met_in_guide_id,omitempty"`
	RegressedAt  *time.Time `json:"regressed_at,omitempty"`
	SnapshotID   string     `json:"evidence_snapshot_id,omitempty"`
	Evidence     any        `json:"evidence,omitempty"`
}

// Board returns the derived status of every step of one guide.
func (r *Repository) Board(ctx context.Context, accountID, guideID string) (BoardView, error) {
	var out BoardView
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		in, regressed, err := r.guideInput(ctx, q, accountID, guideID, false)
		if err != nil {
			return err
		}
		out, err = r.boardView(ctx, q, accountID, in, regressed)
		return err
	})
	return out, err
}

// Mark applies an owner's mark action to one step and returns the board.
func (r *Repository) Mark(ctx context.Context, accountID, guideID, stepID string, action coreprogress.MarkAction, note *string) (BoardView, error) {
	return r.ownerWrite(ctx, accountID, guideID, kindMark, stepID, func(in coreprogress.GuideInput) (coreprogress.OwnerPlan, error) {
		return coreprogress.PlanMark(in, stepID, action, note)
	})
}

// ConfirmImplied confirms every implied mark of the guide.
func (r *Repository) ConfirmImplied(ctx context.Context, accountID, guideID string) (BoardView, error) {
	return r.ownerWrite(ctx, accountID, guideID, kindConfirmImplied, "", func(in coreprogress.GuideInput) (coreprogress.OwnerPlan, error) {
		return coreprogress.PlanConfirmImplied(in), nil
	})
}

// MarkBefore marks every pending step before stepID as manual_done and
// returns the board with the ids it marked.
func (r *Repository) MarkBefore(ctx context.Context, accountID, guideID, stepID string) (BoardView, error) {
	var undo []coreprogress.MarkChange
	out, err := r.ownerWrite(ctx, accountID, guideID, kindMarkBefore, stepID, func(in coreprogress.GuideInput) (coreprogress.OwnerPlan, error) {
		p, u, err := coreprogress.PlanMarkBefore(in, stepID)
		undo = u
		return p, err
	})
	for _, u := range undo {
		out.Marked = append(out.Marked, u.StepID)
		out.Undo = append(out.Undo, MarkRestore{StepID: u.StepID, Mark: u.Mark})
	}
	return out, err
}

// MarkRestore is one step of a batch undo: the mark it gets back (nil for
// none; only implied marks are restored).
type MarkRestore struct {
	StepID string             `json:"step_id"`
	Mark   *coreprogress.Mark `json:"mark"`
}

// RestoreMarks undoes a batch: steps still manual_done get their mark back.
func (r *Repository) RestoreMarks(ctx context.Context, accountID, guideID string, restores []MarkRestore) (BoardView, error) {
	changes := make([]coreprogress.MarkChange, 0, len(restores))
	for _, m := range restores {
		changes = append(changes, coreprogress.MarkChange{StepID: m.StepID, Mark: m.Mark})
	}
	return r.ownerWrite(ctx, accountID, guideID, kindRestore, "", func(in coreprogress.GuideInput) (coreprogress.OwnerPlan, error) {
		return coreprogress.PlanRestore(in, changes), nil
	})
}

// PinCursor pins the guide's cursor on a step; an empty stepID unpins it.
func (r *Repository) PinCursor(ctx context.Context, accountID, guideID, stepID string) (BoardView, error) {
	return r.ownerWrite(ctx, accountID, guideID, kindPin, stepID, func(in coreprogress.GuideInput) (coreprogress.OwnerPlan, error) {
		return coreprogress.PlanPin(in, stepID)
	})
}

// ownerWrite runs one owner command: plan, log it with its effects, apply it
// and return the board, all in one transaction.
func (r *Repository) ownerWrite(ctx context.Context, accountID, guideID, kind, stepID string, plan func(coreprogress.GuideInput) (coreprogress.OwnerPlan, error)) (BoardView, error) {
	var out BoardView
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		in, regressed, err := r.guideInput(ctx, q, accountID, guideID, true)
		if err != nil {
			return err
		}
		p, err := plan(in)
		if err != nil {
			return err
		}
		commandID, err := logCommand(ctx, q, accountID, guideID, kind, actorOwner, stepID, commandMeta(ctx), coreprogress.EffectsOf(in.Marks, p.Marks))
		if err != nil {
			return err
		}
		if applyErr := r.applyOwnerPlan(ctx, q, accountID, &in, p, commandID); applyErr != nil {
			return applyErr
		}
		out, err = r.boardView(ctx, q, accountID, in, regressed)
		out.CommandID = commandID
		return err
	})
	if err == nil {
		r.publishOwner(accountID, 0, guideID)
	}
	return out, err
}

// applyOwnerPlan writes an owner plan and mirrors it in in.
func (r *Repository) applyOwnerPlan(ctx context.Context, q *storedb.Queries, accountID string, in *coreprogress.GuideInput, p coreprogress.OwnerPlan, commandID string) error {
	guideID := in.Guide.ID
	now := r.now()
	for _, c := range p.Marks {
		if c.Mark == nil {
			if err := q.DeleteStepMark(ctx, storedb.DeleteStepMarkParams{AccountID: accountID, GuideID: guideID, StepID: c.StepID}); err != nil {
				return fmt.Errorf("delete mark %s: %w", c.StepID, err)
			}
			delete(in.Marks, c.StepID)
			continue
		}
		if err := upsertMark(ctx, q, accountID, guideID, c.StepID, *c.Mark, now); err != nil {
			return err
		}
		in.Marks[c.StepID] = *c.Mark
	}
	if err := insertEvents(ctx, q, accountID, "", p.Events, func(coreprogress.StepEvent) string { return commandID }); err != nil {
		return err
	}
	if p.Cursor != nil {
		if err := writeCursor(ctx, q, accountID, guideID, *p.Cursor); err != nil {
			return err
		}
		in.Cursor = *p.Cursor
	}
	return nil
}

// guideInput loads the account's state in one guide; lock serializes it with
// reevaluations and other owner writes.
func (r *Repository) guideInput(ctx context.Context, q *storedb.Queries, accountID, guideID string, lock bool) (coreprogress.GuideInput, map[string]bool, error) {
	g, ok := r.content.Guide(guideID)
	if !ok {
		return coreprogress.GuideInput{}, nil, ErrGuideNotFound
	}
	var (
		acc coreprogress.Account
		err error
	)
	if lock {
		acc, err = lockAccount(ctx, q, accountID)
	} else {
		acc, err = getAccount(ctx, q, accountID)
	}
	if err != nil {
		return coreprogress.GuideInput{}, nil, err
	}
	in := coreprogress.GuideInput{
		Guide: g, Account: acc, Verdicts: coreprogress.Verdicts{}, Registry: r.catalog,
		Marks: map[string]coreprogress.Mark{}, LastStatus: map[string]coreprogress.Status{},
	}
	objectives, err := q.ListObjectiveProgress(ctx, accountID)
	if err != nil {
		return in, nil, fmt.Errorf("list objective progress: %w", err)
	}
	regressed := map[string]bool{}
	for _, o := range objectives {
		in.Verdicts[o.ObjectiveKey] = rules.Verdict(o.Verdict)
		if o.RegressedAt.Valid {
			regressed[o.ObjectiveKey] = true
		}
	}
	marks, err := q.ListStepMarksByGuide(ctx, storedb.ListStepMarksByGuideParams{AccountID: accountID, GuideID: guideID})
	if err != nil {
		return in, nil, fmt.Errorf("list step marks: %w", err)
	}
	for _, m := range marks {
		in.Marks[m.StepID] = markOf(m)
	}
	last, err := q.ListLastStepStatusesByGuide(ctx, storedb.ListLastStepStatusesByGuideParams{AccountID: accountID, GuideID: guideID})
	if err != nil {
		return in, nil, fmt.Errorf("list last step statuses: %w", err)
	}
	for _, l := range last {
		in.LastStatus[l.StepID] = coreprogress.Status(l.ToStatus)
	}
	cur, err := q.GetRouteCursor(ctx, storedb.GetRouteCursorParams{AccountID: accountID, GuideID: guideID})
	switch {
	case err == nil:
		in.Cursor = coreprogress.CursorRecord{StepID: cur.StepID, Cause: cur.Cause}
	case !errors.Is(err, pgx.ErrNoRows):
		return in, nil, fmt.Errorf("get cursor: %w", err)
	}
	return in, regressed, nil
}

func getAccount(ctx context.Context, q *storedb.Queries, accountID string) (coreprogress.Account, error) {
	acc, err := q.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreprogress.Account{}, ErrAccountNotFound
		}
		return coreprogress.Account{}, fmt.Errorf("get account: %w", err)
	}
	return coreprogress.Account{ActiveGuideID: acc.GuideID, Layer: acc.Layer, Mode: acc.Mode}, nil
}

func (r *Repository) boardView(ctx context.Context, q *storedb.Queries, accountID string, in coreprogress.GuideInput, regressed map[string]bool) (BoardView, error) {
	out := BoardView{
		Guide: in.Guide.ID, Account: accountID, Active: in.Account.ActiveGuideID == in.Guide.ID,
		Orphans: []Orphan{}, Board: coreprogress.BuildBoard(in, regressed),
	}
	// The stored cursor is the engine's; a guide never reevaluated yet still
	// shows where its route starts.
	cur := in.Cursor
	if cur.StepID == "" {
		cur = coreprogress.PlaceCursor(in.Guide, boardStatus(out.Board), in.Verdicts, in.Registry, in.Account, cur)
	}
	if cur.StepID != "" {
		out.Cursor = &Cursor{StepID: cur.StepID, Cause: cur.Cause}
	}
	orphans, err := q.ListOrphanStepMarks(ctx, storedb.ListOrphanStepMarksParams{AccountID: accountID, GuideID: in.Guide.ID})
	if err != nil {
		return out, fmt.Errorf("list orphan marks: %w", err)
	}
	for _, o := range orphans {
		if _, inContent := in.Guide.Step(o.StepID); inContent {
			continue
		}
		out.Orphans = append(out.Orphans, Orphan{StepID: o.StepID, Mark: markOf(o)})
	}
	return out, nil
}

func boardStatus(b coreprogress.Board) map[string]coreprogress.Status {
	out := make(map[string]coreprogress.Status, len(b.Steps))
	for _, s := range b.Steps {
		out[s.ID] = s.Status
	}
	return out
}

// Objectives returns the account's stored verdicts, shared by every guide.
func (r *Repository) Objectives(ctx context.Context, accountID string) ([]ObjectiveView, error) {
	if _, err := getAccount(ctx, r.queries, accountID); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListObjectiveProgress(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list objective progress: %w", err)
	}
	out := make([]ObjectiveView, 0, len(rows))
	for _, row := range rows {
		v := ObjectiveView{
			Key: row.ObjectiveKey, Verdict: row.Verdict, FirstMetAt: timePtr(row.FirstMetAt),
			MetInGuideID: row.MetInGuideID.String, RegressedAt: timePtr(row.RegressedAt), SnapshotID: row.EvidenceSnapshotID.String,
		}
		if len(row.Evidence) > 0 {
			v.Evidence = row.Evidence
		}
		out = append(out, v)
	}
	return out, nil
}
