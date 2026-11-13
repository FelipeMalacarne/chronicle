package progress

import (
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// ObjectiveState is the verdict of one objective of a step.
type ObjectiveState struct {
	Key     string        `json:"key"`
	Role    string        `json:"role"`
	Verdict rules.Verdict `json:"verdict"`
	// Regressed: met with evidence earlier, not holding in the latest facts.
	Regressed bool `json:"regressed,omitempty"`
	// OwnerMarked: the owner marked the objective as already done; the
	// verdict reads met, but there is no evidence behind it.
	OwnerMarked bool `json:"owner_marked,omitempty"`
	// Observed is the current value of a numeric objective (ObservedValue);
	// absent without one, without a known current value, or when the board
	// was built without the account's facts (GuideInput.State).
	Observed *Observed `json:"observed,omitempty"`
}

// StepState is one step of the board.
type StepState struct {
	ID string `json:"id"`
	// Ref is the author's reference ("1.1.75", "CH1.6"); Title is the start of
	// the text, enough to pick the step in a list.
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	SectionID string `json:"section_id"`
	Status    Status `json:"status"`
	Mark      *Mark  `json:"mark,omitempty"`
	// Manual: the step has no completing objective (a checkbox).
	Manual bool `json:"manual"`
	// Main: the step is on the main route (default variant, not a track).
	Main bool `json:"main"`
	// Visible: the step belongs to the account's layer and mode.
	Visible bool `json:"visible"`
	// BlockedBy are the unmet requirements of a pending step (Blocked): the
	// cursor passes it until they are met.
	BlockedBy  []string         `json:"blocked_by,omitempty"`
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
	// Blocked counts the remaining steps with an unmet requirement.
	Blocked int `json:"blocked"`
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
	Checkbox        int `json:"checkbox"`
	CheckboxDone    int `json:"checkbox_done"`
	CheckboxSkipped int `json:"checkbox_skipped"`
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
	observed := map[string]*Observed{}
	observe := func(o content.Objective) *Observed {
		if in.State == nil {
			return nil
		}
		key := o.Key.String()
		v, ok := observed[key]
		if !ok {
			v = ObservedValue(o.Key, *in.State, in.Registry)
			observed[key] = v
		}
		return v
	}
	for _, st := range steps {
		s := StepState{
			ID: st.ID, Ref: AuthorRef(st), Title: StepTitle(st.Text), SectionID: st.SectionID, Status: status[st.ID],
			Manual: IsManual(st), Main: main[st.ID], Visible: Visible(st, in.Account),
			Objectives: make([]ObjectiveState, 0, len(st.Objectives)),
		}
		if m, ok := in.Marks[st.ID]; ok {
			s.Mark = &m
		}
		if s.Status == Pending {
			s.BlockedBy = Blocked(st, in.Verdicts, in.Registry)
		}
		for _, o := range st.Objectives {
			key := o.Key.String()
			s.Objectives = append(s.Objectives, ObjectiveState{
				Key: key, Role: o.Role, Verdict: in.Verdicts.Of(key), Regressed: regressed[key], OwnerMarked: in.OwnerMarked[key], Observed: observe(o),
			})
		}
		out.Steps = append(out.Steps, s)
		if !s.Visible {
			continue
		}
		if sc, ok := sections[st.SectionID]; ok {
			sc.Counts.add(s)
		}
		if !s.Main {
			continue
		}
		out.Counts.add(s)
		if s.Manual {
			out.Coverage.Checkbox++
			switch {
			case s.Status.Done():
				out.Coverage.CheckboxDone++
			case s.Status == Skipped:
				out.Coverage.CheckboxSkipped++
			}
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

// AuthorRef is the step reference the author uses; the internal id only when
// the source has none (BRUHsailer ids are hashes and are never shown).
func AuthorRef(st *content.Step) string {
	if st.Source != nil && st.Source.OriginalID != "" {
		return st.Source.OriginalID
	}
	return st.ID
}

const titleRunes = 140

// StepTitle is the start of a step's text, enough to pick it in a list.
func StepTitle(text string) string {
	text = strings.Join(strings.Fields(strings.TrimLeft(strings.TrimSpace(text), "-• ")), " ")
	if r := []rune(text); len(r) > titleRunes {
		return strings.TrimSpace(string(r[:titleRunes])) + "…"
	}
	return text
}

func (c *Counts) add(st StepState) {
	s := st.Status
	c.Total++
	if len(st.BlockedBy) > 0 {
		c.Blocked++
	}
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
