package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// wait_until by an objective key (docs/10 phase 4; docs/08, runs and
// routines): a run may end when an objective key of the account becomes met,
// as the server evaluates it from the ingest (the account's facts), never as
// the plugin or the agent judge it. The run reports its key (Run.Until, in
// every TypeEvent); the server's watcher (internal/rununtil) evaluates it
// whenever the account's facts change and, once it is met, sends run_until;
// the agent then stops the run plugin and goes on as after a plugin that
// ended by itself (walk back, start the main activity again). A plugin that
// stops by itself before that waits a grace period for the proof and then
// fails the run: stopping is not the objective.
const CmdRunUntil = "run_until"

// RunUntil is a run's wait_until.
type RunUntil struct {
	// Objective is the key the run waits for, canonical
	// ("skill:fishing>=76", docs/09 section 3.2).
	Objective string `json:"objective"`
	// Account is the account whose facts prove it. The server sets it from
	// the request's account (or the routine's), never the caller: the
	// watcher needs it for an agent that declares no account.
	Account string `json:"account,omitempty"`
}

// RunUntilParams are the parameters of CmdRunUntil: the run, the key it
// waits for and what the server saw.
type RunUntilParams struct {
	UID       string `json:"uid"`
	Objective string `json:"objective"`
	// Evidence is the observed value that proves the key ("nível 76"), for
	// the trail; empty when the evaluation had none.
	Evidence string `json:"evidence,omitempty"`
}

const (
	// untilTimeout is a wait_until run's default timeout: training to a
	// level takes hours, not the minutes of a swap.
	untilTimeout = 2 * time.Hour
	// untilMaxTimeoutS bounds a wait_until run (12 h).
	untilMaxTimeoutS = 12 * 60 * 60
	// untilGrace is how long a run plugin that stopped by itself waits for
	// the server's proof before the run fails.
	untilGrace = 2 * time.Minute
	// untilLevelEvery is how many polls apart a skill run notes the level
	// the client shows (the trail's progress, never the proof).
	untilLevelEvery = 20
	// untilMisses is how many reads of the client's scripts in a row may
	// fail before a wait_until run gives up (the client went down).
	untilMisses = 20
)

var reAccountID = regexp.MustCompile(`^acc_[a-z0-9_-]{1,32}$`)

// errUntilMet ends a run whose objective is met before it touched anything.
var errUntilMet = errors.New("the objective is met")

// check verifies a wait_until: a canonical objective key and a well-formed
// account (when set).
func (u RunUntil) check() error {
	k, err := entities.ParseObjectiveKey(u.Objective)
	switch {
	case err != nil:
		return fmt.Errorf("until: %w", err)
	case k.String() != u.Objective:
		return fmt.Errorf("until: write the key as %s", k)
	case u.Account != "" && !reAccountID.MatchString(u.Account):
		return errors.New("until: bad account")
	}
	return nil
}

// ObjectiveKey parses the run's key (checked by check).
func (u RunUntil) ObjectiveKey() (entities.ObjectiveKey, error) {
	return entities.ParseObjectiveKey(u.Objective)
}

func validateRunUntil(raw json.RawMessage) (RunUntilParams, error) {
	var p RunUntilParams
	if err := decodeParams(raw, &p); err != nil {
		return p, err
	}
	if !reRunUID.MatchString(p.UID) {
		return p, errors.New("run_until: bad run uid")
	}
	if err := (RunUntil{Objective: p.Objective}).check(); err != nil {
		return p, err
	}
	if len(p.Evidence) > 200 || strings.IndexFunc(p.Evidence, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return p, errors.New("run_until: evidence of at most 200 printable characters")
	}
	return p, nil
}

// runUntil serves CmdRunUntil: the server proved the objective the current
// run waits for. The run's goroutine sees it at its next look and stops the
// run plugin; a second call for the same run changes nothing.
func (e *Executor) runUntil(raw json.RawMessage) (any, error) {
	p, err := validateRunUntil(raw)
	if err != nil {
		return nil, err
	}
	r := &e.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	e.loadRuns()
	switch {
	case r.run == nil || r.run.UID != p.UID:
		return nil, errors.New("run_until: no such run")
	case r.run.Final():
		return nil, errors.New("run_until: the run already ended")
	case r.run.Until == nil || r.run.Until.Objective != p.Objective:
		return nil, errors.New("run_until: the run does not wait for that objective")
	}
	if r.metText == "" {
		text := "objetivo cumprido, provado pelo ingest: " + p.Objective
		if p.Evidence != "" {
			text += " (" + p.Evidence + ")"
		}
		r.metText = text
		if r.met == nil {
			r.met = make(chan struct{})
		}
		close(r.met)
		r.run.Events = append(r.run.Events, RunEvent{At: time.Now().UTC(), State: r.run.State, Text: text})
		e.saveRuns()
	}
	return RunStatus{Run: r.snapshot()}, nil
}

// untilMet reports whether the server proved the current run's objective.
func (e *Executor) untilMet() bool {
	e.runs.mu.Lock()
	defer e.runs.mu.Unlock()
	return e.runs.metText != ""
}

// metSignal is closed when the server proves the current run's objective.
func (e *Executor) metSignal() <-chan struct{} {
	e.runs.mu.Lock()
	defer e.runs.mu.Unlock()
	return e.runs.met
}

// sleepUntil waits one poll, or less when the objective is proven; false
// when the run was canceled.
func (e *Executor) sleepUntil(ctx context.Context, met <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-met:
		return true
	case <-time.After(e.poll()):
		return true
	}
}

func (e *Executor) grace() time.Duration {
	if e.untilGrace > 0 {
		return e.untilGrace
	}
	return untilGrace
}

// untilEnd is how a wait_until run's plugin phase ended.
type untilEnd int

const (
	// untilProven: the server proved the objective while the plugin ran.
	untilProven untilEnd = iota
	// untilProvenAfterStop: the plugin stopped by itself and the proof came
	// within the grace period.
	untilProvenAfterStop
)

// superviseUntil watches a started run plugin until the server proves the
// objective. It fails when the plugin stops by itself and no proof comes
// within the grace period, when its last result says it failed, when the
// client stops answering, on the timeout and on a cancel; the caller stops
// the plugin when it still runs.
// before is how many script results the plugin had before it started.
func (e *Executor) superviseUntil(ctx context.Context, className string, until RunUntil, timeout time.Duration, before int) (untilEnd, error) {
	met := e.metSignal()
	key, _ := until.ObjectiveKey()
	start := time.Now()
	var stoppedAt time.Time
	seen := false
	misses, polls, level := 0, 0, 0
	for {
		if !e.sleepUntil(ctx, met) {
			return 0, ctx.Err()
		}
		if e.untilMet() {
			if stoppedAt.IsZero() {
				return untilProven, nil
			}
			return untilProvenAfterStop, nil
		}
		if stoppedAt.IsZero() {
			active, ok := e.scriptActive(ctx, className)
			switch {
			case !ok:
				if misses++; misses >= untilMisses {
					return 0, errors.New("the client stopped answering")
				}
			case active:
				misses, seen = 0, true
			case seen || time.Since(start) > 5*e.poll():
				misses = 0
				stoppedAt = time.Now()
				if n, last := e.results(ctx, className); n > before && last != nil {
					var out struct {
						Outcome string `json:"outcome"`
						Reason  string `json:"reason"`
					}
					_ = json.Unmarshal(last, &out)
					if out.Outcome == "failed" {
						return 0, fmt.Errorf("%s failed: %s", className, clip(out.Reason, 300))
					}
				}
				e.note(RunRunning, fmt.Sprintf("%s parou sozinho; esperando o ingest provar %s por até %s", simple(className), until.Objective, e.grace().Round(time.Second)))
			}
		}
		if !stoppedAt.IsZero() && time.Since(stoppedAt) > e.grace() {
			return 0, fmt.Errorf("%s stopped and the ingest did not prove %s within %s", simple(className), until.Objective, e.grace().Round(time.Second))
		}
		if time.Since(start) > timeout {
			return 0, fmt.Errorf("%s did not reach %s in %s", simple(className), until.Objective, timeout)
		}
		if polls++; key.Ref.Kind == entities.KindSkill && stoppedAt.IsZero() && (polls == 1 || polls%untilLevelEvery == 0) {
			level = e.noteLevel(ctx, key, level)
		}
	}
}

// scriptActive reads whether the plugin is on (the Agent Server's
// /scripts); ok is false when the client did not answer.
func (e *Executor) scriptActive(ctx context.Context, className string) (active, ok bool) {
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/scripts", nil, nil, 15*time.Second)
	if err != nil {
		return false, false
	}
	var s struct {
		Scripts []struct {
			ClassName string `json:"className"`
			Active    bool   `json:"active"`
		} `json:"scripts"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return false, false
	}
	for _, sc := range s.Scripts {
		if sc.ClassName == className {
			return sc.Active, true
		}
	}
	return false, true
}

// noteLevel notes the skill's level the client shows when it changed since
// last: progress on the trail, never the proof (the server's verdict is).
func (e *Executor) noteLevel(ctx context.Context, k entities.ObjectiveKey, last int) int {
	name := skillName(k.Ref.ID)
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/skills", url.Values{"name": {name}}, nil, 10*time.Second)
	if err != nil {
		return last
	}
	var s struct {
		Skills []struct {
			Name  string `json:"name"`
			Level int    `json:"level"`
		} `json:"skills"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return last
	}
	for _, sk := range s.Skills {
		if !strings.EqualFold(sk.Name, name) || sk.Level < 1 || sk.Level == last {
			continue
		}
		e.note(RunRunning, fmt.Sprintf("%s: nível %d no cliente (o término é o ingest provar %d)", name, sk.Level, k.N))
		return sk.Level
	}
	return last
}

// skillName is a skill ref's id as the client names it ("fishing" is
// "Fishing"): every skill is one word.
func skillName(id string) string {
	if id == "" {
		return id
	}
	return strings.ToUpper(id[:1]) + id[1:]
}
