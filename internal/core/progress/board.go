package progress

import "github.com/FelipeMalacarne/ironman-planner/internal/core/rules"

// ObjectiveState is the verdict of one objective of a step.
type ObjectiveState struct {
	Key     string        `json:"key"`
	Role    string        `json:"role"`
	Verdict rules.Verdict `json:"verdict"`
	// Regressed: met with evidence earlier, not holding in the latest facts.
	Regressed bool `json:"regressed,omitempty"`
}

// StepState is one step of the board.
type StepState struct {
	ID        string `json:"id"`
	SectionID string `json:"section_id"`
	Status    Status `json:"status"`
	Mark      *Mark  `json:"mark,omitempty"`
	// Manual: the step has no completing objective (a checkbox).
	Manual bool `json:"manual"`
	// Main: the step is on the main route (default variant, not a track).
	Main bool `json:"main"`
	// Visible: the step belongs to the account's layer and mode.
	Visible    bool             `json:"visible"`
	Objectives []ObjectiveState `json:"objectives"`
}

// Counts summarizes the visible steps of the main route.
type Counts struct {
	Total     int `json:"total"`
	Done      int `json:"done"`
	AutoDone  int `json:"auto_done"`
	Manual    int `json:"manual_done"`
	Implied   int `json:"implied"`
	Skipped   int `json:"skipped"`
	Pinned    int `json:"pinned_open"`
	Remaining int `json:"remaining"`
}

// Board is the derived status of every step of one guide.
type Board struct {
	Steps  []StepState `json:"steps"`
	Counts Counts      `json:"counts"`
}

// BuildBoard derives the board of one guide; regressed lists the met
// objectives whose facts stopped holding.
func BuildBoard(in GuideInput, regressed map[string]bool) Board {
	steps, main := guideSteps(in.Guide)
	status := in.statuses(nil)
	out := Board{Steps: make([]StepState, 0, len(steps))}
	for _, st := range steps {
		s := StepState{
			ID: st.ID, SectionID: st.SectionID, Status: status[st.ID],
			Manual: IsManual(st), Main: main[st.ID], Visible: Visible(st, in.Account),
			Objectives: make([]ObjectiveState, 0, len(st.Objectives)),
		}
		if m, ok := in.Marks[st.ID]; ok {
			s.Mark = &m
		}
		for _, o := range st.Objectives {
			key := o.Key.String()
			s.Objectives = append(s.Objectives, ObjectiveState{Key: key, Role: o.Role, Verdict: in.Verdicts.Of(key), Regressed: regressed[key]})
		}
		out.Steps = append(out.Steps, s)
		if s.Main && s.Visible {
			out.Counts.add(s.Status)
		}
	}
	return out
}

func (c *Counts) add(s Status) {
	c.Total++
	switch s {
	case AutoDone:
		c.AutoDone++
	case ManualDone:
		c.Manual++
	case Implied:
		c.Implied++
	case Skipped:
		c.Skipped++
	case PinnedOpen:
		c.Pinned++
	}
	if s.Done() {
		c.Done++
	} else if s != Skipped {
		c.Remaining++
	}
}
