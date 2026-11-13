package agent

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Link errors.
var (
	// ErrNoAgent: no connected agent serves the account.
	ErrNoAgent = errors.New("agent: no agent connected")
	// ErrAmbiguousAgent: more than one agent could serve the request, and
	// the request did not say which account (or two agents declare it).
	ErrAmbiguousAgent = errors.New("agent: more than one agent connected; say which account")
	// ErrUnknownCommand: the command is not in the closed list.
	ErrUnknownCommand = errors.New("agent: unknown command")
)

// CommandError is a command the agent ran and that failed.
type CommandError struct {
	Command string
	Message string
}

func (e *CommandError) Error() string { return e.Command + ": " + e.Message }

// Link is the server end of the agents' links: the Hub of docs/11. Many
// agents connect at once, each serving one game client or, an attached agent,
// several (Hello.Clients, docs/10 section 10); requests are addressed by
// account (docs/11 section 13.1) and the Hub picks the client. A new
// connection of the same agent (same identity and same worker, or host)
// replaces only that agent's previous one, with all its clients.
type Link struct {
	Log *slog.Logger
	// OnConnect, when set, is called in its own goroutine after an agent's
	// connection is registered, once per client it serves (the workers'
	// reconciler sends the bootstrap to managed agents from it).
	OnConnect func(Info)
	// OnEvent, when set, gets the events an agent sends by itself
	// (TypeEvent) with the Info of the client each comes from, in the order
	// the agent sent them, from one goroutine per connection; a slow
	// OnEvent never holds up the command results.
	OnEvent func(Info, Event)

	mu     sync.Mutex
	agents map[string]*agentConn // the clients, by their Info.ID
	vnc    map[string]vncWait    // browser sessions waiting for a stream, by session id
	// vncLive counts the VNC sessions whose stream is flowing, by the
	// account the client declares (VNCOpen).
	vncLive map[string]int
}

// wsConn is one agent's websocket, shared by the clients it serves.
type wsConn struct {
	id    string
	key   string
	conn  *websocket.Conn
	ident Identity
	since time.Time
	// tagged: the agent drives several clients (Hello.Clients), so every
	// command names its client's account and every event comes with one.
	tagged  bool
	clients []*agentConn // in the hello's order
	// pending are the commands waiting for their result, under Link.mu;
	// nil once the connection ended.
	pending map[string]chan Message
	wmu     sync.Mutex
}

// agentConn is one game client an agent serves: what the Hub routes to.
type agentConn struct {
	id    string
	ws    *wsConn
	hello Hello // the agent's hello, with this client's account
	idx   int   // its place in the hello's clients
}

func (c *agentConn) info() Info {
	kind := c.hello.Kind
	if kind == "" {
		kind = KindAttached
	}
	return Info{
		Connected: true, ID: c.id, Since: c.ws.since, Host: c.hello.Host, Version: c.hello.Version,
		Identity: c.ws.ident.Subject, Account: c.hello.Account, Kind: kind, Worker: c.hello.Worker, Bridge: c.hello.Bridge,
		Commands: slices.Clone(c.hello.Commands),
	}
}

// Info is what the server knows about one connected agent: about one of its
// clients, for an agent that drives several (each has its own ID and account).
type Info struct {
	Connected bool `json:"connected"`
	// ID is this connection's id (a reconnect gets a new one).
	ID      string    `json:"id,omitempty"`
	Since   time.Time `json:"since,omitzero"`
	Host    string    `json:"host,omitempty"`
	Version string    `json:"version,omitempty"`
	// Identity is who the server authenticated: "dev" (loopback), the
	// machine user's subject or "pod:<name>" for a managed worker.
	Identity string `json:"identity,omitempty"`
	// Account is the account the agent declared in its hello ("" for none).
	Account string `json:"account,omitempty"`
	// Kind is KindAttached or KindManaged.
	Kind string `json:"kind,omitempty"`
	// Worker is a managed agent's pod name.
	Worker string `json:"worker,omitempty"`
	// Bridge is the Chronicle Link bridge version the agent reported.
	Bridge string `json:"bridge,omitempty"`
	// Commands are the commands the agent declared in its hello: what it
	// can do, so the server leaves a newer protocol out for an older agent.
	Commands []string `json:"-"`
}

// Supports reports whether the agent declared command in its hello.
func (i Info) Supports(command string) bool { return slices.Contains(i.Commands, command) }

// Identity is who agentAuth authenticated on an agent's request.
type Identity struct {
	// Subject is "dev", a machine user's subject or "pod:<name>".
	Subject string
	// BoundAccount, when set, is the only account this identity may serve:
	// its hello must declare it, with kind managed (a worker's pod).
	BoundAccount string
	// Pod is the worker pod's name, for a pod identity.
	Pod string
}

type identityKey struct{}

// WithIdentity tells Serve (and ServeVNC) who the server authenticated on
// this request.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityOf is the Identity WithIdentity put in ctx (zero when none).
func IdentityOf(ctx context.Context) Identity {
	id, _ := ctx.Value(identityKey{}).(Identity)
	return id
}

// Agents lists the connected agents, oldest first, one Info per client: the
// clients of one agent together, in the order its hello declared them.
func (l *Link) Agents() []Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	cs := slices.Collect(maps.Values(l.agents))
	slices.SortFunc(cs, func(a, b *agentConn) int {
		return cmp.Or(a.ws.since.Compare(b.ws.since), cmp.Compare(a.ws.id, b.ws.id), cmp.Compare(a.idx, b.idx))
	})
	out := make([]Info, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.info())
	}
	return out
}

// Agent is the agent that serves account ("" for the only agent).
func (l *Link) Agent(account string) (Info, bool) {
	info, err := l.Route(account)
	return info, err == nil
}

// Route is the agent that serves account, by the rules of docs/11 section
// 13.1: with no account, the only agent connected (ErrAmbiguousAgent when
// there are more); with an account, the agent that declares it, else the
// only agent that declares none, else ErrNoAgent. Each client of an agent
// that drives several counts as an agent of its own.
func (l *Link) Route(account string) (Info, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, err := l.route(account)
	if err != nil {
		return Info{}, err
	}
	return c.info(), nil
}

// route runs under l.mu.
func (l *Link) route(account string) (*agentConn, error) {
	if account == "" {
		switch len(l.agents) {
		case 0:
			return nil, ErrNoAgent
		case 1:
			for _, c := range l.agents {
				return c, nil
			}
		}
		return nil, ErrAmbiguousAgent
	}
	var declared, undeclared []*agentConn
	for _, c := range l.agents {
		switch c.hello.Account {
		case account:
			declared = append(declared, c)
		case "":
			undeclared = append(undeclared, c)
		}
	}
	switch {
	case len(declared) == 1:
		return declared[0], nil
	case len(declared) > 1:
		return nil, fmt.Errorf("%w (%d agents declare %s)", ErrAmbiguousAgent, len(declared), account)
	case len(undeclared) == 1:
		return undeclared[0], nil
	}
	return nil, ErrNoAgent
}

// agentKey is what makes two connections the same agent: who authenticated
// it and which worker (or host) it runs on. A pod bound to an account is one
// worker per account (a Deployment of one replica, Recreate), so its key is
// the account: a new pod replaces the old pod's lingering link.
func agentKey(id Identity, h Hello) string {
	if id.BoundAccount != "" {
		return "bound\x00" + id.BoundAccount
	}
	return id.Subject + "\x00" + cmp.Or(h.Worker, h.Host)
}

// Serve upgrades an authenticated request to the agent's websocket and holds
// it until it closes. The caller has authenticated the agent (httpapi's
// agentAuth: loopback in dev, a machine user's token in oidc, a pod's token)
// and put its Identity in the request's context.
func (l *Link) Serve(w http.ResponseWriter, r *http.Request) {
	ident := IdentityOf(r.Context())
	// What this link understands, before the hello: an agent with several
	// clients declares them only to a server that routes them.
	w.Header().Set(FeaturesHeader, FeatureClients)
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		l.Log.Warn("agent link: accept", "err", err)
		return
	}
	conn.SetReadLimit(32 << 20)
	ctx := r.Context()
	_, raw, err := conn.Read(ctx)
	var hello Message
	if err == nil {
		err = json.Unmarshal(raw, &hello)
	}
	if err != nil || hello.Type != TypeHello || hello.Hello == nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}
	h := *hello.Hello
	if h.Kind != "" && h.Kind != KindAttached && h.Kind != KindManaged {
		_ = conn.Close(websocket.StatusPolicyViolation, "unknown agent kind")
		return
	}
	if ident.BoundAccount != "" && (h.Account != ident.BoundAccount || h.Kind != KindManaged || len(h.Clients) > 0) {
		l.Log.Warn("agent link: refused a hello outside the identity's account", "identity", ident.Subject, "declared", h.Account, "kind", h.Kind)
		_ = conn.Close(websocket.StatusPolicyViolation, "this identity serves only its own account, as a managed agent")
		return
	}
	if err := checkClients(h); err != nil {
		l.Log.Warn("agent link: refused a hello", "identity", ident.Subject, "host", h.Host, "err", err)
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	ws := newWSConn(conn, ident, h)
	var replaced []*wsConn
	l.mu.Lock()
	if l.agents == nil {
		l.agents = map[string]*agentConn{}
	}
	for id, old := range l.agents {
		if old.ws.key == ws.key {
			delete(l.agents, id)
			if !slices.Contains(replaced, old.ws) {
				replaced = append(replaced, old.ws)
			}
		}
	}
	for _, c := range ws.clients {
		l.agents[c.id] = c
	}
	l.mu.Unlock()
	for _, old := range replaced {
		go func() { _ = old.conn.Close(websocket.StatusNormalClosure, "replaced by a new agent connection") }()
	}
	infos := make([]Info, len(ws.clients))
	accounts := make([]string, len(ws.clients))
	for i, c := range ws.clients {
		infos[i], accounts[i] = c.info(), c.hello.Account
	}
	l.Log.Info("agent link: connected", "id", ws.id, "host", h.Host, "version", h.Version, "identity", ident.Subject,
		"accounts", accounts, "kind", infos[0].Kind, "worker", h.Worker, "replaced", len(replaced))
	if l.OnConnect != nil {
		for _, info := range infos {
			go l.OnConnect(info)
		}
	}
	defer func() {
		l.mu.Lock()
		for _, c := range ws.clients {
			if l.agents[c.id] == c {
				delete(l.agents, c.id)
			}
		}
		for id, ch := range ws.pending {
			close(ch)
			delete(ws.pending, id)
		}
		ws.pending = nil
		l.mu.Unlock()
		_ = conn.CloseNow()
		l.Log.Info("agent link: disconnected", "id", ws.id, "host", h.Host, "accounts", accounts)
	}()
	events := l.eventQueue()
	defer close(events)
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m Message
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.Type == TypeEvent && m.Event != nil {
			i := ws.eventClient(m.Account)
			if i < 0 {
				l.Log.Warn("agent link: event of a client the agent did not declare, dropped", "id", ws.id, "account", m.Account)
				continue
			}
			select {
			case events <- linkEvent{info: infos[i], ev: *m.Event}:
			default:
				// Full: dropped; the agent sends its history again on reconnecting.
				l.Log.Warn("agent link: event queue full, dropped one", "id", ws.id)
			}
			continue
		}
		if m.Type != TypeResult {
			continue
		}
		l.mu.Lock()
		ch, ok := ws.pending[m.ID]
		delete(ws.pending, m.ID)
		l.mu.Unlock()
		if ok {
			ch <- m
		}
	}
}

// checkClients checks the clients a hello declares: only an attached agent
// drives several, each on an account of its own, the first one's in Account.
func checkClients(h Hello) error {
	if len(h.Clients) == 0 {
		return nil
	}
	if h.Kind == KindManaged {
		return errors.New("a managed agent drives one client")
	}
	if len(h.Clients) > MaxClients {
		return fmt.Errorf("at most %d clients per agent", MaxClients)
	}
	seen := map[string]bool{}
	for _, c := range h.Clients {
		if !ValidAccount(c.Account) {
			return errors.New("a client without a valid account")
		}
		if seen[c.Account] {
			return errors.New("two clients on one account")
		}
		seen[c.Account] = true
	}
	if h.Account != "" && h.Account != h.Clients[0].Account {
		return errors.New("the hello's account is not its first client's")
	}
	return nil
}

// newWSConn is the connection of an agent that sent hello h, with one
// agentConn per client: the hello's own for an agent with one client (its
// Info.ID is the connection's id), else one per Hello.Clients.
func newWSConn(conn *websocket.Conn, ident Identity, h Hello) *wsConn {
	ws := &wsConn{
		id: newID(), key: agentKey(ident, h), conn: conn, ident: ident, since: time.Now(),
		tagged: len(h.Clients) > 0, pending: map[string]chan Message{},
	}
	if !ws.tagged {
		ws.clients = []*agentConn{{id: ws.id, ws: ws, hello: h}}
		return ws
	}
	for i, cl := range h.Clients {
		ch := h
		ch.Account, ch.Clients = cl.Account, nil
		ws.clients = append(ws.clients, &agentConn{id: newID(), ws: ws, hello: ch, idx: i})
	}
	return ws
}

// eventClient is the index of the client an event comes from: on a tagged
// connection the one its account names (-1 for none), else the only one.
func (ws *wsConn) eventClient(account string) int {
	if !ws.tagged {
		return 0
	}
	for i, c := range ws.clients {
		if c.hello.Account == account {
			return i
		}
	}
	return -1
}

// linkEvent is an event and the client it comes from.
type linkEvent struct {
	info Info
	ev   Event
}

// eventQueue starts the goroutine that hands one connection's events to
// OnEvent in order; closing the channel ends it.
func (l *Link) eventQueue() chan linkEvent {
	ch := make(chan linkEvent, 256)
	go func() {
		for e := range ch {
			if l.OnEvent != nil {
				l.OnEvent(e.info, e.ev)
			}
		}
	}()
	return ch
}

// Call sends one command to the agent that serves account (Route) and waits
// for its result; into, when not nil, receives the result's data.
func (l *Link) Call(ctx context.Context, account, command string, params, into any) error {
	l.mu.Lock()
	c, err := l.route(account)
	l.mu.Unlock()
	if err != nil {
		return err
	}
	return l.call(ctx, c, command, params, into)
}

// CallAgent is Call to one client, by its Info.ID.
func (l *Link) CallAgent(ctx context.Context, agentID, command string, params, into any) error {
	l.mu.Lock()
	c := l.agents[agentID]
	l.mu.Unlock()
	if c == nil {
		return ErrNoAgent
	}
	return l.call(ctx, c, command, params, into)
}

func (l *Link) call(ctx context.Context, c *agentConn, command string, params, into any) error {
	if !Known(command) {
		return fmt.Errorf("%w: %q", ErrUnknownCommand, command)
	}
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	id := newID()
	ch := make(chan Message, 1)
	ws := c.ws
	l.mu.Lock()
	if ws.pending == nil { // the connection ended meanwhile
		l.mu.Unlock()
		return ErrNoAgent
	}
	ws.pending[id] = ch
	l.mu.Unlock()
	drop := func() {
		l.mu.Lock()
		delete(ws.pending, id)
		l.mu.Unlock()
	}
	m := Message{Type: TypeCommand, ID: id, Command: command, Params: raw}
	if ws.tagged {
		m.Account = c.hello.Account
	}
	b, err := json.Marshal(m)
	if err != nil {
		drop()
		return err
	}
	ws.wmu.Lock()
	err = ws.conn.Write(ctx, websocket.MessageText, b)
	ws.wmu.Unlock()
	if err != nil {
		drop()
		return fmt.Errorf("agent: send %s: %w", command, err)
	}
	select {
	case <-ctx.Done():
		drop()
		return ctx.Err()
	case m, ok := <-ch:
		if !ok {
			return ErrNoAgent
		}
		if !m.OK {
			return &CommandError{Command: command, Message: m.Error}
		}
		if into != nil && len(m.Data) > 0 {
			return json.Unmarshal(m.Data, into)
		}
		return nil
	}
}

// vncWait is a browser session waiting for its agent's stream: only the
// identity of the agent that was asked may bring it.
type vncWait struct {
	stream  chan net.Conn
	subject string
	// account is the account the asked client declares (VNCOpen).
	account string
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
