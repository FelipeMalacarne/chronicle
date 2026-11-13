package progress

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

const (
	shilo  = "quest:shilo_village@finished"
	jungle = "quest:jungle_potion@finished"
	agi50  = "skill:agility>=50"
)

var fleetT0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// fleetMember is an account over the goal guides of goalplan_test.go
// ("act" active): its levels, its marks on "act", its stored verdicts and
// its pins.
func fleetMember(t *testing.T, account string, levels map[string]int, marks map[string]MarkKind, stored Verdicts, pins map[string]PinnedGoal) FleetMember {
	t.Helper()
	in := goalInput(t, shilo)
	in.State = rules.PlayerState{Skills: skills(levels)}
	in.Verdicts = stored
	for i := range in.Guides {
		in.Guides[i].Verdicts = stored
		if in.Guides[i].Guide.ID == "act" {
			in.Guides[i].Marks = map[string]Mark{}
			for id, kind := range marks {
				in.Guides[i].Marks[id] = Mark{Kind: kind}
			}
		}
	}
	return FleetMember{Account: account, Goals: pins, Regressed: map[string]bool{}, Plan: in}
}

func pin(id string, priority int, at time.Time) PinnedGoal {
	return PinnedGoal{ID: id, Label: id, Priority: priority, Status: "active", CreatedAt: at}
}

func fleetFixture(t *testing.T) FleetInput {
	t.Helper()
	low := map[string]int{"agility": 20, "crafting": 30, "herblore": 5}
	high := map[string]int{"agility": 32, "crafting": 30, "herblore": 5}
	a := fleetMember(t, "acc_a", low, map[string]MarkKind{"x1": MarkManualDone}, Verdicts{}, map[string]PinnedGoal{shilo: pin("g_a", 100, fleetT0)})
	// B is the nearest to Shilo Village but does not pursue it.
	b := fleetMember(t, "acc_b", high, nil, Verdicts{}, map[string]PinnedGoal{agi50: pin("g_b", 10, fleetT0.Add(time.Hour))})
	achieved := pin("g_c", 100, fleetT0.Add(-time.Hour))
	achieved.Status = "achieved"
	c := fleetMember(t, "acc_c", low, map[string]MarkKind{"x1": MarkManualDone, "x5": MarkManualDone},
		Verdicts{shilo: rules.Met, jungle: rules.Met}, map[string]PinnedGoal{shilo: achieved})
	c.Regressed[shilo] = true
	return FleetInput{
		Members: []FleetMember{a, b, c},
		Keys:    []entities.ObjectiveKey{entities.MustParseObjectiveKey(agi50), entities.MustParseObjectiveKey(jungle)},
	}
}

func TestBuildFleetGoals(t *testing.T) {
	goals := BuildFleetGoals(fleetFixture(t))
	var order []string
	byKey := map[string]FleetGoal{}
	for _, g := range goals {
		order = append(order, g.Key)
		byKey[g.Key] = g
	}
	// The best priority first; a key asked again is not repeated.
	if fmt.Sprint(order) != "["+agi50+" "+shilo+" "+jungle+"]" {
		t.Fatalf("order = %v", order)
	}

	counts := []struct {
		key                     string
		pinned, met, unknown    int
		closest                 string
		verdicts, goals, levels string
	}{
		// Only B pursues 50 Agility; the level measures it.
		{agi50, 1, 0, 3, "acc_b", "[unknown unknown unknown]", "[- g_b -]", "[20 32 20]"},
		// B has more requirements met, but only A and C pursue it and C has it.
		{shilo, 2, 1, 2, "acc_a", "[unknown unknown met]", "[g_a - g_c]", "[- - -]"},
		// Nobody pursues it: every account without it competes, ties keep the order.
		{jungle, 0, 1, 2, "acc_a", "[unknown unknown met]", "[- - -]", "[- - -]"},
	}
	for _, c := range counts {
		g := byKey[c.key]
		var verdicts, ids, levels []string
		for _, s := range g.Accounts {
			verdicts = append(verdicts, string(s.Verdict))
			id, level := "-", "-"
			if s.Goal != nil {
				id = s.Goal.ID
			}
			if s.Level != nil {
				level = fmt.Sprint(*s.Level)
			}
			ids, levels = append(ids, id), append(levels, level)
		}
		if g.Pinned != c.pinned || g.Met != c.met || g.Unknown != c.unknown || g.Closest != c.closest {
			t.Errorf("%s = pinned %d met %d unknown %d closest %q", c.key, g.Pinned, g.Met, g.Unknown, g.Closest)
		}
		if fmt.Sprint(verdicts) != c.verdicts || fmt.Sprint(ids) != c.goals || fmt.Sprint(levels) != c.levels {
			t.Errorf("%s accounts = %v %v %v", c.key, verdicts, ids, levels)
		}
	}

	standings := []struct {
		key, account string
		req          RequirementCount
		short        string
		step, cursor string
		left         string
		regressed    bool
	}{
		// Druidic Ritual presumed by the mark on x1, Jungle Potion unknown,
		// 32 Agility known unmet (20 now); x2 to x5 still to do.
		{shilo, "acc_a", RequirementCount{Met: 2, Presumed: 1, Unknown: 1, Total: 5}, "[agility:20/32]", "x5", "x2", "4", false},
		// Without the mark Druidic Ritual is unknown too; 32 Agility met.
		{shilo, "acc_b", RequirementCount{Met: 3, Unknown: 2, Total: 5}, "[]", "x5", "x1", "5", false},
		// Jungle Potion latched met, 32 Agility still short; x5 is done: 0 left.
		{shilo, "acc_c", RequirementCount{Met: 3, Presumed: 1, Total: 5}, "[agility:20/32]", "x5", "x2", "0", true},
		// A level goal is its own shortfall; no route step reaches 50.
		{agi50, "acc_a", RequirementCount{}, "[agility:20/50]", "-", "x2", "-", false},
		// Jungle Potion is available for A (only the presumed ritual and Herblore before it).
		{jungle, "acc_a", RequirementCount{Met: 1, Presumed: 1, Total: 2}, "[]", "x3", "x2", "2", false},
		// C has it (stored met) though x3 is not done: nothing left.
		{jungle, "acc_c", RequirementCount{Met: 1, Presumed: 1, Total: 2}, "[]", "x3", "x2", "0", false},
	}
	for _, c := range standings {
		var s FleetStanding
		for _, x := range byKey[c.key].Accounts {
			if x.Account == c.account {
				s = x
			}
		}
		var short []string
		for _, f := range s.Short {
			short = append(short, fmt.Sprintf("%s:%d/%d", f.Skill, *f.Level, f.Target))
		}
		step, cursor, left := "-", "-", "-"
		if s.Step != nil {
			step = s.Step.StepID
		}
		if s.Cursor != nil {
			cursor = s.Cursor.StepID
		}
		if s.StepsLeft != nil {
			left = fmt.Sprint(*s.StepsLeft)
		}
		if s.Requirements != c.req || fmt.Sprint(short) != fmt.Sprint(strings.Fields(strings.Trim(c.short, "[]"))) ||
			step != c.step || cursor != c.cursor || left != c.left || s.Regressed != c.regressed {
			t.Errorf("%s %s = req %+v short %v step %s cursor %s left %s regressed %v", c.key, c.account, s.Requirements, short, step, cursor, left, s.Regressed)
		}
	}
	if cur := byKey[shilo].Accounts[0].Cursor; cur.GuideID != "act" || cur.SectionID != "s1" || cur.SectionPos != 2 || cur.Title == "" {
		t.Errorf("cursor place = %+v", cur)
	}
}

func TestBuildFleetGoalsQuestsLeft(t *testing.T) {
	// Druidic Ritual known unmet for D: a quest left before Shilo Village.
	d := fleetMember(t, "acc_d", map[string]int{"agility": 20}, nil, Verdicts{}, map[string]PinnedGoal{shilo: pin("g_d", 1, fleetT0)})
	d.Plan.State.Quests = map[string]string{"Druidic Ritual": "NOT_STARTED", "Jungle Potion": "NOT_STARTED"}
	goals := BuildFleetGoals(FleetInput{Members: []FleetMember{d}})
	if len(goals) != 1 {
		t.Fatalf("goals = %+v", goals)
	}
	if r := goals[0].Accounts[0].Requirements; r.QuestsLeft != 2 {
		t.Fatalf("requirements = %+v", r)
	}
}

// A fleet without pins answers only the asked keys, with every list empty
// and never null, so the web reads them without a guard.
func TestBuildFleetGoalsEmpty(t *testing.T) {
	if goals := BuildFleetGoals(FleetInput{}); len(goals) != 0 {
		t.Fatalf("no members, no keys = %+v", goals)
	}
	m := fleetMember(t, "acc_a", nil, nil, Verdicts{}, nil)
	goals := BuildFleetGoals(FleetInput{Members: []FleetMember{m}, Keys: []entities.ObjectiveKey{entities.MustParseObjectiveKey("item:7462x1")}})
	b, err := json.Marshal(goals)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"short":[]`, `"verdict":"unknown"`, `"pinned":0`} {
		if !strings.Contains(string(b), field) {
			t.Errorf("goals %s lack %s", b, field)
		}
	}
	for _, field := range []string{`"closest"`, `"goal"`, `"level"`, `"steps_left"`} {
		if strings.Contains(string(b), field) {
			t.Errorf("goals %s have %s", b, field)
		}
	}
}

func TestClosest(t *testing.T) {
	level := func(n int) *int { return &n }
	quest := entities.MustParseObjectiveKey(shilo)
	skill := entities.MustParseObjectiveKey(agi50)
	pinned := &PinnedGoal{ID: "g"}
	req := func(met, total int) RequirementCount { return RequirementCount{Met: met, Total: total} }
	cases := []struct {
		name     string
		key      entities.ObjectiveKey
		accounts []FleetStanding
		want     string
	}{
		{"share of requirements", quest, []FleetStanding{
			{Account: "a", Verdict: rules.Unmet, Requirements: req(1, 4)},
			{Account: "b", Verdict: rules.Unknown, Requirements: req(3, 4)},
		}, "b"},
		{"met never wins", quest, []FleetStanding{
			{Account: "a", Verdict: rules.Unmet, Requirements: req(1, 4)},
			{Account: "b", Verdict: rules.Met, Requirements: req(4, 4)},
		}, "a"},
		{"only the ones that pursue it", quest, []FleetStanding{
			{Account: "a", Verdict: rules.Unmet, Requirements: req(1, 4), Goal: pinned},
			{Account: "b", Verdict: rules.Unmet, Requirements: req(3, 4)},
		}, "a"},
		{"zero is no measure", quest, []FleetStanding{
			{Account: "a", Verdict: rules.Unmet, Requirements: req(0, 4)},
		}, ""},
		{"no requirements, no measure", entities.MustParseObjectiveKey("item:7462x1"), []FleetStanding{
			{Account: "a", Verdict: rules.Unmet},
		}, ""},
		{"ties keep the order", quest, []FleetStanding{
			{Account: "a", Verdict: rules.Unmet, Requirements: req(2, 4)},
			{Account: "b", Verdict: rules.Unmet, Requirements: req(2, 4)},
		}, "a"},
		{"the level of a level goal", skill, []FleetStanding{
			{Account: "a", Verdict: rules.Unmet, Level: level(40), Requirements: req(0, 0)},
			{Account: "b", Verdict: rules.Unknown},
			{Account: "c", Verdict: rules.Unmet, Level: level(45)},
		}, "c"},
		{"nobody", quest, nil, ""},
	}
	for _, c := range cases {
		if got := closest(c.key, c.accounts); got != c.want {
			t.Errorf("%s: closest = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStepsTo(t *testing.T) {
	cases := []struct {
		name  string
		marks map[string]MarkKind
		step  string
		want  string
	}{
		{"nothing done", nil, "x5", "5"},
		{"done and skipped ones do not count", map[string]MarkKind{"x1": MarkManualDone, "x3": MarkSkipped}, "x5", "3"},
		{"the step done", map[string]MarkKind{"x4": MarkManualDone}, "x4", "0"},
		{"a step of another route", nil, "y1", "-"},
	}
	for _, c := range cases {
		m := fleetMember(t, "acc", nil, c.marks, Verdicts{}, nil)
		got := "-"
		if n := routeOf(m.Plan).stepsTo(c.step); n != nil {
			got = fmt.Sprint(*n)
		}
		if got != c.want {
			t.Errorf("%s: steps to %s = %s, want %s", c.name, c.step, got, c.want)
		}
	}
	if r := (activeRoute{}); r.stepsTo("x1") != nil || r.cursor != nil {
		t.Errorf("no active route = %+v", r)
	}
}
