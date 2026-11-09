package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// VNC over the link (docs/08 phase 2). The browser opens /api/microbot/vnc on
// the server; the server asks the agent (CmdVNCOpen) with a session id; the
// agent dials the harness's x11vnc on loopback and opens a second OUTBOUND
// websocket, /api/microbot/agent/vnc?session=<id>, whose bytes the server
// splices to the browser's. The game machine never accepts a connection, and
// the VNC port never leaves loopback.

// ErrVNCTimeout: the agent did not bring the session's stream in time.
var ErrVNCTimeout = errors.New("agent: the VNC stream did not arrive")

// openVNC is the agent's side of CmdVNCOpen: it returns once both ends are
// connected and keeps splicing them in the background.
func (a *Agent) openVNC(ctx context.Context, params json.RawMessage) error {
	var p VNCParams
	if err := json.Unmarshal(params, &p); err != nil || p.Session == "" {
		return errors.New("vnc_open: missing session")
	}
	addr := a.VNCAddr
	if addr == "" {
		addr = "127.0.0.1:5999"
	}
	var d net.Dialer
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	vnc, err := d.DialContext(dctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("vnc_open: x11vnc not reachable on %s (is the client headless?): %w", addr, err)
	}
	u := LinkURL(a.URL) + "/vnc?session=" + url.QueryEscape(p.Session)
	ws, _, err := websocket.Dial(dctx, u, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + a.Token}},
	})
	if err != nil {
		_ = vnc.Close()
		return fmt.Errorf("vnc_open: stream to the server: %w", err)
	}
	ws.SetReadLimit(-1)
	a.Log.Info("agent: vnc session open", "session", p.Session)
	go func() {
		Splice(websocket.NetConn(ctx, ws, websocket.MessageBinary), vnc)
		a.Log.Info("agent: vnc session closed", "session", p.Session)
	}()
	return nil
}

// Splice copies both ways until either side ends, then closes both.
func Splice(a, b net.Conn) {
	var once sync.Once
	done := make(chan struct{})
	closeBoth := func() {
		once.Do(func() {
			_ = a.Close()
			_ = b.Close()
			close(done)
		})
	}
	go func() { _, _ = io.Copy(a, b); closeBoth() }()
	go func() { _, _ = io.Copy(b, a); closeBoth() }()
	<-done
}

// vncStream is the agent's stream of one session on the server; closing it
// ends the agent's request (ServeVNC).
type vncStream struct {
	net.Conn
	once sync.Once
	done chan struct{}
}

func (c *vncStream) Close() error {
	c.once.Do(func() { close(c.done) })
	return c.Conn.Close()
}

// OpenVNC asks the agent for a VNC session and returns its stream once the
// agent brings it; the caller splices it to the browser and closes it.
func (l *Link) OpenVNC(ctx context.Context) (net.Conn, error) {
	session := newID()
	stream := make(chan net.Conn, 1)
	l.mu.Lock()
	if l.vnc == nil {
		l.vnc = map[string]chan net.Conn{}
	}
	l.vnc[session] = stream
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.vnc, session)
		l.mu.Unlock()
		select { // a stream that arrived after the wait gave up
		case c := <-stream:
			_ = c.Close()
		default:
		}
	}()
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := l.Call(cctx, CmdVNCOpen, VNCParams{Session: session}, nil); err != nil {
		return nil, err
	}
	select {
	case c := <-stream:
		return c, nil
	case <-cctx.Done():
		return nil, ErrVNCTimeout
	}
}

// ServeVNC is the agent's per-session stream (the caller checked the machine
// token): it hands the connection to the waiting browser session and holds
// the request until the session ends.
func (l *Link) ServeVNC(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.URL.Query().Get("session"))
	l.mu.Lock()
	waiting, ok := l.vnc[session]
	l.mu.Unlock()
	if !ok {
		http.Error(w, "no browser is waiting for this VNC session", http.StatusNotFound)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(-1)
	ctx := r.Context()
	st := &vncStream{Conn: websocket.NetConn(ctx, conn, websocket.MessageBinary), done: make(chan struct{})}
	select {
	case waiting <- st:
	default:
		_ = conn.Close(websocket.StatusPolicyViolation, "session already has a stream")
		return
	}
	select {
	case <-st.done:
	case <-ctx.Done():
		_ = st.Close()
	}
}
