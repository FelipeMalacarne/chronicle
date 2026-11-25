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
// until it fills, collects it into the inventory (/ge/collect with toBank
// false, so the live inventory shows what arrived) and closes the exchange.
//
// /ge/collect is the exchange's "Collect all": it empties every finished
// slot, not only the run's. So the run refuses to start while another slot
// holds a finished offer, and a sale also while another slot holds any
// offer (it could fill during the sale and pay into the same collect).
//
// An offer that does not fill in time, the owner's cancel and any failure
// after the offer may have been placed (an error answer included: the
// placement can land after the request timed out or was canceled) leave
// the exchange without the run's offer: the run reads GET /ge until the
// Agent Server is idle, aborts a live offer of the item and collects a
// finished one. The pinned client's abort (Rs2GrandExchange.abortOffer(name,
// true)) collects every finished slot into the BANK: what the offer filled
// and what it returned (the items of a sale, the coins of a buy) go there.
// A buy is still proven, since the account's holdings count the bank; a
// sale is not, since its coins must reach the inventory (rules.SaleProven).
//
// The run ending "done" means the offer filled and was collected into the
// inventory, never that the task is done: the server proves it from the
// ingest (the item held for a buy; for a sale, the item gone from the
// inventory and the coins there, recorded when seen). Chronicle Link never
// sees these tasks.

// AgentServerClass is the plugin that executes a Grand Exchange run
// (Run.ClassName): the Agent Server itself.
const AgentServerClass = "net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin"

const (
	// taskGETimeout is how long an offer may take to fill by default.
	taskGETimeout = 10 * time.Minute
	// geCleanupTimeout bounds leaving the exchange without the run's offer,
	// a placement still running in the Agent Server included.
	geCleanupTimeout = 2 * time.Minute
)

// geOffer is one slot of the Agent Server's GET /ge.
type geOffer struct {
	Slot   int    `json:"slot"`
	Name   string `json:"item"`
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

// offer returns the slot that holds item (the GE's slots hold unnoted ids;
// the content verifier keeps a GE task's item unnoted).
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

// geFinished reports whether a slot waits to be collected.
func geFinished(state string) bool { return geFilled(state) || geCanceled(state) }

// geOthers refuses a Grand Exchange whose other slots would mix into the
// run's collect: a finished offer (the collect empties every slot) and, for
// a sale, any offer (it could fill during the sale and pay coins the proof
// would count).
func geOthers(s geState, item int, sale bool) error {
	for _, o := range s.Offers {
		switch {
		case o.ItemID == item:
		case geFinished(o.State):
			return fmt.Errorf("slot %d of the Grand Exchange holds a finished offer of %s (%s) that was not collected: collect it first, or the run's collect would bring it in", o.Slot, o.Name, o.State)
		case sale:
			return fmt.Errorf("slot %d of the Grand Exchange holds an offer of %s (%s): a sale runs alone, since another offer could fill during it and pay into the same collect", o.Slot, o.Name, o.State)
		}
	}
	return nil
}

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

// geLeft is what the cleanup did to the run's offer.
type geLeft struct {
	// aborted: a live offer was aborted, and the Agent Server collected the
	// finished slots into the bank.
	aborted bool
	// collected: a finished offer was collected into the inventory.
	collected bool
}

// text says it to the owner.
func (l geLeft) text() string {
	switch {
	case l.aborted:
		return "a oferta foi abortada; o abort do Agent Server coleta no banco, então o que encheu e o que voltou da oferta estão no banco"
	case l.collected:
		return "a oferta tinha terminado e foi coletada no inventário"
	}
	return "nenhuma oferta do item ficou no Grand Exchange"
}

// geCleanup leaves the exchange without the run's offer, whatever stopped
// the run, and closes it; it outlives the run's context. A live offer of
// the item is aborted and a finished one collected into the inventory,
// until GET /ge has none. /ge/close takes the Agent Server's Grand Exchange
// lock, so a close that succeeds with no offer of the item proves no
// placement is still running there (one whose answer never came).
func (e *Executor) geCleanup(o execspec.GEOffer) (geLeft, error) {
	ctx, cancel := context.WithTimeout(context.Background(), geCleanupTimeout)
	defer cancel()
	var left geLeft
	var last error
	for {
		done, err := e.geSettle(ctx, o, &left)
		if done {
			return left, nil
		}
		if err != nil {
			last = err
		}
		if !e.sleep(ctx) {
			return left, errors.Join(fmt.Errorf("the Grand Exchange still holds the offer of %s after %s: check it in game", o.ItemName, geCleanupTimeout), last)
		}
	}
}

// geSettle is one step of geCleanup; true once the exchange is closed with
// no offer of the item.
func (e *Executor) geSettle(ctx context.Context, o execspec.GEOffer, left *geLeft) (bool, error) {
	s, err := e.geState(ctx)
	if err != nil {
		return false, err
	}
	if slot, ok := s.offer(o.Item); ok {
		if geFinished(slot.State) {
			if err = e.gePost(ctx, "/ge/collect", map[string]any{"toBank": false}); err != nil {
				return false, fmt.Errorf("collecting: %w", err)
			}
			left.collected = true
			return false, nil
		}
		if err = e.gePost(ctx, "/ge/abort", map[string]any{"name": o.ItemName}); err != nil {
			return false, fmt.Errorf("aborting the offer: %w", err)
		}
		left.aborted = true
		return false, nil
	}
	if err = e.gePost(ctx, "/ge/close", map[string]any{}); err != nil {
		return false, fmt.Errorf("closing the exchange: %w", err)
	}
	after, err := e.geState(ctx)
	if err != nil {
		return false, err
	}
	_, ok := after.offer(o.Item)
	return !ok, nil
}

// driveGE is a Grand Exchange run's goroutine: the safe point and the
// checks, the offer placed, followed until it fills, the timeout or the
// owner's cancel, then collected into the inventory.
func (e *Executor) driveGE(ctx context.Context, p TaskRunParams) {
	o, path, verb := execspec.GEOffer{}, "/ge/buy", "comprar"
	sale := p.Task.GESell != nil
	if sale {
		o, path, verb = p.Task.GESell.GEOffer, "/ge/sell", "vender"
	} else {
		o = *p.Task.GEBuy
	}
	// placed: the offer may be in the game, from the moment its request
	// left, whatever answer came back.
	placed := false
	fail := func(err error) {
		var (
			left geLeft
			cerr error
		)
		if placed {
			left, cerr = e.geCleanup(o)
		}
		outcome := left.text()
		if cerr != nil {
			outcome = "deixar o Grand Exchange sem a oferta falhou: " + clip(cerr.Error(), 300)
			err = fmt.Errorf("%w; cleaning up: %w", err, cerr)
		}
		if ctx.Err() != nil {
			e.end(RunCanceled, "cancelado pelo dono; "+outcome, "")
			return
		}
		e.end(RunFailed, err.Error()+"; "+outcome, err.Error())
	}
	if err := e.waitSafe(ctx, false); err != nil {
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
	if err := geOthers(s, o.Item, sale); err != nil {
		fail(err)
		return
	}
	at := *st.Player.Position
	e.note(RunRunning, fmt.Sprintf("em %d, %d, %d; oferta: %s %d × %s a %d gp cada", at.X, at.Y, at.Plane, verb, o.Qty, o.ItemName, o.Price))
	placed = true
	if err := e.gePost(ctx, path, map[string]any{"name": o.ItemName, "quantity": o.Qty, "price": o.Price}); err != nil {
		fail(fmt.Errorf("placing the offer: %w", err))
		return
	}
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
	left, cerr := e.geCleanup(o)
	switch {
	case cerr != nil:
		e.end(RunFailed, "a oferta encheu, mas coletar falhou: "+clip(cerr.Error(), 300), cerr.Error())
	case !left.collected:
		e.end(RunFailed, "a oferta encheu, mas o run não a coletou no inventário: "+left.text(),
			"the filled offer left the Grand Exchange before the run collected it")
	default:
		e.end(RunDone, "oferta completa e coletada no inventário; o término é provado pelo ingest, não pelo agent", "")
	}
}
