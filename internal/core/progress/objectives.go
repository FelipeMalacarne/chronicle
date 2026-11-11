package progress

import (
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// Fact names one player_state row (or item container) an evaluation read.
// The caller maps facts to their snapshot, source and observation time.
type Fact struct {
	Facet string `json:"facet"`
	Key   string `json:"key"`
}

// FactItemContainer is the pseudo-facet of an item container: an item
// objective reads whole containers, whose provenance is the container's
// latest accepted observation (ingest_container_freshness).
const FactItemContainer = "item_container"

// Evaluation is the current verdict of one objective against a PlayerState.
type Evaluation struct {
	Verdict  rules.Verdict    `json:"verdict"`
	Evidence []rules.Evidence `json:"evidence"`
	Facts    []Fact           `json:"facts,omitempty"`
}

// EvaluateObjective maps an objective key to the rule that proves it (docs/09
// section 3.2) and evaluates it. Targets without a fact source (unlocks
// without a proof var, combat achievements) are unknown, never guessed.
func EvaluateObjective(k entities.ObjectiveKey, ps rules.PlayerState, reg *entities.Registry) Evaluation {
	if k.IsQuestPoints() {
		return questPoints(k.N, ps, reg)
	}
	switch k.Ref.Kind {
	case entities.KindQuest:
		name := k.Ref.ID
		if q, ok := lookupQuest(reg, k.Ref.ID); ok {
			name = q
		}
		state := content.QuestFinished
		if k.State == entities.StateStarted {
			state = content.QuestInProgress
		}
		r := content.QuestStateRule{RuleHeader: header(content.TypeQuestState), Quest: name, State: state}
		return evaluate(r, ps, Fact{Facet: "quest", Key: name})
	case entities.KindSkill:
		r := content.SkillLevelRule{RuleHeader: header(content.TypeSkillLevel), Skill: k.Ref.ID, Level: k.N}
		return evaluate(r, ps, Fact{Facet: "skill", Key: k.Ref.ID})
	case entities.KindItem:
		return evaluateItem(k, ps, reg)
	case entities.KindDiary:
		area, tier, ok := strings.Cut(k.Ref.ID, ":")
		if !ok {
			return unknown("invalid diary ref")
		}
		r := content.DiaryTierRule{RuleHeader: header(content.TypeDiaryTier), Area: area, Tier: tier}
		return evaluate(r, ps, Fact{Facet: "diary", Key: k.Ref.ID})
	case entities.KindBoss:
		name := k.Ref.ID
		for observed := range ps.BossKC {
			if entities.Slug(observed) == k.Ref.ID {
				name = observed
				break
			}
		}
		r := content.BossKCRule{RuleHeader: header(content.TypeBossKC), Boss: name, KC: k.N}
		return evaluate(r, ps, Fact{Facet: "activity", Key: name})
	case entities.KindUnlock:
		var proof *entities.VarTest
		if reg != nil {
			if u, ok := reg.Unlock(k.Ref.ID); ok {
				proof = u.Proof
			}
		}
		if proof == nil {
			return unknown("unlock has no proof var in the catalog; only the owner can mark it")
		}
		return evaluateVar(*proof, ps)
	case entities.KindCombatAchievement:
		var proof *entities.VarTest
		if id, ok := k.Ref.Int(); ok && reg != nil {
			if c, ok := reg.CombatAchievement(id); ok {
				proof = c.Proof
			}
		}
		if proof == nil {
			return unknown("combat achievement has no proof var in the catalog")
		}
		return evaluateVar(*proof, ps)
	}
	return unknown("objective kind " + string(k.Ref.Kind) + " has no fact source")
}

// evaluateItem evaluates an item key: an exact id through the item_owned
// rule, a family through rules.OwnsFamily and alternatives through
// rules.OwnsAlternatives, all over the default containers with the same
// freshness rules (restored never proves absence, a met verdict through it
// is stale).
func evaluateItem(k entities.ObjectiveKey, ps rules.PlayerState, reg *entities.Registry) Evaluation {
	var facts []Fact
	for _, c := range rules.DefaultItemContainers {
		if _, ok := ps.Items[c]; ok {
			facts = append(facts, Fact{Facet: FactItemContainer, Key: c})
		}
	}
	catalog := ps.Catalog
	if catalog == nil {
		catalog = reg
	}
	if slug, n, ok := k.ItemFamily(); ok {
		r := content.ItemOwnedRule{RuleHeader: header(content.TypeItemOwned), ItemName: slug, Qty: n}
		return owned(r, ps, facts, func(*content.ItemOwnedRule) rules.Ownership {
			return rules.OwnsFamily(ps.Items, catalog, slug, int64(n), nil)
		})
	}
	if alts, ok := k.ItemAlternatives(); ok {
		r := content.ItemOwnedRule{RuleHeader: header(content.TypeItemOwned)}
		return owned(r, ps, facts, func(r *content.ItemOwnedRule) rules.Ownership {
			o, id := rules.OwnsAlternatives(ps.Items, catalog, alts, nil)
			// The evidence names the alternative that decided the verdict.
			for _, a := range alts {
				if a.ID == id {
					r.ItemID, r.Qty = &id, a.N
				}
			}
			return o
		})
	}
	id, ok := k.Ref.Int()
	if !ok {
		return unknown("invalid item ref")
	}
	r := content.ItemOwnedRule{RuleHeader: header(content.TypeItemOwned), ItemID: &id, Qty: k.N}
	return evaluate(r, ps, facts...)
}

// owned builds the evidence of a non-exact item key the way the item_owned
// rule does (verdict, stale, observed ownership, reason). own may refine r,
// the rule the evidence reports.
func owned(r content.ItemOwnedRule, ps rules.PlayerState, facts []Fact, own func(*content.ItemOwnedRule) rules.Ownership) Evaluation {
	ev := rules.Evidence{Verdict: rules.Unknown}
	if ps.Items == nil {
		ev.Reason = "items not observed"
	} else {
		o := own(&r)
		ev.Verdict, ev.Stale, ev.Observed, ev.Reason = o.Verdict, o.Stale, o, o.Reason
	}
	ev.Rule = r
	out := Evaluation{Verdict: ev.Verdict, Evidence: []rules.Evidence{ev}}
	if ev.Verdict != rules.Unknown {
		out.Facts = facts
	}
	return out
}

// questPoints evaluates qp>=n against the quest points count var of the
// catalog (varp 101, QUEST_POINTS, in content/reference/varbits.json).
func questPoints(n int, ps rules.PlayerState, reg *entities.Registry) Evaluation {
	if reg != nil {
		for _, v := range reg.VarsFor(entities.QuestPoints) {
			if v.Semantics == entities.VarCount {
				return evaluateVar(entities.VarTest{Var: v.Var, ID: v.ID, Min: &n}, ps)
			}
		}
	}
	return unknown("no quest points var in the catalog")
}

// evaluateVar evaluates a varbit/varp test as the matching engine rule; the
// fact is the var row of player_state.
func evaluateVar(t entities.VarTest, ps rules.PlayerState) Evaluation {
	fact := Fact{Facet: string(t.Var), Key: strconv.Itoa(t.ID)}
	if t.Var == entities.Varp {
		return evaluate(content.VarpRule{RuleHeader: header(content.TypeVarp), ID: t.ID, Eq: t.Eq, Min: t.Min, Bit: t.Bit}, ps, fact)
	}
	return evaluate(content.VarbitRule{RuleHeader: header(content.TypeVarbit), ID: t.ID, Eq: t.Eq, Min: t.Min, Bit: t.Bit}, ps, fact)
}

func lookupQuest(reg *entities.Registry, id string) (string, bool) {
	if reg == nil {
		return "", false
	}
	q, ok := reg.Quest(id)
	if !ok {
		return "", false
	}
	return q.Name, true
}

func header(t string) content.RuleHeader { return content.RuleHeader{Type: t} }

func evaluate(r content.Rule, ps rules.PlayerState, facts ...Fact) Evaluation {
	v, evs := rules.EvaluateRule(r, ps)
	out := Evaluation{Verdict: v, Evidence: evs}
	if v != rules.Unknown {
		out.Facts = facts
	}
	return out
}

func unknown(reason string) Evaluation {
	return Evaluation{Verdict: rules.Unknown, Evidence: []rules.Evidence{{Verdict: rules.Unknown, Reason: reason}}}
}
