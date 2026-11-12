package progress

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Graph depth bounds (F10): the graph always enters by a focus and shows its
// requirements up to MaxGraphDepth levels away, never the whole catalog.
const (
	DefaultGraphDepth = 2
	MaxGraphDepth     = 3
)

// GraphInput is a goal plan input whose Goal is the focus, plus the depth
// of requirements to show.
type GraphInput struct {
	GoalPlanInput
	Depth int
}

// GraphNode is one card of the graph: an objective, or the level
// requirements of one quest grouped (Kind "levels").
type GraphNode struct {
	Key     string        `json:"key"`
	Ref     string        `json:"ref,omitempty"`
	Kind    string        `json:"kind"`
	Label   string        `json:"label"`
	Depth   int           `json:"depth"`
	Verdict rules.Verdict `json:"verdict"`
	Group   GoalGroup     `json:"group"`
	// For are the labels of the nodes one column closer to the focus that
	// require this one.
	For []string `json:"for,omitempty"`
	// Missing are the labels of the direct requirements not yet met.
	Missing []string `json:"missing,omitempty"`
	// Requires counts the direct requirements (levels and quests).
	Requires int `json:"requires,omitempty"`
	// Step is the first step of the active route that completes it.
	Step     *StepPlace `json:"step,omitempty"`
	Critical bool       `json:"critical,omitempty"`
	// Levels are the members of a "levels" node.
	Levels []GraphLevel `json:"levels,omitempty"`
}

// GraphLevel is one level requirement inside a "levels" node.
type GraphLevel struct {
	Key     string        `json:"key"`
	Skill   string        `json:"skill"`
	Target  int           `json:"target"`
	Level   *int          `json:"level,omitempty"`
	Verdict rules.Verdict `json:"verdict"`
}

// GraphColumn is the requirements at one depth: the open ones and, apart,
// the done ones (the screen collapses them into "+N feitos").
type GraphColumn struct {
	Depth int         `json:"depth"`
	Nodes []GraphNode `json:"nodes"`
	Done  []GraphNode `json:"done"`
}

// Graph is the focus graph of F10 (V6Grafo1440/640): the requirement
// columns from the focus outward, the focus, and what it unlocks.
type Graph struct {
	Focus GraphNode `json:"focus"`
	Depth int       `json:"depth"`
	// MaxDepth is how deep the requirements go past the shown depth.
	MaxDepth int           `json:"max_depth"`
	Columns  []GraphColumn `json:"columns"`
	// Unlocks are the catalog quests that require the focus directly.
	Unlocks []GraphNode `json:"unlocks"`
	// Critical is the chain of open requirements with the most open work
	// under them, from the focus outward.
	Critical []string `json:"critical"`
	// Nodes and DoneNodes count the cards of the shown columns.
	Nodes     int `json:"nodes"`
	DoneNodes int `json:"done_nodes"`
	// Routes are the steps of each guide that complete or start the focus.
	Routes []StepPlace `json:"routes"`
}

// BuildGraph computes the graph of one focus.
func BuildGraph(in GraphInput) Graph {
	depth := in.Depth
	if depth < 1 || depth > MaxGraphDepth {
		depth = DefaultGraphDepth
	}
	p := planner{in: in.GoalPlanInput, nodes: map[string]*GoalNode{}}
	p.index()
	p.walk(in.Goal)
	for _, n := range p.nodes {
		p.classify(n)
	}
	focusKey := in.Goal.String()
	g := Graph{Depth: depth, Columns: []GraphColumn{}, Unlocks: []GraphNode{}, Critical: []string{}, Routes: p.routes()}
	critical := p.criticalPath(focusKey)
	for i, k := range critical {
		if i > 0 && p.nodes[k].Depth <= depth {
			g.Critical = append(g.Critical, k)
		}
	}
	onPath := map[string]bool{}
	for _, k := range critical {
		onPath[k] = true
	}
	g.Focus = p.graphNode(p.nodes[focusKey], onPath)
	g.Focus.Critical = false

	columns := map[int]*GraphColumn{}
	column := func(d int) *GraphColumn {
		if c, ok := columns[d]; ok {
			return c
		}
		c := &GraphColumn{Depth: d, Nodes: []GraphNode{}, Done: []GraphNode{}}
		columns[d] = c
		return c
	}
	add := func(n GraphNode) {
		c := column(n.Depth)
		if n.Group == GroupDone {
			c.Done = append(c.Done, n)
			g.DoneNodes++
		} else {
			c.Nodes = append(c.Nodes, n)
		}
		g.Nodes++
	}
	for _, key := range p.order() {
		n := p.nodes[key]
		if !isLevel(n.Key) {
			g.MaxDepth = max(g.MaxDepth, n.Depth)
		}
		if n.Depth == 0 || n.Depth > depth || isLevel(n.Key) {
			continue
		}
		add(p.graphNode(n, onPath))
	}
	// Levels go as one card per node that requires them, one column out.
	for _, key := range p.order() {
		n := p.nodes[key]
		lv, ok := p.levelsNode(n, onPath)
		if !ok {
			continue
		}
		g.MaxDepth = max(g.MaxDepth, lv.Depth)
		if lv.Depth <= depth {
			add(lv)
		}
	}
	for d := 1; d <= depth; d++ {
		if c, ok := columns[d]; ok {
			g.Columns = append(g.Columns, *c)
		}
	}
	g.Unlocks = p.unlocks(in.Goal)
	return g
}

func isLevel(key string) bool {
	k, err := entities.ParseObjectiveKey(key)
	return err == nil && k.Ref.Kind == entities.KindSkill
}

func nodeKind(key string) string {
	k, err := entities.ParseObjectiveKey(key)
	switch {
	case err != nil:
		return "unknown"
	case k.IsQuestPoints():
		return "qp"
	}
	return string(k.Ref.Kind)
}

// graphLabel is the card's name: "70 Cooking", "175 quest points", the
// entity's name otherwise.
func (p *planner) graphLabel(n *GoalNode) string {
	k, err := entities.ParseObjectiveKey(n.Key)
	if err != nil {
		return n.Label
	}
	if k.IsQuestPoints() || k.Ref.Kind == entities.KindSkill {
		return strconv.Itoa(k.N) + " " + n.Label
	}
	return n.Label
}

func (p *planner) graphNode(n *GoalNode, onPath map[string]bool) GraphNode {
	out := GraphNode{
		Key: n.Key, Ref: n.Ref, Kind: nodeKind(n.Key), Label: p.graphLabel(n), Depth: n.Depth,
		Verdict: n.Verdict, Group: n.Group, Requires: len(n.Requires), Step: n.Step, Critical: onPath[n.Key],
	}
	for _, f := range n.For {
		if parent, ok := p.nodes[f]; ok && parent.Depth == n.Depth-1 {
			out.For = append(out.For, p.graphLabel(parent))
		}
	}
	for _, m := range n.Missing {
		if dep, ok := p.nodes[m]; ok {
			out.Missing = append(out.Missing, p.graphLabel(dep))
		}
	}
	return out
}

// levelsNode groups n's direct level requirements into one card.
func (p *planner) levelsNode(n *GoalNode, onPath map[string]bool) (GraphNode, bool) {
	var levels []GraphLevel
	open, unknown := 0, 0
	for _, r := range n.Requires {
		dep, ok := p.nodes[r]
		if !ok || !isLevel(r) {
			continue
		}
		k := entities.MustParseObjectiveKey(r)
		lv := GraphLevel{Key: r, Skill: dep.Label, Target: k.N, Verdict: dep.Verdict}
		if s, ok := p.in.State.Skills[k.Ref.ID]; ok {
			level := s.Level
			lv.Level = &level
		}
		switch dep.Verdict {
		case rules.Unmet:
			open++
		case rules.Unknown:
			unknown++
		}
		levels = append(levels, lv)
	}
	if len(levels) == 0 {
		return GraphNode{}, false
	}
	// Open levels first, then by how far they are.
	slices.SortStableFunc(levels, func(a, b GraphLevel) int {
		return cmp.Or(cmp.Compare(verdictRank(a.Verdict), verdictRank(b.Verdict)), cmp.Compare(b.Target, a.Target), cmp.Compare(a.Skill, b.Skill))
	})
	label := ""
	for i, lv := range levels {
		if i == 3 {
			label += "…"
			break
		}
		if i > 0 {
			label += " · "
		}
		label += strconv.Itoa(lv.Target) + " " + lv.Skill
	}
	out := GraphNode{
		Key: "levels:" + n.Key, Kind: "levels", Label: label, Depth: n.Depth + 1, Levels: levels,
		For: []string{p.graphLabel(n)}, Requires: len(levels), Critical: onPath[n.Key] && open > 0,
	}
	switch {
	case open > 0:
		out.Verdict, out.Group = rules.Unmet, GroupBlocked
	case unknown > 0:
		out.Verdict, out.Group = rules.Unknown, GroupUnknown
	default:
		out.Verdict, out.Group = rules.Met, GroupDone
	}
	return out, true
}

func verdictRank(v rules.Verdict) int {
	switch v {
	case rules.Unmet:
		return 0
	case rules.Unknown:
		return 1
	}
	return 2
}

// criticalPath starts at the focus and keeps taking the open requirement
// with the most open nodes under it (ties: the one the active route reaches
// first, then the label).
func (p *planner) criticalPath(focus string) []string {
	memo := map[string]int{}
	var openUnder func(key string, seen map[string]bool) int
	openUnder = func(key string, seen map[string]bool) int {
		if v, ok := memo[key]; ok {
			return v
		}
		if seen[key] {
			return 0
		}
		seen[key] = true
		n := p.nodes[key]
		total := 0
		if !p.satisfied(key) {
			total = 1
		}
		for _, r := range n.Requires {
			if _, ok := p.nodes[r]; ok && !p.satisfied(r) {
				total += openUnder(r, seen)
			}
		}
		memo[key] = total
		return total
	}
	path := []string{focus}
	seen := map[string]bool{focus: true}
	for cur := focus; ; {
		best, bestScore := "", 0
		for _, r := range p.nodes[cur].Requires {
			if _, ok := p.nodes[r]; !ok || seen[r] || p.satisfied(r) || isLevel(r) {
				continue
			}
			score := openUnder(r, map[string]bool{})
			if score > bestScore || (score == bestScore && best != "" && p.before(r, best)) {
				best, bestScore = r, score
			}
		}
		if best == "" {
			return path
		}
		path = append(path, best)
		seen[best] = true
		cur = best
	}
}

// before orders two nodes by the active route's first step, then label.
func (p *planner) before(a, b string) bool {
	na, nb := p.nodes[a], p.nodes[b]
	sa, sb := stepRank(na.Step), stepRank(nb.Step)
	if sa != sb {
		return sa < sb
	}
	return na.Label < nb.Label
}

func stepRank(s *StepPlace) string {
	if s == nil {
		return "￿"
	}
	return s.SectionID + "\x00" + strconv.Itoa(1000000+s.SectionPos)
}

// unlocks are the catalog quests that list the focus among their direct
// requirements, open ones first.
func (p *planner) unlocks(focus entities.ObjectiveKey) []GraphNode {
	out := []GraphNode{}
	if p.in.Registry == nil {
		return out
	}
	want := focus.String()
	for _, id := range p.in.Registry.QuestIDs() {
		q, ok := p.in.Registry.Quest(id)
		if !ok {
			continue
		}
		for _, r := range q.Requires {
			if r.Key.String() != want {
				continue
			}
			k, err := entities.ParseObjectiveKey("quest:" + id + "@finished")
			if err != nil {
				break
			}
			v := p.verdict(k)
			n := GraphNode{Key: k.String(), Ref: k.Ref.String(), Kind: "quest", Label: p.label(k), Depth: -1, Verdict: v, Group: GroupBlocked, Step: p.firstReaching(k)}
			switch v {
			case rules.Met:
				n.Group = GroupDone
			case rules.Unknown:
				n.Group = GroupUnknown
			}
			out = append(out, n)
			break
		}
	}
	slices.SortStableFunc(out, func(a, b GraphNode) int {
		return cmp.Or(cmp.Compare(boolInt(a.Group == GroupDone), boolInt(b.Group == GroupDone)), cmp.Compare(a.Label, b.Label))
	})
	return out
}
