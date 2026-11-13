// Package plan is the account's plan (docs/10 section 9): the ordered list
// it follows, and the next typed task of it. A route plan is one guide, one
// to one, and comes from the content and the account's cursor, so nothing is
// copied; goal and custom plans list items that reference a step of a guide
// (guide, step), a recipe (its objective key) or one standalone typed task,
// never the guide's text (CLAUDE.md rule 7). Pure: the content, the boards
// and the facts come in, the next task comes out.
package plan

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// Plan kinds (docs/10 section 9).
const (
	// KindRoute is one guide, one to one: its items are the guide's steps
	// in route order and its position is the account's cursor.
	KindRoute = "route"
	// KindGoal is the plan of a goal (GET /api/goals/{id}/plan), stored.
	KindGoal = "goal"
	// KindCustom is put together by the owner from steps of several
	// guides, recipes and tasks (decision E7; the UI is phase 5).
	KindCustom = "custom"
)

// Kinds lists the plan kinds.
var Kinds = []string{KindRoute, KindGoal, KindCustom}

// Item kinds.
const (
	ItemStep   = "step"
	ItemRecipe = "recipe"
	ItemTask   = "task"
)

// MaxItems bounds a stored plan.
const MaxItems = 500

// MaxTitle bounds a plan's title, in characters.
const MaxTitle = 120

// routePrefix names the route plan of a guide: it has no stored row.
const routePrefix = "route:"

// RouteID is the id of the route plan of a guide.
func RouteID(guide string) string { return routePrefix + guide }

// RouteGuide returns the guide of a route plan id.
func RouteGuide(id string) (string, bool) {
	g, ok := strings.CutPrefix(id, routePrefix)
	return g, ok && g != ""
}

// Plan is one plan of an account.
type Plan struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Guide is the guide of a route plan.
	Guide string `json:"guide,omitempty"`
	// Goal is the goal (goals.id) of a goal plan.
	Goal string `json:"goal,omitempty"`
	// Items are the stored items of a goal or custom plan, in order; a
	// route plan has none (its steps come from the content).
	Items []Item `json:"items"`
}

// Item is one item of a stored plan: exactly one of a step (Guide, Step),
// a recipe and a task.
type Item struct {
	Kind   string                 `json:"kind"`
	Guide  string                 `json:"guide,omitempty"`
	Step   string                 `json:"step,omitempty"`
	Recipe *entities.ObjectiveKey `json:"recipe,omitempty"`
	Task   *content.Task          `json:"task,omitempty"`
}

// StepKey is the key of a step item ("step:oziris-main::1.1.3"): the guide
// and the step, as the web keys steps (rule 7).
func StepKey(guide, step string) string { return ItemStep + ":" + guide + "::" + step }

// RecipeKey is the key of a recipe item ("recipe:quest:cooks_assistant@finished").
func RecipeKey(k entities.ObjectiveKey) string { return ItemRecipe + ":" + k.String() }

// TaskItemKey is the key of the standalone task at position of a stored plan.
func TaskItemKey(position int) string { return ItemTask + ":" + strconv.Itoa(position) }

// TaskKey names one task of an item: the item's key and the task's index in
// the item's list (the step's tasks for the account's mode, the recipe
// variant's, 0 for a task item).
func TaskKey(item string, index int) string { return item + "#" + strconv.Itoa(index) }

// SplitTaskKey splits a task key into its item key and index.
func SplitTaskKey(key string) (string, int, bool) {
	i := strings.LastIndex(key, "#")
	if i <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(key[i+1:])
	if err != nil || n < 0 {
		return "", 0, false
	}
	return key[:i], n, true
}

// Key is the item's key.
func (it Item) Key(position int) string {
	switch it.Kind {
	case ItemStep:
		return StepKey(it.Guide, it.Step)
	case ItemRecipe:
		if it.Recipe != nil {
			return RecipeKey(*it.Recipe)
		}
	case ItemTask:
		return TaskItemKey(position)
	}
	return it.Kind + ":?"
}

// CheckTitle verifies a plan title.
func CheckTitle(title string) error {
	t := strings.TrimSpace(title)
	if t == "" || utf8.RuneCountInString(t) > MaxTitle || strings.ContainsAny(t, "\x00\r\n") {
		return fmt.Errorf("title: 1 to %d characters, one line", MaxTitle)
	}
	return nil
}

// CheckItems verifies the items of a goal or custom plan against the
// content: a step must be a step of its guide, a recipe must exist and a
// standalone task must pass the task verifier as a recipe's would (no step:
// no goal_met nor owner_mark, a bank task lists its items). It returns
// every problem, prefixed with the item's position.
func CheckItems(items []Item, c *content.Content, reg *entities.Registry) []string {
	var msgs []string
	add := func(i int, format string, args ...any) {
		msgs = append(msgs, fmt.Sprintf("items/%d: ", i)+fmt.Sprintf(format, args...))
	}
	if len(items) > MaxItems {
		return []string{fmt.Sprintf("a plan has at most %d items", MaxItems)}
	}
	for i, it := range items {
		set := 0
		if it.Guide != "" || it.Step != "" {
			set++
		}
		if it.Recipe != nil {
			set++
		}
		if it.Task != nil {
			set++
		}
		switch it.Kind {
		case ItemStep:
			if set != 1 || it.Guide == "" || it.Step == "" {
				add(i, "a step item has guide and step only")
				continue
			}
			g, ok := c.Guide(it.Guide)
			if !ok {
				add(i, "unknown guide %q", it.Guide)
				continue
			}
			if _, ok := g.Step(it.Step); !ok {
				add(i, "%s has no step %q", it.Guide, it.Step)
			}
		case ItemRecipe:
			if set != 1 || it.Recipe == nil {
				add(i, "a recipe item has recipe only")
				continue
			}
			if _, ok := c.Recipe(*it.Recipe); !ok {
				add(i, "no recipe for %s", it.Recipe)
			}
		case ItemTask:
			if set != 1 || it.Task == nil {
				add(i, "a task item has task only")
				continue
			}
			for _, msg := range it.Task.Check(reg, content.TaskContext{Recipe: true}) {
				add(i, "task: %s", msg)
			}
		default:
			add(i, "unknown item kind %q (step, recipe or task)", it.Kind)
		}
	}
	return msgs
}
