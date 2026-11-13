package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

func TestAttachedAgentWithSeveralClients(t *testing.T) {
	t.Setenv("MICROBOT_CLIENT_CMD", "mb-client")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	userHome, _ := os.UserHomeDir()
	env := map[string]string{"MICROBOT_AGENT_CLIENTS": "acc_main=~/.local/share/microbot-dev, acc_alt=/srv/mb/alt/"}
	a, err := attachedAgent("http://localhost:8080", func(k string) string { return env[k] }, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if a.Executor != nil || len(a.Clients) != 2 || a.Hello.Account != "" {
		t.Fatalf("agent = %+v", a)
	}
	for i, want := range []struct {
		account, home, display, vnc string
	}{
		{"acc_main", filepath.Join(userHome, ".local/share/microbot-dev"), ":99", "5999"},
		{"acc_alt", "/srv/mb/alt", ":100", "6000"},
	} {
		c := a.Clients[i]
		e := c.Executor
		env := []string{"MB_DEV_HOME=" + want.home, "MB_HEADLESS_DISPLAY=" + want.display, "MB_VNC_PORT=" + want.vnc}
		if c.Account != want.account || e.Home != want.home || !slices.Equal(e.Env, env) || c.VNCAddr != "127.0.0.1:"+want.vnc {
			t.Errorf("client %d = %+v, executor %+v", i, c, e)
		}
		if run := agent.DefaultRunFile(); run != "" && e.RunFile != strings.TrimSuffix(run, ".json")+"."+want.account+".json" {
			t.Errorf("client %d runs in %s", i, e.RunFile)
		}
		if e.TriggerFile == agent.DefaultTriggerFile() || e.RecurringFile == agent.DefaultRecurringFile() {
			t.Errorf("client %d shares the single client's files: %s %s", i, e.TriggerFile, e.RecurringFile)
		}
	}
	env["MB_VNC_PORT"], env["MB_HEADLESS_DISPLAY"] = "6100", ":20"
	if a, err = attachedAgent("", func(k string) string { return env[k] }, quiet); err != nil || a.Clients[1].VNCAddr != "127.0.0.1:6101" ||
		a.Clients[1].Executor.Env[1] != "MB_HEADLESS_DISPLAY=:21" {
		t.Fatalf("bases from the environment = %+v %v", a, err)
	}
	env["MICROBOT_AGENT_ACCOUNT"] = "acc_main"
	if _, err := attachedAgent("", func(k string) string { return env[k] }, quiet); err == nil {
		t.Fatal("took MICROBOT_AGENT_CLIENTS and MICROBOT_AGENT_ACCOUNT together")
	}
}

func TestAgentClients(t *testing.T) {
	if got, err := agentClients("  "); got != nil || err != nil {
		t.Fatalf("empty = %v %v", got, err)
	}
	got, err := agentClients("acc_a=/a;acc_b=/b\nacc_c=/c")
	if err != nil || len(got) != 3 || got[2] != (clientSpec{"acc_c", "/c"}) {
		t.Fatalf("separators = %v %v", got, err)
	}
	for _, bad := range []string{
		"acc_a", "acc_a=", "=/a", "main=/a", "acc_a=relative/home", "acc_a=/a,acc_a=/b", "acc_a=/a,acc_b=/a/",
		strings.Repeat("acc_x=/x ", agent.MaxClients+1),
	} {
		if _, err := agentClients(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
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
