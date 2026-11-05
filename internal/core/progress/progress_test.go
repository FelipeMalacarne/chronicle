package progress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

var (
	t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Hour)
)

const (
	agility10 = "skill:agility>=10"
	fishing20 = "skill:fishing>=20"
	feathers  = "item:314x120"
	tinderbox = "item:590x1"
	cooks     = "quest:cooks_assistant@started"
)

func loadGuides(t *testing.T) (g1, g2 *content.Guide) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "guides.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gs []*content.Guide
	if err := json.Unmarshal(raw, &gs); err != nil {
		t.Fatal(err)
	}
	for _, g := range gs {
		g.Index()
	}
	return gs[0], gs[1]
}

func registry(t *testing.T) *entities.Registry {
	t.Helper()
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func skills(levels map[string]int) map[string]rules.SkillState {
	out := map[string]rules.SkillState{}
	for k, v := range levels {
		out[k] = rules.SkillState{Level: v}
	}
	return out
}

func liveBank(q map[int]int64) rules.ItemHoldings {
	c := func(q map[int]int64) rules.ContainerItems {
		return rules.ContainerItems{State: ingest.ItemLive, ObservedAt: t0, Complete: true, Quantities: q}
	}
	return rules.ItemHoldings{
		rules.ContainerBank: c(q), rules.ContainerInventory: c(map[int]int64{}), rules.ContainerEquipment: c(map[int]int64{}),
	}
}

func obs(at time.Time, snapshot string, facts ...Fact) map[Fact]FactObservation {
	out := map[Fact]FactObservation{}
	for _, f := range facts {
		out[f] = FactObservation{Source: "plugin", SnapshotID: snapshot, ObservedAt: at}
	}
	return out
}

func TestEvaluateObjective(t *testing.T) {
	reg := registry(t)
	cases := []struct {
		name  string
		key   string
		ps    rules.PlayerState
		want  rules.Verdict
		facts []Fact
	}{
		{"skill met", agility10, rules.PlayerState{Skills: skills(map[string]int{"agility": 12})}, rules.Met, []Fact{{"skill", "agility"}}},
		{"skill unmet", agility10, rules.PlayerState{Skills: skills(map[string]int{"agility": 9})}, rules.Unmet, []Fact{{"skill", "agility"}}},
		{"skill not observed", agility10, rules.PlayerState{}, rules.Unknown, nil},
		{"quest by catalog name", cooks, rules.PlayerState{Quests: map[string]string{"Cook's Assistant": content.QuestInProgress}}, rules.Met, []Fact{{"quest", "Cook's Assistant"}}},
		{"quest finished counts as started", cooks, rules.PlayerState{Quests: map[string]string{"Cook's Assistant": content.QuestFinished}}, rules.Met, []Fact{{"quest", "Cook's Assistant"}}},
		{"quest not started", "quest:cooks_assistant@finished", rules.PlayerState{Quests: map[string]string{"Cook's Assistant": content.QuestInProgress}}, rules.Unmet, []Fact{{"quest", "Cook's Assistant"}}},
		{"quest facet missing", cooks, rules.PlayerState{}, rules.Unknown, nil},
		{"item owned in bank", feathers, rules.PlayerState{Items: liveBank(map[int]int64{314: 200})}, rules.Met,
			[]Fact{{FactItemContainer, "bank"}, {FactItemContainer, "equipment"}, {FactItemContainer, "inventory"}}},
		{"item absent with full live baseline", feathers, rules.PlayerState{Items: liveBank(map[int]int64{314: 5})}, rules.Unmet,
			[]Fact{{FactItemContainer, "bank"}, {FactItemContainer, "equipment"}, {FactItemContainer, "inventory"}}},
		{"items never observed", feathers, rules.PlayerState{}, rules.Unknown, nil},
		{"restored bank never proves absence", feathers, rules.PlayerState{Items: rules.ItemHoldings{
			rules.ContainerBank: {State: ingest.ItemRestored, ObservedAt: t0, Quantities: map[int]int64{}},
		}}, rules.Unknown, nil},
		{"diary without facet", "diary:ardougne:easy@complete", rules.PlayerState{}, rules.Unknown, nil},
		{"diary complete", "diary:ardougne:easy@complete", rules.PlayerState{Diaries: map[string]map[string]bool{"ardougne": {"easy": true}}}, rules.Met, []Fact{{"diary", "ardougne:easy"}}},
		{"boss by hiscores name", "boss:zulrah>=50", rules.PlayerState{BossKC: map[string]int{"Zulrah": 51}}, rules.Met, []Fact{{"activity", "Zulrah"}}},
		{"unlock has no source yet", "unlock:fairy_rings@done", rules.PlayerState{}, rules.Unknown, nil},
		{"combat achievement has no source yet", "ca:574@done", rules.PlayerState{}, rules.Unknown, nil},
		{"quest points have no source yet", "qp>=107", rules.PlayerState{}, rules.Unknown, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateObjective(entities.MustParseObjectiveKey(tc.key), tc.ps, reg)
			if got.Verdict != tc.want {
				t.Fatalf("verdict = %s, want %s (%+v)", got.Verdict, tc.want, got.Evidence)
			}
			facts := append([]Fact(nil), got.Facts...)
			sort.Slice(facts, func(i, j int) bool { return facts[i].Key < facts[j].Key })
			if !reflect.DeepEqual(facts, tc.facts) {
				t.Fatalf("facts = %+v, want %+v", facts, tc.facts)
			}
			if len(got.Evidence) == 0 {
				t.Fatal("every verdict carries evidence")
			}
		})
	}
}

func TestAdvance(t *testing.T) {
	skillFact := Fact{"skill", "agility"}
	metEval := Evaluation{Verdict: rules.Met, Evidence: []rules.Evidence{{Verdict: rules.Met}}, Facts: []Fact{skillFact}}
	unmetEval := Evaluation{Verdict: rules.Unmet, Evidence: []rules.Evidence{{Verdict: rules.Unmet}}, Facts: []Fact{skillFact}}
	unknownEval := Evaluation{Verdict: rules.Unknown, Evidence: []rules.Evidence{{Verdict: rules.Unknown}}}
	metAt := t0
	latched := ObjectiveRecord{Exists: true, Verdict: rules.Met, FirstMetAt: &metAt, MetInGuideID: "g1"}
	regressed := latched
	regressed.RegressedAt = &metAt
	withSnap := obs(t0, "snap-1", skillFact)

	cases := []struct {
		name       string
		rec        ObjectiveRecord
		ev         Evaluation
		obs        map[Fact]FactObservation
		wantUpdate bool
		check      func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange)
	}{
		{"first met records evidence, snapshot and active guide", ObjectiveRecord{}, metEval, withSnap, true, func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange) {
			if u.Verdict != rules.Met || u.EvidenceSnapshotID != "snap-1" || u.MetInGuideID != "g1" || u.FirstMetAt == nil || !u.FirstMetAt.Equal(t1) || !u.ReplaceEvidence {
				t.Fatalf("update = %+v", u)
			}
			if len(u.Evidence) != 1 || len(u.Evidence[0].Facts) != 1 || u.Evidence[0].Facts[0].SnapshotID != "snap-1" {
				t.Fatalf("evidence = %+v", u.Evidence)
			}
			if c == nil || c.From != "" || c.To != rules.Met {
				t.Fatalf("change = %+v", c)
			}
		}},
		{"met without snapshot stays unknown and is not written", ObjectiveRecord{}, metEval, nil, false, nil},
		{"met without snapshot over an unmet row becomes unknown", ObjectiveRecord{Exists: true, Verdict: rules.Unmet}, metEval, nil, true, func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange) {
			if u.Verdict != rules.Unknown || u.EvidenceSnapshotID != "" || c.To != rules.Unknown {
				t.Fatalf("update = %+v change = %+v", u, c)
			}
		}},
		{"unknown without row is not written", ObjectiveRecord{}, unknownEval, nil, false, nil},
		{"unchanged unmet is not written", ObjectiveRecord{Exists: true, Verdict: rules.Unmet}, unmetEval, withSnap, false, nil},
		{"unknown to unmet is an event", ObjectiveRecord{Exists: true, Verdict: rules.Unknown}, unmetEval, withSnap, true, func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange) {
			if u.Verdict != rules.Unmet || c.From != rules.Unknown || c.To != rules.Unmet || u.FirstMetAt != nil {
				t.Fatalf("update = %+v change = %+v", u, c)
			}
		}},
		{"latched met that stops holding is regressed, not unmet", latched, unmetEval, withSnap, true, func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange) {
			if u.Verdict != rules.Met || u.RegressedAt == nil || !u.RegressedAt.Equal(t1) || u.ReplaceEvidence || c != nil {
				t.Fatalf("update = %+v change = %+v", u, c)
			}
			if !u.FirstMetAt.Equal(t0) || u.MetInGuideID != "g1" {
				t.Fatalf("latched fields lost: %+v", u)
			}
		}},
		{"latched met that becomes unknown is regressed", latched, unknownEval, nil, true, func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange) {
			if u.Verdict != rules.Met || u.RegressedAt == nil || c != nil {
				t.Fatalf("update = %+v change = %+v", u, c)
			}
		}},
		{"already regressed stays as is", regressed, unmetEval, withSnap, false, nil},
		{"regressed that holds again is cleared", regressed, metEval, withSnap, true, func(t *testing.T, u *ObjectiveUpdate, c *VerdictChange) {
			if u.Verdict != rules.Met || u.RegressedAt != nil || u.ReplaceEvidence || c != nil {
				t.Fatalf("update = %+v change = %+v", u, c)
			}
		}},
		{"latched met that still holds is not written", latched, metEval, withSnap, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, c := Advance(agility10, tc.rec, tc.ev, tc.obs, "g1", t1)
			if (u != nil) != tc.wantUpdate {
				t.Fatalf("update = %+v, want update %v", u, tc.wantUpdate)
			}
			if tc.check != nil {
				tc.check(t, u, c)
			}
		})
	}
}

func TestAdvanceUsesNewestFactSnapshot(t *testing.T) {
	a, b := Fact{FactItemContainer, "bank"}, Fact{FactItemContainer, "inventory"}
	o := map[Fact]FactObservation{
		a: {Source: "plugin", SnapshotID: "old", ObservedAt: t0},
		b: {Source: "plugin", SnapshotID: "new", ObservedAt: t1},
	}
	u, _ := Advance(feathers, ObjectiveRecord{}, Evaluation{Verdict: rules.Met, Evidence: []rules.Evidence{{Verdict: rules.Met}}, Facts: []Fact{a, b}}, o, "g1", t1)
	if u == nil || u.EvidenceSnapshotID != "new" {
		t.Fatalf("update = %+v", u)
	}
}

func TestStepStatus(t *testing.T) {
	g1, _ := loadGuides(t)
	step := func(id string) *content.Step {
		st, ok := g1.Step(id)
		if !ok {
			t.Fatalf("no step %s", id)
		}
		return st
	}
	cases := []struct {
		name     string
		step     string
		verdicts Verdicts
		mark     *Mark
		want     Status
	}{
		{"checkbox is pending", "a1", nil, nil, Pending},
		{"checkbox marked done", "a1", nil, &Mark{Kind: MarkManualDone}, ManualDone},
		{"all completing met", "a3", Verdicts{agility10: rules.Met}, nil, AutoDone},
		{"unknown leaves pending", "a3", Verdicts{agility10: rules.Unknown}, nil, Pending},
		{"missing verdict leaves pending", "a3", Verdicts{}, nil, Pending},
		{"uses objectives never complete", "a5", Verdicts{feathers: rules.Met, tinderbox: rules.Unmet}, nil, AutoDone},
		{"completing unmet", "a5", Verdicts{feathers: rules.Unmet, tinderbox: rules.Met}, nil, Pending},
		{"mark wins over evidence", "a3", Verdicts{agility10: rules.Met}, &Mark{Kind: MarkPinnedOpen}, PinnedOpen},
		{"skipped mark", "a3", Verdicts{agility10: rules.Met}, &Mark{Kind: MarkSkipped}, Skipped},
		{"implied mark is never auto_done", "a1", nil, &Mark{Kind: MarkImplied, ImpliedBy: "a3"}, Implied},
		{"starts counts as completing", "a6", Verdicts{cooks: rules.Met}, nil, AutoDone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StepStatus(step(tc.step), tc.verdicts, tc.mark); got != tc.want {
				t.Fatalf("status = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPlaceCursor(t *testing.T) {
	g1, _ := loadGuides(t)
	classic := Account{ActiveGuideID: "g1", Layer: "classic", Mode: "ironman"}
	enhanced := Account{ActiveGuideID: "g1", Layer: "enhanced", Mode: "ironman"}
	allDone := func(except ...string) map[string]Status {
		out := map[string]Status{}
		for _, st := range g1.Steps() {
			out[st.ID] = AutoDone
		}
		for _, id := range except {
			out[id] = Pending
		}
		return out
	}
	cases := []struct {
		name   string
		status map[string]Status
		acc    Account
		cur    CursorRecord
		want   CursorRecord
	}{
		{"first pending step", map[string]Status{}, classic, CursorRecord{}, CursorRecord{"a1", CursorAuto}},
		{"skips done, implied and skipped", map[string]Status{"a1": Implied, "a2": Skipped, "a3": AutoDone}, classic, CursorRecord{}, CursorRecord{"a4", CursorAuto}},
		{"stops at pinned_open", map[string]Status{"a1": ManualDone, "a2": PinnedOpen}, classic, CursorRecord{}, CursorRecord{"a2", CursorAuto}},
		{"hidden layer is skipped", allDone("b0", "b1"), classic, CursorRecord{}, CursorRecord{"b1", CursorAuto}},
		{"visible layer is not skipped", allDone("b0", "b1"), enhanced, CursorRecord{}, CursorRecord{"b0", CursorAuto}},
		{"tracks are not the main route", allDone("t1"), classic, CursorRecord{}, CursorRecord{}},
		{"pin is kept", map[string]Status{}, classic, CursorRecord{"b1", CursorPin}, CursorRecord{"b1", CursorPin}},
		{"pin on a track step is kept", map[string]Status{}, classic, CursorRecord{"t1", CursorPin}, CursorRecord{"t1", CursorPin}},
		{"pin dropped when the step is done", map[string]Status{"b1": ManualDone}, classic, CursorRecord{"b1", CursorPin}, CursorRecord{"a1", CursorAuto}},
		{"pin dropped when the step is gone", map[string]Status{}, classic, CursorRecord{"zz", CursorPin}, CursorRecord{"a1", CursorAuto}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlaceCursor(g1, tc.status, Verdicts{}, nil, tc.acc, tc.cur); got != tc.want {
				t.Fatalf("cursor = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// statusAfter applies a plan's events to the previous last status.
func statusAfter(last map[StepRef]Status, plan Plan) map[StepRef]Status {
	out := map[StepRef]Status{}
	for k, v := range last {
		out[k] = v
	}
	for _, e := range plan.StepEvents {
		out[e.Ref] = e.To
	}
	return out
}

func objectivesAfter(prev map[string]ObjectiveRecord, plan Plan) map[string]ObjectiveRecord {
	out := map[string]ObjectiveRecord{}
	for k, v := range prev {
		out[k] = v
	}
	for _, u := range plan.Objectives {
		out[u.Key] = u.Record()
	}
	return out
}

func marksAfter(prev map[StepRef]Mark, plan Plan) map[StepRef]Mark {
	out := map[StepRef]Mark{}
	for k, v := range prev {
		out[k] = v
	}
	for _, w := range plan.Marks {
		out[w.Ref] = w.Mark
	}
	return out
}

func cursorsAfter(prev map[string]CursorRecord, plan Plan) map[string]CursorRecord {
	out := map[string]CursorRecord{}
	for k, v := range prev {
		out[k] = v
	}
	for _, c := range plan.Cursors {
		out[c.GuideID] = c.Cursor
	}
	return out
}

func events(plan Plan) map[StepRef]StepEvent {
	out := map[StepRef]StepEvent{}
	for _, e := range plan.StepEvents {
		out[e.Ref] = e
	}
	return out
}

func ref(g, s string) StepRef { return StepRef{GuideID: g, StepID: s} }

func TestReevaluateSharedProgressCascadeAndCursor(t *testing.T) {
	g1, g2 := loadGuides(t)
	in := Input{
		Guides:       []*content.Guide{g1, g2},
		Registry:     registry(t),
		State:        rules.PlayerState{Skills: skills(map[string]int{"agility": 10, "fishing": 5})},
		Observations: obs(t0, "snap-1", Fact{"skill", "agility"}, Fact{"skill", "fishing"}),
		Account:      Account{ActiveGuideID: "g1", Layer: "classic", Mode: "ironman"},
		Now:          t1,
	}
	plan := Reevaluate(in)

	got := events(plan)
	want := map[StepRef]StepEvent{
		ref("g1", "a3"): {ref("g1", "a3"), Pending, AutoDone, CauseEngine},
		ref("g1", "b2"): {ref("g1", "b2"), Pending, AutoDone, CauseEngine},
		ref("g2", "c2"): {ref("g2", "c2"), Pending, AutoDone, CauseEngine},
		ref("g1", "a1"): {ref("g1", "a1"), Pending, Implied, CauseImplied},
		ref("g1", "a2"): {ref("g1", "a2"), Pending, Implied, CauseImplied},
		ref("g1", "b1"): {ref("g1", "b1"), Pending, Implied, CauseImplied},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v\nwant   %+v", got, want)
	}
	if plan.StepsChanged != 3 {
		t.Fatalf("steps_changed = %d, want 3 (engine changes in both guides)", plan.StepsChanged)
	}
	marks := marksAfter(nil, plan)
	if m := marks[ref("g1", "a1")]; m.Kind != MarkImplied || m.ImpliedBy != "a3" {
		t.Fatalf("a1 mark = %+v", m)
	}
	if m := marks[ref("g1", "b1")]; m.ImpliedBy != "b2" {
		t.Fatalf("b1 mark = %+v", m)
	}
	if _, ok := marks[ref("g1", "b0")]; ok {
		t.Fatal("a step outside the account's layer was implied")
	}
	if _, ok := marks[ref("g2", "c1")]; ok {
		t.Fatal("the cascade ran on a guide that is not the active one")
	}
	if len(plan.ObjectiveEvents) != 2 {
		t.Fatalf("objective events = %+v", plan.ObjectiveEvents)
	}
	cursors := cursorsAfter(nil, plan)
	if cursors["g1"] != (CursorRecord{"a4", CursorAuto}) || cursors["g2"] != (CursorRecord{"c1", CursorAuto}) {
		t.Fatalf("cursors = %+v", cursors)
	}

	// The same facts again change nothing: no event, no cascade, no cursor.
	in.Objectives = objectivesAfter(nil, plan)
	in.LastStatus = statusAfter(nil, plan)
	in.Marks = marks
	in.Cursors = cursors
	again := Reevaluate(in)
	if len(again.StepEvents) != 0 || len(again.Marks) != 0 || len(again.Cursors) != 0 || len(again.Objectives) != 0 || again.StepsChanged != 0 {
		t.Fatalf("second reevaluation wrote %+v", again)
	}

	// The owner clears an implied mark: the next reevaluation does not bring it back.
	delete(in.Marks, ref("g1", "a1"))
	in.LastStatus[ref("g1", "a1")] = Pending
	cleared := Reevaluate(in)
	if len(cleared.Marks) != 0 || len(cleared.StepEvents) != 0 {
		t.Fatalf("cleared implied mark came back: %+v", cleared)
	}
}

func TestReevaluateCascadeOnlyForTheRouteThatMetIt(t *testing.T) {
	g1, g2 := loadGuides(t)
	metAt := t0
	in := Input{
		Guides:   []*content.Guide{g1, g2},
		Registry: registry(t),
		State:    rules.PlayerState{Skills: skills(map[string]int{"agility": 10})},
		// agility 10 was met while g2 was the active guide.
		Objectives:   map[string]ObjectiveRecord{agility10: {Exists: true, Verdict: rules.Met, FirstMetAt: &metAt, MetInGuideID: "g2"}},
		Observations: obs(t0, "snap-1", Fact{"skill", "agility"}),
		Account:      Account{ActiveGuideID: "g1", Layer: "classic"},
		Now:          t1,
	}
	plan := Reevaluate(in)
	if len(plan.Marks) != 0 {
		t.Fatalf("cascade fired for an objective met on another route: %+v", plan.Marks)
	}
	if e := events(plan)[ref("g1", "a3")]; e.To != AutoDone || e.Cause != CauseEngine {
		t.Fatalf("a3 event = %+v", e)
	}
}

func TestReevaluateTrackNeverCascades(t *testing.T) {
	g1, _ := loadGuides(t)
	plan := Reevaluate(Input{
		Guides:       []*content.Guide{g1},
		State:        rules.PlayerState{Skills: skills(map[string]int{"fishing": 20})},
		Observations: obs(t0, "snap-1", Fact{"skill", "fishing"}),
		Account:      Account{ActiveGuideID: "g1"},
		Now:          t1,
	})
	if e := events(plan)[ref("g1", "t2")]; e.To != AutoDone {
		t.Fatalf("t2 event = %+v", e)
	}
	if len(plan.Marks) != 0 {
		t.Fatalf("a track step implied earlier steps: %+v", plan.Marks)
	}
}

func TestReevaluateSpentItemsKeepTheStepDone(t *testing.T) {
	g1, _ := loadGuides(t)
	bank := Fact{FactItemContainer, "bank"}
	inv := Fact{FactItemContainer, "inventory"}
	equip := Fact{FactItemContainer, "equipment"}
	in := Input{
		Guides:       []*content.Guide{g1},
		Registry:     registry(t),
		State:        rules.PlayerState{Items: liveBank(map[int]int64{314: 120}), Catalog: registry(t)},
		Observations: obs(t0, "snap-1", bank, inv, equip),
		Account:      Account{ActiveGuideID: "g1"},
		Now:          t1,
	}
	first := Reevaluate(in)
	if e := events(first)[ref("g1", "a5")]; e.To != AutoDone {
		t.Fatalf("a5 event = %+v", e)
	}
	in.Objectives = objectivesAfter(nil, first)
	in.LastStatus = statusAfter(nil, first)
	in.Marks = marksAfter(nil, first)
	in.Cursors = cursorsAfter(nil, first)
	in.State.Items = liveBank(map[int]int64{314: 3})
	in.Observations = obs(t1, "snap-2", bank, inv, equip)
	in.Now = t1.Add(time.Hour)
	spent := Reevaluate(in)
	if len(spent.StepEvents) != 0 || spent.StepsChanged != 0 || len(spent.ObjectiveEvents) != 0 {
		t.Fatalf("spending the items reopened the step: %+v", spent)
	}
	var feathersUpdate *ObjectiveUpdate
	for i := range spent.Objectives {
		if spent.Objectives[i].Key == feathers {
			feathersUpdate = &spent.Objectives[i]
		}
	}
	if feathersUpdate == nil || feathersUpdate.Verdict != rules.Met || feathersUpdate.RegressedAt == nil || feathersUpdate.ReplaceEvidence {
		t.Fatalf("feathers update = %+v", feathersUpdate)
	}
}

func TestReevaluateMarkWithoutEventIsAMigration(t *testing.T) {
	g1, _ := loadGuides(t)
	plan := Reevaluate(Input{
		Guides:  []*content.Guide{g1},
		Marks:   map[StepRef]Mark{ref("g1", "a1"): {Kind: MarkManualDone}},
		Account: Account{ActiveGuideID: "g1"},
		Now:     t1,
	})
	if e := events(plan)[ref("g1", "a1")]; e.Cause != CauseMigration || e.To != ManualDone {
		t.Fatalf("a1 event = %+v", e)
	}
	if plan.StepsChanged != 0 {
		t.Fatalf("steps_changed = %d", plan.StepsChanged)
	}
}

func guideInput(g *content.Guide) GuideInput {
	return GuideInput{
		Guide: g, Verdicts: Verdicts{}, Marks: map[string]Mark{}, LastStatus: map[string]Status{},
		Cursor: CursorRecord{"a1", CursorAuto}, Account: Account{ActiveGuideID: g.ID, Layer: "classic"},
	}
}

func TestPlanMark(t *testing.T) {
	g1, _ := loadGuides(t)
	note := "done by hand"
	empty := ""
	cases := []struct {
		name    string
		marks   map[string]Mark
		last    map[string]Status
		step    string
		action  MarkAction
		note    *string
		wantErr error
		check   func(t *testing.T, p OwnerPlan)
	}{
		{name: "manual_done records a user event and moves the cursor", step: "a1", action: ActionManualDone, note: &note, check: func(t *testing.T, p OwnerPlan) {
			if len(p.Marks) != 1 || p.Marks[0].Mark.Kind != MarkManualDone || p.Marks[0].Mark.Note != note {
				t.Fatalf("marks = %+v", p.Marks)
			}
			if len(p.Events) != 1 || p.Events[0] != (StepEvent{ref("g1", "a1"), Pending, ManualDone, CauseUser}) {
				t.Fatalf("events = %+v", p.Events)
			}
			if p.Cursor == nil || *p.Cursor != (CursorRecord{"a2", CursorAuto}) {
				t.Fatalf("cursor = %+v", p.Cursor)
			}
		}},
		{name: "a nil note keeps the note", marks: map[string]Mark{"a1": {Kind: MarkSkipped, Note: "keep"}}, last: map[string]Status{"a1": Skipped}, step: "a1", action: ActionManualDone, check: func(t *testing.T, p OwnerPlan) {
			if p.Marks[0].Mark.Note != "keep" {
				t.Fatalf("note = %q", p.Marks[0].Mark.Note)
			}
		}},
		{name: "an empty note clears it", marks: map[string]Mark{"a1": {Kind: MarkSkipped, Note: "keep"}}, last: map[string]Status{"a1": Skipped}, step: "a1", action: ActionSkipped, note: &empty, check: func(t *testing.T, p OwnerPlan) {
			if p.Marks[0].Mark.Note != "" || len(p.Events) != 0 {
				t.Fatalf("plan = %+v", p)
			}
		}},
		{name: "clear removes the mark", marks: map[string]Mark{"a1": {Kind: MarkManualDone}}, last: map[string]Status{"a1": ManualDone}, step: "a1", action: ActionClear, check: func(t *testing.T, p OwnerPlan) {
			if len(p.Marks) != 1 || p.Marks[0].Mark != nil || p.Events[0].To != Pending {
				t.Fatalf("plan = %+v", p)
			}
		}},
		{name: "clear without a mark does nothing", step: "a1", action: ActionClear, check: func(t *testing.T, p OwnerPlan) {
			if len(p.Marks) != 0 || len(p.Events) != 0 || p.Cursor != nil {
				t.Fatalf("plan = %+v", p)
			}
		}},
		{name: "confirm_implied becomes manual_done", marks: map[string]Mark{"a1": {Kind: MarkImplied, ImpliedBy: "a3"}}, last: map[string]Status{"a1": Implied}, step: "a1", action: ActionConfirmImplied, check: func(t *testing.T, p OwnerPlan) {
			if p.Marks[0].Mark.Kind != MarkManualDone || p.Events[0] != (StepEvent{ref("g1", "a1"), Implied, ManualDone, CauseUser}) {
				t.Fatalf("plan = %+v", p)
			}
		}},
		{name: "confirm_implied needs an implied mark", step: "a1", action: ActionConfirmImplied, wantErr: ErrNotImplied},
		{name: "unknown step", step: "zz", action: ActionManualDone, wantErr: ErrStepNotFound},
		{name: "unknown action", step: "a1", action: "done", wantErr: ErrInvalidAction},
		{name: "pinned_open over evidence keeps the step open", step: "a3", action: ActionPinnedOpen, last: map[string]Status{"a3": AutoDone}, check: func(t *testing.T, p OwnerPlan) {
			if p.Events[0] != (StepEvent{ref("g1", "a3"), AutoDone, PinnedOpen, CauseUser}) {
				t.Fatalf("events = %+v", p.Events)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := guideInput(g1)
			in.Verdicts = Verdicts{agility10: rules.Met}
			if tc.marks != nil {
				in.Marks = tc.marks
			}
			if tc.last != nil {
				in.LastStatus = tc.last
			}
			p, err := PlanMark(in, tc.step, tc.action, tc.note)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, p)
		})
	}
}

func TestPlanConfirmImplied(t *testing.T) {
	g1, _ := loadGuides(t)
	in := guideInput(g1)
	in.Marks = map[string]Mark{"a1": {Kind: MarkImplied, ImpliedBy: "a3"}, "a2": {Kind: MarkImplied, ImpliedBy: "a3"}, "a4": {Kind: MarkSkipped}}
	in.LastStatus = map[string]Status{"a1": Implied, "a2": Implied, "a4": Skipped}
	p := PlanConfirmImplied(in)
	if len(p.Marks) != 2 || len(p.Events) != 2 {
		t.Fatalf("plan = %+v", p)
	}
	for _, e := range p.Events {
		if e.From != Implied || e.To != ManualDone || e.Cause != CauseUser {
			t.Fatalf("event = %+v", e)
		}
	}
	if got := PlanConfirmImplied(guideInput(g1)); len(got.Marks) != 0 {
		t.Fatalf("nothing implied, plan = %+v", got)
	}
}

func TestPlanPin(t *testing.T) {
	g1, _ := loadGuides(t)
	in := guideInput(g1)
	p, err := PlanPin(in, "b1")
	if err != nil || *p.Cursor != (CursorRecord{"b1", CursorPin}) {
		t.Fatalf("pin = %+v, %v", p, err)
	}
	in.Marks = map[string]Mark{"b1": {Kind: MarkManualDone}}
	if _, err = PlanPin(in, "b1"); !errors.Is(err, ErrStepDone) {
		t.Fatalf("pinning a done step: %v", err)
	}
	if _, err = PlanPin(in, "zz"); !errors.Is(err, ErrStepNotFound) {
		t.Fatalf("pinning a missing step: %v", err)
	}
	in.Cursor = CursorRecord{"b1", CursorPin}
	p, err = PlanPin(in, "")
	if err != nil || *p.Cursor != (CursorRecord{"a1", CursorAuto}) {
		t.Fatalf("unpin = %+v, %v", p, err)
	}
}

func TestBuildBoard(t *testing.T) {
	g1, _ := loadGuides(t)
	in := guideInput(g1)
	in.Verdicts = Verdicts{agility10: rules.Met, feathers: rules.Met}
	in.Marks = map[string]Mark{"a1": {Kind: MarkImplied, ImpliedBy: "a3"}, "a2": {Kind: MarkSkipped}, "a4": {Kind: MarkPinnedOpen}}
	b := BuildBoard(in, map[string]bool{feathers: true})
	// Main visible steps: a1..a6, b1, b2 (b0 is enhanced-only, t1/t2 are a track).
	want := Counts{Total: 8, Done: 4, AutoDone: 3, Implied: 1, Skipped: 1, Pinned: 1, Remaining: 3}
	if b.Counts != want {
		t.Fatalf("counts = %+v, want %+v", b.Counts, want)
	}
	if len(b.Steps) != 11 {
		t.Fatalf("steps = %d", len(b.Steps))
	}
	// Completing objectives of the visible main route: agility, feathers and
	// Cook's Assistant; two are met. Checkboxes: a1, a2, a4 and b1.
	if b.Coverage != (Coverage{Objectives: 3, Met: 2, Checkbox: 4, CheckboxDone: 1, CheckboxSkipped: 1}) {
		t.Fatalf("coverage = %+v", b.Coverage)
	}
	if len(b.Sections) != 3 || b.Sections[0].Counts.Total != 6 || b.Sections[1].Counts.Total != 2 ||
		!b.Sections[2].Track || b.Sections[2].Counts.Total != 2 {
		t.Fatalf("sections = %+v", b.Sections)
	}
	var a5 StepState
	for _, s := range b.Steps {
		if s.ID == "a5" {
			a5 = s
		}
	}
	if a5.Ref != "a5" || a5.Title != "buy 120 feathers with a tinderbox" || a5.Status != AutoDone || a5.Manual || len(a5.Objectives) != 2 || !a5.Objectives[0].Regressed || a5.Objectives[1].Verdict != rules.Unknown {
		t.Fatalf("a5 = %+v", a5)
	}
}

func TestReevaluateEvaluatesGoalsOutsideTheContent(t *testing.T) {
	g1, _ := loadGuides(t)
	plan := Reevaluate(Input{
		Guides:       []*content.Guide{g1},
		Registry:     registry(t),
		State:        rules.PlayerState{Skills: skills(map[string]int{"attack": 40})},
		Observations: obs(t0, "snap-1", Fact{"skill", "attack"}),
		Goals:        []string{"skill:attack>=40"},
		Account:      Account{ActiveGuideID: "g1"},
		Now:          t1,
	})
	var found bool
	for _, c := range plan.ObjectiveEvents {
		if c.Key == "skill:attack>=40" && c.To == rules.Met {
			found = true
		}
	}
	if !found {
		t.Fatalf("goal objective not evaluated: %+v", plan.ObjectiveEvents)
	}
	if plan.StepsChanged != 0 {
		t.Fatalf("a goal changed steps: %d", plan.StepsChanged)
	}
}

func TestPlanMarkBefore(t *testing.T) {
	g1, _ := loadGuides(t)
	in := guideInput(g1)
	in.Verdicts = Verdicts{agility10: rules.Met}
	in.Marks = map[string]Mark{"a2": {Kind: MarkSkipped}, "a4": {Kind: MarkImplied, ImpliedBy: "a6"}}
	in.LastStatus = map[string]Status{"a2": Skipped, "a3": AutoDone, "a4": Implied}
	// Before b1: a1 (pending), a2 (skipped: kept), a3 (auto_done: kept), a4
	// (implied: confirmed), a5, a6 (pending: marked), b0 (enhanced-only:
	// hidden for a classic account).
	p, undo, err := PlanMarkBefore(in, "b1")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range undo {
		prev := "-"
		if u.Mark != nil {
			prev = string(u.Mark.Kind) + "<" + u.Mark.ImpliedBy
		}
		got = append(got, u.StepID+":"+prev)
	}
	if fmt.Sprint(got) != "[a1:- a4:implied<a6 a5:- a6:-]" {
		t.Fatalf("undo = %v", got)
	}
	for _, e := range p.Events {
		if e.To != ManualDone || e.Cause != CauseUser {
			t.Fatalf("event = %+v", e)
		}
	}
	if p.Cursor == nil || *p.Cursor != (CursorRecord{"b1", CursorAuto}) {
		t.Fatalf("cursor = %+v", p.Cursor)
	}
	if _, _, err = PlanMarkBefore(in, "zz"); !errors.Is(err, ErrStepNotFound) {
		t.Fatalf("missing step: %v", err)
	}
	if p, undo, err = PlanMarkBefore(in, "a1"); err != nil || len(undo) != 0 || len(p.Marks) != 0 {
		t.Fatalf("nothing before a1 = %+v %v %v", p, undo, err)
	}

	// The undo restores the previous marks of steps still manual_done: a1
	// back to pending, a4 back to implied; a2 (skipped since) is left alone.
	for _, id := range []string{"a1", "a4"} {
		in.Marks[id] = Mark{Kind: MarkManualDone}
		in.LastStatus[id] = ManualDone
	}
	back := PlanRestore(in, []MarkChange{
		{StepID: "a1"}, {StepID: "a2"}, {StepID: "a4", Mark: &Mark{Kind: MarkImplied, ImpliedBy: "a6"}},
		{StepID: "a1", Mark: &Mark{Kind: MarkSkipped}}, {StepID: "zz"},
	})
	if len(back.Marks) != 2 || back.Marks[0].Mark != nil || back.Marks[1].Mark.Kind != MarkImplied {
		t.Fatalf("restore marks = %+v", back.Marks)
	}
	if len(back.Events) != 2 || back.Events[0].To != Pending || back.Events[1].To != Implied {
		t.Fatalf("restore events = %+v", back.Events)
	}
}
