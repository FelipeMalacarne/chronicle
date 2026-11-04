package migrate

import (
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
