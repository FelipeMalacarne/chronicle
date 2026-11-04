// Package catalogbuild regenerates the derived entity catalogs under
// content/reference from the offline wiki extracts in the repository. It
// reads only local files, so regeneration never needs the network:
//
//	data/wiki/quests.json                 → quests.json
//	data/wiki/items.json                  → items.json (extracted by tools/extract_wiki_items.py)
//	data/wiki/entity_aliases.json         → curated quest/item aliases, item ambiguities,
//	                                        diary area aliases
//	data/wiki/diary_tasks.json            → diaries.json
//	data/samples/wiki-bucket/ca_all.json  → combat_achievements.json
//	data/samples/wiki-bucket/varbit_achievement_diary_all.json
//	  + data/samples/runelite/VarPlayer.java → varbits.json (diary completion
//	                                          varbits, quest points varp)
//
// Run `go test ./internal/core/entities/catalogbuild -update` to rewrite the
// committed files; the same test fails when they are stale.
package catalogbuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// Input paths relative to the repository root.
const (
	RawQuests  = "data/wiki/quests.json"
	RawAliases = "data/wiki/entity_aliases.json"
	RawDiaries = "data/wiki/diary_tasks.json"
	RawCAs     = "data/samples/wiki-bucket/ca_all.json"
	RawItems   = "data/wiki/items.json"
	RawVarbits = "data/samples/wiki-bucket/varbit_achievement_diary_all.json"
	RawVarps   = "data/samples/runelite/VarPlayer.java"
)

// questPointsVarp is the RuneLite VarPlayer constant holding the quest point total.
const questPointsVarp = "QUEST_POINTS"

var (
	diaryVarbitPattern = regexp.MustCompile(`^DIARY_([A-Z_ ]+)_(EASY|MEDIUM|HARD|ELITE)_COMPLETED$`)
	varpConstPattern   = regexp.MustCompile(`public static final int (\w+) = (\d+);`)
)

type rawVarbits struct {
	Bucket []struct {
		PageName string `json:"page_name"`
		Index    int    `json:"index"`
		Name     string `json:"name"`
	} `json:"bucket"`
}

type rawQuest struct {
	Name        string            `json:"name"`
	Members     bool              `json:"members"`
	IsMiniquest bool              `json:"is_miniquest"`
	Aliases     map[string]string `json:"aliases"`
	WikiURL     string            `json:"wiki_url"`
}

type curated struct {
	Version      string `json:"version"`
	QuestAliases []struct {
		Text  string `json:"text"`
		Quest string `json:"quest"`
		Note  string `json:"note"`
	} `json:"quest_aliases"`
	QuestTextAmbiguous []struct {
		Text              string   `json:"text"`
		Quests            []string `json:"quests"`
		IncludeSubquestOf string   `json:"include_subquests_of"`
		Reason            string   `json:"reason"`
	} `json:"quest_text_ambiguous"`
	DiaryAreaAliases map[string]string `json:"diary_area_aliases"`
	ItemAliases      []struct {
		Text   string `json:"text"`
		ItemID int    `json:"item_id"`
		Note   string `json:"note"`
	} `json:"item_aliases"`
	ItemAmbiguous []struct {
		Text    string `json:"text"`
		ItemIDs []int  `json:"item_ids"`
		Family  string `json:"family"`
		Reason  string `json:"reason"`
	} `json:"item_ambiguous"`
}

// rawItems is data/wiki/items.json.
type rawItems struct {
	Source struct {
		Wiki            string `json:"wiki"`
		RuneLiteItemIDs string `json:"runelite_item_ids"`
		FetchedAt       string `json:"fetched_at"`
	} `json:"source"`
	Items []struct {
		ID            int     `json:"id"`
		Name          string  `json:"name"`
		WikiKey       string  `json:"wiki_key"`
		Family        *string `json:"family"`
		Variant       *string `json:"variant"`
		NotedID       *int    `json:"noted_id"`
		Stackable     bool    `json:"stackable"`
		EquipmentSlot *string `json:"equipment_slot"`
	} `json:"items"`
}

type rawDiary struct {
	Area      string `json:"area"`
	Tier      string `json:"tier"`
	TaskCount int    `json:"task_count"`
}

type rawCAs struct {
	Bucket []struct {
		PageName string `json:"page_name"`
		Name     string `json:"name"`
		ID       *int   `json:"id"`
		Tier     string `json:"tier"`
		Type     string `json:"type"`
		Monster  string `json:"monster"`
	} `json:"bucket"`
}

// Build returns the generated catalog files keyed by file name.
func Build(root string) (map[string][]byte, error) {
	var (
		quests  []rawQuest
		cur     curated
		diaries []rawDiary
		cas     rawCAs
		varbits rawVarbits
		items   rawItems
	)
	sums := map[string]string{}
	varps, readErr := os.ReadFile(filepath.Join(root, RawVarps))
	if readErr != nil {
		return nil, fmt.Errorf("catalogbuild: %w", readErr)
	}
	varpSum := sha256.Sum256(varps)
	sums[RawVarps] = hex.EncodeToString(varpSum[:])
	for path, into := range map[string]any{RawQuests: &quests, RawAliases: &cur, RawDiaries: &diaries, RawCAs: &cas, RawVarbits: &varbits, RawItems: &items} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, fmt.Errorf("catalogbuild: %w", err)
		}
		sum := sha256.Sum256(data)
		sums[path] = hex.EncodeToString(sum[:])
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(into); err != nil {
			return nil, fmt.Errorf("catalogbuild: %s: %w", path, err)
		}
	}
	source := func(paths ...string) string {
		parts := make([]string, len(paths))
		for i, p := range paths {
			parts[i] = fmt.Sprintf("%s (sha256 %s)", p, sums[p])
		}
		return strings.Join(parts, ", ")
	}

	qc, err := buildQuests(quests, cur)
	if err != nil {
		return nil, err
	}
	qc.CatalogHeader = entities.CatalogHeader{
		Catalog: "quests",
		Version: cur.Version,
		Source:  source(RawQuests, RawAliases),
		Notes:   "Generated by internal/core/entities/catalogbuild. id is the slug of the canonical wiki name; wiki_key is the page title the article URL is derived from. Aliases come from WikiSync/RuneLite names or the curated list.",
	}
	dc, err := buildDiaries(diaries, cur.DiaryAreaAliases)
	if err != nil {
		return nil, err
	}
	dc.CatalogHeader = entities.CatalogHeader{
		Catalog: "diaries",
		Version: cur.Version,
		Source:  source(RawDiaries, RawAliases),
		Notes:   "Generated by internal/core/entities/catalogbuild. id is <area slug>:<tier>; every tier of an area shares the area's wiki page.",
	}
	cc, err := buildCAs(cas)
	if err != nil {
		return nil, err
	}
	cc.CatalogHeader = entities.CatalogHeader{
		Catalog: "combat_achievements",
		Version: cur.Version,
		Source:  source(RawCAs),
		Notes:   "Generated by internal/core/entities/catalogbuild. id is the game's combat achievement task id from the wiki bucket.",
	}

	ic, err := buildItems(items, cur)
	if err != nil {
		return nil, err
	}
	ic.CatalogHeader = entities.CatalogHeader{
		Catalog: "items",
		Version: cur.Version,
		Source:  source(RawItems, RawAliases) + "; wiki " + items.Source.Wiki + " fetched " + items.Source.FetchedAt + "; noted ids from " + items.Source.RuneLiteItemIDs,
		Notes:   "Generated by internal/core/entities/catalogbuild from the items both guides cite plus the families they touch (docs/09 section 7). id is always the unnoted form; noted_id is the noted form (a different id the plugin reports for noted stacks). Variants of one wiki page (doses, charges) are distinct ids sharing family; a family name alone is ambiguous (variant_unspecified). ambiguous lists names that must never resolve to one id. An item not listed stays unresolved (not_in_catalog).",
	}
	vc, err := buildVars(varbits, varps, dc)
	if err != nil {
		return nil, err
	}
	vc.CatalogHeader = entities.CatalogHeader{
		Catalog: "varbits",
		Version: cur.Version,
		Source:  source(RawVarbits, RawVarps, RawDiaries, RawAliases),
		Notes:   "Generated by internal/core/entities/catalogbuild. Reference vars the ingest manifest asks for besides those named by content rules (docs/09 section 3.2): one DIARY_<AREA>_<TIER>_COMPLETED varbit per diary tier and the quest points varp. subject is the objective subject the var measures; flag vars decide a state through test, count vars hold the measured number. Unlock proofs live in unlocks.json.",
	}

	out := map[string][]byte{}
	for name, v := range map[string]any{entities.QuestsFile: qc, entities.DiariesFile: dc, entities.CombatAchievementsFile: cc, entities.VarbitsFile: vc, entities.ItemsFile: ic} {
		b, err := encode(v)
		if err != nil {
			return nil, fmt.Errorf("catalogbuild: encode %s: %w", name, err)
		}
		out[name] = b
	}
	return out, nil
}

func buildQuests(raw []rawQuest, cur curated) (*entities.QuestCatalog, error) {
	qc := &entities.QuestCatalog{}
	byName := map[string]int{}
	seen := map[string]string{}
	for _, q := range raw {
		link, err := entities.ParseWikiURL(q.WikiURL)
		if err != nil {
			return nil, fmt.Errorf("catalogbuild: quest %q: %w", q.Name, err)
		}
		id := entities.Slug(q.Name)
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("catalogbuild: quests %q and %q share id %q", prev, q.Name, id)
		}
		seen[id] = q.Name
		e := entities.QuestEntry{ID: id, Name: q.Name, WikiKey: link.Key, Members: q.Members, Miniquest: q.IsMiniquest}
		for _, src := range []string{"wikisync", "runelite"} {
			if a := q.Aliases[src]; a != "" && a != q.Name && !hasAlias(e.Aliases, a) {
				e.Aliases = append(e.Aliases, entities.Alias{Text: a, Source: src})
			}
		}
		byName[q.Name] = len(qc.Quests)
		qc.Quests = append(qc.Quests, e)
	}
	for _, a := range cur.QuestAliases {
		i, ok := byName[a.Quest]
		if !ok {
			return nil, fmt.Errorf("catalogbuild: curated alias %q: unknown quest %q", a.Text, a.Quest)
		}
		qc.Quests[i].Aliases = append(qc.Quests[i].Aliases, entities.Alias{Text: a.Text, Source: "curated", Note: a.Note})
	}
	for _, t := range cur.QuestTextAmbiguous {
		amb := entities.AmbiguousName{Text: t.Text, Reason: t.Reason}
		for _, name := range t.Quests {
			i, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("catalogbuild: text_ambiguous %q: unknown quest %q", t.Text, name)
			}
			amb.Candidates = append(amb.Candidates, qc.Quests[i].ID)
		}
		if t.IncludeSubquestOf != "" {
			for _, q := range qc.Quests {
				if strings.HasPrefix(q.Name, t.IncludeSubquestOf+"/") {
					amb.Candidates = append(amb.Candidates, q.ID)
				}
			}
		}
		qc.TextAmbiguous = append(qc.TextAmbiguous, amb)
	}
	sort.Slice(qc.Quests, func(i, j int) bool { return qc.Quests[i].ID < qc.Quests[j].ID })
	return qc, nil
}

func hasAlias(as []entities.Alias, text string) bool {
	for _, a := range as {
		if a.Text == text {
			return true
		}
	}
	return false
}

func buildDiaries(raw []rawDiary, aliases map[string]string) (*entities.DiaryCatalog, error) {
	dc := &entities.DiaryCatalog{AreaAliases: map[string]string{}}
	areaIDs := map[string]string{}
	tierRank := map[string]int{}
	for i, t := range entities.DiaryTiers {
		tierRank[t] = i
	}
	for _, d := range raw {
		tier := strings.ToLower(d.Tier)
		if _, ok := tierRank[tier]; !ok {
			return nil, fmt.Errorf("catalogbuild: diary %s: unknown tier %q", d.Area, d.Tier)
		}
		ref, err := entities.DiaryRef(d.Area, tier)
		if err != nil {
			return nil, fmt.Errorf("catalogbuild: %w", err)
		}
		areaID := entities.Slug(d.Area)
		areaIDs[d.Area] = areaID
		dc.Diaries = append(dc.Diaries, entities.DiaryEntry{
			ID: ref.ID, Area: d.Area, AreaID: areaID, Tier: tier,
			WikiKey: entities.CanonicalWikiKey(d.Area + " Diary"), TaskCount: d.TaskCount,
		})
	}
	for alias, area := range aliases {
		id, ok := areaIDs[area]
		if !ok {
			return nil, fmt.Errorf("catalogbuild: diary area alias %q: unknown area %q", alias, area)
		}
		dc.AreaAliases[alias] = id
	}
	sort.Slice(dc.Diaries, func(i, j int) bool {
		a, b := dc.Diaries[i], dc.Diaries[j]
		if a.AreaID != b.AreaID {
			return a.AreaID < b.AreaID
		}
		return tierRank[a.Tier] < tierRank[b.Tier]
	})
	return dc, nil
}

func buildCAs(raw rawCAs) (*entities.CombatAchievementCatalog, error) {
	cc := &entities.CombatAchievementCatalog{}
	seen := map[int]string{}
	for _, c := range raw.Bucket {
		if c.ID == nil {
			return nil, fmt.Errorf("catalogbuild: combat achievement %q has no id", c.Name)
		}
		if prev, dup := seen[*c.ID]; dup {
			return nil, fmt.Errorf("catalogbuild: combat achievements %q and %q share id %d", prev, c.Name, *c.ID)
		}
		seen[*c.ID] = c.Name
		cc.Achievements = append(cc.Achievements, entities.CombatAchievementEntry{
			ID: *c.ID, Name: c.Name, Tier: strings.ToLower(c.Tier), Type: c.Type, Monster: c.Monster,
			WikiKey: entities.CanonicalWikiKey(c.PageName),
		})
	}
	sort.Slice(cc.Achievements, func(i, j int) bool { return cc.Achievements[i].ID < cc.Achievements[j].ID })
	return cc, nil
}

// buildItems converts the wiki extract into the item catalog and applies the
// curated aliases and ambiguous names.
func buildItems(raw rawItems, cur curated) (*entities.ItemCatalog, error) {
	ic := &entities.ItemCatalog{}
	at := map[int]int{}
	families := map[string][]int{}
	for _, it := range raw.Items {
		if it.ID <= 0 || it.Name == "" || it.WikiKey == "" {
			return nil, fmt.Errorf("catalogbuild: item %d %q: missing id, name or wiki_key", it.ID, it.Name)
		}
		if _, dup := at[it.ID]; dup {
			return nil, fmt.Errorf("catalogbuild: duplicate item id %d", it.ID)
		}
		e := entities.ItemEntry{
			ID: it.ID, Name: it.Name, WikiKey: entities.CanonicalWikiKey(it.WikiKey), NotedID: it.NotedID, Stackable: it.Stackable,
		}
		if it.Family != nil {
			e.Family = *it.Family
			families[e.Family] = append(families[e.Family], it.ID)
		}
		if it.Variant != nil {
			e.Variant = *it.Variant
		}
		if it.EquipmentSlot != nil {
			e.EquipmentSlot = *it.EquipmentSlot
		}
		at[it.ID] = len(ic.Items)
		ic.Items = append(ic.Items, e)
	}
	for _, a := range cur.ItemAliases {
		i, ok := at[a.ItemID]
		if !ok {
			return nil, fmt.Errorf("catalogbuild: curated item alias %q: unknown item %d", a.Text, a.ItemID)
		}
		ic.Items[i].Aliases = append(ic.Items[i].Aliases, a.Text)
	}
	for _, a := range cur.ItemAmbiguous {
		ids := a.ItemIDs
		if a.Family != "" {
			ids = families[a.Family]
			if len(ids) == 0 {
				return nil, fmt.Errorf("catalogbuild: item_ambiguous %q: unknown family %q", a.Text, a.Family)
			}
		}
		amb := entities.AmbiguousName{Text: a.Text, Reason: a.Reason}
		for _, id := range ids {
			if _, ok := at[id]; !ok {
				return nil, fmt.Errorf("catalogbuild: item_ambiguous %q: unknown item %d", a.Text, id)
			}
			amb.Candidates = append(amb.Candidates, strconv.Itoa(id))
		}
		ic.Ambiguous = append(ic.Ambiguous, amb)
	}
	sort.Slice(ic.Items, func(i, j int) bool { return ic.Items[i].ID < ic.Items[j].ID })
	return ic, nil
}

// buildVars maps the wiki's diary completion varbits onto diary ids and adds
// the quest points varp. Every diary tier must get exactly one varbit.
func buildVars(raw rawVarbits, varps []byte, dc *entities.DiaryCatalog) (*entities.VarCatalog, error) {
	areas := map[string]string{}
	for _, d := range dc.Diaries {
		areas[d.AreaID] = d.AreaID
	}
	for alias, id := range dc.AreaAliases {
		areas[entities.Slug(alias)] = id
	}
	one := 1
	vc := &entities.VarCatalog{}
	byDiary := map[string]int{}
	for _, row := range raw.Bucket {
		m := diaryVarbitPattern.FindStringSubmatch(row.Name)
		if m == nil || !strings.HasPrefix(row.PageName, "RuneScape:Varbit/") {
			continue
		}
		area, ok := areas[entities.Slug(m[1])]
		if !ok {
			return nil, fmt.Errorf("catalogbuild: varbit %d %s: unknown diary area %q", row.Index, row.Name, m[1])
		}
		id := area + ":" + strings.ToLower(m[2])
		if prev, dup := byDiary[id]; dup {
			return nil, fmt.Errorf("catalogbuild: diary %s has varbits %d and %d", id, prev, row.Index)
		}
		byDiary[id] = row.Index
		vc.Vars = append(vc.Vars, entities.VarEntry{
			Var: entities.Varbit, ID: row.Index, Name: row.Name, Subject: "diary:" + id, Semantics: entities.VarFlag,
			Test: &entities.VarTest{Var: entities.Varbit, ID: row.Index, Eq: &one}, Source: "wiki bucket varbit",
		})
	}
	for _, d := range dc.Diaries {
		if _, ok := byDiary[d.ID]; !ok {
			return nil, fmt.Errorf("catalogbuild: diary %s has no completion varbit", d.ID)
		}
	}
	qp := -1
	for _, m := range varpConstPattern.FindAllStringSubmatch(string(varps), -1) {
		if m[1] == questPointsVarp {
			qp, _ = strconv.Atoi(m[2])
		}
	}
	if qp < 0 {
		return nil, fmt.Errorf("catalogbuild: %s: no %s constant", RawVarps, questPointsVarp)
	}
	vc.Vars = append(vc.Vars, entities.VarEntry{
		Var: entities.Varp, ID: qp, Name: questPointsVarp, Subject: entities.QuestPoints, Semantics: entities.VarCount,
		Source: "RuneLite VarPlayer",
	})
	sort.Slice(vc.Vars, func(i, j int) bool {
		a, b := vc.Vars[i], vc.Vars[j]
		if a.Var != b.Var {
			return a.Var < b.Var
		}
		return a.ID < b.ID
	})
	return vc, nil
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
