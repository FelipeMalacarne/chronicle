package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
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
	if strings.Join(e.ClientCmd, " ") != "mb-client" || e.AccountCmd != nil || e.RunFile != "" || e.TriggerFile != "" {
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
