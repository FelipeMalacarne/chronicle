package content

import (
	"fmt"
	"unicode/utf8"
)

// checkStepMarks validates a step's colored runs: each span selects a
// non-empty, rune-aligned slice of Text, in order and without overlap, with a
// known color.
func checkStepMarks(name, ptr string, st *Step) Errors {
	var issues Errors
	prev := 0
	for i, m := range st.Marks {
		mPtr := fmt.Sprintf("%s/marks/%d", ptr, i)
		start, end := m.Span[0], m.Span[1]
		switch {
		case start < prev || end <= start || end > len(st.Text):
			issues = append(issues, Issue{File: name, Pointer: mPtr + "/span", Message: fmt.Sprintf("span %v is out of order or outside the text (%d bytes)", m.Span, len(st.Text))})
		case !utf8.RuneStart(st.Text[start]) || (end < len(st.Text) && !utf8.RuneStart(st.Text[end])):
			issues = append(issues, Issue{File: name, Pointer: mPtr + "/span", Message: fmt.Sprintf("span %v splits a character", m.Span)})
		}
		if m.Color != MarkRed && m.Color != MarkOrange && m.Color != MarkGreen {
			issues = append(issues, Issue{File: name, Pointer: mPtr + "/color", Message: fmt.Sprintf("unknown mark color %q", m.Color)})
		}
		if end > prev {
			prev = end
		}
	}
	return issues
}
