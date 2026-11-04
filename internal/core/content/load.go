package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/FelipeMalacarne/ironman-planner/content/schema"
)

// GuidesSubdir is the directory under the content dir that holds one folder per guide.
const GuidesSubdir = "guides"

// GuideFileName is the file inside each guide folder.
const GuideFileName = "guide.json"

// Content is everything loaded from a content directory.
type Content struct {
	Dir    string
	Guides []*Guide

	steps map[string]*Step
}

// Load reads <dir>/guides/*/guide.json, validates each file against the
// schema, decodes it and builds the indexes. All problems across all files
// are collected and returned together as an Errors value.
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

	c := &Content{Dir: dir, steps: map[string]*Step{}}
	var issues Errors
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
		for _, st := range g.Steps() {
			if prev, dup := c.steps[st.ID]; dup {
				issues = append(issues, Issue{File: f, Pointer: "", Message: fmt.Sprintf("step id %q already used by guide %q", st.ID, prev.GuideID)})
				continue
			}
			c.steps[st.ID] = st
		}
		c.Guides = append(c.Guides, g)
	}
	if err := issues.errOrNil(); err != nil {
		return nil, err
	}
	return c, nil
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
	seenSec := map[string]int{}
	seenSlug := map[string]int{}
	seenStep := map[string]string{}
	for si, sec := range g.Sections {
		secPtr := fmt.Sprintf("/sections/%d", si)
		if j, dup := seenSec[sec.ID]; dup {
			issues = append(issues, Issue{File: name, Pointer: secPtr + "/id", Message: fmt.Sprintf("duplicate section id %q (first at /sections/%d)", sec.ID, j)})
		}
		seenSec[sec.ID] = si
		if j, dup := seenSlug[sec.Slug]; dup {
			issues = append(issues, Issue{File: name, Pointer: secPtr + "/slug", Message: fmt.Sprintf("duplicate section slug %q (first at /sections/%d)", sec.Slug, j)})
		}
		seenSlug[sec.Slug] = si
		for ti, st := range sec.Steps {
			ptr := fmt.Sprintf("%s/steps/%d", secPtr, ti)
			if first, dup := seenStep[st.ID]; dup {
				issues = append(issues, Issue{File: name, Pointer: ptr + "/id", Message: fmt.Sprintf("duplicate step id %q (first at %s)", st.ID, first)})
			} else {
				seenStep[st.ID] = ptr
			}
		}
	}
	// References to other steps must resolve (quests/skills/items are
	// resolved later by the graph builder against the wiki).
	for si, sec := range g.Sections {
		for ti, st := range sec.Steps {
			ptr := fmt.Sprintf("/sections/%d/steps/%d", si, ti)
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
			var issues Errors
			flattenValidation(name, ve, &issues)
			return issues.errOrNil()
		}
		return Errors{{File: name, Message: err.Error()}}
	}
	return nil
}

// flattenValidation turns the error tree into leaf issues. For a oneOf/anyOf
// with no matching branch it follows only the branch with the fewest leaf
// errors (the one the author most likely meant), which keeps the output for
// a bad rule to one or two lines instead of one per rule type.
func flattenValidation(file string, e *jsonschema.ValidationError, out *Errors) {
	if len(e.Causes) == 0 {
		*out = append(*out, Issue{File: file, Pointer: pointer(e.InstanceLocation), Message: e.ErrorKind.LocalizedString(printer)})
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

// Step returns a step by id across all guides.
func (c *Content) Step(id string) (*Step, bool) {
	s, ok := c.steps[id]
	return s, ok
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

// Section returns the first section matching id or slug across all guides,
// together with its guide.
func (c *Content) Section(idOrSlug string) (*Guide, *Section, bool) {
	for _, g := range c.Guides {
		if s, ok := g.Section(idOrSlug); ok {
			return g, s, true
		}
	}
	return nil, nil, false
}

// StepCount returns the total number of steps across guides.
func (c *Content) StepCount() int { return len(c.steps) }

// Steps returns every step across guides in guide order.
func (c *Content) Steps() []*Step {
	var out []*Step
	for _, g := range c.Guides {
		out = append(out, g.Steps()...)
	}
	return out
}
