package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// saveRuns writes the history; the caller holds the runner's lock.
func (e *Executor) saveRuns() {
	r := &e.runs
	if e.RunFile == "" || r.run == nil {
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
