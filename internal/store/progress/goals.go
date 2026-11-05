package progress

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/state"
)

// Goal errors.
var (
	ErrInvalidGoal  = errors.New("invalid goal")
	ErrGoalNotFound = errors.New("goal not found")
)

// GoalView is one pinned goal with the current verdict of its objective.
type GoalView struct {
	ID         string     `json:"id"`
	Key        string     `json:"objective_key"`
	Label      string     `json:"label"`
	Priority   int        `json:"priority"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	AchievedAt *time.Time `json:"achieved_at,omitempty"`
	Verdict    string     `json:"verdict"`
	Regressed  bool       `json:"regressed,omitempty"`
}

// Goals returns the account's active and achieved goals.
func (r *Repository) Goals(ctx context.Context, accountID string) ([]GoalView, error) {
	if _, err := getAccount(ctx, r.queries, accountID); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListGoalsByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	out := make([]GoalView, 0, len(rows))
	for _, g := range rows {
		out = append(out, GoalView{
			ID: g.ID, Key: g.ObjectiveKey, Label: g.Label, Priority: int(g.Priority), Status: g.Status,
			CreatedAt: g.CreatedAt.Time, AchievedAt: timePtr(g.AchievedAt), Verdict: g.Verdict, Regressed: g.ObjectiveRegressedAt.Valid,
		})
	}
	return out, nil
}

// AddGoal pins an objective as a goal (F9). The key must be canonical and
// name a catalog entry; pinning the same objective again returns the goal
// that already exists. An objective already met is achieved at once.
func (r *Repository) AddGoal(ctx context.Context, accountID, key, label string) (GoalView, error) {
	k, err := entities.ParseObjectiveKey(key)
	if err != nil {
		return GoalView{}, fmt.Errorf("%w: %v", ErrInvalidGoal, err)
	}
	if r.catalog != nil {
		if why := r.catalog.KnownObjective(k); why != "" {
			return GoalView{}, fmt.Errorf("%w: %s", ErrInvalidGoal, why)
		}
	}
	key = k.String()
	if pinErr := r.inTx(ctx, func(tx pgx.Tx) error { return r.pinGoal(ctx, r.queries.WithTx(tx), accountID, key, label) }); pinErr != nil {
		return GoalView{}, pinErr
	}
	r.publishOwner(accountID, 0)
	goals, err := r.Goals(ctx, accountID)
	if err != nil {
		return GoalView{}, err
	}
	for _, g := range goals {
		if g.Key == key {
			return g, nil
		}
	}
	return GoalView{}, ErrGoalNotFound
}

func (r *Repository) pinGoal(ctx context.Context, q *storedb.Queries, accountID, key, label string) error {
	if _, err := lockAccount(ctx, q, accountID); err != nil {
		return err
	}
	_, err := q.FindActiveGoal(ctx, storedb.FindActiveGoalParams{AccountID: accountID, ObjectiveKey: key})
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("find goal: %w", err)
	}
	if _, err = q.InsertGoal(ctx, storedb.InsertGoalParams{ID: newID("goal_"), AccountID: accountID, ObjectiveKey: key, Label: label, Priority: 100}); err != nil {
		return fmt.Errorf("insert goal: %w", err)
	}
	met, err := r.objectiveMet(ctx, q, accountID, key)
	if err != nil || !met {
		return err
	}
	return q.AchieveGoals(ctx, storedb.AchieveGoalsParams{AccountID: accountID, Keys: []string{key}})
}

func (r *Repository) objectiveMet(ctx context.Context, q *storedb.Queries, accountID, key string) (bool, error) {
	rows, err := q.ListObjectiveProgress(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("list objective progress: %w", err)
	}
	for _, row := range rows {
		if row.ObjectiveKey == key {
			return row.Verdict == "met", nil
		}
	}
	return false, nil
}

// RemoveGoal unpins a goal.
func (r *Repository) RemoveGoal(ctx context.Context, accountID, goalID string) error {
	if _, err := getAccount(ctx, r.queries, accountID); err != nil {
		return err
	}
	n, err := r.queries.DeleteGoal(ctx, storedb.DeleteGoalParams{ID: goalID, AccountID: accountID})
	if err != nil {
		return fmt.Errorf("delete goal: %w", err)
	}
	if n == 0 {
		return ErrGoalNotFound
	}
	r.publishOwner(accountID, 0)
	return nil
}

// GoalPlanView is a goal and its plan (F9 part B).
type GoalPlanView struct {
	Goal GoalView              `json:"goal"`
	Plan coreprogress.GoalPlan `json:"plan"`
}

// GoalPlan computes the plan of one of the account's goals: the closure of
// its requirements in the catalog against the account's facts, verdicts and
// marks of every guide (docs/09 section 3).
func (r *Repository) GoalPlan(ctx context.Context, accountID, goalID string) (GoalPlanView, error) {
	goals, err := r.Goals(ctx, accountID)
	if err != nil {
		return GoalPlanView{}, err
	}
	var out GoalPlanView
	for _, g := range goals {
		if g.ID == goalID {
			out.Goal = g
		}
	}
	if out.Goal.ID == "" {
		return GoalPlanView{}, ErrGoalNotFound
	}
	key, err := entities.ParseObjectiveKey(out.Goal.Key)
	if err != nil {
		return GoalPlanView{}, fmt.Errorf("%w: %v", ErrInvalidGoal, err)
	}
	err = r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		ps, _, loadErr := state.Load(ctx, q, accountID, r.catalog)
		if loadErr != nil {
			return loadErr
		}
		in := coreprogress.GoalPlanInput{Goal: key, Registry: r.catalog, State: ps}
		for _, g := range r.content.Guides {
			gi, _, inErr := r.guideInput(ctx, q, accountID, g.ID, false)
			if inErr != nil {
				return inErr
			}
			in.Verdicts = gi.Verdicts
			in.Guides = append(in.Guides, gi)
		}
		out.Plan = coreprogress.BuildGoalPlan(in)
		return nil
	})
	return out, err
}
