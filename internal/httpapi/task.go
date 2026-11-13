package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// The typed task of the active plan, run by the account's agent (docs/10
// phase 3; docs/08, the closed list's task_run). The server decides what:
// the plan's next task, resolved to game ids, with the account's mode and
// the risks, and E9 applied before anything reaches the agent (which
// applies it again). Its termination is proven only by the ingest: GET
// /api/microbot/task evaluates it on the account's facts now.

// taskRequest is POST /api/microbot/task.
type taskRequest struct {
	// Key is the task the caller saw (GET /api/plan/next); when the plan's
	// next task is another one by now, nothing runs. Empty takes the next.
	Key string `json:"key,omitempty"`
	// Supervised: the owner is watching (E9: an HCIM with a death risk
	// runs only so).
	Supervised bool `json:"supervised,omitempty"`
	// TimeoutS bounds the plugin's machine (30 to 3600; the agent's default
	// otherwise).
	TimeoutS int `json:"timeoutS,omitempty"`
}

// handleTaskRun starts the next task of the account's active plan on its
// agent; the attempt goes to the audit, whatever its result.
func (s *Server) handleTaskRun(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var req taskRequest
	if r.ContentLength != 0 && !decodeBody(w, r, &req) {
		return
	}
	in, err := s.planInput(r.Context(), account)
	if err != nil {
		s.writePlanError(w, err)
		return
	}
	next := plan.Next(in)
	refuse := func(msg string) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": msg, "status": http.StatusConflict, "plan": next})
	}
	switch {
	case next.Status != plan.StatusTask || next.Task == nil:
		refuse("o plano não tem tarefa a executar agora (" + next.Status + "): " + next.Reason)
		return
	case req.Key != "" && req.Key != next.Task.Key:
		refuse("a próxima tarefa do plano mudou: agora é " + next.Task.Key)
		return
	case !next.Task.Executable:
		refuse(next.Task.NotExecutable)
		return
	case next.Task.Supervised && !req.Supervised:
		refuse("HCIM com risco de morte: a tarefa só roda com o dono olhando (supervised, E9)")
		return
	}
	spec, view, err := plan.Resolve(in, next.Task.Key)
	if err != nil {
		refuse(err.Error())
		return
	}
	risks := slices.Clone(view.Risks)
	for _, k := range spec.Risks() {
		if !slices.Contains(risks, k) {
			risks = append(risks, k)
		}
	}
	slices.Sort(risks)
	params, _ := json.Marshal(agent.TaskRunParams{
		Key: view.Key, Plan: in.Plan.ID, Label: view.Label, Mode: in.Facts.Mode, Risks: risks,
		Supervised: req.Supervised, TimeoutS: req.TimeoutS, Account: account, Task: spec,
	})
	if _, err = agent.ValidateParams(agent.CmdTaskRun, params); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	link := s.opts.Microbot.Link
	served, err := link.Route(account)
	if err == nil && !served.Supports(agent.CmdTaskRun) {
		err = errAgentTooOld
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err == nil && spec.Train != nil {
		// A train is delegated: the agent must end it on the server's
		// proof, and the client must have the plugin.
		if !served.Supports(agent.CmdRunUntil) {
			err = errAgentNoUntil
		} else {
			err = s.pluginInstalled(ctx, served, spec.Train.Plugin)
		}
	}
	var data json.RawMessage
	if err == nil {
		err = link.CallAgent(ctx, served.ID, agent.CmdTaskRun, json.RawMessage(params), &data)
	}
	s.auditMicrobot(r, account, served, agent.CmdTaskRun, params, err)
	var missing *pluginMissingError
	switch {
	case errors.Is(err, errAgentTooOld):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.As(err, &missing):
		refuse(missing.Error())
		return
	}
	if err != nil {
		s.writeMicrobotError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": data, "task": view})
}

var errAgentTooOld = errors.New("the agent predates task_run: update chronicle on the game machine")

// pluginMissingError: the client of the account does not have the plugin a
// train delegates to.
type pluginMissingError struct{ class string }

func (e *pluginMissingError) Error() string {
	return "o plugin " + e.class + " não está instalado no cliente da conta (pelo perfil da conta, pelo modelo da frota ou pelo Hub): instale antes de treinar"
}

// pluginInstalled asks the account's agent for the client's plugins (the
// Agent Server's /scripts, by class: its own, the Hub's it installed, the
// sideloaded ones) and refuses a plugin the client does not have. The agent
// checks again when the run starts.
func (s *Server) pluginInstalled(ctx context.Context, served agent.Info, class string) error {
	var p agent.Plugins
	if err := s.opts.Microbot.Link.CallAgent(ctx, served.ID, agent.CmdPlugins, nil, &p); err != nil {
		return err
	}
	var scripts struct {
		Scripts []struct {
			ClassName string `json:"className"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(p.Scripts, &scripts); err != nil {
		return fmt.Errorf("the client's plugins: %w", err)
	}
	for _, sc := range scripts.Scripts {
		if sc.ClassName == class {
			return nil
		}
	}
	return &pluginMissingError{class: class}
}

// taskStatus is GET /api/microbot/task: the agent's current or last run
// and, for a task run, its termination on the account's facts now.
type taskStatus struct {
	Run *agent.Run `json:"run"`
	// Proof is the run's task evaluated now; Proven is its termination met
	// by the ingest, the only proof (the run's own "done" is the plugin's
	// machine ending, never the task done).
	Proof  *plan.TaskView `json:"proof"`
	Proven bool           `json:"proven"`
	// Reason says why there is no proof (no task run, a task the plan no
	// longer has).
	Reason string `json:"reason,omitempty"`
}

func (s *Server) handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var st agent.RunStatus
	if err := s.opts.Microbot.Link.Call(ctx, account, agent.CmdRunStatus, nil, &st); err != nil {
		s.writeMicrobotError(w, err)
		return
	}
	out := taskStatus{Run: st.Run}
	switch {
	case st.Run == nil || st.Run.Task == nil:
		out.Reason = "o último run não é de uma tarefa"
	default:
		p, err := s.opts.Plans.Get(r.Context(), account, st.Run.Task.Plan)
		if err != nil {
			out.Reason = "o plano da tarefa não existe mais"
			break
		}
		in, err := s.planInputFor(r.Context(), account, p)
		if err != nil {
			s.writePlanError(w, err)
			return
		}
		v, found := plan.Find(in, st.Run.Task.Key)
		if !found {
			out.Reason = "a tarefa não está mais no plano"
			break
		}
		out.Proof, out.Proven = &v, v.State == rules.Met
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
