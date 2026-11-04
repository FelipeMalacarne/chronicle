package entities

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Status is the outcome of resolving one piece of source text.
type Status string

const (
	// Resolved: exactly one entity, by a deterministic exact mapping.
	Resolved Status = "resolved"
	// Ambiguous: several entities fit; Candidates lists them. Never guessed.
	Ambiguous Status = "ambiguous"
	// Unresolved: no catalog entry fits (or the text gives no context).
	Unresolved Status = "unresolved"
)

// Match records which deterministic mapping produced a resolution.
type Match string

const (
	MatchID      Match = "id"       // a numeric/slug id taken verbatim
	MatchName    Match = "name"     // canonical name, ignoring case/spacing/typographic punctuation
	MatchAlias   Match = "alias"    // catalog alias (WikiSync, RuneLite or curated)
	MatchArticle Match = "article"  // canonical name without its leading "The"
	MatchPlural  Match = "plural"   // name with a trailing plural "s" removed
	MatchWikiKey Match = "wiki_key" // wiki link reduced to a page key
)

// Reasons for ambiguous and unresolved results (machine-readable).
const (
	ReasonNotInCatalog       = "not_in_catalog"
	ReasonAmbiguousName      = "ambiguous_name"
	ReasonVariantUnspecified = "variant_unspecified"
	ReasonTextAmbiguous      = "text_ambiguous"
	ReasonNotedFormUnknown   = "noted_form_unknown"
	ReasonNoCatalog          = "no_catalog"
	ReasonNoQuestContext     = "no_quest_context"
	ReasonSequelSuffix       = "sequel_suffix"
	ReasonMediaLink          = "media_link"
	ReasonNotWikiArticle     = "not_wiki_article"
	ReasonAggregate          = "aggregate"
)

// Resolution is the result of resolving one name, phrase or link.
type Resolution struct {
	Status     Status
	Ref        Ref // set only when Status == Resolved
	Match      Match
	Candidates []Ref
	Reason     string
}

func resolved(ref Ref, m Match) Resolution { return Resolution{Status: Resolved, Ref: ref, Match: m} }

func unresolved(reason string, candidates ...Ref) Resolution {
	return Resolution{Status: Unresolved, Reason: reason, Candidates: candidates}
}

func ambiguous(reason string, candidates []Ref) Resolution {
	sortRefs(candidates)
	return Resolution{Status: Ambiguous, Reason: reason, Candidates: candidates}
}

func sortRefs(refs []Ref) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		a, errA := strconv.Atoi(refs[i].ID)
		b, errB := strconv.Atoi(refs[j].ID)
		if errA == nil && errB == nil {
			return a < b
		}
		return refs[i].ID < refs[j].ID
	})
}

// fromHits turns name-index hits into a resolution of kind.
func fromHits(kind Kind, hits []nameHit) (Resolution, bool) {
	switch len(hits) {
	case 0:
		return Resolution{}, false
	case 1:
		return resolved(Ref{Kind: kind, ID: hits[0].id}, hits[0].match), true
	}
	refs := make([]Ref, len(hits))
	for i, h := range hits {
		refs[i] = Ref{Kind: kind, ID: h.id}
	}
	return ambiguous(ReasonAmbiguousName, refs), true
}

// ResolveQuest resolves a structured quest name (a rule's quest field, a
// link label) by canonical name, catalog alias or article-less spelling.
func (r *Registry) ResolveQuest(name string) Resolution {
	if res, ok := fromHits(KindQuest, r.questNames[NormalizeName(name)]); ok {
		return res
	}
	return unresolved(ReasonNotInCatalog)
}

// ResolveSkill resolves a skill name or id ("Herblore", "herblore").
func (r *Registry) ResolveSkill(name string) Resolution {
	if res, ok := fromHits(KindSkill, r.skillNames[NormalizeName(name)]); ok {
		return res
	}
	return unresolved(ReasonNotInCatalog)
}

// ResolveDiary resolves an area/tier pair. Area "all" is an aggregate over
// every area and has no single identity.
func (r *Registry) ResolveDiary(area, tier string) Resolution {
	a := NormalizeName(area)
	t := strings.ToLower(strings.TrimSpace(tier))
	if a == "all" {
		return unresolved(ReasonAggregate)
	}
	areaID, ok := r.diaryAreas[a]
	if !ok {
		return unresolved(ReasonNotInCatalog)
	}
	id := areaID + ":" + t
	if _, ok := r.diaries[id]; !ok {
		return unresolved(ReasonNotInCatalog)
	}
	m := MatchName
	if a != NormalizeName(r.diaries[id].Area) && a != areaID {
		m = MatchAlias
	}
	return resolved(Ref{Kind: KindDiary, ID: id}, m)
}

// ResolveCombatAchievement resolves a combat achievement task name.
func (r *Registry) ResolveCombatAchievement(name string) Resolution {
	if res, ok := fromHits(KindCombatAchievement, r.caNames[NormalizeName(name)]); ok {
		return res
	}
	return unresolved(ReasonNotInCatalog)
}

// ResolveUnlock resolves an unlock by name or alias.
func (r *Registry) ResolveUnlock(name string) Resolution {
	if res, ok := fromHits(KindUnlock, r.unlockNames[NormalizeName(name)]); ok {
		return res
	}
	return unresolved(ReasonNotInCatalog)
}

// ResolveLocation resolves a location by name or alias.
func (r *Registry) ResolveLocation(name string) Resolution {
	if res, ok := fromHits(KindLocation, r.locationNames[NormalizeName(name)]); ok {
		return res
	}
	return unresolved(ReasonNotInCatalog)
}

// ResolveItemID checks a source-provided item id against the catalog.
func (r *Registry) ResolveItemID(id int) Resolution {
	if _, ok := r.Item(id); ok {
		return resolved(ItemRef(id), MatchID)
	}
	return unresolved(ReasonNotInCatalog)
}

// ItemPhrase is an item mention split into what the catalog needs (Name,
// Noted) and what it does not identify (the quantity text).
type ItemPhrase struct {
	Source   string
	Quantity string // "232", "1k"; empty when absent
	Noted    bool
	Name     string
}

var (
	qtyPrefix   = regexp.MustCompile(`(?i)^(\d+(?:[.,]\d+)?\s*[km]?)\s*x?\s+`)
	notedPrefix = regexp.MustCompile(`(?i)^noted\s+`)
	notedSuffix = regexp.MustCompile(`(?i)\s*\(noted\)$`)
)

// ParseItemPhrase splits "120 noted planks" into quantity "120", noted, "planks".
func ParseItemPhrase(s string) ItemPhrase {
	p := ItemPhrase{Source: s}
	rest := strings.TrimSpace(foldPunct(s))
	if m := qtyPrefix.FindStringSubmatch(rest); m != nil {
		p.Quantity = strings.ReplaceAll(m[1], " ", "")
		rest = rest[len(m[0]):]
	}
	if loc := notedPrefix.FindStringIndex(rest); loc != nil {
		p.Noted = true
		rest = rest[loc[1]:]
	}
	if loc := notedSuffix.FindStringIndex(rest); loc != nil {
		p.Noted = true
		rest = rest[:loc[0]]
	}
	p.Name = strings.TrimSpace(rest)
	return p
}

// ResolveItem resolves an item phrase to one concrete item id. The noted
// form resolves to the noted id; a name that covers several variants or ids
// is ambiguous, never defaulted to one of them.
func (r *Registry) ResolveItem(phrase string) (Resolution, ItemPhrase) {
	p := ParseItemPhrase(phrase)
	res := r.resolveItemName(NormalizeName(p.Name))
	if res.Status == Unresolved && strings.HasSuffix(NormalizeName(p.Name), "s") {
		if alt := r.resolveItemName(strings.TrimSuffix(NormalizeName(p.Name), "s")); alt.Status != Unresolved {
			res = alt
			if res.Status == Resolved {
				res.Match = MatchPlural
			}
		}
	}
	if res.Status != Resolved || !p.Noted {
		return res, p
	}
	base, _ := res.Ref.Int()
	noted, ok := r.NotedID(base)
	if !ok {
		return unresolved(ReasonNotedFormUnknown, res.Ref), p
	}
	res.Ref = ItemRef(noted)
	return res, p
}

func (r *Registry) resolveItemName(k string) Resolution {
	if k == "" {
		return unresolved(ReasonNotInCatalog)
	}
	if a, ok := r.itemAmbig[k]; ok {
		refs := make([]Ref, len(a.Candidates))
		for i, c := range a.Candidates {
			refs[i] = Ref{Kind: KindItem, ID: c}
		}
		return ambiguous(ReasonAmbiguousName, refs)
	}
	if res, ok := fromHits(KindItem, r.itemNames[k]); ok {
		return res
	}
	if ids := r.itemFamily[k]; len(ids) > 0 {
		if len(ids) == 1 {
			return resolved(ItemRef(ids[0]), MatchName)
		}
		refs := make([]Ref, len(ids))
		for i, id := range ids {
			refs[i] = ItemRef(id)
		}
		return ambiguous(ReasonVariantUnspecified, refs)
	}
	return unresolved(ReasonNotInCatalog)
}

// ResolveWikiURL resolves a wiki link through its page key. A page shared
// by several entries (diary tiers, potion doses) is ambiguous.
func (r *Registry) ResolveWikiURL(raw string) (Resolution, WikiLink) {
	link, err := ParseWikiURL(raw)
	if err != nil {
		return unresolved(ReasonNotWikiArticle), link
	}
	if link.IsMedia() {
		return unresolved(ReasonMediaLink), link
	}
	refs := r.ByWikiKey(link.Key)
	switch len(refs) {
	case 0:
		// Redirect titles ("Hand in the Sand") are quest names or aliases, not page keys.
		if hits := r.questNames[NormalizeName(string(link.Key))]; len(hits) == 1 {
			return resolved(Ref{Kind: KindQuest, ID: hits[0].id}, MatchAlias), link
		}
		return unresolved(ReasonNotInCatalog), link
	case 1:
		return resolved(refs[0], MatchWikiKey), link
	}
	return ambiguous(ReasonVariantUnspecified, refs), link
}

// TextMention is an entity name found in free text. Start/End are byte
// offsets into the original text and Text is the exact original substring.
type TextMention struct {
	Start, End int
	Text       string
	Resolution Resolution
}

type textForm struct {
	norm string
	hits []nameHit
	amb  *AmbiguousName
}

type textIndex struct {
	byFirstWord map[string][]textForm
	diary       *regexp.Regexp
}

func (r *Registry) text() *textIndex {
	r.textOnce.Do(func() { r.textIx = r.buildTextIndex() })
	return r.textIx
}

func (r *Registry) buildTextIndex() *textIndex {
	ix := &textIndex{byFirstWord: map[string][]textForm{}}
	add := func(f textForm) {
		ix.byFirstWord[leadingWord(f.norm)] = append(ix.byFirstWord[leadingWord(f.norm)], f)
	}
	for k, hits := range r.questNames {
		f := textForm{norm: k, hits: hits}
		if a, ok := r.questText[k]; ok {
			f.amb = a
		}
		add(f)
	}
	for k, a := range r.questText {
		if _, ok := r.questNames[k]; !ok {
			add(textForm{norm: k, amb: a})
		}
	}
	for _, forms := range ix.byFirstWord {
		sort.Slice(forms, func(i, j int) bool {
			if len(forms[i].norm) != len(forms[j].norm) {
				return len(forms[i].norm) > len(forms[j].norm)
			}
			return forms[i].norm < forms[j].norm
		})
	}
	areas := make([]string, 0, len(r.diaryAreas))
	for a := range r.diaryAreas {
		areas = append(areas, regexp.QuoteMeta(a))
	}
	sort.Slice(areas, func(i, j int) bool {
		if len(areas[i]) != len(areas[j]) {
			return len(areas[i]) > len(areas[j])
		}
		return areas[i] < areas[j]
	})
	ix.diary = regexp.MustCompile(`(?:^|[^a-z0-9])((` + strings.Join(areas, "|") + `) (easy|medium|hard|elite) diar(?:y|ies))(?:[^a-z0-9]|$)`)
	return ix
}

// foldedText is a case- and punctuation-folded copy of a text with a map
// from each folded byte back to its original byte offset.
type foldedText struct {
	orig string
	norm string
	off  []int // len(norm)+1 entries
}

func foldText(s string) foldedText {
	var sb strings.Builder
	off := make([]int, 0, len(s)+1)
	lastSpace := false
	for i, r := range s {
		mapped := foldPunct(string(r))
		if len(mapped) == 1 && unicode.IsSpace(rune(mapped[0])) || unicode.IsSpace(r) {
			if lastSpace {
				continue
			}
			lastSpace = true
			sb.WriteByte(' ')
			off = append(off, i)
			continue
		}
		lastSpace = false
		for _, mr := range strings.ToLower(mapped) {
			var buf [utf8.UTFMax]byte
			n := utf8.EncodeRune(buf[:], mr)
			for b := 0; b < n; b++ {
				off = append(off, i)
			}
			sb.Write(buf[:n])
		}
	}
	off = append(off, len(s))
	return foldedText{orig: s, norm: sb.String(), off: off}
}

// span maps a folded [start,end) back to original offsets. end maps to the
// start of the next original rune, so the original substring is complete.
func (f foldedText) span(start, end int) (int, int) {
	return f.off[start], f.off[end]
}

// leadingWord returns the leading run of word bytes, the same unit
// FindQuestMentions uses to look up candidate forms.
func leadingWord(s string) string {
	i := 0
	for i < len(s) && isWordByte(s[i]) {
		i++
	}
	return s[:i]
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b >= 0x80
}

var (
	questCueBefore = regexp.MustCompile(`(?:^|[^a-z0-9])(?:start|starts|starting|started|begin|begins|beginning|finish|finishes|finishing|complete|completes|completing|do|doing|continue|continues|continuing|progress|progressing) (?:the )?$`)
	questWordAfter = regexp.MustCompile(`^ (?:quest|miniquest)(?:[^a-z0-9]|$)`)
	sequelAfter    = regexp.MustCompile(`^ (?:i|ii|iii|iv|1|2|3|4|part)(?:[^a-z0-9]|$)`)
)

// FindQuestMentions scans free text for quest names. A mention resolves only
// when the text marks it as a quest: a quest verb right before it ("start",
// "finish", "do", ...), the word "quest" right after it, or "quest" in the
// name itself. Everything else is reported, not resolved: a bare name
// without that context, a name shared with a minigame, or a name followed by
// a sequel numeral ("Monkey Madness 2").
func (r *Registry) FindQuestMentions(text string) []TextMention {
	ix := r.text()
	f := foldText(text)
	norm := f.norm
	var out []TextMention
	for i := 0; i < len(norm); {
		if !isWordByte(norm[i]) || (i > 0 && isWordByte(norm[i-1])) {
			i++
			continue
		}
		j := i
		for j < len(norm) && isWordByte(norm[j]) {
			j++
		}
		matched := false
		for _, form := range ix.byFirstWord[norm[i:j]] {
			end := i + len(form.norm)
			if end > len(norm) || norm[i:end] != form.norm || (end < len(norm) && isWordByte(norm[end]) && isWordByte(norm[end-1])) {
				continue
			}
			start, stop := f.span(i, end)
			m := TextMention{Start: start, End: stop, Text: text[start:stop]}
			m.Resolution = r.classifyQuestMention(form, norm[:i], norm[end:])
			out = append(out, m)
			i = end
			matched = true
			break
		}
		if !matched {
			i = j
		}
	}
	return out
}

func (r *Registry) classifyQuestMention(form textForm, before, after string) Resolution {
	candidates := func() []Ref {
		var refs []Ref
		if form.amb != nil {
			for _, c := range form.amb.Candidates {
				refs = append(refs, Ref{Kind: KindQuest, ID: c})
			}
			return refs
		}
		for _, h := range form.hits {
			refs = append(refs, Ref{Kind: KindQuest, ID: h.id})
		}
		return refs
	}
	questAfter := questWordAfter.MatchString(after)
	if form.amb != nil && (!questAfter || len(form.hits) != 1) {
		return ambiguous(ReasonTextAmbiguous, candidates())
	}
	if len(form.hits) > 1 {
		return ambiguous(ReasonAmbiguousName, candidates())
	}
	if sequelAfter.MatchString(after) {
		return ambiguous(ReasonSequelSuffix, candidates())
	}
	selfEvident := strings.Contains(" "+form.norm+" ", " quest ")
	if !questAfter && !selfEvident && !questCueBefore.MatchString(tail(before, 24)) {
		return unresolved(ReasonNoQuestContext, candidates()...)
	}
	return resolved(Ref{Kind: KindQuest, ID: form.hits[0].id}, form.hits[0].match)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// FindDiaryMentions scans free text for "<area> <tier> diary" phrases
// ("the Ardougne Hard diary", "Western Provinces hard diary").
func (r *Registry) FindDiaryMentions(text string) []TextMention {
	ix := r.text()
	f := foldText(text)
	var out []TextMention
	for _, loc := range ix.diary.FindAllStringSubmatchIndex(f.norm, -1) {
		start, stop := f.span(loc[2], loc[3])
		res := r.ResolveDiary(f.norm[loc[4]:loc[5]], f.norm[loc[6]:loc[7]])
		out = append(out, TextMention{Start: start, End: stop, Text: text[start:stop], Resolution: res})
	}
	return out
}
