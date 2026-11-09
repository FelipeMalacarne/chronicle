package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	Observe   json.RawMessage   `json:"observe,omitempty"`
	Inventory json.RawMessage   `json:"inventory,omitempty"` // GET /inventory: the items' actions
	Skills    json.RawMessage   `json:"skills,omitempty"`
	Bank      json.RawMessage   `json:"bank,omitempty"`
	Ground    json.RawMessage   `json:"ground,omitempty"`   // GET /ground-items nearby
	Machines  json.RawMessage   `json:"machines,omitempty"` // GET /debug/snapshot
	Errors    map[string]string `json:"errors,omitempty"`
}

// nearby is how far the view looks for NPCs, objects and ground items.
const nearby = "15"

func (e *Executor) gameView(ctx context.Context) GameView {
	var v GameView
	get := func(name, path string, q url.Values) json.RawMessage {
		raw, err := e.Game.JSON(ctx, http.MethodGet, path, q, nil, 10*time.Second)
		if err != nil {
			if v.Errors == nil {
				v.Errors = map[string]string{}
			}
			v.Errors[name] = err.Error()
		}
		return raw
	}
	v.Observe = get("observe", "/observe", url.Values{"nearby": {nearby}, "nearbyLimit": {"12"}})
	var st struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if v.Observe == nil || json.Unmarshal(v.Observe, &st) != nil || !st.LoggedIn {
		// Logged out: the rest has nothing to say.
		return v
	}
	v.Inventory = get("inventory", "/inventory", nil)
	v.Skills = get("skills", "/skills", nil)
	v.Bank = get("bank", "/bank", nil)
	v.Ground = get("ground", "/ground-items", url.Values{"maxDistance": {nearby}, "limit": {"12"}})
	v.Machines = get("machines", "/debug/snapshot", nil)
	return v
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
)

// GameActions are the game action commands.
var GameActions = []string{
	CmdWalk, CmdInventoryUse, CmdInventoryDrop, CmdBankOpen, CmdBankClose, CmdBankDeposit,
	CmdBankWithdraw, CmdNPCInteract, CmdObjectInteract, CmdGroundPickup, CmdDialogueContinue, CmdDialogueSelect,
}

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

// WithdrawParams are the parameters of CmdBankWithdraw.
type WithdrawParams struct {
	Name     string `json:"name,omitempty"`
	ID       int    `json:"id,omitempty"`
	Quantity int    `json:"quantity"`
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
		if p.Quantity < 1 {
			return nil, errors.New("withdraw: quantity of at least 1")
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
		body["quantity"] = pp.Quantity
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
