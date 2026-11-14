// Package mta computes what the Mage Training Arena rewards an account chose cost in runes:
// the pizazz points still missing in each of the four rooms and the law, nature and cosmic
// runes that earning them takes. Pure: the table is mta.json (written by tools/extract_mta.py
// from the OSRS Wiki), and the Microbot plugin fm/mtaarena carries the same table and the same
// arithmetic, pinned by testdata/vectors.json (copied to the plugin's tests; TestVectorsCopy).
package mta

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

//go:embed mta.json
var tableJSON []byte

// Points is an amount of pizazz points in each room.
type Points struct {
	Telekinetic int `json:"telekinetic"`
	Graveyard   int `json:"graveyard"`
	Enchantment int `json:"enchantment"`
	Alchemist   int `json:"alchemist"`
}

// Runes are the runes a room spends, summed. Staves are assumed for the elemental ones.
type Runes struct {
	Law    int `json:"law"`
	Nature int `json:"nature"`
	Cosmic int `json:"cosmic"`
}

// Reward is one item of the Rewards Guardian's stock.
type Reward struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	ItemID int    `json:"item_id"`
	Cost   Points `json:"cost"`
	// Requires is the reward whose item the purchase turns in (the wands upgrade the one below).
	Requires string `json:"requires,omitempty"`
}

// Room is a room's average rune spend per point and the points a player can hold.
type Room struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Rune          string `json:"rune"`
	RunesPerPoint struct {
		Num int `json:"num"`
		Den int `json:"den"`
	} `json:"runes_per_point"`
	Cap int `json:"cap"`
}

// EnchantLevel is a Lvl-N Enchant and the Magic level that casts it.
type EnchantLevel struct {
	Level int `json:"level"`
	Magic int `json:"magic"`
}

// Table is mta.json.
type Table struct {
	Source        string         `json:"source"`
	Rooms         []Room         `json:"rooms"`
	EnchantLevels []EnchantLevel `json:"enchant_levels"`
	Rewards       []Reward       `json:"rewards"`
}

var (
	table   Table
	rewards = map[string]Reward{}
)

func init() {
	if err := json.Unmarshal(tableJSON, &table); err != nil {
		panic(fmt.Sprintf("mta: mta.json: %v", err))
	}
	for _, r := range table.Rewards {
		rewards[r.ID] = r
	}
}

// Data is the reward table, as extracted from the wiki.
func Data() Table { return table }

// Input is what the calculator needs.
type Input struct {
	// Rewards the owner wants, by id.
	Rewards []string `json:"rewards"`
	// Owned are the rewards the account already has; a wand also covers the tiers below it.
	Owned []string `json:"owned"`
	// Points are the pizazz points held now.
	Points Points `json:"points"`
	// Magic is the Magic level; it picks the best Enchant for the Enchanting Chamber.
	Magic int `json:"magic"`
	// EnchantLevel (1 to 6) overrides the one Magic implies, e.g. to plan with a cheaper spell.
	EnchantLevel int `json:"enchant_level"`
}

// Result is the plan: what to buy, the points it takes and the runes it costs.
type Result struct {
	// Buy lists the purchases in order, upgrade prerequisites first.
	Buy []string `json:"buy"`
	// Needed is the total of Buy; Missing is what is left after the points held.
	Needed  Points `json:"needed"`
	Missing Points `json:"missing"`
	Runes   Runes  `json:"runes"`
	// EnchantLevel is the Enchant the cosmic runes assume (0 when the chamber is not needed).
	EnchantLevel int `json:"enchant_level"`
}

// ErrUnknownReward: a reward id the table does not have.
var ErrUnknownReward = errors.New("mta: unknown reward")

// ErrInvalidInput: a negative point amount, Magic level or Enchant level.
var ErrInvalidInput = errors.New("mta: points, Magic and Enchant level cannot be negative")

// ErrNoEnchant: the Enchanting Chamber has points to earn but the Magic level (or the chosen
// level) casts no Enchant.
var ErrNoEnchant = errors.New("mta: the Enchanting Chamber needs Magic 7 or an Enchant level")

// Calculate prices the chosen rewards. An unknown reward id is an error; a point amount already
// held beyond what is needed counts as zero missing, never as a credit to another room.
func Calculate(in Input) (Result, error) {
	if in.Points.Telekinetic < 0 || in.Points.Graveyard < 0 || in.Points.Enchantment < 0 || in.Points.Alchemist < 0 ||
		in.Magic < 0 || in.EnchantLevel < 0 {
		return Result{}, ErrInvalidInput
	}
	owned := map[string]bool{}
	for _, id := range in.Owned {
		if _, ok := rewards[id]; !ok {
			return Result{}, fmt.Errorf("%w %q", ErrUnknownReward, id)
		}
		// A wand upgrade turned in the one below it: owning one means owning the tiers under it.
		for ; id != ""; id = rewards[id].Requires {
			owned[id] = true
		}
	}
	var res Result
	res.Buy = []string{}
	var add func(id string)
	add = func(id string) {
		if owned[id] || slices.Contains(res.Buy, id) {
			return
		}
		r := rewards[id]
		if r.Requires != "" {
			add(r.Requires)
		}
		res.Buy = append(res.Buy, id)
	}
	for _, id := range in.Rewards {
		if _, ok := rewards[id]; !ok {
			return Result{}, fmt.Errorf("%w %q", ErrUnknownReward, id)
		}
		add(id)
	}
	for _, id := range res.Buy {
		c := rewards[id].Cost
		res.Needed.Telekinetic += c.Telekinetic
		res.Needed.Graveyard += c.Graveyard
		res.Needed.Enchantment += c.Enchantment
		res.Needed.Alchemist += c.Alchemist
	}
	res.Missing = Points{
		Telekinetic: missing(res.Needed.Telekinetic, in.Points.Telekinetic),
		Graveyard:   missing(res.Needed.Graveyard, in.Points.Graveyard),
		Enchantment: missing(res.Needed.Enchantment, in.Points.Enchantment),
		Alchemist:   missing(res.Needed.Alchemist, in.Points.Alchemist),
	}
	if res.Missing.Enchantment > 0 {
		res.EnchantLevel = enchantLevel(in)
		if res.EnchantLevel == 0 {
			return Result{}, ErrNoEnchant
		}
	}
	res.Runes = Runes{
		Law:    perPoint("telekinetic", res.Missing.Telekinetic, 1),
		Nature: perPoint("graveyard", res.Missing.Graveyard, 1) + perPoint("alchemist", res.Missing.Alchemist, 1),
		Cosmic: perPoint("enchantment", res.Missing.Enchantment, res.EnchantLevel),
	}
	return res, nil
}

func missing(needed, held int) int { return max(0, needed-held) }

// enchantLevel is the explicit level when given and valid, else the best one Magic casts.
func enchantLevel(in Input) int {
	if in.EnchantLevel > 0 {
		if slices.ContainsFunc(table.EnchantLevels, func(e EnchantLevel) bool { return e.Level == in.EnchantLevel }) {
			return in.EnchantLevel
		}
		return 0
	}
	best := 0
	for _, e := range table.EnchantLevels {
		if in.Magic >= e.Magic {
			best = max(best, e.Level)
		}
	}
	return best
}

// perPoint is ceil(points * num / (den * divisor)) for the room, in whole runes.
func perPoint(room string, points, divisor int) int {
	if points == 0 {
		return 0
	}
	for _, r := range table.Rooms {
		if r.ID == room {
			den := r.RunesPerPoint.Den * divisor
			return (points*r.RunesPerPoint.Num + den - 1) / den
		}
	}
	panic("mta: no room " + room)
}
