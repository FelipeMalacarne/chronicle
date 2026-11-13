package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	"github.com/FelipeMalacarne/chronicle/internal/store/state"
)

// FactSource is where a fact came from and when it was observed.
type FactSource struct {
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
}

// SkillView is one skill of the account (the Conta, V6Conta1440).
type SkillView struct {
	Skill string `json:"skill"`
	Level int    `json:"level"`
	XP    int64  `json:"xp"`
	FactSource
}

// QuestView is one quest the account reported, with its canonical name.
type QuestView struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	State string `json:"state"`
	FactSource
}

// DiaryView is one diary tier the account reported.
type DiaryView struct {
	Area     string `json:"area"`
	Tier     string `json:"tier"`
	Complete bool   `json:"complete"`
	FactSource
}

// ItemView is one item stack of a container. Slot is the item's equipment
// slot from the catalog ("head", "2h", ...): an item fits one slot only, so
// the equipment container needs no slot positions from the plugin.
type ItemView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int64  `json:"qty"`
	Slot string `json:"slot,omitempty"`
}

// ContainerView is one item container with its freshness (docs/04 section
// 6.4): live, restored (a mirror, never live) or unavailable.
type ContainerView struct {
	Container  string     `json:"container"`
	State      string     `json:"state"`
	ObservedAt time.Time  `json:"observed_at"`
	Complete   bool       `json:"complete"`
	Items      []ItemView `json:"items"`
}

// CATierView is one combat achievement tier: how many of the catalog's tasks
// the account has done, from each task's bit in the CA_TASK_COMPLETED_<n>
// varps, and the game's own total of the tier (CA_TOTAL_TASKS_COMPLETED_<tier>),
// which should agree.
type CATierView struct {
	Tier     string `json:"tier"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	GameDone *int64 `json:"game_done,omitempty"`
	FactSource
}

// CollectionLogView is the game's collection log count (COLLECTION_COUNT and
// COLLECTION_COUNT_MAX), which the game keeps in varps; the items of each page
// come only from the pages the player opened (CollectionLogPageView).
type CollectionLogView struct {
	Obtained int64  `json:"obtained"`
	Total    *int64 `json:"total,omitempty"`
	FactSource
}

// CollectionLogPageView is one collection log page as the plugin last read it
// with the log open in game: the slots it showed, obtained or not. A page the
// player never opened is absent, so its items stay unknown, never missing.
type CollectionLogPageView struct {
	Page string `json:"page"`
	// Tab is BOSSES, RAIDS, CLUES, MINIGAMES or OTHER; empty when the plugin
	// could not tell.
	Tab      string `json:"tab,omitempty"`
	Obtained int    `json:"obtained"`
	Total    int    `json:"total"`
	// Items are the obtained items with how many, Missing the slots the page
	// showed as not obtained yet; both by item id.
	Items   []CollectionLogItemView `json:"items"`
	Missing []CollectionLogItemView `json:"missing"`
	FactSource
}

// CollectionLogItemView is one slot of a collection log page.
type CollectionLogItemView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int64  `json:"qty,omitempty"`
}

// AccountState is what the account screen shows: skills and quests with
// source and age first, then the containers with their freshness. Facets the
// account never reported are empty, never guessed.
type AccountState struct {
	Account     AccountView `json:"account"`
	TotalLevel  int         `json:"total_level,omitempty"`
	QuestPoints *int64      `json:"quest_points,omitempty"`
	Skills      []SkillView `json:"skills"`
	Quests      []QuestView `json:"quests"`
	Diaries     []DiaryView `json:"diaries"`
	// CombatAchievements is empty until the plugin sends the CA varps.
	CombatAchievements []CATierView       `json:"combat_achievements"`
	CollectionLog      *CollectionLogView `json:"collection_log,omitempty"`
	// CollectionLogPages are the collection log pages the plugin read, by
	// tab and title; empty until the player opens the log in game.
	CollectionLogPages []CollectionLogPageView `json:"collection_log_pages"`
	Containers         []ContainerView         `json:"containers"`
}

// questPointsVarp is the varp the game keeps the quest point total in.
const questPointsVarp = 101

// AccountState reads the account's merged player state.
func (r *Repository) AccountState(ctx context.Context, accountID string) (AccountState, error) {
	acc, err := r.queries.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountState{}, ErrAccountNotFound
		}
		return AccountState{}, fmt.Errorf("get account: %w", err)
	}
	ps, obs, err := state.Load(ctx, r.queries, accountID, r.catalog)
	if err != nil {
		return AccountState{}, err
	}
	out := AccountState{
		Account: accountView(acc),
		Skills:  []SkillView{}, Quests: []QuestView{}, Diaries: []DiaryView{}, Containers: []ContainerView{},
		CombatAchievements: []CATierView{},
	}
	pages, err := r.queries.ListPlayerStateByFacet(ctx, storedb.ListPlayerStateByFacetParams{AccountID: accountID, Facet: coreingest.FacetCollectionLog})
	if err != nil {
		return AccountState{}, fmt.Errorf("list collection log: %w", err)
	}
	out.CollectionLogPages = collectionLogPages(r.catalog, pages)
	src := func(facet, key string) FactSource {
		o := obs[coreprogress.Fact{Facet: facet, Key: key}]
		return FactSource{Source: o.Source, ObservedAt: o.ObservedAt}
	}
	sum := 0
	for skill, s := range ps.Skills {
		if skill == "overall" {
			out.TotalLevel = s.Level
			continue
		}
		sum += s.Level
		out.Skills = append(out.Skills, SkillView{Skill: skill, Level: s.Level, XP: s.XP, FactSource: src("skill", skill)})
	}
	if out.TotalLevel == 0 {
		out.TotalLevel = sum
	}
	sort.Slice(out.Skills, func(i, j int) bool { return out.Skills[i].Skill < out.Skills[j].Skill })
	// Quest facts are keyed by the canonical quest name (the normalizer resolves
	// the RuneLite name); the id is the catalog's, for the entity panel.
	for name, st := range ps.Quests {
		id := ""
		if r.catalog != nil {
			if res := r.catalog.ResolveQuest(name); res.Status == entities.Resolved {
				id = res.Ref.ID
			}
		}
		out.Quests = append(out.Quests, QuestView{ID: id, Name: name, State: st, FactSource: src("quest", name)})
	}
	sort.Slice(out.Quests, func(i, j int) bool { return out.Quests[i].Name < out.Quests[j].Name })
	for area, tiers := range ps.Diaries {
		for tier, done := range tiers {
			out.Diaries = append(out.Diaries, DiaryView{Area: area, Tier: tier, Complete: done, FactSource: src("diary", area+":"+tier)})
		}
	}
	sort.Slice(out.Diaries, func(i, j int) bool {
		if out.Diaries[i].Area != out.Diaries[j].Area {
			return out.Diaries[i].Area < out.Diaries[j].Area
		}
		return tierRank(out.Diaries[i].Tier) < tierRank(out.Diaries[j].Tier)
	})
	if qp, ok := ps.Varps[questPointsVarp]; ok {
		out.QuestPoints = &qp
	}
	if r.catalog != nil {
		out.CombatAchievements = caTiers(r.catalog, ps.Varps, ps.Varbits, src)
		out.CollectionLog = collectionLog(r.catalog, ps.Varps, src)
	}
	for name, c := range ps.Items {
		cv := ContainerView{Container: name, State: string(c.State), ObservedAt: c.ObservedAt, Complete: c.Complete, Items: []ItemView{}}
		for id, qty := range c.Quantities {
			item := ItemView{ID: id, Name: strconv.Itoa(id), Qty: qty}
			if r.catalog != nil {
				if f, ok := r.catalog.Item(id); ok {
					item.Name = f.Name
					item.Slot = f.EquipmentSlot
				}
			}
			cv.Items = append(cv.Items, item)
		}
		sort.Slice(cv.Items, func(i, j int) bool {
			if cv.Items[i].Qty != cv.Items[j].Qty {
				return cv.Items[i].Qty > cv.Items[j].Qty
			}
			return cv.Items[i].ID < cv.Items[j].ID
		})
		out.Containers = append(out.Containers, cv)
	}
	sort.Slice(out.Containers, func(i, j int) bool {
		return containerRank(out.Containers[i].Container) < containerRank(out.Containers[j].Container)
	})
	return out, nil
}

func tierRank(t string) int {
	switch t {
	case "easy":
		return 0
	case "medium":
		return 1
	case "hard":
		return 2
	case "elite":
		return 3
	}
	return 4
}

func containerRank(c string) int {
	switch c {
	case "inventory":
		return 0
	case "equipment":
		return 1
	case "bank":
		return 2
	}
	return 3
}

// caTiers counts the done combat achievements per tier, in tier order. A tier
// counts only when every varp holding its tasks was observed.
func caTiers(reg *entities.Registry, varps, varbits map[int]int64, src func(facet, key string) FactSource) []CATierView {
	type tally struct {
		done, total int
		seen        bool
		varp        int
	}
	by := map[string]*tally{}
	var order []string
	for _, c := range reg.CombatAchievements() {
		t := by[c.Tier]
		if t == nil {
			t = &tally{seen: true}
			by[c.Tier] = t
			order = append(order, c.Tier)
		}
		t.total++
		if c.Proof == nil || c.Proof.Bit == nil {
			t.seen = false
			continue
		}
		v, ok := varps[c.Proof.ID]
		if !ok {
			t.seen = false
			continue
		}
		t.varp = c.Proof.ID
		if v&(1<<*c.Proof.Bit) != 0 {
			t.done++
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return caTierRank(order[i]) < caTierRank(order[j]) })
	out := []CATierView{}
	for _, tier := range order {
		t := by[tier]
		if !t.seen {
			continue
		}
		view := CATierView{Tier: tier, Done: t.done, Total: t.total, FactSource: src("varp", strconv.Itoa(t.varp))}
		for _, v := range reg.VarsFor(entities.CountSubject + "ca:" + tier) {
			if n, ok := varbits[v.ID]; ok && v.Var == entities.Varbit {
				view.GameDone = &n
			}
		}
		out = append(out, view)
	}
	return out
}

func caTierRank(t string) int {
	for i, x := range []string{"easy", "medium", "hard", "elite", "master", "grandmaster"} {
		if x == t {
			return i
		}
	}
	return 6
}

// collectionLog reads the game's collection log counts, nil until observed.
func collectionLog(reg *entities.Registry, varps map[int]int64, src func(facet, key string) FactSource) *CollectionLogView {
	var out *CollectionLogView
	for _, v := range reg.VarsFor(entities.CountSubject + "collection_log") {
		if n, ok := varps[v.ID]; ok && v.Var == entities.Varp {
			out = &CollectionLogView{Obtained: n, FactSource: src("varp", strconv.Itoa(v.ID))}
		}
	}
	if out == nil {
		return nil
	}
	for _, v := range reg.VarsFor(entities.CountSubject + "collection_log_max") {
		if n, ok := varps[v.ID]; ok && v.Var == entities.Varp && n > 0 {
			out.Total = &n
		}
	}
	return out
}

// collectionLogPages reads the page facts of the collection_log facet (key
// coreingest.CollectionLogPageKey) into the account screen's pages, ordered
// by tab (the game's order, unknown last) and title. Slot facts keyed by item
// id (the collectionLogSlots bitset) are not pages and are skipped, as is a
// value that does not decode.
func collectionLogPages(reg *entities.Registry, rows []storedb.PlayerState) []CollectionLogPageView {
	out := []CollectionLogPageView{}
	for _, row := range rows {
		title, ok := strings.CutPrefix(row.Key, coreingest.CollectionLogPagePrefix)
		if !ok {
			continue
		}
		var v struct {
			Tab   string           `json:"tab"`
			Items map[string]int64 `json:"items"`
		}
		if json.Unmarshal(row.Value, &v) != nil {
			continue
		}
		page := CollectionLogPageView{
			Page: title, Tab: v.Tab, Items: []CollectionLogItemView{}, Missing: []CollectionLogItemView{},
			FactSource: FactSource{Source: row.Source, ObservedAt: row.ObservedAt.Time},
		}
		for rawID, qty := range v.Items {
			id, err := strconv.Atoi(rawID)
			if err != nil || id <= 0 || qty < 0 {
				continue
			}
			item := CollectionLogItemView{ID: id, Name: rawID}
			if reg != nil {
				if f, found := reg.Item(id); found {
					item.Name = f.Name
				}
			}
			page.Total++
			if qty == 0 {
				page.Missing = append(page.Missing, item)
				continue
			}
			item.Qty = qty
			page.Obtained++
			page.Items = append(page.Items, item)
		}
		byID := func(items []CollectionLogItemView) func(i, j int) bool {
			return func(i, j int) bool { return items[i].ID < items[j].ID }
		}
		sort.Slice(page.Items, byID(page.Items))
		sort.Slice(page.Missing, byID(page.Missing))
		out = append(out, page)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := collectionLogTabRank(out[i].Tab), collectionLogTabRank(out[j].Tab); a != b {
			return a < b
		}
		return out[i].Page < out[j].Page
	})
	return out
}

func collectionLogTabRank(tab string) int {
	for i, t := range coreingest.CollectionLogTabs {
		if t == tab {
			return i
		}
	}
	return len(coreingest.CollectionLogTabs)
}
