package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/microbot"
)

// geGame is a logged-in character at the Grand Exchange, as the pinned
// client's Agent Server answers it (GrandExchangeHandler): GET /ge, and
// POST /ge/buy, /ge/sell, /ge/collect, /ge/abort and /ge/close; the rest is
// a runGame's.
type geGame struct {
	*runGame
	gmu    sync.Mutex
	offers []map[string]any
	// fill is how much an offer fills per GET /ge; 0 never fills.
	fill   int
	refuse string // the Agent Server refuses to place the offer
	posts  []string
}

func newGEGame() *geGame { return &geGame{runGame: newRunGame("")} }

func (g *geGame) JSON(ctx context.Context, method, path string, q url.Values, body any, d time.Duration) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/ge") {
		return g.runGame.JSON(ctx, method, path, q, body, d)
	}
	g.gmu.Lock()
	defer g.gmu.Unlock()
	m, _ := body.(map[string]any)
	if method == http.MethodPost {
		g.posts = append(g.posts, strings.TrimPrefix(path, "/ge/"))
	}
	switch method + " " + path {
	case "GET /ge":
		for _, o := range g.offers {
			if st := o["state"].(string); st == "BUYING" || st == "SELLING" {
				o["done"] = min(o["done"].(int)+g.fill, o["total"].(int))
				if o["done"] == o["total"] {
					o["state"] = map[string]string{"BUYING": "BOUGHT", "SELLING": "SOLD"}[st]
				}
			}
		}
		return json.Marshal(map[string]any{"open": true, "offers": g.offers})
	case "POST /ge/buy", "POST /ge/sell":
		if g.refuse != "" {
			return nil, &microbot.APIError{Path: path, Status: http.StatusConflict, Message: g.refuse}
		}
		state := "BUYING"
		if path == "/ge/sell" {
			state = "SELLING"
		}
		g.posts[len(g.posts)-1] += " " + m["name"].(string)
		g.offers = append(g.offers, map[string]any{"slot": len(g.offers) + 1, "item": m["name"], "itemId": 1775,
			"state": state, "price": m["price"], "done": 0, "total": m["quantity"], "spent": 0})
		return json.RawMessage(`{"success":true}`), nil
	case "POST /ge/abort":
		for _, o := range g.offers {
			if o["item"] == m["name"] {
				o["state"] = map[string]string{"BUYING": "CANCELLED_BUY", "SELLING": "CANCELLED_SELL"}[o["state"].(string)]
			}
		}
		return json.RawMessage(`{"success":true}`), nil
	case "POST /ge/collect":
		if tb, _ := m["toBank"].(bool); tb {
			g.posts[len(g.posts)-1] += " toBank"
		}
		g.offers = slices.DeleteFunc(g.offers, func(o map[string]any) bool {
			st := o["state"].(string)
			return st != "BUYING" && st != "SELLING"
		})
		return json.RawMessage(`{"success":true}`), nil
	case "POST /ge/close":
		return json.RawMessage(`{"success":true}`), nil
	}
	return nil, &microbot.APIError{Path: path, Status: http.StatusNotFound, Message: "Unknown endpoint"}
}

func (g *geGame) actions() []string {
	g.gmu.Lock()
	defer g.gmu.Unlock()
	return slices.Clone(g.posts)
}

func geParams(mode string, task execspec.Task) TaskRunParams {
	return TaskRunParams{Key: "recipe:item:1775x1000#0", Plan: "pln_abcdefgh", Label: "Grand Exchange", Mode: mode, Task: task}
}

func glassBuy() execspec.Task {
	return execspec.Task{Type: execspec.TypeGEBuy, GEBuy: &execspec.GEOffer{Item: 1775, ItemName: "Molten glass", Qty: 100, Price: 147}}
}

func glassSale() execspec.Task {
	return execspec.Task{Type: execspec.TypeGESell, GESell: &execspec.GESell{
		GEOffer: execspec.GEOffer{Item: 1775, ItemName: "Molten glass", Qty: 100, Price: 123}, CoinsAtLeast: 12_100,
	}}
}

func newGEExecutor() (*Executor, *geGame) {
	g := newGEGame()
	return &Executor{Game: g, Bridge: &taskBridge{version: "1.18.0"}, RunPoll: time.Millisecond}, g
}

func TestValidateGERun(t *testing.T) {
	for _, task := range []execspec.Task{glassBuy(), glassSale()} {
		if _, err := ValidateParams(CmdTaskRun, raw(t, geParams("regular", task))); err != nil {
			t.Errorf("%s regular refused: %v", task.Type, err)
		}
		for _, mode := range []string{"ironman", "hcim", "uim", "gim"} {
			if _, err := ValidateParams(CmdTaskRun, raw(t, geParams(mode, task))); err == nil || !strings.Contains(err.Error(), "regular accounts only") {
				t.Errorf("%s %s: %v", task.Type, mode, err)
			}
		}
	}
	if err := checkRunTask(RunTask{Key: "recipe:item:1775x1000#0", Plan: "pln_abcdefgh", Type: "ge_buy", Label: "x", CoinsAtLeast: 5}); err == nil {
		t.Error("a buy carried a sale's coins")
	}
}

// TestGERunPlacesFollowsAndCollects: the offer is placed by name, followed
// until it fills, collected into the inventory (never the bank) and the
// exchange closed; the sale's run carries the coins its proof waits for.
func TestGERunPlacesFollowsAndCollects(t *testing.T) {
	for _, tc := range []struct {
		task  execspec.Task
		place string
		coins int64
	}{
		{glassBuy(), "buy Molten glass", 0},
		{glassSale(), "sell Molten glass", 12_100},
	} {
		t.Run(tc.task.Type, func(t *testing.T) {
			e, g := newGEExecutor()
			g.fill = 40
			started := startTask(t, e, geParams("regular", tc.task))
			if started.ClassName != AgentServerClass || started.Until != nil || started.Task.CoinsAtLeast != tc.coins {
				t.Fatalf("started = %+v %+v", started, started.Task)
			}
			r := waitRun(t, e)
			if r.State != RunDone || !strings.Contains(r.Events[len(r.Events)-1].Text, "provado pelo ingest") {
				t.Fatalf("run = %+v", r)
			}
			if got, want := g.actions(), []string{tc.place, "collect", "close"}; !slices.Equal(got, want) {
				t.Fatalf("actions = %v, want %v", got, want)
			}
		})
	}
}

// TestGERunAbortsAnOfferThatDoesNotFill: past its timeout the offer is
// aborted, what filled is collected, and the run fails.
func TestGERunAbortsAnOfferThatDoesNotFill(t *testing.T) {
	e, g := newGEExecutor()
	e.taskTimeout = 30 * time.Millisecond
	startTask(t, e, geParams("regular", glassBuy()))
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "did not fill") {
		t.Fatalf("run = %+v", r)
	}
	if got, want := g.actions(), []string{"buy Molten glass", "abort", "collect", "close"}; !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

// TestGERunRefusesAnOfferAlreadyThere: an offer of the item in a slot is
// someone else's: the run touches nothing.
func TestGERunRefusesAnOfferAlreadyThere(t *testing.T) {
	e, g := newGEExecutor()
	g.offers = []map[string]any{{"slot": 1, "item": "Molten glass", "itemId": 1775, "state": "SOLD", "done": 5, "total": 5}}
	startTask(t, e, geParams("regular", glassSale()))
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "already has an offer") {
		t.Fatalf("run = %+v", r)
	}
	if got := g.actions(); len(got) != 0 {
		t.Fatalf("actions = %v", got)
	}
}

// TestGERunReportsTheAgentServersRefusal: the Agent Server's reason (not at
// the exchange) fails the run, with nothing to clean up.
func TestGERunReportsTheAgentServersRefusal(t *testing.T) {
	e, g := newGEExecutor()
	g.refuse = "Not at the Grand Exchange; POST /walk there first"
	startTask(t, e, geParams("regular", glassBuy()))
	if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, "Not at the Grand Exchange") {
		t.Fatalf("run = %+v", r)
	}
	if got := g.actions(); !slices.Equal(got, []string{"buy"}) {
		t.Fatalf("actions = %v", got)
	}
}
