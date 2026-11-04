package entities

import "math"

// MaxLevel is the highest skill level reached through experience.
const MaxLevel = 99

// xpTable[n] is the experience needed for level n (index 0 unused), from the
// game's formula: sum over l < n of floor(l + 300 * 2^(l/7)), divided by 4.
var xpTable = func() [MaxLevel + 1]int {
	var t [MaxLevel + 1]int
	points := 0
	for lvl := 1; lvl < MaxLevel; lvl++ {
		points += int(math.Floor(float64(lvl) + 300*math.Pow(2, float64(lvl)/7)))
		t[lvl+1] = points / 4
	}
	return t
}()

// XPForLevel returns the experience needed for a level (1..99).
func XPForLevel(level int) int {
	if level <= 1 {
		return 0
	}
	return xpTable[min(level, MaxLevel)]
}

// LevelForXP returns the level an amount of experience gives (1..99).
func LevelForXP(xp int) int {
	lvl := 1
	for lvl < MaxLevel && xpTable[lvl+1] <= xp {
		lvl++
	}
	return lvl
}

// StartingXP is the experience a new account has in a skill: Hitpoints
// starts at level 10, every other skill at level 1.
func StartingXP(skill string) int {
	if skill == "hitpoints" {
		return XPForLevel(10)
	}
	return 0
}
