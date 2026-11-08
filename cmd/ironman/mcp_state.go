package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/client"
)

// accountStatePart cuts the account state into what one MCP call can carry:
// the whole state (every quest, the whole bank) is tens of thousands of
// characters, more than a client accepts as one tool result.
func accountStatePart(st client.AccountState, part, search string) (any, error) {
	search = strings.ToLower(strings.TrimSpace(search))
	match := func(s string) bool { return search == "" || strings.Contains(strings.ToLower(s), search) }
	switch part {
	case "", "summary":
		type container struct {
			Container  string    `json:"container"`
			State      string    `json:"state"`
			ObservedAt time.Time `json:"observed_at"`
			Complete   bool      `json:"complete"`
			Items      int       `json:"items"`
		}
		out := struct {
			Account     client.Account `json:"account"`
			TotalLevel  int            `json:"total_level,omitempty"`
			QuestPoints *int64         `json:"quest_points,omitempty"`
			Skills      map[string]int `json:"skills"`
			Quests      map[string]int `json:"quests"`
			Diaries     []string       `json:"diaries_complete"`
			Containers  []container    `json:"containers"`
		}{Account: st.Account, TotalLevel: st.TotalLevel, QuestPoints: st.QuestPoints,
			Skills: map[string]int{}, Quests: map[string]int{}, Diaries: []string{}, Containers: []container{}}
		for _, s := range st.Skills {
			out.Skills[s.Skill] = s.Level
		}
		for _, q := range st.Quests {
			out.Quests[q.State]++
		}
		for _, d := range st.Diaries {
			if d.Complete {
				out.Diaries = append(out.Diaries, d.Area+" "+d.Tier)
			}
		}
		for _, c := range st.Containers {
			out.Containers = append(out.Containers, container{c.Container, c.State, c.ObservedAt, c.Complete, len(c.Items)})
		}
		return out, nil
	case "skills":
		return st.Skills, nil
	case "quests":
		type quest struct {
			Name  string `json:"name"`
			State string `json:"state"`
		}
		out := []quest{}
		for _, q := range st.Quests {
			if match(q.Name) || match(q.State) {
				out = append(out, quest{q.Name, q.State})
			}
		}
		return out, nil
	case "diaries":
		return st.Diaries, nil
	case "items":
		type item struct {
			Container string `json:"container"`
			Name      string `json:"name"`
			Qty       int64  `json:"qty"`
		}
		out := []item{}
		for _, c := range st.Containers {
			for _, it := range c.Items {
				if match(it.Name) {
					out = append(out, item{c.Container, it.Name, it.Qty})
				}
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}
	return nil, fmt.Errorf("unknown part %q (summary, skills, quests, diaries, items)", part)
}
