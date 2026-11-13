package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/appenv"
)

// The runs' history (docs/08 phase 4): the agent keeps its last runs with
// their trails in a file on its machine, written at every line of the trail,
// so the last run survives the agent restarting and a run the agent died in
// the middle of shows as failed instead of vanishing.
const CmdRunHistory = "run_history" // the last runs, newest first

// runKeep is how many runs the history keeps.
const runKeep = 30

// RunHistory is the answer to CmdRunHistory.
type RunHistory struct {
	Runs []Run `json:"runs"`
	// Error is why the history file could not be written.
	Error string `json:"error,omitempty"`
}

// DefaultRunFile is where `chronicle agent` keeps the history: agent-runs.json
// next to agent-triggers.json.
func DefaultRunFile() string {
	dir, err := appenv.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agent-runs.json")
}

// loadRuns reads the history once; the caller holds the runner's lock. The
// last run becomes the current one, and the numbering goes on after it.
func (e *Executor) loadRuns() {
	r := &e.runs
	if r.loaded {
		return
	}
	r.loaded = true
	if e.RunFile == "" {
		return
	}
	b, err := os.ReadFile(e.RunFile)
	if err != nil {
		return
	}
	var runs []Run
	if json.Unmarshal(b, &runs) != nil || len(runs) == 0 {
		return
	}
	for i := range runs {
		if runs[i].UID == "" {
			// A run from before the server kept them: named once, then saved.
			runs[i].UID = newID()
		}
	}
	for _, x := range runs {
		if n, err := strconv.Atoi(strings.TrimPrefix(x.ID, "run-")); err == nil && n > r.seq {
			r.seq = n
		}
	}
	last := runs[len(runs)-1]
	if !last.Final() {
		// The agent stopped in the middle: whatever it had stopped stays stopped.
		now := time.Now().UTC()
		last.State, last.FinishedAt = RunFailed, &now
		last.Error = "the agent stopped during the run"
		last.Events = append(last.Events, RunEvent{At: now, State: RunFailed, Text: "o agent parou no meio do run; o que estava parado continua parado"})
	}
	r.past, r.run = runs[:len(runs)-1], &last
	r.cancel = func() {}
	e.saveRuns()
}

// saveRuns publishes the current run (OnRun) and writes the history; the
// caller holds the runner's lock.
func (e *Executor) saveRuns() {
	r := &e.runs
	if r.run == nil {
		return
	}
	if e.OnRun != nil {
		e.OnRun(*r.snapshot())
	}
	if e.RunFile == "" {
		return
	}
	runs := append(slices.Clone(r.past), *r.run)
	if err := writeJSONFile(e.RunFile, runs); err != nil {
		r.saveErr = err.Error()
	} else {
		r.saveErr = ""
	}
}

// retire moves the current run to the past before a new one; the caller
// holds the runner's lock.
func (r *runner) retire() {
	if r.run == nil {
		return
	}
	r.past = append(r.past, *r.run)
	if n := len(r.past) - (runKeep - 1); n > 0 {
		r.past = slices.Delete(r.past, 0, n)
	}
}

func (e *Executor) runHistory() RunHistory {
	r := &e.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	e.loadRuns()
	h := RunHistory{Runs: []Run{}, Error: r.saveErr}
	if c := r.snapshot(); c != nil {
		h.Runs = append(h.Runs, *c)
	}
	for _, x := range slices.Backward(r.past) {
		h.Runs = append(h.Runs, x)
	}
	return h
}

// writeJSONFile writes v atomically, readable by the owner only.
func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

var (
	reRunUID = regexp.MustCompile(`^[0-9a-f]{8,64}$`)
	runState = []string{RunWaiting, RunStopping, RunRunning, RunReturning, RunRestarting, RunDone, RunFailed, RunCanceled}
)

// ValidateReport checks a run an agent sent by itself (TypeEvent) before the
// server stores it: the server does not trust the link's data any more than
// a request's.
func ValidateReport(r Run) error {
	switch {
	case !reRunUID.MatchString(r.UID):
		return errors.New("run report: uid must be 8 to 64 hex characters")
	case r.ID == "" || len(r.ID) > 32:
		return errors.New("run report: bad id")
	case len(r.ClassName) > 256 || !reClassName.MatchString(r.ClassName):
		return errors.New("run report: bad className")
	case len(r.Stop) > 8:
		return errors.New("run report: too many plugins stopped")
	case r.Trigger != "" && !slices.Contains(Events, r.Trigger):
		return errors.New("run report: unknown trigger")
	case r.Routine != "" && (!reRoutineID.MatchString(r.Routine) || r.Trigger != ""):
		return errors.New("run report: bad routine")
	case !slices.Contains(runState, r.State):
		return errors.New("run report: unknown state")
	case r.StartedAt.IsZero():
		return errors.New("run report: no start time")
	case len(r.Events) > 500:
		return errors.New("run report: trail too long")
	case len(r.Result) > 64<<10 || len(r.Error) > 2000:
		return errors.New("run report: result or error too long")
	}
	for _, c := range r.Stop {
		if len(c) > 256 || !reClassName.MatchString(c) {
			return errors.New("run report: bad plugin stopped")
		}
	}
	if len(r.Result) > 0 && !json.Valid(r.Result) {
		return errors.New("run report: result is not JSON")
	}
	if r.Task != nil {
		if err := checkRunTask(*r.Task); err != nil {
			return fmt.Errorf("run report: %w", err)
		}
	}
	for _, ev := range r.Events {
		if !slices.Contains(runState, ev.State) || len(ev.Text) > 1000 || ev.At.IsZero() {
			return errors.New("run report: bad trail line")
		}
	}
	return nil
}
