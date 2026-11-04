package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const realContentDir = "../../../content"

func TestLoadRealContent(t *testing.T) {
	c, err := Load(realContentDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Guides) != 2 {
		t.Fatalf("guides = %d, want 2", len(c.Guides))
	}
	g, ok := c.Guide("oziris-main")
	if !ok {
		t.Fatal("oziris-main guide not loaded")
	}
	if got := g.StepCount(); got != 576 {
		t.Errorf("steps = %d, want 576", got)
	}
	bruh, ok := c.Guide("bruhsailer-main")
	if !ok || bruh.StepCount() != 226 {
		t.Fatalf("bruhsailer-main = %#v, %v; want 226 steps", bruh, ok)
	}
	if got := c.StepCount(); got != 802 {
		t.Errorf("indexed steps = %d, want 802", got)
	}
	for _, guide := range c.Guides {
		seen := map[string]bool{}
		for _, st := range guide.Steps() {
			if seen[st.ID] {
				t.Errorf("guide %s: duplicate step id %q", guide.ID, st.ID)
			}
			seen[st.ID] = true
			if st.SectionID == "" || st.VariantID == "" || st.SegmentID == "" || st.GuideID != guide.ID {
				t.Errorf("step %s: back references not set (variant=%q section=%q segment=%q guide=%q)",
					st.ID, st.VariantID, st.SectionID, st.SegmentID, st.GuideID)
			}
		}
	}
	st, ok := c.Step("1.1.258a")
	if !ok {
		t.Fatal("step 1.1.258a not found")
	}
	if len(st.CompleteWhen) != 2 {
		t.Fatalf("1.1.258a complete_when = %d rules, want 2", len(st.CompleteWhen))
	}
	if _, ok := st.CompleteWhen[0].(QuestStateRule); !ok {
		t.Errorf("1.1.258a rule 0 = %T, want QuestStateRule", st.CompleteWhen[0])
	}
	if sl, ok := st.CompleteWhen[1].(SkillLevelRule); !ok || sl.Skill != "runecraft" || sl.Level != 10 {
		t.Errorf("1.1.258a rule 1 = %#v", st.CompleteWhen[1])
	}
	if _, sec, ok := c.Section("early-game"); !ok || sec.ID != "1.1" {
		t.Errorf("section by slug: %v %v", sec, ok)
	}
	if _, sec, ok := c.Section("S"); !ok || !sec.IsTrack() {
		t.Errorf("section S should be a track")
	}
	if q := g.QuestNames(); len(q) == 0 || q[0] > q[len(q)-1] {
		t.Errorf("quest names not built/sorted: %v", q)
	}
}

func writeGuide(t *testing.T, dir, id, body string) string {
	t.Helper()
	p := filepath.Join(dir, "guides", id)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(p, "guide.json")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

const minimalGuide = `{"schema_version":1,"id":"t","title":"T","sections":[{"id":"1","slug":"one","title":"One","steps":[
 {"id":"a","text":"A","complete_when":[{"type":"skill_level","skill":"agility","level":10}]},
 {"id":"b","text":"B","requires":["step:a"],"items":["Spade",{"name":"Rope","qty":2}]}
]}]}`

func TestLoadMinimalAndItems(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "t", minimalGuide)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, _ := c.Step("b")
	if len(b.Items) != 2 || b.Items[0].Name != "Spade" || b.Items[1].Qty == nil || *b.Items[1].Qty != 2 {
		t.Errorf("items = %#v", b.Items)
	}
	out, err := json.Marshal(b.Items)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `["Spade",{"name":"Rope","qty":2}]` {
		t.Errorf("items round-trip = %s", out)
	}
}

func TestLoadMultipleGuidesWithCollidingIDs(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "z-alpha", `{"schema_version":1,"id":"alpha","title":"Alpha","sections":[{"id":"shared-section","slug":"shared-slug","title":"Alpha section","steps":[
 {"id":"shared-step","text":"Alpha shared"},{"id":"alpha-only","text":"Alpha only"}
]}]}`)
	writeGuide(t, dir, "a-beta", `{"schema_version":1,"id":"beta","title":"Beta","sections":[{"id":"shared-section","slug":"shared-slug","title":"Beta section","steps":[
 {"id":"shared-step","text":"Beta shared"},{"id":"beta-only","text":"Beta only"}
]}]}`)

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.StepCount(); got != 4 {
		t.Fatalf("StepCount = %d, want 4", got)
	}

	alphaStep, ok := c.StepInGuide("alpha", "shared-step")
	if !ok || alphaStep.Text != "Alpha shared" || alphaStep.GuideID != "alpha" {
		t.Fatalf("alpha shared step = %#v, %v", alphaStep, ok)
	}
	betaStep, ok := c.StepInGuide("beta", "shared-step")
	if !ok || betaStep.Text != "Beta shared" || betaStep.GuideID != "beta" {
		t.Fatalf("beta shared step = %#v, %v", betaStep, ok)
	}
	alphaSection, ok := c.SectionInGuide("alpha", "shared-section")
	if !ok || alphaSection.Title != "Alpha section" {
		t.Fatalf("alpha shared section = %#v, %v", alphaSection, ok)
	}
	betaSection, ok := c.SectionInGuide("beta", "shared-slug")
	if !ok || betaSection.Title != "Beta section" {
		t.Fatalf("beta shared section = %#v, %v", betaSection, ok)
	}

	if st, ok := c.Step("shared-step"); ok || st != nil {
		t.Errorf("ambiguous legacy Step = %#v, %v; want nil, false", st, ok)
	}
	if g, sec, ok := c.Section("shared-section"); ok || g != nil || sec != nil {
		t.Errorf("ambiguous legacy Section = %#v, %#v, %v; want nil, nil, false", g, sec, ok)
	}
	if st, ok := c.Step("alpha-only"); !ok || st.Text != "Alpha only" || st.GuideID != "alpha" {
		t.Errorf("unambiguous legacy Step = %#v, %v", st, ok)
	}

	if g, ok := c.SelectGuide("alpha"); !ok || g.ID != "alpha" {
		t.Errorf("SelectGuide(alpha) = %#v, %v", g, ok)
	}
	if g, ok := c.SelectGuide(""); ok || g != nil {
		t.Errorf("SelectGuide(empty) with multiple guides = %#v, %v; want nil, false", g, ok)
	}
}

func TestSelectGuideSingleGuideCompatibility(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "t", minimalGuide)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	g, ok := c.SelectGuide("")
	if !ok || g.ID != "t" {
		t.Fatalf("SelectGuide(empty) = %#v, %v", g, ok)
	}
}

func TestLoadReportsPointer(t *testing.T) {
	dir := t.TempDir()
	f := writeGuide(t, dir, "bad", `{"schema_version":1,"id":"bad","title":"B","sections":[{"id":"1","slug":"one","title":"One","steps":[
 {"id":"a","text":"A","complete_when":[{"type":"skill_level","skill":"agility","level":"ten"}]}
]}]}`)
	_, err := Load(dir)
	issues, ok := AsErrors(err)
	if !ok {
		t.Fatalf("want Errors, got %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %v, want exactly one (best oneOf branch)", issues)
	}
	is := issues[0]
	if is.File != f || is.Pointer != "/sections/0/steps/0/complete_when/0/level" {
		t.Errorf("issue = %+v", is)
	}
}

func TestLoadSemanticChecks(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "dup", `{"schema_version":1,"id":"dup","title":"D","sections":[{"id":"1","slug":"one","title":"One","steps":[
 {"id":"a","text":"A"},{"id":"a","text":"A again","requires":["step:zzz"]}
]}]}`)
	_, err := Load(dir)
	issues, ok := AsErrors(err)
	if !ok {
		t.Fatalf("want Errors, got %v", err)
	}
	var msgs []string
	for _, is := range issues {
		msgs = append(msgs, is.Pointer+": "+is.Message)
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, `/sections/0/steps/1/id: duplicate step id "a"`) {
		t.Errorf("missing duplicate id issue:\n%s", joined)
	}
	if !strings.Contains(joined, `/sections/0/steps/1/requires/0: unknown step "zzz"`) {
		t.Errorf("missing unknown step issue:\n%s", joined)
	}
}

func TestDecodeRuleUnion(t *testing.T) {
	src := `{"any":[{"type":"quest_state","quest":"Q","state":"FINISHED","op":"eq"},{"not":{"type":"manual","label":"x"}}]}`
	r, err := DecodeRule([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	anyR, ok := r.(AnyRule)
	if !ok || len(anyR.Any) != 2 {
		t.Fatalf("got %#v", r)
	}
	if q, isQ := anyR.Any[0].(QuestStateRule); !isQ || q.Op != "eq" {
		t.Errorf("child 0 = %#v", anyR.Any[0])
	}
	n, isNot := anyR.Any[1].(NotRule)
	if !isNot {
		t.Fatalf("child 1 = %#v", anyR.Any[1])
	}
	if m, isM := n.Not.(ManualRule); !isM || m.Label != "x" {
		t.Errorf("not child = %#v", n.Not)
	}
	if _, badErr := DecodeRule([]byte(`{"type":"teleport"}`)); badErr == nil {
		t.Error("unknown type should fail")
	}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeRule(out)
	if err != nil {
		t.Fatalf("re-decode %s: %v", out, err)
	}
	if back.RuleType() != TypeAny {
		t.Errorf("round-trip type = %s", back.RuleType())
	}
}
