package content

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// Guide is one ordered list of route variants. The Sections field is the
// flattened compatibility view populated by Index; canonical JSON stores
// sections under Variants. Legacy JSON with root sections remains readable.
type Guide struct {
	SchemaVersion  int        `json:"schema_version"`
	ID             string     `json:"id"`
	Slug           string     `json:"slug,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Version        string     `json:"version,omitempty"`
	DefaultModes   []string   `json:"default_modes,omitempty"`
	DefaultLayers  []string   `json:"default_layers,omitempty"`
	Source         *Source    `json:"source,omitempty"`
	DefaultVariant string     `json:"default_variant,omitempty"`
	Variants       []*Variant `json:"variants,omitempty"`
	Sections       []*Section `json:"sections,omitempty"`

	// File is the path the guide was loaded from (empty when built in memory).
	File string `json:"-"`

	stepsByID    map[string]*Step
	variantsByID map[string]*Variant
	sectionsByID map[string]*Section
	sectionsBySl map[string]*Section
	segmentsByID map[string]*Segment
	questNames   []string
}

// MarshalJSON suppresses the flattened legacy sections view when canonical
// variants are present.
func (g Guide) MarshalJSON() ([]byte, error) {
	type guideAlias Guide
	out := guideAlias(g)
	if len(out.Variants) > 0 {
		out.Sections = nil
	}
	return json.Marshal(out)
}

// Variant is one independently selectable route through a guide.
type Variant struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Modes       []string   `json:"modes,omitempty"`
	Layers      []string   `json:"layers,omitempty"`
	Source      *Source    `json:"source,omitempty"`
	Sections    []*Section `json:"sections"`
}

// Section groups segments. Steps is the flattened compatibility view populated
// by Index; canonical JSON stores steps under Segments.
type Section struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Kind        string     `json:"kind,omitempty"`
	Related     []Related  `json:"related,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Segments    []*Segment `json:"segments,omitempty"`
	Steps       []*Step    `json:"steps,omitempty"`

	VariantID    string              `json:"-"`
	segmentsByID map[string]*Segment `json:"-"`
}

// MarshalJSON suppresses the flattened legacy steps view when canonical
// segments are present.
func (s Section) MarshalJSON() ([]byte, error) {
	type sectionAlias Section
	out := sectionAlias(s)
	if len(out.Segments) > 0 {
		out.Steps = nil
	}
	return json.Marshal(out)
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

// Segment is an ordered sequence of steps. A bank_leg begins at an explicit
// bank/deposit/withdraw/setup boundary; a sequence has no claimed bank boundary.
type Segment struct {
	ID          string       `json:"id"`
	Title       string       `json:"title,omitempty"`
	Kind        string       `json:"kind"`
	BankActions []BankAction `json:"bank_actions,omitempty"`
	Loadout     *Loadout     `json:"loadout,omitempty"`
	Source      Provenance   `json:"source"`
	Steps       []*Step      `json:"steps"`

	VariantID string `json:"-"`
	SectionID string `json:"-"`
}

const (
	SegmentSequence = "sequence"
	SegmentBankLeg  = "bank_leg"
)

// BankAction is an explicit action preserved from a source step.
type BankAction struct {
	Type     string     `json:"type"`
	Position string     `json:"position"`
	Text     string     `json:"text"`
	Source   Provenance `json:"source"`
}

const (
	BankOpen       = "open_bank"
	BankDepositAll = "deposit_all"
	BankDeposit    = "deposit"
	BankWithdraw   = "withdraw"
	BankSetup      = "bank_setup"
	BankClose      = "close_bank"

	PositionStart  = "start"
	PositionWithin = "within"
	PositionEnd    = "end"
)

// Loadout distinguishes explicitly complete, partial and unknown setups.
// Unknown is represented by Status=unknown, never by empty item arrays.
type Loadout struct {
	Status     string            `json:"status"`
	Inventory  []ItemRequirement `json:"inventory,omitempty"`
	Equipment  []ItemRequirement `json:"equipment,omitempty"`
	EmptySlots *int              `json:"empty_slots,omitempty"`
	Source     *Provenance       `json:"source,omitempty"`
}

const (
	LoadoutKnown   = "known"
	LoadoutPartial = "partial"
	LoadoutUnknown = "unknown"
)

// ItemRequirement is one required item of a loadout or a step (docs/09
// section 3.4). Once resolved, exactly one variant is set:
//
//	exact   ItemID     that id ("Hammer")
//	any_of  AnyOf      any one of the ids, in preference order ("Axe")
//	family  Family     any variant of a catalog family, noted or not, by
//	                   family slug ("prayer_potion")
//
// None set means the name is not resolved to an item yet. Qty is nil when the
// source does not state a quantity; Noted means the guide asks for the noted
// form; Optional defaults false. Non-exact requirements have no objective key:
// they are evaluated by readiness and ownership only.
type ItemRequirement struct {
	Name       string `json:"name"`
	ItemID     *int   `json:"item_id,omitempty"`
	AnyOf      []int  `json:"any_of,omitempty"`
	Family     string `json:"family,omitempty"`
	Qty        *int   `json:"qty,omitempty"`
	Noted      bool   `json:"noted,omitempty"`
	Optional   bool   `json:"optional,omitempty"`
	SourceText string `json:"source_text,omitempty"`
}

// Item requirement variants.
const (
	RequireExact      = "exact"
	RequireAnyOf      = "any_of"
	RequireFamily     = "family"
	RequireUnresolved = "unresolved"
)

// Variant names the requirement's variant; RequireUnresolved when none is set.
func (r ItemRequirement) Variant() string {
	switch {
	case r.ItemID != nil:
		return RequireExact
	case len(r.AnyOf) > 0:
		return RequireAnyOf
	case r.Family != "":
		return RequireFamily
	}
	return RequireUnresolved
}

// Objective is one objective of a step: a canonical key, the role it plays in
// the step and where it came from (docs/09 sections 3.2, 3.3 and 4.1).
type Objective struct {
	Key      entities.ObjectiveKey `json:"key"`
	Role     string                `json:"role"`
	Evidence *ObjectiveEvidence    `json:"evidence,omitempty"`
	Source   ObjectiveSource       `json:"source"`
}

// Objective roles.
const (
	// RoleCompletes: the step completes the objective.
	RoleCompletes = "completes"
	// RoleStarts: the step starts a quest; counts as completes of @started.
	RoleStarts = "starts"
	// RoleRequires: must hold before the step; never completes it.
	RoleRequires = "requires"
	// RoleUses: the step consumes or uses it (items, runes).
	RoleUses = "uses"
)

// Completing reports whether the role makes the step auto_done when met.
func (o Objective) Completing() bool { return o.Role == RoleCompletes || o.Role == RoleStarts }

// ObjectiveEvidence points at the step value that supports an objective.
// Field is a JSON pointer relative to the step; Quote and Span are set for
// objectives read from text (Span is the exact [start, end) bytes of the
// original). Objectives converted from rules point at the rule and carry no
// quote.
type ObjectiveEvidence struct {
	Field string `json:"field"`
	Quote string `json:"quote,omitempty"`
	Span  []int  `json:"span,omitempty"`
}

// ObjectiveSource is the provenance of an objective. ConfirmedBy "owner" marks
// what the owner decided during extraction; SkillVersion identifies the
// extraction skill for llm objectives.
type ObjectiveSource struct {
	Method       string `json:"method"`
	Confidence   string `json:"confidence"`
	SkillVersion string `json:"skill_version,omitempty"`
	ConfirmedBy  string `json:"confirmed_by,omitempty"`
}

// ConfirmedByOwner is the only ObjectiveSource.ConfirmedBy value.
const ConfirmedByOwner = "owner"

// Provenance describes how structured segment metadata was obtained.
type Provenance struct {
	Method     string   `json:"method"`
	Confidence string   `json:"confidence"`
	StepIDs    []string `json:"step_ids"`
	Evidence   string   `json:"evidence,omitempty"`
}

const (
	ProvenanceExplicit = "explicit"
	ProvenanceDerived  = "derived"
	// ProvenanceLLM is an agent proposal that passed deterministic verification.
	ProvenanceLLM = "llm"
	// ProvenanceManual is human curation.
	ProvenanceManual = "manual"
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

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
	// Entities are the exactly resolved entity references of the step,
	// written by internal/core/entities/enrich.
	Entities []EntityMention `json:"entities,omitempty"`
	// Objectives are the step's objectives. guide.json may state them; the
	// loader adds those converted from complete_when/prerequisites and those
	// of the step's annotation (docs/09 section 4.2).
	Objectives []Objective `json:"objectives,omitempty"`
	// LocationRef is the canonical location of the step (from guide.json or
	// its annotation); Location stays the source text.
	LocationRef *entities.Ref `json:"location_ref,omitempty"`
	// Requirements and BankActions come only from annotations. Requirements
	// are the items the step needs; BankActions are the annotated bank
	// actions that could not replace a segment's own (the step does not start
	// its segment). Both are composed by the loader, never read from guide.json.
	Requirements []ItemRequirement `json:"requirements,omitempty"`
	BankActions  []BankAction      `json:"bank_actions,omitempty"`
	// TextHash is the hash of the step's source text the annotations are
	// pinned to (set by the loader; see StepTextHash). It is never encoded:
	// guide.json does not store it.
	TextHash string `json:"-"`

	// SectionID is the id of the section that contains the step (set by the loader).
	SectionID string `json:"-"`
	// GuideID is the id of the guide that contains the step (set by the loader).
	GuideID string `json:"-"`
	// VariantID is the variant that contains the step (set by the loader).
	VariantID string `json:"-"`
	// SegmentID is the segment that contains the step (set by the loader).
	SegmentID string `json:"-"`
}

// IsManual reports whether the step has nothing that completes it
// automatically: no complete_when rule and no completing objective
// (checkbox).
func (s *Step) IsManual() bool {
	if len(s.CompleteWhen) > 0 {
		return false
	}
	for _, o := range s.Objectives {
		if o.Completing() {
			return false
		}
	}
	return true
}

// EntityMention is one entity reference of a step that resolved with exact
// confidence. Ref is the identity; SourceText (the original text as written)
// and Field (a JSON pointer into the step) are the evidence. Display names
// and wiki URLs are not stored: they derive from the entity registry.
type EntityMention struct {
	Ref        entities.Ref `json:"ref"`
	SourceText string       `json:"source_text"`
	Field      string       `json:"field"`
	// Span is [start, end) in bytes within the string at Field, for
	// mentions found in free text.
	Span   []int  `json:"span,omitempty"`
	Method string `json:"method"`
	Match  string `json:"match"`
}

// Mention methods: where in the step the reference was found.
const (
	MentionRule = "rule"
	MentionItem = "item"
	MentionLink = "link"
	MentionText = "text"
)

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
	g.variantsByID = make(map[string]*Variant, len(g.Variants))
	g.sectionsByID = make(map[string]*Section)
	g.sectionsBySl = make(map[string]*Section)
	g.segmentsByID = make(map[string]*Segment)
	quests := map[string]struct{}{}

	indexSection := func(sec *Section, variantID string) {
		sec.VariantID = variantID
		g.sectionsByID[sec.ID] = sec
		if sec.Slug != "" {
			g.sectionsBySl[sec.Slug] = sec
		}
		if len(sec.Segments) > 0 {
			sec.Steps = nil
			sec.segmentsByID = make(map[string]*Segment, len(sec.Segments))
			for _, segment := range sec.Segments {
				segment.VariantID = variantID
				segment.SectionID = sec.ID
				sec.segmentsByID[segment.ID] = segment
				g.segmentsByID[segment.ID] = segment
				sec.Steps = append(sec.Steps, segment.Steps...)
				for _, st := range segment.Steps {
					st.SegmentID = segment.ID
				}
			}
		} else {
			sec.segmentsByID = nil
		}
		for _, st := range sec.Steps {
			st.SectionID = sec.ID
			st.GuideID = g.ID
			st.VariantID = variantID
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

	if len(g.Variants) > 0 {
		g.Sections = nil
		for _, variant := range g.Variants {
			g.variantsByID[variant.ID] = variant
			for _, sec := range variant.Sections {
				indexSection(sec, variant.ID)
				g.Sections = append(g.Sections, sec)
			}
		}
	} else {
		for _, sec := range g.Sections {
			indexSection(sec, "")
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

// Variant returns the route variant with the given id.
func (g *Guide) Variant(id string) (*Variant, bool) {
	v, ok := g.variantsByID[id]
	return v, ok
}

// Section returns the section with the given id or slug.
func (g *Guide) Section(idOrSlug string) (*Section, bool) {
	if s, ok := g.sectionsByID[idOrSlug]; ok {
		return s, true
	}
	s, ok := g.sectionsBySl[idOrSlug]
	return s, ok
}

// Segment returns the segment with the given id.
func (g *Guide) Segment(id string) (*Segment, bool) {
	segment, ok := g.segmentsByID[id]
	return segment, ok
}

// Segment returns a segment within this section.
func (s *Section) Segment(id string) (*Segment, bool) {
	segment, ok := s.segmentsByID[id]
	return segment, ok
}

// Steps returns every step in guide order.
func (g *Guide) Steps() []*Step {
	var out []*Step
	for _, sec := range g.Sections {
		out = append(out, sec.Steps...)
	}
	return out
}

// Segments returns every segment in guide order.
func (g *Guide) Segments() []*Segment {
	var out []*Segment
	for _, sec := range g.Sections {
		out = append(out, sec.Segments...)
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
