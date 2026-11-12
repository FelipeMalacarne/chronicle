package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/ironman-planner/internal/auth"
	"github.com/FelipeMalacarne/ironman-planner/internal/auth/authtest"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/events"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/identity"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

const (
	testClientID       = "planner-web"
	testNativeClientID = "planner-native"
)

// memIdentity is Identity in memory with the same claim rule as the store:
// the first OIDC user takes the dev user's accounts.
type memIdentity struct {
	mu     sync.Mutex
	users  map[string]identity.User // by id
	subs   map[string]string        // subject -> id
	owners map[string]string        // account -> user id
}

func newMemIdentity() *memIdentity {
	return &memIdentity{
		users:  map[string]identity.User{"dev": {ID: "dev", Username: "dev", Role: "owner"}},
		subs:   map[string]string{"dev": "dev"},
		owners: map[string]string{"acc-1": "dev", "acc-2": "someone-else"},
	}
}

func (m *memIdentity) LoginOIDC(_ context.Context, in identity.Login) (identity.LoginResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.subs[in.Subject]; ok {
		if id == "dev" {
			return identity.LoginResult{}, identity.ErrSubjectConflict
		}
		return identity.LoginResult{User: m.users[id]}, nil
	}
	first := len(m.subs) == 1
	u := identity.User{ID: "usr_" + in.Subject, Username: in.Username, Email: in.Email, Role: "owner"}
	m.users[u.ID], m.subs[in.Subject] = u, u.ID
	res := identity.LoginResult{User: u, Created: true}
	if first {
		for acc, owner := range m.owners {
			if owner == "dev" {
				m.owners[acc] = u.ID
				res.ClaimedAccounts++
			}
		}
	}
	return res, nil
}

func (m *memIdentity) User(_ context.Context, id string) (identity.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return identity.User{}, identity.ErrUserNotFound
	}
	return u, nil
}

func (m *memIdentity) DevUser(context.Context) (identity.User, bool, error) {
	return identity.User{ID: "dev", Username: "dev", Role: "owner"}, true, nil
}

func (m *memIdentity) Accounts(_ context.Context, userID string) ([]storeprogress.AccountView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []storeprogress.AccountView{}
	for acc, owner := range m.owners {
		if owner == userID {
			out = append(out, storeprogress.AccountView{ID: acc})
		}
	}
	return out, nil
}

func (m *memIdentity) UserBySubject(_ context.Context, _, subject string) (identity.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.subs[subject]
	if !ok || id == "dev" {
		return identity.User{}, identity.ErrUserNotFound
	}
	return m.users[id], nil
}

func (m *memIdentity) OwnsAccount(_ context.Context, userID, accountID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.owners[accountID] == userID, nil
}

// oidcEnv is a planner in AUTH_MODE=oidc against the fake issuer.
type oidcEnv struct {
	t    *testing.T
	fake *authtest.Provider
	srv  *httptest.Server
	opts *OIDCOptions
	hub  *events.Hub
	// ingest records the plugin credentials the handlers pass on.
	ingest *fakeIngestService
}

func newOIDCEnv(t *testing.T, ident Identity, store scs.Store, policy auth.Policy) *oidcEnv {
	t.Helper()
	e := &oidcEnv{t: t, fake: authtest.New(t, testClientID), hub: events.NewHub([]string{"oziris-main"}), ingest: newFakeIngestService()}
	var handler http.Handler
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(e.srv.Close)
	provider, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: e.fake.Issuer(), ClientID: testClientID, RedirectURL: e.srv.URL + "/auth/callback",
		NativeClientID: testNativeClientID,
	})
	if err != nil {
		t.Fatal(err)
	}
	sealer, err := auth.NewSealer("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	e.opts = &OIDCOptions{
		Provider: provider, Policy: policy, Sessions: NewSessionManager(store, false), Flow: sealer,
		PublicBaseURL: e.srv.URL,
	}
	g := &content.Guide{ID: "oziris-main", Title: "Oziris", Sections: []*content.Section{{ID: "s", Slug: "s", Steps: []*content.Step{{ID: "1", Text: "x"}}}}}
	g.Index()
	handler = New(Options{
		Version: "test", Content: &content.Content{Guides: []*content.Guide{g}}, Progress: &fakeProgress{},
		Accounts: fakeAccounts{}, Events: e.hub, EventsHeartbeat: time.Hour, OIDC: e.opts, Identity: ident,
		Ingest: e.ingest, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return e
}

// client is a browser: a cookie jar and no automatic redirects.
func (e *oidcEnv) client() *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *oidcEnv) get(c *http.Client, rawURL string) *http.Response {
	e.t.Helper()
	resp, err := c.Get(rawURL)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// startLogin follows /auth/login to the issuer and logs u in there; it
// returns the callback URL the issuer sends the browser to.
func (e *oidcEnv) startLogin(c *http.Client, ret string, u authtest.User) string {
	e.t.Helper()
	resp := e.get(c, e.srv.URL+"/auth/login?return="+url.QueryEscape(ret))
	if resp.StatusCode != http.StatusFound {
		e.t.Fatalf("login = %d", resp.StatusCode)
	}
	back, err := e.fake.Authorize(resp.Header.Get("Location"), u)
	if err != nil {
		e.t.Fatal(err)
	}
	return back
}

// login runs the whole flow and returns the callback response.
func (e *oidcEnv) login(c *http.Client, ret string, u authtest.User) *http.Response {
	e.t.Helper()
	return e.get(c, e.startLogin(c, ret, u))
}

func (e *oidcEnv) do(c *http.Client, method, path, body string, csrf bool) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	if csrf {
		req.Header.Set(CSRFHeader, "1")
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

var owner = authtest.User{Subject: "100", Username: "felipe", Email: "f@example.org", Roles: []string{"owner"}}

func TestOIDCLoginFlow(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		testOIDCLoginFlow(t, newMemIdentity(), memstore.New())
	})
	t.Run("postgres", func(t *testing.T) {
		pool := authTestPool(context.Background(), t)
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
			-- Not an OIDC user, so the owner's first login still claims acc-1.
			INSERT INTO users (id, subject, username) VALUES ('someone-else', 'x', 'x');
			INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
			  ('acc-1', 'dev', 'One', 'one', 'ironman', 'oziris-main', 'enhanced'),
			  ('acc-2', 'someone-else', 'Two', 'two', 'ironman', 'oziris-main', 'enhanced');
		`); err != nil {
			t.Fatal(err)
		}
		testOIDCLoginFlow(t, identity.New(pool), pgxstore.NewWithCleanupInterval(pool, 0))
	})
}

func testOIDCLoginFlow(t *testing.T, ident Identity, store scs.Store) {
	e := newOIDCEnv(t, ident, store, auth.Policy{RequiredRole: "owner"})
	c := e.client()

	// No session: account routes, /api/me and the stream answer 401.
	status, body := e.do(c, "GET", "/api/progress?account=acc-1", "", false)
	if status != http.StatusUnauthorized || body["login"] != "/auth/login" {
		t.Fatalf("anonymous progress = %d %v", status, body)
	}
	if status, _ = e.do(c, "GET", "/api/me", "", false); status != http.StatusUnauthorized {
		t.Fatalf("anonymous me = %d", status)
	}
	if resp := e.get(c, e.srv.URL+"/api/events?account=acc-1"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous events = %d", resp.StatusCode)
	}
	// Content stays public.
	if status, _ = e.do(c, "GET", "/api/guides", "", false); status != http.StatusOK {
		t.Fatalf("guides = %d", status)
	}

	// Login: the issuer URL carries PKCE S256, state and nonce (checked by
	// authtest), the flow cookie is HttpOnly and scoped to the callback.
	resp := e.get(c, e.srv.URL+"/auth/login?return=%2Frotas%3Ftab%3D1")
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), e.fake.Issuer()) {
		t.Fatalf("login = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	flow := cookieNamed(resp, flowCookie)
	if flow == nil || !flow.HttpOnly || flow.Path != "/auth/callback" || flow.SameSite != http.SameSiteLaxMode {
		t.Fatalf("flow cookie = %+v", flow)
	}
	back, err := e.fake.Authorize(resp.Header.Get("Location"), owner)
	if err != nil {
		t.Fatal(err)
	}
	resp = e.get(c, back)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/rotas?tab=1" {
		t.Fatalf("callback = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	sess := cookieNamed(resp, sessionCookie)
	if sess == nil || !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode || sess.Secure || sess.MaxAge <= 0 {
		t.Fatalf("session cookie = %+v", sess)
	}

	// /api/me: the first OIDC user claimed the dev account.
	status, body = e.do(c, "GET", "/api/me", "", false)
	if status != http.StatusOK || body["auth_mode"] != "oidc" || body["user"].(map[string]any)["username"] != "felipe" {
		t.Fatalf("me = %d %v", status, body)
	}
	if accs := body["accounts"].([]any); len(accs) != 1 || accs[0].(map[string]any)["id"] != "acc-1" {
		t.Fatalf("me accounts = %v", body["accounts"])
	}
	if roles := body["roles"].([]any); len(roles) != 1 || roles[0] != "owner" {
		t.Fatalf("me roles = %v", body["roles"])
	}

	// Own account: allowed. Another user's account and a missing one: 404.
	if status, body = e.do(c, "GET", "/api/guides/oziris-main/board?account=acc-1", "", false); status != http.StatusOK || body["account"] != "acc-1" {
		t.Fatalf("own board = %d %v", status, body)
	}
	for _, acc := range []string{"acc-2", "acc-missing"} {
		if status, _ = e.do(c, "GET", "/api/guides/oziris-main/board?account="+acc, "", false); status != http.StatusNotFound {
			t.Fatalf("board of %s = %d", acc, status)
		}
	}

	// CSRF: a write without the header is refused before the handler runs.
	mark := `{"kind":"manual_done"}`
	if status, _ = e.do(c, "PATCH", "/api/guides/oziris-main/steps/1/mark?account=acc-1", mark, false); status != http.StatusForbidden {
		t.Fatalf("mark without csrf = %d", status)
	}
	if status, _ = e.do(c, "PATCH", "/api/guides/oziris-main/steps/1/mark?account=acc-1", mark, true); status != http.StatusOK {
		t.Fatalf("mark with csrf = %d", status)
	}
	if status, _ = e.do(c, "PATCH", "/api/guides/oziris-main/steps/1/mark?account=acc-2", mark, true); status != http.StatusNotFound {
		t.Fatalf("mark other's account = %d", status)
	}

	// SSE with the session cookie.
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.srv.URL+"/api/events?account=acc-1", http.NoBody)
	stream, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("events = %d", stream.StatusCode)
	}
	sc := bufio.NewScanner(stream.Body)
	if m := readSSE(t, sc); m.Retry != "3000" {
		t.Fatalf("first message = %+v", m)
	}
	if hello := readSSE(t, sc); hello.Event != "hello" || !strings.Contains(hello.Data, `"account":"acc-1"`) {
		t.Fatalf("hello = %+v", hello)
	}
	e.hub.Publish(events.Event{Type: events.TypeProgress, Account: "acc-1", Source: events.SourceOwner})
	if got := readSSE(t, sc); got.Event != "progress" {
		t.Fatalf("event = %+v", got)
	}
	cancel()
	_ = stream.Body.Close()
	if resp = e.get(c, e.srv.URL+"/api/events?account=acc-2"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("events of another account = %d", resp.StatusCode)
	}

	// The setup works for the session user.
	if status, body = e.do(c, "POST", "/api/accounts", `{"rsn":"New Guy","mode":"hcim","guide_id":"oziris-main","layer":"enhanced"}`, true); status != http.StatusCreated {
		t.Fatalf("setup create = %d %v", status, body)
	}

	// Logout: POST with the header; RP-initiated logout at the issuer.
	if status, _ = e.do(c, "POST", "/auth/logout", "", false); status != http.StatusForbidden {
		t.Fatalf("logout without csrf = %d", status)
	}
	status, body = e.do(c, "POST", "/auth/logout", "", true)
	end, _ := url.Parse(fmt.Sprint(body["redirect"]))
	if status != http.StatusOK || !strings.HasPrefix(end.String(), e.fake.EndSessionEndpoint()) ||
		end.Query().Get("id_token_hint") == "" || end.Query().Get("post_logout_redirect_uri") != e.srv.URL+"/" {
		t.Fatalf("logout = %d %v", status, body)
	}
	if status, _ = e.do(c, "GET", "/api/me", "", false); status != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d", status)
	}
	// The old session token is gone from the store, not only from the jar.
	replay := e.client()
	u, _ := url.Parse(e.srv.URL)
	replay.Jar.SetCookies(u, []*http.Cookie{{Name: sessionCookie, Value: sess.Value}})
	if status, _ = e.do(replay, "GET", "/api/me", "", false); status != http.StatusUnauthorized {
		t.Fatalf("replayed session = %d", status)
	}
}

func cookieNamed(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestOIDCLoginDenials(t *testing.T) {
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{AllowedSubjects: []string{"100", "300"}, RequiredRole: "owner"})
	expectNoSession := func(name string, c *http.Client) {
		t.Helper()
		if status, _ := e.do(c, "GET", "/api/me", "", false); status != http.StatusUnauthorized {
			t.Errorf("%s: session opened (me = %d)", name, status)
		}
	}

	t.Run("wrong state", func(t *testing.T) {
		c := e.client()
		back, _ := url.Parse(e.startLogin(c, "/", owner))
		q := back.Query()
		q.Set("state", "forged")
		back.RawQuery = q.Encode()
		if resp := e.get(c, back.String()); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", resp.StatusCode)
		}
		expectNoSession("wrong state", c)
	})
	t.Run("callback without the login cookie", func(t *testing.T) {
		back := e.startLogin(e.client(), "/", owner)
		c := e.client() // another browser
		if resp := e.get(c, back); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", resp.StatusCode)
		}
		expectNoSession("no cookie", c)
	})
	t.Run("wrong nonce", func(t *testing.T) {
		c := e.client()
		e.fake.WrongNonce = true
		defer func() { e.fake.WrongNonce = false }()
		if resp := e.login(c, "/", owner); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", resp.StatusCode)
		}
		expectNoSession("wrong nonce", c)
	})
	t.Run("issuer error", func(t *testing.T) {
		c := e.client()
		back, _ := url.Parse(e.startLogin(c, "/", owner))
		q := back.Query()
		q.Del("code")
		q.Set("error", "access_denied")
		back.RawQuery = q.Encode()
		if resp := e.get(c, back.String()); resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})
	t.Run("subject not allowed and no role", func(t *testing.T) {
		c := e.client()
		resp := e.login(c, "/", authtest.User{Subject: "999", Roles: []string{"member"}})
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(raw), "Acesso negado") ||
			!strings.Contains(resp.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(raw), e.fake.EndSessionEndpoint()) {
			t.Errorf("denied = %d %s", resp.StatusCode, raw)
		}
		expectNoSession("denied", c)
	})
	// The policy lists subjects and requires a role: a login needs both.
	t.Run("listed subject without the role", func(t *testing.T) {
		c := e.client()
		if resp := e.login(c, "/", authtest.User{Subject: "300"}); resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d", resp.StatusCode)
		}
		expectNoSession("listed without role", c)
	})
	t.Run("role without a listed subject", func(t *testing.T) {
		c := e.client()
		if resp := e.login(c, "/", authtest.User{Subject: "400", Roles: []string{"owner"}}); resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d", resp.StatusCode)
		}
		expectNoSession("role without subject", c)
	})
	t.Run("role in the project-scoped claim", func(t *testing.T) {
		c := e.client()
		if resp := e.login(c, "/", authtest.User{Subject: "300", Roles: []string{"owner"}, ProjectID: "123456"}); resp.StatusCode != http.StatusSeeOther {
			t.Errorf("status = %d", resp.StatusCode)
		}
		// Allowed in, but still not the owner of another user's account.
		if status, _ := e.do(c, "GET", "/api/progress?account=acc-2", "", false); status != http.StatusNotFound {
			t.Errorf("other's account = %d", status)
		}
	})
	t.Run("dev subject", func(t *testing.T) {
		c := e.client()
		e.opts.Policy.AllowedSubjects = append(e.opts.Policy.AllowedSubjects, "dev")
		if resp := e.login(c, "/", authtest.User{Subject: "dev"}); resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d", resp.StatusCode)
		}
		expectNoSession("dev subject", c)
	})
	t.Run("open redirect", func(t *testing.T) {
		c := e.client()
		resp := e.login(c, "//evil.example/x", owner)
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
			t.Errorf("callback = %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
	})
	t.Run("policy re-checked on open sessions", func(t *testing.T) {
		c := e.client()
		if resp := e.login(c, "/", owner); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("login = %d", resp.StatusCode)
		}
		if status, _ := e.do(c, "GET", "/api/me", "", false); status != http.StatusOK {
			t.Fatalf("me = %d", status)
		}
		saved := e.opts.Policy
		e.opts.Policy = auth.Policy{RequiredRole: "admin"}
		defer func() { e.opts.Policy = saved }()
		if status, body := e.do(c, "GET", "/api/me", "", false); status != http.StatusForbidden || !strings.Contains(fmt.Sprint(body["error"]), "Acesso negado") {
			t.Fatalf("me after policy change = %d %v", status, body)
		}
		e.opts.Policy = saved
		expectNoSession("revoked", c) // the session was destroyed
	})
}

func TestDevModeHasNoAuthRoutes(t *testing.T) {
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: &content.Content{}, Progress: &fakeProgress{}, Identity: newMemIdentity(), DevMode: true,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	t.Cleanup(srv.Close)
	for _, p := range []string{"/auth/login", "/auth/callback"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		// The SPA fallback answers (404 without a static FS); never a redirect.
		if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
			t.Errorf("%s in dev = %d", p, resp.StatusCode)
		}
	}
	status, body := do(t, "GET", srv.URL+"/api/me", "")
	if status != http.StatusOK || body["auth_mode"] != "dev" || body["user"].(map[string]any)["id"] != "dev" {
		t.Fatalf("dev me = %d %v", status, body)
	}
	// No session, no CSRF header: dev writes keep working as before.
	if status, _ = do(t, "PATCH", srv.URL+"/api/guides/oziris-main/steps/1/mark?account=acc-1", `{"kind":"manual_done"}`); status == http.StatusUnauthorized || status == http.StatusForbidden {
		t.Fatalf("dev mark = %d", status)
	}
}

func TestOIDCRequiresIdentityAndExcludesDev(t *testing.T) {
	for name, opts := range map[string]Options{
		"no identity": {Content: &content.Content{}, OIDC: &OIDCOptions{}},
		"dev too":     {Content: &content.Content{}, OIDC: &OIDCOptions{}, Identity: newMemIdentity(), DevMode: true},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			New(opts)
		}()
	}
}

// authTestPool applies the migrations in a throwaway schema of DATABASE_URL.
func authTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL session test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("httpapi_auth_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, dropErr := admin.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE"); dropErr != nil {
			t.Errorf("drop test schema: %v", dropErr)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if pool, err = pgxpool.NewWithConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		up, _, found := strings.Cut(string(raw), "-- +goose Down")
		if !found {
			t.Fatalf("%s has no goose Down marker", f)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	return pool
}
