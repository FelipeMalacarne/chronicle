package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

// The recurring routines (docs/08 phase 4, delivery 4; docs/11 section 16):
// kept per account in the database and sent to the account's agent, whose
// watcher starts them when their clock is ready or their interval passed.
// Always addressed by ?account= (accountParam), behind authed and
// microbotControl; saving and deleting go to the audit.

// Audit actions of the recurring routines (not agent commands: the agent
// gets them in routines_sync).
const (
	auditRecurringSet    = "recurring_set"
	auditRecurringDelete = "recurring_delete"
)

// recurringTarget is the account of a recurring routine request and the agent
// that serves it, if one is connected.
type recurringTarget struct {
	account   string
	served    agent.Info
	connected bool
}

// runs reports whether the connected agent runs recurring routines (it
// declares recurring_get: older agents refuse them in routines_sync).
func (t recurringTarget) runs() bool { return t.connected && t.served.Supports(agent.CmdRecurringGet) }

func (s *Server) recurringTarget(w http.ResponseWriter, r *http.Request) (recurringTarget, bool) {
	if s.opts.Microbot.Routines == nil {
		writeError(w, http.StatusServiceUnavailable, "recurring routines need the database")
		return recurringTarget{}, false
	}
	account, ok := s.accountParam(w, r)
	if !ok {
		return recurringTarget{}, false
	}
	t := recurringTarget{account: account}
	if served, err := s.opts.Microbot.Link.Route(account); err == nil {
		t.served, t.connected = served, true
	}
	return t, true
}

// storedRecurring is the recurring routines as the database holds them,
// with why the agent's view is missing.
func storedRecurring(list []agent.Recurring, why string) agent.RecurringStatus {
	st := agent.RecurringStatus{Recurring: make([]agent.RecurringState, len(list)), WatchError: why}
	for i, rc := range list {
		st.Recurring[i] = agent.RecurringState{Recurring: rc}
	}
	return st
}

// recurringAway says why the agent's view of the recurring routines is
// missing.
func (t recurringTarget) away() string {
	if !t.connected {
		return "no agent connected: it gets the recurring routines when it links"
	}
	return "the agent does not run recurring routines yet: update chronicle on the game machine"
}

// handleRecurringGet is the agent's watcher as it stands (which routines are
// due and why); with no agent that runs them, the database's.
func (s *Server) handleRecurringGet(w http.ResponseWriter, r *http.Request) {
	t, ok := s.recurringTarget(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	why := t.away()
	if t.runs() {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		var st agent.RecurringStatus
		err := s.opts.Microbot.Link.CallAgent(ctx, t.served.ID, agent.CmdRecurringGet, nil, &st)
		if err == nil {
			writeJSON(w, http.StatusOK, st)
			return
		}
		why = "the agent did not answer: " + err.Error()
	}
	list, err := s.opts.Microbot.Routines.Recurring(r.Context(), t.account)
	if err != nil {
		s.log.Error("routines: recurring", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the recurring routines")
		return
	}
	writeJSON(w, http.StatusOK, storedRecurring(list, why))
}

// handleRecurringSave saves one recurring routine (a new one without id)
// in the database, then sends the account's routines to its agent, which
// gets them on its next connection when it is away.
func (s *Server) handleRecurringSave(w http.ResponseWriter, r *http.Request) {
	t, ok := s.recurringTarget(w, r)
	if !ok {
		return
	}
	var rc agent.Recurring
	if !decodeBody(w, r, &rc) {
		return
	}
	// The server's own fields: never taken from a request.
	rc.UpdatedAt, rc.LastRun = nil, nil
	rc, err := agent.ValidateRecurring(rc)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rc.Run.Until != nil {
		// The account's agent must end the run on the objective; one that
		// is away gets the routine only once it can (agent.RoutinesFor).
		if err = s.untilFor(t.account, rc.Run.Until); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if t.connected && !t.served.Supports(agent.CmdRunUntil) {
			s.writeMicrobotError(w, errAgentNoUntil)
			return
		}
	}
	id, list, err := s.opts.Microbot.Routines.PutRecurring(r.Context(), t.account, rc)
	if id != "" {
		rc.ID = id
	}
	params, _ := json.Marshal(rc)
	s.auditMicrobot(r, t.account, t.served, auditRecurringSet, params, err)
	if err != nil {
		s.writeRecurringError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "result": s.pushRecurring(r.Context(), t, list)})
}

// handleRecurringDelete removes one recurring routine; its runs stay in the
// history.
func (s *Server) handleRecurringDelete(w http.ResponseWriter, r *http.Request) {
	t, ok := s.recurringTarget(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	list, err := s.opts.Microbot.Routines.DeleteRecurring(r.Context(), t.account, id)
	params, _ := json.Marshal(map[string]string{"id": id})
	s.auditMicrobot(r, t.account, t.served, auditRecurringDelete, params, err)
	if err != nil {
		s.writeRecurringError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": s.pushRecurring(r.Context(), t, list)})
}

// pushRecurring sends the account's routines (the triggers with the
// recurring ones, as routines_sync always does) to the agent that runs them
// and returns its watcher's view; otherwise the database's, saying why.
func (s *Server) pushRecurring(ctx context.Context, t recurringTarget, list []agent.Recurring) agent.RecurringStatus {
	if !t.runs() {
		return storedRecurring(list, t.away())
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	link := s.opts.Microbot.Link
	ts, err := s.opts.Microbot.Routines.Triggers(ctx, t.account)
	if err == nil {
		err = link.CallAgent(ctx, t.served.ID, agent.CmdRoutinesSync, agent.RoutinesFor(t.served, ts, list), nil)
	}
	var st agent.RecurringStatus
	if err == nil {
		err = link.CallAgent(ctx, t.served.ID, agent.CmdRecurringGet, nil, &st)
	}
	if err != nil {
		// Saved: the agent gets them on its next connection.
		return storedRecurring(list, "saved; the agent did not take it now ("+err.Error()+") and gets it when it links again")
	}
	return st
}

func (s *Server) writeRecurringError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storeroutines.ErrNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, storeroutines.ErrRoutineNotFound):
		writeError(w, http.StatusNotFound, "recurring routine not found")
	case errors.Is(err, storeroutines.ErrNameTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, storeroutines.ErrTooMany):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.log.Error("routines: recurring", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save the recurring routine")
	}
}
