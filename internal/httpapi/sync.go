package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/store/syncrun"
)

// SyncService runs and lists the polling syncs of an account (hiscores).
type SyncService interface {
	Run(ctx context.Context, accountID, trigger string) (syncrun.RunView, error)
	Runs(ctx context.Context, accountID string, limit int) ([]syncrun.RunView, error)
}

type syncRunsResponse struct {
	Account string            `json:"account"`
	Runs    []syncrun.RunView `json:"runs"`
}

type tooSoonResponse struct {
	Error   string          `json:"error"`
	Status  int             `json:"status"`
	RetryAt time.Time       `json:"retry_at"`
	Last    syncrun.RunView `json:"last"`
}

// handleSync runs the hiscores sync now (POST /api/sync): debounced and never
// during a backoff, which answer 429 with retry_at.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	run, err := s.opts.Sync.Run(r.Context(), account, syncrun.TriggerManual)
	if err != nil {
		s.writeSyncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleSyncRuns(w http.ResponseWriter, r *http.Request) {
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
	runs, err := s.opts.Sync.Runs(r.Context(), account, limit)
	if err != nil {
		s.writeSyncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, syncRunsResponse{Account: account, Runs: runs})
}

func (s *Server) writeSyncError(w http.ResponseWriter, err error) {
	var tooSoon *syncrun.TooSoonError
	switch {
	case errors.As(err, &tooSoon):
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(tooSoon.RetryAt).Seconds()))))
		writeJSON(w, http.StatusTooManyRequests, tooSoonResponse{
			Error: "sync too soon", Status: http.StatusTooManyRequests, RetryAt: tooSoon.RetryAt, Last: tooSoon.Last,
		})
	case errors.Is(err, syncrun.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, syncrun.ErrNoName):
		writeError(w, http.StatusConflict, "account has no RuneScape name or is inactive")
	default:
		s.log.Error("sync", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sync")
	}
}
