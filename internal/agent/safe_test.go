package agent

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

// unsafeGame is a runGame whose character starts in a dialogue, then fights,
// then stands in the Fishing Trawler, and is safe after that.
type unsafeGame struct {
	*runGame
	reads int
}

func (g *unsafeGame) State(ctx context.Context) (json.RawMessage, error) {
	g.mu.Lock()
	g.reads++
	n := g.reads
	pos := g.pos
	g.mu.Unlock()
	health := -1
	switch {
	case n <= 4:
		// in the dialogue below
	case n <= 6:
		health = 20
	case n <= 8:
		pos = WalkParams{X: 1885, Y: 4825} // region 7499
	}
	b, _ := json.Marshal(map[string]any{"loggedIn": true, "player": map[string]any{"position": pos, "healthRatio": health}})
	return b, nil
}

func (g *unsafeGame) JSON(ctx context.Context, method, path string, q url.Values, body any, t time.Duration) (json.RawMessage, error) {
	if path == "/dialogue" {
		g.mu.Lock()
		defer g.mu.Unlock()
		return json.RawMessage(`{"inDialogue":` + map[bool]string{true: "true", false: "false"}[g.reads <= 4] + `}`), nil
	}
	return g.runGame.JSON(ctx, method, path, q, body, t)
}

func TestRunWaitsForASafePoint(t *testing.T) {
	g := &unsafeGame{runGame: newRunGame("done")}
	e := &Executor{Game: g, RunPoll: time.Millisecond}
	if _, err := e.Execute(context.Background(), CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`)); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunDone {
		t.Fatalf("run = %+v", r)
	}
	var trail []string
	for _, ev := range r.Events {
		if ev.State == RunWaiting {
			trail = append(trail, ev.Text)
		}
	}
	want := []string{"esperando: num diálogo", "esperando: em combate", "esperando: em Fishing Trawler", "ponto seguro"}
	if strings.Join(trail, "|") != strings.Join(want, "|") {
		t.Fatalf("waiting trail = %q, want %q", trail, want)
	}
	// Nothing was stopped before the safe point.
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.reads < 9 || !strings.HasPrefix(firstPost(g.requests), "POST /scripts/stop") {
		t.Fatalf("reads %d, requests %v", g.reads, g.requests)
	}
}

func TestRunGivesUpWithoutASafePoint(t *testing.T) {
	g := newRunGame("done")
	e := &Executor{Game: &breakGame{g}, RunPoll: time.Millisecond, SafeWait: 20 * time.Millisecond}
	if _, err := e.Execute(context.Background(), CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`)); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "fora do jogo") {
		t.Fatalf("run = %+v", r)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if p := firstPost(g.requests); p != "" || !g.active[fisher] {
		t.Fatalf("touched the game without a safe point: %v, active %v", g.requests, g.active)
	}
}

// breakGame is logged out (a break) for good.
type breakGame struct{ *runGame }

func (breakGame) State(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"loggedIn":false}`), nil
}

func firstPost(reqs []string) string {
	for _, q := range reqs {
		if strings.HasPrefix(q, "POST") {
			return q
		}
	}
	return ""
}

func TestRegion(t *testing.T) {
	if r := region(WalkParams{X: 1885, Y: 4825}); r != 7499 {
		t.Fatalf("region = %d", r)
	}
	if _, ok := unsafeRegions[region(WalkParams{X: 2600, Y: 3400})]; ok {
		t.Fatal("the fishing spot of the tests is not a minigame")
	}
}
