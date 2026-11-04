package annotate

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// SubmitResult says, per step, what was accepted and why the rest was not.
// A step is accepted whole or not at all.
type SubmitResult struct {
	GuideID   string         `json:"guide_id"`
	SectionID string         `json:"section_id"`
	Accepted  []AcceptedStep `json:"accepted"`
	Rejected  []RejectedStep `json:"rejected"`
	// Pending counts the steps of the section still without a current
	// annotation after this submit.
	Pending int    `json:"pending"`
	File    string `json:"file,omitempty"`
}

// AcceptedStep is the annotation written for a step, in short form.
type AcceptedStep struct {
	StepID      string          `json:"step_id"`
	Objectives  []ObjectiveView `json:"objectives,omitempty"`
	Items       []string        `json:"items,omitempty"`
	BankActions []string        `json:"bank_actions,omitempty"`
	Location    string          `json:"location,omitempty"`
}

// RejectedStep is a step whose proposal was not written, with every reason.
type RejectedStep struct {
	StepID  string   `json:"step_id"`
	Reasons []string `json:"reasons"`
}

// Submit verifies proposals for steps of one section and writes the accepted
// ones to annotations/<section>.json, replacing earlier annotations of the
// same steps. Checks, per step: the names resolve (the ref is the
// resolver's, never the agent's), each quote is in the step, inferences
// carry the owner's confirmation, the loader's own annotation checks pass,
// and the objectives keep the route in order (a quest does not start after
// it is finished, nothing is completed twice, a requirement is not met only
// later). Rejections go to the report as well as the result.
func (w *Workspace) Submit(guideID, sectionID, skillVersion string, steps []StepProposal) (*SubmitResult, error) {
	gs, sec, err := w.section(guideID, sectionID)
	if err != nil {
		return nil, err
	}
	if skillVersion == "" {
		return nil, fmt.Errorf("skill_version is required (the annotate-guide skill's version, e.g. annotate-guide@1)")
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps to submit")
	}
	res := &SubmitResult{GuideID: guideID, SectionID: sec.ID, Accepted: []AcceptedStep{}, Rejected: []RejectedStep{}}
	candidates := map[string]content.StepAnnotation{}
	reasons := map[string][]string{}
	var order []string
	for _, p := range steps {
		if _, dup := reasons[p.StepID]; dup {
			return nil, fmt.Errorf("step %q is submitted twice", p.StepID)
		}
		reasons[p.StepID] = nil
		order = append(order, p.StepID)
		st, ok := gs.guide.Step(p.StepID)
		if !ok {
			reasons[p.StepID] = []string{fmt.Sprintf("unknown step %q", p.StepID)}
			continue
		}
		if st.SectionID != sec.ID {
			reasons[p.StepID] = []string{fmt.Sprintf("step %q belongs to section %q, not %q", p.StepID, st.SectionID, sec.ID)}
			continue
		}
		sa, errs := build(w.reg, st, p, skillVersion)
		if len(errs) == 0 {
			issues, _ := gs.guide.CheckStepAnnotation(&sa, gs.decisions, w.reg)
			for _, is := range issues {
				errs = append(errs, is.Pointer+": "+is.Message)
			}
		}
		if len(errs) > 0 {
			reasons[p.StepID] = errs
			continue
		}
		candidates[p.StepID] = sa
	}
	for id, errs := range gs.orderIssues(w.reg, candidates) {
		reasons[id] = append(reasons[id], errs...)
		delete(candidates, id)
	}

	if len(candidates) > 0 {
		file, err := gs.write(sec, skillVersion, candidates)
		if err != nil {
			return nil, err
		}
		res.File = file
	}
	byID := map[string]StepProposal{}
	for _, p := range steps {
		byID[p.StepID] = p
	}
	for _, id := range order {
		if sa, ok := candidates[id]; ok {
			res.Accepted = append(res.Accepted, accepted(sa))
		} else {
			res.Rejected = append(res.Rejected, RejectedStep{StepID: id, Reasons: reasons[id]})
		}
		if _, known := gs.index[id]; known {
			gs.report.log(id, sec.ID, reasons[id], byID[id].Comment)
		}
	}
	res.Pending = gs.coverage(sec).Pending + gs.coverage(sec).Stale
	if err := gs.writeReport(w.reg); err != nil {
		return nil, err
	}
	return res, nil
}

func accepted(sa content.StepAnnotation) AcceptedStep {
	a := AcceptedStep{StepID: sa.StepID}
	for _, o := range sa.Objectives {
		a.Objectives = append(a.Objectives, view(o))
	}
	for _, it := range sa.Items {
		a.Items = append(a.Items, describeItem(it.ItemRequirement))
	}
	for _, ba := range sa.BankActions {
		a.BankActions = append(a.BankActions, ba.Type+"@"+ba.Position)
	}
	if sa.Location != nil {
		a.Location = sa.Location.Ref.String()
	}
	return a
}

func describeItem(r content.ItemRequirement) string {
	s := r.Name
	switch r.Variant() {
	case content.RequireExact:
		s += fmt.Sprintf(" = item:%d", *r.ItemID)
	case content.RequireAnyOf:
		s += fmt.Sprintf(" = any_of %v", r.AnyOf)
	case content.RequireFamily:
		s += " = family " + r.Family
	}
	if r.Qty != nil {
		s += fmt.Sprintf(" x%d", *r.Qty)
	}
	if r.Noted {
		s += " noted"
	}
	return s
}

// write merges accepted annotations into the section sidecar, in guide
// order, after checking the whole file against the schema.
func (gs *guideState) write(sec *content.Section, skillVersion string, accepted map[string]content.StepAnnotation) (string, error) {
	a := gs.sections[sec.ID]
	next := &content.AnnotationFile{SchemaVersion: 1, Kind: content.AnnotationKindSection, GuideID: gs.guide.ID, SectionID: sec.ID, SkillVersion: skillVersion}
	if a != nil {
		for _, sa := range a.Steps {
			if _, replaced := accepted[sa.StepID]; !replaced {
				next.Steps = append(next.Steps, sa)
			}
		}
	}
	for _, sa := range accepted {
		next.Steps = append(next.Steps, sa)
	}
	sort.SliceStable(next.Steps, func(i, j int) bool { return gs.index[next.Steps[i].StepID] < gs.index[next.Steps[j].StepID] })
	path := filepath.Join(gs.dir, content.AnnotationsSubdir, sec.ID+".json")
	data, err := encodeJSON(next)
	if err != nil {
		return "", err
	}
	if err := content.ValidateAnnotationJSON(path, data); err != nil {
		return "", fmt.Errorf("annotate: built an annotation the schema rejects (a bug): %w", err)
	}
	if err := writeJSON(path, next); err != nil {
		return "", err
	}
	gs.sections[sec.ID] = next
	return path, nil
}

// placed is an objective at its step's position in the route.
type placed struct {
	step string
	at   int
	sec  *content.Section
	obj  content.Objective
}

// orderIssues checks the objectives of the candidate annotations against
// the rest of the route: the objectives converted from rules plus the
// current annotations, with the candidates replacing their steps' ones.
// Steps of different variants are never compared, nor steps of a parallel
// track with steps outside it.
func (gs *guideState) orderIssues(reg *entities.Registry, candidates map[string]content.StepAnnotation) map[string][]string {
	if len(candidates) == 0 {
		return nil
	}
	var all []placed
	for _, st := range gs.guide.Steps() {
		sec, _ := gs.guide.Section(st.SectionID)
		objs, _ := content.RuleObjectives(st, reg)
		if sa, ok := candidates[st.ID]; ok {
			objs = append(objs, sa.Objectives...)
		} else if sa := gs.annotation(st); sa != nil && sa.TextHash == content.StepTextHash(st) {
			objs = append(objs, sa.Objectives...)
		}
		// A rule objective the annotation repeats is placed once, so its
		// conflicts are reported once.
		seen := map[string]bool{}
		for _, o := range objs {
			if k := o.Key.String() + " " + o.Role; !seen[k] {
				seen[k] = true
				all = append(all, placed{step: st.ID, at: gs.index[st.ID], sec: sec, obj: o})
			}
		}
	}
	out := map[string][]string{}
	for _, p := range all {
		sa, isCandidate := candidates[p.step]
		if !isCandidate || !annotated(sa, p.obj) {
			continue
		}
		for _, q := range all {
			if q.step == p.step || !ordered(p, q) {
				continue
			}
			if msg := conflict(reg, p, q, all); msg != "" {
				out[p.step] = append(out[p.step], fmt.Sprintf("order: %s %s %s", p.obj.Key, p.obj.Role, msg))
			}
		}
	}
	return out
}

// annotated reports whether o comes from the annotation (not a rule).
func annotated(sa content.StepAnnotation, o content.Objective) bool {
	for _, x := range sa.Objectives {
		if x.Key == o.Key && x.Role == o.Role {
			return true
		}
	}
	return false
}

// ordered reports whether two placed objectives are in a known order: same
// variant, and the same section or neither in a parallel track.
func ordered(p, q placed) bool {
	if p.sec.VariantID != q.sec.VariantID {
		return false
	}
	return p.sec.ID == q.sec.ID || (!p.sec.IsTrack() && !q.sec.IsTrack())
}

// conflict explains why objective p cannot stand given objective q, or
// returns "".
func conflict(reg *entities.Registry, p, q placed, all []placed) string {
	pk, qk := p.obj.Key, q.obj.Key
	if pk.Ref != qk.Ref || pk.IsQuestPoints() != qk.IsQuestPoints() {
		return ""
	}
	isQuest := pk.Ref.Kind == entities.KindQuest
	switch p.obj.Role {
	case content.RoleStarts:
		if q.at < p.at && q.obj.Role == content.RoleCompletes && isQuest && qk.State == entities.StateFinished {
			return fmt.Sprintf("starts the quest after step %s finishes it", q.step)
		}
	case content.RoleCompletes:
		if isQuest && pk.State == entities.StateFinished && q.at > p.at && q.obj.Role == content.RoleStarts {
			return fmt.Sprintf("finishes the quest before step %s starts it", q.step)
		}
		if q.at < p.at && q.obj.Role == content.RoleCompletes && implies(qk, pk) {
			return fmt.Sprintf("is already completed by step %s (%s)", q.step, qk)
		}
	case content.RoleRequires:
		if q.at > p.at && q.obj.Role == content.RoleCompletes && implies(qk, pk) && !metBefore(p, all) && !questXPMeets(reg, p, all) {
			return fmt.Sprintf("is required here but the route only completes it later, at step %s", q.step)
		}
	}
	return ""
}

// implies reports whether reaching a means b is reached: the same key, or a
// count target at least as high on the same ref.
func implies(a, b entities.ObjectiveKey) bool {
	if a == b {
		return true
	}
	return a.Ref == b.Ref && a.Op == entities.OpAtLeast && b.Op == entities.OpAtLeast && a.N >= b.N
}

// metBefore reports whether some step before p (or p itself) completes what
// p requires.
func metBefore(p placed, all []placed) bool {
	for _, q := range all {
		if q.at <= p.at && ordered(p, q) && q.obj.Role == content.RoleCompletes && implies(q.obj.Key, p.obj.Key) {
			return true
		}
	}
	return false
}

// questXPMeets reports whether a skill level p requires is reached by the
// fixed experience of the quests the route finishes before p, on top of the
// skill's starting experience. Training is not modeled, so this only ever
// accepts a requirement, never rejects one.
func questXPMeets(reg *entities.Registry, p placed, all []placed) bool {
	pk := p.obj.Key
	if pk.Ref.Kind != entities.KindSkill || pk.Op != entities.OpAtLeast {
		return false
	}
	xp, counted := entities.StartingXP(pk.Ref.ID), map[string]bool{}
	for _, q := range all {
		qk := q.obj.Key
		if q.at >= p.at || !ordered(p, q) || q.obj.Role != content.RoleCompletes ||
			qk.Ref.Kind != entities.KindQuest || qk.State != entities.StateFinished || counted[qk.Ref.ID] {
			continue
		}
		counted[qk.Ref.ID] = true
		if quest, ok := reg.Quest(qk.Ref.ID); ok {
			for _, r := range quest.XPRewards {
				if r.Skill == pk.Ref.ID {
					xp += r.XP
				}
			}
		}
	}
	return entities.LevelForXP(xp) >= pk.N
}
