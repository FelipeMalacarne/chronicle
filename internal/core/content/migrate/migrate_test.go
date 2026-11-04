package migrate

import (
	"bytes"
	"os"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

const normalizedPath = "../../../../data/guide/normalized/guide.json"

func TestConvertRealNormalizedGuide(t *testing.T) {
	n, err := ReadNormalized(normalizedPath)
	if err != nil {
		t.Skipf("normalized guide not available: %v", err)
	}
	g, err := Convert(n, Options{})
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
		t.Fatal("checked-in Oziris guide differs from deterministic migration; run `go run ./cmd/ironman content migrate`")
	}
	checkedReadme, err := os.ReadFile("../../../../content/guides/oziris-main/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := readmeFor(n, g, "data/guide/normalized/guide.json"); string(checkedReadme) != want {
		t.Fatal("checked-in Oziris README differs from deterministic migration")
	}
	parsed, err := content.ParseGuide("guide.json", data)
	if err != nil {
		t.Fatalf("output invalid against canonical schema: %v", err)
	}
	if parsed.StepCount() != 576 {
		t.Errorf("steps = %d, want 576", parsed.StepCount())
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
	if len(parsed.Segments()) != 32 || bankLegs != 25 {
		t.Fatalf("segments = %d, bank legs = %d; want 32, 25", len(parsed.Segments()), bankLegs)
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
