package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Live stream defaults (docs/04 section 6.6).
const (
	defaultHeartbeat = 25 * time.Second
	// retryMillis is the reconnect delay hinted to EventSource.
	retryMillis = 3000
)

// handleLiveEvents streams the account's invalidation signals as
// text/event-stream (GET /api/events?account=): `event: hello` first, then one
// `event: progress` or `event: sync` per published event, and a comment as
// heartbeat. It ends when the client leaves, the server shuts down (the base
// context) or the hub drops this subscriber for being slow.
func (s *Server) handleLiveEvents(w http.ResponseWriter, r *http.Request) {
	account, ok := accountParam(w, r)
	if !ok {
		return
	}
	rc := http.NewResponseController(w)
	// A server-wide WriteTimeout would cut the stream; lift it for this
	// request (unsupported writers, such as recorders, just return an error).
	_ = rc.SetWriteDeadline(time.Time{})

	sub := s.opts.Events.Subscribe(account)
	defer sub.Close()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-store")
	h.Set("Connection", "keep-alive")
	// Proxies that buffer (nginx) must pass every event through at once.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "retry: %d\n\n", retryMillis)
	hello := map[string]any{"type": "hello", "account": account, "at": time.Now().UTC()}
	if err := writeSSE(w, rc, "hello", hello); err != nil {
		return
	}

	beat := s.opts.EventsHeartbeat
	if beat <= 0 {
		beat = defaultHeartbeat
	}
	ticker := time.NewTicker(beat)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-sub.C:
			if !open {
				return // dropped as a slow consumer: the client reconnects and refetches
			}
			if err := writeSSE(w, rc, e.Type, e); err != nil {
				return
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// writeSSE writes one named event with a JSON data line and flushes it.
func writeSSE(w http.ResponseWriter, rc *http.ResponseController, name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return err
	}
	return rc.Flush()
}
