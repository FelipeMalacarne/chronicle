package enrich_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content/migrate"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities/enrich"
	"github.com/FelipeMalacarne/ironman-planner/internal/importer/bruhsailer"
)

const root = "../../../.."

var guideDirs = map[string]string{
	"oziris-main":     filepath.Join(root, "content", "guides", "oziris-main"),
	"bruhsailer-main": filepath.Join(root, "content", "guides", "bruhsailer-main"),
}

func registry(t *testing.T) *entities.Registry {
	t.Helper()
	r, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func loadGuide(t *testing.T, id string) *content.Guide {
	t.Helper()
	g, err := content.LoadGuideFile(filepath.Join(guideDirs[id], content.GuideFileName))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func loadReport(t *testing.T, id string) *enrich.Report {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(guideDirs[id], enrich.ReportFileName))
	if err != nil {
		t.Fatal(err)
	}
	var rep enrich.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	return &rep
}

// mentionsByRef collects, for each ref, the distinct source texts that
// resolved to it.
func mentionsByRef(g *content.Guide) map[entities.Ref]map[string]bool {
	out := map[entities.Ref]map[string]bool{}
	for _, st := range g.Steps() {
		for _, m := range st.Entities {
			if out[m.Ref] == nil {
				out[m.Ref] = map[string]bool{}
			}
			out[m.Ref][m.SourceText] = true
		}
	}
	return out
}

// TestBothGuidesShareIdentities proves that differently spelled references in
// the two independent guides resolve to the same canonical quest/item refs.
func TestBothGuidesShareIdentities(t *testing.T) {
	oz := mentionsByRef(loadGuide(t, "oziris-main"))
	bruh := mentionsByRef(loadGuide(t, "bruhsailer-main"))
	cases := []struct {
		ref        string
		ozText     string
		bruhText   string
		sameString bool
	}{
		{"quest:the_restless_ghost", "The Restless Ghost", "Restless Ghost", false},
		{"quest:x_marks_the_spot", "X Marks the Spot", "X Marks the Spot", true},
		{"quest:rune_mysteries", "Rune Mysteries", "Rune Mysteries", true},
		{"quest:heroes_quest", "Heroes' Quest", "Heroes’ Quest", false},
		{"quest:dragon_slayer_i", "Dragon Slayer I", "Dragon Slayer I", true},
		{"item:1353", "Steel axe", "steel axe", false},
		{"item:590", "Tinderbox", "tinderbox", false},
		{"item:995", "GP", "232 gp", false},
		{"item:1511", "1 normal log", "logs", false},
	}
	for _, c := range cases {
		ref := entities.MustParseRef(c.ref)
		if !oz[ref][c.ozText] {
			t.Errorf("oziris-main: %s not resolved from %q (have %v)", ref, c.ozText, oz[ref])
		}
		if !bruh[ref][c.bruhText] {
			t.Errorf("bruhsailer-main: %s not resolved from %q (have %v)", ref, c.bruhText, bruh[ref])
		}
	}
	shared := 0
	for ref := range oz {
		if _, ok := bruh[ref]; ok && ref.Kind == entities.KindQuest {
			shared++
		}
	}
	if shared < 60 {
		t.Errorf("only %d quests resolved in both guides", shared)
	}
}

// TestCounts pins the resolved/ambiguous/unresolved totals of the committed
// reports; a catalog or resolver change must update them deliberately.
func TestCounts(t *testing.T) {
	want := map[string]struct {
		total, loadouts enrich.Tally
		quest, item     enrich.Tally
	}{
		"oziris-main": {
			total:    enrich.Tally{Resolved: 684, Ambiguous: 20, Unresolved: 97},
			loadouts: enrich.Tally{Resolved: 15, Ambiguous: 1, Unresolved: 7},
			quest:    enrich.Tally{Resolved: 455, Ambiguous: 7, Unresolved: 47},
			item:     enrich.Tally{Resolved: 131, Ambiguous: 13, Unresolved: 50},
		},
		"bruhsailer-main": {
			total:    enrich.Tally{Resolved: 1894, Ambiguous: 184, Unresolved: 582},
			loadouts: enrich.Tally{Resolved: 738, Ambiguous: 68, Unresolved: 141},
			quest:    enrich.Tally{Resolved: 391, Ambiguous: 30, Unresolved: 261},
			item:     enrich.Tally{Resolved: 1439, Ambiguous: 153, Unresolved: 309},
		},
	}
	for id, w := range want {
		g := loadGuide(t, id)
		rep := loadReport(t, id)
		c := rep.Counts
		if c.Tally != w.total || c.Loadouts != w.loadouts || c.ByKind["quest"] != w.quest || c.ByKind["item"] != w.item {
			t.Errorf("%s counts = %+v", id, c)
		}
		// The report agrees with the guide and with itself.
		mentions := 0
		for _, st := range g.Steps() {
			mentions += len(st.Entities)
		}
		if mentions != c.Resolved {
			t.Errorf("%s: %d mentions in guide, report says %d resolved", id, mentions, c.Resolved)
		}
		if len(rep.Ambiguous) != c.Ambiguous || len(rep.Unresolved) != c.Unresolved {
			t.Errorf("%s: %d/%d report entries, counts %d/%d", id, len(rep.Ambiguous), len(rep.Unresolved), c.Ambiguous, c.Unresolved)
		}
		var sum enrich.Tally
		for _, k := range c.ByKind {
			sum.Resolved += k.Resolved
			sum.Ambiguous += k.Ambiguous
			sum.Unresolved += k.Unresolved
		}
		if sum != c.Tally {
			t.Errorf("%s: by_kind sums to %+v, total %+v", id, sum, c.Tally)
		}
	}
}

// TestAmbiguousAndUnknownStayExplicit checks that names the resolver must
// not guess are reported with their reason and kept out of the guide.
func TestAmbiguousAndUnknownStayExplicit(t *testing.T) {
	type key struct{ text, reason string }
	cases := map[string][]struct {
		text, reason string
		ambiguous    bool
	}{
		"oziris-main": {
			{"Few cakes", entities.ReasonNotInCatalog, false},
			{"Some food", entities.ReasonNotInCatalog, false},
		},
		"bruhsailer-main": {
			{"RFD", entities.ReasonTextAmbiguous, true},
			{"prayer potions", entities.ReasonVariantUnspecified, true},
			{"games necklace", entities.ReasonVariantUnspecified, true},
			{"https://oldschool.runescape.wiki/w/Clue_hunter_garb#/media/File:Crack_the_Clue!_-_Week_2_map.png", entities.ReasonMediaLink, false},
		},
	}
	for id, wants := range cases {
		g := loadGuide(t, id)
		rep := loadReport(t, id)
		amb, unres := map[key]enrich.Entry{}, map[key]enrich.Entry{}
		for _, e := range rep.Ambiguous {
			amb[key{e.SourceText, e.Reason}] = e
			if len(e.Candidates) == 0 {
				t.Errorf("%s: ambiguous %q without candidates", id, e.SourceText)
			}
		}
		for _, e := range rep.Unresolved {
			unres[key{e.SourceText, e.Reason}] = e
		}
		for _, w := range wants {
			set := unres
			if w.ambiguous {
				set = amb
			}
			e, ok := set[key{w.text, w.reason}]
			if !ok {
				t.Errorf("%s: %q (%s) not reported", id, w.text, w.reason)
				continue
			}
			st, ok := g.Step(e.StepID)
			if !ok {
				t.Errorf("%s: report names unknown step %q", id, e.StepID)
				continue
			}
			for _, m := range st.Entities {
				if m.Field == e.Field && m.SourceText == e.SourceText && spanEq(m.Span, e.Span) {
					t.Errorf("%s: %q is both reported and resolved as %s", id, w.text, m.Ref)
				}
			}
		}
	}
}

func spanEq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestNotedPlanksKeepNotedID shows that a noted mention resolves to the
// noted id, and that it is the same item as the unnoted plank only under an
// explicit comparison.
func TestNotedPlanksKeepNotedID(t *testing.T) {
	reg := registry(t)
	g := loadGuide(t, "oziris-main")
	st, ok := g.Step("1.1.75")
	if !ok {
		t.Fatal("step 1.1.75 missing")
	}
	var got *content.EntityMention
	for i, m := range st.Entities {
		if m.SourceText == "120 noted planks" {
			got = &st.Entities[i]
		}
	}
	if got == nil || got.Ref.String() != "item:961" || got.Match != string(entities.MatchPlural) {
		t.Fatalf("noted planks mention = %+v", got)
	}
	seg, _ := g.Segment(st.SegmentID)
	found := false
	for _, it := range seg.Loadout.Inventory {
		if it.SourceText == "120 noted planks" {
			found = it.ItemID != nil && *it.ItemID == 961
		}
	}
	if !found {
		t.Error("loadout item for noted planks lacks item_id 961")
	}
	if reg.SameItem(961, 960, entities.CompareExact) || !reg.SameItem(961, 960, entities.CompareIgnoreNoted) {
		t.Error("noted/unnoted comparison is not explicit")
	}
}

// TestEnrichmentIsIdempotentAndPreservesSource strips everything enrichment
// writes from the committed guides, re-enriches them and expects the exact
// committed bytes and report: enrichment only adds refs, never rewrites the
// source text, and is deterministic.
func TestEnrichmentIsIdempotentAndPreservesSource(t *testing.T) {
	reg := registry(t)
	for id, dir := range guideDirs {
		committed, err := os.ReadFile(filepath.Join(dir, content.GuideFileName))
		if err != nil {
			t.Fatal(err)
		}
		g := loadGuide(t, id)
		for _, st := range g.Steps() {
			st.Entities = nil
		}
		for _, seg := range g.Segments() {
			if seg.Loadout == nil {
				continue
			}
			for i := range seg.Loadout.Inventory {
				seg.Loadout.Inventory[i].ItemID = nil
			}
			for i := range seg.Loadout.Equipment {
				seg.Loadout.Equipment[i].ItemID = nil
			}
		}
		rep := enrich.Guide(g, reg)
		again := enrich.Guide(g, reg)
		got := encodeLike(t, id, g)
		if !bytes.Equal(got, committed) {
			t.Errorf("%s: re-enriched guide differs from committed guide.json", id)
		}
		a, _ := rep.Encode()
		b, _ := again.Encode()
		wantRep, err := os.ReadFile(filepath.Join(dir, enrich.ReportFileName))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, wantRep) || !bytes.Equal(a, b) {
			t.Errorf("%s: report is not deterministic or differs from committed %s", id, enrich.ReportFileName)
		}
	}
}

// encodeLike encodes a guide the way its importer does.
func encodeLike(t *testing.T, id string, g *content.Guide) []byte {
	t.Helper()
	if id == "oziris-main" {
		data, err := migrate.Encode(g)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

// TestImportersRegenerateCommittedContent regenerates both guides and their
// reports offline from the pinned inputs and expects the committed bytes.
func TestImportersRegenerateCommittedContent(t *testing.T) {
	n, err := migrate.ReadNormalized(filepath.Join(root, "data", "guide", "normalized", "guide.json"))
	if err != nil {
		t.Fatal(err)
	}
	g, rep, err := migrate.ConvertWithReport(n, migrate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ozGuide, _ := migrate.Encode(g)
	ozReport, _ := rep.Encode()
	bruhGuide, result, err := bruhsailer.Generate(filepath.Join(root, "data", "sources", "bruhsailer"))
	if err != nil {
		t.Fatal(err)
	}
	for path, got := range map[string][]byte{
		filepath.Join(guideDirs["oziris-main"], content.GuideFileName):     ozGuide,
		filepath.Join(guideDirs["oziris-main"], enrich.ReportFileName):     ozReport,
		filepath.Join(guideDirs["bruhsailer-main"], content.GuideFileName): bruhGuide,
		filepath.Join(guideDirs["bruhsailer-main"], enrich.ReportFileName): result.EntityReport,
	} {
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; regenerate with `ironman content migrate` / `ironman content import-bruhsailer`", path)
		}
	}
	if result.Entities != (enrich.Tally{Resolved: 1894, Ambiguous: 184, Unresolved: 582}) {
		t.Errorf("bruhsailer result tally = %+v", result.Entities)
	}
}

// TestWikiLinksDeriveFromKeys checks that every resolved wiki link in both
// guides points at the article the registry derives from the entity's key.
func TestWikiLinksDeriveFromKeys(t *testing.T) {
	reg := registry(t)
	checked := 0
	for id := range guideDirs {
		g := loadGuide(t, id)
		for _, st := range g.Steps() {
			for _, m := range st.Entities {
				if m.Method != content.MentionLink || m.Match != string(entities.MatchWikiKey) {
					continue
				}
				derived, ok := reg.WikiURL(m.Ref)
				link, err := entities.ParseWikiURL(m.SourceText)
				if !ok || err != nil {
					t.Errorf("%s %s: %s has no wiki url (%v)", id, st.ID, m.Ref, err)
					continue
				}
				back, _ := entities.ParseWikiURL(derived)
				if back.Key != link.Key {
					t.Errorf("%s %s: link %q has key %q, derived %q", id, st.ID, m.SourceText, link.Key, derived)
				}
				checked++
			}
		}
	}
	if checked < 140 {
		t.Errorf("only %d wiki links checked", checked)
	}
}
