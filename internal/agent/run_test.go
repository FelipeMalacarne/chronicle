package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fisher  = "a.b.TickFisherPlugin"
	catSwap = "a.b.CatSwapPlugin"
)

// runGame plays the Agent Server for a run: the run plugin walks away, leaves
// its outcome in the results and switches itself off; /walk arrives at once.
type runGame struct {
	fakeGame
	mu       sync.Mutex
	active   map[string]bool
	pos      WalkParams
	results  []json.RawMessage
	outcome  string // the run plugin's outcome; "" never ends it
	requests []string
	fight    bool              // the health bar shows: no safe point
	levels   map[string]int    // /skills by name
	settings map[string]string // /settings/plugin writes, "group.key" → value
}

func newRunGame(outcome string) *runGame {
	return &runGame{active: map[string]bool{fisher: true}, pos: WalkParams{X: 2600, Y: 3400}, outcome: outcome}
}

func (g *runGame) State(context.Context) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	player := map[string]any{"position": g.pos}
	if g.fight {
		player["healthRatio"] = 20
	}
	b, _ := json.Marshal(map[string]any{"loggedIn": true, "player": player})
	return b, nil
}

func (g *runGame) JSON(_ context.Context, method, path string, query url.Values, body any, _ time.Duration) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	class := ""
	if m, ok := body.(map[string]any); ok {
		class, _ = m["className"].(string)
	}
	g.requests = append(g.requests, method+" "+path+" "+class)
	switch method + " " + path {
	case "GET /skills":
		name := query.Get("name")
		if lvl, ok := g.levels[name]; ok {
			return json.RawMessage(fmt.Sprintf(`{"skills":[{"name":%q,"level":%d}]}`, name, lvl)), nil
		}
		return json.RawMessage(`{"skills":[]}`), nil
	case "POST /settings/plugin":
		m := body.(map[string]any)
		if g.settings == nil {
			g.settings = map[string]string{}
		}
		switch v := m["value"].(type) {
		case string:
			g.settings[m["group"].(string)+"."+m["key"].(string)] = v
		case *string:
			g.settings[m["group"].(string)+"."+m["key"].(string)] = *v
		}
	case "POST /scripts/stop":
		g.active[class] = false
	case "POST /scripts/start":
		g.active[class] = true
		if class == catSwap && g.outcome != "" {
			go func() {
				time.Sleep(5 * time.Millisecond)
				g.mu.Lock()
				defer g.mu.Unlock()
				g.pos = WalkParams{X: 3151, Y: 3409}
				g.results = append(g.results, json.RawMessage(`{"outcome":"`+g.outcome+`","reason":"no coins"}`))
				g.active[catSwap] = false
			}()
		}
	case "GET /scripts":
		var s []map[string]any
		for c, a := range g.active {
			s = append(s, map[string]any{"className": c, "active": a})
		}
		b, _ := json.Marshal(map[string]any{"scripts": s})
		return b, nil
	case "GET /scripts/results":
		b, _ := json.Marshal(map[string]any{"results": g.results})
		return b, nil
	case "POST /walk":
		m := body.(map[string]any)
		g.pos = WalkParams{X: m["x"].(int), Y: m["y"].(int), Plane: m["plane"].(int)}
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func waitRun(t *testing.T, e *Executor) *Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, err := e.Execute(context.Background(), CmdRunStatus, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r := out.(RunStatus).Run; r != nil && r.Final() {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never ended: %+v", out.(RunStatus).Run)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunStopsRunsWalksBackAndRestarts(t *testing.T) {
	ctx := context.Background()
	g := newRunGame("done")
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	if _, err := e.Execute(ctx, CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`)); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunDone || r.From == nil || r.From.X != 2600 || !strings.Contains(string(r.Result), `"done"`) {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pos.X != 2600 || !g.active[fisher] || g.active[catSwap] {
		t.Fatalf("after the run: pos %+v, active %v", g.pos, g.active)
	}
	// The order that matters: stop the fisher, start the swap, walk back, start the fisher.
	var order []string
	for _, q := range g.requests {
		if strings.HasPrefix(q, "POST") {
			order = append(order, q)
		}
	}
	want := []string{"POST /scripts/stop " + fisher, "POST /scripts/start " + catSwap, "POST /walk ", "POST /scripts/start " + fisher}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %v, want %v", order, want)
	}
}

func TestRunFailureLeavesTheMainActivityStopped(t *testing.T) {
	g := newRunGame("failed")
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	if _, err := e.Execute(context.Background(), CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`)); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "no coins") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[fisher] {
		t.Fatal("the fisher was restarted after a failed run")
	}
}

func TestRunRefusesActionsAndCancels(t *testing.T) {
	ctx := context.Background()
	g := newRunGame("") // the run plugin never ends
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	if _, err := e.Execute(ctx, CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(ctx, CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`"}`)); !errors.Is(err, ErrRunGoing) {
		t.Fatalf("second run = %v", err)
	}
	if _, err := e.Execute(ctx, CmdInventoryDrop, json.RawMessage(`{"name":"Logs"}`)); !errors.Is(err, ErrRunGoing) {
		t.Fatalf("action during a run = %v", err)
	}
	if _, err := e.Execute(ctx, CmdTimers, nil); errors.Is(err, ErrRunGoing) {
		t.Fatal("a read was refused during a run")
	}
	if _, err := e.Execute(ctx, CmdRunCancel, nil); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunCanceled {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[catSwap] {
		t.Fatal("the run plugin was left running after a cancel")
	}
}

func TestRunParams(t *testing.T) {
	for _, c := range []struct{ params, bad string }{
		{`{"className":"a.b.CatSwapPlugin","stop":["a.b.TickFisherPlugin"],"timeoutS":900}`, ""},
		{`{"className":"CatSwapPlugin"}`, "fully qualified"},
		{`{"className":"a.b.CatSwapPlugin","stop":["a.b.CatSwapPlugin"]}`, "repeated"},
		{`{"className":"a.b.CatSwapPlugin","stop":["net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin"]}`, "not controlled"},
		{`{"className":"a.b.CatSwapPlugin","timeoutS":5}`, "timeoutS"},
	} {
		_, err := ValidateParams(CmdRunStart, json.RawMessage(c.params))
		if (c.bad == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.bad)) {
			t.Errorf("%s: %v, want %q", c.params, err, c.bad)
		}
	}
}
