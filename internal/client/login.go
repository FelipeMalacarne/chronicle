package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

// Errors of the login.
var (
	// ErrDevMode: the server runs AUTH_MODE=dev and needs no login.
	ErrDevMode = errors.New("client: the server is in dev mode and needs no login")
	// ErrNoNativeClient: the server is in oidc mode without a native
	// application (OIDC_NATIVE_CLIENT_ID), so it takes no bearer tokens.
	ErrNoNativeClient = errors.New("client: the server has no native client for the device flow (OIDC_NATIVE_CLIENT_ID)")
)

// AuthClient is GET /api/auth/client: how this server wants native clients
// to log in.
type AuthClient struct {
	AuthMode string   `json:"auth_mode"`
	Issuer   string   `json:"issuer,omitempty"`
	ClientID string   `json:"client_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

// DevicePrompt is what the user must do to approve the login: open URL (or
// VerificationURI and type UserCode).
type DevicePrompt struct {
	UserCode        string
	VerificationURI string
	// URL is the verification URL with the code filled in, when the issuer
	// gives one; else VerificationURI.
	URL string
}

// Login runs the device authorization grant (RFC 8628) against the issuer
// the server at baseURL names, telling the user what to do through prompt,
// and returns the credential to store. hc talks to the server and the
// issuer (nil: http.DefaultClient).
func Login(ctx context.Context, hc *http.Client, baseURL string, prompt func(DevicePrompt)) (Credential, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	var info AuthClient
	if err := getJSON(ctx, hc, NormalizeURL(baseURL)+"/api/auth/client", &info); err != nil {
		return Credential{}, err
	}
	switch {
	case info.AuthMode == "dev":
		return Credential{}, ErrDevMode
	case info.AuthMode != "oidc":
		return Credential{}, fmt.Errorf("client: the server has no login (auth_mode %q)", info.AuthMode)
	case info.ClientID == "":
		return Credential{}, ErrNoNativeClient
	}
	var disc struct {
		Issuer        string `json:"issuer"`
		TokenURL      string `json:"token_endpoint"`
		DeviceAuthURL string `json:"device_authorization_endpoint"`
	}
	if err := getJSON(ctx, hc, strings.TrimRight(info.Issuer, "/")+"/.well-known/openid-configuration", &disc); err != nil {
		return Credential{}, err
	}
	switch {
	case disc.Issuer != info.Issuer:
		return Credential{}, fmt.Errorf("client: the issuer calls itself %q, not %q", disc.Issuer, info.Issuer)
	case disc.DeviceAuthURL == "" || disc.TokenURL == "":
		return Credential{}, fmt.Errorf("client: the issuer %s offers no device authorization", info.Issuer)
	}
	cfg := oauth2.Config{
		ClientID: info.ClientID, Scopes: info.Scopes,
		Endpoint: oauth2.Endpoint{DeviceAuthURL: disc.DeviceAuthURL, TokenURL: disc.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return Credential{}, fmt.Errorf("client: device authorization: %w", err)
	}
	p := DevicePrompt{UserCode: da.UserCode, VerificationURI: da.VerificationURI, URL: da.VerificationURIComplete}
	if p.URL == "" {
		p.URL = da.VerificationURI
	}
	prompt(p)
	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return Credential{}, fmt.Errorf("client: device login: %w", err)
	}
	return Credential{
		Issuer: info.Issuer, ClientID: info.ClientID, TokenURL: disc.TokenURL, Scopes: info.Scopes,
		AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, Expiry: tok.Expiry,
	}, nil
}

// tokenSource renews the stored access token with the refresh token and
// writes every new token back to the store, so the next run starts from it
// (the issuer rotates refresh tokens).
type tokenSource struct {
	mu    sync.Mutex
	src   oauth2.TokenSource
	store *Store
	url   string
	cred  Credential
}

func newTokenSource(ctx context.Context, hc *http.Client, store *Store, url string, cred Credential) *tokenSource {
	cfg := oauth2.Config{
		ClientID: cred.ClientID, Scopes: cred.Scopes,
		Endpoint: oauth2.Endpoint{TokenURL: cred.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	tok := &oauth2.Token{AccessToken: cred.AccessToken, RefreshToken: cred.RefreshToken, Expiry: cred.Expiry, TokenType: "Bearer"}
	return &tokenSource{src: cfg.TokenSource(ctx, tok), store: store, url: url, cred: cred}
}

func (t *tokenSource) Token() (*oauth2.Token, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tok, err := t.src.Token()
	if err != nil {
		if _, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
			return nil, fmt.Errorf("%w: %v", ErrLoginExpired, err)
		}
		return nil, err
	}
	rotated := tok.RefreshToken != "" && tok.RefreshToken != t.cred.RefreshToken
	if tok.AccessToken != t.cred.AccessToken || rotated {
		t.cred.AccessToken, t.cred.Expiry = tok.AccessToken, tok.Expiry
		if rotated {
			t.cred.RefreshToken = tok.RefreshToken
		}
		if t.store != nil {
			if err := t.store.SaveCredential(t.url, t.cred); err != nil {
				return nil, err
			}
		}
	}
	return tok, nil
}

func getJSON(ctx context.Context, hc *http.Client, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("client: GET %s: HTTP %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("client: GET %s: %w", url, err)
	}
	return nil
}
