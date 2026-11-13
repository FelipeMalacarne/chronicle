package recording

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

func ev(kind string, edit func(*Event)) Event {
	e := Event{Recording: "rec-5b1c", Seq: 1, At: t0, Kind: kind, World: 330, X: 3222, Y: 3218}
	if edit != nil {
		edit(&e)
	}
	return e
}

func TestEventCheck(t *testing.T) {
	cases := []struct {
		name string
		e    Event
		want string // "" = valid; else a substring of the joined messages
	}{
		{"move", ev(KindMove, nil), ""},
		{"npc", ev(KindNPC, func(e *Event) { e.NPC, e.Option, e.Name = Int(3080), "Talk-to", "Duke Horacio" }), ""},
		{"npc id zero", ev(KindNPC, func(e *Event) { e.NPC, e.Option = Int(0), "Talk-to" }), ""},
		{"object with tile", ev(KindObject, func(e *Event) { e.Object, e.Option, e.TargetX, e.TargetY = Int(10583), "Bank", Int(3208), Int(3220) }), ""},
		{"dialogue", ev(KindDialogue, func(e *Event) { e.Option, e.NPC = "Have you any quests for me?", Int(3080) }), ""},
		{"shop buy", ev(KindShopBuy, func(e *Event) { e.Item, e.Qty, e.NPC = Int(1351), Int(1), Int(2813) }), ""},
		{"bank open", ev(KindBankOpen, func(e *Event) { e.Object = Int(10583) }), ""},
		{"withdraw", ev(KindBankWithdraw, func(e *Event) { e.Item, e.Qty = Int(995), Int(250) }), ""},
		{"deposit", ev(KindBankDeposit, func(e *Event) { e.Item, e.Qty = Int(1511), Int(27) }), ""},
		{"item on npc", ev(KindItemUse, func(e *Event) { e.Item, e.NPC, e.Option = Int(1925), Int(4626), "Use" }), ""},
		{"item op", ev(KindItemOp, func(e *Event) { e.Item, e.Option = Int(8007), "Break" }), ""},

		{"unknown kind", ev("walk", nil), `kind "walk" is not one of`},
		{"bad recording", ev(KindMove, func(e *Event) { e.Recording = "Rec 1" }), "recording must be"},
		{"long recording", ev(KindMove, func(e *Event) { e.Recording = strings.Repeat("a", 65) }), "recording must be"},
		{"seq zero", ev(KindMove, func(e *Event) { e.Seq = 0 }), "seq must be positive"},
		{"no time", ev(KindMove, func(e *Event) { e.At = time.Time{} }), "at is required"},
		{"after sent", ev(KindMove, func(e *Event) { e.At = t0.Add(time.Second) }), "at must not be after sent_at"},
		{"off map", ev(KindMove, func(e *Event) { e.X = 20000 }), "outside the game map"},
		{"bad plane", ev(KindMove, func(e *Event) { e.Plane = 4 }), "outside the game map"},
		{"move with npc", ev(KindMove, func(e *Event) { e.NPC = Int(1) }), "move takes no npc_id"},
		{"npc without option", ev(KindNPC, func(e *Event) { e.NPC = Int(3080) }), "npc needs option"},
		{"npc without id", ev(KindNPC, func(e *Event) { e.Option = "Talk-to" }), "npc needs npc_id"},
		{"negative id", ev(KindNPC, func(e *Event) { e.NPC, e.Option = Int(-1), "Talk-to" }), "npc_id must not be negative"},
		{"half a tile", ev(KindObject, func(e *Event) { e.Object, e.Option, e.TargetX = Int(1), "Open", Int(3208) }), "go together"},
		{"buy without qty", ev(KindShopBuy, func(e *Event) { e.Item = Int(1351) }), "shop_buy needs qty"},
		{"buy zero", ev(KindShopBuy, func(e *Event) { e.Item, e.Qty = Int(1351), Int(0) }), "qty must be positive"},
		{"use on nothing", ev(KindItemUse, func(e *Event) { e.Item = Int(1925) }), "exactly one of"},
		{"use on two", ev(KindItemUse, func(e *Event) { e.Item, e.NPC, e.Object = Int(1925), Int(1), Int(2) }), "exactly one of"},
		{"tags in option", ev(KindNPC, func(e *Event) { e.NPC, e.Option = Int(1), "<col=ff9040>Talk-to" }), "tags"},
		{"untrimmed name", ev(KindNPC, func(e *Event) { e.NPC, e.Option, e.Name = Int(1), "Talk-to", " Hans" }), "name must be trimmed"},
		{"long option", ev(KindDialogue, func(e *Event) { e.Option = strings.Repeat("x", MaxTextLength+1) }), "longer than 120 bytes"},
		{"control char", ev(KindDialogue, func(e *Event) { e.Option = "Yes\n" }), "must be trimmed"},
		{"inner control char", ev(KindDialogue, func(e *Event) { e.Option = "Y\x07es" }), "control characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(tc.e.Check(t0), "; ")
			switch {
			case tc.want == "" && got != "":
				t.Fatalf("want valid, got %s", got)
			case tc.want != "" && !strings.Contains(got, tc.want):
				t.Fatalf("want %q in %q", tc.want, got)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	a := ev(KindMove, nil)
	b := ev(KindMove, func(e *Event) { e.Seq = 2 })
	if msgs := Validate("data.recording", []Event{a, b}, t0); len(msgs) != 0 {
		t.Fatalf("valid push: %v", msgs)
	}
	if msgs := Validate("data.recording", []Event{a, a}, t0); len(msgs) != 1 || !strings.Contains(msgs[0], "appears twice") {
		t.Fatalf("duplicate: %v", msgs)
	}
	many := make([]Event, MaxEventsPerPush+1)
	if msgs := Validate("data.recording", many, t0); len(msgs) != 1 || !strings.Contains(msgs[0], "more than 500 events") {
		t.Fatalf("too many: %v", msgs)
	}
	bad := ev(KindNPC, func(e *Event) { e.Seq = 3 })
	if msgs := Validate("data.recording", []Event{a, bad}, t0); len(msgs) != 2 || !strings.HasPrefix(msgs[0], "data.recording[1]: ") {
		t.Fatalf("path: %v", msgs)
	}
}

// TestWireShape pins the JSON the plugin sends (RecordedEvent in Chronicle
// Link, RecordingTest.wireShape), so both sides of the contract agree.
func TestWireShape(t *testing.T) {
	const wire = `{"recording":"rec-5b1c","seq":3,"at":"2026-11-13T12:00:00Z","kind":"object","world":330,"x":3222,"y":3218,"plane":0,` +
		`"object_id":10583,"target_x":3208,"target_y":3220,"option":"Bank","name":"Bank booth"}`
	var e Event
	if err := json.Unmarshal([]byte(wire), &e); err != nil {
		t.Fatal(err)
	}
	if msgs := e.Check(t0); len(msgs) != 0 {
		t.Fatalf("plugin event invalid: %v", msgs)
	}
	out, _ := json.Marshal(e)
	if string(out) != wire {
		t.Fatalf("round trip:\n got %s\nwant %s", out, wire)
	}
}

func TestCompare(t *testing.T) {
	a := ev(KindMove, nil)
	b := ev(KindMove, func(e *Event) { e.Seq = 2 })
	c := ev(KindMove, func(e *Event) { e.At = t0.Add(-time.Second); e.Seq = 9 })
	if Compare(a, b) >= 0 || Compare(c, a) >= 0 || Compare(a, a) != 0 {
		t.Fatal("order: time, then recording and seq")
	}
}
