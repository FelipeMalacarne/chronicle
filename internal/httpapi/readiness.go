package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/planning"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
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
	h, marks, ok := s.holdings(w, r)
	if !ok {
		return
	}
	current, next := planning.Legs(g, stepID)
	resp := readinessResponse{Guide: g.ID, Step: stepID, Current: s.evaluate(g, current, h, marks), Next: s.evaluate(g, next, h, marks)}
	if resp.Current != nil && resp.Current.LoadoutStatus == content.LoadoutUnknown {
		resp.Ahead = planning.LookAhead(current, stepID, h, s.opts.Catalog, marks.For(g.ID, current.ID))
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
	h, marks, ok := s.holdings(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.evaluate(g, seg, h, marks))
}

func (s *Server) evaluate(g *content.Guide, seg *content.Segment, h rules.ItemHoldings, marks planning.OwnerMarks) *planning.Segment {
	if seg == nil {
		return nil
	}
	out := planning.EvaluateSegment(g, seg, h, s.opts.Catalog, marks.For(g.ID, seg.ID))
	return &out
}

// holdings loads what a readiness answer reads: the account's item holdings
// and the owner's marks on the legs' items.
func (s *Server) holdings(w http.ResponseWriter, r *http.Request) (rules.ItemHoldings, planning.OwnerMarks, bool) {
	accountID, ok := s.accountParam(w, r)
	if !ok {
		return nil, nil, false
	}
	h, err := s.opts.Holdings.ItemHoldings(r.Context(), accountID)
	switch {
	case errors.Is(err, storeingest.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
		return nil, nil, false
	case err != nil:
		s.log.Error("readiness: load item holdings", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load item holdings")
		return nil, nil, false
	}
	marks, ok := s.loadoutMarks(r.Context(), w, accountID)
	return h, marks, ok
}

// loadoutMarks reads the owner's marks on the legs' items; without a progress
// store there are none.
func (s *Server) loadoutMarks(ctx context.Context, w http.ResponseWriter, accountID string) (planning.OwnerMarks, bool) {
	if s.opts.Progress == nil {
		return nil, true
	}
	marks, err := s.opts.Progress.LoadoutMarks(ctx, accountID)
	if err != nil {
		s.log.Error("readiness: load loadout marks", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load loadout marks")
		return nil, false
	}
	return marks, true
}
