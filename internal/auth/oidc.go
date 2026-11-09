package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Errors of the callback, mapped by the transport to a 400 or 403 page.
var (
	ErrNoIDToken = errors.New("auth: token response has no id_token")
	ErrNonce     = errors.New("auth: ID token nonce does not match the login")
	// ErrNoNativeClient: bearer login is off (no native application).
	ErrNoNativeClient = errors.New("auth: no native client configured")
	// ErrNoProject: machine tokens need the project id (their audience).
	ErrNoProject = errors.New("auth: no project id configured")
)

// Config is the client registration at the issuer: a public Web application
// with PKCE and no client secret.
type Config struct {
	Issuer      string
	ClientID    string
	RedirectURL string
	// ProjectID pins the project-scoped role claim to one Zitadel project;
	// empty accepts any project id.
	ProjectID string
	// NativeClientID is the public native application of the device flow
	// (CLI, MCP, plugin). Its JWT access tokens must carry it in `aud`;
	// empty turns bearer login off.
	NativeClientID string
	// HTTPClient talks to the issuer (discovery, JWKS, token, userinfo); nil
	// uses http.DefaultClient.
	HTTPClient *http.Client
}

// Identity is what a verified login says about the user.
type Identity struct {
	Issuer      string
	Subject     string
	Username    string
	Email       string
	DisplayName string
	// Roles are the Zitadel project role keys of the ID token (or of the
	// userinfo response when the ID token has none).
	Roles []string
}

// Provider runs the authorization code flow against one issuer.
type Provider struct {
	provider   *oidc.Provider
	oauth      oauth2.Config
	verifier   *oidc.IDTokenVerifier
	client     *http.Client
	issuer     string
	endSession string
	projectID  string
	native     string
	access     *oidc.IDTokenVerifier
	machine    *oidc.IDTokenVerifier
}

// NewProvider discovers the issuer (/.well-known/openid-configuration). The
// provider keeps using cfg.HTTPClient for the JWKS and token calls, so ctx
// only bounds the discovery.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, client), cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: discovery of %s: %w", cfg.Issuer, err)
	}
	var extra struct {
		EndSession string `json:"end_session_endpoint"`
	}
	if err := p.Claims(&extra); err != nil {
		return nil, fmt.Errorf("auth: discovery document: %w", err)
	}
	endpoint := p.Endpoint()
	// Public client: client_id goes in the token request body, no secret.
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	var access *oidc.IDTokenVerifier
	if cfg.NativeClientID != "" {
		// A JWT access token has the shape go-oidc verifies for ID tokens:
		// signature by the JWKS, iss, aud (the native client) and exp.
		access = p.Verifier(&oidc.Config{ClientID: cfg.NativeClientID})
	}
	var machine *oidc.IDTokenVerifier
	if cfg.ProjectID != "" {
		// A machine user's token (client_credentials) asked with the
		// project's audience scope carries the project id in `aud`.
		machine = p.Verifier(&oidc.Config{ClientID: cfg.ProjectID})
	}
	return &Provider{
		provider: p,
		oauth: oauth2.Config{
			ClientID: cfg.ClientID, Endpoint: endpoint, RedirectURL: cfg.RedirectURL,
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier:   p.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		client:     client,
		issuer:     cfg.Issuer,
		endSession: extra.EndSession,
		projectID:  cfg.ProjectID,
		native:     cfg.NativeClientID,
		access:     access,
		machine:    machine,
	}, nil
}

// NativeClientID is the public native application of the device flow, ""
// when bearer login is off.
func (p *Provider) NativeClientID() string { return p.native }

// DeviceScopes are the scopes a native client asks for: the refresh token
// (offline_access), the project roles in the tokens and, with a project id,
// the project in the access token's audience.
func (p *Provider) DeviceScopes() []string {
	scopes := []string{oidc.ScopeOpenID, "profile", "email", oidc.ScopeOfflineAccess, "urn:zitadel:iam:org:projects:roles"}
	if p.projectID != "" {
		scopes = append(scopes, "urn:zitadel:iam:org:project:id:"+p.projectID+":aud")
	}
	return scopes
}

// VerifyAccessToken checks a JWT access token of the native client (the
// bearer of the CLI, MCP and plugin) without calling the issuer: signature by
// the cached JWKS, iss, aud and exp. The identity carries the issuer, the
// subject and the project roles of the token; Zitadel puts the roles there
// with "Assert Roles on Authentication" on the project.
func (p *Provider) VerifyAccessToken(ctx context.Context, raw string) (Identity, error) {
	if p.access == nil {
		return Identity{}, ErrNoNativeClient
	}
	tok, err := p.access.Verify(oidc.ClientContext(ctx, p.client), raw)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: verify access token: %w", err)
	}
	var claims map[string]json.RawMessage
	if err := tok.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("auth: access token claims: %w", err)
	}
	return Identity{Issuer: tok.Issuer, Subject: tok.Subject, Roles: RolesFromClaims(claims, p.projectID)}, nil
}

// MachineScopes are the scopes a machine user (the Microbot agent) asks for
// in the client_credentials grant: the project roles in the token and the
// project in its audience. Nil without a project id.
func (p *Provider) MachineScopes() []string {
	if p.projectID == "" {
		return nil
	}
	return []string{oidc.ScopeOpenID, "urn:zitadel:iam:org:projects:roles", "urn:zitadel:iam:org:project:id:" + p.projectID + ":aud"}
}

// VerifyMachineToken checks a JWT access token whose audience is the project
// (a machine user's, docs/08): signature, iss, aud and exp, offline like
// VerifyAccessToken. The caller decides by the roles; a user's device-flow
// token also has the project in its audience, so a role only machine users
// hold is what tells them apart.
func (p *Provider) VerifyMachineToken(ctx context.Context, raw string) (Identity, error) {
	if p.machine == nil {
		return Identity{}, ErrNoProject
	}
	tok, err := p.machine.Verify(oidc.ClientContext(ctx, p.client), raw)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: verify machine token: %w", err)
	}
	var claims map[string]json.RawMessage
	if err := tok.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("auth: machine token claims: %w", err)
	}
	return Identity{Issuer: tok.Issuer, Subject: tok.Subject, Roles: RolesFromClaims(claims, p.projectID)}, nil
}

// Issuer is the configured issuer URL (the `iss` every ID token must carry).
func (p *Provider) Issuer() string { return p.issuer }

// AuthCodeURL is the issuer's authorization URL for one login: state, nonce
// and the S256 challenge of verifier.
func (p *Provider) AuthCodeURL(state, nonce, verifier string) string {
	return p.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

// Exchange trades the code for tokens, verifies the ID token (signature by
// the issuer's JWKS, iss, aud, exp) and its nonce, and returns the identity
// and the raw ID token (kept for the logout hint).
func (p *Provider) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, string, error) {
	ctx = oidc.ClientContext(ctx, p.client)
	tok, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, "", fmt.Errorf("auth: code exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return Identity{}, "", ErrNoIDToken
	}
	idt, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, "", fmt.Errorf("auth: verify ID token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(nonce)) != 1 {
		return Identity{}, "", ErrNonce
	}
	var std struct {
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
		Name              string `json:"name"`
	}
	var all map[string]json.RawMessage
	if err := idt.Claims(&std); err != nil {
		return Identity{}, "", fmt.Errorf("auth: ID token claims: %w", err)
	}
	if err := idt.Claims(&all); err != nil {
		return Identity{}, "", fmt.Errorf("auth: ID token claims: %w", err)
	}
	id := Identity{
		Issuer: idt.Issuer, Subject: idt.Subject, Username: std.PreferredUsername,
		Email: std.Email, DisplayName: std.Name, Roles: RolesFromClaims(all, p.projectID),
	}
	if len(id.Roles) == 0 || id.Username == "" {
		p.fillFromUserinfo(ctx, tok, &id)
	}
	return id, raw, nil
}

// fillFromUserinfo completes the identity from the userinfo endpoint. Zitadel
// puts profile claims in the ID token only with "User Info inside ID Token"
// and roles only with "User roles inside ID Token"; with "Assert roles on
// authentication" userinfo has both. Failures leave the identity as is.
func (p *Provider) fillFromUserinfo(ctx context.Context, tok *oauth2.Token, id *Identity) {
	info, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil || info.Subject != id.Subject {
		return
	}
	var std struct {
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	var claims map[string]json.RawMessage
	if info.Claims(&std) != nil || info.Claims(&claims) != nil {
		return
	}
	if id.Username == "" {
		id.Username = std.PreferredUsername
	}
	if id.Email == "" {
		id.Email = info.Email
	}
	if id.DisplayName == "" {
		id.DisplayName = std.Name
	}
	if len(id.Roles) == 0 {
		id.Roles = RolesFromClaims(claims, p.projectID)
	}
}

// EndSessionURL is the RP-initiated logout URL at the issuer, or "" when the
// discovery document has no end_session_endpoint.
func (p *Provider) EndSessionURL(idTokenHint, postLogoutRedirect string) string {
	if p.endSession == "" {
		return ""
	}
	u, err := url.Parse(p.endSession)
	if err != nil {
		return ""
	}
	q := u.Query()
	if idTokenHint != "" {
		q.Set("id_token_hint", idTokenHint)
	}
	q.Set("client_id", p.oauth.ClientID)
	if postLogoutRedirect != "" {
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
