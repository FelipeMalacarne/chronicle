package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// AccountService finds and creates the accounts of the setup (V4Setup).
type AccountService interface {
	FindAccount(ctx context.Context, rsn string) (storeprogress.AccountView, bool, error)
	CreateAccount(ctx context.Context, in storeprogress.AccountInput) (storeprogress.AccountView, bool, error)
}

// ModeDetector probes the hiscores tables for a name.
type ModeDetector interface {
	Detect(ctx context.Context, rsn string) (hiscores.Detection, error)
}

type lookupSkill struct {
	Skill string `json:"skill"`
	Level int64  `json:"level"`
}

// lookupResponse is step 1 of the setup: an existing account to enter, or
// the mode and levels the hiscores show for a new one.
type lookupResponse struct {
	RSN     string                     `json:"rsn"`
	Account *storeprogress.AccountView `json:"account"`
	// Found: some hiscores table knows the name; Mode is the most specific
	// one ("" when only the normal table does).
	Found  bool          `json:"found"`
	Mode   string        `json:"mode"`
	Total  int64         `json:"total_level,omitempty"`
	Skills []lookupSkill `json:"skills"`
	// HiscoresError is set when the hiscores could not be asked; the setup
	// goes on with a mode the owner picks.
	HiscoresError string `json:"hiscores_error,omitempty"`
}

func (s *Server) handleSetupLookup(w http.ResponseWriter, r *http.Request) {
	rsn := strings.TrimSpace(r.URL.Query().Get("rsn"))
	if rsn == "" || len(rsn) > 12 {
		writeError(w, http.StatusBadRequest, "rsn must be 1 to 12 characters")
		return
	}
	out := lookupResponse{RSN: rsn, Skills: []lookupSkill{}}
	acc, ok, err := s.opts.Accounts.FindAccount(r.Context(), rsn)
	if err != nil {
		s.log.Error("setup: find account", "err", err)
		writeError(w, http.StatusInternalServerError, "could not look up the account")
		return
	}
	if ok {
		out.Account = &acc
		writeJSON(w, http.StatusOK, out)
		return
	}
	if s.opts.Hiscores == nil {
		out.HiscoresError = "hiscores not configured"
		writeJSON(w, http.StatusOK, out)
		return
	}
	d, err := s.opts.Hiscores.Detect(r.Context(), rsn)
	if err != nil {
		out.HiscoresError = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Found, out.Mode = d.Found, d.Mode
	for _, sk := range d.Snapshot.Skills {
		if strings.EqualFold(sk.Name, "Overall") {
			out.Total = sk.Level
			continue
		}
		if sk.Rank >= 0 {
			out.Skills = append(out.Skills, lookupSkill{Skill: strings.ToLower(sk.Name), Level: sk.Level})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var in storeprogress.AccountInput
	if !decodeBody(w, r, &in) {
		return
	}
	acc, created, err := s.opts.Accounts.CreateAccount(r.Context(), in)
	switch {
	case errors.Is(err, storeprogress.ErrInvalidAccount):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		s.log.Error("setup: create account", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create the account")
	case created:
		writeJSON(w, http.StatusCreated, acc)
	default:
		writeJSON(w, http.StatusOK, acc)
	}
}
