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
// agents connect at once, each serving one game client; requests are
// addressed by account (docs/11 section 13.1) and the Hub picks the
// connection. A new connection of the same agent (same identity and same
// worker, or host) replaces only that agent's previous one.
type Link struct {
	Log *slog.Logger
	// OnConnect, when set, is called in its own goroutine after an agent's
	// connection is registered (the workers' reconciler sends the bootstrap
	// to managed agents from it).
	OnConnect func(Info)
	// OnEvent, when set, gets the events an agent sends by itself
	// (TypeEvent), in the order it sent them, from one goroutine per
	// connection; a slow OnEvent never holds up the command results.
	OnEvent func(Info, Event)

	mu     sync.Mutex
	agents map[string]*agentConn // by connection id
	vnc    map[string]vncWait    // browser sessions waiting for a stream, by session id
	// vncLive counts the VNC sessions whose stream is flowing, by the
	// account the agent declares (VNCOpen).
	vncLive map[string]int
}

// agentConn is one agent's websocket.
type agentConn struct {
	id      string
	key     string
	conn    *websocket.Conn
	hello   Hello
	ident   Identity
	since   time.Time
	pending map[string]chan Message
	wmu     sync.Mutex
}

func (c *agentConn) info() Info {
	kind := c.hello.Kind
	if kind == "" {
		kind = KindAttached
	}
	return Info{
		Connected: true, ID: c.id, Since: c.since, Host: c.hello.Host, Version: c.hello.Version,
		Identity: c.ident.Subject, Account: c.hello.Account, Kind: kind, Worker: c.hello.Worker, Bridge: c.hello.Bridge,
		Commands: slices.Clone(c.hello.Commands),
	}
}

// Info is what the server knows about one connected agent.
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

// Agents lists the connected agents, oldest first.
func (l *Link) Agents() []Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Info, 0, len(l.agents))
	for _, c := range l.agents {
		out = append(out, c.info())
	}
	slices.SortFunc(out, func(a, b Info) int {
		return cmp.Or(a.Since.Compare(b.Since), cmp.Compare(a.ID, b.ID))
	})
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
// only agent that declares none, else ErrNoAgent.
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
	if ident.BoundAccount != "" && (h.Account != ident.BoundAccount || h.Kind != KindManaged) {
		l.Log.Warn("agent link: refused a hello outside the identity's account", "identity", ident.Subject, "declared", h.Account, "kind", h.Kind)
		_ = conn.Close(websocket.StatusPolicyViolation, "this identity serves only its own account, as a managed agent")
		return
	}
	ac := &agentConn{
		id: newID(), key: agentKey(ident, h), conn: conn, hello: h, ident: ident, since: time.Now(),
		pending: map[string]chan Message{},
	}
	var replaced []*agentConn
	l.mu.Lock()
	if l.agents == nil {
		l.agents = map[string]*agentConn{}
	}
	for id, old := range l.agents {
		if old.key == ac.key {
			delete(l.agents, id)
			replaced = append(replaced, old)
		}
	}
	l.agents[ac.id] = ac
	l.mu.Unlock()
	for _, old := range replaced {
		go func() { _ = old.conn.Close(websocket.StatusNormalClosure, "replaced by a new agent connection") }()
	}
	info := ac.info()
	l.Log.Info("agent link: connected", "id", ac.id, "host", h.Host, "version", h.Version, "identity", ident.Subject,
		"account", h.Account, "kind", info.Kind, "worker", h.Worker, "replaced", len(replaced))
	if l.OnConnect != nil {
		go l.OnConnect(info)
	}
	defer func() {
		l.mu.Lock()
		if l.agents[ac.id] == ac {
			delete(l.agents, ac.id)
		}
		for id, ch := range ac.pending {
			close(ch)
			delete(ac.pending, id)
		}
		ac.pending = nil
		l.mu.Unlock()
		_ = conn.CloseNow()
		l.Log.Info("agent link: disconnected", "id", ac.id, "host", h.Host, "account", h.Account)
	}()
	events := l.eventQueue(info)
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
			select {
			case events <- *m.Event:
			default:
				// Full: dropped; the agent sends its history again on reconnecting.
				l.Log.Warn("agent link: event queue full, dropped one", "id", ac.id)
			}
			continue
		}
		if m.Type != TypeResult {
			continue
		}
		l.mu.Lock()
		ch, ok := ac.pending[m.ID]
		delete(ac.pending, m.ID)
		l.mu.Unlock()
		if ok {
			ch <- m
		}
	}
}

// eventQueue starts the goroutine that hands one connection's events to
// OnEvent in order; closing the channel ends it.
func (l *Link) eventQueue(info Info) chan Event {
	ch := make(chan Event, 256)
	go func() {
		for ev := range ch {
			if l.OnEvent != nil {
				l.OnEvent(info, ev)
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

// CallAgent is Call to one connection, by its Info.ID.
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
	l.mu.Lock()
	if c.pending == nil { // the connection ended meanwhile
		l.mu.Unlock()
		return ErrNoAgent
	}
	c.pending[id] = ch
	l.mu.Unlock()
	drop := func() {
		l.mu.Lock()
		delete(c.pending, id)
		l.mu.Unlock()
	}
	b, err := json.Marshal(Message{Type: TypeCommand, ID: id, Command: command, Params: raw})
	if err != nil {
		drop()
		return err
	}
	c.wmu.Lock()
	err = c.conn.Write(ctx, websocket.MessageText, b)
	c.wmu.Unlock()
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
	// account is the account the asked agent declares (VNCOpen).
	account string
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
