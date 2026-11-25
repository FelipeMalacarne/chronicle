// Package execspec is the typed task the phase 3 executor runs (docs/10
// sections 10 and 13): what the server sends, the agent validates and
// carries in its closed command task_run (docs/08), and the Chronicle Link
// plugin executes as a local state machine over Microbot's primitives. It
// holds game ids, tiles and numbers only, already resolved from the
// catalog by the server (internal/core/plan.Resolve); nothing here is text
// to interpret. Pure: the agent and the server share the same checks, and
// the plugin's TaskSpec mirrors the JSON, pinned by the wire fixtures both
// test suites read. A train (phase 4), a quest (E19) and the Grand Exchange's
// ge_buy and ge_sell (docs/12 section 6) are the agent's alone: they delegate
// to the client (a train's plugin, the client's Quest Helper, the Agent
// Server's /ge) and never go to Chronicle Link, which keeps refusing them.
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

// TypePlugin is a job's task (docs/12 section 13): turn a plugin of the
// client's profile on with its settings and keep it on until the server
// proves the job's goal. It is not a content task type: no guide step or
// recipe has one, only a job of kind plugin starts it.
const TypePlugin = "plugin"

// Types the executor runs (decision E5: the first primitives). Travel,
// bank and buy run on Chronicle Link's machine (phase 3); train is
// delegated by the agent to a plugin of the client (phase 4), quest to the
// client's Quest Helper (E19) and ge_buy and ge_sell to the Agent Server's
// Grand Exchange (docs/12 section 6); none of these goes to the bridge.
const (
	TypeTravel = content.TaskTravel
	TypeBank   = content.TaskBank
	TypeBuy    = content.TaskBuy
	TypeTrain  = content.TaskTrain
	TypeQuest  = content.TaskQuest
	TypeGEBuy  = content.TaskGEBuy
	TypeGESell = content.TaskGESell
)

// Types lists them.
var Types = []string{TypeTravel, TypeBank, TypeBuy, TypeTrain, TypeQuest, TypeGEBuy, TypeGESell}

// QuestRisks are the risk kinds of a quest the owner did not confirm safe:
// a quest can fight and can have hazards, so it is a death risk (E9: in
// HCIM it runs only with the owner watching).
var QuestRisks = []string{content.RiskCombat, content.RiskHazard}

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
	// MaxCoins is the most one offer may move: the GE's coin stack.
	MaxCoins  = 2_147_483_647
	MaxName   = 80
	maxCoord  = 16_383
	maxPlane  = 3
	maxGameID = 1 << 20
)

// Task is one typed task: its type and the parameters of that type.
type Task struct {
	Type   string  `json:"type"`
	Travel *Travel `json:"travel,omitempty"`
	Bank   *Bank   `json:"bank,omitempty"`
	Buy    *Buy    `json:"buy,omitempty"`
	Train  *Train  `json:"train,omitempty"`
	Quest  *Quest  `json:"quest,omitempty"`
	// GEBuy buys Qty of the item; its termination is the content's
	// item:<id>x<qty> (the account's holdings prove the item).
	GEBuy  *GEOffer `json:"geBuy,omitempty"`
	GESell *GESell  `json:"geSell,omitempty"`
	Plugin *Plugin  `json:"plugin,omitempty"`
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

// Quest delegates a whole quest to the Quest Helper the Microbot client
// ships (docs/10, E19): the agent sets its "Obtain Missing Items" to No,
// selects the quest through the Agent Server (/quest-helper/start) and lets
// the helper's bot do it; the run ends when the server proves Until from the
// ingest. The bot decides how; never the bot nor the agent says when it is
// done.
type Quest struct {
	// Quest is the catalog ref ("quest:cooks_assistant") and Name its
	// display name (the trail).
	Quest string `json:"quest"`
	Name  string `json:"name"`
	// Helper is the quest's name as the client's Quest Helper lists it
	// (/quest-helper/list: RuneLite's quest name, which /quest-helper/start
	// takes). The server matches it to Quest on the client's own list
	// before the run; the agent finds it on the list again before starting.
	Helper string `json:"helper"`
	// Safe: the owner confirmed (in the recipe or the plan) that the quest
	// cannot kill; otherwise it carries QuestRisks.
	Safe bool `json:"safe,omitempty"`
}

// Until is the quest's termination, the objective key the server proves.
func (q Quest) Until() string { return q.Quest + "@finished" }

// GEOffer is one Grand Exchange offer (docs/12 section 6), placed through
// the Agent Server's /ge/buy or /ge/sell and collected into the inventory.
// The server resolves the item, the quantity and the price from the
// account's facts and the stored market price (internal/core/ge); nothing
// here is chosen in game.
type GEOffer struct {
	// Item is the game id (unnoted) and ItemName its name as the GE search
	// lists it: the Agent Server takes the name, exactly.
	Item     int    `json:"item"`
	ItemName string `json:"itemName"`
	Qty      int    `json:"qty"`
	// Price is per item: the most a buy pays, the least a sale takes.
	Price int64 `json:"price"`
}

// GESell sells Qty of the item, all the inventory held when the server
// resolved it. Its termination is proven on the live inventory: the item
// left it (the content's sold) and the coins reached CoinsAtLeast, what it
// held then plus the least the sale pays after the GE tax.
type GESell struct {
	GEOffer
	CoinsAtLeast int64 `json:"coinsAtLeast"`
}

// Plugin delegates the whole job to a plugin of the client (a Microbot Hub
// plugin or one of ours): the agent sets its settings, starts it and stops
// it when the server proves Until from the ingest. The plugin decides how,
// never when it is done.
type Plugin struct {
	// Plugin is the plugin's fully qualified class and Name what the trail
	// calls it.
	Plugin string `json:"plugin"`
	Name   string `json:"name"`
	// Config are the settings it starts with.
	Config []clientcfg.Setting `json:"config,omitempty"`
	// Until is the job's goal as the agent's run_until names it: an
	// objective key or a var label (VarUntil).
	Until string `json:"until"`
}

// VarUntil is the label of a goal on a varp or varbit reaching a minimum: the
// string a run waits for ("varp:281>=1000"), which the server proves from
// the account's facts as it does an objective key.
func VarUntil(kind string, id int, atLeast int64) string {
	return fmt.Sprintf("%s:%d>=%d", kind, id, atLeast)
}

var reVarUntil = regexp.MustCompile(`^(varp|varbit):(0|[1-9][0-9]{0,5})>=(0|[1-9][0-9]{0,9})$`)

// IsVarUntil reports whether s is a var label (VarUntil).
func IsVarUntil(s string) bool { return reVarUntil.MatchString(s) }

// CheckUntil verifies what a run waits for: a var label or a canonical
// objective key.
func CheckUntil(s string) error {
	if IsVarUntil(s) {
		return nil
	}
	k, err := entities.ParseObjectiveKey(s)
	if err != nil {
		return fmt.Errorf("until: %w", err)
	}
	if k.String() != s {
		return fmt.Errorf("until: write the key as %s", k)
	}
	return nil
}

var (
	reRef   = regexp.MustCompile(`^(location|shop):[a-z0-9_.'-]{1,120}$`)
	reSkill = regexp.MustCompile(`^skill:[a-z]{1,40}$`)
	reQuest = regexp.MustCompile(`^quest:[a-z0-9]+(?:_[a-z0-9]+)*$`)
)

// Check verifies a task: a type the executor runs, its parameters only,
// tiles in the world, an area of a location (not a region), a target
// inside it, the wilderness flag as the area says, bounded lists and
// positive ids and quantities. A type outside the phase 3 list is refused
// with a clear error.
func (t Task) Check() error {
	set := 0
	for _, p := range []bool{t.Travel != nil, t.Bank != nil, t.Buy != nil, t.Train != nil, t.Quest != nil, t.GEBuy != nil, t.GESell != nil, t.Plugin != nil} {
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
	case TypeQuest:
		if set != 1 || t.Quest == nil {
			return errors.New("task: a quest has only quest")
		}
		return t.Quest.check()
	case TypePlugin:
		if set != 1 || t.Plugin == nil {
			return errors.New("task: a plugin has only plugin")
		}
		return t.Plugin.check()
	case TypeGEBuy:
		if set != 1 || t.GEBuy == nil {
			return errors.New("task: a ge_buy has only geBuy")
		}
		return t.GEBuy.check("ge_buy")
	case TypeGESell:
		if set != 1 || t.GESell == nil {
			return errors.New("task: a ge_sell has only geSell")
		}
		if err := t.GESell.check("ge_sell"); err != nil {
			return err
		}
		if t.GESell.CoinsAtLeast < 1 || t.GESell.CoinsAtLeast > MaxCoins {
			return fmt.Errorf("ge_sell: coinsAtLeast 1 to %d", MaxCoins)
		}
		return nil
	case content.TaskTalk, content.TaskInteract, content.TaskManual:
		return fmt.Errorf("task: %s is not executed yet (the executor runs travel, bank, buy, train, quest, ge_buy, ge_sell and plugin)", t.Type)
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

func (p *Plugin) check() error {
	if err := clientcfg.CheckClass(p.Plugin); err != nil {
		return fmt.Errorf("plugin: %w", err)
	}
	if err := checkName("plugin: name", p.Name); err != nil {
		return err
	}
	if err := clientcfg.CheckPluginSettings(p.Plugin, p.Config); err != nil {
		return fmt.Errorf("plugin: %w", err)
	}
	if err := CheckUntil(p.Until); err != nil {
		return fmt.Errorf("plugin: %w", err)
	}
	return nil
}

func (q *Quest) check() error {
	if len(q.Quest) > 128 || !reQuest.MatchString(q.Quest) {
		return errors.New("quest: quest must be a quest ref")
	}
	if err := checkName("quest: name", q.Name); err != nil {
		return err
	}
	return checkName("quest: helper", q.Helper)
}

func (o *GEOffer) check(what string) error {
	if err := checkName(what+": itemName", o.ItemName); err != nil {
		return err
	}
	switch {
	case o.Item <= 0 || o.Item >= maxGameID:
		return fmt.Errorf("%s: bad item id", what)
	case o.Qty < 1 || o.Qty > MaxQty:
		return fmt.Errorf("%s: qty 1 to %d", what, MaxQty)
	case o.Price < 1 || o.Price > MaxCoins/int64(o.Qty):
		return fmt.Errorf("%s: price of at least 1, and qty times price at most %d", what, MaxCoins)
	}
	return nil
}

// GE reports whether the task trades on the Grand Exchange.
func (t Task) GE() bool { return t.Type == TypeGEBuy || t.Type == TypeGESell }

// CheckMode verifies that an account of mode may run the task: the Grand
// Exchange is for regular accounts only (docs/12 section 3; an ironman never
// trades).
func (t Task) CheckMode(mode string) error {
	if t.GE() && mode != content.ModeRegular {
		return fmt.Errorf("task: %s is for regular accounts only, not %s", t.Type, mode)
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
// a travel whose area touches it, combat for a train that fights, combat
// and hazard for a quest not confirmed safe. The server adds the recipe's
// (and the wilderness of a train's location).
func (t Task) Risks() []string {
	if t.Travel != nil && AreaInWilderness(t.Travel.Area) {
		return []string{content.RiskWilderness}
	}
	if t.Train != nil && t.Train.Combat {
		return []string{content.RiskCombat}
	}
	if t.Quest != nil && !t.Quest.Safe {
		return slices.Clone(QuestRisks)
	}
	return nil
}
