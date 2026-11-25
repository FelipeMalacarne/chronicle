package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/job"
)

// Job is a job of the account (GET /api/jobs, docs/12 section 13).
type Job struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	State      string          `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	Source     string          `json:"source"`
	Goal       json.RawMessage `json:"goal"`
	Metric     *job.Metric     `json:"metric,omitempty"`
	Progress   *job.Progress   `json:"progress,omitempty"`
	StallAfter int             `json:"stall_after_s"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

// JobDetail is one job with the runs that attempted it and its events.
type JobDetail struct {
	Job  Job `json:"job"`
	Runs []struct {
		UID       string `json:"uid"`
		ClassName string `json:"className"`
		State     string `json:"state"`
	} `json:"runs"`
	Events []struct {
		ID   int64     `json:"id"`
		Type string    `json:"type"`
		At   time.Time `json:"at"`
	} `json:"events"`
}

// Jobs lists the account's jobs, newest first; state "" takes every state.
func (c *Client) Jobs(ctx context.Context, state string) ([]Job, error) {
	q, err := c.accountQuery(ctx)
	if err != nil {
		return nil, err
	}
	if state != "" {
		q.Set("state", state)
	}
	var out struct {
		Jobs []Job `json:"jobs"`
	}
	err = c.do(ctx, http.MethodGet, "/api/jobs", q, nil, &out)
	return out.Jobs, err
}

// Job reads one job with its runs and events.
func (c *Client) Job(ctx context.Context, id string) (JobDetail, error) {
	var out JobDetail
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out, err
	}
	err = c.do(ctx, http.MethodGet, "/api/jobs/"+url.PathEscape(id), q, nil, &out)
	return out, err
}

// CancelJob cancels an active job and, when its run is the agent's current
// one, that run.
func (c *Client) CancelJob(ctx context.Context, id string) (Job, error) {
	var out struct {
		Job Job `json:"job"`
	}
	q, err := c.accountQuery(ctx)
	if err != nil {
		return out.Job, err
	}
	err = c.do(ctx, http.MethodPost, "/api/jobs/"+url.PathEscape(id)+"/cancel", q, nil, &out)
	return out.Job, err
}
