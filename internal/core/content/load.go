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
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/FelipeMalacarne/chronicle/content/schema"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// GuidesSubdir is the directory under the content dir that holds one folder per guide.
const GuidesSubdir = "guides"

// GuideFileName is the file inside each guide folder.
const GuideFileName = "guide.json"

// Content is everything loaded from a content directory.
type Content struct {
	Dir    string
	Guides []*Guide
	// Annotations holds the sidecars read for each guide, by guide id.
	Annotations map[string]*Annotations
	// Recipes holds the recipes of content/recipes by objective key
	// (docs/10 section 8).
	Recipes map[string]*Recipe
	// Warnings are problems that do not stop the content from loading: stale
	// annotations, which the loader skips. `content validate` fails on them.
	Warnings Errors
}

// Load reads <dir>/guides/*/guide.json, validates each file against the
// schema, decodes it, builds the indexes, then reads, verifies and composes
// the annotation sidecars of each guide (docs/09 section 4.2), tasks
// included, and reads and verifies the recipes of <dir>/recipes (docs/10
// sections 7 and 8). All problems across all files are collected and
// returned together as an Errors value; stale annotations are skipped and
// reported in Content.Warnings.
func Load(dir string) (*Content, error) {
	pattern := filepath.Join(dir, GuidesSubdir, "*", GuideFileName)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("content: glob %s: %w", pattern, err)
	}
	if len(files) == 0 {
		return nil, Errors{{File: pattern, Message: "no guide files found"}}
	}
	sort.Strings(files)

	c := &Content{Dir: dir, Annotations: map[string]*Annotations{}, Recipes: map[string]*Recipe{}}
	var issues Errors
	reg, regErr := entities.Default()
	if regErr != nil {
		return nil, fmt.Errorf("content: %w", regErr)
	}
	for _, f := range files {
		g, err := LoadGuideFile(f)
		if err != nil {
			if es, ok := AsErrors(err); ok {
				issues = append(issues, es...)
				continue
			}
			return nil, err
		}
		for _, prev := range c.Guides {
			if prev.ID == g.ID {
				issues = append(issues, Issue{File: f, Pointer: "/id", Message: fmt.Sprintf("duplicate guide id %q (also in %s)", g.ID, prev.File)})
			}
		}
		g.composeRuleObjectives(reg)
		ann, annIssues, annWarnings := loadAnnotations(filepath.Dir(f), g, reg)
		issues = append(issues, annIssues...)
		c.Warnings = append(c.Warnings, annWarnings...)
		if ann != nil {
			c.Annotations[g.ID] = ann
		}
		c.Guides = append(c.Guides, g)
	}
	recipes, recipeIssues := loadRecipes(dir, reg)
	issues = append(issues, recipeIssues...)
	c.Recipes = recipes
	if err := issues.errOrNil(); err != nil {
		return nil, err
	}
	return c, nil
}

// TaskCount returns how many tasks the steps of every guide carry.
func (c *Content) TaskCount() int {
	n := 0
	for _, st := range c.Steps() {
		n += len(st.Tasks)
	}
	return n
}

// LoadGuideFile validates and decodes a single guide.json.
func LoadGuideFile(path string) (*Guide, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	g, err := ParseGuide(path, data)
	if err != nil {
		return nil, err
	}
	g.File = path
	return g, nil
}

// ParseGuide validates data against the schema, decodes it and runs the
// semantic checks that the schema cannot express (unique ids, resolvable
// step references). name is used in error messages only.
func ParseGuide(name string, data []byte) (*Guide, error) {
	if err := ValidateJSON(name, data); err != nil {
		return nil, err
	}
	var g Guide
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&g); err != nil {
		return nil, Errors{{File: name, Message: "decode: " + err.Error()}}
	}
	g.File = name
	g.Index()
	if err := g.check(name).errOrNil(); err != nil {
		return nil, err
	}
	return &g, nil
}

// check runs semantic validation on an indexed guide.
func (g *Guide) check(name string) Errors {
	var issues Errors
	seenVariant := map[string]string{}
	seenSec := map[string]string{}
	seenSlug := map[string]string{}
	seenSegment := map[string]string{}
	seenStep := map[string]string{}
	type sectionAt struct {
		section *Section
		pointer string
	}
	type stepAt struct {
		step    *Step
		pointer string
	}
	var sections []sectionAt
	if len(g.Variants) > 0 {
		for vi, variant := range g.Variants {
			variantPtr := fmt.Sprintf("/variants/%d", vi)
			if first, dup := seenVariant[variant.ID]; dup {
				issues = append(issues, Issue{File: name, Pointer: variantPtr + "/id", Message: fmt.Sprintf("duplicate variant id %q (first at %s)", variant.ID, first)})
			} else {
				seenVariant[variant.ID] = variantPtr
			}
			for si, sec := range variant.Sections {
				sections = append(sections, sectionAt{section: sec, pointer: fmt.Sprintf("%s/sections/%d", variantPtr, si)})
			}
		}
		if _, ok := seenVariant[g.DefaultVariant]; !ok {
			issues = append(issues, Issue{File: name, Pointer: "/default_variant", Message: fmt.Sprintf("unknown variant %q", g.DefaultVariant)})
		}
	} else {
		for si, sec := range g.Sections {
			sections = append(sections, sectionAt{section: sec, pointer: fmt.Sprintf("/sections/%d", si)})
		}
	}

	var steps []stepAt
	reg, regErr := entities.Default()
	if regErr != nil {
		issues = append(issues, Issue{File: name, Message: regErr.Error()})
	}
	checkProvenance := func(source Provenance, ptr string, allowed map[string]bool) {
		for i, stepID := range source.StepIDs {
			if _, ok := g.stepsByID[stepID]; !ok {
				issues = append(issues, Issue{File: name, Pointer: fmt.Sprintf("%s/step_ids/%d", ptr, i), Message: fmt.Sprintf("unknown provenance step %q", stepID)})
				continue
			}
			if allowed != nil && !allowed[stepID] {
				issues = append(issues, Issue{File: name, Pointer: fmt.Sprintf("%s/step_ids/%d", ptr, i), Message: fmt.Sprintf("provenance step %q is outside this segment", stepID)})
			}
		}
	}
	for _, entry := range sections {
		sec, secPtr := entry.section, entry.pointer
		if first, dup := seenSec[sec.ID]; dup {
			issues = append(issues, Issue{File: name, Pointer: secPtr + "/id", Message: fmt.Sprintf("duplicate section id %q (first at %s)", sec.ID, first)})
		} else {
			seenSec[sec.ID] = secPtr
		}
		if first, dup := seenSlug[sec.Slug]; dup {
			issues = append(issues, Issue{File: name, Pointer: secPtr + "/slug", Message: fmt.Sprintf("duplicate section slug %q (first at %s)", sec.Slug, first)})
		} else {
			seenSlug[sec.Slug] = secPtr
		}
		if len(sec.Segments) == 0 {
			for ti, st := range sec.Steps {
				steps = append(steps, stepAt{step: st, pointer: fmt.Sprintf("%s/steps/%d", secPtr, ti)})
			}
			continue
		}
		for mi, segment := range sec.Segments {
			segmentPtr := fmt.Sprintf("%s/segments/%d", secPtr, mi)
			segmentStepIDs := make(map[string]bool, len(segment.Steps))
			for _, step := range segment.Steps {
				segmentStepIDs[step.ID] = true
			}
			if first, dup := seenSegment[segment.ID]; dup {
				issues = append(issues, Issue{File: name, Pointer: segmentPtr + "/id", Message: fmt.Sprintf("duplicate segment id %q (first at %s)", segment.ID, first)})
			} else {
				seenSegment[segment.ID] = segmentPtr
			}
			if segment.Kind == SegmentBankLeg && len(segment.BankActions) == 0 {
				issues = append(issues, Issue{File: name, Pointer: segmentPtr + "/bank_actions", Message: "bank_leg requires at least one explicit bank action"})
			}
			if segment.Kind == SegmentSequence && len(segment.BankActions) > 0 {
				issues = append(issues, Issue{File: name, Pointer: segmentPtr + "/bank_actions", Message: "sequence cannot claim bank actions"})
			}
			checkProvenance(segment.Source, segmentPtr+"/source", segmentStepIDs)
			for ai, action := range segment.BankActions {
				checkProvenance(action.Source, fmt.Sprintf("%s/bank_actions/%d/source", segmentPtr, ai), segmentStepIDs)
			}
			if segment.Loadout != nil && segment.Loadout.Source != nil {
				checkProvenance(*segment.Loadout.Source, segmentPtr+"/loadout/source", segmentStepIDs)
			}
			if reg != nil {
				issues = append(issues, checkLoadoutItemIDs(name, segmentPtr, segment.Loadout, reg)...)
			}
			for ti, st := range segment.Steps {
				steps = append(steps, stepAt{step: st, pointer: fmt.Sprintf("%s/steps/%d", segmentPtr, ti)})
			}
		}
	}

	for _, entry := range steps {
		st, ptr := entry.step, entry.pointer
		if first, dup := seenStep[st.ID]; dup {
			issues = append(issues, Issue{File: name, Pointer: ptr + "/id", Message: fmt.Sprintf("duplicate step id %q (first at %s)", st.ID, first)})
		} else {
			seenStep[st.ID] = ptr
		}
		st.TextHash = StepTextHash(st)
		issues = append(issues, checkStepMarks(name, ptr, st)...)
		if reg != nil {
			issues = append(issues, g.checkStepEntities(name, ptr, st, reg)...)
			issues = append(issues, g.checkStepObjectives(name, ptr, st, reg)...)
		}
		// References to other steps must resolve (quests/skills/items are
		// resolved later by the graph builder against the wiki).
		for field, refs := range map[string][]string{"requires": st.Requires, "recommended_after": st.RecommendedAfter} {
			for ri, ref := range refs {
				id, ok := strings.CutPrefix(ref, "step:")
				if !ok {
					continue
				}
				if _, found := g.stepsByID[id]; !found {
					issues = append(issues, Issue{File: name, Pointer: fmt.Sprintf("%s/%s/%d", ptr, field, ri), Message: fmt.Sprintf("unknown step %q", id)})
				}
				if id == st.ID {
					issues = append(issues, Issue{File: name, Pointer: fmt.Sprintf("%s/%s/%d", ptr, field, ri), Message: "step references itself"})
				}
			}
		}
	}
	return issues
}

var (
	schemaOnce     sync.Once
	schemaCompiled *jsonschema.Schema
	schemaErr      error
	printer        = message.NewPrinter(language.English)
)

// Schema returns the compiled canonical guide schema.
func Schema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.GuideSchema))
		if err != nil {
			schemaErr = fmt.Errorf("content: embedded schema is not valid JSON: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		c.AssertFormat()
		if err := c.AddResource(schema.GuideSchemaURL, doc); err != nil {
			schemaErr = fmt.Errorf("content: add schema resource: %w", err)
			return
		}
		schemaCompiled, schemaErr = c.Compile(schema.GuideSchemaURL)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("content: compile schema: %w", schemaErr)
		}
	})
	return schemaCompiled, schemaErr
}

// ValidateJSON checks raw guide JSON against the schema and returns the
// violations as Errors (file name + JSON pointer + message).
func ValidateJSON(name string, data []byte) error {
	sch, err := Schema()
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
			return schemaIssues(name, ve).errOrNil()
		}
		return Errors{{File: name, Message: err.Error()}}
	}
	return nil
}

// notAllowed is the message of a property a `false` schema refuses: one
// unevaluatedProperties does not let through (a task parameter of another
// type) or one declared false (a recipe's decision).
const notAllowed = "property not allowed here"

// schemaIssues flattens a validation error into leaf issues. A refused
// property inside an object that has other errors is dropped: when a task
// fails its type's schema, none of its properties count as evaluated, and
// listing each as refused would only hide the real error.
func schemaIssues(file string, ve *jsonschema.ValidationError) Errors {
	var issues Errors
	flattenValidation(file, ve, &issues)
	var other []string
	for _, is := range issues {
		if is.Message != notAllowed {
			other = append(other, is.Pointer)
		}
	}
	out := issues[:0]
	for _, is := range issues {
		if is.Message == notAllowed {
			parent := is.Pointer[:strings.LastIndex(is.Pointer, "/")+1]
			if slices.ContainsFunc(other, func(p string) bool { return strings.HasPrefix(p+"/", parent) }) {
				continue
			}
		}
		out = append(out, is)
	}
	return out
}

// flattenValidation turns the error tree into leaf issues. For a oneOf/anyOf
// with no matching branch it follows only the branch with the fewest leaf
// errors (the one the author most likely meant), which keeps the output for
// a bad rule to one or two lines instead of one per rule type.
func flattenValidation(file string, e *jsonschema.ValidationError, out *Errors) {
	if len(e.Causes) == 0 {
		msg := e.ErrorKind.LocalizedString(printer)
		if _, refused := e.ErrorKind.(*kind.FalseSchema); refused {
			msg = notAllowed
		}
		*out = append(*out, Issue{File: file, Pointer: pointer(e.InstanceLocation), Message: msg})
		return
	}
	switch e.ErrorKind.(type) {
	case *kind.OneOf, *kind.AnyOf:
		best := e.Causes[0]
		bestN := countLeaves(best)
		for _, c := range e.Causes[1:] {
			if n := countLeaves(c); n < bestN {
				best, bestN = c, n
			}
		}
		flattenValidation(file, best, out)
		return
	}
	for _, c := range e.Causes {
		flattenValidation(file, c, out)
	}
}

func countLeaves(e *jsonschema.ValidationError) int {
	if len(e.Causes) == 0 {
		return 1
	}
	n := 0
	for _, c := range e.Causes {
		n += countLeaves(c)
	}
	return n
}

// pointer renders an instance location as an RFC 6901 JSON pointer.
func pointer(loc []string) string {
	if len(loc) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, tok := range loc {
		sb.WriteByte('/')
		sb.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(tok))
	}
	return sb.String()
}

// SelectGuide resolves an explicit guide id. An empty id selects the guide only
// when exactly one guide is loaded. The empty-id behavior is the compatibility
// path for callers that predate guide-scoped routing; it deliberately refuses
// to choose between multiple guides.
func (c *Content) SelectGuide(id string) (*Guide, bool) {
	if id != "" {
		return c.Guide(id)
	}
	if len(c.Guides) != 1 {
		return nil, false
	}
	return c.Guides[0], true
}

// StepInGuide returns a step from the named guide. Step ids are unique within
// a guide, but may be reused by other guides.
func (c *Content) StepInGuide(guideID, stepID string) (*Step, bool) {
	g, ok := c.Guide(guideID)
	if !ok {
		return nil, false
	}
	return g.Step(stepID)
}

// SectionInGuide returns a section by id or slug from the named guide.
// Section ids and slugs are unique within a guide, but may be reused by other
// guides.
func (c *Content) SectionInGuide(guideID, idOrSlug string) (*Section, bool) {
	g, ok := c.Guide(guideID)
	if !ok {
		return nil, false
	}
	return g.Section(idOrSlug)
}

// Step returns a step by id when that id has exactly one match across all
// guides. It is kept for single-guide compatibility. When multiple guides use
// the id, Step returns (nil, false); multi-guide callers must use StepInGuide.
func (c *Content) Step(id string) (*Step, bool) {
	var found *Step
	for _, g := range c.Guides {
		s, ok := g.Step(id)
		if !ok {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = s
	}
	return found, found != nil
}

// Guide returns a guide by id.
func (c *Content) Guide(id string) (*Guide, bool) {
	for _, g := range c.Guides {
		if g.ID == id {
			return g, true
		}
	}
	return nil, false
}

// Section returns a section matching id or slug when there is exactly one
// match across all guides, together with its guide. It is kept for single-guide
// compatibility. When multiple guides match, Section returns (nil, nil, false);
// multi-guide callers must use SectionInGuide.
func (c *Content) Section(idOrSlug string) (*Guide, *Section, bool) {
	var foundGuide *Guide
	var foundSection *Section
	for _, g := range c.Guides {
		if s, ok := g.Section(idOrSlug); ok {
			if foundSection != nil {
				return nil, nil, false
			}
			foundGuide, foundSection = g, s
		}
	}
	return foundGuide, foundSection, foundSection != nil
}

// StepCount returns the total number of steps across guides.
func (c *Content) StepCount() int {
	n := 0
	for _, g := range c.Guides {
		n += g.StepCount()
	}
	return n
}

// Steps returns every step across guides in guide order.
func (c *Content) Steps() []*Step {
	var out []*Step
	for _, g := range c.Guides {
		out = append(out, g.Steps()...)
	}
	return out
}
