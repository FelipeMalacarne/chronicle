package plan

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Statuses of a Next.
const (
	// StatusTask: Task is the next task to do (its termination is unmet or
	// unknown, never proven met).
	StatusTask = "task"
	// StatusWaiting: every task of the current item is met, but the item is
	// not done yet (its step waits for its objectives or the owner's mark,
	// its recipe for its objective).
	StatusWaiting = "waiting"
	// StatusManual: the current item has no typed task the plan can give
	// (a step without tasks, a requirement without a recipe): the owner
	// does it by the guide's text.
	StatusManual = "manual"
	// StatusDone: nothing is left (the route is finished, every item done).
	StatusDone = "done"
)

// Board is the account's board of one guide and its cursor.
type Board struct {
	Steps  []progress.StepState
	Cursor string
}

// Input is what Next reads.
type Input struct {
	Plan     Plan
	Content  *content.Content
	Registry *entities.Registry
	// Boards are the account's boards of the guides the plan uses (the
	// route's guide, the guide of each step item), by guide id.
	Boards   map[string]Board
	Holdings rules.ItemHoldings
	// Facts are the account's mode, merged facts and last position.
	Facts panel.Facts
	Now   time.Time
}

// Ref names the plan of an answer.
type Ref struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Guide string `json:"guide,omitempty"`
}

// ItemView is the plan's current item.
type ItemView struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	// Position is the item's index in the plan; for a route plan, the
	// step's index in the route.
	Position int `json:"position"`
	// A step item: the guide, the step, the author's reference, the start
	// of its text, its status and the guide's credit line (rule 6).
	Guide       string          `json:"guide,omitempty"`
	Step        string          `json:"step,omitempty"`
	Ref         string          `json:"ref,omitempty"`
	Title       string          `json:"title,omitempty"`
	Status      progress.Status `json:"status,omitempty"`
	Attribution string          `json:"attribution,omitempty"`
	// A recipe item: its objective key and label.
	Recipe string `json:"recipe,omitempty"`
	Label  string `json:"label,omitempty"`
}

// TaskView is one task of the plan, evaluated now.
type TaskView struct {
	// Key names the task (TaskKey): the POST /api/microbot/task guard and
	// the executor's record.
	Key   string `json:"key"`
	Item  string `json:"item"`
	Index int    `json:"index"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Until string `json:"until"`
	// State is the termination's verdict now; unknown is never "to do"
	// by default, but only met is skipped.
	State  rules.Verdict `json:"state"`
	Reason string        `json:"reason,omitempty"`
	// Recipe is the recipe the task comes from (its objective key).
	Recipe string `json:"recipe,omitempty"`
	// For is the unmet requirement of the item the task was inserted for
	// (docs/10 section 7, rule 3: only from a recipe).
	For string `json:"for,omitempty"`
	// Risks are the risk kinds (E9); Supervised says the task runs only
	// with the owner watching (HCIM and a death risk).
	Risks      []string `json:"risks"`
	Supervised bool     `json:"supervised"`
	// Executable: the phase 3 executor runs it (travel, bank, buy);
	// NotExecutable says why not.
	Executable    bool   `json:"executable"`
	NotExecutable string `json:"not_executable,omitempty"`
	// Task is the typed task itself: catalog refs, numbers and ids.
	Task content.Task `json:"task"`
}

// Result is the next task of a plan.
type Result struct {
	Plan   Ref    `json:"plan"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// Item is the current item; null when the plan is done.
	Item *ItemView `json:"item"`
	// Task is the next task; null unless Status is task.
	Task *TaskView `json:"task"`
	// Skipped are the tasks before Task whose termination is already met,
	// in order: the plan steps over them (idempotence, docs/10 section 7
	// rule 2).
	Skipped []TaskView `json:"skipped"`
}

// scope is what a task's termination is evaluated against: its step's
// state, segment and guide, or a recipe's objective.
type scope struct {
	state   progress.StepState
	segment *content.Segment
	guide   *content.Guide
	recipe  *content.Recipe
	variant *content.RecipeVariant
	forKey  string
}

// Next returns the next task of the plan: the first task of the first
// unfinished item whose termination is not met.
func Next(in Input) Result {
	out := Result{Plan: Ref{ID: in.Plan.ID, Kind: in.Plan.Kind, Title: in.Plan.Title, Guide: in.Plan.Guide}, Skipped: []TaskView{}}
	if in.Plan.Kind == KindRoute {
		g, ok := in.Content.Guide(in.Plan.Guide)
		b, bok := in.Boards[in.Plan.Guide]
		if !ok || !bok {
			out.Status, out.Reason = StatusManual, "a rota não está no conteúdo carregado"
			return out
		}
		if b.Cursor == "" {
			out.Status, out.Reason = StatusDone, "a rota terminou"
			return out
		}
		for i, s := range b.Steps {
			if s.ID == b.Cursor {
				in.stepItem(&out, g, s, i)
				return out
			}
		}
		out.Status, out.Reason = StatusManual, "o cursor não é um passo da rota"
		return out
	}
	for i, it := range in.Plan.Items {
		switch it.Kind {
		case ItemStep:
			g, ok := in.Content.Guide(it.Guide)
			s, sok := in.stepState(it.Guide, it.Step)
			if !ok || !sok {
				out.Item = &ItemView{Kind: ItemStep, Key: it.Key(i), Position: i, Guide: it.Guide, Step: it.Step}
				out.Status, out.Reason = StatusManual, "o passo não existe mais no guia"
				return out
			}
			if s.Status.Done() || s.Status == progress.Skipped {
				continue
			}
			in.stepItem(&out, g, s, i)
			return out
		case ItemRecipe:
			if it.Recipe == nil {
				continue
			}
			if in.verdict(*it.Recipe) == rules.Met {
				continue
			}
			in.recipeItem(&out, *it.Recipe, i)
			return out
		case ItemTask:
			if it.Task == nil {
				continue
			}
			v := in.view(*it.Task, TaskItemKey(i), 0, scope{})
			if v.State == rules.Met {
				out.Skipped = append(out.Skipped, v)
				continue
			}
			out.Item = &ItemView{Kind: ItemTask, Key: TaskItemKey(i), Position: i, Label: v.Label}
			out.Status, out.Task = StatusTask, &v
			return out
		}
	}
	out.Status, out.Reason = StatusDone, "o plano não tem mais itens a fazer"
	return out
}

// stepState finds a step of a guide on the account's board.
func (in Input) stepState(guide, step string) (progress.StepState, bool) {
	b, ok := in.Boards[guide]
	if !ok {
		return progress.StepState{}, false
	}
	for _, s := range b.Steps {
		if s.ID == step {
			return s, true
		}
	}
	return progress.StepState{}, false
}

// stepItem fills out with the next task of a step: first the recipes of
// its unmet requirements, then its own tasks (the recipes' of what it
// completes and the step's).
func (in Input) stepItem(out *Result, g *content.Guide, s progress.StepState, position int) {
	key := StepKey(g.ID, s.ID)
	out.Item = &ItemView{
		Kind: ItemStep, Key: key, Position: position, Guide: g.ID, Step: s.ID, Ref: s.Ref, Title: s.Title,
		Status: s.Status, Attribution: panel.Attribution(g),
	}
	st, ok := g.Step(s.ID)
	if !ok {
		out.Status, out.Reason = StatusManual, "o passo não existe mais no guia"
		return
	}
	// Rule 3: an unmet requirement is produced by its recipe; without one,
	// the owner gets it. Only proven unmet requirements block (BlockedBy).
	var lists [][]sourced
	for _, req := range s.BlockedBy {
		k, err := entities.ParseObjectiveKey(req)
		if err != nil {
			continue
		}
		r, ok := in.Content.Recipe(k)
		var v *content.RecipeVariant
		if ok {
			v, ok = r.VariantFor(in.Facts.Mode)
		}
		if !ok {
			out.Status = StatusManual
			out.Reason = fmt.Sprintf("falta %s e não há receita para isso: faça à mão", panel.Label(req, in.Registry))
			return
		}
		list := make([]sourced, len(v.Tasks))
		for i, t := range v.Tasks {
			list[i] = sourced{t: t, key: TaskKey(RecipeKey(k), i), index: i, sc: in.recipeScope(r, v, req)}
		}
		lists = append(lists, list)
	}
	seg, _ := g.Segment(st.SegmentID)
	own := in.Content.StepTasks(st, in.Facts.Mode)
	if len(lists) == 0 && len(own) == 0 {
		out.Status, out.Reason = StatusManual, "o passo não tem tarefas tipadas: siga o texto do guia"
		return
	}
	list := make([]sourced, len(own))
	for i, t := range own {
		list[i] = sourced{t: t.Task, key: TaskKey(key, i), index: i, sc: scope{state: s, segment: seg, guide: g, recipe: t.Recipe, variant: t.Variant}}
	}
	lists = append(lists, list)
	if in.first(out, lists) {
		return
	}
	out.Status, out.Reason = StatusWaiting, "as tarefas do passo estão cumpridas; falta o passo concluir (objetivos ou a sua marca)"
}

// recipeItem fills out with the next task of a recipe item.
func (in Input) recipeItem(out *Result, k entities.ObjectiveKey, position int) {
	key := RecipeKey(k)
	out.Item = &ItemView{Kind: ItemRecipe, Key: key, Position: position, Recipe: k.String(), Label: panel.Label(k.String(), in.Registry)}
	r, ok := in.Content.Recipe(k)
	var v *content.RecipeVariant
	if ok {
		v, ok = r.VariantFor(in.Facts.Mode)
	}
	if !ok {
		out.Status, out.Reason = StatusManual, "a receita não tem variante para o modo da conta"
		return
	}
	list := make([]sourced, len(v.Tasks))
	for i, t := range v.Tasks {
		list[i] = sourced{t: t, key: TaskKey(key, i), index: i, sc: in.recipeScope(r, v, "")}
	}
	if in.first(out, [][]sourced{list}) {
		return
	}
	out.Status, out.Reason = StatusWaiting, "as tarefas da receita estão cumpridas; falta o objetivo dela"
}

// sourced is one task of an item's list with its key and scope.
type sourced struct {
	t     content.Task
	key   string
	index int
	sc    scope
}

// first sets the first task of the lists whose termination is not met,
// noting the met ones before it as skipped; false when every one is met.
func (in Input) first(out *Result, lists [][]sourced) bool {
	for _, list := range lists {
		for _, s := range list {
			item, _, _ := SplitTaskKey(s.key)
			v := in.view(s.t, item, s.index, s.sc)
			if v.State == rules.Met {
				out.Skipped = append(out.Skipped, v)
				continue
			}
			out.Status, out.Task = StatusTask, &v
			return true
		}
	}
	return false
}

// recipeScope evaluates a recipe's tasks: goal_met is the recipe's own
// objective, on the account's facts.
func (in Input) recipeScope(r *content.Recipe, v *content.RecipeVariant, forKey string) scope {
	k := r.Objective.String()
	return scope{
		state:  progress.StepState{Objectives: []progress.ObjectiveState{{Key: k, Role: content.RoleCompletes, Verdict: in.verdict(r.Objective)}}},
		recipe: r, variant: v, forKey: forKey,
	}
}

// verdict evaluates an objective key on the account's facts.
func (in Input) verdict(k entities.ObjectiveKey) rules.Verdict {
	if in.Facts.State == nil {
		return rules.Unknown
	}
	return progress.EvaluateObjective(k, *in.Facts.State, in.Registry).Verdict
}

// view evaluates one task now.
func (in Input) view(t content.Task, item string, index int, sc scope) TaskView {
	v := TaskView{
		Key: TaskKey(item, index), Item: item, Index: index, Type: t.Type, Label: panel.TaskLabel(t, in.Registry),
		For: sc.forKey, Task: t,
	}
	if sc.recipe != nil {
		v.Recipe = sc.recipe.Objective.String()
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if u, ok := t.Ends(); ok {
		v.Until = panel.UntilLabel(t, u, in.Registry)
		v.State, v.Reason = panel.EvaluateUntil(t, u, sc.state, sc.segment, sc.guide, in.Holdings, in.Facts, in.Registry, now)
	} else {
		v.State, v.Reason = rules.Unknown, "a tarefa não diz quando termina"
	}
	v.Risks = Risks(t, sc.variant, in.Registry)
	v.Supervised = NeedsSupervision(in.Facts.Mode, v.Risks)
	v.Executable, v.NotExecutable = Executable(t, len(t.Items) > 0 || segmentItems(sc.segment), in.Facts.Mode, in.Registry)
	return v
}

// segmentItems reports whether a segment's loadout lists items a bank task
// can gather.
func segmentItems(seg *content.Segment) bool {
	if seg == nil || seg.Loadout == nil || seg.Loadout.Status == content.LoadoutUnknown {
		return false
	}
	return len(seg.Loadout.Inventory)+len(seg.Loadout.Equipment) > 0
}

// Find evaluates the task a key names, now: what the executor ran and the
// ingest must prove (POST /api/microbot/task, the run's proof). False when
// the key names no task of the plan for the account's mode.
func Find(in Input, key string) (TaskView, bool) {
	s, ok := in.locate(key)
	if !ok {
		return TaskView{}, false
	}
	item, _, _ := SplitTaskKey(key)
	return in.view(s.t, item, s.index, s.sc), true
}

// locate finds the task a key names and its scope.
func (in Input) locate(key string) (sourced, bool) {
	item, index, ok := SplitTaskKey(key)
	if !ok {
		return sourced{}, false
	}
	switch {
	case strings.HasPrefix(item, ItemStep+":"):
		guide, step, ok := strings.Cut(strings.TrimPrefix(item, ItemStep+":"), "::")
		if !ok {
			return sourced{}, false
		}
		g, gok := in.Content.Guide(guide)
		s, sok := in.stepState(guide, step)
		if !gok || !sok {
			return sourced{}, false
		}
		st, ok := g.Step(step)
		if !ok {
			return sourced{}, false
		}
		own := in.Content.StepTasks(st, in.Facts.Mode)
		if index >= len(own) {
			return sourced{}, false
		}
		seg, _ := g.Segment(st.SegmentID)
		t := own[index]
		return sourced{t: t.Task, key: key, index: index, sc: scope{state: s, segment: seg, guide: g, recipe: t.Recipe, variant: t.Variant}}, true
	case strings.HasPrefix(item, ItemRecipe+":"):
		k, err := entities.ParseObjectiveKey(strings.TrimPrefix(item, ItemRecipe+":"))
		if err != nil {
			return sourced{}, false
		}
		r, ok := in.Content.Recipe(k)
		if !ok {
			return sourced{}, false
		}
		v, ok := r.VariantFor(in.Facts.Mode)
		if !ok || index >= len(v.Tasks) {
			return sourced{}, false
		}
		return sourced{t: v.Tasks[index], key: key, index: index, sc: in.recipeScope(r, v, "")}, true
	case strings.HasPrefix(item, ItemTask+":"):
		pos, err := strconv.Atoi(strings.TrimPrefix(item, ItemTask+":"))
		if err != nil || pos < 0 || pos >= len(in.Plan.Items) || index != 0 {
			return sourced{}, false
		}
		it := in.Plan.Items[pos]
		if it.Kind != ItemTask || it.Task == nil {
			return sourced{}, false
		}
		return sourced{t: *it.Task, key: key, sc: scope{}}, true
	}
	return sourced{}, false
}
