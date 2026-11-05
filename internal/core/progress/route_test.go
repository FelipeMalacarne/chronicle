package progress

import (
	"slices"
	"testing"
)

func TestPlanRoute(t *testing.T) {
	plan := Plan{
		StepEvents: []StepEvent{
			{Ref: StepRef{GuideID: "a", StepID: "1"}, From: Pending, To: AutoDone, Cause: CauseEngine},
			{Ref: StepRef{GuideID: "b", StepID: "9"}, From: Pending, To: AutoDone, Cause: CauseEngine},
			{Ref: StepRef{GuideID: "a", StepID: "0"}, From: Pending, To: Implied, Cause: CauseImplied},
		},
		Cursors: []CursorUpdate{{GuideID: "b", Cursor: CursorRecord{StepID: "10", Cause: CursorAuto}}},
	}
	before := CursorRecord{StepID: "2", Cause: CursorPin}
	got := plan.Route("a", before)
	if got.GuideID != "a" || got.Cursor != before || !slices.Equal(got.Changed, []string{"1", "0"}) {
		t.Fatalf("route a = %+v", got)
	}
	got = plan.Route("b", CursorRecord{StepID: "9", Cause: CursorAuto})
	if got.Cursor.StepID != "10" || !slices.Equal(got.Changed, []string{"9"}) {
		t.Fatalf("route b = %+v", got)
	}
	if got := (Plan{}).Route("a", CursorRecord{}); got.Changed == nil || len(got.Changed) != 0 {
		t.Fatalf("empty plan changed = %#v, want an empty list", got.Changed)
	}
}
