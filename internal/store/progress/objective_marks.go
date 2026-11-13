package progress

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// ErrInvalidObjective: the key to mark is not a canonical objective key or no
// step of any guide references it.
var ErrInvalidObjective = errors.New("invalid objective")

// ObjectiveMarkResult is what an objective mark or unmark did.
type ObjectiveMarkResult struct {
	Key string `json:"key"`
	// Marked: the objective is marked after the command.
	Marked bool `json:"marked"`
	// StepsChanged counts the steps whose status changed, in every guide.
	StepsChanged int `json:"steps_changed"`
}

// MarkObjective records the owner's mark on an objective: it is satisfied for
// the whole account, in every guide and step (docs/09 section 5, owner
// decision of 2026-11-13). The mark is an input of the status derivation, not
// evidence: objective_progress is untouched and the implied cascade does not
// run. The step statuses of every guide are re-derived and the changes
// recorded as cause user, all in one transaction that locks the account.
//
// It is not appended to mark_commands: that log replays step_marks, and an
// objective mark is not a step mark. The row (note, marked_at) and the
// step_status_events it causes are its record.
func (r *Repository) MarkObjective(ctx context.Context, accountID, key, note string) (ObjectiveMarkResult, error) {
	k, err := entities.ParseObjectiveKey(strings.TrimSpace(key))
	if err != nil {
		return ObjectiveMarkResult{}, fmt.Errorf("%w: %v", ErrInvalidObjective, err)
	}
	canonical := k.String()
	if !r.routeObjectives()[canonical] {
		return ObjectiveMarkResult{}, fmt.Errorf("%w: no step of any guide references %s", ErrInvalidObjective, canonical)
	}
	return r.writeObjectiveMark(ctx, accountID, canonical, func(q *storedb.Queries) error {
		if err := q.UpsertObjectiveMark(ctx, storedb.UpsertObjectiveMarkParams{AccountID: accountID, ObjectiveKey: canonical, Note: note}); err != nil {
			return fmt.Errorf("upsert objective mark %s: %w", canonical, err)
		}
		return nil
	}, true)
}

// UnmarkObjective takes the owner's mark back; the objective reads as its
// stored verdict again. Unmarking an objective that is not marked changes
// nothing. A key that no step references any more can still be unmarked.
func (r *Repository) UnmarkObjective(ctx context.Context, accountID, key string) (ObjectiveMarkResult, error) {
	canonical := strings.TrimSpace(key)
	if k, err := entities.ParseObjectiveKey(canonical); err == nil {
		canonical = k.String()
	}
	return r.writeObjectiveMark(ctx, accountID, canonical, func(q *storedb.Queries) error {
		if _, err := q.DeleteObjectiveMark(ctx, storedb.DeleteObjectiveMarkParams{AccountID: accountID, ObjectiveKey: canonical}); err != nil {
			return fmt.Errorf("delete objective mark %s: %w", canonical, err)
		}
		return nil
	}, false)
}

// writeObjectiveMark runs write, then re-derives every guide's statuses and
// cursors from the stored verdicts and the account's marks (StatusOnly).
func (r *Repository) writeObjectiveMark(ctx context.Context, accountID, key string, write func(*storedb.Queries) error, marked bool) (ObjectiveMarkResult, error) {
	out := ObjectiveMarkResult{Key: key, Marked: marked}
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		acc, err := lockAccount(ctx, q, accountID)
		if err != nil {
			return err
		}
		if wErr := write(q); wErr != nil {
			return wErr
		}
		in, err := r.engineInput(ctx, q, accountID, acc)
		if err != nil {
			return err
		}
		in.StatusOnly = true
		plan := coreprogress.Reevaluate(in)
		if pErr := writePlan(ctx, q, accountID, "", in.Now, plan); pErr != nil {
			return pErr
		}
		out.StepsChanged = len(plan.StepEvents)
		return nil
	})
	if err == nil {
		r.publishOwner(accountID, out.StepsChanged)
	}
	return out, err
}

// routeObjectives is the set of canonical keys some guide step references
// (any role) or whose quest requirements the steps complete.
func (r *Repository) routeObjectives() map[string]bool {
	out := map[string]bool{}
	for _, g := range r.content.Guides {
		for _, st := range g.Steps() {
			for _, o := range st.Objectives {
				out[o.Key.String()] = true
			}
			for _, k := range coreprogress.CatalogRequires(st, r.catalog) {
				out[k] = true
			}
		}
	}
	return out
}

// loadObjectiveMarks reads the account's marked objective keys.
func loadObjectiveMarks(ctx context.Context, q *storedb.Queries, accountID string) (map[string]bool, error) {
	rows, err := q.ListObjectiveMarks(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list objective marks: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.ObjectiveKey] = true
	}
	return out, nil
}
