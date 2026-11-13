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
