// Package routinesync keeps the agents' routines and the server's in step
// (docs/11 section 4.4, phase F6). The executor stays in the agent (docs/08,
// R1); the database is the source of the triggers and the record of the
// runs:
//
//   - on every connection the agent gets the account's triggers
//     (routines_sync), so a worker's pod, which loses its disk, keeps them;
//     an attached agent whose account has none yet seeds them from its own
//     file once;
//   - the agent sends each run as it changes (a TypeEvent), which is stored
//     with its trail; what the executor did by itself (a run a trigger
//     started, and how it ended) goes to the Microbot audit.
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
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

// Store is the database side (internal/store/routines).
type Store interface {
	Triggers(ctx context.Context, account string) ([]agent.Trigger, error)
	ImportTriggers(ctx context.Context, account string, ts []agent.Trigger) (bool, error)
	RecordRun(ctx context.Context, account, agentName string, run agent.Run) (newRun, finished bool, err error)
}

// Hub calls one connected agent (agent.Link).
type Hub interface {
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
}

// OnConnect sends the account's triggers to an agent that just linked.
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
		return
	}
	if err := s.Hub.CallAgent(ctx, info.ID, agent.CmdRoutinesSync, agent.RoutinesParams{Triggers: ts}, nil); err != nil {
		s.Log.Warn("routines: sync triggers", "account", info.Account, "agent", info.ID, "err", err)
		return
	}
	s.Log.Info("routines: triggers sent", "account", info.Account, "agent", info.ID, "triggers", len(ts))
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
	if ev.Run == nil || info.Account == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := *ev.Run
	newRun, finished, err := s.Store.RecordRun(ctx, info.Account, cmp.Or(info.Worker, info.Host), run)
	switch {
	case errors.Is(err, storeroutines.ErrNotFound):
		s.Log.Debug("routines: run of an unknown account", "account", info.Account)
		return
	case err != nil:
		s.Log.Warn("routines: record run", "account", info.Account, "agent", info.ID, "run", run.ID, "err", err)
		return
	}
	if run.Trigger == "" || s.Audit == nil {
		// The owner's runs are audited where they started (run_start, run_cancel).
		return
	}
	actor := "trigger:" + run.Trigger
	params, _ := json.Marshal(map[string]any{"run": run.ID, "className": run.ClassName})
	if newRun {
		s.append(ctx, storemicrobot.Entry{Actor: actor, Origin: "agent", Action: agent.CmdRunStart, Params: params, OK: true, AccountID: info.Account, AgentID: info.ID})
	}
	if finished {
		e := storemicrobot.Entry{Actor: actor, Origin: "agent", Action: "run_end", Params: params, OK: run.State == agent.RunDone, AccountID: info.Account, AgentID: info.ID}
		if !e.OK {
			e.Error = cmp.Or(run.Error, run.State)
		}
		s.append(ctx, e)
	}
}

func (s *Sync) append(ctx context.Context, e storemicrobot.Entry) {
	if _, err := s.Audit.Append(ctx, e); err != nil {
		s.Log.Warn("routines: audit", "action", e.Action, "err", err)
	}
}
