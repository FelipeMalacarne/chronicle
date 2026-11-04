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

// SectionCounts summarizes the visible steps of one section (tracks
// included; the guide's Counts leave them out).
type SectionCounts struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Track  bool   `json:"track"`
	Counts Counts `json:"counts"`
}

// Coverage is how much of the route the account's facts answer: the distinct
// completing objectives of the visible main route and how many are met.
type Coverage struct {
	Objectives int `json:"objectives"`
	Met        int `json:"met"`
	// Checkbox counts the visible main steps without a completing objective;
	// only a mark (or the cascade) completes them.
	Checkbox int `json:"checkbox"`
}

// Board is the derived status of every step of one guide.
type Board struct {
	Steps    []StepState     `json:"steps"`
	Counts   Counts          `json:"counts"`
	Sections []SectionCounts `json:"sections"`
	Coverage Coverage        `json:"coverage"`
}

// BuildBoard derives the board of one guide; regressed lists the met
// objectives whose facts stopped holding.
func BuildBoard(in GuideInput, regressed map[string]bool) Board {
	steps, main := guideSteps(in.Guide)
	status := in.statuses(nil)
	out := Board{Steps: make([]StepState, 0, len(steps)), Sections: []SectionCounts{}}
	sections := map[string]*SectionCounts{}
	for _, sec := range in.Guide.Sections {
		out.Sections = append(out.Sections, SectionCounts{ID: sec.ID, Title: sec.Title, Track: sec.IsTrack()})
	}
	for i := range out.Sections {
		sections[out.Sections[i].ID] = &out.Sections[i]
	}
	objectives := map[string]bool{}
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
		if !s.Visible {
			continue
		}
		if sc, ok := sections[st.SectionID]; ok {
			sc.Counts.add(s.Status)
		}
		if !s.Main {
			continue
		}
		out.Counts.add(s.Status)
		if s.Manual {
			out.Coverage.Checkbox++
		}
		for _, o := range Completing(st) {
			objectives[o.Key.String()] = true
		}
	}
	for key := range objectives {
		out.Coverage.Objectives++
		if in.Verdicts.Of(key) == rules.Met {
			out.Coverage.Met++
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
