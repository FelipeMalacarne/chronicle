package entities

import (
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/FelipeMalacarne/ironman-planner/content/reference"
)

func TestParseObjectiveKey(t *testing.T) {
	valid := []string{
		"quest:x_marks_the_spot@finished", "quest:rune_mysteries@started", "skill:agility>=70",
		"skill:attack>=1", "skill:herblore>=99", "diary:ardougne:easy@complete", "item:960x120", "item:952x1",
		"unlock:fairy_rings@done", "boss:zulrah>=50", "ca:574@done", "ca:0@done", "qp>=107", "qp>=1",
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
	}
	for _, s := range invalid {
		if _, err := ParseObjectiveKey(s); err == nil {
			t.Errorf("ParseObjectiveKey(%q) accepted an invalid key", s)
		}
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
		"unlock:fairy_rings@done":         false,
		"qp>=107":                         true,
	}
	for s, want := range cases {
		msg := r.KnownObjective(MustParseObjectiveKey(s))
		if (msg == "") != want {
			t.Errorf("KnownObjective(%s) = %q, want known=%v", s, msg, want)
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
		{"id":"draynor_village","name":"Draynor Village","wiki_key":"Draynor Village"}]}`)}
	reg, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := reg.Display(MustParseRef("location:draynor_village")); !ok || name != "Draynor Village" {
		t.Errorf("location display = %q %v", name, ok)
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
