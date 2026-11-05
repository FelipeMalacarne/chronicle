package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/planning"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
)

// HoldingsSource returns the merged item containers of an account.
type HoldingsSource interface {
	ItemHoldings(ctx context.Context, accountID string) (rules.ItemHoldings, error)
}

// readinessResponse is the leg that contains a step and the next bank leg of
// the same route. Either may be null.
type readinessResponse struct {
	Guide   string            `json:"guide"`
	Step    string            `json:"step,omitempty"`
	Current *planning.Segment `json:"current"`
	Next    *planning.Segment `json:"next"`
	// Ahead previews what the rest of the current leg asks for, from step on,
	// only when the guide does not list that leg's setup.
	Ahead *planning.Ahead `json:"ahead,omitempty"`
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	g, ok := s.opts.Content.Guide(r.PathValue("guide"))
	if !ok {
		writeError(w, http.StatusNotFound, "guide not found")
		return
	}
	stepID := strings.TrimSpace(r.URL.Query().Get("step"))
	if stepID != "" {
		if _, found := g.Step(stepID); !found {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
	}
	h, ok := s.holdings(w, r)
	if !ok {
		return
	}
	current, next := planning.Legs(g, stepID)
	resp := readinessResponse{Guide: g.ID, Step: stepID, Current: s.evaluate(g, current, h), Next: s.evaluate(g, next, h)}
	if resp.Current != nil && resp.Current.LoadoutStatus == content.LoadoutUnknown {
		resp.Ahead = planning.LookAhead(current, stepID, h, s.opts.Catalog)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSegmentReadiness(w http.ResponseWriter, r *http.Request) {
	g, ok := s.opts.Content.Guide(r.PathValue("guide"))
	if !ok {
		writeError(w, http.StatusNotFound, "guide not found")
		return
	}
	seg, ok := g.Segment(r.PathValue("segment"))
	if !ok {
		writeError(w, http.StatusNotFound, "segment not found")
		return
	}
	h, ok := s.holdings(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.evaluate(g, seg, h))
}

func (s *Server) evaluate(g *content.Guide, seg *content.Segment, h rules.ItemHoldings) *planning.Segment {
	if seg == nil {
		return nil
	}
	out := planning.EvaluateSegment(g, seg, h, s.opts.Catalog)
	return &out
}

func (s *Server) holdings(w http.ResponseWriter, r *http.Request) (rules.ItemHoldings, bool) {
	accountID, ok := s.accountParam(w, r)
	if !ok {
		return nil, false
	}
	h, err := s.opts.Holdings.ItemHoldings(r.Context(), accountID)
	switch {
	case errors.Is(err, storeingest.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
		return nil, false
	case err != nil:
		s.log.Error("readiness: load item holdings", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load item holdings")
		return nil, false
	}
	return h, true
}
