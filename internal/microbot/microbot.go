// Package microbot is a typed client of the Microbot Agent Server (docs/08):
// HTTP over the Unix socket of the dev client's home, authenticated by the
// token file the client writes. Only `ironman agent` uses it, on the machine
// that runs the game; the planner server never talks to the game directly.
package microbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotRunning: the client is not up (no socket, or nothing listening).
var ErrNotRunning = errors.New("microbot: client not running")

// Client talks to one Agent Server.
type Client struct {
	// Home is the client's user.home (the harness dev home,
	// ~/.local/share/microbot-dev). The socket and the token live under
	// Home/.runelite.
	Home string
	// Timeout bounds one call that does not set its own; zero means 30 s.
	Timeout time.Duration
}

// DefaultHome is the harness dev home, resolved like the harness's lib.sh:
// MICROBOT_HOME, else MB_DEV_HOME, else $XDG_DATA_HOME/microbot-dev, else
// ~/.local/share/microbot-dev.
func DefaultHome() string {
	for _, k := range []string{"MICROBOT_HOME", "MB_DEV_HOME"} {
		if h := os.Getenv(k); h != "" {
			return h
		}
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "microbot-dev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "microbot-dev")
}

// SocketPath is the Agent Server's Unix socket.
func (c *Client) SocketPath() string {
	return filepath.Join(c.Home, ".runelite", "agent", "agent.sock")
}

// TokenPath is the file with the X-Agent-Token; it changes when the client
// restarts, so it is read on every call.
func (c *Client) TokenPath() string { return filepath.Join(c.Home, ".runelite", ".agent-token") }

// APIError is a non-2xx answer of the Agent Server.
type APIError struct {
	Path    string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("microbot %s: HTTP %d: %s", e.Path, e.Status, e.Message)
}

func (c *Client) httpClient(timeout time.Duration) *http.Client {
	sock := c.SocketPath()
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
			DisableKeepAlives: true,
		},
	}
}

// Do sends one request and returns the raw body of a 2xx answer.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, timeout time.Duration) ([]byte, string, error) {
	if _, err := os.Stat(c.SocketPath()); err != nil {
		return nil, "", ErrNotRunning
	}
	token, err := os.ReadFile(c.TokenPath())
	if err != nil {
		return nil, "", fmt.Errorf("%w: no agent token", ErrNotRunning)
	}
	if timeout == 0 {
		timeout = c.Timeout
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	var rd io.Reader
	if body != nil {
		b, merr := json.Marshal(body)
		if merr != nil {
			return nil, "", merr
		}
		rd = bytes.NewReader(b)
	}
	// The Agent Server checks the Host header (DNS rebinding): loopback, as
	// the harness sends it.
	u := "http://127.0.0.1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Agent-Token", strings.TrimSpace(string(token)))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient(timeout).Do(req)
	if err != nil {
		var op *net.OpError
		if errors.As(err, &op) && op.Op == "dial" {
			return nil, "", ErrNotRunning
		}
		return nil, "", fmt.Errorf("microbot %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", fmt.Errorf("microbot %s: %w", path, err)
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		return nil, "", &APIError{Path: path, Status: resp.StatusCode, Message: e.Error}
	}
	return raw, resp.Header.Get("Content-Type"), nil
}

// JSON is Do for a JSON answer, passed through as is.
func (c *Client) JSON(ctx context.Context, method, path string, query url.Values, body any, timeout time.Duration) (json.RawMessage, error) {
	raw, _, err := c.Do(ctx, method, path, query, body, timeout)
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("microbot %s: answer is not JSON", path)
	}
	return raw, nil
}

// State is GET /state: game state, player, scripts paused, instance.
func (c *Client) State(ctx context.Context) (json.RawMessage, error) {
	return c.JSON(ctx, http.MethodGet, "/state", nil, nil, 10*time.Second)
}

// LoginState is GET /login: login screen, active profile, world, errors.
func (c *Client) LoginState(ctx context.Context) (json.RawMessage, error) {
	return c.JSON(ctx, http.MethodGet, "/login", nil, nil, 10*time.Second)
}

// Login is POST /login with the active profile, blocking up to 60 s; world
// 0 keeps the profile's world.
func (c *Client) Login(ctx context.Context, world int) (json.RawMessage, error) {
	body := map[string]any{"wait": true, "timeout": 60}
	if world > 0 {
		body["world"] = world
	}
	return c.JSON(ctx, http.MethodPost, "/login", nil, body, 75*time.Second)
}

// Logout is POST /logout.
func (c *Client) Logout(ctx context.Context) (json.RawMessage, error) {
	return c.JSON(ctx, http.MethodPost, "/logout", nil, map[string]any{}, 30*time.Second)
}

// Screenshot is GET /screenshot: the next rendered frame as PNG.
func (c *Client) Screenshot(ctx context.Context) ([]byte, error) {
	raw, ctype, err := c.Do(ctx, http.MethodGet, "/screenshot", nil, nil, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(ctype, "image/") {
		return nil, fmt.Errorf("microbot /screenshot: answer is %q, not an image", ctype)
	}
	return raw, nil
}
