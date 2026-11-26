package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Agents with slots: the server fills them with worker_bootstrap addressed to
// the agent itself, and the clients it holds are routed like any other.

// slotFarm builds the clients of an agent with slots: each its own fake
// mb-client and Agent Server, as podExecutor does for one.
type slotFarm struct {
	t     *testing.T
	mu    sync.Mutex
	slots map[string]int    // account → the slot NewSlot was last asked for
	dirs  map[string]string // account → its fake client's directory
	built int
	// startDelay is how long each fake mb-client start takes ("" for none);
	// startFail makes it fail after that.
	startDelay string
	startFail  bool
}

func (f *slotFarm) newSlot(slot int, account string) Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.built++
	dir := f.t.TempDir()
	f.slots[account], f.dirs[account] = slot, dir
	game := &podGame{fakeGame: &fakeGame{}, marker: filepath.Join(dir, "running")}
	script := fmt.Sprintf(`D=%q; case "$1" in
start) sleep %s; [ %t = true ] && exit 1; echo start >> "$D/starts"; touch "$D/running";;
stop) rm -f "$D/running"; echo stop >> "$D/stops";;
esac`, dir, cmp.Or(f.startDelay, "0"), f.startFail)
	return Client{Account: account, Executor: &Executor{
		Game: game, Home: filepath.Join(dir, "home"), ClientCmd: []string{"sh", "-c", script, "mb-client"},
		Worker: &WorkerConfig{Account: account, HWIDStoreDir: filepath.Join(dir, "secrets"), ReadyWait: 5 * time.Second},
	}}
}

func (f *slotFarm) slot(account string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.slots[account]
}

func (f *slotFarm) lines(account, file string) int {
	f.mu.Lock()
	dir := f.dirs[account]
	f.mu.Unlock()
	b, _ := os.ReadFile(filepath.Join(dir, file))
	return strings.Count(string(b), "\n")
}

func startSlotAgent(t *testing.T, url string, slots int) (*Agent, *slotFarm) {
	t.Helper()
	f := &slotFarm{t: t, slots: map[string]int{}, dirs: map[string]string{}}
	a := &Agent{Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached}, Slots: slots, NewSlot: f.newSlot}
	startAgent(t, a, url, "zaros")
	return a, f
}

func slotParams(account string) json.RawMessage {
	return bootstrapParams(func(p *BootstrapParams) { p.Account = account })
}

func bootstrapSlot(t *testing.T, link *Link, account string) (BootstrapResult, error) {
	t.Helper()
	pool, ok := PoolOf(link.Agents(), "zaros")
	if !ok {
		t.Fatalf("no pool in %+v", link.Agents())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var res BootstrapResult
	err := link.CallAgent(ctx, pool.ID, CmdWorkerBootstrap, json.RawMessage(slotParams(account)), &res)
	return res, err
}

func releaseSlot(t *testing.T, link *Link, account string) error {
	t.Helper()
	pool, ok := PoolOf(link.Agents(), "zaros")
	if !ok {
		t.Fatalf("no pool in %+v", link.Agents())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return link.CallAgent(ctx, pool.ID, CmdWorkerRelease, ReleaseParams{Account: account}, nil)
}

func poolInfo(t *testing.T, link *Link) Info {
	t.Helper()
	pool, ok := PoolOf(link.Agents(), "zaros")
	if !ok {
		t.Fatalf("no pool in %+v", link.Agents())
	}
	return pool
}

func TestSlotAgentLinksWithNoClients(t *testing.T) {
	link, url := hubServer(t)
	startSlotAgent(t, url, 3)
	agents := waitAgents(t, link, 1)
	if p := agents[0]; p.Slots != 3 || p.Used != 0 || p.Slot || p.Account != "" || p.Kind != KindAttached || !p.Supports(CmdWorkerRelease) {
		t.Fatalf("pool = %+v", p)
	}
	// The pool is never what a command routes to, with or without an account.
	for _, account := range []string{"", "acc_iron"} {
		if _, err := clientStart(t, link, account); !errors.Is(err, ErrNoAgent) {
			t.Fatalf("command to %q reached the pool: %v", account, err)
		}
	}
	if _, ok := link.Agent("acc_iron"); ok {
		t.Fatal("the pool serves an account")
	}
}

func TestSlotBootstrapAddsAClientTheServerRoutesTo(t *testing.T) {
	link, url := hubServer(t)
	var mu sync.Mutex
	var connected []Info
	var events []Info
	link.OnConnect = func(i Info) { mu.Lock(); connected = append(connected, i); mu.Unlock() }
	link.OnEvent = func(i Info, _ Event) { mu.Lock(); events = append(events, i); mu.Unlock() }
	a, farm := startSlotAgent(t, url, 2)
	waitAgents(t, link, 1)

	res, err := bootstrapSlot(t, link, "acc_iron")
	if err != nil || !res.Started {
		t.Fatalf("bootstrap = %+v %v", res, err)
	}
	agents := waitAgents(t, link, 2)
	pool, client := agents[0], agents[1]
	if pool.Slots != 2 || pool.Used != 1 || pool.Slot || client.Account != "acc_iron" || !client.Slot || client.Host != "zaros" {
		t.Fatalf("agents = %+v", agents)
	}
	if c, ok := SlotClientOf(agents, "zaros", "acc_iron"); !ok || c.ID != client.ID {
		t.Fatalf("SlotClientOf = %+v %v", c, ok)
	}
	// Commands for the account now reach its client, and only it.
	var st Status
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := link.Call(ctx, "acc_iron", CmdStatus, nil, &st); err != nil || !st.Running {
		t.Fatalf("status of the slot's client = %+v %v", st, err)
	}
	if _, err := clientStart(t, link, "acc_other"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("another account = %v", err)
	}
	if farm.slot("acc_iron") != 0 {
		t.Fatalf("slot = %d", farm.slot("acc_iron"))
	}
	// Its runs reach the server as the client's.
	a.out.put("acc_iron", Run{UID: "run-1"})
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(events) == 1 })
	mu.Lock()
	defer mu.Unlock()
	if events[0].Account != "acc_iron" || events[0].ID != client.ID || !events[0].Slot {
		t.Fatalf("event from %+v", events[0])
	}
	var accounts []string
	for _, i := range connected {
		accounts = append(accounts, fmt.Sprintf("%s/%d", i.Account, i.Slots))
	}
	if strings.Join(accounts, ",") != "/2,acc_iron/0" {
		t.Fatalf("OnConnect = %v", accounts)
	}
}

func TestSlotBootstrapOfTheSameAccountReusesItsSlot(t *testing.T) {
	link, url := hubServer(t)
	_, farm := startSlotAgent(t, url, 2)
	waitAgents(t, link, 1)
	for range 2 {
		if res, err := bootstrapSlot(t, link, "acc_iron"); err != nil || !res.Started {
			t.Fatalf("bootstrap = %+v %v", res, err)
		}
	}
	if farm.built != 1 || farm.lines("acc_iron", "starts") != 1 {
		t.Fatalf("clients built %d, starts %d", farm.built, farm.lines("acc_iron", "starts"))
	}
	if p := poolInfo(t, link); p.Used != 1 {
		t.Fatalf("pool = %+v", p)
	}
}

func TestSlotBootstrapWithNoFreeSlot(t *testing.T) {
	link, url := hubServer(t)
	_, farm := startSlotAgent(t, url, 1)
	waitAgents(t, link, 1)
	if _, err := bootstrapSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	var cerr *CommandError
	if _, err := bootstrapSlot(t, link, "acc_b"); !errors.As(err, &cerr) || !strings.Contains(cerr.Message, "no free slot (1 of 1 in use)") {
		t.Fatalf("second account = %v", err)
	}
	if farm.built != 1 {
		t.Fatalf("a client was built for the refused account: %d", farm.built)
	}
}

func TestSlotBootstrapSkipsABusyDisplay(t *testing.T) {
	link, url := hubServer(t)
	f := &slotFarm{t: t, slots: map[string]int{}, dirs: map[string]string{}}
	a := &Agent{
		Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached}, Slots: 2, NewSlot: f.newSlot,
		SlotBusy: func(slot int) bool { return slot == 0 },
	}
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 1)
	if _, err := bootstrapSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	if f.slot("acc_a") != 1 {
		t.Fatalf("slot = %d, want the free display's", f.slot("acc_a"))
	}
	var cerr *CommandError
	if _, err := bootstrapSlot(t, link, "acc_b"); !errors.As(err, &cerr) || !strings.Contains(cerr.Message, "(2 of 2 in use)") {
		t.Fatalf("second account = %v", err)
	}
}

// An agent restarted while a client kept running puts its account back in the
// slot of the display the client runs on, not in the lowest free one, whose
// display would then go to the next account twice.
func TestSlotBootstrapAdoptsTheClientsOwnSlot(t *testing.T) {
	link, url := hubServer(t)
	f := &slotFarm{t: t, slots: map[string]int{}, dirs: map[string]string{}}
	a := &Agent{
		Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached}, Slots: 2, NewSlot: f.newSlot,
		SlotBusy: func(slot int) bool { return slot == 1 },
		SlotOf:   func(account string) (int, bool) { return 1, account == "acc_a" },
	}
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 1)
	if _, err := bootstrapSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrapSlot(t, link, "acc_b"); err != nil {
		t.Fatal(err)
	}
	if f.slot("acc_a") != 1 || f.slot("acc_b") != 0 {
		t.Fatalf("slots a=%d b=%d, want 1 and 0", f.slot("acc_a"), f.slot("acc_b"))
	}
}

// An agent that starts while clients of its slots still run (it restarted,
// they did not) declares them in its first hello, in their own slots: the
// server must not bootstrap them again, which would mint a token that
// revokes the one their Chronicle Link runs with.
func TestSlotAgentAdoptsTheClientsStillRunning(t *testing.T) {
	link, url := hubServer(t)
	f := &slotFarm{t: t, slots: map[string]int{}, dirs: map[string]string{}}
	a := &Agent{
		Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached}, Slots: 2, NewSlot: f.newSlot,
		Live:   func() []string { return []string{"acc_a"} },
		SlotOf: func(account string) (int, bool) { return 1, account == "acc_a" },
	}
	startAgent(t, a, url, "zaros")
	agents := waitAgents(t, link, 2)
	if _, ok := SlotClientOf(agents, "zaros", "acc_a"); !ok {
		t.Fatalf("the running client was not declared: %+v", agents)
	}
	if p := poolInfo(t, link); p.Used != 1 {
		t.Fatalf("pool = %+v", p)
	}
	if f.slot("acc_a") != 1 || f.lines("acc_a", "starts") != 0 {
		t.Fatalf("slot %d, starts %d: want its own slot and no start", f.slot("acc_a"), f.lines("acc_a", "starts"))
	}
}

// A slot that changes after the link dropped and came back is told on the
// new link: told on the dead one, the server would keep a client that is gone.
func TestSlotChangeAfterAReconnectReachesTheNewLink(t *testing.T) {
	link, url := hubServer(t)
	f := &slotFarm{t: t, slots: map[string]int{}, dirs: map[string]string{}, startDelay: "3", startFail: true}
	a := &Agent{Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached}, Slots: 1, NewSlot: f.newSlot}
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 1)
	go func() { _, _ = bootstrapSlot(t, link, "acc_a") }()
	old := waitAgents(t, link, 2) // the slot is taken; the start runs
	link.mu.Lock()
	conn := link.agents[old[0].ID].ws.conn
	link.mu.Unlock()
	_ = conn.CloseNow()
	// The new hello still declares the slot; then the start fails and frees it.
	waitFor(t, func() bool {
		agents := link.Agents()
		return len(agents) == 1 && agents[0].ID != old[0].ID && agents[0].Used == 0
	})
	if _, err := link.Route("acc_a"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("route after the failed start = %v", err)
	}
}

func TestSlotBootstrapThatFailsGivesTheSlotBack(t *testing.T) {
	link, url := hubServer(t)
	_, farm := startSlotAgent(t, url, 1)
	waitAgents(t, link, 1)
	pool := poolInfo(t, link)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bad := bootstrapParams(func(p *BootstrapParams) { p.Account = "acc_a"; p.PluginToken = "nope" })
	if err := link.CallAgent(ctx, pool.ID, CmdWorkerBootstrap, json.RawMessage(bad), nil); err == nil {
		t.Fatal("a bad session bootstrapped")
	}
	waitFor(t, func() bool { return poolInfo(t, link).Used == 0 && len(link.Agents()) == 1 })
	if _, err := link.Route("acc_a"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("route after the failure = %v", err)
	}
	if _, err := bootstrapSlot(t, link, "acc_b"); err != nil {
		t.Fatalf("the slot was not free again: %v", err)
	}
	if farm.slot("acc_b") != 0 {
		t.Fatalf("slot = %d", farm.slot("acc_b"))
	}
}

func TestSlotReleaseStopsTheClientAndFreesTheSlot(t *testing.T) {
	link, url := hubServer(t)
	_, farm := startSlotAgent(t, url, 1)
	waitAgents(t, link, 1)
	if err := releaseSlot(t, link, "acc_ghost"); err != nil {
		t.Fatalf("release of an account in no slot = %v", err)
	}
	if _, err := bootstrapSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	waitAgents(t, link, 2)
	if err := releaseSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	waitAgents(t, link, 1)
	if _, err := link.Route("acc_a"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("route after the release = %v", err)
	}
	if p := poolInfo(t, link); p.Used != 0 || farm.lines("acc_a", "stops") != 1 {
		t.Fatalf("pool = %+v, stops = %d", p, farm.lines("acc_a", "stops"))
	}
	// The slot is free for another account, and again for the first.
	if _, err := bootstrapSlot(t, link, "acc_b"); err != nil {
		t.Fatal(err)
	}
	if err := releaseSlot(t, link, "acc_b"); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrapSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
}

// A release that comes while the account's client is still starting waits
// for the start: a stop that landed first would leave the client up with no
// slot.
func TestSlotReleaseDuringABootstrapWaitsForIt(t *testing.T) {
	link, url := hubServer(t)
	f := &slotFarm{t: t, slots: map[string]int{}, dirs: map[string]string{}, startDelay: "0.5"}
	a := &Agent{Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached}, Slots: 1, NewSlot: f.newSlot}
	startAgent(t, a, url, "zaros")
	waitAgents(t, link, 1)
	booted := make(chan error, 1)
	go func() {
		_, err := bootstrapSlot(t, link, "acc_a")
		booted <- err
	}()
	waitAgents(t, link, 2) // the slot is taken before the client starts
	if err := releaseSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	if err := <-booted; err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	dir := f.dirs["acc_a"]
	f.mu.Unlock()
	if _, err := os.Stat(filepath.Join(dir, "running")); err == nil {
		t.Fatal("the client kept running after its release")
	}
	if p := poolInfo(t, link); p.Used != 0 {
		t.Fatalf("pool = %+v", p)
	}
}

func TestSlotPoolTakesNoOtherCommand(t *testing.T) {
	link, url := hubServer(t)
	startSlotAgent(t, url, 1)
	waitAgents(t, link, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cerr *CommandError
	if err := link.CallAgent(ctx, poolInfo(t, link).ID, CmdClientStart, nil, nil); !errors.As(err, &cerr) || !strings.Contains(cerr.Message, "no account of its own") {
		t.Fatalf("client_start to the pool = %v", err)
	}
}

func TestSlotAgentDeclaresItsClientsOnReconnect(t *testing.T) {
	link, url := hubServer(t)
	_, farm := startSlotAgent(t, url, 2)
	waitAgents(t, link, 1)
	if _, err := bootstrapSlot(t, link, "acc_a"); err != nil {
		t.Fatal(err)
	}
	old := waitAgents(t, link, 2)
	// The link drops; the agent keeps its client and says so in the new hello.
	link.mu.Lock()
	conn := link.agents[old[0].ID].ws.conn
	link.mu.Unlock()
	_ = conn.CloseNow()
	waitFor(t, func() bool {
		agents := link.Agents()
		return len(agents) == 2 && agents[0].ID != old[0].ID && agents[0].Used == 1 && agents[1].Account == "acc_a"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var st Status
	if err := link.Call(ctx, "acc_a", CmdStatus, nil, &st); err != nil || !st.Running {
		t.Fatalf("status after the reconnect = %+v %v", st, err)
	}
	if farm.built != 1 {
		t.Fatalf("clients built = %d", farm.built)
	}
}

// A server without FeatureSlots would take the slots' hello as a plain agent:
// the agent links with none.
func TestSlotAgentWithAnOlderServerLinksWithNoSlots(t *testing.T) {
	hellos := make(chan Hello, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(FeaturesHeader, FeatureClients)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_, raw, err := conn.Read(r.Context())
		var m Message
		if err == nil && json.Unmarshal(raw, &m) == nil && m.Hello != nil {
			hellos <- *m.Hello
		}
		_, _, _ = conn.Read(r.Context())
	}))
	t.Cleanup(srv.Close)
	startSlotAgent(t, srv.URL, 2)
	select {
	case h := <-hellos:
		if h.Slots != 0 || len(h.Clients) != 0 || h.Account != "" {
			t.Fatalf("hello = %+v", h)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no hello")
	}
}

func sendFrame(t *testing.T, conn *websocket.Conn, m Message) {
	t.Helper()
	b, _ := json.Marshal(m)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func clientsFrame(accounts ...string) Message {
	m := Message{Type: TypeClients}
	for _, a := range accounts {
		m.Clients = append(m.Clients, ClientHello{Account: a})
	}
	return m
}

func TestLinkTakesClientsFramesOnlyFromAgentsWithSlots(t *testing.T) {
	link, url := hubServer(t)
	// A plain agent cannot add clients.
	plain := dialHello(t, url, "plain", Hello{Host: "other", Kind: KindAttached, Account: "acc_p", Commands: Commands})
	waitAgents(t, link, 1)
	sendFrame(t, plain, clientsFrame("acc_x"))
	time.Sleep(100 * time.Millisecond)
	if got := link.Agents(); len(got) != 1 || got[0].Account != "acc_p" {
		t.Fatalf("agents after a plain agent's clients frame = %+v", got)
	}
	// An agent with slots takes valid frames and ignores invalid ones.
	conn := dialHello(t, url, "slots", Hello{Host: "zaros", Kind: KindAttached, Slots: 2, Clients: []ClientHello{{Account: "acc_a"}}, Commands: Commands})
	waitAgents(t, link, 3)
	sendFrame(t, conn, clientsFrame("acc_a", "acc_b"))
	waitAgents(t, link, 4)
	for _, bad := range [][]string{{"acc_a", "acc_b", "acc_c"}, {"acc_a", "acc_a"}, {"main"}} {
		sendFrame(t, conn, clientsFrame(bad...))
	}
	sendFrame(t, conn, clientsFrame("acc_b"))
	waitAgents(t, link, 3)
	got := map[string]bool{}
	for _, i := range link.Agents() {
		got[i.Account] = true
	}
	if !got["acc_b"] || got["acc_a"] || !got["acc_p"] {
		t.Fatalf("agents = %+v", link.Agents())
	}
}

func TestLinkRefusesAMalformedSlotsHello(t *testing.T) {
	link, url := hubServer(t)
	for name, h := range map[string]Hello{
		"an account of its own": {Host: "zaros", Kind: KindAttached, Slots: 2, Account: "acc_a"},
		"managed":               {Host: "zaros", Kind: KindManaged, Slots: 2},
		"too many slots":        {Host: "zaros", Kind: KindAttached, Slots: MaxClients + 1},
		"more clients":          {Host: "zaros", Kind: KindAttached, Slots: 1, Clients: []ClientHello{{Account: "acc_a"}, {Account: "acc_b"}}},
	} {
		conn := dialHello(t, url, "zaros", h)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if _, _, err := conn.Read(ctx); err == nil {
			t.Errorf("%s: the link stayed open", name)
		}
		cancel()
	}
	if got := link.Agents(); len(got) != 0 {
		t.Fatalf("agents = %+v", got)
	}
}

// The agent with slots and the dev client's agent of one host, under one
// machine user, link side by side: neither replaces the other.
func TestSlotAgentAndDevAgentOfOneHostLinkSideBySide(t *testing.T) {
	link, url := hubServer(t)
	startSlotAgent(t, url, 1)
	waitAgents(t, link, 1)
	dev := &Agent{Log: quiet, Hello: Hello{Host: "zaros", Version: "test", Kind: KindAttached, Account: "acc_dev"}, Executor: &Executor{Game: &fakeGame{}}}
	startAgent(t, dev, url, "zaros")
	waitAgents(t, link, 2)
	time.Sleep(200 * time.Millisecond) // a replaced link would be gone by now
	agents := link.Agents()
	if _, ok := PoolOf(agents, "zaros"); !ok || len(agents) != 2 {
		t.Fatalf("agents = %+v", agents)
	}
	if info, err := link.Route("acc_dev"); err != nil || info.Slots != 0 || info.Slot {
		t.Fatalf("route to the dev client = %+v %v", info, err)
	}
}
