package client

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

// ProfileDef is a client profile as `chronicle profiles put` sends it and
// `chronicle profiles show` prints it: its model and its own plugins and
// settings (docs/11 section 4.3).
type ProfileDef struct {
	Parent   string                 `json:"parent,omitempty"`
	Plugins  []agent.ProfilePlugin  `json:"plugins"`
	Settings []agent.ProfileSetting `json:"settings"`
}

// Profile is one of the user's client profiles.
type Profile struct {
	Name string `json:"name"`
	ProfileDef
	Accounts  []string  `json:"accounts"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AccountClient is an account's client profile: the profile it names, its
// own settings and what its worker gets from both.
type AccountClient struct {
	AccountID string                 `json:"account_id"`
	Profile   string                 `json:"profile"`
	Settings  []agent.ProfileSetting `json:"settings"`
	Effective agent.ClientProfile    `json:"effective"`
}

// ListProfiles is GET /api/client-profiles.
func (c *Client) ListProfiles(ctx context.Context) ([]Profile, error) {
	var out struct {
		Profiles []Profile `json:"profiles"`
	}
	err := c.do(ctx, http.MethodGet, "/api/client-profiles", nil, nil, &out)
	return out.Profiles, err
}

// GetProfile is GET /api/client-profiles/{name}.
func (c *Client) GetProfile(ctx context.Context, name string) (Profile, error) {
	var out Profile
	err := c.do(ctx, http.MethodGet, "/api/client-profiles/"+url.PathEscape(name), nil, nil, &out)
	return out, err
}

// PutProfile creates or replaces a profile.
func (c *Client) PutProfile(ctx context.Context, name string, def ProfileDef) (Profile, error) {
	var out Profile
	err := c.do(ctx, http.MethodPut, "/api/client-profiles/"+url.PathEscape(name), nil, def, &out)
	return out, err
}

// DeleteProfile deletes a profile no account or profile uses.
func (c *Client) DeleteProfile(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/client-profiles/"+url.PathEscape(name), nil, nil, nil)
}

// AccountClient is GET /api/accounts/client.
func (c *Client) AccountClient(ctx context.Context, account string) (AccountClient, error) {
	var out AccountClient
	err := c.do(ctx, http.MethodGet, "/api/accounts/client", url.Values{"account": {account}}, nil, &out)
	return out, err
}

// SetAccountClient sets the account's profile ("" for none) and replaces
// its own settings.
func (c *Client) SetAccountClient(ctx context.Context, account, profile string, settings []agent.ProfileSetting) (AccountClient, error) {
	if settings == nil {
		settings = []agent.ProfileSetting{}
	}
	body := map[string]any{"profile": profile, "settings": settings}
	var out AccountClient
	err := c.do(ctx, http.MethodPut, "/api/accounts/client", url.Values{"account": {account}}, body, &out)
	return out, err
}
