package entities

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/FelipeMalacarne/ironman-planner/content/reference"
)

// Catalog file names under content/reference.
const (
	QuestsFile             = "quests.json"
	ItemsFile              = "items.json"
	SkillsFile             = "skills.json"
	DiariesFile            = "diaries.json"
	CombatAchievementsFile = "combat_achievements.json"
)

// CatalogHeader versions a catalog file.
type CatalogHeader struct {
	Catalog string `json:"catalog"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Notes   string `json:"notes,omitempty"`
}

// Alias is an alternative spelling of an entity name and where it comes from
// ("wikisync", "runelite", "curated").
type Alias struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Note   string `json:"note,omitempty"`
}

// AmbiguousName is a name that must never be resolved to a single entity on
// its own (in free text for quests, anywhere for items). Candidates are ids
// of the catalog's kind.
type AmbiguousName struct {
	Text       string   `json:"text"`
	Candidates []string `json:"candidates"`
	Reason     string   `json:"reason"`
}

// QuestCatalog is content/reference/quests.json.
type QuestCatalog struct {
	CatalogHeader
	Quests []QuestEntry `json:"quests"`
	// TextAmbiguous lists names that collide with non-quest things (a
	// minigame, an abbreviation shared by subquests). Free-text mentions of
	// them resolve only when the text itself says "quest".
	TextAmbiguous []AmbiguousName `json:"text_ambiguous,omitempty"`
}

// QuestEntry is one quest or miniquest.
type QuestEntry struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	WikiKey   WikiKey `json:"wiki_key"`
	Members   bool    `json:"members"`
	Miniquest bool    `json:"miniquest,omitempty"`
	Aliases   []Alias `json:"aliases,omitempty"`
}

// ItemCatalog is content/reference/items.json.
type ItemCatalog struct {
	CatalogHeader
	Items     []ItemEntry     `json:"items"`
	Ambiguous []AmbiguousName `json:"ambiguous,omitempty"`
}

// ItemEntry is one unnoted item id. Noted forms are not separate entries:
// NotedID names the id of the noted form when the item can be noted.
// Variants of one item (potion doses, jewelry charges) are separate ids
// that share Family; Variant describes the difference.
type ItemEntry struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	WikiKey   WikiKey  `json:"wiki_key"`
	NotedID   *int     `json:"noted_id,omitempty"`
	Stackable bool     `json:"stackable,omitempty"`
	Family    string   `json:"family,omitempty"`
	Variant   string   `json:"variant,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
}

// SkillCatalog is content/reference/skills.json.
type SkillCatalog struct {
	CatalogHeader
	Skills []SkillEntry `json:"skills"`
}

// SkillEntry is one skill; ID is the lower-case RuneLite Skill enum name.
type SkillEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	WikiKey WikiKey  `json:"wiki_key"`
	Aliases []string `json:"aliases,omitempty"`
}

// DiaryCatalog is content/reference/diaries.json.
type DiaryCatalog struct {
	CatalogHeader
	Diaries []DiaryEntry `json:"diaries"`
	// AreaAliases maps short area names ("Kourend") to area ids.
	AreaAliases map[string]string `json:"area_aliases,omitempty"`
}

// DiaryEntry is one area/tier pair; ID is "<area id>:<tier>".
type DiaryEntry struct {
	ID        string  `json:"id"`
	Area      string  `json:"area"`
	AreaID    string  `json:"area_id"`
	Tier      string  `json:"tier"`
	WikiKey   WikiKey `json:"wiki_key"`
	TaskCount int     `json:"task_count"`
}

// CombatAchievementCatalog is content/reference/combat_achievements.json.
type CombatAchievementCatalog struct {
	CatalogHeader
	Achievements []CombatAchievementEntry `json:"achievements"`
}

// CombatAchievementEntry is one task; ID is the game's task id.
type CombatAchievementEntry struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Tier    string  `json:"tier"`
	Type    string  `json:"type"`
	Monster string  `json:"monster,omitempty"`
	WikiKey WikiKey `json:"wiki_key"`
}

// Registry indexes every catalog. It is immutable after Load.
type Registry struct {
	Versions map[Kind]string

	quests     map[string]*QuestEntry
	questNames nameIndex
	questText  map[string]*AmbiguousName

	items      map[int]*ItemEntry
	notedBase  map[int]int
	itemNames  nameIndex
	itemFamily map[string][]int
	itemAmbig  map[string]*AmbiguousName

	skills     map[string]*SkillEntry
	skillNames nameIndex

	diaries     map[string]*DiaryEntry
	diaryAreas  map[string]string
	diaryByArea map[string][]string

	cas     map[int]*CombatAchievementEntry
	caNames nameIndex

	byWiki map[WikiKey][]Ref

	textOnce sync.Once
	textIx   *textIndex
}

// nameIndex maps a NormalizeName key to the ids that carry it and how.
type nameIndex map[string][]nameHit

type nameHit struct {
	id    string
	match Match
}

func (ix nameIndex) add(name, id string, m Match) {
	k := NormalizeName(name)
	if k == "" {
		return
	}
	for _, h := range ix[k] {
		if h.id == id {
			return
		}
	}
	ix[k] = append(ix[k], nameHit{id: id, match: m})
}

var (
	defaultOnce sync.Once
	defaultReg  *Registry
	defaultErr  error
)

// Default returns the registry built from the catalogs embedded from
// content/reference. It never touches the network or the working directory.
func Default() (*Registry, error) {
	defaultOnce.Do(func() { defaultReg, defaultErr = Load(reference.FS) })
	return defaultReg, defaultErr
}

// Load builds a registry from catalog files in fsys and checks their
// internal consistency (unique ids, valid refs, aliases that point at
// existing entries).
func Load(fsys fs.FS) (*Registry, error) {
	var (
		qc  QuestCatalog
		ic  ItemCatalog
		sc  SkillCatalog
		dc  DiaryCatalog
		cac CombatAchievementCatalog
	)
	for name, into := range map[string]any{QuestsFile: &qc, ItemsFile: &ic, SkillsFile: &sc, DiariesFile: &dc, CombatAchievementsFile: &cac} {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("entities: %w", err)
		}
		if err := json.Unmarshal(data, into); err != nil {
			return nil, fmt.Errorf("entities: %s: %w", name, err)
		}
	}
	r := &Registry{
		Versions: map[Kind]string{
			KindQuest: qc.Version, KindItem: ic.Version, KindSkill: sc.Version,
			KindDiary: dc.Version, KindCombatAchievement: cac.Version,
		},
		quests: map[string]*QuestEntry{}, questNames: nameIndex{}, questText: map[string]*AmbiguousName{},
		items: map[int]*ItemEntry{}, notedBase: map[int]int{}, itemNames: nameIndex{}, itemFamily: map[string][]int{}, itemAmbig: map[string]*AmbiguousName{},
		skills: map[string]*SkillEntry{}, skillNames: nameIndex{},
		diaries: map[string]*DiaryEntry{}, diaryAreas: map[string]string{}, diaryByArea: map[string][]string{},
		cas: map[int]*CombatAchievementEntry{}, caNames: nameIndex{},
		byWiki: map[WikiKey][]Ref{},
	}
	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	for i := range qc.Quests {
		q := &qc.Quests[i]
		ref := Ref{Kind: KindQuest, ID: q.ID}
		if err := ref.Validate(); err != nil {
			fail("%s: %v", QuestsFile, err)
			continue
		}
		if _, dup := r.quests[q.ID]; dup {
			fail("%s: duplicate quest id %q", QuestsFile, q.ID)
			continue
		}
		r.quests[q.ID] = q
		r.questNames.add(q.Name, q.ID, MatchName)
		for _, a := range q.Aliases {
			r.questNames.add(a.Text, q.ID, MatchAlias)
		}
		r.addWiki(q.WikiKey, ref)
	}
	// Article-less spellings ("Restless Ghost") are added last so that a
	// collision with a real name or alias surfaces as an ambiguity.
	for _, q := range qc.Quests {
		for _, n := range append([]string{q.Name}, aliasTexts(q.Aliases)...) {
			if rest, ok := stripArticle(NormalizeName(n)); ok {
				r.questNames.add(rest, q.ID, MatchArticle)
			}
		}
	}
	for i := range qc.TextAmbiguous {
		a := &qc.TextAmbiguous[i]
		for _, c := range a.Candidates {
			if _, ok := r.quests[c]; !ok {
				fail("%s: text_ambiguous %q: unknown quest %q", QuestsFile, a.Text, c)
			}
		}
		r.questText[NormalizeName(a.Text)] = a
	}

	for i := range ic.Items {
		it := &ic.Items[i]
		if it.ID <= 0 {
			fail("%s: item %q: invalid id %d", ItemsFile, it.Name, it.ID)
			continue
		}
		if _, dup := r.items[it.ID]; dup {
			fail("%s: duplicate item id %d", ItemsFile, it.ID)
			continue
		}
		r.items[it.ID] = it
	}
	for _, it := range ic.Items {
		if it.NotedID != nil {
			n := *it.NotedID
			if _, clash := r.items[n]; clash {
				fail("%s: item %d: noted id %d is also an unnoted entry", ItemsFile, it.ID, n)
			} else if prev, dup := r.notedBase[n]; dup {
				fail("%s: noted id %d claimed by %d and %d", ItemsFile, n, prev, it.ID)
			} else {
				r.notedBase[n] = it.ID
			}
		}
		id := strconv.Itoa(it.ID)
		r.itemNames.add(it.Name, id, MatchName)
		for _, a := range it.Aliases {
			r.itemNames.add(a, id, MatchAlias)
		}
		if it.Family != "" {
			k := NormalizeName(it.Family)
			r.itemFamily[k] = append(r.itemFamily[k], it.ID)
		}
		r.addWiki(it.WikiKey, ItemRef(it.ID))
	}
	for i := range ic.Ambiguous {
		a := &ic.Ambiguous[i]
		for _, c := range a.Candidates {
			n, err := strconv.Atoi(c)
			if _, ok := r.items[n]; err != nil || !ok {
				fail("%s: ambiguous %q: unknown item %q", ItemsFile, a.Text, c)
			}
		}
		r.itemAmbig[NormalizeName(a.Text)] = a
	}

	for i := range sc.Skills {
		s := &sc.Skills[i]
		ref := Ref{Kind: KindSkill, ID: s.ID}
		if err := ref.Validate(); err != nil {
			fail("%s: %v", SkillsFile, err)
			continue
		}
		if _, dup := r.skills[s.ID]; dup {
			fail("%s: duplicate skill id %q", SkillsFile, s.ID)
			continue
		}
		r.skills[s.ID] = s
		r.skillNames.add(s.Name, s.ID, MatchName)
		r.skillNames.add(s.ID, s.ID, MatchName)
		for _, a := range s.Aliases {
			r.skillNames.add(a, s.ID, MatchAlias)
		}
		r.addWiki(s.WikiKey, ref)
	}

	for i := range dc.Diaries {
		d := &dc.Diaries[i]
		ref := Ref{Kind: KindDiary, ID: d.ID}
		if err := ref.Validate(); err != nil {
			fail("%s: %v", DiariesFile, err)
			continue
		}
		if _, dup := r.diaries[d.ID]; dup {
			fail("%s: duplicate diary id %q", DiariesFile, d.ID)
			continue
		}
		r.diaries[d.ID] = d
		r.diaryAreas[NormalizeName(d.Area)] = d.AreaID
		r.diaryAreas[NormalizeName(d.AreaID)] = d.AreaID
		r.diaryByArea[d.AreaID] = append(r.diaryByArea[d.AreaID], d.ID)
		r.addWiki(d.WikiKey, ref)
	}
	for alias, area := range dc.AreaAliases {
		if _, ok := r.diaryByArea[area]; !ok {
			fail("%s: area alias %q: unknown area %q", DiariesFile, alias, area)
		}
		r.diaryAreas[NormalizeName(alias)] = area
	}

	for i := range cac.Achievements {
		c := &cac.Achievements[i]
		if _, dup := r.cas[c.ID]; dup {
			fail("%s: duplicate combat achievement id %d", CombatAchievementsFile, c.ID)
			continue
		}
		r.cas[c.ID] = c
		r.caNames.add(c.Name, strconv.Itoa(c.ID), MatchName)
		r.addWiki(c.WikiKey, Ref{Kind: KindCombatAchievement, ID: strconv.Itoa(c.ID)})
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return nil, fmt.Errorf("entities: invalid catalogs:\n  %s", strings.Join(errs, "\n  "))
	}
	return r, nil
}

func (r *Registry) addWiki(k WikiKey, ref Ref) {
	if k == "" {
		return
	}
	k = CanonicalWikiKey(string(k))
	for _, prev := range r.byWiki[k] {
		if prev == ref {
			return
		}
	}
	r.byWiki[k] = append(r.byWiki[k], ref)
}

func aliasTexts(as []Alias) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Text
	}
	return out
}

// HasCatalog reports whether refs of kind are checked against a catalog.
// Boss and goal refs are validated syntactically only; step refs are
// validated against the guide that contains them.
func (r *Registry) HasCatalog(k Kind) bool {
	switch k {
	case KindQuest, KindItem, KindSkill, KindDiary, KindCombatAchievement:
		return true
	}
	return false
}

// Known reports whether a catalog-backed ref names an existing entry.
// Noted item ids are known when their unnoted entry declares them.
func (r *Registry) Known(ref Ref) bool {
	switch ref.Kind {
	case KindQuest:
		_, ok := r.quests[ref.ID]
		return ok
	case KindSkill:
		_, ok := r.skills[ref.ID]
		return ok
	case KindDiary:
		_, ok := r.diaries[ref.ID]
		return ok
	case KindItem:
		n, ok := ref.Int()
		if !ok {
			return false
		}
		_, ok = r.Item(n)
		return ok
	case KindCombatAchievement:
		n, ok := ref.Int()
		if !ok {
			return false
		}
		_, ok = r.cas[n]
		return ok
	}
	return false
}

// Quest returns a quest entry by id.
func (r *Registry) Quest(id string) (*QuestEntry, bool) {
	q, ok := r.quests[id]
	return q, ok
}

// Skill returns a skill entry by id.
func (r *Registry) Skill(id string) (*SkillEntry, bool) {
	s, ok := r.skills[id]
	return s, ok
}

// Diary returns a diary entry by id ("ardougne:easy").
func (r *Registry) Diary(id string) (*DiaryEntry, bool) {
	d, ok := r.diaries[id]
	return d, ok
}

// CombatAchievement returns a combat achievement by task id.
func (r *Registry) CombatAchievement(id int) (*CombatAchievementEntry, bool) {
	c, ok := r.cas[id]
	return c, ok
}

// Display returns the canonical display name of a known ref.
func (r *Registry) Display(ref Ref) (string, bool) {
	switch ref.Kind {
	case KindQuest:
		if q, ok := r.quests[ref.ID]; ok {
			return q.Name, true
		}
	case KindSkill:
		if s, ok := r.skills[ref.ID]; ok {
			return s.Name, true
		}
	case KindDiary:
		if d, ok := r.diaries[ref.ID]; ok {
			return d.Area + " " + titleTier(d.Tier) + " Diary", true
		}
	case KindItem:
		if n, ok := ref.Int(); ok {
			if f, ok := r.Item(n); ok {
				if f.Noted {
					return f.Name + " (noted)", true
				}
				return f.Name, true
			}
		}
	case KindCombatAchievement:
		if n, ok := ref.Int(); ok {
			if c, ok := r.cas[n]; ok {
				return c.Name, true
			}
		}
	}
	return "", false
}

// WikiKeyOf returns the wiki page key of a known ref.
func (r *Registry) WikiKeyOf(ref Ref) (WikiKey, bool) {
	var k WikiKey
	switch ref.Kind {
	case KindQuest:
		if q, ok := r.quests[ref.ID]; ok {
			k = q.WikiKey
		}
	case KindSkill:
		if s, ok := r.skills[ref.ID]; ok {
			k = s.WikiKey
		}
	case KindDiary:
		if d, ok := r.diaries[ref.ID]; ok {
			k = d.WikiKey
		}
	case KindItem:
		if n, ok := ref.Int(); ok {
			if f, ok := r.Item(n); ok {
				k = r.items[f.BaseID].WikiKey
			}
		}
	case KindCombatAchievement:
		if n, ok := ref.Int(); ok {
			if c, ok := r.cas[n]; ok {
				k = c.WikiKey
			}
		}
	}
	return k, k != ""
}

// WikiURL derives the wiki article URL of a known ref from its wiki key.
func (r *Registry) WikiURL(ref Ref) (string, bool) {
	k, ok := r.WikiKeyOf(ref)
	if !ok {
		return "", false
	}
	return k.URL(), true
}

// ByWikiKey returns every ref whose catalog entry lives on the page key.
// More than one ref (diary tiers, potion doses) means the page alone does
// not identify a single entity.
func (r *Registry) ByWikiKey(k WikiKey) []Ref {
	return append([]Ref(nil), r.byWiki[CanonicalWikiKey(string(k))]...)
}

func titleTier(t string) string {
	if t == "" {
		return t
	}
	return string(t[0]-'a'+'A') + t[1:]
}
