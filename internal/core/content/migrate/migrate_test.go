package migrate

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

const (
	normalizedPath = "../../../../data/guide/normalized/guide.json"
	patchesPath    = "../../../../data/guide/route-patches.json"
)

func TestConvertRealNormalizedGuide(t *testing.T) {
	n, err := ReadNormalized(normalizedPath)
	if err != nil {
		t.Skipf("normalized guide not available: %v", err)
	}
	patches, err := ReadRoutePatches(patchesPath)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Convert(n, Options{Patches: patches})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	data, err := Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	checkedIn, err := os.ReadFile("../../../../content/guides/oziris-main/guide.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, checkedIn) {
		t.Fatal("checked-in Oziris guide differs from deterministic migration; run `go run ./cmd/chronicle content migrate`")
	}
	checkedReadme, err := os.ReadFile("../../../../content/guides/oziris-main/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := readmeFor(n, g, "data/guide/normalized/guide.json", "data/guide/route-patches.json", patches); string(checkedReadme) != want {
		t.Fatal("checked-in Oziris README differs from deterministic migration")
	}
	parsed, err := content.ParseGuide("guide.json", data)
	if err != nil {
		t.Fatalf("output invalid against canonical schema: %v", err)
	}
	if parsed.StepCount() != 578 {
		t.Errorf("steps = %d, want 578 (576 imported, 1 dropped and 3 added by route patches)", parsed.StepCount())
	}
	if parsed.ID != "oziris-main" || parsed.SchemaVersion != 1 {
		t.Errorf("header = %q v%d", parsed.ID, parsed.SchemaVersion)
	}
	if parsed.DefaultVariant != "main" || len(parsed.Variants) != 1 {
		t.Fatalf("variants = default %q, count %d; want main, 1", parsed.DefaultVariant, len(parsed.Variants))
	}
	bankLegs := 0
	for _, segment := range parsed.Segments() {
		if segment.Kind == content.SegmentBankLeg {
			bankLegs++
		}
	}
	if len(parsed.Segments()) != 31 || bankLegs != 24 {
		t.Fatalf("segments = %d, bank legs = %d; want 31, 24", len(parsed.Segments()), bankLegs)
	}
	withRules := 0
	for _, st := range parsed.Steps() {
		if !st.IsManual() {
			withRules++
		}
		for _, r := range st.CompleteWhen {
			if r.RuleType() == content.TypeManual {
				t.Errorf("step %s: manual rule should have been dropped", st.ID)
			}
		}
	}
	if withRules != 209 {
		t.Errorf("steps with rules = %d, want 209", withRules)
	}
	st, _ := parsed.Step("1.5.5b")
	if len(st.Prerequisites) != 2 || len(st.Links) != 1 || st.Links[0].Type != "wiki" {
		t.Errorf("1.5.5b = %+v", st)
	}
	if st.Layers[0] != "enhanced" || len(st.Layers) != 1 {
		t.Errorf("1.5.5b layers = %v", st.Layers)
	}
	sail, _ := parsed.Section("S")
	if !sail.IsTrack() {
		t.Error("section S should be kind track")
	}
	bank, ok := parsed.Step("1.1.131")
	if !ok {
		t.Fatal("representative bank step 1.1.131 missing")
	}
	segment, ok := parsed.Segment(bank.SegmentID)
	if !ok || segment.Kind != content.SegmentBankLeg {
		t.Fatalf("1.1.131 segment = %#v, %v", segment, ok)
	}
	if segment.Loadout == nil || segment.Loadout.Status != content.LoadoutPartial || len(segment.Loadout.Inventory) != 5 || len(segment.Loadout.Equipment) != 1 {
		t.Fatalf("1.1.131 loadout = %#v", segment.Loadout)
	}
	if got := segment.Loadout.Inventory[4]; got.Name != "cakes" || got.Qty == nil || *got.Qty != 18 || got.SourceText != "18 cakes" {
		t.Errorf("1.1.131 cake requirement = %#v", got)
	}
	if got := segment.Loadout.Equipment[0]; got.Name != "Gloves" || got.SourceText != "Gloves (equip)" {
		t.Errorf("1.1.131 equipment = %#v", got)
	}
	if source := segment.Loadout.Source; source == nil || source.Method != content.ProvenanceExplicit || len(source.StepIDs) != 1 || source.StepIDs[0] != bank.ID {
		t.Errorf("1.1.131 loadout provenance = %#v", source)
	}
}

func TestConvertTags(t *testing.T) {
	got := convertTags([]string{"boss:The Corrupted Gauntlet", "new-quest", "quest", "quest", "phase:late", "boss:Calvar'ion"})
	want := []string{"boss:the-corrupted-gauntlet", "new-content", "quest", "phase:late", "boss:calvarion"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tag %d = %q want %q", i, got[i], want[i])
		}
	}
}

func TestConvertRuleQuestOp(t *testing.T) {
	r, err := convertRule(NRule{Type: "quest_state", Quest: "Sea Slug", State: "IN_PROGRESS", OrFinished: true, Source: "questStatus", Confidence: "high"})
	if err != nil {
		t.Fatal(err)
	}
	q := r.(content.QuestStateRule)
	if q.Op != "gte" || q.Meta == nil || q.Meta.Origin != "questStatus" {
		t.Errorf("got %#v", q)
	}
	r, _ = convertRule(NRule{Type: "quest_state", Quest: "Sea Slug", State: "FINISHED"})
	if q := r.(content.QuestStateRule); q.Op != "" || q.Meta != nil {
		t.Errorf("FINISHED without meta: %#v", q)
	}
	if _, err := convertRule(NRule{Type: "combat_achievement"}); err == nil {
		t.Error("unsupported type should error")
	}
}

func TestApplyPatches(t *testing.T) {
	section := func() []*content.Section {
		return []*content.Section{{ID: "1", Steps: []*content.Step{
			{ID: "a", Text: "A", Notes: []content.Note{{Kind: content.NoteUpdate, Text: "old"}}},
			{ID: "b", Text: "B"},
			{ID: "c", Text: "C", Requires: []string{"step:a"}},
		}}}
	}
	ids := func(secs []*content.Section) string {
		var out []string
		for _, st := range secs[0].Steps {
			out = append(out, st.ID)
		}
		return strings.Join(out, ",")
	}
	patch := func(ops ...PatchOp) *RoutePatches {
		return &RoutePatches{SchemaVersion: 1, GuideID: "g", Patches: []RoutePatch{{ID: "p", Reason: "r", Ops: ops}}}
	}

	secs := section()
	err := applyPatches(secs, patch(
		PatchOp{Op: OpMove, Step: "a", After: "b"},
		PatchOp{Op: OpInsert, After: "a", New: &NewStep{ID: "a1", Text: "New", Items: []string{"Rope"}}},
		PatchOp{Op: OpDrop, Step: "b"},
		PatchOp{Op: OpNote, Step: "a", Note: "new"},
		PatchOp{Op: OpItems, Step: "c", Items: &[]string{"Bucket"}},
	), "g", "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(secs); got != "a,a1,c" {
		t.Errorf("order = %s, want a,a1,c", got)
	}
	added := secs[0].Steps[1]
	if added.Source == nil || added.Source.Origin != "own" || len(added.Items) != 1 || len(added.Modes) == 0 {
		t.Errorf("added step = %+v", added)
	}
	if secs[0].Steps[0].Notes[0].Text != "new" || secs[0].Steps[2].Items[0].Name != "Bucket" {
		t.Errorf("note/items not replaced: %+v %+v", secs[0].Steps[0], secs[0].Steps[2])
	}

	for name, p := range map[string]*RoutePatches{
		"stale step":       patch(PatchOp{Op: OpMove, Step: "zz", After: "a"}),
		"required step":    patch(PatchOp{Op: OpDrop, Step: "a"}),
		"duplicate id":     patch(PatchOp{Op: OpInsert, After: "a", New: &NewStep{ID: "b", Text: "x"}}),
		"no update note":   patch(PatchOp{Op: OpNote, Step: "b", Note: "x"}),
		"unknown op":       patch(PatchOp{Op: "rename", Step: "a"}),
		"other guide":      {SchemaVersion: 1, GuideID: "h"},
		"missing a reason": {SchemaVersion: 1, GuideID: "g", Patches: []RoutePatch{{ID: "p"}}},
	} {
		if err := applyPatches(section(), p, "g", ""); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
