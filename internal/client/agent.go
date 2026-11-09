package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// ErrNoAgentCredentials: the server is in oidc mode and the agent has no
// machine user to log in with.
var ErrNoAgentCredentials = errors.New("client: the server is in oidc mode; set MICROBOT_AGENT_CLIENT_ID and MICROBOT_AGENT_CLIENT_SECRET (a Zitadel machine user with the microbot:agent role)")

// AgentAuth is the bearer of `ironman agent` (docs/08). It asks the server
// how to authenticate on the first connection and again after a failure: a
// dev server takes no token; an oidc server takes the access token of a
// Zitadel machine user, from the client_credentials grant at the issuer the
// server names, with the scopes it gives. The token is cached and renewed
// before it expires.
type AgentAuth struct {
	// URL is the planner server.
	URL string
	// ClientID and ClientSecret are the machine user's credentials; unused
	// against a dev server.
	ClientID     string
	ClientSecret string
	// HTTP talks to the server and the issuer; nil is http.DefaultClient.
	HTTP *http.Client

	mu  sync.Mutex
	dev bool
	src oauth2.TokenSource
}

// Token is the bearer of the next connection; "" against a dev server.
func (a *AgentAuth) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.dev && a.src == nil {
		if err := a.discover(ctx); err != nil {
			return "", err
		}
	}
	if a.dev {
		return "", nil
	}
	tok, err := a.src.Token()
	if err != nil {
		// Ask the server again next time: its mode or scopes may have changed.
		a.src = nil
		return "", fmt.Errorf("client: machine token: %w", err)
	}
	return tok.AccessToken, nil
}

func (a *AgentAuth) discover(ctx context.Context) error {
	hc := a.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	var info AuthClient
	if err := getJSON(ctx, hc, NormalizeURL(a.URL)+"/api/auth/client", &info); err != nil {
		return err
	}
	switch {
	case info.AuthMode == "dev":
		a.dev = true
		return nil
	case info.AuthMode != "oidc":
		return fmt.Errorf("client: the server has no login (auth_mode %q)", info.AuthMode)
	case len(info.AgentScopes) == 0:
		return errors.New("client: the server offers no agent login (MICROBOT_CONTROL off or no OIDC_PROJECT_ID)")
	case a.ClientID == "" || a.ClientSecret == "":
		return ErrNoAgentCredentials
	}
	var disc struct {
		Issuer   string `json:"issuer"`
		TokenURL string `json:"token_endpoint"`
	}
	if err := getJSON(ctx, hc, strings.TrimRight(info.Issuer, "/")+"/.well-known/openid-configuration", &disc); err != nil {
		return err
	}
	if disc.Issuer != info.Issuer || disc.TokenURL == "" {
		return fmt.Errorf("client: the issuer %s has no token endpoint for it", info.Issuer)
	}
	cfg := clientcredentials.Config{
		ClientID: a.ClientID, ClientSecret: a.ClientSecret, TokenURL: disc.TokenURL,
		Scopes: info.AgentScopes, AuthStyle: oauth2.AuthStyleInHeader,
	}
	// The token source outlives this call: it renews with its own context.
	a.src = cfg.TokenSource(context.WithValue(context.Background(), oauth2.HTTPClient, hc))
	return nil
}
