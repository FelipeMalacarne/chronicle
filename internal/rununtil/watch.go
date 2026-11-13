// Package rununtil is the server side of a run's wait_until (docs/10 phase
// 4; docs/08, runs and routines): the agent runs a plugin until an
// objective key of the account is met, and the server is who says it is met,
// from the ingest (the account's merged facts), never the plugin or the
// agent.
//
// The agent reports every run as it changes (a TypeEvent, docs/11 section
// 4.4), so a run that waits for a key reaches the watcher with it
// (agent.Run.Until). The watcher follows each such run: it evaluates the key
// on the account's facts when the run appears, after each progress event of
// the account (an ingest, a hiscores sync, an owner's mark) and every
// Interval as a safety net; once the verdict is met, on a live fact, it
// sends run_until to the agent that serves the account, audited as the
// server's own action. A final run, a run the agent no longer has and a run
// past the longest wait_until stop the watch. Nothing here touches the game:
// the agent stops the plugin.
package rununtil

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
)

// Facts reads an account's facts (internal/store/progress.TaskFacts).
type Facts interface {
	TaskFacts(ctx context.Context, account string) (panel.Facts, error)
}

// Hub reaches the agent that serves an account (agent.Link).
type Hub interface {
	Route(account string) (agent.Info, error)
	CallAgent(ctx context.Context, agentID, command string, params, into any) error
}

// Audit appends to the Microbot audit (internal/store/microbot).
type Audit interface {
	Append(ctx context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error)
}

// Actor is the audit's actor of the watcher's run_until.
const Actor = "wait_until"

const (
	defaultInterval = 30 * time.Second
	// maxAge drops a watch past the longest wait_until a run may take (the
	// agent's 12 h) with margin: its agent is gone for good.
	maxAge = 13 * time.Hour
	// maxWatches bounds the runs followed at once (one per agent).
	maxWatches = 256
)

// Watcher follows the runs that wait for an objective.
type Watcher struct {
	Facts    Facts
	Registry *entities.Registry
	Hub      Hub
	// Events, when set, wakes a watch at each event of its account.
	Events *events.Hub
	// Audit, when set, records each run_until sent.
	Audit Audit
	Log   *slog.Logger
	// Interval is the safety net between evaluations (default 30 s).
	Interval time.Duration

	mu   sync.Mutex
	ctx  context.Context
	runs map[string]*watch // by run uid
}

// watch is one run being followed.
type watch struct {
	uid     string
	account string
	key     entities.ObjectiveKey
	since   time.Time
	// sentAt is when the proof went to the agent; the entry stays until the
	// run's final event (or sentKeep), so a report in between does not
	// start the watch again.
	sentAt time.Time
	cancel context.CancelFunc
}

// sentKeep is how long a proven watch waits for its run's final event.
const sentKeep = time.Hour

// Start sets the context the watches live in (the server's): they end with
// it. Before Start they live in context.Background.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctx = ctx
}

// Watching lists the uids of the runs followed now (tests, diagnostics).
func (w *Watcher) Watching() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.runs))
	for uid := range w.runs {
		out = append(out, uid)
	}
	return out
}

func (w *Watcher) log() *slog.Logger {
	if w.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return w.Log
}

// OnEvent follows a reported run that waits for an objective, and stops
// following it once it is final. The account is the one the agent declares;
// an agent that declares none (an attached one) is taken at the run's word,
// which the server itself wrote when it started or stored the run.
func (w *Watcher) OnEvent(info agent.Info, ev agent.Event) {
	run := ev.Run
	if run == nil || run.Until == nil || agent.ValidateReport(*run) != nil {
		return
	}
	if run.Final() {
		w.drop(run.UID)
		return
	}
	account := info.Account
	switch {
	case account == "":
		account = run.Until.Account
	case run.Until.Account != "" && run.Until.Account != account:
		w.log().Warn("wait_until: a run of another account", "agent", info.ID, "declared", account, "run", run.UID)
		return
	}
	if account == "" {
		w.log().Warn("wait_until: a run with no account to prove it on", "agent", info.ID, "run", run.UID)
		return
	}
	key, err := run.Until.ObjectiveKey()
	if err != nil {
		return
	}
	if w.Registry != nil {
		if msg := w.Registry.KnownObjective(key); msg != "" {
			w.log().Warn("wait_until: an objective the catalog does not know", "run", run.UID, "objective", run.Until.Objective, "why", msg)
			return
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.runs == nil {
		w.runs = map[string]*watch{}
	}
	for uid, wt := range w.runs {
		// A run whose agent never came back, or whose end never came.
		if time.Since(wt.since) > maxAge || (!wt.sentAt.IsZero() && time.Since(wt.sentAt) > sentKeep) {
			wt.cancel()
			delete(w.runs, uid)
		}
	}
	if old, ok := w.runs[run.UID]; ok {
		if old.account == account && old.key == key {
			return
		}
		old.cancel() // the same run cannot change its key; follow what it says now
	}
	if len(w.runs) >= maxWatches {
		w.log().Warn("wait_until: too many runs followed", "run", run.UID)
		return
	}
	base := w.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	wt := &watch{uid: run.UID, account: account, key: key, since: time.Now(), cancel: cancel}
	w.runs[run.UID] = wt
	go w.follow(ctx, wt)
}

// drop stops following a run.
func (w *Watcher) drop(uid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if wt, ok := w.runs[uid]; ok {
		wt.cancel()
		delete(w.runs, uid)
	}
}

// follow is a watch's goroutine: an evaluation now, at each event of the
// account and every Interval, until the proof is sent or the watch ends.
func (w *Watcher) follow(ctx context.Context, wt *watch) {
	var events <-chan events.Event
	if w.Events != nil {
		sub := w.Events.Subscribe(wt.account)
		defer sub.Close()
		events = sub.C
	}
	t := time.NewTicker(cmp.Or(w.Interval, defaultInterval))
	defer t.Stop()
	for {
		if w.check(ctx, wt) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case _, open := <-events:
			if !open {
				events = nil // dropped as a slow subscriber: the ticker goes on
			}
		case <-t.C:
		}
	}
}

// check evaluates the watch's key once and sends the proof when it is met;
// true when the watch is over.
func (w *Watcher) check(ctx context.Context, wt *watch) bool {
	if ctx.Err() != nil {
		return true
	}
	if time.Since(wt.since) > maxAge {
		w.drop(wt.uid)
		return true
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	facts, err := w.Facts.TaskFacts(cctx, wt.account)
	if err != nil || facts.State == nil {
		if err != nil && ctx.Err() == nil {
			w.log().Debug("wait_until: facts", "account", wt.account, "err", err)
		}
		return false
	}
	ev := progress.EvaluateObjective(wt.key, *facts.State, w.Registry)
	if !Proven(ev) {
		return false
	}
	served, err := w.Hub.Route(wt.account)
	if err != nil || !served.Supports(agent.CmdRunUntil) {
		return false // the agent links again and reports the run again
	}
	p := agent.RunUntilParams{UID: wt.uid, Objective: wt.key.String(), Evidence: Evidence(ev)}
	err = w.Hub.CallAgent(cctx, served.ID, agent.CmdRunUntil, p, nil)
	w.audit(ctx, wt, served, p, err)
	var refused *agent.CommandError
	switch {
	case err == nil:
		w.mu.Lock()
		wt.sentAt = time.Now()
		w.mu.Unlock()
		w.log().Info("wait_until: proven", "account", wt.account, "run", wt.uid, "objective", p.Objective, "evidence", p.Evidence)
		return true // the run's final event drops the watch
	case errors.As(err, &refused):
		// The agent has no such run waiting (it ended, or another one
		// replaced it): nothing left to prove.
		w.drop(wt.uid)
		return true
	}
	w.log().Warn("wait_until: send", "account", wt.account, "run", wt.uid, "err", err)
	return false
}

func (w *Watcher) audit(ctx context.Context, wt *watch, served agent.Info, p agent.RunUntilParams, err error) {
	if w.Audit == nil {
		return
	}
	params, _ := json.Marshal(p)
	e := storemicrobot.Entry{
		Actor: Actor, Origin: "server", Action: agent.CmdRunUntil, Params: params, OK: err == nil,
		AccountID: wt.account, AgentID: served.ID,
	}
	if err != nil {
		e.Error = err.Error()
	}
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, aerr := w.Audit.Append(actx, e); aerr != nil {
		w.log().Warn("wait_until: audit", "err", aerr)
	}
}

// Proven reports whether an evaluation proves its key for a wait_until: met,
// and on live facts only (an item met on a restored bank mirror is not
// proof enough to stop a run, CLAUDE.md rule 8).
func Proven(ev progress.Evaluation) bool {
	if ev.Verdict != rules.Met {
		return false
	}
	for _, e := range ev.Evidence {
		if e.Stale {
			return false
		}
	}
	return true
}

// Evidence is what the evaluation observed, for the run's trail ("nível
// 76"); empty when it says nothing short.
func Evidence(ev progress.Evaluation) string {
	var parts []string
	for _, e := range ev.Evidence {
		switch v := e.Observed.(type) {
		case nil:
		case rules.SkillState:
			parts = append(parts, fmt.Sprintf("nível %d", v.Level))
		case string:
			parts = append(parts, "observado "+v)
		default:
			if b, err := json.Marshal(v); err == nil {
				parts = append(parts, "observado "+string(b))
			}
		}
	}
	out := strings.Join(parts, "; ")
	if len(out) > 200 {
		out = out[:200]
		for !utf8.ValidString(out) {
			out = out[:len(out)-1]
		}
	}
	return out
}
