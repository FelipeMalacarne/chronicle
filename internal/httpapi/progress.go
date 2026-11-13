package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// ProgressService reads and writes account progress (docs/09 section 5).
type ProgressService interface {
	Board(ctx context.Context, accountID, guideID string) (storeprogress.BoardView, error)
	ActiveGuide(ctx context.Context, accountID string) (string, error)
	Mark(ctx context.Context, accountID, guideID, stepID string, action coreprogress.MarkAction, note *string) (storeprogress.BoardView, error)
	ConfirmImplied(ctx context.Context, accountID, guideID string) (storeprogress.BoardView, error)
	PinCursor(ctx context.Context, accountID, guideID, stepID string) (storeprogress.BoardView, error)
	MarkBefore(ctx context.Context, accountID, guideID, stepID string) (storeprogress.BoardView, error)
	RestoreMarks(ctx context.Context, accountID, guideID string, restores []storeprogress.MarkRestore) (storeprogress.BoardView, error)
	Objectives(ctx context.Context, accountID string) ([]storeprogress.ObjectiveView, error)
	Events(ctx context.Context, accountID, guideID string, limit int) (storeprogress.EventsView, error)
	SetActiveGuide(ctx context.Context, accountID, guideID string) error
	Sources(ctx context.Context, accountID string) ([]storeprogress.SourceView, error)
	Goals(ctx context.Context, accountID string) ([]storeprogress.GoalView, error)
	AddGoal(ctx context.Context, accountID, key, label string) (storeprogress.GoalView, error)
	RemoveGoal(ctx context.Context, accountID, goalID string) error
	GoalPlan(ctx context.Context, accountID, goalID string) (storeprogress.GoalPlanView, error)
	FleetGoals(ctx context.Context, accountIDs, keys []string) ([]coreprogress.FleetGoal, error)
	Graph(ctx context.Context, accountID string, focus entities.ObjectiveKey, depth int) (coreprogress.Graph, error)
	AccountState(ctx context.Context, accountID string) (storeprogress.AccountState, error)
	Timers(ctx context.Context, accountID string) (storeprogress.TimersView, error)
	Nearby(ctx context.Context, accountID, guideID string, limit int) (storeprogress.NearbyView, error)
	Check(ctx context.Context, accountID string) (storeprogress.CheckReport, error)
}

// handleAccountState is the account screen (V6Conta1440): skills, quests and
// diaries with source and age, the containers with their freshness.
func (s *Server) handleAccountState(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	st, err := s.opts.Progress.AccountState(r.Context(), account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleTimers is the Agora's Recorrentes (decision 31 of doc 01): the
// account's Time Tracking clocks as the plugin last sent them.
func (s *Server) handleTimers(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	v, err := s.opts.Progress.Timers(r.Context(), account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// routeSummary is one route of the Rotas list: its cursor, counts per section
// and how much of it the account's facts answer.
type routeSummary struct {
	Guide    string                `json:"guide"`
	Title    string                `json:"title"`
	Version  string                `json:"version,omitempty"`
	Source   *content.Source       `json:"source,omitempty"`
	Steps    int                   `json:"steps"`
	Segments int                   `json:"segments"`
	Active   bool                  `json:"active"`
	Cursor   *storeprogress.Cursor `json:"cursor"`
	// CursorRef and CursorSection locate the cursor without the guide's
	// content: the author's reference and the section id.
	CursorRef     string                       `json:"cursor_ref,omitempty"`
	CursorSection string                       `json:"cursor_section,omitempty"`
	Counts        coreprogress.Counts          `json:"counts"`
	Coverage      coreprogress.Coverage        `json:"coverage"`
	Sections      []coreprogress.SectionCounts `json:"sections"`
}

type progressResponse struct {
	Account string                     `json:"account"`
	Active  string                     `json:"active_guide"`
	Routes  []routeSummary             `json:"routes"`
	Sources []storeprogress.SourceView `json:"sources"`
}

type activeGuideRequest struct {
	GuideID string `json:"guide_id"`
}

type markRequest struct {
	Kind coreprogress.MarkAction `json:"kind"`
	// Note: absent keeps the current note, "" clears it.
	Note *string `json:"note,omitempty"`
	// Revokes: for an undo, the command_id of the write it takes back.
	Revokes string `json:"revokes,omitempty"`
}

// batchMarkRequest is either a batch (every pending or implied step before
// before_step_id becomes manual_done) or its undo (the board's undo list).
type batchMarkRequest struct {
	BeforeStepID string                      `json:"before_step_id,omitempty"`
	Restore      []storeprogress.MarkRestore `json:"restore,omitempty"`
	// Revokes: with restore, the command_id of the batch it takes back.
	Revokes string `json:"revokes,omitempty"`
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
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	board, err := s.opts.Progress.Board(r.Context(), account, r.PathValue("guide"))
	s.writeProgress(w, board, err)
}

// handleNearby is the Agora's "Aqui perto": the open steps of the route near
// the position the plugin last sent (item 16 of docs/04 section 9).
func (s *Server) handleNearby(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	v, err := s.opts.Progress.Nearby(r.Context(), account, r.PathValue("guide"), 5)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleMark(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var req markRequest
	if !decodeBody(w, r, &req) {
		return
	}
	ctx := storeprogress.WithCommand(r.Context(), storeprogress.CommandMeta{Via: storeprogress.ViaWeb, Revokes: req.Revokes})
	board, err := s.opts.Progress.Mark(ctx, account, r.PathValue("guide"), r.PathValue("id"), req.Kind, req.Note)
	s.writeProgress(w, board, err)
}

func (s *Server) handleBatchMarks(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var req batchMarkRequest
	if !decodeBody(w, r, &req) {
		return
	}
	ctx := storeprogress.WithCommand(r.Context(), storeprogress.CommandMeta{Via: storeprogress.ViaWeb, Revokes: req.Revokes})
	switch {
	case req.BeforeStepID != "" && len(req.Restore) == 0 && req.Revokes == "":
		board, err := s.opts.Progress.MarkBefore(ctx, account, r.PathValue("guide"), req.BeforeStepID)
		s.writeProgress(w, board, err)
	case req.BeforeStepID == "" && len(req.Restore) > 0 && len(req.Restore) <= 2000:
		board, err := s.opts.Progress.RestoreMarks(ctx, account, r.PathValue("guide"), req.Restore)
		s.writeProgress(w, board, err)
	default:
		writeError(w, http.StatusBadRequest, "send either before_step_id or restore (1 to 2000 steps)")
	}
}

func (s *Server) handleConfirmImplied(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	board, err := s.opts.Progress.ConfirmImplied(r.Context(), account, r.PathValue("guide"))
	s.writeProgress(w, board, err)
}

func (s *Server) handleCursor(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
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
	account, ok := s.accountParam(w, r)
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
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	s.writeProgressSummary(w, r, account)
}

// handleProgressCheck is GET /api/progress/check: every projection of the
// account recomputed from its sources, read only (`chronicle progress check`,
// docs/10 section 6).
func (s *Server) handleProgressCheck(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	rep, err := s.opts.Progress.Check(r.Context(), account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) writeProgressSummary(w http.ResponseWriter, r *http.Request, account string) {
	sources, err := s.opts.Progress.Sources(r.Context(), account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	out := progressResponse{Account: account, Routes: []routeSummary{}, Sources: sources}
	for _, g := range s.opts.Content.Guides {
		board, err := s.opts.Progress.Board(r.Context(), account, g.ID)
		if err != nil {
			s.writeProgressError(w, err)
			return
		}
		if board.Active {
			out.Active = g.ID
		}
		route := routeSummary{
			Guide: g.ID, Title: g.Title, Version: g.Version, Source: g.Source, Steps: g.StepCount(), Segments: len(g.Segments()),
			Active: board.Active, Cursor: board.Cursor, Counts: board.Counts, Coverage: board.Coverage, Sections: board.Sections,
		}
		if board.Cursor != nil {
			if st, ok := g.Step(board.Cursor.StepID); ok {
				route.CursorRef, route.CursorSection = authorRef(st), st.SectionID
			}
		}
		out.Routes = append(out.Routes, route)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleActiveGuide(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
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
	account, ok := s.accountParam(w, r)
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
	if status, ok := isGoalError(err); ok {
		writeError(w, status, err.Error())
		return
	}
	switch {
	case errors.Is(err, storeprogress.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, storeprogress.ErrGuideNotFound):
		writeError(w, http.StatusNotFound, "guide not found")
	case errors.Is(err, coreprogress.ErrStepNotFound):
		writeError(w, http.StatusNotFound, "step not found")
	case errors.Is(err, coreprogress.ErrInvalidAction):
		writeError(w, http.StatusBadRequest, "kind must be manual_done, skipped, pinned_open, confirm_implied or clear")
	case errors.Is(err, storeprogress.ErrInvalidRevokes):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, coreprogress.ErrNotImplied), errors.Is(err, coreprogress.ErrStepDone):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.log.Error("progress", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update progress")
	}
}
