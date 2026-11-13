package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
)

// Plans is GET /api/plans: the account's plans and the one it follows.
type Plans struct {
	Account string      `json:"account"`
	Active  string      `json:"active"`
	Plans   []plan.Plan `json:"plans"`
}

// NewPlan is the body of POST /api/plans: a goal or custom plan.
type NewPlan struct {
	Kind  string      `json:"kind"`
	Title string      `json:"title"`
	Goal  string      `json:"goal,omitempty"`
	Items []plan.Item `json:"items"`
}

// Plans lists the account's plans.
func (c *Client) Plans(ctx context.Context) (Plans, error) {
	var out Plans
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/plans", q, nil, &out)
	return out, err
}

// CreatePlan stores a plan; the server verifies every item.
func (c *Client) CreatePlan(ctx context.Context, p NewPlan) (plan.Plan, error) {
	var out plan.Plan
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodPost, "/api/plans", q, p, &out)
	return out, err
}

// UsePlan makes the account follow a plan ("" or a route id: its route).
func (c *Client) UsePlan(ctx context.Context, id string) (plan.Plan, error) {
	var out plan.Plan
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodPut, "/api/plans/active", q, map[string]string{"plan": id}, &out)
	return out, err
}

// DeletePlan removes a stored plan.
func (c *Client) DeletePlan(ctx context.Context, id string) error {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, "/api/plans/"+url.PathEscape(id), q, nil, nil)
}
