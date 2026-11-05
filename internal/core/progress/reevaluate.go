package progress

import (
	"sort"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// Input is everything a reevaluation of one account reads.
type Input struct {
	Guides   []*content.Guide
	Registry *entities.Registry
	State    rules.PlayerState
	// Observations is the provenance of each fact in State.
	Observations map[Fact]FactObservation
	Objectives   map[string]ObjectiveRecord
	Marks        map[StepRef]Mark
	// LastStatus is the to_status of the newest step_status_events row of
	// each step; a step without events was Pending.
	LastStatus map[StepRef]Status
	Cursors    map[string]CursorRecord
	// Goals are extra objective keys to evaluate: the account's goals may
	// name objectives no step uses.
	Goals   []string
	Account Account
	Now     time.Time
}

// StepEvent is one step_status_events row.
type StepEvent struct {
	Ref   StepRef
	From  Status
	To    Status
	Cause Cause
}

// MarkWrite is a mark the engine writes (only implied marks).
type MarkWrite struct {
	Ref  StepRef
	Mark Mark
}

// CursorUpdate is a cursor to write; an empty StepID deletes it.
type CursorUpdate struct {
	GuideID string
	Cursor  CursorRecord
}

// Plan is what a reevaluation writes.
type Plan struct {
	Objectives      []ObjectiveUpdate
	ObjectiveEvents []VerdictChange
	Marks           []MarkWrite
	StepEvents      []StepEvent
	Cursors         []CursorUpdate
	// StepsChanged counts the engine-caused step status changes in every
	// guide (the steps_changed of the ingest response).
	StepsChanged int
}

// Reevaluate runs the reevaluation of docs/09 section 5, in order: evaluate
// every objective referenced by any guide (any role, plus the catalog
// requirements of the quests the steps complete), advance the stored
// verdicts, derive the status of every step of every guide and record what
// changed, apply the implied cascade on the active guide only, and place the
// cursors.
func Reevaluate(in Input) Plan {
	var plan Plan
	verdicts := Verdicts{}
	met := map[string]ObjectiveRecord{}
	for key, rec := range in.Objectives {
		if rec.Exists {
			verdicts[key] = rec.Verdict
			met[key] = rec
		}
	}
	for _, key := range objectiveKeys(in.Guides, in.Registry, in.Goals) {
		k, err := entities.ParseObjectiveKey(key)
		if err != nil {
			continue
		}
		ev := EvaluateObjective(k, in.State, in.Registry)
		upd, change := Advance(key, in.Objectives[key], ev, in.Observations, in.Account.ActiveGuideID, in.Now)
		if upd != nil {
			plan.Objectives = append(plan.Objectives, *upd)
			verdicts[key] = upd.Verdict
			met[key] = upd.Record()
		}
		if change != nil {
			plan.ObjectiveEvents = append(plan.ObjectiveEvents, *change)
		}
	}

	for _, g := range in.Guides {
		status := map[string]Status{}
		implied := map[string]bool{}
		steps, _ := guideSteps(g)
		for _, st := range steps {
			ref := StepRef{GuideID: g.ID, StepID: st.ID}
			var mark *Mark
			if m, ok := in.Marks[ref]; ok {
				mark = &m
			}
			status[st.ID] = StepStatus(st, verdicts, mark)
		}
		if g.ID == in.Account.ActiveGuideID {
			for _, w := range cascade(g, status, in, met) {
				plan.Marks = append(plan.Marks, w)
				status[w.Ref.StepID] = Implied
				implied[w.Ref.StepID] = true
			}
		}
		for _, st := range steps {
			ref := StepRef{GuideID: g.ID, StepID: st.ID}
			from := lastStatus(in.LastStatus, ref)
			to := status[st.ID]
			if from == to {
				continue
			}
			cause := CauseEngine
			switch {
			case implied[st.ID]:
				cause = CauseImplied
			case hasMark(in.Marks, ref):
				// A mark without a matching event (copied by a migration or
				// moved by an alias): record it, it is not an engine change.
				cause = CauseMigration
			default:
				plan.StepsChanged++
			}
			plan.StepEvents = append(plan.StepEvents, StepEvent{Ref: ref, From: from, To: to, Cause: cause})
		}
		cur := PlaceCursor(g, status, verdicts, in.Registry, in.Account, in.Cursors[g.ID])
		if cur != in.Cursors[g.ID] {
			plan.Cursors = append(plan.Cursors, CursorUpdate{GuideID: g.ID, Cursor: cur})
		}
	}
	return plan
}

// cascade returns the implied marks of the active guide (docs/09 decision
// 9.1). A trigger is a step that became AutoDone in this reevaluation, outside
// tracks, whose most recently met completing objective was met while this
// guide was the active one, and one of whose completing objectives went from
// a stored non-met verdict to met in this reevaluation (metAfterObservedUnmet). Every earlier visible checkbox step of the same
// section that is still Pending and unmarked becomes implied by the nearest
// later trigger. Because only transitions trigger, an implied mark the owner
// cleared does not come back.
func cascade(g *content.Guide, status map[string]Status, in Input, met map[string]ObjectiveRecord) []MarkWrite {
	var out []MarkWrite
	marked := map[string]bool{}
	for _, sec := range g.Sections {
		if sec.IsTrack() {
			continue
		}
		for i, st := range sec.Steps {
			ref := StepRef{GuideID: g.ID, StepID: st.ID}
			if status[st.ID] != AutoDone || lastStatus(in.LastStatus, ref) == AutoDone {
				continue
			}
			if latestMetGuide(st, met) != in.Account.ActiveGuideID || !metAfterObservedUnmet(st, in.Objectives, met) {
				continue
			}
			for _, prev := range sec.Steps[:i] {
				pref := StepRef{GuideID: g.ID, StepID: prev.ID}
				if marked[prev.ID] || hasMark(in.Marks, pref) || status[prev.ID] != Pending || !IsManual(prev) || !Visible(prev, in.Account) {
					continue
				}
				marked[prev.ID] = true
				out = append(out, MarkWrite{Ref: pref, Mark: Mark{Kind: MarkImplied, ImpliedBy: st.ID}})
			}
		}
	}
	return out
}

// metAfterObservedUnmet reports whether a completing objective of the step
// became met in this reevaluation after a stored verdict that was not met:
// progress the account just made. An objective met at its first observation
// (a new source or facet reporting what was done long ago, out of the
// route's order) is history, not progress, and triggers no cascade.
func metAfterObservedUnmet(st *content.Step, prev, met map[string]ObjectiveRecord) bool {
	for _, o := range Completing(st) {
		key := o.Key.String()
		before := prev[key]
		if before.Exists && before.Verdict != rules.Met && met[key].Verdict == rules.Met {
			return true
		}
	}
	return false
}

// latestMetGuide returns the active guide at the time the most recently met
// completing objective of the step became met.
func latestMetGuide(st *content.Step, met map[string]ObjectiveRecord) string {
	var (
		guide  string
		latest time.Time
	)
	for _, o := range Completing(st) {
		rec := met[o.Key.String()]
		if rec.Verdict != rules.Met || rec.FirstMetAt == nil {
			continue
		}
		if guide == "" || rec.FirstMetAt.After(latest) {
			guide, latest = rec.MetInGuideID, *rec.FirstMetAt
		}
	}
	return guide
}

func lastStatus(last map[StepRef]Status, ref StepRef) Status {
	if s, ok := last[ref]; ok && s.Valid() {
		return s
	}
	return Pending
}

func hasMark(marks map[StepRef]Mark, ref StepRef) bool {
	_, ok := marks[ref]
	return ok
}

// objectiveKeys returns the sorted keys of every objective of every guide,
// the catalog requirements that can block their steps, and the extra ones
// (goals).
func objectiveKeys(guides []*content.Guide, reg *entities.Registry, extra []string) []string {
	seen := map[string]bool{}
	for _, k := range extra {
		seen[k] = true
	}
	for _, g := range guides {
		for _, st := range g.Steps() {
			for _, o := range st.Objectives {
				seen[o.Key.String()] = true
			}
			for _, k := range CatalogRequires(st, reg) {
				seen[k] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
