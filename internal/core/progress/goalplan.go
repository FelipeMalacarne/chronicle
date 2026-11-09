package progress

import (
	"cmp"
	"slices"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// GoalPlanInput is everything a goal plan reads (F9, docs/09 section 3):
// the goal, the catalog with the quest requirements, the account's facts,
// the stored (latched) verdicts and one GuideInput per guide. The active
// guide is the one named by Account.ActiveGuideID.
type GoalPlanInput struct {
	Goal     entities.ObjectiveKey
	Registry *entities.Registry
	State    rules.PlayerState
	Verdicts Verdicts
	Guides   []GuideInput
}

// GoalGroup is the column of a goal requirement.
type GoalGroup string

// Goal groups. Unknown never counts as done (rule 2); Presumed is an unknown
// objective whose completing step the owner marked done: it unblocks what
// depends on it in the suggested order but stays out of the met count.
const (
	GroupDone      GoalGroup = "done"
	GroupPresumed  GoalGroup = "presumed"
	GroupAvailable GoalGroup = "available"
	GroupBlocked   GoalGroup = "blocked"
	GroupUnknown   GoalGroup = "unknown"
)

// StepPlace is where a guide does something: the step and its position
// within its section (1-based).
type StepPlace struct {
	GuideID    string `json:"guide_id"`
	StepID     string `json:"step_id"`
	Ref        string `json:"ref"`
	Title      string `json:"title"`
	SectionID  string `json:"section_id"`
	SectionPos int    `json:"section_pos"`
	Role       string `json:"role,omitempty"`
	// Goal: the step completes the goal key itself.
	Goal bool `json:"goal,omitempty"`
	Done bool `json:"done,omitempty"`
}

// GoalNode is the goal or one of its requirements (direct or transitive).
type GoalNode struct {
	Key   string `json:"key"`
	Ref   string `json:"ref,omitempty"`
	Label string `json:"label"`
	// Depth is 0 for the goal, 1 for its direct requirements, and so on.
	Depth       int           `json:"depth"`
	Verdict     rules.Verdict `json:"verdict"`
	Presumed    bool          `json:"presumed,omitempty"`
	Group       GoalGroup     `json:"group"`
	Boostable   bool          `json:"boostable,omitempty"`
	DuringQuest bool          `json:"during_quest,omitempty"`
	Requires    []string      `json:"requires,omitempty"`
	// Missing are the direct requirements neither met nor presumed.
	Missing []string `json:"missing,omitempty"`
	// For are the nodes of the plan that require this one.
	For []string `json:"for,omitempty"`
	// Step is the first step of the active route that completes it.
	Step *StepPlace `json:"step,omitempty"`
}

// Bottleneck is the highest level the plan needs in one skill (or the quest
// points, Skill "qp") against what the facts say.
type Bottleneck struct {
	Key         string `json:"key"`
	Skill       string `json:"skill"`
	Level       *int   `json:"level"`
	Target      int    `json:"target"`
	Met         bool   `json:"met"`
	Boostable   bool   `json:"boostable,omitempty"`
	DuringQuest bool   `json:"during_quest,omitempty"`
	// For are the quests that need this level.
	For []string `json:"for,omitempty"`
	// Step is the first step of the active route that reaches the target;
	// without one, Reach is the highest level the route completes (ReachStep).
	Step      *StepPlace `json:"step,omitempty"`
	Reach     int        `json:"reach,omitempty"`
	ReachStep *StepPlace `json:"reach_step,omitempty"`
}

// GoalPlan is the closure of a goal's requirements minus what is done, in
// a suggested order (F9).
type GoalPlan struct {
	Goal string `json:"goal"`
	// Requirements: the catalog knows requirements for the goal (quests
	// only today; an item goal has none).
	Requirements bool `json:"requirements"`
	// Met of Total requirements (the goal excluded); Presumed apart.
	Met      int `json:"met"`
	Total    int `json:"total"`
	Presumed int `json:"presumed"`
	// Nodes: the goal and its requirements, requirements before what needs
	// them, ties broken by the active route's order.
	Nodes       []GoalNode   `json:"nodes"`
	Bottlenecks []Bottleneck `json:"bottlenecks"`
	// Routes are the steps of each guide that complete or start the goal.
	Routes []StepPlace `json:"routes"`
}

const routePlacesPerGuide = 8

// BuildGoalPlan computes the plan of one goal.
func BuildGoalPlan(in GoalPlanInput) GoalPlan {
	p := planner{in: in, nodes: map[string]*GoalNode{}}
	p.index()
	goal := in.Goal.String()
	p.walk(in.Goal)

	out := GoalPlan{Goal: goal, Requirements: len(p.nodes[goal].Requires) > 0}
	for _, n := range p.nodes {
		p.classify(n)
	}
	for _, key := range p.order() {
		n := p.nodes[key]
		if n.Depth > 0 {
			out.Total++
			switch n.Group {
			case GroupDone:
				out.Met++
			case GroupPresumed:
				out.Presumed++
			}
		}
		out.Nodes = append(out.Nodes, *n)
	}
	out.Bottlenecks = p.bottlenecks()
	out.Routes = p.routes()
	return out
}

type planner struct {
	in    GoalPlanInput
	nodes map[string]*GoalNode
	// active is the active guide's main, visible steps in order.
	active []*content.Step
	places map[*content.Step]StepPlace
	// presumed: unknown keys completed by a step the owner marked done.
	presumed map[string]bool
}

// index walks every guide once: the active route's steps, each step's place
// and the keys whose completing step is done by a mark.
func (p *planner) index() {
	p.places = map[*content.Step]StepPlace{}
	p.presumed = map[string]bool{}
	for i := range p.in.Guides {
		gi := &p.in.Guides[i]
		g := gi.Guide
		status := gi.statuses(nil)
		_, main := guideSteps(g)
		isActive := g.ID == gi.Account.ActiveGuideID
		for _, sec := range g.Sections {
			for pos, st := range sec.Steps {
				if _, seen := p.places[st]; seen {
					continue
				}
				p.places[st] = StepPlace{
					GuideID: g.ID, StepID: st.ID, Ref: AuthorRef(st), Title: StepTitle(st.Text),
					SectionID: sec.ID, SectionPos: pos + 1, Done: status[st.ID].Done(),
				}
				if !Visible(st, gi.Account) {
					continue
				}
				if isActive && main[st.ID] {
					p.active = append(p.active, st)
				}
				if s := status[st.ID]; s == ManualDone || s == Implied {
					for _, o := range Completing(st) {
						p.presumed[o.Key.String()] = true
					}
				}
			}
		}
	}
}

// walk adds the goal and, through the quests, every requirement under it.
func (p *planner) walk(goal entities.ObjectiveKey) {
	type item struct {
		key   entities.ObjectiveKey
		depth int
		req   *entities.QuestRequirement
	}
	queue := []item{{key: goal}}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		key := it.key.String()
		if n, ok := p.nodes[key]; ok {
			// A requirement reached on another path keeps the shallowest depth;
			// it is boostable or during-quest only when every path says so.
			if it.req != nil {
				n.Boostable = n.Boostable && it.req.Boostable
				n.DuringQuest = n.DuringQuest && it.req.DuringQuest
			}
			continue
		}
		n := &GoalNode{Key: key, Label: p.label(it.key), Depth: it.depth, Verdict: p.verdict(it.key)}
		if !it.key.IsQuestPoints() {
			n.Ref = it.key.Ref.String()
		}
		if it.req != nil {
			n.Boostable, n.DuringQuest = it.req.Boostable, it.req.DuringQuest
		}
		n.Presumed = n.Verdict == rules.Unknown && p.presumed[key]
		n.Step = p.firstReaching(it.key)
		p.nodes[key] = n
		if it.key.Ref.Kind != entities.KindQuest || p.in.Registry == nil {
			continue
		}
		q, ok := p.in.Registry.Quest(it.key.Ref.ID)
		if !ok {
			continue
		}
		for i := range q.Requires {
			req := q.Requires[i]
			n.Requires = append(n.Requires, req.Key.String())
			queue = append(queue, item{key: req.Key, depth: it.depth + 1, req: &req})
		}
	}
	for _, n := range p.nodes {
		for _, r := range n.Requires {
			if dep, ok := p.nodes[r]; ok {
				dep.For = append(dep.For, n.Key)
			}
		}
	}
	for _, n := range p.nodes {
		slices.Sort(n.For)
	}
}

// verdict is the latched verdict when met, else the live evaluation.
func (p *planner) verdict(k entities.ObjectiveKey) rules.Verdict {
	if p.in.Verdicts.Of(k.String()) == rules.Met {
		return rules.Met
	}
	return EvaluateObjective(k, p.in.State, p.in.Registry).Verdict
}

func (p *planner) satisfied(key string) bool {
	n, ok := p.nodes[key]
	return ok && (n.Verdict == rules.Met || n.Presumed)
}

func (p *planner) classify(n *GoalNode) {
	for _, r := range n.Requires {
		if !p.satisfied(r) {
			n.Missing = append(n.Missing, r)
		}
	}
	switch {
	case n.Verdict == rules.Met:
		n.Group = GroupDone
	case n.Presumed:
		n.Group = GroupPresumed
	case n.Verdict == rules.Unknown:
		n.Group = GroupUnknown
	case len(n.Missing) == 0 && !trainOnly(n.Key):
		// Nothing left before it (a quest without requirements included).
		n.Group = GroupAvailable
	default:
		// An unmet level or quest point total is a bottleneck: there is
		// nothing to start, only to train.
		n.Group = GroupBlocked
	}
}

// trainOnly reports a level or quest point total: never "available", since
// there is nothing to start, only to train.
func trainOnly(key string) bool {
	k, err := entities.ParseObjectiveKey(key)
	return err != nil || k.IsQuestPoints() || k.Ref.Kind == entities.KindSkill
}

// order is a topological order of the plan (requirements first); among the
// ready nodes, the one the active route reaches first goes first, then the
// deepest, then by label.
func (p *planner) order() []string {
	pending := map[string]int{}
	for k, n := range p.nodes {
		pending[k] = 0
		for _, r := range n.Requires {
			if _, ok := p.nodes[r]; ok {
				pending[k]++
			}
		}
	}
	pos := map[string]int{}
	for i, st := range p.active {
		pos[st.ID] = i
	}
	rank := func(n *GoalNode) int {
		if n.Step != nil {
			if i, ok := pos[n.Step.StepID]; ok {
				return i
			}
		}
		return len(p.active)
	}
	less := func(a, b string) int {
		na, nb := p.nodes[a], p.nodes[b]
		return cmp.Or(cmp.Compare(rank(na), rank(nb)), cmp.Compare(nb.Depth, na.Depth), cmp.Compare(na.Label, nb.Label), cmp.Compare(a, b))
	}
	var ready, out []string
	for k, c := range pending {
		if c == 0 {
			ready = append(ready, k)
		}
	}
	for len(ready) > 0 {
		slices.SortFunc(ready, less)
		k := ready[0]
		ready = ready[1:]
		out = append(out, k)
		for _, d := range p.nodes[k].For {
			pending[d]--
			if pending[d] == 0 {
				ready = append(ready, d)
			}
		}
	}
	// A cycle in the catalog (it validates against self-requirements only)
	// would leave nodes out; append them so nothing is lost.
	if len(out) < len(p.nodes) {
		var rest []string
		for k := range p.nodes {
			if !slices.Contains(out, k) {
				rest = append(rest, k)
			}
		}
		slices.SortFunc(rest, less)
		out = append(out, rest...)
	}
	return out
}

// firstReaching is the first step of the active route that completes k: the
// key itself or, for a level, the same skill at that level or higher.
func (p *planner) firstReaching(k entities.ObjectiveKey) *StepPlace {
	for _, st := range p.active {
		for _, o := range Completing(st) {
			if reaches(o.Key, k) {
				pl := p.places[st]
				pl.Role = o.Role
				return &pl
			}
		}
	}
	return nil
}

func reaches(have, want entities.ObjectiveKey) bool {
	if have == want {
		return true
	}
	if want.Ref.Kind == entities.KindSkill || want.IsQuestPoints() {
		return have.Ref == want.Ref && have.Op == want.Op && have.N >= want.N
	}
	return false
}

func (p *planner) bottlenecks() []Bottleneck {
	best := map[string]*GoalNode{}
	for _, n := range p.nodes {
		k, err := entities.ParseObjectiveKey(n.Key)
		if err != nil || (k.Ref.Kind != entities.KindSkill && !k.IsQuestPoints()) {
			continue
		}
		skill := "qp"
		if !k.IsQuestPoints() {
			skill = k.Ref.ID
		}
		if cur, ok := best[skill]; !ok || target(cur) < k.N {
			best[skill] = n
		}
	}
	out := []Bottleneck{}
	for skill, n := range best {
		k := entities.MustParseObjectiveKey(n.Key)
		b := Bottleneck{
			Key: n.Key, Skill: skill, Target: k.N, Met: n.Verdict == rules.Met,
			Boostable: n.Boostable, DuringQuest: n.DuringQuest, For: n.For, Step: n.Step,
		}
		if s, ok := p.in.State.Skills[skill]; ok && skill != "qp" {
			level := s.Level
			b.Level = &level
		}
		if b.Step == nil {
			b.Reach, b.ReachStep = p.reach(k)
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b Bottleneck) int {
		return cmp.Or(
			cmp.Compare(boolInt(a.Met), boolInt(b.Met)),
			cmp.Compare(boolInt(a.Skill == "qp"), boolInt(b.Skill == "qp")),
			cmp.Compare(gap(b), gap(a)),
			cmp.Compare(a.Skill, b.Skill),
		)
	})
	return out
}

// reach is the highest level of k's skill the active route completes, and
// the step that completes it.
func (p *planner) reach(k entities.ObjectiveKey) (int, *StepPlace) {
	level, at := 0, (*StepPlace)(nil)
	for _, st := range p.active {
		for _, o := range Completing(st) {
			if o.Key.Ref == k.Ref && o.Key.Op == k.Op && o.Key.N > level {
				pl := p.places[st]
				pl.Role = o.Role
				level, at = o.Key.N, &pl
			}
		}
	}
	return level, at
}

func target(n *GoalNode) int {
	k, err := entities.ParseObjectiveKey(n.Key)
	if err != nil {
		return 0
	}
	return k.N
}

func gap(b Bottleneck) int {
	if b.Level == nil {
		return 0
	}
	return b.Target - *b.Level
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// routes lists, per guide, the steps that complete or start the goal's
// entity (the active guide first).
func (p *planner) routes() []StepPlace {
	out := []StepPlace{}
	goal := p.in.Goal
	guides := slices.Clone(p.in.Guides)
	slices.SortStableFunc(guides, func(a, b GuideInput) int {
		return cmp.Compare(boolInt(b.Guide.ID == b.Account.ActiveGuideID), boolInt(a.Guide.ID == a.Account.ActiveGuideID))
	})
	for _, gi := range guides {
		steps, main := guideSteps(gi.Guide)
		n := 0
		for _, st := range steps {
			if !main[st.ID] || !Visible(st, gi.Account) {
				continue
			}
			for _, o := range Completing(st) {
				if o.Key.Ref != goal.Ref || (goal.IsQuestPoints() && !o.Key.IsQuestPoints()) {
					continue
				}
				pl := p.places[st]
				pl.Role, pl.Goal = o.Role, reaches(o.Key, goal)
				out = append(out, pl)
				n++
				break
			}
			if n == routePlacesPerGuide {
				break
			}
		}
	}
	return out
}

func (p *planner) label(k entities.ObjectiveKey) string {
	if k.IsQuestPoints() {
		return "quest points"
	}
	if p.in.Registry != nil {
		if name, ok := p.in.Registry.Display(k.Ref); ok {
			return name
		}
	}
	return k.Ref.ID
}
