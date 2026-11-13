package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
)

// PlanStore stores the accounts' plans (docs/10 section 9,
// internal/store/plans).
type PlanStore interface {
	List(ctx context.Context, account string) (storeplans.List, error)
	Active(ctx context.Context, account string) (plan.Plan, error)
	Create(ctx context.Context, account string, in storeplans.NewPlan) (plan.Plan, error)
	Delete(ctx context.Context, account, id string) error
	SetActive(ctx context.Context, account, id string) (plan.Plan, error)
}

// planRoutes are the plans of an account (docs/10 section 9): the next
// task of the active plan, and the goal and custom plans (their screen is
// phase 5). Account-addressed like the others (rule 10).
func (s *Server) planRoutes() {
	if s.opts.Plans == nil || s.opts.Progress == nil || !s.accountRoutes() {
		return
	}
	account := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(h)) }
	account("GET /api/plan/next", s.handlePlanNext)
	account("GET /api/plans", s.handlePlans)
	account("POST /api/plans", s.handleCreatePlan)
	account("PUT /api/plans/active", s.handleActivePlan)
	account("DELETE /api/plans/{id}", s.handleDeletePlan)
}

// planNext is the answer of GET /api/plan/next.
type planNext struct {
	Account string `json:"account"`
	plan.Result
}

// handlePlanNext is the next typed task of the account's active plan, each
// termination evaluated on its facts now; the tasks already met are
// skipped (docs/10 section 7, rule 2).
func (s *Server) handlePlanNext(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	in, err := s.planInput(r.Context(), account)
	if err != nil {
		s.writePlanError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, planNext{Account: account, Result: plan.Next(in)})
}

// planInput reads what the active plan's next task is evaluated on: the
// plan, the boards of its guides, the item holdings and the account's facts.
func (s *Server) planInput(ctx context.Context, account string) (plan.Input, error) {
	p, err := s.opts.Plans.Active(ctx, account)
	if err != nil {
		return plan.Input{}, err
	}
	boards := map[string]plan.Board{}
	for _, guide := range storeplans.Guides(p) {
		if _, ok := s.opts.Content.Guide(guide); !ok {
			continue
		}
		b, berr := s.opts.Progress.Board(ctx, account, guide)
		if berr != nil {
			return plan.Input{}, berr
		}
		cursor := ""
		if b.Cursor != nil {
			cursor = b.Cursor.StepID
		}
		boards[guide] = plan.Board{Steps: b.Steps, Cursor: cursor}
	}
	var h rules.ItemHoldings
	if s.opts.Holdings != nil {
		if h, err = s.opts.Holdings.ItemHoldings(ctx, account); err != nil {
			return plan.Input{}, errors.Join(errHoldings, err)
		}
	}
	facts, err := s.opts.Progress.TaskFacts(ctx, account)
	if err != nil {
		return plan.Input{}, err
	}
	return plan.Input{
		Plan: p, Content: s.opts.Content, Registry: s.opts.Catalog, Boards: boards, Holdings: h, Facts: facts, Now: time.Now(),
	}, nil
}

func (s *Server) handlePlans(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	list, err := s.opts.Plans.List(r.Context(), account)
	if err != nil {
		s.writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var in storeplans.NewPlan
	if !decodeBody(w, r, &in) {
		return
	}
	p, err := s.opts.Plans.Create(r.Context(), account, in)
	if err != nil {
		s.writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// activePlanRequest names the plan to follow: a stored plan's id, or the
// route (its id, or empty).
type activePlanRequest struct {
	Plan string `json:"plan"`
}

func (s *Server) handleActivePlan(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var in activePlanRequest
	if !decodeBody(w, r, &in) {
		return
	}
	p, err := s.opts.Plans.SetActive(r.Context(), account, strings.TrimSpace(in.Plan))
	if err != nil {
		s.writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	if err := s.opts.Plans.Delete(r.Context(), account, r.PathValue("id")); err != nil {
		s.writePlanError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writePlanError(w http.ResponseWriter, err error) {
	var invalid *storeplans.InvalidError
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid plan", "status": http.StatusBadRequest, "problems": invalid.Problems})
	case errors.Is(err, storeplans.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, storeplans.ErrPlanNotFound):
		writeError(w, http.StatusNotFound, "plan not found")
	case errors.Is(err, errHoldings):
		s.log.Error("plan: load item holdings", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load item holdings")
	default:
		s.writeProgressError(w, err)
	}
}
