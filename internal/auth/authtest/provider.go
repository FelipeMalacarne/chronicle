// Package authtest is an in-process OpenID Connect provider for tests: it
// serves discovery, JWKS, token, userinfo, device authorization and
// end-session endpoints, and signs ID and access tokens with a throwaway RSA
// key. The browser's visit to the authorization endpoint is simulated by
// Authorize, the user's approval of a device code by ApproveDevice.
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
	// ProfileInUserinfo moves preferred_username, email and name from the ID
	// token to userinfo (Zitadel without "User Info inside ID Token").
	ProfileInUserinfo bool
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
	// AccessTTL is the lifetime of the JWT access tokens of the device flow
	// and of the refresh grant; zero means an hour.
	AccessTTL time.Duration

	key     *rsa.PrivateKey
	mu      sync.Mutex
	codes   map[string]grant
	tokens  map[string]User
	devices map[string]*deviceGrant
	refresh map[string]refreshGrant
	// DeviceScope is the scope of the last device authorization request.
	DeviceScope string
	// Refreshes counts the refresh_token grants served.
	Refreshes int
	machines  map[string]machine
	// MachineGrants counts the client_credentials grants served.
	MachineGrants int
}

// machine is a machine user of the client_credentials grant.
type machine struct {
	secret string
	user   User
}

type deviceGrant struct {
	clientID string
	userCode string
	user     *User
}

type refreshGrant struct {
	user     User
	clientID string
}

// New starts the fake issuer; it stops with the test.
func New(t testing.TB, clientID string) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{
		ClientID: clientID, key: key, codes: map[string]grant{}, tokens: map[string]User{},
		devices: map[string]*deviceGrant{}, refresh: map[string]refreshGrant{}, machines: map[string]machine{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /keys", p.jwks)
	mux.HandleFunc("POST /oauth/v2/token", p.token)
	mux.HandleFunc("GET /oidc/v1/userinfo", p.userinfo)
	mux.HandleFunc("POST /oauth/v2/device_authorization", p.deviceAuthorization)
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
		"device_authorization_endpoint":         p.Issuer() + "/oauth/v2/device_authorization",
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
	switch r.PostForm.Get("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		p.deviceToken(w, r)
		return
	case "refresh_token":
		p.refreshToken(w, r)
		return
	case "client_credentials":
		p.clientCredentials(w, r)
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
	}
	if !g.user.ProfileInUserinfo {
		claims["preferred_username"], claims["email"], claims["name"] = g.user.Username, g.user.Email, g.user.Name
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

// AccessToken signs a JWT access token of clientID for u, as Zitadel issues
// with "Auth Token Type: JWT": iss, sub, aud, client_id, exp and the roles.
func (p *Provider) AccessToken(u User, clientID string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"iss": p.Issuer(), "sub": u.Subject, "aud": []string{clientID}, "client_id": clientID,
		"iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	}
	addRoles(claims, u)
	return p.sign(claims)
}

// AddMachine registers a machine user for the client_credentials grant: its
// client id, secret and who the tokens are for (subject, roles).
func (p *Provider) AddMachine(clientID, secret string, u User) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.machines[clientID] = machine{secret: secret, user: u}
}

// clientCredentials serves a machine user's grant like Zitadel: client id and
// secret by HTTP Basic (or the form), and a JWT access token whose audience
// has the client and, with the project audience scope, the project id.
func (p *Provider) clientCredentials(w http.ResponseWriter, r *http.Request) {
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	p.mu.Lock()
	m, known := p.machines[id]
	if known && m.secret == secret {
		p.MachineGrants++
	}
	p.mu.Unlock()
	if !known || m.secret != secret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	aud := []string{id}
	for s := range strings.FieldsSeq(r.PostForm.Get("scope")) {
		if rest, found := strings.CutPrefix(s, "urn:zitadel:iam:org:project:id:"); found {
			aud = append(aud, strings.TrimSuffix(rest, ":aud"))
		}
	}
	ttl := p.AccessTTL
	if ttl == 0 {
		ttl = time.Hour
	}
	now := time.Now()
	claims := map[string]any{
		"iss": p.Issuer(), "sub": m.user.Subject, "aud": aud, "client_id": id,
		"iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	}
	addRoles(claims, m.user)
	access, err := p.sign(claims)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(ttl.Seconds())})
}

// deviceAuthorization starts a device flow (RFC 8628) for any client id.
func (p *Provider) deviceAuthorization(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm.Get("client_id") == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	device, userCode := randomString(), strings.ToUpper(randomString()[:8])
	p.mu.Lock()
	p.devices[device] = &deviceGrant{clientID: r.PostForm.Get("client_id"), userCode: userCode}
	p.DeviceScope = r.PostForm.Get("scope")
	p.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code": device, "user_code": userCode,
		"verification_uri":          p.Issuer() + "/device",
		"verification_uri_complete": p.Issuer() + "/device?user_code=" + userCode,
		"expires_in":                600, "interval": 1,
	})
}

// ApproveDevice plays the user who enters userCode at the issuer and logs in
// as u; the client's next poll gets the tokens.
func (p *Provider) ApproveDevice(userCode string, u User) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, g := range p.devices {
		if g.userCode == userCode {
			g.user = &u
			return nil
		}
	}
	return fmt.Errorf("authtest: no device authorization with user code %q", userCode)
}

func (p *Provider) deviceToken(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	g, ok := p.devices[r.PostForm.Get("device_code")]
	switch {
	case !ok || g.clientID != r.PostForm.Get("client_id"):
		p.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	case g.user == nil:
		p.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
		return
	}
	delete(p.devices, r.PostForm.Get("device_code"))
	p.mu.Unlock()
	p.issueTokens(w, *g.user, g.clientID)
}

func (p *Provider) refreshToken(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	g, ok := p.refresh[r.PostForm.Get("refresh_token")]
	delete(p.refresh, r.PostForm.Get("refresh_token"))
	if ok && g.clientID == r.PostForm.Get("client_id") {
		p.Refreshes++
	}
	p.mu.Unlock()
	if !ok || g.clientID != r.PostForm.Get("client_id") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	p.issueTokens(w, g.user, g.clientID)
}

// issueTokens answers a device or refresh grant: a JWT access token and a
// rotated refresh token.
func (p *Provider) issueTokens(w http.ResponseWriter, u User, clientID string) {
	ttl := p.AccessTTL
	if ttl == 0 {
		ttl = time.Hour
	}
	access, err := p.AccessToken(u, clientID, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	refresh := randomString()
	p.mu.Lock()
	p.refresh[refresh] = refreshGrant{user: u, clientID: clientID}
	p.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int(ttl.Seconds()), "refresh_token": refresh,
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
	claims := map[string]any{"sub": u.Subject, "preferred_username": u.Username, "email": u.Email, "name": u.Name}
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
