// Package events is the in-process pub/sub behind GET /api/events: after a
// transaction that changed an account's progress commits, the store publishes a
// lightweight invalidation signal and every open stream of that account gets
// it. Clients refetch over the regular API; an event is never a second source
// of truth (docs/04 section 6.5).
//
// The hub lives outside internal/core because it is runtime plumbing
// (goroutines, channels, a mutex) shared by internal/store and
// internal/httpapi, not domain logic; it imports neither the database nor HTTP.
// It is single-process by design: one `ironman serve` owns the database, so no
// broker (Redis, LISTEN/NOTIFY) is needed.
package events

import (
	"sync"
	"time"
)

// Event types.
const (
	// TypeProgress: steps, objectives, marks, cursor or facts of the account
	// changed; refetch what depends on progress.
	TypeProgress = "progress"
	// TypeSync: a polling sync run finished (ok, unchanged, error or
	// rate_limited); refetch the source status.
	TypeSync = "sync"
)

// Event sources.
const (
	SourcePlugin   = "plugin"
	SourceHiscores = "hiscores"
	SourceOwner    = "owner"
)

// Event is the payload of one SSE message (`data:`), named by Type (`event:`).
type Event struct {
	Type    string `json:"type"`
	Account string `json:"account"`
	Source  string `json:"source"`
	// Guides the event concerns. A progress event published without guides
	// (a reevaluation covers every route) gets the hub's full list.
	Guides []string `json:"guides,omitempty"`
	// StepsChanged counts the step status changes caused by the engine; owner
	// actions may report 0.
	StepsChanged int `json:"steps_changed"`
	// Status of the sync run (sync events only).
	Status string    `json:"status,omitempty"`
	At     time.Time `json:"at"`
}

// Publisher is what the store needs: publish after commit, never block.
type Publisher interface {
	Publish(Event)
}

var _ Publisher = (*Hub)(nil)

// DefaultBuffer is the per-subscriber queue length.
const DefaultBuffer = 16

// Hub fans events out to the subscribers of each account. Publish never
// blocks: a subscriber whose queue is full is dropped (its channel closes), so
// a slow browser can never stall a reevaluation; its stream ends and the
// client reconnects and refetches everything.
type Hub struct {
	mu     sync.Mutex
	subs   map[string]map[*Subscription]struct{}
	guides []string
	buffer int
	now    func() time.Time
}

// NewHub returns a hub; guides is the list given to progress events published
// without guides (every loaded guide).
func NewHub(guides []string) *Hub {
	return &Hub{
		subs:   map[string]map[*Subscription]struct{}{},
		guides: append([]string(nil), guides...),
		buffer: DefaultBuffer,
		now:    time.Now,
	}
}

// Subscription is one open stream. C closes when the subscription ends:
// Close was called, or the hub dropped it for being too slow.
type Subscription struct {
	C       <-chan Event
	c       chan Event
	hub     *Hub
	account string
	closed  bool // guarded by hub.mu
}

// Subscribe opens a subscription to the account's events.
func (h *Hub) Subscribe(account string) *Subscription {
	c := make(chan Event, h.buffer)
	s := &Subscription{C: c, c: c, hub: h, account: account}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[account] == nil {
		h.subs[account] = map[*Subscription]struct{}{}
	}
	h.subs[account][s] = struct{}{}
	return s
}

// Close ends the subscription; it is safe to call more than once and after
// the hub dropped it.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.removeLocked(s)
}

func (h *Hub) removeLocked(s *Subscription) {
	if s.closed {
		return
	}
	s.closed = true
	close(s.c)
	if set := h.subs[s.account]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, s.account)
		}
	}
}

// Publish delivers e to every subscriber of e.Account without blocking. A zero
// At becomes now; a progress event without guides gets the hub's list.
func (h *Hub) Publish(e Event) {
	if e.At.IsZero() {
		e.At = h.now().UTC()
	}
	if e.Type == TypeProgress && len(e.Guides) == 0 {
		e.Guides = h.guides
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[e.Account] {
		select {
		case s.c <- e:
		default:
			h.removeLocked(s)
		}
	}
}

// Subscribers returns how many subscriptions the account has open.
func (h *Hub) Subscribers(account string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[account])
}
