package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sync"
	"time"
	"unicode"
)

// Game view and actions (docs/08, "ver e agir no jogo"). CmdGameView is one
// read of everything the cockpit shows; each part is the Agent Server's own
// JSON and a part that failed carries its error instead.
const CmdGameView = "game_view"

// GameView is the answer to CmdGameView.
type GameView struct {
	// Observe is GET /observe with the nearest NPCs and objects: state,
	// inventory, equipment, dialogue, game chat (never player chat), recent
	// errors and script heartbeats.
	Observe   json.RawMessage `json:"observe,omitempty"`
	Inventory json.RawMessage `json:"inventory,omitempty"` // GET /inventory: the items' actions
	Skills    json.RawMessage `json:"skills,omitempty"`
	Bank      json.RawMessage `json:"bank,omitempty"`
	Ground    json.RawMessage `json:"ground,omitempty"`   // GET /ground-items nearby
	Machines  json.RawMessage `json:"machines,omitempty"` // GET /debug/snapshot
	// Bridge is what the control bridge adds: the real menus of NPCs,
	// objects and worn items, the prayers and the spellbook. Absent without
	// the bridge (errors["bridge"] says why).
	Bridge *BridgeView       `json:"bridge,omitempty"`
	Errors map[string]string `json:"errors,omitempty"`
}

// BridgeView is the bridge's part of GameView, each the bridge's own JSON.
type BridgeView struct {
	Entities  json.RawMessage `json:"entities,omitempty"`  // GET /bridge/entities
	Equipment json.RawMessage `json:"equipment,omitempty"` // GET /bridge/equipment
	Prayers   json.RawMessage `json:"prayers,omitempty"`   // GET /bridge/prayers
	Spells    json.RawMessage `json:"spells,omitempty"`    // GET /bridge/spells
}

// nearby is how far the view looks for NPCs, objects and ground items.
const nearby = "15"

func (e *Executor) gameView(ctx context.Context) GameView {
	var v GameView
	var mu sync.Mutex // v.Errors: the bridge reads in its own goroutine
	fail := func(name, err string) {
		mu.Lock()
		defer mu.Unlock()
		if v.Errors == nil {
			v.Errors = map[string]string{}
		}
		v.Errors[name] = err
	}
	get := func(name, path string, q url.Values) json.RawMessage {
		raw, err := e.Game.JSON(ctx, http.MethodGet, path, q, nil, 10*time.Second)
		if err != nil {
			fail(name, err.Error())
		}
		return raw
	}
	// The plain observe is cheap and says whether the rest has anything to say.
	v.Observe = get("observe", "/observe", nil)
	var st struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if v.Observe == nil || json.Unmarshal(v.Observe, &st) != nil || !st.LoggedIn {
		return v
	}
	// The bridge has its own socket and server: it reads while the Agent Server does.
	bridge := make(chan *BridgeView, 1)
	go func() { bridge <- e.bridgeView(ctx, fail) }()
	v.Inventory = get("inventory", "/inventory", nil)
	v.Skills = get("skills", "/skills", nil)
	v.Bank = get("bank", "/bank", nil)
	v.Ground = get("ground", "/ground-items", url.Values{"maxDistance": {nearby}, "limit": {"12"}})
	v.Machines = get("machines", "/debug/snapshot", nil)
	v.Bridge = <-bridge
	if v.Bridge == nil {
		// Without the bridge, nearby NPCs and objects come from the Agent Server's observe, which
		// takes seconds (it reads them on the client thread); with it, the bridge's have the menus.
		if o := get("observe", "/observe", url.Values{"nearby": {nearby}, "nearbyLimit": {"12"}}); o != nil {
			v.Observe = o
		}
	}
	return v
}

// bridgeView reads the bridge; the first failure (no bridge on this client)
// is reported once as "bridge" and the rest is skipped.
func (e *Executor) bridgeView(ctx context.Context, fail func(name, err string)) *BridgeView {
	if e.Bridge == nil {
		fail("bridge", "no bridge configured")
		return nil
	}
	var b BridgeView
	for _, part := range []struct {
		into  *json.RawMessage
		path  string
		query url.Values
	}{
		{&b.Entities, "/bridge/entities", url.Values{"distance": {nearby}, "limit": {"12"}}},
		{&b.Equipment, "/bridge/equipment", nil},
		{&b.Prayers, "/bridge/prayers", nil},
		{&b.Spells, "/bridge/spells", nil},
	} {
		raw, err := e.Bridge.JSON(ctx, http.MethodGet, part.path, part.query, nil, 10*time.Second)
		if err != nil {
			fail("bridge", fmt.Sprintf("%s: %v (the Chronicle Link plugin 1.4+ serves the bridge in the dev client)", part.path, err))
			return nil
		}
		*part.into = raw
	}
	return &b
}

// Game actions: each one is a fixed Agent Server request from typed,
// validated parameters, audited by the server. Raw keyboard, widgets and the
// Grand Exchange (an ironman account) stay out.
const (
	CmdWalk             = "walk"            // webwalker to a tile, non-blocking
	CmdInventoryUse     = "inventory_use"   // an item's action ("Eat", "Wield")
	CmdInventoryDrop    = "inventory_drop"  // one, or every one with all
	CmdBankOpen         = "bank_open"       // the nearest bank
	CmdBankClose        = "bank_close"      // the open bank
	CmdBankDeposit      = "bank_deposit"    // an item, or everything with all
	CmdBankWithdraw     = "bank_withdraw"   // an item and a quantity
	CmdNPCInteract      = "npc_interact"    // an NPC's action ("Talk-to", "Bank")
	CmdObjectInteract   = "object_interact" // an object's action ("Chop down")
	CmdGroundPickup     = "ground_pickup"   // an item on the ground
	CmdDialogueContinue = "dialogue_continue"
	CmdDialogueSelect   = "dialogue_select" // an option by text or 1-based index

	// Through the control bridge of the Chronicle Link plugin.
	CmdEquipmentUse = "equipment_use"  // a worn item's action (Remove, Rub, Operate)
	CmdPrayerToggle = "prayer_toggle"  // a prayer on or off
	CmdSpellCast    = "spell_cast"     // a spell, on an NPC or an inventory item when it takes one
	CmdBankDepositX = "bank_deposit_x" // Deposit-X with the bank open
	CmdItemUseOn    = "item_use_on"    // the game's "Use X -> Y": on an item, an NPC or an object
)

// GameActions are the game action commands.
var GameActions = []string{
	CmdWalk, CmdInventoryUse, CmdInventoryDrop, CmdBankOpen, CmdBankClose, CmdBankDeposit,
	CmdBankWithdraw, CmdNPCInteract, CmdObjectInteract, CmdGroundPickup, CmdDialogueContinue, CmdDialogueSelect,
	CmdEquipmentUse, CmdPrayerToggle, CmdSpellCast, CmdBankDepositX, CmdItemUseOn,
}

// bridgeActions are the game actions served by the bridge.
var bridgeActions = []string{CmdEquipmentUse, CmdPrayerToggle, CmdSpellCast, CmdBankDepositX, CmdItemUseOn}

// PrayerParams are the parameters of CmdPrayerToggle: the client's
// Rs2PrayerEnum key (THICK_SKIN).
type PrayerParams struct {
	Key string `json:"key"`
	On  bool   `json:"on"`
}

// SpellParams are the parameters of CmdSpellCast: the client's spell key
// (Rs2Spells or Rs2CombatSpells) and at most one target, an inventory item
// (alchemy, Superheat) or a nearby NPC by its index (combat spells, curses).
// The bridge checks that the spell takes that target.
type SpellParams struct {
	Key  string `json:"key"`
	Item string `json:"item,omitempty"`
	NPC  *int   `json:"npc,omitempty"`
}

// UseOnParams are the parameters of CmdItemUseOn: an inventory item and one
// target, another inventory item, a nearby NPC by its index or an object by
// its id.
type UseOnParams struct {
	Item   string `json:"item"`
	OnItem string `json:"onItem,omitempty"`
	NPC    *int   `json:"npc,omitempty"`
	Object int    `json:"object,omitempty"`
}

// npcIndex checks an NPC index: the client's are below 65536.
func npcIndex(i *int) error {
	if i != nil && (*i < 0 || *i > 65535) {
		return errors.New("npc: bad index")
	}
	return nil
}

// DepositXParams are the parameters of CmdBankDepositX.
type DepositXParams struct {
	Name   string `json:"name"`
	Amount int    `json:"amount"`
}

var reEnumKey = regexp.MustCompile(`^[A-Z0-9_]{1,48}$`)

// WalkParams are the parameters of CmdWalk: a world tile.
type WalkParams struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Plane int `json:"plane"`
}

// TargetParams name what an action is about (by name or id) and, for
// interactions, the action as the game's menu shows it.
type TargetParams struct {
	Name   string `json:"name,omitempty"`
	ID     int    `json:"id,omitempty"`
	Action string `json:"action,omitempty"`
}

// AllParams are the parameters of CmdInventoryDrop and CmdBankDeposit: an
// item by name, or every one (drop) / everything (deposit) with All.
type AllParams struct {
	Name string `json:"name,omitempty"`
	All  bool   `json:"all,omitempty"`
}

// WithdrawParams are the parameters of CmdBankWithdraw: a quantity, or All
// (the game's Withdraw-All).
type WithdrawParams struct {
	Name     string `json:"name,omitempty"`
	ID       int    `json:"id,omitempty"`
	Quantity int    `json:"quantity,omitempty"`
	All      bool   `json:"all,omitempty"`
	Noted    *bool  `json:"noted,omitempty"`
}

// SelectParams are the parameters of CmdDialogueSelect: the option's text or
// its 1-based index.
type SelectParams struct {
	Option string `json:"option,omitempty"`
	Index  int    `json:"index,omitempty"`
}

// gameText checks a name, action or option: one printable line.
func gameText(what, s string, limit int, required bool) error {
	if s == "" {
		if required {
			return fmt.Errorf("%s is required", what)
		}
		return nil
	}
	if len(s) > limit {
		return fmt.Errorf("%s: at most %d characters", what, limit)
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return fmt.Errorf("%s: one printable line", what)
		}
	}
	return nil
}

func checkTarget(p TargetParams, action bool) error {
	if (p.Name == "") == (p.ID == 0) {
		return errors.New("target: a name or an id, not both")
	}
	if p.ID < 0 || p.ID > 200000 {
		return errors.New("target: bad id")
	}
	if err := gameText("name", p.Name, 80, false); err != nil {
		return err
	}
	if !action {
		if p.Action != "" {
			return errors.New("target: no action here")
		}
		return nil
	}
	return gameText("action", p.Action, 40, true)
}

// validateGame decodes and checks a game action's parameters.
func validateGame(command string, raw json.RawMessage) (any, error) {
	switch command {
	case CmdWalk:
		var p WalkParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.X < 1 || p.X > 16383 || p.Y < 1 || p.Y > 16383 || p.Plane < 0 || p.Plane > 3 {
			return nil, errors.New("walk: x and y between 1 and 16383, plane 0 to 3")
		}
		return p, nil
	case CmdInventoryUse, CmdNPCInteract, CmdObjectInteract, CmdGroundPickup:
		var p TargetParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := checkTarget(p, command != CmdGroundPickup); err != nil {
			return nil, err
		}
		return p, nil
	case CmdInventoryDrop, CmdBankDeposit:
		var p AllParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if command == CmdInventoryDrop && p.Name == "" {
			return nil, errors.New("drop: the item's name is required")
		}
		if command == CmdBankDeposit && (p.Name == "") == !p.All {
			return nil, errors.New("deposit: an item's name or all, not both")
		}
		if err := gameText("name", p.Name, 80, false); err != nil {
			return nil, err
		}
		return p, nil
	case CmdBankWithdraw:
		var p WithdrawParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := checkTarget(TargetParams{Name: p.Name, ID: p.ID}, false); err != nil {
			return nil, err
		}
		if (p.Quantity < 1) == !p.All {
			return nil, errors.New("withdraw: a quantity of at least 1 or all, not both")
		}
		return p, nil
	case CmdDialogueSelect:
		var p SelectParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if (p.Option == "") == (p.Index == 0) || p.Index < 0 || p.Index > 10 {
			return nil, errors.New("dialogue: an option's text or its index (1 to 10), not both")
		}
		if err := gameText("option", p.Option, 120, false); err != nil {
			return nil, err
		}
		return p, nil
	case CmdEquipmentUse:
		var p TargetParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.ID != 0 {
			return nil, errors.New("equipment: the worn item by name")
		}
		if err := checkTarget(p, true); err != nil {
			return nil, err
		}
		return p, nil
	case CmdPrayerToggle:
		var p PrayerParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if !reEnumKey.MatchString(p.Key) {
			return nil, errors.New("prayer: the client's prayer key (THICK_SKIN)")
		}
		return p, nil
	case CmdSpellCast:
		var p SpellParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if !reEnumKey.MatchString(p.Key) {
			return nil, errors.New("spell: the client's spell key (LOW_LEVEL_ALCHEMY)")
		}
		if err := gameText("item", p.Item, 80, false); err != nil {
			return nil, err
		}
		if p.Item != "" && p.NPC != nil {
			return nil, errors.New("spell: one target, an item or an npc")
		}
		if err := npcIndex(p.NPC); err != nil {
			return nil, err
		}
		return p, nil
	case CmdItemUseOn:
		var p UseOnParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := gameText("item", p.Item, 80, true); err != nil {
			return nil, err
		}
		if err := gameText("onItem", p.OnItem, 80, false); err != nil {
			return nil, err
		}
		n := 0
		for _, set := range []bool{p.OnItem != "", p.NPC != nil, p.Object != 0} {
			if set {
				n++
			}
		}
		if n != 1 {
			return nil, errors.New("use: one target, onItem, npc or object")
		}
		if err := npcIndex(p.NPC); err != nil {
			return nil, err
		}
		if p.Object < 0 || p.Object > 200000 {
			return nil, errors.New("use: bad object id")
		}
		return p, nil
	case CmdBankDepositX:
		var p DepositXParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := gameText("name", p.Name, 80, true); err != nil {
			return nil, err
		}
		if p.Amount < 1 {
			return nil, errors.New("deposit: amount of at least 1")
		}
		return p, nil
	case CmdBankOpen, CmdBankClose, CmdDialogueContinue:
		return nil, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownCommand, command)
}

// act runs one game action.
func (e *Executor) act(ctx context.Context, command string, params json.RawMessage) (any, error) {
	p, err := validateGame(command, params)
	if err != nil {
		return nil, err
	}
	if slices.Contains(bridgeActions, command) {
		return e.bridgeAct(ctx, p)
	}
	post := func(path string, body any, timeout time.Duration) (any, error) {
		return e.Game.JSON(ctx, http.MethodPost, path, nil, body, timeout)
	}
	target := func(t TargetParams) map[string]any {
		b := map[string]any{}
		if t.Name != "" {
			b["name"] = t.Name
		} else {
			b["id"] = t.ID
		}
		if t.Action != "" {
			b["action"] = t.Action
		}
		return b
	}
	switch pp := p.(type) {
	case WalkParams:
		return post("/walk", map[string]any{"x": pp.X, "y": pp.Y, "plane": pp.Plane, "wait": false}, 15*time.Second)
	case TargetParams:
		path := map[string]string{
			CmdInventoryUse: "/inventory/interact", CmdNPCInteract: "/npcs/interact",
			CmdObjectInteract: "/objects/interact", CmdGroundPickup: "/ground-items/pickup",
		}[command]
		return post(path, target(pp), 20*time.Second)
	case AllParams:
		path := "/inventory/drop"
		if command == CmdBankDeposit {
			path = "/bank/deposit"
		}
		body := map[string]any{}
		if pp.Name != "" {
			body["name"] = pp.Name
		}
		if pp.All {
			body["all"] = true
		}
		return post(path, body, 20*time.Second)
	case WithdrawParams:
		body := target(TargetParams{Name: pp.Name, ID: pp.ID})
		body["quantity"] = pp.Quantity // the Agent Server withdraws all with 0
		if pp.Noted != nil {
			body["noted"] = *pp.Noted
		}
		return post("/bank/withdraw", body, 20*time.Second)
	case SelectParams:
		if pp.Option != "" {
			return post("/dialogue/select", map[string]any{"option": pp.Option}, 15*time.Second)
		}
		return post("/dialogue/select", map[string]any{"index": pp.Index}, 15*time.Second)
	case nil:
		path := map[string]string{CmdBankOpen: "/bank/open", CmdBankClose: "/bank/close", CmdDialogueContinue: "/dialogue/continue"}[command]
		return post(path, map[string]any{}, 20*time.Second)
	}
	return nil, fmt.Errorf("command %q is not in the agent's closed list", command)
}

// bridgeAct runs an action through the control bridge.
func (e *Executor) bridgeAct(ctx context.Context, p any) (any, error) {
	if e.Bridge == nil {
		return nil, errors.New("this action needs the control bridge (Chronicle Link plugin 1.4+ in the dev client)")
	}
	post := func(path string, body any) (any, error) {
		return e.Bridge.JSON(ctx, http.MethodPost, path, nil, body, 30*time.Second)
	}
	switch pp := p.(type) {
	case TargetParams:
		return post("/bridge/equipment/interact", map[string]any{"name": pp.Name, "action": pp.Action})
	case PrayerParams:
		return post("/bridge/prayers/toggle", map[string]any{"key": pp.Key, "on": pp.On})
	case SpellParams:
		body := map[string]any{"key": pp.Key}
		if pp.Item != "" {
			body["item"] = pp.Item
		}
		if pp.NPC != nil {
			body["npc"] = *pp.NPC
		}
		return post("/bridge/spells/cast", body)
	case UseOnParams:
		body := map[string]any{"item": pp.Item}
		switch {
		case pp.OnItem != "":
			body["onItem"] = pp.OnItem
		case pp.NPC != nil:
			body["npc"] = *pp.NPC
		default:
			body["object"] = pp.Object
		}
		return post("/bridge/inventory/use-on", body)
	case DepositXParams:
		return post("/bridge/bank/deposit-x", map[string]any{"name": pp.Name, "amount": pp.Amount})
	}
	return nil, errors.New("not a bridge action")
}
