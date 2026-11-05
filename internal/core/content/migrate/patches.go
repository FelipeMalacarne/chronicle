package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

// RoutePatches are the owner's corrections to the imported route
// (data/guide/route-patches.json): steps the game made impossible or
// misordered, fixed at import time so guide.json stays generated
// (docs/05 section 12). Patches apply in order, ops in order.
type RoutePatches struct {
	SchemaVersion int          `json:"schema_version"`
	GuideID       string       `json:"guide_id"`
	Patches       []RoutePatch `json:"patches"`
}

// RoutePatch is one correction with its reason.
type RoutePatch struct {
	ID     string    `json:"id"`
	Reason string    `json:"reason"`
	Ops    []PatchOp `json:"ops"`
}

// PatchOp is one operation of a patch:
//   - drop: remove Step (no other step may require it);
//   - move: put Step right after After (same section);
//   - insert: add New right after After (a new id, origin "own");
//   - note: replace Step's update note with Note (the note that asked for the patch);
//   - items: replace Step's items with Items.
type PatchOp struct {
	Op    string    `json:"op"`
	Step  string    `json:"step,omitempty"`
	After string    `json:"after,omitempty"`
	New   *NewStep  `json:"new,omitempty"`
	Note  string    `json:"note,omitempty"`
	Items *[]string `json:"items,omitempty"`
}

// NewStep is a step the planner adds to the route.
type NewStep struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Location string   `json:"location,omitempty"`
	Items    []string `json:"items,omitempty"`
}

// Patch operations.
const (
	OpDrop   = "drop"
	OpMove   = "move"
	OpInsert = "insert"
	OpNote   = "note"
	OpItems  = "items"
)

// ReadRoutePatches parses a route patch file.
func ReadRoutePatches(path string) (*RoutePatches, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p RoutePatches
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("migrate: parse %s: %w", path, err)
	}
	if p.SchemaVersion != 1 {
		return nil, fmt.Errorf("migrate: %s has schema_version %d, want 1", path, p.SchemaVersion)
	}
	return &p, nil
}

// applyPatches edits the converted sections in place. Every op must apply:
// a patch that no longer matches the imported guide is an error, never
// silently skipped.
func applyPatches(sections []*content.Section, p *RoutePatches, guideID, imported string) error {
	if p == nil {
		return nil
	}
	if p.GuideID != guideID {
		return fmt.Errorf("route patches are for %q, not %q", p.GuideID, guideID)
	}
	for _, patch := range p.Patches {
		if patch.ID == "" || strings.TrimSpace(patch.Reason) == "" {
			return fmt.Errorf("route patch %q needs an id and a reason", patch.ID)
		}
		for i, op := range patch.Ops {
			if err := applyOp(sections, op, imported); err != nil {
				return fmt.Errorf("route patch %s op %d (%s): %w", patch.ID, i, op.Op, err)
			}
		}
	}
	return nil
}

func applyOp(sections []*content.Section, op PatchOp, imported string) error {
	switch op.Op {
	case OpDrop:
		sec, at, err := findStep(sections, op.Step)
		if err != nil {
			return err
		}
		for _, s := range sections {
			for _, st := range s.Steps {
				if contains(st.Requires, "step:"+op.Step) {
					return fmt.Errorf("step %s is required by %s", op.Step, st.ID)
				}
			}
		}
		sec.Steps = append(sec.Steps[:at], sec.Steps[at+1:]...)
	case OpMove:
		sec, at, err := findStep(sections, op.Step)
		if err != nil {
			return err
		}
		st := sec.Steps[at]
		sec.Steps = append(sec.Steps[:at], sec.Steps[at+1:]...)
		return insertAfter(sections, sec, op.After, st)
	case OpInsert:
		n := op.New
		if n == nil || n.ID == "" || strings.TrimSpace(n.Text) == "" {
			return fmt.Errorf("insert needs new.id and new.text")
		}
		if _, _, err := findStep(sections, n.ID); err == nil {
			return fmt.Errorf("step id %s already exists", n.ID)
		}
		sec, _, err := findStep(sections, op.After)
		if err != nil {
			return err
		}
		st := &content.Step{
			ID:       n.ID,
			Text:     n.Text,
			Location: n.Location,
			Modes:    []string{"ironman", "hcim", "uim", "gim"},
			Layers:   []string{"classic", "enhanced"},
			Source:   &content.Source{Origin: "own", ImportedAt: imported},
		}
		for _, it := range n.Items {
			st.Items = append(st.Items, content.Item{Name: it})
		}
		return insertAfter(sections, sec, op.After, st)
	case OpNote:
		sec, at, err := findStep(sections, op.Step)
		if err != nil {
			return err
		}
		st := sec.Steps[at]
		for i := range st.Notes {
			if st.Notes[i].Kind == content.NoteUpdate {
				st.Notes[i].Text = op.Note
				return nil
			}
		}
		return fmt.Errorf("step %s has no update note", op.Step)
	case OpItems:
		if op.Items == nil {
			return fmt.Errorf("items op needs items")
		}
		sec, at, err := findStep(sections, op.Step)
		if err != nil {
			return err
		}
		st := sec.Steps[at]
		st.Items = nil
		for _, it := range *op.Items {
			st.Items = append(st.Items, content.Item{Name: it})
		}
	default:
		return fmt.Errorf("unknown op %q", op.Op)
	}
	return nil
}

func findStep(sections []*content.Section, id string) (*content.Section, int, error) {
	for _, s := range sections {
		for i, st := range s.Steps {
			if st.ID == id {
				return s, i, nil
			}
		}
	}
	return nil, 0, fmt.Errorf("unknown step %q", id)
}

// insertAfter places st right after the step after, which must be in sec.
func insertAfter(sections []*content.Section, sec *content.Section, after string, st *content.Step) error {
	at, inSec, err := findStep(sections, after)
	if err != nil {
		return err
	}
	if at != sec {
		return fmt.Errorf("step %s is in section %s, not %s", after, at.ID, sec.ID)
	}
	sec.Steps = append(sec.Steps[:inSec+1], append([]*content.Step{st}, sec.Steps[inSec+1:]...)...)
	return nil
}
