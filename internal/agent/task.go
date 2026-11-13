package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

// The typed task (docs/10 phase 3, sections 10 and 13): the server decides
// what (the plan's next task, resolved to game ids, with the account's mode
// and the risks) and the Chronicle Link plugin decides how (a local state
// machine over Microbot's primitives, started through its bridge; nothing
// clicks over the network). The agent runs it as a run: the per-client lock,
// the safe point, the trail, the timeout, the cancel and the history. The
// run ending "done" means the plugin's machine finished, never that the
// task is done: its termination is proven by the ingest (the position in
// the area, the item containers), which the server evaluates.
const CmdTaskRun = "task_run"

// ChronicleLinkClass is the plugin that executes the task (Run.ClassName of
// a task run).
const ChronicleLinkClass = "net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin"

// TaskBridgeVersion is the first Chronicle Link whose bridge executes tasks.
const TaskBridgeVersion = "1.15.0"

// TaskRunParams are the parameters of CmdTaskRun.
type TaskRunParams struct {
	// Key and Plan name the task in the account's plan (internal/core/plan):
	// the server proves its termination by them.
	Key  string `json:"key"`
	Plan string `json:"plan"`
	// Label is what the trail calls the task (PT-BR).
	Label string `json:"label"`
	// Mode is the account's mode and Risks the task's risk kinds (its
	// recipe's and the wilderness): with them the agent applies E9 again.
	Mode  string   `json:"mode"`
	Risks []string `json:"risks,omitempty"`
	// Supervised: the owner says they are watching (E9: an HCIM with a
	// death risk runs only so).
	Supervised bool `json:"supervised,omitempty"`
	// TimeoutS bounds the plugin's machine (default 900 for a travel, 300
	// otherwise; 30 to 3600); a train's plugin (default 7200, 60 to 43200).
	TimeoutS int `json:"timeoutS,omitempty"`
	// Account is the account whose facts prove a train's level (the
	// server's word, as a wait_until's); required for a train.
	Account string `json:"account,omitempty"`
	// Task is what the plugin executes.
	Task execspec.Task `json:"task"`
}

// RunTask is the task a run executes.
type RunTask struct {
	Key        string `json:"key"`
	Plan       string `json:"plan"`
	Type       string `json:"type"`
	Label      string `json:"label"`
	Supervised bool   `json:"supervised,omitempty"`
}

var (
	reTaskKey  = regexp.MustCompile(`^(step|recipe|task):[\x21-\x7e]{1,280}#[0-9]{1,4}$`)
	rePlanID   = regexp.MustCompile(`^(route:[a-z0-9-]{1,64}|pln_[a-z0-9]{8,32})$`)
	taskStates = []string{"running", "done", "failed", "canceled"}
)

const (
	taskTravelTimeout = 15 * time.Minute
	taskTimeout       = 5 * time.Minute
	// taskBridgeMisses is how many bridge reads in a row may fail before
	// the run gives up (the plugin restarted, the client went down).
	taskBridgeMisses = 10
	// maxTaskEvents bounds the plugin's lines a run notes (a report holds 500).
	maxTaskEvents = 200
)

func validateTaskRun(raw json.RawMessage) (TaskRunParams, error) {
	var p TaskRunParams
	if err := decodeParams(raw, &p); err != nil {
		return p, err
	}
	if err := checkRunTask(RunTask{Key: p.Key, Plan: p.Plan, Type: p.Task.Type, Label: p.Label}); err != nil {
		return p, err
	}
	if !slices.Contains(content.Modes, p.Mode) {
		return p, fmt.Errorf("task: mode must be one of %s", strings.Join(content.Modes, ", "))
	}
	if len(p.Risks) > len(content.RiskKinds) {
		return p, errors.New("task: too many risks")
	}
	for i, r := range p.Risks {
		if !execspec.KnownRisk(r) || slices.Contains(p.Risks[:i], r) {
			return p, fmt.Errorf("task: unknown or repeated risk %q", r)
		}
	}
	if p.Account != "" && !reAccountID.MatchString(p.Account) {
		return p, errors.New("task: bad account")
	}
	if p.Task.Type == execspec.TypeTrain {
		// A train waits for the server's proof of its level: hours, on
		// the account's facts.
		if p.Account == "" {
			return p, errors.New("task: a train needs the account whose facts prove its level")
		}
		if p.TimeoutS != 0 && (p.TimeoutS < 60 || p.TimeoutS > untilMaxTimeoutS) {
			return p, fmt.Errorf("task: a train's timeoutS between 60 and %d", untilMaxTimeoutS)
		}
	} else if p.TimeoutS != 0 && (p.TimeoutS < 30 || p.TimeoutS > 3600) {
		return p, errors.New("task: timeoutS between 30 and 3600")
	}
	if err := p.Task.Check(); err != nil {
		return p, err
	}
	// The task's own risks (a travel into the wilderness) are declared,
	// whatever the server says.
	for _, r := range p.Task.Risks() {
		if !slices.Contains(p.Risks, r) {
			return p, fmt.Errorf("task: the %s risk of the task is not declared", r)
		}
	}
	if execspec.NeedsSupervision(p.Mode, p.Risks) && !p.Supervised {
		return p, fmt.Errorf("task: an HCIM with a death risk (%s) runs only with the owner watching (supervised, docs/10 E9)", strings.Join(p.Risks, ", "))
	}
	return p, nil
}

// checkRunTask verifies the task part of a run (the params and a report).
func checkRunTask(t RunTask) error {
	switch {
	case !reTaskKey.MatchString(t.Key):
		return errors.New("task: bad key")
	case !rePlanID.MatchString(t.Plan):
		return errors.New("task: bad plan")
	case !slices.Contains(content.TaskTypes, t.Type):
		return fmt.Errorf("task: unknown type %q", t.Type)
	case strings.TrimSpace(t.Label) == "" || len(t.Label) > 200 || strings.IndexFunc(t.Label, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0:
		return errors.New("task: label of 1 to 200 printable characters, one line")
	}
	return nil
}

// taskRun starts a task run.
func (e *Executor) taskRun(raw json.RawMessage) (any, error) {
	p, err := validateTaskRun(raw)
	if err != nil {
		return nil, err
	}
	if e.Bridge == nil {
		return nil, errors.New("task: no Chronicle Link bridge (the executor runs only in the harness's client)")
	}
	run := Run{
		ClassName: ChronicleLinkClass,
		Task:      &RunTask{Key: p.Key, Plan: p.Plan, Type: p.Task.Type, Label: p.Label, Supervised: p.Supervised},
	}
	drive := func(ctx context.Context) { e.driveTask(ctx, p) }
	if tr := p.Task.Train; tr != nil {
		// Delegated: the run is the plugin's, and ends on the server's
		// proof of the level (wait_until).
		run.ClassName = tr.Plugin
		run.Until = &RunUntil{Objective: tr.Until(), Account: p.Account}
		drive = func(ctx context.Context) { e.driveTrain(ctx, p) }
	}
	why := "tarefa: " + p.Label
	if len(p.Risks) > 0 {
		why += " (riscos: " + strings.Join(p.Risks, ", ")
		if p.Supervised {
			why += "; com o dono olhando"
		}
		why += ")"
	}
	r, err := e.launch(run, why, drive)
	if err != nil {
		return nil, err
	}
	return RunStatus{Run: r}, nil
}

// bridgeTask is the plugin's machine as GET /bridge/task answers it.
type bridgeTask struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	Events []struct {
		Seq  int    `json:"seq"`
		Text string `json:"text"`
	} `json:"events"`
}

// driveTask is a task run's goroutine: the safe point, the start on the
// bridge, then the plugin's machine read every poll until it ends, the
// timeout or the cancel (which cancels the machine too).
func (e *Executor) driveTask(ctx context.Context, p TaskRunParams) {
	uid := e.currentUID()
	stop := func() {
		// The machine stops on the bridge whatever stopped the run.
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = e.Bridge.JSON(sctx, http.MethodPost, "/bridge/task/cancel", nil, map[string]any{"id": uid}, 15*time.Second)
	}
	fail := func(err error) {
		if ctx.Err() != nil {
			e.end(RunCanceled, "cancelado pelo dono; a tarefa parou onde estava", "")
			return
		}
		e.end(RunFailed, err.Error()+"; a tarefa parou onde estava", err.Error())
	}
	if err := e.waitSafe(ctx); err != nil {
		fail(err)
		return
	}
	st, err := e.runState(ctx)
	if err != nil {
		fail(err)
		return
	}
	if !st.LoggedIn || st.Player.Position == nil {
		fail(errors.New("the character is not logged in"))
		return
	}
	at := *st.Player.Position
	if execspec.InWilderness(at.X, at.Y) && p.Mode == content.ModeHCIM && !p.Supervised {
		fail(errors.New("the character is in the wilderness: an HCIM runs a task there only with the owner watching"))
		return
	}
	if v := e.bridgeVersion(ctx); !versionAtLeast(v, TaskBridgeVersion) {
		fail(fmt.Errorf("the bridge answered %q: tasks need Chronicle Link %s or newer", v, TaskBridgeVersion))
		return
	}
	e.note(RunRunning, fmt.Sprintf("em %d, %d, %d; começando: %s", at.X, at.Y, at.Plane, p.Label))
	if others := e.activeScripts(ctx); len(others) > 0 {
		// Plugins keep passive loops all the time, so this only warns: a
		// main activity left on would click alongside the task.
		e.note(RunRunning, "plugins ligados durante a tarefa: "+strings.Join(others, ", ")+" (pare a atividade principal antes)")
	}
	if _, err = e.Bridge.JSON(ctx, http.MethodPost, "/bridge/task/start", nil, map[string]any{"id": uid, "task": p.Task}, 15*time.Second); err != nil {
		fail(fmt.Errorf("the bridge did not start the task: %w", err))
		return
	}
	timeout := taskTimeout
	if p.Task.Type == execspec.TypeTravel {
		timeout = taskTravelTimeout
	}
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	if e.taskTimeout > 0 {
		timeout = e.taskTimeout
	}
	deadline := time.Now().Add(timeout)
	seen, noted, misses := 0, 0, 0
	for {
		if !e.sleep(ctx) {
			stop()
			fail(ctx.Err())
			return
		}
		raw, rerr := e.Bridge.JSON(ctx, http.MethodGet, "/bridge/task", nil, nil, 10*time.Second)
		var bt bridgeTask
		if rerr == nil {
			rerr = json.Unmarshal(raw, &bt)
		}
		if rerr != nil {
			if misses++; misses >= taskBridgeMisses {
				stop()
				fail(fmt.Errorf("the bridge stopped answering: %w", rerr))
				return
			}
			continue
		}
		misses = 0
		if bt.ID != uid {
			fail(errors.New("the bridge lost the task (the plugin restarted or another task replaced it)"))
			return
		}
		for _, ev := range bt.Events {
			if ev.Seq <= seen {
				continue
			}
			seen = ev.Seq
			if noted < maxTaskEvents {
				e.note(RunRunning, clip(ev.Text, 300))
				noted++
			}
		}
		switch bt.State {
		case "done":
			e.end(RunDone, "o executor terminou; o término é provado pelo ingest, não pelo plugin", "")
			return
		case "failed":
			fail(errors.New(clip(nonEmpty(bt.Reason, "the plugin's machine failed"), 500)))
			return
		case "canceled":
			fail(errors.New(clip("the plugin canceled the task: "+nonEmpty(bt.Reason, "no reason"), 500)))
			return
		}
		if !slices.Contains(taskStates, bt.State) {
			stop()
			fail(fmt.Errorf("the bridge answered an unknown state %q", bt.State))
			return
		}
		if time.Now().After(deadline) {
			stop()
			fail(fmt.Errorf("the task did not finish in %s", timeout))
			return
		}
	}
}

// activeScripts are the simple names of the active plugins besides the
// control path and Chronicle Link (clientcfg.ProtectedClasses), from the Agent
// Server's /scripts; nil when it does not answer.
func (e *Executor) activeScripts(ctx context.Context) []string {
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/scripts", nil, nil, 15*time.Second)
	if err != nil {
		return nil
	}
	var s struct {
		Scripts []struct {
			ClassName string `json:"className"`
			Active    bool   `json:"active"`
		} `json:"scripts"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	var out []string
	for _, sc := range s.Scripts {
		if sc.Active && reClassName.MatchString(sc.ClassName) && !Protected(sc.ClassName) {
			out = append(out, simple(sc.ClassName))
		}
	}
	slices.Sort(out)
	return out
}

// currentUID is the uid of the run being driven.
func (e *Executor) currentUID() string {
	e.runs.mu.Lock()
	defer e.runs.mu.Unlock()
	if e.runs.run == nil {
		return ""
	}
	return e.runs.run.UID
}

// versionAtLeast compares dotted numeric versions ("1.15.0" >= "1.15.0");
// an empty or malformed version is never enough.
func versionAtLeast(v, least string) bool {
	parse := func(s string) ([]int, bool) {
		parts := strings.Split(strings.TrimSpace(s), ".")
		out := make([]int, 0, len(parts))
		for _, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 {
				return nil, false
			}
			out = append(out, n)
		}
		return out, len(out) > 0
	}
	a, ok := parse(v)
	b, okB := parse(least)
	if !ok || !okB {
		return false
	}
	for i := 0; i < max(len(a), len(b)); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return true
}

// clip cuts s to at most n bytes, on a rune boundary.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
