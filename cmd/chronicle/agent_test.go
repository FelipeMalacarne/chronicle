package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

func TestWorkerAgentFromTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"CHRONICLE_URL": "http://chronicle.chronicle.svc/", "CHRONICLE_AGENT_TOKEN_FILE": tokenFile,
		"WORKER_ACCOUNT": "acc_iron", "HOSTNAME": "worker-acc-iron-6d9f", "MB_DEV_HOME": filepath.Join(dir, "home"),
		"MB_HWID_STORE_DIR": "/run/chronicle",
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, missing := range []string{"CHRONICLE_URL", "CHRONICLE_AGENT_TOKEN_FILE", "WORKER_ACCOUNT"} {
		partial := map[string]string{}
		for k, v := range env {
			if k != missing {
				partial[k] = v
			}
		}
		if _, err := workerAgent("", func(k string) string { return partial[k] }, quiet); err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("without %s = %v", missing, err)
		}
	}
	a, err := workerAgent("", func(k string) string { return env[k] }, quiet)
	if err != nil {
		t.Fatal(err)
	}
	h := a.Hello
	if a.URL != "http://chronicle.chronicle.svc" || h.Kind != agent.KindManaged || h.Account != "acc_iron" || h.Worker != "worker-acc-iron-6d9f" || h.Bridge != "" {
		t.Fatalf("agent = %s %+v", a.URL, h)
	}
	e := a.Executor
	if e.Worker == nil || e.Worker.Account != "acc_iron" || e.Worker.HWIDStoreDir != "/run/chronicle" || e.Home != env["MB_DEV_HOME"] {
		t.Fatalf("executor = %+v %+v", e, e.Worker)
	}
	if strings.Join(e.ClientCmd, " ") != "mb-client" || e.AccountCmd != nil || e.RunFile != "" || e.TriggerFile != "" || e.RecurringFile != "" {
		t.Fatalf("executor commands and files = %v %v %q %q", e.ClientCmd, e.AccountCmd, e.RunFile, e.TriggerFile)
	}
	if _, err := e.Execute(context.Background(), agent.CmdAccountUse, json.RawMessage(`{"index":1}`)); err == nil {
		t.Fatal("worker ran account_use")
	}
	// The projected token is read again on every connection.
	for _, want := range []string{"first", "rotated"} {
		if want == "rotated" {
			if err := os.WriteFile(tokenFile, []byte("rotated"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if tok, err := a.Token(context.Background()); err != nil || tok != want {
			t.Fatalf("token = %q %v, want %q", tok, err, want)
		}
	}
}

func TestAttachedAgentWithOneClient(t *testing.T) {
	t.Setenv("MICROBOT_CLIENT_CMD", "mb-client")
	t.Setenv("MICROBOT_HOME", "/homes/main")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := map[string]string{"MICROBOT_AGENT_ACCOUNT": "acc_main", "MB_VNC_PORT": "5901"}
	a, err := attachedAgent("http://localhost:8080", func(k string) string { return env[k] }, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Clients) != 0 || a.Hello.Account != "acc_main" || a.Hello.Kind != agent.KindAttached || a.VNCAddr != "127.0.0.1:5901" {
		t.Fatalf("agent = %+v", a)
	}
	e := a.Executor
	if e.Home != "/homes/main" || e.Env != nil || e.RunFile != agent.DefaultRunFile() || strings.Join(e.ClientCmd, " ") != "mb-client" {
		t.Fatalf("executor = %+v", e)
	}
}

func TestAttachedAgentWithSlots(t *testing.T) {
	t.Setenv("MICROBOT_CLIENT_CMD", "mb-client")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := map[string]string{"MICROBOT_AGENT_SLOTS": "3", "MICROBOT_SLOTS_HOME": "/srv/mb/slots/", "MB_VNC_PORT": "6100", "MB_HEADLESS_DISPLAY": ":20"}
	a, err := attachedAgent("http://localhost:8080", func(k string) string { return env[k] }, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if a.Slots != 3 || a.NewSlot == nil || a.Executor != nil || len(a.Clients) != 0 || a.Hello.Account != "" || a.Hello.Kind != agent.KindAttached {
		t.Fatalf("agent = %+v", a)
	}
	c := a.NewSlot(1, "acc_alt")
	e := c.Executor
	home := "/srv/mb/slots/acc_alt"
	wantEnv := []string{"MB_DEV_HOME=" + home, "MB_HEADLESS_DISPLAY=:21", "MB_VNC_PORT=6101", "MB_HWID_STORE_DIR=" + home + "/hwid"}
	if c.Account != "acc_alt" || c.VNCAddr != "127.0.0.1:6101" || e.Home != home || !slices.Equal(e.Env, wantEnv) {
		t.Fatalf("client = %+v, executor %+v", c, e)
	}
	// A slot's client is a worker: its own account, its own HWID store.
	if e.Worker == nil || e.Worker.Account != "acc_alt" || e.Worker.HWIDStoreDir != home+"/hwid" || e.AccountCmd != nil {
		t.Fatalf("worker = %+v account cmd %v", e.Worker, e.AccountCmd)
	}
	if _, err := e.Execute(context.Background(), agent.CmdAccountUse, json.RawMessage(`{"index":1}`)); err == nil {
		t.Fatal("a slot's client ran account_use")
	}
	if run := agent.DefaultRunFile(); run != "" && e.RunFile != strings.TrimSuffix(run, ".json")+".acc_alt.json" {
		t.Errorf("the slot's runs go to %s", e.RunFile)
	}
	if e.TriggerFile == agent.DefaultTriggerFile() || e.RecurringFile == agent.DefaultRecurringFile() {
		t.Errorf("the slot shares the single client's files: %s %s", e.TriggerFile, e.RecurringFile)
	}
	if other := a.NewSlot(0, "acc_main").Executor; other.Home == e.Home || other.Worker.HWIDStoreDir == e.Worker.HWIDStoreDir {
		t.Error("two slots share a home or a HWID store")
	}

	env["MICROBOT_AGENT_ACCOUNT"] = "acc_main"
	if _, err := attachedAgent("", func(k string) string { return env[k] }, quiet); err == nil {
		t.Fatal("took MICROBOT_AGENT_SLOTS and MICROBOT_AGENT_ACCOUNT together")
	}
	delete(env, "MICROBOT_AGENT_ACCOUNT")
	for _, bad := range []string{"0", "-1", "two", strconv.Itoa(agent.MaxClients + 1)} {
		env["MICROBOT_AGENT_SLOTS"] = bad
		if _, err := attachedAgent("", func(k string) string { return env[k] }, quiet); err == nil {
			t.Errorf("MICROBOT_AGENT_SLOTS=%q accepted", bad)
		}
	}
	env["MICROBOT_AGENT_SLOTS"], env["MICROBOT_SLOTS_HOME"] = "2", "relative/slots"
	if _, err := attachedAgent("", func(k string) string { return env[k] }, quiet); err == nil {
		t.Error("a relative MICROBOT_SLOTS_HOME accepted")
	}
}

func TestSlotsHomeDefaultsUnderTheUserHome(t *testing.T) {
	userHome, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		"":                filepath.Join(userHome, ".local/share/microbot-slots"),
		"~/mb/slots":      filepath.Join(userHome, "mb/slots"),
		"/var/lib/mb/../": "/var/lib",
	} {
		if got, err := slotsHome(in); err != nil || got != want {
			t.Errorf("slotsHome(%q) = %q %v, want %q", in, got, err, want)
		}
	}
}

func TestPortsDisplaysAndFiles(t *testing.T) {
	if _, err := basePort("80"); err == nil {
		t.Error("MB_VNC_PORT 80 accepted")
	}
	if _, err := baseDisplay("display"); err == nil {
		t.Error("MB_HEADLESS_DISPLAY display accepted")
	}
	if got := clientFile("/c/agent-runs.json", "acc_a"); got != "/c/agent-runs.acc_a.json" {
		t.Errorf("clientFile = %s", got)
	}
	if got := clientFile("", "acc_a"); got != "" {
		t.Errorf("clientFile of memory = %q", got)
	}
}

func TestHarnessCmd(t *testing.T) {
	t.Setenv("MICROBOT_CLIENT_CMD", "")
	t.Setenv("MICROBOT_HARNESS_DIR", "")
	if got := harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"); got != nil {
		t.Fatalf("no harness: %v", got)
	}
	// The chronicle checkout: its microbot dev shell and microbot/bin.
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "microbot", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "microbot", "bin", "mb-client"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MICROBOT_HARNESS_DIR", repo)
	want := []string{"nix", "develop", repo + "#microbot", "--command", filepath.Join(repo, "microbot", "bin", "mb-client")}
	if got := harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"); !slices.Equal(got, want) {
		t.Fatalf("monorepo: %v", got)
	}
	// An old microbot-plugins checkout: its default shell and bin/.
	old := t.TempDir()
	t.Setenv("MICROBOT_HARNESS_DIR", old)
	want = []string{"nix", "develop", old, "--command", filepath.Join(old, "bin", "mb-client")}
	if got := harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"); !slices.Equal(got, want) {
		t.Fatalf("old checkout: %v", got)
	}
	// The explicit command wins.
	t.Setenv("MICROBOT_CLIENT_CMD", "/usr/bin/mb-client --x")
	if got := harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"); !slices.Equal(got, []string{"/usr/bin/mb-client", "--x"}) {
		t.Fatalf("explicit: %v", got)
	}
}

func TestLiveSlotReadsTheDisplayOfARunningClient(t *testing.T) {
	home := t.TempDir()
	if _, ok := liveSlot(home, 99); ok {
		t.Fatal("a home with no client has a slot")
	}
	run := filepath.Join(home, "run")
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "xvfb.display"), []byte(":101\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := liveSlot(home, 99); ok {
		t.Fatal("a display file without a live Xvfb gave a slot")
	}
	if err := os.WriteFile(filepath.Join(run, "xvfb.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if slot, ok := liveSlot(home, 99); !ok || slot != 2 {
		t.Fatalf("slot = %d, %v; want 2", slot, ok)
	}
}
