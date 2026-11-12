package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"
	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/auth"
	"github.com/FelipeMalacarne/ironman-planner/internal/auth/authtest"
)

// fakeWorkers takes "pod:<name>:<account>" tokens.
type fakeWorkers struct{}

func (fakeWorkers) VerifyWorkerToken(_ context.Context, token string) (WorkerIdentity, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 || parts[0] != "pod" {
		return WorkerIdentity{}, errors.New("not a pod token")
	}
	return WorkerIdentity{Pod: parts[1], Account: parts[2]}, nil
}

// startAgent links a real agent whose fake mb-client echoes marker.
func startAgent(t *testing.T, url string, token string, hello agent.Hello, marker string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := &agent.Agent{URL: url, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Hello: hello, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: stubGame{}, Home: t.TempDir(), ClientCmd: []string{"echo", marker}}}
	if token != "" {
		a.Token = agent.StaticToken(token)
	}
	go func() { _ = a.Run(ctx) }()
}

func waitLinked(t *testing.T, link *agent.Link, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(link.Agents()) != n {
		if time.Now().After(deadline) {
			t.Fatalf("agents = %+v, want %d", link.Agents(), n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Two agents (dev mode, both from loopback): ?account= picks each one, no
// account is a 409, the status lists both and the audit names account and agent.
func TestMicrobotRoutesByAccount(t *testing.T) {
	audit := &memAudit{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: link, Audit: audit},
	}))
	t.Cleanup(srv.Close)
	startAgent(t, srv.URL, "", agent.Hello{Host: "zaros", Account: "acc-a"}, "A")
	startAgent(t, srv.URL, "", agent.Hello{Host: "box", Account: "acc-b", Kind: agent.KindManaged, Worker: "worker-acc-b-1"}, "B")
	waitLinked(t, link, 2)

	for account, want := range map[string]string{"acc-a": "A ", "acc-b": "B "} {
		status, body, _ := call(t, "POST", srv.URL+"/api/microbot/client/start?account="+account, "")
		if status != http.StatusOK {
			t.Fatalf("%s start = %d %v", account, status, body)
		}
		if out := body["result"].(map[string]any)["output"].(string); !strings.HasPrefix(out, want) {
			t.Fatalf("%s went to %q", account, out)
		}
		served, _ := link.Agent(account)
		if last := audit.last(); last.AccountID != account || last.AgentID != served.ID || last.AgentID == "" || !last.OK {
			t.Fatalf("%s audit = %+v (agent %s)", account, last, served.ID)
		}
	}
	status, body, _ := call(t, "POST", srv.URL+"/api/microbot/client/start", "")
	if status != http.StatusConflict {
		t.Fatalf("no account with two agents = %d %v", status, body)
	}
	if last := audit.last(); last.OK || last.AgentID != "" {
		t.Fatalf("ambiguous audit = %+v", last)
	}
	if status, _, _ = call(t, "GET", srv.URL+"/api/microbot/plugins", ""); status != http.StatusConflict {
		t.Fatalf("read with two agents = %d", status)
	}
	if status, _, _ = call(t, "GET", srv.URL+"/api/microbot/plugins?account=acc-b", ""); status != http.StatusOK {
		t.Fatalf("read with the account = %d", status)
	}
	if shot, _, raw := call(t, "GET", srv.URL+"/api/microbot/screenshot?account=acc-a", ""); shot != http.StatusOK || string(raw) != "\x89PNG" {
		t.Fatalf("screenshot = %d", shot)
	}

	// Status: the list, and the account's agent with its client (the shape of today plus fields).
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/status?account=acc-b", "")
	if status != http.StatusOK || len(body["agents"].([]any)) != 2 {
		t.Fatalf("status = %d %v", status, body)
	}
	if a := body["agent"].(map[string]any); a["connected"] != true || a["account"] != "acc-b" || a["kind"] != "managed" || a["worker"] != "worker-acc-b-1" {
		t.Fatalf("status agent = %v", a)
	}
	if body["client"].(map[string]any)["running"] != true || body["account"] != "acc-b" {
		t.Fatalf("status client = %v", body)
	}
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/status", "")
	if status != http.StatusOK || body["agent"].(map[string]any)["connected"] != false || body["error"] == nil || body["client"] != nil {
		t.Fatalf("status without account = %d %v", status, body)
	}

	// The audit by account.
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/audit?account=acc-a", "")
	entries, _ := body["entries"].([]any)
	if status != http.StatusOK || len(entries) != 1 || entries[0].(map[string]any)["account_id"] != "acc-a" {
		t.Fatalf("audit of acc-a = %d %v", status, body)
	}

	// VNC goes through the account's agent: without x11vnc it fails, audited with the agent.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/microbot/vnc"
	if _, resp, err := websocket.Dial(ctx, wsURL, nil); err == nil || resp == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("vnc without an account = %v", err)
	}
	ws, _, err := websocket.Dial(ctx, wsURL+"?account=acc-b", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ws.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusTryAgainLater {
		t.Fatalf("vnc without x11vnc = %v", err)
	}
	served, _ := link.Agent("acc-b")
	if last := audit.last(); last.Action != agent.CmdVNCOpen || last.AccountID != "acc-b" || last.AgentID != served.ID {
		t.Fatalf("vnc audit = %+v", last)
	}
}

// The pod path of agentAuth: a worker token, bound to its account.
func TestMicrobotAgentWorkerToken(t *testing.T) {
	var got agent.Identity
	ok := func(w http.ResponseWriter, r *http.Request) {
		got = agent.IdentityOf(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
	run := func(s *Server, remote, token string) int {
		r := httptest.NewRequest("GET", "/api/microbot/agent", nil)
		r.RemoteAddr = remote
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.agentAuth(ok)(w, r)
		return w.Code
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	dev := &Server{opts: Options{DevMode: true, Microbot: &MicrobotOptions{Workers: fakeWorkers{}}}, log: quiet}
	if code := run(dev, "127.0.0.1:5000", ""); code != http.StatusNoContent || got.Subject != "dev" || got.BoundAccount != "" {
		t.Fatalf("dev loopback = %d %+v", code, got)
	}
	if code := run(dev, "10.42.0.9:5000", "pod:worker-acc-b-7f:acc_b"); code != http.StatusNoContent ||
		got != (agent.Identity{Subject: "pod:worker-acc-b-7f", BoundAccount: "acc_b", Pod: "worker-acc-b-7f"}) {
		t.Fatalf("dev pod = %d %+v", code, got)
	}
	if code := run(dev, "10.42.0.9:5000", "garbage"); code != http.StatusUnauthorized {
		t.Fatalf("dev bad pod token = %d", code)
	}
	if code := run(dev, "10.42.0.9:5000", ""); code != http.StatusForbidden {
		t.Fatalf("dev remote without token = %d", code)
	}
	// Without a verifier the remote peer is refused as before.
	plain := &Server{opts: Options{DevMode: true, Microbot: &MicrobotOptions{}}, log: quiet}
	if code := run(plain, "10.42.0.9:5000", "pod:w:acc_b"); code != http.StatusForbidden {
		t.Fatalf("dev without workers = %d", code)
	}
}

// In oidc: a pod links with its own token, a pod declaring another account
// is refused, and ?account= of another user's account is a 404.
func TestMicrobotWorkersOIDC(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	audit := &memAudit{}
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) {
		o.Microbot = &MicrobotOptions{Link: link, Audit: audit, Workers: fakeWorkers{}}
	})
	startAgent(t, e.srv.URL, "pod:worker-acc-1-x:acc-1", agent.Hello{Host: "worker-acc-1-x", Account: "acc-1", Kind: agent.KindManaged, Worker: "worker-acc-1-x"}, "POD")
	waitLinked(t, link, 1)
	if a, _ := link.Agent("acc-1"); a.Identity != "pod:worker-acc-1-x" {
		t.Fatalf("pod agent = %+v", a)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, agent.LinkURL(e.srv.URL), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer pod:worker-acc-1-y:acc-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	hello, _ := json.Marshal(agent.Message{Type: agent.TypeHello, Hello: &agent.Hello{Host: "y", Account: "acc-2", Kind: agent.KindManaged}})
	_ = conn.Write(ctx, websocket.MessageText, hello)
	if _, _, err = conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("pod declaring another account = %v", err)
	}
	if n := len(link.Agents()); n != 1 {
		t.Fatalf("agents after the refusal = %d", n)
	}

	c := e.client()
	e.login(c, "/", authtest.User{Subject: "100", Username: "felipe", Roles: []string{"owner", MicrobotRole}})
	if status, body := e.do(c, "GET", "/api/microbot/status?account=acc-1", "", false); status != http.StatusOK ||
		body["agent"].(map[string]any)["account"] != "acc-1" || len(body["agents"].([]any)) != 1 {
		t.Fatalf("own account status = %d %v", status, body)
	}
	for _, acc := range []string{"acc-2", "acc-missing"} {
		if status, _ := e.do(c, "GET", "/api/microbot/status?account="+acc, "", false); status != http.StatusNotFound {
			t.Fatalf("status of %s = %d", acc, status)
		}
		if status, _ := e.do(c, "POST", "/api/microbot/client/start?account="+acc, "", true); status != http.StatusNotFound {
			t.Fatalf("start on %s = %d", acc, status)
		}
	}
	status, body := e.do(c, "POST", "/api/microbot/client/start?account=acc-1", "", true)
	if status != http.StatusOK || !strings.HasPrefix(body["result"].(map[string]any)["output"].(string), "POD ") {
		t.Fatalf("own start = %d %v", status, body)
	}
	if last := audit.last(); last.AccountID != "acc-1" || last.Actor == "dev" {
		t.Fatalf("audit = %+v", last)
	}
}
