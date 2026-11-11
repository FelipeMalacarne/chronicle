package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// eventBridge plays /bridge/events: a boot id and the events recorded so far.
type eventBridge struct {
	mu     sync.Mutex
	boot   string
	events []string // kinds; seq is the index + 1
}

func (b *eventBridge) add(kind string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, kind)
}

func (b *eventBridge) JSON(_ context.Context, _, path string, q url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	if path != "/bridge/events" {
		return nil, fmt.Errorf("unexpected %s", path)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var after int
	_, _ = fmt.Sscan(q.Get("after"), &after)
	type ev struct {
		Seq  int    `json:"seq"`
		Kind string `json:"kind"`
	}
	out := []ev{}
	for i, k := range b.events {
		if i+1 > after {
			out = append(out, ev{i + 1, k})
		}
	}
	return json.Marshal(map[string]any{"boot": b.boot, "last": len(b.events), "events": out})
}

func setTrigger(t *testing.T, e *Executor, enabled bool) {
	t.Helper()
	params := fmt.Sprintf(`{"event":"kitten_grown","enabled":%v,"run":{"className":"%s","stop":["%s"]}}`, enabled, catSwap, fisher)
	if _, err := e.Execute(context.Background(), CmdTriggerSet, json.RawMessage(params)); err != nil {
		t.Fatal(err)
	}
}

func lastFiring(e *Executor) *Firing {
	out, _ := e.Execute(context.Background(), CmdTriggerGet, nil)
	return out.(TriggerStatus).Last
}

func TestTriggerStartsTheSavedRun(t *testing.T) {
	ctx := context.Background()
	g := newRunGame("done")
	b := &eventBridge{boot: "a", events: []string{EventKittenGrown}} // grown before the agent looked
	e := &Executor{Game: g, Bridge: b, RunPoll: time.Millisecond, TriggerFile: filepath.Join(t.TempDir(), "t.json")}
	setTrigger(t, e, true)

	e.watchOnce(ctx)
	if r, _ := e.Execute(ctx, CmdRunStatus, nil); r.(RunStatus).Run != nil {
		t.Fatal("an event from before the watcher's first look started a run")
	}
	b.add(EventKittenGrown)
	e.watchOnce(ctx)
	r := waitRun(t, e)
	if r.State != RunDone || r.Trigger != EventKittenGrown {
		t.Fatalf("run = %+v", r)
	}
	if f := lastFiring(e); f == nil || f.RunID != r.ID || f.Skipped != "" {
		t.Fatalf("firing = %+v", f)
	}
	// Seen once: the next poll starts nothing.
	e.watchOnce(ctx)
	if r2 := waitRun(t, e); r2.ID != r.ID {
		t.Fatalf("the same event started %s", r2.ID)
	}
}

func TestTriggerOffAndPluginRestart(t *testing.T) {
	ctx := context.Background()
	b := &eventBridge{boot: "a"}
	e := &Executor{Game: newRunGame("done"), Bridge: b, RunPoll: time.Millisecond}
	setTrigger(t, e, false)
	e.watchOnce(ctx)
	b.add(EventKittenGrown)
	e.watchOnce(ctx)
	if f := lastFiring(e); f == nil || f.Skipped != "trigger off" || f.RunID != "" {
		t.Fatalf("firing = %+v", f)
	}
	// The plugin started again: its numbering restarts and nothing old fires.
	setTrigger(t, e, true)
	b.mu.Lock()
	b.boot, b.events = "b", []string{EventKittenGrown}
	b.mu.Unlock()
	e.watchOnce(ctx)
	if r, _ := e.Execute(ctx, CmdRunStatus, nil); r.(RunStatus).Run != nil {
		t.Fatal("a new boot replayed its events")
	}
}

func TestTriggerWatchErrorAndFile(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "ironman", "t.json")
	e := &Executor{Game: &fakeGame{}, Bridge: failing{}, TriggerFile: file}
	setTrigger(t, e, true)
	e.watchOnce(ctx)
	out, _ := e.Execute(ctx, CmdTriggerGet, nil)
	if s := out.(TriggerStatus); s.Watching || !strings.Contains(s.WatchError, "1.7+") {
		t.Fatalf("status = %+v", s)
	}
	// Another agent on the same file finds the trigger saved.
	out, _ = (&Executor{Game: &fakeGame{}, TriggerFile: file}).Execute(ctx, CmdTriggerGet, nil)
	if ts := out.(TriggerStatus).Triggers; len(ts) != 1 || !ts[0].Enabled || ts[0].Run.ClassName != catSwap {
		t.Fatalf("loaded = %+v", ts)
	}
}

func TestTriggerParams(t *testing.T) {
	for _, c := range []struct{ params, bad string }{
		{`{"event":"kitten_grown","enabled":true,"run":{"className":"a.b.CatSwapPlugin"}}`, ""},
		{`{"event":"level_up","enabled":true,"run":{"className":"a.b.CatSwapPlugin"}}`, "unknown event"},
		{`{"event":"kitten_grown","enabled":true,"run":{"className":"CatSwap"}}`, "fully qualified"},
		{`{"event":"kitten_grown","run":{"className":"a.b.CatSwapPlugin"},"extra":1}`, "unknown field"},
	} {
		_, err := ValidateParams(CmdTriggerSet, json.RawMessage(c.params))
		if (c.bad == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.bad)) {
			t.Errorf("%s: %v, want %q", c.params, err, c.bad)
		}
	}
	if _, err := ValidateParams(CmdTriggerGet, nil); err != nil {
		t.Fatal(err)
	}
}
