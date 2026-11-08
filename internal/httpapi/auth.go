package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/FelipeMalacarne/ironman-planner/internal/auth"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/identity"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// Session lifetime of AUTH_MODE=oidc (docs/04 section 2.5): a session ends
// after a week without requests or 30 days after the login, whichever comes
// first; then the next API call answers 401 and the web goes to /auth/login.
const (
	SessionIdleTimeout = 7 * 24 * time.Hour
	SessionLifetime    = 30 * 24 * time.Hour
	sessionCookie      = "ironman_session"
	flowCookie         = "ironman_oidc"
	flowCookiePath     = "/auth/callback"
)

// CSRF (docs/04 section 2.5): every state-changing request authenticated by
// the session cookie must carry this header. A cross-site page cannot set a
// custom header without a CORS preflight, which this server never grants;
// SameSite=Lax already keeps the cookie off cross-site POSTs, and the header
// covers the browsers and same-site subdomains that Lax alone does not.
const (
	CSRFHeader = "X-Ironman-CSRF"
	csrfValue  = "1"
)

// Session keys.
const (
	keyUserID  = "user_id"
	keySubject = "sub"
	keyRoles   = "roles"
	keyIDToken = "id_token"
)

// Identity persists the users of the web login and answers who owns an
// account (internal/store/identity).
type Identity interface {
	LoginOIDC(ctx context.Context, in identity.Login) (identity.LoginResult, error)
	User(ctx context.Context, id string) (identity.User, error)
	DevUser(ctx context.Context) (identity.User, bool, error)
	Accounts(ctx context.Context, userID string) ([]storeprogress.AccountView, error)
	OwnsAccount(ctx context.Context, userID, accountID string) (bool, error)
	// UserBySubject is the existing user of a bearer token (never created).
	UserBySubject(ctx context.Context, issuer, subject string) (identity.User, error)
}

// OIDCOptions turns on AUTH_MODE=oidc: the /auth routes and the session that
// the account-addressed routes require.
type OIDCOptions struct {
	Provider *auth.Provider
	Policy   auth.Policy
	// Sessions is the session manager (NewSessionManager over pgxstore).
	Sessions *scs.SessionManager
	// Flow seals the login state cookie (keyed by SESSION_SECRET).
	Flow *auth.Sealer
	// PublicBaseURL is where the issuer sends the browser after logout.
	PublicBaseURL string
	// SecureCookies sets Secure on every cookie (PUBLIC_BASE_URL is https).
	SecureCookies bool
}

// NewSessionManager is the session manager of AUTH_MODE=oidc over store:
// HttpOnly, SameSite=Lax, Secure when secure, persistent, with the idle and
// absolute lifetimes above.
func NewSessionManager(store scs.Store, secure bool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = store
	sm.IdleTimeout = SessionIdleTimeout
	sm.Lifetime = SessionLifetime
	sm.Cookie.Name = sessionCookie
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = secure
	sm.Cookie.Path = "/"
	sm.Cookie.Persist = true
	sm.ErrorFunc = func(w http.ResponseWriter, _ *http.Request, err error) {
		slog.Default().Error("auth: session store", "err", err)
		writeError(w, http.StatusInternalServerError, "session store unavailable")
	}
	return sm
}

type (
	userKey  struct{}
	rolesKey struct{}
)

// sessionUser is the user of the request in AUTH_MODE=oidc (of the session
// or of the bearer token); "" in dev mode (the store then means the dev
// user).
func sessionUser(r *http.Request) string {
	id, _ := r.Context().Value(userKey{}).(string)
	return id
}

// requestRoles are the project roles of the request's session or token.
func requestRoles(r *http.Request) []string {
	roles, _ := r.Context().Value(rolesKey{}).([]string)
	return roles
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// accountRoutes reports whether the account-addressed routes exist: in dev
// mode, and in oidc mode behind a session (CLAUDE.md rule 10).
func (s *Server) accountRoutes() bool { return s.opts.DevMode || s.opts.OIDC != nil }

// authed wraps an account-addressed handler. Dev mode passes through. OIDC
// mode authenticates by a bearer token when the request carries
// Authorization (bearerAuthed), else by the session: it loads the session,
// answers 401 without a user, re-checks the policy (an allow list changed by
// a restart applies to open sessions too), demands the CSRF header on
// state-changing methods and puts the user in the context.
func (s *Server) authed(h http.HandlerFunc) http.Handler {
	if s.opts.DevMode {
		return h
	}
	o := s.opts.OIDC
	session := o.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		uid := o.Sessions.GetString(ctx, keyUserID)
		if uid == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "login required", "status": http.StatusUnauthorized, "login": "/auth/login"})
			return
		}
		roles, _ := o.Sessions.Get(ctx, keyRoles).([]string)
		if !o.Policy.Allows(o.Sessions.GetString(ctx, keySubject), roles) {
			_ = o.Sessions.Destroy(ctx)
			writeError(w, http.StatusForbidden, deniedMessage)
			return
		}
		if !safeMethod(r.Method) && r.Header.Get(CSRFHeader) != csrfValue {
			writeError(w, http.StatusForbidden, "missing "+CSRFHeader+" header")
			return
		}
		ctx = context.WithValue(ctx, userKey{}, uid)
		h(w, r.WithContext(context.WithValue(ctx, rolesKey{}, roles)))
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			s.bearerAuthed(h, w, r)
			return
		}
		session.ServeHTTP(w, r)
	})
}

// bearerAuthed authenticates by the JWT access token of the native client
// (CLI, `ironman mcp`, later the plugin; docs/04 section 2.5): signature,
// issuer, audience and expiry by the cached JWKS, the same policy as the
// session, and an existing user of (issuer, sub). The session is never
// consulted, so a bad token is 401 even with a valid cookie, and no CSRF
// header is needed: the browser never attaches a bearer token by itself.
func (s *Server) bearerAuthed(h http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	o := s.opts.OIDC
	unauthorized := func(msg string) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="ironman"`)
		writeError(w, http.StatusUnauthorized, msg)
	}
	scheme, raw, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	raw = strings.TrimSpace(raw)
	if !strings.EqualFold(scheme, "Bearer") || raw == "" {
		unauthorized("Authorization must be a Bearer token")
		return
	}
	id, err := o.Provider.VerifyAccessToken(r.Context(), raw)
	if errors.Is(err, auth.ErrNoNativeClient) {
		unauthorized("bearer login is not configured (OIDC_NATIVE_CLIENT_ID)")
		return
	}
	if err != nil {
		s.log.Info("auth: bearer token refused", "err", err)
		unauthorized("invalid or expired token")
		return
	}
	if !o.Policy.Allows(id.Subject, id.Roles) {
		writeError(w, http.StatusForbidden, deniedMessage)
		return
	}
	u, err := s.opts.Identity.UserBySubject(r.Context(), id.Issuer, id.Subject)
	if errors.Is(err, identity.ErrUserNotFound) {
		writeError(w, http.StatusForbidden, "unknown user: sign in on the web once before using a token")
		return
	}
	if err != nil {
		s.log.Error("auth: bearer user", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the user")
		return
	}
	ctx := context.WithValue(r.Context(), userKey{}, u.ID)
	h(w, r.WithContext(context.WithValue(ctx, rolesKey{}, id.Roles)))
}

// accountParam reads ?account=. In oidc mode the account must belong to the
// session user; another user's account and a missing one are the same 404.
func (s *Server) accountParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	account := strings.TrimSpace(r.URL.Query().Get("account"))
	if account == "" {
		writeError(w, http.StatusBadRequest, "account query parameter is required")
		return "", false
	}
	if s.opts.DevMode {
		return account, true
	}
	uid := sessionUser(r)
	if uid == "" || s.opts.Identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "login required", "status": http.StatusUnauthorized, "login": "/auth/login"})
		return "", false
	}
	ok, err := s.opts.Identity.OwnsAccount(r.Context(), uid, account)
	if err != nil {
		s.log.Error("auth: account owner", "err", err)
		writeError(w, http.StatusInternalServerError, "could not check the account")
		return "", false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "account not found")
		return "", false
	}
	return account, true
}

// handleLogin starts the authorization code flow: state, nonce and PKCE
// verifier go in a sealed cookie scoped to the callback, and the browser goes
// to the issuer. ?return= is where to land afterwards (a local path).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	o := s.opts.OIDC
	f := auth.NewFlow(r.URL.Query().Get("return"))
	sealed, err := o.Flow.Seal(f)
	if err != nil {
		s.log.Error("auth: seal login state", "err", err)
		s.authPage(w, http.StatusInternalServerError, "Não foi possível iniciar o login", "Tente de novo em instantes.", true)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: flowCookie, Value: sealed, Path: flowCookiePath, MaxAge: int(auth.FlowTTL.Seconds()),
		HttpOnly: true, Secure: o.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, o.Provider.AuthCodeURL(f.State, f.Nonce, f.Verifier), http.StatusFound)
}

// handleCallback finishes the flow: the sealed state must match, the code is
// exchanged with the verifier, the ID token and its nonce are verified, the
// policy decides, the user is upserted by (issuer, sub) and a fresh session
// token is issued (no fixation).
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	o := s.opts.OIDC
	ctx := r.Context()
	http.SetCookie(w, &http.Cookie{
		Name: flowCookie, Value: "", Path: flowCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: o.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	var f auth.Flow
	c, err := r.Cookie(flowCookie)
	if err == nil {
		f, err = o.Flow.Open(c.Value)
	}
	q := r.URL.Query()
	switch {
	case err != nil:
		s.authPage(w, http.StatusBadRequest, "Login expirado", "Este retorno do login não corresponde a um login iniciado aqui, ou demorou mais de 10 minutos.", true)
		return
	case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.State)) != 1:
		s.log.Warn("auth: callback state mismatch")
		s.authPage(w, http.StatusBadRequest, "Login inválido", "O estado devolvido pelo provedor não confere com o login iniciado.", true)
		return
	case q.Get("error") != "":
		s.log.Warn("auth: issuer returned an error", "error", q.Get("error"), "description", q.Get("error_description"))
		s.authPage(w, http.StatusForbidden, "Login recusado", "O provedor de login recusou a entrada ("+q.Get("error")+").", true)
		return
	case q.Get("code") == "":
		s.authPage(w, http.StatusBadRequest, "Login inválido", "O provedor não devolveu o código de autorização.", true)
		return
	}
	id, rawIDToken, err := o.Provider.Exchange(ctx, q.Get("code"), f.Verifier, f.Nonce)
	if err != nil {
		s.log.Warn("auth: callback rejected", "err", err)
		s.authPage(w, http.StatusBadRequest, "Login inválido", "Não foi possível validar o login no provedor.", true)
		return
	}
	if !o.Policy.Allows(id.Subject, id.Roles) {
		s.log.Warn("auth: login denied by policy", "sub", id.Subject, "roles", strings.Join(id.Roles, ","))
		s.deniedPage(w, rawIDToken)
		return
	}
	res, err := s.opts.Identity.LoginOIDC(ctx, identity.Login{
		Issuer: id.Issuer, Subject: id.Subject, Username: id.Username, Email: id.Email, DisplayName: id.DisplayName,
	})
	if errors.Is(err, identity.ErrSubjectConflict) {
		s.log.Warn("auth: subject belongs to another issuer", "sub", id.Subject, "iss", id.Issuer)
		s.deniedPage(w, rawIDToken)
		return
	}
	if err != nil {
		s.log.Error("auth: persist login", "err", err)
		s.authPage(w, http.StatusInternalServerError, "Não foi possível entrar", "Erro ao gravar o usuário. Tente de novo em instantes.", true)
		return
	}
	if res.ClaimedAccounts > 0 {
		s.log.Info("auth: first OIDC login claimed the dev accounts", "user", res.User.ID, "accounts", res.ClaimedAccounts)
	}
	if err := o.Sessions.RenewToken(ctx); err != nil {
		s.log.Error("auth: renew session token", "err", err)
		s.authPage(w, http.StatusInternalServerError, "Não foi possível entrar", "Erro ao abrir a sessão. Tente de novo em instantes.", true)
		return
	}
	o.Sessions.Put(ctx, keyUserID, res.User.ID)
	o.Sessions.Put(ctx, keySubject, id.Subject)
	o.Sessions.Put(ctx, keyRoles, id.Roles)
	o.Sessions.Put(ctx, keyIDToken, rawIDToken)
	s.log.Info("auth: login", "user", res.User.ID, "created", res.Created)
	http.Redirect(w, r, f.ReturnTo, http.StatusSeeOther)
}

// handleLogout destroys the session and answers where the browser goes next:
// the issuer's end_session_endpoint (RP-initiated logout, back to
// PUBLIC_BASE_URL) when discovery advertises one, else "/". POST only, with
// the CSRF header, so no other site can log the user out.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	o := s.opts.OIDC
	if r.Header.Get(CSRFHeader) != csrfValue {
		writeError(w, http.StatusForbidden, "missing "+CSRFHeader+" header")
		return
	}
	ctx := r.Context()
	hint := o.Sessions.GetString(ctx, keyIDToken)
	if err := o.Sessions.Destroy(ctx); err != nil {
		s.log.Error("auth: destroy session", "err", err)
		writeError(w, http.StatusInternalServerError, "could not end the session")
		return
	}
	redirect := "/"
	if end := o.Provider.EndSessionURL(hint, o.PublicBaseURL+"/"); end != "" {
		redirect = end
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect": redirect})
}

// meResponse is GET /api/me: who is logged in and which accounts are theirs.
type meResponse struct {
	AuthMode string                      `json:"auth_mode"`
	User     identity.User               `json:"user"`
	Roles    []string                    `json:"roles"`
	Accounts []storeprogress.AccountView `json:"accounts"`
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := meResponse{AuthMode: "dev", Roles: []string{}}
	var err error
	if s.opts.DevMode {
		var found bool
		out.User, found, err = s.opts.Identity.DevUser(ctx)
		if err == nil && !found {
			out.User = identity.User{ID: identity.DevSubject, Username: identity.DevSubject, Role: "owner"}
		}
	} else {
		out.AuthMode = "oidc"
		out.User, err = s.opts.Identity.User(ctx, sessionUser(r))
		if errors.Is(err, identity.ErrUserNotFound) {
			if r.Header.Get("Authorization") == "" {
				_ = s.opts.OIDC.Sessions.Destroy(ctx)
			}
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "login required", "status": http.StatusUnauthorized, "login": "/auth/login"})
			return
		}
		if roles := requestRoles(r); roles != nil {
			out.Roles = roles
		}
	}
	if err != nil {
		s.log.Error("auth: me", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the user")
		return
	}
	if out.Accounts, err = s.opts.Identity.Accounts(ctx, out.User.ID); err != nil {
		s.log.Error("auth: me accounts", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the accounts")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// deniedMessage is the PT-BR answer when the policy refuses a user.
const deniedMessage = "Acesso negado: este usuário não tem permissão no Ironman Planner (sub fora de OIDC_ALLOWED_SUBJECTS e sem a role exigida)."

// deniedPage answers a login the policy refused. The link ends the session
// at the issuer too, so another user can log in.
func (s *Server) deniedPage(w http.ResponseWriter, idTokenHint string) {
	o := s.opts.OIDC
	other := o.Provider.EndSessionURL(idTokenHint, o.PublicBaseURL+"/")
	page := authPageData{
		Title: "Acesso negado",
		Text:  "Este usuário entrou no provedor de login, mas não tem permissão no Ironman Planner: o sub não está em OIDC_ALLOWED_SUBJECTS e o token não traz a role exigida do projeto.",
	}
	if other != "" {
		page.Link, page.LinkText = other, "Entrar com outro usuário"
	}
	s.renderAuthPage(w, http.StatusForbidden, page)
}

type authPageData struct {
	Title, Text, Link, LinkText string
}

// authPage is the bare PT-BR page of a failed login (the canvas has no login
// screen; see docs/04 section 2.5).
func (s *Server) authPage(w http.ResponseWriter, status int, title, text string, retry bool) {
	page := authPageData{Title: title, Text: text}
	if retry {
		page.Link, page.LinkText = "/auth/login", "Tentar de novo"
	}
	s.renderAuthPage(w, status, page)
}

var authPageTmpl = template.Must(template.New("auth").Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · Ironman Planner</title>
<style>body{margin:0;padding:48px 16px;background:#d9ccaa;color:#2b2114;font:15px/1.5 Inter,system-ui,sans-serif}main{max-width:560px;margin:0 auto}h1{font:700 28px/1.1 Alegreya,Georgia,serif}a{color:#6b4700}</style>
</head><body><main><h1>{{.Title}}</h1><p>{{.Text}}</p>{{if .Link}}<p><a href="{{.Link}}">{{.LinkText}}</a></p>{{end}}</main></body></html>
`))

func (s *Server) renderAuthPage(w http.ResponseWriter, status int, page authPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := authPageTmpl.Execute(w, page); err != nil {
		s.log.Warn("auth: render page", "err", err)
	}
}
