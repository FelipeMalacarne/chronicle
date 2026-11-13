package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/microbot"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
	"github.com/FelipeMalacarne/chronicle/internal/workerctl"
)

// memWorkers is an in-memory WorkerStore with the repository's rules that
// the routes rely on (no running without a session).
type memWorkers struct {
	mu       sync.Mutex
	sessions map[string]agent.GameSession
	workers  map[string]storeworkers.Worker
	logins   map[string]string // the last login result by account
	busy     bool
}

func newMemWorkers() *memWorkers {
	return &memWorkers{sessions: map[string]agent.GameSession{}, workers: map[string]storeworkers.Worker{}, logins: map[string]string{}}
}

func (m *memWorkers) RecordLogin(_ context.Context, account, result string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[account]; !ok {
		return storeworkers.ErrNoSession
	}
	m.logins[account] = result
	return nil
}

func (m *memWorkers) lastLogin(account string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.logins[account]
	return r, ok
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
	usage    map[string]workerctl.Usage
	capacity *workerctl.Capacity
}

func (c *memControl) Kick() { c.mu.Lock(); c.kicks++; c.mu.Unlock() }
func (c *memControl) Pods() map[string]workerctl.PodInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pods
}
func (c *memControl) Usage() map[string]workerctl.Usage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usage
}
func (c *memControl) Capacity() (workerctl.Capacity, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.capacity == nil {
		return workerctl.Capacity{}, false
	}
	return *c.capacity, true
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
	if status, body, _ := call(t, "GET", srv.URL+"/api/workers", ""); status != http.StatusOK || body["enabled"] != false || body["capacity"] != nil {
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
	// The pod runs an older image: the list says so. No quota or metrics
	// yet: no capacity and no usage.
	control.pods = map[string]workerctl.PodInfo{"acc_a": {Deployment: "worker-acc-a", State: "running", Image: "ghcr.io/felipemalacarne/chronicle-worker:old"}}
	_, body, raw := call(t, "GET", srv.URL+"/api/workers", "")
	ws := body["workers"].([]any)
	if len(ws) != 1 || ws[0].(map[string]any)["outdated"] != true || ws[0].(map[string]any)["pod"] == nil {
		t.Fatalf("list = %s", raw)
	}
	if _, ok := body["capacity"]; ok || ws[0].(map[string]any)["usage"] != nil {
		t.Fatalf("capacity or usage without them: %s", raw)
	}
	// The metrics and the quota: usage next to the pod, capacity on top.
	cpuFree := 0.78
	control.mu.Lock()
	control.usage = map[string]workerctl.Usage{
		"acc_a":    {CPUCores: 0.404, MemoryBytes: 877350912, MeasuredAt: time.Now()},
		"acc_gone": {CPUCores: 1}, // no pod in the view: not shown
	}
	control.capacity = &workerctl.Capacity{Fits: 1, LimitedBy: "requests.cpu", CPUFreeCores: &cpuFree, WorkerCPUCores: 0.7, WorkerMemoryBytes: 1 << 30}
	control.mu.Unlock()
	_, body, raw = call(t, "GET", srv.URL+"/api/workers", "")
	ws = body["workers"].([]any)
	usage, _ := ws[0].(map[string]any)["usage"].(map[string]any)
	capacity, _ := body["capacity"].(map[string]any)
	if usage["cpu_cores"] != 0.404 || usage["memory_bytes"] != float64(877350912) || capacity["fits"] != float64(1) ||
		capacity["cpu_free_cores"] != 0.78 || capacity["limited_by"] != "requests.cpu" {
		t.Fatalf("usage and capacity = %s", raw)
	}
	if _, ok := capacity["memory_free_bytes"]; ok {
		t.Fatalf("an unknown free memory was answered: %s", raw)
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

// loginGame answers POST /login as the Agent Server does: 200, or 401 with
// the game's reason, or 409 while another login runs.
type loginGame struct {
	stubGame
	status atomic.Int32
}

func (g *loginGame) Login(context.Context, int) (json.RawMessage, error) {
	switch g.status.Load() {
	case http.StatusUnauthorized:
		return nil, &microbot.APIError{Path: "/login", Status: 401, Message: `{"success":false,"message":"Login failed: Invalid credentials","currentWorld":330,"loginIndex":4,"loginError":"Invalid credentials"}`}
	case http.StatusConflict:
		return nil, &microbot.APIError{Path: "/login", Status: 409, Message: "Login attempt already in progress"}
	}
	return json.RawMessage(`{"success":true,"message":"Login successful","currentWorld":330}`), nil
}

// A login from the Cockpit on a managed worker records its outcome on the
// account's session (the login_failed flag); a failure that says nothing
// about the session, or a login on the attached agent, records nothing.
func TestWorkerLoginRecorded(t *testing.T) {
	srv, store, _, link := workersServer(t, &memControl{})
	for _, account := range []string{"acc_a", "acc_z"} {
		if status, _, raw := call(t, "PUT", srv.URL+"/api/accounts/session?account="+account, sessionBody); status != http.StatusOK {
			t.Fatalf("put session = %d %s", status, raw)
		}
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	run := func(hello agent.Hello, game agent.Game) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: hello, VNCAddr: "127.0.0.1:1",
			Executor: &agent.Executor{Game: game, Home: t.TempDir(), ClientCmd: []string{"echo"}}}
		go func() { _ = a.Run(ctx) }()
	}
	worker := &loginGame{}
	run(agent.Hello{Host: "worker-acc-a-1", Account: "acc_a", Kind: agent.KindManaged, Worker: "worker-acc-a-1"}, worker)
	attached := &loginGame{}
	attached.status.Store(http.StatusUnauthorized)
	run(agent.Hello{Host: "zaros", Account: "acc_z"}, attached)
	waitLinked(t, link, 2)

	login := func(account string) int {
		status, _, _ := call(t, "POST", srv.URL+"/api/microbot/login?account="+account, `{"world":330}`)
		return status
	}
	worker.status.Store(http.StatusUnauthorized)
	if status := login("acc_a"); status != http.StatusBadGateway {
		t.Fatalf("refused login = %d", status)
	}
	if got, _ := store.lastLogin("acc_a"); got != "Invalid credentials" {
		t.Fatalf("refused login recorded as %q", got)
	}
	worker.status.Store(http.StatusConflict)
	login("acc_a")
	if got, _ := store.lastLogin("acc_a"); got != "Invalid credentials" {
		t.Fatalf("a login in progress changed the result to %q", got)
	}
	worker.status.Store(http.StatusOK)
	if status := login("acc_a"); status != http.StatusOK {
		t.Fatalf("login = %d", status)
	}
	if got, _ := store.lastLogin("acc_a"); got != storeworkers.LoginOK {
		t.Fatalf("login recorded as %q", got)
	}
	// The attached agent logs in with the launcher's session, not the stored one.
	login("acc_z")
	if got, ok := store.lastLogin("acc_z"); ok {
		t.Fatalf("attached login recorded: %q", got)
	}
}
