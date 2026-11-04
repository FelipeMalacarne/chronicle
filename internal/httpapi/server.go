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
	// DB is pinged by /api/health; nil means no database is configured.
	DB DBPinger
	// Static is the SPA filesystem rooted at the site (index.html at its root).
	// nil disables the SPA and / returns 404.
	Static fs.FS
	// Logger receives request logs; nil uses slog.Default().
	Logger *slog.Logger
}

// Server is the HTTP handler.
type Server struct {
	opts Options
	log  *slog.Logger
	mux  *http.ServeMux
}

// New builds the handler with all routes registered.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{opts: opts, log: opts.Logger, mux: http.NewServeMux()}
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
	s.mux.HandleFunc("GET /api/guide", s.handleGuide)
	s.mux.HandleFunc("GET /api/guide/{section}", s.handleSection)
	s.mux.HandleFunc("GET /api/steps/{id}", s.handleStep)
	s.mux.HandleFunc("GET /api/now", s.handleNow)
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
