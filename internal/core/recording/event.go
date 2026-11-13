// Package recording is the demonstration recording of docs/10 section 11
// (phase 2, decision E8): while the owner plays a stretch of a route with
// "Record demonstration" on, the Chronicle Link plugin records their own
// interactions as typed events, never acting, and sends them in the ingest
// pushes (data.recording). The events become proposed tasks for a step,
// which enter content only after the verifier and the owner's confirmation
// (CLAUDE.md rule 9).
//
// It is pure: the event type, the closed list of kinds and the validation
// shared by the ingest contract and the content verifier.
package recording

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Event kinds: the closed list of what the plugin records.
const (
	// KindMove is a position sample: the first one of a recording, another
	// world or plane, another map region, or where the character stopped
	// after walking a few tiles. Walking is never recorded tile by tile.
	KindMove = "move"
	// KindNPC is a menu option clicked on an NPC (npc_id, option, name).
	KindNPC = "npc"
	// KindObject is a menu option clicked on a game object (object_id,
	// option, name and the object's tile in target_x/target_y).
	KindObject = "object"
	// KindDialogue is a dialogue option chosen (option: its text; npc_id: the
	// NPC last interacted with, when known).
	KindDialogue = "dialogue"
	// KindShopBuy is an item that entered the inventory while a shop was open
	// (item_id, qty; npc_id: the shopkeeper last interacted with).
	KindShopBuy = "shop_buy"
	// KindBankOpen is the bank interface opening (npc_id or object_id: the
	// banker or booth clicked just before, when known).
	KindBankOpen = "bank_open"
	// KindBankWithdraw and KindBankDeposit are what left or entered the bank
	// while it was open: the change of the inventory and equipment together
	// (item_id, qty), so equipping an item is neither.
	KindBankWithdraw = "bank_withdraw"
	KindBankDeposit  = "bank_deposit"
	// KindItemUse is an item used on an NPC, an object or another item
	// (item_id: the item used; npc_id, object_id or target_item_id).
	KindItemUse = "item_use"
	// KindItemOp is an inventory item's own menu option ("Rub", "Break",
	// "Read"; item_id, option): a teleport is the via of a travel.
	KindItemOp = "item_op"
)

// Kinds lists the event kinds in a stable order.
var Kinds = []string{KindMove, KindNPC, KindObject, KindDialogue, KindShopBuy, KindBankOpen, KindBankWithdraw, KindBankDeposit, KindItemUse, KindItemOp}

// Event is one recorded interaction. Recording and Seq identify it: Seq
// grows within a recording (one per login with recording on), so a push
// resent after a timeout stores nothing twice. World, X, Y and Plane are
// where the character stood. The other fields are the target, by kind (see
// the kinds); ids are game ids (NPC, object and item ids of the pinned
// client), never catalog refs: the proposals resolve them.
type Event struct {
	Recording string    `json:"recording"`
	Seq       int64     `json:"seq"`
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	World     int       `json:"world"`
	X         int       `json:"x"`
	Y         int       `json:"y"`
	Plane     int       `json:"plane"`
	NPC       *int      `json:"npc_id,omitempty"`
	Object    *int      `json:"object_id,omitempty"`
	Item      *int      `json:"item_id,omitempty"`
	// TargetItem is the item another item was used on (item_use).
	TargetItem *int `json:"target_item_id,omitempty"`
	// TargetX and TargetY are the object's tile (object), on Plane.
	TargetX *int `json:"target_x,omitempty"`
	TargetY *int `json:"target_y,omitempty"`
	Qty     *int `json:"qty,omitempty"`
	// Option is the menu option clicked ("Talk-to", "Open", "Rub") or the
	// dialogue option's text, as the game shows it without tags.
	Option string `json:"option,omitempty"`
	// Name is the target's name as the menu showed it, without tags.
	Name string `json:"name,omitempty"`
}

// Bounds of a recording in one push and of an event's text.
const (
	MaxEventsPerPush = 500
	MaxTextLength    = 120
	MaxRecordingID   = 64
)

var recordingID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ValidRecordingID reports whether s is a recording id: lower-case letters,
// digits and dashes, up to 64 bytes ("rec-" and a UUID).
func ValidRecordingID(s string) bool { return recordingID.MatchString(s) }

// fields says, per kind, which target fields an event must and may carry.
var fields = map[string]struct{ required, allowed []string }{
	KindMove:         {nil, nil},
	KindNPC:          {[]string{"npc_id", "option"}, []string{"name"}},
	KindObject:       {[]string{"object_id", "option"}, []string{"name", "target_x", "target_y"}},
	KindDialogue:     {[]string{"option"}, []string{"npc_id"}},
	KindShopBuy:      {[]string{"item_id", "qty"}, []string{"npc_id"}},
	KindBankOpen:     {nil, []string{"npc_id", "object_id"}},
	KindBankWithdraw: {[]string{"item_id", "qty"}, nil},
	KindBankDeposit:  {[]string{"item_id", "qty"}, nil},
	KindItemUse:      {[]string{"item_id"}, []string{"npc_id", "object_id", "target_item_id", "option", "name"}},
	KindItemOp:       {[]string{"item_id", "option"}, []string{"name"}},
}

// set names the target fields e carries.
func (e Event) set() []string {
	var out []string
	for name, on := range map[string]bool{
		"npc_id": e.NPC != nil, "object_id": e.Object != nil, "item_id": e.Item != nil, "target_item_id": e.TargetItem != nil,
		"target_x": e.TargetX != nil, "target_y": e.TargetY != nil, "qty": e.Qty != nil, "option": e.Option != "", "name": e.Name != "",
	} {
		if on {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// Check returns every problem of one event, or nothing. sentAt bounds its
// time when not zero.
func (e Event) Check(sentAt time.Time) []string {
	var msgs []string
	add := func(format string, args ...any) { msgs = append(msgs, fmt.Sprintf(format, args...)) }
	if !ValidRecordingID(e.Recording) {
		add("recording must be a lower-case id of letters, digits and dashes (at most %d bytes)", MaxRecordingID)
	}
	if e.Seq < 1 {
		add("seq must be positive")
	}
	switch {
	case e.At.IsZero():
		add("at is required")
	case !sentAt.IsZero() && e.At.After(sentAt):
		add("at must not be after sent_at")
	}
	if !onMap(e.X, e.Y) || e.Plane < 0 || e.Plane > 3 || e.World < 0 || e.World > 9999 {
		add("position is outside the game map")
	}
	spec, known := fields[e.Kind]
	if !known {
		add("kind %q is not one of %s", e.Kind, strings.Join(Kinds, ", "))
		return msgs
	}
	have := e.set()
	for _, f := range spec.required {
		if !slices.Contains(have, f) {
			add("%s needs %s", e.Kind, f)
		}
	}
	for _, f := range have {
		if !slices.Contains(spec.required, f) && !slices.Contains(spec.allowed, f) {
			add("%s takes no %s", e.Kind, f)
		}
	}
	for _, id := range []struct {
		name string
		v    *int
	}{{"npc_id", e.NPC}, {"object_id", e.Object}, {"item_id", e.Item}, {"target_item_id", e.TargetItem}} {
		if id.v != nil && *id.v < 0 {
			add("%s must not be negative", id.name)
		}
	}
	if e.Qty != nil && *e.Qty < 1 {
		add("qty must be positive")
	}
	if (e.TargetX == nil) != (e.TargetY == nil) {
		add("target_x and target_y go together")
	} else if e.TargetX != nil && !onMap(*e.TargetX, *e.TargetY) {
		add("target is outside the game map")
	}
	if e.Kind == KindItemUse {
		targets := 0
		for _, t := range []*int{e.NPC, e.Object, e.TargetItem} {
			if t != nil {
				targets++
			}
		}
		if targets != 1 {
			add("item_use needs exactly one of npc_id, object_id and target_item_id")
		}
	}
	if msg := checkText(e.Option); msg != "" {
		add("option %s", msg)
	}
	if msg := checkText(e.Name); msg != "" {
		add("name %s", msg)
	}
	return msgs
}

func onMap(x, y int) bool { return x >= 0 && x <= 16383 && y >= 0 && y <= 16383 }

// checkText checks a text field: trimmed, at most MaxTextLength bytes, no
// tags and no control characters (the plugin strips the game's tags).
func checkText(s string) string {
	switch {
	case s == "":
		return ""
	case strings.TrimSpace(s) != s:
		return "must be trimmed"
	case len(s) > MaxTextLength:
		return fmt.Sprintf("is longer than %d bytes", MaxTextLength)
	case strings.ContainsAny(s, "<>"):
		return "must not carry the game's tags"
	case strings.IndexFunc(s, unicode.IsControl) >= 0:
		return "must not carry control characters"
	}
	return ""
}

// Validate checks the events of one push: at most MaxEventsPerPush, each
// valid, and no (recording, seq) twice. path prefixes each message
// ("data.recording").
func Validate(path string, events []Event, sentAt time.Time) []string {
	if len(events) > MaxEventsPerPush {
		return []string{fmt.Sprintf("%s has more than %d events", path, MaxEventsPerPush)}
	}
	var msgs []string
	seen := map[string]bool{}
	for i, e := range events {
		for _, m := range e.Check(sentAt) {
			msgs = append(msgs, fmt.Sprintf("%s[%d]: %s", path, i, m))
		}
		k := fmt.Sprintf("%s#%d", e.Recording, e.Seq)
		if seen[k] {
			msgs = append(msgs, fmt.Sprintf("%s[%d]: recording %q seq %d appears twice", path, i, e.Recording, e.Seq))
		}
		seen[k] = true
	}
	return msgs
}

// Compare orders events by time, then recording and seq (slices.SortFunc).
func Compare(a, b Event) int {
	if c := a.At.Compare(b.At); c != 0 {
		return c
	}
	if c := strings.Compare(a.Recording, b.Recording); c != 0 {
		return c
	}
	switch {
	case a.Seq < b.Seq:
		return -1
	case a.Seq > b.Seq:
		return 1
	}
	return 0
}

// Int returns a pointer to v, for building events.
func Int(v int) *int { return &v }
