// Package httpapi serves the JSON API under /api and the embedded SPA at /.
package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
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
	// Events streams each account's invalidation signals (GET /api/events, an
	// account-addressed route like the others). nil disables it.
	Events *events.Hub
	// EventsHeartbeat is the period of the stream's keep-alive comment; zero
	// means 25 s.
	EventsHeartbeat time.Duration
	// DevMode exposes the one-time ingest-token bootstrap endpoint and the
	// account-addressed routes without a session (single local user), and
	// accepts tokenless plugin requests from loopback. It must never be
	// enabled by a production authentication mode, and it excludes OIDC.
	DevMode bool
	// OIDC turns on AUTH_MODE=oidc: /auth/login, /auth/callback, /auth/logout
	// and the account-addressed routes behind a session, restricted to the
	// accounts of the session user. Requires Identity.
	OIDC *OIDCOptions
	// Identity serves GET /api/me and the account ownership checks of oidc
	// mode. nil disables /api/me.
	Identity Identity
	// Rhythm serves the accounts' rhythm (GET /api/account/rhythm and, with
	// Identity, the fleet's GET /api/rhythm); nil leaves them out.
	Rhythm RhythmStore
	// Static is the SPA filesystem rooted at the site (index.html at its root).
	// nil disables the SPA and / returns 404.
	Static fs.FS
	// Microbot turns on the Microbot control module (docs/08); nil leaves
	// every /api/microbot route out.
	Microbot *MicrobotOptions
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
	if opts.OIDC != nil && (opts.DevMode || opts.Identity == nil) {
		panic("httpapi: OIDC needs Identity and excludes DevMode")
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
	s.mux.HandleFunc("GET /api/auth/client", s.handleAuthClient)
	if s.opts.Ingest != nil {
		s.mux.HandleFunc("GET /api/ingest/manifest", s.handleIngestManifest)
		s.mux.HandleFunc("POST /api/ingest", s.handleIngestPush)
		if s.opts.DevMode {
			s.mux.HandleFunc("POST /api/dev/ingest-token", s.handleCreateIngestToken)
		}
	}
	// The plugin panel (docs/04 section 2.5) is addressed by the plugin's
	// credential, like ingest, never by an account id: a token in every mode,
	// tokenless only in dev mode from loopback (pluginCredential).
	if s.opts.Ingest != nil && s.opts.Progress != nil {
		s.mux.HandleFunc("GET /api/plugin/route", s.handlePluginRoute)
		s.mux.HandleFunc("PATCH /api/plugin/guides/{guide}/steps/{id}/mark", s.handlePluginMark)
		s.mux.HandleFunc("POST /api/plugin/guides/{guide}/marks", s.handlePluginRestore)
	}
	// Every route below addresses an account by id: in dev mode without a
	// session, in oidc mode behind one and only for the session user's
	// accounts (CLAUDE.md rule 10). Without either, they do not exist.
	if s.opts.OIDC != nil {
		sessions := s.opts.OIDC.Sessions
		s.mux.HandleFunc("GET /auth/login", s.handleLogin)
		s.mux.Handle("GET /auth/callback", sessions.LoadAndSave(http.HandlerFunc(s.handleCallback)))
		s.mux.Handle("POST /auth/logout", sessions.LoadAndSave(http.HandlerFunc(s.handleLogout)))
	}
	if s.opts.Identity != nil && s.accountRoutes() {
		s.mux.Handle("GET /api/me", s.authed(s.handleMe))
	}
	account := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(h)) }
	if s.opts.Holdings != nil && s.accountRoutes() {
		account("GET /api/guides/{guide}/readiness", s.handleReadiness)
		account("GET /api/guides/{guide}/segments/{segment}/readiness", s.handleSegmentReadiness)
	}
	if s.opts.Progress != nil && s.accountRoutes() {
		account("GET /api/guides/{guide}/board", s.handleBoard)
		account("GET /api/guides/{guide}/nearby", s.handleNearby)
		account("GET /api/route", s.handleRoute)
		account("PATCH /api/guides/{guide}/steps/{id}/mark", s.handleMark)
		account("POST /api/guides/{guide}/implied/confirm", s.handleConfirmImplied)
		account("POST /api/guides/{guide}/marks", s.handleBatchMarks)
		account("PUT /api/guides/{guide}/cursor", s.handleCursor)
		account("GET /api/objectives", s.handleObjectives)
		account("GET /api/guides/{guide}/events", s.handleEvents)
		account("GET /api/progress", s.handleProgress)
		account("GET /api/progress/check", s.handleProgressCheck)
		account("PUT /api/account/guide", s.handleActiveGuide)
		account("GET /api/goals", s.handleGoals)
		account("POST /api/goals", s.handleAddGoal)
		account("DELETE /api/goals/{id}", s.handleRemoveGoal)
		account("GET /api/goals/{id}/plan", s.handleGoalPlan)
		account("GET /api/graph", s.handleGraph)
		account("GET /api/world", s.handleWorld)
		account("GET /api/world/entity", s.handleWorldEntity)
		account("GET /api/account/state", s.handleAccountState)
		account("GET /api/account/timers", s.handleTimers)
	}
	// The setup picks or creates an account by name, of the dev user or of
	// the session user.
	if s.opts.Accounts != nil && s.accountRoutes() {
		account("GET /api/setup/lookup", s.handleSetupLookup)
		account("POST /api/accounts", s.handleCreateAccount)
	}
	// The live stream is addressed by account id; EventSource sends the
	// session cookie (same origin).
	if s.opts.Events != nil && s.accountRoutes() {
		account("GET /api/events", s.handleLiveEvents)
	}
	if s.opts.Sync != nil && s.accountRoutes() {
		account("POST /api/sync", s.handleSync)
		account("GET /api/sync/runs", s.handleSyncRuns)
	}
	s.microbotRoutes()
	s.workerRoutes()
	s.profileRoutes()
	s.rhythmRoutes()
	s.driftRoutes()
	s.fleetPluginRoutes()
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

// Flush forwards to the underlying writer when it supports streaming.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection (flush, write
// deadline) through the logging wrapper.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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
