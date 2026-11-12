package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/events"
)

// sseMessage is one parsed message of the stream (a comment has only Comment).
type sseMessage struct {
	Event, Data, Comment string
	Retry                string
}

func readSSE(t *testing.T, sc *bufio.Scanner) sseMessage {
	t.Helper()
	var m sseMessage
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			return m
		case strings.HasPrefix(line, ":"):
			m.Comment = strings.TrimSpace(line[1:])
		case strings.HasPrefix(line, "event: "):
			m.Event = line[len("event: "):]
		case strings.HasPrefix(line, "data: "):
			m.Data = line[len("data: "):]
		case strings.HasPrefix(line, "retry: "):
			m.Retry = line[len("retry: "):]
		}
	}
	t.Fatalf("stream ended: %v", sc.Err())
	return m
}

func newLiveServer(t *testing.T, hub *events.Hub, dev bool, beat time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: &content.Content{}, Events: hub, DevMode: dev, EventsHeartbeat: beat,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	t.Cleanup(srv.Close)
	return srv
}

func openStream(ctx context.Context, t *testing.T, url string) (*http.Response, *bufio.Scanner) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp, bufio.NewScanner(resp.Body)
}

func TestLiveEventsStream(t *testing.T) {
	hub := events.NewHub([]string{"oziris-main", "bruhsailer-main"})
	srv := newLiveServer(t, hub, true, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, sc := openStream(ctx, t, srv.URL+"/api/events?account=acc_live")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	if m := readSSE(t, sc); m.Retry != "3000" {
		t.Fatalf("first message = %+v, want the retry hint", m)
	}
	hello := readSSE(t, sc)
	if hello.Event != "hello" || !strings.Contains(hello.Data, `"account":"acc_live"`) {
		t.Fatalf("hello = %+v", hello)
	}

	// The subscription exists once hello is out; other accounts get nothing.
	hub.Publish(events.Event{Type: events.TypeProgress, Account: "acc_other", Source: events.SourcePlugin, StepsChanged: 9})
	hub.Publish(events.Event{Type: events.TypeProgress, Account: "acc_live", Source: events.SourcePlugin, StepsChanged: 2})

	var got sseMessage
	for got.Event == "" { // skip heartbeats
		got = readSSE(t, sc)
	}
	if got.Event != "progress" {
		t.Fatalf("event = %+v", got)
	}
	var e events.Event
	if err := json.Unmarshal([]byte(got.Data), &e); err != nil {
		t.Fatal(err)
	}
	if e.Type != "progress" || e.Account != "acc_live" || e.Source != "plugin" || e.StepsChanged != 2 ||
		len(e.Guides) != 2 || e.At.IsZero() {
		t.Errorf("payload = %+v", e)
	}

	// Heartbeat: a comment arrives with nothing published.
	beat := readSSE(t, sc)
	if beat.Comment != "ping" {
		t.Errorf("heartbeat = %+v", beat)
	}

	// Disconnect: the handler unsubscribes.
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for hub.Subscribers("acc_live") != 0 {
		if time.Now().After(deadline) {
			t.Fatal("subscription still open after the client left")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLiveEventsRoute(t *testing.T) {
	hub := events.NewHub(nil)
	t.Run("absent outside dev mode", func(t *testing.T) {
		srv := newLiveServer(t, hub, false, 0)
		var body map[string]any
		if resp := getJSON(t, srv.URL+"/api/events?account=acc", &body); resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
	t.Run("absent without a hub", func(t *testing.T) {
		srv := newLiveServer(t, nil, true, 0)
		var body map[string]any
		if resp := getJSON(t, srv.URL+"/api/events?account=acc", &body); resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
	t.Run("account is required", func(t *testing.T) {
		srv := newLiveServer(t, hub, true, 0)
		var body map[string]any
		if resp := getJSON(t, srv.URL+"/api/events", &body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})
}
