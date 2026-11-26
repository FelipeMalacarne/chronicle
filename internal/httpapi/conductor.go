package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/conductor"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	storeconductor "github.com/FelipeMalacarne/chronicle/internal/store/conductor"
)

// The conductor (docs/12 section 12.5; FEL-126): for an account whose owner
// turned it on, internal/conduct dispatches the next task of the plan as a
// job, the same way POST /api/microbot/task does, and the next one when the
// job succeeds. The owner turns it on and off here.

// ConductorActor is the conductor in the audit, with the origin "server".
const ConductorActor = "conductor"

// ConductorStore is the conductor's state per account (internal/store/conductor).
type ConductorStore interface {
	Get(ctx context.Context, account string) (conductor.State, error)
	TurnOn(ctx context.Context, account string) error
	TurnOff(ctx context.Context, account string) error
}

// conductorRoutes are account-addressed like the others (rule 10): behind
// authed and accountParam, never outside them. Turning it on acts in game, so
// it also needs Microbot control.
func (s *Server) conductorRoutes() {
	if s.opts.Conductor == nil || !s.accountRoutes() {
		return
	}
	account := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(h)) }
	account("GET /api/conductor", s.handleConductor)
	account("POST /api/conductor", s.microbotControl(s.handleConductorSet))
}

// conductorView is GET /api/conductor.
type conductorView struct {
	Account string `json:"account"`
	State   string `json:"state"`
	Job     string `json:"job,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

func (s *Server) handleConductor(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	s.writeConductor(w, r, account)
}

func (s *Server) writeConductor(w http.ResponseWriter, r *http.Request, account string) {
	st, err := s.opts.Conductor.Get(r.Context(), account)
	if err != nil {
		s.log.Error("conductor", "err", err)
		writeError(w, http.StatusInternalServerError, "reading the conductor state")
		return
	}
	writeJSON(w, http.StatusOK, conductorView{Account: account, State: string(st.Mode), Job: st.Job, Reason: st.Reason})
}

// handleConductorSet turns the account's conductor on (idle: it dispatches
// the plan's next task as soon as the account has no active job) or off.
func (s *Server) handleConductorSet(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var req struct {
		On *bool `json:"on"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.On == nil {
		writeError(w, http.StatusBadRequest, `required: "on" (true or false)`)
		return
	}
	set := s.opts.Conductor.TurnOff
	if *req.On {
		set = s.opts.Conductor.TurnOn
	}
	if err := set(r.Context(), account); err != nil {
		if errors.Is(err, storeconductor.ErrAccountNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		s.log.Error("conductor", "err", err)
		writeError(w, http.StatusInternalServerError, "writing the conductor state")
		return
	}
	s.writeConductor(w, r, account)
}

// DispatchNext starts the next task of the account's plan for the conductor:
// dispatchTask unsupervised, audited as the conductor, its job of the
// conductor's source. A refusal the owner must act on (the plan done, a
// manual step, E9, the client cannot run it) is Refused; the plan waiting
// for something or the agent away is Wait.
func (s *Server) DispatchNext(ctx context.Context, account string) conductor.Outcome {
	d, err := s.dispatchTask(ctx, account, taskRequest{}, auditor{actor: ConductorActor, origin: "server"})
	if err == nil {
		return conductor.Outcome{Kind: conductor.Dispatched, Job: d.job}
	}
	var refused *taskRefusal
	switch {
	case errors.As(err, &refused) && refused.next.Status == plan.StatusWaiting:
		return conductor.Outcome{Kind: conductor.Wait, Reason: refused.msg}
	case errors.As(err, &refused):
		return conductor.Outcome{Kind: conductor.Refused, Reason: refused.msg}
	case errors.Is(err, agent.ErrNoAgent):
		return conductor.Outcome{Kind: conductor.Wait, Reason: "nenhum agent conectado para a conta"}
	case errors.Is(err, errReadingPlan), errors.Is(err, errReadingPrice), errors.Is(err, context.DeadlineExceeded):
		return conductor.Outcome{Kind: conductor.Wait, Reason: err.Error()}
	default:
		// The agent refused the command, or it is too old: asking again
		// would only fill the audit.
		return conductor.Outcome{Kind: conductor.Refused, Reason: err.Error()}
	}
}
