package progress

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/state"
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

// ItemView is one item stack of a container.
type ItemView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int64  `json:"qty"`
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

// AccountState is what the account screen shows: skills and quests with
// source and age first, then the containers with their freshness. Facets the
// account never reported are empty, never guessed.
type AccountState struct {
	Account     AccountView     `json:"account"`
	TotalLevel  int             `json:"total_level,omitempty"`
	QuestPoints *int64          `json:"quest_points,omitempty"`
	Skills      []SkillView     `json:"skills"`
	Quests      []QuestView     `json:"quests"`
	Diaries     []DiaryView     `json:"diaries"`
	Containers  []ContainerView `json:"containers"`
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
	}
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
	for name, c := range ps.Items {
		cv := ContainerView{Container: name, State: string(c.State), ObservedAt: c.ObservedAt, Complete: c.Complete, Items: []ItemView{}}
		for id, qty := range c.Quantities {
			item := ItemView{ID: id, Name: strconv.Itoa(id), Qty: qty}
			if r.catalog != nil {
				if f, ok := r.catalog.Item(id); ok {
					item.Name = f.Name
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
