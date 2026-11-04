package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/FelipeMalacarne/ironman-planner/content/schema"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// Annotation sidecars live next to guide.json (docs/09 section 4.2). Only
// `ironman content mcp` writes them; the loader composes them into the steps.
const (
	AnnotationsSubdir = "annotations"
	DecisionsFileName = "decisions.json"
)

// Annotation file kinds.
const (
	AnnotationKindSection   = "section"
	AnnotationKindDecisions = "decisions"
)

// AnnotationFile is annotations/<section id>.json: the accepted extraction
// output of one section.
type AnnotationFile struct {
	SchemaVersion int              `json:"schema_version"`
	Kind          string           `json:"kind"`
	GuideID       string           `json:"guide_id"`
	SectionID     string           `json:"section_id"`
	SkillVersion  string           `json:"skill_version"`
	Steps         []StepAnnotation `json:"steps"`
}

// StepAnnotation is the extraction output of one step, pinned to the hash of
// the step text it was made from. Items and BankActions, when present,
// replace what internal/core/content/derive derived for the step.
type StepAnnotation struct {
	StepID      string                `json:"step_id"`
	TextHash    string                `json:"text_hash"`
	Objectives  []Objective           `json:"objectives,omitempty"`
	Items       []AnnotatedItem       `json:"items,omitempty"`
	BankActions []AnnotatedBankAction `json:"bank_actions,omitempty"`
	Location    *AnnotatedLocation    `json:"location,omitempty"`
}

// AnnotatedItem is an item requirement read from the step text. Slot is
// "inventory" (default) or "equipment".
type AnnotatedItem struct {
	ItemRequirement
	Slot     string            `json:"slot,omitempty"`
	Evidence ObjectiveEvidence `json:"evidence"`
	Source   ObjectiveSource   `json:"source"`
}

// AnnotatedBankAction is a bank action read from the step text; the quote is
// the action's text.
type AnnotatedBankAction struct {
	Type     string            `json:"type"`
	Position string            `json:"position"`
	Evidence ObjectiveEvidence `json:"evidence"`
	Source   ObjectiveSource   `json:"source"`
}

// AnnotatedLocation is the canonical location of the step.
type AnnotatedLocation struct {
	Ref      entities.Ref      `json:"ref"`
	Evidence ObjectiveEvidence `json:"evidence"`
	Source   ObjectiveSource   `json:"source"`
}

// DecisionsFile is annotations/decisions.json: general rules the owner
// decided during extraction, reapplied to later batches.
type DecisionsFile struct {
	SchemaVersion int        `json:"schema_version"`
	Kind          string     `json:"kind"`
	GuideID       string     `json:"guide_id"`
	Decisions     []Decision `json:"decisions"`
}

// Decision is one question asked to the owner and the answer.
type Decision struct {
	ID        string   `json:"id"`
	Question  string   `json:"question"`
	Answer    string   `json:"answer"`
	Scope     string   `json:"scope"`
	DecidedAt string   `json:"decided_at"`
	StepIDs   []string `json:"step_ids,omitempty"`
}

// Slots of an annotated item.
const (
	SlotInventory = "inventory"
	SlotEquipment = "equipment"
)

// Annotations is what was read for one guide.
type Annotations struct {
	Sections  []*AnnotationFile
	Decisions *DecisionsFile
}

// loadAnnotations reads, validates and composes the sidecars of g, which was
// loaded from guideDir. Stale annotations (text hash differs from the
// current step) are skipped and returned as warnings: `content validate`
// fails on them, `serve` logs them and goes on. Every other problem is an
// error.
func loadAnnotations(guideDir string, g *Guide, reg *entities.Registry) (*Annotations, Errors, Errors) {
	dir := filepath.Join(guideDir, AnnotationsSubdir)
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		return nil, nil, nil
	}
	sort.Strings(files)
	out := &Annotations{}
	var issues, warnings Errors
	seenStep := map[string]string{}
	stale := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			issues = append(issues, Issue{File: f, Message: err.Error()})
			continue
		}
		if err := ValidateAnnotationJSON(f, data); err != nil {
			if es, ok := AsErrors(err); ok {
				issues = append(issues, es...)
				continue
			}
			return nil, nil, Errors{{File: f, Message: err.Error()}}
		}
		if filepath.Base(f) == DecisionsFileName {
			var d DecisionsFile
			if err := json.Unmarshal(data, &d); err != nil || d.Kind != AnnotationKindDecisions {
				issues = append(issues, Issue{File: f, Message: "decisions.json must be a decisions file"})
				continue
			}
			if d.GuideID != g.ID {
				issues = append(issues, Issue{File: f, Pointer: "/guide_id", Message: fmt.Sprintf("guide_id %q is not %q", d.GuideID, g.ID)})
			}
			out.Decisions = &d
			continue
		}
		var a AnnotationFile
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&a); err != nil {
			issues = append(issues, Issue{File: f, Message: "decode: " + err.Error()})
			continue
		}
		fileIssues, fileWarnings := g.checkAnnotationFile(f, &a, seenStep, stale, reg)
		issues = append(issues, fileIssues...)
		warnings = append(warnings, fileWarnings...)
		if len(fileIssues) == 0 {
			out.Sections = append(out.Sections, &a)
		}
	}
	if len(issues) > 0 {
		return nil, issues, warnings
	}
	for _, a := range out.Sections {
		for i := range a.Steps {
			if sa := &a.Steps[i]; !stale[sa.StepID] {
				g.composeStep(sa, a.SkillVersion)
			}
		}
	}
	g.Index()
	return out, nil, warnings
}

// checkAnnotationFile validates one section sidecar against the guide and
// records the ids of stale steps in stale.
func (g *Guide) checkAnnotationFile(file string, a *AnnotationFile, seenStep map[string]string, stale map[string]bool, reg *entities.Registry) (Errors, Errors) {
	var issues, warnings Errors
	add := func(ptr, format string, args ...any) {
		issues = append(issues, Issue{File: file, Pointer: ptr, Message: fmt.Sprintf(format, args...)})
	}
	if a.Kind != AnnotationKindSection {
		add("/kind", "annotation files other than decisions.json must have kind %q", AnnotationKindSection)
		return issues, nil
	}
	if a.GuideID != g.ID {
		add("/guide_id", "guide_id %q is not %q", a.GuideID, g.ID)
	}
	sec, ok := g.sectionsByID[a.SectionID]
	if !ok {
		add("/section_id", "unknown section %q", a.SectionID)
		return issues, nil
	}
	if want := a.SectionID + ".json"; filepath.Base(file) != want {
		add("/section_id", "section %q must live in %s", a.SectionID, want)
	}
	for i, sa := range a.Steps {
		ptr := fmt.Sprintf("/steps/%d", i)
		st, ok := g.stepsByID[sa.StepID]
		if !ok {
			add(ptr+"/step_id", "unknown step %q", sa.StepID)
			continue
		}
		if st.SectionID != sec.ID {
			add(ptr+"/step_id", "step %q belongs to section %q, not %q", sa.StepID, st.SectionID, sec.ID)
		}
		if prev, dup := seenStep[sa.StepID]; dup {
			add(ptr+"/step_id", "step %q is already annotated in %s", sa.StepID, prev)
			continue
		}
		seenStep[sa.StepID] = file
		if sa.TextHash != StepTextHash(st) {
			warnings = append(warnings, Issue{File: file, Pointer: ptr + "/text_hash",
				Message: fmt.Sprintf("annotation of step %q is stale (text_hash differs); reprocess the step", sa.StepID)})
			stale[sa.StepID] = true
			continue
		}
		doc := stepDoc(st)
		for j, o := range sa.Objectives {
			oPtr := fmt.Sprintf("%s/objectives/%d", ptr, j)
			if o.Evidence == nil || o.Evidence.Quote == "" {
				add(oPtr+"/evidence", "annotated objectives need a quote")
			}
			for _, msg := range checkObjective(st, doc, o, reg) {
				add(oPtr, "%s", msg)
			}
		}
		for j, it := range sa.Items {
			iPtr := fmt.Sprintf("%s/items/%d", ptr, j)
			for _, msg := range checkRequirement(it.ItemRequirement, reg) {
				add(iPtr, "%s", msg)
			}
			if it.Variant() == RequireUnresolved {
				add(iPtr, "annotated items must be resolved (item_id, any_of or family)")
			}
			if msg := verifyQuote(doc, it.Evidence); msg != "" {
				add(iPtr+"/evidence", "%s", msg)
			}
		}
		for j, ba := range sa.BankActions {
			if msg := verifyQuote(doc, ba.Evidence); msg != "" {
				add(fmt.Sprintf("%s/bank_actions/%d/evidence", ptr, j), "%s", msg)
			}
		}
		if sa.Location != nil {
			if sa.Location.Ref.Kind != entities.KindLocation {
				add(ptr+"/location/ref", "location must be a location ref, got %s", sa.Location.Ref)
			} else if !reg.Known(sa.Location.Ref) {
				add(ptr+"/location/ref", "unknown entity %s", sa.Location.Ref)
			}
			if msg := verifyQuote(doc, sa.Location.Evidence); msg != "" {
				add(ptr+"/location/evidence", "%s", msg)
			}
		}
	}
	return issues, warnings
}

// composeStep overlays a verified annotation on its step. Objectives merge
// (the annotation wins for the same key and role); the location ref is set.
// Items and bank actions replace the derived loadout and bank actions of the
// step's segment when the step starts it, since derive builds both from a
// segment's first step; otherwise they stay on the step.
func (g *Guide) composeStep(sa *StepAnnotation, skillVersion string) {
	st := g.stepsByID[sa.StepID]
	for i := range sa.Objectives {
		if sa.Objectives[i].Source.SkillVersion == "" && sa.Objectives[i].Source.Method == ProvenanceLLM {
			sa.Objectives[i].Source.SkillVersion = skillVersion
		}
	}
	st.Objectives = mergeObjectives(st.Objectives, sa.Objectives)
	if sa.Location != nil {
		ref := sa.Location.Ref
		st.LocationRef = &ref
	}
	seg := g.segmentsByID[st.SegmentID]
	starts := seg != nil && len(seg.Steps) > 0 && seg.Steps[0] == st
	if len(sa.Items) > 0 {
		inv, equip := splitItems(sa.Items)
		if starts {
			lo := &Loadout{Status: LoadoutPartial, Inventory: inv, Equipment: equip, Source: annotationProvenance(st.ID, sa.Items[0].Source, "annotation items")}
			if seg.Loadout != nil {
				lo.EmptySlots = seg.Loadout.EmptySlots
			}
			seg.Loadout = lo
		}
		st.Requirements = append(append([]ItemRequirement(nil), inv...), equip...)
	}
	if len(sa.BankActions) > 0 {
		actions := make([]BankAction, len(sa.BankActions))
		for i, ba := range sa.BankActions {
			actions[i] = BankAction{Type: ba.Type, Position: ba.Position, Text: ba.Evidence.Quote,
				Source: *annotationProvenance(st.ID, ba.Source, ba.Evidence.Quote)}
		}
		if starts {
			kept := seg.BankActions[:0:0]
			for _, a := range seg.BankActions {
				if !slices.Contains(a.Source.StepIDs, st.ID) {
					kept = append(kept, a)
				}
			}
			kept = append(kept, actions...)
			seg.BankActions = kept
			seg.Kind = SegmentBankLeg
		} else {
			st.BankActions = actions
		}
	}
}

func splitItems(items []AnnotatedItem) (inv, equip []ItemRequirement) {
	for _, it := range items {
		r := it.ItemRequirement
		if r.SourceText == "" {
			r.SourceText = it.Evidence.Quote
		}
		if it.Slot == SlotEquipment {
			equip = append(equip, r)
		} else {
			inv = append(inv, r)
		}
	}
	return inv, equip
}

func annotationProvenance(stepID string, src ObjectiveSource, evidence string) *Provenance {
	return &Provenance{Method: src.Method, Confidence: src.Confidence, StepIDs: []string{stepID}, Evidence: evidence}
}

var (
	annotationSchemaOnce     sync.Once
	annotationSchemaCompiled *jsonschema.Schema
	annotationSchemaErr      error
)

// AnnotationSchema returns the compiled annotations schema.
func AnnotationSchema() (*jsonschema.Schema, error) {
	annotationSchemaOnce.Do(func() {
		c := jsonschema.NewCompiler()
		c.AssertFormat()
		for url, raw := range map[string][]byte{schema.GuideSchemaURL: schema.GuideSchema, schema.AnnotationsSchemaURL: schema.AnnotationsSchema} {
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				annotationSchemaErr = fmt.Errorf("content: embedded schema %s is not valid JSON: %w", url, err)
				return
			}
			if err := c.AddResource(url, doc); err != nil {
				annotationSchemaErr = fmt.Errorf("content: add schema resource: %w", err)
				return
			}
		}
		annotationSchemaCompiled, annotationSchemaErr = c.Compile(schema.AnnotationsSchemaURL)
		if annotationSchemaErr != nil {
			annotationSchemaErr = fmt.Errorf("content: compile annotations schema: %w", annotationSchemaErr)
		}
	})
	return annotationSchemaCompiled, annotationSchemaErr
}

// ValidateAnnotationJSON checks one sidecar against the annotations schema.
func ValidateAnnotationJSON(name string, data []byte) error {
	sch, err := AnnotationSchema()
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return Errors{{File: name, Message: "invalid JSON: " + err.Error()}}
	}
	if err := sch.Validate(doc); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			var issues Errors
			flattenValidation(name, ve, &issues)
			return issues.errOrNil()
		}
		return Errors{{File: name, Message: err.Error()}}
	}
	return nil
}
