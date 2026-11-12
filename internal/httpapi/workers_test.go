package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	storeworkers "github.com/FelipeMalacarne/ironman-planner/internal/store/workers"
	"github.com/FelipeMalacarne/ironman-planner/internal/workerctl"
)

// memWorkers is an in-memory WorkerStore with the repository's rules that
// the routes rely on (no running without a session).
type memWorkers struct {
	mu       sync.Mutex
	sessions map[string]agent.GameSession
	workers  map[string]storeworkers.Worker
	busy     bool
}

func newMemWorkers() *memWorkers {
	return &memWorkers{sessions: map[string]agent.GameSession{}, workers: map[string]storeworkers.Worker{}}
}

func (m *memWorkers) PutSession(_ context.Context, account, _ string, s agent.GameSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.SessionID == "" || s.CharacterID == "" || s.Character == "" {
		return storeworkers.ErrInvalid
	}
	m.sessions[account] = s
	return nil
}

func (m *memWorkers) SessionInfo(_ context.Context, account string) (storeworkers.SessionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[account]
	if !ok {
		return storeworkers.SessionInfo{}, storeworkers.ErrNoSession
	}
	return storeworkers.SessionInfo{AccountID: account, Character: s.Character, PushedAt: time.Now(), PushedBy: "dev"}, nil
}

func (m *memWorkers) DeleteSession(_ context.Context, account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workers[account].DesiredState == storeworkers.StateRunning {
		return storeworkers.ErrWorkerRunning
	}
	delete(m.sessions, account)
	return nil
}

func (m *memWorkers) SetDesired(_ context.Context, account, state string, world int, image string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[account]; !ok && state == storeworkers.StateRunning {
		return storeworkers.ErrNoSession
	}
	w := m.workers[account]
	w.AccountID, w.DesiredState, w.World = account, state, world
	if image != "" {
		w.Image = image
	}
	m.workers[account] = w
	return nil
}

func (m *memWorkers) Get(_ context.Context, account string) (storeworkers.Worker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workers[account]
	if !ok {
		return storeworkers.Worker{}, storeworkers.ErrNotFound
	}
	return w, nil
}

func (m *memWorkers) List(context.Context) ([]storeworkers.Worker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []storeworkers.Worker
	for _, w := range m.workers {
		out = append(out, w)
	}
	return out, nil
}

func (m *memWorkers) ListForUser(ctx context.Context, _ string) ([]storeworkers.Worker, error) {
	return m.List(ctx)
}

func (m *memWorkers) OtherClientSince(context.Context, string, time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.busy, nil
}

type memControl struct {
	mu       sync.Mutex
	kicks    int
	restarts []string
	pods     map[string]workerctl.PodInfo
}

func (c *memControl) Kick() { c.mu.Lock(); c.kicks++; c.mu.Unlock() }
func (c *memControl) Pods() map[string]workerctl.PodInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pods
}
func (c *memControl) Restart(_ context.Context, account string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restarts = append(c.restarts, account)
	return nil
}

func workersServer(t *testing.T, control WorkerControl) (*httptest.Server, *memWorkers, *memAudit, *agent.Link) {
	t.Helper()
	store, audit := newMemWorkers(), &memAudit{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: link, Audit: audit, Sessions: store, Control: control, WorkerImage: "ghcr.io/felipemalacarne/chronicle-worker:new"},
	}))
	t.Cleanup(srv.Close)
	return srv, store, audit, link
}

const sessionBody = `{"character":"EventSourced","session_id":"jx-secret-sid","character_id":"123","device_ids":"a=b"}`

func TestSessionRoutes(t *testing.T) {
	srv, store, audit, _ := workersServer(t, nil)
	if status, _, _ := call(t, "GET", srv.URL+"/api/accounts/session?account=acc_a", ""); status != http.StatusConflict {
		t.Fatalf("no session = %d", status)
	}
	if status, _, _ := call(t, "PUT", srv.URL+"/api/accounts/session?account=acc_a", `{"character":"x"}`); status != http.StatusBadRequest {
		t.Fatalf("incomplete session = %d", status)
	}
	status, body, raw := call(t, "PUT", srv.URL+"/api/accounts/session?account=acc_a", sessionBody)
	if status != http.StatusOK || body["character"] != "EventSourced" {
		t.Fatalf("put session = %d %s", status, raw)
	}
	if strings.Contains(string(raw), "jx-secret-sid") {
		t.Fatal("the session went back to the client")
	}
	if store.sessions["acc_a"].SessionID != "jx-secret-sid" {
		t.Fatal("session not stored")
	}
	_, _, raw = call(t, "GET", srv.URL+"/api/accounts/session?account=acc_a", "")
	if strings.Contains(string(raw), "jx-secret-sid") {
		t.Fatal("GET answered the session")
	}
	e := audit.last()
	if e.Action != "session_put" || e.AccountID != "acc_a" || strings.Contains(string(e.Params), "jx") {
		t.Fatalf("audit = %+v", e)
	}
	// WORKERS=off: sessions work, the lifecycle does not.
	if status, _, _ := call(t, "POST", srv.URL+"/api/workers/start?account=acc_a", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("start with workers off = %d", status)
	}
	if status, body, _ := call(t, "GET", srv.URL+"/api/workers", ""); status != http.StatusOK || body["enabled"] != false {
		t.Fatalf("list with workers off = %d %v", status, body)
	}
}

func TestWorkerLifecycleRoutes(t *testing.T) {
	control := &memControl{}
	srv, store, audit, link := workersServer(t, control)
	if status, _, _ := call(t, "POST", srv.URL+"/api/workers/start?account=acc_a", ""); status != http.StatusConflict {
		t.Fatalf("start without session = %d", status)
	}
	call(t, "PUT", srv.URL+"/api/accounts/session?account=acc_a", sessionBody)
	if status, _, _ := call(t, "POST", srv.URL+"/api/workers/start?account=acc_a", `{"world":1000}`); status != http.StatusBadRequest {
		t.Fatalf("bad world = %d", status)
	}
	// Someone plays the account on another client: refused, and audited.
	store.busy = true
	if status, body, _ := call(t, "POST", srv.URL+"/api/workers/start?account=acc_a", ""); status != http.StatusConflict || !strings.Contains(body["error"].(string), "another client") {
		t.Fatalf("start while played elsewhere = %d %v", status, body)
	}
	if e := audit.last(); e.Action != "worker_start" || e.OK {
		t.Fatalf("refusal not audited: %+v", e)
	}
	store.busy = false
	status, _, raw := call(t, "POST", srv.URL+"/api/workers/start?account=acc_a", `{"world":420}`)
	if status != http.StatusOK {
		t.Fatalf("start = %d %s", status, raw)
	}
	w := store.workers["acc_a"]
	if w.DesiredState != storeworkers.StateRunning || w.World != 420 || w.Image != "ghcr.io/felipemalacarne/chronicle-worker:new" || control.kicks != 1 {
		t.Fatalf("after start: %+v kicks %d", w, control.kicks)
	}
	// The pod runs an older image: the list says so.
	control.pods = map[string]workerctl.PodInfo{"acc_a": {Deployment: "worker-acc-a", State: "running", Image: "ghcr.io/felipemalacarne/chronicle-worker:old"}}
	_, body, raw := call(t, "GET", srv.URL+"/api/workers", "")
	ws := body["workers"].([]any)
	if len(ws) != 1 || ws[0].(map[string]any)["outdated"] != true || ws[0].(map[string]any)["pod"] == nil {
		t.Fatalf("list = %s", raw)
	}
	// Same image as the server's: restart deletes the pod.
	if status, _, _ := call(t, "POST", srv.URL+"/api/workers/restart?account=acc_a", ""); status != http.StatusOK || len(control.restarts) != 1 {
		t.Fatalf("restart = %d %v", status, control.restarts)
	}
	if status, _, _ := call(t, "DELETE", srv.URL+"/api/accounts/session?account=acc_a", ""); status != http.StatusConflict {
		t.Fatalf("delete session of a running worker = %d", status)
	}
	if status, _, _ := call(t, "POST", srv.URL+"/api/workers/stop?account=acc_a", ""); status != http.StatusOK || store.workers["acc_a"].DesiredState != storeworkers.StateStopped {
		t.Fatalf("stop = %d %+v", status, store.workers["acc_a"])
	}
	if status, _, _ := call(t, "POST", srv.URL+"/api/workers/restart?account=acc_a", ""); status != http.StatusConflict {
		t.Fatalf("restart of a stopped worker = %d", status)
	}

	// The account is active on the attached agent (zaros): start refused.
	startAgent(t, srv.URL, "", agent.Hello{Host: "zaros", Account: "acc_a"}, "z")
	waitLinked(t, link, 1)
	if status, got, _ := call(t, "POST", srv.URL+"/api/workers/start?account=acc_a", ""); status != http.StatusConflict || !strings.Contains(got["error"].(string), "attached agent") {
		t.Fatalf("start while attached = %d %v", status, got)
	}
	_, body, _ = call(t, "GET", srv.URL+"/api/workers", "")
	if att := body["attached"].([]any); len(att) != 1 || att[0].(map[string]any)["host"] != "zaros" {
		t.Fatalf("attached = %v", body["attached"])
	}
}
