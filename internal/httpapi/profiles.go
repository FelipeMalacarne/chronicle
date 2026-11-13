package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// ProfileStore is the database side of the client profiles (docs/11
// section 4.3, internal/store/workers): the user's profiles and each
// account's profile and own settings. A change reaches a worker on its next
// start or restart, never a running client.
type ProfileStore interface {
	ListProfiles(ctx context.Context, userID string) ([]storeworkers.Profile, error)
	GetProfile(ctx context.Context, userID, name string) (storeworkers.Profile, error)
	PutProfile(ctx context.Context, userID, name, parent string, layer agent.ClientProfile) (storeworkers.Profile, error)
	DeleteProfile(ctx context.Context, userID, name string) error
	AccountClient(ctx context.Context, account string) (storeworkers.AccountClient, error)
	SetAccountClient(ctx context.Context, account, profile string, settings []agent.ProfileSetting) (storeworkers.AccountClient, error)
}

func (s *Server) profileRoutes() {
	m := s.opts.Microbot
	if m == nil || m.Profiles == nil {
		return
	}
	control := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(s.microbotControl(h))) }
	control("GET /api/client-profiles", s.handleProfiles)
	control("GET /api/client-profiles/{name}", s.handleProfileGet)
	control("PUT /api/client-profiles/{name}", s.handleProfilePut)
	control("DELETE /api/client-profiles/{name}", s.handleProfileDelete)
	control("GET /api/accounts/client", s.handleAccountClientGet)
	control("PUT /api/accounts/client", s.handleAccountClientPut)
}

// profileUser owns the profiles of the request: the session's user, or the
// dev user in AUTH_MODE=dev (where the routes have no session).
func profileUser(r *http.Request) string {
	if u := sessionUser(r); u != "" {
		return u
	}
	return "dev"
}

type profilesAnswer struct {
	Profiles []storeworkers.Profile `json:"profiles"`
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.opts.Microbot.Profiles.ListProfiles(r.Context(), profileUser(r))
	if err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profilesAnswer{Profiles: list})
}

func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	p, err := s.opts.Microbot.Profiles.GetProfile(r.Context(), profileUser(r), r.PathValue("name"))
	if err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// profileBody is PUT /api/client-profiles/{name}: the whole profile.
type profileBody struct {
	Parent   string                 `json:"parent"`
	Plugins  []agent.ProfilePlugin  `json:"plugins"`
	Settings []agent.ProfileSetting `json:"settings"`
}

func (s *Server) handleProfilePut(w http.ResponseWriter, r *http.Request) {
	var body profileBody
	if !decodeBody(w, r, &body) {
		return
	}
	p, err := s.opts.Microbot.Profiles.PutProfile(r.Context(), profileUser(r), r.PathValue("name"), body.Parent,
		agent.ClientProfile{Plugins: body.Plugins, Settings: body.Settings})
	if err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.opts.Microbot.Profiles.DeleteProfile(r.Context(), profileUser(r), r.PathValue("name")); err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAccountClientGet(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	ac, err := s.opts.Microbot.Profiles.AccountClient(r.Context(), account)
	if err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ac)
}

// accountClientBody is PUT /api/accounts/client: the account's profile (""
// for none) and its own settings, all at once.
type accountClientBody struct {
	Profile  string                 `json:"profile"`
	Settings []agent.ProfileSetting `json:"settings"`
}

func (s *Server) handleAccountClientPut(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var body accountClientBody
	if !decodeBody(w, r, &body) {
		return
	}
	ac, err := s.opts.Microbot.Profiles.SetAccountClient(r.Context(), account, body.Profile, body.Settings)
	if err != nil {
		s.writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ac)
}

func (s *Server) writeProfileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storeworkers.ErrProfileNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, storeworkers.ErrNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, storeworkers.ErrProfileInUse):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, storeworkers.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.log.Error("client profiles", "err", err)
		writeError(w, http.StatusInternalServerError, "could not read or write the client profile")
	}
}
