package progress

import (
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// Verdicts are the current verdicts by objective key; a missing key is unknown.
type Verdicts map[string]rules.Verdict

// Of returns the verdict of key.
func (v Verdicts) Of(key string) rules.Verdict {
	if got, ok := v[key]; ok {
		return got
	}
	return rules.Unknown
}

// StepStatus derives the status of a step (docs/09 section 5): the mark wins;
// otherwise AutoDone when the step has completing objectives and all of them
// are met; otherwise Pending. A step without completing objectives is a
// checkbox and is Pending until marked.
func StepStatus(st *content.Step, verdicts Verdicts, mark *Mark) Status {
	if mark != nil {
		return Status(mark.Kind)
	}
	completing := Completing(st)
	if len(completing) == 0 {
		return Pending
	}
	for _, o := range completing {
		if verdicts.Of(o.Key.String()) != rules.Met {
			return Pending
		}
	}
	return AutoDone
}

// Cursor causes.
const (
	CursorPin  = "pin"
	CursorAuto = "auto"
)

// CursorRecord is the stored cursor of one guide; an empty StepID means none.
type CursorRecord struct {
	StepID string
	Cause  string
}

// PlaceCursor returns the cursor of a guide (docs/09 section 3.5): a pin stays
// while the pinned step exists and is not done; otherwise the cursor is the
// first visible step of the main route that is neither done nor skipped.
// Implied steps count as done here (owner decision of 2026-11-04), so a
// cascade never pulls the cursor back. The empty record means the main route
// is finished.
func PlaceCursor(g *content.Guide, status map[string]Status, acc Account, cur CursorRecord) CursorRecord {
	if cur.Cause == CursorPin {
		if _, ok := g.Step(cur.StepID); ok && !status[cur.StepID].Done() {
			return cur
		}
	}
	steps, main := guideSteps(g)
	for _, st := range steps {
		if !main[st.ID] || !Visible(st, acc) {
			continue
		}
		s := status[st.ID]
		if s.Done() || s == Skipped {
			continue
		}
		return CursorRecord{StepID: st.ID, Cause: CursorAuto}
	}
	return CursorRecord{}
}
