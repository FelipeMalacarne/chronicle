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
	UnlocksFile            = "unlocks.json"
	LocationsFile          = "locations.json"
	VarbitsFile            = "varbits.json"
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
	// XPRewards is the fixed skill experience the quest grants on completion
	// (choice lamps excluded), from the wiki's {{Quest rewards}}.
	XPRewards []SkillXP `json:"xp_rewards,omitempty"`
	// Requires are the quest's own requirements from the wiki's quest
	// details: the direct quest requirements (finished), skill levels and
	// quest points, as objective keys. They hold whatever guide routes the
	// quest (docs/09 section 3, "Requires").
	Requires []QuestRequirement `json:"requires,omitempty"`
	// CombatLevel is a combat level requirement; it has no objective key.
	CombatLevel int `json:"combat_level,omitempty"`
}

// QuestRequirement is one requirement of a quest. Boostable says a skill
// level may be met with a temporary boost; DuringQuest says it is needed
// only for part of the quest, not to start it.
type QuestRequirement struct {
	Key         ObjectiveKey `json:"key"`
	Boostable   bool         `json:"boostable,omitempty"`
	DuringQuest bool         `json:"during_quest,omitempty"`
}

// SkillXP is an amount of experience in one skill (skill id).
type SkillXP struct {
	Skill string `json:"skill"`
	XP    int    `json:"xp"`
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
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	WikiKey   WikiKey `json:"wiki_key"`
	NotedID   *int    `json:"noted_id,omitempty"`
	Stackable bool    `json:"stackable,omitempty"`
	Family    string  `json:"family,omitempty"`
	Variant   string  `json:"variant,omitempty"`
	// EquipmentSlot is the wiki's equipment slot ("weapon", "head", ...) of
	// equipable items.
	EquipmentSlot string `json:"equipment_slot,omitempty"`
	// Unobtainable marks ids the wiki documents as unobtainable or
	// animation-only items ("Plant pot (unobtainable item)"): they resolve by
	// id, never by name, so a name never lands on an item no player can hold.
	Unobtainable bool     `json:"unobtainable,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
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
	// Proof is the task's bit in its CA_TASK_COMPLETED_<id/32> varp.
	Proof *VarTest `json:"proof,omitempty"`
}

// UnlockCatalog is content/reference/unlocks.json. Entries are added only
// through owner-confirmed proposals during guide extraction (docs/09 section
// 3.1), never by importing the whole wiki.
type UnlockCatalog struct {
	CatalogHeader
	Unlocks []UnlockEntry `json:"unlocks"`
}

// UnlockEntry is one unlock (fairy rings, a spellbook, a teleport, access to
// an area). Proof is the var that shows it, when one is known; without it the
// unlock can only be marked by the owner.
type UnlockEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	WikiKey WikiKey  `json:"wiki_key,omitempty"`
	Proof   *VarTest `json:"proof,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// VarTest is a check on one varbit or varp: equal to Eq or at least Min.
type VarTest struct {
	Var VarKind `json:"var"`
	ID  int     `json:"id"`
	Eq  *int    `json:"eq,omitempty"`
	Min *int    `json:"min,omitempty"`
	// Bit: the var has this bit set (a combat achievement in its
	// CA_TASK_COMPLETED_<n> varp).
	Bit *int `json:"bit,omitempty"`
}

// VarKind is "varbit" or "varp".
type VarKind string

// Var kinds.
const (
	Varbit VarKind = "varbit"
	Varp   VarKind = "varp"
)

// LocationCatalog is content/reference/locations.json. Like unlocks, entries
// come from owner-confirmed proposals during extraction.
type LocationCatalog struct {
	CatalogHeader
	Locations []LocationEntry `json:"locations"`
}

// LocationEntry is one place steps can be grouped by.
type LocationEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	WikiKey WikiKey  `json:"wiki_key,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// VarCatalog is content/reference/varbits.json: the reference vars the ingest
// manifest asks the plugin for besides those named by content rules
// (docs/09 sections 3.2 and 7).
type VarCatalog struct {
	CatalogHeader
	Vars []VarEntry `json:"vars"`
}

// VarEntry is one varbit or varp and what it proves. Subject is the objective
// subject it measures: a ref ("diary:ardougne:easy", "unlock:fairy_rings") or
// "qp". Semantics "flag" means Test decides the state; "count" means the
// value itself is the measured number (quest points).
type VarEntry struct {
	Var       VarKind  `json:"var"`
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Subject   string   `json:"subject"`
	Semantics string   `json:"semantics"`
	Test      *VarTest `json:"test,omitempty"`
	Source    string   `json:"source,omitempty"`
}

// Var semantics.
const (
	VarFlag  = "flag"
	VarCount = "count"
)

// CountSubject prefixes the subjects of count vars that measure no objective,
// only a number the account screen shows: "count:ca:<tier>" (the game's own
// total of done combat achievements of a tier), "count:collection_log" and
// "count:collection_log_max".
const CountSubject = "count:"

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

	unlocks       map[string]*UnlockEntry
	unlockNames   nameIndex
	locations     map[string]*LocationEntry
	locationNames nameIndex
	vars          []VarEntry

	familyBySlug map[string]string

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
		uc  UnlockCatalog
		lc  LocationCatalog
		vc  VarCatalog
	)
	for name, into := range map[string]any{
		QuestsFile: &qc, ItemsFile: &ic, SkillsFile: &sc, DiariesFile: &dc, CombatAchievementsFile: &cac,
		UnlocksFile: &uc, LocationsFile: &lc, VarbitsFile: &vc,
	} {
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
			KindUnlock: uc.Version, KindLocation: lc.Version,
		},
		quests: map[string]*QuestEntry{}, questNames: nameIndex{}, questText: map[string]*AmbiguousName{},
		items: map[int]*ItemEntry{}, notedBase: map[int]int{}, itemNames: nameIndex{}, itemFamily: map[string][]int{}, itemAmbig: map[string]*AmbiguousName{},
		skills: map[string]*SkillEntry{}, skillNames: nameIndex{},
		diaries: map[string]*DiaryEntry{}, diaryAreas: map[string]string{}, diaryByArea: map[string][]string{},
		cas: map[int]*CombatAchievementEntry{}, caNames: nameIndex{},
		unlocks: map[string]*UnlockEntry{}, unlockNames: nameIndex{},
		locations: map[string]*LocationEntry{}, locationNames: nameIndex{},
		familyBySlug: map[string]string{},
		byWiki:       map[WikiKey][]Ref{},
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
		for _, a := range it.Aliases {
			r.itemNames.add(a, id, MatchAlias)
		}
		r.addWiki(it.WikiKey, ItemRef(it.ID))
		if it.Unobtainable {
			continue
		}
		r.itemNames.add(it.Name, id, MatchName)
		if it.Family != "" {
			k := NormalizeName(it.Family)
			r.itemFamily[k] = append(r.itemFamily[k], it.ID)
			slug := FamilySlug(it.Family)
			if prev, ok := r.familyBySlug[slug]; ok && prev != k {
				fail("%s: families %q and %q share slug %q", ItemsFile, prev, it.Family, slug)
			}
			r.familyBySlug[slug] = k
		}
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

	for _, q := range qc.Quests {
		for _, x := range q.XPRewards {
			if _, ok := r.skills[x.Skill]; !ok || x.XP <= 0 {
				fail("%s: quest %q: invalid xp reward %+v", QuestsFile, q.ID, x)
			}
		}
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
		if c.Proof != nil {
			if msg := c.Proof.check(); msg != "" {
				fail("%s: %d: proof: %s", CombatAchievementsFile, c.ID, msg)
			}
		}
		r.cas[c.ID] = c
		r.caNames.add(c.Name, strconv.Itoa(c.ID), MatchName)
		r.addWiki(c.WikiKey, Ref{Kind: KindCombatAchievement, ID: strconv.Itoa(c.ID)})
	}

	for i := range uc.Unlocks {
		u := &uc.Unlocks[i]
		ref := Ref{Kind: KindUnlock, ID: u.ID}
		if err := ref.Validate(); err != nil {
			fail("%s: %v", UnlocksFile, err)
			continue
		}
		if _, dup := r.unlocks[u.ID]; dup {
			fail("%s: duplicate unlock id %q", UnlocksFile, u.ID)
			continue
		}
		if u.Proof != nil {
			if msg := u.Proof.check(); msg != "" {
				fail("%s: unlock %q: %s", UnlocksFile, u.ID, msg)
			}
		}
		r.unlocks[u.ID] = u
		r.unlockNames.add(u.Name, u.ID, MatchName)
		for _, a := range u.Aliases {
			r.unlockNames.add(a, u.ID, MatchAlias)
		}
		r.addWiki(u.WikiKey, ref)
	}
	for i := range lc.Locations {
		l := &lc.Locations[i]
		ref := Ref{Kind: KindLocation, ID: l.ID}
		if err := ref.Validate(); err != nil {
			fail("%s: %v", LocationsFile, err)
			continue
		}
		if _, dup := r.locations[l.ID]; dup {
			fail("%s: duplicate location id %q", LocationsFile, l.ID)
			continue
		}
		r.locations[l.ID] = l
		r.locationNames.add(l.Name, l.ID, MatchName)
		for _, a := range l.Aliases {
			r.locationNames.add(a, l.ID, MatchAlias)
		}
		r.addWiki(l.WikiKey, ref)
	}
	seenVar := map[string]bool{}
	for _, v := range vc.Vars {
		key := fmt.Sprintf("%s/%d", v.Var, v.ID)
		switch {
		case v.Var != Varbit && v.Var != Varp:
			fail("%s: %s: unknown var kind %q", VarbitsFile, key, v.Var)
		case seenVar[key]:
			fail("%s: duplicate %s", VarbitsFile, key)
		case v.Semantics == VarFlag && v.Test == nil:
			fail("%s: %s: flag vars need a test", VarbitsFile, key)
		case v.Semantics != VarFlag && v.Semantics != VarCount:
			fail("%s: %s: unknown semantics %q", VarbitsFile, key, v.Semantics)
		}
		if v.Test != nil {
			if msg := v.Test.check(); msg != "" {
				fail("%s: %s: %s", VarbitsFile, key, msg)
			} else if v.Test.Var != v.Var || v.Test.ID != v.ID {
				fail("%s: %s: test checks another var", VarbitsFile, key)
			}
		}
		seenVar[key] = true
		if v.Subject != QuestPoints && !strings.HasPrefix(v.Subject, CountSubject) {
			ref, err := ParseRef(v.Subject)
			if err != nil {
				fail("%s: %s: %v", VarbitsFile, key, err)
				continue
			}
			if ref.Kind != KindUnlock && !r.Known(ref) {
				fail("%s: %s: unknown subject %s", VarbitsFile, key, ref)
			}
		}
		r.vars = append(r.vars, v)
	}
	for _, v := range r.vars {
		if ref, err := ParseRef(v.Subject); err == nil && ref.Kind == KindUnlock && !r.Known(ref) {
			fail("%s: %s/%d: unknown subject %s", VarbitsFile, v.Var, v.ID, ref)
		}
	}
	for _, q := range qc.Quests {
		for _, req := range q.Requires {
			if msg := r.KnownObjective(req.Key); msg != "" {
				fail("%s: %s requires %s: %s", QuestsFile, q.ID, req.Key, msg)
			}
			if req.Key.Ref == (Ref{Kind: KindQuest, ID: q.ID}) {
				fail("%s: %s requires itself", QuestsFile, q.ID)
			}
		}
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
	case KindQuest, KindItem, KindSkill, KindDiary, KindCombatAchievement, KindUnlock, KindLocation:
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
	case KindUnlock:
		_, ok := r.unlocks[ref.ID]
		return ok
	case KindLocation:
		_, ok := r.locations[ref.ID]
		return ok
	}
	return false
}

// DiaryAreaIDs returns every diary area id, sorted.
func (r *Registry) DiaryAreaIDs() []string {
	out := make([]string, 0, len(r.diaryByArea))
	for a := range r.diaryByArea {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Unlock returns an unlock entry by id.
func (r *Registry) Unlock(id string) (*UnlockEntry, bool) {
	u, ok := r.unlocks[id]
	return u, ok
}

// ProofVars returns the var tests of every unlock that has a proof, sorted by
// unlock id. With Vars and the combat achievements' proofs they are the
// reference vars the ingest manifest asks for.
func (r *Registry) ProofVars() []VarTest {
	ids := make([]string, 0, len(r.unlocks))
	for id, u := range r.unlocks {
		if u.Proof != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]VarTest, 0, len(ids))
	for _, id := range ids {
		out = append(out, *r.unlocks[id].Proof)
	}
	return out
}

// Location returns a location entry by id.
func (r *Registry) Location(id string) (*LocationEntry, bool) {
	l, ok := r.locations[id]
	return l, ok
}

// Vars returns the reference vars in catalog order.
func (r *Registry) Vars() []VarEntry { return append([]VarEntry(nil), r.vars...) }

// VarsFor returns the reference vars that measure subject ("qp" or a ref).
func (r *Registry) VarsFor(subject string) []VarEntry {
	var out []VarEntry
	for _, v := range r.vars {
		if v.Subject == subject {
			out = append(out, v)
		}
	}
	return out
}

// Family returns the unnoted ids of an item family by its slug
// ("prayer_potion", see FamilySlug), in catalog order.
func (r *Registry) Family(slug string) ([]int, bool) {
	k, ok := r.familyBySlug[slug]
	if !ok {
		return nil, false
	}
	return append([]int(nil), r.itemFamily[k]...), true
}

// FamilyOf returns the family slug of an item id (noted or unnoted).
func (r *Registry) FamilyOf(id int) (string, bool) {
	f, ok := r.Item(id)
	if !ok || f.Family == "" {
		return "", false
	}
	return FamilySlug(f.Family), true
}

func (t VarTest) check() string {
	switch {
	case t.Var != Varbit && t.Var != Varp:
		return fmt.Sprintf("unknown var kind %q", t.Var)
	case t.ID < 0:
		return fmt.Sprintf("invalid var id %d", t.ID)
	case btoi(t.Eq != nil)+btoi(t.Min != nil)+btoi(t.Bit != nil) != 1:
		return "a var test needs exactly one of eq, min and bit"
	case t.Bit != nil && (*t.Bit < 0 || *t.Bit > 31):
		return fmt.Sprintf("invalid bit %d", *t.Bit)
	}
	return ""
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CombatAchievements returns every task, sorted by id.
func (r *Registry) CombatAchievements() []*CombatAchievementEntry {
	out := make([]*CombatAchievementEntry, 0, len(r.cas))
	for _, c := range r.cas {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
		if name, ok := r.displayItemSet(ref.ID); ok {
			return name, true
		}
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
	case KindUnlock:
		if u, ok := r.unlocks[ref.ID]; ok {
			return u.Name, true
		}
	case KindLocation:
		if l, ok := r.locations[ref.ID]; ok {
			return l.Name, true
		}
	}
	return "", false
}

// displayItemSet names the subject of an item family key (the family name,
// "Graceful hood") or of an alternatives key (the item names joined with
// " / ").
func (r *Registry) displayItemSet(id string) (string, bool) {
	if slug, ok := strings.CutPrefix(id, itemFamilyPrefix); ok {
		ids, known := r.Family(slug)
		if !known || len(ids) == 0 {
			return "", false
		}
		f, _ := r.Item(ids[0])
		return f.Family, f.Family != ""
	}
	alts, err := parseAlternatives(id)
	if err != nil {
		return "", false
	}
	names := make([]string, len(alts))
	for i, a := range alts {
		f, ok := r.Item(a.ID)
		if !ok {
			return "", false
		}
		names[i] = f.Name
	}
	return strings.Join(names, " / "), true
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
	case KindUnlock:
		if u, ok := r.unlocks[ref.ID]; ok {
			k = u.WikiKey
		}
	case KindLocation:
		if l, ok := r.locations[ref.ID]; ok {
			k = l.WikiKey
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

// QuestIDs returns every quest id of the catalog, sorted.
func (r *Registry) QuestIDs() []string {
	out := make([]string, 0, len(r.quests))
	for id := range r.quests {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
