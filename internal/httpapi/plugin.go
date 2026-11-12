package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// Headers of a tokenless dev plugin request (docs/04 section 2.5). They are
// never logged: the request log records method, path, status and peer only.
const (
	// HeaderInstallation carries the plugin's installation id (a UUID the
	// plugin generates once).
	HeaderInstallation = "X-Ironman-Installation"
	// HeaderAccount carries the plugin's optional "Conta" setting (an
	// account id) when several dev accounts exist.
	HeaderAccount = "X-Ironman-Account"
)

// pluginCredential authenticates a plugin request (ingest, manifest, panel).
// A bearer is used whenever the Authorization header is present, in every
// mode: an ipl_ token, or in AUTH_MODE=oidc the JWT access token of the
// plugin's device-flow login, which acts for the installation as that user,
// on that user's accounts, with the scopes of the user's project roles
// (pluginScopes). Without Authorization, only AUTH_MODE=dev accepts the
// request, and only when the TCP peer is loopback: RemoteAddr is the
// connection's address, and X-Forwarded-For or any other header is never
// trusted for this.
func (s *Server) pluginCredential(w http.ResponseWriter, r *http.Request) (storeingest.Credential, bool) {
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		token, ok := bearerToken(w, r)
		if !ok {
			return storeingest.Credential{}, false
		}
		if s.opts.OIDC == nil || strings.HasPrefix(token, storeingest.TokenPrefix) {
			return storeingest.Credential{Token: token}, true
		}
		installation, ok := installationHeader(w, r, "with a login")
		if !ok {
			return storeingest.Credential{}, false
		}
		uid, roles, ok := s.verifyBearer(w, r, token, "ingest")
		if !ok {
			return storeingest.Credential{}, false
		}
		scopes := pluginScopes(roles)
		if !slices.Contains(scopes, coreingest.ScopeStateWrite) {
			writeError(w, http.StatusForbidden, "the login lacks the project role "+RoleIngest)
			return storeingest.Credential{}, false
		}
		return storeingest.Credential{
			Installation: installation, Account: strings.TrimSpace(r.Header.Get(HeaderAccount)),
			User: uid, Scopes: scopes,
		}, true
	}
	if !s.opts.DevMode || !isLoopback(r.RemoteAddr) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="ingest"`)
		writeError(w, http.StatusUnauthorized, "bearer ingest token required")
		return storeingest.Credential{}, false
	}
	installation, ok := installationHeader(w, r, "without a token")
	if !ok {
		return storeingest.Credential{}, false
	}
	return storeingest.Credential{Installation: installation, Account: strings.TrimSpace(r.Header.Get(HeaderAccount))}, true
}

// RoleIngest is the project role that lets a plugin login push state; the
// item and mark scopes are project roles of the same names (docs/04
// section 2.5).
const RoleIngest = "ingest"

// pluginScopes maps the project roles of a plugin login onto the ingest
// scopes: ingest is state:write, and the item and mark scopes are roles of
// the same name. Other roles are ignored.
func pluginScopes(roles []string) []coreingest.TokenScope {
	var scopes []coreingest.TokenScope
	for _, role := range roles {
		switch scope := coreingest.TokenScope(role); {
		case role == RoleIngest:
			scopes = append(scopes, coreingest.ScopeStateWrite)
		case scope == coreingest.ScopeItemsBank, scope == coreingest.ScopeItemsInventory,
			scope == coreingest.ScopeItemsEquipment, scope == coreingest.ScopeMarksWrite:
			scopes = append(scopes, scope)
		}
	}
	slices.Sort(scopes)
	return slices.Compact(scopes)
}

// installationHeader reads the plugin's installation id, which every
// request without an ipl_ token carries.
func installationHeader(w http.ResponseWriter, r *http.Request, how string) (string, bool) {
	installation := strings.TrimSpace(r.Header.Get(HeaderInstallation))
	if !coreingest.ValidInstallationID(installation) {
		writeError(w, http.StatusBadRequest, how+", the "+HeaderInstallation+" header must carry the plugin's installation id (a lower-case UUID)")
		return "", false
	}
	return installation, true
}

// isLoopback reports whether the connection's peer is 127.0.0.0/8 or ::1.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// pluginPrincipal authenticates a panel request and returns its account.
func (s *Server) pluginPrincipal(w http.ResponseWriter, r *http.Request) (storeingest.Principal, bool) {
	cred, ok := s.pluginCredential(w, r)
	if !ok {
		return storeingest.Principal{}, false
	}
	p, err := s.opts.Ingest.Principal(r.Context(), cred)
	if err != nil {
		s.writeIngestError(w, err)
		return storeingest.Principal{}, false
	}
	return p, true
}

// pluginWriter is pluginPrincipal for the panel's mark writes, which need
// marks:write (a dev installation has it).
func (s *Server) pluginWriter(w http.ResponseWriter, r *http.Request) (storeingest.Principal, bool) {
	p, ok := s.pluginPrincipal(w, r)
	if ok && !p.Allows(coreingest.ScopeMarksWrite) {
		writeError(w, http.StatusForbidden, "the plugin token lacks the marks:write scope")
		return storeingest.Principal{}, false
	}
	return p, ok
}

// windowLimit reads ?limit= (steps after the cursor), default 15.
func windowLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return panel.DefaultLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > panel.MaxLimit {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(panel.MaxLimit))
		return 0, false
	}
	return n, true
}

// handlePluginRoute is GET /api/plugin/route: the panel's window of the
// active route.
func (s *Server) handlePluginRoute(w http.ResponseWriter, r *http.Request) {
	limit, ok := windowLimit(w, r)
	if !ok {
		return
	}
	p, ok := s.pluginPrincipal(w, r)
	if !ok {
		return
	}
	s.writeRouteWindow(w, r, p.AccountID, limit)
}

// handlePluginMark is PATCH /api/plugin/guides/{guide}/steps/{id}/mark: the
// web's mark (same body, same operation, same progress event) for the
// panel's Feito, Pular and Desfazer; it answers with the new window.
func (s *Server) handlePluginMark(w http.ResponseWriter, r *http.Request) {
	limit, ok := windowLimit(w, r)
	if !ok {
		return
	}
	p, ok := s.pluginWriter(w, r)
	if !ok {
		return
	}
	var req markRequest
	if !decodeBody(w, r, &req) {
		return
	}
	ctx := storeprogress.WithCommand(r.Context(), storeprogress.CommandMeta{Via: storeprogress.ViaPluginPanel, Revokes: req.Revokes})
	if _, err := s.opts.Progress.Mark(ctx, p.AccountID, r.PathValue("guide"), r.PathValue("id"), req.Kind, req.Note); err != nil {
		s.writeProgressError(w, err)
		return
	}
	s.writeRouteWindow(w, r, p.AccountID, limit)
}

// handlePluginRestore is POST /api/plugin/guides/{guide}/marks with
// {restore: [...]}: the web's batch undo, which the panel's Desfazer uses to
// put an implied mark back. "Mark everything before" stays a web action.
func (s *Server) handlePluginRestore(w http.ResponseWriter, r *http.Request) {
	limit, ok := windowLimit(w, r)
	if !ok {
		return
	}
	p, ok := s.pluginWriter(w, r)
	if !ok {
		return
	}
	var req batchMarkRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.BeforeStepID != "" || len(req.Restore) == 0 || len(req.Restore) > 2000 {
		writeError(w, http.StatusBadRequest, "send restore (1 to 2000 steps)")
		return
	}
	ctx := storeprogress.WithCommand(r.Context(), storeprogress.CommandMeta{Via: storeprogress.ViaPluginPanel, Revokes: req.Revokes})
	if _, err := s.opts.Progress.RestoreMarks(ctx, p.AccountID, r.PathValue("guide"), req.Restore); err != nil {
		s.writeProgressError(w, err)
		return
	}
	s.writeRouteWindow(w, r, p.AccountID, limit)
}

func (s *Server) writeRouteWindow(w http.ResponseWriter, r *http.Request, accountID string, limit int) {
	window, err := s.routeWindow(r.Context(), accountID, limit)
	if err != nil {
		if errors.Is(err, errHoldings) {
			s.log.Error("plugin route: load item holdings", "err", err)
			writeError(w, http.StatusInternalServerError, "could not load item holdings")
			return
		}
		s.writeProgressError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, window)
}

var errHoldings = errors.New("item holdings")

// routeWindow assembles the window of the account's active route: the
// board, the item holdings and the catalog go into panel.Build.
func (s *Server) routeWindow(ctx context.Context, accountID string, limit int) (panel.Window, error) {
	guideID, err := s.opts.Progress.ActiveGuide(ctx, accountID)
	if err != nil {
		return panel.Window{}, err
	}
	g, ok := s.opts.Content.Guide(guideID)
	if !ok {
		return panel.Window{}, storeprogress.ErrGuideNotFound
	}
	board, err := s.opts.Progress.Board(ctx, accountID, guideID)
	if err != nil {
		return panel.Window{}, err
	}
	var h rules.ItemHoldings
	if s.opts.Holdings != nil {
		if h, err = s.opts.Holdings.ItemHoldings(ctx, accountID); err != nil {
			return panel.Window{}, errors.Join(errHoldings, err)
		}
	}
	cur := coreprogress.CursorRecord{}
	if board.Cursor != nil {
		cur = coreprogress.CursorRecord{StepID: board.Cursor.StepID, Cause: board.Cursor.Cause}
	}
	return panel.Build(panel.Input{
		Guide: g, Board: board.Board, Cursor: cur, Holdings: h, Registry: s.opts.Catalog, Limit: limit,
	}), nil
}
