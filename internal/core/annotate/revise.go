package annotate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// EntityRevision changes or removes an owner-confirmed unlock or location.
// An empty Name keeps the old one; WikiKey, Aliases and Note, when present,
// replace the old values ("" clears them). NewID renames the entry and
// rewrites its refs in every sidecar.
type EntityRevision struct {
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	NewID       string    `json:"new_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	WikiKey     *string   `json:"wiki_key,omitempty"`
	Aliases     *[]string `json:"aliases,omitempty"`
	Note        *string   `json:"note,omitempty"`
	Remove      bool      `json:"remove,omitempty"`
	ConfirmedBy string    `json:"confirmed_by"`
}

// RevisionResult is what revise_entity answers.
type RevisionResult struct {
	// Ref is the entry's ref after the revision; empty when it was removed.
	Ref     string `json:"ref,omitempty"`
	Removed bool   `json:"removed,omitempty"`
	// Rewritten lists the sidecar files whose refs were renamed.
	Rewritten []string `json:"rewritten,omitempty"`
}

// catalogEntry is an unlock or location entry as a revision edits it; proof
// is carried along for unlocks.
type catalogEntry struct {
	ID      string
	Name    string
	WikiKey entities.WikiKey
	Aliases []string
	Note    string
	proof   *entities.VarTest
}

// ReviseEntity renames, edits or removes an unlock or location entry of
// content/reference. A rename rewrites the entry's refs in every guide's
// sidecars; a removal is refused while any sidecar or rule still refers to
// the entry. Like propose_entity it needs the owner's confirmation.
func (w *Workspace) ReviseEntity(rv EntityRevision) (*RevisionResult, error) {
	if rv.ConfirmedBy != content.ConfirmedByOwner {
		return nil, fmt.Errorf("catalog changes need the owner's confirmation: ask, then send confirmed_by %q", content.ConfirmedByOwner)
	}
	kind := entities.Kind(rv.Kind)
	if kind != entities.KindUnlock && kind != entities.KindLocation {
		return nil, fmt.Errorf("only unlock and location entries can be revised, not %q", rv.Kind)
	}
	ref, err := entities.NewRef(kind, rv.ID)
	if err != nil {
		return nil, err
	}
	if !w.reg.Known(ref) {
		return nil, fmt.Errorf("%s does not exist", ref)
	}
	uses, ruleUses := w.refUses(ref)

	file := entities.UnlocksFile
	if kind == entities.KindLocation {
		file = entities.LocationsFile
	}
	path := filepath.Join(w.Dir, ReferenceSubdir, file)
	old, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if rv.Remove {
		if len(uses)+len(ruleUses) > 0 {
			return nil, fmt.Errorf("%s is still used by steps %s; revise those annotations first", ref, strings.Join(append(ruleUses, stepIDs(uses)...), ", "))
		}
		err = w.editCatalog(path, kind, func(es []catalogEntry) ([]catalogEntry, error) {
			return slices.DeleteFunc(es, func(e catalogEntry) bool { return e.ID == ref.ID }), nil
		})
		if err != nil {
			return nil, err
		}
		return &RevisionResult{Removed: true}, w.reloadOrRestore(path, old)
	}

	next := ref
	if rv.NewID != "" && rv.NewID != rv.ID {
		if next, err = entities.NewRef(kind, rv.NewID); err != nil {
			return nil, err
		}
		if w.reg.Known(next) {
			return nil, fmt.Errorf("%s already exists", next)
		}
		if len(ruleUses) > 0 {
			return nil, fmt.Errorf("%s is named by the rules of steps %s, which the importers generate; it cannot be renamed here", ref, strings.Join(ruleUses, ", "))
		}
	}
	err = w.editCatalog(path, kind, func(es []catalogEntry) ([]catalogEntry, error) {
		i := slices.IndexFunc(es, func(e catalogEntry) bool { return e.ID == ref.ID })
		if i < 0 {
			return nil, fmt.Errorf("%s is not in %s", ref, file)
		}
		e := &es[i]
		e.ID = next.ID
		if rv.Name != "" {
			e.Name = rv.Name
		}
		if rv.WikiKey != nil {
			e.WikiKey = ""
			if *rv.WikiKey != "" {
				e.WikiKey = entities.CanonicalWikiKey(*rv.WikiKey)
			}
		}
		if rv.Aliases != nil {
			e.Aliases = *rv.Aliases
		}
		if rv.Note != nil {
			e.Note = *rv.Note
		}
		for _, name := range append([]string{e.Name}, e.Aliases...) {
			res, _ := resolveName(w.reg, rv.Kind, name, "")
			for _, other := range append([]entities.Ref{res.Ref}, res.Candidates...) {
				if !other.IsZero() && other != ref {
					return nil, fmt.Errorf("%q already names %s", name, other)
				}
			}
		}
		return es, nil
	})
	if err != nil {
		return nil, err
	}
	err = w.reloadOrRestore(path, old)
	if err != nil {
		return nil, err
	}
	out := &RevisionResult{Ref: next.String()}
	if next != ref {
		if out.Rewritten, err = w.renameRefs(ref, next, uses); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// refUse is one sidecar step annotation that refers to an entry.
type refUse struct {
	gs      *guideState
	section string
	step    string
}

func stepIDs(us []refUse) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.gs.guide.ID + "/" + u.step
	}
	return out
}

// refUses finds the sidecar annotations and the rule objectives that refer
// to ref.
func (w *Workspace) refUses(ref entities.Ref) (uses []refUse, rules []string) {
	for _, id := range w.order {
		gs := w.guides[id]
		for _, st := range gs.guide.Steps() {
			objs, _ := content.RuleObjectives(st, w.reg)
			if slices.ContainsFunc(objs, func(o content.Objective) bool { return o.Key.Ref == ref }) {
				rules = append(rules, id+"/"+st.ID)
			}
		}
		secs := make([]string, 0, len(gs.sections))
		for sec := range gs.sections {
			secs = append(secs, sec)
		}
		sort.Strings(secs)
		for _, sec := range secs {
			for _, sa := range gs.sections[sec].Steps {
				if refersTo(sa, ref) {
					uses = append(uses, refUse{gs: gs, section: sec, step: sa.StepID})
				}
			}
		}
	}
	return uses, rules
}

func refersTo(sa content.StepAnnotation, ref entities.Ref) bool {
	if sa.Location != nil && sa.Location.Ref == ref {
		return true
	}
	return slices.ContainsFunc(sa.Objectives, func(o content.Objective) bool { return o.Key.Ref == ref })
}

// renameRefs rewrites ref to next in the sidecars that use it and refreshes
// the reports of the guides touched.
func (w *Workspace) renameRefs(ref, next entities.Ref, uses []refUse) ([]string, error) {
	touched := map[*guideState]map[string]bool{}
	for _, u := range uses {
		if touched[u.gs] == nil {
			touched[u.gs] = map[string]bool{}
		}
		touched[u.gs][u.section] = true
	}
	var files []string
	for _, id := range w.order {
		gs := w.guides[id]
		for sec := range touched[gs] {
			a := gs.sections[sec]
			for i := range a.Steps {
				sa := &a.Steps[i]
				if sa.Location != nil && sa.Location.Ref == ref {
					sa.Location.Ref = next
				}
				for j := range sa.Objectives {
					if sa.Objectives[j].Key.Ref == ref {
						sa.Objectives[j].Key.Ref = next
					}
				}
			}
			path := filepath.Join(gs.dir, content.AnnotationsSubdir, sec+".json")
			data, err := encodeJSON(a)
			if err != nil {
				return files, err
			}
			if err := content.ValidateAnnotationJSON(path, data); err != nil {
				return files, err
			}
			if err := writeJSON(path, a); err != nil {
				return files, err
			}
			files = append(files, path)
		}
		if touched[gs] != nil {
			if err := gs.writeReport(w.reg); err != nil {
				return files, err
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// editCatalog applies edit to the entries of the unlock or location catalog
// at path and writes it back sorted by id.
func (w *Workspace) editCatalog(path string, kind entities.Kind, edit func([]catalogEntry) ([]catalogEntry, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if kind == entities.KindUnlock {
		var c entities.UnlockCatalog
		if err = json.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("annotate: %s: %w", path, err)
		}
		es := make([]catalogEntry, len(c.Unlocks))
		for i, u := range c.Unlocks {
			es[i] = catalogEntry{ID: u.ID, Name: u.Name, WikiKey: u.WikiKey, Aliases: u.Aliases, Note: u.Note, proof: u.Proof}
		}
		var edited []catalogEntry
		if edited, err = edit(es); err != nil {
			return err
		}
		c.Unlocks = c.Unlocks[:0]
		for _, e := range edited {
			c.Unlocks = append(c.Unlocks, entities.UnlockEntry{ID: e.ID, Name: e.Name, WikiKey: e.WikiKey, Proof: e.proof, Aliases: e.Aliases, Note: e.Note})
		}
		sort.Slice(c.Unlocks, func(i, j int) bool { return c.Unlocks[i].ID < c.Unlocks[j].ID })
		return writeJSON(path, c)
	}
	var c entities.LocationCatalog
	if err = json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("annotate: %s: %w", path, err)
	}
	es := make([]catalogEntry, len(c.Locations))
	for i, l := range c.Locations {
		es[i] = catalogEntry{ID: l.ID, Name: l.Name, WikiKey: l.WikiKey, Aliases: l.Aliases, Note: l.Note}
	}
	edited, err := edit(es)
	if err != nil {
		return err
	}
	c.Locations = c.Locations[:0]
	for _, e := range edited {
		c.Locations = append(c.Locations, entities.LocationEntry{ID: e.ID, Name: e.Name, WikiKey: e.WikiKey, Aliases: e.Aliases, Note: e.Note})
	}
	sort.Slice(c.Locations, func(i, j int) bool { return c.Locations[i].ID < c.Locations[j].ID })
	return writeJSON(path, c)
}

// reloadOrRestore reloads the registry after a catalog write and puts the
// old file back when the new one does not load.
func (w *Workspace) reloadOrRestore(path string, old []byte) error {
	if err := w.reloadRegistry(); err != nil {
		_ = os.WriteFile(path, old, 0o644)
		_ = w.reloadRegistry()
		return err
	}
	return nil
}
