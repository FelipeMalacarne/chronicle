package conductor

import (
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/job"
)

func TestDecide(t *testing.T) {
	jobIn := func(s job.State, reason string) *job.Job { return &job.Job{ID: "job_a", State: s, Reason: reason} }
	running := State{Mode: Running, Job: "job_a"}
	for _, tc := range []struct {
		name     string
		st       State
		seen     Seen
		want     Mode
		dispatch bool
		reason   string
	}{
		{"off stays off", State{Mode: Off}, Seen{}, Off, false, ""},
		{"off ignores a free account", State{Mode: Off, Job: "job_a"}, Seen{Job: jobIn(job.Succeeded, "")}, Off, false, ""},
		{"blocked stays blocked", State{Mode: Blocked, Reason: "x"}, Seen{}, Blocked, false, "x"},
		{"idle and free dispatches", State{Mode: Idle}, Seen{}, Idle, true, ""},
		{"idle waits for another job", State{Mode: Idle}, Seen{OtherActive: true}, Idle, false, ""},
		{"running job queued", running, Seen{Job: jobIn(job.Queued, "")}, Running, false, ""},
		{"running job running", running, Seen{Job: jobIn(job.Running, "")}, Running, false, ""},
		{"a stalled job waits", running, Seen{Job: jobIn(job.Stalled, "no progress")}, Running, false, ""},
		{"succeeded dispatches the next", running, Seen{Job: jobIn(job.Succeeded, "goal met")}, Idle, true, ""},
		{"succeeded waits for another job", running, Seen{Job: jobIn(job.Succeeded, ""), OtherActive: true}, Idle, false, ""},
		{"failed blocks", running, Seen{Job: jobIn(job.Failed, "run ended: failed")}, Blocked, false, "terminou failed: run ended: failed"},
		{"canceled blocks", running, Seen{Job: jobIn(job.Canceled, "canceled by the owner")}, Blocked, false, "terminou canceled"},
		{"a gone job frees the account", running, Seen{}, Idle, true, ""},
	} {
		d := Decide(tc.st, tc.seen)
		if d.State.Mode != tc.want || d.Dispatch != tc.dispatch || !strings.Contains(d.State.Reason, tc.reason) {
			t.Errorf("%s: %+v", tc.name, d)
		}
		if tc.st.Mode == Running && d.State.Job != "job_a" {
			t.Errorf("%s: lost the job: %+v", tc.name, d)
		}
	}
}

func TestSettle(t *testing.T) {
	idle := State{Mode: Idle, Job: "job_old"}
	for _, tc := range []struct {
		name string
		out  Outcome
		want State
	}{
		{"dispatched runs its job", Outcome{Kind: Dispatched, Job: "job_new"}, State{Mode: Running, Job: "job_new"}},
		{"wait stays idle with why", Outcome{Kind: Wait, Reason: "no agent"}, State{Mode: Idle, Job: "job_old", Reason: "no agent"}},
		{"refused blocks with why", Outcome{Kind: Refused, Reason: "plan done"}, State{Mode: Blocked, Job: "job_old", Reason: "plan done"}},
	} {
		if got := Settle(idle, tc.out); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
