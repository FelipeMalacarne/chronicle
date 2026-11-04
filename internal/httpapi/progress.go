package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
