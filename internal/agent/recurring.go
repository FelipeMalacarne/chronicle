package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/appenv"
)

// Recurring routines (docs/08 phase 4, delivery 4): a run the agent starts
// by itself when a Time Tracking clock is ready (the bridge's
// /bridge/timers: the herb patches, the trees, the bird houses), or a fixed
// interval after its last run ended when the clock does not know the
// patches. The server keeps them (docs/11 section 4.4) and sends them in
// routines_sync; the watcher looks every half minute and starts the due one
// of highest priority: one run at a time, never while a run goes, and only
// with the character at a safe point (safe.go), which also keeps it out of
// a break (logged out). Whether a break is near is not observable yet
// (docs/08, Breaks).
//
// Two holds keep a loop or a surprise from repeating: after a run that
// failed or was canceled, the routine waits for the owner to save it again
// (its updatedAt), and every routine waits while the agent's last run of any
// kind failed or was canceled after the last save of any routine (such a run
// leaves the main activity stopped where it failed). A routine its clock
// fired fires by the clock again only after the clock was seen not ready: a
// run that did not replant would loop otherwise.
const CmdRecurringGet = "recurring_get" // the recurring routines and the watcher's last look

// Clocks are the Time Tracking clocks a recurring routine may follow: the
// keys of /bridge/timers (the Tab names of RuneLite's Time Tracking, plus
// the farming contract).
var Clocks = []string{
	"HERB", "TREE", "FRUIT_TREE", "BIRD_HOUSE", "ALLOTMENT", "FLOWER", "BUSH", "GRAPE", "HOPS", "SPECIAL",
	"FARMING_CONTRACT",
}

// Bounds of a recurring routine.
const (
	RecurringIntervalMin = 10          // minutes
	RecurringIntervalMax = 7 * 24 * 60 // minutes
	RecurringPriorityMax = 100
	RecurringMax         = 20 // per account
)

// Recurring is one recurring routine.
type Recurring struct {
	// ID is the server's (rtn_...); empty in a request that creates one.
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	// Enabled: off, it never starts by itself.
	Enabled bool `json:"enabled"`
	// Clock is the Time Tracking clock it follows (Clocks); it is due when
	// the clock is ready.
	Clock string `json:"clock,omitempty"`
	// IntervalMin is due this many minutes after its last run ended, when
	// the clock does not know the patches (or there is no clock).
	IntervalMin int `json:"intervalMin,omitempty"`
	// Priority: when several are due, the highest starts first.
	Priority int       `json:"priority"`
	Run      RunParams `json:"run"`
	// Set by the server (a save ignores them): when the owner last saved it,
	// and its last run as the database holds it.
	UpdatedAt *time.Time    `json:"updatedAt,omitempty"`
	LastRun   *RecurringRun `json:"lastRun,omitempty"`
}

// RecurringRun is the last run of a recurring routine.
type RecurringRun struct {
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Why a recurring routine is or is not due (RecurringState.Reason).
const (
	ReasonOff             = "off"              // switched off
	ReasonClockReady      = "clock_ready"      // due: its clock is ready
	ReasonClockWaiting    = "clock_waiting"    // its clock grows (NextAt), or is occupied or empty
	ReasonClockStale      = "clock_stale"      // its clock still reads ready after the run it fired: waits for it to change
	ReasonFirst           = "first"            // due: by its interval, and it never ran
	ReasonInterval        = "interval"         // due: the interval since its last run ended passed
	ReasonIntervalWaiting = "interval_waiting" // the interval has not passed (NextAt)
	ReasonNoClock         = "no_clock"         // its clock is not known (or unread) and it has no interval
	ReasonHeld            = "held"             // its last run failed or was canceled after it was saved
	ReasonHeldAgent       = "held_agent"       // the agent's last run failed or was canceled after the last save
)

// RecurringState is a routine as the watcher saw it at its last look.
type RecurringState struct {
	Recurring
	// Due: it would start now (with no run going and at a safe point).
	Due bool `json:"due"`
	// Reason is why it is or is not due; empty before the watcher looked.
	Reason string `json:"reason,omitempty"`
	// NextAt is when it is due next, when known.
	NextAt *time.Time `json:"nextAt,omitempty"`
}

// RecurringFiring is a run the watcher started, or tried to.
type RecurringFiring struct {
	At      time.Time `json:"at"`
	Routine string    `json:"routine"`
	Name    string    `json:"name"`
	RunID   string    `json:"runId,omitempty"`
	Skipped string    `json:"skipped,omitempty"`
}

// RecurringStatus is the answer to CmdRecurringGet.
type RecurringStatus struct {
	Recurring []RecurringState `json:"recurring"`
	// CheckedAt is the watcher's last look; nil before the first.
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
	// Waiting says why nothing started at the last look though one was due
	// ("um run em andamento", "esperando: em combate").
	Waiting string `json:"waiting,omitempty"`
	// ClockError says why the clocks were not read (the bridge, the Time
	// Tracking plugin off).
	ClockError string `json:"clockError,omitempty"`
	// WatchError is set by the server when the agent does not answer.
	WatchError string           `json:"watchError,omitempty"`
	Last       *RecurringFiring `json:"last,omitempty"`
}

var reRoutineID = regexp.MustCompile(`^rtn_[0-9a-f]{8,64}$`)

// ValidateRecurring checks a recurring routine (the web's save and the
// server's routines_sync) and returns it with the name trimmed.
func ValidateRecurring(rc Recurring) (Recurring, error) {
	rc.Name = strings.TrimSpace(rc.Name)
	switch {
	case rc.ID != "" && !reRoutineID.MatchString(rc.ID):
		return rc, errors.New("recurring: bad id")
	case rc.Name == "" || utf8.RuneCountInString(rc.Name) > 80 || strings.ContainsFunc(rc.Name, unicode.IsControl):
		return rc, errors.New("recurring: name of 1 to 80 characters, one line")
	case rc.Clock != "" && !slices.Contains(Clocks, rc.Clock):
		return rc, fmt.Errorf("recurring: unknown clock %q", rc.Clock)
	case rc.IntervalMin != 0 && (rc.IntervalMin < RecurringIntervalMin || rc.IntervalMin > RecurringIntervalMax):
		return rc, fmt.Errorf("recurring: intervalMin between %d and %d", RecurringIntervalMin, RecurringIntervalMax)
	case rc.Clock == "" && rc.IntervalMin == 0:
		return rc, errors.New("recurring: a clock, an interval or both")
	case rc.Priority < 0 || rc.Priority > RecurringPriorityMax:
		return rc, fmt.Errorf("recurring: priority between 0 and %d", RecurringPriorityMax)
	case rc.LastRun != nil && (!slices.Contains(runState, rc.LastRun.State) || rc.LastRun.StartedAt.IsZero()):
		return rc, errors.New("recurring: bad last run")
	}
	b, _ := json.Marshal(rc.Run)
	p, err := validateRun(b)
	if err != nil {
		return rc, err
	}
	rc.Run = p
	return rc, nil
}

// validateRecurringList checks the recurring routines of a routines_sync:
// each one, with the server's id, at most RecurringMax.
func validateRecurringList(list []Recurring) error {
	if len(list) > RecurringMax {
		return fmt.Errorf("routines: at most %d recurring routines", RecurringMax)
	}
	seen := map[string]bool{}
	for _, rc := range list {
		if _, err := ValidateRecurring(rc); err != nil {
			return err
		}
		if rc.ID == "" || seen[rc.ID] {
			return errors.New("routines: a recurring routine without an id, or twice")
		}
		seen[rc.ID] = true
	}
	return nil
}

// DefaultRecurringFile is where `chronicle agent` keeps the recurring
// routines the server sent: agent-recurring.json next to the triggers, so
// they hold when the agent restarts with the server away.
func DefaultRecurringFile() string {
	dir, err := appenv.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agent-recurring.json")
}

// recurringPoll is how often the watcher looks at the recurring routines.
const recurringPoll = 30 * time.Second

// recurrer is the agent's recurring routines and the watcher's last look.
// Zero value ready.
type recurrer struct {
	mu     sync.Mutex
	loaded bool
	list   []Recurring
	gen    int // changes with the list
	// disarmed holds the routines their clock fired, until the watcher sees
	// the clock not ready.
	disarmed map[string]bool
	states   []RecurringState
	checked  *time.Time
	waiting  string
	clockErr string
	last     *RecurringFiring
	wake     chan struct{}
}

// wakeCh is the watcher's wake-up; the caller holds the lock.
func (rw *recurrer) wakeCh() chan struct{} {
	if rw.wake == nil {
		rw.wake = make(chan struct{}, 1)
	}
	return rw.wake
}

// loadRecurring reads the file once; the caller holds the lock.
func (e *Executor) loadRecurring() {
	rw := &e.recur
	if rw.loaded {
		return
	}
	rw.loaded = true
	if e.RecurringFile == "" {
		return
	}
	b, err := os.ReadFile(e.RecurringFile)
	if err != nil {
		return
	}
	var list []Recurring
	if json.Unmarshal(b, &list) == nil && validateRecurringList(list) == nil {
		rw.list = list
		rw.states = unlooked(list)
	}
}

// setRecurring replaces the routines (routines_sync, already validated),
// writes the file and wakes the watcher.
func (e *Executor) setRecurring(list []Recurring) error {
	rw := &e.recur
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.loaded = true
	next := slices.Clone(list)
	if next == nil {
		next = []Recurring{}
	}
	if e.RecurringFile != "" {
		if err := writeJSONFile(e.RecurringFile, next); err != nil {
			return fmt.Errorf("recurring: %w", err)
		}
	}
	// A routine gone, or saved again by the owner, starts armed.
	maps.DeleteFunc(rw.disarmed, func(id string, _ bool) bool {
		i := slices.IndexFunc(next, func(rc Recurring) bool { return rc.ID == id })
		j := slices.IndexFunc(rw.list, func(rc Recurring) bool { return rc.ID == id })
		return i < 0 || j < 0 || !sameTime(next[i].UpdatedAt, rw.list[j].UpdatedAt)
	})
	rw.list, rw.gen = next, rw.gen+1
	rw.states = unlooked(next)
	select {
	case rw.wakeCh() <- struct{}{}:
	default:
	}
	return nil
}

func sameTime(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

func unlooked(list []Recurring) []RecurringState {
	out := make([]RecurringState, len(list))
	for i, rc := range list {
		out[i] = RecurringState{Recurring: rc}
	}
	return out
}

// recurringStatus copies the watcher's state.
func (e *Executor) recurringStatus() RecurringStatus {
	rw := &e.recur
	rw.mu.Lock()
	defer rw.mu.Unlock()
	e.loadRecurring()
	s := RecurringStatus{Recurring: slices.Clone(rw.states), Waiting: rw.waiting, ClockError: rw.clockErr}
	if s.Recurring == nil {
		s.Recurring = []RecurringState{}
	}
	if rw.checked != nil {
		c := *rw.checked
		s.CheckedAt = &c
	}
	if rw.last != nil {
		l := *rw.last
		s.Last = &l
	}
	return s
}

// WatchRecurring looks at the recurring routines until ctx ends: every
// RecurringPoll, and at once when the server sends new ones.
func (e *Executor) WatchRecurring(ctx context.Context) {
	poll := e.RecurringPoll
	if poll <= 0 {
		poll = recurringPoll
	}
	e.recur.mu.Lock()
	wake := e.recur.wakeCh()
	e.recur.mu.Unlock()
	for {
		e.recurringOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(poll):
		}
	}
}

func (e *Executor) clockNow() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

// recurringOnce is one look: which routines are due and, when one is and
// nothing else drives the character, start the one of highest priority.
func (e *Executor) recurringOnce(ctx context.Context) {
	rw := &e.recur
	rw.mu.Lock()
	e.loadRecurring()
	list, gen, disarmed := slices.Clone(rw.list), rw.gen, maps.Clone(rw.disarmed)
	rw.mu.Unlock()

	now := e.clockNow().UTC()
	look := recurringLook{now: now}
	look.latest, look.lastOf = e.lastRuns()
	if look.latest != nil && !look.latest.Final() {
		rw.mu.Lock()
		rw.checked, rw.waiting = &now, "um run em andamento"
		rw.mu.Unlock()
		return
	}
	var clockErr string
	if slices.ContainsFunc(list, func(rc Recurring) bool { return rc.Enabled && rc.Clock != "" }) {
		t, err := e.timers(ctx)
		switch {
		case err != nil:
			clockErr = err.Error()
		case !t.Enabled:
			clockErr = "the Time Tracking plugin is off in the client"
		default:
			look.clocks, look.clockNow = map[string]Timer{}, t.Now
			for _, c := range t.Timers {
				look.clocks[c.Key] = c
			}
		}
	}
	for _, rc := range list {
		if rc.UpdatedAt != nil && rc.UpdatedAt.After(look.saved) {
			look.saved = *rc.UpdatedAt
		}
	}
	states := make([]RecurringState, len(list))
	byClock := make([]bool, len(list))
	rearm := map[string]bool{}
	best := -1
	for i, rc := range list {
		var seen bool
		states[i], byClock[i], seen = assess(rc, look, !disarmed[rc.ID])
		if seen {
			rearm[rc.ID] = true
		}
		if states[i].Due && (best < 0 || before(rc, list[best])) {
			best = i
		}
	}
	var (
		waiting string
		fired   *RecurringFiring
		disarm  string
	)
	if best >= 0 {
		rc := list[best]
		if why := e.unsafe(ctx); why != "" {
			waiting = "esperando: " + why
		} else {
			fired = &RecurringFiring{At: now, Routine: rc.ID, Name: rc.Name}
			run, err := e.startRun(rc.Run, runOrigin{routine: rc.ID, why: firingLine(rc, states[best].Reason, look)})
			if err != nil {
				fired.Skipped = err.Error()
			} else {
				fired.RunID = run.ID
				if byClock[best] {
					disarm = rc.ID
				}
			}
		}
	}

	rw.mu.Lock()
	defer rw.mu.Unlock()
	for id := range rearm {
		delete(rw.disarmed, id)
	}
	if disarm != "" {
		if rw.disarmed == nil {
			rw.disarmed = map[string]bool{}
		}
		rw.disarmed[disarm] = true
	}
	if rw.gen == gen {
		rw.states = states
	}
	rw.checked, rw.waiting, rw.clockErr = &now, waiting, clockErr
	if fired != nil {
		rw.last = fired
	}
}

// before orders the due routines: the highest priority, then by name and id.
func before(a, b Recurring) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.ID, b.ID)) < 0
}

// firingLine is the first line of the trail of a run the watcher started.
func firingLine(rc Recurring, reason string, look recurringLook) string {
	switch reason {
	case ReasonClockReady:
		label := rc.Clock
		if c, ok := look.clocks[rc.Clock]; ok && c.Label != "" {
			label = c.Label
		}
		return fmt.Sprintf("rotina %s: relógio %s pronto", rc.Name, label)
	case ReasonFirst:
		return fmt.Sprintf("rotina %s: primeira vez (a cada %d min)", rc.Name, rc.IntervalMin)
	default:
		return fmt.Sprintf("rotina %s: %d min desde o fim do último run", rc.Name, rc.IntervalMin)
	}
}

// recurringLook is what one look knows.
type recurringLook struct {
	now time.Time
	// clocks by key; nil when they were not read.
	clocks   map[string]Timer
	clockNow int64 // the game machine's clock when they were read, epoch seconds
	// latest is the agent's last run of any kind; lastOf the last local run
	// of each routine.
	latest *Run
	lastOf map[string]Run
	// saved is the latest save of any routine.
	saved time.Time
}

// lastRuns returns the agent's last run and the last run of each recurring
// routine it knows.
func (e *Executor) lastRuns() (*Run, map[string]Run) {
	r := &e.runs
	r.mu.Lock()
	defer r.mu.Unlock()
	e.loadRuns()
	by := map[string]Run{}
	for _, x := range r.past {
		if x.Routine != "" {
			by[x.Routine] = x
		}
	}
	latest := r.snapshot()
	if latest != nil && latest.Routine != "" {
		by[latest.Routine] = *latest
	}
	return latest, by
}

// lastRunOf is the routine's last run: the agent's own or, when newer (a pod
// that lost its history), the server's.
func lastRunOf(rc Recurring, lastOf map[string]Run) *RecurringRun {
	local, ok := lastOf[rc.ID]
	if ok && (rc.LastRun == nil || !rc.LastRun.StartedAt.After(local.StartedAt)) {
		return &RecurringRun{State: local.State, StartedAt: local.StartedAt, FinishedAt: local.FinishedAt}
	}
	return rc.LastRun
}

// assess says whether one routine is due. armed is false after its clock
// fired it, until the clock is seen not ready; seenNotReady reports that the
// clock was (also while the routine is held).
func assess(rc Recurring, look recurringLook, armed bool) (st RecurringState, byClock, seenNotReady bool) {
	st = RecurringState{Recurring: rc}
	if !rc.Enabled {
		st.Reason = ReasonOff
		return st, false, false
	}
	// The clock, when the Time Tracking knows the patches.
	var clock *Timer
	if c, ok := look.clocks[rc.Clock]; ok && rc.Clock != "" && c.State != "" && c.State != "UNKNOWN" {
		clock = &c
	}
	ready := clock != nil && (clock.State == "COMPLETED" || (clock.State == "IN_PROGRESS" && clock.ReadyAt != nil && *clock.ReadyAt <= look.clockNow))
	seenNotReady = clock != nil && !ready

	var saved time.Time
	if rc.UpdatedAt != nil {
		saved = *rc.UpdatedAt
	}
	last := lastRunOf(rc, look.lastOf)
	var lastEnd time.Time
	if last != nil {
		// A run that never finished here (the agent's pod went away in the
		// middle) counts as failed when it started.
		lastEnd = last.StartedAt
		if last.FinishedAt != nil {
			lastEnd = *last.FinishedAt
		}
		if (last.FinishedAt == nil || last.State != RunDone) && lastEnd.After(saved) {
			st.Reason = ReasonHeld
			return st, false, seenNotReady
		}
	}
	if l := look.latest; l != nil && l.Final() && l.State != RunDone && l.FinishedAt != nil && l.FinishedAt.After(look.saved) {
		st.Reason = ReasonHeldAgent
		return st, false, seenNotReady
	}
	switch {
	case clock != nil && !ready:
		st.Reason = ReasonClockWaiting
		if clock.ReadyAt != nil {
			next := time.Unix(*clock.ReadyAt, 0).UTC()
			st.NextAt = &next
		}
		return st, false, true
	case clock != nil && !armed:
		st.Reason = ReasonClockStale
		return st, false, false
	case clock != nil:
		st.Due, st.Reason = true, ReasonClockReady
		return st, true, false
	}
	// The clock does not know the patches (or there is none): the interval.
	if rc.IntervalMin == 0 {
		st.Reason = ReasonNoClock
		return st, false, false
	}
	if last == nil {
		st.Due, st.Reason = true, ReasonFirst
		return st, false, false
	}
	next := lastEnd.Add(time.Duration(rc.IntervalMin) * time.Minute).UTC()
	st.NextAt = &next
	if look.now.Before(next) {
		st.Reason = ReasonIntervalWaiting
		return st, false, false
	}
	st.Due, st.Reason = true, ReasonInterval
	return st, false, false
}
