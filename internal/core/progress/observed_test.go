package progress

import (
	"reflect"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func TestObservedValue(t *testing.T) {
	reg := registry(t)
	restored := rules.ItemHoldings{rules.ContainerBank: {State: ingest.ItemRestored, ObservedAt: t0, Complete: true, Quantities: map[int]int64{314: 40}}}
	cases := []struct {
		name string
		key  string
		ps   rules.PlayerState
		want *Observed
	}{
		{"skill below the target", agility10, rules.PlayerState{Skills: skills(map[string]int{"agility": 9})}, &Observed{Value: 9, Target: 10}},
		{"skill past the target", agility10, rules.PlayerState{Skills: skills(map[string]int{"agility": 12})}, &Observed{Value: 12, Target: 10}},
		{"skill not observed", agility10, rules.PlayerState{}, nil},
		{"item short, proven by a live baseline", feathers, rules.PlayerState{Items: liveBank(map[int]int64{314: 40})}, &Observed{Value: 40, Target: 120}},
		{"item held", feathers, rules.PlayerState{Items: liveBank(map[int]int64{314: 200})}, &Observed{Value: 200, Target: 120}},
		// A restored bank cannot prove the rest is missing: unknown, no number.
		{"item short in a restored bank", feathers, rules.PlayerState{Items: restored}, nil},
		{"items never observed", feathers, rules.PlayerState{}, nil},
		{"item family", hoodFamily, rules.PlayerState{Items: liveBank(map[int]int64{})}, &Observed{Value: 0, Target: 1}},
		{"quest points", "qp>=107", rules.PlayerState{Varps: map[int]int64{101: 33}}, &Observed{Value: 33, Target: 107}},
		{"quest points not observed", "qp>=107", rules.PlayerState{}, nil},
		{"boss kill count", "boss:zulrah>=50", rules.PlayerState{BossKC: map[string]int{"Zulrah": 12}}, &Observed{Value: 12, Target: 50}},
		// Quests, diaries and combat achievements have no number; a combat
		// achievement's var is a bitfield, never shown as a count.
		{"quest", cooks, rules.PlayerState{Quests: map[string]string{"Cook's Assistant": content.QuestNotStarted}}, nil},
		{"combat achievement", "ca:574@done", rules.PlayerState{Varps: map[int]int64{4204: 1<<29 | 1<<31}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ObservedValue(entities.MustParseObjectiveKey(tc.key), tc.ps, reg)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("observed = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The board carries the observed value only when built with the account's
// facts, next to the stored (latched) verdict.
func TestBuildBoardObserved(t *testing.T) {
	reg := registry(t)
	g := &content.Guide{ID: "g", Sections: []*content.Section{{ID: "s1", Title: "S", Steps: []*content.Step{
		{ID: "a", Text: "Train agility", Objectives: []content.Objective{{Key: entities.MustParseObjectiveKey(agility10), Role: content.RoleCompletes}}},
		{ID: "b", Text: "Cook", Objectives: []content.Objective{{Key: entities.MustParseObjectiveKey(cooks), Role: content.RoleCompletes}}},
	}}}}
	g.Index()
	in := guideInput(g)
	in.Verdicts, in.Registry = Verdicts{agility10: rules.Unmet}, reg
	if b := BuildBoard(in, nil); b.Steps[0].Objectives[0].Observed != nil {
		t.Fatalf("observed without facts: %+v", b.Steps[0].Objectives[0])
	}
	ps := rules.PlayerState{Skills: skills(map[string]int{"agility": 7})}
	in.State = &ps
	b := BuildBoard(in, nil)
	if got := b.Steps[0].Objectives[0]; got.Verdict != rules.Unmet || !reflect.DeepEqual(got.Observed, &Observed{Value: 7, Target: 10}) {
		t.Fatalf("agility objective = %+v", got)
	}
	if got := b.Steps[1].Objectives[0]; got.Observed != nil {
		t.Fatalf("quest objective = %+v", got)
	}
}
