package progress

import (
	"reflect"
	"testing"
)

func mk(kind MarkKind) *Mark { return &Mark{Kind: kind} }

func TestEffectsOf(t *testing.T) {
	current := map[string]Mark{"a": {Kind: MarkSkipped}, "b": {Kind: MarkManualDone}}
	got := EffectsOf(current, []MarkChange{
		{StepID: "a", Mark: mk(MarkManualDone)}, // changed
		{StepID: "b", Mark: mk(MarkManualDone)}, // unchanged: no effect
		{StepID: "c", Mark: mk(MarkSkipped)},    // new
		{StepID: "a", Mark: nil},                // cleared: from the mark the first change set
	})
	want := []MarkEffect{
		{StepID: "a", From: mk(MarkSkipped), To: mk(MarkManualDone)},
		{StepID: "c", From: nil, To: mk(MarkSkipped)},
		{StepID: "a", From: mk(MarkManualDone), To: nil},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effects = %+v", got)
	}
}

func TestReplayMarks(t *testing.T) {
	implied := &Mark{Kind: MarkImplied, ImpliedBy: "d"}
	tests := []struct {
		name     string
		commands [][]MarkEffect
		want     map[string]Mark
		gaps     int
	}{
		{name: "empty log", commands: nil, want: map[string]Mark{}},
		{
			name: "mark, engine implied, owner confirms, undo clears",
			commands: [][]MarkEffect{
				{{StepID: "a", To: mk(MarkManualDone)}},
				{{StepID: "b", To: implied}, {StepID: "c", To: implied}},
				{{StepID: "b", From: implied, To: mk(MarkManualDone)}},
				{{StepID: "a", From: mk(MarkManualDone), To: nil}},
			},
			want: map[string]Mark{"b": {Kind: MarkManualDone}, "c": *implied},
		},
		{
			name: "a write that bypassed the log is a gap",
			commands: [][]MarkEffect{
				{{StepID: "a", To: mk(MarkManualDone)}},
				{{StepID: "a", From: mk(MarkSkipped), To: nil}},
			},
			want: map[string]Mark{},
			gaps: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gaps := ReplayMarks(tt.commands)
			if !reflect.DeepEqual(got, tt.want) || len(gaps) != tt.gaps {
				t.Fatalf("marks %+v, gaps %v", got, gaps)
			}
		})
	}
}
