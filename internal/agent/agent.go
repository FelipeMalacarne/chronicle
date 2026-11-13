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

	"github.com/FelipeMalacarne/chronicle/internal/microbot"
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
	// Env is added to the environment of the harness tools (mb-client,
	// mb-account) as KEY=value: for each client of an agent that drives
	// several, its MB_DEV_HOME, MB_HEADLESS_DISPLAY and MB_VNC_PORT; nil
	// leaves them to the agent's own environment.
	Env []string
	// Bridge is the control bridge socket of the Chronicle Link plugin (microbot.BridgeSocket):
	// the menus and actions the Agent Server lacks; nil turns them off.
	Bridge Requester

	// RunPoll is how often a run reads the game (default 3 s; tests shorten it).
	RunPoll time.Duration
	// SafeWait bounds a run's wait for a safe point (default 1 h).
	SafeWait time.Duration
	// RunFile keeps the runs' history (DefaultRunFile); empty keeps it in
	// memory only.
	RunFile string
	// TriggerFile keeps the triggers (DefaultTriggerFile); empty keeps them in
	// memory only.
	TriggerFile string
	// WatchPoll is how often the triggers' watcher reads the bridge (default 5 s).
	WatchPoll time.Duration
	// RecurringFile keeps the recurring routines the server sent
	// (DefaultRecurringFile); empty keeps them in memory only.
	RecurringFile string
	// RecurringPoll is how often the recurring routines' watcher looks
	// (default 30 s).
	RecurringPoll time.Duration
	// Worker is the --worker mode (docs/11): set, the executor serves one
	// managed account, takes CmdWorkerBootstrap and refuses CmdAccountUse;
	// nil outside that mode.
	Worker *WorkerConfig
	// Log receives the executor's own warnings; nil discards them.
	Log *slog.Logger
	// OnRun, when set, gets the current run at every change of it (each
	// line of its trail), under the runner's lock: it must not block. The
	// agent queues it for the server (TypeEvent).
	OnRun func(Run)

	mu    sync.Mutex // one client lifecycle/login action at a time
	runs  runner
	watch watcher
	recur recurrer
	// now is the recurring watcher's clock (tests); nil is time.Now.
	now func() time.Time
	// taskTimeout, when set, bounds every task run (tests).
	taskTimeout time.Duration
	// untilGrace, when set, replaces the wait for the server's proof after
	// a run plugin stopped by itself (tests).
	untilGrace time.Duration
}

// Execute runs one command of the closed list.
func (e *Executor) Execute(ctx context.Context, command string, params json.RawMessage) (any, error) {
	if !slices.Contains(readCommands, command) && e.going() {
		return nil, ErrRunGoing
	}
	if e.Worker != nil && command == CmdAccountUse {
		return nil, errors.New("account_use: a worker serves only its own account")
	}
	switch command {
	case CmdRunStart, CmdRunStatus, CmdRunHistory, CmdRunCancel:
		return e.runCommand(command, params)
	case CmdTaskRun:
		return e.taskRun(params)
	case CmdRunUntil:
		return e.runUntil(params)
	case CmdTriggerGet, CmdTriggerSet, CmdRoutinesSync:
		return e.triggerCommand(command, params)
	case CmdRecurringGet:
		return e.recurringStatus(), nil
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
	case CmdTimers:
		return e.timers(ctx)
	case CmdClientLogs, CmdAccountShow:
		return e.client(ctx, command, params)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch command {
	case CmdWorkerBootstrap:
		return e.bootstrap(ctx, params)
	case CmdClientStart:
		if e.Worker != nil {
			out, problems, err := e.workerStart(ctx)
			for _, p := range problems {
				out += "\nprofile: " + p
			}
			return ClientOutput{Output: out}, err
		}
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

func (e *Executor) log() *slog.Logger {
	if e.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return e.Log
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
	if st.Bridge = e.bridgeVersion(ctx); st.Bridge != "" {
		st.Break = e.bridgeBreak(ctx)
	}
	return st
}

// runClient runs the harness's mb-client with args and returns the tail of
// its output.
func (e *Executor) runClient(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	if len(e.ClientCmd) == 0 {
		return "", errors.New("no mb-client command configured (MICROBOT_HARNESS_DIR or MICROBOT_CLIENT_CMD)")
	}
	env := slices.Clone(e.Env)
	if e.Worker != nil {
		env = append(env, e.Worker.clientEnv()...)
	}
	return runArgv(ctx, e.ClientCmd, timeout, env, args...)
}

// runArgv runs a harness tool (base is its argv, e.g. nix develop ...
// --command <dir>/bin/mb-client) with args and returns the tail of its
// output. env is added to this process's environment for the child only.
func runArgv(ctx context.Context, base []string, timeout time.Duration, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := append(append([]string{}, base[1:]...), args...)
	cmd := exec.CommandContext(ctx, base[0], argv...) //nolint:gosec // the closed list fixes the argv
	cmd.Env = append(os.Environ(), env...)
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
	Token func(ctx context.Context) (string, error)
	// Executor drives the agent's one game client; Hello.Account is its
	// account.
	Executor *Executor
	Hello    Hello
	Log      *slog.Logger
	// VNCAddr is the harness's x11vnc (loopback only); empty means
	// 127.0.0.1:5999.
	VNCAddr string
	// Clients, when set, are the game clients of an attached agent that
	// drives several (docs/10 section 10), in place of Executor, VNCAddr and
	// Hello.Account: each with its own account, executor (its own lock, run,
	// triggers and recurring routines) and x11vnc. The server tags every
	// command with the client's account (Message.Account).
	Clients []Client

	out outbox
}

// Client is one game client of an agent: a harness instance (its own dev
// home), the executor that drives it and the account it serves.
type Client struct {
	// Account is the Chronicle account the client serves.
	Account  string
	Executor *Executor
	// VNCAddr is the client's x11vnc (loopback only) when its dev home does
	// not say (vncAddr); empty means 127.0.0.1:5999.
	VNCAddr string
}

// clients are the game clients the agent drives: Clients, else the one of
// Executor.
func (a *Agent) clients() []Client {
	if len(a.Clients) > 0 {
		return a.Clients
	}
	return []Client{{Account: a.Hello.Account, Executor: a.Executor, VNCAddr: a.VNCAddr}}
}

// outbox holds the runs waiting for the link, the latest state of each:
// a run that changes twice before it goes out goes once.
type outbox struct {
	mu      sync.Mutex
	pending map[outKey]Run
	order   []outKey
	wake    chan struct{}
}

// outKey is a run of one client: the account the client serves and the
// run's uid.
type outKey struct{ account, uid string }

// outRun is a queued run and the account of its client.
type outRun struct {
	account string
	run     Run
}

func (o *outbox) init() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.wake == nil {
		o.wake = make(chan struct{}, 1)
		o.pending = map[outKey]Run{}
	}
}

// put queues a run of the client of account.
func (o *outbox) put(account string, r Run) {
	if r.UID == "" {
		return
	}
	k := outKey{account, r.UID}
	o.mu.Lock()
	cur, ok := o.pending[k]
	switch {
	case !ok:
		o.order = append(o.order, k)
		o.pending[k] = r
	case len(r.Events) >= len(cur.Events):
		// A run's trail only grows: the longer one is the newer state.
		o.pending[k] = r
	}
	o.mu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

// take empties the queue of the runs whose account linked accepts, in the
// order they were queued; the others stay for a link that serves them.
func (o *outbox) take(linked func(account string) bool) []outRun {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]outRun, 0, len(o.order))
	var keep []outKey
	for _, k := range o.order {
		if !linked(k.account) {
			keep = append(keep, k)
			continue
		}
		out = append(out, outRun{account: k.account, run: o.pending[k]})
		delete(o.pending, k)
	}
	o.order = keep
	return out
}

// Run connects and reconnects until ctx ends. The watchers of the triggers
// and of the recurring routines run beside the link, one pair per client, so
// they fire while the server is away.
func (a *Agent) Run(ctx context.Context) error {
	a.out.init()
	for _, c := range a.clients() {
		if c.Executor.OnRun == nil {
			account := c.Account
			c.Executor.OnRun = func(r Run) { a.out.put(account, r) }
		}
		go c.Executor.Watch(ctx)
		go c.Executor.WatchRecurring(ctx)
	}
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

// sendEvents writes the queued runs of the linked clients as TypeEvent
// frames until the session's ctx ends or a write fails; what did not go out
// goes back to the queue for the next connection. tagged names each run's
// client (Message.Account).
func (a *Agent) sendEvents(ctx context.Context, write func(Message) error, linked []Client, tagged bool) {
	served := func(account string) bool {
		return slices.ContainsFunc(linked, func(c Client) bool { return c.Account == account })
	}
	for {
		runs := a.out.take(served)
		for i := range runs {
			m := Message{Type: TypeEvent, Event: &Event{Run: &runs[i].run}}
			if tagged {
				m.Account = runs[i].account
			}
			if ctx.Err() != nil || write(m) != nil {
				for _, r := range runs[i:] {
					a.out.put(r.account, r.run)
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-a.out.wake:
		}
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
	hello, linked, tagged := a.hello(resp)
	if err := write(Message{Type: TypeHello, Hello: &hello}); err != nil {
		return err
	}
	a.Log.Info("agent: linked", "server", a.URL, "clients", len(linked))
	// The history goes out again on every connection, oldest first: what
	// changed while the link was down reaches the server (it applies a run
	// idempotently), then every change as it happens.
	for _, c := range linked {
		for _, r := range slices.Backward(c.Executor.runHistory().Runs) {
			a.out.put(c.Account, r)
		}
	}
	sctx, stop := context.WithCancel(ctx)
	defer stop()
	go a.sendEvents(sctx, write, linked, tagged)
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
			c, cerr := clientFor(linked, tagged, m.Account)
			switch {
			case !Known(m.Command):
				res.Error = fmt.Sprintf("command %q is not in the agent's closed list", m.Command)
			case cerr != nil:
				res.Error = cerr.Error()
			case m.Command == CmdVNCOpen:
				if err := a.openVNC(ctx, c, m.Params); err != nil {
					res.Error = err.Error()
				} else {
					res.OK = true
				}
			default:
				data, err := c.Executor.Execute(ctx, m.Command, m.Params)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.OK = true
					res.Data, _ = json.Marshal(data)
				}
			}
			if m.Command != CmdStatus && m.Command != CmdScreenshot {
				a.Log.Info("agent: command", "command", m.Command, "account", c.Account, "ok", res.OK, "err", res.Error)
			}
			if err := write(res); err != nil {
				a.Log.Warn("agent: write result", "err", err)
			}
		}(m)
	}
}

// hello is what the agent declares on a connection whose upgrade answer was
// resp, and the clients the connection serves. An agent with several
// clients declares them all (tagged) to a server that announces
// FeatureClients; an older server would take the commands and events of
// every client as the first one's, so it gets the first client alone, and
// the other clients' runs wait in the outbox for a server that knows them.
func (a *Agent) hello(resp *http.Response) (h Hello, linked []Client, tagged bool) {
	h = a.Hello
	h.Commands = Commands
	h.Clients = nil
	all := a.clients()
	if len(a.Clients) == 0 {
		return h, all, false
	}
	h.Account = all[0].Account
	if len(all) == 1 {
		return h, all, false
	}
	if resp == nil || !slices.Contains(strings.Split(resp.Header.Get(FeaturesHeader), ","), FeatureClients) {
		a.Log.Warn("agent: the server does not route several clients per agent (older than this agent); linking only the first",
			"account", all[0].Account, "left_out", len(all)-1)
		return h, all[:1], false
	}
	for _, c := range all {
		h.Clients = append(h.Clients, ClientHello{Account: c.Account})
	}
	return h, all, true
}

// clientFor is the client a command goes to: on a tagged link the one its
// account names, else the only one linked (a command naming another account
// is refused there too).
func clientFor(linked []Client, tagged bool, account string) (Client, error) {
	if !tagged {
		if account == "" || account == linked[0].Account {
			return linked[0], nil
		}
		return Client{}, fmt.Errorf("this agent does not drive a client of %s", account)
	}
	for _, c := range linked {
		if c.Account == account {
			return c, nil
		}
	}
	if account == "" {
		return Client{}, errors.New("this agent drives several clients: the command must name the account")
	}
	return Client{}, fmt.Errorf("this agent does not drive a client of %s", account)
}
