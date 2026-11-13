package panel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/planning"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Assisted mode (docs/10 sections 10 and 13, phase 2): the panel shows the
// cursor step's typed tasks as a checklist, each with whether the server
// sees its termination met. The plugin only shows it; nothing acts.

// PositionMaxAge is how old a position may be for at_location: an older
// one says nothing about where the character is now.
const PositionMaxAge = 15 * time.Minute

// Facts is what a task's termination is evaluated on, besides the board:
// the account's mode (the recipes' variants), its merged facts and where
// the character last stood (the plugin's "Send position").
type Facts struct {
	Mode     string
	State    *rules.PlayerState
	Position *Position
}

// Position is the character's last tile and when it was observed.
type Position struct {
	X, Y, Plane int
	ObservedAt  time.Time
}

// Task is one line of the checklist: what to do, what ends it and whether
// that is met now. State is unknown when the server cannot tell (Reason
// says why), never unmet by default.
type Task struct {
	Type   string        `json:"type"`
	Label  string        `json:"label"`
	Until  string        `json:"until"`
	State  rules.Verdict `json:"state"`
	Reason string        `json:"reason,omitempty"`
	// Recipe marks a task the step inherits from the recipe of an
	// objective it completes (docs/10 section 8).
	Recipe bool `json:"recipe,omitempty"`
}

// stepTasks builds the checklist of the cursor step.
func stepTasks(c *content.Content, st *content.Step, s progress.StepState, seg *content.Segment, g *content.Guide, h rules.ItemHoldings, f Facts, reg *entities.Registry, now time.Time) []Task {
	all := c.TasksOf(st, f.Mode)
	own := len(st.Tasks)
	out := make([]Task, 0, len(all))
	for i, t := range all {
		line := Task{Type: t.Type, Label: taskLabel(t, reg), Recipe: i < len(all)-own}
		u, ok := t.Ends()
		if !ok {
			line.Until, line.State, line.Reason = "", rules.Unknown, "a tarefa não diz quando termina"
			out = append(out, line)
			continue
		}
		line.Until = untilLabel(t, u, reg)
		line.State, line.Reason = evaluateUntil(t, u, s, seg, g, h, f, reg, now)
		out = append(out, line)
	}
	return out
}

// evaluateUntil says whether a termination is met, from what the server
// knows: the board's verdicts and marks, the facts, the position and the
// item containers.
func evaluateUntil(t content.Task, u content.TaskUntil, s progress.StepState, seg *content.Segment, g *content.Guide, h rules.ItemHoldings, f Facts, reg *entities.Registry, now time.Time) (rules.Verdict, string) {
	if u.Objective != nil {
		key := u.Objective.String()
		for _, o := range s.Objectives {
			if o.Key == key {
				// the step's own objective: the board's latched verdict
				return o.Verdict, ""
			}
		}
		if f.State == nil {
			return rules.Unknown, "sem fatos da conta"
		}
		ev := progress.EvaluateObjective(*u.Objective, *f.State, reg)
		if ev.Verdict == rules.Unknown {
			return rules.Unknown, firstReason(ev.Evidence)
		}
		return ev.Verdict, ""
	}
	switch u.State {
	case content.UntilAtLocation:
		if t.Location == nil {
			return rules.Unknown, "a tarefa não tem local"
		}
		area, ok := reg.LocationArea(t.Location.ID)
		switch {
		case !ok:
			return rules.Unknown, "o local não tem área no mapa"
		case f.Position == nil:
			return rules.Unknown, "sem posição: ligue Send position"
		case now.Sub(f.Position.ObservedAt) > PositionMaxAge:
			return rules.Unknown, "posição velha"
		case area.Contains(f.Position.X, f.Position.Y, f.Position.Plane):
			return rules.Met, ""
		}
		return rules.Unmet, ""
	case content.UntilLoadoutReady:
		target := seg
		if len(t.Items) > 0 {
			target = &content.Segment{ID: "task", Loadout: &content.Loadout{Status: content.LoadoutPartial, Inventory: t.Items}}
		}
		if target == nil || g == nil {
			return rules.Unknown, "o trecho não tem setup"
		}
		ev := planning.EvaluateSegment(g, target, h, reg)
		switch ev.Status {
		case planning.Ready, planning.ListedReady:
			return rules.Met, ""
		case planning.NotReady:
			return rules.Unmet, ""
		case planning.NoLoadout:
			return rules.Unknown, "o trecho não tem setup"
		}
		return rules.Unknown, "inventário ou equipamento não compartilhados ao vivo"
	case content.UntilGoalMet:
		var vs []rules.Verdict
		for _, o := range s.Objectives {
			if o.Role == content.RoleCompletes || o.Role == content.RoleStarts {
				vs = append(vs, o.Verdict)
			}
		}
		if len(vs) == 0 {
			return rules.Unknown, "o passo não conclui objetivos"
		}
		v := rules.Met
		for _, x := range vs {
			if x == rules.Unmet {
				return rules.Unmet, ""
			}
			if x != rules.Met {
				v = rules.Unknown
			}
		}
		if v == rules.Unknown {
			return v, "objetivos do passo sem veredito"
		}
		return v, ""
	case content.UntilOwnerMark:
		if s.Mark != nil && s.Mark.Kind == progress.MarkManualDone {
			return rules.Met, ""
		}
		return rules.Unmet, ""
	}
	return rules.Unknown, "término desconhecido"
}

func firstReason(evs []rules.Evidence) string {
	for _, e := range evs {
		if e.Reason != "" {
			return e.Reason
		}
	}
	return "sem fatos para avaliar"
}

// taskLabel says what to do, in the panel's language (PT-BR, game terms in
// English): "Ir até Lumbridge Castle", "Falar com Duke Horacio".
func taskLabel(t content.Task, reg *entities.Registry) string {
	name := func(r *entities.Ref) string {
		if r == nil {
			return "?"
		}
		if d, ok := display(reg, *r); ok {
			return d
		}
		return titleCase(r.ID)
	}
	switch t.Type {
	case content.TaskTravel:
		s := "Ir até " + name(t.Location)
		if t.Via != nil {
			s += " (com " + name(t.Via) + ")"
		}
		return s
	case content.TaskBank:
		if len(t.Items) == 0 {
			return "Banco: montar o setup do trecho"
		}
		parts := make([]string, 0, len(t.Items))
		for _, it := range t.Items {
			parts = append(parts, itemText(it, reg))
		}
		return "Banco: pegar " + strings.Join(parts, ", ")
	case content.TaskBuy:
		return fmt.Sprintf("Comprar %s em %s", counted(max(t.Qty, 1), name(t.Item)), name(t.Shop))
	case content.TaskTalk:
		s := "Falar com " + name(t.NPC)
		if len(t.Options) > 0 {
			quoted := make([]string, len(t.Options))
			for i, o := range t.Options {
				quoted[i] = "“" + o + "”"
			}
			s += ": " + strings.Join(quoted, " › ")
		}
		return s
	case content.TaskInteract:
		return fmt.Sprintf("“%s” em %s", t.Action, name(t.Object))
	case content.TaskTrain:
		s := "Treinar " + name(t.Skill)
		if u, ok := t.Ends(); ok && u.Objective != nil {
			s += " até " + strconv.Itoa(u.Objective.N)
		}
		switch {
		case t.NPC != nil:
			s += " em " + name(t.NPC)
		case t.Object != nil:
			s += " em " + name(t.Object)
		}
		return s
	case content.TaskManual:
		if note := strings.TrimSpace(t.Note); note != "" {
			return note
		}
		return "À mão: siga o texto do passo"
	}
	return t.Type
}

func itemText(it content.ItemRequirement, reg *entities.Registry) string {
	n := 1
	if it.Qty != nil {
		n = *it.Qty
	}
	name := it.Name
	if it.ItemID != nil {
		if d, ok := display(reg, entities.ItemRef(*it.ItemID)); ok {
			name = d
		}
	}
	if it.Noted {
		name += " (noted)"
	}
	return counted(n, name)
}

// untilLabel says what ends the task.
func untilLabel(t content.Task, u content.TaskUntil, reg *entities.Registry) string {
	if u.Objective != nil {
		return Label(u.Objective.String(), reg)
	}
	switch u.State {
	case content.UntilAtLocation:
		return "na área do local"
	case content.UntilLoadoutReady:
		return "inventário e equipamento prontos"
	case content.UntilGoalMet:
		return "objetivos do passo"
	case content.UntilOwnerMark:
		return "sua marca no passo"
	}
	return u.State
}
