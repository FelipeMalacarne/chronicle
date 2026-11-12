package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/auth/authtest"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/store/identity"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
)

// bearer sends one request with Authorization and no cookie.
func (e *oidcEnv) bearer(method, path, authorization, body string) (int, map[string]any, http.Header) {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Authorization", authorization)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp.Header
}

func (e *oidcEnv) accessToken(u authtest.User, clientID string, ttl time.Duration) string {
	e.t.Helper()
	tok, err := e.fake.AccessToken(u, clientID, ttl)
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

func TestOIDCBearerToken(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		testOIDCBearerToken(t, newMemIdentity(), memstore.New())
	})
	t.Run("postgres", func(t *testing.T) {
		pool := authTestPool(context.Background(), t)
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
			INSERT INTO users (id, subject, username) VALUES ('someone-else', 'x', 'x');
			INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
			  ('acc-1', 'dev', 'One', 'one', 'ironman', 'oziris-main', 'enhanced'),
			  ('acc-2', 'someone-else', 'Two', 'two', 'ironman', 'oziris-main', 'enhanced');
		`); err != nil {
			t.Fatal(err)
		}
		testOIDCBearerToken(t, identity.New(pool), pgxstore.NewWithCleanupInterval(pool, 0))
	})
}

func testOIDCBearerToken(t *testing.T, ident Identity, store scs.Store) {
	e := newOIDCEnv(t, ident, store, auth.Policy{RequiredRole: "owner"})
	valid := "Bearer " + e.accessToken(owner, testNativeClientID, time.Hour)

	// Discovery tells a native client how to log in.
	c := e.client()
	status, body := e.do(c, "GET", "/api/auth/client", "", false)
	scopes, _ := body["scopes"].([]any)
	if status != http.StatusOK || body["auth_mode"] != "oidc" || body["issuer"] != e.fake.Issuer() ||
		body["client_id"] != testNativeClientID || !slices.Contains(scopes, any("offline_access")) {
		t.Fatalf("auth client = %d %v", status, body)
	}

	// A token never creates a user: the first login is the web's.
	if status, body, _ = e.bearer("GET", "/api/progress?account=acc-1", valid, ""); status != http.StatusForbidden {
		t.Fatalf("bearer before any web login = %d %v", status, body)
	}
	if resp := e.login(c, "/", owner); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("web login = %d", resp.StatusCode)
	}

	// The token now reads the user's account and writes without the CSRF
	// header; another user's account is the same 404 as with the session.
	if status, body, _ = e.bearer("GET", "/api/progress?account=acc-1", valid, ""); status != http.StatusOK {
		t.Fatalf("bearer progress = %d %v", status, body)
	}
	status, body, _ = e.bearer("GET", "/api/me", valid, "")
	if roles, _ := body["roles"].([]any); status != http.StatusOK || !slices.Contains(roles, any("owner")) {
		t.Fatalf("bearer me = %d %v", status, body)
	}
	if status, body, _ = e.bearer("PATCH", "/api/guides/oziris-main/steps/1/mark?account=acc-1", valid, `{"kind":"manual_done"}`); status != http.StatusOK {
		t.Fatalf("bearer mark without CSRF = %d %v", status, body)
	}
	if status, _, _ = e.bearer("GET", "/api/progress?account=acc-2", valid, ""); status != http.StatusNotFound {
		t.Fatalf("bearer on another user's account = %d", status)
	}

	// Refusals: 401 with a challenge for a bad token, 403 for the policy.
	other := e.accessToken(owner, "some-other-app", time.Hour)
	expired := e.accessToken(owner, testNativeClientID, -time.Minute)
	for name, authz := range map[string]string{
		"tampered":       valid[:len(valid)-4] + "AAAA",
		"expired":        "Bearer " + expired,
		"other audience": "Bearer " + other,
		"not a JWT":      "Bearer opaque-token",
		"basic":          "Basic dXNlcjpwYXNz",
		"empty bearer":   "Bearer ",
	} {
		got, body, hdr := e.bearer("GET", "/api/progress?account=acc-1", authz, "")
		if got != http.StatusUnauthorized || !strings.HasPrefix(hdr.Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("%s token = %d %v", name, got, body)
		}
	}
	noRole := authtest.User{Subject: owner.Subject, Username: owner.Username}
	if status, _, _ = e.bearer("GET", "/api/progress?account=acc-1", "Bearer "+e.accessToken(noRole, testNativeClientID, time.Hour), ""); status != http.StatusForbidden {
		t.Fatalf("token without the role = %d", status)
	}

	// A bad token never falls back to a valid session cookie.
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/progress?account=acc-1", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token with a session cookie = %d", resp.StatusCode)
	}
}

// Without OIDC_NATIVE_CLIENT_ID the server takes no bearer tokens and the
// discovery says so by leaving out the client.
func TestOIDCBearerOff(t *testing.T) {
	fake := authtest.New(t, testClientID)
	provider, err := auth.NewProvider(context.Background(), auth.Config{Issuer: fake.Issuer(), ClientID: testClientID, RedirectURL: "http://planner/auth/callback"})
	if err != nil {
		t.Fatal(err)
	}
	sealer, _ := auth.NewSealer("0123456789abcdef0123456789abcdef")
	srv := httptest.NewServer(New(Options{
		Version: "test", Progress: &fakeProgress{}, Identity: newMemIdentity(),
		OIDC:   &OIDCOptions{Provider: provider, Policy: auth.Policy{RequiredRole: "owner"}, Sessions: NewSessionManager(memstore.New(), false), Flow: sealer},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer srv.Close()
	e := &oidcEnv{t: t, fake: fake, srv: srv}
	status, body := e.do(http.DefaultClient, "GET", "/api/auth/client", "", false)
	if status != http.StatusOK || body["auth_mode"] != "oidc" || body["client_id"] != nil {
		t.Fatalf("auth client = %d %v", status, body)
	}
	tok := e.accessToken(owner, testNativeClientID, time.Hour)
	if status, body, _ = e.bearer("GET", "/api/progress?account=acc-1", "Bearer "+tok, ""); status != http.StatusUnauthorized {
		t.Fatalf("bearer with login off = %d %v", status, body)
	}
}

func TestAuthClientDev(t *testing.T) {
	srv := httptest.NewServer(New(Options{Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/auth/client")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusOK || body["auth_mode"] != "dev" || len(body) != 1 {
		t.Fatalf("dev auth client = %d %v", resp.StatusCode, body)
	}
}

// In oidc the plugin signs in through the same device flow: its JWT acts for
// the installation as that user, with the scopes of the user's project roles
// (docs/04 section 2.5). ipl_ tokens keep their own path.
func TestOIDCPluginLogin(t *testing.T) {
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"})
	plugin := authtest.User{Subject: owner.Subject, Username: owner.Username, Roles: []string{"owner", "ingest", "items:bank", "marks:write", "other"}}
	jwt := e.accessToken(plugin, testNativeClientID, time.Hour)
	manifest := func(authorization, installation string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest("GET", e.srv.URL+"/api/ingest/manifest", nil)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		if installation != "" {
			req.Header.Set(HeaderInstallation, installation)
		}
		req.Header.Set(HeaderAccount, "acc-1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	last := func() storeingest.Credential {
		e.ingest.mu.Lock()
		defer e.ingest.mu.Unlock()
		if len(e.ingest.creds) == 0 {
			return storeingest.Credential{}
		}
		return e.ingest.creds[len(e.ingest.creds)-1]
	}

	if status, body := manifest("", testInstallation); status != http.StatusUnauthorized {
		t.Fatalf("no token in oidc = %d %v", status, body)
	}
	if status, body := manifest("Bearer "+jwt, testInstallation); status != http.StatusForbidden {
		t.Fatalf("plugin before any web login = %d %v", status, body)
	}
	if resp := e.login(e.client(), "/", owner); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("web login = %d", resp.StatusCode)
	}
	if status, body := manifest("Bearer "+jwt, ""); status != http.StatusBadRequest {
		t.Fatalf("login without the installation id = %d %v", status, body)
	}
	if status, body := manifest("Bearer "+jwt, testInstallation); status != http.StatusOK {
		t.Fatalf("plugin login manifest = %d %v", status, body)
	}
	got := last()
	want := []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeMarksWrite, coreingest.ScopeStateWrite}
	if got.Token != "" || got.User != "usr_"+owner.Subject || got.Installation != testInstallation || got.Account != "acc-1" || !slices.Equal(got.Scopes, want) {
		t.Fatalf("plugin login credential = %+v", got)
	}

	// Without the ingest role nothing is pushed; a bad JWT is 401.
	noIngest := e.accessToken(authtest.User{Subject: owner.Subject, Roles: []string{"owner", "items:bank"}}, testNativeClientID, time.Hour)
	if status, body := manifest("Bearer "+noIngest, testInstallation); status != http.StatusForbidden {
		t.Fatalf("login without the ingest role = %d %v", status, body)
	}
	expired := e.accessToken(plugin, testNativeClientID, -time.Minute)
	if status, body := manifest("Bearer "+expired, testInstallation); status != http.StatusUnauthorized {
		t.Fatalf("expired plugin login = %d %v", status, body)
	}

	// An ipl_ token is still a token, never verified as a JWT.
	if status, body := manifest("Bearer ipl_whatever", ""); status != http.StatusUnauthorized {
		t.Fatalf("unknown ipl_ token = %d %v", status, body)
	}
	if got = last(); got.Token != "ipl_whatever" || got.User != "" {
		t.Fatalf("ipl_ credential = %+v", got)
	}
}
