package content

import (
	"strings"
	"testing"
)

func TestLoadChecksMarks(t *testing.T) {
	dir := t.TempDir()
	// "café" ends in a two-byte rune: [12,16] splits it; [0,3] comes after [4,10].
	writeGuide(t, dir, "marks", `{"schema_version":1,"id":"marks","title":"M","sections":[{"id":"1","slug":"one","title":"One","steps":[
 {"id":"a","text":"Buy potato, café","marks":[{"span":[4,10],"color":"red"},{"span":[12,16],"color":"orange"}]},
 {"id":"b","text":"Buy lime","marks":[{"span":[4,8],"color":"orange"},{"span":[0,3],"color":"red"},{"span":[4,9],"color":"red"}]}
]}]}`)
	_, err := Load(dir)
	issues, ok := AsErrors(err)
	if !ok {
		t.Fatalf("want Errors, got %v", err)
	}
	var msgs []string
	for _, is := range issues {
		msgs = append(msgs, is.Pointer+": "+is.Message)
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{
		"/sections/0/steps/0/marks/1/span: span [12 16] splits a character",
		"/sections/0/steps/1/marks/1/span: span [0 3] is out of order",
		"/sections/0/steps/1/marks/2/span: span [4 9] is out of order or outside the text",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "/sections/0/steps/0/marks/0") {
		t.Errorf("valid mark reported:\n%s", joined)
	}
}
