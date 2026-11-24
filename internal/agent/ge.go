package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

// Grand Exchange by delegation (docs/12 section 6): a task_run whose task is
// a ge_buy or a ge_sell, for a regular account only, places one offer
// through the Agent Server's /ge endpoints (GrandExchangeHandler of the
// pinned client: the exchange must be open or nearby), follows it on GET /ge
// until it fills, collects it into the inventory (never the bank, so the
// live inventory shows what arrived) and closes the exchange. An offer that
// does not fill in time, the owner's cancel and a failure abort it and
// collect what is there. The run ending "done" means the offer filled and
// was collected, never that the task is done: the server proves it from the
// ingest (the item held for a buy; for a sale, the item gone from the
// inventory and the coins there, rules.SaleProven). Chronicle Link never
// sees these tasks.

// AgentServerClass is the plugin that executes a Grand Exchange run
// (Run.ClassName): the Agent Server itself.
const AgentServerClass = "net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin"

// taskGETimeout is how long an offer may take to fill by default.
const taskGETimeout = 10 * time.Minute

// geOffer is one slot of the Agent Server's GET /ge.
type geOffer struct {
	Slot   int    `json:"slot"`
	ItemID int    `json:"itemId"`
	State  string `json:"state"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}

// geState is the Agent Server's GET /ge.
type geState struct {
	Open   bool      `json:"open"`
	Offers []geOffer `json:"offers"`
}

// offer returns the slot that holds item.
func (s geState) offer(item int) (geOffer, bool) {
	for _, o := range s.Offers {
		if o.ItemID == item {
			return o, true
		}
	}
	return geOffer{}, false
}

// geFilled reports whether a slot's offer is complete (GrandExchangeOfferState).
func geFilled(state string) bool { return state == "BOUGHT" || state == "SOLD" }

// geCanceled reports whether a slot's offer was aborted.
func geCanceled(state string) bool { return state == "CANCELLED_BUY" || state == "CANCELLED_SELL" }

func (e *Executor) geState(ctx context.Context) (geState, error) {
	var s geState
	raw, err := e.Game.JSON(ctx, http.MethodGet, "/ge", nil, nil, 15*time.Second)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("the Grand Exchange's offers: %w", err)
	}
	return s, nil
}

// gePost is one Grand Exchange action; the Agent Server answers 409 with
// its reason when it did not happen.
func (e *Executor) gePost(ctx context.Context, path string, body map[string]any) error {
	_, err := e.Game.JSON(ctx, http.MethodPost, path, nil, body, 60*time.Second)
	return err
}

// geCleanup aborts the offer of item (when it is still there), collects
// into the inventory and closes the exchange, whatever stopped the run; it
// outlives the run's context.
func (e *Executor) geCleanup(o execspec.GEOffer, abort bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var errs []error
	if abort {
		if s, err := e.geState(ctx); err != nil {
			errs = append(errs, err)
		} else if slot, ok := s.offer(o.Item); ok && !geFilled(slot.State) && !geCanceled(slot.State) {
			if err := e.gePost(ctx, "/ge/abort", map[string]any{"name": o.ItemName}); err != nil {
				errs = append(errs, fmt.Errorf("aborting the offer: %w", err))
			}
		}
	}
	if err := e.gePost(ctx, "/ge/collect", map[string]any{"toBank": false}); err != nil {
		errs = append(errs, fmt.Errorf("collecting: %w", err))
	}
	if err := e.gePost(ctx, "/ge/close", map[string]any{}); err != nil {
		errs = append(errs, fmt.Errorf("closing the exchange: %w", err))
	}
	return errors.Join(errs...)
}

// driveGE is a Grand Exchange run's goroutine: the safe point and the
// checks, the offer placed, followed until it fills, the timeout or the
// owner's cancel, then collected into the inventory.
func (e *Executor) driveGE(ctx context.Context, p TaskRunParams) {
	o, path, verb := execspec.GEOffer{}, "/ge/buy", "comprar"
	if p.Task.GEBuy != nil {
		o = *p.Task.GEBuy
	} else {
		o, path, verb = p.Task.GESell.GEOffer, "/ge/sell", "vender"
	}
	placed := false
	fail := func(err error) {
		cerr := error(nil)
		if placed {
			cerr = e.geCleanup(o, true)
		}
		if ctx.Err() != nil {
			text := "cancelado pelo dono; a oferta foi abortada e coletada"
			if cerr != nil {
				text = "cancelado pelo dono; abortar e coletar falhou: " + clip(cerr.Error(), 300)
			}
			e.end(RunCanceled, text, "")
			return
		}
		if cerr != nil {
			err = fmt.Errorf("%w; cleaning up: %w", err, cerr)
		}
		e.end(RunFailed, err.Error()+"; o que encheu da oferta ficou no inventário", err.Error())
	}
	if err := e.waitSafe(ctx); err != nil {
		fail(err)
		return
	}
	st, err := e.runState(ctx)
	if err != nil {
		fail(err)
		return
	}
	if !st.LoggedIn || st.Player.Position == nil {
		fail(errors.New("the character is not logged in"))
		return
	}
	// The proof is the inventory Chronicle Link tracks: without it the
	// ingest could not prove the trade.
	if v := e.bridgeVersion(ctx); v == "" {
		fail(errors.New("the Chronicle Link bridge did not answer: without its tracking the ingest cannot prove the trade"))
		return
	}
	s, err := e.geState(ctx)
	if err != nil {
		fail(err)
		return
	}
	if slot, ok := s.offer(o.Item); ok {
		fail(fmt.Errorf("the Grand Exchange already has an offer of %s (slot %d, %s): collect or abort it first", o.ItemName, slot.Slot, slot.State))
		return
	}
	at := *st.Player.Position
	e.note(RunRunning, fmt.Sprintf("em %d, %d, %d; oferta: %s %d × %s a %d gp cada", at.X, at.Y, at.Plane, verb, o.Qty, o.ItemName, o.Price))
	if err := e.gePost(ctx, path, map[string]any{"name": o.ItemName, "quantity": o.Qty, "price": o.Price}); err != nil {
		fail(fmt.Errorf("placing the offer: %w", err))
		return
	}
	placed = true
	timeout := taskGETimeout
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	if e.taskTimeout > 0 {
		timeout = e.taskTimeout
	}
	deadline := time.Now().Add(timeout)
	done, misses := -1, 0
	for {
		if !e.sleep(ctx) {
			fail(ctx.Err())
			return
		}
		s, err := e.geState(ctx)
		if err != nil {
			if misses++; misses >= taskBridgeMisses {
				fail(fmt.Errorf("the Agent Server stopped answering: %w", err))
				return
			}
			continue
		}
		misses = 0
		slot, ok := s.offer(o.Item)
		switch {
		case !ok:
			fail(errors.New("the offer left the Grand Exchange before it filled (collected or aborted by someone else)"))
			return
		case geCanceled(slot.State):
			fail(errors.New("the offer was aborted in game"))
			return
		}
		if slot.Done != done {
			done = slot.Done
			e.note(RunRunning, fmt.Sprintf("oferta: %d de %d", slot.Done, slot.Total))
		}
		if geFilled(slot.State) {
			break
		}
		if time.Now().After(deadline) {
			fail(fmt.Errorf("the offer did not fill in %s", timeout))
			return
		}
	}
	placed = false
	if err := e.geCleanup(o, false); err != nil {
		e.end(RunFailed, "a oferta encheu, mas coletar falhou: "+clip(err.Error(), 300), err.Error())
		return
	}
	e.end(RunDone, "oferta completa e coletada no inventário; o término é provado pelo ingest, não pelo agent", "")
}
