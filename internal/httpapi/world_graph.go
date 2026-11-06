package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
)

// graphResponse is the requirement graph of one focus (Mundo › Grafo,
// V6Grafo1440 and V6Grafo640).
type graphResponse struct {
	Account string `json:"account"`
	coreprogress.Graph
}

// handleWorldGraph is GET /api/world/graph?account=&focus=<objective key>
// &depth=1..6 (default 3).
func (s *Server) handleWorldGraph(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	focus := strings.TrimSpace(r.URL.Query().Get("focus"))
	if focus == "" {
		writeError(w, http.StatusBadRequest, "focus is required (an objective key, e.g. quest:shilo_village@finished)")
		return
	}
	depth := coreprogress.DefaultGraphDepth
	if raw := r.URL.Query().Get("depth"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > coreprogress.MaxGraphDepth {
			writeError(w, http.StatusBadRequest, "depth must be between 1 and "+strconv.Itoa(coreprogress.MaxGraphDepth))
			return
		}
		depth = n
	}
	g, err := s.opts.Progress.Graph(r.Context(), account, focus, depth)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graphResponse{Account: account, Graph: g})
}
