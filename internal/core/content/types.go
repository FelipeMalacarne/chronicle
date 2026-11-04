package content

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Guide is one ordered list of sections, each an ordered list of steps.
// It mirrors the root object of guide.schema.json.
type Guide struct {
	SchemaVersion int        `json:"schema_version"`
	ID            string     `json:"id"`
	Slug          string     `json:"slug,omitempty"`
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Version       string     `json:"version,omitempty"`
	DefaultModes  []string   `json:"default_modes,omitempty"`
	DefaultLayers []string   `json:"default_layers,omitempty"`
	Source        *Source    `json:"source,omitempty"`
	Sections      []*Section `json:"sections"`

	// File is the path the guide was loaded from (empty when built in memory).
	File string `json:"-"`

	stepsByID    map[string]*Step
	sectionsByID map[string]*Section
	sectionsBySl map[string]*Section
	questNames   []string
}

// Section groups steps. Kind "track" marks a parallel optional track (e.g. Sailing)
// that does not take part in "what is next" of the main guide.
type Section struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Related     []Related `json:"related,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Steps       []*Step   `json:"steps"`
}

// Section kinds.
const (
	KindMain  = "main"
	KindTrack = "track"
)

// IsTrack reports whether the section is a parallel track.
func (s *Section) IsTrack() bool { return s.Kind == KindTrack }

// Related is a link to companion content (articles on ironman.guide, wiki pages).
type Related struct {
	Href  string `json:"href,omitempty"`
	Label string `json:"label"`
}

// Step is one actionable line of the guide.
type Step struct {
	ID               string   `json:"id"`
	Text             string   `json:"text"`
	Location         string   `json:"location,omitempty"`
	Notes            []Note   `json:"notes,omitempty"`
	Items            []Item   `json:"items,omitempty"`
	Links            []Link   `json:"links,omitempty"`
	CompleteWhen     RuleList `json:"complete_when,omitempty"`
	Prerequisites    RuleList `json:"prerequisites,omitempty"`
	ManualOK         *bool    `json:"manual_ok,omitempty"`
	Requires         []string `json:"requires,omitempty"`
	RecommendedAfter []string `json:"recommended_after,omitempty"`
	Unlocks          []string `json:"unlocks,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Modes            []string `json:"modes,omitempty"`
	Layers           []string `json:"layers,omitempty"`
	Phase            string   `json:"phase,omitempty"`
	Optional         bool     `json:"optional,omitempty"`
	EstimateMinutes  *int     `json:"estimate_minutes,omitempty"`
	Replaces         []string `json:"replaces,omitempty"`
	Source           *Source  `json:"source,omitempty"`

	// SectionID is the id of the section that contains the step (set by the loader).
	SectionID string `json:"-"`
	// GuideID is the id of the guide that contains the step (set by the loader).
	GuideID string `json:"-"`
}

// IsManual reports whether the step has no automatic completion rule
// (absent or empty complete_when ⇒ checkbox).
func (s *Step) IsManual() bool { return len(s.CompleteWhen) == 0 }

// Note is an inline annotation shown with the step.
type Note struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Note kinds.
const (
	NoteTip     = "tip"
	NoteHCIM    = "hcim"
	NoteUpdate  = "update"
	NoteWarning = "warning"
)

// Link is an external resource attached to a step.
type Link struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

// Source records where a guide or step came from, for attribution and diffing.
type Source struct {
	Origin     string `json:"origin,omitempty"`
	OriginalID string `json:"original_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Author     string `json:"author,omitempty"`
	License    string `json:"license,omitempty"`
	ImportedAt string `json:"imported_at,omitempty"`
}

// Item is an item to bring. In JSON it is either a bare string or an object;
// a bare string decodes into Name and an Item with only Name encodes back to a string.
type Item struct {
	Name   string `json:"name"`
	ItemID *int   `json:"item_id,omitempty"`
	Qty    *int   `json:"qty,omitempty"`
	Equip  bool   `json:"equip,omitempty"`
	Note   string `json:"note,omitempty"`
}

type itemObject Item

// UnmarshalJSON accepts both the string and the object forms.
func (it *Item) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*it = Item{Name: s}
		return nil
	}
	var obj itemObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("item: %w", err)
	}
	*it = Item(obj)
	return nil
}

// MarshalJSON writes the compact string form when only Name is set.
func (it Item) MarshalJSON() ([]byte, error) {
	if it.ItemID == nil && it.Qty == nil && !it.Equip && it.Note == "" {
		return json.Marshal(it.Name)
	}
	return json.Marshal(itemObject(it))
}

// Index (re)builds the lookup maps and per-step back references. Load calls
// it; call it yourself after building a Guide in memory.
func (g *Guide) Index() {
	g.stepsByID = make(map[string]*Step)
	g.sectionsByID = make(map[string]*Section, len(g.Sections))
	g.sectionsBySl = make(map[string]*Section, len(g.Sections))
	quests := map[string]struct{}{}
	for _, sec := range g.Sections {
		g.sectionsByID[sec.ID] = sec
		if sec.Slug != "" {
			g.sectionsBySl[sec.Slug] = sec
		}
		for _, st := range sec.Steps {
			st.SectionID = sec.ID
			st.GuideID = g.ID
			if _, dup := g.stepsByID[st.ID]; !dup {
				g.stepsByID[st.ID] = st
			}
			for _, r := range st.CompleteWhen {
				collectQuests(r, quests)
			}
			for _, r := range st.Prerequisites {
				collectQuests(r, quests)
			}
		}
	}
	g.questNames = g.questNames[:0]
	for q := range quests {
		g.questNames = append(g.questNames, q)
	}
	sort.Strings(g.questNames)
}

func collectQuests(r Rule, into map[string]struct{}) {
	switch v := r.(type) {
	case QuestStateRule:
		into[v.Quest] = struct{}{}
	case AllRule:
		for _, c := range v.All {
			collectQuests(c, into)
		}
	case AnyRule:
		for _, c := range v.Any {
			collectQuests(c, into)
		}
	case NotRule:
		collectQuests(v.Not, into)
	}
}

// Step returns the step with the given id.
func (g *Guide) Step(id string) (*Step, bool) {
	s, ok := g.stepsByID[id]
	return s, ok
}

// Section returns the section with the given id or slug.
func (g *Guide) Section(idOrSlug string) (*Section, bool) {
	if s, ok := g.sectionsByID[idOrSlug]; ok {
		return s, true
	}
	s, ok := g.sectionsBySl[idOrSlug]
	return s, ok
}

// Steps returns every step in guide order.
func (g *Guide) Steps() []*Step {
	var out []*Step
	for _, sec := range g.Sections {
		out = append(out, sec.Steps...)
	}
	return out
}

// StepCount returns the number of steps in the guide.
func (g *Guide) StepCount() int {
	n := 0
	for _, sec := range g.Sections {
		n += len(sec.Steps)
	}
	return n
}

// QuestNames returns the sorted set of quest names referenced by rules.
func (g *Guide) QuestNames() []string {
	return append([]string(nil), g.questNames...)
}
