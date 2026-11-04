package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// ProgressService reads and writes account progress (docs/09 section 5).
type ProgressService interface {
	Board(ctx context.Context, accountID, guideID string) (storeprogress.BoardView, error)
	Mark(ctx context.Context, accountID, guideID, stepID string, action coreprogress.MarkAction, note *string) (storeprogress.BoardView, error)
	ConfirmImplied(ctx context.Context, accountID, guideID string) (storeprogress.BoardView, error)
	PinCursor(ctx context.Context, accountID, guideID, stepID string) (storeprogress.BoardView, error)
	Objectives(ctx context.Context, accountID string) ([]storeprogress.ObjectiveView, error)
	Events(ctx context.Context, accountID, guideID string, limit int) (storeprogress.EventsView, error)
	SetActiveGuide(ctx context.Context, accountID, guideID string) error
}

// routeSummary is one route of the Rotas list: its cursor, counts per section
// and how much of it the account's facts answer.
type routeSummary struct {
	Guide    string                       `json:"guide"`
	Title    string                       `json:"title"`
	Version  string                       `json:"version,omitempty"`
	Source   *content.Source              `json:"source,omitempty"`
	Steps    int                          `json:"steps"`
	Active   bool                         `json:"active"`
	Cursor   *storeprogress.Cursor        `json:"cursor"`
	Counts   coreprogress.Counts          `json:"counts"`
	Coverage coreprogress.Coverage        `json:"coverage"`
	Sections []coreprogress.SectionCounts `json:"sections"`
}

type progressResponse struct {
	Account string         `json:"account"`
	Active  string         `json:"active_guide"`
	Routes  []routeSummary `json:"routes"`
}

type activeGuideRequest struct {
	GuideID string `json:"guide_id"`
}

type markRequest struct {
	Kind coreprogress.MarkAction `json:"kind"`
	// Note: absent keeps the current note, "" clears it.
	Note *string `json:"note,omitempty"`
}

type cursorRequest struct {
	// StepID: a step pins the cursor there; null unpins it.
	StepID *string `json:"step_id"`
}

type objectivesResponse struct {
	Account    string                        `json:"account"`
	Objectives []storeprogress.ObjectiveView `json:"objectives"`
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	board, err := s.opts.Progress.Board(r.Context(), account, r.PathValue("guide"))
	s.writeProgress(w, board, err)
}

func (s *Server) handleMark(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	var req markRequest
	if !decodeBody(w, r, &req) {
		return
	}
	board, err := s.opts.Progress.Mark(r.Context(), account, r.PathValue("guide"), r.PathValue("id"), req.Kind, req.Note)
	s.writeProgress(w, board, err)
}

func (s *Server) handleConfirmImplied(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	board, err := s.opts.Progress.ConfirmImplied(r.Context(), account, r.PathValue("guide"))
	s.writeProgress(w, board, err)
}

func (s *Server) handleCursor(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	var req cursorRequest
	if !decodeBody(w, r, &req) {
		return
	}
	step := ""
	if req.StepID != nil {
		step = strings.TrimSpace(*req.StepID)
		if step == "" {
			writeError(w, http.StatusBadRequest, "step_id must be a step id or null")
			return
		}
	}
	board, err := s.opts.Progress.PinCursor(r.Context(), account, r.PathValue("guide"), step)
	s.writeProgress(w, board, err)
}

func (s *Server) handleObjectives(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	objectives, err := s.opts.Progress.Objectives(r.Context(), account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectivesResponse{Account: account, Objectives: objectives})
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	s.writeProgressSummary(w, r, account)
}

func (s *Server) writeProgressSummary(w http.ResponseWriter, r *http.Request, account string) {
	out := progressResponse{Account: account, Routes: []routeSummary{}}
	for _, g := range s.opts.Content.Guides {
		board, err := s.opts.Progress.Board(r.Context(), account, g.ID)
		if err != nil {
			s.writeProgressError(w, err)
			return
		}
		if board.Active {
			out.Active = g.ID
		}
		out.Routes = append(out.Routes, routeSummary{
			Guide: g.ID, Title: g.Title, Version: g.Version, Source: g.Source, Steps: g.StepCount(),
			Active: board.Active, Cursor: board.Cursor, Counts: board.Counts, Coverage: board.Coverage, Sections: board.Sections,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleActiveGuide(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	var req activeGuideRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.opts.Progress.SetActiveGuide(r.Context(), account, strings.TrimSpace(req.GuideID)); err != nil {
		s.writeProgressError(w, err)
		return
	}
	s.writeProgressSummary(w, r, account)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = n
	}
	events, err := s.opts.Progress.Events(r.Context(), account, r.PathValue("guide"), limit)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func accountParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	account := strings.TrimSpace(r.URL.Query().Get("account"))
	if account == "" {
		writeError(w, http.StatusBadRequest, "account query parameter is required")
		return "", false
	}
	return account, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	raw, err := readJSONBody(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if err := decodeStrictJSON(raw, into); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return false
	}
	return true
}

func (s *Server) writeProgress(w http.ResponseWriter, board storeprogress.BoardView, err error) {
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (s *Server) writeProgressError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storeprogress.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, storeprogress.ErrGuideNotFound):
		writeError(w, http.StatusNotFound, "guide not found")
	case errors.Is(err, coreprogress.ErrStepNotFound):
		writeError(w, http.StatusNotFound, "step not found")
	case errors.Is(err, coreprogress.ErrInvalidAction):
		writeError(w, http.StatusBadRequest, "kind must be manual_done, skipped, pinned_open, confirm_implied or clear")
	case errors.Is(err, coreprogress.ErrNotImplied), errors.Is(err, coreprogress.ErrStepDone):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.log.Error("progress", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update progress")
	}
}
