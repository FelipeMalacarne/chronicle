package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
	storerecording "github.com/FelipeMalacarne/chronicle/internal/store/recording"
)

// RecordingStore reads and deletes an account's demonstration recording
// (internal/store/recording; docs/10 section 11). The ingest is its only
// writer.
type RecordingStore interface {
	Events(ctx context.Context, account string, since, until time.Time, limit int) ([]recording.Event, error)
	Delete(ctx context.Context, account, recordingID string) (int64, error)
}

const (
	// recordingDefaultSince is the window without ?since=.
	recordingDefaultSince = 24 * time.Hour
	recordingDefaultLimit = 1000
	recordingMaxLimit     = 5000
)

// recordingRoutes registers the owner's routes of the recording: addressed
// by ?account=, behind authed and accountParam (CLAUDE.md rule 10).
func (s *Server) recordingRoutes() {
	if s.opts.Recording == nil || !s.accountRoutes() {
		return
	}
	s.mux.Handle("GET /api/account/recording", s.authed(s.handleRecording))
	s.mux.Handle("DELETE /api/account/recording", s.authed(s.handleDeleteRecording))
}

// accountRecording is GET /api/account/recording: the events recorded from
// since to until, in the order they happened. Truncated says the page ended
// at limit: ask again from the last event's time.
type accountRecording struct {
	Account   string            `json:"account"`
	Since     time.Time         `json:"since"`
	Until     time.Time         `json:"until"`
	Events    []recording.Event `json:"events"`
	Truncated bool              `json:"truncated"`
}

// recordingQuery reads ?since= and ?until= (RFC 3339; default the last 24
// hours up to now) and ?limit= (1 to recordingMaxLimit). msg is the 400
// answer.
func recordingQuery(r *http.Request, now time.Time) (since, until time.Time, limit int, msg string) {
	since, until, limit = now.Add(-recordingDefaultSince), now.Add(time.Second), recordingDefaultLimit
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return since, until, limit, "since must be an RFC 3339 time"
		}
		since = t.UTC()
	}
	if raw := strings.TrimSpace(q.Get("until")); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return since, until, limit, "until must be an RFC 3339 time"
		}
		until = t.UTC()
	}
	if !until.After(since) {
		return since, until, limit, "until must be after since"
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > recordingMaxLimit {
			return since, until, limit, "limit must be between 1 and " + strconv.Itoa(recordingMaxLimit)
		}
		limit = n
	}
	return since, until, limit, ""
}

func (s *Server) handleRecording(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	since, until, limit, msg := recordingQuery(r, time.Now().UTC())
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	events, err := s.opts.Recording.Events(r.Context(), account, since, until, limit+1)
	if errors.Is(err, storerecording.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		s.log.Error("recording", "account", account, "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the recording")
		return
	}
	out := accountRecording{Account: account, Since: since, Until: until, Events: events}
	if len(events) > limit {
		out.Events, out.Truncated = events[:limit], true
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteRecording is DELETE /api/account/recording: the owner's
// request to drop the recording, every event or one recording's
// (?recording=).
func (s *Server) handleDeleteRecording(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("recording"))
	if id != "" && !recording.ValidRecordingID(id) {
		writeError(w, http.StatusBadRequest, "recording must be a recording id")
		return
	}
	n, err := s.opts.Recording.Delete(r.Context(), account, id)
	if errors.Is(err, storerecording.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		s.log.Error("recording: delete", "account", account, "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete the recording")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": account, "recording": id, "deleted": n})
}
