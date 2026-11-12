package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"
	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/auth"
	"github.com/FelipeMalacarne/ironman-planner/internal/auth/authtest"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
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

func (m *memAudit) List(_ context.Context, limit int, _ string) ([]storemicrobot.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]storemicrobot.Entry(nil), m.entries...), nil
}

func (m *memAudit) last() storemicrobot.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[len(m.entries)-1]
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

// JSON answers every plugin command with the request it got.
func (stubGame) JSON(_ context.Context, method, path string, _ url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	return json.Marshal(map[string]string{"request": method + " " + path})
}

func microbotServer(t *testing.T) (*httptest.Server, *memAudit) {
	t.Helper()
	audit := &memAudit{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: &agent.Link{Log: quiet}, Audit: audit},
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

	if status, _, _ = call(t, "GET", srv.URL+"/api/microbot/vnc", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("vnc without agent = %d", status)
	}

	// In dev mode the agent links from loopback with no token; a VNC stream
	// nobody waits for is refused.
	if status, _, _ = call(t, "GET", srv.URL+"/api/microbot/agent/vnc?session=x", ""); status != http.StatusNotFound {
		t.Fatalf("agent vnc stream for no session = %d", status)
	}
	vncAddr := fakeX11vnc(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros"}, VNCAddr: vncAddr,
		Executor: &agent.Executor{Game: stubGame{}, Home: t.TempDir(), ClientCmd: []string{"echo"}}}
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, body, _ = call(t, "GET", srv.URL+"/api/microbot/status", "")
		if info := body["agent"].(map[string]any); info["connected"] == true {
			if info["identity"] != "dev" {
				t.Fatalf("dev agent identity = %v", info)
			}
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

	// Plugins (phase 3): reads go through the agent, refusals never reach it,
	// actions are audited with the params that passed validation.
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/plugins", "")
	if status != http.StatusOK || body["scripts"] == nil {
		t.Fatalf("plugins = %d %v", status, body)
	}
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/config?group=qol&keys=speed,%20mode", "")
	if status != http.StatusOK || body["group"] != "qol" {
		t.Fatalf("config = %d %v", status, body)
	}
	before := len(audit.entries)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/microbot/scripts/stop", `{"className":"net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin"}`},
		{"POST", "/api/microbot/config", `{"group":"agentServer","key":"port","value":"1"}`},
		{"POST", "/api/microbot/plugins/install", `{"name":"x/../y"}`},
		{"GET", "/api/microbot/config?group=fm-ironmanplanner&keys=url", ""},
	} {
		if status, _, _ = call(t, c.method, srv.URL+c.path, c.body); status != http.StatusBadRequest {
			t.Fatalf("%s %s = %d, want 400", c.method, c.path, status)
		}
	}
	if len(audit.entries) != before {
		t.Fatalf("refused requests were audited: %+v", audit.entries[before:])
	}
	status, body, _ = call(t, "POST", srv.URL+"/api/microbot/scripts/start", `{"className":"a.b.GotrPlugin"}`)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("start = %d %v", status, body)
	}
	if last := audit.last(); !last.OK || last.Action != agent.CmdScriptStart || string(last.Params) != `{"className":"a.b.GotrPlugin"}` {
		t.Fatalf("start audit = %+v", last)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/scripts/pause", ""); status != http.StatusOK {
		t.Fatalf("pause = %d", status)
	}

	// Game actions (docs/08): typed, validated before the agent, audited.
	status, body, _ = call(t, "POST", srv.URL+"/api/microbot/game/walk", `{"x":3222,"y":3218,"plane":0}`)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("walk = %d %v", status, body)
	}
	if last := audit.last(); !last.OK || last.Action != agent.CmdWalk || string(last.Params) != `{"x":3222,"y":3218,"plane":0}` {
		t.Fatalf("walk audit = %+v", last)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/game/walk", `{"x":-1,"y":3218}`); status != http.StatusBadRequest {
		t.Fatalf("bad walk = %d", status)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/game/keyboard", `{}`); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("unknown game action = %d", status)
	}
	// Logs, account and world (docs/08): world hop is audited; bad params are 400.
	if status, body, _ = call(t, "POST", srv.URL+"/api/microbot/world", `{"world":330}`); status != http.StatusOK {
		t.Fatalf("world hop = %d %v", status, body)
	}
	if last := audit.last(); last.Action != agent.CmdWorldHop || string(last.Params) != `{"world":330}` {
		t.Fatalf("world hop audit = %+v", last)
	}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/microbot/world", `{"world":42}`},
		{"POST", "/api/microbot/account", `{"index":0}`},
		{"GET", "/api/microbot/client/logs?lines=5000", ""},
		{"GET", "/api/microbot/config/schema?className=NotQualified", ""},
	} {
		if status, _, _ = call(t, c.method, srv.URL+c.path, c.body); status != http.StatusBadRequest {
			t.Fatalf("%s %s = %d, want 400", c.method, c.path, status)
		}
	}
	status, body, _ = call(t, "GET", srv.URL+"/api/microbot/game", "")
	if status != http.StatusOK || body["observe"] == nil {
		t.Fatalf("game view = %d %v", status, body)
	}

	// Live VNC: the browser's websocket reaches x11vnc through the agent, and
	// opening it is audited. Another site's origin is refused.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/microbot/vnc"
	if _, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"https://evil.example"}},
	}); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("vnc from another origin = %v", err)
	}
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("vnc dial: %v", err)
	}
	vnc := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	_ = vnc.SetDeadline(time.Now().Add(5 * time.Second))
	greet := make([]byte, 12)
	if _, err = io.ReadFull(vnc, greet); err != nil || string(greet) != "RFB 003.008\n" {
		t.Fatalf("vnc greeting = %q %v", greet, err)
	}
	_ = vnc.Close()
	if last := audit.last(); !last.OK || last.Action != agent.CmdVNCOpen {
		t.Fatalf("vnc audit = %+v", last)
	}
}

// fakeX11vnc greets like RFB on loopback.
func fakeX11vnc(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("RFB 003.008\n"))
			go func() { _, _ = io.Copy(io.Discard, c); _ = c.Close() }()
		}
	}()
	return ln.Addr().String()
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

// In dev mode the agent links only from loopback.
func TestMicrobotAgentDevLoopback(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	s := &Server{opts: Options{DevMode: true}}
	for remote, want := range map[string]int{"127.0.0.1:5000": http.StatusNoContent, "100.64.0.7:5000": http.StatusForbidden} {
		r := httptest.NewRequest("GET", "/api/microbot/agent", nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		s.agentAuth(ok)(w, r)
		if w.Code != want {
			t.Errorf("dev agent from %s = %d, want %d", remote, w.Code, want)
		}
	}
}

// In oidc mode the agent is a Zitadel machine user: it learns the scopes from
// /api/auth/client, gets a token by client_credentials, and the server takes
// it only with the project audience and the microbot:agent role.
func TestMicrobotAgentOIDC(t *testing.T) {
	const projectID = "proj-1"
	fake := authtest.New(t, testClientID)
	fake.AddMachine("agent-zaros", "s3cret", authtest.User{Subject: "machine-agent", Roles: []string{MicrobotAgentRole}, ProjectID: projectID})
	fake.AddMachine("not-agent", "s3cret", authtest.User{Subject: "machine-other", Roles: []string{"owner"}, ProjectID: projectID})
	provider, err := auth.NewProvider(context.Background(), auth.Config{
		Issuer: fake.Issuer(), ClientID: testClientID, RedirectURL: "http://planner/auth/callback",
		ProjectID: projectID, NativeClientID: testNativeClientID,
	})
	if err != nil {
		t.Fatal(err)
	}
	sealer, _ := auth.NewSealer("0123456789abcdef0123456789abcdef")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", Progress: &fakeProgress{}, Identity: newMemIdentity(), Logger: quiet,
		OIDC:     &OIDCOptions{Provider: provider, Policy: auth.Policy{RequiredRole: "owner"}, Sessions: NewSessionManager(memstore.New(), false), Flow: sealer},
		Microbot: &MicrobotOptions{Link: link, Audit: &memAudit{}},
	}))
	defer srv.Close()

	dial := func(header http.Header) int {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, resp, derr := websocket.Dial(ctx, agent.LinkURL(srv.URL), &websocket.DialOptions{HTTPHeader: header})
		if derr == nil {
			_ = conn.CloseNow()
			return http.StatusSwitchingProtocols
		}
		if resp == nil {
			t.Fatalf("dial: %v", derr)
		}
		return resp.StatusCode
	}
	if code := dial(nil); code != http.StatusUnauthorized {
		t.Fatalf("agent without a token = %d", code)
	}
	// A person's device-flow token has the project audience too, but not the role.
	userTok, _ := fake.AccessToken(authtest.User{Subject: "owner-1", Roles: []string{"owner", MicrobotRole}, ProjectID: projectID}, projectID, time.Hour)
	if code := dial(http.Header{"Authorization": {"Bearer " + userTok}}); code != http.StatusForbidden {
		t.Fatalf("agent with a person's token = %d", code)
	}
	// A token for the native client alone (no project audience) is not taken.
	nativeTok, _ := fake.AccessToken(authtest.User{Subject: "machine-agent", Roles: []string{MicrobotAgentRole}, ProjectID: projectID}, testNativeClientID, time.Hour)
	if code := dial(http.Header{"Authorization": {"Bearer " + nativeTok}}); code != http.StatusUnauthorized {
		t.Fatalf("agent token without the project audience = %d", code)
	}
	other := &client.AgentAuth{URL: srv.URL, ClientID: "not-agent", ClientSecret: "s3cret"}
	tok, err := other.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if code := dial(http.Header{"Authorization": {"Bearer " + tok}}); code != http.StatusForbidden {
		t.Fatalf("machine user without the agent role = %d", code)
	}
	if _, err = (&client.AgentAuth{URL: srv.URL}).Token(context.Background()); !errors.Is(err, client.ErrNoAgentCredentials) {
		t.Fatalf("oidc agent without credentials = %v", err)
	}

	// The real agent, end to end.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &agent.Agent{
		URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros"},
		Token:    (&client.AgentAuth{URL: srv.URL, ClientID: "agent-zaros", ClientSecret: "s3cret"}).Token,
		Executor: &agent.Executor{Game: stubGame{}, Home: t.TempDir(), ClientCmd: []string{"echo"}},
	}
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for !link.Info().Connected {
		if time.Now().After(deadline) {
			t.Fatal("machine-user agent never linked")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if info := link.Info(); info.Identity != "machine-agent" {
		t.Fatalf("agent identity = %+v", info)
	}
}
