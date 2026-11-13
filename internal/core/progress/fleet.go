package progress

import (
	"cmp"
	"slices"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// PinnedGoal is a goal one account pinned (F9), as the fleet shows it.
type PinnedGoal struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Priority   int        `json:"priority"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	AchievedAt *time.Time `json:"achieved_at,omitempty"`
}

// FleetMember is one account of the fleet as the fleet goals read it.
type FleetMember struct {
	Account string
	// Goals are the account's pinned goals (active or achieved) by canonical
	// objective key.
	Goals map[string]PinnedGoal
	// Regressed are the keys whose latched met verdict regressed.
	Regressed map[string]bool
	// Plan is what the account's goal plans read (Goal is set per key). Its
	// Verdicts are the stored, latched ones (objective_progress).
	Plan GoalPlanInput
}

// FleetInput is everything the fleet goals read (the Mundo da frota,
// V7Mundo1440, slice V10 of docs/ui/decisoes-canvas.md section 7.5).
type FleetInput struct {
	// Members are the accounts in the fleet's order.
	Members []FleetMember
	// Keys are more objectives of interest, listed after the pinned ones.
	Keys []entities.ObjectiveKey
}

// FleetGoal is one objective across the fleet.
type FleetGoal struct {
	Key string `json:"objective_key"`
	// Pinned counts the accounts that pursue it (pin it as a goal); Met and
	// Unknown count the accounts whose stored verdict is met or unknown.
	// Unknown is never counted as unmet.
	Pinned  int `json:"pinned"`
	Met     int `json:"met"`
	Unknown int `json:"unknown"`
	// Closest is the account nearest to it among the ones without it (the
	// ones that pursue it, when any does); empty when none has a measure
	// (closer).
	Closest string `json:"closest,omitempty"`
	// Accounts are the fleet's accounts, in its order.
	Accounts []FleetStanding `json:"accounts"`
}

// FleetStanding is one account against one objective.
type FleetStanding struct {
	Account string `json:"account"`
	// Verdict is the stored (latched) verdict; unknown without one.
	Verdict   rules.Verdict `json:"verdict"`
	Regressed bool          `json:"regressed,omitempty"`
	// Goal is the account's goal of this objective; nil when it does not
	// pursue it.
	Goal *PinnedGoal `json:"goal,omitempty"`
	// Requirements summarizes the account's goal plan (BuildGoalPlan).
	Requirements RequirementCount `json:"requirements"`
	// Level is the account's level in the skill of a level goal, from its
	// facts; nil for any other goal or without a fact.
	Level *int `json:"level,omitempty"`
	// Short are the levels and quest point totals of the plan known to be
	// below their target (unknown ones are left out). Level is nil without
	// a level fact, and always for quest points.
	Short []Shortfall `json:"short"`
	// Step is the first step of the account's active route that completes
	// the objective; StepsLeft are the steps of that route (main, visible)
	// neither done nor skipped up to it, itself included: 0 when it is done
	// or the stored verdict is met.
	Step      *StepPlace `json:"step,omitempty"`
	StepsLeft *int       `json:"steps_left,omitempty"`
	// Cursor is the account's cursor in its active route; nil when the main
	// route is finished or the account has no route.
	Cursor *StepPlace `json:"cursor,omitempty"`
}

// RequirementCount is how many of a goal plan's requirements are done (Met),
// presumed done by a mark, unknown, and how many there are; the rest are
// known unmet. QuestsLeft are the quest requirements known unmet.
type RequirementCount struct {
	Met        int `json:"met"`
	Presumed   int `json:"presumed"`
	Unknown    int `json:"unknown"`
	Total      int `json:"total"`
	QuestsLeft int `json:"quests_left"`
}

// Shortfall is a level (or the quest points, Skill "qp") a plan needs and
// the account does not have.
type Shortfall struct {
	Key    string `json:"key"`
	Skill  string `json:"skill"`
	Level  *int   `json:"level"`
	Target int    `json:"target"`
}

// BuildFleetGoals computes the fleet goals: every objective one of the
// members pinned (by the best priority among its pins, then the earliest
// pin, then the key), then the asked keys nobody pinned, each with every
// member's standing.
func BuildFleetGoals(in FleetInput) []FleetGoal {
	keys := fleetKeys(in)
	routes := make([]activeRoute, len(in.Members))
	for i, m := range in.Members {
		routes[i] = routeOf(m.Plan)
	}
	out := make([]FleetGoal, 0, len(keys))
	for _, k := range keys {
		g := FleetGoal{Key: k.String(), Accounts: make([]FleetStanding, 0, len(in.Members))}
		for i, m := range in.Members {
			s := standing(m, k, routes[i])
			if s.Goal != nil {
				g.Pinned++
			}
			switch s.Verdict {
			case rules.Met:
				g.Met++
			case rules.Unknown:
				g.Unknown++
			}
			g.Accounts = append(g.Accounts, s)
		}
		g.Closest = closest(k, g.Accounts)
		out = append(out, g)
	}
	return out
}

// fleetKeys orders the objectives: the pinned ones first, then in.Keys.
func fleetKeys(in FleetInput) []entities.ObjectiveKey {
	type pinned struct {
		key      entities.ObjectiveKey
		priority int
		at       time.Time
	}
	byKey := map[string]*pinned{}
	for _, m := range in.Members {
		for raw, g := range m.Goals {
			k, err := entities.ParseObjectiveKey(raw)
			if err != nil {
				continue
			}
			p, ok := byKey[k.String()]
			if !ok {
				byKey[k.String()] = &pinned{key: k, priority: g.Priority, at: g.CreatedAt}
				continue
			}
			p.priority = min(p.priority, g.Priority)
			if g.CreatedAt.Before(p.at) {
				p.at = g.CreatedAt
			}
		}
	}
	list := make([]*pinned, 0, len(byKey))
	for _, p := range byKey {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b *pinned) int {
		return cmp.Or(cmp.Compare(a.priority, b.priority), a.at.Compare(b.at), cmp.Compare(a.key.String(), b.key.String()))
	})
	out := make([]entities.ObjectiveKey, 0, len(list)+len(in.Keys))
	seen := map[string]bool{}
	for _, p := range list {
		out = append(out, p.key)
		seen[p.key.String()] = true
	}
	for _, k := range in.Keys {
		if !seen[k.String()] {
			out = append(out, k)
			seen[k.String()] = true
		}
	}
	return out
}

// standing is one member against one objective.
func standing(m FleetMember, k entities.ObjectiveKey, route activeRoute) FleetStanding {
	key := k.String()
	s := FleetStanding{
		Account: m.Account, Verdict: m.Plan.Verdicts.Of(key), Regressed: m.Regressed[key],
		Short: []Shortfall{}, Cursor: route.cursor,
	}
	if g, ok := m.Goals[key]; ok {
		s.Goal = &g
	}
	in := m.Plan
	in.Goal = k
	p := BuildGoalPlan(in)
	s.Requirements = RequirementCount{Met: p.Met, Presumed: p.Presumed, Total: p.Total}
	verdicts := make(map[string]rules.Verdict, len(p.Nodes))
	for _, n := range p.Nodes {
		verdicts[n.Key] = n.Verdict
		if n.Depth == 0 {
			s.Step = n.Step
			continue
		}
		switch n.Group {
		case GroupUnknown:
			s.Requirements.Unknown++
		case GroupAvailable, GroupBlocked:
			if nk, err := entities.ParseObjectiveKey(n.Key); err == nil && nk.Ref.Kind == entities.KindQuest {
				s.Requirements.QuestsLeft++
			}
		}
	}
	for _, b := range p.Bottlenecks {
		if verdicts[b.Key] == rules.Unmet {
			s.Short = append(s.Short, Shortfall{Key: b.Key, Skill: b.Skill, Level: b.Level, Target: b.Target})
		}
	}
	if k.Ref.Kind == entities.KindSkill && !k.IsQuestPoints() {
		if st, ok := m.Plan.State.Skills[k.Ref.ID]; ok {
			level := st.Level
			s.Level = &level
		}
	}
	switch {
	case s.Step != nil && s.Verdict == rules.Met:
		s.StepsLeft = new(int)
	case s.Step != nil:
		s.StepsLeft = route.stepsTo(s.Step.StepID)
	}
	return s
}

// closest is the account nearest to k among the ones whose stored verdict
// is not met: the ones that pursue it when any does, else all of them. The
// measure is the level of a level goal, else the share of the plan's
// requirements met; an account without one, or at zero, never wins. Ties
// keep the fleet's order.
func closest(k entities.ObjectiveKey, accounts []FleetStanding) string {
	pursued := slices.ContainsFunc(accounts, func(s FleetStanding) bool { return s.Goal != nil })
	best, bestNum, bestDen := "", 0, 1
	for _, s := range accounts {
		if s.Verdict == rules.Met || (pursued && s.Goal == nil) {
			continue
		}
		num, den, ok := closeness(k, s)
		if !ok || num <= 0 {
			continue
		}
		if best == "" || num*bestDen > bestNum*den {
			best, bestNum, bestDen = s.Account, num, den
		}
	}
	return best
}

// closeness is how near s is to k, as num of den.
func closeness(k entities.ObjectiveKey, s FleetStanding) (num, den int, ok bool) {
	if k.Ref.Kind == entities.KindSkill && !k.IsQuestPoints() {
		if s.Level == nil || k.N <= 0 {
			return 0, 0, false
		}
		return *s.Level, k.N, true
	}
	if s.Requirements.Total == 0 {
		return 0, 0, false
	}
	return s.Requirements.Met, s.Requirements.Total, true
}

// activeRoute is a member's active route as the fleet reads it: its guide,
// the derived status of each step and the cursor.
type activeRoute struct {
	in     *GuideInput
	status map[string]Status
	cursor *StepPlace
}

// routeOf finds the active route of a plan's input and places its cursor
// (PlaceCursor, as the board does).
func routeOf(plan GoalPlanInput) activeRoute {
	for i := range plan.Guides {
		gi := &plan.Guides[i]
		if gi.Guide == nil || gi.Guide.ID != gi.Account.ActiveGuideID {
			continue
		}
		r := activeRoute{in: gi, status: gi.statuses(nil)}
		reg := gi.Registry
		if reg == nil {
			reg = plan.Registry
		}
		cur := PlaceCursor(gi.Guide, r.status, gi.Verdicts, reg, gi.Account, gi.Cursor)
		if cur.StepID != "" {
			r.cursor = placeOf(gi.Guide, cur.StepID, r.status[cur.StepID].Done())
		}
		return r
	}
	return activeRoute{}
}

// stepsTo counts the steps of the main route, visible for the account,
// neither done nor skipped, up to stepID (included); 0 when stepID is done,
// nil when the route does not have it.
func (r activeRoute) stepsTo(stepID string) *int {
	if r.in == nil {
		return nil
	}
	steps, main := guideSteps(r.in.Guide)
	n := 0
	for _, st := range steps {
		if !main[st.ID] || !Visible(st, r.in.Account) {
			continue
		}
		s := r.status[st.ID]
		if st.ID == stepID && s.Done() {
			n = 0
			return &n
		}
		if !s.Done() && s != Skipped {
			n++
		}
		if st.ID == stepID {
			return &n
		}
	}
	return nil
}

// placeOf is where stepID is in g.
func placeOf(g *content.Guide, stepID string, done bool) *StepPlace {
	for _, sec := range g.Sections {
		for pos, st := range sec.Steps {
			if st.ID == stepID {
				return &StepPlace{
					GuideID: g.ID, StepID: st.ID, Ref: AuthorRef(st), Title: StepTitle(st.Text),
					SectionID: sec.ID, SectionPos: pos + 1, Done: done,
				}
			}
		}
	}
	return nil
}
