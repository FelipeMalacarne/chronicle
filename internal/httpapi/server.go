// Package httpapi serves the JSON API under /api and the embedded SPA at /.
package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
)

// DBPinger is the subset of a connection pool the API needs for health checks.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// Options configures the handler.
type Options struct {
	// Version reported by /api/health and /api/version.
	Version string
	// Content is the loaded guide content (required).
	Content *content.Content
	// DefaultGuideID is used only by legacy unscoped routes. New callers must
	// always address a guide explicitly.
	DefaultGuideID string
	// DB is pinged by /api/health; nil means no database is configured.
	DB DBPinger
	// Ingest handles authenticated plugin manifests and pushes. nil disables
	// the ingest routes (for example when no database is configured).
	Ingest storeingest.Service
	// Holdings serves the item containers behind the readiness routes. nil
	// disables them.
	Holdings HoldingsSource
	// Progress serves the board, marks, cursor and objective verdicts. nil
	// disables those routes.
	Progress ProgressService
	// Sync runs the hiscores on demand and lists the runs. nil disables it.
	Sync SyncService
	// Accounts and Hiscores serve the setup (/setup, V4Setup). nil Accounts
	// disables it; nil Hiscores skips the mode detection.
	Accounts AccountService
	Hiscores ModeDetector
	// Catalog relates noted/unnoted item ids and knows stackability for the
	// readiness evaluator; nil compares ids exactly.
	Catalog *entities.Registry
	// DevMode exposes the one-time ingest-token bootstrap endpoint and the
	// account-addressed readiness routes. It must never be enabled by a
	// production authentication mode.
	DevMode bool
	// Static is the SPA filesystem rooted at the site (index.html at its root).
	// nil disables the SPA and / returns 404.
	Static fs.FS
	// Logger receives request logs; nil uses slog.Default().
	Logger *slog.Logger
}

// Server is the HTTP handler.
type Server struct {
	opts  Options
	log   *slog.Logger
	mux   *http.ServeMux
	index *contentIndex
}

// New builds the handler with all routes registered.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{opts: opts, log: opts.Logger, mux: http.NewServeMux(), index: buildContentIndex(opts.Content)}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler with request logging.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logRequests(s.log, s.mux).ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/guides", s.handleGuides)
	s.mux.HandleFunc("GET /api/guides/{guide}", s.handleGuide)
	s.mux.HandleFunc("GET /api/guides/{guide}/sections/{section}", s.handleSection)
	s.mux.HandleFunc("GET /api/guides/{guide}/steps/{id}", s.handleStep)
	s.mux.HandleFunc("GET /api/guides/{guide}/now", s.handleNow)
	// Compatibility paths deliberately select one configured guide; they never
	// concatenate multiple routes or resolve ambiguous ids across guides.
	s.mux.HandleFunc("GET /api/guide", s.handleLegacyGuides)
	s.mux.HandleFunc("GET /api/guide/{section}", s.handleLegacySection)
	s.mux.HandleFunc("GET /api/steps/{id}", s.handleLegacyStep)
	s.mux.HandleFunc("GET /api/now", s.handleLegacyNow)
	if s.opts.Ingest != nil {
		s.mux.HandleFunc("GET /api/ingest/manifest", s.handleIngestManifest)
		s.mux.HandleFunc("POST /api/ingest", s.handleIngestPush)
		if s.opts.DevMode {
			s.mux.HandleFunc("POST /api/dev/ingest-token", s.handleCreateIngestToken)
		}
	}
	// Readiness exposes an account's item containers by account id. Until
	// session authentication exists it is a dev-mode-only route.
	if s.opts.Holdings != nil && s.opts.DevMode {
		s.mux.HandleFunc("GET /api/guides/{guide}/readiness", s.handleReadiness)
		s.mux.HandleFunc("GET /api/guides/{guide}/segments/{segment}/readiness", s.handleSegmentReadiness)
	}
	// Progress is addressed by account id too: dev-mode-only until sessions
	// exist (CLAUDE.md rule 10).
	if s.opts.Progress != nil && s.opts.DevMode {
		s.mux.HandleFunc("GET /api/guides/{guide}/board", s.handleBoard)
		s.mux.HandleFunc("PATCH /api/guides/{guide}/steps/{id}/mark", s.handleMark)
		s.mux.HandleFunc("POST /api/guides/{guide}/implied/confirm", s.handleConfirmImplied)
		s.mux.HandleFunc("POST /api/guides/{guide}/marks", s.handleBatchMarks)
		s.mux.HandleFunc("PUT /api/guides/{guide}/cursor", s.handleCursor)
		s.mux.HandleFunc("GET /api/objectives", s.handleObjectives)
		s.mux.HandleFunc("GET /api/guides/{guide}/events", s.handleEvents)
		s.mux.HandleFunc("GET /api/progress", s.handleProgress)
		s.mux.HandleFunc("PUT /api/account/guide", s.handleActiveGuide)
		s.mux.HandleFunc("GET /api/goals", s.handleGoals)
		s.mux.HandleFunc("POST /api/goals", s.handleAddGoal)
		s.mux.HandleFunc("DELETE /api/goals/{id}", s.handleRemoveGoal)
		s.mux.HandleFunc("GET /api/goals/{id}/plan", s.handleGoalPlan)
		s.mux.HandleFunc("GET /api/world", s.handleWorld)
	}
	// The setup picks or creates the account by name: dev-mode-only, like
	// every route that addresses an account without a session (rule 10).
	if s.opts.Accounts != nil && s.opts.DevMode {
		s.mux.HandleFunc("GET /api/setup/lookup", s.handleSetupLookup)
		s.mux.HandleFunc("POST /api/accounts", s.handleCreateAccount)
	}
	if s.opts.Sync != nil && s.opts.DevMode {
		s.mux.HandleFunc("POST /api/sync", s.handleSync)
		s.mux.HandleFunc("GET /api/sync/runs", s.handleSyncRuns)
	}
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	s.mux.Handle("/", spaHandler(s.opts.Static))
}

// writeJSON encodes v with the JSON content type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		slog.Default().Warn("httpapi: encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "status": status})
}

// statusWriter captures the status code and byte count for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Flush forwards to the underlying writer when it supports streaming (SSE later).
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"dur", time.Since(start).Round(time.Microsecond),
			"remote", r.RemoteAddr,
		)
	})
}
