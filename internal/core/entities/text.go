package entities

import (
	"strings"
	"unicode"
)

// Slug derives the canonical slug of a display name: lower case ASCII,
// "&" spelled "and", apostrophes dropped, other non-alphanumeric runs
// collapsed to "_". "Cook's Assistant" → "cooks_assistant",
// "Kourend & Kebos" → "kourend_and_kebos".
func Slug(name string) string {
	s := strings.ReplaceAll(foldPunct(name), "&", " and ")
	s = strings.ReplaceAll(s, "'", "")
	var sb strings.Builder
	sep := false
	for _, r := range s {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			if sep && sb.Len() > 0 {
				sb.WriteByte('_')
			}
			sep = false
			sb.WriteRune(unicode.ToLower(r))
			continue
		}
		sep = true
	}
	return sb.String()
}

// FamilySlug is the slug of an item family name. Unlike Slug it keeps "+"
// (as "plus") and "(-)" (as "minus"), because families such as Antidote+ and
// Antidote++, or Antipoison and Antipoison (-), differ only there:
// "Antidote++" → "antidote_plus_plus", "Antipoison (-)" → "antipoison_minus".
func FamilySlug(name string) string {
	return Slug(familySigns.Replace(name))
}

var familySigns = strings.NewReplacer("+", " plus ", "(-)", " minus ")

// foldPunct maps typographic punctuation that guides use interchangeably
// with ASCII onto ASCII: curly quotes/apostrophes and non-breaking spaces.
func foldPunct(s string) string {
	return punctFolder.Replace(s)
}

var punctFolder = strings.NewReplacer(
	"’", "'", "‘", "'", "ʼ", "'", "`", "'",
	"“", `"`, "”", `"`,
	" ", " ", "–", "-", "—", "-",
)

// NormalizeName is the lookup key for names: typographic punctuation folded
// to ASCII, case folded, whitespace collapsed. It never removes words, so two
// names with the same key are spelled identically up to case and spacing.
func NormalizeName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(foldPunct(s)), " "))
}

// stripArticle removes one leading "the " from a normalized name.
func stripArticle(norm string) (string, bool) {
	rest, ok := strings.CutPrefix(norm, "the ")
	return rest, ok && rest != ""
}
