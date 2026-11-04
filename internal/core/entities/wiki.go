package entities

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WikiBaseURL is the article prefix of the Old School RuneScape Wiki.
const WikiBaseURL = "https://oldschool.runescape.wiki/w/"

// WikiKey is a canonical wiki page title with spaces ("Cook's Assistant").
// It is an attribute of an entity, never its identity: URLs are derived from
// keys, and links in guides are reduced to keys before any lookup.
type WikiKey string

// CanonicalWikiKey normalizes a page title the way MediaWiki does: underscores
// become spaces, runs of whitespace collapse, and the first letter is upper
// case (titles are case-insensitive only in their first character).
func CanonicalWikiKey(title string) WikiKey {
	t := strings.Join(strings.Fields(strings.ReplaceAll(title, "_", " ")), " ")
	if t == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(t)
	return WikiKey(string(unicode.ToUpper(r)) + t[size:])
}

// URL derives the article URL. Spaces become underscores and only bytes that
// are not valid in a path (plus '?', '#' and '%') are percent-encoded, which
// matches the form the wiki itself links to ("Romeo_&_Juliet", "Wanted!").
func (k WikiKey) URL() string {
	if k == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(WikiBaseURL)
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c == ' ':
			sb.WriteByte('_')
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			strings.IndexByte("-._~!$&'()*+,;=:@/", c) >= 0:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "%%%02X", c)
		}
	}
	return sb.String()
}

// WikiLink is a link to the wiki reduced to its page key.
type WikiLink struct {
	Key WikiKey
	// Fragment is the part after '#', unescaped ("Levels 46-55: Summer Shore").
	Fragment string
}

// IsMedia reports whether the link opens a media viewer ("#/media/File:...")
// rather than pointing at the page's subject.
func (l WikiLink) IsMedia() bool { return strings.HasPrefix(l.Fragment, "/media/") }

// ParseWikiURL reduces an OSRS Wiki article URL to its key. It accepts
// http/https, the "/w/<title>" form and "index.php?title=<title>".
func ParseWikiURL(raw string) (WikiLink, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return WikiLink{}, fmt.Errorf("wiki url %q: %w", raw, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return WikiLink{}, fmt.Errorf("wiki url %q: unsupported scheme", raw)
	}
	if h := strings.ToLower(u.Hostname()); h != "oldschool.runescape.wiki" {
		return WikiLink{}, fmt.Errorf("wiki url %q: not an OSRS Wiki host", raw)
	}
	var title string
	switch {
	case u.Path == "/w/index.php" || u.Path == "/index.php":
		title = u.Query().Get("title")
	case strings.HasPrefix(u.Path, "/w/"):
		title = strings.TrimPrefix(u.Path, "/w/")
	}
	key := CanonicalWikiKey(title)
	if key == "" {
		return WikiLink{}, fmt.Errorf("wiki url %q: no page title", raw)
	}
	return WikiLink{Key: key, Fragment: u.Fragment}, nil
}
