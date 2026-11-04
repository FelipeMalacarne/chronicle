package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaHandler serves files from static and falls back to index.html for any
// path without a file, so client-side routes deep-link correctly.
func spaHandler(static fs.FS) http.Handler {
	if static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "no web UI embedded in this build")
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(static, name); err == nil && !info.IsDir() {
				http.ServeFileFS(w, r, static, name)
				return
			}
		}
		data, err := fs.ReadFile(static, "index.html")
		if err != nil {
			writeError(w, http.StatusNotFound, "index.html missing from embedded build")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	})
}
