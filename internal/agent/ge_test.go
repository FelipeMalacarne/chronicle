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
// a runGame's. Like the real one, /ge/collect empties every finished slot
// ("Collect all"), /ge/abort aborts the named offer and then collects every
// finished slot into the bank (Rs2GrandExchange.abortOffer(name, true)),
// and a request while another one runs is refused.
type geGame struct {
	*runGame
	gmu    sync.Mutex
	offers []map[string]any
	// fill is how much an offer fills per GET /ge; 0 never fills.
	fill   int
	refuse string // the Agent Server refuses to place the offer
	// failAfter: the offer is placed, then the Agent Server answers 409
	// (processOffer false after the confirm).
	failAfter bool
	// late: the placement's answer never comes (the request timed out) and
	// the Agent Server stays busy with it for that many more requests; the
	// offer lands when it is done.
	late    int
	pending map[string]any
	// hang: the placement blocks until the run's context ends (the owner's
	// cancel), and the offer lands.
	hang  bool
	posts []string
	// toBank counts what was collected into the bank.
	toBank int
}

func newGEGame() *geGame { return &geGame{runGame: newRunGame("")} }

// collectAll empties the finished slots.
func (g *geGame) collectAll() int {
	n := len(g.offers)
	g.offers = slices.DeleteFunc(g.offers, func(o map[string]any) bool {
		st := o["state"].(string)
		return st != "BUYING" && st != "SELLING"
	})
	return n - len(g.offers)
}

func (g *geGame) JSON(ctx context.Context, method, path string, q url.Values, body any, d time.Duration) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/ge") {
		return g.runGame.JSON(ctx, method, path, q, body, d)
	}
	g.gmu.Lock()
	defer g.gmu.Unlock()
	m, _ := body.(map[string]any)
	if method == http.MethodPost {
		g.posts = append(g.posts, strings.TrimPrefix(path, "/ge/"))
		if g.late > 0 && g.pending != nil {
			if g.late--; g.late == 0 {
				g.offers = append(g.offers, g.pending)
			}
			return nil, &microbot.APIError{Path: path, Status: http.StatusConflict, Message: "Another Grand Exchange request is still running"}
		}
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
		offer := map[string]any{"slot": len(g.offers) + 1, "item": m["name"], "itemId": 1775,
			"state": state, "price": m["price"], "done": 0, "total": m["quantity"], "spent": 0}
		switch {
		case g.late > 0:
			g.pending = offer
			return nil, context.DeadlineExceeded
		case g.hang:
			g.gmu.Unlock()
			<-ctx.Done()
			g.gmu.Lock()
			g.offers = append(g.offers, offer)
			return nil, ctx.Err()
		}
		g.offers = append(g.offers, offer)
		if g.failAfter {
			return nil, &microbot.APIError{Path: path, Status: http.StatusConflict, Message: `{"success":false}`}
		}
		return json.RawMessage(`{"success":true}`), nil
	case "POST /ge/abort":
		for _, o := range g.offers {
			if o["item"] == m["name"] {
				o["state"] = map[string]string{"BUYING": "CANCELLED_BUY", "SELLING": "CANCELLED_SELL"}[o["state"].(string)]
			}
		}
		g.toBank += g.collectAll()
		return json.RawMessage(`{"success":true}`), nil
	case "POST /ge/collect":
		if tb, _ := m["toBank"].(bool); tb {
			g.posts[len(g.posts)-1] += " toBank"
		}
		g.collectAll()
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
// until it fills, collected into the inventory (not the bank) and the
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
// aborted (the Agent Server's abort collects into the bank, never the
// inventory), the exchange closed, and the run fails saying so.
func TestGERunAbortsAnOfferThatDoesNotFill(t *testing.T) {
	e, g := newGEExecutor()
	e.taskTimeout = 30 * time.Millisecond
	startTask(t, e, geParams("regular", glassBuy()))
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "did not fill") || !strings.Contains(r.Events[len(r.Events)-1].Text, "estão no banco") {
		t.Fatalf("run = %+v", r)
	}
	if got, want := g.actions(), []string{"buy Molten glass", "abort", "close"}; !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if len(g.offers) != 0 || g.toBank != 1 {
		t.Fatalf("offers = %v, to the bank %d", g.offers, g.toBank)
	}
}

// TestGERunLeavesNoOfferAfterAPlacementError: whatever the placement
// answered, the run reads the exchange and leaves no offer of the item in
// it: one placed before a 409, one that lands after the request timed out
// (the Agent Server still busy with it), and one placed while the owner
// canceled the run.
func TestGERunLeavesNoOfferAfterAPlacementError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*geGame)
		state string
		want  []string
	}{
		{"placed, then refused", func(g *geGame) { g.failAfter = true }, RunFailed, []string{"buy Molten glass", "abort", "close"}},
		{"the answer never came", func(g *geGame) { g.late = 2 }, RunFailed, []string{"buy Molten glass", "close", "close", "abort", "close"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, g := newGEExecutor()
			tc.setup(g)
			startTask(t, e, geParams("regular", glassBuy()))
			r := waitRun(t, e)
			if r.State != tc.state || !strings.Contains(r.Events[len(r.Events)-1].Text, "abortada") {
				t.Fatalf("run = %+v", r)
			}
			if got := g.actions(); !slices.Equal(got, tc.want) {
				t.Fatalf("actions = %v, want %v", got, tc.want)
			}
			if len(g.offers) != 0 {
				t.Fatalf("offers left = %v", g.offers)
			}
		})
	}
	t.Run("the owner's cancel mid-request", func(t *testing.T) {
		e, g := newGEExecutor()
		g.hang = true
		startTask(t, e, geParams("regular", glassSale()))
		for deadline := time.Now().Add(5 * time.Second); !slices.Contains(g.actions(), "sell Molten glass"); {
			if time.Now().After(deadline) {
				t.Fatal("the sale was never placed")
			}
			time.Sleep(time.Millisecond)
		}
		if _, err := e.Execute(context.Background(), CmdRunCancel, nil); err != nil {
			t.Fatal(err)
		}
		r := waitRun(t, e)
		if r.State != RunCanceled || !strings.Contains(r.Events[len(r.Events)-1].Text, "abortada") {
			t.Fatalf("run = %+v", r)
		}
		if got, want := g.actions(), []string{"sell Molten glass", "abort", "close"}; !slices.Equal(got, want) {
			t.Fatalf("actions = %v, want %v", got, want)
		}
		if len(g.offers) != 0 {
			t.Fatalf("offers left = %v", g.offers)
		}
	})
}

// TestGERunRefusesAnOfferAlreadyThere: an offer of the item in a slot is
// someone else's, and so is another slot's finished offer (the collect
// would bring it in) or, for a sale, any other offer (it could pay coins
// into the sale's collect): the run touches nothing.
func TestGERunRefusesAnOfferAlreadyThere(t *testing.T) {
	ore := func(state string) map[string]any {
		return map[string]any{"slot": 2, "item": "Iron ore", "itemId": 440, "state": state, "done": 0, "total": 5}
	}
	for _, tc := range []struct {
		name  string
		task  execspec.Task
		offer map[string]any
		want  string
	}{
		{"the item's own offer", glassSale(), map[string]any{"slot": 1, "item": "Molten glass", "itemId": 1775, "state": "SOLD", "done": 5, "total": 5}, "already has an offer"},
		{"another finished offer, buying", glassBuy(), ore("SOLD"), "was not collected"},
		{"another aborted offer, selling", glassSale(), ore("CANCELLED_BUY"), "was not collected"},
		{"another live offer, selling", glassSale(), ore("BUYING"), "a sale runs alone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, g := newGEExecutor()
			g.offers = []map[string]any{tc.offer}
			startTask(t, e, geParams("regular", tc.task))
			if r := waitRun(t, e); r.State != RunFailed || !strings.Contains(r.Error, tc.want) {
				t.Fatalf("run = %+v", r)
			}
			if got := g.actions(); len(got) != 0 {
				t.Fatalf("actions = %v", got)
			}
		})
	}
	t.Run("another live offer does not stop a buy", func(t *testing.T) {
		e, g := newGEExecutor()
		g.fill = 100
		g.offers = []map[string]any{ore("SELLING")}
		g.offers[0]["total"] = 1000
		startTask(t, e, geParams("regular", glassBuy()))
		if r := waitRun(t, e); r.State != RunDone {
			t.Fatalf("run = %+v", r)
		}
	})
}

// TestGERunReportsTheAgentServersRefusal: the Agent Server's reason (not at
// the exchange) fails the run; the exchange holds no offer of the item.
func TestGERunReportsTheAgentServersRefusal(t *testing.T) {
	e, g := newGEExecutor()
	g.refuse = "Not at the Grand Exchange; POST /walk there first"
	startTask(t, e, geParams("regular", glassBuy()))
	r := waitRun(t, e)
	if r.State != RunFailed || !strings.Contains(r.Error, "Not at the Grand Exchange") || !strings.Contains(r.Events[len(r.Events)-1].Text, "nenhuma oferta") {
		t.Fatalf("run = %+v", r)
	}
	if got := g.actions(); !slices.Equal(got, []string{"buy", "close"}) {
		t.Fatalf("actions = %v", got)
	}
}
