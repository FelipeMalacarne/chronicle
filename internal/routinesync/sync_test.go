package routinesync

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

type memStore struct {
	mu        sync.Mutex
	triggers  map[string][]agent.Trigger
	recurring map[string][]agent.Recurring
	imported  []string
	recorded  []agent.Run
	newRun    bool
	finished  bool
	err       error
}

func (m *memStore) Recurring(_ context.Context, account string) ([]agent.Recurring, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recurring[account], nil
}

func (m *memStore) Triggers(_ context.Context, account string) ([]agent.Trigger, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.triggers[account], nil
}

func (m *memStore) ImportTriggers(_ context.Context, account string, ts []agent.Trigger) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.triggers[account]) > 0 {
		return false, nil
	}
	m.triggers[account] = ts
	m.imported = append(m.imported, account)
	return true, nil
}

func (m *memStore) RecordRun(_ context.Context, _, _ string, run agent.Run) (bool, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recorded = append(m.recorded, run)
	return m.newRun, m.finished, m.err
}

type call struct {
	command string
	params  any
}

type fakeHub struct {
	calls []call
	own   []agent.Trigger // what the agent's file holds (trigger_get)
}

func (h *fakeHub) CallAgent(_ context.Context, _, command string, params, into any) error {
	h.calls = append(h.calls, call{command, params})
	if command == agent.CmdTriggerGet {
		b, _ := json.Marshal(agent.TriggerStatus{Triggers: h.own})
		return json.Unmarshal(b, into)
	}
	return nil
}

type memAudit struct{ entries []storemicrobot.Entry }

func (a *memAudit) Append(_ context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error) {
	a.entries = append(a.entries, e)
	return e, nil
}

var cat = agent.Trigger{Event: agent.EventKittenGrown, Enabled: true, Run: agent.RunParams{ClassName: "a.b.CatSwapPlugin"}}

func newSync(store *memStore, hub *fakeHub, audit *memAudit) *Sync {
	return &Sync{Store: store, Hub: hub, Audit: audit, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func info(kind string, commands ...string) agent.Info {
	return agent.Info{ID: "c1", Account: "acc_a", Kind: kind, Host: "zaros", Worker: "worker-acc-a", Commands: commands}
}

func TestOnConnect(t *testing.T) {
	// An agent from before F6 keeps its own triggers: nothing is sent.
	store, hub := &memStore{triggers: map[string][]agent.Trigger{"acc_a": {cat}}}, &fakeHub{}
	newSync(store, hub, &memAudit{}).OnConnect(info(agent.KindManaged, agent.CmdTriggerGet))
	if len(hub.calls) != 0 {
		t.Fatalf("old agent got %+v", hub.calls)
	}
	// A worker gets the database's triggers.
	newSync(store, hub, &memAudit{}).OnConnect(info(agent.KindManaged, agent.CmdRoutinesSync))
	if len(hub.calls) != 1 || hub.calls[0].command != agent.CmdRoutinesSync ||
		!reflect.DeepEqual(hub.calls[0].params, agent.RoutinesParams{Triggers: []agent.Trigger{cat}}) {
		t.Fatalf("worker calls = %+v", hub.calls)
	}
	// An attached agent whose account has none seeds them from its file once.
	store, hub = &memStore{triggers: map[string][]agent.Trigger{}}, &fakeHub{own: []agent.Trigger{cat}}
	s := newSync(store, hub, &memAudit{})
	s.OnConnect(info(agent.KindAttached, agent.CmdRoutinesSync))
	if !reflect.DeepEqual(store.imported, []string{"acc_a"}) || len(hub.calls) != 1 || hub.calls[0].command != agent.CmdTriggerGet {
		t.Fatalf("import: imported %v, calls %+v", store.imported, hub.calls)
	}
	hub.calls = nil
	s.OnConnect(info(agent.KindAttached, agent.CmdRoutinesSync))
	if len(store.imported) != 1 || len(hub.calls) != 1 || hub.calls[0].command != agent.CmdRoutinesSync {
		t.Fatalf("second connection: imported %v, calls %+v", store.imported, hub.calls)
	}
	// A worker never imports, even with an empty database.
	store, hub = &memStore{triggers: map[string][]agent.Trigger{}}, &fakeHub{own: []agent.Trigger{cat}}
	newSync(store, hub, &memAudit{}).OnConnect(info(agent.KindManaged, agent.CmdRoutinesSync))
	if len(store.imported) != 0 || len(hub.calls) != 1 || hub.calls[0].command != agent.CmdRoutinesSync {
		t.Fatalf("worker with an empty database: imported %v, calls %+v", store.imported, hub.calls)
	}
}

func TestOnEvent(t *testing.T) {
	now := time.Now().UTC()
	run := agent.Run{ID: "run-1", UID: "0123456789abcdef", ClassName: "a.b.CatSwapPlugin", Trigger: agent.EventKittenGrown, State: agent.RunWaiting, StartedAt: now}
	in := info(agent.KindManaged, agent.CmdRoutinesSync)

	// A run a trigger started: audited when it starts and when it ends.
	store, audit := &memStore{newRun: true}, &memAudit{}
	s := newSync(store, &fakeHub{}, audit)
	s.OnEvent(in, agent.Event{Run: &run})
	store.newRun, store.finished = false, true
	failed := run
	failed.State, failed.Error = agent.RunFailed, "no coins"
	s.OnEvent(in, agent.Event{Run: &failed})
	if len(store.recorded) != 2 || len(audit.entries) != 2 {
		t.Fatalf("recorded %d, audit %+v", len(store.recorded), audit.entries)
	}
	start, end := audit.entries[0], audit.entries[1]
	if start.Action != agent.CmdRunStart || !start.OK || start.Actor != "trigger:kitten_grown" || start.Origin != "agent" || start.AccountID != "acc_a" || start.AgentID != "c1" {
		t.Fatalf("start entry = %+v", start)
	}
	if end.Action != "run_end" || end.OK || end.Error != "no coins" {
		t.Fatalf("end entry = %+v", end)
	}

	// The owner's runs are audited where they started; an unknown account's
	// runs are dropped quietly.
	store, audit = &memStore{newRun: true, finished: true}, &memAudit{}
	owner := run
	owner.Trigger = ""
	newSync(store, &fakeHub{}, audit).OnEvent(in, agent.Event{Run: &owner})
	store.err = storeroutines.ErrNotFound
	newSync(store, &fakeHub{}, audit).OnEvent(in, agent.Event{Run: &run})
	if len(store.recorded) != 2 || len(audit.entries) != 0 {
		t.Fatalf("recorded %d, audit %+v", len(store.recorded), audit.entries)
	}
	// No account, no run: nothing.
	newSync(store, &fakeHub{}, audit).OnEvent(agent.Info{ID: "c2"}, agent.Event{Run: &run})
	newSync(store, &fakeHub{}, audit).OnEvent(in, agent.Event{})
	if len(store.recorded) != 2 {
		t.Fatalf("recorded %d", len(store.recorded))
	}
}

var herb = agent.Recurring{ID: "rtn_000000000001", Name: "Herb run", Enabled: true, Clock: "HERB", Run: agent.RunParams{ClassName: "a.b.HerbrunPlugin"}}

// An agent that runs recurring routines gets them with the triggers; one
// from before them (routines_sync only) never sees the field; an attached
// agent that imports its triggers still gets its recurring routines.
func TestOnConnectRecurring(t *testing.T) {
	store := &memStore{
		triggers:  map[string][]agent.Trigger{"acc_a": {cat}},
		recurring: map[string][]agent.Recurring{"acc_a": {herb}},
	}
	hub := &fakeHub{}
	newSync(store, hub, &memAudit{}).OnConnect(info(agent.KindManaged, agent.CmdRoutinesSync, agent.CmdRecurringGet))
	want := agent.RoutinesParams{Triggers: []agent.Trigger{cat}, Recurring: []agent.Recurring{herb}}
	if len(hub.calls) != 1 || !reflect.DeepEqual(hub.calls[0].params, want) {
		t.Fatalf("calls = %+v", hub.calls)
	}
	hub.calls = nil
	newSync(store, hub, &memAudit{}).OnConnect(info(agent.KindManaged, agent.CmdRoutinesSync))
	if p, ok := hub.calls[0].params.(agent.RoutinesParams); !ok || p.Recurring != nil {
		t.Fatalf("an agent from before them got %+v", hub.calls[0].params)
	}
	store.triggers = map[string][]agent.Trigger{}
	hub = &fakeHub{own: []agent.Trigger{cat}}
	newSync(store, hub, &memAudit{}).OnConnect(info(agent.KindAttached, agent.CmdRoutinesSync, agent.CmdRecurringGet))
	if len(hub.calls) != 2 || hub.calls[0].command != agent.CmdTriggerGet || !reflect.DeepEqual(hub.calls[1].params, want) {
		t.Fatalf("import then sync: %+v", hub.calls)
	}
}

// A run a recurring routine started is audited with the routine as actor.
func TestOnEventRecurring(t *testing.T) {
	now := time.Now().UTC()
	run := agent.Run{ID: "run-2", UID: "0123456789abcdef", ClassName: "a.b.HerbrunPlugin", Routine: herb.ID, State: agent.RunDone, StartedAt: now, FinishedAt: &now}
	store, audit := &memStore{newRun: true, finished: true}, &memAudit{}
	newSync(store, &fakeHub{}, audit).OnEvent(info(agent.KindManaged, agent.CmdRoutinesSync), agent.Event{Run: &run})
	if len(audit.entries) != 2 || audit.entries[0].Actor != "recurring:"+herb.ID || audit.entries[0].Action != agent.CmdRunStart ||
		audit.entries[1].Action != "run_end" || !audit.entries[1].OK {
		t.Fatalf("audit = %+v", audit.entries)
	}
}
