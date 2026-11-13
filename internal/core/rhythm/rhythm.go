// Package rhythm derives an account's rhythm from its snapshots, what the
// v7 shows in a lane, the account's overview and the fleet's summary
// (docs/ui/decisoes-canvas.md section 7.4, slice V5): the skill being
// trained with its XP per hour and when the next level comes, the session's
// XP and items (gained, and running out), today's XP and levels per skill,
// and the GP held with today's change.
// Pure: the store loads the inputs, this computes.
package rhythm

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

const (
	// SessionGap: a pause this long between the plugin's pushes ends a
	// session (the character logged out, or the client was off).
	SessionGap = 20 * time.Minute
	// TrainWindow: the skill being trained is the one that gained the most
	// XP in this window.
	TrainWindow = 30 * time.Minute
	// RateWindow: the XP per hour is measured over this window.
	RateWindow = time.Hour
	// Idle: with no XP gained for this long, nothing is being trained.
	Idle = 10 * time.Minute
	// minRateSpan: a rate over less time than this is noise, not shown.
	minRateSpan = 2 * time.Minute

	// CoinsID and PlatinumID are the items GP counts (a platinum token is
	// 1 000 coins), in the bank and the inventory.
	CoinsID    = 995
	PlatinumID = 13204
)

// GPContainers are the containers GP counts.
var GPContainers = []string{rules.ContainerBank, rules.ContainerInventory}

// ItemContainers are the containers a session's items count. Bank and
// inventory must both be known at both ends of the session; the equipment
// counts when it is, so that equipping an item is not a loss.
var ItemContainers = []string{rules.ContainerBank, rules.ContainerInventory, rules.ContainerEquipment}

const (
	// MaxGained and MaxRunningOut bound the session's item lists.
	MaxGained     = 10
	MaxRunningOut = 5
)

// Point is one observation of a skill's XP.
type Point struct {
	At    time.Time
	Skill string
	XP    int
}

// CoinObs is what one snapshot saw of the GP items in one container: a
// full observation is the whole container (an absent item is 0), a delta
// only the items it lists (nil is not listed).
type CoinObs struct {
	At        time.Time
	Full      bool
	Container string
	Coins     *int64
	Platinum  *int64
}

// ItemObs is what one snapshot saw of one container: a full observation is
// the whole container (an absent item is 0), a delta only the items it lists
// (0: the item left).
type ItemObs struct {
	At        time.Time
	Full      bool
	Container string
	Items     map[int]int64
}

// Input is everything Compute needs.
type Input struct {
	Now time.Time
	// Since is the start of "today" in the viewer's time zone.
	Since time.Time
	// SessionStart is when the latest session began; nil without one in
	// the last day.
	SessionStart *time.Time
	// Current is each skill's XP now (player_state).
	Current map[string]int
	// AtSince and AtSession are each skill's XP at Since and at
	// SessionStart; a missing skill did not change.
	AtSince, AtSession map[string]int
	// Recent are the XP points of the last RateWindow, oldest first.
	Recent []Point
	// Coins are the GP items' observations from the last full one of each
	// container before Since, oldest first.
	Coins []CoinObs
	// GPNow is the GP held now (GP(holdings)); nil when the bank was never
	// observed.
	GPNow *int64
	// Items are the item containers' observations from the last full one
	// of each container at or before SessionStart, oldest first.
	Items []ItemObs
	// Base maps an item id to the id its quantities add up under (a noted
	// form to its unnoted one); nil keeps each id apart.
	Base func(id int) int
}

// Rhythm is one account's rhythm.
type Rhythm struct {
	// Training is the skill being trained; nil when none gained XP lately.
	Training *Training `json:"training"`
	// Session is the latest session's XP; nil without a session today.
	Session *Session `json:"session"`
	Today   Today    `json:"today"`
	// GP is what the account holds; nil when the bank was never seen.
	GP *GP `json:"gp"`
}

// Training is the skill being trained.
type Training struct {
	Skill string `json:"skill"`
	Level int    `json:"level"`
	XP    int    `json:"xp"`
	// XPPerHour is 0 when the window is too short to measure.
	XPPerHour int `json:"xp_per_hour"`
	// NextLevelInS is how long the next level takes at that rate; absent at
	// 99 or without a rate.
	NextLevelInS *int      `json:"next_level_in_s,omitempty"`
	LastGainAt   time.Time `json:"last_gain_at"`
}

// Session is the XP and the items of the latest session.
type Session struct {
	StartedAt time.Time `json:"started_at"`
	XP        int       `json:"xp"`
	// Gained are the items held more of than when the session began (bank,
	// inventory and equipment together, so moving one is not a gain), most
	// first; RunningOut the ones it is using up, soonest out first. Both
	// null when the bank or the inventory was not known at both ends.
	Gained     []ItemCount  `json:"gained"`
	RunningOut []ItemRunOut `json:"running_out"`
}

// ItemCount is an item and how many.
type ItemCount struct {
	Item int    `json:"item"`
	Name string `json:"name,omitempty"`
	Qty  int64  `json:"qty"`
}

// ItemRunOut is an item the session is using up: how many it used, how many
// are left, and when they run out at the session's pace.
type ItemRunOut struct {
	Item int    `json:"item"`
	Name string `json:"name,omitempty"`
	Used int64  `json:"used"`
	Left int64  `json:"left"`
	InS  int    `json:"in_s"`
}

// Today is the XP and levels since Since.
type Today struct {
	Since  time.Time  `json:"since"`
	XP     int        `json:"xp"`
	Levels int        `json:"levels"`
	Skills []SkillDay `json:"skills"`
}

// SkillDay is one skill's gain today.
type SkillDay struct {
	Skill  string `json:"skill"`
	XP     int    `json:"xp"`
	Levels int    `json:"levels"`
	Level  int    `json:"level"`
}

// GP is the GP held and today's change (absent when no observation shows
// what it was at Since).
type GP struct {
	Value int64  `json:"value"`
	Today *int64 `json:"today,omitempty"`
}

// Compute derives the rhythm. XP never goes down and the hiscores may lag
// the plugin, so a lower XP observed after a higher one is stale: each
// skill's points run at the most seen so far, and the current XP is at
// least the most a point saw.
func Compute(in Input) Rhythm {
	in = monotonic(in)
	out := Rhythm{Training: training(in), Today: today(in)}
	if in.SessionStart != nil {
		out.Session = &Session{StartedAt: *in.SessionStart, XP: gained(in.Current, in.AtSession)}
		out.Session.Gained, out.Session.RunningOut = sessionItems(in)
	}
	if in.GPNow != nil {
		out.GP = &GP{Value: *in.GPNow, Today: gpToday(in)}
	}
	return out
}

// monotonic gives each skill's points (oldest first) a running maximum and
// raises the current XP to the most a point saw.
func monotonic(in Input) Input {
	seen := map[string]int{}
	recent := make([]Point, len(in.Recent))
	for i, p := range in.Recent {
		if most, ok := seen[p.Skill]; ok && most > p.XP {
			p.XP = most
		}
		seen[p.Skill] = p.XP
		recent[i] = p
	}
	current := make(map[string]int, len(in.Current))
	for skill, xp := range in.Current {
		current[skill] = xp
	}
	for skill, most := range seen {
		if most > current[skill] {
			current[skill] = most
		}
	}
	in.Recent, in.Current = recent, current
	return in
}

// GPOf counts the GP in holdings; nil when the bank was never observed.
func GPOf(h rules.ItemHoldings) *int64 {
	if c, ok := h[rules.ContainerBank]; !ok || !c.Readable() {
		return nil
	}
	var gp int64
	for _, c := range GPContainers {
		gp += h.Count(c, CoinsID, entities.CompareExact, nil) + 1000*h.Count(c, PlatinumID, entities.CompareExact, nil)
	}
	return &gp
}

func training(in Input) *Training {
	bySkill := map[string][]Point{}
	for _, p := range in.Recent {
		bySkill[p.Skill] = append(bySkill[p.Skill], p)
	}
	var (
		best     string
		bestGain int
	)
	for skill, pts := range bySkill {
		if g := gainSince(pts, in.Now.Add(-TrainWindow)); g > bestGain || (g == bestGain && g > 0 && skill < best) {
			best, bestGain = skill, g
		}
	}
	if bestGain <= 0 {
		return nil
	}
	pts := bySkill[best]
	last := pts[len(pts)-1]
	// When the XP reached its latest value: the first point that shows it.
	lastGain := last
	for _, p := range slices.Backward(pts) {
		if p.XP < last.XP {
			break
		}
		lastGain = p
	}
	if in.Now.Sub(lastGain.At) > Idle {
		return nil
	}
	xp := cmp.Or(in.Current[best], last.XP)
	t := &Training{Skill: best, Level: entities.LevelForXP(xp), XP: xp, LastGainAt: lastGain.At}
	// The rate runs from the first gain in the window to the last one: idle
	// points before the training began would slow it down. With a single
	// gain (sparse points, as from the hiscores) it runs from the point
	// before it.
	first := pts[0]
	for i, p := range pts {
		if p.XP > pts[0].XP {
			first = p
			if p.XP == lastGain.XP && i > 0 {
				first = pts[i-1]
			}
			break
		}
	}
	if span := lastGain.At.Sub(first.At); span >= minRateSpan && lastGain.XP > first.XP {
		t.XPPerHour = int(float64(lastGain.XP-first.XP) / span.Hours())
	}
	if t.XPPerHour > 0 && t.Level < entities.MaxLevel {
		s := int(float64(entities.XPForLevel(t.Level+1)-xp) / float64(t.XPPerHour) * 3600)
		t.NextLevelInS = &s
	}
	return t
}

// gainSince is the XP a skill's points (oldest first) gained after from:
// the last point's XP less the XP it had at from (the last point at or
// before from, else the first after it).
func gainSince(pts []Point, from time.Time) int {
	start := pts[0].XP
	for _, p := range pts {
		if p.At.After(from) {
			break
		}
		start = p.XP
	}
	return pts[len(pts)-1].XP - start
}

func today(in Input) Today {
	out := Today{Since: in.Since, Skills: []SkillDay{}}
	for skill, xp := range in.Current {
		before, ok := in.AtSince[skill]
		if !ok || xp <= before {
			continue
		}
		d := SkillDay{Skill: skill, XP: xp - before, Level: entities.LevelForXP(xp)}
		d.Levels = d.Level - entities.LevelForXP(before)
		out.Skills = append(out.Skills, d)
		out.XP += d.XP
		out.Levels += d.Levels
	}
	slices.SortFunc(out.Skills, func(a, b SkillDay) int { return cmp.Or(b.XP-a.XP, cmp.Compare(a.Skill, b.Skill)) })
	return out
}

// gained sums the XP each skill gained over before.
func gained(current, before map[string]int) int {
	total := 0
	for skill, xp := range current {
		if b, ok := before[skill]; ok && xp > b {
			total += xp - b
		}
	}
	return total
}

// gpToday is the GP gained since Since from the observations: each
// container's GP then (its last observation at or before Since, else its
// first full one after) and now (its last observation). Nil when no
// container has both.
func gpToday(in Input) *int64 {
	type holding struct{ coins, platinum int64 }
	apply := func(h *holding, o CoinObs) {
		if o.Full {
			*h = holding{}
		}
		if o.Coins != nil {
			h.coins = *o.Coins
		}
		if o.Platinum != nil {
			h.platinum = *o.Platinum
		}
	}
	var (
		delta int64
		known bool
	)
	for _, c := range GPContainers {
		var (
			now, then        holding
			seen, seenBefore bool
		)
		for _, o := range in.Coins {
			if o.Container != c || (!seen && !o.Full) {
				continue // a delta before any full image says nothing of the rest
			}
			apply(&now, o)
			seen = true
			if !o.At.After(in.Since) {
				then, seenBefore = now, true
			} else if !seenBefore {
				// First seen after Since: what it held then is taken as what it held at Since.
				then, seenBefore = now, true
			}
		}
		if !seen {
			continue
		}
		known = true
		delta += (now.coins + 1000*now.platinum) - (then.coins + 1000*then.platinum)
	}
	if !known {
		return nil
	}
	return &delta
}

// sessionItems compares what the account held when the session began with
// what it holds now, item by item over the containers known at both ends.
func sessionItems(in Input) ([]ItemCount, []ItemRunOut) {
	start := *in.SessionStart
	base := in.Base
	if base == nil {
		base = func(id int) int { return id }
	}
	then, now := map[int]int64{}, map[int]int64{}
	for _, c := range ItemContainers {
		before, after, ok := containerAt(in.Items, c, start)
		if !ok {
			if c == rules.ContainerEquipment {
				continue
			}
			return nil, nil
		}
		for id, q := range before {
			then[base(id)] += q
		}
		for id, q := range after {
			now[base(id)] += q
		}
	}
	gained, out := []ItemCount{}, []ItemRunOut{}
	elapsed := in.Now.Sub(start)
	for id, q := range now {
		if d := q - then[id]; d > 0 {
			gained = append(gained, ItemCount{Item: id, Qty: d})
		}
	}
	for id, q := range then {
		used, left := q-now[id], now[id]
		if used <= 0 || left <= 0 || elapsed < Idle {
			continue
		}
		out = append(out, ItemRunOut{Item: id, Used: used, Left: left, InS: int(float64(left) / float64(used) * elapsed.Seconds())})
	}
	slices.SortFunc(gained, func(a, b ItemCount) int { return cmp.Or(cmp.Compare(b.Qty, a.Qty), cmp.Compare(a.Item, b.Item)) })
	slices.SortFunc(out, func(a, b ItemRunOut) int { return cmp.Or(cmp.Compare(a.InS, b.InS), cmp.Compare(a.Item, b.Item)) })
	return gained[:min(len(gained), MaxGained)], out[:min(len(out), MaxRunningOut)]
}

// containerAt is what a container held at start and holds now; ok is false
// when no observation shows it at or before start (a container first seen
// later would count what moved before it as gained or used).
func containerAt(obs []ItemObs, container string, start time.Time) (then, now map[int]int64, ok bool) {
	for _, o := range obs {
		if o.Container != container || (now == nil && !o.Full) {
			continue // a delta before any full image says nothing of the rest
		}
		if o.Full || now == nil {
			now = map[int]int64{}
		}
		for id, q := range o.Items {
			if q > 0 {
				now[id] = q
			} else {
				delete(now, id)
			}
		}
		if !o.At.After(start) {
			then, ok = maps.Clone(now), true
		}
	}
	return then, now, ok
}
