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
	// OpQuantity: at least N of an item ("x120"): an exact id, any variant of
	// a family, or (with N zero) any one of several alternatives.
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
//	qp>=107                           item:family/graceful_hoodx1
//	item:any(1355x2|1357x1|1359x1)
//
// Ref is zero for quest point objectives. Item quantities are always
// explicit (item:952x1, never item:952). Besides an exact id, an item key
// names a family (item:family/<slug>xN: every variant counts, summed) or
// alternatives (item:any(<id>x<n>|...): one alternative must reach its own
// quantity alone). Their Ref ids ("family/<slug>", "any(...)") are not item
// ids, so Ref.Int fails and Ref.Validate rejects them: only the key carries
// them. Alternatives have no outer quantity (N is 0) and one canonical
// spelling: ids ascending without repeats, at least two, every quantity
// explicit and >= 1. The key text is the objective_progress primary key, so
// any other spelling is rejected rather than normalized.
type ObjectiveKey struct {
	Ref   Ref
	Op    TargetOp
	State string // OpState only
	N     int    // OpAtLeast and OpQuantity only; 0 for item alternatives
}

// ItemQty is one alternative of an item:any(...) key: N of exact item ID.
type ItemQty struct{ ID, N int }

// Ref id prefixes of the non-exact item keys.
const (
	itemFamilyPrefix = "family/"
	itemAnyPrefix    = "any("
)

// ItemFamilyKey returns the key "item:family/<slug>x<n>".
func ItemFamilyKey(slug string, n int) (ObjectiveKey, error) {
	k := ObjectiveKey{Ref: Ref{Kind: KindItem, ID: itemFamilyPrefix + slug}, Op: OpQuantity, N: n}
	return k, k.Validate()
}

// ItemAlternativesKey returns the key "item:any(...)" of alts in canonical
// order (ids ascending). Repeated ids and fewer than two alternatives are
// errors, not merged.
func ItemAlternativesKey(alts []ItemQty) (ObjectiveKey, error) {
	sorted := slices.Clone(alts)
	slices.SortFunc(sorted, func(a, b ItemQty) int { return a.ID - b.ID })
	k := ObjectiveKey{Ref: Ref{Kind: KindItem, ID: renderAlternatives(sorted)}, Op: OpQuantity}
	return k, k.Validate()
}

func renderAlternatives(alts []ItemQty) string {
	parts := make([]string, len(alts))
	for i, a := range alts {
		parts[i] = strconv.Itoa(a.ID) + string(OpQuantity) + strconv.Itoa(a.N)
	}
	return itemAnyPrefix + strings.Join(parts, "|") + ")"
}

// parseAlternatives reads the Ref id "any(<id>x<n>|...)" and accepts only
// the canonical spelling.
func parseAlternatives(id string) ([]ItemQty, error) {
	inner, ok := strings.CutPrefix(id, itemAnyPrefix)
	if !ok {
		return nil, fmt.Errorf("not an item alternatives id")
	}
	inner, ok = strings.CutSuffix(inner, ")")
	if !ok {
		return nil, fmt.Errorf("item alternatives need a closing parenthesis")
	}
	parts := strings.Split(inner, "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("item alternatives need at least two items")
	}
	alts := make([]ItemQty, len(parts))
	for i, p := range parts {
		idPart, nPart, ok := strings.Cut(p, string(OpQuantity))
		if !ok {
			return nil, fmt.Errorf("alternative %q needs an explicit quantity (<id>x<n>)", p)
		}
		itemID, err := parseCount(idPart)
		if err != nil || itemID < 1 {
			return nil, fmt.Errorf("alternative %q: invalid item id", p)
		}
		n, err := parseCount(nPart)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("alternative %q: quantity must be >= 1", p)
		}
		if i > 0 && itemID <= alts[i-1].ID {
			return nil, fmt.Errorf("alternatives must list each id once, ascending (%d after %d)", itemID, alts[i-1].ID)
		}
		alts[i] = ItemQty{ID: itemID, N: n}
	}
	return alts, nil
}

// ItemFamily returns the family slug and quantity of an item:family key.
func (k ObjectiveKey) ItemFamily() (slug string, n int, ok bool) {
	if k.Ref.Kind != KindItem {
		return "", 0, false
	}
	slug, ok = strings.CutPrefix(k.Ref.ID, itemFamilyPrefix)
	if !ok {
		return "", 0, false
	}
	return slug, k.N, true
}

// ItemAlternatives returns the alternatives of an item:any key, ids
// ascending.
func (k ObjectiveKey) ItemAlternatives() ([]ItemQty, bool) {
	if k.Ref.Kind != KindItem {
		return nil, false
	}
	alts, err := parseAlternatives(k.Ref.ID)
	return alts, err == nil
}

// ParseObjectiveRef parses the subject of an objective key: a ref, or the
// item family/alternatives subject that only keys carry ("item:family/x",
// "item:any(...)"), checked syntactically.
func ParseObjectiveRef(s string) (Ref, error) {
	if rest, ok := strings.CutPrefix(s, string(KindItem)+":"); ok {
		if strings.HasPrefix(rest, itemFamilyPrefix) || strings.HasPrefix(rest, itemAnyPrefix) {
			ref := Ref{Kind: KindItem, ID: rest}
			return ref, validItemSubject(ref.ID)
		}
	}
	return ParseRef(s)
}

// validItemSubject checks a non-exact item subject id.
func validItemSubject(id string) error {
	if slug, ok := strings.CutPrefix(id, itemFamilyPrefix); ok {
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid item family slug %q", slug)
		}
		return nil
	}
	_, err := parseAlternatives(id)
	return err
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
	case strings.HasPrefix(s, string(KindItem)+":"+itemAnyPrefix):
		k = ObjectiveKey{Ref: Ref{Kind: KindItem, ID: strings.TrimPrefix(s, string(KindItem)+":")}, Op: OpQuantity}
		if err := k.Validate(); err != nil {
			return ObjectiveKey{}, err
		}
		return k, nil
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
	ref, err := ParseObjectiveRef(refPart)
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
	if k.Ref.Kind == KindItem && !k.isExactItem() {
		return k.validateItemSet()
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

// isExactItem reports whether an item key names one item id (not a family
// or alternatives).
func (k ObjectiveKey) isExactItem() bool {
	return !strings.HasPrefix(k.Ref.ID, itemFamilyPrefix) && !strings.HasPrefix(k.Ref.ID, itemAnyPrefix)
}

// validateItemSet validates an item family or alternatives key.
func (k ObjectiveKey) validateItemSet() error {
	if err := validItemSubject(k.Ref.ID); err != nil {
		return fmt.Errorf("objective %q: %w", k.String(), err)
	}
	if k.Op != OpQuantity || k.State != "" {
		return fmt.Errorf("objective %q: items take x<n>", k.String())
	}
	if _, _, family := k.ItemFamily(); family {
		if k.N < 1 {
			return fmt.Errorf("objective %q: items take x<n> with n >= 1", k.String())
		}
		return nil
	}
	if k.N != 0 {
		return fmt.Errorf("objective %q: item alternatives carry their own quantities, not an outer x<n>", k.String())
	}
	return nil
}

// String renders the canonical form.
func (k ObjectiveKey) String() string {
	subject := QuestPoints
	if !k.IsQuestPoints() {
		subject = k.Ref.String()
	}
	if strings.HasPrefix(k.Ref.ID, itemAnyPrefix) && k.Ref.Kind == KindItem && k.N == 0 {
		return subject
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
	if slug, _, ok := k.ItemFamily(); ok {
		if _, known := r.Family(slug); !known {
			return fmt.Sprintf("unknown item family %s", slug)
		}
		return ""
	}
	if alts, ok := k.ItemAlternatives(); ok {
		for _, a := range alts {
			if ref := ItemRef(a.ID); !r.Known(ref) {
				return fmt.Sprintf("unknown entity %s", ref)
			}
		}
		return ""
	}
	if k.IsQuestPoints() || !r.HasCatalog(k.Ref.Kind) || r.Known(k.Ref) {
		return ""
	}
	return fmt.Sprintf("unknown entity %s", k.Ref)
}
