package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/microbot"
)

// podGame is the Agent Server of a worker: up while the fake mb-client's
// marker file exists.
type podGame struct {
	*fakeGame
	marker   string
	loggedIn bool
	mu       sync.Mutex
	logouts  int
}

func (g *podGame) State(context.Context) (json.RawMessage, error) {
	if _, err := os.Stat(g.marker); err != nil {
		return nil, microbot.ErrNotRunning
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loggedIn {
		return json.RawMessage(`{"loggedIn":true}`), nil
	}
	return json.RawMessage(`{"loggedIn":false}`), nil
}

func (g *podGame) Logout(context.Context) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.logouts++
	g.loggedIn = false
	return json.RawMessage(`{"success":true}`), nil
}

// podExecutor is a --worker executor whose fake mb-client writes its own
// environment to env.txt, counts its starts and keeps the marker while
// "running".
func podExecutor(t *testing.T) (*Executor, *podGame, string) {
	t.Helper()
	dir := t.TempDir()
	game := &podGame{fakeGame: &fakeGame{}, marker: filepath.Join(dir, "running")}
	script := `case "$1" in
start) env > "$D/env.txt"; echo start >> "$D/starts"; touch "$D/running"; echo "client up, token=ipl_secretvalue";;
stop) rm -f "$D/running"; echo stop >> "$D/stops";;
esac`
	t.Setenv("D", dir)
	e := &Executor{
		Game: game, Home: filepath.Join(dir, "home"),
		ClientCmd: []string{"sh", "-c", script, "mb-client"},
		Worker:    &WorkerConfig{Account: "acc_iron", HWIDStoreDir: filepath.Join(dir, "secrets"), ReadyWait: 5 * time.Second},
	}
	return e, game, dir
}

func bootstrapParams(mut func(*BootstrapParams)) json.RawMessage {
	p := BootstrapParams{
		Account: "acc_iron",
		Session: GameSession{
			Character: "Iron Man", SessionID: "jx-session-secret", CharacterID: "12345",
			DeviceIDs: "iron.device=abc\n", RandomDats: "iron.dat=def",
		},
		PluginToken: "ipl_worker_token_secret", ServerURL: "http://chronicle.chronicle.svc", World: 303,
	}
	if mut != nil {
		mut(&p)
	}
	b, _ := json.Marshal(p)
	return b
}

func TestBootstrapOnlyInWorkerMode(t *testing.T) {
	e := &Executor{Game: &fakeGame{}, Home: t.TempDir(), ClientCmd: []string{"echo"}}
	if _, err := e.Execute(context.Background(), CmdWorkerBootstrap, bootstrapParams(nil)); err == nil || !strings.Contains(err.Error(), "--worker") {
		t.Fatalf("bootstrap outside worker mode = %v", err)
	}
	// And never from the web.
	if _, err := ValidateParams(CmdWorkerBootstrap, bootstrapParams(nil)); err == nil {
		t.Fatal("ValidateParams took worker_bootstrap")
	}
}

func TestBootstrapValidates(t *testing.T) {
	e, _, dir := podExecutor(t)
	for name, mut := range map[string]func(*BootstrapParams){
		"other account":   func(p *BootstrapParams) { p.Account = "acc_other" },
		"no session id":   func(p *BootstrapParams) { p.Session.SessionID = "" },
		"no character id": func(p *BootstrapParams) { p.Session.CharacterID = "" },
		"no character":    func(p *BootstrapParams) { p.Session.Character = "" },
		"newline":         func(p *BootstrapParams) { p.Session.SessionID = "jx-session-secret\nX=1" },
		"token":           func(p *BootstrapParams) { p.PluginToken = "tok_worker_token_secret" },
		"url":             func(p *BootstrapParams) { p.ServerURL = "file:///etc/passwd" },
		"relative url":    func(p *BootstrapParams) { p.ServerURL = "chronicle.svc" },
		"device lines":    func(p *BootstrapParams) { p.Session.DeviceIDs = "jx-session-secret" },
		"world":           func(p *BootstrapParams) { p.World = 5000 },
	} {
		_, err := e.Execute(context.Background(), CmdWorkerBootstrap, bootstrapParams(mut))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: the error carries a secret: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "starts")); err == nil {
		t.Fatal("a refused bootstrap started the client")
	}
}

func TestBootstrapStartsTheClient(t *testing.T) {
	e, game, dir := podExecutor(t)
	ctx := context.Background()
	if _, err := e.Execute(ctx, CmdClientStart, nil); err == nil {
		t.Fatal("client_start before the bootstrap")
	}
	res, err := e.Execute(ctx, CmdWorkerBootstrap, bootstrapParams(nil))
	if err != nil {
		t.Fatal(err)
	}
	if r := res.(BootstrapResult); !r.Started || strings.Contains(r.Output, "ipl_secretvalue") {
		t.Fatalf("bootstrap = %+v", r)
	}
	// The HWID agent's lines, mode 0600.
	for name, want := range map[string]string{
		"spoofed-device-ids.properties":  "iron.device=abc\n",
		"spoofed-random-dats.properties": "iron.dat=def\n",
	} {
		f := filepath.Join(dir, "secrets", name)
		st, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v", name, st.Mode().Perm())
		}
		if b, _ := os.ReadFile(f); string(b) != want {
			t.Errorf("%s = %q", name, b)
		}
	}
	// The session is in the child's environment, never in the agent's.
	env, err := os.ReadFile(filepath.Join(dir, "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range []string{
		"JX_SESSION_ID=jx-session-secret", "JX_CHARACTER_ID=12345", "JX_DISPLAY_NAME=Iron Man",
		"CHRONICLE_LINK_SERVER_URL=http://chronicle.chronicle.svc", "CHRONICLE_LINK_TOKEN=ipl_worker_token_secret",
	} {
		if !strings.Contains(string(env), kv+"\n") {
			t.Errorf("child env lacks %s", kv)
		}
	}
	for _, k := range []string{"JX_SESSION_ID", "JX_CHARACTER_ID", "CHRONICLE_LINK_TOKEN"} {
		if _, ok := os.LookupEnv(k); ok {
			t.Errorf("%s leaked into the agent's environment", k)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Home, ".runelite", "jagexcache", ".chronicle-boot.lock")); err != nil {
		t.Errorf("boot lock: %v", err)
	}
	// The fixed settings, through the Agent Server.
	game.mu.Lock()
	reqs := strings.Join(game.requests, "\n")
	game.mu.Unlock()
	for _, want := range []string{`"group":"fpscontrol","key":"limitFps","value":"true"`, `"group":"fpscontrol","key":"maxFps","value":"20"`} {
		if !strings.Contains(reqs, want) {
			t.Errorf("settings: %q not in %s", want, reqs)
		}
	}

	// Idempotent: with the client running, no second start.
	res, err = e.Execute(ctx, CmdWorkerBootstrap, bootstrapParams(nil))
	if err != nil || !res.(BootstrapResult).Started {
		t.Fatalf("second bootstrap = %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "starts")); strings.Count(string(b), "start") != 1 {
		t.Fatalf("starts = %q", b)
	}

	// A later client_start reuses the session.
	if _, err := e.Execute(ctx, CmdClientStop, nil); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "env.txt"))
	if _, err := e.Execute(ctx, CmdClientStart, nil); err != nil {
		t.Fatal(err)
	}
	if env, _ := os.ReadFile(filepath.Join(dir, "env.txt")); !strings.Contains(string(env), "JX_SESSION_ID=jx-session-secret\n") {
		t.Fatal("client_start after the bootstrap lost the session")
	}

	// SIGTERM: logout when logged in, then stop.
	game.mu.Lock()
	game.loggedIn = true
	game.mu.Unlock()
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := e.Shutdown(sctx); err != nil {
		t.Fatal(err)
	}
	if game.logouts != 1 {
		t.Errorf("logouts = %d", game.logouts)
	}
	if _, err := os.Stat(game.marker); !errors.Is(err, os.ErrNotExist) {
		t.Error("the client still runs after Shutdown")
	}
}

func TestWorkerRefusesAccountUse(t *testing.T) {
	e, _, _ := podExecutor(t)
	e.AccountCmd = []string{"echo"}
	if _, err := e.Execute(context.Background(), CmdAccountUse, json.RawMessage(`{"index":1}`)); err == nil || !strings.Contains(err.Error(), "own account") {
		t.Fatalf("account_use in worker mode = %v", err)
	}
}
