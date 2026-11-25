package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

// The dispatch of a plugin job (docs/12 section 13.7): the owner starts a
// queued job, the server resolves it into the typed task and sends the
// account's agent task_run with the job's id; the agent turns the plugin on
// with the job's settings, the tracker proves the goal on the ingest and sends
// run_until, and the agent turns the plugin off. The event-driven conductor
// that dispatches by itself is later work.

// errAgentNoPlugin: the agent does not run plugin tasks.
var errAgentNoPlugin = errors.New("the agent predates plugin jobs (plugin_task): update chronicle on the game machine")

// pluginJobSpec checks the spec of a plugin job and completes it: a plugin
// task whose until is the job's goal as a run waits for it (an objective key
// or a varp or varbit minimum), the name defaulting to the class's.
func pluginJobSpec(raw json.RawMessage, goal job.Goal) (json.RawMessage, error) {
	task, err := decodePluginSpec(raw)
	if err != nil {
		return nil, err
	}
	until, ok := goal.Until()
	if !ok {
		return nil, errors.New("goal: a plugin job ends on an objective key or a varp or varbit minimum, the only goals the agent waits for")
	}
	if task.Plugin.Until != "" && task.Plugin.Until != until {
		return nil, fmt.Errorf("spec: until %q is not the goal's %q (leave it out)", task.Plugin.Until, until)
	}
	task.Plugin.Until = until
	if task.Plugin.Name == "" {
		task.Plugin.Name = simpleClass(task.Plugin.Plugin)
	}
	if err := task.Check(); err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	return json.Marshal(task)
}

// decodePluginSpec reads {"type":"plugin","plugin":{...}} and nothing else.
func decodePluginSpec(raw json.RawMessage) (execspec.Task, error) {
	var task execspec.Task
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&task); err != nil {
		return task, fmt.Errorf("spec: a plugin job's spec is {\"type\":\"plugin\",\"plugin\":{\"plugin\":<class>,\"config\":[...]}}: %w", err)
	}
	if task.Type != execspec.TypePlugin || task.Plugin == nil {
		return task, errors.New("spec: a plugin job's spec is a plugin task")
	}
	return task, nil
}

// simpleClass is the class's simple name.
func simpleClass(class string) string { return class[strings.LastIndex(class, ".")+1:] }

// runJobRequest is POST /api/jobs/{id}/run.
type runJobRequest struct {
	// Supervised: the owner is watching (E9).
	Supervised bool `json:"supervised,omitempty"`
	// TimeoutS bounds the run (60 to 43200; the agent's default otherwise).
	TimeoutS int `json:"timeoutS,omitempty"`
}

// handleRunJob starts a queued (or stalled) plugin job on the account's
// agent; the attempt goes to the audit, whatever its result. A refused or
// undelivered start leaves the job as it was, so it can be started again.
func (s *Server) handleRunJob(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	m := s.opts.Microbot
	if m == nil || s.opts.Progress == nil {
		writeError(w, http.StatusServiceUnavailable, "the Microbot control module is off")
		return
	}
	var req runJobRequest
	if r.ContentLength != 0 && !decodeBody(w, r, &req) {
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
	conflict := func(msg string) { writeError(w, http.StatusConflict, msg) }
	if j.Kind != job.KindPlugin {
		conflict("only a plugin job is started here; the other kinds run from the plan's task")
		return
	}
	if j.State != job.Queued && j.State != job.Stalled {
		conflict("the job is " + string(j.State) + ": only a queued or stalled job starts")
		return
	}
	if active, aerr := s.opts.Jobs.ActiveRuns(r.Context(), j.ID); aerr != nil {
		writeError(w, http.StatusInternalServerError, "reading the job's runs")
		return
	} else if len(active) > 0 {
		conflict("the job already has a run going")
		return
	}
	task, err := decodePluginSpec(j.Spec)
	if err == nil {
		if until, _ := j.Goal.Until(); until != task.Plugin.Until {
			err = errors.New("spec: the plugin task does not wait for the job's goal")
		} else {
			err = task.Check()
		}
	}
	if err != nil {
		conflict("the job's spec is not a runnable plugin task: " + err.Error())
		return
	}
	facts, err := s.opts.Progress.TaskFacts(r.Context(), account)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reading the account's facts")
		return
	}
	// A plugin's risks are unknown (any one may fight), so an HCIM runs a
	// plugin job only with the owner watching (docs/10 E9).
	if facts.Mode == content.ModeHCIM && !req.Supervised {
		conflict("an HCIM runs a plugin job only with the owner watching (supervised, docs/10 E9)")
		return
	}
	params, _ := json.Marshal(agent.TaskRunParams{
		JobID: j.ID, Label: task.Plugin.Name, Mode: facts.Mode, Supervised: req.Supervised,
		TimeoutS: req.TimeoutS, Account: account, Task: task,
	})
	if _, err = agent.ValidateParams(agent.CmdTaskRun, params); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	served, err := m.Link.Route(account)
	if err == nil {
		switch {
		case !served.Supports(agent.CmdTaskRun):
			err = errAgentTooOld
		case !served.Supports(agent.CmdRunUntil):
			err = errAgentNoUntil
		case !served.Has(agent.AgentFeaturePlugin) || !served.Has(agent.AgentFeatureJobID):
			err = errAgentNoPlugin
		default:
			err = s.pluginInstalled(ctx, served, task.Plugin.Plugin)
		}
	}
	var data json.RawMessage
	if err == nil {
		err = m.Link.CallAgent(ctx, served.ID, agent.CmdTaskRun, json.RawMessage(params), &data)
	}
	s.auditMicrobot(r, account, served, agent.CmdTaskRun, params, err)
	var missing *pluginMissingError
	switch {
	case errors.Is(err, errAgentTooOld), errors.Is(err, errAgentNoPlugin):
		conflict(err.Error())
	case errors.As(err, &missing):
		conflict(missing.Error())
	case err != nil:
		s.writeMicrobotError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": data, "job": j.ID})
	}
}
