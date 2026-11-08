// Package client talks to the planner's HTTP API for the CLI and `ironman
// mcp` (docs/04 section 2.2). It never opens the database (CLAUDE.md rule
// 1): in AUTH_MODE=dev it sends nothing, in oidc it sends the bearer token of
// the device-flow login (Login) or of IRONMAN_TOKEN.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
)

// Errors the commands explain to the user.
var (
	// ErrLoginRequired: the server wants a login and the client has none.
	ErrLoginRequired = errors.New("client: login required")
	// ErrLoginExpired: the refresh token was refused (revoked or expired).
	ErrLoginExpired = errors.New("client: the login expired")
	// ErrNoAccount: the user has no account on the server.
	ErrNoAccount = errors.New("client: no account")
)

// APIError is a non-2xx answer of the API.
type APIError struct {
	Method  string
	Path    string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Message)
}

// ManyAccountsError: the user has several accounts and none was chosen.
type ManyAccountsError struct {
	Accounts []Account
}

func (e *ManyAccountsError) Error() string {
	ids := make([]string, len(e.Accounts))
	for i, a := range e.Accounts {
		ids[i] = a.ID
	}
	return "client: several accounts (" + strings.Join(ids, ", ") + "); choose one"
}

// Options build a Client.
type Options struct {
	// URL is the server (DefaultURL when empty).
	URL string
	// Account is the account id; empty picks the user's only account.
	Account string
	// Token is a bearer token used as is (IRONMAN_TOKEN); it wins over the
	// stored credential.
	Token string
	// Store holds the login of `ironman login`; nil sends no credential
	// unless Token is set.
	Store *Store
	// HTTP is the transport; nil uses a client with a 30 s timeout.
	HTTP *http.Client
}

// Client is a session with one server, for one account.
type Client struct {
	url     string
	account string
	hc      *http.Client
	tokens  oauth2.TokenSource
}

// New builds the client. It reads the stored credential of the server but
// makes no request.
func New(ctx context.Context, o Options) (*Client, error) {
	c := &Client{url: NormalizeURL(o.URL), account: strings.TrimSpace(o.Account), hc: o.HTTP}
	if c.url == "" {
		c.url = DefaultURL
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: 30 * time.Second}
	}
	switch {
	case o.Token != "":
		c.tokens = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: o.Token, TokenType: "Bearer"})
	case o.Store != nil:
		cred, ok, err := o.Store.Credential(c.url)
		if err != nil {
			return nil, err
		}
		if ok {
			c.tokens = newTokenSource(ctx, c.hc, o.Store, c.url, cred)
		}
	}
	return c, nil
}

// URL is the server.
func (c *Client) URL() string { return c.url }

// LoggedIn reports whether requests carry a bearer token.
func (c *Client) LoggedIn() bool { return c.tokens != nil }

// do sends one request; a non-nil body is JSON, out receives the JSON
// answer (nil discards it).
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.url + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.tokens != nil {
		tok, terr := c.tokens.Token()
		if terr != nil {
			return terr
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("client: %s %s: %w", method, path, err)
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		if resp.StatusCode == http.StatusUnauthorized && c.tokens == nil {
			return fmt.Errorf("%w: %s", ErrLoginRequired, e.Error)
		}
		return &APIError{Method: method, Path: path, Status: resp.StatusCode, Message: e.Error}
	}
	if out == nil {
		return nil
	}
	if raw2, ok := out.(*json.RawMessage); ok {
		*raw2 = append((*raw2)[:0], raw...)
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("client: %s %s: %w", method, path, err)
	}
	return nil
}

// AuthClient is GET /api/auth/client.
func (c *Client) AuthClient(ctx context.Context) (AuthClient, error) {
	var out AuthClient
	err := c.do(ctx, http.MethodGet, "/api/auth/client", nil, nil, &out)
	return out, err
}

// Account is an account of the user.
type Account struct {
	ID      string `json:"id"`
	RSN     string `json:"rsn"`
	Mode    string `json:"mode"`
	GuideID string `json:"guide_id"`
	Layer   string `json:"layer"`
}

// User is the user of the session or token.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role"`
}

// Me is GET /api/me.
type Me struct {
	AuthMode string    `json:"auth_mode"`
	User     User      `json:"user"`
	Roles    []string  `json:"roles"`
	Accounts []Account `json:"accounts"`
}

// Me is GET /api/me.
func (c *Client) Me(ctx context.Context) (Me, error) {
	var out Me
	err := c.do(ctx, http.MethodGet, "/api/me", nil, nil, &out)
	return out, err
}

// Account is the account the client works on: the chosen one, else the
// user's only account (ErrNoAccount, *ManyAccountsError otherwise).
func (c *Client) Account(ctx context.Context) (string, error) {
	if c.account != "" {
		return c.account, nil
	}
	me, err := c.Me(ctx)
	if err != nil {
		return "", err
	}
	switch len(me.Accounts) {
	case 0:
		return "", ErrNoAccount
	case 1:
		c.account = me.Accounts[0].ID
		return c.account, nil
	}
	return "", &ManyAccountsError{Accounts: me.Accounts}
}

func (c *Client) accountQuery(ctx context.Context) (url.Values, error) {
	acc, err := c.Account(ctx)
	if err != nil {
		return nil, err
	}
	return url.Values{"account": {acc}}, nil
}

// Route is GET /api/route: the window of the active route from the cursor
// (limit steps after it; 0 is the server's default).
func (c *Client) Route(ctx context.Context, limit int) (panel.Window, error) {
	var out panel.Window
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	err = c.do(ctx, http.MethodGet, "/api/route", q, nil, &out)
	return out, err
}

// Mark is PATCH /api/guides/{guide}/steps/{id}/mark: kind is one of the
// coreprogress.MarkAction values; a nil note keeps the step's note.
func (c *Client) Mark(ctx context.Context, guide, stepID string, kind coreprogress.MarkAction, note *string) error {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{"kind": kind}
	if note != nil {
		body["note"] = *note
	}
	return c.do(ctx, http.MethodPatch, "/api/guides/"+url.PathEscape(guide)+"/steps/"+url.PathEscape(stepID)+"/mark", q, body, nil)
}

// Goal is one of the account's goals.
type Goal struct {
	ID         string     `json:"id"`
	Key        string     `json:"objective_key"`
	Label      string     `json:"label"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Priority   int        `json:"priority"`
	Status     string     `json:"status"`
	Verdict    string     `json:"verdict"`
	AchievedAt *time.Time `json:"achieved_at,omitempty"`
}

// Goals is GET /api/goals.
func (c *Client) Goals(ctx context.Context) ([]Goal, error) {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Goals []Goal `json:"goals"`
	}
	err = c.do(ctx, http.MethodGet, "/api/goals", q, nil, &out)
	return out.Goals, err
}

// GoalPlan is GET /api/goals/{id}/plan.
type GoalPlan struct {
	Goal Goal                  `json:"goal"`
	Plan coreprogress.GoalPlan `json:"plan"`
}

// GoalPlan is GET /api/goals/{id}/plan.
func (c *Client) GoalPlan(ctx context.Context, id string) (GoalPlan, error) {
	var out GoalPlan
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/goals/"+url.PathEscape(id)+"/plan", q, nil, &out)
	return out, err
}

// SyncRun is the result of POST /api/sync.
type SyncRun struct {
	ID            string     `json:"id"`
	Source        string     `json:"source"`
	Status        string     `json:"status"`
	Error         string     `json:"error,omitempty"`
	StepsChanged  int        `json:"steps_changed"`
	NextAllowedAt *time.Time `json:"next_allowed_at,omitempty"`
}

// Sync is POST /api/sync (hiscores now).
func (c *Client) Sync(ctx context.Context) (SyncRun, error) {
	var out SyncRun
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodPost, "/api/sync", q, nil, &out)
	return out, err
}

// AccountState is GET /api/account/state.
type AccountState struct {
	Account     Account `json:"account"`
	TotalLevel  int     `json:"total_level,omitempty"`
	QuestPoints *int64  `json:"quest_points,omitempty"`
	Skills      []struct {
		Skill  string `json:"skill"`
		Level  int    `json:"level"`
		XP     int64  `json:"xp"`
		Source string `json:"source"`
	} `json:"skills"`
	Quests []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"quests"`
	Diaries []struct {
		Area     string `json:"area"`
		Tier     string `json:"tier"`
		Complete bool   `json:"complete"`
	} `json:"diaries"`
}

// AccountState is GET /api/account/state.
func (c *Client) AccountState(ctx context.Context) (AccountState, error) {
	var out AccountState
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/account/state", q, nil, &out)
	return out, err
}

// Get is a GET of any API path as raw JSON (for `ironman mcp`). Account
// routes get ?account= when accountScoped.
func (c *Client) Get(ctx context.Context, path string, query url.Values, accountScoped bool) (json.RawMessage, error) {
	if query == nil {
		query = url.Values{}
	}
	if accountScoped {
		acc, err := c.Account(ctx)
		if err != nil {
			return nil, err
		}
		query.Set("account", acc)
	}
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, path, query, nil, &out)
	return out, err
}

// GuideInfo is one guide of GET /api/guides, with the source its credit
// line comes from (panel.Attribution).
type GuideInfo struct {
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Source *content.Source `json:"source,omitempty"`
}

// Guides is GET /api/guides.
func (c *Client) Guides(ctx context.Context) ([]GuideInfo, error) {
	var out struct {
		Guides []GuideInfo `json:"guides"`
	}
	err := c.do(ctx, http.MethodGet, "/api/guides", nil, nil, &out)
	return out.Guides, err
}
