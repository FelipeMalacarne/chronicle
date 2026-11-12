package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// hubServer is a server with a Link whose test auth reads the identity from
// the bearer: "subject" or "subject|bound account".
func hubServer(t *testing.T) (*Link, string) {
	t.Helper()
	link := &Link{Log: quiet}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		subject, bound, _ := strings.Cut(tok, "|")
		id := Identity{Subject: subject, BoundAccount: bound}
		if bound != "" {
			id.Pod = strings.TrimPrefix(subject, "pod:")
		}
		r = r.WithContext(WithIdentity(r.Context(), id))
		if strings.HasSuffix(r.URL.Path, "/vnc") {
			link.ServeVNC(w, r)
			return
		}
		link.Serve(w, r)
	}))
	t.Cleanup(srv.Close)
	return link, srv.URL
}

// runAgent links a real agent whose fake mb-client echoes marker.
func runAgent(t *testing.T, url, token string, hello Hello, marker, vncAddr string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := &Agent{
		URL: url, Token: StaticToken(token), Log: quiet, Hello: hello, VNCAddr: vncAddr,
		Executor: &Executor{Game: &fakeGame{}, Home: t.TempDir(), ClientCmd: []string{"echo", marker}},
	}
	go func() { _ = a.Run(ctx) }()
}

func waitAgents(t *testing.T, link *Link, n int) []Info {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := link.Agents(); len(got) == n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("agents = %+v, want %d", link.Agents(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// dialHello is a bare agent: the websocket and its hello, nothing else.
func dialHello(t *testing.T, url, token string, hello Hello) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, LinkURL(url), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + token}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	b, _ := json.Marshal(Message{Type: TypeHello, Hello: &hello})
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	return conn
}

func clientStart(t *testing.T, link *Link, account string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out ClientOutput
	err := link.Call(ctx, account, CmdClientStart, nil, &out)
	return out.Output, err
}

func TestHubRoutesByAccount(t *testing.T) {
	link, url := hubServer(t)
	var mu sync.Mutex
	var connected []Info
	link.OnConnect = func(i Info) { mu.Lock(); connected = append(connected, i); mu.Unlock() }
	runAgent(t, url, "machine-a", Hello{Host: "zaros", Account: "acc-a"}, "A", "")
	waitAgents(t, link, 1)
	// One agent: it serves the request without an account, as the web does today.
	if out, err := clientStart(t, link, ""); err != nil || !strings.HasPrefix(out, "A ") {
		t.Fatalf("only agent = %q %v", out, err)
	}
	runAgent(t, url, "pod:worker-acc-b|acc-b", Hello{Host: "worker-acc-b-1", Account: "acc-b", Kind: KindManaged, Worker: "worker-acc-b-1"}, "B", "")
	agents := waitAgents(t, link, 2)
	for account, want := range map[string]string{"acc-a": "A ", "acc-b": "B "} {
		if out, err := clientStart(t, link, account); err != nil || !strings.HasPrefix(out, want) {
			t.Fatalf("%s = %q %v", account, out, err)
		}
	}
	// Two agents and no account: ambiguous. An account nobody declares, with
	// no undeclared agent: none.
	if _, err := clientStart(t, link, ""); !errors.Is(err, ErrAmbiguousAgent) {
		t.Fatalf("no account with two agents = %v", err)
	}
	if _, err := clientStart(t, link, "acc-c"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("undeclared account = %v", err)
	}
	if _, ok := link.Agent(""); ok {
		t.Fatal("Agent(\"\") with two agents")
	}
	b, ok := link.Agent("acc-b")
	if !ok || b.Kind != KindManaged || b.Worker != "worker-acc-b-1" || b.Identity != "pod:worker-acc-b" || b.Account != "acc-b" {
		t.Fatalf("Agent(acc-b) = %+v %v", b, ok)
	}
	if a, _ := link.Agent("acc-a"); a.Kind != KindAttached || a.Identity != "machine-a" {
		t.Fatalf("Agent(acc-a) = %+v", a)
	}
	var out ClientOutput
	if err := link.CallAgent(context.Background(), b.ID, CmdClientStart, nil, &out); err != nil || !strings.HasPrefix(out.Output, "B ") {
		t.Fatalf("CallAgent = %+v %v", out, err)
	}
	if err := link.CallAgent(context.Background(), "nope", CmdStatus, nil, nil); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("CallAgent of no agent = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(connected)
		mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("OnConnect calls = %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	ids := map[string]bool{connected[0].ID: true, connected[1].ID: true}
	mu.Unlock()
	if !ids[agents[0].ID] || !ids[agents[1].ID] {
		t.Fatalf("OnConnect = %+v, agents %+v", connected, agents)
	}
}

func TestHubFallsBackToTheAgentWithoutAccount(t *testing.T) {
	link, url := hubServer(t)
	runAgent(t, url, "machine-a", Hello{Host: "zaros"}, "ZAROS", "")
	runAgent(t, url, "pod:w|acc-b", Hello{Host: "w", Account: "acc-b", Kind: KindManaged, Worker: "w"}, "B", "")
	waitAgents(t, link, 2)
	for account, want := range map[string]string{"acc-a": "ZAROS ", "acc-z": "ZAROS ", "acc-b": "B "} {
		if out, err := clientStart(t, link, account); err != nil || !strings.HasPrefix(out, want) {
			t.Fatalf("%s = %q %v", account, out, err)
		}
	}
	// Two agents without an account: neither is "the only one".
	runAgent(t, url, "machine-c", Hello{Host: "other"}, "C", "")
	waitAgents(t, link, 3)
	if _, err := clientStart(t, link, "acc-a"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("two undeclared agents = %v", err)
	}
	if out, err := clientStart(t, link, "acc-b"); err != nil || !strings.HasPrefix(out, "B ") {
		t.Fatalf("declared account still routes = %q %v", out, err)
	}
}

func TestHubReplacesOnlyTheSameAgent(t *testing.T) {
	link, url := hubServer(t)
	first := dialHello(t, url, "machine-a", Hello{Host: "zaros"})
	dialHello(t, url, "machine-a", Hello{Host: "laptop"})
	byHost := func(agents []Info) map[string]string {
		m := map[string]string{}
		for _, a := range agents {
			m[a.Host] = a.ID
		}
		return m
	}
	before := byHost(waitAgents(t, link, 2))
	dialHello(t, url, "machine-a", Hello{Host: "zaros"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		after := byHost(link.Agents())
		if len(link.Agents()) == 2 && after["zaros"] != before["zaros"] && after["laptop"] == before["laptop"] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("agents before %v, after %+v", before, link.Agents())
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := first.Read(ctx); websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("replaced connection = %v", err)
	}
}

func TestHubRefusesAPodOutsideItsAccount(t *testing.T) {
	link, url := hubServer(t)
	for _, hello := range []Hello{
		{Host: "w", Account: "acc-other", Kind: KindManaged, Worker: "w"},
		{Host: "w", Account: "acc-b", Kind: KindAttached, Worker: "w"},
		{Host: "w", Kind: KindManaged, Worker: "w"},
	} {
		conn := dialHello(t, url, "pod:w|acc-b", hello)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err := conn.Read(ctx)
		cancel()
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Fatalf("hello %+v = %v", hello, err)
		}
	}
	if n := len(link.Agents()); n != 0 {
		t.Fatalf("refused pods registered: %d", n)
	}
}

// greeter is an x11vnc stand-in that greets with its own name.
func greeter(t *testing.T, name string) string {
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
			_, _ = c.Write([]byte(name))
			go func() { _, _ = io.Copy(io.Discard, c); _ = c.Close() }()
		}
	}()
	return ln.Addr().String()
}

func TestHubVNCGoesToTheAccountsAgent(t *testing.T) {
	link, url := hubServer(t)
	runAgent(t, url, "machine-a", Hello{Host: "zaros", Account: "acc-a"}, "A", greeter(t, "AAAA"))
	runAgent(t, url, "pod:w|acc-b", Hello{Host: "w", Account: "acc-b", Kind: KindManaged, Worker: "w"}, "B", greeter(t, "BBBB"))
	waitAgents(t, link, 2)
	for account, want := range map[string]string{"acc-a": "AAAA", "acc-b": "BBBB"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		stream, err := link.OpenVNC(ctx, account)
		if err != nil {
			cancel()
			t.Fatalf("open %s: %v", account, err)
		}
		_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
		got := make([]byte, 4)
		_, err = io.ReadFull(stream, got)
		_ = stream.Close()
		cancel()
		if err != nil || string(got) != want {
			t.Fatalf("vnc of %s = %q %v", account, got, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := link.OpenVNC(ctx, ""); !errors.Is(err, ErrAmbiguousAgent) {
		t.Fatalf("vnc without an account = %v", err)
	}
}

// A stream for a session is taken only from the identity that was asked.
func TestHubVNCStreamFromAnotherIdentity(t *testing.T) {
	link, url := hubServer(t)
	link.vnc = map[string]vncWait{"s1": {stream: make(chan net.Conn, 1), subject: "machine-a"}}
	req, _ := http.NewRequest(http.MethodGet, url+"/api/microbot/agent/vnc?session=s1", nil)
	req.Header.Set("Authorization", "Bearer pod:w|acc-b")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stream from another identity = %d", resp.StatusCode)
	}
}
