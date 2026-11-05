package progress

import (
	"slices"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
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

// Blocked returns the keys of the requirements that block a step (docs/09
// section 3.5), sorted: its requires objectives whose verdict is unmet, and
// the unmet catalog requirements of each quest it completes (CatalogRequires).
// Only unmet blocks: unknown never does, and a met verdict is latched.
func Blocked(st *content.Step, verdicts Verdicts, reg *entities.Registry) []string {
	var out []string
	add := func(key string) {
		if verdicts.Of(key) == rules.Unmet && !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	for _, o := range st.Objectives {
		if o.Role == content.RoleRequires {
			add(o.Key.String())
		}
	}
	for _, k := range CatalogRequires(st, reg) {
		add(k)
	}
	slices.Sort(out)
	return out
}

// CatalogRequires returns the catalog requirements (Registry.Quest(id).Requires)
// of every quest the step completes, as objective keys: a floating step
// ("Complete Perilous Moons when you reach 48 Slayer") is blocked by the
// quest's own requirements even when its content only names the quest. Left
// out: requirements met during the quest, boostable ones (the guides count on
// boosts) and the objectives the step itself completes.
func CatalogRequires(st *content.Step, reg *entities.Registry) []string {
	if reg == nil {
		return nil
	}
	completing := Completing(st)
	completes := func(k entities.ObjectiveKey) bool {
		return slices.ContainsFunc(completing, func(o content.Objective) bool { return o.Key == k })
	}
	var out []string
	for _, o := range completing {
		if o.Role != content.RoleCompletes || o.Key.Ref.Kind != entities.KindQuest || o.Key.State != entities.StateFinished {
			continue
		}
		q, ok := reg.Quest(o.Key.Ref.ID)
		if !ok {
			continue
		}
		for _, req := range q.Requires {
			if req.DuringQuest || req.Boostable || completes(req.Key) {
				continue
			}
			if k := req.Key.String(); !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// PlaceCursor returns the cursor of a guide (docs/09 section 3.5): a pin stays
// while the pinned step exists and is not done; otherwise the cursor is the
// first visible step of the main route that is neither done nor skipped nor
// blocked (a pending step with an unmet requirement, Blocked). Implied steps
// count as done here (owner decision of 2026-11-04), so a cascade never pulls
// the cursor back; a blocked step pulls it back once its requirements are met.
// A pinned_open mark holds the cursor even on a blocked step. The empty
// record means the main route is finished.
func PlaceCursor(g *content.Guide, status map[string]Status, verdicts Verdicts, reg *entities.Registry, acc Account, cur CursorRecord) CursorRecord {
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
		if s != PinnedOpen && len(Blocked(st, verdicts, reg)) > 0 {
			continue
		}
		return CursorRecord{StepID: st.ID, Cause: CursorAuto}
	}
	return CursorRecord{}
}
