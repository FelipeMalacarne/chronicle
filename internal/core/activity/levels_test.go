package activity

import (
	"reflect"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func TestCompute(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	xp := entities.XPForLevel
	obs := func(m int, skill string, v int) Observation { return Observation{At: at(m), Skill: skill, XP: v} }
	up := func(m int, skill string, level int) LevelUp { return LevelUp{Skill: skill, Level: level, At: at(m)} }

	cases := []struct {
		name     string
		baseline map[string]int
		window   []Observation
		events   []LevelUp
		skills   []SkillGain
	}{
		{
			name:   "nothing",
			events: []LevelUp{}, skills: []SkillGain{},
		},
		{
			name:   "first observation has no before",
			window: []Observation{obs(0, "fishing", xp(76)+10)},
			events: []LevelUp{}, skills: []SkillGain{},
		},
		{
			name:     "one level from the baseline",
			baseline: map[string]int{"fishing": xp(76) - 100},
			window:   []Observation{obs(5, "fishing", xp(76)-40), obs(10, "fishing", xp(76)+20)},
			events:   []LevelUp{up(10, "fishing", 76)},
			skills:   []SkillGain{{Skill: "fishing", FromLevel: 75, ToLevel: 76, XP: 120}},
		},
		{
			name:     "a jump of two levels is two events at the same instant",
			baseline: map[string]int{"fishing": xp(74)},
			window:   []Observation{obs(30, "fishing", xp(76)+1)},
			events:   []LevelUp{up(30, "fishing", 76), up(30, "fishing", 75)},
			skills:   []SkillGain{{Skill: "fishing", FromLevel: 74, ToLevel: 76, XP: xp(76) + 1 - xp(74)}},
		},
		{
			// The plugin saw 76; the hiscores, a sync later, still show 75;
			// the plugin then sees 76 again and later 77.
			name:     "a lower value after a higher one is stale",
			baseline: map[string]int{"fishing": xp(75) + 5},
			window: []Observation{
				obs(10, "fishing", xp(76)+50),
				obs(15, "fishing", xp(75)+900),
				obs(20, "fishing", xp(76)+50),
				obs(40, "fishing", xp(77)),
			},
			events: []LevelUp{up(40, "fishing", 77), up(10, "fishing", 76)},
			skills: []SkillGain{{Skill: "fishing", FromLevel: 75, ToLevel: 77, XP: xp(77) - xp(75) - 5}},
		},
		{
			// A stale baseline (the hiscores behind the window's first push)
			// is still a baseline: the plugin's higher value is the level-up.
			name:     "the window's first observation of a skill with a baseline counts",
			baseline: map[string]int{"farming": xp(38) + 10},
			window:   []Observation{obs(0, "farming", xp(39))},
			events:   []LevelUp{up(0, "farming", 39)},
			skills:   []SkillGain{{Skill: "farming", FromLevel: 38, ToLevel: 39, XP: xp(39) - xp(38) - 10}},
		},
		{
			name:   "no baseline: the first observation starts the gain",
			window: []Observation{obs(0, "hunter", xp(33)+1), obs(60, "hunter", xp(34)+1)},
			events: []LevelUp{up(60, "hunter", 34)},
			skills: []SkillGain{{Skill: "hunter", FromLevel: 33, ToLevel: 34, XP: xp(34) - xp(33)}},
		},
		{
			name:     "observations in any order",
			baseline: map[string]int{"fishing": xp(70)},
			window:   []Observation{obs(20, "fishing", xp(72)), obs(10, "fishing", xp(71))},
			events:   []LevelUp{up(20, "fishing", 72), up(10, "fishing", 71)},
			skills:   []SkillGain{{Skill: "fishing", FromLevel: 70, ToLevel: 72, XP: xp(72) - xp(70)}},
		},
		{
			name:     "gain within a level is no event",
			baseline: map[string]int{"woodcutting": xp(68) + 1},
			window:   []Observation{obs(5, "woodcutting", xp(68)+5000)},
			events:   []LevelUp{},
			skills:   []SkillGain{{Skill: "woodcutting", FromLevel: 68, ToLevel: 68, XP: 4999}},
		},
		{
			name:     "negative xp is no value",
			baseline: map[string]int{"fishing": -1, "cooking": xp(50)},
			window:   []Observation{obs(0, "fishing", xp(60)), obs(5, "cooking", -1), obs(9, "fishing", -1)},
			events:   []LevelUp{}, skills: []SkillGain{},
		},
		{
			name:     "no level past 99",
			baseline: map[string]int{"attack": xp(98)},
			window:   []Observation{obs(0, "attack", 200_000_000)},
			events:   []LevelUp{up(0, "attack", 99)},
			skills:   []SkillGain{{Skill: "attack", FromLevel: 98, ToLevel: 99, XP: 200_000_000 - xp(98)}},
		},
		{
			name:     "several skills: newest first, then by skill; gains by xp",
			baseline: map[string]int{"fishing": xp(75), "farming": xp(38), "hunter": xp(33)},
			window: []Observation{
				obs(0, "fishing", xp(76)), obs(0, "farming", xp(39)),
				obs(30, "hunter", xp(34)),
			},
			events: []LevelUp{up(30, "hunter", 34), up(0, "farming", 39), up(0, "fishing", 76)},
			skills: []SkillGain{
				{Skill: "fishing", FromLevel: 75, ToLevel: 76, XP: xp(76) - xp(75)},
				{Skill: "farming", FromLevel: 38, ToLevel: 39, XP: xp(39) - xp(38)},
				{Skill: "hunter", FromLevel: 33, ToLevel: 34, XP: xp(34) - xp(33)},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Compute(c.baseline, c.window)
			if !reflect.DeepEqual(got.Events, c.events) {
				t.Errorf("events = %+v\nwant     %+v", got.Events, c.events)
			}
			if !reflect.DeepEqual(got.Skills, c.skills) {
				t.Errorf("skills = %+v\nwant     %+v", got.Skills, c.skills)
			}
		})
	}
}
