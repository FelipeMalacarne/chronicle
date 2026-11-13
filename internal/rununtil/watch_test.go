package rununtil

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
)

// facts is the accounts' fishing levels.
type facts struct {
	mu     sync.Mutex
	levels map[string]int
	reads  int
}

func (f *facts) set(account string, level int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.levels[account] = level
}

func (f *facts) TaskFacts(_ context.Context, account string) (panel.Facts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	ps := rules.PlayerState{Skills: map[string]rules.SkillState{}}
	if lvl, ok := f.levels[account]; ok {
		ps.Skills["fishing"] = rules.SkillState{Level: lvl, XP: 1}
	}
	return panel.Facts{Mode: "ironman", State: &ps}, nil
}

// hub is the agents' link: one agent per account, and what was sent.
type hub struct {
	mu     sync.Mutex
	agents map[string]agent.Info
	sent   []agent.RunUntilParams
	refuse bool
}

func (h *hub) Route(account string) (agent.Info, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a, ok := h.agents[account]; ok {
		return a, nil
	}
	return agent.Info{}, agent.ErrNoAgent
}

func (h *hub) CallAgent(_ context.Context, _, command string, params, _ any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if command != agent.CmdRunUntil {
		return agent.ErrUnknownCommand
	}
	if h.refuse {
		return &agent.CommandError{Command: command, Message: "run_until: no such run"}
	}
	h.sent = append(h.sent, params.(agent.RunUntilParams))
	return nil
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.sent)
}

type audit struct {
	mu      sync.Mutex
	entries []storemicrobot.Entry
}

func (a *audit) Append(_ context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return e, nil
}

var current = agent.Info{ID: "c1", Account: "acc_zezima", Commands: []string{agent.CmdRunUntil}}

func run(uid, state, account string) *agent.Run {
	return &agent.Run{
		ID: "run-1", UID: uid, ClassName: "a.b.Trainer", State: state, StartedAt: time.Now(), Events: []agent.RunEvent{},
		Until: &agent.RunUntil{Objective: "skill:fishing>=76", Account: account},
	}
}

func setup(t *testing.T) (*Watcher, *facts, *hub, *audit, *events.Hub) {
	t.Helper()
	f := &facts{levels: map[string]int{"acc_zezima": 75}}
	h := &hub{agents: map[string]agent.Info{"acc_zezima": current}}
	a := &audit{}
	ev := events.NewHub(nil)
	w := &Watcher{Facts: f, Hub: h, Events: ev, Audit: a, Interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w.Start(ctx)
	return w, f, h, a, ev
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestProvenAtTheAccountsNextEvent(t *testing.T) {
	w, f, h, a, ev := setup(t)
	w.OnEvent(current, agent.Event{Run: run("00000000000000a1", agent.RunRunning, "acc_zezima")})
	eventually(t, "the first look", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.reads > 0 })
	time.Sleep(10 * time.Millisecond)
	if h.count() != 0 {
		t.Fatal("proven at level 75")
	}
	f.set("acc_zezima", 76)
	ev.Publish(events.Event{Type: events.TypeProgress, Account: "acc_zezima", Source: events.SourcePlugin})
	eventually(t, "the proof", func() bool { return h.count() == 1 })
	if p := h.sent[0]; p.UID != "00000000000000a1" || p.Objective != "skill:fishing>=76" || p.Evidence != "nível 76" {
		t.Fatalf("sent %+v", p)
	}
	eventually(t, "the audit", func() bool { a.mu.Lock(); defer a.mu.Unlock(); return len(a.entries) == 1 })
	if e := a.entries[0]; e.Actor != Actor || e.Origin != "server" || e.Action != agent.CmdRunUntil || !e.OK || e.AccountID != "acc_zezima" {
		t.Fatalf("audit %+v", e)
	}
	// A report in between (the trail's proof line) does not prove it again;
	// the final one ends the watch.
	w.OnEvent(current, agent.Event{Run: run("00000000000000a1", agent.RunRunning, "acc_zezima")})
	ev.Publish(events.Event{Type: events.TypeProgress, Account: "acc_zezima"})
	time.Sleep(10 * time.Millisecond)
	if h.count() != 1 || len(w.Watching()) != 1 {
		t.Fatalf("sent %d, watching %v", h.count(), w.Watching())
	}
	w.OnEvent(current, agent.Event{Run: run("00000000000000a1", agent.RunDone, "acc_zezima")})
	if len(w.Watching()) != 0 {
		t.Fatalf("still watching %v", w.Watching())
	}
}

func TestAlreadyProvenAtOnceAndUndeclaredAgent(t *testing.T) {
	w, f, h, _, _ := setup(t)
	f.set("acc_zezima", 80)
	attached := agent.Info{ID: "c2", Commands: []string{agent.CmdRunUntil}}
	h.agents["acc_zezima"] = attached
	w.OnEvent(attached, agent.Event{Run: run("00000000000000b1", agent.RunWaiting, "acc_zezima")})
	eventually(t, "the proof", func() bool { return h.count() == 1 })
	if h.sent[0].Evidence != "nível 80" {
		t.Fatalf("sent %+v", h.sent[0])
	}
}

func TestIgnoredRuns(t *testing.T) {
	w, f, h, _, _ := setup(t)
	f.set("acc_zezima", 99)
	f.set("acc_other", 99)
	for _, c := range []struct {
		name string
		info agent.Info
		run  *agent.Run
	}{
		{"another account than the agent's", current, run("00000000000000c1", agent.RunRunning, "acc_other")},
		{"no account at all", agent.Info{ID: "c3", Commands: []string{agent.CmdRunUntil}}, run("00000000000000c2", agent.RunRunning, "")},
		{"no until", current, &agent.Run{ID: "run-2", UID: "00000000000000c3", ClassName: "a.b.T", State: agent.RunRunning, StartedAt: time.Now()}},
		{"a bad report", current, run("nothex", agent.RunRunning, "acc_zezima")},
		{"final", current, run("00000000000000c4", agent.RunFailed, "acc_zezima")},
	} {
		w.OnEvent(c.info, agent.Event{Run: c.run})
		if len(w.Watching()) != 0 {
			t.Errorf("%s: watching %v", c.name, w.Watching())
		}
	}
	time.Sleep(10 * time.Millisecond)
	if h.count() != 0 {
		t.Fatalf("sent %v", h.sent)
	}
}

func TestRefusedOrAwayAgent(t *testing.T) {
	w, f, h, _, ev := setup(t)
	f.set("acc_zezima", 76)
	h.refuse = true
	w.OnEvent(current, agent.Event{Run: run("00000000000000d1", agent.RunRunning, "acc_zezima")})
	eventually(t, "the watch dropped", func() bool { return len(w.Watching()) == 0 })

	// With no agent serving the account, the watch waits for it.
	h.mu.Lock()
	h.refuse = false
	delete(h.agents, "acc_zezima")
	h.mu.Unlock()
	w.OnEvent(current, agent.Event{Run: run("00000000000000d2", agent.RunRunning, "acc_zezima")})
	time.Sleep(10 * time.Millisecond)
	if h.count() != 0 || !slices.Contains(w.Watching(), "00000000000000d2") {
		t.Fatalf("sent %d, watching %v", h.count(), w.Watching())
	}
	h.mu.Lock()
	h.agents["acc_zezima"] = current
	h.mu.Unlock()
	ev.Publish(events.Event{Type: events.TypeProgress, Account: "acc_zezima"})
	eventually(t, "the proof once the agent is back", func() bool { return h.count() == 1 })
}

func TestUnknownObjectiveIsNotWatched(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	w, _, _, _, _ := setup(t)
	w.Registry = reg
	r := run("00000000000000e1", agent.RunRunning, "acc_zezima")
	r.Until.Objective = "quest:no_such_quest@finished"
	w.OnEvent(current, agent.Event{Run: r})
	if len(w.Watching()) != 0 {
		t.Fatalf("watching %v", w.Watching())
	}
}

func TestProvenAndEvidence(t *testing.T) {
	met := progress.Evaluation{Verdict: rules.Met, Evidence: []rules.Evidence{{Verdict: rules.Met, Observed: rules.SkillState{Level: 76}}}}
	if !Proven(met) || Evidence(met) != "nível 76" {
		t.Fatalf("met: %v %q", Proven(met), Evidence(met))
	}
	stale := progress.Evaluation{Verdict: rules.Met, Evidence: []rules.Evidence{{Verdict: rules.Met, Stale: true}}}
	if Proven(stale) {
		t.Fatal("a restored mirror proved a wait_until")
	}
	if Proven(progress.Evaluation{Verdict: rules.Unknown}) || Proven(progress.Evaluation{Verdict: rules.Unmet}) {
		t.Fatal("unknown or unmet proved it")
	}
	quest := progress.Evaluation{Verdict: rules.Met, Evidence: []rules.Evidence{{Rule: content.QuestStateRule{}, Observed: map[string]string{"state": "FINISHED"}}}}
	if got := Evidence(quest); got != `observado {"state":"FINISHED"}` {
		t.Fatalf("quest evidence %q", got)
	}
	b, _ := json.Marshal(agent.RunUntilParams{UID: "00000000000000f1", Objective: "skill:fishing>=76", Evidence: Evidence(met)})
	if _, err := agent.ValidateParams(agent.CmdRunUntil, b); err == nil {
		t.Fatal("the web can send run_until")
	}
}
