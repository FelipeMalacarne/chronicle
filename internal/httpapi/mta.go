package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/FelipeMalacarne/chronicle/internal/core/mta"
)

// maxMTABody caps POST /api/mta/calculate: a request is a few dozen ids and numbers.
const maxMTABody = 64 << 10

// mtaRoutes are the Mage Training Arena calculator's two routes. They read no account (the table
// is the wiki's and the input comes with the request), so they exist wherever the account routes
// do and wear the same authentication, never open in oidc mode.
func (s *Server) mtaRoutes() {
	if !s.accountRoutes() {
		return
	}
	s.mux.Handle("GET /api/mta/rewards", s.authed(s.handleMTARewards))
	s.mux.Handle("POST /api/mta/calculate", s.authed(s.handleMTACalculate))
}

// handleMTARewards is the Rewards Guardian's table: what each reward costs in each room's pizazz
// points, the rooms' runes per point and the Magic level of each Enchant.
func (s *Server) handleMTARewards(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, mta.Data())
}

// handleMTACalculate prices the rewards in the body (mta.Input): what to buy, the points still
// missing in each room and the runes that earning them takes (mta.Result).
func (s *Server) handleMTACalculate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMTABody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in mta.Input
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	res, err := mta.Calculate(in)
	switch {
	case errors.Is(err, mta.ErrNoEnchant):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, res)
	}
}
