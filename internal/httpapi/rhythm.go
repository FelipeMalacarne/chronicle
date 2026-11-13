package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	corerhythm "github.com/FelipeMalacarne/chronicle/internal/core/rhythm"
	"github.com/FelipeMalacarne/chronicle/internal/store/identity"
	storerhythm "github.com/FelipeMalacarne/chronicle/internal/store/rhythm"
)

// RhythmStore reads an account's rhythm (internal/store/rhythm): what the
// v7 shows as XP per hour, the session's and today's gains and the GP
// (docs/ui/decisoes-canvas.md section 7.4, slice V5).
type RhythmStore interface {
	Rhythm(ctx context.Context, account string, now, since time.Time) (corerhythm.Rhythm, error)
}

func (s *Server) rhythmRoutes() {
	if s.opts.Rhythm == nil || !s.accountRoutes() {
		return
	}
	s.mux.Handle("GET /api/account/rhythm", s.authed(s.handleRhythm))
	if s.opts.Identity != nil {
		s.mux.Handle("GET /api/rhythm", s.authed(s.handleFleetRhythm))
	}
}

// rhythmSince is ?since= (RFC 3339, the viewer's midnight, which the web
// knows and the server does not); without it, midnight UTC.
func rhythmSince(r *http.Request, now time.Time) (time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("since"))
	if raw == "" {
		y, m, d := now.UTC().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil || t.After(now) || now.Sub(t) > 48*time.Hour {
		return time.Time{}, false
	}
	return t.UTC(), true
}

type accountRhythm struct {
	Account string `json:"account"`
	corerhythm.Rhythm
}

func (s *Server) handleRhythm(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	since, ok := rhythmSince(r, now)
	if !ok {
		writeError(w, http.StatusBadRequest, "since must be an RFC 3339 time in the last 48 hours")
		return
	}
	out, err := s.opts.Rhythm.Rhythm(r.Context(), account, now, since)
	if errors.Is(err, storerhythm.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		s.log.Error("rhythm", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the rhythm")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, accountRhythm{Account: account, Rhythm: out})
}

// fleetRhythm is the rhythm of each of the user's accounts and the fleet's
// summary ("XP hoje 412k · 7 níveis · 3 contas", "GP na frota 6,1M · +84k hoje").
type fleetRhythm struct {
	Since    time.Time       `json:"since"`
	Accounts []accountRhythm `json:"accounts"`
	Totals   fleetTotals     `json:"totals"`
}

type fleetTotals struct {
	XPToday     int `json:"xp_today"`
	LevelsToday int `json:"levels_today"`
	// Gaining counts the accounts that gained XP today.
	Gaining int `json:"gaining"`
	// GP and GPToday sum the accounts whose bank was seen (and whose
	// change today is known); absent when none.
	GP      *int64 `json:"gp,omitempty"`
	GPToday *int64 `json:"gp_today,omitempty"`
}

func (s *Server) handleFleetRhythm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	since, ok := rhythmSince(r, now)
	if !ok {
		writeError(w, http.StatusBadRequest, "since must be an RFC 3339 time in the last 48 hours")
		return
	}
	userID := sessionUser(r)
	if s.opts.DevMode {
		userID = identity.DevSubject
		if u, found, err := s.opts.Identity.DevUser(ctx); err == nil && found {
			userID = u.ID
		}
	}
	accounts, err := s.opts.Identity.Accounts(ctx, userID)
	if err != nil {
		s.log.Error("rhythm: accounts", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the accounts")
		return
	}
	out := fleetRhythm{Since: since, Accounts: []accountRhythm{}}
	for _, a := range accounts {
		rh, err := s.opts.Rhythm.Rhythm(ctx, a.ID, now, since)
		if err != nil {
			s.log.Error("rhythm", "account", a.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "could not read the rhythm")
			return
		}
		out.Accounts = append(out.Accounts, accountRhythm{Account: a.ID, Rhythm: rh})
		t := &out.Totals
		t.XPToday += rh.Today.XP
		t.LevelsToday += rh.Today.Levels
		if rh.Today.XP > 0 {
			t.Gaining++
		}
		if rh.GP != nil {
			t.GP = addInt64(t.GP, rh.GP.Value)
			if rh.GP.Today != nil {
				t.GPToday = addInt64(t.GPToday, *rh.GP.Today)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func addInt64(sum *int64, v int64) *int64 {
	if sum == nil {
		return &v
	}
	n := *sum + v
	return &n
}
