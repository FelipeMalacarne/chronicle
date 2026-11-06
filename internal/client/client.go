// Package client is the typed HTTP client of the planner's API, shared by the
// CLI subcommands (now, done, plan, state, sync) and the account MCP server.
// It never opens the database (CLAUDE.md rule 1): every call goes to a running
// `ironman serve` at IRONMAN_URL with the token in IRONMAN_TOKEN.
//
// The response types mirror the JSON of internal/httpapi. Pure types of
// internal/core are reused where the API embeds them; the store's types are
// mirrored here because CLI and MCP code never import internal/store.
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
	"os"
	"strings"
	"time"
)

// DefaultURL is the server of `just dev` and of the stack on zaros.
const DefaultURL = "http://127.0.0.1:8080"

// Client names of the X-Ironman-Client header: the server records them as the
// origin of the owner's mark commands (mark_commands.via).
const (
	ViaCLI = "cli"
	ViaMCP = "mcp"
)

// Client talks to one server for one kind of caller.
type Client struct {
	BaseURL string
	Token   string
	// Via is sent as X-Ironman-Client ("cli" or "mcp").
	Via  string
	HTTP *http.Client
}

// New returns a client of baseURL (DefaultURL when empty).
func New(baseURL, token, via string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Via: via,
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

// FromEnv is New with IRONMAN_URL and IRONMAN_TOKEN.
func FromEnv(via string) *Client {
	return New(os.Getenv("IRONMAN_URL"), os.Getenv("IRONMAN_TOKEN"), via)
}

// Error is an answer of the server other than 2xx.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// IsNotFound reports a 404 of the server (unknown account, step, goal…).
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Via != "" {
		req.Header.Set("X-Ironman-Client", c.Via)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var e struct {
			Error string `json:"error"`
		}
		msg := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return &Error{Status: resp.StatusCode, Message: msg}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: unreadable answer: %w", method, path, err)
	}
	return nil
}

// call runs one request and decodes its answer into a T.
func call[T any](ctx context.Context, c *Client, method, path string, query url.Values, body any) (T, error) {
	var out T
	err := c.do(ctx, method, path, query, body, &out)
	return out, err
}

func accountQuery(account string, kv ...string) url.Values {
	q := url.Values{"account": {account}}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			q.Set(kv[i], kv[i+1])
		}
	}
	return q
}

func seg(s string) string { return url.PathEscape(s) }

// Me is GET /api/me: the user and their accounts.
func (c *Client) Me(ctx context.Context) (Me, error) {
	return call[Me](ctx, c, http.MethodGet, "/api/me", nil, nil)
}

// ResolveAccount returns account when set; otherwise the only account of the
// user (GET /api/me). With several, the error lists them.
func (c *Client) ResolveAccount(ctx context.Context, account string) (string, error) {
	if a := strings.TrimSpace(account); a != "" {
		return a, nil
	}
	me, err := c.Me(ctx)
	if err != nil {
		return "", fmt.Errorf("no account given and %w", err)
	}
	switch len(me.Accounts) {
	case 0:
		return "", errors.New("the server has no account yet: create one in /setup or with `just account-create <rsn>`")
	case 1:
		return me.Accounts[0].ID, nil
	}
	ids := make([]string, 0, len(me.Accounts))
	for _, a := range me.Accounts {
		ids = append(ids, a.ID+" ("+a.RSN+")")
	}
	return "", fmt.Errorf("several accounts, pass --account or IRONMAN_ACCOUNT: %s", strings.Join(ids, ", "))
}

// Progress is GET /api/progress: every route's cursor and counts, and the
// account's sources.
func (c *Client) Progress(ctx context.Context, account string) (Progress, error) {
	return call[Progress](ctx, c, http.MethodGet, "/api/progress", accountQuery(account), nil)
}

// Route is GET /api/route: the window of the active route from the cursor
// (the plugin panel's view), limit steps after the cursor (0 = server default).
func (c *Client) Route(ctx context.Context, account string, limit int) (Window, error) {
	q := accountQuery(account)
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	return call[Window](ctx, c, http.MethodGet, "/api/route", q, nil)
}

// Board is GET /api/guides/{guide}/board.
func (c *Client) Board(ctx context.Context, account, guide string) (Board, error) {
	return call[Board](ctx, c, http.MethodGet, "/api/guides/"+seg(guide)+"/board", accountQuery(account), nil)
}

// Step is GET /api/guides/{guide}/steps/{id}: the step's content.
func (c *Client) Step(ctx context.Context, guide, step string) (StepContent, error) {
	return call[StepContent](ctx, c, http.MethodGet, "/api/guides/"+seg(guide)+"/steps/"+seg(step), nil, nil)
}

// Mark is PATCH /api/guides/{guide}/steps/{id}/mark. note nil keeps the
// step's note; "" clears it.
func (c *Client) Mark(ctx context.Context, account, guide, step string, kind MarkAction, note *string) (Board, error) {
	body := map[string]any{"kind": kind}
	if note != nil {
		body["note"] = *note
	}
	return call[Board](ctx, c, http.MethodPatch, "/api/guides/"+seg(guide)+"/steps/"+seg(step)+"/mark", accountQuery(account), body)
}

// Goals is GET /api/goals.
func (c *Client) Goals(ctx context.Context, account string) ([]Goal, error) {
	out, err := call[struct {
		Goals []Goal `json:"goals"`
	}](ctx, c, http.MethodGet, "/api/goals", accountQuery(account), nil)
	return out.Goals, err
}

// AddGoal is POST /api/goals: pins an objective key; pinning it again
// returns the goal that exists.
func (c *Client) AddGoal(ctx context.Context, account, key, label string) (Goal, error) {
	body := map[string]string{"objective_key": key}
	if label != "" {
		body["label"] = label
	}
	return call[Goal](ctx, c, http.MethodPost, "/api/goals", accountQuery(account), body)
}

// RemoveGoal is DELETE /api/goals/{id}.
func (c *Client) RemoveGoal(ctx context.Context, account, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/goals/"+seg(id), accountQuery(account), nil, nil)
}

// GoalPlan is GET /api/goals/{id}/plan.
func (c *Client) GoalPlan(ctx context.Context, account, id string) (GoalPlanResponse, error) {
	return call[GoalPlanResponse](ctx, c, http.MethodGet, "/api/goals/"+seg(id)+"/plan", accountQuery(account), nil)
}

// AccountState is GET /api/account/state.
func (c *Client) AccountState(ctx context.Context, account string) (AccountState, error) {
	return call[AccountState](ctx, c, http.MethodGet, "/api/account/state", accountQuery(account), nil)
}

// Entity is GET /api/world/entity: one entity with its requirements, what it
// unlocks and where each route does it.
func (c *Client) Entity(ctx context.Context, account, ref string) (Entity, error) {
	return call[Entity](ctx, c, http.MethodGet, "/api/world/entity", accountQuery(account, "ref", ref), nil)
}

// Graph is GET /api/world/graph: the requirements of an objective key by
// depth, what it unlocks and the critical path.
func (c *Client) Graph(ctx context.Context, account, focus string, depth int) (Graph, error) {
	q := accountQuery(account, "focus", focus)
	if depth > 0 {
		q.Set("depth", fmt.Sprint(depth))
	}
	return call[Graph](ctx, c, http.MethodGet, "/api/world/graph", q, nil)
}

// Sync is POST /api/sync: a hiscores run now.
func (c *Client) Sync(ctx context.Context, account string) (SyncRun, error) {
	return call[SyncRun](ctx, c, http.MethodPost, "/api/sync", accountQuery(account), nil)
}
