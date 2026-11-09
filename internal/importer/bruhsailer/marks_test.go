package bruhsailer

import (
	"reflect"
	"testing"
)

func TestMarkColor(t *testing.T) {
	for hex, want := range map[string]string{
		"ff0000": "red", "ff3838": "red", "c9211e": "red", "990000": "red", "e06666": "red", "ff4000": "red",
		"ff8000": "orange", "ff9900": "orange", "b45f06": "orange",
		"069a2e": "green", "00a933": "green",
		// black, gray, link blues and the purple of the metadata lines carry no emphasis
		"000000": "", "999999": "", "0000ee": "", "1155cc": "", "000080": "", "8d1d75": "", "auto": "",
	} {
		if got := markColor(hex); got != want {
			t.Errorf("markColor(%q) = %q, want %q", hex, got, want)
		}
	}
}

func TestFormatBulletsMovesMarks(t *testing.T) {
	text, marks := formatBullets([]rawBullet{
		{Level: 1, Text: "Buy a potato.", Marks: []rawMark{{Start: 6, End: 12, Color: "red"}}},
		{Level: 2, Text: "Then a lime.", Marks: []rawMark{{Start: 7, End: 11, Color: "orange"}}},
	})
	if want := "- Buy a potato.\n  - Then a lime."; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	var got []string
	for _, m := range marks {
		got = append(got, m.Color+":"+text[m.Span[0]:m.Span[1]])
	}
	if want := []string{"red:potato", "orange:lime"}; !reflect.DeepEqual(got, want) {
		t.Errorf("marks select %q, want %q", got, want)
	}
}

func TestShiftAndMergeMarks(t *testing.T) {
	p := &paragraph{}
	text := "  potato cabbage, tomato"
	addMark(p, text, 2, 9, "red")   // "potato " (trailing space trimmed)
	addMark(p, text, 9, 16, "red")  // "cabbage" touches it: merged
	addMark(p, text, 18, 24, "red") // "tomato" after ", ": kept apart
	got := shiftMarks(p.Marks, 2, len(text)-2)
	want := []rawMark{{Start: 0, End: 14, Color: "red"}, {Start: 16, End: 22, Color: "red"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("marks = %+v, want %+v", got, want)
	}
}
