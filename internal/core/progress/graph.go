package progress

import (
	"cmp"
	"slices"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// Depth bounds of the requirement graph (Mundo › Grafo, V6Grafo1440): the
// columns to the left of the focus.
const (
	DefaultGraphDepth = 3
	MaxGraphDepth     = 6
	maxGraphUnlocks   = 12
)

// GraphNode is the focus or one of its requirements in the graph.
type GraphNode struct {
	GoalNode
	// Kind is the entity kind of the key (quest, skill, diary, item, ...;
	// "qp" for a quest point total).
	Kind string `json:"kind"`
	// Have is the account's level of a skill node (the quest point count of
	// a qp node) when a fact knows it.
	Have *int `json:"have,omitempty"`
	// Critical: the node is on the critical path.
	Critical bool `json:"critical,omitempty"`
}

// PathStep is one node of the critical path.
type PathStep struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// GraphLink is an objective the focus unlocks: a quest whose catalog
// requirements the focus reaches.
type GraphLink struct {
	Key     string        `json:"key"`
	Ref     string        `json:"ref"`
	Label   string        `json:"label"`
	Kind    string        `json:"kind"`
	Verdict rules.Verdict `json:"verdict"`
	// Missing counts its direct requirements that are not met (the focus
	// included when it is not met).
	Missing int `json:"missing"`
}

// Graph is the requirement graph of one focus (F10, docs/03): the closure
// of its catalog requirements against the account's facts, by depth, the
// critical path and what the focus unlocks. Only quests carry requirements in
// the catalog today; any other focus has no requirement columns.
type Graph struct {
	Focus string `json:"focus"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Depth is the deepest column returned; MaxDepth the deepest requirement
	// of the closure; Hidden counts the requirements deeper than Depth.
	Depth    int `json:"depth"`
	MaxDepth int `json:"max_depth"`
	Hidden   int `json:"hidden"`
	// Requirements, Met, Total and Presumed are the goal plan's counts over
	// the whole closure (the focus excluded).
	Requirements bool `json:"requirements"`
	Met          int  `json:"met"`
	Total        int  `json:"total"`
	Presumed     int  `json:"presumed"`
	// Nodes: the focus (depth 0) and its requirements up to Depth, in the
	// plan's suggested order (requirements before what needs them).
	Nodes []GraphNode `json:"nodes"`
	// Critical is the longest chain of requirements neither met nor
	// presumed, from the focus down to the farthest one, whatever the depth
	// shown; empty when the focus is met.
	Critical     []PathStep   `json:"critical"`
	Unlocks      []GraphLink  `json:"unlocks"`
	UnlocksTotal int          `json:"unlocks_total"`
	Bottlenecks  []Bottleneck `json:"bottlenecks"`
	// Routes are the steps of each guide that complete or start the focus.
	Routes []StepPlace `json:"routes"`
}

// BuildGraph computes the graph of in.Goal (the focus) up to depth columns
// (DefaultGraphDepth when out of 1..MaxGraphDepth).
func BuildGraph(in GoalPlanInput, depth int) Graph {
	if depth < 1 || depth > MaxGraphDepth {
		depth = DefaultGraphDepth
	}
	plan := BuildGoalPlan(in)
	p := planner{in: in}
	focus := in.Goal.String()
	out := Graph{
		Focus: focus, Label: p.label(in.Goal), Kind: keyKind(in.Goal), Depth: depth,
		Requirements: plan.Requirements, Met: plan.Met, Total: plan.Total, Presumed: plan.Presumed,
		Nodes: []GraphNode{}, Critical: []PathStep{}, Unlocks: []GraphLink{},
		Bottlenecks: plan.Bottlenecks, Routes: plan.Routes,
	}
	if out.Bottlenecks == nil {
		out.Bottlenecks = []Bottleneck{}
	}
	if out.Routes == nil {
		out.Routes = []StepPlace{}
	}
	labels := make(map[string]string, len(plan.Nodes))
	for _, n := range plan.Nodes {
		labels[n.Key] = n.Label
	}
	critical := map[string]bool{}
	for _, k := range criticalPath(plan.Nodes, focus) {
		critical[k] = true
		out.Critical = append(out.Critical, PathStep{Key: k, Label: labels[k]})
	}
	for _, n := range plan.Nodes {
		out.MaxDepth = max(out.MaxDepth, n.Depth)
		if n.Depth > depth {
			out.Hidden++
			continue
		}
		k, err := entities.ParseObjectiveKey(n.Key)
		if err != nil {
			continue
		}
		out.Nodes = append(out.Nodes, GraphNode{GoalNode: n, Kind: keyKind(k), Have: have(k, in), Critical: critical[n.Key]})
	}
	out.Unlocks, out.UnlocksTotal = p.unlocks(in.Goal)
	return out
}

func keyKind(k entities.ObjectiveKey) string {
	if k.IsQuestPoints() {
		return entities.QuestPoints
	}
	return string(k.Ref.Kind)
}

// have is the account's level in a skill key's skill, or the quest point
// count of a qp key, when the facts know it.
func have(k entities.ObjectiveKey, in GoalPlanInput) *int {
	switch {
	case k.Ref.Kind == entities.KindSkill:
		if s, ok := in.State.Skills[k.Ref.ID]; ok {
			level := s.Level
			return &level
		}
	case k.IsQuestPoints() && in.Registry != nil:
		for _, v := range in.Registry.VarsFor(entities.QuestPoints) {
			if v.Semantics != entities.VarCount {
				continue
			}
			vars := in.State.Varbits
			if v.Var == entities.Varp {
				vars = in.State.Varps
			}
			if n, ok := vars[v.ID]; ok {
				count := int(n)
				return &count
			}
		}
	}
	return nil
}

// criticalPath is the longest chain of unsatisfied requirements (neither met
// nor presumed) from the focus, following each node's direct requirements.
// Ties go to the requirement the plan orders first. Length is in nodes, not
// in time: the catalog has no estimates.
func criticalPath(nodes []GoalNode, focus string) []string {
	byKey := make(map[string]*GoalNode, len(nodes))
	order := make(map[string]int, len(nodes))
	for i := range nodes {
		byKey[nodes[i].Key] = &nodes[i]
		order[nodes[i].Key] = i
	}
	open := func(n *GoalNode) bool { return n.Group != GroupDone && n.Group != GroupPresumed }
	if f, ok := byKey[focus]; !ok || !open(f) {
		return []string{}
	}
	longest := map[string][]string{}
	visiting := map[string]bool{}
	var walk func(key string) []string
	walk = func(key string) []string {
		if p, ok := longest[key]; ok {
			return p
		}
		if visiting[key] {
			// A cycle in the catalog: the node is already on the path.
			return nil
		}
		visiting[key] = true
		var best []string
		reqs := slices.Clone(byKey[key].Requires)
		slices.SortFunc(reqs, func(a, b string) int { return cmp.Compare(order[a], order[b]) })
		for _, r := range reqs {
			n, ok := byKey[r]
			if !ok || !open(n) {
				continue
			}
			if p := walk(r); len(p) > len(best) {
				best = p
			}
		}
		visiting[key] = false
		path := append([]string{key}, best...)
		longest[key] = path
		return path
	}
	return walk(focus)
}

// unlocks are the quests whose catalog requirements the focus reaches: the
// available ones first, then by fewest missing requirements, then by name.
func (p *planner) unlocks(focus entities.ObjectiveKey) ([]GraphLink, int) {
	reg := p.in.Registry
	if reg == nil {
		return []GraphLink{}, 0
	}
	var out []GraphLink
	for _, id := range reg.QuestIDs() {
		q, _ := reg.Quest(id)
		hit := false
		for _, req := range q.Requires {
			if reaches(focus, req.Key) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		key := entities.ObjectiveKey{Ref: entities.Ref{Kind: entities.KindQuest, ID: id}, Op: entities.OpState, State: "finished"}
		l := GraphLink{Key: key.String(), Ref: key.Ref.String(), Label: p.label(key), Kind: string(entities.KindQuest), Verdict: p.verdict(key)}
		for _, req := range q.Requires {
			if p.verdict(req.Key) != rules.Met {
				l.Missing++
			}
		}
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b GraphLink) int {
		return cmp.Or(
			cmp.Compare(boolInt(a.Verdict == rules.Met), boolInt(b.Verdict == rules.Met)),
			cmp.Compare(a.Missing, b.Missing),
			cmp.Compare(a.Label, b.Label),
		)
	})
	total := len(out)
	if total > maxGraphUnlocks {
		out = out[:maxGraphUnlocks]
	}
	if out == nil {
		out = []GraphLink{}
	}
	return out, total
}
