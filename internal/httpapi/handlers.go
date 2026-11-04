package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

// DB status values reported by /api/health.
const (
	DBNotConfigured = "not_configured"
	DBOK            = "ok"
	DBError         = "error"
)

type healthResponse struct {
	Status  string        `json:"status"`
	Version string        `json:"version"`
	Content contentCounts `json:"content"`
	DB      string        `json:"db"`
}

type contentCounts struct {
	Guides int `json:"guides"`
	Steps  int `json:"steps"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status:  "ok",
		Version: s.opts.Version,
		Content: contentCounts{Guides: len(s.opts.Content.Guides), Steps: s.opts.Content.StepCount()},
		DB:      DBNotConfigured,
	}
	if s.opts.DB != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.opts.DB.Ping(ctx); err != nil {
			s.log.Warn("health: db ping failed", "err", err)
			resp.DB = DBError
		} else {
			resp.DB = DBOK
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.opts.Version})
}

type sectionSummary struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Track bool   `json:"track"`
	Steps int    `json:"steps"`
	Guide string `json:"guide"`
}

func summarize(g *content.Guide, sec *content.Section) sectionSummary {
	kind := sec.Kind
	if kind == "" {
		kind = content.KindMain
	}
	return sectionSummary{ID: sec.ID, Slug: sec.Slug, Title: sec.Title, Kind: kind, Track: sec.IsTrack(), Steps: len(sec.Steps), Guide: g.ID}
}

type guideOut struct {
	ID       string           `json:"id"`
	Slug     string           `json:"slug,omitempty"`
	Title    string           `json:"title"`
	Version  string           `json:"version,omitempty"`
	Source   *content.Source  `json:"source,omitempty"`
	Steps    int              `json:"steps"`
	Segments int              `json:"segments"`
	Sections []sectionSummary `json:"sections"`
}

func guideOutput(g *content.Guide) guideOut {
	item := guideOut{ID: g.ID, Slug: g.Slug, Title: g.Title, Version: g.Version, Source: g.Source, Steps: g.StepCount(), Segments: len(g.Segments())}
	for _, sec := range g.Sections {
		item.Sections = append(item.Sections, summarize(g, sec))
	}
	return item
}

func (s *Server) handleGuides(w http.ResponseWriter, _ *http.Request) {
	out := make([]guideOut, 0, len(s.opts.Content.Guides))
	for _, g := range s.opts.Content.Guides {
		out = append(out, guideOutput(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"guides": out})
}

func (s *Server) handleGuide(w http.ResponseWriter, r *http.Request) {
	g, ok := s.opts.Content.Guide(r.PathValue("guide"))
	if !ok {
		writeError(w, http.StatusNotFound, "guide not found")
		return
	}
	writeJSON(w, http.StatusOK, guideOutput(g))
}

type sectionOut struct {
	*content.Section
	Guide string            `json:"guide"`
	Steps []stepOut         `json:"-"`
	Names map[string]string `json:"-"`
}

// MarshalJSON keeps the section endpoint's established flattened `steps`
// view even when the canonical content model stores steps under segments.
// content.Section has its own marshaler, so relying on anonymous embedding
// would otherwise also drop the outer Guide field.
func (s sectionOut) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(s.Section)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	out["guide"] = s.Guide
	out["steps"] = s.Steps
	out["names"] = s.Names
	return json.Marshal(out)
}

func (s *Server) handleSection(w http.ResponseWriter, r *http.Request) {
	s.writeSection(w, r.PathValue("guide"), r.PathValue("section"))
}

func (s *Server) writeSection(w http.ResponseWriter, guideID, sectionID string) {
	sec, ok := s.opts.Content.SectionInGuide(guideID, sectionID)
	if !ok {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}
	out := sectionOut{Section: sec, Guide: guideID, Steps: make([]stepOut, 0, len(sec.Steps)), Names: map[string]string{}}
	for _, st := range sec.Steps {
		out.Steps = append(out.Steps, s.outStep(st))
	}
	sectionNames(sec, s.opts.Catalog, out.Names)
	writeJSON(w, http.StatusOK, out)
}

// stepOut is a step plus its position for API consumers: the author's
// reference, the position in the main route, the display names of the refs it
// uses and where the other routes complete the same entities.
type stepOut struct {
	*content.Step
	Section  string                  `json:"section"`
	Guide    string                  `json:"guide"`
	Manual   bool                    `json:"manual"`
	Ref      string                  `json:"ref"`
	Position int                     `json:"position,omitempty"`
	Total    int                     `json:"total"`
	Names    map[string]string       `json:"names"`
	Also     map[string][]occurrence `json:"also,omitempty"`
}

func (s *Server) outStep(st *content.Step) stepOut {
	out := stepOut{
		Step: st, Section: st.SectionID, Guide: st.GuideID, Manual: st.IsManual(),
		Ref: authorRef(st), Position: s.index.position[guideStep{st.GuideID, st.ID}], Total: s.index.total[st.GuideID],
		Names: map[string]string{}, Also: s.index.also(st),
	}
	stepNames(st, s.opts.Catalog, out.Names)
	return out
}

func (s *Server) handleStep(w http.ResponseWriter, r *http.Request) {
	s.writeStep(w, r.PathValue("guide"), r.PathValue("id"))
}

func (s *Server) writeStep(w http.ResponseWriter, guideID, stepID string) {
	st, ok := s.opts.Content.StepInGuide(guideID, stepID)
	if !ok {
		writeError(w, http.StatusNotFound, "step not found")
		return
	}
	writeJSON(w, http.StatusOK, s.outStep(st))
}

type nowResponse struct {
	Guide    string    `json:"guide"`
	Current  *stepOut  `json:"current"`
	Next     []stepOut `json:"next"`
	Progress progress  `json:"progress"`
}

type progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// handleNow is a stub until progress exists: the first step and five following
// steps from one explicitly selected guide. Routes are never concatenated.
func (s *Server) handleNow(w http.ResponseWriter, r *http.Request) {
	s.writeNow(w, r.PathValue("guide"))
}

func (s *Server) writeNow(w http.ResponseWriter, guideID string) {
	const lookahead = 5
	g, ok := s.opts.Content.Guide(guideID)
	if !ok {
		writeError(w, http.StatusNotFound, "guide not found")
		return
	}
	var main []*content.Step
	for _, sec := range g.Sections {
		if sec.IsTrack() {
			continue
		}
		main = append(main, sec.Steps...)
	}
	resp := nowResponse{Guide: guideID, Next: []stepOut{}, Progress: progress{Done: 0, Total: len(main)}}
	if len(main) > 0 {
		cur := s.outStep(main[0])
		resp.Current = &cur
		for _, st := range main[1:min(len(main), 1+lookahead)] {
			resp.Next = append(resp.Next, s.outStep(st))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) defaultGuide() (string, bool) {
	if s.opts.DefaultGuideID != "" {
		_, ok := s.opts.Content.Guide(s.opts.DefaultGuideID)
		return s.opts.DefaultGuideID, ok
	}
	if _, ok := s.opts.Content.Guide("oziris-main"); ok {
		return "oziris-main", true
	}
	if len(s.opts.Content.Guides) == 1 {
		return s.opts.Content.Guides[0].ID, true
	}
	return "", false
}

func (s *Server) writeNoDefaultGuide(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "legacy route has no default guide")
}

func (s *Server) handleLegacyGuides(w http.ResponseWriter, r *http.Request) {
	s.handleGuides(w, r)
}

func (s *Server) handleLegacySection(w http.ResponseWriter, r *http.Request) {
	guideID, ok := s.defaultGuide()
	if !ok {
		s.writeNoDefaultGuide(w)
		return
	}
	s.writeSection(w, guideID, r.PathValue("section"))
}

func (s *Server) handleLegacyStep(w http.ResponseWriter, r *http.Request) {
	guideID, ok := s.defaultGuide()
	if !ok {
		s.writeNoDefaultGuide(w)
		return
	}
	s.writeStep(w, guideID, r.PathValue("id"))
}

func (s *Server) handleLegacyNow(w http.ResponseWriter, _ *http.Request) {
	guideID, ok := s.defaultGuide()
	if !ok {
		s.writeNoDefaultGuide(w)
		return
	}
	s.writeNow(w, guideID)
}
