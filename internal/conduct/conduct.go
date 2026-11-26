// Package conduct is the conductor's server side (docs/12 section 12.5;
// FEL-126): for every account whose conductor is on, it dispatches the next
// task of the account's plan as a job, and the next one when the job
// succeeds. It follows the durable event log from its own cursor, like the
// job tracker, and decides with internal/core/conductor on the state it reads
// again each time, so a restart resumes where it stopped and a redelivered
// event changes nothing. Nothing here proves a job: the tracker does.
package conduct

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/conductor"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

// Consumer is the conductor's cursor in the event log.
const Consumer = "conductor"

const (
	defaultInterval = 30 * time.Second
	// retryEvery bounds how often the conductor asks again after a dispatch
	// that had to wait (no agent, the plan waiting): every ask that reaches
	// the agent goes to the audit.
	retryEvery = time.Minute
	batchSize  = 200
)

// States is the conductor's state per account (internal/store/conductor).
type States interface {
	Get(ctx context.Context, account string) (conductor.State, error)
	Accounts(ctx context.Context) ([]string, error)
	Move(ctx context.Context, account string, from, to conductor.State) (bool, error)
}

// Jobs reads the accounts' jobs (internal/store/jobs).
type Jobs interface {
	Get(ctx context.Context, account, id string) (job.Job, error)
	Active(ctx context.Context, account string) ([]job.Job, error)
}

// Log is the event log (internal/store/eventlog).
type Log interface {
	After(ctx context.Context, after int64, limit int) ([]eventlog.Event, error)
	Cursor(ctx context.Context, consumer string) (int64, error)
	SetCursor(ctx context.Context, consumer string, id int64) error
}

// Dispatcher starts the next task of an account's plan on its agent, the way
// POST /api/microbot/task does (internal/httpapi).
type Dispatcher interface {
	DispatchNext(ctx context.Context, account string) conductor.Outcome
}

// Conductor runs the accounts whose conductor is on.
type Conductor struct {
	States   States
	Jobs     Jobs
	Log      Log
	Dispatch Dispatcher
	// Events wakes the conductor at each event of any account; the safety
	// tick covers a nil one.
	Events *events.Hub
	Logr   *slog.Logger
	// Interval is the safety tick (default 30 s).
	Interval time.Duration
	// Now is the clock; nil is time.Now.
	Now func() time.Time

	// waited is when each account's last dispatch had to wait.
	waited map[string]time.Time
}

func (c *Conductor) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Conductor) log() *slog.Logger { return cmp.Or(c.Logr, slog.Default()) }

// Run steps until ctx ends: at each event of the hub (that account), and at
// the safety tick (every account the conductor is on for).
func (c *Conductor) Run(ctx context.Context) {
	var wake <-chan events.Event
	var sub *events.Subscription
	subscribe := func() {
		if c.Events != nil {
			sub = c.Events.SubscribeAll()
			wake = sub.C
		}
	}
	subscribe()
	defer func() {
		if sub != nil {
			sub.Close()
		}
	}()
	tick := time.NewTicker(cmp.Or(c.Interval, defaultInterval))
	defer tick.Stop()
	all, woken := true, map[string]bool{}
	for {
		c.Step(ctx, all, woken)
		all, woken = false, map[string]bool{}
		select {
		case <-ctx.Done():
			return
		case ev, open := <-wake:
			if !open { // dropped as a slow subscriber
				subscribe()
				all = true
			} else if ev.Account != "" {
				woken[ev.Account] = true
			}
		case <-tick.C:
			all = true
		}
	}
}

// Step reads the log after the cursor and steps every account a job event
// names, the woken ones and, with all, every account the conductor is on for.
func (c *Conductor) Step(ctx context.Context, all bool, woken map[string]bool) {
	dirty := map[string]bool{}
	for a := range woken {
		dirty[a] = true
	}
	if err := c.drain(ctx, dirty); err != nil && ctx.Err() == nil {
		c.log().Warn("conductor: events", "err", err)
	}
	if all {
		accounts, err := c.States.Accounts(ctx)
		if err != nil {
			c.log().Warn("conductor: accounts", "err", err)
		}
		for _, a := range accounts {
			dirty[a] = true
		}
	}
	accounts := make([]string, 0, len(dirty))
	for a := range dirty {
		accounts = append(accounts, a)
	}
	slices.Sort(accounts)
	for _, a := range accounts {
		c.step(ctx, a)
	}
}

// drain moves the cursor over the log, noting the accounts of the job events.
// The state is read again when the account steps, so an event handled twice
// after a crash changes nothing.
func (c *Conductor) drain(ctx context.Context, dirty map[string]bool) error {
	cursor, err := c.Log.Cursor(ctx, Consumer)
	if err != nil {
		return err
	}
	for {
		batch, err := c.Log.After(ctx, cursor, batchSize)
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, ev := range batch {
			switch ev.Type {
			case eventlog.TypeJobSucceeded, eventlog.TypeJobFailed, eventlog.TypeJobCanceled:
				dirty[ev.Account] = true
			}
		}
		cursor = batch[len(batch)-1].ID
		if err := c.Log.SetCursor(ctx, Consumer, cursor); err != nil {
			return err
		}
		if len(batch) < batchSize {
			return nil
		}
	}
}

// step decides for one account and acts: a move of its state, a dispatch.
func (c *Conductor) step(ctx context.Context, account string) {
	st, err := c.States.Get(ctx, account)
	if err != nil {
		c.log().Warn("conductor: state", "account", account, "err", err)
		return
	}
	if st.Mode != conductor.Idle && st.Mode != conductor.Running {
		return
	}
	seen, err := c.seen(ctx, account, st)
	if err != nil {
		c.log().Warn("conductor: jobs", "account", account, "err", err)
		return
	}
	d := conductor.Decide(st, seen)
	if d.State != st {
		if !c.move(ctx, account, st, d.State) {
			return
		}
		st = d.State
	}
	if !d.Dispatch {
		return
	}
	if at, ok := c.waited[account]; ok && st.Reason != "" && c.now().Sub(at) < retryEvery {
		return
	}
	out := c.Dispatch.DispatchNext(ctx, account)
	if out.Kind == conductor.Wait {
		if c.waited == nil {
			c.waited = map[string]time.Time{}
		}
		c.waited[account] = c.now()
	} else {
		delete(c.waited, account)
	}
	next := conductor.Settle(st, out)
	if next == st {
		return
	}
	if !c.move(ctx, account, st, next) && out.Kind == conductor.Dispatched {
		// The owner turned the conductor off while the task was sent: the
		// job goes on, as one the owner started.
		c.log().Info("conductor: dispatched after the owner changed the state", "account", account, "job", out.Job)
	}
}

// seen reads the conductor's job and whether another job is active.
func (c *Conductor) seen(ctx context.Context, account string, st conductor.State) (conductor.Seen, error) {
	var seen conductor.Seen
	if st.Job != "" {
		j, err := c.Jobs.Get(ctx, account, st.Job)
		switch {
		case err == nil:
			seen.Job = &j
		case !errors.Is(err, storejobs.ErrNotFound):
			return seen, err
		}
	}
	active, err := c.Jobs.Active(ctx, account)
	if err != nil {
		return seen, err
	}
	for _, j := range active {
		if j.ID != st.Job {
			seen.OtherActive = true
		}
	}
	return seen, nil
}

func (c *Conductor) move(ctx context.Context, account string, from, to conductor.State) bool {
	ok, err := c.States.Move(ctx, account, from, to)
	if err != nil {
		c.log().Warn("conductor: move", "account", account, "err", err)
		return false
	}
	if ok {
		c.log().Info("conductor", "account", account, "from", from.Mode, "to", to.Mode, "job", to.Job, "reason", to.Reason)
	}
	return ok
}
