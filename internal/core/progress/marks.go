package progress

import (
	"errors"
	"fmt"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

// MarkAction is what the owner asks for in PATCH .../steps/{id}/mark.
type MarkAction string

// Mark actions. ConfirmImplied turns an implied mark into manual_done; Clear
// removes the mark.
const (
	ActionManualDone     MarkAction = "manual_done"
	ActionSkipped        MarkAction = "skipped"
	ActionPinnedOpen     MarkAction = "pinned_open"
	ActionConfirmImplied MarkAction = "confirm_implied"
	ActionClear          MarkAction = "clear"
)

// Errors returned by the owner operations.
var (
	ErrStepNotFound  = errors.New("step not found")
	ErrInvalidAction = errors.New("invalid mark action")
	ErrNotImplied    = errors.New("step is not implied")
	ErrStepDone      = errors.New("step is already done")
)

// GuideInput is the account's state in one guide.
type GuideInput struct {
	Guide      *content.Guide
	Verdicts   Verdicts
	Marks      map[string]Mark
	LastStatus map[string]Status
	Cursor     CursorRecord
	Account    Account
}

// MarkChange is a mark to write; a nil Mark deletes it.
type MarkChange struct {
	StepID string
	Mark   *Mark
}

// OwnerPlan is what an owner operation writes in one guide.
type OwnerPlan struct {
	Marks  []MarkChange
	Events []StepEvent
	// Cursor is set when the cursor changes; an empty StepID deletes it.
	Cursor *CursorRecord
}

// PlanMark applies one mark action to one step. A nil note keeps the note of
// the current mark. The engine never overrides the result (docs/09 section 5).
func PlanMark(in GuideInput, stepID string, action MarkAction, note *string) (OwnerPlan, error) {
	if _, ok := in.Guide.Step(stepID); !ok {
		return OwnerPlan{}, ErrStepNotFound
	}
	cur, hasCur := in.Marks[stepID]
	var next *Mark
	switch action {
	case ActionManualDone, ActionSkipped, ActionPinnedOpen:
		m := Mark{Kind: MarkKind(action)}
		if hasCur {
			m.Note = cur.Note
		}
		next = &m
	case ActionConfirmImplied:
		if !hasCur || cur.Kind != MarkImplied {
			return OwnerPlan{}, ErrNotImplied
		}
		next = &Mark{Kind: MarkManualDone, Note: cur.Note}
	case ActionClear:
		next = nil
	default:
		return OwnerPlan{}, fmt.Errorf("%w %q", ErrInvalidAction, action)
	}
	if next != nil && note != nil {
		next.Note = *note
	}
	if !hasCur && next == nil {
		return OwnerPlan{}, nil
	}
	return in.plan([]MarkChange{{StepID: stepID, Mark: next}}), nil
}

// PlanConfirmImplied confirms every implied mark of the guide ("confirmar
// tudo").
func PlanConfirmImplied(in GuideInput) OwnerPlan {
	var changes []MarkChange
	steps, _ := guideSteps(in.Guide)
	for _, st := range steps {
		if m, ok := in.Marks[st.ID]; ok && m.Kind == MarkImplied {
			changes = append(changes, MarkChange{StepID: st.ID, Mark: &Mark{Kind: MarkManualDone, Note: m.Note}})
		}
	}
	if len(changes) == 0 {
		return OwnerPlan{}
	}
	return in.plan(changes)
}

// PlanPin pins the cursor of the guide on a step; an empty stepID unpins it
// and lets the cursor follow the route again. A done step cannot be pinned:
// the pin would be dropped at once (docs/09 section 3.5).
func PlanPin(in GuideInput, stepID string) (OwnerPlan, error) {
	cur := CursorRecord{}
	if stepID != "" {
		if _, ok := in.Guide.Step(stepID); !ok {
			return OwnerPlan{}, ErrStepNotFound
		}
		cur = CursorRecord{StepID: stepID, Cause: CursorPin}
	}
	status := in.statuses(nil)
	if cur.StepID != "" && status[cur.StepID].Done() {
		return OwnerPlan{}, ErrStepDone
	}
	placed := PlaceCursor(in.Guide, status, in.Account, cur)
	return OwnerPlan{Cursor: &placed}, nil
}

// plan applies mark changes, records the status changes they cause and
// places the cursor again.
func (in GuideInput) plan(changes []MarkChange) OwnerPlan {
	out := OwnerPlan{Marks: changes}
	override := map[string]*Mark{}
	for _, c := range changes {
		override[c.StepID] = c.Mark
	}
	status := in.statuses(override)
	for _, c := range changes {
		from := Pending
		if s, ok := in.LastStatus[c.StepID]; ok && s.Valid() {
			from = s
		}
		if to := status[c.StepID]; to != from {
			out.Events = append(out.Events, StepEvent{Ref: StepRef{GuideID: in.Guide.ID, StepID: c.StepID}, From: from, To: to, Cause: CauseUser})
		}
	}
	placed := PlaceCursor(in.Guide, status, in.Account, in.Cursor)
	if placed != in.Cursor {
		out.Cursor = &placed
	}
	return out
}

// statuses derives every step status with some marks replaced (a nil entry
// removes the mark).
func (in GuideInput) statuses(override map[string]*Mark) map[string]Status {
	out := map[string]Status{}
	steps, _ := guideSteps(in.Guide)
	for _, st := range steps {
		var mark *Mark
		if m, ok := override[st.ID]; ok {
			mark = m
		} else if m, ok := in.Marks[st.ID]; ok {
			mark = &m
		}
		out[st.ID] = StepStatus(st, in.Verdicts, mark)
	}
	return out
}
