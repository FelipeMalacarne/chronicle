package content

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// Task is one typed task of the execution model (docs/10 section 7): what the
// executor does, with parameters that are catalog refs, numbers and ids only,
// and the condition that ends it. Tasks live in step annotations (Step.Tasks)
// and in recipes; content/schema/task.schema.json is their schema and Check
// their verifier.
//
// Parameters per type (the others must be empty):
//
//	travel    Location, Via?                  until at_location (the default)
//	bank      Items?                          until loadout_ready (the default)
//	buy       Shop, Item, Qty                 until item:<id>x<qty> (the default)
//	talk      NPC, Options?, Location?        until an objective or goal_met
//	interact  Object, Action, Location?       until an objective or goal_met
//	train     Skill, NPC? or Object?, Location?   until skill:<skill>>=N
//	manual    Note?                           until owner_mark, goal_met or an objective
//
// Options (dialogue options) and Action (a menu option) are the game's own
// text, matched exactly by the executor, never interpreted; Note is text for
// the owner.
type Task struct {
	Type     string            `json:"type"`
	Location *entities.Ref     `json:"location,omitempty"`
	Via      *entities.Ref     `json:"via,omitempty"`
	Items    []ItemRequirement `json:"items,omitempty"`
	Shop     *entities.Ref     `json:"shop,omitempty"`
	Item     *entities.Ref     `json:"item,omitempty"`
	Qty      int               `json:"qty,omitempty"`
	NPC      *entities.Ref     `json:"npc,omitempty"`
	Options  []string          `json:"options,omitempty"`
	Object   *entities.Ref     `json:"object,omitempty"`
	Action   string            `json:"action,omitempty"`
	Skill    *entities.Ref     `json:"skill,omitempty"`
	Note     string            `json:"note,omitempty"`
	Until    *TaskUntil        `json:"until,omitempty"`
}

// TaskUntil is the termination of a task: exactly one of an objective key
// (evaluated by the engine) and a state predicate.
type TaskUntil struct {
	Objective *entities.ObjectiveKey `json:"objective,omitempty"`
	State     string                 `json:"state,omitempty"`
}

// Task types: the executor's primitive list (docs/10, decision E5).
const (
	TaskTravel   = "travel"
	TaskBank     = "bank"
	TaskBuy      = "buy"
	TaskTalk     = "talk"
	TaskInteract = "interact"
	TaskTrain    = "train"
	TaskManual   = "manual"
	// TaskQuest is planned (E5: quest later, with quest recipes or the
	// Quest Helper's data, docs/10 phase 5); the verifier refuses it.
	TaskQuest = "quest"
)

// TaskTypes lists the task types the executor knows, in a stable order.
var TaskTypes = []string{TaskTravel, TaskBank, TaskBuy, TaskTalk, TaskInteract, TaskTrain, TaskManual}

// State predicates a task can end on.
const (
	// UntilAtLocation: the character stands in the area of the task's location.
	UntilAtLocation = "at_location"
	// UntilLoadoutReady: the carried setup meets the loadout (readiness).
	UntilLoadoutReady = "loadout_ready"
	// UntilGoalMet: the objectives the step completes (completes and starts),
	// or the recipe's objective, are met.
	UntilGoalMet = "goal_met"
	// UntilOwnerMark: the owner marks the step (manual tasks only).
	UntilOwnerMark = "owner_mark"
)

// TaskContext is what a task's termination and parameters can lean on: the
// objectives its step or recipe completes (Goal), whether the step's segment
// has a loadout a bank task without items is ready for, and whether the task
// is part of a recipe (no step text, no step to mark).
type TaskContext struct {
	Goal    []entities.ObjectiveKey
	Loadout bool
	Recipe  bool
}

// DefaultUntil is the termination a task type implies from its parameters
// (travel, bank and buy); ok is false for the types that must state one.
func (t Task) DefaultUntil() (TaskUntil, bool) {
	switch t.Type {
	case TaskTravel:
		return TaskUntil{State: UntilAtLocation}, true
	case TaskBank:
		return TaskUntil{State: UntilLoadoutReady}, true
	case TaskBuy:
		if t.Item == nil || t.Qty < 1 {
			return TaskUntil{}, false
		}
		id, ok := t.Item.Int()
		if !ok {
			return TaskUntil{}, false
		}
		k := entities.ObjectiveKey{Ref: entities.ItemRef(id), Op: entities.OpQuantity, N: t.Qty}
		return TaskUntil{Objective: &k}, true
	}
	return TaskUntil{}, false
}

// Ends returns the effective termination: Until, or the default of the type.
func (t Task) Ends() (TaskUntil, bool) {
	if t.Until != nil {
		return *t.Until, true
	}
	return t.DefaultUntil()
}

func (u TaskUntil) String() string {
	if u.Objective != nil {
		return u.Objective.String()
	}
	return u.State
}

// endsOn reports whether the termination is the objective key k or goal_met
// (the goal of a recipe is its key).
func (u TaskUntil) endsOn(k entities.ObjectiveKey) bool {
	return u.State == UntilGoalMet || u.Objective != nil && *u.Objective == k
}

// taskParams lists the parameters each type takes.
var taskParams = map[string][]string{
	TaskTravel:   {"location", "via"},
	TaskBank:     {"items"},
	TaskBuy:      {"shop", "item", "qty"},
	TaskTalk:     {"npc", "options", "location"},
	TaskInteract: {"object", "action", "location"},
	TaskTrain:    {"skill", "npc", "object", "location"},
	TaskManual:   {"note"},
}

// params names the parameters t sets.
func (t Task) params() []string {
	var out []string
	for name, set := range map[string]bool{
		"location": t.Location != nil, "via": t.Via != nil, "items": len(t.Items) > 0, "shop": t.Shop != nil,
		"item": t.Item != nil, "qty": t.Qty != 0, "npc": t.NPC != nil, "options": len(t.Options) > 0,
		"object": t.Object != nil, "action": t.Action != "", "skill": t.Skill != nil, "note": t.Note != "",
	} {
		if set {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// Check verifies a task (docs/10 section 7, rule 4): a type of the
// executor's primitive list, parameters of that type only, every ref a
// catalog entry of the right kind (a shop that sells the item, a travel
// destination with an area), and a termination the engine can evaluate in
// ctx. It returns every problem found, or nothing.
func (t Task) Check(reg *entities.Registry, ctx TaskContext) []string {
	var msgs []string
	add := func(format string, args ...any) { msgs = append(msgs, fmt.Sprintf(format, args...)) }
	allowed, known := taskParams[t.Type]
	switch {
	case t.Type == TaskQuest:
		return []string{"task type quest is not an executor primitive yet (docs/10, E5: quest later)"}
	case !known:
		return []string{fmt.Sprintf("unknown task type %q (one of %s)", t.Type, strings.Join(TaskTypes, ", "))}
	}
	for _, p := range t.params() {
		if !slices.Contains(allowed, p) {
			add("%s takes no %s", t.Type, p)
		}
	}
	// ref checks one ref parameter: set when required, of the kind, known.
	ref := func(name string, r *entities.Ref, required bool, kinds ...entities.Kind) {
		switch {
		case r == nil:
			if required {
				add("%s needs %s", t.Type, name)
			}
		case !slices.Contains(kinds, r.Kind):
			add("%s: %s is not of kind %s", name, r, kindList(kinds))
		case !reg.Known(*r):
			add("%s: unknown entity %s", name, r)
		}
	}
	switch t.Type {
	case TaskTravel:
		ref("location", t.Location, true, entities.KindLocation)
		if t.Location != nil && reg.Known(*t.Location) {
			if _, ok := reg.LocationArea(t.Location.ID); !ok {
				add("location: %s has no area (no wiki map), so arriving there cannot be evaluated", t.Location)
			}
		}
		ref("via", t.Via, false, entities.KindUnlock, entities.KindItem)
	case TaskBank:
		for i, it := range t.Items {
			for _, msg := range checkRequirement(it, reg) {
				add("items/%d: %s", i, msg)
			}
			if it.Variant() == RequireUnresolved {
				add("items/%d: a bank item must be resolved (item_id, any_of or family)", i)
			}
		}
		if len(t.Items) == 0 {
			switch {
			case ctx.Recipe:
				add("bank in a recipe needs items: a recipe has no segment loadout")
			case !ctx.Loadout:
				add("bank needs items: the step's segment has no loadout with items")
			}
		}
	case TaskBuy:
		ref("shop", t.Shop, true, entities.KindShop)
		ref("item", t.Item, true, entities.KindItem)
		if t.Qty < 1 {
			add("buy needs qty >= 1")
		}
		if t.Shop != nil && t.Item != nil && reg.Known(*t.Shop) && reg.Known(*t.Item) {
			id, _ := t.Item.Int()
			if _, ok := reg.ShopStockOf(t.Shop.ID, id); !ok {
				add("shop: %s does not sell %s (not in its wiki stock)", t.Shop, t.Item)
			}
		}
	case TaskTalk:
		ref("npc", t.NPC, true, entities.KindNPC)
		ref("location", t.Location, false, entities.KindLocation)
		for i, o := range t.Options {
			if strings.TrimSpace(o) == "" {
				add("options/%d: a dialogue option is the game's text, not empty", i)
			}
		}
	case TaskInteract:
		ref("object", t.Object, true, entities.KindObject)
		ref("location", t.Location, false, entities.KindLocation)
		if strings.TrimSpace(t.Action) == "" {
			add("interact needs action (the menu option)")
		}
	case TaskTrain:
		ref("skill", t.Skill, true, entities.KindSkill)
		ref("npc", t.NPC, false, entities.KindNPC)
		ref("object", t.Object, false, entities.KindObject)
		ref("location", t.Location, false, entities.KindLocation)
		if t.NPC != nil && t.Object != nil {
			add("train takes an npc or an object, not both")
		}
	case TaskManual:
		if ctx.Recipe && strings.TrimSpace(t.Note) == "" {
			add("manual in a recipe needs a note: there is no step text to show the owner")
		}
	}
	msgs = append(msgs, t.checkUntil(reg, ctx)...)
	return msgs
}

// checkUntil verifies that the termination fits the type and can be
// evaluated in ctx.
func (t Task) checkUntil(reg *entities.Registry, ctx TaskContext) []string {
	if t.Until != nil && (t.Until.Objective == nil) == (t.Until.State == "") {
		return []string{"until needs exactly one of objective and state"}
	}
	u, ok := t.Ends()
	switch {
	case !ok && (t.Type == TaskBuy || t.Type == TaskTravel || t.Type == TaskBank):
		return nil // the missing parameters are already reported
	case !ok:
		return []string{fmt.Sprintf("%s needs until", t.Type)}
	}
	if def, hasDef := t.DefaultUntil(); hasDef && t.Until != nil && def.String() != t.Until.String() {
		return []string{fmt.Sprintf("%s ends on %s; until must be omitted or say so, not %s", t.Type, def, t.Until)}
	}
	var msgs []string
	add := func(format string, args ...any) { msgs = append(msgs, fmt.Sprintf(format, args...)) }
	if u.Objective != nil {
		if msg := reg.KnownObjective(*u.Objective); msg != "" {
			add("until: %s", msg)
		}
	}
	states := map[string][]string{
		TaskTravel: {UntilAtLocation}, TaskBank: {UntilLoadoutReady}, TaskBuy: nil, TaskTrain: nil,
		TaskTalk: {UntilGoalMet}, TaskInteract: {UntilGoalMet}, TaskManual: {UntilGoalMet, UntilOwnerMark},
	}[t.Type]
	switch {
	case u.State != "" && !slices.Contains([]string{UntilAtLocation, UntilLoadoutReady, UntilGoalMet, UntilOwnerMark}, u.State):
		add("until: unknown state %q", u.State)
	case u.State != "" && !slices.Contains(states, u.State):
		add("until: %s cannot end on %s", t.Type, u.State)
	case u.State == UntilGoalMet && len(ctx.Goal) == 0:
		add("until goal_met: the step completes no objective, so there is nothing to evaluate")
	case u.State == UntilOwnerMark && ctx.Recipe:
		add("until owner_mark: a recipe has no step for the owner to mark")
	case u.Objective == nil && (t.Type == TaskBuy || t.Type == TaskTrain):
		add("until: %s ends on an objective", t.Type)
	}
	if t.Type == TaskTrain && u.Objective != nil && t.Skill != nil {
		if k := *u.Objective; k.Ref != *t.Skill || k.Op != entities.OpAtLeast {
			add("until: train %s ends on %s>=N, not %s", t.Skill, t.Skill, k)
		}
	}
	return msgs
}

func kindList(kinds []entities.Kind) string {
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = string(k)
	}
	return strings.Join(parts, " or ")
}

// Refs returns pointers to the entity refs the task names (its parameters
// and the subject of its until objective), so tools can find a ref or
// rename it in place.
func (t *Task) Refs() []*entities.Ref {
	var out []*entities.Ref
	for _, r := range []*entities.Ref{t.Location, t.Via, t.Shop, t.Item, t.NPC, t.Object, t.Skill} {
		if r != nil {
			out = append(out, r)
		}
	}
	if t.Until != nil && t.Until.Objective != nil && !t.Until.Objective.IsQuestPoints() {
		out = append(out, &t.Until.Objective.Ref)
	}
	return out
}

// AnnotatedTask is a task read from the step text: the task, the quote it
// came from and its provenance, like an annotated objective.
type AnnotatedTask struct {
	Task
	Evidence ObjectiveEvidence `json:"evidence"`
	Source   ObjectiveSource   `json:"source"`
}

// stepGoal returns the keys of the objectives a step completes, after the
// annotation sa is composed (its suppressions dropped, its objectives added).
func stepGoal(st *Step, sa *StepAnnotation) []entities.ObjectiveKey {
	objs := slices.DeleteFunc(slices.Clone(st.Objectives), sa.Suppresses)
	objs = mergeObjectives(objs, sa.Objectives)
	var out []entities.ObjectiveKey
	for _, o := range objs {
		if o.Completing() {
			out = append(out, o.Key)
		}
	}
	return out
}

// hasLoadoutItems reports whether a loadout lists any item.
func hasLoadoutItems(lo *Loadout) bool {
	return lo != nil && len(lo.Inventory)+len(lo.Equipment) > 0
}

// taskPointer is the JSON pointer of task i of a list.
func taskPointer(i int) string { return "/tasks/" + strconv.Itoa(i) }
