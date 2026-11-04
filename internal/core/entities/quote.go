package entities

import "strings"

// FindQuote locates quote in text after folding case, whitespace runs and
// typographic punctuation on both sides (the same folding mention detection
// uses), and returns the [start, end) byte span of the first match in the
// original text. Agents tend to normalize curly quotes and spacing, so an
// exact substring test would reject faithful quotes.
func FindQuote(text, quote string) (int, int, bool) {
	q := foldText(strings.TrimSpace(quote)).norm
	if q == "" {
		return 0, 0, false
	}
	f := foldText(text)
	i := strings.Index(f.norm, q)
	if i < 0 {
		return 0, 0, false
	}
	start, end := f.span(i, i+len(q))
	return start, end, true
}

// QuoteAt reports whether text[start:end] is quote up to the same folding.
func QuoteAt(text string, start, end int, quote string) bool {
	if start < 0 || end <= start || end > len(text) {
		return false
	}
	q := foldText(strings.TrimSpace(quote)).norm
	return q != "" && foldText(text[start:end]).norm == q
}
