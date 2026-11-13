// Package plans stores an account's plans (docs/10 section 9, migration
// 0018): the goal and custom plans with their items, and which plan the
// account follows. The route plan of a guide has no row: it is the content
// and the cursor (internal/core/plan). It is the only writer of plans,
// plan_items and accounts.active_plan_id.
package plans

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

var (
	// ErrAccountNotFound: the account does not exist.
	ErrAccountNotFound = errors.New("plans: account not found")
	// ErrPlanNotFound: the account has no such plan.
	ErrPlanNotFound = errors.New("plans: plan not found")
)

// MaxPlans bounds the stored plans of an account.
const MaxPlans = 50

// InvalidError lists why a new plan was refused.
type InvalidError struct{ Problems []string }

func (e *InvalidError) Error() string { return "plans: " + strings.Join(e.Problems, "; ") }

// Repository reads and writes plans.
type Repository struct {
	pool    *pgxpool.Pool
	queries *storedb.Queries
	content *content.Content
	catalog *entities.Registry
}

// New returns a repository over pool; the content and the catalog verify
// the items of a new plan.
func New(pool *pgxpool.Pool, c *content.Content, catalog *entities.Registry) *Repository {
	return &Repository{pool: pool, queries: storedb.New(pool), content: c, catalog: catalog}
}

// NewPlan is a goal or custom plan to store.
type NewPlan struct {
	Kind  string      `json:"kind"`
	Title string      `json:"title"`
	Goal  string      `json:"goal,omitempty"`
	Items []plan.Item `json:"items"`
}

// List is the account's plans: the route of its guide first, then the
// stored ones in creation order, and the id of the one it follows.
type List struct {
	Account string      `json:"account"`
	Active  string      `json:"active"`
	Plans   []plan.Plan `json:"plans"`
}

// List returns the account's plans.
func (r *Repository) List(ctx context.Context, account string) (List, error) {
	acc, err := r.queries.GetActivePlan(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return List{}, ErrAccountNotFound
	}
	if err != nil {
		return List{}, fmt.Errorf("read account: %w", err)
	}
	rows, err := r.queries.ListPlans(ctx, account)
	if err != nil {
		return List{}, fmt.Errorf("list plans: %w", err)
	}
	stored, err := r.withItems(ctx, rows)
	if err != nil {
		return List{}, err
	}
	out := List{Account: account, Active: plan.RouteID(acc.GuideID), Plans: append([]plan.Plan{r.route(acc.GuideID)}, stored...)}
	if acc.ActivePlanID.Valid {
		out.Active = acc.ActivePlanID.String
	}
	return out, nil
}

// Active returns the plan the account follows, with its items: the route
// of its guide unless it chose another.
func (r *Repository) Active(ctx context.Context, account string) (plan.Plan, error) {
	acc, err := r.queries.GetActivePlan(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Plan{}, ErrAccountNotFound
	}
	if err != nil {
		return plan.Plan{}, fmt.Errorf("read account: %w", err)
	}
	if !acc.ActivePlanID.Valid {
		return r.route(acc.GuideID), nil
	}
	return r.Get(ctx, account, acc.ActivePlanID.String)
}

// Get returns one plan of the account (a route plan id is the route of
// the account's guide).
func (r *Repository) Get(ctx context.Context, account, id string) (plan.Plan, error) {
	if guide, ok := plan.RouteGuide(id); ok {
		acc, err := r.queries.GetActivePlan(ctx, account)
		if errors.Is(err, pgx.ErrNoRows) {
			return plan.Plan{}, ErrAccountNotFound
		}
		if err != nil {
			return plan.Plan{}, fmt.Errorf("read account: %w", err)
		}
		if guide != acc.GuideID {
			return plan.Plan{}, ErrPlanNotFound
		}
		return r.route(guide), nil
	}
	row, err := r.queries.GetPlan(ctx, storedb.GetPlanParams{ID: id, AccountID: account})
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Plan{}, ErrPlanNotFound
	}
	if err != nil {
		return plan.Plan{}, fmt.Errorf("read plan: %w", err)
	}
	out, err := r.withItems(ctx, []storedb.Plan{row})
	if err != nil {
		return plan.Plan{}, err
	}
	return out[0], nil
}

// Create stores a goal or custom plan after verifying it: the title, the
// goal (of this account) of a goal plan, and every item against the
// content (plan.CheckItems). A route plan comes from the content and is
// never created.
func (r *Repository) Create(ctx context.Context, account string, in NewPlan) (plan.Plan, error) {
	var problems []string
	switch in.Kind {
	case plan.KindGoal:
		if strings.TrimSpace(in.Goal) == "" {
			problems = append(problems, "a goal plan names its goal")
		}
	case plan.KindCustom:
		if in.Goal != "" {
			problems = append(problems, "a custom plan has no goal")
		}
	case plan.KindRoute:
		problems = append(problems, "a route plan comes from the content and the cursor; it is not created")
	default:
		problems = append(problems, fmt.Sprintf("unknown kind %q (goal or custom)", in.Kind))
	}
	if err := plan.CheckTitle(in.Title); err != nil {
		problems = append(problems, err.Error())
	}
	problems = append(problems, plan.CheckItems(in.Items, r.content, r.catalog)...)
	if len(problems) > 0 {
		return plan.Plan{}, &InvalidError{Problems: problems}
	}
	id := newID()
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		if _, err := q.GetActivePlan(ctx, account); errors.Is(err, pgx.ErrNoRows) {
			return ErrAccountNotFound
		} else if err != nil {
			return fmt.Errorf("read account: %w", err)
		}
		if n, err := q.CountPlans(ctx, account); err != nil {
			return fmt.Errorf("count plans: %w", err)
		} else if n >= MaxPlans {
			return &InvalidError{Problems: []string{fmt.Sprintf("an account keeps at most %d plans", MaxPlans)}}
		}
		goal := pgtype.Text{}
		if in.Kind == plan.KindGoal {
			if _, err := q.GetGoalOfAccount(ctx, storedb.GetGoalOfAccountParams{ID: in.Goal, AccountID: account}); errors.Is(err, pgx.ErrNoRows) {
				return &InvalidError{Problems: []string{fmt.Sprintf("the account has no goal %q", in.Goal)}}
			} else if err != nil {
				return fmt.Errorf("read goal: %w", err)
			}
			goal = text(in.Goal)
		}
		if err := q.InsertPlan(ctx, storedb.InsertPlanParams{ID: id, AccountID: account, Kind: in.Kind, Title: strings.TrimSpace(in.Title), GoalID: goal}); err != nil {
			return fmt.Errorf("insert plan: %w", err)
		}
		for i, it := range in.Items {
			p := storedb.InsertPlanItemParams{PlanID: id, Position: int32(i), Kind: it.Kind}
			switch it.Kind {
			case plan.ItemStep:
				p.GuideID, p.StepID = text(it.Guide), text(it.Step)
			case plan.ItemRecipe:
				p.RecipeKey = text(it.Recipe.String())
			case plan.ItemTask:
				raw, err := json.Marshal(it.Task)
				if err != nil {
					return fmt.Errorf("encode task: %w", err)
				}
				p.Task = raw
			}
			if err := q.InsertPlanItem(ctx, p); err != nil {
				return fmt.Errorf("insert plan item %d: %w", i, err)
			}
		}
		return nil
	})
	if err != nil {
		return plan.Plan{}, err
	}
	return r.Get(ctx, account, id)
}

// Delete removes a stored plan; an account that followed it goes back to
// its route.
func (r *Repository) Delete(ctx context.Context, account, id string) error {
	if _, ok := plan.RouteGuide(id); ok {
		return &InvalidError{Problems: []string{"the route plan comes from the content; it is not deleted"}}
	}
	n, err := r.queries.DeletePlan(ctx, storedb.DeletePlanParams{ID: id, AccountID: account})
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	if n == 0 {
		return ErrPlanNotFound
	}
	return nil
}

// SetActive makes the account follow a plan: a stored plan by id, or its
// route (the route plan's id, or "").
func (r *Repository) SetActive(ctx context.Context, account, id string) (plan.Plan, error) {
	var target pgtype.Text
	if _, route := plan.RouteGuide(id); !route && id != "" {
		target = text(id)
	}
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		acc, err := q.GetActivePlan(ctx, account)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccountNotFound
		}
		if err != nil {
			return fmt.Errorf("read account: %w", err)
		}
		if guide, route := plan.RouteGuide(id); route && guide != acc.GuideID {
			// Another guide's route is followed by switching the active guide.
			return ErrPlanNotFound
		}
		if target.Valid {
			if _, err := q.GetPlan(ctx, storedb.GetPlanParams{ID: id, AccountID: account}); errors.Is(err, pgx.ErrNoRows) {
				return ErrPlanNotFound
			} else if err != nil {
				return fmt.Errorf("read plan: %w", err)
			}
		}
		if _, err := q.SetActivePlan(ctx, storedb.SetActivePlanParams{AccountID: account, PlanID: target}); err != nil {
			return fmt.Errorf("set active plan: %w", err)
		}
		return nil
	})
	if err != nil {
		return plan.Plan{}, err
	}
	return r.Active(ctx, account)
}

// route is the route plan of a guide.
func (r *Repository) route(guide string) plan.Plan {
	title := guide
	if r.content != nil {
		if g, ok := r.content.Guide(guide); ok && g.Title != "" {
			title = g.Title
		}
	}
	return plan.Plan{ID: plan.RouteID(guide), Kind: plan.KindRoute, Title: title, Guide: guide, Items: []plan.Item{}}
}

// withItems reads the items of stored plans.
func (r *Repository) withItems(ctx context.Context, rows []storedb.Plan) ([]plan.Plan, error) {
	out := make([]plan.Plan, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, len(rows))
	for i, p := range rows {
		ids[i] = p.ID
	}
	items, err := r.queries.ListPlanItems(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list plan items: %w", err)
	}
	for _, p := range rows {
		pl := plan.Plan{ID: p.ID, Kind: p.Kind, Title: p.Title, Guide: p.GuideID.String, Goal: p.GoalID.String, Items: []plan.Item{}}
		for _, it := range items {
			if it.PlanID != p.ID {
				continue
			}
			item := plan.Item{Kind: it.Kind, Guide: it.GuideID.String, Step: it.StepID.String}
			switch it.Kind {
			case plan.ItemRecipe:
				k, err := entities.ParseObjectiveKey(it.RecipeKey.String)
				if err != nil {
					return nil, fmt.Errorf("plan %s item %d: %w", p.ID, it.Position, err)
				}
				item.Recipe = &k
			case plan.ItemTask:
				var t content.Task
				if err := json.Unmarshal(it.Task, &t); err != nil {
					return nil, fmt.Errorf("plan %s item %d: %w", p.ID, it.Position, err)
				}
				item.Task = &t
			}
			pl.Items = append(pl.Items, item)
		}
		out = append(out, pl)
	}
	return out, nil
}

// Guides returns the guides a plan's steps come from, sorted.
func Guides(p plan.Plan) []string {
	var out []string
	if p.Kind == plan.KindRoute && p.Guide != "" {
		out = append(out, p.Guide)
	}
	for _, it := range p.Items {
		if it.Kind == plan.ItemStep && !slices.Contains(out, it.Guide) {
			out = append(out, it.Guide)
		}
	}
	slices.Sort(out)
	return out
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
	return tx.Commit(ctx)
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

// newID is a plan id: pln_ and 16 hex digits.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "pln_" + hex.EncodeToString(b)
}
