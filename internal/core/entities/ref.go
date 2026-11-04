package entities

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind is the type of a canonical domain entity.
type Kind string

// Entity kinds. The string values are the ref prefixes; "ca" and "diary"
// match the prefixes already used by graph refs in guide content.
const (
	KindQuest             Kind = "quest"
	KindItem              Kind = "item"
	KindSkill             Kind = "skill"
	KindDiary             Kind = "diary"
	KindBoss              Kind = "boss"
	KindCombatAchievement Kind = "ca"
	KindGoal              Kind = "goal"
	KindStep              Kind = "step"
)

// Kinds lists every entity kind in a stable order.
var Kinds = []Kind{KindQuest, KindItem, KindSkill, KindDiary, KindBoss, KindCombatAchievement, KindGoal, KindStep}

// DiaryTiers are the achievement diary tiers in ascending order.
var DiaryTiers = []string{"easy", "medium", "hard", "elite"}

var (
	slugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
	diaryPattern  = regexp.MustCompile(`^([a-z0-9]+(?:_[a-z0-9]+)*):(easy|medium|hard|elite)$`)
	stableIDPart  = `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`
	stepIDPattern = regexp.MustCompile(`^(?:` + stableIDPart + `/)?` + stableIDPart + `$`)
)

// Ref is a typed identity of one domain entity, written "kind:id". It
// carries identity only: display names come from the Registry and source
// text/evidence travel separately (see content.EntityMention).
//
// Id formats per kind:
//
//	quest, skill, boss, goal  lower-case slug            quest:the_restless_ghost
//	item                      OSRS/RuneLite item id      item:1511 (item:1512 is the noted form)
//	diary                     <area slug>:<tier>         diary:ardougne:easy
//	ca                        combat achievement id      ca:574
//	step                      [<guide id>/]<step id>     step:1.1.75, step:oziris-main/1.1.75
type Ref struct {
	Kind Kind
	ID   string
}

// NewRef builds and validates a ref.
func NewRef(kind Kind, id string) (Ref, error) {
	r := Ref{Kind: kind, ID: id}
	return r, r.Validate()
}

// ItemRef returns the ref of an item id.
func ItemRef(id int) Ref { return Ref{Kind: KindItem, ID: strconv.Itoa(id)} }

// ParseRef parses "kind:id" and validates it.
func ParseRef(s string) (Ref, error) {
	kind, id, ok := strings.Cut(s, ":")
	if !ok {
		return Ref{}, fmt.Errorf("entity ref %q: missing kind prefix", s)
	}
	return NewRef(Kind(kind), id)
}

// MustParseRef is ParseRef for literals in tests and tables.
func MustParseRef(s string) Ref {
	r, err := ParseRef(s)
	if err != nil {
		panic(err)
	}
	return r
}

// String renders "kind:id".
func (r Ref) String() string { return string(r.Kind) + ":" + r.ID }

// IsZero reports whether the ref is unset.
func (r Ref) IsZero() bool { return r.Kind == "" && r.ID == "" }

// Validate checks the id format for the ref's kind.
func (r Ref) Validate() error {
	ok := false
	switch r.Kind {
	case KindQuest, KindSkill, KindBoss, KindGoal:
		ok = slugPattern.MatchString(r.ID)
	case KindItem, KindCombatAchievement:
		n, err := strconv.Atoi(r.ID)
		ok = err == nil && n >= 0 && strconv.Itoa(n) == r.ID && (r.Kind != KindItem || n > 0)
	case KindDiary:
		ok = diaryPattern.MatchString(r.ID)
	case KindStep:
		ok = stepIDPattern.MatchString(r.ID)
	default:
		return fmt.Errorf("entity ref %q: unknown kind %q", r.String(), r.Kind)
	}
	if !ok {
		return fmt.Errorf("entity ref %q: invalid %s id %q", r.String(), r.Kind, r.ID)
	}
	return nil
}

// Int returns the numeric id of an item or combat achievement ref.
func (r Ref) Int() (int, bool) {
	if r.Kind != KindItem && r.Kind != KindCombatAchievement {
		return 0, false
	}
	n, err := strconv.Atoi(r.ID)
	return n, err == nil
}

// MarshalText implements encoding.TextMarshaler (JSON string form).
func (r Ref) MarshalText() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return []byte(r.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler and rejects invalid refs.
func (r *Ref) UnmarshalText(b []byte) error {
	parsed, err := ParseRef(string(b))
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// DiaryRef returns the ref of an area/tier pair, e.g. ("Kourend & Kebos", "Hard").
func DiaryRef(area, tier string) (Ref, error) {
	return NewRef(KindDiary, Slug(area)+":"+strings.ToLower(strings.TrimSpace(tier)))
}
