package job

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func TestTransitions(t *testing.T) {
	all := []State{Queued, Running, Stalled, Succeeded, Failed, Canceled}
	legal := map[[2]State]bool{
		{Queued, Running}: true, {Queued, Canceled}: true,
		{Running, Succeeded}: true, {Running, Stalled}: true, {Running, Failed}: true, {Running, Canceled}: true,
		{Stalled, Running}: true, {Stalled, Succeeded}: true, {Stalled, Failed}: true, {Stalled, Canceled}: true,
	}
	for _, from := range all {
		for _, to := range all {
			err := Check(from, to)
			if legal[[2]State{from, to}] != (err == nil) {
				t.Errorf("%s to %s: err = %v, legal = %v", from, to, err, legal[[2]State{from, to}])
			}
			if err != nil && !errors.Is(err, ErrIllegalTransition) {
				t.Errorf("%s to %s: %v is not ErrIllegalTransition", from, to, err)
			}
		}
		if from.Final() && from.Active() {
			t.Errorf("%s is final and active", from)
		}
	}
}

func TestParseGoal(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		ok       bool
	}{
		{"objective key", `"skill:mining>=15"`, true},
		{"key written loosely", `"skill:mining>=015"`, false},
		{"unknown key", `"nonsense"`, false},
		{"varp rule", `{"type":"varp","id":281,"min":1000}`, true},
		{"combinator", `{"all":[{"type":"varp","id":281,"min":1000},{"type":"skill_level","skill":"mining","level":15}]}`, true},
		{"unknown rule", `{"type":"nope"}`, false},
		{"task", `{"task":{"type":"travel","location":"location:lumbridge"}}`, true},
		{"task without termination", `{"task":{"type":"talk","npc":"npc:hans"}}`, false},
		{"empty", ``, false},
		{"null", `null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := ParseGoal([]byte(tc.in))
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok = %v", err, tc.ok)
			}
			if err == nil {
				var back Goal
				if err := json.Unmarshal(g.JSON(), &back); err != nil || string(back.JSON()) != string(g.JSON()) {
					t.Fatalf("round trip: %v %s", err, back.JSON())
				}
			}
		})
	}
}

func state(mut func(*rules.PlayerState)) panel.Facts {
	ps := rules.PlayerState{Skills: map[string]rules.SkillState{}, Varps: map[int]int64{}}
	mut(&ps)
	return panel.Facts{Mode: "ironman", State: &ps}
}

func TestEvaluateGoalForms(t *testing.T) {
	varp, _ := ParseGoal([]byte(`{"type":"varp","id":281,"min":1000}`))
	key, _ := ParseGoal([]byte(`"skill:mining>=15"`))
	for _, tc := range []struct {
		name  string
		g     Goal
		facts panel.Facts
		want  rules.Verdict
	}{
		{"varp below", varp, state(func(p *rules.PlayerState) { p.Varps[281] = 999 }), rules.Unmet},
		{"varp met", varp, state(func(p *rules.PlayerState) { p.Varps[281] = 1000 }), rules.Met},
		{"varp never observed is unknown", varp, state(func(*rules.PlayerState) {}), rules.Unknown},
		{"no facts is unknown", varp, panel.Facts{}, rules.Unknown},
		{"key met", key, state(func(p *rules.PlayerState) { p.Skills["mining"] = rules.SkillState{Level: 15, XP: 2411} }), rules.Met},
		{"key unmet", key, state(func(p *rules.PlayerState) { p.Skills["mining"] = rules.SkillState{Level: 14, XP: 2000} }), rules.Unmet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.g.Evaluate(Env{Facts: tc.facts, Now: time.Now()})
			if r.Verdict != tc.want || r.Proven() != (tc.want == rules.Met) {
				t.Fatalf("verdict = %s proven = %v, want %s", r.Verdict, r.Proven(), tc.want)
			}
		})
	}
	if r := key.Evaluate(Env{Facts: state(func(p *rules.PlayerState) { p.Skills["mining"] = rules.SkillState{Level: 15} })}); r.Evidence != "nível 15" {
		t.Fatalf("evidence = %q", r.Evidence)
	}
}

// TestSaleGoalAgreesWithSaleProven: the sale's goal is the rule engine's
// statement of rules.SaleProven, never more permissive: the same inventories
// prove the same sales.
func TestSaleGoalAgreesWithSaleProven(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	const glass, noted, want = 1775, 1776, 170_000
	inv := func(state ingest.ItemState, complete bool, q map[int]int64) rules.ItemHoldings {
		return rules.ItemHoldings{rules.ContainerInventory: {State: state, Complete: complete, Quantities: q}}
	}
	goal, err := SaleGoal(glass, want)
	if err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]rules.ItemHoldings{
		"sold and paid":          inv(ingest.ItemLive, true, map[int]int64{entities.CoinsID: want}),
		"items left, coins not":  inv(ingest.ItemLive, true, map[int]int64{entities.CoinsID: want - 1}),
		"noted stack there":      inv(ingest.ItemLive, true, map[int]int64{noted: 3, entities.CoinsID: want}),
		"unnoted there":          inv(ingest.ItemLive, true, map[int]int64{glass: 1, entities.CoinsID: want}),
		"restored inventory":     inv(ingest.ItemRestored, true, map[int]int64{entities.CoinsID: want}),
		"deltas only":            inv(ingest.ItemLive, false, map[int]int64{entities.CoinsID: want}),
		"never observed":         nil,
		"bank holds the glass":   {rules.ContainerInventory: {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{entities.CoinsID: 5}}, rules.ContainerBank: {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{glass: 100}}},
		"coins only in the bank": {rules.ContainerInventory: {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{}}, rules.ContainerBank: {State: ingest.ItemLive, Complete: true, Quantities: map[int]int64{entities.CoinsID: want}}},
	} {
		t.Run(name, func(t *testing.T) {
			v, _ := rules.SaleProven(h, reg, glass, want)
			ps := rules.PlayerState{Items: h, Catalog: reg}
			got := goal.Evaluate(Env{Facts: panel.Facts{State: &ps}, Registry: reg, Now: time.Now()})
			if got.Proven() != (v == rules.Met) {
				t.Fatalf("goal proven = %v (%s), SaleProven = %s", got.Proven(), got.Verdict, v)
			}
		})
	}
}

func TestMetricAndProgress(t *testing.T) {
	g, _ := ParseGoal([]byte(`{"type":"varp","id":281,"min":1000}`))
	m := g.Metric()
	if m == nil || m.Kind != MetricVarp || m.ID != 281 || m.Target != 1000 {
		t.Fatalf("metric = %+v", m)
	}
	skill, _ := ParseGoal([]byte(`"skill:mining>=15"`))
	if sm := skill.Metric(); sm == nil || sm.Kind != MetricSkillXP || sm.Skill != "mining" || sm.Target != int64(entities.XPForLevel(15)) {
		t.Fatalf("skill metric = %+v", sm)
	}
	if quest, _ := ParseGoal([]byte(`"quest:cooks_assistant@finished"`)); quest.Metric() != nil {
		t.Fatal("a quest has no single number")
	}

	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	facts := func(v int64) rules.PlayerState { return rules.PlayerState{Varps: map[int]int64{281: v}} }
	p, changed := Observe(*m, nil, rules.PlayerState{}, at)
	if p != nil || changed {
		t.Fatal("an unobserved fact is not a progress")
	}
	p, changed = Observe(*m, nil, facts(10), at)
	if !changed || p.From != 10 || p.Current != 10 || p.Target != 1000 || !p.At.Equal(at) {
		t.Fatalf("first = %+v %v", p, changed)
	}
	p2, changed := Observe(*m, p, facts(10), at.Add(time.Minute))
	if changed || p2 != p {
		t.Fatal("the same value changed the progress")
	}
	p3, changed := Observe(*m, p, facts(60), at.Add(2*time.Minute))
	if !changed || p3.From != 10 || p3.Current != 60 || !p3.At.Equal(at.Add(2*time.Minute)) || p.Current != 10 {
		t.Fatalf("moved = %+v (prev %+v)", p3, p)
	}
}

func TestStall(t *testing.T) {
	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	g, _ := ParseGoal([]byte(`{"type":"varp","id":281,"min":1000}`))
	j, err := New("acc_x", KindPlugin, nil, g, nil, 0, SourceTest)
	if err != nil {
		t.Fatal(err)
	}
	if j.StallAfter != DefaultStallAfter(KindPlugin) || j.Metric == nil || j.State != Queued {
		t.Fatalf("job = %+v", j)
	}
	if d := j.DeadlineFrom(at); d == nil || !d.Equal(at.Add(j.StallAfter)) {
		t.Fatalf("deadline = %v", d)
	}
	sale, _ := SaleGoal(1775, 100)
	noMetric, _ := New("acc_x", KindGESell, nil, sale, nil, 0, SourceOwner)
	if noMetric.Metric != nil || noMetric.DeadlineFrom(at) != nil {
		t.Fatal("a job without a metric has no deadline")
	}
	for _, k := range Kinds {
		if d := DefaultStallAfter(k); d < MinStallAfter || d > MaxStallAfter {
			t.Errorf("%s default %s out of bounds", k, d)
		}
	}
	for _, bad := range []struct {
		name string
		kind Kind
		src  Source
		d    time.Duration
	}{
		{"kind", "fly", SourceTest, 0}, {"source", KindTrain, "me", 0}, {"stall too short", KindTrain, SourceTest, time.Second},
	} {
		if _, err := New("acc_x", bad.kind, nil, g, nil, bad.d, bad.src); err == nil {
			t.Errorf("%s accepted", bad.name)
		}
	}
}
