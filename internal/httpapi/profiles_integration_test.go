package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/auth/authtest"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
	"github.com/FelipeMalacarne/chronicle/internal/vault"
)

// The client profile routes over the real store (dev mode): a model and a
// profile on it, the account's profile and own settings, and the refusals.
func TestClientProfileRoutesPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_farm', 'dev', 'Farm', 'farm', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	repo := storeworkers.New(pool, testVaultKey(t), nil)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: &agent.Link{Log: quiet}, Audit: &memAudit{}, Sessions: repo, Profiles: repo},
	}))
	t.Cleanup(srv.Close)
	api := srv.URL + "/api"

	model := `{"plugins":[{"source":"core","name":"net.runelite.client.plugins.entityhider.EntityHiderPlugin","start":true}],
		"settings":[{"group":"entityhider","key":"hidePlayers","value":"true"}]}`
	if status, body, _ := call(t, "PUT", api+"/client-profiles/farm-base", model); status != http.StatusOK || body["name"] != "farm-base" {
		t.Fatalf("put model = %d %v", status, body)
	}
	profile := `{"parent":"farm-base","plugins":[{"source":"own","name":"a.BlackjackerPlugin","start":true}],
		"settings":[{"group":"blackjacker","key":"room","value":"THUG_TENT"}]}`
	status, body, _ := call(t, "PUT", api+"/client-profiles/blackjack", profile)
	if status != http.StatusOK || body["parent"] != "farm-base" || len(body["plugins"].([]any)) != 1 {
		t.Fatalf("put profile = %d %v", status, body)
	}
	for name, c := range map[string]struct {
		method, path, body string
		want               int
	}{
		"protected group":   {"PUT", "/client-profiles/x", `{"settings":[{"group":"agentServer","key":"port","value":"1"}]}`, http.StatusBadRequest},
		"unknown field":     {"PUT", "/client-profiles/x", `{"plugin":[]}`, http.StatusBadRequest},
		"bad name":          {"PUT", "/client-profiles/Farm%20Base", `{}`, http.StatusBadRequest},
		"unknown model":     {"PUT", "/client-profiles/x", `{"parent":"nope"}`, http.StatusNotFound},
		"unknown profile":   {"GET", "/client-profiles/nope", ``, http.StatusNotFound},
		"model in use":      {"DELETE", "/client-profiles/farm-base", ``, http.StatusConflict},
		"no account":        {"GET", "/accounts/client", ``, http.StatusBadRequest},
		"unknown account":   {"GET", "/accounts/client?account=acc_nope", ``, http.StatusNotFound},
		"account, no model": {"PUT", "/accounts/client?account=acc_farm", `{"profile":"nope"}`, http.StatusNotFound},
		"secret override":   {"PUT", "/accounts/client?account=acc_farm", `{"settings":[{"group":"g","key":"password","value":"x"}]}`, http.StatusBadRequest},
	} {
		if got, answer, _ := call(t, c.method, api+c.path, c.body); got != c.want {
			t.Errorf("%s = %d %v, want %d", name, got, answer, c.want)
		}
	}

	status, body, _ = call(t, "PUT", api+"/accounts/client?account=acc_farm",
		`{"profile":"blackjack","settings":[{"group":"entityhider","key":"hidePlayers","value":"false"}]}`)
	if status != http.StatusOK || body["profile"] != "blackjack" {
		t.Fatalf("put account client = %d %v", status, body)
	}
	eff := body["effective"].(map[string]any)
	if plugins := eff["plugins"].([]any); len(plugins) != 2 || plugins[0].(map[string]any)["name"] != "net.runelite.client.plugins.entityhider.EntityHiderPlugin" {
		t.Fatalf("effective plugins = %v", plugins)
	}
	if settings := eff["settings"].([]any); len(settings) != 2 || settings[0].(map[string]any)["value"] != "false" {
		t.Fatalf("effective settings = %v", settings)
	}
	if got, answer, _ := call(t, "GET", api+"/accounts/client?account=acc_farm", ""); got != http.StatusOK || answer["profile"] != "blackjack" {
		t.Fatalf("get account client = %d %v", got, answer)
	}
	status, body, _ = call(t, "GET", api+"/client-profiles", "")
	if status != http.StatusOK || len(body["profiles"].([]any)) != 2 {
		t.Fatalf("list = %d %v", status, body)
	}
	for _, p := range body["profiles"].([]any) {
		if p := p.(map[string]any); p["name"] == "blackjack" && len(p["accounts"].([]any)) != 1 {
			t.Fatalf("listed blackjack = %v", p)
		}
	}
	if status, _, _ = call(t, "DELETE", api+"/client-profiles/blackjack", ""); status != http.StatusConflict {
		t.Fatalf("delete an account's profile = %d", status)
	}
	if status, _, _ = call(t, "PUT", api+"/accounts/client?account=acc_farm", `{}`); status != http.StatusOK {
		t.Fatalf("clear account client = %d", status)
	}
	for _, name := range []string{"blackjack", "farm-base"} {
		if status, _, _ = call(t, "DELETE", api+"/client-profiles/"+name, ""); status != http.StatusOK {
			t.Fatalf("delete %s = %d", name, status)
		}
	}
}

// testVaultKey is a fixed CHRONICLE_VAULT_KEY for the tests.
func testVaultKey(t *testing.T) vault.Key {
	t.Helper()
	key, err := vault.ParseKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// Without a profile store the routes do not exist.
func TestClientProfileRoutesOff(t *testing.T) {
	srv, _, _, _ := workersServer(t, nil)
	if status, _, _ := call(t, "GET", srv.URL+"/api/client-profiles", ""); status != http.StatusNotFound {
		t.Fatalf("profiles without a store = %d", status)
	}
}

// memProfiles records the user and account each call got.
type memProfiles struct{ users, accounts []string }

func (m *memProfiles) ListProfiles(_ context.Context, userID string) ([]storeworkers.Profile, error) {
	m.users = append(m.users, userID)
	return []storeworkers.Profile{}, nil
}

func (m *memProfiles) GetProfile(_ context.Context, userID, name string) (storeworkers.Profile, error) {
	m.users = append(m.users, userID)
	return storeworkers.Profile{Name: name}, nil
}

func (m *memProfiles) PutProfile(_ context.Context, userID, name, parent string, _ agent.ClientProfile) (storeworkers.Profile, error) {
	m.users = append(m.users, userID)
	return storeworkers.Profile{Name: name, Parent: parent}, nil
}

func (m *memProfiles) DeleteProfile(_ context.Context, userID, _ string) error {
	m.users = append(m.users, userID)
	return nil
}

func (m *memProfiles) AccountClient(_ context.Context, account string) (storeworkers.AccountClient, error) {
	m.accounts = append(m.accounts, account)
	return storeworkers.AccountClient{AccountID: account}, nil
}

func (m *memProfiles) SetAccountClient(_ context.Context, account, profile string, _ []agent.ProfileSetting) (storeworkers.AccountClient, error) {
	m.accounts = append(m.accounts, account)
	return storeworkers.AccountClient{AccountID: account, Profile: profile}, nil
}

// In oidc the profile routes need a session and the Microbot role, the
// profiles are the session user's, and an account route takes only the
// user's own accounts.
func TestClientProfileRoutesOIDC(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	profiles := &memProfiles{}
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) {
		o.Microbot = &MicrobotOptions{Link: &agent.Link{Log: quiet}, Audit: &memAudit{}, Profiles: profiles}
	})
	c := e.client()
	if status, _ := e.do(c, "GET", "/api/client-profiles", "", false); status != http.StatusUnauthorized {
		t.Fatalf("profiles without a session = %d", status)
	}
	e.login(c, "/", authtest.User{Subject: "100", Username: "felipe", Roles: []string{"owner", MicrobotRole}})
	if status, _ := e.do(c, "GET", "/api/client-profiles", "", false); status != http.StatusOK {
		t.Fatalf("profiles = %d", status)
	}
	if status, _ := e.do(c, "PUT", "/api/client-profiles/farm", `{}`, true); status != http.StatusOK {
		t.Fatalf("put profile = %d", status)
	}
	for _, u := range profiles.users {
		if u == "" || u == "dev" {
			t.Fatalf("profiles of %q, not the session's user (%v)", u, profiles.users)
		}
	}
	if status, _ := e.do(c, "GET", "/api/accounts/client?account=acc-1", "", false); status != http.StatusOK {
		t.Fatalf("own account = %d", status)
	}
	for _, acc := range []string{"acc-2", "acc-missing"} {
		if status, _ := e.do(c, "PUT", "/api/accounts/client?account="+acc, `{"profile":"farm"}`, true); status != http.StatusNotFound {
			t.Fatalf("account client of %s = %d", acc, status)
		}
	}
	if len(profiles.accounts) != 1 || profiles.accounts[0] != "acc-1" {
		t.Fatalf("store got accounts %v", profiles.accounts)
	}
}
