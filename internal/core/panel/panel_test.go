package panel

import (
	"slices"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/planning"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

const (
	hammer    = 2347
	agility10 = "skill:agility>=10"
	// Dragon Slayer I needs 32 QP; the step requires it.
	qp32 = "qp>=32"
)

func registry(t *testing.T) *entities.Registry {
	t.Helper()
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func objective(key, role string) content.Objective {
	return content.Objective{Key: entities.MustParseObjectiveKey(key), Role: role}
}

func ip(n int) *int { return &n }

// testGuide: one main section with a bank leg (loadout: a hammer) and a
// sequence, a uim-only step, and a track that the window never shows.
func testGuide() *content.Guide {
	g := &content.Guide{
		ID: "g", Title: "Test guide", DefaultVariant: "main",
		Source: &content.Source{Author: "Alice and Bob", Origin: "example.guide"},
		Variants: []*content.Variant{{ID: "main", Sections: []*content.Section{
			{ID: "s1", Title: "Early game", Description: " Starting out. ", Segments: []*content.Segment{
				{
					ID: "leg1", Kind: content.SegmentBankLeg, Title: "Bank in Lumbridge",
					BankActions: []content.BankAction{{Type: content.BankWithdraw, Text: "Withdraw a hammer."}},
					Loadout: &content.Loadout{Status: content.LoadoutKnown, Inventory: []content.ItemRequirement{
						{Name: "Hammer", ItemID: ip(hammer), Qty: ip(1)},
					}},
					Steps: []*content.Step{
						{ID: "1", Text: "Train agility.", Objectives: []content.Objective{objective(agility10, content.RoleCompletes)}},
						{ID: "2", Text: "  Talk to Bob.  ", Location: "Lumbridge", Notes: []content.Note{
							{Kind: content.NoteTip, Text: " Bob sells axes. "},
							{Kind: "update", Text: "Skip this step after the rework."},
							{Kind: content.NoteTip, Text: "GP after step: 10k"},
							{Kind: content.NoteTip, Text: "  "},
						}},
						{ID: "3", Text: "Start Dragon Slayer I.", Objectives: []content.Objective{objective(qp32, content.RoleRequires)}},
						{ID: "uim", Text: "UIM only.", Modes: []string{"uim"}},
					},
				},
				{ID: "seq1", Kind: content.SegmentSequence, Steps: []*content.Step{
					{ID: "4", Text: "Walk."},
					{ID: "5", Text: "Run."},
				}},
			}},
			{ID: "t", Title: "Track", Kind: content.KindTrack, Segments: []*content.Segment{
				{ID: "tseg", Kind: content.SegmentSequence, Steps: []*content.Step{{ID: "t1", Text: "Birdhouses."}}},
			}},
		}}},
	}
	g.Index()
	return g
}

func liveInventory(q map[int]int64) rules.ItemHoldings {
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	c := func(q map[int]int64) rules.ContainerItems {
		return rules.ContainerItems{State: ingest.ItemLive, ObservedAt: at, Complete: true, Quantities: q}
	}
	return rules.ItemHoldings{
		rules.ContainerBank: c(map[int]int64{}), rules.ContainerInventory: c(q), rules.ContainerEquipment: c(map[int]int64{}),
	}
}

func board(g *content.Guide, reg *entities.Registry, verdicts progress.Verdicts, marks map[string]progress.Mark) progress.Board {
	return progress.BuildBoard(progress.GuideInput{
		Guide: g, Verdicts: verdicts, Marks: marks, Registry: reg,
		Account: progress.Account{ActiveGuideID: g.ID, Mode: "ironman"},
	}, nil)
}

func ids(w Window) []string {
	out := make([]string, 0, len(w.Steps))
	for _, s := range w.Steps {
		out = append(out, s.ID)
	}
	return out
}

func TestBuildWindow(t *testing.T) {
	reg := registry(t)
	g := testGuide()
	verdicts := progress.Verdicts{agility10: rules.Met, qp32: rules.Unmet}
	b := board(g, reg, verdicts, map[string]progress.Mark{"4": {Kind: progress.MarkSkipped}})
	cases := []struct {
		name   string
		cursor string
		limit  int
		want   []string
	}{
		{name: "cursor plus the visible main steps, no track, no other mode", cursor: "2", want: []string{"2", "3", "4", "5"}},
		{name: "limit counts the steps after the cursor", cursor: "2", limit: 2, want: []string{"2", "3", "4"}},
		{name: "out of range limit is the default", cursor: "2", limit: 500, want: []string{"2", "3", "4", "5"}},
		{name: "a pinned cursor on a track step shows it, then the main steps after it", cursor: "t1", want: []string{"t1"}},
		{name: "unknown cursor", cursor: "zzz", want: []string{}},
		{name: "finished route", cursor: "", want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := Build(Input{Guide: g, Board: b, Cursor: progress.CursorRecord{StepID: tc.cursor, Cause: progress.CursorAuto}, Registry: reg, Limit: tc.limit})
			if got := ids(w); !slices.Equal(got, tc.want) {
				t.Fatalf("steps = %v, want %v", got, tc.want)
			}
			if tc.cursor == "" && w.Cursor != nil {
				t.Fatalf("cursor = %+v, want null", w.Cursor)
			}
		})
	}

	w := Build(Input{Guide: g, Board: b, Cursor: progress.CursorRecord{StepID: "2", Cause: progress.CursorAuto}, Holdings: liveInventory(map[int]int64{}), Registry: reg})
	if w.Attribution != "Guia por Alice e Bob · example.guide" || w.Title != "Test guide" || w.Guide != "g" {
		t.Fatalf("header = %q %q %q", w.Guide, w.Title, w.Attribution)
	}
	if w.Cursor == nil || w.Cursor.StepID != "2" || w.Cursor.Cause != progress.CursorAuto {
		t.Fatalf("cursor = %+v", w.Cursor)
	}
	cur := w.Steps[0]
	if !cur.Current || cur.Text != "Talk to Bob." || cur.Location != "Lumbridge" || cur.SectionTitle != "Early game" || cur.Status != progress.Pending {
		t.Fatalf("cursor step = %+v", cur)
	}
	blocked := w.Steps[1]
	if blocked.Current || len(blocked.BlockedBy) != 1 || blocked.BlockedBy[0] != (Requirement{Key: qp32, Label: "32 quest points"}) {
		t.Fatalf("blocked step = %+v", blocked)
	}
	if len(blocked.Objectives) != 1 || blocked.Objectives[0].Verdict != rules.Unmet || blocked.Objectives[0].Role != content.RoleRequires {
		t.Fatalf("blocked objectives = %+v", blocked.Objectives)
	}
	if skipped := w.Steps[2]; skipped.Status != progress.Skipped || skipped.Mark == nil || skipped.Mark.Kind != progress.MarkSkipped {
		t.Fatalf("skipped step = %+v", skipped)
	}
	if w.Steps[0].BlockedBy == nil || w.Steps[0].Objectives == nil {
		t.Fatal("empty lists must encode as [] for the plugin")
	}
	c := w.Carry
	if c == nil || c.SegmentID != "leg1" || c.Derived || c.LoadoutStatus != content.LoadoutKnown || c.Status != planning.NotReady {
		t.Fatalf("carry = %+v", c)
	}
	if len(c.Items) != 1 || c.Items[0].Name != "Hammer" || c.Items[0].Qty != 1 || c.Items[0].Verdict != rules.Unmet || c.Items[0].Slot != planning.SlotInventory {
		t.Fatalf("carry items = %+v", c.Items)
	}
	if !slices.Equal(c.BankActions, []string{"Withdraw a hammer."}) {
		t.Fatalf("bank actions = %v", c.BankActions)
	}

	ready := Build(Input{Guide: g, Board: b, Cursor: progress.CursorRecord{StepID: "2"}, Holdings: liveInventory(map[int]int64{hammer: 1}), Registry: reg})
	if ready.Carry.Status != planning.Ready || ready.Carry.Items[0].Verdict != rules.Met || ready.Carry.Items[0].Carried != 1 {
		t.Fatalf("carry with a hammer = %+v", ready.Carry)
	}
	seq := Build(Input{Guide: g, Board: b, Cursor: progress.CursorRecord{StepID: "5"}, Registry: reg})
	if seq.Carry == nil || seq.Carry.SegmentID != "seq1" || seq.Carry.Status != planning.NoLoadout || len(seq.Carry.Items) != 0 {
		t.Fatalf("carry of a sequence = %+v", seq.Carry)
	}
}

func TestCarryDerivedFromNextSteps(t *testing.T) {
	g := &content.Guide{ID: "g", Title: "G", Variants: []*content.Variant{{ID: "main", Sections: []*content.Section{{ID: "s", Segments: []*content.Segment{
		{ID: "leg", Kind: content.SegmentBankLeg, Steps: []*content.Step{
			{ID: "1", Text: "bank"},
			{ID: "2", Text: "build", Requirements: []content.ItemRequirement{{Name: "Hammer", ItemID: ip(hammer)}}},
		}},
	}}}}}}
	g.Index()
	b := board(g, nil, progress.Verdicts{}, nil)
	w := Build(Input{Guide: g, Board: b, Cursor: progress.CursorRecord{StepID: "1"}, Holdings: liveInventory(map[int]int64{})})
	if w.Carry == nil || !w.Carry.Derived || len(w.Carry.Items) != 1 || w.Carry.Items[0].Name != "Hammer" {
		t.Fatalf("derived carry = %+v", w.Carry)
	}
	if w.Attribution != "G" {
		t.Fatalf("attribution without a source = %q", w.Attribution)
	}
}

func TestLabel(t *testing.T) {
	reg := registry(t)
	cases := map[string]string{
		"skill:woodcutting>=35":           "35 Woodcutting",
		"quest:x_marks_the_spot@finished": "X Marks the Spot",
		"item:314x120":                    "120 Feather",
		"item:590x1":                      "Tinderbox",
		"qp>=107":                         "107 quest points",
		"item:family/graceful_hoodx1":     "Graceful hood (qualquer variante)",
		"item:any(1355x2|1357x1|1359x1)":  "2 Mithril axe ou Adamant axe ou Rune axe",
		"not a key":                       "not a key",
	}
	for key, want := range cases {
		if got := Label(key, reg); got != want {
			t.Errorf("Label(%q) = %q, want %q", key, got, want)
		}
	}
	// Without the catalog the id still reads as words.
	if got := Label("quest:x_marks_the_spot@finished", nil); got != "X Marks The Spot" {
		t.Errorf("Label without a catalog = %q", got)
	}
	if got := Label("item:family/graceful_hoodx1", nil); got != "Graceful hood (qualquer variante)" {
		t.Errorf("family without a catalog = %q", got)
	}
}

func TestAttributionOfTheShippedGuides(t *testing.T) {
	cases := []struct {
		src  content.Source
		want string
	}{
		{content.Source{Author: "OzirisLoL", Origin: "ironman.guide"}, "Guia por OzirisLoL · ironman.guide"},
		{content.Source{Author: "So Iron BRUH and ParasailerOSRS", Origin: "other"}, "Guia por So Iron BRUH e ParasailerOSRS"},
	}
	for _, tc := range cases {
		src := tc.src
		if got := Attribution(&content.Guide{Title: "x", Source: &src}); got != tc.want {
			t.Errorf("Attribution(%+v) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestWindowNotesAndSection(t *testing.T) {
	g, reg := testGuide(), registry(t)
	w := Build(Input{Guide: g, Board: board(g, reg, progress.Verdicts{}, nil), Cursor: progress.CursorRecord{StepID: "1", Cause: progress.CursorAuto}, Registry: reg})
	if w.Section == nil || w.Section.ID != "s1" || w.Section.Title != "Early game" || w.Section.Description != "Starting out." {
		t.Fatalf("section = %+v", w.Section)
	}
	var notes []Note
	for _, s := range w.Steps {
		if s.ID == "2" {
			notes = s.Notes
		}
		if s.ID == "1" && s.Notes != nil {
			t.Errorf("step without notes = %+v", s.Notes)
		}
	}
	want := []Note{{Kind: content.NoteTip, Text: "Bob sells axes."}, {Kind: "update", Text: "Skip this step after the rework."}}
	if !slices.Equal(notes, want) {
		t.Fatalf("notes = %+v, want %+v (metadata and blank lines dropped)", notes, want)
	}
	if empty := Build(Input{Guide: g, Board: board(g, reg, progress.Verdicts{}, nil), Registry: reg}); empty.Section != nil {
		t.Errorf("no cursor, section = %+v", empty.Section)
	}
}
