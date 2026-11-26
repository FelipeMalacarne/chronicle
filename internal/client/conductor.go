package client

import (
	"context"
	"net/http"
)

// Conductor is the account's conductor (GET /api/conductor, docs/12 section
// 12.5): off, idle, running (its job) or blocked (and why).
type Conductor struct {
	Account string `json:"account"`
	State   string `json:"state"`
	Job     string `json:"job,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Conductor reads the account's conductor state.
func (c *Client) Conductor(ctx context.Context) (Conductor, error) {
	var out Conductor
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/conductor", q, nil, &out)
	return out, err
}

// SetConductor turns the account's conductor on or off.
func (c *Client) SetConductor(ctx context.Context, on bool) (Conductor, error) {
	var out Conductor
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodPost, "/api/conductor", q, map[string]bool{"on": on}, &out)
	return out, err
}
