package httpapi

import (
	"context"
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

func (s *Server) handleGuide(w http.ResponseWriter, _ *http.Request) {
	type guideOut struct {
		ID       string           `json:"id"`
		Slug     string           `json:"slug,omitempty"`
		Title    string           `json:"title"`
		Version  string           `json:"version,omitempty"`
		Source   *content.Source  `json:"source,omitempty"`
		Sections []sectionSummary `json:"sections"`
	}
	out := make([]guideOut, 0, len(s.opts.Content.Guides))
	for _, g := range s.opts.Content.Guides {
		item := guideOut{ID: g.ID, Slug: g.Slug, Title: g.Title, Version: g.Version, Source: g.Source}
		for _, sec := range g.Sections {
			item.Sections = append(item.Sections, summarize(g, sec))
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"guides": out})
}

type sectionOut struct {
	*content.Section
	Guide string `json:"guide"`
}

func (s *Server) handleSection(w http.ResponseWriter, r *http.Request) {
	g, sec, ok := s.opts.Content.Section(r.PathValue("section"))
	if !ok {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}
	writeJSON(w, http.StatusOK, sectionOut{Section: sec, Guide: g.ID})
}

// stepOut is a step plus its position for API consumers.
type stepOut struct {
	*content.Step
	Section string `json:"section"`
	Guide   string `json:"guide"`
	Manual  bool   `json:"manual"`
}

func outStep(st *content.Step) stepOut {
	return stepOut{Step: st, Section: st.SectionID, Guide: st.GuideID, Manual: st.IsManual()}
}

func (s *Server) handleStep(w http.ResponseWriter, r *http.Request) {
	st, ok := s.opts.Content.Step(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "step not found")
		return
	}
	writeJSON(w, http.StatusOK, outStep(st))
}

type nowResponse struct {
	Current  *stepOut  `json:"current"`
	Next     []stepOut `json:"next"`
	Progress progress  `json:"progress"`
}

type progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// handleNow is a stub until progress exists: the first step of the first
// non-track section, the 5 that follow it, and the main-guide step total.
func (s *Server) handleNow(w http.ResponseWriter, _ *http.Request) {
	const lookahead = 5
	var main []*content.Step
	for _, g := range s.opts.Content.Guides {
		for _, sec := range g.Sections {
			if sec.IsTrack() {
				continue
			}
			main = append(main, sec.Steps...)
		}
	}
	resp := nowResponse{Next: []stepOut{}, Progress: progress{Done: 0, Total: len(main)}}
	if len(main) > 0 {
		cur := outStep(main[0])
		resp.Current = &cur
		for _, st := range main[1:min(len(main), 1+lookahead)] {
			resp.Next = append(resp.Next, outStep(st))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
