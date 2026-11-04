package content

import (
	"encoding/json"
	"fmt"
)

// Rule is a completion rule: an atomic check or one of the combinators
// all/any/not. It is a closed tagged union: atomic rules carry a "type"
// field, combinators are recognized by their single key. Decode JSON with
// DecodeRule or through RuleList.
type Rule interface {
	// RuleType returns the discriminator: the "type" of an atomic rule or
	// "all", "any", "not" for combinators.
	RuleType() string
}

// Rule type discriminators.
const (
	TypeAll               = "all"
	TypeAny               = "any"
	TypeNot               = "not"
	TypeSkillLevel        = "skill_level"
	TypeSkillXP           = "skill_xp"
	TypeQuestState        = "quest_state"
	TypeDiaryTask         = "diary_task"
	TypeDiaryTier         = "diary_tier"
	TypeCombatAchievement = "combat_achievement"
	TypeBossKC            = "boss_kc"
	TypeActivityScore     = "activity_score"
	TypeCollectionLogItem = "collection_log_item"
	TypeVarbit            = "varbit"
	TypeVarp              = "varp"
	TypeItemOwned         = "item_owned"
	TypeManual            = "manual"
)

// Quest states, ordered NOT_STARTED < IN_PROGRESS < FINISHED.
const (
	QuestNotStarted = "NOT_STARTED"
	QuestInProgress = "IN_PROGRESS"
	QuestFinished   = "FINISHED"
)

// RuleMeta is the provenance annotation of an atomic rule (schema $defs/ruleMeta).
type RuleMeta struct {
	Origin     string `json:"origin,omitempty"`
	Inferred   bool   `json:"inferred,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Match      string `json:"match,omitempty"`
}

// RuleHeader is embedded by every atomic rule: the discriminator plus provenance.
type RuleHeader struct {
	Type string    `json:"type"`
	Meta *RuleMeta `json:"meta,omitempty"`
}

// RuleType implements Rule.
func (h RuleHeader) RuleType() string { return h.Type }

// AllRule is met when every child is met.
type AllRule struct {
	All RuleList `json:"all"`
}

// AnyRule is met when at least one child is met.
type AnyRule struct {
	Any RuleList `json:"any"`
}

// NotRule inverts its child.
type NotRule struct {
	Not Rule `json:"not"`
}

// RuleType implements Rule.
func (AllRule) RuleType() string { return TypeAll }

// RuleType implements Rule.
func (AnyRule) RuleType() string { return TypeAny }

// RuleType implements Rule.
func (NotRule) RuleType() string { return TypeNot }

// UnmarshalJSON decodes the nested rule through DecodeRule.
func (n *NotRule) UnmarshalJSON(data []byte) error {
	var raw struct {
		Not json.RawMessage `json:"not"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Not) == 0 {
		return fmt.Errorf("rule: \"not\" is required")
	}
	r, err := DecodeRule(raw.Not)
	if err != nil {
		return err
	}
	n.Not = r
	return nil
}

// SkillLevelRule: the skill's level is at least Level.
type SkillLevelRule struct {
	RuleHeader
	Skill   string `json:"skill"`
	Level   int    `json:"level"`
	Boosted bool   `json:"boosted,omitempty"`
}

// SkillXPRule: the skill's experience is at least XP.
type SkillXPRule struct {
	RuleHeader
	Skill string `json:"skill"`
	XP    int64  `json:"xp"`
}

// QuestStateRule: the quest is in State; Op "gte" (default) also accepts
// later states, "eq" requires exactly State.
type QuestStateRule struct {
	RuleHeader
	Quest string `json:"quest"`
	State string `json:"state"`
	Op    string `json:"op,omitempty"`
}

// DiaryTaskRule: one achievement diary task is complete. Task is an index
// (number) or the task text (string).
type DiaryTaskRule struct {
	RuleHeader
	Area string `json:"area"`
	Tier string `json:"tier"`
	Task any    `json:"task"`
}

// DiaryTierRule: a whole diary tier is complete. Area "all" means every area.
type DiaryTierRule struct {
	RuleHeader
	Area string `json:"area"`
	Tier string `json:"tier"`
}

// CombatAchievementRule: a named combat achievement, or a tier (optionally a
// minimum count within it), is complete.
type CombatAchievementRule struct {
	RuleHeader
	Name     string `json:"name,omitempty"`
	Tier     string `json:"tier,omitempty"`
	MinCount int    `json:"min_count,omitempty"`
}

// BossKCRule: hiscores kill count for Boss is at least KC.
type BossKCRule struct {
	RuleHeader
	Boss string `json:"boss"`
	KC   int    `json:"kc"`
}

// ActivityScoreRule: a hiscores activity score is at least Score.
type ActivityScoreRule struct {
	RuleHeader
	Activity string `json:"activity"`
	Score    int    `json:"score"`
}

// CollectionLogItemRule: the collection log holds Qty (default 1) of the item.
type CollectionLogItemRule struct {
	RuleHeader
	ItemID   *int   `json:"item_id,omitempty"`
	ItemName string `json:"item_name,omitempty"`
	Qty      int    `json:"qty,omitempty"`
}

// VarbitRule: a varbit equals Eq, is at least Min, or has bit Bit set.
type VarbitRule struct {
	RuleHeader
	ID    int    `json:"id"`
	Eq    *int   `json:"eq,omitempty"`
	Min   *int   `json:"min,omitempty"`
	Bit   *int   `json:"bit,omitempty"`
	Label string `json:"label,omitempty"`
}

// VarpRule: like VarbitRule for a varp.
type VarpRule struct {
	RuleHeader
	ID    int    `json:"id"`
	Eq    *int   `json:"eq,omitempty"`
	Min   *int   `json:"min,omitempty"`
	Bit   *int   `json:"bit,omitempty"`
	Label string `json:"label,omitempty"`
}

// ItemOwnedRule: the player owns Qty (default 1) of the item in Containers.
type ItemOwnedRule struct {
	RuleHeader
	ItemID     *int     `json:"item_id,omitempty"`
	ItemName   string   `json:"item_name,omitempty"`
	Qty        int      `json:"qty,omitempty"`
	Containers []string `json:"containers,omitempty"`
}

// ManualRule is only ever satisfied by the user ticking the checkbox.
type ManualRule struct {
	RuleHeader
	Label string `json:"label,omitempty"`
}

// RuleList is a JSON array of rules (AND implied where it is used as complete_when).
type RuleList []Rule

// UnmarshalJSON decodes each element through DecodeRule.
func (l *RuleList) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	out := make(RuleList, 0, len(raws))
	for i, raw := range raws {
		r, err := DecodeRule(raw)
		if err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
		out = append(out, r)
	}
	*l = out
	return nil
}

// DecodeRule decodes one rule object, choosing the concrete type from the
// "all"/"any"/"not" key or the "type" discriminator.
func DecodeRule(data []byte) (Rule, error) {
	var probe struct {
		Type string          `json:"type"`
		All  json.RawMessage `json:"all"`
		Any  json.RawMessage `json:"any"`
		Not  json.RawMessage `json:"not"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}
	switch {
	case probe.All != nil:
		return decodeAs[AllRule](data)
	case probe.Any != nil:
		return decodeAs[AnyRule](data)
	case probe.Not != nil:
		return decodeAs[NotRule](data)
	}
	switch probe.Type {
	case TypeSkillLevel:
		return decodeAs[SkillLevelRule](data)
	case TypeSkillXP:
		return decodeAs[SkillXPRule](data)
	case TypeQuestState:
		return decodeAs[QuestStateRule](data)
	case TypeDiaryTask:
		return decodeAs[DiaryTaskRule](data)
	case TypeDiaryTier:
		return decodeAs[DiaryTierRule](data)
	case TypeCombatAchievement:
		return decodeAs[CombatAchievementRule](data)
	case TypeBossKC:
		return decodeAs[BossKCRule](data)
	case TypeActivityScore:
		return decodeAs[ActivityScoreRule](data)
	case TypeCollectionLogItem:
		return decodeAs[CollectionLogItemRule](data)
	case TypeVarbit:
		return decodeAs[VarbitRule](data)
	case TypeVarp:
		return decodeAs[VarpRule](data)
	case TypeItemOwned:
		return decodeAs[ItemOwnedRule](data)
	case TypeManual:
		return decodeAs[ManualRule](data)
	case "":
		return nil, fmt.Errorf("rule: missing \"type\" (and none of all/any/not)")
	}
	return nil, fmt.Errorf("rule: unknown type %q", probe.Type)
}

// decodeAs unmarshals data into a concrete rule type.
func decodeAs[T Rule](data []byte) (Rule, error) {
	var r T
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return r, nil
}
