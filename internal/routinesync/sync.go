// Package routinesync keeps the agents' routines and the server's in step
// (docs/11 section 4.4, phase F6). The executor stays in the agent (docs/08,
// R1); the database is the source of the triggers and the record of the
// runs:
//
//   - on every connection the agent gets the account's triggers and, when
//     it runs them (recurring_get in its hello), its recurring routines
//     (routines_sync), so a worker's pod, which loses its disk, keeps them;
//     an attached agent whose account has no triggers yet seeds them from
//     its own file once;
//   - the agent sends each run as it changes (a TypeEvent), which is stored
//     with its trail; what the executor did by itself (a run a trigger or a
//     recurring routine started, and how it ended) goes to the Microbot
//     audit.
//
// An agent that does not declare routines_sync in its hello (older than F6)
// is left alone: its triggers and history stay in its files.
package routinesync

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

// Store is the database side (internal/store/routines).
type Store interface {
	Triggers(ctx context.Context, account string) ([]agent.Trigger, error)
	Recurring(ctx context.Context, account string) ([]agent.Recurring, error)
	ImportTriggers(ctx context.Context, account string, ts []agent.Trigger) (bool, error)
	RecordRun(ctx context.Context, account, agentName string, run agent.Run) (newRun, finished bool, err error)
	// JobAccount is the account of a job; "" when there is none.
	JobAccount(ctx context.Context, jobID string) (string, error)
}

// Hub calls one connected agent (agent.Link).
type Hub interface {
	Route(account string) (agent.Info, error)
	CallAgent(ctx context.Context, agentID, command string, params, into any) error
}

// Audit appends to the Microbot audit (internal/store/microbot).
type Audit interface {
	Append(ctx context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error)
}

// Sync is the server side of the routines.
type Sync struct {
	Store Store
	Hub   Hub
	Audit Audit
	Log   *slog.Logger
	// Wake, when set, tells the event log's consumers that a run started or
	// ended (RecordRun appended run.started or run.ended).
	Wake events.Publisher
}

// OnConnect sends the account's routines to an agent that just linked.
func (s *Sync) OnConnect(info agent.Info) {
	if info.Account == "" || !info.Supports(agent.CmdRoutinesSync) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ts, err := s.Store.Triggers(ctx, info.Account)
	if err != nil {
		s.Log.Warn("routines: read triggers", "account", info.Account, "err", err)
		return
	}
	if len(ts) == 0 && info.Kind == agent.KindAttached && s.importTriggers(ctx, info) {
		if !info.Supports(agent.CmdRecurringGet) {
			return // the agent already holds them
		}
		if ts, err = s.Store.Triggers(ctx, info.Account); err != nil {
			s.Log.Warn("routines: read triggers", "account", info.Account, "err", err)
			return
		}
	}
	var rs []agent.Recurring
	if info.Supports(agent.CmdRecurringGet) {
		if rs, err = s.Store.Recurring(ctx, info.Account); err != nil {
			s.Log.Warn("routines: read recurring routines", "account", info.Account, "err", err)
			return
		}
	}
	if err := s.Hub.CallAgent(ctx, info.ID, agent.CmdRoutinesSync, agent.RoutinesFor(info, ts, rs), nil); err != nil {
		s.Log.Warn("routines: sync", "account", info.Account, "agent", info.ID, "err", err)
		return
	}
	s.Log.Info("routines: sent", "account", info.Account, "agent", info.ID, "triggers", len(ts), "recurring", len(rs))
}

// importTriggers seeds an account with no triggers from an attached agent's
// own file (triggers saved before the server kept them); it reports whether
// it did, in which case the agent already holds them.
func (s *Sync) importTriggers(ctx context.Context, info agent.Info) bool {
	var st agent.TriggerStatus
	if err := s.Hub.CallAgent(ctx, info.ID, agent.CmdTriggerGet, nil, &st); err != nil || len(st.Triggers) == 0 {
		return false
	}
	imported, err := s.Store.ImportTriggers(ctx, info.Account, st.Triggers)
	if err != nil {
		s.Log.Warn("routines: import the agent's triggers", "account", info.Account, "err", err)
		return false
	}
	if imported {
		s.Log.Info("routines: imported the agent's triggers", "account", info.Account, "agent", info.ID, "triggers", len(st.Triggers))
	}
	return imported
}

// OnEvent stores a run an agent reported and audits what its executor did
// by itself.
func (s *Sync) OnEvent(info agent.Info, ev agent.Event) {
	if ev.Run == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := *ev.Run
	account := s.accountOf(ctx, info, run)
	if account == "" {
		return
	}
	newRun, finished, err := s.Store.RecordRun(ctx, account, cmp.Or(info.Worker, info.Host), run)
	switch {
	case errors.Is(err, storeroutines.ErrNotFound):
		s.Log.Debug("routines: run of an unknown account", "account", account)
		return
	case err != nil:
		s.Log.Warn("routines: record run", "account", account, "agent", info.ID, "run", run.ID, "err", err)
		return
	}
	if newRun || finished {
		storejobs.Wake(s.Wake, account)
	}
	var actor string
	switch {
	case s.Audit == nil:
		return
	case run.Trigger != "":
		actor = "trigger:" + run.Trigger
	case run.Routine != "":
		actor = "recurring:" + run.Routine
	default:
		// The owner's runs are audited where they started (run_start, run_cancel).
		return
	}
	params, _ := json.Marshal(map[string]any{"run": run.ID, "className": run.ClassName})
	if newRun {
		s.append(ctx, storemicrobot.Entry{Actor: actor, Origin: "agent", Action: agent.CmdRunStart, Params: params, OK: true, AccountID: account, AgentID: info.ID})
	}
	if finished {
		e := storemicrobot.Entry{Actor: actor, Origin: "agent", Action: "run_end", Params: params, OK: run.State == agent.RunDone, AccountID: account, AgentID: info.ID}
		if !e.OK {
			e.Error = cmp.Or(run.Error, run.State)
		}
		s.append(ctx, e)
	}
}

// accountOf is the account a reported run belongs to: the one the agent
// declares. An agent that declares none (an attached one) is taken at the
// run's word, which the server wrote itself when it started the run: the
// account its wait_until names, or the one of the job it attempts, and only
// when the server routes that account to this very agent (Link.route gives a
// single undeclared agent any account).
func (s *Sync) accountOf(ctx context.Context, info agent.Info, run agent.Run) string {
	if info.Account != "" {
		return info.Account
	}
	var named string
	switch {
	case run.Until != nil:
		named = run.Until.Account
	case run.Task != nil && run.Task.JobID != "":
		var err error
		if named, err = s.Store.JobAccount(ctx, run.Task.JobID); err != nil {
			s.Log.Warn("routines: job of a run", "agent", info.ID, "run", run.ID, "err", err)
		}
	}
	if named == "" {
		return ""
	}
	if routed, err := s.Hub.Route(named); err != nil || routed.ID != info.ID {
		return ""
	}
	return named
}

func (s *Sync) append(ctx context.Context, e storemicrobot.Entry) {
	if _, err := s.Audit.Append(ctx, e); err != nil {
		s.Log.Warn("routines: audit", "action", e.Action, "err", err)
	}
}
