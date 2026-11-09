package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // decode the Agent Server's frames
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/ironman-planner/internal/microbot"
)

// Game is what the executor needs from the Agent Server (microbot.Client).
type Game interface {
	State(ctx context.Context) (json.RawMessage, error)
	LoginState(ctx context.Context) (json.RawMessage, error)
	Login(ctx context.Context, world int) (json.RawMessage, error)
	Logout(ctx context.Context) (json.RawMessage, error)
	Screenshot(ctx context.Context) ([]byte, error)
	// JSON is one Agent Server request; only the executor's fixed commands
	// call it (internal/microbot.Client.JSON).
	JSON(ctx context.Context, method, path string, query url.Values, body any, timeout time.Duration) (json.RawMessage, error)
}

// Requester is one JSON request to a socket of the dev client.
type Requester interface {
	JSON(ctx context.Context, method, path string, query url.Values, body any, timeout time.Duration) (json.RawMessage, error)
}

// Executor runs the closed list of commands on this machine.
type Executor struct {
	Game Game
	// Home is the harness dev home (pid files of Xvfb and x11vnc under
	// Home/run).
	Home string
	// ClientCmd runs the harness's mb-client: argv before the subcommand
	// ("start --headless", "stop"), e.g. [nix develop <dir> --command
	// <dir>/bin/mb-client].
	ClientCmd []string
	// AccountCmd runs the harness's mb-account the same way.
	AccountCmd []string
	// Bridge is the control bridge socket of the Chronicle Link plugin (microbot.BridgeSocket):
	// the menus and actions the Agent Server lacks; nil turns them off.
	Bridge Requester

	mu sync.Mutex // one client lifecycle/login action at a time
}

// Execute runs one command of the closed list.
func (e *Executor) Execute(ctx context.Context, command string, params json.RawMessage) (any, error) {
	switch command {
	case CmdStatus:
		return e.status(ctx), nil
	case CmdScreenshot:
		png, err := e.Game.Screenshot(ctx)
		if err != nil {
			return nil, err
		}
		return toJPEG(png), nil
	case CmdPlugins, CmdHubSearch, CmdScriptDetail, CmdConfigGet, CmdConfigSchema:
		return e.plugin(ctx, command, params)
	case CmdGameView:
		return e.gameView(ctx), nil
	case CmdClientLogs, CmdAccountShow:
		return e.client(ctx, command, params)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch command {
	case CmdClientStart:
		out, err := e.runClient(ctx, 5*time.Minute, "start", "--headless")
		return ClientOutput{Output: out}, err
	case CmdClientStop:
		out, err := e.runClient(ctx, 2*time.Minute, "stop")
		return ClientOutput{Output: out}, err
	case CmdLogin:
		var p LoginParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("login params: %w", err)
			}
		}
		if p.World < 0 || p.World > 999 {
			return nil, fmt.Errorf("login: world %d out of range", p.World)
		}
		return e.Game.Login(ctx, p.World)
	case CmdLogout:
		return e.Game.Logout(ctx)
	case CmdVNCOpen:
		return nil, errors.New("vnc_open is served by the agent's link, not the executor")
	case CmdPluginInstall, CmdPluginRemove, CmdScriptStart, CmdScriptStop, CmdScriptsPause, CmdScriptsResume, CmdConfigSet:
		return e.plugin(ctx, command, params)
	}
	if slices.Contains(GameActions, command) {
		return e.act(ctx, command, params)
	}
	if command == CmdAccountUse || command == CmdWorldHop {
		return e.client(ctx, command, params)
	}
	return nil, fmt.Errorf("command %q is not in the agent's closed list", command)
}

func (e *Executor) status(ctx context.Context) Status {
	st := Status{Headless: pidAlive(filepath.Join(e.Home, "run", "xvfb.pid")), VNC: pidAlive(filepath.Join(e.Home, "run", "vnc.pid"))}
	state, err := e.Game.State(ctx)
	if err != nil {
		if !errors.Is(err, microbot.ErrNotRunning) {
			st.Error = err.Error()
		}
		return st
	}
	st.Running, st.State = true, state
	if login, err := e.Game.LoginState(ctx); err == nil {
		st.Login = login
	}
	return st
}

// runClient runs the harness's mb-client with args and returns the tail of
// its output.
func (e *Executor) runClient(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	if len(e.ClientCmd) == 0 {
		return "", errors.New("no mb-client command configured (MICROBOT_HARNESS_DIR or MICROBOT_CLIENT_CMD)")
	}
	return runArgv(ctx, e.ClientCmd, timeout, args...)
}

// runArgv runs a harness tool (base is its argv, e.g. nix develop ...
// --command <dir>/bin/mb-client) with args and returns the tail of its
// output.
func runArgv(ctx context.Context, base []string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := append(append([]string{}, base[1:]...), args...)
	cmd := exec.CommandContext(ctx, base[0], argv...) //nolint:gosec // the closed list fixes the argv
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	tail := lastLines(string(out), 60)
	if err != nil {
		return tail, fmt.Errorf("%s %s: %w", filepath.Base(base[len(base)-1]), strings.Join(args, " "), err)
	}
	return tail, nil
}

// toJPEG re-encodes the Agent Server's PNG frame as JPEG (quality 75): a few
// hundred KB instead of megabytes per screenshot. A frame that does not
// decode goes out as it came.
func toJPEG(png []byte) Screenshot {
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return Screenshot{Image: png, Type: "image/png"}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75}); err != nil {
		return Screenshot{Image: png, Type: "image/png"}
	}
	return Screenshot{Image: buf.Bytes(), Type: "image/jpeg"}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func pidAlive(file string) bool {
	b, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// Agent keeps the link to the server and serves its commands.
type Agent struct {
	// URL is the planner server (http(s)://host:port).
	URL string
	// Token gives the bearer of each connection (the link and every VNC
	// stream); nil or "" sends none, as a dev server expects.
	Token    func(ctx context.Context) (string, error)
	Executor *Executor
	Hello    Hello
	Log      *slog.Logger
	// VNCAddr is the harness's x11vnc (loopback only); empty means
	// 127.0.0.1:5999.
	VNCAddr string
}

// Run connects and reconnects until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		start := time.Now()
		err := a.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		a.Log.Warn("agent: link down, reconnecting", "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// header is the Authorization of one connection to the server.
func (a *Agent) header(ctx context.Context) (http.Header, error) {
	h := http.Header{}
	if a.Token == nil {
		return h, nil
	}
	tok, err := a.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent token: %w", err)
	}
	if tok != "" {
		h.Set("Authorization", "Bearer "+tok)
	}
	return h, nil
}

// StaticToken is a Token that always gives tok.
func StaticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}

// LinkURL is the server's agent endpoint for base.
func LinkURL(base string) string {
	u := strings.TrimRight(base, "/")
	u = strings.Replace(u, "http://", "ws://", 1)
	u = strings.Replace(u, "https://", "wss://", 1)
	return u + "/api/microbot/agent"
}

func (a *Agent) session(ctx context.Context) error {
	header, err := a.header(ctx)
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(ctx, LinkURL(a.URL), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial: HTTP %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(1 << 20)
	var wmu sync.Mutex
	write := func(m Message) error {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		wmu.Lock()
		defer wmu.Unlock()
		wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return conn.Write(wctx, websocket.MessageText, b)
	}
	hello := a.Hello
	hello.Commands = Commands
	if err := write(Message{Type: TypeHello, Hello: &hello}); err != nil {
		return err
	}
	a.Log.Info("agent: linked", "server", a.URL)
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var m Message
		if err := json.Unmarshal(raw, &m); err != nil || m.Type != TypeCommand || m.ID == "" {
			a.Log.Warn("agent: ignoring frame", "err", err)
			continue
		}
		go func(m Message) {
			res := Message{Type: TypeResult, ID: m.ID}
			switch {
			case !Known(m.Command):
				res.Error = fmt.Sprintf("command %q is not in the agent's closed list", m.Command)
			case m.Command == CmdVNCOpen:
				if err := a.openVNC(ctx, m.Params); err != nil {
					res.Error = err.Error()
				} else {
					res.OK = true
				}
			default:
				data, err := a.Executor.Execute(ctx, m.Command, m.Params)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.OK = true
					res.Data, _ = json.Marshal(data)
				}
			}
			if m.Command != CmdStatus && m.Command != CmdScreenshot {
				a.Log.Info("agent: command", "command", m.Command, "ok", res.OK, "err", res.Error)
			}
			if err := write(res); err != nil {
				a.Log.Warn("agent: write result", "err", err)
			}
		}(m)
	}
}
