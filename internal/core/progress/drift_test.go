package progress

import (
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func TestPlanDrift(t *testing.T) {
	if got := (Plan{}).Drift(); len(got) != 0 {
		t.Fatalf("empty plan drift = %v", got)
	}
	p := Plan{
		Objectives: []ObjectiveUpdate{{Key: "qp>=1", Verdict: rules.Met}},
		Marks:      []MarkWrite{{Ref: StepRef{GuideID: "g1", StepID: "a1"}, Mark: Mark{Kind: MarkImplied, ImpliedBy: "a3"}}},
		StepEvents: []StepEvent{{Ref: StepRef{GuideID: "g1", StepID: "a2"}, To: AutoDone, Cause: CauseEngine}},
		Cursors:    []CursorUpdate{{GuideID: "g1", Cursor: CursorRecord{StepID: "a4", Cause: CursorAuto}}},
	}
	want := []string{
		"objective qp>=1: stored verdict would become met",
		"g1/a1: missing implied mark (by a3)",
		"g1/a2: last recorded status none, engine says auto_done",
		"g1: cursor would move to a4 (auto)",
	}
	got := p.Drift()
	if len(got) != len(want) {
		t.Fatalf("drift = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drift[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
