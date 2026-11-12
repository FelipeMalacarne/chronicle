// Package progress is the pure core of account progress over objectives
// (docs/09 section 5): it evaluates objective keys against a PlayerState,
// latches verdicts with evidence, derives each step's status from its
// objectives and the owner's marks, applies the implied cascade on the
// active guide and places the cursor of each guide. It performs no IO and
// does not read the clock: callers pass now.
package progress

import (
	"slices"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
)

// Status is the derived status of a step in one guide.
type Status string

// Step statuses. A mark always wins; otherwise a step is AutoDone when every
// completing objective is met, and Pending in every other case (unknown
// leaves a step pending, CLAUDE.md rule 2).
const (
	Pending    Status = "pending"
	AutoDone   Status = "auto_done"
	ManualDone Status = "manual_done"
	Skipped    Status = "skipped"
	PinnedOpen Status = "pinned_open"
	Implied    Status = "implied"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case Pending, AutoDone, ManualDone, Skipped, PinnedOpen, Implied:
		return true
	}
	return false
}

// Done reports whether the step counts as done: by evidence, by the owner,
// or implied by a later step (still awaiting the owner's confirmation).
func (s Status) Done() bool { return s == AutoDone || s == ManualDone || s == Implied }

// MarkKind is the kind of an owner's mark on a step.
type MarkKind string

// Mark kinds. Implied is written only by the cascade and becomes ManualDone
// when the owner confirms it.
const (
	MarkManualDone MarkKind = "manual_done"
	MarkSkipped    MarkKind = "skipped"
	MarkPinnedOpen MarkKind = "pinned_open"
	MarkImplied    MarkKind = "implied"
)

// Valid reports whether k is a known mark kind.
func (k MarkKind) Valid() bool {
	switch k {
	case MarkManualDone, MarkSkipped, MarkPinnedOpen, MarkImplied:
		return true
	}
	return false
}

// Mark is the owner's mark on a step of one guide.
type Mark struct {
	Kind      MarkKind `json:"kind"`
	ImpliedBy string   `json:"implied_by_step_id,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// Cause is why a step status changed (step_status_events.cause).
type Cause string

// Causes.
const (
	CauseEngine    Cause = "engine"
	CauseUser      Cause = "user"
	CauseImplied   Cause = "implied"
	CauseMigration Cause = "migration"
	CauseImport    Cause = "import"
)

// StepRef addresses a step: local ids repeat across guides (CLAUDE.md rule 7).
type StepRef struct {
	GuideID string
	StepID  string
}

// Account is what the engine needs to know about the account.
type Account struct {
	// ActiveGuideID is the route the owner follows (accounts.guide_id).
	ActiveGuideID string
	Layer         string
	Mode          string
}

// Completing returns the step's completes/starts objectives.
func Completing(st *content.Step) []content.Objective {
	var out []content.Objective
	for _, o := range st.Objectives {
		if o.Completing() {
			out = append(out, o)
		}
	}
	return out
}

// IsManual reports whether the step has no completing objective (a checkbox).
func IsManual(st *content.Step) bool { return len(Completing(st)) == 0 }

// Visible reports whether the step belongs to the account's layer and mode.
// A step without explicit layers or modes is visible in all of them.
func Visible(st *content.Step, acc Account) bool {
	if len(st.Layers) > 0 && acc.Layer != "" && !slices.Contains(st.Layers, acc.Layer) {
		return false
	}
	if len(st.Modes) > 0 && acc.Mode != "" && !slices.Contains(st.Modes, acc.Mode) {
		return false
	}
	return true
}

// guideSteps returns the guide's steps in order with duplicate ids dropped
// (the first occurrence wins, as in Guide.Step), together with whether each
// step belongs to the main route: the default variant, outside tracks.
func guideSteps(g *content.Guide) (steps []*content.Step, main map[string]bool) {
	main = map[string]bool{}
	seen := map[string]bool{}
	for _, sec := range g.Sections {
		inMain := !sec.IsTrack() && (sec.VariantID == "" || g.DefaultVariant == "" || sec.VariantID == g.DefaultVariant)
		for _, st := range sec.Steps {
			if seen[st.ID] {
				continue
			}
			seen[st.ID] = true
			steps = append(steps, st)
			if inMain {
				main[st.ID] = true
			}
		}
	}
	return steps, main
}
