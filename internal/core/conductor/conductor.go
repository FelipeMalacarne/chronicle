// Package conductor decides, for one account, what the conductor does next
// (docs/12 section 12.5): dispatch the next task of the account's plan as a
// job, wait, or stop. It is pure; internal/conduct reads the state and the
// jobs, calls Decide and Settle and acts on what they say.
//
// This is the first slice (FEL-126): the states off, idle, running and
// blocked. Sessions (FEL-104), the owner's approvals (FEL-105), the
// escalation ladder (FEL-101) and the GP upkeep (FEL-94) add rows here.
package conductor

import (
	"fmt"

	"github.com/FelipeMalacarne/chronicle/internal/core/job"
)

// Mode is the conductor's state for an account.
type Mode string

const (
	// Off: the conductor does nothing for the account (the default, until
	// the owner turns it on).
	Off Mode = "off"
	// Idle: no job of the conductor goes; the next task is dispatched when
	// the account has no active job.
	Idle Mode = "idle"
	// Running: a job the conductor dispatched is active.
	Running Mode = "running"
	// Blocked: the conductor stopped and says why; the owner turns it on
	// again.
	Blocked Mode = "blocked"
)

// Modes are the valid modes, in the order of the table above.
var Modes = []Mode{Off, Idle, Running, Blocked}

// State is what the conductor keeps per account (conductor_state).
type State struct {
	Mode Mode
	// Job is the job the conductor dispatched, while Running and after it
	// ended until the next one.
	Job string
	// Reason says why the conductor is Blocked, or why it waits.
	Reason string
}

// Seen is what the conductor reads before it decides.
type Seen struct {
	// Job is the conductor's job (State.Job) as it is now; nil when the
	// state names none or the job is gone.
	Job *job.Job
	// OtherActive is an active job of the account that is not the
	// conductor's: the owner's, or a routine's. The conductor never runs
	// alongside it.
	OtherActive bool
}

// Decision is what Decide says: the state to keep and whether to dispatch the
// next task from it.
type Decision struct {
	State    State
	Dispatch bool
}

// Decide is one step for one account:
//
//	| mode    | seen                                   | state           | dispatch |
//	|---------|----------------------------------------|-----------------|----------|
//	| off     | anything                               | off             | no       |
//	| blocked | anything                               | blocked         | no       |
//	| running | its job active (queued/running/stalled) | running         | no       |
//	| running | its job succeeded                      | idle            | if free  |
//	| running | its job failed or canceled             | blocked, reason | no       |
//	| running | its job gone                           | idle            | if free  |
//	| idle    | another job active                     | idle            | no       |
//	| idle    | free                                   | idle            | yes      |
//
// A stalled job waits: it can run again (FEL-133), and the escalation ladder
// is what will act on a stall. A failed or canceled one blocks the account
// until the owner turns the conductor on again (decision C of FEL-126).
func Decide(st State, seen Seen) Decision {
	switch st.Mode {
	case Running:
		if seen.Job != nil {
			switch seen.Job.State {
			case job.Failed, job.Canceled:
				return Decision{State: State{Mode: Blocked, Job: st.Job,
					Reason: fmt.Sprintf("o job %s terminou %s: %s", st.Job, seen.Job.State, seen.Job.Reason)}}
			case job.Succeeded:
			default:
				return Decision{State: st}
			}
		}
		idle := State{Mode: Idle, Job: st.Job}
		return Decision{State: idle, Dispatch: !seen.OtherActive}
	case Idle:
		return Decision{State: st, Dispatch: !seen.OtherActive}
	default:
		return Decision{State: st}
	}
}

// Outcome is what a dispatch gave.
type Outcome struct {
	Kind OutcomeKind
	// Job is the job the dispatched run attempts (Dispatched).
	Job string
	// Reason says why nothing was dispatched (Wait, Refused).
	Reason string
}

// OutcomeKind sorts a dispatch's result.
type OutcomeKind int

const (
	// Dispatched: the agent took the task; Job is its job.
	Dispatched OutcomeKind = iota
	// Wait: nothing to run now, but it may come by itself: the plan waits
	// for something, the agent is away or did not answer. The conductor
	// tries again at the next event or tick.
	Wait
	// Refused: nothing the conductor can run until the owner acts: the
	// plan's task needs the owner (manual, E9), the plan is done, the
	// client cannot run it.
	Refused
)

// Settle is the state after a dispatch from st (an Idle one).
func Settle(st State, out Outcome) State {
	switch out.Kind {
	case Dispatched:
		return State{Mode: Running, Job: out.Job}
	case Refused:
		return State{Mode: Blocked, Job: st.Job, Reason: out.Reason}
	default:
		return State{Mode: Idle, Job: st.Job, Reason: out.Reason}
	}
}
