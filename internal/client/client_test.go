package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/auth/authtest"
)

const nativeClient = "planner-native"

var owner = authtest.User{Subject: "100", Username: "felipe", Roles: []string{"owner"}}

// fakeAPI is a planner in oidc mode: /api/auth/client names the fake issuer
// and /api/me checks the bearer with the provider the server uses.
type fakeAPI struct {
	srv      *httptest.Server
	issuer   *authtest.Provider
	accounts []Account
	mode     string
	noNative bool
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{issuer: authtest.New(t, "planner-web"), mode: "oidc", accounts: []Account{{ID: "acc_one", RSN: "One", GuideID: "oziris-main"}}}
	provider, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: f.issuer.Issuer(), ClientID: "planner-web", RedirectURL: "http://planner/auth/callback", NativeClientID: nativeClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/client", func(w http.ResponseWriter, _ *http.Request) {
		out := AuthClient{AuthMode: f.mode}
		if f.mode == "oidc" {
			out.Issuer = f.issuer.Issuer()
			if !f.noNative {
				out.ClientID, out.Scopes = nativeClient, provider.DeviceScopes()
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if f.mode == "oidc" {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "login required"})
				return
			}
			if _, err := provider.VerifyAccessToken(r.Context(), raw); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid or expired token"})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(Me{AuthMode: f.mode, User: User{ID: "usr_100"}, Accounts: f.accounts})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// login runs the device flow, approving the code as u when prompted.
func (f *fakeAPI) login(t *testing.T, u authtest.User) Credential {
	t.Helper()
	var prompted DevicePrompt
	cred, err := Login(context.Background(), nil, f.srv.URL+"/", func(p DevicePrompt) {
		prompted = p
		if err := f.issuer.ApproveDevice(p.UserCode, u); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if prompted.UserCode == "" || !strings.Contains(prompted.URL, prompted.UserCode) {
		t.Fatalf("prompt = %+v", prompted)
	}
	return cred
}

func TestDeviceLoginAndRefresh(t *testing.T) {
	f := newFakeAPI(t)
	f.issuer.AccessTTL = 5 * time.Second // inside oauth2's expiry margin: the first use refreshes
	cred := f.login(t, owner)
	if cred.RefreshToken == "" || cred.ClientID != nativeClient || cred.Issuer != f.issuer.Issuer() {
		t.Fatalf("credential = %+v", cred)
	}
	for _, scope := range []string{"openid", "offline_access", "urn:zitadel:iam:org:projects:roles"} {
		if !strings.Contains(" "+f.issuer.DeviceScope+" ", " "+scope+" ") {
			t.Errorf("device scope %q lacks %s", f.issuer.DeviceScope, scope)
		}
	}

	store := &Store{Dir: t.TempDir()}
	if err := store.SaveCredential(f.srv.URL+"/", cred); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.Dir, "credentials.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials file = %v %v", info, err)
	}

	c, err := New(context.Background(), Options{URL: f.srv.URL, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := c.Account(context.Background())
	if err != nil || acc != "acc_one" {
		t.Fatalf("account = %q %v", acc, err)
	}
	if f.issuer.Refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1", f.issuer.Refreshes)
	}
	// The renewed tokens are stored for the next run.
	saved, _, err := store.Credential(f.srv.URL)
	if err != nil || saved.RefreshToken == cred.RefreshToken || !saved.Expiry.After(cred.Expiry.Add(-time.Second)) {
		t.Fatalf("stored credential not renewed: %v", err)
	}
	if ok, err := store.DeleteCredential(f.srv.URL); !ok || err != nil {
		t.Fatalf("delete = %v %v", ok, err)
	}
}

func TestLoginRefusals(t *testing.T) {
	f := newFakeAPI(t)
	f.mode = "dev"
	if _, err := Login(context.Background(), nil, f.srv.URL, func(DevicePrompt) {}); !errors.Is(err, ErrDevMode) {
		t.Fatalf("dev login = %v", err)
	}
	f.mode, f.noNative = "oidc", true
	if _, err := Login(context.Background(), nil, f.srv.URL, func(DevicePrompt) {}); !errors.Is(err, ErrNoNativeClient) {
		t.Fatalf("login without native client = %v", err)
	}
}

func TestAccountChoice(t *testing.T) {
	f := newFakeAPI(t)
	ctx := context.Background()

	// Without a login an oidc server answers 401: the user must log in.
	c, _ := New(ctx, Options{URL: f.srv.URL})
	if _, err := c.Account(ctx); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("anonymous = %v", err)
	}

	f.mode = "dev"
	f.accounts = []Account{{ID: "acc_a"}, {ID: "acc_b"}}
	var many *ManyAccountsError
	if _, err := c.Account(ctx); !errors.As(err, &many) || len(many.Accounts) != 2 {
		t.Fatalf("two accounts = %v", err)
	}
	c, _ = New(ctx, Options{URL: f.srv.URL, Account: "acc_b"})
	if acc, err := c.Account(ctx); acc != "acc_b" || err != nil {
		t.Fatalf("chosen account = %q %v", acc, err)
	}
	f.accounts = nil
	c, _ = New(ctx, Options{URL: f.srv.URL})
	if _, err := c.Account(ctx); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("no account = %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "nested")}
	if s, err := store.Settings(); err != nil || s != (Settings{}) {
		t.Fatalf("missing settings = %+v %v", s, err)
	}
	if err := store.SaveSettings(Settings{URL: "https://planner.example/", Account: "acc_x"}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Settings()
	if err != nil || s.URL != "https://planner.example" || s.Account != "acc_x" {
		t.Fatalf("settings = %+v %v", s, err)
	}
}
