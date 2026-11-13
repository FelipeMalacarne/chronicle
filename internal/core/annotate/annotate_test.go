package annotate

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/content/reference"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// testGuide: section 1.1 has a sequence (a, a2) and a bank leg started by b;
// section 1.2 comes after it. Step c quotes Cook’s Assistant with a curly
// apostrophe.
const testGuide = `{"schema_version":1,"id":"t","title":"T","default_variant":"main","variants":[{"id":"main","title":"Main","sections":[
 {"id":"1.1","slug":"s","title":"S","segments":[
  {"id":"seg-a","kind":"sequence","source":{"method":"derived","confidence":"high","step_ids":["a"]},"steps":[
   {"id":"a","text":"Buy a spade, start X marks the spot quest","location":"Lumbridge",
    "complete_when":[{"type":"quest_state","meta":{"origin":"questStatus","confidence":"high"},"quest":"X Marks the Spot","state":"IN_PROGRESS","op":"gte"}]},
   {"id":"a2","text":"Finish X marks the spot and talk to Veos"}
  ]},
  {"id":"seg-b","kind":"bank_leg","source":{"method":"derived","confidence":"high","step_ids":["b"]},"loadout":{"status":"unknown"},
   "bank_actions":[{"type":"open_bank","position":"start","text":"Bank","source":{"method":"derived","confidence":"high","step_ids":["b"]}}],"steps":[
   {"id":"b","text":"Bank everything, withdraw an axe and 2 prayer potions, then chop until 30 woodcutting",
    "complete_when":[{"type":"skill_level","meta":{"origin":"text","inferred":true,"confidence":"medium","match":"30 woodcutting"},"skill":"woodcutting","level":30}]},
   {"id":"c","text":"Do the Cook’s Assistant quest in Lumbridge"}
  ]}
 ]},
 {"id":"1.2","slug":"t","title":"T2","segments":[
  {"id":"seg-c","kind":"sequence","source":{"method":"derived","confidence":"high","step_ids":["d"]},"steps":[{"id":"d","text":"Do Cook's Assistant, then start X marks the spot again"}]}
 ]}
]}]}`

// workspace writes the test guide and a copy of the real catalogs to a
// temporary content dir and opens it.
func workspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	dir := t.TempDir()
	ref := filepath.Join(dir, ReferenceSubdir)
	if err := os.MkdirAll(ref, 0o755); err != nil {
		t.Fatal(err)
	}
	err := fs.WalkDir(reference.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(reference.FS, p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(ref, p), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	// The extraction fills unlocks and locations over time; the tests start
	// from empty catalogs so they do not depend on what was proposed so far.
	for _, name := range []string{"unlocks", "locations"} {
		empty := `{"catalog": "` + name + `", "version": "test", "` + name + `": []}`
		if err = os.WriteFile(filepath.Join(ref, name+".json"), []byte(empty), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gdir := filepath.Join(dir, content.GuidesSubdir, "t")
	if err = os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(gdir, content.GuideFileName), []byte(testGuide), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Today = func() string { return "2026-11-04" }
	return w, dir
}

func objective(role, kind, name, state string, n int, quote string, src Source) ObjectiveProposal {
	return ObjectiveProposal{Role: role, Kind: kind, Name: name, State: state, N: n, Evidence: Evidence{Quote: quote}, Source: src}
}

var hi = Source{Confidence: content.ConfidenceHigh}

func TestObjectiveKey(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		p       ObjectiveProposal
		want    string
		wantErr string
	}{
		{ObjectiveProposal{Kind: "quest", Name: "X Marks the Spot", State: "started"}, "quest:x_marks_the_spot@started", ""},
		{ObjectiveProposal{Kind: "quest", Name: "Cook’s Assistant", State: "finished"}, "quest:cooks_assistant@finished", ""},
		{ObjectiveProposal{Kind: "quest", Name: "Cook's Assistant", State: "done"}, "", "state started or finished"},
		{ObjectiveProposal{Kind: "quest", Name: "Not A Quest", State: "finished"}, "", "does not resolve"},
		{ObjectiveProposal{Kind: "skill", Name: "Woodcutting", N: 30}, "skill:woodcutting>=30", ""},
		{ObjectiveProposal{Kind: "skill", Name: "Woodcutting", N: 120}, "", ""},
		{ObjectiveProposal{Kind: "diary", Name: "Lumbridge", Tier: "easy"}, "diary:lumbridge_and_draynor:easy@complete", ""},
		{ObjectiveProposal{Kind: "qp", N: 32}, "qp>=32", ""},
		{ObjectiveProposal{Kind: "boss", Name: "Obor", N: 1}, "boss:obor>=1", ""},
		{ObjectiveProposal{Kind: "item", Name: "spade", N: 1}, "item:952x1", ""},
		{ObjectiveProposal{Kind: "item", Name: "axe", N: 1}, "", "ambiguous"},
		{ObjectiveProposal{Kind: "item", Name: "axe", ID: "1359", N: 1}, "item:1359x1", ""},
		{ObjectiveProposal{Kind: "item", Name: "axe", ID: "item:1359", N: 1}, "item:1359x1", ""},
		{ObjectiveProposal{Kind: "item", Name: "spade", ID: "item:952", N: 1}, "item:952x1", ""},
		{ObjectiveProposal{Kind: "item", Name: "axe", ID: "952", N: 1}, "", "not a candidate"},
		{ObjectiveProposal{Kind: "item", Name: "spade", ID: "1359", N: 1}, "", "resolves to item:952"},
		{ObjectiveProposal{Kind: "item", Name: "noted spade", N: 1}, "", "noted"},
		{ObjectiveProposal{Kind: "item", Family: "Graceful hood", N: 1}, "item:family/graceful_hoodx1", ""},
		{ObjectiveProposal{Kind: "item", Family: "prayer_potion", N: 4}, "item:family/prayer_potionx4", ""},
		{ObjectiveProposal{Kind: "item", Family: "not a family", N: 1}, "", "unknown item family"},
		{ObjectiveProposal{Kind: "item", Family: "Graceful hood"}, "", "n >= 1"},
		{ObjectiveProposal{Kind: "item", Name: "Graceful hood", Family: "Graceful hood", N: 1}, "", "no name or id"},
		{ObjectiveProposal{Kind: "skill", Family: "Graceful hood", N: 1}, "", "for item objectives"},
		{ObjectiveProposal{Kind: "item", AnyOf: []AlternativeProposal{{Name: "Rune axe", N: 1}, {Name: "Mithril axe", N: 2}, {Name: "Adamant axe", N: 1}}},
			"item:any(1355x2|1357x1|1359x1)", ""},
		{ObjectiveProposal{Kind: "item", AnyOf: []AlternativeProposal{{Name: "axe", ID: "1359", N: 1}, {Name: "Mithril axe", N: 2}}}, "item:any(1355x2|1359x1)", ""},
		{ObjectiveProposal{Kind: "item", AnyOf: []AlternativeProposal{{Name: "axe", N: 1}, {Name: "Mithril axe", N: 2}}}, "", "ambiguous"},
		{ObjectiveProposal{Kind: "item", AnyOf: []AlternativeProposal{{Name: "Mithril axe", N: 1}, {Name: "Mithril axe", N: 2}}}, "", "each id once"},
		{ObjectiveProposal{Kind: "item", AnyOf: []AlternativeProposal{{Name: "Mithril axe", N: 2}}}, "", "at least two"},
		{ObjectiveProposal{Kind: "item", AnyOf: []AlternativeProposal{{Name: "Mithril axe", N: 2}, {Name: "Rune axe"}}}, "", ">= 1"},
		{ObjectiveProposal{Kind: "item", N: 1, AnyOf: []AlternativeProposal{{Name: "Mithril axe", N: 2}, {Name: "Rune axe", N: 1}}}, "", "own n"},
		{ObjectiveProposal{Kind: "item", Family: "Graceful hood", N: 1, AnyOf: []AlternativeProposal{{Name: "Mithril axe", N: 2}, {Name: "Rune axe", N: 1}}}, "", "only one"},
		{ObjectiveProposal{Kind: "unlock", Name: "Not an unlock"}, "", "propose_entity"},
		{ObjectiveProposal{Kind: "planet", Name: "x"}, "", "unknown kind"},
	}
	for _, c := range cases {
		k, err := objectiveKey(reg, c.p)
		if c.want == "" && c.wantErr == "" {
			if err == nil {
				t.Errorf("%+v: want an error, got %s", c.p, k)
			}
			continue
		}
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%+v: err %v, want %q", c.p, err, c.wantErr)
			}
			continue
		}
		if err != nil || k.String() != c.want {
			t.Errorf("%+v: got %s, %v; want %s", c.p, k, err, c.want)
		}
	}
}

func TestItemRequirement(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		p       ItemProposal
		want    string
		wantErr string
	}{
		{ItemProposal{Name: "spade", Qty: 1}, "spade = item:952 x1", ""},
		{ItemProposal{Name: "noted chisel", Qty: 5}, "noted chisel = item:1755 x5 noted", ""},
		{ItemProposal{Name: "axe"}, "", "use any_of / family"},
		{ItemProposal{Name: "axe", ID: "1351"}, "axe = item:1351", ""},
		{ItemProposal{Name: "axe", ID: "item:1351"}, "axe = item:1351", ""},
		{ItemProposal{Name: "axe", AnyOf: []string{"item:1359", "Iron axe"}}, "axe = any_of [1359 1349]", ""},
		{ItemProposal{Name: "axe", AnyOf: []string{"1359", "Iron axe"}}, "axe = any_of [1359 1349]", ""},
		{ItemProposal{Name: "axe", AnyOf: []string{"952"}}, "", "not a candidate"},
		{ItemProposal{Name: "axe", AnyOf: []string{"Rune axe", "1359"}}, "", "twice"},
		{ItemProposal{Name: "prayer potions", Family: "Prayer potion", Qty: 2}, "prayer potions = family prayer_potion x2", ""},
		{ItemProposal{Name: "prayer potions", Family: "prayer_potion"}, "prayer potions = family prayer_potion", ""},
		{ItemProposal{Name: "prayer potions", Family: "prayer potions"}, "prayer potions = family prayer_potion", ""},
		{ItemProposal{Name: "prayer potions", Family: "Elixir of life"}, "", "unknown item family"},
		{ItemProposal{Name: "axe", ID: "1351", Family: "prayer_potion"}, "", "only one of"},
		{ItemProposal{Name: "fishing pass", Noted: true}, "", "no noted form"},
	}
	for _, c := range cases {
		r, err := itemRequirement(reg, c.p)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%+v: err %v, want %q", c.p, err, c.wantErr)
			}
			continue
		}
		if err != nil || describeItem(r) != c.want {
			t.Errorf("%+v: got %q, %v; want %q", c.p, describeItem(r), err, c.want)
		}
	}
}

func submit(t *testing.T, w *Workspace, section string, steps ...StepProposal) *SubmitResult {
	t.Helper()
	res, err := w.Submit("t", section, "annotate-guide@1", steps)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func reasons(res *SubmitResult, step string) string {
	for _, r := range res.Rejected {
		if r.StepID == step {
			return strings.Join(r.Reasons, "; ")
		}
	}
	return ""
}

func TestSubmitChecks(t *testing.T) {
	cases := []struct {
		name    string
		section string
		step    StepProposal
		want    string // "" = accepted, else a substring of the reasons
	}{
		{"explicit objective, folded quote", "1.1", StepProposal{StepID: "c", Objectives: []ObjectiveProposal{
			objective("completes", "quest", "Cook's Assistant", "finished", 0, "do the cook's assistant quest", hi)}}, ""},
		{"quote not in the text", "1.1", StepProposal{StepID: "c", Objectives: []ObjectiveProposal{
			objective("completes", "quest", "Cook's Assistant", "finished", 0, "finish Cook's Assistant", hi)}}, "is not in /text"},
		{"quote in another field", "1.1", StepProposal{StepID: "a", Location: &LocationProposal{Name: "Lumbridge", Evidence: Evidence{Field: "/location", Quote: "Lumbridge"}, Source: hi}}, "propose_entity"},
		{"inference without the owner", "1.1", StepProposal{StepID: "a2", Objectives: []ObjectiveProposal{
			objective("completes", "quest", "X Marks the Spot", "finished", 0, "Finish X marks the spot", Source{Confidence: content.ConfidenceMedium})}}, "needs confirmed_by owner"},
		{"inference confirmed", "1.1", StepProposal{StepID: "a2", Objectives: []ObjectiveProposal{
			objective("completes", "quest", "X Marks the Spot", "finished", 0, "Finish X marks the spot", Source{Confidence: content.ConfidenceMedium, ConfirmedBy: content.ConfirmedByOwner})}}, ""},
		{"unknown decision", "1.1", StepProposal{StepID: "b", Items: []ItemProposal{{Name: "axe", Family: "", AnyOf: []string{"Rune axe"},
			Evidence: Evidence{Quote: "an axe"}, Source: Source{Confidence: content.ConfidenceMedium, Decision: "axe-means-best-axe"}}}}, "unknown decision"},
		{"starts on a non-quest", "1.1", StepProposal{StepID: "b", Objectives: []ObjectiveProposal{
			objective("starts", "skill", "Woodcutting", "", 30, "chop until 30 woodcutting", hi)}}, "role starts needs a quest@started key"},
		{"uses on a non-item", "1.1", StepProposal{StepID: "b", Objectives: []ObjectiveProposal{
			objective("uses", "skill", "Woodcutting", "", 30, "chop until 30 woodcutting", hi)}}, "role uses needs an item key"},
		{"step of another section", "1.1", StepProposal{StepID: "d"}, "belongs to section"},
		{"unknown step", "1.1", StepProposal{StepID: "zz"}, "unknown step"},
		{"bank leg items and actions", "1.1", StepProposal{StepID: "b",
			Items: []ItemProposal{
				{Name: "axe", AnyOf: []string{"Rune axe", "Adamant axe"}, Qty: 1, Evidence: Evidence{Quote: "an axe"}, Source: hi},
				{Name: "prayer potions", Family: "prayer potion", Qty: 2, Evidence: Evidence{Quote: "2 prayer potions"}, Source: hi},
			},
			BankActions: []BankActionProposal{{Type: "deposit_all", Position: "start", Evidence: Evidence{Quote: "Bank everything"}, Source: hi}},
			Objectives:  []ObjectiveProposal{objective("completes", "skill", "Woodcutting", "", 30, "30 woodcutting", hi)},
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, _ := workspace(t)
			res := submit(t, w, c.section, c.step)
			got := reasons(res, c.step.StepID)
			if c.want == "" && (got != "" || len(res.Accepted) != 1) {
				t.Fatalf("rejected: %s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("reasons %q, want %q", got, c.want)
			}
		})
	}
}

func TestSubmitOrder(t *testing.T) {
	w, _ := workspace(t)
	res := submit(t, w, "1.1",
		StepProposal{StepID: "a2", Objectives: []ObjectiveProposal{objective("completes", "quest", "X Marks the Spot", "finished", 0, "Finish X marks the spot", hi)}},
		StepProposal{StepID: "c", Objectives: []ObjectiveProposal{objective("completes", "quest", "Cook's Assistant", "finished", 0, "Cook’s Assistant quest", hi)}},
	)
	if len(res.Accepted) != 2 {
		t.Fatalf("setup rejected: %+v", res.Rejected)
	}
	cases := []struct {
		section string
		step    StepProposal
		want    string
	}{
		{"1.2", StepProposal{StepID: "d", Objectives: []ObjectiveProposal{objective("starts", "quest", "X Marks the Spot", "started", 0, "start X marks the spot", hi)}},
			"starts the quest after step a2 finishes it"},
		{"1.2", StepProposal{StepID: "d", Objectives: []ObjectiveProposal{objective("completes", "quest", "Cook's Assistant", "finished", 0, "Do Cook's Assistant", hi)}},
			"already completed by step c"},
		{"1.1", StepProposal{StepID: "a", Objectives: []ObjectiveProposal{objective("requires", "quest", "Cook's Assistant", "finished", 0, "start X marks the spot quest", hi)}},
			"only completes it later, at step c"},
		{"1.1", StepProposal{StepID: "a2", Objectives: []ObjectiveProposal{objective("completes", "skill", "Woodcutting", "", 20, "Finish X marks the spot", hi)}},
			""},
		{"1.1", StepProposal{StepID: "c", Objectives: []ObjectiveProposal{objective("completes", "skill", "Woodcutting", "", 20, "Do the Cook", hi)}},
			"already completed by step b (skill:woodcutting>=30)"},
	}
	for _, c := range cases {
		res := submit(t, w, c.section, c.step)
		got := reasons(res, c.step.StepID)
		if c.want == "" && got != "" {
			t.Errorf("%s: rejected: %s", c.step.StepID, got)
		}
		if c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: reasons %q, want %q", c.step.StepID, got, c.want)
		}
	}
}

// TestSuppress: a suppressed rule objective needs the owner, must be one of
// the step's rules, leaves the route order and the composed step, and is
// reported as a suppressed divergence.
func TestSuppress(t *testing.T) {
	w, dir := workspace(t)
	owner := content.ConfirmedByOwner
	cases := []struct {
		name string
		step StepProposal
		want string
	}{
		{"without the owner", StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "skill:woodcutting>=30", Role: "completes", Reason: "x"}}},
			"needs confirmed_by owner"},
		{"not a rule of the step", StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "skill:woodcutting>=31", Role: "completes", Reason: "x", ConfirmedBy: owner}}},
			"is not an objective of the step's rules"},
		{"other role", StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "skill:woodcutting>=30", Role: "requires", Reason: "x", ConfirmedBy: owner}}},
			"is not an objective of the step's rules"},
		{"without a reason", StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "skill:woodcutting>=30", Role: "completes", ConfirmedBy: owner}}},
			"needs a reason"},
		{"suppressed and annotated", StepProposal{StepID: "b",
			Objectives: []ObjectiveProposal{objective("completes", "skill", "Woodcutting", "", 30, "30 woodcutting", hi)},
			Suppress:   []SuppressProposal{{Key: "skill:woodcutting>=30", Role: "completes", Reason: "x", ConfirmedBy: owner}}},
			"both annotated and suppressed"},
		{"bad key", StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "woodcutting 30", Role: "completes", Reason: "x", ConfirmedBy: owner}}},
			"rule objective key"},
	}
	for _, c := range cases {
		res := submit(t, w, "1.1", c.step)
		if got := reasons(res, c.step.StepID); !strings.Contains(got, c.want) {
			t.Errorf("%s: reasons %q, want %q", c.name, got, c.want)
		}
	}

	// b's rule completes woodcutting>=30, so c cannot complete 20 ...
	if got := reasons(submit(t, w, "1.1", StepProposal{StepID: "c", Objectives: []ObjectiveProposal{
		objective("completes", "skill", "Woodcutting", "", 20, "Do the Cook", hi)}}), "c"); !strings.Contains(got, "already completed by step b") {
		t.Fatalf("before suppressing: %q", got)
	}
	// ... until b suppresses it, in the same submit or an earlier one.
	res := submit(t, w, "1.1",
		StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "skill:woodcutting>=30", Role: "completes", Reason: "only chops; the level comes at c", Decision: "gather-later"}}},
		StepProposal{StepID: "c", Objectives: []ObjectiveProposal{objective("completes", "skill", "Woodcutting", "", 20, "Do the Cook", hi)}},
	)
	if got := reasons(res, "b"); !strings.Contains(got, "unknown decision") {
		t.Fatalf("unknown decision: %q", got)
	}
	if _, err := w.RecordDecision("t", content.Decision{ID: "gather-later", Question: "q", Answer: "a", Scope: "s"}); err != nil {
		t.Fatal(err)
	}
	res = submit(t, w, "1.1",
		StepProposal{StepID: "b", Suppress: []SuppressProposal{{Key: "skill:woodcutting>=30", Role: "completes", Reason: "only chops; the level comes at c", Decision: "gather-later"}}},
		StepProposal{StepID: "c", Objectives: []ObjectiveProposal{objective("completes", "skill", "Woodcutting", "", 20, "Do the Cook", hi)}},
	)
	if len(res.Accepted) != 2 || len(res.Accepted[0].Suppressed) != 1 {
		t.Fatalf("submit = %+v", res)
	}

	c, err := content.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := c.Guide("t")
	if b, _ := g.Step("b"); len(b.Objectives) != 0 {
		t.Errorf("composed b = %+v", b.Objectives)
	}
	r, err := w.Report("t")
	if err != nil {
		t.Fatal(err)
	}
	d := r.RuleAgreement.Divergent
	if len(d) != 1 || d[0].StepID != "b" || !d[0].Suppressed || d[0].Reason == "" {
		t.Errorf("divergent = %+v", d)
	}
	if r.Summary.DecisionsApplied != 1 {
		t.Errorf("summary = %+v", r.Summary)
	}
	if len(r.OrderIssues) != 0 {
		t.Errorf("order issues = %+v", r.OrderIssues)
	}

	// Dropping the suppression is accepted for b (its rule is not annotated),
	// but c's woodcutting>=20 now conflicts: the report shows it.
	if res := submit(t, w, "1.1", StepProposal{StepID: "b"}); len(res.Accepted) != 1 {
		t.Fatalf("resubmit b = %+v", res)
	}
	r, _ = w.Report("t")
	if len(r.OrderIssues) != 1 || r.OrderIssues[0].StepID != "c" || !strings.Contains(r.OrderIssues[0].Reasons[0], "already completed by step b") {
		t.Errorf("order issues = %+v", r.OrderIssues)
	}
}

// TestNoBankActions: a step can drop the bank actions derive read from its
// text, only with the owner and never together with its own bank actions.
func TestNoBankActions(t *testing.T) {
	w, dir := workspace(t)
	cases := []struct {
		name string
		step StepProposal
		want string
	}{
		{"without the owner", StepProposal{StepID: "b", NoBankActions: &NoBankActionsProposal{Reason: "not a bank"}}, "needs confirmed_by owner"},
		{"without a reason", StepProposal{StepID: "b", NoBankActions: &NoBankActionsProposal{ConfirmedBy: content.ConfirmedByOwner}}, "needs a reason"},
		{"with bank actions", StepProposal{StepID: "b", NoBankActions: &NoBankActionsProposal{Reason: "x", ConfirmedBy: content.ConfirmedByOwner},
			BankActions: []BankActionProposal{{Type: "deposit_all", Position: "start", Evidence: Evidence{Quote: "Bank everything"}, Source: hi}}}, "excludes bank_actions"},
	}
	for _, c := range cases {
		if got := reasons(submit(t, w, "1.1", c.step), "b"); !strings.Contains(got, c.want) {
			t.Errorf("%s: reasons %q, want %q", c.name, got, c.want)
		}
	}
	res := submit(t, w, "1.1", StepProposal{StepID: "b", NoBankActions: &NoBankActionsProposal{Reason: "not a bank", ConfirmedBy: content.ConfirmedByOwner}})
	if len(res.Accepted) != 1 || len(res.Accepted[0].BankActions) != 1 || res.Accepted[0].BankActions[0] != "none" {
		t.Fatalf("submit = %+v", res)
	}
	c, err := content.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := c.Guide("t")
	if seg, _ := g.Segment("seg-b"); len(seg.BankActions) != 0 {
		t.Errorf("seg-b bank actions = %+v", seg.BankActions)
	}
}

// TestRoundTrip: what submit writes is what the loader composes, a stale
// step comes back in the next batch, and the report counts it all.
func TestRoundTrip(t *testing.T) {
	w, dir := workspace(t)
	if _, err := w.RecordDecision("t", content.Decision{ID: "axe-means-best-axe", Question: "Which axe?", Answer: "Any axe, best first", Scope: "whole guide"}); err != nil {
		t.Fatal(err)
	}
	b, err := w.NextBatch("t", "1.1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Steps) != 2 || b.Steps[0].StepID != "a" || b.Remaining != 2 || len(b.Decisions) != 1 {
		t.Fatalf("batch = %+v", b)
	}
	if got := b.Steps[0].RuleObjectives; len(got) != 1 || got[0].Key != "quest:x_marks_the_spot@started" || got[0].Role != "starts" {
		t.Errorf("rule objectives = %+v", got)
	}
	res := submit(t, w, "1.1",
		StepProposal{StepID: "a", Objectives: []ObjectiveProposal{
			objective("starts", "quest", "X Marks the Spot", "started", 0, "start X marks the spot quest", hi),
			objective("uses", "item", "spade", "", 1, "Buy a spade", hi)}},
		StepProposal{StepID: "a2", Comment: "talking to Veos is not an objective"},
		StepProposal{StepID: "b", Items: []ItemProposal{{Name: "axe", AnyOf: []string{"Rune axe", "Iron axe"}, Qty: 1,
			Evidence: Evidence{Quote: "an axe"}, Source: Source{Confidence: content.ConfidenceMedium, Decision: "axe-means-best-axe"}}},
			BankActions: []BankActionProposal{{Type: "deposit_all", Position: "start", Evidence: Evidence{Quote: "Bank everything"}, Source: hi}}},
	)
	if len(res.Accepted) != 3 || res.Pending != 1 {
		t.Fatalf("submit = %+v", res)
	}

	c, err := content.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := c.Guide("t")
	a, _ := g.Step("a")
	if len(a.Objectives) != 2 || a.Objectives[0].Source.Method != content.ProvenanceLLM || a.Objectives[1].Key.String() != "item:952x1" {
		t.Errorf("composed a = %+v", a.Objectives)
	}
	seg, _ := g.Segment("seg-b")
	if seg.Loadout == nil || len(seg.Loadout.Inventory) != 1 || seg.Loadout.Inventory[0].AnyOf[0] != 1359 || seg.BankActions[0].Type != "deposit_all" {
		t.Errorf("seg-b = %+v %+v", seg.Loadout, seg.BankActions)
	}

	r, err := w.Report("t")
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary.Annotated != 3 || r.Summary.FirstSubmitAccepted != 3 || r.Summary.DecisionsApplied != 1 || r.Summary.OwnerConfirmed != 0 {
		t.Errorf("summary = %+v", r.Summary)
	}
	// b's rule woodcutting>=30 was not repeated: a divergence.
	if r.RuleAgreement.Rules != 2 || r.RuleAgreement.Reproduced != 1 || len(r.RuleAgreement.Divergent) != 1 || r.RuleAgreement.Divergent[0].StepID != "b" {
		t.Errorf("agreement = %+v", r.RuleAgreement)
	}
	if len(r.Inferred) != 1 || r.Inferred[0].Decision != "axe-means-best-axe" {
		t.Errorf("inferred = %+v", r.Inferred)
	}

	// Edit step a upstream: its annotation goes stale and a is pending again.
	gp := filepath.Join(dir, content.GuidesSubdir, "t", content.GuideFileName)
	if err = os.WriteFile(gp, []byte(strings.Replace(testGuide, "Buy a spade,", "Buy a spade and a rope,", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	w2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = w2.NextBatch("t", "1.1", 0)
	if len(b.Steps) != 2 || b.Steps[0].StepID != "a" || !b.Steps[0].Stale || b.Steps[0].Previous == nil {
		t.Errorf("after edit, batch = %+v", b.Steps)
	}
	st, _ := w2.Status("t")
	if cov := st.Guides[0].Sections[0].Coverage; cov.Stale != 1 || cov.Annotated != 2 || cov.Pending != 1 {
		t.Errorf("coverage = %+v", cov)
	}
	if r, _ := w2.Report("t"); r.Summary.Submitted != 3 {
		t.Errorf("report log did not survive reopening: %+v", r.Summary)
	}

	// Drop step a2 upstream: the loader rejects its annotation until the next
	// submit of the section rewrites the sidecar without it.
	if err = os.WriteFile(gp, []byte(strings.Replace(testGuide, `,
   {"id":"a2","text":"Finish X marks the spot and talk to Veos"}`, "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = content.Load(dir); err == nil {
		t.Fatal("annotation of a dropped step should not load")
	}
	w3, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	submit(t, w3, "1.1", StepProposal{StepID: "c", Comment: "the quest is in section 1.2's objectives"})
	if _, err = content.Load(dir); err != nil {
		t.Errorf("after resubmitting, load = %v", err)
	}
}

func TestResolveWorldEntities(t *testing.T) {
	w, _ := workspace(t)
	cases := []struct {
		kind, name string
		status     entities.Status
		ref        string
	}{
		{"npc", "Duke Horacio", entities.Resolved, "npc:duke_horacio"},
		{"npc", "Cook", entities.Ambiguous, ""},
		{"npc", "Cook (Lumbridge)", entities.Resolved, "npc:cook_lumbridge"},
		{"shop", "Lumbridge General Store", entities.Resolved, "shop:lumbridge_general_store"},
		{"object", "Bank booth", entities.Resolved, "object:bank_booth"},
	}
	for _, c := range cases {
		r := w.ResolveEntity(c.kind, c.name, "")
		if r.Status != c.status || r.Ref != c.ref {
			t.Errorf("%s %q = %+v", c.kind, c.name, r)
		}
		if c.status == entities.Ambiguous && (len(r.Candidates) < 2 || r.Candidates[0].Name == "") {
			t.Errorf("%s %q candidates = %+v", c.kind, c.name, r.Candidates)
		}
	}
}

func TestProposeEntity(t *testing.T) {
	w, dir := workspace(t)
	if r := w.ResolveEntity("item", "2 prayer potions", ""); r.Status != entities.Ambiguous || r.Family != "prayer_potion" {
		t.Errorf("prayer potions = %+v", r)
	}
	if _, err := w.ProposeEntity(EntityProposal{Kind: "location", Name: "Lumbridge"}); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Errorf("unconfirmed proposal: %v", err)
	}
	if _, err := w.ProposeEntity(EntityProposal{Kind: "quest", Name: "X", ConfirmedBy: "owner"}); err == nil {
		t.Error("quest proposals must be refused")
	}
	res, err := w.ProposeEntity(EntityProposal{Kind: "location", Name: "Lumbridge", WikiKey: "Lumbridge", Aliases: []string{"Lumby"}, ConfirmedBy: "owner"})
	if err != nil || res.Ref != "location:lumbridge" {
		t.Fatalf("propose = %+v, %v", res, err)
	}
	if _, err = w.ProposeEntity(EntityProposal{Kind: "location", ID: "lumbridge_castle", Name: "Lumby", ConfirmedBy: "owner"}); err == nil || !strings.Contains(err.Error(), "already names") {
		t.Errorf("alias collision: %v", err)
	}
	sub := submit(t, w, "1.1", StepProposal{StepID: "a", Location: &LocationProposal{Name: "lumby", Evidence: Evidence{Field: "/location", Quote: "Lumbridge"}, Source: hi}})
	if len(sub.Accepted) != 1 || sub.Accepted[0].Location != "location:lumbridge" {
		t.Fatalf("location submit = %+v", sub)
	}
	reg, err := entities.Load(os.DirFS(filepath.Join(dir, ReferenceSubdir)))
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := reg.Location("lumbridge"); !ok || e.WikiKey != "Lumbridge" {
		t.Errorf("catalog on disk = %+v", e)
	}
}

func TestReviseEntity(t *testing.T) {
	w, dir := workspace(t)
	if _, err := w.ProposeEntity(EntityProposal{Kind: "location", ID: "h_a_m_hideout", Name: "H.A.M. Hideout", ConfirmedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	sub := submit(t, w, "1.1", StepProposal{StepID: "a", Location: &LocationProposal{Name: "H.A.M. Hideout", Evidence: Evidence{Field: "/location", Quote: "Lumbridge"}, Source: hi}})
	if len(sub.Accepted) != 1 {
		t.Fatalf("setup = %+v", sub)
	}
	if _, err := w.ReviseEntity(EntityRevision{Kind: "location", ID: "h_a_m_hideout", NewID: "ham_hideout"}); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Errorf("unconfirmed revision: %v", err)
	}
	if _, err := w.ReviseEntity(EntityRevision{Kind: "location", ID: "h_a_m_hideout", Remove: true, ConfirmedBy: "owner"}); err == nil || !strings.Contains(err.Error(), "t/a") {
		t.Errorf("removing a used entry: %v", err)
	}
	aliases := []string{"HAM Hideout"}
	res, err := w.ReviseEntity(EntityRevision{Kind: "location", ID: "h_a_m_hideout", NewID: "ham_hideout", Aliases: &aliases, ConfirmedBy: "owner"})
	if err != nil || res.Ref != "location:ham_hideout" || len(res.Rewritten) != 1 {
		t.Fatalf("rename = %+v, %v", res, err)
	}
	if r := w.ResolveEntity("location", "HAM Hideout", ""); r.Ref != "location:ham_hideout" {
		t.Errorf("alias after rename = %+v", r)
	}
	data, err := os.ReadFile(filepath.Join(dir, content.GuidesSubdir, "t", content.AnnotationsSubdir, "1.1.json"))
	if err != nil || !strings.Contains(string(data), "location:ham_hideout") || strings.Contains(string(data), "h_a_m_hideout") {
		t.Errorf("sidecar not rewritten: %v\n%s", err, data)
	}
	if _, err := Open(dir); err != nil {
		t.Errorf("reopening after the rename: %v", err)
	}
	if _, err := w.ProposeEntity(EntityProposal{Kind: "location", Name: "Unused Place", ConfirmedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	if res, err := w.ReviseEntity(EntityRevision{Kind: "location", ID: "unused_place", Remove: true, ConfirmedBy: "owner"}); err != nil || !res.Removed {
		t.Errorf("remove = %+v, %v", res, err)
	}
	if r := w.ResolveEntity("location", "Unused Place", ""); r.Status != entities.Unresolved {
		t.Errorf("removed entry still resolves: %+v", r)
	}
}

func TestSyncRegistry(t *testing.T) {
	w, dir := workspace(t)
	if r := w.ResolveEntity("location", "Lumbridge", ""); r.Status != entities.Unresolved {
		t.Fatalf("precondition: %+v", r)
	}
	loc := `{"catalog": "locations", "version": "test", "locations": [{"id": "lumbridge", "name": "Lumbridge"}]}`
	if err := os.WriteFile(filepath.Join(dir, ReferenceSubdir, "locations.json"), []byte(loc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.SyncRegistry(); err != nil {
		t.Fatal(err)
	}
	if r := w.ResolveEntity("location", "Lumbridge", ""); r.Ref != "location:lumbridge" {
		t.Errorf("catalog edited on disk not picked up: %+v", r)
	}
}

func TestSubmitOrderQuestXP(t *testing.T) {
	w, _ := workspace(t)
	// Cook's Assistant rewards 300 Cooking xp: level 4 (276 xp), not 5 (388).
	res := submit(t, w, "1.1", StepProposal{StepID: "a2", Objectives: []ObjectiveProposal{
		objective("completes", "quest", "Cook's Assistant", "finished", 0, "Finish X marks the spot", hi)}})
	res2 := submit(t, w, "1.2", StepProposal{StepID: "d", Objectives: []ObjectiveProposal{
		objective("completes", "skill", "Cooking", "", 10, "Do Cook's Assistant", hi)}})
	if len(res.Accepted) != 1 || len(res2.Accepted) != 1 {
		t.Fatalf("setup rejected: %+v %+v", res.Rejected, res2.Rejected)
	}
	met := submit(t, w, "1.1", StepProposal{StepID: "c", Objectives: []ObjectiveProposal{
		objective("requires", "skill", "Cooking", "", 4, "Do the Cook", hi)}})
	if len(met.Accepted) != 1 {
		t.Errorf("cooking 4 is met by the quest xp: %s", reasons(met, "c"))
	}
	short := submit(t, w, "1.1", StepProposal{StepID: "c", Objectives: []ObjectiveProposal{
		objective("requires", "skill", "Cooking", "", 5, "Do the Cook", hi)}})
	if got := reasons(short, "c"); strings.Count(got, "only completes it later") != 1 {
		t.Errorf("cooking 5 must be rejected once, got %q", got)
	}
}

func TestReportCountsAndVariants(t *testing.T) {
	w, _ := workspace(t)
	if _, err := w.RecordDecision("t", content.Decision{ID: "do-quest-means-finish", Question: "?", Answer: "!", Scope: "all"}); err != nil {
		t.Fatal(err)
	}
	sub := submit(t, w, "1.1",
		StepProposal{StepID: "c", Objectives: []ObjectiveProposal{objective("completes", "quest", "Cook's Assistant", "finished", 0, "Do the Cook",
			Source{Confidence: content.ConfidenceMedium, Decision: "do-quest-means-finish"})}},
		StepProposal{StepID: "a2", Objectives: []ObjectiveProposal{objective("completes", "quest", "X Marks the Spot", "finished", 0, "Finish X marks the spot",
			Source{Confidence: content.ConfidenceMedium, ConfirmedBy: content.ConfirmedByOwner})}},
	)
	if len(sub.Accepted) != 2 {
		t.Fatalf("setup rejected: %+v", sub.Rejected)
	}
	r, err := w.Report("t")
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary.OwnerConfirmed != 1 || r.Summary.DecisionsApplied != 1 {
		t.Errorf("owner_confirmed %d, decisions_applied %d, want 1 and 1", r.Summary.OwnerConfirmed, r.Summary.DecisionsApplied)
	}
	submit(t, w, "1.1", StepProposal{StepID: "a", Comment: "old explanation"})
	submit(t, w, "1.1", StepProposal{StepID: "a"})
	if c := w.guides["t"].report.Steps["a"].Comment; c != "" {
		t.Errorf("an accepted resubmit keeps the old comment %q", c)
	}
	wig := w.ResolveEntity("item", "wig", "")
	if wig.Status != entities.Ambiguous || len(wig.Candidates) < 2 || wig.Candidates[0].Variant == "" {
		t.Errorf("wig candidates carry no variant: %+v", wig)
	}
}
