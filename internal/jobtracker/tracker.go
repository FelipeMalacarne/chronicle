// Package jobtracker is the server side of the jobs (docs/12 section 13.5):
// it follows the durable event log, evaluates the goal and the metric of each
// active job on the account's facts, and is the only piece that ends a job as
// succeeded, always from the ingest, never from the plugin or the agent.
//
// It consumes fact.changed, run.started and run.ended from the log, from its
// own cursor, so a restart resumes where it stopped and a redelivered event
// changes nothing (each handler reads the state again). Besides the events it
// has a deadline timer (a running job with a metric stalls when the metric
// does not change for stall_after) and a safety tick that evaluates the active
// jobs of every account, which covers the facts that are not logged as
// changed (the position). When a job succeeds while its run still goes, the
// tracker sends the agent run_until, which stops the run; it is how the old
// wait_until watcher (internal/rununtil) proves a train or a quest, now for
// every job. It also writes membership.low from the membership fact.
//
// Nothing here touches the game: the agent stops the plugin.
package jobtracker

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
)

// Consumer is the tracker's name in event_cursors.
const Consumer = "job_tracker"

// Actor is the audit's actor of the tracker's run_until.
const Actor = "job_tracker"

// MembershipLowDays is the days left of membership at which membership.low is
// written.
const MembershipLowDays = 7

const (
	defaultInterval = 30 * time.Second
	batchSize       = 200
)

// Jobs is the jobs' store (internal/store/jobs).
type Jobs interface {
	Active(ctx context.Context, account string) ([]job.Job, error)
	ActiveAccounts(ctx context.Context) ([]string, error)
	Start(ctx context.Context, id, reason string, progress *job.Progress, deadline *time.Time) (job.Job, error)
	Stall(ctx context.Context, id, reason string) (job.Job, error)
	Succeed(ctx context.Context, id string, proof job.Proof) (job.Job, error)
	Progress(ctx context.Context, id string, p job.Progress, deadline *time.Time) error
	Due(ctx context.Context, now time.Time) ([]job.Job, error)
	NextDeadline(ctx context.Context) (time.Time, bool, error)
	RunsToStop(ctx context.Context) ([]storejobs.RunToStop, error)
}

// Log is the event log (internal/store/eventlog).
type Log interface {
	After(ctx context.Context, after int64, limit int) ([]eventlog.Event, error)
	Cursor(ctx context.Context, consumer string) (int64, error)
	SetCursor(ctx context.Context, consumer string, id int64) error
	Write(ctx context.Context, account, typ, subject string, payload any) (eventlog.Event, error)
}

// Facts reads an account's facts (internal/store/progress).
type Facts interface {
	TaskFacts(ctx context.Context, account string) (panel.Facts, error)
	Membership(ctx context.Context, account string) (days int, member, ok bool, err error)
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

// Tracker evaluates the active jobs on the event log.
type Tracker struct {
	Jobs     Jobs
	Log      Log
	Facts    Facts
	Registry *entities.Registry
	Hub      Hub
	// Events wakes the tracker at each event of any account; the safety tick
	// covers a nil one.
	Events *events.Hub
	// Audit, when set, records each run_until sent.
	Audit Audit
	Logr  *slog.Logger
	// Interval is the safety tick (default 30 s).
	Interval time.Duration
	// Now is the clock; nil is time.Now.
	Now func() time.Time

	// sent holds the runs the agent was already told to stop, until their run
	// is final: a report in between does not tell it again.
	sent map[string]time.Time
}

func (t *Tracker) now() time.Time {
	if t.Now != nil {
		return t.Now().UTC()
	}
	return time.Now().UTC()
}

func (t *Tracker) log() *slog.Logger {
	if t.Logr == nil {
		return slog.New(slog.DiscardHandler)
	}
	return t.Logr
}

// Run follows the log until ctx ends: it handles what is pending, then waits
// for a wake, the next deadline or the safety tick.
func (t *Tracker) Run(ctx context.Context) {
	var wake <-chan events.Event
	var sub *events.Subscription
	subscribe := func() {
		if t.Events != nil {
			sub = t.Events.SubscribeAll()
			wake = sub.C
		}
	}
	subscribe()
	defer func() {
		if sub != nil {
			sub.Close()
		}
	}()
	tick := time.NewTicker(cmp.Or(t.Interval, defaultInterval))
	defer tick.Stop()
	evaluateAll := true // after a start, every active job is evaluated once
	for {
		t.Step(ctx, evaluateAll)
		evaluateAll = false
		timer := time.NewTimer(t.untilDeadline(ctx))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case _, open := <-wake:
			if !open { // dropped as a slow subscriber: subscribe again and look at everything
				subscribe()
				evaluateAll = true
			}
		case <-timer.C:
		case <-tick.C:
			evaluateAll = true
		}
		timer.Stop()
	}
}

// untilDeadline is how long to wait for the first running job's deadline,
// bounded by the safety tick.
func (t *Tracker) untilDeadline(ctx context.Context) time.Duration {
	limit := cmp.Or(t.Interval, defaultInterval)
	at, ok, err := t.Jobs.NextDeadline(ctx)
	if err != nil || !ok {
		return limit
	}
	return min(max(at.Sub(t.now()), 0)+time.Millisecond, limit)
}

// Step is one pass: the events after the cursor, the jobs past their
// deadline, and the runs whose job is proven. With all, it also evaluates
// the active jobs of every account.
func (t *Tracker) Step(ctx context.Context, all bool) {
	dirty := map[string]bool{}
	if err := t.drain(ctx, dirty); err != nil && ctx.Err() == nil {
		t.log().Warn("jobs: events", "err", err)
		return // the cursor did not move: the next pass reads them again
	}
	if all {
		accounts, err := t.Jobs.ActiveAccounts(ctx)
		if err != nil {
			t.log().Warn("jobs: active accounts", "err", err)
		}
		for _, a := range accounts {
			dirty[a] = true
		}
	}
	t.stallDue(ctx, dirty)
	for account := range dirty {
		t.evaluate(ctx, account)
	}
	t.stopProvenRuns(ctx)
}

// drain handles the events after the cursor in batches and moves the cursor
// after each. Accounts whose facts changed are left in dirty for the caller.
func (t *Tracker) drain(ctx context.Context, dirty map[string]bool) error {
	cursor, err := t.Log.Cursor(ctx, Consumer)
	if err != nil {
		return err
	}
	for {
		batch, err := t.Log.After(ctx, cursor, batchSize)
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, ev := range batch {
			if err := t.handle(ctx, ev, dirty); err != nil {
				return err
			}
		}
		// The effects of the batch come before its cursor: after a crash the
		// events are handled again, which changes nothing.
		for account := range dirty {
			t.evaluate(ctx, account)
			delete(dirty, account)
		}
		cursor = batch[len(batch)-1].ID
		if err := t.Log.SetCursor(ctx, Consumer, cursor); err != nil {
			return err
		}
		if len(batch) < batchSize {
			return nil
		}
	}
}

// payload is what the tracker reads of an event.
type payload struct {
	Job   string              `json:"job"`
	State string              `json:"state"`
	Facts map[string][]string `json:"facts"`
}

func (t *Tracker) handle(ctx context.Context, ev eventlog.Event, dirty map[string]bool) error {
	var p payload
	_ = json.Unmarshal(ev.Payload, &p)
	switch ev.Type {
	case eventlog.TypeFactChanged:
		dirty[ev.Account] = true
		if _, ok := p.Facts["membership"]; ok {
			return t.membership(ctx, ev.Account)
		}
	case eventlog.TypeRunStarted:
		if p.Job != "" {
			t.runStarted(ctx, ev.Account, p.Job)
			dirty[ev.Account] = true
		}
	case eventlog.TypeRunEnded:
		delete(t.sent, ev.Subject)
		if p.Job != "" {
			t.runEnded(ctx, ev.Account, p.Job, p.State)
		}
	}
	return nil
}

// membership writes membership.low when the days left fell to the threshold.
func (t *Tracker) membership(ctx context.Context, account string) error {
	days, member, ok, err := t.Facts.Membership(ctx, account)
	if err != nil {
		return err
	}
	if !ok || !member || days > MembershipLowDays {
		return nil
	}
	_, err = t.Log.Write(ctx, account, eventlog.TypeMembershipLow, "", map[string]int{"days": days})
	return err
}

// find is the account's active job with an id.
func (t *Tracker) find(ctx context.Context, account, id string) (job.Job, bool) {
	jobs, err := t.Jobs.Active(ctx, account)
	if err != nil {
		t.log().Warn("jobs: read", "account", account, "err", err)
		return job.Job{}, false
	}
	for _, j := range jobs {
		if j.ID == id {
			return j, true
		}
	}
	return job.Job{}, false
}

// runStarted moves a queued job, or a stalled one given a new run, to running,
// with the metric's value as the run begins.
func (t *Tracker) runStarted(ctx context.Context, account, id string) {
	j, ok := t.find(ctx, account, id)
	if !ok || (j.State != job.Queued && j.State != job.Stalled) {
		return
	}
	now := t.now()
	var progress *job.Progress
	if f, err := t.Facts.TaskFacts(ctx, account); err == nil && f.State != nil && j.Metric != nil {
		progress, _ = job.Observe(*j.Metric, nil, *f.State, now)
	}
	deadline := j.DeadlineFrom(now)
	if _, err := t.Jobs.Start(ctx, id, "run started", progress, deadline); err != nil && !errors.Is(err, storejobs.ErrState) {
		t.log().Warn("jobs: start", "job", id, "err", err)
	}
}

// runEnded stalls a running job whose run ended before its goal was proven
// on the facts as they are now.
func (t *Tracker) runEnded(ctx context.Context, account, id, state string) {
	t.evaluate(ctx, account)
	j, ok := t.find(ctx, account, id)
	if !ok || j.State != job.Running {
		return
	}
	if _, err := t.Jobs.Stall(ctx, id, "run ended: "+cmp.Or(state, "unknown")); err != nil && !errors.Is(err, storejobs.ErrState) {
		t.log().Warn("jobs: stall", "job", id, "err", err)
	}
}

// evaluate looks at the account's running and stalled jobs on its facts now:
// a goal met on live facts ends the job, a changed metric is its progress.
func (t *Tracker) evaluate(ctx context.Context, account string) {
	jobs, err := t.Jobs.Active(ctx, account)
	if err != nil || !hasEvaluable(jobs) {
		if err != nil {
			t.log().Warn("jobs: read", "account", account, "err", err)
		}
		return
	}
	facts, err := t.Facts.TaskFacts(ctx, account)
	if err != nil || facts.State == nil {
		if err != nil && ctx.Err() == nil {
			t.log().Debug("jobs: facts", "account", account, "err", err)
		}
		return
	}
	now := t.now()
	for _, j := range jobs {
		if j.State == job.Queued {
			continue // a job is evaluated once a run attempts it
		}
		res := j.Goal.Evaluate(job.Env{Facts: facts, Registry: t.Registry, Now: now})
		if proof, ok := res.Prove(); ok {
			if _, err := t.Jobs.Succeed(ctx, j.ID, proof); err != nil && !errors.Is(err, storejobs.ErrState) {
				t.log().Warn("jobs: succeed", "job", j.ID, "err", err)
			}
			continue
		}
		if j.State != job.Running || j.Metric == nil {
			continue
		}
		if p, changed := job.Observe(*j.Metric, j.Progress, *facts.State, now); changed {
			if err := t.Jobs.Progress(ctx, j.ID, *p, j.DeadlineFrom(p.At)); err != nil && !errors.Is(err, storejobs.ErrState) {
				t.log().Warn("jobs: progress", "job", j.ID, "err", err)
			}
		}
	}
}

func hasEvaluable(jobs []job.Job) bool {
	for _, j := range jobs {
		if j.State != job.Queued {
			return true
		}
	}
	return false
}

// stallDue stalls the running jobs whose metric did not change for their
// stall_after. It looks at the facts of their accounts first: a change the
// events have not delivered yet is progress, not a stall.
func (t *Tracker) stallDue(ctx context.Context, dirty map[string]bool) {
	due, err := t.Jobs.Due(ctx, t.now())
	if err != nil || len(due) == 0 {
		if err != nil {
			t.log().Warn("jobs: deadlines", "err", err)
		}
		return
	}
	for _, j := range due {
		t.evaluate(ctx, j.Account)
		delete(dirty, j.Account)
	}
	still, err := t.Jobs.Due(ctx, t.now())
	if err != nil {
		return
	}
	for _, j := range still {
		if _, err := t.Jobs.Stall(ctx, j.ID, "no progress for "+j.StallAfter.String()); err != nil && !errors.Is(err, storejobs.ErrState) {
			t.log().Warn("jobs: stall", "job", j.ID, "err", err)
		}
	}
}

// stopProvenRuns tells the agent to stop each run still going whose job the
// tracker proved (run_until), once; a refused one is not asked again, an
// absent agent is asked again at the next pass.
func (t *Tracker) stopProvenRuns(ctx context.Context) {
	if t.Hub == nil {
		return // no control module: no agent to tell
	}
	runs, err := t.Jobs.RunsToStop(ctx)
	if err != nil {
		t.log().Warn("jobs: runs to stop", "err", err)
		return
	}
	if t.sent == nil {
		t.sent = map[string]time.Time{}
	}
	live := map[string]bool{}
	for _, r := range runs {
		live[r.RunUID] = true
	}
	for uid := range t.sent {
		if !live[uid] {
			delete(t.sent, uid)
		}
	}
	for _, r := range runs {
		key, ok := r.Goal.ObjectiveKey()
		if !ok || !t.sent[r.RunUID].IsZero() {
			continue // only an objective key is a wait_until the agent knows
		}
		t.sendRunUntil(ctx, r, key)
	}
}

func (t *Tracker) sendRunUntil(ctx context.Context, r storejobs.RunToStop, key entities.ObjectiveKey) {
	served, err := t.Hub.Route(r.Account)
	if err != nil || !served.Supports(agent.CmdRunUntil) {
		return // the agent links again and reports the run again
	}
	p := agent.RunUntilParams{UID: r.RunUID, Objective: key.String()}
	if f, ferr := t.Facts.TaskFacts(ctx, r.Account); ferr == nil {
		p.Evidence = r.Goal.Evaluate(job.Env{Facts: f, Registry: t.Registry, Now: t.now()}).Evidence
	}
	// The agent may end the run before its answer arrives: the call outlives
	// the pass (the server's context and its own timeout bound it).
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err = t.Hub.CallAgent(callCtx, served.ID, agent.CmdRunUntil, p, nil)
	t.audit(callCtx, r, served, p, err)
	var refused *agent.CommandError
	switch {
	case err == nil:
		t.sent[r.RunUID] = t.now()
		t.log().Info("jobs: run stopped, goal proven", "account", r.Account, "job", r.JobID, "run", r.RunUID, "evidence", p.Evidence)
	case errors.As(err, &refused):
		// The agent has no such run waiting (it ended, or another replaced
		// it): nothing left to tell it.
		t.sent[r.RunUID] = t.now()
	default:
		t.log().Warn("jobs: run_until", "account", r.Account, "run", r.RunUID, "err", err)
	}
}

func (t *Tracker) audit(ctx context.Context, r storejobs.RunToStop, served agent.Info, p agent.RunUntilParams, err error) {
	if t.Audit == nil {
		return
	}
	params, _ := json.Marshal(p)
	e := storemicrobot.Entry{
		Actor: Actor, Origin: "server", Action: agent.CmdRunUntil, Params: params, OK: err == nil,
		AccountID: r.Account, AgentID: served.ID,
	}
	if err != nil {
		e.Error = err.Error()
	}
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, aerr := t.Audit.Append(actx, e); aerr != nil {
		t.log().Warn("jobs: audit", "err", aerr)
	}
}
