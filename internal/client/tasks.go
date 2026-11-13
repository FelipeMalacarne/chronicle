package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
)

// PlanNext is GET /api/plan/next: the next typed task of the account's
// active plan (docs/10 section 9).
type PlanNext struct {
	Account string `json:"account"`
	plan.Result
}

// PlanNext reads the next task of the active plan.
func (c *Client) PlanNext(ctx context.Context) (PlanNext, error) {
	var out PlanNext
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/plan/next", q, nil, &out)
	return out, err
}

// TaskRun is the answer of POST /api/microbot/task.
type TaskRun struct {
	Result agent.RunStatus `json:"result"`
	Task   plan.TaskView   `json:"task"`
}

// RunTask is POST /api/microbot/task: the account's agent starts the
// plan's next task (key, when given, must still be the next one). It acts
// in game.
func (c *Client) RunTask(ctx context.Context, key string, supervised bool, timeoutS int) (TaskRun, error) {
	var out TaskRun
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	body := map[string]any{"supervised": supervised}
	if key != "" {
		body["key"] = key
	}
	if timeoutS > 0 {
		body["timeoutS"] = timeoutS
	}
	err = c.do(ctx, http.MethodPost, "/api/microbot/task", q, body, &out)
	return out, err
}

// TaskStatus is GET /api/microbot/task: the agent's current or last run
// and, for a task run, its termination on the account's facts now.
type TaskStatus struct {
	Run    *agent.Run     `json:"run"`
	Proof  *plan.TaskView `json:"proof"`
	Proven bool           `json:"proven"`
	Reason string         `json:"reason,omitempty"`
}

// TaskStatus reads the last run and the proof of its task.
func (c *Client) TaskStatus(ctx context.Context) (TaskStatus, error) {
	var out TaskStatus
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/microbot/task", q, nil, &out)
	return out, err
}

// CancelRun is POST /api/microbot/run/cancel: the run stops (a task's
// machine stops on the bridge) and what it left stays as it is.
func (c *Client) CancelRun(ctx context.Context) error {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return err
	}
	var out json.RawMessage
	return c.do(ctx, http.MethodPost, "/api/microbot/run/cancel", q, map[string]any{}, &out)
}
