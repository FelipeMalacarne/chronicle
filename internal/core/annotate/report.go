package annotate

import (
	"fmt"
	"path/filepath"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// ReportKind is the kind of annotations.report.json.
const ReportKind = "report"

// Report is content/guides/<id>/annotations.report.json: what the owner
// reviews before committing an extraction (docs/09 sections 6.5 and 6.6).
// Steps is the per-step log the aggregates are computed from; it is what
// survives between sessions.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	GuideID       string          `json:"guide_id"`
	Summary       SectionReport   `json:"summary"`
	Sections      []SectionReport `json:"sections"`
	RuleAgreement RuleAgreement   `json:"rule_agreement"`
	// Rejected lists the steps whose last submit was rejected.
	Rejected []RejectedStep `json:"rejected"`
	// Inferred lists what entered the sidecars with medium confidence, i.e.
	// by the owner's confirmation or decision.
	Inferred []InferredEntry     `json:"inferred"`
	Steps    map[string]*StepLog `json:"steps,omitempty"`
}

// SectionReport aggregates one section (or the guide, in Summary).
type SectionReport struct {
	SectionID string `json:"section_id,omitempty"`
	Coverage
	// Submitted counts steps submitted at least once; FirstSubmitAccepted
	// those accepted on their first submit (the pilot's acceptance metric).
	Submitted           int `json:"submitted"`
	FirstSubmitAccepted int `json:"first_submit_accepted"`
	// OwnerConfirmed counts annotated entries confirmed by the owner;
	// DecisionsApplied those that apply a recorded decision.
	OwnerConfirmed   int `json:"owner_confirmed"`
	DecisionsApplied int `json:"decisions_applied"`
}

// RuleAgreement compares annotated steps with the objectives converted from
// their rules: Reproduced counts rule objectives the annotation repeats with
// the same key and role.
type RuleAgreement struct {
	Rules      int          `json:"rules"`
	Reproduced int          `json:"reproduced"`
	Divergent  []Divergence `json:"divergent"`
}

// Divergence is a rule objective an annotated step does not repeat. Comment
// is the agent's explanation for the step, when it gave one.
type Divergence struct {
	StepID  string `json:"step_id"`
	Key     string `json:"key"`
	Role    string `json:"role"`
	Comment string `json:"comment,omitempty"`
}

// InferredEntry is one medium-confidence entry of a sidecar.
type InferredEntry struct {
	StepID   string `json:"step_id"`
	What     string `json:"what"`
	Quote    string `json:"quote"`
	Decision string `json:"decision,omitempty"`
}

// StepLog is the extraction history of one step.
type StepLog struct {
	SectionID           string `json:"section_id"`
	Submits             int    `json:"submits"`
	FirstSubmitAccepted bool   `json:"first_submit_accepted,omitempty"`
	// Reasons are those of the last rejection; cleared when accepted.
	Reasons []string `json:"reasons,omitempty"`
	Comment string   `json:"comment,omitempty"`
}

func (r *Report) log(stepID, sectionID string, reasons []string, comment string) {
	l := r.Steps[stepID]
	if l == nil {
		l = &StepLog{}
		r.Steps[stepID] = l
	}
	l.SectionID = sectionID
	l.Submits++
	l.Reasons = reasons
	if len(reasons) == 0 && l.Submits == 1 {
		l.FirstSubmitAccepted = true
	}
	if comment != "" {
		l.Comment = comment
	}
}

// Report recomputes and returns the report of a guide, without the per-step
// log.
func (w *Workspace) Report(guideID string) (*Report, error) {
	gs, err := w.guide(guideID)
	if err != nil {
		return nil, err
	}
	gs.refreshReport(w.reg)
	out := *gs.report
	out.Steps = nil
	return &out, nil
}

func (gs *guideState) writeReport(reg *entities.Registry) error {
	gs.refreshReport(reg)
	return writeJSON(filepath.Join(gs.dir, ReportFileName), gs.report)
}

// refreshReport recomputes the aggregates from the sidecars and the log.
func (gs *guideState) refreshReport(reg *entities.Registry) {
	r := gs.report
	r.SchemaVersion, r.Kind, r.GuideID = 1, ReportKind, gs.guide.ID
	r.Summary = SectionReport{}
	r.Sections = nil
	r.RuleAgreement = RuleAgreement{Divergent: []Divergence{}}
	r.Rejected = []RejectedStep{}
	r.Inferred = []InferredEntry{}
	for _, sec := range gs.guide.Sections {
		s := SectionReport{SectionID: sec.ID, Coverage: gs.coverage(sec)}
		for _, st := range sec.Steps {
			if l := r.Steps[st.ID]; l != nil {
				s.Submitted++
				if l.FirstSubmitAccepted {
					s.FirstSubmitAccepted++
				}
				if len(l.Reasons) > 0 {
					r.Rejected = append(r.Rejected, RejectedStep{StepID: st.ID, Reasons: l.Reasons})
				}
			}
			sa := gs.annotation(st)
			if sa == nil || sa.TextHash != content.StepTextHash(st) {
				continue
			}
			gs.tally(&s, st, sa)
			rules, _ := content.RuleObjectives(st, reg)
			for _, o := range rules {
				r.RuleAgreement.Rules++
				if annotated(*sa, o) {
					r.RuleAgreement.Reproduced++
					continue
				}
				d := Divergence{StepID: st.ID, Key: o.Key.String(), Role: o.Role}
				if l := r.Steps[st.ID]; l != nil {
					d.Comment = l.Comment
				}
				r.RuleAgreement.Divergent = append(r.RuleAgreement.Divergent, d)
			}
		}
		r.Summary.add(s.Coverage)
		r.Summary.Submitted += s.Submitted
		r.Summary.FirstSubmitAccepted += s.FirstSubmitAccepted
		r.Summary.OwnerConfirmed += s.OwnerConfirmed
		r.Summary.DecisionsApplied += s.DecisionsApplied
		r.Sections = append(r.Sections, s)
	}
}

// tally counts the confirmed and inferred entries of one annotation.
func (gs *guideState) tally(s *SectionReport, st *content.Step, sa *content.StepAnnotation) {
	count := func(what string, ev content.ObjectiveEvidence, src content.ObjectiveSource) {
		if src.ConfirmedBy == content.ConfirmedByOwner {
			s.OwnerConfirmed++
		}
		if src.Decision != "" {
			s.DecisionsApplied++
		}
		if src.Confidence != content.ConfidenceHigh {
			gs.report.Inferred = append(gs.report.Inferred, InferredEntry{StepID: st.ID, What: what, Quote: ev.Quote, Decision: src.Decision})
		}
	}
	for _, o := range sa.Objectives {
		count(fmt.Sprintf("%s %s", o.Key, o.Role), *o.Evidence, o.Source)
	}
	for _, it := range sa.Items {
		count("item "+describeItem(it.ItemRequirement), it.Evidence, it.Source)
	}
	for _, ba := range sa.BankActions {
		count("bank "+ba.Type+"@"+ba.Position, ba.Evidence, ba.Source)
	}
	if l := sa.Location; l != nil {
		count("location "+l.Ref.String(), l.Evidence, l.Source)
	}
}
