package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/appenv"
)

// Triggers (docs/08 phase 4, delivery 3): a game event starts a run by
// itself, with the params the owner saved. The Chronicle Link plugin (1.7+)
// records the events it recognizes in the chat (a closed list, never player
// chat) and serves them on the bridge's /bridge/events; the agent's watcher
// reads them every few seconds and starts the saved run of an enabled
// trigger. The triggers live in a file on the agent's machine, so they hold
// across the agent restarting and need no server.
const (
	CmdTriggerGet = "trigger_get" // the triggers and the watcher's state
	CmdTriggerSet = "trigger_set" // save one trigger (enable, disable, its run)
	// CmdRoutinesSync: server → agent, on every connection (docs/11 section
	// 4.4, F6) and when the owner saves a recurring routine: the account's
	// triggers and recurring routines as the database holds them, which
	// replace the agent's. The files stay as the attached agent's cache, so
	// they still fire with the server down. Never from the web.
	CmdRoutinesSync = "routines_sync"
)

// RoutinesParams are the parameters of CmdRoutinesSync.
type RoutinesParams struct {
	Triggers []Trigger `json:"triggers"`
	// Recurring goes only to an agent that declares CmdRecurringGet (an
	// agent from before them refuses the unknown field); absent is none.
	Recurring []Recurring `json:"recurring,omitempty"`
}

// RoutinesFor is the routines_sync of an agent: the triggers and, when it
// runs them, the recurring routines. A routine whose run waits for an
// objective (RunParams.Until) goes only to an agent that declares
// run_until: an older one would refuse the whole list for the unknown
// field, and could never end that run.
func RoutinesFor(info Info, triggers []Trigger, recurring []Recurring) RoutinesParams {
	until := info.Supports(CmdRunUntil)
	p := RoutinesParams{Triggers: []Trigger{}}
	for _, t := range triggers {
		if t.Run.Until == nil || until {
			p.Triggers = append(p.Triggers, t)
		}
	}
	if info.Supports(CmdRecurringGet) {
		for _, rc := range recurring {
			if rc.Run.Until == nil || until {
				p.Recurring = append(p.Recurring, rc)
			}
		}
	}
	return p
}

// EventKittenGrown is "Your kitten has grown into a healthy cat that can hunt
// for itself." in the game chat.
const EventKittenGrown = "kitten_grown"

// Events are the game events a trigger may wait for.
var Events = []string{EventKittenGrown}

// Trigger is one saved trigger: the event and the run it starts.
type Trigger struct {
	Event   string    `json:"event"`
	Enabled bool      `json:"enabled"`
	Run     RunParams `json:"run"`
}

// Firing is what the watcher did with one event.
type Firing struct {
	At    time.Time `json:"at"`
	Event string    `json:"event"`
	RunID string    `json:"runId,omitempty"`
	// Skipped says why no run started (the trigger was off, another run was
	// going).
	Skipped string `json:"skipped,omitempty"`
}

// TriggerStatus is the answer to CmdTriggerGet and CmdTriggerSet.
type TriggerStatus struct {
	Triggers []Trigger `json:"triggers"`
	// Watching is true while the bridge answers the watcher; WatchError is
	// why it does not (the client is off, Chronicle Link older than 1.7).
	Watching   bool   `json:"watching"`
	WatchError string `json:"watchError,omitempty"`
	// Last is the last event the watcher saw, fired or not.
	Last *Firing `json:"last,omitempty"`
}

// DefaultTriggerFile is where `chronicle agent` keeps the triggers:
// agent-triggers.json in the user's config dir, next to the CLI's (~/.config/chronicle).
func DefaultTriggerFile() string {
	dir, err := appenv.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agent-triggers.json")
}

// watchPoll is how often the watcher reads the bridge's events.
const watchPoll = 5 * time.Second

// watcher is the agent's triggers and its cursor in the bridge's events.
// Zero value ready; File empty keeps the triggers in memory only.
type watcher struct {
	mu       sync.Mutex
	loaded   bool
	triggers []Trigger
	boot     string
	cursor   int64
	watching bool
	err      string
	last     *Firing
}

func validateTrigger(raw json.RawMessage) (Trigger, error) {
	var t Trigger
	if err := decodeParams(raw, &t); err != nil {
		return t, err
	}
	if !slices.Contains(Events, t.Event) {
		return t, fmt.Errorf("trigger: unknown event %q", t.Event)
	}
	b, _ := json.Marshal(t.Run)
	if _, err := validateRun(b); err != nil {
		return t, err
	}
	return t, nil
}

// validateRoutines checks CmdRoutinesSync's params: every trigger, one per
// event, and every recurring routine.
func validateRoutines(raw json.RawMessage) (RoutinesParams, error) {
	var p RoutinesParams
	if err := decodeParams(raw, &p); err != nil {
		return p, err
	}
	seen := map[string]bool{}
	for _, t := range p.Triggers {
		b, _ := json.Marshal(t)
		if _, err := validateTrigger(b); err != nil {
			return p, err
		}
		if seen[t.Event] {
			return p, fmt.Errorf("routines: trigger %q twice", t.Event)
		}
		seen[t.Event] = true
	}
	return p, validateRecurringList(p.Recurring)
}

// triggerCommand serves CmdTriggerGet, CmdTriggerSet and CmdRoutinesSync.
func (e *Executor) triggerCommand(command string, raw json.RawMessage) (any, error) {
	w := &e.watch
	if command == CmdRoutinesSync {
		p, err := validateRoutines(raw)
		if err != nil {
			return nil, err
		}
		if err := e.setRecurring(p.Recurring); err != nil {
			return nil, err
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		w.loaded = true
		next := slices.Clone(p.Triggers)
		if next == nil {
			next = []Trigger{}
		}
		if err := e.saveTriggers(next); err != nil {
			return nil, err
		}
		w.triggers = next
		return e.triggerStatus(), nil
	}
	if command == CmdTriggerSet {
		t, err := validateTrigger(raw)
		if err != nil {
			return nil, err
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		e.loadTriggers()
		next := slices.Clone(w.triggers)
		if i := slices.IndexFunc(next, func(o Trigger) bool { return o.Event == t.Event }); i >= 0 {
			next[i] = t
		} else {
			next = append(next, t)
		}
		if err := e.saveTriggers(next); err != nil {
			return nil, err
		}
		w.triggers = next
		return e.triggerStatus(), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	e.loadTriggers()
	return e.triggerStatus(), nil
}

// triggerStatus copies the watcher's state; the caller holds the lock.
func (e *Executor) triggerStatus() TriggerStatus {
	w := &e.watch
	s := TriggerStatus{Triggers: slices.Clone(w.triggers), Watching: w.watching, WatchError: w.err}
	if s.Triggers == nil {
		s.Triggers = []Trigger{}
	}
	if w.last != nil {
		l := *w.last
		s.Last = &l
	}
	return s
}

// loadTriggers reads the file once; the caller holds the lock. A missing or
// unreadable file means no triggers (they are off until saved again).
func (e *Executor) loadTriggers() {
	w := &e.watch
	if w.loaded {
		return
	}
	w.loaded = true
	if e.TriggerFile == "" {
		return
	}
	b, err := os.ReadFile(e.TriggerFile)
	if err != nil {
		return
	}
	var ts []Trigger
	if json.Unmarshal(b, &ts) == nil {
		w.triggers = ts
	}
}

// saveTriggers writes the file atomically; the caller holds the lock.
func (e *Executor) saveTriggers(ts []Trigger) error {
	if e.TriggerFile == "" {
		return nil
	}
	if err := writeJSONFile(e.TriggerFile, ts); err != nil {
		return fmt.Errorf("trigger: %w", err)
	}
	return nil
}

// bridgeEvents is the answer of the bridge's /bridge/events.
type bridgeEvents struct {
	// Boot changes when the plugin starts again (its numbering restarts).
	Boot   string `json:"boot"`
	Last   int64  `json:"last"`
	Events []struct {
		Seq  int64  `json:"seq"`
		Kind string `json:"kind"`
	} `json:"events"`
}

// Watch reads the bridge's events until ctx ends and starts the run of each
// enabled trigger whose event appears. Events from before the watcher first
// saw a plugin start are not replayed: an agent that restarts after a swap
// must not swap again.
func (e *Executor) Watch(ctx context.Context) {
	if e.Bridge == nil {
		return
	}
	poll := e.WatchPoll
	if poll <= 0 {
		poll = watchPoll
	}
	for {
		e.watchOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
	}
}

func (e *Executor) watchOnce(ctx context.Context) {
	w := &e.watch
	w.mu.Lock()
	e.loadTriggers()
	q := url.Values{"after": {strconv.FormatInt(w.cursor, 10)}}
	w.mu.Unlock()
	raw, err := e.Bridge.JSON(ctx, http.MethodGet, "/bridge/events", q, nil, 10*time.Second)
	var ev bridgeEvents
	if err == nil {
		err = json.Unmarshal(raw, &ev)
	}
	if err == nil && ev.Boot == "" {
		err = errors.New("no boot id")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.watching = false
		w.err = fmt.Sprintf("events: %v (Chronicle Link 1.7+ serves them in the dev client)", err)
		return
	}
	w.watching, w.err = true, ""
	if ev.Boot != w.boot {
		// A plugin start the watcher has not seen: begin after what it holds.
		w.boot, w.cursor = ev.Boot, ev.Last
		return
	}
	for _, x := range ev.Events {
		if x.Seq <= w.cursor {
			continue
		}
		w.cursor = x.Seq
		f := &Firing{At: time.Now().UTC(), Event: x.Kind}
		i := slices.IndexFunc(w.triggers, func(t Trigger) bool { return t.Event == x.Kind })
		switch {
		case i < 0 || !w.triggers[i].Enabled:
			f.Skipped = "trigger off"
		default:
			if run, err := e.startRun(w.triggers[i].Run, runOrigin{trigger: x.Kind}); err != nil {
				f.Skipped = err.Error()
			} else {
				f.RunID = run.ID
			}
		}
		w.last = f
	}
	if ev.Last > w.cursor {
		w.cursor = ev.Last
	}
}
