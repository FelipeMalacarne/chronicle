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
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	storemicrobot "github.com/FelipeMalacarne/ironman-planner/internal/store/microbot"
)

type memAudit struct {
	mu      sync.Mutex
	entries []storemicrobot.Entry
}

func (m *memAudit) Append(_ context.Context, e storemicrobot.Entry) (storemicrobot.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = int64(len(m.entries) + 1)
	m.entries = append(m.entries, e)
	return e, nil
}

func (m *memAudit) List(_ context.Context, limit int) ([]storemicrobot.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]storemicrobot.Entry(nil), m.entries...), nil
}

type stubGame struct{}

func (stubGame) State(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"loggedIn":true,"player":{"name":"EventSourced"}}`), nil
}
func (stubGame) LoginState(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"currentWorld":303}`), nil
}
func (stubGame) Login(context.Context, int) (json.RawMessage, error) {
	return json.RawMessage(`{"success":true}`), nil
}
func (stubGame) Logout(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"success":true}`), nil
}
func (stubGame) Screenshot(context.Context) ([]byte, error) { return []byte("\x89PNG"), nil }

const agentToken = "0123456789abcdef0123456789abcdef"

func microbotServer(t *testing.T) (*httptest.Server, *memAudit) {
	t.Helper()
	audit := &memAudit{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: &agent.Link{Log: quiet}, Audit: audit, AgentToken: agentToken},
	}))
	t.Cleanup(srv.Close)
	return srv, audit
}

func call(t *testing.T, method, url, body string) (int, map[string]any, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, raw
}

func TestMicrobotRoutes(t *testing.T) {
	srv, audit := microbotServer(t)

	// Without an agent: status says so, actions are 503 and still audited.
	status, body, _ := call(t, "GET", srv.URL+"/api/microbot/status", "")
	if status != http.StatusOK || body["enabled"] != true || body["agent"].(map[string]any)["connected"] != false {
		t.Fatalf("status without agent = %d %v", status, body)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/logout", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("logout without agent = %d", status)
	}
	if len(audit.entries) != 1 || audit.entries[0].OK || audit.entries[0].Action != "logout" || audit.entries[0].Actor != "dev" {
		t.Fatalf("audit = %+v", audit.entries)
	}

	// The agent link needs the machine token.
	if status, _, _ = call(t, "GET", srv.URL+"/api/microbot/agent", ""); status != http.StatusUnauthorized {
		t.Fatalf("agent without token = %d", status)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := &agent.Agent{URL: srv.URL, Token: agentToken, Log: quiet, Hello: agent.Hello{Host: "zaros"},
		Executor: &agent.Executor{Game: stubGame{}, Home: t.TempDir(), ClientCmd: []string{"echo"}}}
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, body, _ = call(t, "GET", srv.URL+"/api/microbot/status", "")
		if body["agent"].(map[string]any)["connected"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent never linked")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if client := body["client"].(map[string]any); client["running"] != true {
		t.Fatalf("status with agent = %v", body)
	}

	status, body, _ = call(t, "POST", srv.URL+"/api/microbot/login", `{"world":303}`)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("login = %d %v", status, body)
	}
	if last := audit.entries[len(audit.entries)-1]; !last.OK || last.Action != "login" || string(last.Params) != `{"world":303}` {
		t.Fatalf("login audit = %+v", last)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/login", `{"world":-1}`); status != http.StatusBadRequest {
		t.Fatalf("bad world = %d", status)
	}
	status, _, raw := call(t, "GET", srv.URL+"/api/microbot/screenshot", "")
	if status != http.StatusOK || string(raw) != "\x89PNG" {
		t.Fatalf("screenshot = %d %q", status, raw)
	}
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/audit", "")
	if entries, _ := body["entries"].([]any); status != http.StatusOK || len(entries) != 2 {
		t.Fatalf("audit route = %d %v", status, body)
	}
}

// Without MICROBOT_CONTROL no route exists.
func TestMicrobotOff(t *testing.T) {
	srv := httptest.NewServer(New(Options{Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	defer srv.Close()
	for _, path := range []string{"/api/microbot/status", "/api/microbot/agent"} {
		if status, _, _ := call(t, "GET", srv.URL+path, ""); status != http.StatusNotFound {
			t.Errorf("%s with control off = %d", path, status)
		}
	}
}

// The control check: loopback only in dev, the role in oidc.
func TestMicrobotControlCheck(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	run := func(s *Server, remote string, roles []string) int {
		r := httptest.NewRequest("GET", "/api/microbot/status", nil)
		r.RemoteAddr = remote
		if roles != nil {
			r = r.WithContext(context.WithValue(r.Context(), rolesKey{}, roles))
		}
		w := httptest.NewRecorder()
		s.microbotControl(ok)(w, r)
		return w.Code
	}
	dev := &Server{opts: Options{DevMode: true}}
	if code := run(dev, "127.0.0.1:5000", nil); code != http.StatusNoContent {
		t.Errorf("dev loopback = %d", code)
	}
	if code := run(dev, "100.64.0.7:5000", nil); code != http.StatusForbidden {
		t.Errorf("dev from the tailnet = %d", code)
	}
	oidc := &Server{opts: Options{}}
	if code := run(oidc, "100.64.0.7:5000", []string{"owner"}); code != http.StatusForbidden {
		t.Errorf("oidc without the role = %d", code)
	}
	if code := run(oidc, "100.64.0.7:5000", []string{"owner", MicrobotRole}); code != http.StatusNoContent {
		t.Errorf("oidc with the role = %d", code)
	}
}
