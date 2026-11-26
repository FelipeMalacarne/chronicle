// Package job is the pure model of a job (docs/12 section 13): an intent with
// a state and a measured progress, which executes and ends. It holds the state
// machine, the goal the rule engine evaluates on the account's facts, the
// metric that measures the progress and the stall rule. It has no IO and does
// not read the clock; the tracker (internal/jobtracker) and the store
// (internal/store/jobs) drive it.
package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Kind is what a job makes the account do.
type Kind string

// Kinds (docs/12 section 13.2). The execspec task types are the first seven;
// Plugin runs a plugin of the client's profile.
const (
	KindPlugin Kind = "plugin"
	KindTrain  Kind = "train"
	KindQuest  Kind = "quest"
	KindTravel Kind = "travel"
	KindBank   Kind = "bank"
	KindBuy    Kind = "buy"
	KindGEBuy  Kind = "ge_buy"
	KindGESell Kind = "ge_sell"
)

// Kinds lists every kind.
var Kinds = []Kind{KindPlugin, KindTrain, KindQuest, KindTravel, KindBank, KindBuy, KindGEBuy, KindGESell}

// Valid reports whether k is a kind.
func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

// State is where a job is in its life (docs/12 section 13.3).
type State string

// States.
const (
	Queued    State = "queued"
	Running   State = "running"
	Stalled   State = "stalled"
	Succeeded State = "succeeded"
	Failed    State = "failed"
	Canceled  State = "canceled"
)

// Active reports whether the job can still change: it is not final.
func (s State) Active() bool { return s == Queued || s == Running || s == Stalled }

// Final reports whether the job ended.
func (s State) Final() bool { return s == Succeeded || s == Failed || s == Canceled }

// transitions is the state machine of docs/12 section 13.3, one row per
// legal move. Stalled to Succeeded is the one the table leaves implicit: a
// run that ended before the ingest showed the goal stalls the job, and the
// fact that proves it can come after.
var transitions = map[State][]State{
	Queued:  {Running, Canceled},
	Running: {Succeeded, Stalled, Failed, Canceled},
	Stalled: {Running, Succeeded, Failed, Canceled},
}

// ErrIllegalTransition is a move the state machine does not have.
var ErrIllegalTransition = errors.New("job: illegal state transition")

// Check says whether a job may go from one state to another.
func Check(from, to State) error {
	if !slices.Contains(transitions[from], to) {
		return fmt.Errorf("%w: %s to %s", ErrIllegalTransition, from, to)
	}
	return nil
}

// Source is who created a job.
type Source string

// Sources. Routine is a job adopted from a run a trigger or a recurring
// routine (or the owner's "Fazer agora" with an objective) started.
const (
	SourceConductor Source = "conductor"
	SourceOwner     Source = "owner"
	SourceRoutine   Source = "routine"
	SourceTest      Source = "test"
)

// Valid reports whether s is a source.
func (s Source) Valid() bool {
	return s == SourceConductor || s == SourceOwner || s == SourceRoutine || s == SourceTest
}

// Job is one job as the store holds it.
type Job struct {
	ID      string
	Account string
	Kind    Kind
	// Spec is the typed task the agent executes (internal/core/execspec).
	Spec  json.RawMessage
	Goal  Goal
	State State
	// Reason says why the job is in its state ("goal met", "run ended: failed").
	Reason     string
	Source     Source
	Metric     *Metric
	Progress   *Progress
	StallAfter time.Duration
	// Plan and TaskKey name the plan task a job executes, when one does.
	Plan, TaskKey string
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
	// Deadline is when a running job with a metric stalls.
	Deadline *time.Time
}

// stallDefaults is how long without progress counts as stalled, by kind: a
// walk or a bank trip is minutes, a Grand Exchange offer waits for the market,
// a training or a quest legitimately goes a long time between changes.
var stallDefaults = map[Kind]time.Duration{
	KindTravel: 5 * time.Minute,
	KindBank:   5 * time.Minute,
	KindBuy:    5 * time.Minute,
	KindGEBuy:  15 * time.Minute,
	KindGESell: 15 * time.Minute,
	KindTrain:  30 * time.Minute,
	KindQuest:  30 * time.Minute,
	KindPlugin: 30 * time.Minute,
}

// DefaultStallAfter is the stall_after of a kind.
func DefaultStallAfter(k Kind) time.Duration {
	if d, ok := stallDefaults[k]; ok {
		return d
	}
	return 30 * time.Minute
}

// Stall bounds, matching the table's check.
const (
	MinStallAfter = 30 * time.Second
	MaxStallAfter = 24 * time.Hour
)

// New is a job of the account in the queue: the goal is parsed, the metric is
// the goal's own when none is given, and a zero stallAfter takes the kind's
// default. The caller sets ID and the timestamps.
func New(account string, kind Kind, spec json.RawMessage, goal Goal, metric *Metric, stallAfter time.Duration, source Source) (Job, error) {
	switch {
	case account == "":
		return Job{}, errors.New("job: no account")
	case !kind.Valid():
		return Job{}, fmt.Errorf("job: unknown kind %q", kind)
	case !source.Valid():
		return Job{}, fmt.Errorf("job: unknown source %q", source)
	case goal.IsZero():
		return Job{}, errors.New("job: no goal")
	}
	if stallAfter == 0 {
		stallAfter = DefaultStallAfter(kind)
	}
	if stallAfter < MinStallAfter || stallAfter > MaxStallAfter {
		return Job{}, fmt.Errorf("job: stall_after between %s and %s", MinStallAfter, MaxStallAfter)
	}
	if metric == nil {
		metric = goal.Metric()
	}
	if metric != nil {
		if err := metric.Check(); err != nil {
			return Job{}, err
		}
	}
	if len(spec) == 0 {
		spec = json.RawMessage(`{}`)
	}
	return Job{
		Account: account, Kind: kind, Spec: spec, Goal: goal, State: Queued, Source: source,
		Metric: metric, StallAfter: stallAfter,
	}, nil
}

// DeadlineFrom is when a running job whose metric last changed at lastChange
// stalls (the store keeps it as deadline_at and the tracker stalls the job
// when it passes); nil for a job without a metric, which stalls only when its
// run ends without the goal proven.
func (j Job) DeadlineFrom(lastChange time.Time) *time.Time {
	if j.Metric == nil {
		return nil
	}
	d := lastChange.Add(j.StallAfter)
	return &d
}
