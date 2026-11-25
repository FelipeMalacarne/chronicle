package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

// JobStore stores the accounts' jobs (docs/12 section 13, internal/store/jobs).
type JobStore interface {
	Create(ctx context.Context, j job.Job) (job.Job, error)
	Get(ctx context.Context, account, id string) (job.Job, error)
	List(ctx context.Context, account, state string, limit int) ([]job.Job, error)
	Cancel(ctx context.Context, account, id, reason string) (job.Job, error)
	ActiveRuns(ctx context.Context, id string) ([]string, error)
	// ProvenSales reads a plan's proven Grand Exchange sales: the job that
	// proved each (plan.Input.SalesProven).
	ProvenSales(ctx context.Context, account, planID string) (map[string]string, error)
}

// JobEventStore reads a job's events (internal/store/eventlog).
type JobEventStore interface {
	OfJob(ctx context.Context, account, jobID string, limit int) ([]eventlog.Event, error)
}

// jobRoutes are the account's jobs. Account-addressed like the others (rule
// 10): behind authed and accountParam, never outside them.
func (s *Server) jobRoutes() {
	if s.opts.Jobs == nil || !s.accountRoutes() {
		return
	}
	account := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.authed(h)) }
	account("GET /api/jobs", s.handleJobs)
	account("GET /api/jobs/{id}", s.handleJob)
	account("POST /api/jobs", s.handleCreateJob)
	account("POST /api/jobs/{id}/cancel", s.handleCancelJob)
}

// jobView is a job as the API shows it.
type jobView struct {
	ID         string          `json:"id"`
	Account    string          `json:"account"`
	Kind       job.Kind        `json:"kind"`
	State      job.State       `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	Source     job.Source      `json:"source"`
	Spec       json.RawMessage `json:"spec"`
	Goal       job.Goal        `json:"goal"`
	Metric     *job.Metric     `json:"metric,omitempty"`
	Progress   *job.Progress   `json:"progress,omitempty"`
	StallAfter int             `json:"stall_after_s"`
	Plan       string          `json:"plan,omitempty"`
	TaskKey    string          `json:"task_key,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	DeadlineAt *time.Time      `json:"deadline_at,omitempty"`
}

func viewOf(j job.Job) jobView {
	return jobView{
		ID: j.ID, Account: j.Account, Kind: j.Kind, State: j.State, Reason: j.Reason, Source: j.Source,
		Spec: j.Spec, Goal: j.Goal, Metric: j.Metric, Progress: j.Progress, StallAfter: int(j.StallAfter / time.Second),
		Plan: j.Plan, TaskKey: j.TaskKey, CreatedAt: j.CreatedAt, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
		DeadlineAt: j.Deadline,
	}
}

// jobPage is how many jobs a listing returns by default and at most.
const (
	jobPage    = 50
	jobPageMax = 200
	// jobEventsMax bounds the events of one job's detail.
	jobEventsMax = 500
)

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	state := r.URL.Query().Get("state")
	if state != "" && !job.State(state).Active() && !job.State(state).Final() {
		writeError(w, http.StatusBadRequest, "state must be one of queued, running, stalled, succeeded, failed, canceled")
		return
	}
	limit := jobPage
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > jobPageMax {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(jobPageMax))
			return
		}
		limit = n
	}
	jobs, err := s.opts.Jobs.List(r.Context(), account, state, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing the jobs")
		return
	}
	out := make([]jobView, len(jobs))
	for i, j := range jobs {
		out[i] = viewOf(j)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"account": account, "jobs": out})
}

// jobDetail is GET /api/jobs/{id}: the job, the runs that attempted it and
// its events, oldest first.
type jobDetail struct {
	Job    jobView          `json:"job"`
	Runs   []agent.Run      `json:"runs"`
	Events []eventlog.Event `json:"events"`
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	j, err := s.opts.Jobs.Get(r.Context(), account, r.PathValue("id"))
	if errors.Is(err, storejobs.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reading the job")
		return
	}
	out := jobDetail{Job: viewOf(j), Runs: []agent.Run{}, Events: []eventlog.Event{}}
	if m := s.opts.Microbot; m != nil && m.Routines != nil {
		if runs, err := m.Routines.RunsOfJob(r.Context(), account, j.ID); err == nil {
			out.Runs = runs
		}
	}
	if s.opts.JobEvents != nil {
		evs, err := s.opts.JobEvents.OfJob(r.Context(), account, j.ID, jobEventsMax)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "reading the job's events")
			return
		}
		out.Events = evs
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// createJobRequest is POST /api/jobs. The metric defaults to the goal's own
// and stall_after_s to the kind's.
type createJobRequest struct {
	Kind        job.Kind        `json:"kind"`
	Spec        json.RawMessage `json:"spec,omitempty"`
	Goal        job.Goal        `json:"goal"`
	Metric      *job.Metric     `json:"metric,omitempty"`
	StallAfterS int             `json:"stall_after_s,omitempty"`
}

// handleCreateJob queues a job of the owner's, never one of a plan's task (the
// plan reads a ge_sell job's proof by its plan and task key; the server's task
// run is the only one that sets them). Running it is the conductor's
// work (and, for a plugin, the dispatch of the client's profile): this only
// records the intent for the tracker to prove.
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var req createJobRequest
	if !decodeBody(w, r, &req) {
		return
	}
	j, err := job.New(account, req.Kind, req.Spec, req.Goal, req.Metric, time.Duration(req.StallAfterS)*time.Second, job.SourceOwner)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.opts.Jobs.Create(r.Context(), j)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating the job")
		return
	}
	writeJSON(w, http.StatusCreated, viewOf(created))
}

// cancelJobResponse is POST /api/jobs/{id}/cancel.
type cancelJobResponse struct {
	Job jobView `json:"job"`
	// RunCanceled: the job's run was the agent's current one and was told to stop.
	RunCanceled bool `json:"run_canceled"`
}

// handleCancelJob cancels an active job and, when its run is the one the
// account's agent is on, stops that run.
func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	j, err := s.opts.Jobs.Cancel(r.Context(), account, id, "canceled by the owner")
	switch {
	case errors.Is(err, storejobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "job not found")
		return
	case errors.Is(err, storejobs.ErrState):
		writeError(w, http.StatusConflict, "the job already ended")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "canceling the job")
		return
	}
	out := cancelJobResponse{Job: viewOf(j), RunCanceled: s.stopJobRun(r, account, j)}
	writeJSON(w, http.StatusOK, out)
}

// stopJobRun cancels the agent's current run when it attempts the job. The
// job is canceled whatever happens here: an agent that is away or on another
// run leaves nothing to stop.
func (s *Server) stopJobRun(r *http.Request, account string, j job.Job) bool {
	m := s.opts.Microbot
	if m == nil {
		return false
	}
	uids, err := s.opts.Jobs.ActiveRuns(r.Context(), j.ID)
	if err != nil || len(uids) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	served, err := m.Link.Route(account)
	if err != nil {
		return false
	}
	var st agent.RunStatus
	if cerr := m.Link.CallAgent(ctx, served.ID, agent.CmdRunStatus, nil, &st); cerr != nil || st.Run == nil || st.Run.Final() {
		return false
	}
	current := false
	for _, uid := range uids {
		current = current || uid == st.Run.UID
	}
	if !current {
		return false
	}
	err = m.Link.CallAgent(ctx, served.ID, agent.CmdRunCancel, nil, nil)
	s.auditMicrobot(r, account, served, agent.CmdRunCancel, nil, err)
	return err == nil
}

// taskGoal is the goal that proves a plan task the server runs: the key of a
// train or a quest, the sale's rule for a ge_sell, the content's own
// termination for the rest (a key, or the task for the panel's evaluation).
func taskGoal(spec execspec.Task, task content.Task) (job.Goal, error) {
	switch {
	case spec.GESell != nil:
		return job.SaleGoal(spec.GESell.Item, spec.GESell.CoinsAtLeast)
	case spec.Train != nil:
		return job.ParseGoal(mustJSON(spec.Train.Until()))
	case spec.Quest != nil:
		return job.ParseGoal(mustJSON(spec.Quest.Until()))
	}
	u, ok := task.Ends()
	if !ok {
		return job.Goal{}, errors.New("a tarefa não diz quando termina")
	}
	if u.Objective != nil {
		return job.ObjectiveGoal(*u.Objective), nil
	}
	return job.TaskGoal(task)
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// createTaskJob queues the job a task run attempts and returns its id.
func (s *Server) createTaskJob(ctx context.Context, account, planID, taskKey string, task content.Task, spec execspec.Task) (string, error) {
	goal, err := taskGoal(spec, task)
	if err != nil {
		return "", fmt.Errorf("goal of the task: %w", err)
	}
	raw, _ := json.Marshal(spec)
	j, err := job.New(account, job.Kind(spec.Type), raw, goal, nil, 0, job.SourceOwner)
	if err != nil {
		return "", err
	}
	j.Plan, j.TaskKey = planID, taskKey
	created, err := s.opts.Jobs.Create(ctx, j)
	if err != nil {
		return "", fmt.Errorf("creating the job: %w", err)
	}
	return created.ID, nil
}

// notDelivered reports whether a failed task_run certainly did not start a run:
// the agent refused it, or there was no agent to take it. Any other error (a
// timeout, a lost link) may have started it, and the run's report links the job.
func notDelivered(err error) bool {
	var refused *agent.CommandError
	return errors.As(err, &refused) || errors.Is(err, agent.ErrNoAgent) || errors.Is(err, agent.ErrAmbiguousAgent)
}

// dropTaskJob cancels the job of a task run the agent did not start.
func (s *Server) dropTaskJob(account, id string, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reason := "the run did not start: " + cause.Error()
	if len(reason) > 200 {
		reason = reason[:200]
	}
	if _, err := s.opts.Jobs.Cancel(ctx, account, id, reason); err != nil {
		slog.Warn("jobs: cancel the job of a run that did not start", "job", id, "err", err)
	}
}
