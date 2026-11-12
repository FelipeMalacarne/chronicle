package annotate

import (
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
)

// Batch limits: a batch stops at DefaultBatchSteps steps or once its text
// passes MaxBatchChars, but always carries at least one step (a BRUHsailer
// step alone can be several thousand characters).
const (
	DefaultBatchSteps = 8
	MaxBatchChars     = 12000
)

// Batch is the next group of steps of a section to annotate, in guide order.
type Batch struct {
	GuideID      string      `json:"guide_id"`
	SectionID    string      `json:"section_id"`
	SectionTitle string      `json:"section_title"`
	Steps        []BatchStep `json:"steps"`
	// Remaining counts the steps of the section still pending after this batch.
	Remaining int `json:"remaining"`
	// Decisions are the owner's general rules for this guide, to apply
	// without asking again.
	Decisions []content.Decision `json:"decisions,omitempty"`
}

// BatchStep is one step as the agent sees it: the fields an annotation may
// quote, plus context that is not quotable.
type BatchStep struct {
	StepID   string         `json:"step_id"`
	Text     string         `json:"text"`
	Location string         `json:"location,omitempty"`
	Notes    []content.Note `json:"notes,omitempty"`
	Items    []content.Item `json:"items,omitempty"`
	Links    []content.Link `json:"links,omitempty"`
	// Mentions are the entities the importers already resolved in the step.
	Mentions []content.EntityMention `json:"mentions,omitempty"`
	// RuleObjectives are the objectives the loader converts from the step's
	// rules; an annotation does not need to repeat them, but may.
	RuleObjectives []ObjectiveView `json:"rule_objectives,omitempty"`
	Segment        SegmentView     `json:"segment"`
	// Stale marks a step annotated before its text changed; Previous is that
	// annotation.
	Stale    bool                    `json:"stale,omitempty"`
	Previous *content.StepAnnotation `json:"previous,omitempty"`
	// PreviousText is the text of the step before, for context only.
	PreviousText string `json:"previous_text,omitempty"`
}

// ObjectiveView is an objective in short form.
type ObjectiveView struct {
	Key        string `json:"key"`
	Role       string `json:"role"`
	Method     string `json:"method"`
	Confidence string `json:"confidence"`
}

// SegmentView tells whether annotated items and bank actions of the step
// replace its segment's derived loadout and bank actions (only when the step
// starts the segment) and shows what was derived.
type SegmentView struct {
	ID          string               `json:"id"`
	Kind        string               `json:"kind"`
	StartsHere  bool                 `json:"starts_here"`
	Loadout     *content.Loadout     `json:"derived_loadout,omitempty"`
	BankActions []content.BankAction `json:"derived_bank_actions,omitempty"`
}

// NextBatch returns the next steps of a section without a current
// annotation (never annotated, or stale). limit <= 0 means
// DefaultBatchSteps.
func (w *Workspace) NextBatch(guideID, sectionID string, limit int) (*Batch, error) {
	gs, sec, err := w.section(guideID, sectionID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultBatchSteps
	}
	b := &Batch{GuideID: guideID, SectionID: sec.ID, SectionTitle: sec.Title, Steps: []BatchStep{}}
	if gs.decisions != nil {
		b.Decisions = gs.decisions.Decisions
	}
	chars := 0
	for i, st := range sec.Steps {
		if gs.fresh(st) {
			continue
		}
		if len(b.Steps) >= limit || (len(b.Steps) > 0 && chars+stepChars(st) > MaxBatchChars) {
			b.Remaining++
			continue
		}
		chars += stepChars(st)
		bs := w.batchStep(gs, st)
		if i > 0 {
			bs.PreviousText = sec.Steps[i-1].Text
		}
		b.Steps = append(b.Steps, bs)
	}
	return b, nil
}

func stepChars(st *content.Step) int {
	n := len(st.Text) + len(st.Location)
	for _, nt := range st.Notes {
		n += len(nt.Text)
	}
	return n
}

func (w *Workspace) batchStep(gs *guideState, st *content.Step) BatchStep {
	bs := BatchStep{
		StepID: st.ID, Text: st.Text, Location: st.Location, Notes: st.Notes, Items: st.Items, Links: st.Links,
		Mentions: st.Entities,
	}
	objs, _ := content.RuleObjectives(st, w.reg)
	for _, o := range objs {
		bs.RuleObjectives = append(bs.RuleObjectives, view(o))
	}
	if seg, ok := gs.guide.Segment(st.SegmentID); ok {
		bs.Segment = SegmentView{ID: seg.ID, Kind: seg.Kind, StartsHere: len(seg.Steps) > 0 && seg.Steps[0] == st}
		if bs.Segment.StartsHere {
			bs.Segment.Loadout = seg.Loadout
			bs.Segment.BankActions = seg.BankActions
		}
	}
	if sa := gs.annotation(st); sa != nil {
		bs.Stale = true
		bs.Previous = sa
	}
	return bs
}

func view(o content.Objective) ObjectiveView {
	return ObjectiveView{Key: o.Key.String(), Role: o.Role, Method: o.Source.Method, Confidence: o.Source.Confidence}
}

// Status is the annotation coverage of the guides.
type Status struct {
	Guides []GuideStatus `json:"guides"`
}

// GuideStatus is the coverage of one guide.
type GuideStatus struct {
	GuideID string `json:"guide_id"`
	Title   string `json:"title"`
	Coverage
	Decisions int             `json:"decisions"`
	Sections  []SectionStatus `json:"sections"`
}

// SectionStatus is the coverage of one section.
type SectionStatus struct {
	SectionID string `json:"section_id"`
	Title     string `json:"title"`
	Coverage
}

// Coverage counts steps by annotation state: annotated (current), stale
// (annotated, then the text changed) and pending (never annotated).
type Coverage struct {
	Steps     int `json:"steps"`
	Annotated int `json:"annotated"`
	Stale     int `json:"stale"`
	Pending   int `json:"pending"`
}

func (c *Coverage) add(o Coverage) {
	c.Steps += o.Steps
	c.Annotated += o.Annotated
	c.Stale += o.Stale
	c.Pending += o.Pending
}

// Status reports coverage for one guide, or for all when guideID is empty.
func (w *Workspace) Status(guideID string) (*Status, error) {
	ids := w.order
	if guideID != "" {
		if _, err := w.guide(guideID); err != nil {
			return nil, err
		}
		ids = []string{guideID}
	}
	out := &Status{}
	for _, id := range ids {
		gs := w.guides[id]
		g := GuideStatus{GuideID: id, Title: gs.guide.Title}
		if gs.decisions != nil {
			g.Decisions = len(gs.decisions.Decisions)
		}
		for _, sec := range gs.guide.Sections {
			s := SectionStatus{SectionID: sec.ID, Title: sec.Title, Coverage: gs.coverage(sec)}
			g.add(s.Coverage)
			g.Sections = append(g.Sections, s)
		}
		out.Guides = append(out.Guides, g)
	}
	return out, nil
}

func (gs *guideState) coverage(sec *content.Section) Coverage {
	c := Coverage{Steps: len(sec.Steps)}
	for _, st := range sec.Steps {
		switch sa := gs.annotation(st); {
		case sa == nil:
			c.Pending++
		case sa.TextHash == content.StepTextHash(st):
			c.Annotated++
		default:
			c.Stale++
		}
	}
	return c
}
