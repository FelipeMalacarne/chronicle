// Package annotate is the core of guide extraction by an agent (docs/09
// section 6): it hands out batches of steps, resolves the names the agent
// proposes into canonical refs, verifies each proposal with the same checks
// the content loader runs, and writes the accepted annotations to the
// sidecars under content/guides/<id>/annotations/. It never calls an LLM;
// `chronicle content mcp` is a thin MCP layer over it.
//
// Only this package writes the sidecars, decisions.json, the extraction
// report and the owner-confirmed entries of unlocks.json and locations.json.
package annotate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// ReferenceSubdir is the catalog directory under the content dir.
const ReferenceSubdir = "reference"

// ReportFileName is the extraction report next to each guide.json.
const ReportFileName = "annotations.report.json"

// Workspace is the extraction state of a content directory: its guides, the
// sidecars read so far and the registry built from the catalogs on disk
// (not the embedded ones, so entities proposed in this session resolve at
// once). It is not safe for concurrent use.
type Workspace struct {
	Dir string
	// Today returns the date recorded on decisions (YYYY-MM-DD).
	Today func() string

	reg *entities.Registry
	// regStamp fingerprints the catalog files reg was built from.
	regStamp string
	guides   map[string]*guideState
	order    []string
}

type guideState struct {
	guide     *content.Guide
	dir       string
	sections  map[string]*content.AnnotationFile
	decisions *content.DecisionsFile
	report    *Report
	// index is each step's position in guide order.
	index map[string]int
}

// Open loads every guide of dir, its sidecars and report, and the catalogs
// of dir/reference.
func Open(dir string) (*Workspace, error) {
	w := &Workspace{Dir: dir, Today: func() string { return time.Now().Format(time.DateOnly) }, guides: map[string]*guideState{}}
	if err := w.reloadRegistry(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, content.GuidesSubdir, "*", content.GuideFileName))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("annotate: no guides under %s", dir)
	}
	sort.Strings(files)
	for _, f := range files {
		g, err := content.LoadGuideFile(f)
		if err != nil {
			return nil, err
		}
		gs := &guideState{guide: g, dir: filepath.Dir(f), sections: map[string]*content.AnnotationFile{}, index: map[string]int{}}
		for i, st := range g.Steps() {
			if _, dup := gs.index[st.ID]; !dup {
				gs.index[st.ID] = i
			}
		}
		if err := gs.readSidecars(); err != nil {
			return nil, err
		}
		w.guides[g.ID] = gs
		w.order = append(w.order, g.ID)
	}
	return w, nil
}

// Registry returns the registry the workspace resolves names with.
func (w *Workspace) Registry() *entities.Registry { return w.reg }

func (w *Workspace) reloadRegistry() error {
	stamp, err := w.referenceStamp()
	if err != nil {
		return err
	}
	reg, err := entities.Load(os.DirFS(filepath.Join(w.Dir, ReferenceSubdir)))
	if err != nil {
		return fmt.Errorf("annotate: %w", err)
	}
	w.reg, w.regStamp = reg, stamp
	return nil
}

// SyncRegistry reloads the registry when a catalog file changed on disk since
// it was built (a regenerated items.json, an edited locations.json), so a
// running server never resolves against stale catalogs.
func (w *Workspace) SyncRegistry() error {
	stamp, err := w.referenceStamp()
	if err != nil || stamp == w.regStamp {
		return err
	}
	return w.reloadRegistry()
}

// referenceStamp is the name, size and modification time of every catalog
// file under dir/reference.
func (w *Workspace) referenceStamp() (string, error) {
	files, err := filepath.Glob(filepath.Join(w.Dir, ReferenceSubdir, "*.json"))
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	var b bytes.Buffer
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s %d %d\n", filepath.Base(f), fi.Size(), fi.ModTime().UnixNano())
	}
	return b.String(), nil
}

func (w *Workspace) guide(id string) (*guideState, error) {
	gs, ok := w.guides[id]
	if !ok {
		return nil, fmt.Errorf("unknown guide %q (known: %v)", id, w.order)
	}
	return gs, nil
}

func (w *Workspace) section(guideID, sectionID string) (*guideState, *content.Section, error) {
	gs, err := w.guide(guideID)
	if err != nil {
		return nil, nil, err
	}
	sec, ok := gs.guide.Section(sectionID)
	if !ok {
		ids := make([]string, len(gs.guide.Sections))
		for i, s := range gs.guide.Sections {
			ids[i] = s.ID
		}
		return nil, nil, fmt.Errorf("unknown section %q in %s (known: %v)", sectionID, guideID, ids)
	}
	return gs, sec, nil
}

// readSidecars reads annotations/*.json and the report. Files are checked
// against the schema only; whether they still match the steps is what
// status and next_batch report.
func (gs *guideState) readSidecars() error {
	files, globErr := filepath.Glob(filepath.Join(gs.dir, content.AnnotationsSubdir, "*.json"))
	if globErr != nil {
		return globErr
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := content.ValidateAnnotationJSON(f, data); err != nil {
			return err
		}
		if filepath.Base(f) == content.DecisionsFileName {
			var d content.DecisionsFile
			if err := json.Unmarshal(data, &d); err != nil {
				return fmt.Errorf("annotate: %s: %w", f, err)
			}
			gs.decisions = &d
			continue
		}
		var a content.AnnotationFile
		if err := json.Unmarshal(data, &a); err != nil {
			return fmt.Errorf("annotate: %s: %w", f, err)
		}
		gs.sections[a.SectionID] = &a
	}
	gs.report = &Report{SchemaVersion: 1, Kind: ReportKind, GuideID: gs.guide.ID, Steps: map[string]*StepLog{}}
	data, err := os.ReadFile(filepath.Join(gs.dir, ReportFileName))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, gs.report); err != nil {
			return fmt.Errorf("annotate: %s: %w", ReportFileName, err)
		}
		if gs.report.Steps == nil {
			gs.report.Steps = map[string]*StepLog{}
		}
	}
	return nil
}

// annotation returns the sidecar entry of a step, if any.
func (gs *guideState) annotation(st *content.Step) *content.StepAnnotation {
	a := gs.sections[st.SectionID]
	if a == nil {
		return nil
	}
	for i := range a.Steps {
		if a.Steps[i].StepID == st.ID {
			return &a.Steps[i]
		}
	}
	return nil
}

// fresh reports whether a step has an annotation made from its current text.
func (gs *guideState) fresh(st *content.Step) bool {
	sa := gs.annotation(st)
	return sa != nil && sa.TextHash == content.StepTextHash(st)
}

// writeJSON writes v as indented JSON (no HTML escaping, so objective keys
// keep their ">=") through a temporary file, so a crash never leaves half a
// sidecar behind.
func writeJSON(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// encodeJSON is writeJSON's encoding without the file, for schema checks.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
