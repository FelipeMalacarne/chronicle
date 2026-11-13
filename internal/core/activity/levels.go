// Package activity derives what an account did from its snapshots, for the
// v7's Histórico and Atividade (docs/ui/decisoes-canvas.md section 7.4):
// for now, the level-ups ("Fishing 76 · subiu de nível") and each skill's
// progress over the window ("Fishing 71 → 76 +412k"). Pure: the store reads
// the observations, this package decides what they mean.
package activity

import (
	"sort"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// Observation is a skill's XP as one snapshot saw it (any source: the
// plugin, the hiscores).
type Observation struct {
	At    time.Time
	Skill string
	XP    int
}

// LevelUp is a skill reaching a level, at the first observation that showed
// it. There is one per level: a jump of two levels between two observations
// (an account only the hiscores follow, a level-up while the plugin was off)
// is two events at the same instant.
type LevelUp struct {
	Skill string    `json:"skill"`
	Level int       `json:"level"`
	At    time.Time `json:"at"`
}

// SkillGain is a skill's progress over the window: the level it started at,
// the level it reached and the XP gained.
type SkillGain struct {
	Skill     string `json:"skill"`
	FromLevel int    `json:"from_level"`
	ToLevel   int    `json:"to_level"`
	XP        int    `json:"xp"`
}

// Levels is what a window of observations shows.
type Levels struct {
	// Events are the level-ups, newest first (same instant: by skill, then
	// the highest level first).
	Events []LevelUp
	// Skills are the skills that gained XP in the window, most XP first.
	Skills []SkillGain
}

// Compute derives the level-ups and gains of a window from each skill's
// baseline (the most XP it was seen with before the window) and the
// window's observations, in any order.
//
// XP never goes down, and the hiscores can lag the plugin: an observation
// lower than (or equal to) the most seen so far for its skill is stale and
// changes nothing. The level comes from the XP (entities.LevelForXP), never
// from a reported level. A skill's first observation, with no baseline,
// never produces an event: there is no "before" to compare with; it is the
// start of its gain instead. Negative XP (no value) is ignored.
func Compute(baseline map[string]int, window []Observation) Levels {
	best := map[string]int{}
	start := map[string]int{}
	for skill, xp := range baseline {
		if xp >= 0 {
			best[skill], start[skill] = xp, xp
		}
	}
	obs := make([]Observation, 0, len(window))
	for _, o := range window {
		if o.XP >= 0 {
			obs = append(obs, o)
		}
	}
	sort.SliceStable(obs, func(i, j int) bool { return obs[i].At.Before(obs[j].At) })

	out := Levels{Events: []LevelUp{}, Skills: []SkillGain{}}
	for _, o := range obs {
		prev, seen := best[o.Skill]
		if !seen {
			best[o.Skill], start[o.Skill] = o.XP, o.XP
			continue
		}
		if o.XP <= prev {
			continue
		}
		for lvl := entities.LevelForXP(prev) + 1; lvl <= entities.LevelForXP(o.XP); lvl++ {
			out.Events = append(out.Events, LevelUp{Skill: o.Skill, Level: lvl, At: o.At})
		}
		best[o.Skill] = o.XP
	}
	sort.Slice(out.Events, func(i, j int) bool {
		a, b := out.Events[i], out.Events[j]
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		if a.Skill != b.Skill {
			return a.Skill < b.Skill
		}
		return a.Level > b.Level
	})

	for skill, xp := range best {
		if from := start[skill]; xp > from {
			out.Skills = append(out.Skills, SkillGain{
				Skill: skill, FromLevel: entities.LevelForXP(from), ToLevel: entities.LevelForXP(xp), XP: xp - from,
			})
		}
	}
	sort.Slice(out.Skills, func(i, j int) bool {
		if out.Skills[i].XP != out.Skills[j].XP {
			return out.Skills[i].XP > out.Skills[j].XP
		}
		return out.Skills[i].Skill < out.Skills[j].Skill
	})
	return out
}
