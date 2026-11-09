package microbot

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// fakeServer serves an Agent Server stand-in on the Unix socket of a temp
// home, with the token file the client reads.
func fakeServer(t *testing.T, h http.Handler) *Client {
	t.Helper()
	home, err := os.MkdirTemp("", "mb") // short path: Unix socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	c := &Client{Home: home}
	if err = os.MkdirAll(filepath.Dir(c.SocketPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(c.TokenPath(), []byte("tok123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", c.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return c
}

func TestClientAgainstAgentServer(t *testing.T) {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Agent-Token") != "tok123" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"bad token"}`))
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /state", auth(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"loggedIn":false,"gameState":"LOGIN_SCREEN"}`))
	}))
	mux.HandleFunc("GET /screenshot", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG fake"))
	}))
	mux.HandleFunc("POST /login", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Non-member account"}`))
	}))
	c := fakeServer(t, mux)
	ctx := context.Background()

	state, err := c.State(ctx)
	if err != nil || string(state) != `{"loggedIn":false,"gameState":"LOGIN_SCREEN"}` {
		t.Fatalf("state = %s %v", state, err)
	}
	png, err := c.Screenshot(ctx)
	if err != nil || string(png) != "\x89PNG fake" {
		t.Fatalf("screenshot = %q %v", png, err)
	}
	var apiErr *APIError
	if _, err := c.Login(ctx, 303); !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Message != "Non-member account" {
		t.Fatalf("login = %v", err)
	}
	// The token is read on every call: a restarted client has a new one.
	if err := os.WriteFile(c.TokenPath(), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.State(ctx); !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("state with a stale token = %v", err)
	}
}

func TestClientNotRunning(t *testing.T) {
	c := &Client{Home: t.TempDir()}
	if _, err := c.State(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("state without a client = %v", err)
	}
}
