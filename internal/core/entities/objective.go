package entities

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// TargetOp is the operator of an objective target.
type TargetOp string

// Target operators (docs/09 section 3.2).
const (
	// OpState: the entity is in a named state ("@finished").
	OpState TargetOp = "@"
	// OpAtLeast: a level, kill count or quest point total is at least N (">=70").
	OpAtLeast TargetOp = ">="
	// OpQuantity: at least N of an exact item id ("x120").
	OpQuantity TargetOp = "x"
)

// Objective states.
const (
	StateStarted  = "started"
	StateFinished = "finished"
	StateComplete = "complete"
	StateDone     = "done"
)

// QuestPoints is the subject of quest point objectives ("qp>=107"). It is not
// an entity kind: quest points have no catalog entry and no ref.
const QuestPoints = "qp"

// MaxSkillLevel is the highest level a skill objective may target.
const MaxSkillLevel = 99

// statesByKind lists the states each kind accepts with OpState; kinds absent
// from atLeastKinds and quantityKinds accept only these.
var statesByKind = map[Kind][]string{
	KindQuest:             {StateStarted, StateFinished},
	KindDiary:             {StateComplete},
	KindUnlock:            {StateDone},
	KindCombatAchievement: {StateDone},
}

// ObjectiveKey is the canonical identity of a verifiable state of the world:
// a ref plus a target. The same objective in two guides is the same key, and
// the key is what objective_progress and goals store.
//
//	quest:x_marks_the_spot@finished   quest:rune_mysteries@started
//	skill:agility>=70                 diary:ardougne:easy@complete
//	item:960x120                      unlock:fairy_rings@done
//	boss:zulrah>=50                   ca:574@done
//	qp>=107
//
// Ref is zero for quest point objectives. Item quantities are always
// explicit (item:952x1, never item:952).
type ObjectiveKey struct {
	Ref   Ref
	Op    TargetOp
	State string // OpState only
	N     int    // OpAtLeast and OpQuantity only
}

// ParseObjectiveKey parses and validates the canonical form.
func ParseObjectiveKey(s string) (ObjectiveKey, error) {
	if rest, ok := strings.CutPrefix(s, QuestPoints+string(OpAtLeast)); ok {
		n, err := parseCount(rest)
		if err != nil {
			return ObjectiveKey{}, fmt.Errorf("objective %q: %w", s, err)
		}
		k := ObjectiveKey{Op: OpAtLeast, N: n}
		return k, k.Validate()
	}
	var (
		refPart string
		k       ObjectiveKey
	)
	switch {
	case strings.Contains(s, string(OpState)):
		refPart, k.State, _ = strings.Cut(s, string(OpState))
		k.Op = OpState
	case strings.Contains(s, string(OpAtLeast)):
		var rest string
		refPart, rest, _ = strings.Cut(s, string(OpAtLeast))
		k.Op = OpAtLeast
		n, err := parseCount(rest)
		if err != nil {
			return ObjectiveKey{}, fmt.Errorf("objective %q: %w", s, err)
		}
		k.N = n
	case strings.HasPrefix(s, string(KindItem)+":"):
		i := strings.LastIndex(s, string(OpQuantity))
		if i < 0 {
			return ObjectiveKey{}, fmt.Errorf("objective %q: item objectives need an explicit quantity (item:<id>x<n>)", s)
		}
		refPart = s[:i]
		k.Op = OpQuantity
		n, err := parseCount(s[i+1:])
		if err != nil {
			return ObjectiveKey{}, fmt.Errorf("objective %q: %w", s, err)
		}
		k.N = n
	default:
		return ObjectiveKey{}, fmt.Errorf("objective %q: missing target (@state, >=n or x<n>)", s)
	}
	ref, err := ParseRef(refPart)
	if err != nil {
		return ObjectiveKey{}, fmt.Errorf("objective %q: %w", s, err)
	}
	k.Ref = ref
	return k, k.Validate()
}

// MustParseObjectiveKey is ParseObjectiveKey for literals in tests and tables.
func MustParseObjectiveKey(s string) ObjectiveKey {
	k, err := ParseObjectiveKey(s)
	if err != nil {
		panic(err)
	}
	return k
}

func parseCount(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || strconv.Itoa(n) != s {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return n, nil
}

// IsQuestPoints reports whether the key is a quest point total.
func (k ObjectiveKey) IsQuestPoints() bool { return k.Ref.IsZero() }

// Validate checks that the target fits the ref's kind.
func (k ObjectiveKey) Validate() error {
	if k.IsQuestPoints() {
		if k.Op != OpAtLeast || k.N < 1 {
			return fmt.Errorf("objective %q: quest points take >=n with n >= 1", k.String())
		}
		return nil
	}
	if err := k.Ref.Validate(); err != nil {
		return fmt.Errorf("objective: %w", err)
	}
	switch k.Ref.Kind {
	case KindSkill:
		if k.Op != OpAtLeast || k.N < 1 || k.N > MaxSkillLevel {
			return fmt.Errorf("objective %q: skills take >=n with 1 <= n <= %d", k.String(), MaxSkillLevel)
		}
	case KindBoss:
		if k.Op != OpAtLeast || k.N < 1 {
			return fmt.Errorf("objective %q: bosses take >=n with n >= 1", k.String())
		}
	case KindItem:
		if k.Op != OpQuantity || k.N < 1 {
			return fmt.Errorf("objective %q: items take x<n> with n >= 1", k.String())
		}
	default:
		states, ok := statesByKind[k.Ref.Kind]
		if !ok {
			return fmt.Errorf("objective %q: %s refs cannot be objectives", k.String(), k.Ref.Kind)
		}
		if k.Op != OpState || !slices.Contains(states, k.State) {
			return fmt.Errorf("objective %q: %s takes @%s", k.String(), k.Ref.Kind, strings.Join(states, " or @"))
		}
	}
	return nil
}

// String renders the canonical form.
func (k ObjectiveKey) String() string {
	subject := QuestPoints
	if !k.IsQuestPoints() {
		subject = k.Ref.String()
	}
	if k.Op == OpState {
		return subject + string(OpState) + k.State
	}
	return subject + string(k.Op) + strconv.Itoa(k.N)
}

// IsZero reports whether the key is unset.
func (k ObjectiveKey) IsZero() bool { return k == ObjectiveKey{} }

// MarshalText implements encoding.TextMarshaler (JSON string form).
func (k ObjectiveKey) MarshalText() ([]byte, error) {
	if err := k.Validate(); err != nil {
		return nil, err
	}
	return []byte(k.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler and rejects invalid keys.
func (k *ObjectiveKey) UnmarshalText(b []byte) error {
	parsed, err := ParseObjectiveKey(string(b))
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}

// KnownObjective reports why a key does not name a catalog entry, or "" when
// it does. Refs of kinds without a catalog (boss) are checked syntactically.
func (r *Registry) KnownObjective(k ObjectiveKey) string {
	if err := k.Validate(); err != nil {
		return err.Error()
	}
	if k.IsQuestPoints() || !r.HasCatalog(k.Ref.Kind) || r.Known(k.Ref) {
		return ""
	}
	return fmt.Sprintf("unknown entity %s", k.Ref)
}
