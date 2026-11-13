package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

// One attached agent with two clients (docs/10 section 10), through the
// real routes: the server announces the feature through its middleware, the
// agent declares both clients, and ?account= reaches each one, audited with
// the client's own agent id.
func TestMicrobotRoutesTheClientsOfOneAgent(t *testing.T) {
	audit := &memAudit{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: link, Audit: audit},
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := &agent.Agent{
		URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Kind: agent.KindAttached},
		Clients: []agent.Client{
			{Account: "acc_main", Executor: &agent.Executor{Game: stubGame{}, Home: t.TempDir(), ClientCmd: []string{"echo", "MAIN"}}},
			{Account: "acc_alt", Executor: &agent.Executor{Game: stubGame{}, Home: t.TempDir(), ClientCmd: []string{"echo", "ALT"}}},
		},
	}
	go func() { _ = a.Run(ctx) }()
	waitLinked(t, link, 2)

	ids := map[string]bool{}
	for account, want := range map[string]string{"acc_main": "MAIN ", "acc_alt": "ALT "} {
		status, body, _ := call(t, "POST", srv.URL+"/api/microbot/client/start?account="+account, "")
		if status != http.StatusOK {
			t.Fatalf("%s start = %d %v", account, status, body)
		}
		if out := body["result"].(map[string]any)["output"].(string); !strings.HasPrefix(out, want) {
			t.Fatalf("%s went to %q", account, out)
		}
		served, _ := link.Agent(account)
		if last := audit.last(); last.AccountID != account || last.AgentID != served.ID || !last.OK {
			t.Fatalf("%s audit = %+v (agent %s)", account, last, served.ID)
		}
		ids[served.ID] = true
		status, body, _ = call(t, "GET", srv.URL+"/api/microbot/status?account="+account, "")
		if ag := body["agent"].(map[string]any); status != http.StatusOK || ag["account"] != account || ag["host"] != "zaros" || ag["kind"] != "attached" {
			t.Fatalf("%s status = %d %v", account, status, body)
		}
		if len(body["agents"].([]any)) != 2 || body["client"].(map[string]any)["running"] != true {
			t.Fatalf("%s status = %v", account, body)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("the two clients share an agent id: %v", ids)
	}
	if status, _, _ := call(t, "POST", srv.URL+"/api/microbot/client/start", ""); status != http.StatusConflict {
		t.Fatalf("no account with two clients = %d", status)
	}
}
