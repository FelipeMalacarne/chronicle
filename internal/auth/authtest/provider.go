// Package authtest is an in-process OpenID Connect provider for tests: it
// serves discovery, JWKS, token, userinfo and end-session endpoints, and signs
// ID tokens with a throwaway RSA key. The browser's visit to the authorization
// endpoint is simulated by Authorize.
package authtest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const keyID = "test-key"

// User is who logs in at the fake issuer.
type User struct {
	Subject  string
	Username string
	Email    string
	Name     string
	// Roles go in the urn:zitadel:iam:org:project:roles claim, or in the
	// project-scoped claim when ProjectID is set.
	Roles     []string
	ProjectID string
	// RolesInUserinfo moves the roles from the ID token to userinfo.
	RolesInUserinfo bool
}

type grant struct {
	user      User
	nonce     string
	challenge string
	redirect  string
	clientID  string
}

// Provider is the fake issuer.
type Provider struct {
	Server   *httptest.Server
	ClientID string
	// WrongNonce makes the next ID tokens carry a nonce other than the one
	// the login sent.
	WrongNonce bool

	key    *rsa.PrivateKey
	mu     sync.Mutex
	codes  map[string]grant
	tokens map[string]User
}

// New starts the fake issuer; it stops with the test.
func New(t testing.TB, clientID string) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{ClientID: clientID, key: key, codes: map[string]grant{}, tokens: map[string]User{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /keys", p.jwks)
	mux.HandleFunc("POST /oauth/v2/token", p.token)
	mux.HandleFunc("GET /oidc/v1/userinfo", p.userinfo)
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Server.Close)
	return p
}

// Issuer is the issuer URL.
func (p *Provider) Issuer() string { return p.Server.URL }

// EndSessionEndpoint is the advertised RP-initiated logout endpoint.
func (p *Provider) EndSessionEndpoint() string { return p.Server.URL + "/oidc/v1/end_session" }

func (p *Provider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.Issuer(),
		"authorization_endpoint":                p.Issuer() + "/oauth/v2/authorize",
		"token_endpoint":                        p.Issuer() + "/oauth/v2/token",
		"userinfo_endpoint":                     p.Issuer() + "/oidc/v1/userinfo",
		"jwks_uri":                              p.Issuer() + "/keys",
		"end_session_endpoint":                  p.EndSessionEndpoint(),
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (p *Provider) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := p.key.PublicKey
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": keyID,
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// Authorize plays the browser at the authorization endpoint: it reads the
// login's parameters from authURL, "logs in" u and returns the redirect back
// to the application (redirect_uri?code=&state=).
func (p *Provider) Authorize(authURL string, u User) (string, error) {
	parsed, err := url.Parse(authURL)
	if err != nil {
		return "", err
	}
	q := parsed.Query()
	switch {
	case !strings.HasPrefix(authURL, p.Issuer()+"/oauth/v2/authorize"):
		return "", fmt.Errorf("authtest: not this issuer's authorize URL: %s", authURL)
	case q.Get("response_type") != "code":
		return "", fmt.Errorf("authtest: response_type = %q", q.Get("response_type"))
	case q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "":
		return "", fmt.Errorf("authtest: missing S256 PKCE challenge")
	case q.Get("state") == "" || q.Get("nonce") == "":
		return "", fmt.Errorf("authtest: missing state or nonce")
	case q.Get("client_id") != p.ClientID:
		return "", fmt.Errorf("authtest: client_id = %q", q.Get("client_id"))
	case !strings.Contains(" "+q.Get("scope")+" ", " openid "):
		return "", fmt.Errorf("authtest: scope = %q", q.Get("scope"))
	}
	code := randomString()
	p.mu.Lock()
	p.codes[code] = grant{user: u, nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), clientID: q.Get("client_id")}
	p.mu.Unlock()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		return "", err
	}
	bq := back.Query()
	bq.Set("code", code)
	bq.Set("state", q.Get("state"))
	back.RawQuery = bq.Encode()
	return back.String(), nil
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	p.mu.Lock()
	g, ok := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code"))
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case !ok:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	case r.PostForm.Get("grant_type") != "authorization_code",
		r.PostForm.Get("client_id") != g.clientID,
		r.PostForm.Get("redirect_uri") != g.redirect,
		b64(sum[:]) != g.challenge:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "pkce or client mismatch"})
		return
	}
	nonce := g.nonce
	if p.WrongNonce {
		nonce = "not-the-login-nonce"
	}
	now := time.Now()
	claims := map[string]any{
		"iss": p.Issuer(), "sub": g.user.Subject, "aud": []string{g.clientID}, "azp": g.clientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "nonce": nonce,
		"preferred_username": g.user.Username, "email": g.user.Email, "name": g.user.Name,
	}
	if !g.user.RolesInUserinfo {
		addRoles(claims, g.user)
	}
	idToken, err := p.sign(claims)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	access := randomString()
	p.mu.Lock()
	p.tokens[access] = g.user
	p.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": 3600, "id_token": idToken,
	})
}

func (p *Provider) userinfo(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	u, ok := p.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	p.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}
	claims := map[string]any{"sub": u.Subject, "preferred_username": u.Username}
	addRoles(claims, u)
	writeJSON(w, http.StatusOK, claims)
}

func addRoles(claims map[string]any, u User) {
	if len(u.Roles) == 0 {
		return
	}
	roles := map[string]any{}
	for _, r := range u.Roles {
		roles[r] = map[string]string{"org-1": "example.org"}
	}
	name := "urn:zitadel:iam:org:project:roles"
	if u.ProjectID != "" {
		name = "urn:zitadel:iam:org:project:" + u.ProjectID + ":roles"
	}
	claims[name] = roles
}

// sign builds an RS256 JWS in compact form.
func (p *Provider) sign(claims map[string]any) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := b64(header) + "." + b64(payload)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64(sig), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func randomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return b64(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
