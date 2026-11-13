package ingest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

func talkEvent(seq int64) recording.Event {
	return recording.Event{
		Recording: "rec-5b1c", Seq: seq, At: testTime.Add(-time.Minute), Kind: recording.KindNPC,
		World: 330, X: 3210, Y: 3220, NPC: recording.Int(3080), Option: "Talk-to", Name: "Duke Horacio",
	}
}

func TestRecordingValidation(t *testing.T) {
	cases := []struct {
		name    string
		events  *[]recording.Event
		wantErr string
	}{
		{"events", &[]recording.Event{talkEvent(1), talkEvent(2)}, ""},
		{"empty array", &[]recording.Event{}, ""},
		{"null array", func() *[]recording.Event { var e []recording.Event; return &e }(), "data.recording must be a JSON array"},
		{"invalid event", &[]recording.Event{func() recording.Event { e := talkEvent(1); e.NPC = nil; return e }()}, "data.recording[0]: npc needs npc_id"},
		{"after sent_at", &[]recording.Event{func() recording.Event { e := talkEvent(1); e.At = testTime.Add(time.Second); return e }()}, "at must not be after sent_at"},
		{"twice", &[]recording.Event{talkEvent(1), talkEvent(1)}, "appears twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The same rules in a full push and in a delta.
			for _, push := range []Push{fullPush(), deltaPush(nil)} {
				push.Data.Recording = tc.events
				err := push.Validate()
				if tc.wantErr == "" && err != nil {
					t.Fatalf("%s: Validate() = %v", push.Kind, err)
				}
				if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
					t.Fatalf("%s: Validate() = %v, want %q", push.Kind, err, tc.wantErr)
				}
			}
		})
	}
}

func TestRecordingNeedsManifest(t *testing.T) {
	manifest := Manifest{
		ContractVersion: ContractVersion, ManifestVersion: 7, TokenScopes: []TokenScope{ScopeStateWrite},
		Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10,
	}
	push := deltaPush(nil)
	push.Data.Items = nil
	push.Data.Recording = &[]recording.Event{talkEvent(1)}
	if err := push.ValidateAgainst(manifest); err == nil || !strings.Contains(err.Error(), "data.recording is not enabled") {
		t.Fatalf("ValidateAgainst() = %v, want the gate", err)
	}
	manifest.Recording = true
	if err := push.ValidateAgainst(manifest); err != nil {
		t.Fatalf("ValidateAgainst() with the flag = %v", err)
	}

	// The wire shape the plugin sends (List<RecordedEvent>), read back
	// strictly like the ingest handler does.
	raw := `{"recording":[{"recording":"rec-5b1c","seq":1,"at":"2026-11-04T11:59:00Z","kind":"npc","world":330,"x":3210,"y":3220,"plane":0,"npc_id":3080,"option":"Talk-to","name":"Duke Horacio"}]}`
	var data PushData
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Recording == nil || len(*data.Recording) != 1 || (*data.Recording)[0].Seq != 1 || *(*data.Recording)[0].NPC != 3080 {
		t.Fatalf("decoded = %+v", data.Recording)
	}
	if encoded, _ := json.Marshal(data); string(encoded) != raw {
		t.Fatalf("round trip = %s", encoded)
	}
	var m map[string]any
	encoded, _ := json.Marshal(manifest)
	if json.Unmarshal(encoded, &m) != nil || m["recording"] != true {
		t.Fatalf("manifest wire = %s", encoded)
	}
}

func TestClampTimesBoundsRecordedEvents(t *testing.T) {
	push := deltaPush(nil)
	ahead := talkEvent(1)
	ahead.At = testTime.Add(time.Hour)
	events := []recording.Event{ahead, talkEvent(2)}
	push.Data.Recording = &events
	now := testTime.Add(time.Minute)
	got := push.ClampTimes(now)
	if !(*got.Data.Recording)[0].At.Equal(now) || !(*got.Data.Recording)[1].At.Equal(talkEvent(2).At) {
		t.Fatalf("clamped = %+v", *got.Data.Recording)
	}
	if !events[0].At.Equal(testTime.Add(time.Hour)) {
		t.Fatal("the caller's events changed")
	}
}
