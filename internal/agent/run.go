package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// "Fazer agora" (docs/08 phase 4, delivery 2): one run of a plugin between
// the main activity and the return, executed by the agent so it survives the
// server restarting or the link dropping. The run waits for a safe point
// (safe.go), stops the plugins it is
// told to (the main activity), waits for the character to settle, starts the
// run plugin and waits for it to switch itself off, walks back to where the
// character stood and starts the stopped plugins again. A failure, a timeout
// or a cancel leaves everything stopped for the owner to look at.
const (
	CmdRunStart  = "run_start"  // start one run; refused while another is going
	CmdRunStatus = "run_status" // the current or last run and its trail
	CmdRunCancel = "run_cancel" // stop the run plugin, leave the rest stopped
)

// Run states, in order; the last three are final.
const (
	RunWaiting    = "waiting" // for a safe point (safe.go)
	RunStopping   = "stopping"
	RunRunning    = "running"
	RunReturning  = "returning"
	RunRestarting = "restarting"
	RunDone       = "done"
	RunFailed     = "failed"
	RunCanceled   = "canceled"
)

// RunParams are the parameters of CmdRunStart.
type RunParams struct {
	// ClassName is the plugin to run; it must stop by itself when it is done
	// (Cat Swap does) and may leave its outcome in its script results, where
	// {"outcome":"failed"} fails the run.
	ClassName string `json:"className"`
	// Stop are the plugins to stop during the run and start again after it
	// (the main activity); the ones left out keep running (the kitten).
	Stop []string `json:"stop,omitempty"`
	// TimeoutS bounds the run plugin (default 1800, 60 to 7200).
	TimeoutS int `json:"timeoutS,omitempty"`
	// Stay skips the walk back.
	Stay bool `json:"stay,omitempty"`
}

// RunEvent is one line of a run's trail.
type RunEvent struct {
	At    time.Time `json:"at"`
	State string    `json:"state"`
	Text  string    `json:"text"`
}

// Run is a run and its trail.
type Run struct {
	ID        string   `json:"id"`
	ClassName string   `json:"className"`
	Stop      []string `json:"stop"`
	// Trigger is the game event that started the run (EventKittenGrown); empty
	// when the owner did.
	Trigger    string     `json:"trigger,omitempty"`
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	// From is where the character stood when the run began.
	From   *WalkParams     `json:"from,omitempty"`
	Result json.RawMessage `json:"result,omitempty"` // the run plugin's last script result
	Error  string          `json:"error,omitempty"`
	Events []RunEvent      `json:"events"`
}

// RunStatus is the answer to CmdRunStatus; Run is nil before the first run.
type RunStatus struct {
	Run *Run `json:"run"`
}

// Final reports whether the run ended.
func (r *Run) Final() bool {
	return r.State == RunDone || r.State == RunFailed || r.State == RunCanceled
}

// readCommands are what the owner may still do while a run drives the character.
var readCommands = []string{
	CmdStatus, CmdScreenshot, CmdVNCOpen, CmdPlugins, CmdHubSearch, CmdScriptDetail, CmdConfigGet,
	CmdConfigSchema, CmdGameView, CmdClientLogs, CmdAccountShow, CmdTimers, CmdRunStatus, CmdRunHistory, CmdRunCancel,
	// Not reads, but they never touch the game: switching the trigger off during its own run is fine.
	CmdTriggerGet, CmdTriggerSet,
}

// ErrRunGoing refuses an action while a run drives the character.
var ErrRunGoing = errors.New("a run is driving the character: cancel it first")

const (
	runDefaultTimeout = 30 * time.Minute
	// How long the character may take to settle after the main activity stops.
	runSettle = 30 * time.Second
	// How long the walk back may take, and how many times it is reissued when
	// the character stands still away from the spot.
	runWalkTimeout = 15 * time.Minute
	runWalkTries   = 5
	// Within this many tiles of the spot the walk back has arrived.
	runArrived = 3
)

// runner holds the agent's one run. Zero value ready.
type runner struct {
	mu     sync.Mutex
	run    *Run
	cancel context.CancelFunc
	seq    int
	// The history (history.go): the runs before run, oldest first.
	loaded  bool
	past    []Run
	saveErr string
}

func validateRun(raw json.RawMessage) (RunParams, error) {
	var p RunParams
	if err := decodeParams(raw, &p); err != nil {
		return p, err
	}
	check := func(c string) error {
		if len(c) > 256 || !reClassName.MatchString(c) {
			return errors.New("run: className must be a fully qualified class name")
		}
		if Protected(c) {
			return fmt.Errorf("run: %s is not controlled from the planner", c)
		}
		return nil
	}
	if err := check(p.ClassName); err != nil {
		return p, err
	}
	if len(p.Stop) > 8 {
		return p, errors.New("run: at most 8 plugins to stop")
	}
	for i, c := range p.Stop {
		if err := check(c); err != nil {
			return p, err
		}
		if c == p.ClassName || slices.Contains(p.Stop[:i], c) {
			return p, errors.New("run: a plugin to stop is repeated or is the run plugin")
		}
	}
	if p.TimeoutS != 0 && (p.TimeoutS < 60 || p.TimeoutS > 7200) {
		return p, errors.New("run: timeoutS between 60 and 7200")
	}
	return p, nil
}

// runCommand serves CmdRunStart, CmdRunStatus and CmdRunCancel.
func (e *Executor) runCommand(command string, raw json.RawMessage) (any, error) {
	r := &e.runs
	switch command {
	case CmdRunStatus:
		r.mu.Lock()
		defer r.mu.Unlock()
		e.loadRuns()
		return RunStatus{Run: r.snapshot()}, nil
	case CmdRunHistory:
		return e.runHistory(), nil
	case CmdRunCancel:
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.run == nil || r.run.Final() {
			return nil, errors.New("run: nothing is running")
		}
		r.cancel()
		return RunStatus{Run: r.snapshot()}, nil
	}
	p, err := validateRun(raw)
	if err != nil {
		return nil, err
	}
	run, err := e.startRun(p, "")
	if err != nil {
		return nil, err
	}
	return RunStatus{Run: run}, nil
}

// startRun starts a run of validated params; trigger names the event that
// fired it ("" when the owner did).
func (e *Executor) startRun(p RunParams, trigger string) (*Run, error) {
	r := &e.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	e.loadRuns()
	if r.run != nil && !r.run.Final() {
		return nil, errors.New("run: another run is going")
	}
	r.retire()
	r.seq++
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.run = &Run{
		ID:        "run-" + strconv.Itoa(r.seq),
		ClassName: p.ClassName,
		Stop:      append([]string{}, p.Stop...),
		Trigger:   trigger,
		State:     RunWaiting,
		StartedAt: time.Now().UTC(),
		Events:    []RunEvent{},
	}
	e.saveRuns()
	go e.drive(ctx, p)
	return r.snapshot(), nil
}

// going reports whether a run is driving the character.
func (e *Executor) going() bool {
	e.runs.mu.Lock()
	defer e.runs.mu.Unlock()
	return e.runs.run != nil && !e.runs.run.Final()
}

// snapshot copies the run under the lock.
func (r *runner) snapshot() *Run {
	if r.run == nil {
		return nil
	}
	c := *r.run
	c.Stop = slices.Clone(r.run.Stop)
	c.Events = slices.Clone(r.run.Events)
	return &c
}

func (e *Executor) note(state, text string) {
	r := &e.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	r.run.State = state
	r.run.Events = append(r.run.Events, RunEvent{At: time.Now().UTC(), State: state, Text: text})
	e.saveRuns()
}

// end finishes the run in one step under the lock: the final state, the
// event, FinishedAt and the error. A run is Final() as soon as its state is,
// and a new run may start right then, so finishing in two steps let a late
// half write into, and cancel, the next run.
func (e *Executor) end(state, text, errText string) {
	r := &e.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.run.State = state
	r.run.Events = append(r.run.Events, RunEvent{At: now, State: state, Text: text})
	r.run.FinishedAt = &now
	r.run.Error = errText
	e.saveRuns()
	r.cancel()
}

func (e *Executor) poll() time.Duration {
	if e.RunPoll > 0 {
		return e.RunPoll
	}
	return 3 * time.Second
}

// drive is the run's goroutine.
func (e *Executor) drive(ctx context.Context, p RunParams) {
	fail := func(err error) {
		if ctx.Err() != nil {
			e.end(RunCanceled, "cancelado pelo dono; o que estava parado continua parado", "")
			return
		}
		e.end(RunFailed, err.Error()+"; o que estava parado continua parado", err.Error())
	}
	// Nothing moves before the character is at a safe point.
	if err := e.waitSafe(ctx); err != nil {
		fail(err)
		return
	}
	// PARANDO: where the character is, stop the main activity, let it settle.
	st, err := e.runState(ctx)
	if err != nil {
		fail(err)
		return
	}
	if !st.LoggedIn || st.Player.Position == nil {
		fail(errors.New("the character is not logged in"))
		return
	}
	from := *st.Player.Position
	e.runs.mu.Lock()
	e.runs.run.From = &from
	e.runs.mu.Unlock()
	e.note(RunStopping, fmt.Sprintf("posição guardada: %d, %d, %d", from.X, from.Y, from.Plane))
	for _, c := range p.Stop {
		if _, err = e.Game.JSON(ctx, http.MethodPost, "/scripts/stop", nil, map[string]any{"className": c}, 30*time.Second); err != nil {
			fail(fmt.Errorf("stopping %s: %w", c, err))
			return
		}
		e.note(RunStopping, "parado: "+simple(c))
	}
	if !e.settle(ctx) {
		if ctx.Err() != nil {
			fail(ctx.Err())
			return
		}
		e.note(RunStopping, "o personagem não ficou parado em 30 s; seguindo")
	}

	// RUN: start the plugin and wait for it to switch itself off.
	before, _ := e.results(ctx, p.ClassName)
	if _, err = e.Game.JSON(ctx, http.MethodPost, "/scripts/start", nil, map[string]any{"className": p.ClassName}, 30*time.Second); err != nil {
		fail(fmt.Errorf("starting %s: %w", p.ClassName, err))
		return
	}
	e.note(RunRunning, "iniciado: "+simple(p.ClassName))
	timeout := runDefaultTimeout
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	if err = e.waitInactive(ctx, p.ClassName, timeout); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _ = e.Game.JSON(stopCtx, http.MethodPost, "/scripts/stop", nil, map[string]any{"className": p.ClassName}, 30*time.Second)
		cancel()
		fail(err)
		return
	}
	after, last := e.results(ctx, p.ClassName)
	if after > before && last != nil {
		e.runs.mu.Lock()
		e.runs.run.Result = last
		e.runs.mu.Unlock()
		var out struct {
			Outcome string `json:"outcome"`
			Reason  string `json:"reason"`
		}
		_ = json.Unmarshal(last, &out)
		if out.Outcome == "failed" {
			fail(fmt.Errorf("%s failed: %s", p.ClassName, out.Reason))
			return
		}
	}
	e.note(RunRunning, "terminou: "+simple(p.ClassName))

	// RETORNO: walk back to the spot.
	if !p.Stay {
		if err = e.walkBack(ctx, from); err != nil {
			fail(err)
			return
		}
	}

	// Start the main activity again.
	for _, c := range p.Stop {
		if _, err = e.Game.JSON(ctx, http.MethodPost, "/scripts/start", nil, map[string]any{"className": c}, 30*time.Second); err != nil {
			fail(fmt.Errorf("restarting %s: %w", c, err))
			return
		}
		e.note(RunRestarting, "religado: "+simple(c))
	}
	e.end(RunDone, "feito", "")
}

// runPlayer is the part of the Agent Server's /state a run reads.
type runPlayer struct {
	LoggedIn bool `json:"loggedIn"`
	Player   struct {
		// HealthRatio is -1 unless the health bar shows (a fight).
		HealthRatio *int        `json:"healthRatio"`
		Animating   bool        `json:"animating"`
		Moving      bool        `json:"moving"`
		Position    *WalkParams `json:"position"`
	} `json:"player"`
}

func (e *Executor) runState(ctx context.Context) (runPlayer, error) {
	var s runPlayer
	raw, err := e.Game.State(ctx)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	return s, err
}

// sleep waits one poll; false when the run was canceled.
func (e *Executor) sleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(e.poll()):
		return true
	}
}

// settle waits until the character neither moves nor animates on two reads in a row.
func (e *Executor) settle(ctx context.Context) bool {
	still := 0
	for deadline := time.Now().Add(runSettle); time.Now().Before(deadline); {
		if st, err := e.runState(ctx); err == nil && st.LoggedIn && !st.Player.Moving && !st.Player.Animating {
			if still++; still >= 2 {
				return true
			}
		} else {
			still = 0
		}
		if !e.sleep(ctx) {
			return false
		}
	}
	return false
}

// results counts the plugin's script results and returns the last one.
func (e *Executor) results(ctx context.Context, className string) (int, json.RawMessage) {
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/scripts/results", url.Values{"className": {className}}, nil, 10*time.Second)
	if err != nil {
		return 0, nil
	}
	var r struct {
		Results []json.RawMessage `json:"results"`
	}
	if json.Unmarshal(raw, &r) != nil || len(r.Results) == 0 {
		return 0, nil
	}
	return len(r.Results), r.Results[len(r.Results)-1]
}

// waitInactive waits for the plugin to switch itself off. It counts as off
// once it was seen on, or when it never shows as on within a grace period
// (it ended before the first look).
func (e *Executor) waitInactive(ctx context.Context, className string, timeout time.Duration) error {
	start := time.Now()
	seen := false
	for {
		if !e.sleep(ctx) {
			return ctx.Err()
		}
		raw, err := e.Game.JSON(ctx, http.MethodGet, "/scripts", nil, nil, 15*time.Second)
		if err == nil {
			var s struct {
				Scripts []struct {
					ClassName string `json:"className"`
					Active    bool   `json:"active"`
				} `json:"scripts"`
			}
			if json.Unmarshal(raw, &s) == nil {
				active := false
				for _, sc := range s.Scripts {
					if sc.ClassName == className {
						active = sc.Active
					}
				}
				seen = seen || active
				if !active && (seen || time.Since(start) > 5*e.poll()) {
					return nil
				}
			}
		}
		if time.Since(start) > timeout {
			return fmt.Errorf("%s did not finish in %s", className, timeout)
		}
	}
}

// walkBack walks to the spot with the webwalker, reissuing the walk when the
// character stands still away from it.
func (e *Executor) walkBack(ctx context.Context, to WalkParams) error {
	body := map[string]any{"x": to.X, "y": to.Y, "plane": to.Plane, "wait": false}
	walks := 0
	still := 0
	for deadline := time.Now().Add(runWalkTimeout); time.Now().Before(deadline); {
		st, err := e.runState(ctx)
		if err == nil && st.Player.Position != nil {
			at := st.Player.Position
			if at.Plane == to.Plane && abs(at.X-to.X) <= runArrived && abs(at.Y-to.Y) <= runArrived {
				e.note(RunReturning, fmt.Sprintf("de volta em %d, %d, %d", at.X, at.Y, at.Plane))
				return nil
			}
			if st.Player.Moving {
				still = 0
			} else {
				still++
			}
		}
		if walks == 0 || still >= 3 {
			if walks++; walks > runWalkTries {
				return fmt.Errorf("the walk back to %d, %d stopped short %d times", to.X, to.Y, runWalkTries)
			}
			if _, err = e.Game.JSON(ctx, http.MethodPost, "/walk", nil, body, 15*time.Second); err != nil && ctx.Err() == nil {
				return fmt.Errorf("walking back: %w", err)
			}
			e.note(RunReturning, fmt.Sprintf("andando de volta a %d, %d, %d", to.X, to.Y, to.Plane))
			still = 0
		}
		if !e.sleep(ctx) {
			return ctx.Err()
		}
	}
	return fmt.Errorf("the walk back to %d, %d took more than %s", to.X, to.Y, runWalkTimeout)
}

// simple is a class name without its package, as the trail shows it.
func simple(className string) string { return className[strings.LastIndex(className, ".")+1:] }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
