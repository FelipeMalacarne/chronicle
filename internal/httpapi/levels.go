package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	coreactivity "github.com/FelipeMalacarne/chronicle/internal/core/activity"
	storeactivity "github.com/FelipeMalacarne/chronicle/internal/store/activity"
	"github.com/FelipeMalacarne/chronicle/internal/store/identity"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// LevelsStore reads the accounts' level-ups (internal/store/activity): the
// "subiu de nível" of the v7's Histórico and Atividade
// (docs/ui/decisoes-canvas.md section 7.4).
type LevelsStore interface {
	Levels(ctx context.Context, account string, since time.Time) (coreactivity.Levels, error)
}

const (
	// levelsWindow is how far back ?since= may go: the Histórico shows the
	// last 7 days.
	levelsWindow = 7 * 24 * time.Hour
	// levelsDefaultSince is the window without ?since=.
	levelsDefaultSince = 24 * time.Hour
	levelsDefaultLimit = 50
	levelsMaxLimit     = 500
)

func (s *Server) levelRoutes() {
	if s.opts.Levels == nil || !s.accountRoutes() {
		return
	}
	s.mux.Handle("GET /api/account/levels", s.authed(s.handleAccountLevels))
	if s.opts.Identity != nil {
		s.mux.Handle("GET /api/levels", s.authed(s.handleFleetLevels))
	}
}

// levelEvent is one account's level-up ("14:06 Felipe Pesca subiu para
// Fishing 76").
type levelEvent struct {
	Account string `json:"account"`
	coreactivity.LevelUp
}

// accountLevels is GET /api/account/levels: the account's level-ups and
// each skill's progress since ?since= ("Fishing 71 → 76 +412k").
type accountLevels struct {
	Account string    `json:"account"`
	Since   time.Time `json:"since"`
	// Total counts the window's level-ups; Events has the newest ?limit= of
	// them.
	Total  int                      `json:"total"`
	Events []levelEvent             `json:"events"`
	Skills []coreactivity.SkillGain `json:"skills"`
}

// fleetLevels is GET /api/levels: the level-ups of every account of the
// user, merged newest first (the Atividade's Diário).
type fleetLevels struct {
	Since  time.Time    `json:"since"`
	Total  int          `json:"total"`
	Events []levelEvent `json:"events"`
}

// levelsQuery reads ?since= (RFC 3339, not in the future, at most
// levelsWindow ago; default the last 24 hours) and ?limit= (1 to
// levelsMaxLimit; default levelsDefaultLimit). msg is the 400 answer.
func levelsQuery(r *http.Request, now time.Time) (since time.Time, limit int, msg string) {
	since, limit = now.Add(-levelsDefaultSince).Truncate(time.Second), levelsDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil || t.After(now) || now.Sub(t) > levelsWindow {
			return since, limit, "since must be an RFC 3339 time in the last 7 days"
		}
		since = t.UTC()
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > levelsMaxLimit {
			return since, limit, "limit must be between 1 and " + strconv.Itoa(levelsMaxLimit)
		}
		limit = n
	}
	return since, limit, ""
}

func (s *Server) handleAccountLevels(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	since, limit, msg := levelsQuery(r, time.Now().UTC())
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	lv, err := s.opts.Levels.Levels(r.Context(), account, since)
	if errors.Is(err, storeactivity.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		s.log.Error("levels", "account", account, "err", err)
		writeError(w, http.StatusInternalServerError, "could not read the level-ups")
		return
	}
	events := accountEvents(account, lv.Events)
	out := accountLevels{Account: account, Since: since, Total: len(events), Events: events[:min(limit, len(events))], Skills: lv.Skills}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFleetLevels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	since, limit, msg := levelsQuery(r, time.Now().UTC())
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	accounts, err := s.requestAccounts(r)
	if err != nil {
		s.log.Error("levels: accounts", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the accounts")
		return
	}
	events := []levelEvent{}
	for _, a := range accounts {
		lv, lerr := s.opts.Levels.Levels(ctx, a.ID, since)
		if errors.Is(lerr, storeactivity.ErrNotFound) {
			continue // removed since the list was read
		}
		if lerr != nil {
			s.log.Error("levels", "account", a.ID, "err", lerr)
			writeError(w, http.StatusInternalServerError, "could not read the level-ups")
			return
		}
		events = append(events, accountEvents(a.ID, lv.Events)...)
	}
	// Each account's events are already newest first; the merge keeps that
	// order within an instant and puts the accounts in id order.
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.After(events[j].At)
		}
		return events[i].Account < events[j].Account
	})
	out := fleetLevels{Since: since, Total: len(events), Events: events[:min(limit, len(events))]}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// requestAccounts are the accounts of the request's user, found as handleMe
// finds them: the dev user's in dev mode (the dev subject before the dev
// user exists), the session or token user's in oidc.
func (s *Server) requestAccounts(r *http.Request) ([]storeprogress.AccountView, error) {
	ctx := r.Context()
	userID := sessionUser(r)
	if s.opts.DevMode {
		u, found, err := s.opts.Identity.DevUser(ctx)
		if err != nil {
			return nil, err
		}
		userID = identity.DevSubject
		if found {
			userID = u.ID
		}
	}
	return s.opts.Identity.Accounts(ctx, userID)
}

func accountEvents(account string, ups []coreactivity.LevelUp) []levelEvent {
	out := make([]levelEvent, len(ups))
	for i, u := range ups {
		out[i] = levelEvent{Account: account, LevelUp: u}
	}
	return out
}
