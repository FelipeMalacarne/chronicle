package annotate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

var decisionID = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)

// RecordDecision stores an answer of the owner that holds as a general rule
// in annotations/decisions.json, dated today. A decision with the same id is
// replaced (the owner changed their mind); annotations that cite it keep
// citing it.
func (w *Workspace) RecordDecision(guideID string, d content.Decision) (*content.Decision, error) {
	gs, err := w.guide(guideID)
	if err != nil {
		return nil, err
	}
	if !decisionID.MatchString(d.ID) {
		return nil, fmt.Errorf("decision id %q must be a lower-case slug (axe-means-any-axe)", d.ID)
	}
	if strings.TrimSpace(d.Question) == "" || strings.TrimSpace(d.Answer) == "" || strings.TrimSpace(d.Scope) == "" {
		return nil, fmt.Errorf("question, answer and scope are required")
	}
	for _, id := range d.StepIDs {
		if _, ok := gs.guide.Step(id); !ok {
			return nil, fmt.Errorf("unknown step %q", id)
		}
	}
	d.DecidedAt = w.Today()
	next := content.DecisionsFile{SchemaVersion: 1, Kind: content.AnnotationKindDecisions, GuideID: gs.guide.ID}
	if gs.decisions != nil {
		for _, x := range gs.decisions.Decisions {
			if x.ID != d.ID {
				next.Decisions = append(next.Decisions, x)
			}
		}
	}
	next.Decisions = append(next.Decisions, d)
	path := filepath.Join(gs.dir, content.AnnotationsSubdir, content.DecisionsFileName)
	data, err := encodeJSON(next)
	if err != nil {
		return nil, err
	}
	if err := content.ValidateAnnotationJSON(path, data); err != nil {
		return nil, err
	}
	if err := writeJSON(path, next); err != nil {
		return nil, err
	}
	gs.decisions = &next
	return &d, nil
}

// Decisions returns the decisions recorded for a guide.
func (w *Workspace) Decisions(guideID string) ([]content.Decision, error) {
	gs, err := w.guide(guideID)
	if err != nil {
		return nil, err
	}
	if gs.decisions == nil {
		return []content.Decision{}, nil
	}
	return gs.decisions.Decisions, nil
}

// EntityProposal is a new unlock or location the owner confirmed during
// extraction. ID defaults to the slug of Name.
type EntityProposal struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	WikiKey     string   `json:"wiki_key,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Note        string   `json:"note,omitempty"`
	ConfirmedBy string   `json:"confirmed_by"`
}

// ProposeEntity adds an owner-confirmed unlock or location to
// content/reference and reloads the registry, so the next submit resolves
// it. A name or alias that already resolves to an entry of the kind is
// refused: the agent should use that entry.
func (w *Workspace) ProposeEntity(p EntityProposal) (*EntityResult, error) {
	if p.ConfirmedBy != content.ConfirmedByOwner {
		return nil, fmt.Errorf("catalog entries need the owner's confirmation: ask, then send confirmed_by %q", content.ConfirmedByOwner)
	}
	kind := entities.Kind(p.Kind)
	if kind != entities.KindUnlock && kind != entities.KindLocation {
		return nil, fmt.Errorf("only unlock and location entries can be proposed, not %q", p.Kind)
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if p.ID == "" {
		p.ID = entities.Slug(p.Name)
	}
	ref, err := entities.NewRef(kind, p.ID)
	if err != nil {
		return nil, err
	}
	if w.reg.Known(ref) {
		return nil, fmt.Errorf("%s already exists", ref)
	}
	for _, name := range append([]string{p.Name}, p.Aliases...) {
		if res, _ := resolveName(w.reg, p.Kind, name, ""); res.Status != entities.Unresolved {
			return nil, fmt.Errorf("%q already names %s", name, refList(w.reg, append([]entities.Ref{res.Ref}, res.Candidates...)))
		}
	}
	var wiki entities.WikiKey
	if p.WikiKey != "" {
		wiki = entities.CanonicalWikiKey(p.WikiKey)
	}
	file := entities.UnlocksFile
	if kind == entities.KindLocation {
		file = entities.LocationsFile
	}
	path := filepath.Join(w.Dir, ReferenceSubdir, file)
	old, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var next any
	if kind == entities.KindUnlock {
		var c entities.UnlockCatalog
		if err := json.Unmarshal(old, &c); err != nil {
			return nil, fmt.Errorf("annotate: %s: %w", file, err)
		}
		c.Unlocks = append(c.Unlocks, entities.UnlockEntry{ID: p.ID, Name: p.Name, WikiKey: wiki, Aliases: p.Aliases, Note: p.Note})
		sort.Slice(c.Unlocks, func(i, j int) bool { return c.Unlocks[i].ID < c.Unlocks[j].ID })
		next = c
	} else {
		var c entities.LocationCatalog
		if err := json.Unmarshal(old, &c); err != nil {
			return nil, fmt.Errorf("annotate: %s: %w", file, err)
		}
		c.Locations = append(c.Locations, entities.LocationEntry{ID: p.ID, Name: p.Name, WikiKey: wiki, Aliases: p.Aliases, Note: p.Note})
		sort.Slice(c.Locations, func(i, j int) bool { return c.Locations[i].ID < c.Locations[j].ID })
		next = c
	}
	if err := writeJSON(path, next); err != nil {
		return nil, err
	}
	if err := w.reloadRegistry(); err != nil {
		_ = os.WriteFile(path, old, 0o644)
		_ = w.reloadRegistry()
		return nil, err
	}
	res := w.ResolveEntity(p.Kind, p.Name, "")
	return &res, nil
}
