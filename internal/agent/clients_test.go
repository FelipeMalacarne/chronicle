package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Several clients per agent (docs/10 section 10): one agent, one executor
// and one lock per client, routed by account.

// twoClients is an agent of zaros that drives acc_a and acc_b, each with its
// own game, home and fake mb-client (echoing its marker).
func twoClients(t *testing.T, ga, gb Game) *Agent {
	t.Helper()
	return &Agent{
		Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached},
		Clients: []Client{
			{Account: "acc_a", Executor: &Executor{Game: ga, Home: t.TempDir(), ClientCmd: []string{"echo", "A"}, RunPoll: time.Millisecond}},
			{Account: "acc_b", Executor: &Executor{Game: gb, Home: t.TempDir(), ClientCmd: []string{"echo", "B"}, RunPoll: time.Millisecond}},
		},
	}
}

func startAgent(t *testing.T, a *Agent, url, token string) {
	t.Helper()
	a.URL, a.Token = url, StaticToken(token)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = a.Run(ctx) }()
}

func TestHubRoutesTheClientsOfOneAgent(t *testing.T) {
	link, url := hubServer(t)
	var mu sync.Mutex
	var connected []string
	link.OnConnect = func(i Info) { mu.Lock(); connected = append(connected, i.Account); mu.Unlock() }
	startAgent(t, twoClients(t, &fakeGame{}, &fakeGame{}), url, "zaros")
	agents := waitAgents(t, link, 2)
	if agents[0].Account != "acc_a" || agents[1].Account != "acc_b" || agents[0].ID == agents[1].ID ||
		agents[0].Host != "zaros" || agents[1].Kind != KindAttached || !agents[1].Supports(CmdTaskRun) {
		t.Fatalf("agents = %+v", agents)
	}
	for account, want := range map[string]string{"acc_a": "A ", "acc_b": "B "} {
		if out, err := clientStart(t, link, account); err != nil || !strings.HasPrefix(out, want) {
			t.Fatalf("%s = %q %v", account, out, err)
		}
	}
	var out ClientOutput
	if err := link.CallAgent(context.Background(), agents[1].ID, CmdClientStart, nil, &out); err != nil || !strings.HasPrefix(out.Output, "B ") {
		t.Fatalf("CallAgent(acc_b) = %+v %v", out, err)
	}
	// Two clients are two agents for the routing: none is "the only one".
	if _, err := clientStart(t, link, ""); !errors.Is(err, ErrAmbiguousAgent) {
		t.Fatalf("no account = %v", err)
	}
	if _, err := clientStart(t, link, "acc_c"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("an account no client declares = %v", err)
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(connected) == 2 })
	mu.Lock()
	slices.Sort(connected)
	got := strings.Join(connected, ",")
	mu.Unlock()
	if got != "acc_a,acc_b" {
		t.Fatalf("OnConnect = %s", got)
	}
}

// heldCmd is a fake mb-client that marks started, then blocks until release
// exists: a client_start that holds its client's lock until the test lets go.
func heldCmd(dir string) (cmd []string, started, release string) {
	release = filepath.Join(dir, "release")
	script := `touch "$0.started"; while [ ! -e "$0" ]; do sleep 0.02; done; echo held`
	return []string{"sh", "-c", script, release}, release + ".started", release
}

// A client's lock and run are its own: a lifecycle action holding acc_a's
// lock and a run driving acc_a's character leave acc_b free.
func TestClientsHaveTheirOwnLockAndRun(t *testing.T) {
	link, url := hubServer(t)
	ga, gb := newRunGame(""), newRunGame("done")
	a := twoClients(t, ga, gb)
	cmd, started, release := heldCmd(t.TempDir())
	a.Clients[0].Executor.ClientCmd = cmd
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	held := make(chan error, 1)
	go func() { held <- link.Call(ctx, "acc_a", CmdClientStart, nil, nil) }()
	waitFor(t, func() bool { _, err := os.Stat(started); return err == nil })
	// acc_a's lock is taken: acc_b's login goes through at once.
	bctx, bcancel := context.WithTimeout(ctx, 2*time.Second)
	if err := link.Call(bctx, "acc_b", CmdLogin, LoginParams{World: 303}, nil); err != nil {
		bcancel()
		t.Fatalf("acc_b's login while acc_a's lock is held = %v", err)
	}
	bcancel()
	select {
	case err := <-held:
		t.Fatalf("acc_a's start ended before the release: %v", err)
	default:
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}

	// A run on acc_a (its plugin never ends) refuses acc_a's actions only.
	if err := link.Call(ctx, "acc_a", CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`"}`), nil); err != nil {
		t.Fatal(err)
	}
	var cmdErr *CommandError
	if err := link.Call(ctx, "acc_a", CmdInventoryDrop, json.RawMessage(`{"name":"Logs"}`), nil); !errors.As(err, &cmdErr) || !strings.Contains(cmdErr.Message, ErrRunGoing.Error()) {
		t.Fatalf("acc_a's action during its run = %v", err)
	}
	if err := link.Call(ctx, "acc_b", CmdLogin, LoginParams{World: 303}, nil); err != nil {
		t.Fatalf("acc_b's action during acc_a's run = %v", err)
	}
	var st RunStatus
	if err := link.Call(ctx, "acc_b", CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`","stop":["`+fisher+`"]}`), &st); err != nil {
		t.Fatalf("acc_b's run during acc_a's = %v", err)
	}
	if r := waitRun(t, a.Clients[1].Executor); r.State != RunDone {
		t.Fatalf("acc_b's run = %+v", r)
	}
	if err := link.Call(ctx, "acc_a", CmdRunStatus, nil, &st); err != nil || st.Run == nil || st.Run.Final() || st.Run.ClassName != catSwap {
		t.Fatalf("acc_a's run after acc_b's = %+v %v", st.Run, err)
	}
	if err := link.Call(ctx, "acc_a", CmdRunCancel, nil, nil); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, a.Clients[0].Executor); r.State != RunCanceled {
		t.Fatalf("acc_a's run = %+v", r)
	}
}

// One client failing leaves the other working, and each run reaches the
// server as its own client's.
func TestOneClientFailingLeavesTheOther(t *testing.T) {
	link, url := hubServer(t)
	var (
		mu   sync.Mutex
		runs = map[string]map[string]Run{} // account → uid → run
	)
	link.OnEvent = func(info Info, ev Event) {
		mu.Lock()
		defer mu.Unlock()
		if ev.Run == nil {
			return
		}
		if runs[info.Account] == nil {
			runs[info.Account] = map[string]Run{}
		}
		if cur, ok := runs[info.Account][ev.Run.UID]; !ok || len(ev.Run.Events) >= len(cur.Events) {
			runs[info.Account][ev.Run.UID] = *ev.Run
		}
	}
	ga, gb := newRunGame("failed"), newRunGame("done")
	a := twoClients(t, ga, gb)
	a.Clients[0].Executor.ClientCmd = []string{"false"}
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := clientStart(t, link, "acc_a"); err == nil {
		t.Fatal("acc_a's broken mb-client started")
	}
	if out, err := clientStart(t, link, "acc_b"); err != nil || !strings.HasPrefix(out, "B ") {
		t.Fatalf("acc_b after acc_a failed = %q %v", out, err)
	}
	var sa, sb RunStatus
	params := json.RawMessage(`{"className":"` + catSwap + `","stop":["` + fisher + `"]}`)
	if err := link.Call(ctx, "acc_a", CmdRunStart, params, &sa); err != nil {
		t.Fatal(err)
	}
	if err := link.Call(ctx, "acc_b", CmdRunStart, params, &sb); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, a.Clients[0].Executor); r.State != RunFailed {
		t.Fatalf("acc_a's run = %+v", r)
	}
	if r := waitRun(t, a.Clients[1].Executor); r.State != RunDone {
		t.Fatalf("acc_b's run = %+v", r)
	}
	gb.mu.Lock()
	fisherBack := gb.active[fisher]
	gb.mu.Unlock()
	ga.mu.Lock()
	fisherA := ga.active[fisher]
	ga.mu.Unlock()
	if !fisherBack || fisherA {
		t.Fatalf("main activity: acc_b restarted %v, acc_a restarted %v", fisherBack, fisherA)
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runs["acc_a"][sa.Run.UID].State == RunFailed && runs["acc_b"][sb.Run.UID].State == RunDone
	})
	mu.Lock()
	defer mu.Unlock()
	if _, ok := runs["acc_b"][sa.Run.UID]; ok {
		t.Fatal("acc_a's run reached the server as acc_b's")
	}
	if _, ok := runs["acc_a"][sb.Run.UID]; ok {
		t.Fatal("acc_b's run reached the server as acc_a's")
	}
}

func TestHubVNCGoesToTheClientsVNC(t *testing.T) {
	link, url := hubServer(t)
	a := twoClients(t, &fakeGame{}, &fakeGame{})
	a.Clients[0].VNCAddr, a.Clients[1].VNCAddr = greeter(t, "AAAA"), greeter(t, "BBBB")
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 2)
	for account, want := range map[string]string{"acc_a": "AAAA", "acc_b": "BBBB"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		stream, err := link.OpenVNC(ctx, account)
		if err != nil {
			cancel()
			t.Fatalf("open %s: %v", account, err)
		}
		_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
		got := make([]byte, 4)
		_, err = io.ReadFull(stream, got)
		if !link.VNCOpen(account) {
			t.Errorf("VNCOpen(%s) while its stream flows", account)
		}
		_ = stream.Close()
		cancel()
		if err != nil || string(got) != want {
			t.Fatalf("vnc of %s = %q %v", account, got, err)
		}
	}
}

// A running x11vnc's port in the dev home wins over the configured one: a
// client started before the agent's configuration changed keeps its port.
func TestClientVNCAddrFromTheDevHome(t *testing.T) {
	home := t.TempDir()
	c := Client{Executor: &Executor{Home: home}, VNCAddr: "127.0.0.1:6001"}
	if got := c.vncAddr(); got != "127.0.0.1:6001" {
		t.Fatalf("no x11vnc running = %s", got)
	}
	run := filepath.Join(home, "run")
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "vnc.port"), []byte("6007\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := c.vncAddr(); got != "127.0.0.1:6001" {
		t.Fatalf("port file without a live x11vnc = %s", got)
	}
	if err := os.WriteFile(filepath.Join(run, "vnc.pid"), fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := c.vncAddr(); got != "127.0.0.1:6007" {
		t.Fatalf("live x11vnc = %s", got)
	}
	if got := (Client{}).vncAddr(); got != "127.0.0.1:5999" {
		t.Fatalf("default = %s", got)
	}
}

// Each client's harness tools get its own dev home, display and VNC port.
func TestExecutorEnvReachesTheHarness(t *testing.T) {
	e := &Executor{
		Game: &fakeGame{}, Env: []string{"MB_DEV_HOME=/homes/b", "MB_VNC_PORT=6000"},
		ClientCmd:  []string{"sh", "-c", `echo "$MB_DEV_HOME $MB_VNC_PORT $1"`, "mb-client"},
		AccountCmd: []string{"sh", "-c", `echo "$MB_DEV_HOME $1"`, "mb-account"},
	}
	out, err := e.Execute(context.Background(), CmdClientStart, nil)
	if err != nil || out.(ClientOutput).Output != "/homes/b 6000 start" {
		t.Fatalf("mb-client = %+v %v", out, err)
	}
	if got, err := e.runAccount(context.Background(), "show"); err != nil || strings.TrimSpace(got) != "/homes/b show" {
		t.Fatalf("mb-account = %q %v", got, err)
	}
}

func TestHubRefusesBadClients(t *testing.T) {
	link, url := hubServer(t)
	clients := func(accounts ...string) []ClientHello {
		var cs []ClientHello
		for _, a := range accounts {
			cs = append(cs, ClientHello{Account: a})
		}
		return cs
	}
	many := make([]string, MaxClients+1)
	for i := range many {
		many[i] = fmt.Sprintf("acc_%d", i)
	}
	for name, c := range map[string]struct {
		token string
		hello Hello
	}{
		"managed":         {"zaros", Hello{Host: "zaros", Kind: KindManaged, Clients: clients("acc_a", "acc_b")}},
		"pod":             {"pod:w|acc_a", Hello{Host: "w", Account: "acc_a", Kind: KindManaged, Worker: "w", Clients: clients("acc_a")}},
		"twice":           {"zaros", Hello{Host: "zaros", Clients: clients("acc_a", "acc_a")}},
		"no account":      {"zaros", Hello{Host: "zaros", Clients: clients("acc_a", "")}},
		"bad account":     {"zaros", Hello{Host: "zaros", Clients: clients("acc_a", "../etc")}},
		"too many":        {"zaros", Hello{Host: "zaros", Clients: clients(many...)}},
		"another account": {"zaros", Hello{Host: "zaros", Account: "acc_b", Clients: clients("acc_a", "acc_b")}},
	} {
		conn := dialHello(t, url, c.token, c.hello)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err := conn.Read(ctx)
		cancel()
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Errorf("%s: %v", name, err)
		}
	}
	if n := len(link.Agents()); n != 0 {
		t.Fatalf("refused hellos registered: %d", n)
	}
}

// A new connection of the agent replaces all its clients, and only them.
func TestHubReplacesEveryClientOfTheAgent(t *testing.T) {
	link, url := hubServer(t)
	hello := Hello{Host: "zaros", Account: "acc_a", Clients: []ClientHello{{Account: "acc_a"}, {Account: "acc_b"}}}
	dialHello(t, url, "zaros", hello)
	dialHello(t, url, "zaros", Hello{Host: "laptop", Account: "acc_c"})
	before := waitAgents(t, link, 3)
	dialHello(t, url, "zaros", hello)
	waitFor(t, func() bool {
		after := link.Agents()
		if len(after) != 3 {
			return false
		}
		ids := map[string]string{}
		for _, a := range after {
			ids[a.Account] = a.ID
		}
		old := map[string]string{}
		for _, a := range before {
			old[a.Account] = a.ID
		}
		return ids["acc_a"] != old["acc_a"] && ids["acc_b"] != old["acc_b"] && ids["acc_c"] == old["acc_c"]
	})
}

// Events of a tagged link go to the client they name; one naming no client
// of the agent is dropped.
func TestHubEventsOfATaggedLink(t *testing.T) {
	link, url := hubServer(t)
	var (
		mu  sync.Mutex
		got []string
	)
	link.OnEvent = func(info Info, ev Event) { mu.Lock(); got = append(got, info.Account+":"+ev.Run.UID); mu.Unlock() }
	conn := dialHello(t, url, "zaros", Hello{Host: "zaros", Clients: []ClientHello{{Account: "acc_a"}, {Account: "acc_b"}}})
	waitAgents(t, link, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, m := range []Message{
		{Type: TypeEvent, Account: "acc_b", Event: &Event{Run: &Run{UID: "0000000000000001"}}},
		{Type: TypeEvent, Account: "acc_x", Event: &Event{Run: &Run{UID: "0000000000000002"}}},
		{Type: TypeEvent, Event: &Event{Run: &Run{UID: "0000000000000003"}}},
		{Type: TypeEvent, Account: "acc_a", Event: &Event{Run: &Run{UID: "0000000000000004"}}},
	} {
		b, _ := json.Marshal(m)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 2 })
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "acc_b:0000000000000001,acc_a:0000000000000004" {
		t.Fatalf("events = %v", got)
	}
}

// An older server announces nothing and would take every client's commands
// and events as the hello's account: it gets the first client alone, untagged,
// and the other clients' runs never reach it.
func TestAgentWithClientsOnAnOlderServer(t *testing.T) {
	type frame struct {
		m   Message
		err error
	}
	hellos := make(chan Hello, 1)
	frames := make(chan Message, 64)
	commands := make(chan Message, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil) // no FeaturesHeader
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		read := func() frame {
			_, raw, err := conn.Read(ctx)
			var m Message
			if err == nil {
				err = json.Unmarshal(raw, &m)
			}
			return frame{m, err}
		}
		f := read()
		if f.err != nil || f.m.Hello == nil {
			return
		}
		hellos <- *f.m.Hello
		go func() {
			for c := range commands {
				b, _ := json.Marshal(c)
				_ = conn.Write(ctx, websocket.MessageText, b)
			}
		}()
		for {
			f := read()
			if f.err != nil {
				return
			}
			frames <- f.m
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(commands) })
	ga, gb := newRunGame("done"), newRunGame("done")
	a := twoClients(t, ga, gb)
	startAgent(t, a, srv.URL, "zaros")
	var hello Hello
	select {
	case hello = <-hellos:
	case <-time.After(5 * time.Second):
		t.Fatal("no hello")
	}
	if hello.Account != "acc_a" || len(hello.Clients) != 0 || hello.Host != "zaros" {
		t.Fatalf("hello to an older server = %+v", hello)
	}
	// acc_b runs by itself (a trigger, a recurring routine): its run must not
	// reach a server that would store it as acc_a's.
	if _, err := a.Clients[1].Executor.Execute(context.Background(), CmdRunStart, json.RawMessage(`{"className":"`+catSwap+`"}`)); err != nil {
		t.Fatal(err)
	}
	b := waitRun(t, a.Clients[1].Executor)
	commands <- Message{Type: TypeCommand, ID: "c1", Command: CmdClientStart}
	commands <- Message{Type: TypeCommand, ID: "c2", Command: CmdClientStart, Account: "acc_b"}
	results := map[string]Message{}
	deadline := time.After(5 * time.Second)
	for len(results) < 2 {
		select {
		case m := <-frames:
			switch m.Type {
			case TypeResult:
				results[m.ID] = m
			case TypeEvent:
				if m.Account != "" || m.Event.Run.UID == b.UID {
					t.Fatalf("event to an older server = %+v", m)
				}
			}
		case <-deadline:
			t.Fatalf("results = %+v", results)
		}
	}
	var out ClientOutput
	if r := results["c1"]; !r.OK || json.Unmarshal(r.Data, &out) != nil || !strings.HasPrefix(out.Output, "A ") {
		t.Fatalf("untagged command = %+v", r)
	}
	if r := results["c2"]; r.OK || !strings.Contains(r.Error, "acc_b") {
		t.Fatalf("command for the client left out = %+v", r)
	}
	// Nothing of acc_b arrives later either.
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case m := <-frames:
			if m.Type == TypeEvent && (m.Account != "" || m.Event.Run.UID == b.UID) {
				t.Fatalf("event to an older server = %+v", m)
			}
		case <-timeout:
			return
		}
	}
}

func TestClientFor(t *testing.T) {
	a, b := Client{Account: "acc_a"}, Client{Account: "acc_b"}
	for _, c := range []struct {
		linked  []Client
		tagged  bool
		account string
		want    string // the account picked, or "!" for an error
	}{
		{[]Client{a}, false, "", "acc_a"},
		{[]Client{a}, false, "acc_a", "acc_a"},
		{[]Client{a}, false, "acc_b", "!"},
		{[]Client{{}}, false, "", ""},
		{[]Client{a, b}, true, "acc_b", "acc_b"},
		{[]Client{a, b}, true, "acc_a", "acc_a"},
		{[]Client{a, b}, true, "", "!"},
		{[]Client{a, b}, true, "acc_c", "!"},
	} {
		got, err := clientFor(c.linked, c.tagged, c.account)
		if (err != nil) != (c.want == "!") || (err == nil && got.Account != c.want) {
			t.Errorf("clientFor(%v, %v, %q) = %+v %v, want %s", c.linked, c.tagged, c.account, got, err, c.want)
		}
	}
}
