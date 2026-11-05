package rules

import (
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

func hdr(t string) content.RuleHeader { return content.RuleHeader{Type: t} }

func skill(name string, lvl int) content.Rule {
	return content.SkillLevelRule{RuleHeader: hdr(content.TypeSkillLevel), Skill: name, Level: lvl}
}

func quest(name, state, op string) content.Rule {
	return content.QuestStateRule{RuleHeader: hdr(content.TypeQuestState), Quest: name, State: state, Op: op}
}

func intp(i int) *int { return &i }

func TestEvaluate(t *testing.T) {
	full := PlayerState{
		Skills:  map[string]SkillState{"agility": {Level: 56, XP: 200_000}, "firemaking": {Level: 50, XP: 101_333}},
		Quests:  map[string]string{"Sea Slug": "IN_PROGRESS", "Cook's Assistant": "FINISHED", "Dragon Slayer I": "NOT_STARTED"},
		Diaries: map[string]map[string]bool{"ardougne": {"easy": true, "medium": false}, "varrock": {"easy": true}},
		BossKC:  map[string]int{"Wintertodt": 120},
		Varbits: map[int]int64{4458: 1, 4459: 0, 10: 0b101},
		Varps:   map[int]int64{101: 33},
	}
	varbit := func(id int, eq, atLeast, bit *int) content.Rule {
		return content.VarbitRule{RuleHeader: hdr(content.TypeVarbit), ID: id, Eq: eq, Min: atLeast, Bit: bit}
	}
	varp := func(id int, eq, atLeast *int) content.Rule {
		return content.VarpRule{RuleHeader: hdr(content.TypeVarp), ID: id, Eq: eq, Min: atLeast}
	}
	allEasy := map[string]map[string]bool{}
	for _, a := range DiaryAreas {
		allEasy[a] = map[string]bool{"easy": true}
	}
	allButOne := map[string]map[string]bool{}
	for _, a := range DiaryAreas {
		allButOne[a] = map[string]bool{"easy": true}
	}
	allButOne["wilderness"] = map[string]bool{"easy": false}

	cases := []struct {
		name      string
		rules     []content.Rule
		ps        PlayerState
		want      Verdict
		monotonic bool
	}{
		{"skill level met", []content.Rule{skill("agility", 50)}, full, Met, true},
		{"skill level exact boundary", []content.Rule{skill("agility", 56)}, full, Met, true},
		{"skill level unmet", []content.Rule{skill("agility", 60)}, full, Unmet, true},
		{"skill facet absent -> unknown", []content.Rule{skill("agility", 10)}, PlayerState{}, Unknown, true},
		{"skill key missing -> unknown", []content.Rule{skill("slayer", 10)}, full, Unknown, true},
		{"skill xp met", []content.Rule{content.SkillXPRule{RuleHeader: hdr(content.TypeSkillXP), Skill: "firemaking", XP: 100_000}}, full, Met, true},
		{"skill xp unmet", []content.Rule{content.SkillXPRule{RuleHeader: hdr(content.TypeSkillXP), Skill: "firemaking", XP: 200_000}}, full, Unmet, true},
		{"quest finished gte", []content.Rule{quest("Cook's Assistant", "FINISHED", "")}, full, Met, true},
		{"quest in progress satisfies gte IN_PROGRESS", []content.Rule{quest("Sea Slug", "IN_PROGRESS", "gte")}, full, Met, true},
		{"quest finished satisfies gte IN_PROGRESS", []content.Rule{quest("Cook's Assistant", "IN_PROGRESS", "gte")}, full, Met, true},
		{"quest finished fails eq IN_PROGRESS", []content.Rule{quest("Cook's Assistant", "IN_PROGRESS", "eq")}, full, Unmet, true},
		{"quest in progress fails FINISHED", []content.Rule{quest("Sea Slug", "FINISHED", "")}, full, Unmet, true},
		{"quest not observed -> unknown", []content.Rule{quest("Sea Slug", "FINISHED", "")}, PlayerState{Skills: full.Skills}, Unknown, true},
		{"quest name missing -> unknown", []content.Rule{quest("Monkey Madness I", "FINISHED", "")}, full, Unknown, true},
		{"diary tier met", []content.Rule{content.DiaryTierRule{RuleHeader: hdr(content.TypeDiaryTier), Area: "ardougne", Tier: "easy"}}, full, Met, true},
		{"diary tier unmet", []content.Rule{content.DiaryTierRule{RuleHeader: hdr(content.TypeDiaryTier), Area: "ardougne", Tier: "medium"}}, full, Unmet, true},
		{"diary tier not observed", []content.Rule{content.DiaryTierRule{RuleHeader: hdr(content.TypeDiaryTier), Area: "ardougne", Tier: "hard"}}, full, Unknown, true},
		{"diary all areas met", []content.Rule{content.DiaryTierRule{RuleHeader: hdr(content.TypeDiaryTier), Area: "all", Tier: "easy"}}, PlayerState{Diaries: allEasy}, Met, true},
		{"diary all areas one false -> unmet", []content.Rule{content.DiaryTierRule{RuleHeader: hdr(content.TypeDiaryTier), Area: "all", Tier: "easy"}}, PlayerState{Diaries: allButOne}, Unmet, true},
		{"diary all areas partial -> unknown", []content.Rule{content.DiaryTierRule{RuleHeader: hdr(content.TypeDiaryTier), Area: "all", Tier: "easy"}}, full, Unknown, true},
		{"boss kc met", []content.Rule{content.BossKCRule{RuleHeader: hdr(content.TypeBossKC), Boss: "Wintertodt", KC: 100}}, full, Met, true},
		{"boss kc unmet", []content.Rule{content.BossKCRule{RuleHeader: hdr(content.TypeBossKC), Boss: "Wintertodt", KC: 500}}, full, Unmet, true},
		{"boss kc not observed", []content.Rule{content.BossKCRule{RuleHeader: hdr(content.TypeBossKC), Boss: "Zulrah", KC: 1}}, full, Unknown, true},
		{"manual not done", []content.Rule{content.ManualRule{RuleHeader: hdr(content.TypeManual)}}, full, Unmet, true},
		{"manual done", []content.Rule{content.ManualRule{RuleHeader: hdr(content.TypeManual)}}, PlayerState{ManualDone: true}, Met, true},
		{"empty list is manual", nil, full, Unmet, true},
		{"empty list manual done", nil, PlayerState{ManualDone: true}, Met, true},
		{"implicit AND: met + unmet", []content.Rule{skill("agility", 10), skill("agility", 99)}, full, Unmet, true},
		{"implicit AND: met + unknown", []content.Rule{skill("agility", 10), quest("Zogre Flesh Eaters", "FINISHED", "")}, full, Unknown, true},
		{"implicit AND: unmet beats unknown", []content.Rule{skill("agility", 99), quest("Zogre Flesh Eaters", "FINISHED", "")}, full, Unmet, true},
		{"any: one met", []content.Rule{content.AnyRule{Any: content.RuleList{skill("agility", 99), skill("firemaking", 50)}}}, full, Met, true},
		{"any: met beats unknown", []content.Rule{content.AnyRule{Any: content.RuleList{quest("Zogre Flesh Eaters", "FINISHED", ""), skill("firemaking", 50)}}}, full, Met, true},
		{"any: unmet + unknown -> unknown", []content.Rule{content.AnyRule{Any: content.RuleList{quest("Zogre Flesh Eaters", "FINISHED", ""), skill("firemaking", 99)}}}, full, Unknown, true},
		{"any: all unmet", []content.Rule{content.AnyRule{Any: content.RuleList{skill("agility", 99), skill("firemaking", 99)}}}, full, Unmet, true},
		{"all nested", []content.Rule{content.AllRule{All: content.RuleList{skill("agility", 10), content.AnyRule{Any: content.RuleList{skill("firemaking", 99), quest("Sea Slug", "IN_PROGRESS", "")}}}}}, full, Met, true},
		{"not met -> unmet, non monotonic", []content.Rule{content.NotRule{Not: skill("agility", 10)}}, full, Unmet, false},
		{"not unknown stays unknown", []content.Rule{content.NotRule{Not: skill("slayer", 10)}}, full, Unknown, false},
		{"unsupported type -> unknown", []content.Rule{content.CombatAchievementRule{RuleHeader: hdr(content.TypeCombatAchievement), Name: "x"}}, full, Unknown, true},
		{"item_owned unknown and non monotonic", []content.Rule{content.ItemOwnedRule{RuleHeader: hdr(content.TypeItemOwned), ItemName: "Spade"}}, full, Unknown, false},
		{"varbit with min is monotonic", []content.Rule{varbit(1, nil, intp(1), nil)}, full, Unknown, true},
		{"varbit with eq is not monotonic", []content.Rule{varbit(1, intp(1), nil, nil)}, full, Unknown, false},
		{"varbit eq met", []content.Rule{varbit(4458, intp(1), nil, nil)}, full, Met, false},
		{"varbit eq unmet", []content.Rule{varbit(4459, intp(1), nil, nil)}, full, Unmet, false},
		{"varbit bit set", []content.Rule{varbit(10, nil, nil, intp(2))}, full, Met, false},
		{"varbit bit clear", []content.Rule{varbit(10, nil, nil, intp(1))}, full, Unmet, false},
		{"varbit facet absent -> unknown", []content.Rule{varbit(4458, intp(1), nil, nil)}, PlayerState{}, Unknown, false},
		{"varp min met (quest points)", []content.Rule{varp(101, nil, intp(32))}, full, Met, true},
		{"varp min boundary", []content.Rule{varp(101, nil, intp(33))}, full, Met, true},
		{"varp min unmet", []content.Rule{varp(101, nil, intp(34))}, full, Unmet, true},
		{"varp key missing -> unknown", []content.Rule{varp(102, nil, intp(1))}, full, Unknown, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Evaluate(tc.rules, tc.ps)
			if res.Verdict != tc.want {
				t.Errorf("verdict = %s, want %s (evidence %+v)", res.Verdict, tc.want, res.Evidence)
			}
			if res.Monotonic != tc.monotonic {
				t.Errorf("monotonic = %v, want %v", res.Monotonic, tc.monotonic)
			}
			if len(res.Evidence) == 0 {
				t.Error("no evidence returned")
			}
		})
	}
}

func TestEvidenceCarriesObservation(t *testing.T) {
	ps := PlayerState{Skills: map[string]SkillState{"agility": {Level: 56, XP: 1}}}
	res := Evaluate([]content.Rule{skill("agility", 60)}, ps)
	ev := res.Evidence[0]
	if ev.Verdict != Unmet {
		t.Fatalf("verdict %s", ev.Verdict)
	}
	if obs, ok := ev.Observed.(SkillState); !ok || obs.Level != 56 {
		t.Errorf("observed = %#v", ev.Observed)
	}
	if ev.Rule.RuleType() != content.TypeSkillLevel {
		t.Errorf("rule = %v", ev.Rule)
	}
}
