// Package rules is the pure rule engine: it evaluates a step's completion
// rules against a PlayerState and returns a three-valued verdict with
// evidence. It performs no IO and does not read the clock.
package rules

import (
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// Verdict is the outcome of evaluating a rule.
type Verdict string

// Verdicts. Unknown means the facts the rule needs were never observed for
// this account (e.g. quest states without a plugin/WikiSync), as opposed to
// observed and not satisfied.
const (
	Met     Verdict = "met"
	Unmet   Verdict = "unmet"
	Unknown Verdict = "unknown"
)

// SkillState is the observed level and experience of one skill.
type SkillState struct {
	Level int   `json:"level"`
	XP    int64 `json:"xp"`
}

// PlayerState is the merged, already-observed state of one account. A nil
// map means the facet was never observed; a missing key inside a non-nil
// map is also treated as not observed (hiscores and plugins report whole
// facets at once).
type PlayerState struct {
	// Skills by canonical skill name ("attack", "runecraft", ...).
	Skills map[string]SkillState
	// Quests by canonical wiki name → NOT_STARTED | IN_PROGRESS | FINISHED.
	Quests map[string]string
	// Diaries by area ("ardougne") → tier ("easy") → complete.
	Diaries map[string]map[string]bool
	// BossKC by hiscores boss name → kill count.
	BossKC map[string]int
	// Varbits and Varps by id → value, as the plugin read them (the ids the
	// ingest manifest asks for).
	Varbits map[int]int64
	Varps   map[int]int64
	// Items by container; nil means no item container was ever observed.
	Items ItemHoldings
	// Catalog relates noted/unnoted item ids; nil compares ids exactly.
	Catalog *entities.Registry

	// ManualDone is per-step context supplied by the caller: whether the user
	// ticked the step being evaluated. Manual rules are met only when true.
	ManualDone bool
}

// Evidence explains one atomic verdict.
type Evidence struct {
	Rule     content.Rule `json:"rule"`
	Verdict  Verdict      `json:"verdict"`
	Observed any          `json:"observed,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	// Stale marks a met verdict that relies on a restored (not live) mirror.
	Stale bool `json:"stale,omitempty"`
}

// Result is the evaluation of a rule list.
type Result struct {
	Verdict  Verdict    `json:"verdict"`
	Evidence []Evidence `json:"evidence"`
	// Monotonic is true when no rule in the list can regress once met
	// (skills, quests, diaries, kill counts). item_owned and varbit/varp with
	// eq/bit are not monotonic.
	Monotonic bool `json:"monotonic"`
}

// DiaryAreas is the canonical list used to expand area "all".
var DiaryAreas = []string{
	"ardougne", "desert", "falador", "fremennik", "kandarin", "karamja",
	"kourend", "lumbridge", "morytania", "varrock", "western", "wilderness",
}

// Evaluate evaluates rules with AND semantics (the implicit AND of
// complete_when). An empty list is a manual step: met only when ps.ManualDone.
func Evaluate(rules []content.Rule, ps PlayerState) Result {
	if len(rules) == 0 {
		v := manualVerdict(ps)
		return Result{
			Verdict:   v,
			Evidence:  []Evidence{{Rule: content.ManualRule{RuleHeader: content.RuleHeader{Type: content.TypeManual}}, Verdict: v, Reason: "manual step"}},
			Monotonic: true,
		}
	}
	res := Result{Monotonic: true}
	verdicts := make([]Verdict, 0, len(rules))
	for _, r := range rules {
		v, ev := evaluate(r, ps)
		verdicts = append(verdicts, v)
		res.Evidence = append(res.Evidence, ev...)
		if !monotonic(r) {
			res.Monotonic = false
		}
	}
	res.Verdict = combineAll(verdicts)
	return res
}

// EvaluateRule evaluates a single rule (which may be a combinator).
func EvaluateRule(r content.Rule, ps PlayerState) (Verdict, []Evidence) {
	return evaluate(r, ps)
}

func evaluate(r content.Rule, ps PlayerState) (Verdict, []Evidence) {
	switch v := r.(type) {
	case content.AllRule:
		var vs []Verdict
		var evs []Evidence
		for _, c := range v.All {
			cv, cev := evaluate(c, ps)
			vs = append(vs, cv)
			evs = append(evs, cev...)
		}
		return combineAll(vs), evs
	case content.AnyRule:
		var vs []Verdict
		var evs []Evidence
		for _, c := range v.Any {
			cv, cev := evaluate(c, ps)
			vs = append(vs, cv)
			evs = append(evs, cev...)
		}
		return combineAny(vs), evs
	case content.NotRule:
		cv, evs := evaluate(v.Not, ps)
		return negate(cv), evs
	}
	ev := atomic(r, ps)
	return ev.Verdict, []Evidence{ev}
}

func atomic(r content.Rule, ps PlayerState) Evidence {
	ev := Evidence{Rule: r, Verdict: Unknown}
	switch v := r.(type) {
	case content.SkillLevelRule:
		s, ok := ps.Skills[v.Skill]
		if !ok {
			ev.Reason = "skill not observed"
			return ev
		}
		ev.Observed = s
		ev.Verdict = boolVerdict(s.Level >= v.Level)
	case content.SkillXPRule:
		s, ok := ps.Skills[v.Skill]
		if !ok {
			ev.Reason = "skill not observed"
			return ev
		}
		ev.Observed = s
		ev.Verdict = boolVerdict(s.XP >= v.XP)
	case content.QuestStateRule:
		state, ok := ps.Quests[v.Quest]
		if !ok {
			ev.Reason = "quest not observed"
			return ev
		}
		have, okHave := questRank(state)
		want, okWant := questRank(v.State)
		if !okHave || !okWant {
			ev.Reason = "unknown quest state value"
			return ev
		}
		ev.Observed = map[string]string{"state": state}
		if v.Op == "eq" {
			ev.Verdict = boolVerdict(have == want)
		} else {
			ev.Verdict = boolVerdict(have >= want)
		}
	case content.DiaryTierRule:
		if v.Area == "all" {
			vs := make([]Verdict, 0, len(DiaryAreas))
			obs := map[string]bool{}
			for _, area := range DiaryAreas {
				done, ok := diaryTier(ps, area, v.Tier)
				if !ok {
					vs = append(vs, Unknown)
					continue
				}
				obs[area] = done
				vs = append(vs, boolVerdict(done))
			}
			ev.Observed = obs
			ev.Verdict = combineAll(vs)
			if ev.Verdict == Unknown {
				ev.Reason = "some diary areas not observed"
			}
			return ev
		}
		done, ok := diaryTier(ps, v.Area, v.Tier)
		if !ok {
			ev.Reason = "diary not observed"
			return ev
		}
		ev.Observed = map[string]bool{"complete": done}
		ev.Verdict = boolVerdict(done)
	case content.BossKCRule:
		kc, ok := ps.BossKC[v.Boss]
		if !ok {
			ev.Reason = "boss kc not observed"
			return ev
		}
		ev.Observed = map[string]int{"kc": kc}
		ev.Verdict = boolVerdict(kc >= v.KC)
	case content.VarbitRule:
		varCheck(ps.Varbits, v.ID, v.Eq, v.Min, v.Bit, "varbit", &ev)
	case content.VarpRule:
		varCheck(ps.Varps, v.ID, v.Eq, v.Min, v.Bit, "varp", &ev)
	case content.ItemOwnedRule:
		itemOwned(v, ps, &ev)
	case content.ManualRule:
		ev.Verdict = manualVerdict(ps)
		ev.Reason = "manual rule"
	default:
		ev.Reason = "rule type " + r.RuleType() + " not evaluable yet"
	}
	return ev
}

// varCheck decides a varbit/varp rule: the value equals eq, is at least atLeast,
// or has bit set (exactly one is given by the schema).
func varCheck(values map[int]int64, id int, eq, atLeast, bit *int, kind string, ev *Evidence) {
	value, ok := values[id]
	if !ok {
		ev.Reason = kind + " not observed"
		return
	}
	ev.Observed = map[string]int64{"value": value}
	switch {
	case eq != nil:
		ev.Verdict = boolVerdict(value == int64(*eq))
	case atLeast != nil:
		ev.Verdict = boolVerdict(value >= int64(*atLeast))
	case bit != nil && *bit >= 0 && *bit < 63:
		ev.Verdict = boolVerdict(value&(int64(1)<<uint(*bit)) != 0)
	default:
		ev.Observed = nil
		ev.Reason = kind + " rule has no test"
	}
}

func diaryTier(ps PlayerState, area, tier string) (bool, bool) {
	tiers, ok := ps.Diaries[area]
	if !ok {
		return false, false
	}
	done, ok := tiers[tier]
	return done, ok
}

func manualVerdict(ps PlayerState) Verdict {
	if ps.ManualDone {
		return Met
	}
	return Unmet
}

func questRank(state string) (int, bool) {
	switch state {
	case content.QuestNotStarted:
		return 0, true
	case content.QuestInProgress:
		return 1, true
	case content.QuestFinished:
		return 2, true
	}
	return 0, false
}

func boolVerdict(b bool) Verdict {
	if b {
		return Met
	}
	return Unmet
}

func negate(v Verdict) Verdict {
	switch v {
	case Met:
		return Unmet
	case Unmet:
		return Met
	}
	return Unknown
}

// combineAll: any unmet ⇒ unmet; else any unknown ⇒ unknown; else met.
func combineAll(vs []Verdict) Verdict {
	out := Met
	for _, v := range vs {
		switch v {
		case Unmet:
			return Unmet
		case Unknown:
			out = Unknown
		}
	}
	return out
}

// combineAny: any met ⇒ met; else any unknown ⇒ unknown; else unmet.
func combineAny(vs []Verdict) Verdict {
	out := Unmet
	for _, v := range vs {
		switch v {
		case Met:
			return Met
		case Unknown:
			out = Unknown
		}
	}
	return out
}

// monotonic reports whether a rule can never regress once met.
func monotonic(r content.Rule) bool {
	switch v := r.(type) {
	case content.AllRule:
		for _, c := range v.All {
			if !monotonic(c) {
				return false
			}
		}
		return true
	case content.AnyRule:
		for _, c := range v.Any {
			if !monotonic(c) {
				return false
			}
		}
		return true
	case content.NotRule:
		return false
	case content.ItemOwnedRule:
		return false
	case content.VarbitRule:
		return v.Min != nil
	case content.VarpRule:
		return v.Min != nil
	}
	return true
}
