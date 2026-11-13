package progress

import (
	"reflect"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func TestVerdictsWithOwner(t *testing.T) {
	v := Verdicts{agility10: rules.Unmet, fishing20: rules.Met}
	got := v.WithOwner(map[string]bool{agility10: true, feathers: true, tinderbox: false})
	want := Verdicts{agility10: rules.Met, fishing20: rules.Met, feathers: rules.Met}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("overlay = %v, want %v", got, want)
	}
	if v[agility10] != rules.Unmet || len(v) != 2 {
		t.Fatalf("the stored verdicts changed: %v", v)
	}
	if got.Of(tinderbox) != rules.Unknown {
		t.Fatal("an unmarked, unobserved key must stay unknown")
	}
}

func TestReevaluateOwnerObjectiveClosesStepWithoutCascadeOrEvidence(t *testing.T) {
	g1, g2 := loadGuides(t)
	in := Input{
		Guides:       []*content.Guide{g1, g2},
		Registry:     registry(t),
		State:        rules.PlayerState{Skills: skills(map[string]int{"agility": 1, "fishing": 5})},
		Observations: obs(t0, "snap-1", Fact{"skill", "agility"}, Fact{"skill", "fishing"}),
		Objectives:   map[string]ObjectiveRecord{agility10: {Exists: true, Verdict: rules.Unmet}},
		Account:      Account{ActiveGuideID: "g1", Layer: "classic", Mode: "ironman"},
		Now:          t1,
	}
	before := Reevaluate(in)
	if got := events(before); got[ref("g1", "a3")].To == AutoDone {
		t.Fatalf("a3 is done without the mark: %+v", got)
	}

	// An owner mark closes the step in every guide that completes it, with
	// no evidence written and no implied marks (the cascade is evidence only).
	in.OwnerObjectives = map[string]bool{agility10: true}
	in.LastStatus = statusAfter(nil, before)
	in.Cursors = cursorsAfter(nil, before)
	plan := Reevaluate(in)
	if e := events(plan)[ref("g1", "a3")]; e.To != AutoDone {
		t.Fatalf("a3 event = %+v", e)
	}
	for _, u := range plan.Objectives {
		if u.Key == agility10 {
			t.Fatalf("the owner mark was written as a verdict: %+v", u)
		}
	}
	if len(plan.Marks) != 0 {
		t.Fatalf("an owner mark triggered the cascade: %+v", plan.Marks)
	}
	if cursorsAfter(in.Cursors, plan)["g1"].StepID == "a3" {
		t.Fatal("the cursor stays on a step the owner's mark closed")
	}
}

func TestReevaluateStatusOnlyOwnerMark(t *testing.T) {
	g1, g2 := loadGuides(t)
	in := Input{
		Guides:          []*content.Guide{g1, g2},
		Registry:        registry(t),
		State:           rules.PlayerState{Skills: skills(map[string]int{"agility": 10})}, // would cascade if evaluated
		Observations:    obs(t0, "snap-1", Fact{"skill", "agility"}),
		Objectives:      map[string]ObjectiveRecord{agility10: {Exists: true, Verdict: rules.Unmet}},
		OwnerObjectives: map[string]bool{fishing20: true},
		Account:         Account{ActiveGuideID: "g1", Layer: "classic", Mode: "ironman"},
		Now:             t1,
		StatusOnly:      true,
	}
	plan := Reevaluate(in)
	if len(plan.Objectives) != 0 || len(plan.ObjectiveEvents) != 0 || len(plan.Marks) != 0 || plan.StepsChanged != 0 {
		t.Fatalf("status-only wrote %+v", plan)
	}
	if len(plan.StepEvents) == 0 {
		t.Fatal("the marked objective closed no step")
	}
	for _, e := range plan.StepEvents {
		if e.Cause != CauseUser || e.To != AutoDone {
			t.Fatalf("event %+v: want cause user and auto_done", e)
		}
	}
}

func TestOwnerMarkReleasesBlockedRequirement(t *testing.T) {
	g, reg := floatingGuide(t), registry(t)
	in := GuideInput{
		Guide: g, Verdicts: Verdicts{slayer48: rules.Unmet, agility62: rules.Unmet}, Registry: reg,
		Marks: map[string]Mark{}, LastStatus: map[string]Status{},
	}
	if b := BuildBoard(in, nil); !reflect.DeepEqual(b.Steps[0].BlockedBy, []string{slayer48}) {
		t.Fatalf("blocked_by = %v", b.Steps[0].BlockedBy)
	}
	// A requirement the owner marked counts as satisfied: it blocks nothing.
	in.OwnerMarked = map[string]bool{slayer48: true}
	in.Verdicts = in.Verdicts.WithOwner(in.OwnerMarked)
	b := BuildBoard(in, nil)
	if b.Steps[0].BlockedBy != nil {
		t.Fatalf("a marked requirement still blocks: %v", b.Steps[0].BlockedBy)
	}
	if b.Counts.Blocked != 1 { // the agility 62 step stays blocked
		t.Fatalf("counts = %+v", b.Counts)
	}
}

func TestBuildBoardOwnerMarked(t *testing.T) {
	g1, _ := loadGuides(t)
	in := guideInput(g1)
	in.Verdicts = Verdicts{agility10: rules.Unmet}.WithOwner(map[string]bool{agility10: true})
	in.OwnerMarked = map[string]bool{agility10: true}
	b := BuildBoard(in, nil)
	var seen bool
	for _, s := range b.Steps {
		for _, o := range s.Objectives {
			switch {
			case o.Key == agility10:
				seen = true
				if o.Verdict != rules.Met || !o.OwnerMarked {
					t.Fatalf("marked objective = %+v", o)
				}
			case o.OwnerMarked:
				t.Fatalf("%s flagged as marked", o.Key)
			case o.Verdict == rules.Unmet:
				t.Fatalf("%s shown as unmet without being observed", o.Key)
			}
		}
	}
	if !seen {
		t.Fatal("agility objective not on the board")
	}
}
