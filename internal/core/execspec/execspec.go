// Package execspec is the typed task the phase 3 executor runs (docs/10
// sections 10 and 13): what the server sends, the agent validates and
// carries in its closed command task_run (docs/08), and the Chronicle Link
// plugin executes as a local state machine over Microbot's primitives. It
// holds game ids, tiles and numbers only, already resolved from the
// catalog by the server (internal/core/plan.Resolve); nothing here is text
// to interpret. Pure: the agent and the server share the same checks, and
// the plugin's TaskSpec mirrors the JSON, pinned by the wire fixtures both
// test suites read. A train (phase 4) is the agent's alone: it delegates to
// a plugin of the client and never goes to Chronicle Link, which keeps
// refusing it.
package execspec

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// Types the executor runs (decision E5: the first primitives). Travel,
// bank and buy run on Chronicle Link's machine (phase 3); train is
// delegated by the agent to a plugin of the client (phase 4) and never goes
// to the bridge.
const (
	TypeTravel = "travel"
	TypeBank   = "bank"
	TypeBuy    = "buy"
	TypeTrain  = "train"
)

// Types lists them.
var Types = []string{TypeTravel, TypeBank, TypeBuy, TypeTrain}

// CombatSkills are the skills trained only by fighting: a train of one of
// them is a combat risk whatever its method (docs/10, E9).
var CombatSkills = []string{"attack", "strength", "defence", "hitpoints", "ranged", "slayer"}

// Bounds of what a task may ask.
const (
	MaxPolygon  = 256 // points: Lumbridge Swamp has 102
	MaxAreaSide = 256 // tiles: a location's area, never a region
	MaxBank     = 39  // 28 inventory slots and 11 equipment slots
	MaxIDs      = 16  // candidate ids of one bank item (any_of, a family)
	MaxNPCs     = 32  // game ids of a shop's keepers
	MaxQty      = 1_000_000_000
	MaxBuy      = 10_000
	MaxName     = 80
	maxCoord    = 16_383
	maxPlane    = 3
	maxGameID   = 1 << 20
)

// Task is one typed task: its type and the parameters of that type.
type Task struct {
	Type   string  `json:"type"`
	Travel *Travel `json:"travel,omitempty"`
	Bank   *Bank   `json:"bank,omitempty"`
	Buy    *Buy    `json:"buy,omitempty"`
	Train  *Train  `json:"train,omitempty"`
}

// Tile is a world tile.
type Tile struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Plane int `json:"plane"`
}

// Area is where a location is: a box on one plane and, when the wiki draws
// one, a polygon inside it (internal/core/entities.Area without its rule).
type Area struct {
	Plane   int      `json:"plane"`
	MinX    int      `json:"minX"`
	MinY    int      `json:"minY"`
	MaxX    int      `json:"maxX"`
	MaxY    int      `json:"maxY"`
	Polygon [][2]int `json:"polygon,omitempty"`
}

// Travel walks to a location's area with the web walker.
type Travel struct {
	// Location is the catalog ref and Name its display name (the trail).
	Location string `json:"location"`
	Name     string `json:"name"`
	// Target is the tile the walker heads to, inside Area.
	Target Tile `json:"target"`
	// Area is where the walk ends: standing inside it.
	Area Area `json:"area"`
	// Wilderness: the destination touches the wilderness, so the walk may
	// enter it (a declared risk); otherwise the executor stops a walk that
	// enters it.
	Wilderness bool `json:"wilderness"`
}

// Bank opens the nearest bank and leaves the inventory and the equipment
// with the items, depositing what is not one of them.
type Bank struct {
	Items []BankItem `json:"items"`
}

// BankItem is one item to carry: any one of IDs (unnoted ids, preferred
// first), Qty of it, noted or worn.
type BankItem struct {
	Name     string `json:"name"`
	IDs      []int  `json:"ids"`
	Qty      int    `json:"qty"`
	Noted    bool   `json:"noted,omitempty"`
	Equip    bool   `json:"equip,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

// Buy trades with a shop's keeper nearby until Qty more of the item are in
// the inventory.
type Buy struct {
	Shop     string `json:"shop"`
	ShopName string `json:"shopName"`
	// NPCs are the game ids of the shop's keepers (every version).
	NPCs     []int  `json:"npcs"`
	Item     int    `json:"item"`
	ItemName string `json:"itemName"`
	Qty      int    `json:"qty"`
}

// Train delegates a skill's training to a plugin of the client (docs/10
// phase 4): the agent sets the plugin's settings, starts it and stops it
// when the server proves Until from the ingest; the plugin decides how to
// train, never when it is done.
type Train struct {
	// Skill is the catalog ref ("skill:fishing") and SkillName its name as
	// the client shows it ("Fishing").
	Skill     string `json:"skill"`
	SkillName string `json:"skillName"`
	// Level is the N of the termination skill:<skill>>=N.
	Level int `json:"level"`
	// Plugin is the plugin's fully qualified class, Config the settings it
	// starts with.
	Plugin string              `json:"plugin"`
	Config []clientcfg.Setting `json:"config,omitempty"`
	// Combat: the method fights (a monster, or a skill trained only by
	// fighting), a death risk (E9).
	Combat bool `json:"combat,omitempty"`
}

// Until is the train's termination, the objective key the server proves.
func (t Train) Until() string { return t.Skill + ">=" + strconv.Itoa(t.Level) }

var (
	reRef   = regexp.MustCompile(`^(location|shop):[a-z0-9_.'-]{1,120}$`)
	reSkill = regexp.MustCompile(`^skill:[a-z]{1,40}$`)
)

// Check verifies a task: a type the executor runs, its parameters only,
// tiles in the world, an area of a location (not a region), a target
// inside it, the wilderness flag as the area says, bounded lists and
// positive ids and quantities. A type outside the phase 3 list is refused
// with a clear error.
func (t Task) Check() error {
	set := 0
	for _, p := range []bool{t.Travel != nil, t.Bank != nil, t.Buy != nil, t.Train != nil} {
		if p {
			set++
		}
	}
	switch t.Type {
	case TypeTravel:
		if set != 1 || t.Travel == nil {
			return errors.New("task: a travel has only travel")
		}
		return t.Travel.check()
	case TypeBank:
		if set != 1 || t.Bank == nil {
			return errors.New("task: a bank has only bank")
		}
		return t.Bank.check()
	case TypeBuy:
		if set != 1 || t.Buy == nil {
			return errors.New("task: a buy has only buy")
		}
		return t.Buy.check()
	case TypeTrain:
		if set != 1 || t.Train == nil {
			return errors.New("task: a train has only train")
		}
		return t.Train.check()
	case content.TaskTalk, content.TaskInteract, content.TaskManual, content.TaskQuest:
		return fmt.Errorf("task: %s is not executed yet (the executor runs travel, bank, buy and train)", t.Type)
	}
	return fmt.Errorf("task: unknown type %q", t.Type)
}

func (v *Travel) check() error {
	if !reRef.MatchString(v.Location) || !strings.HasPrefix(v.Location, "location:") {
		return errors.New("travel: location must be a location ref")
	}
	if err := checkName("travel: name", v.Name); err != nil {
		return err
	}
	a := v.Area
	switch {
	case a.Plane < 0 || a.Plane > maxPlane:
		return errors.New("travel: area plane out of range")
	case a.MinX < 0 || a.MinY < 0 || a.MaxX > maxCoord || a.MaxY > maxCoord || a.MinX > a.MaxX || a.MinY > a.MaxY:
		return errors.New("travel: area box out of the world or empty")
	case a.MaxX-a.MinX >= MaxAreaSide || a.MaxY-a.MinY >= MaxAreaSide:
		return fmt.Errorf("travel: an area is at most %d tiles across", MaxAreaSide)
	case len(a.Polygon) > MaxPolygon || (len(a.Polygon) > 0 && len(a.Polygon) < 3):
		return fmt.Errorf("travel: a polygon has 3 to %d points", MaxPolygon)
	}
	for _, p := range a.Polygon {
		if p[0] < a.MinX || p[0] > a.MaxX || p[1] < a.MinY || p[1] > a.MaxY {
			return errors.New("travel: a polygon point is outside the box")
		}
	}
	if !a.Contains(v.Target.X, v.Target.Y, v.Target.Plane) {
		return errors.New("travel: the target is not inside the area")
	}
	if v.Wilderness != AreaInWilderness(a) {
		return errors.New("travel: the wilderness flag does not match the area")
	}
	return nil
}

func (b *Bank) check() error {
	if len(b.Items) == 0 || len(b.Items) > MaxBank {
		return fmt.Errorf("bank: 1 to %d items", MaxBank)
	}
	required := 0
	for i, it := range b.Items {
		if err := checkName(fmt.Sprintf("bank: items/%d name", i), it.Name); err != nil {
			return err
		}
		if len(it.IDs) == 0 || len(it.IDs) > MaxIDs {
			return fmt.Errorf("bank: items/%d has 1 to %d ids", i, MaxIDs)
		}
		for j, id := range it.IDs {
			if id <= 0 || id >= maxGameID || slices.Contains(it.IDs[:j], id) {
				return fmt.Errorf("bank: items/%d: bad or repeated id %d", i, id)
			}
		}
		if it.Qty < 1 || it.Qty > MaxQty {
			return fmt.Errorf("bank: items/%d: qty 1 to %d", i, MaxQty)
		}
		if it.Noted && it.Equip {
			return fmt.Errorf("bank: items/%d: a worn item is not noted", i)
		}
		if !it.Optional {
			required++
		}
	}
	if required == 0 {
		return errors.New("bank: at least one item is required (not optional)")
	}
	return nil
}

func (b *Buy) check() error {
	if !reRef.MatchString(b.Shop) || !strings.HasPrefix(b.Shop, "shop:") {
		return errors.New("buy: shop must be a shop ref")
	}
	if err := checkName("buy: shopName", b.ShopName); err != nil {
		return err
	}
	if err := checkName("buy: itemName", b.ItemName); err != nil {
		return err
	}
	if len(b.NPCs) == 0 || len(b.NPCs) > MaxNPCs {
		return fmt.Errorf("buy: 1 to %d shopkeeper ids", MaxNPCs)
	}
	for i, id := range b.NPCs {
		if id < 0 || id >= maxGameID || slices.Contains(b.NPCs[:i], id) {
			return fmt.Errorf("buy: bad or repeated shopkeeper id %d", id)
		}
	}
	if b.Item <= 0 || b.Item >= maxGameID {
		return errors.New("buy: bad item id")
	}
	if b.Qty < 1 || b.Qty > MaxBuy {
		return fmt.Errorf("buy: qty 1 to %d", MaxBuy)
	}
	return nil
}

func (t *Train) check() error {
	if !reSkill.MatchString(t.Skill) {
		return errors.New("train: skill must be a skill ref")
	}
	if err := checkName("train: skillName", t.SkillName); err != nil {
		return err
	}
	if t.Level < 1 || t.Level > entities.MaxSkillLevel {
		return fmt.Errorf("train: level 1 to %d", entities.MaxSkillLevel)
	}
	if err := clientcfg.CheckClass(t.Plugin); err != nil {
		return fmt.Errorf("train: %w", err)
	}
	if err := clientcfg.CheckSettings(t.Config); err != nil {
		return fmt.Errorf("train: %w", err)
	}
	if slices.Contains(CombatSkills, strings.TrimPrefix(t.Skill, "skill:")) && !t.Combat {
		return fmt.Errorf("train: %s is trained by fighting, so combat must be set", t.Skill)
	}
	return nil
}

// checkName refuses an empty, long or multi-line name.
func checkName(what, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", what)
	}
	if len(s) > MaxName || strings.IndexFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return fmt.Errorf("%s: at most %d printable characters, one line", what, MaxName)
	}
	return nil
}

// Contains reports whether the tile is inside the area: in the box and,
// with a polygon, in it or on its edge (as entities.Area.Contains).
func (a Area) Contains(x, y, plane int) bool {
	if plane != a.Plane || x < a.MinX || x > a.MaxX || y < a.MinY || y > a.MaxY {
		return false
	}
	if len(a.Polygon) < 3 {
		return true
	}
	inside := false
	n := len(a.Polygon)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := a.Polygon[i][0], a.Polygon[i][1]
		xj, yj := a.Polygon[j][0], a.Polygon[j][1]
		if onSegment(x, y, xi, yi, xj, yj) {
			return true
		}
		if (yi > y) != (yj > y) {
			lhs := (x - xi) * (yj - yi)
			rhs := (xj - xi) * (y - yi)
			if (yj > yi && lhs < rhs) || (yj < yi && lhs > rhs) {
				inside = !inside
			}
		}
	}
	return inside
}

func onSegment(x, y, x1, y1, x2, y2 int) bool {
	if (x-x1)*(y2-y1) != (x2-x1)*(y-y1) {
		return false
	}
	return min(x1, x2) <= x && x <= max(x1, x2) && min(y1, y2) <= y && y <= max(y1, y2)
}

// Risks returns the risk kinds of the task itself, sorted: wilderness for
// a travel whose area touches it, combat for a train that fights. The
// server adds the recipe's (and the wilderness of a train's location).
func (t Task) Risks() []string {
	if t.Travel != nil && AreaInWilderness(t.Travel.Area) {
		return []string{content.RiskWilderness}
	}
	if t.Train != nil && t.Train.Combat {
		return []string{content.RiskCombat}
	}
	return nil
}
