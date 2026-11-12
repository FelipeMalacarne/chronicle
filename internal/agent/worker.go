package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The --worker mode of the executor (docs/11 sections 6.3 and 13): the agent
// in a worker pod serves one account and gets the game session from the
// server (CmdWorkerBootstrap). The session lives only in this process's
// memory and in the environment of the client's process; it never goes to a
// log, and the HWID agent's device lines go to files of mode 0600 in the
// pod's in-memory secrets volume.

// WorkerConfig is the executor's --worker mode.
type WorkerConfig struct {
	// Account is WORKER_ACCOUNT: the only account the worker serves.
	Account string
	// HWIDStoreDir is MB_HWID_STORE_DIR, where the HWID agent reads its
	// spoofed-device-ids.properties and spoofed-random-dats.properties.
	HWIDStoreDir string
	// LockFile is the boot lock of the shared game cache (docs/11 section
	// 7.4); empty is <Home>/.runelite/jagexcache/.chronicle-boot.lock.
	LockFile string
	// ReadyWait bounds the wait for the Agent Server after mb-client start
	// (default 2 min).
	ReadyWait time.Duration

	mu  sync.Mutex
	env []string // the client's environment from the last bootstrap
}

// clientEnv is what the client's process gets on top of the agent's own
// environment: the session of the last bootstrap.
func (w *WorkerConfig) clientEnv() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.env...)
}

// character is the display name of the last bootstrap's session ("" before
// one): what the client logs in as.
func (w *WorkerConfig) character() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, kv := range w.env {
		if v, ok := strings.CutPrefix(kv, "JX_DISPLAY_NAME="); ok {
			return v
		}
	}
	return ""
}

func (w *WorkerConfig) hasSession() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.env) > 0
}

// workerSettings are the RuneLite settings a worker applies through the
// Agent Server once the client is up: a fixed list, never from the server.
// The FPS limit keeps a software-rendered client inside the pod's CPU limit.
// Checked in the pinned client 2.6.52: FpsConfig is @ConfigGroup("fpscontrol")
// with boolean limitFps and int maxFps; FpsPlugin (workerPlugins) must be on
// for them to apply.
var workerSettings = []struct{ group, key, value string }{
	{"fpscontrol", "limitFps", "true"},
	{"fpscontrol", "maxFps", "20"},
}

// validateBootstrap checks CmdWorkerBootstrap's params. Its errors never
// carry a value of the session.
func (e *Executor) validateBootstrap(raw json.RawMessage) (BootstrapParams, error) {
	var p BootstrapParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, errors.New("worker_bootstrap: params do not decode")
	}
	if p.Account != e.Worker.Account {
		return p, fmt.Errorf("worker_bootstrap: this worker serves %s, not %s", e.Worker.Account, p.Account)
	}
	for name, v := range map[string]string{
		"character": p.Session.Character, "session_id": p.Session.SessionID, "character_id": p.Session.CharacterID,
	} {
		if v == "" || len(v) > 4096 || strings.ContainsAny(v, "\x00\r\n") {
			return p, fmt.Errorf("worker_bootstrap: session.%s is missing or malformed", name)
		}
	}
	for name, v := range map[string]string{"device_ids": p.Session.DeviceIDs, "random_dats": p.Session.RandomDats} {
		if len(v) > 64<<10 || strings.ContainsRune(v, 0) {
			return p, fmt.Errorf("worker_bootstrap: session.%s is malformed", name)
		}
		for line := range strings.SplitSeq(v, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") && !strings.Contains(line, "=") {
				return p, fmt.Errorf("worker_bootstrap: session.%s has a line that is not key=value", name)
			}
		}
	}
	if !strings.HasPrefix(p.PluginToken, "ipl_") || len(p.PluginToken) < 8 || len(p.PluginToken) > 512 ||
		strings.ContainsAny(p.PluginToken, " \t\r\n\x00") {
		return p, errors.New("worker_bootstrap: plugin_token must be an ipl_ token")
	}
	u, err := url.Parse(p.ServerURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return p, errors.New("worker_bootstrap: server_url must be an http or https URL")
	}
	if p.World < 0 || p.World > 999 {
		return p, errors.New("worker_bootstrap: world out of range")
	}
	return p, nil
}

// bootstrap is CmdWorkerBootstrap (under e.mu): it keeps the session for the
// client's process, writes the HWID agent's files and starts the client,
// unless it is already running.
func (e *Executor) bootstrap(ctx context.Context, raw json.RawMessage) (any, error) {
	if e.Worker == nil {
		return nil, errors.New("worker_bootstrap: this agent is not in --worker mode")
	}
	p, err := e.validateBootstrap(raw)
	if err != nil {
		return nil, err
	}
	w := e.Worker
	w.mu.Lock()
	w.env = []string{
		"JX_SESSION_ID=" + p.Session.SessionID,
		"JX_CHARACTER_ID=" + p.Session.CharacterID,
		"JX_DISPLAY_NAME=" + p.Session.Character,
		"CHRONICLE_LINK_SERVER_URL=" + p.ServerURL,
		"CHRONICLE_LINK_TOKEN=" + p.PluginToken,
	}
	w.mu.Unlock()
	if e.status(ctx).Running {
		return BootstrapResult{Started: true}, nil
	}
	if herr := e.writeHWID(p.Session); herr != nil {
		return nil, herr
	}
	out, err := e.workerStart(ctx)
	if err != nil {
		return BootstrapResult{Output: out}, err
	}
	return BootstrapResult{Started: true, Output: out}, nil
}

// workerStart starts the client in worker mode (under e.mu): with the
// session's environment, holding the boot lock until the Agent Server
// answers, then the fixed settings.
func (e *Executor) workerStart(ctx context.Context) (string, error) {
	if !e.Worker.hasSession() {
		return "", errors.New("client_start: no game session yet (the server sends it in the bootstrap)")
	}
	if e.status(ctx).Running {
		return "the client is already running", nil
	}
	unlock, err := e.bootLock(ctx)
	if err != nil {
		return "", err
	}
	out, err := e.runClient(ctx, 5*time.Minute, "start", "--headless")
	out = maskLines(out)
	if err == nil {
		err = e.waitReady(ctx)
	}
	unlock()
	if err != nil {
		return out, err
	}
	e.applyWorkerSettings(ctx)
	return out, nil
}

// waitReady waits for the Agent Server to answer.
func (e *Executor) waitReady(ctx context.Context) error {
	limit := e.Worker.ReadyWait
	if limit <= 0 {
		limit = 2 * time.Minute
	}
	deadline := time.Now().Add(limit)
	for !e.status(ctx).Running {
		if time.Now().After(deadline) {
			return errors.New("client_start: the Agent Server did not answer after mb-client start")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

// workerPlugins are started through the Agent Server once the client is up
// (a fixed list): the pod's home is new on every boot, and the Chronicle
// Link ships disabled by default, so nothing would track the account; the
// FPS Control plugin makes the FPS limit apply.
var workerPlugins = []string{
	"net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin",
	"net.runelite.client.plugins.fps.FpsPlugin",
}

func (e *Executor) applyWorkerSettings(ctx context.Context) {
	for _, class := range workerPlugins {
		if _, err := e.Game.JSON(ctx, http.MethodPost, "/scripts/start", nil, map[string]any{"className": class}, 30*time.Second); err != nil {
			e.log().Warn("worker: could not start a plugin", "class", class, "err", err)
		}
	}
	for _, s := range workerSettings {
		v := s.value
		if _, err := e.Game.JSON(ctx, http.MethodPost, "/settings/plugin", nil,
			map[string]any{"group": s.group, "key": s.key, "value": &v}, 10*time.Second); err != nil {
			e.log().Warn("worker: could not apply a client setting", "group", s.group, "key", s.key, "err", err)
		}
	}
}

// bootLock holds flock on the game cache's boot lock (docs/11 section 7.4)
// until the returned func runs.
func (e *Executor) bootLock(ctx context.Context) (func(), error) {
	file := e.Worker.LockFile
	if file == "" {
		file = filepath.Join(e.Home, ".runelite", "jagexcache", ".chronicle-boot.lock")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil { //nolint:gosec // the shared game cache, not a secret
		return nil, fmt.Errorf("boot lock: %w", err)
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // a fixed file of the dev home
	if err != nil {
		return nil, fmt.Errorf("boot lock: %w", err)
	}
	fd := int(f.Fd()) //nolint:gosec // a file descriptor fits an int
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, fmt.Errorf("boot lock: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("boot lock: %w", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// writeHWID writes the character's lines of the HWID agent's stores, mode
// 0600; an empty part writes nothing.
func (e *Executor) writeHWID(s GameSession) error {
	dir := e.Worker.HWIDStoreDir
	if dir == "" {
		return errors.New("worker_bootstrap: no HWID store directory (MB_HWID_STORE_DIR)")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("worker_bootstrap: HWID store: %w", err)
	}
	for name, content := range map[string]string{
		"spoofed-device-ids.properties":  s.DeviceIDs,
		"spoofed-random-dats.properties": s.RandomDats,
	} {
		if strings.TrimSpace(content) == "" {
			continue
		}
		if err := writePrivate(filepath.Join(dir, name), strings.TrimRight(content, "\n")+"\n"); err != nil {
			return fmt.Errorf("worker_bootstrap: write %s: %w", name, err)
		}
	}
	return nil
}

// writePrivate replaces file with content, mode 0600, atomically.
func writePrivate(file, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.WriteString(content)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// maskLines masks what looks like a credential in a harness command's output.
func maskLines(out string) string {
	lines := strings.Split(out, "\n")
	for i := range lines {
		lines[i] = maskSecrets(lines[i])
	}
	return strings.Join(lines, "\n")
}

// Shutdown is the worker's SIGTERM (docs/11 section 6.5): logout when logged
// in, then mb-client stop, within ctx (the caller gives it about 45 s).
func (e *Executor) Shutdown(ctx context.Context) error {
	if st, err := e.Game.State(ctx); err == nil {
		var s struct {
			LoggedIn bool `json:"loggedIn"`
		}
		if json.Unmarshal(st, &s) == nil && s.LoggedIn {
			lctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if _, err := e.Game.Logout(lctx); err != nil {
				e.log().Warn("worker: logout on shutdown", "err", err)
			}
			cancel()
		}
	}
	if len(e.ClientCmd) == 0 {
		return nil
	}
	timeout := 30 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		timeout = max(time.Until(dl), time.Second)
	}
	_, err := e.runClient(ctx, timeout, "stop")
	return err
}
