package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Link errors.
var (
	// ErrNoAgent: no agent is connected.
	ErrNoAgent = errors.New("agent: no agent connected")
	// ErrUnknownCommand: the command is not in the closed list.
	ErrUnknownCommand = errors.New("agent: unknown command")
)

// CommandError is a command the agent ran and that failed.
type CommandError struct {
	Command string
	Message string
}

func (e *CommandError) Error() string { return e.Command + ": " + e.Message }

// Link is the server end: at most one agent at a time (one game client per
// machine); a new connection replaces the old one.
type Link struct {
	Log *slog.Logger

	mu      sync.Mutex
	conn    *websocket.Conn
	hello   Hello
	since   time.Time
	pending map[string]chan Message
	vnc     map[string]chan net.Conn
	wmu     sync.Mutex
}

// Info is what the server knows about the connected agent.
type Info struct {
	Connected bool      `json:"connected"`
	Since     time.Time `json:"since,omitzero"`
	Host      string    `json:"host,omitempty"`
	Version   string    `json:"version,omitempty"`
}

// Info reports the agent's connection.
func (l *Link) Info() Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return Info{}
	}
	return Info{Connected: true, Since: l.since, Host: l.hello.Host, Version: l.hello.Version}
}

// Serve upgrades an authenticated request to the agent's websocket and holds
// it until it closes. The caller has checked the machine token.
func (l *Link) Serve(w http.ResponseWriter, r *http.Request) {
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
	l.mu.Lock()
	if l.conn != nil {
		_ = l.conn.Close(websocket.StatusNormalClosure, "replaced by a new agent connection")
	}
	l.conn, l.hello, l.since = conn, *hello.Hello, time.Now()
	if l.pending == nil {
		l.pending = map[string]chan Message{}
	}
	l.mu.Unlock()
	l.Log.Info("agent link: connected", "host", hello.Hello.Host, "version", hello.Hello.Version)
	defer func() {
		l.mu.Lock()
		if l.conn == conn {
			l.conn = nil
			for id, ch := range l.pending {
				close(ch)
				delete(l.pending, id)
			}
		}
		l.mu.Unlock()
		_ = conn.CloseNow()
		l.Log.Info("agent link: disconnected", "host", hello.Hello.Host)
	}()
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var m Message
		if json.Unmarshal(raw, &m) != nil || m.Type != TypeResult {
			continue
		}
		l.mu.Lock()
		ch, ok := l.pending[m.ID]
		delete(l.pending, m.ID)
		l.mu.Unlock()
		if ok {
			ch <- m
		}
	}
}

// Call sends one command to the agent and waits for its result; into, when
// not nil, receives the result's data.
func (l *Link) Call(ctx context.Context, command string, params, into any) error {
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
	conn := l.conn
	if conn == nil {
		l.mu.Unlock()
		return ErrNoAgent
	}
	l.pending[id] = ch
	l.mu.Unlock()
	drop := func() {
		l.mu.Lock()
		delete(l.pending, id)
		l.mu.Unlock()
	}
	b, err := json.Marshal(Message{Type: TypeCommand, ID: id, Command: command, Params: raw})
	if err != nil {
		drop()
		return err
	}
	l.wmu.Lock()
	err = conn.Write(ctx, websocket.MessageText, b)
	l.wmu.Unlock()
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

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
