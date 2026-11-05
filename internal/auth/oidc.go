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
)

// Config is the client registration at the issuer: a public Web application
// with PKCE and no client secret.
type Config struct {
	Issuer      string
	ClientID    string
	RedirectURL string
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
	}, nil
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
		Email: std.Email, DisplayName: std.Name, Roles: RolesFromClaims(all),
	}
	if len(id.Roles) == 0 {
		// Zitadel puts the roles in the ID token only with "User roles inside
		// ID token"; with "Assert roles on authentication" userinfo has them.
		if info, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil && info.Subject == id.Subject {
			var claims map[string]json.RawMessage
			if info.Claims(&claims) == nil {
				id.Roles = RolesFromClaims(claims)
			}
		}
	}
	return id, raw, nil
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
