package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

// SessionInfo is what the server tells of an account's stored game session
// (never the session itself).
type SessionInfo struct {
	AccountID       string     `json:"account_id"`
	Character       string     `json:"character"`
	KeyID           string     `json:"key_id"`
	PushedAt        time.Time  `json:"pushed_at"`
	PushedBy        string     `json:"pushed_by"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	LastLoginResult string     `json:"last_login_result"`
	// LoginFailed: the last login with this session did not get in.
	LoginFailed bool `json:"login_failed"`
}

// WorkerUsage is a worker pod's CPU (cores) and memory (bytes) from the
// cluster's metrics API.
type WorkerUsage struct {
	CPUCores    float64 `json:"cpu_cores"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// WorkerCapacity is how many more workers fit in the namespace's quota.
type WorkerCapacity struct {
	Fits      int    `json:"fits"`
	LimitedBy string `json:"limited_by"`
}

// WorkerPod is the cluster's view of a worker.
type WorkerPod struct {
	Deployment string `json:"deployment"`
	Pod        string `json:"pod"`
	State      string `json:"state"`
	Reason     string `json:"reason"`
	Image      string `json:"image"`
}

// WorkerAgent is the linked agent of a worker or the attached one.
type WorkerAgent struct {
	ID      string `json:"id"`
	Host    string `json:"host"`
	Account string `json:"account"`
	Kind    string `json:"kind"`
	Worker  string `json:"worker"`
	Version string `json:"version"`
}

// Worker is one managed account of GET /api/workers.
type Worker struct {
	Account   string       `json:"account"`
	RSN       string       `json:"rsn"`
	Session   *SessionInfo `json:"session"`
	Desired   string       `json:"desired"`
	World     int          `json:"world"`
	Image     string       `json:"image"`
	LastError string       `json:"last_error"`
	AutoLogin bool         `json:"auto_login"`
	Pod       *WorkerPod   `json:"pod"`
	Usage     *WorkerUsage `json:"usage"`
	Agent     *WorkerAgent `json:"agent"`
	Outdated  bool         `json:"outdated"`
}

// Workers is GET /api/workers.
type Workers struct {
	Enabled  bool            `json:"enabled"`
	Image    string          `json:"image"`
	Workers  []Worker        `json:"workers"`
	Attached []WorkerAgent   `json:"attached"`
	Capacity *WorkerCapacity `json:"capacity"`
}

// PutSession stores an account's game session (docs/11 section 5); the
// answer carries only what the server tells of it.
func (c *Client) PutSession(ctx context.Context, account string, s agent.GameSession) (SessionInfo, error) {
	var out SessionInfo
	err := c.do(ctx, http.MethodPut, "/api/accounts/session", url.Values{"account": {account}}, s, &out)
	return out, err
}

// ListWorkers is GET /api/workers.
func (c *Client) ListWorkers(ctx context.Context) (Workers, error) {
	var out Workers
	err := c.do(ctx, http.MethodGet, "/api/workers", nil, nil, &out)
	return out, err
}

// WorkerAction is POST /api/workers/{start,stop,restart} for one account.
func (c *Client) WorkerAction(ctx context.Context, account, action string, world int) error {
	var body any = struct{}{}
	if world > 0 {
		body = map[string]int{"world": world}
	}
	var out json.RawMessage
	return c.do(ctx, http.MethodPost, "/api/workers/"+action, url.Values{"account": {account}}, body, &out)
}

// SetAutoLogin turns the account's worker's automatic login after a new pod
// on or off (docs/11 section 15.5).
func (c *Client) SetAutoLogin(ctx context.Context, account string, on bool) error {
	var out json.RawMessage
	return c.do(ctx, http.MethodPut, "/api/workers/auto-login", url.Values{"account": {account}}, map[string]bool{"enabled": on}, &out)
}
