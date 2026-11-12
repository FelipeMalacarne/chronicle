package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// objectiveGuide has one bank leg started by step b, so annotations of b can
// replace the derived loadout and bank actions.
const objectiveGuide = `{"schema_version":1,"id":"t","title":"T","default_variant":"main","variants":[{"id":"main","title":"Main","sections":[
 {"id":"1.1","slug":"s","title":"S","segments":[
  {"id":"seg-a","kind":"sequence","source":{"method":"derived","confidence":"high","step_ids":["a"]},"loadout":{"status":"unknown"},"steps":[
   {"id":"a","text":"Buy a spade, start X marks the spot quest","location":"Lumbridge",
    "complete_when":[{"type":"quest_state","meta":{"origin":"questStatus","confidence":"high"},"quest":"X Marks the Spot","state":"IN_PROGRESS","op":"gte"}]}
  ]},
  {"id":"seg-b","kind":"bank_leg","source":{"method":"derived","confidence":"high","step_ids":["b"]},
   "bank_actions":[{"type":"withdraw","position":"start","text":"withdraw","source":{"method":"derived","confidence":"high","step_ids":["b"]}}],
   "loadout":{"status":"unknown"},"steps":[
   {"id":"b","text":"Bank, withdraw an axe and 2 prayer potions, then chop until 30 woodcutting",
    "complete_when":[{"type":"skill_level","meta":{"origin":"text","inferred":true,"confidence":"medium","match":"30 woodcutting"},"skill":"woodcutting","level":30}],
    "prerequisites":[{"type":"quest_state","quest":"X Marks the Spot","state":"FINISHED"}]},
   {"id":"c","text":"Withdraw a hammer at the deposit box"}
  ]}
 ]},
 {"id":"1.2","slug":"t","title":"T2","segments":[
  {"id":"seg-c","kind":"sequence","source":{"method":"derived","confidence":"high","step_ids":["d"]},"steps":[{"id":"d","text":"Do Cook's Assistant"}]}
 ]}
]}]}`

func TestRuleObjectives(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "t", objectiveGuide)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.StepInGuide("t", "a")
	b, _ := c.StepInGuide("t", "b")
	want := map[string][]string{
		"a": {"quest:x_marks_the_spot@started starts explicit /complete_when/0"},
		"b": {"skill:woodcutting>=30 completes derived /complete_when/0", "quest:x_marks_the_spot@finished requires explicit /prerequisites/0"},
	}
	for id, st := range map[string]*Step{"a": a, "b": b} {
		var got []string
		for _, o := range st.Objectives {
			got = append(got, o.Key.String()+" "+o.Role+" "+o.Source.Method+" "+o.Evidence.Field)
		}
		if strings.Join(got, "|") != strings.Join(want[id], "|") {
			t.Errorf("step %s objectives = %v, want %v", id, got, want[id])
		}
		if st.IsManual() {
			t.Errorf("step %s has completing objectives but is manual", id)
		}
		if !strings.HasPrefix(st.TextHash, "sha256:") {
			t.Errorf("step %s text hash = %q", id, st.TextHash)
		}
	}
	if b.Objectives[0].Source.Confidence != ConfidenceMedium {
		t.Errorf("inferred rule confidence = %q", b.Objectives[0].Source.Confidence)
	}
}

func TestRuleObjectivesRejectUnexpressible(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`[{"type":"quest_state","quest":"X Marks the Spot","state":"NOT_STARTED"}]`:           "has no objective target",
		`[{"type":"quest_state","quest":"X Marks the Spot","state":"IN_PROGRESS","op":"eq"}]`: "op",
		`[{"type":"quest_state","quest":"Not A Real Quest","state":"FINISHED"}]`:              "does not resolve",
		`[{"any":[{"type":"skill_level","skill":"agility","level":10}]}]`:                     "has no objective target",
		`[{"type":"varbit","id":1,"eq":1}]`:                                                   "has no objective target",
		`[{"type":"item_owned","item_id":952,"containers":["bank"]}]`:                         "containers",
		`[{"type":"skill_level","skill":"agility","level":10,"boosted":true}]`:                "boosted",
	}
	for rules, want := range cases {
		var st Step
		if err := json.Unmarshal([]byte(`{"id":"x","text":"x","complete_when":`+rules+`}`), &st); err != nil {
			t.Fatal(err)
		}
		objs, issues := RuleObjectives(&st, reg)
		if len(objs) != 0 || len(issues) != 1 || !strings.Contains(issues[0].Message, want) {
			t.Errorf("%s: objectives %v issues %v, want one issue containing %q", rules, objs, issues, want)
		}
	}
	var st Step
	_ = json.Unmarshal([]byte(`{"id":"x","text":"x","complete_when":[{"type":"manual"},{"type":"diary_tier","area":"all","tier":"easy"},{"type":"item_owned","item_id":952}]}`), &st)
	objs, issues := RuleObjectives(&st, reg)
	if len(issues) != 0 || len(objs) != 13 || objs[12].Key.String() != "item:952x1" {
		t.Errorf("manual skipped, all diaries expanded, item qty explicit: got %d objectives %v", len(objs), issues)
	}
}

// annotation writes annotations/<section>.json for guide t in dir.
func annotation(t *testing.T, dir, section, body string) {
	t.Helper()
	p := filepath.Join(dir, "guides", "t", AnnotationsSubdir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, section), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := c.Guide("t")
	out := map[string]string{}
	for _, st := range g.Steps() {
		out[st.ID] = st.TextHash
	}
	return out
}

const llm = `{"method":"llm","confidence":"high","confirmed_by":"owner"}`

func TestAnnotationsCompose(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "t", objectiveGuide)
	h := hashes(t, dir)
	annotation(t, dir, "1.1.json", `{"schema_version":1,"kind":"section","guide_id":"t","section_id":"1.1","skill_version":"annotate-guide@1","steps":[
 {"step_id":"a","text_hash":"`+h["a"]+`",
  "objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"buy a spade","span":[0,11]},"source":`+llm+`},
                {"key":"quest:x_marks_the_spot@started","role":"starts","evidence":{"field":"/text","quote":"start X marks the spot quest","span":[13,41]},"source":`+llm+`}]},
 {"step_id":"b","text_hash":"`+h["b"]+`",
  "objectives":[{"key":"item:any(1349x1|1351x1)","role":"uses","evidence":{"field":"/text","quote":"an axe","span":[15,21]},"source":`+llm+`},
                {"key":"item:family/prayer_potionx2","role":"uses","evidence":{"field":"/text","quote":"2 prayer potions","span":[26,42]},"source":`+llm+`}],
  "items":[{"name":"Axe","any_of":[1351,1349],"evidence":{"field":"/text","quote":"an axe","span":[15,21]},"source":`+llm+`},
           {"name":"Prayer potion","family":"prayer_potion","qty":2,"evidence":{"field":"/text","quote":"2 prayer potions","span":[26,42]},"source":`+llm+`}],
  "bank_actions":[{"type":"withdraw","position":"start","evidence":{"field":"/text","quote":"withdraw an axe","span":[6,21]},"source":`+llm+`}]},
 {"step_id":"c","text_hash":"`+h["c"]+`",
  "bank_actions":[{"type":"withdraw","position":"within","evidence":{"field":"/text","quote":"Withdraw a hammer","span":[0,17]},"source":`+llm+`}]}
]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Warnings) != 0 {
		t.Fatalf("warnings: %v", c.Warnings)
	}
	a, _ := c.StepInGuide("t", "a")
	var keys []string
	for _, o := range a.Objectives {
		keys = append(keys, o.Key.String()+" "+o.Role+" "+o.Source.Method)
	}
	// The annotation replaces the rule objective with the same key and role.
	if got := strings.Join(keys, "|"); got != "quest:x_marks_the_spot@started starts llm|item:952x1 uses llm" {
		t.Errorf("a objectives = %s", got)
	}
	if a.Objectives[1].Source.SkillVersion != "annotate-guide@1" {
		t.Errorf("skill version not stamped: %+v", a.Objectives[1].Source)
	}
	b, _ := c.StepInGuide("t", "b")
	keys = keys[:0]
	for _, o := range b.Objectives {
		keys = append(keys, o.Key.String())
	}
	if got := strings.Join(keys, "|"); !strings.HasSuffix(got, "|item:any(1349x1|1351x1)|item:family/prayer_potionx2") {
		t.Errorf("b objectives = %s", got)
	}
	g, _ := c.Guide("t")
	seg, _ := g.Segment("seg-b")
	if seg.Loadout.Status != LoadoutPartial || len(seg.Loadout.Inventory) != 2 || seg.Loadout.Inventory[0].Variant() != RequireAnyOf ||
		seg.Loadout.Inventory[1].Variant() != RequireFamily || seg.Loadout.Source.Method != ProvenanceLLM {
		t.Errorf("seg-b loadout = %+v", seg.Loadout)
	}
	if len(seg.BankActions) != 1 || seg.BankActions[0].Text != "withdraw an axe" {
		t.Errorf("seg-b bank actions = %+v", seg.BankActions)
	}
	cStep, _ := c.StepInGuide("t", "c")
	if len(cStep.BankActions) != 1 || len(seg.BankActions) != 1 {
		t.Errorf("a bank action of a step that does not start its segment stays on the step: %+v", cStep.BankActions)
	}
	if c.Annotations["t"] == nil || len(c.Annotations["t"].Sections) != 1 {
		t.Errorf("annotations not recorded: %+v", c.Annotations)
	}
}

func TestAnnotationsStaleAreWarnings(t *testing.T) {
	dir := t.TempDir()
	writeGuide(t, dir, "t", objectiveGuide)
	annotation(t, dir, "1.1.json", `{"schema_version":1,"kind":"section","guide_id":"t","section_id":"1.1","skill_version":"annotate-guide@1","steps":[
 {"step_id":"a","text_hash":"sha256:`+strings.Repeat("0", 64)+`",
  "objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"buy a spade","span":[0,11]},"source":`+llm+`}]}]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0].Message, "stale") {
		t.Fatalf("warnings = %v", c.Warnings)
	}
	a, _ := c.StepInGuide("t", "a")
	for _, o := range a.Objectives {
		if o.Key.String() == "item:952x1" {
			t.Error("stale annotation was composed")
		}
	}
}

func TestAnnotationsRejected(t *testing.T) {
	cases := []struct {
		name, file, step, body, want string
	}{
		{"quote not in text", "1.1.json", "a", `"objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"buy a rope","span":[0,10]},"source":` + llm + `}]`, "is not in /text"},
		{"span off", "1.1.json", "a", `"objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"a spade","span":[0,7]},"source":` + llm + `}]`, "it is at [4, 11]"},
		{"unknown quest", "1.1.json", "a", `"objectives":[{"key":"quest:not_a_quest@finished","role":"completes","evidence":{"field":"/text","quote":"start X marks","span":[13,26]},"source":` + llm + `}]`, "unknown entity quest:not_a_quest"},
		{"role mismatch", "1.1.json", "a", `"objectives":[{"key":"quest:x_marks_the_spot@finished","role":"uses","evidence":{"field":"/text","quote":"start X marks","span":[13,26]},"source":` + llm + `}]`, "role uses needs an item key"},
		{"unknown location", "1.1.json", "a", `"location":{"ref":"location:not_a_place","evidence":{"field":"/location","quote":"Lumbridge","span":[0,9]},"source":` + llm + `}`, "unknown entity location:not_a_place"},
		{"unknown family", "1.1.json", "b", `"items":[{"name":"x","family":"not_a_family","evidence":{"field":"/text","quote":"an axe","span":[15,21]},"source":` + llm + `}]`, "unknown item family"},
		{"unknown family key", "1.1.json", "b", `"objectives":[{"key":"item:family/not_a_familyx2","role":"uses","evidence":{"field":"/text","quote":"2 prayer potions","span":[26,42]},"source":` + llm + `}]`, "unknown item family not_a_family"},
		{"unknown alternative", "1.1.json", "b", `"objectives":[{"key":"item:any(1351x1|999999x1)","role":"uses","evidence":{"field":"/text","quote":"an axe","span":[15,21]},"source":` + llm + `}]`, "unknown entity item:999999"},
		{"alternatives out of order", "1.1.json", "b", `"objectives":[{"key":"item:any(1351x1|1349x1)","role":"uses","evidence":{"field":"/text","quote":"an axe","span":[15,21]},"source":` + llm + `}]`, "ascending"},
		{"step of another section", "1.2.json", "a", `"objectives":[]`, "belongs to section"},
		{"unconfirmed inference", "1.1.json", "a", `"objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"buy a spade","span":[0,11]},"source":{"method":"llm","confidence":"medium"}}]`, "needs confirmed_by owner"},
		{"unknown decision", "1.1.json", "a", `"objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"buy a spade","span":[0,11]},"source":{"method":"llm","confidence":"medium","confirmed_by":"owner","decision":"spade-rule"}}]`, "unknown decision \"spade-rule\""},
		{"rule source in sidecar", "1.1.json", "a", `"objectives":[{"key":"item:952x1","role":"uses","evidence":{"field":"/text","quote":"buy a spade","span":[0,11]},"source":{"method":"explicit","confidence":"high"}}]`, "value must be one of"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeGuide(t, dir, "t", objectiveGuide)
			h := hashes(t, dir)
			section := strings.TrimSuffix(tc.file, ".json")
			annotation(t, dir, tc.file, `{"schema_version":1,"kind":"section","guide_id":"t","section_id":"`+section+`","skill_version":"annotate-guide@1","steps":[
 {"step_id":"`+tc.step+`","text_hash":"`+h[tc.step]+`",`+tc.body+`}]}`)
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want %q", err, tc.want)
			}
		})
	}
}
