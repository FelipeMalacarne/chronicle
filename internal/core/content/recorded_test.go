package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

var recordedAt = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// rec builds a recorded event of recording rec-test at the Lumbridge Castle
// courtyard (3222, 3218), inside location:lumbridge_castle's area.
func rec(seq int64, kind string, edit func(*recording.Event)) recording.Event {
	e := recording.Event{Recording: "rec-test", Seq: seq, At: recordedAt, Kind: kind, World: 330, X: 3222, Y: 3218}
	if edit != nil {
		edit(&e)
	}
	return e
}

func evidence(events ...recording.Event) RecordedEvidence {
	return RecordedEvidence{Recording: "rec-test", Events: events}
}

// TestCheckRecorded is the table of the recorded-evidence verifier (docs/10
// section 11): the events must show what the task says happened, by the
// catalog's game ids.
func TestCheckRecorded(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	talkCook := func(e *recording.Event) { e.NPC, e.Option = recording.Int(4626), "Talk-to" }
	option := func(text string) func(*recording.Event) { return func(e *recording.Event) { e.Option = text } }
	cases := []struct {
		name string
		task Task
		ev   RecordedEvidence
		want string // "" when it holds
	}{
		{"travel inside the area", Task{Type: TaskTravel, Location: ref("location:lumbridge_castle")},
			evidence(rec(1, recording.KindMove, nil)), ""},
		{"travel never there", Task{Type: TaskTravel, Location: ref("location:lumbridge")},
			evidence(rec(1, recording.KindMove, nil)), "no recorded position is in the area of location:lumbridge"},
		{"talk with its options in order", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Options: []string{"What's wrong?", "Yes."}},
			evidence(rec(1, recording.KindNPC, talkCook), rec(2, recording.KindDialogue, option("What's wrong?")), rec(3, recording.KindDialogue, option("Yes."))), ""},
		{"talk to another npc", Task{Type: TaskTalk, NPC: ref("npc:hans")},
			evidence(rec(1, recording.KindNPC, talkCook)), "no recorded click on npc:hans"},
		{"options out of order", Task{Type: TaskTalk, NPC: ref("npc:cook_lumbridge"), Options: []string{"Yes.", "What's wrong?"}},
			evidence(rec(1, recording.KindNPC, talkCook), rec(2, recording.KindDialogue, option("What's wrong?")), rec(3, recording.KindDialogue, option("Yes."))),
			`dialogue option "What's wrong?" is not recorded`},
		{"interact", Task{Type: TaskInteract, Object: ref("object:bank_booth"), Action: "Bank"},
			evidence(rec(1, recording.KindObject, func(e *recording.Event) { e.Object, e.Option = recording.Int(10583), "Bank" })), ""},
		{"interact with another action", Task{Type: TaskInteract, Object: ref("object:bank_booth"), Action: "Collect"},
			evidence(rec(1, recording.KindObject, func(e *recording.Event) { e.Object, e.Option = recording.Int(10583), "Bank" })), `no recorded "Collect"`},
		{"buy from the owner", Task{Type: TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 1},
			evidence(rec(1, recording.KindShopBuy, func(e *recording.Event) {
				e.Item, e.Qty, e.NPC = recording.Int(1351), recording.Int(1), recording.Int(10619)
			})), ""},
		{"buy from someone else", Task{Type: TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1351"), Qty: 1},
			evidence(rec(1, recording.KindShopBuy, func(e *recording.Event) {
				e.Item, e.Qty, e.NPC = recording.Int(1351), recording.Int(1), recording.Int(2813)
			})),
			"the recorded shopkeeper (npc 2813) does not own shop:bobs_brilliant_axes"},
		{"buy what was not bought", Task{Type: TaskBuy, Shop: ref("shop:bobs_brilliant_axes"), Item: ref("item:1265"), Qty: 1},
			evidence(rec(1, recording.KindShopBuy, func(e *recording.Event) { e.Item, e.Qty = recording.Int(1351), recording.Int(1) })), "no recorded purchase of item:1265"},
		{"bank", Task{Type: TaskBank}, evidence(rec(1, recording.KindBankWithdraw, func(e *recording.Event) { e.Item, e.Qty = recording.Int(2347), recording.Int(1) })), ""},
		{"bank without the bank", Task{Type: TaskBank}, evidence(rec(1, recording.KindMove, nil)), "no recorded bank event"},
		{"train on an npc", Task{Type: TaskTrain, Skill: ref("skill:attack"), NPC: ref("npc:hans")},
			evidence(rec(1, recording.KindNPC, func(e *recording.Event) { e.NPC, e.Option = recording.Int(3105), "Attack" })), ""},
		{"manual", Task{Type: TaskManual}, evidence(rec(1, recording.KindMove, nil)), ""},
		{"no events", Task{Type: TaskManual}, evidence(), "needs the events"},
		{"an invalid event", Task{Type: TaskManual}, evidence(rec(1, recording.KindNPC, nil)), "events/0: npc needs npc_id"},
		{"another recording's event", Task{Type: TaskManual},
			evidence(rec(1, recording.KindMove, func(e *recording.Event) { e.Recording = "rec-other" })), `from recording "rec-other"`},
		{"a bad recording id", Task{Type: TaskManual}, RecordedEvidence{Recording: "Rec 1", Events: []recording.Event{rec(1, recording.KindMove, nil)}}, "is not a recording id"},
	}
	for _, c := range cases {
		got := strings.Join(CheckRecorded(c.task, c.ev, reg), "; ")
		switch {
		case c.want == "" && got != "":
			t.Errorf("%s: unexpected %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestRecordedTaskLoads puts a task that came only from a recording, with
// the owner's confirmation, in the fixture: the loader takes it like a
// quoted one.
func TestRecordedTaskLoads(t *testing.T) {
	dir := copyFixture(t)
	path := filepath.Join(dir, GuidesSubdir, "t", AnnotationsSubdir, "1.1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc AnnotationFile
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	buy := rec(7, recording.KindShopBuy, func(e *recording.Event) {
		e.Item, e.Qty, e.NPC = recording.Int(1351), recording.Int(1), recording.Int(10619)
	})
	for i := range doc.Steps {
		if doc.Steps[i].StepID == "b" {
			doc.Steps[i].Tasks[1].Evidence = TaskEvidence{Recorded: &RecordedEvidence{Recording: "rec-test", Events: []recording.Event{buy}}}
			doc.Steps[i].Tasks[1].Source = ObjectiveSource{Method: ProvenanceRecording, Confidence: ConfidenceMedium, ConfirmedBy: ConfirmedByOwner}
		}
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	if err = os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, _ := c.StepInGuide("t", "b")
	if len(b.Tasks) != 2 || b.Tasks[1].Type != TaskBuy || b.Tasks[1].Shop.String() != "shop:bobs_brilliant_axes" {
		t.Fatalf("step b tasks = %+v", b.Tasks)
	}
}
