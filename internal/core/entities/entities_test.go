package entities

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func registry(t *testing.T) *Registry {
	t.Helper()
	r, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParseRef(t *testing.T) {
	valid := []string{
		"quest:the_restless_ghost", "item:1511", "item:961", "skill:herblore",
		"diary:kourend_and_kebos:elite", "boss:zulrah", "ca:0", "ca:574", "goal:fire_cape",
		"step:1.1.75", "step:oziris-main/1.1.76a", "step:bruh-1.1-f7e685efb210",
	}
	for _, s := range valid {
		r, err := ParseRef(s)
		if err != nil {
			t.Errorf("ParseRef(%q): %v", s, err)
			continue
		}
		if r.String() != s {
			t.Errorf("ParseRef(%q).String() = %q", s, r.String())
		}
	}
	invalid := []string{
		"", "quest", "quest:", "quest:The Restless Ghost", "quest:restless-ghost", "item:0", "item:-1",
		"item:01511", "item:logs", "skill:Herblore", "diary:ardougne", "diary:ardougne:master",
		"ca:x", "step:", "step:a/b/c", "unknown:x", "wiki:Logs",
	}
	for _, s := range invalid {
		if _, err := ParseRef(s); err == nil {
			t.Errorf("ParseRef(%q) accepted an invalid ref", s)
		}
	}
}

func TestRefJSON(t *testing.T) {
	in := struct {
		Ref  Ref   `json:"ref"`
		Refs []Ref `json:"refs"`
	}{MustParseRef("quest:x_marks_the_spot"), []Ref{ItemRef(961), MustParseRef("diary:ardougne:easy")}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"ref":"quest:x_marks_the_spot","refs":["item:961","diary:ardougne:easy"]}`; string(data) != want {
		t.Fatalf("got %s want %s", data, want)
	}
	out := in
	out.Ref, out.Refs = Ref{}, nil
	if err := json.Unmarshal(data, &out); err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip: %v %+v", err, out)
	}
	if err := json.Unmarshal([]byte(`{"ref":"quest:Bad Name"}`), &out); err == nil {
		t.Fatal("invalid ref decoded")
	}
	if _, err := json.Marshal(struct{ R Ref }{Ref{Kind: KindItem, ID: "logs"}}); err == nil {
		t.Fatal("invalid ref encoded")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Cook's Assistant":                       "cooks_assistant",
		"Heroes’ Quest":                          "heroes_quest",
		"Romeo & Juliet":                         "romeo_and_juliet",
		"Kourend & Kebos":                        "kourend_and_kebos",
		"Recipe for Disaster/Freeing Evil Dave":  "recipe_for_disaster_freeing_evil_dave",
		"Desert Treasure II - The Fallen Empire": "desert_treasure_ii_the_fallen_empire",
		"Forgettable Tale...":                    "forgettable_tale",
		"Wanted!":                                "wanted",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q want %q", in, got, want)
		}
	}
}

func TestWikiURLDerivation(t *testing.T) {
	for key, want := range map[WikiKey]string{
		"Cook's Assistant":                      "https://oldschool.runescape.wiki/w/Cook's_Assistant",
		"Romeo & Juliet":                        "https://oldschool.runescape.wiki/w/Romeo_&_Juliet",
		"Defence? What Defence?":                "https://oldschool.runescape.wiki/w/Defence%3F_What_Defence%3F", //nolint:misspell // game term
		"Recipe for Disaster/Freeing Evil Dave": "https://oldschool.runescape.wiki/w/Recipe_for_Disaster/Freeing_Evil_Dave",
		"Mourning's End Part I":                 "https://oldschool.runescape.wiki/w/Mourning's_End_Part_I",
	} {
		if got := key.URL(); got != want {
			t.Errorf("%q.URL() = %q want %q", key, got, want)
		}
		link, err := ParseWikiURL(want)
		if err != nil || link.Key != key {
			t.Errorf("ParseWikiURL(%q) = %+v, %v; want key %q", want, link, err, key)
		}
	}
	for raw, want := range map[string]WikiLink{
		"https://oldschool.runescape.wiki/w/Can%27t_Touch_Me":                           {Key: "Can't Touch Me"},
		"http://oldschool.runescape.wiki/w/logs":                                        {Key: "Logs"},
		"https://oldschool.runescape.wiki/w/Mark_of_grace#Obtaining_marks_of_grace":     {Key: "Mark of grace", Fragment: "Obtaining_marks_of_grace"},
		"https://oldschool.runescape.wiki/w/index.php?title=Rune_Mysteries&action=edit": {Key: "Rune Mysteries"},
	} {
		if got, err := ParseWikiURL(raw); err != nil || got != want {
			t.Errorf("ParseWikiURL(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	media, err := ParseWikiURL("https://oldschool.runescape.wiki/w/Clue_hunter_garb#/media/File:Crack_the_Clue!_-_Week_2_map.png")
	if err != nil || !media.IsMedia() {
		t.Errorf("media link not detected: %+v %v", media, err)
	}
	for _, bad := range []string{"https://i.imgur.com/EJimQCW.png", "https://oldschool.runescape.wiki/", "ftp://oldschool.runescape.wiki/w/Logs"} {
		if _, err := ParseWikiURL(bad); err == nil {
			t.Errorf("ParseWikiURL(%q) accepted a non-article URL", bad)
		}
	}
}

// TestQuestWikiURLsMatchExtract proves that the URL derived from every
// wiki key in the catalog is exactly the URL recorded by the offline extract.
func TestQuestWikiURLsMatchExtract(t *testing.T) {
	r := registry(t)
	data, err := os.ReadFile("../../../data/wiki/quests.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw []struct {
		Name    string `json:"name"`
		WikiURL string `json:"wiki_url"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, q := range raw {
		ref := Ref{Kind: KindQuest, ID: Slug(q.Name)}
		got, ok := r.WikiURL(ref)
		if !ok || got != q.WikiURL {
			t.Errorf("%s: derived %q, extract has %q", ref, got, q.WikiURL)
		}
	}
	if len(raw) != len(r.quests) {
		t.Errorf("catalog has %d quests, extract %d", len(r.quests), len(raw))
	}
}

func TestRegistryDisplayAndWiki(t *testing.T) {
	r := registry(t)
	for ref, want := range map[string][2]string{
		"quest:the_restless_ghost":     {"The Restless Ghost", "https://oldschool.runescape.wiki/w/The_Restless_Ghost"},
		"item:1511":                    {"Logs", "https://oldschool.runescape.wiki/w/Logs"},
		"item:1512":                    {"Logs (noted)", "https://oldschool.runescape.wiki/w/Logs"},
		"item:2434":                    {"Prayer potion(4)", "https://oldschool.runescape.wiki/w/Prayer_potion"},
		"skill:runecraft":              {"Runecraft", "https://oldschool.runescape.wiki/w/Runecraft"},
		"diary:kourend_and_kebos:hard": {"Kourend & Kebos Hard Diary", "https://oldschool.runescape.wiki/w/Kourend_&_Kebos_Diary"},
		"ca:574":                       {"You're a wizard", "https://oldschool.runescape.wiki/w/You're_a_wizard"},
	} {
		parsed := MustParseRef(ref)
		if !r.Known(parsed) {
			t.Errorf("%s not known", ref)
		}
		if got, _ := r.Display(parsed); got != want[0] {
			t.Errorf("Display(%s) = %q want %q", ref, got, want[0])
		}
		if got, _ := r.WikiURL(parsed); got != want[1] {
			t.Errorf("WikiURL(%s) = %q want %q", ref, got, want[1])
		}
	}
	for _, ref := range []string{"quest:not_a_quest", "item:999999", "skill:dungeoneering", "diary:ardougne:master_x", "ca:99999"} {
		if p, err := ParseRef(ref); err == nil && r.Known(p) {
			t.Errorf("%s reported as known", ref)
		}
	}
	if r.HasCatalog(KindBoss) || r.HasCatalog(KindGoal) || r.HasCatalog(KindStep) {
		t.Error("boss/goal/step must not claim a catalog")
	}
}

func TestResolveQuestAliases(t *testing.T) {
	r := registry(t)
	cases := map[string]struct {
		id    string
		match Match
	}{
		"The Restless Ghost":                  {"the_restless_ghost", MatchName},
		"the restless ghost":                  {"the_restless_ghost", MatchName},
		"Restless Ghost":                      {"the_restless_ghost", MatchArticle},
		"Hand in the Sand":                    {"the_hand_in_the_sand", MatchArticle},
		"Forgettable Tale of a Drunken Dwarf": {"forgettable_tale", MatchAlias},
		"Heroes’ Quest":                       {"heroes_quest", MatchName},
		"Recipe for Disaster - Evil Dave":     {"recipe_for_disaster_freeing_evil_dave", MatchAlias},
		"Dragon Slayer":                       {"dragon_slayer_i", MatchAlias},
		"Dragon Slayer II":                    {"dragon_slayer_ii", MatchName},
	}
	for name, want := range cases {
		res := r.ResolveQuest(name)
		if res.Status != Resolved || res.Ref.ID != want.id || res.Match != want.match {
			t.Errorf("ResolveQuest(%q) = %+v want %s/%s", name, res, want.id, want.match)
		}
	}
	for _, name := range []string{"Restless Spirit", "Barbarian Assault", ""} {
		if res := r.ResolveQuest(name); res.Status != Unresolved || res.Reason != ReasonNotInCatalog {
			t.Errorf("ResolveQuest(%q) = %+v, want unresolved", name, res)
		}
	}
}

func TestResolveItems(t *testing.T) {
	r := registry(t)
	type want struct {
		status Status
		ref    string
		match  Match
		reason string
		noted  bool
	}
	for phrase, w := range map[string]want{
		"Steel axe":            {Resolved, "item:1353", MatchName, "", false},
		"steel axe":            {Resolved, "item:1353", MatchName, "", false},
		"232 gp":               {Resolved, "item:995", MatchAlias, "", false},
		"GP":                   {Resolved, "item:995", MatchAlias, "", false},
		"1 normal log":         {Resolved, "item:1511", MatchAlias, "", false},
		"law runes":            {Resolved, "item:563", MatchPlural, "", false},
		"planks":               {Resolved, "item:960", MatchPlural, "", false},
		"shrimp":               {Resolved, "item:315", MatchPlural, "", false},
		"30 balls of wool":     {Resolved, "item:1759", MatchPlural, "", false},
		"buckets of milk":      {Resolved, "item:1927", MatchPlural, "", false},
		"eyes of newt":         {Resolved, "item:221", MatchPlural, "", false},
		"8 woad leaves":        {Resolved, "item:1793", MatchPlural, "", false},
		"120 noted planks":     {Resolved, "item:961", MatchPlural, "", true},
		"Logs (noted)":         {Resolved, "item:1512", MatchName, "", true},
		"Prayer potion(4)":     {Resolved, "item:2434", MatchName, "", false},
		"prayer potions":       {Ambiguous, "", "", ReasonVariantUnspecified, false},
		"games necklace":       {Ambiguous, "", "", ReasonVariantUnspecified, false},
		"pickaxe":              {Ambiguous, "", "", ReasonAmbiguousName, false},
		"Amulet of glory":      {Ambiguous, "", "", ReasonAmbiguousName, false},
		"noted chisel":         {Resolved, "item:1756", MatchName, "", true},
		"noted fishing pass":   {Unresolved, "", "", ReasonNotedFormUnknown, true},
		"Few cakes":            {Unresolved, "", "", ReasonNotInCatalog, false},
		"1k bronze arrowheads": {Resolved, "item:39", MatchAlias, "", false},
		"law rune":             {Resolved, "item:563", MatchName, "", false},
		"Some food":            {Unresolved, "", "", ReasonNotInCatalog, false},
		// Only unobtainable items are named "Cooking pot" and "Plant pot" (the
		// latter resolves through the curated alias, never to item 5356);
		// animation-only copies of Rope never compete with the real one.
		"Cooking pot":  {Unresolved, "", "", ReasonNotInCatalog, false},
		"3 plant pots": {Resolved, "item:5350", MatchPlural, "", false},
		"rope":         {Resolved, "item:954", MatchName, "", false},
	} {
		res, p := r.ResolveItem(phrase)
		if res.Status != w.status || res.Reason != w.reason || p.Noted != w.noted {
			t.Errorf("ResolveItem(%q) = %+v (noted %v), want %+v", phrase, res, p.Noted, w)
			continue
		}
		if w.status == Resolved && (res.Ref.String() != w.ref || res.Match != w.match) {
			t.Errorf("ResolveItem(%q) = %s/%s want %s/%s", phrase, res.Ref, res.Match, w.ref, w.match)
		}
		if w.status == Ambiguous && len(res.Candidates) < 2 {
			t.Errorf("ResolveItem(%q): ambiguous without candidates", phrase)
		}
	}
}

func TestUnobtainableItemsResolveByID(t *testing.T) {
	r := registry(t)
	it, ok := r.Item(5356)
	if !ok || !it.Unobtainable {
		t.Fatalf("item 5356 = %+v, %v; want the unobtainable Plant pot", it, ok)
	}
	if res := r.ResolveItemID(5356); res.Status != Resolved {
		t.Errorf("ResolveItemID(5356) = %+v, want resolved", res)
	}
}

func TestSameItem(t *testing.T) {
	r := registry(t)
	cases := []struct {
		a, b int
		cmp  ItemComparison
		want bool
	}{
		{960, 960, CompareExact, true},
		{960, 961, CompareExact, false},
		{960, 961, CompareIgnoreNoted, true},
		{961, 960, CompareFamily, true},
		{2434, 143, CompareIgnoreNoted, false},
		{2434, 143, CompareFamily, true},
		{1704, 1712, CompareFamily, true},
		{2434, 3008, CompareFamily, false},
		{1353, 1351, CompareFamily, false},
		{999998, 999999, CompareFamily, false},
		{960, 999999, CompareIgnoreNoted, false},
	}
	for _, c := range cases {
		if got := r.SameItem(c.a, c.b, c.cmp); got != c.want {
			t.Errorf("SameItem(%d, %d, %d) = %v want %v", c.a, c.b, c.cmp, got, c.want)
		}
	}
	f, ok := r.Item(961)
	if !ok || !f.Noted || f.BaseID != 960 || f.Name != "Plank" {
		t.Errorf("Item(961) = %+v", f)
	}
}

func TestResolveWikiURL(t *testing.T) {
	r := registry(t)
	for raw, want := range map[string]string{
		"https://oldschool.runescape.wiki/w/X_Marks_the_Spot":           "quest:x_marks_the_spot",
		"https://oldschool.runescape.wiki/w/Defence%3F_What_Defence%3F": "ca:" + caID(t, r, "Defence? What Defence?"), //nolint:misspell // game term
		"https://oldschool.runescape.wiki/w/Steel_axe":                  "item:1353",
		"https://oldschool.runescape.wiki/w/herblore":                   "skill:herblore",
	} {
		res, _ := r.ResolveWikiURL(raw)
		if res.Status != Resolved || res.Ref.String() != want || res.Match != MatchWikiKey {
			t.Errorf("ResolveWikiURL(%q) = %+v want %s", raw, res, want)
		}
	}
	for raw, want := range map[string]string{
		"https://oldschool.runescape.wiki/w/Hand_in_the_Sand":                    "quest:the_hand_in_the_sand",
		"https://oldschool.runescape.wiki/w/Forgettable_Tale_of_a_Drunken_Dwarf": "quest:forgettable_tale",
	} {
		res, _ := r.ResolveWikiURL(raw)
		if res.Status != Resolved || res.Ref.String() != want || res.Match != MatchAlias {
			t.Errorf("ResolveWikiURL(%q) = %+v want redirect alias %s", raw, res, want)
		}
	}
	for raw, reason := range map[string]string{
		"https://oldschool.runescape.wiki/w/Ardougne_Diary":                     ReasonVariantUnspecified,
		"https://oldschool.runescape.wiki/w/Prayer_potion":                      ReasonVariantUnspecified,
		"https://oldschool.runescape.wiki/w/Sailing_training#x":                 ReasonNotInCatalog,
		"https://oldschool.runescape.wiki/w/Clue_hunter_garb#/media/File:a.png": ReasonMediaLink,
		"https://www.youtube.com/watch?v=3ysPgln5qZ4":                           ReasonNotWikiArticle,
	} {
		if res, _ := r.ResolveWikiURL(raw); res.Status == Resolved || res.Reason != reason {
			t.Errorf("ResolveWikiURL(%q) = %+v want reason %s", raw, res, reason)
		}
	}
}

func caID(t *testing.T, r *Registry, name string) string {
	t.Helper()
	res := r.ResolveCombatAchievement(name)
	if res.Status != Resolved {
		t.Fatalf("combat achievement %q: %+v", name, res)
	}
	return res.Ref.ID
}

func TestFindQuestMentions(t *testing.T) {
	r := registry(t)
	text := "Talk to Veos to start X Marks the Spot. Talk to father Aereck to start Restless Ghost. " +
		"Dig for X Marks the Spot. Do the Tears of Guthix quest, then play Tears of Guthix. " +
		"Finish RFD. Do Monkey Madness 2 later. Complete Heroes’ Quest and continue Dragon Slayer II."
	type want struct {
		text   string
		status Status
		ref    string
		reason string
	}
	wants := []want{
		{"X Marks the Spot", Resolved, "quest:x_marks_the_spot", ""},
		{"Restless Ghost", Resolved, "quest:the_restless_ghost", ""},
		{"X Marks the Spot", Unresolved, "", ReasonNoQuestContext},
		{"Tears of Guthix", Resolved, "quest:tears_of_guthix", ""},
		{"Tears of Guthix", Ambiguous, "", ReasonTextAmbiguous},
		{"RFD", Ambiguous, "", ReasonTextAmbiguous},
		{"Monkey Madness", Ambiguous, "", ReasonSequelSuffix},
		{"Heroes’ Quest", Resolved, "quest:heroes_quest", ""},
		{"Dragon Slayer II", Resolved, "quest:dragon_slayer_ii", ""},
	}
	got := r.FindQuestMentions(text)
	if len(got) != len(wants) {
		for _, m := range got {
			t.Logf("%q %+v", m.Text, m.Resolution)
		}
		t.Fatalf("got %d mentions want %d", len(got), len(wants))
	}
	for i, w := range wants {
		m := got[i]
		if m.Text != w.text || text[m.Start:m.End] != w.text || m.Resolution.Status != w.status || m.Resolution.Reason != w.reason {
			t.Errorf("mention %d = %q %+v, want %+v", i, m.Text, m.Resolution, w)
		}
		if w.status == Resolved && m.Resolution.Ref.String() != w.ref {
			t.Errorf("mention %d ref %s want %s", i, m.Resolution.Ref, w.ref)
		}
	}
}

func TestFindDiaryMentions(t *testing.T) {
	r := registry(t)
	got := r.FindDiaryMentions("Finish the Ardougne Hard diary, then the Western Provinces hard Diary and the Kourend elite diary.")
	want := []string{"diary:ardougne:hard", "diary:western_provinces:hard", "diary:kourend_and_kebos:elite"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, m := range got {
		if m.Resolution.Status != Resolved || m.Resolution.Ref.String() != want[i] {
			t.Errorf("mention %d %q = %+v want %s", i, m.Text, m.Resolution, want[i])
		}
	}
}

func TestResolveDiaryAndSkill(t *testing.T) {
	r := registry(t)
	if res := r.ResolveDiary("all", "easy"); res.Status != Unresolved || res.Reason != ReasonAggregate {
		t.Errorf("diary all: %+v", res)
	}
	if res := r.ResolveDiary("fremennik", "hard"); res.Status != Resolved || res.Ref.String() != "diary:fremennik:hard" {
		t.Errorf("diary fremennik: %+v", res)
	}
	if res := r.ResolveSkill("Runecrafting"); res.Status != Resolved || res.Ref.String() != "skill:runecraft" || res.Match != MatchAlias {
		t.Errorf("skill: %+v", res)
	}
}
