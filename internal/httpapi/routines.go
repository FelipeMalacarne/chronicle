package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

// RoutineStore is the database side of the routines (docs/11 section 4.4,
// F6; internal/store/routines): the account's triggers and its runs'
// history, which survive the agent's machine and a worker's pod.
type RoutineStore interface {
	Triggers(ctx context.Context, account string) ([]agent.Trigger, error)
	PutTrigger(ctx context.Context, account string, t agent.Trigger) ([]agent.Trigger, error)
	Runs(ctx context.Context, account string, limit int) ([]agent.Run, error)
	// The recurring routines (docs/08 phase 4, delivery 4).
	Recurring(ctx context.Context, account string) ([]agent.Recurring, error)
	PutRecurring(ctx context.Context, account string, rc agent.Recurring) (string, []agent.Recurring, error)
	DeleteRecurring(ctx context.Context, account, id string) ([]agent.Recurring, error)
}

// runPage is how many runs the history shows (the agent kept 30).
const runPage = 30

// routineTarget is where a routine request goes: the account (the query's,
// or the one the only agent declares), the agent that serves it, and whether
// the database answers. An agent older than F6 (no routines_sync in its
// hello) keeps its own triggers and history, so the request goes to it.
type routineTarget struct {
	account   string
	served    agent.Info
	connected bool
	db        bool
}

func (s *Server) routineTarget(w http.ResponseWriter, r *http.Request) (routineTarget, bool) {
	account, ok := s.microbotAccount(w, r)
	if !ok {
		return routineTarget{}, false
	}
	t := routineTarget{account: account}
	if served, err := s.opts.Microbot.Link.Route(account); err == nil {
		t.served, t.connected = served, true
		if t.account == "" {
			t.account = served.Account
		}
	}
	t.db = s.opts.Microbot.Routines != nil && t.account != "" && (!t.connected || t.served.Supports(agent.CmdRoutinesSync))
	return t, true
}

// handleRuns is the runs' history, newest first, from the database.
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	t, ok := s.routineTarget(w, r)
	if !ok {
		return
	}
	if !t.db {
		s.microbotRead(agent.CmdRunHistory, func(*http.Request) any { return nil })(w, r)
		return
	}
	runs, err := s.opts.Microbot.Routines.Runs(r.Context(), t.account, runPage)
	if err != nil {
		s.log.Error("routines: runs", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the runs")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, agent.RunHistory{Runs: runs})
}

// handleTriggersGet is the agent's watcher as it stands (it knows whether it
// is watching); with no agent connected, the account's triggers from the
// database.
func (s *Server) handleTriggersGet(w http.ResponseWriter, r *http.Request) {
	t, ok := s.routineTarget(w, r)
	if !ok {
		return
	}
	if !t.db || t.connected {
		s.microbotRead(agent.CmdTriggerGet, func(*http.Request) any { return nil })(w, r)
		return
	}
	ts, err := s.opts.Microbot.Routines.Triggers(r.Context(), t.account)
	if err != nil {
		s.log.Error("routines: triggers", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the triggers")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, agent.TriggerStatus{Triggers: ts, WatchError: "no agent connected: it gets the triggers when it links"})
}

// handleTriggersSet saves one trigger in the database, then on the agent,
// which gets it on its next connection when it is away.
func (s *Server) handleTriggersSet(w http.ResponseWriter, r *http.Request) {
	t, ok := s.routineTarget(w, r)
	if !ok {
		return
	}
	if !t.db {
		s.microbotAction(agent.CmdTriggerSet, 30*time.Second)(w, r)
		return
	}
	var raw json.RawMessage
	if !decodeBody(w, r, &raw) {
		return
	}
	p, err := agent.ValidateParams(agent.CmdTriggerSet, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	trig, isTrigger := p.(agent.Trigger)
	if !isTrigger {
		writeError(w, http.StatusBadRequest, "not a trigger")
		return
	}
	params, _ := json.Marshal(trig)
	ts, err := s.opts.Microbot.Routines.PutTrigger(r.Context(), t.account, trig)
	if err != nil {
		s.auditMicrobot(r, t.account, t.served, agent.CmdTriggerSet, params, err)
		s.log.Error("routines: save trigger", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save the trigger")
		return
	}
	status := agent.TriggerStatus{Triggers: ts, WatchError: "no agent connected: it gets the trigger when it links"}
	if t.connected {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		var live agent.TriggerStatus
		if err = s.opts.Microbot.Link.CallAgent(ctx, t.served.ID, agent.CmdTriggerSet, params, &live); err == nil {
			status = live
		} else {
			// Saved: the agent gets it on its next connection.
			status.WatchError = "saved; the agent did not take it now (" + err.Error() + ") and gets it when it links again"
		}
	}
	s.auditMicrobot(r, t.account, t.served, agent.CmdTriggerSet, params, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": status})
}
