package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// An agent with slots (Agent.Slots, docs/11): it starts with no client; the
// server puts an account in a free slot with CmdWorkerBootstrap, addressed to
// the agent itself (Message.Account empty), and frees it with
// CmdWorkerRelease. The agent tells the server which clients it has now with
// a TypeClients frame, and the server routes each client's commands and
// events by account like any other.

// releaseTimeout bounds CmdWorkerRelease: logout, then mb-client stop (which
// waits up to 30 s for the java process, then for Xvfb and x11vnc).
const releaseTimeout = 75 * time.Second

// slotSet is the clients in the slots of an agent, safe for the link's
// goroutines.
type slotSet struct {
	mu   sync.Mutex
	held map[string]*slotClient
	// base is the agent's Run context: the clients' watchers outlive a link.
	base context.Context
	// sendMu keeps the TypeClients frames in the order of the sets they carry.
	sendMu sync.Mutex
}

// slotClient is a client and the slot it holds.
type slotClient struct {
	Client
	slot int
	// op serializes its bootstrap and its release: a stop that lands while
	// mb-client start runs would leave the client up with no slot.
	op     sync.Mutex
	cancel context.CancelFunc // ends its watchers
	ctx    context.Context
}

// clients is a snapshot of the clients, by slot.
func (s *slotSet) clients() []Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := make([]*slotClient, 0, len(s.held))
	for _, h := range s.held {
		held = append(held, h)
	}
	slices.SortFunc(held, func(a, b *slotClient) int { return a.slot - b.slot })
	out := make([]Client, len(held))
	for i, h := range held {
		out[i] = h.Client
	}
	return out
}

func (s *slotSet) lookup(account string) (*slotClient, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.held[account]
	return h, ok
}

// client is the client a command to account goes to.
func (s *slotSet) client(account string) (Client, error) {
	h, ok := s.lookup(account)
	if !ok {
		return Client{}, fmt.Errorf("this agent does not drive a client of %s", account)
	}
	return h.Client, nil
}

// take is account's slot: the one it holds, else the lowest free one (no
// client of this agent and, with SlotBusy, no one else on its display).
func (a *Agent) take(account string) (h *slotClient, fresh bool, err error) {
	s := &a.slots
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.held[account]; ok {
		return h, false, nil
	}
	used := map[int]bool{}
	for _, c := range s.held {
		used[c.slot] = true
	}
	unavailable, slot := 0, -1
	for i := range a.Slots {
		if used[i] || (a.SlotBusy != nil && a.SlotBusy(i)) {
			unavailable++
		} else if slot < 0 {
			slot = i
		}
	}
	if slot < 0 {
		return nil, false, fmt.Errorf("no free slot (%d of %d in use)", unavailable, a.Slots)
	}
	c := a.NewSlot(slot, account)
	c.Account = account
	ctx, cancel := context.WithCancel(s.base)
	h = &slotClient{Client: c, slot: slot, cancel: cancel, ctx: ctx}
	if s.held == nil {
		s.held = map[string]*slotClient{}
	}
	s.held[account] = h
	return h, true, nil
}

// drop frees h's slot: its watchers end and its queued runs go.
func (a *Agent) drop(h *slotClient) {
	h.cancel()
	a.slots.mu.Lock()
	if a.slots.held[h.Account] == h {
		delete(a.slots.held, h.Account)
	}
	a.slots.mu.Unlock()
	a.out.forget(h.Account)
}

// announce sends the clients the agent has now.
func (a *Agent) announce(write func(Message) error) {
	a.slots.sendMu.Lock()
	defer a.slots.sendMu.Unlock()
	var hellos []ClientHello
	for _, c := range a.slots.clients() {
		hellos = append(hellos, ClientHello{Account: c.Account})
	}
	if err := write(Message{Type: TypeClients, Clients: hellos}); err != nil {
		a.Log.Warn("agent: write clients", "err", err)
	}
}

// poolCommand runs a command addressed to the agent itself.
func (a *Agent) poolCommand(ctx context.Context, m Message, write func(Message) error) (any, error) {
	switch m.Command {
	case CmdWorkerBootstrap:
		return a.slotBootstrap(ctx, m.Params, write)
	case CmdWorkerRelease:
		return nil, a.slotRelease(ctx, m.Params, write)
	}
	return nil, fmt.Errorf("%s: this agent has no account of its own; it takes only %s and %s", m.Command, CmdWorkerBootstrap, CmdWorkerRelease)
}

// stillRunning reports whether e's client answers, on a context of its own: the
// caller's may be the one that ended.
func stillRunning(ctx context.Context, e *Executor) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return e.status(ctx).Running
}

// slotBootstrap puts the account in a slot (the one it holds, else a free
// one), tells the server, then runs the executor's own bootstrap, which
// validates the session and adopts a client that is already running. A client
// that fails to come up gives its slot back.
func (a *Agent) slotBootstrap(ctx context.Context, raw json.RawMessage, write func(Message) error) (any, error) {
	var p struct {
		Account string `json:"account"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || !ValidAccount(p.Account) {
		return nil, errors.New("worker_bootstrap: params do not name a valid account")
	}
	h, fresh, err := a.take(p.Account)
	if err != nil {
		return nil, fmt.Errorf("worker_bootstrap: %w", err)
	}
	if fresh {
		a.watch(h.ctx, h.Client)
		a.announce(write)
		a.Log.Info("agent: slot taken", "account", p.Account, "slot", h.slot)
	}
	h.op.Lock()
	defer h.op.Unlock()
	res, err := h.Executor.Execute(ctx, CmdWorkerBootstrap, raw)
	if err != nil && !stillRunning(ctx, h.Executor) {
		a.drop(h)
		a.announce(write)
		a.Log.Info("agent: slot given back, the client did not come up", "account", p.Account, "slot", h.slot)
	}
	return res, err
}

// slotRelease logs the account out, stops its client and frees its slot. A
// release that fails with the client still running keeps the slot, so the
// server can try again; one whose client is gone frees it, whatever the stop
// said.
func (a *Agent) slotRelease(ctx context.Context, raw json.RawMessage, write func(Message) error) error {
	var p ReleaseParams
	if err := json.Unmarshal(raw, &p); err != nil || !ValidAccount(p.Account) {
		return errors.New("worker_release: params do not name a valid account")
	}
	h, ok := a.slots.lookup(p.Account)
	if !ok {
		return nil
	}
	h.op.Lock()
	defer h.op.Unlock()
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if err := h.Executor.Shutdown(sctx); err != nil {
		if stillRunning(ctx, h.Executor) {
			return fmt.Errorf("worker_release: %w (the client is still running: the slot stays)", err)
		}
		a.Log.Warn("agent: release: the stop failed but the client is gone", "account", p.Account, "err", err)
	}
	a.drop(h)
	a.announce(write)
	a.Log.Info("agent: slot released", "account", p.Account, "slot", h.slot)
	return nil
}
