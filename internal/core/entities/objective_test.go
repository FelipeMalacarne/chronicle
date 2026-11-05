package entities

import (
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FelipeMalacarne/ironman-planner/content/reference"
)

func TestParseObjectiveKey(t *testing.T) {
	valid := []string{
		"quest:x_marks_the_spot@finished", "quest:rune_mysteries@started", "skill:agility>=70",
		"skill:attack>=1", "skill:herblore>=99", "diary:ardougne:easy@complete", "item:960x120", "item:952x1",
		"unlock:fairy_rings@done", "boss:zulrah>=50", "ca:574@done", "ca:0@done", "qp>=107", "qp>=1",
		"item:family/graceful_hoodx1", "item:family/prayer_potionx4", "item:family/antidote_plus_plusx2",
		"item:any(1355x2|1357x1|1359x1)", "item:any(1x1|2x1)",
	}
	for _, s := range valid {
		k, err := ParseObjectiveKey(s)
		if err != nil {
			t.Errorf("ParseObjectiveKey(%q): %v", s, err)
			continue
		}
		if k.String() != s {
			t.Errorf("ParseObjectiveKey(%q).String() = %q", s, k.String())
		}
	}
	invalid := []string{
		"", "quest:x_marks_the_spot", "quest:x_marks_the_spot@complete", "quest:x_marks_the_spot>=1",
		"skill:agility>=0", "skill:agility>=100", "skill:agility@done", "skill:agility>=7a", "skill:agility>=07",
		"diary:ardougne:easy@finished", "item:952", "item:952x0", "item:952>=1", "item:0x1",
		"unlock:fairy_rings@complete", "boss:zulrah>=0", "ca:574@finished", "qp>=0", "qp@done", "qp",
		"location:draynor_village@done", "goal:fire_cape@done", "step:1.1.5@done", "quest:Cook's Assistant@finished",
		// item families
		"item:family/graceful_hood", "item:family/graceful_hoodx0", "item:family/x1", "item:family/Graceful_hoodx1",
		"item:family/graceful hoodx1", "item:family/graceful_hood>=1", "item:family/graceful_hood@done", "item:family/graceful_hoodx01",
		// item alternatives: canonical form only
		"item:any(1355x2|1357x1|1359x1)x1", "item:any(1357x1|1355x2)", "item:any(1355x2|1355x1)", "item:any(1355x2)",
		"item:any()", "item:any(1355|1357x1)", "item:any(1355x0|1357x1)", "item:any(1355x02|1357x1)", "item:any(01355x2|1357x1)",
		"item:any(1355x2|1357x1", "item:any(1355x2,1357x1)", "item:any(1355x2||1357x1)", "item:any(0x1|1357x1)", "item:any(1355x2|1357x1|)",
		"item:any(1355x2|1357x1)@done", "item:any(1355x2|1357x1)>=1",
	}
	for _, s := range invalid {
		if _, err := ParseObjectiveKey(s); err == nil {
			t.Errorf("ParseObjectiveKey(%q) accepted an invalid key", s)
		}
	}
}

func TestItemKeyAccessors(t *testing.T) {
	fam := MustParseObjectiveKey("item:family/graceful_hoodx1")
	if slug, n, ok := fam.ItemFamily(); !ok || slug != "graceful_hood" || n != 1 {
		t.Errorf("ItemFamily = %q %d %v", slug, n, ok)
	}
	if _, ok := fam.ItemAlternatives(); ok {
		t.Error("a family key has alternatives")
	}
	if _, ok := fam.Ref.Int(); ok {
		t.Error("a family ref is an item id")
	}
	anyKey := MustParseObjectiveKey("item:any(1355x2|1357x1|1359x1)")
	alts, ok := anyKey.ItemAlternatives()
	if want := []ItemQty{{1355, 2}, {1357, 1}, {1359, 1}}; !ok || !slices.Equal(alts, want) {
		t.Errorf("ItemAlternatives = %v %v", alts, ok)
	}
	if _, _, ok := anyKey.ItemFamily(); ok {
		t.Error("an alternatives key is a family")
	}
	if _, ok := anyKey.Ref.Int(); ok {
		t.Error("an alternatives ref is an item id")
	}
	exact := MustParseObjectiveKey("item:1355x1")
	if _, _, ok := exact.ItemFamily(); ok {
		t.Error("an exact key is a family")
	}
	if _, ok := exact.ItemAlternatives(); ok {
		t.Error("an exact key has alternatives")
	}

	built, err := ItemAlternativesKey([]ItemQty{{1359, 1}, {1355, 2}, {1357, 1}})
	if err != nil || built != anyKey {
		t.Errorf("ItemAlternativesKey = %v %v, want %v", built, err, anyKey)
	}
	for _, bad := range [][]ItemQty{{{1355, 1}}, {{1355, 1}, {1355, 2}}, {{1355, 0}, {1357, 1}}, {{0, 1}, {1357, 1}}} {
		if k, err := ItemAlternativesKey(bad); err == nil {
			t.Errorf("ItemAlternativesKey(%v) = %v", bad, k)
		}
	}
	if k, err := ItemFamilyKey("graceful_hood", 1); err != nil || k != fam {
		t.Errorf("ItemFamilyKey = %v %v", k, err)
	}
	for _, bad := range []ObjectiveKey{
		{Ref: Ref{Kind: KindItem, ID: "family/graceful_hood"}, Op: OpQuantity},
		{Ref: Ref{Kind: KindItem, ID: "any(1355x2|1357x1)"}, Op: OpQuantity, N: 1},
		{Ref: Ref{Kind: KindItem, ID: "any(1355x2|1357x1)"}, Op: OpState, State: StateDone},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("Validate(%+v) accepted", bad)
		}
	}
	if _, err := ParseRef("item:family/graceful_hood"); err == nil {
		t.Error("ParseRef accepted a family subject")
	}
	if ref, err := ParseObjectiveRef("item:family/graceful_hood"); err != nil || ref != fam.Ref {
		t.Errorf("ParseObjectiveRef = %v %v", ref, err)
	}
}

func TestObjectiveKeyJSON(t *testing.T) {
	in := struct {
		Keys []ObjectiveKey `json:"keys"`
	}{[]ObjectiveKey{MustParseObjectiveKey("quest:x_marks_the_spot@started"), MustParseObjectiveKey("qp>=32")}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	// encoding/json escapes '>' as \u003e by default; the decoded value is the same.
	if want := `{"keys":["quest:x_marks_the_spot@started","qp\u003e=32"]}`; string(data) != want {
		t.Fatalf("got %s want %s", data, want)
	}
	var out struct {
		Keys []ObjectiveKey `json:"keys"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Keys[0] != in.Keys[0] || out.Keys[1] != in.Keys[1] {
		t.Fatalf("round trip: %v %+v", err, out)
	}
	if err := json.Unmarshal([]byte(`{"keys":["item:952"]}`), &out); err == nil {
		t.Fatal("decoded an item key without quantity")
	}
}

func TestKnownObjective(t *testing.T) {
	r := registry(t)
	cases := map[string]bool{
		"quest:x_marks_the_spot@finished": true,
		"quest:not_a_quest@finished":      false,
		"skill:agility>=70":               true,
		"skill:sailing_plus>=70":          false,
		"diary:ardougne:easy@complete":    true,
		"item:1511x1":                     true,
		"item:999999x1":                   false,
		"boss:zulrah>=50":                 true, // no boss catalog: syntax only
		"unlock:not_an_unlock@done":       false,
		"qp>=107":                         true,
		"item:family/graceful_hoodx1":     true,
		"item:family/not_a_familyx1":      false,
		"item:any(1355x2|1357x1|1359x1)":  true,
		"item:any(1355x2|999999x1)":       false,
	}
	for s, want := range cases {
		msg := r.KnownObjective(MustParseObjectiveKey(s))
		if (msg == "") != want {
			t.Errorf("KnownObjective(%s) = %q, want known=%v", s, msg, want)
		}
	}
	for s, want := range map[string]string{
		"item:family/graceful_hoodx1":    "Graceful hood",
		"item:any(1355x2|1357x1|1359x1)": "Mithril axe / Adamant axe / Rune axe",
	} {
		if name, ok := r.Display(MustParseObjectiveKey(s).Ref); !ok || name != want {
			t.Errorf("Display(%s) = %q %v, want %q", s, name, ok, want)
		}
	}
}

func TestUnlockLocationAndVarCatalogs(t *testing.T) {
	r := registry(t)
	for _, s := range []string{"unlock:fairy_rings", "location:draynor_village"} {
		ref := MustParseRef(s)
		if !r.HasCatalog(ref.Kind) {
			t.Errorf("%s: kind has no catalog", s)
		}
	}
	if vs := r.VarsFor("diary:ardougne:easy"); len(vs) != 1 || vs[0].ID != 4458 || vs[0].Var != Varbit {
		t.Errorf("ardougne easy varbit = %+v", vs)
	}
	if vs := r.VarsFor(QuestPoints); len(vs) != 1 || vs[0].ID != 101 || vs[0].Semantics != VarCount {
		t.Errorf("quest points varp = %+v", vs)
	}
	for _, d := range []string{"lumbridge_and_draynor:elite", "kourend_and_kebos:easy", "western_provinces:hard"} {
		if len(r.VarsFor("diary:"+d)) != 1 {
			t.Errorf("diary %s has no completion varbit", d)
		}
	}

	fsys := catalogFS(t)
	fsys[UnlocksFile] = &fstest.MapFile{Data: []byte(`{"catalog":"unlocks","version":"t","source":"t","unlocks":[
		{"id":"fairy_rings","name":"Fairy rings","wiki_key":"Fairy ring","proof":{"var":"varbit","id":1,"min":1}}]}`)}
	fsys[LocationsFile] = &fstest.MapFile{Data: []byte(`{"catalog":"locations","version":"t","source":"t","locations":[
		{"id":"draynor_village","name":"Draynor Village","wiki_key":"Draynor Village","aliases":["Draynor"]}]}`)}
	reg, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := reg.Display(MustParseRef("location:draynor_village")); !ok || name != "Draynor Village" {
		t.Errorf("location display = %q %v", name, ok)
	}
	for name, want := range map[string]string{"draynor village": "location:draynor_village", "Draynor": "location:draynor_village"} {
		if res := reg.ResolveLocation(name); res.Status != Resolved || res.Ref.String() != want {
			t.Errorf("ResolveLocation(%q) = %+v", name, res)
		}
	}
	if res := reg.ResolveUnlock("fairy rings"); res.Status != Resolved || res.Ref.String() != "unlock:fairy_rings" {
		t.Errorf("ResolveUnlock = %+v", res)
	}
	if res := reg.ResolveLocation("Lumbridge"); res.Status != Unresolved {
		t.Errorf("unknown location resolved: %+v", res)
	}
	if msg := reg.KnownObjective(MustParseObjectiveKey("unlock:fairy_rings@done")); msg != "" {
		t.Errorf("fairy rings unlock: %s", msg)
	}
	if url, ok := reg.WikiURL(MustParseRef("unlock:fairy_rings")); !ok || url != "https://oldschool.runescape.wiki/w/Fairy_ring" {
		t.Errorf("unlock wiki url = %q %v", url, ok)
	}

	bad := catalogFS(t)
	bad[UnlocksFile] = &fstest.MapFile{Data: []byte(`{"unlocks":[{"id":"Fairy Rings","name":"x"},{"id":"a","name":"a","proof":{"var":"varbit","id":1}}]}`)}
	bad[VarbitsFile] = &fstest.MapFile{Data: []byte(`{"vars":[{"var":"varbit","id":1,"name":"X","subject":"unlock:nope","semantics":"flag","test":{"var":"varbit","id":1,"eq":1}}]}`)}
	if _, err := Load(bad); err == nil {
		t.Fatal("loaded invalid unlock and var catalogs")
	}
}

func TestFamilies(t *testing.T) {
	r := registry(t)
	ids, ok := r.Family("prayer_potion")
	if !ok || len(ids) < 4 {
		t.Fatalf("prayer_potion family = %v %v", ids, ok)
	}
	for _, id := range ids {
		if slug, ok := r.FamilyOf(id); !ok || slug != "prayer_potion" {
			t.Errorf("FamilyOf(%d) = %q %v", id, slug, ok)
		}
	}
	if _, ok := r.Family("not_a_family"); ok {
		t.Error("unknown family resolved")
	}
}

// catalogFS copies the embedded catalogs into a writable test FS.
func catalogFS(t *testing.T) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	for _, name := range []string{QuestsFile, ItemsFile, SkillsFile, DiariesFile, CombatAchievementsFile, UnlocksFile, LocationsFile, VarbitsFile} {
		data, err := readEmbedded(name)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}

func readEmbedded(name string) ([]byte, error) { return fs.ReadFile(reference.FS, name) }

func TestQuestRequirements(t *testing.T) {
	r := registry(t)
	q, ok := r.Quest("perilous_moons")
	if !ok {
		t.Fatal("perilous_moons not in the catalog")
	}
	want := []string{"quest:twilights_promise@finished", "skill:slayer>=48", "skill:hunter>=20", "skill:fishing>=20", "skill:runecraft>=20", "skill:construction>=10"}
	if len(q.Requires) != len(want) {
		t.Fatalf("requires = %+v", q.Requires)
	}
	for i, w := range want {
		if q.Requires[i].Key.String() != w {
			t.Errorf("requires[%d] = %s, want %s", i, q.Requires[i].Key, w)
		}
	}
	legends, _ := r.Quest("legends_quest")
	var qp, boostable bool
	for _, req := range legends.Requires {
		qp = qp || req.Key.String() == "qp>=107"
		boostable = boostable || (req.Key.String() == "skill:agility>=50" && req.Boostable)
	}
	if !qp || !boostable {
		t.Errorf("legends_quest requires = %+v", legends.Requires)
	}

	fsys := catalogFS(t)
	fsys[QuestsFile] = &fstest.MapFile{Data: []byte(`{"catalog":"quests","version":"t","source":"t","quests":[
		{"id":"a","name":"A","wiki_key":"A","members":false,"requires":[{"key":"quest:not_a_quest@finished"}]},
		{"id":"b","name":"B","wiki_key":"B","members":false,"requires":[{"key":"quest:b@finished"}]}]}`)}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "a requires quest:not_a_quest@finished") || !strings.Contains(err.Error(), "b requires itself") {
		t.Errorf("Load = %v", err)
	}
}
