package progress

import (
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Observed is the current value of a numeric objective next to its target,
// what the web shows as "240 de 1 250" or "41 · 43 Magic": a skill's level,
// the units of an item held, the quest points, a boss's kill count.
type Observed struct {
	Value  int64 `json:"value"`
	Target int64 `json:"target"`
}

// ObservedValue reads the current value of a numeric objective from ps. It is
// nil for an objective without a number (quest, diary, unlock, combat
// achievement: their vars are flags, not counts) and whenever the current
// evaluation is unknown, which is never shown as missing. The stored verdict
// is latched and its evidence kept from when it changed; this is always the
// value of the latest facts.
func ObservedValue(k entities.ObjectiveKey, ps rules.PlayerState, reg *entities.Registry) *Observed {
	if k.Op != entities.OpAtLeast && k.Op != entities.OpQuantity {
		return nil
	}
	ev := EvaluateObjective(k, ps, reg)
	if ev.Verdict == rules.Unknown || len(ev.Evidence) == 0 {
		return nil
	}
	switch o := ev.Evidence[0].Observed.(type) {
	case rules.SkillState:
		if k.Ref.Kind == entities.KindSkill {
			return &Observed{Value: int64(o.Level), Target: int64(k.N)}
		}
	case rules.Ownership:
		return &Observed{Value: o.Held, Target: o.Required}
	case map[string]int:
		if kc, ok := o["kc"]; ok && k.Ref.Kind == entities.KindBoss {
			return &Observed{Value: int64(kc), Target: int64(k.N)}
		}
	case map[string]int64:
		if v, ok := o["value"]; ok && k.IsQuestPoints() {
			return &Observed{Value: v, Target: int64(k.N)}
		}
	}
	return nil
}
