package progress

import (
	"errors"
	"fmt"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
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
	Guide *content.Guide
	// Verdicts are the stored verdicts with the owner's objective marks
	// overlaid (Verdicts.WithOwner); OwnerMarked tells which keys they are.
	Verdicts    Verdicts
	OwnerMarked map[string]bool
	Marks       map[string]Mark
	LastStatus  map[string]Status
	Cursor      CursorRecord
	Account     Account
	// Registry is the catalog with the quest requirements that block steps.
	Registry *entities.Registry
	// State is the account's current facts; with it, BuildBoard adds each
	// numeric objective's observed value. Nil leaves them out.
	State *rules.PlayerState
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
	placed := PlaceCursor(in.Guide, status, in.Verdicts, in.Registry, in.Account, cur)
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
	placed := PlaceCursor(in.Guide, status, in.Verdicts, in.Registry, in.Account, in.Cursor)
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

// PlanMarkBefore marks as manual_done every step of the main route that comes
// before stepID and is visible for the account: the pending ones without a
// mark and the implied ones (the owner catching up with where they are, and
// confirming what the cascade presumed). Done and skipped steps, other marks,
// tracks and the step itself are left as they are. It returns the plan and,
// per step it marks, the mark it had (nil for none), so the batch can be undone.
func PlanMarkBefore(in GuideInput, stepID string) (OwnerPlan, []MarkChange, error) {
	if _, ok := in.Guide.Step(stepID); !ok {
		return OwnerPlan{}, nil, ErrStepNotFound
	}
	status := in.statuses(nil)
	steps, main := guideSteps(in.Guide)
	var changes, undo []MarkChange
	for _, st := range steps {
		if st.ID == stepID {
			break
		}
		if !main[st.ID] || !Visible(st, in.Account) {
			continue
		}
		var prev *Mark
		if m, marked := in.Marks[st.ID]; marked {
			if m.Kind != MarkImplied {
				continue
			}
			prev = &m
		} else if status[st.ID] != Pending {
			continue
		}
		changes = append(changes, MarkChange{StepID: st.ID, Mark: &Mark{Kind: MarkManualDone}})
		undo = append(undo, MarkChange{StepID: st.ID, Mark: prev})
	}
	if len(changes) == 0 {
		return OwnerPlan{}, nil, nil
	}
	return in.plan(changes), undo, nil
}

// PlanRestore undoes a batch: each step still marked manual_done gets back
// the mark it had (nil removes the mark; only implied marks are restored).
// Steps the owner changed since, and unknown ids, are left alone.
func PlanRestore(in GuideInput, restores []MarkChange) OwnerPlan {
	var changes []MarkChange
	for _, r := range restores {
		m, ok := in.Marks[r.StepID]
		if !ok || m.Kind != MarkManualDone {
			continue
		}
		if r.Mark != nil && (r.Mark.Kind != MarkImplied || r.Mark.ImpliedBy == "") {
			continue
		}
		changes = append(changes, MarkChange{StepID: r.StepID, Mark: r.Mark})
	}
	if len(changes) == 0 {
		return OwnerPlan{}
	}
	return in.plan(changes)
}
