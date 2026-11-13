package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/routinesync"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

// The routines over the real store, wired as serve does (docs/11 section
// 4.4): a trigger saved while the agent is away reaches it when it links,
// the history comes from the database, and an agent from before F6 keeps
// its own.
func TestRoutineRoutesPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_farm', 'dev', 'Farm', 'farm', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	repo := storeroutines.New(pool)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	audit := &memAudit{}
	sync := &routinesync.Sync{Store: repo, Hub: link, Audit: audit, Log: quiet}
	link.OnConnect, link.OnEvent = sync.OnConnect, sync.OnEvent
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: link, Audit: audit, Routines: repo},
	}))
	t.Cleanup(srv.Close)
	api := srv.URL + "/api/microbot"
	trigger := `{"event":"kitten_grown","enabled":true,"run":{"className":"a.b.CatSwapPlugin","stop":["a.b.FisherPlugin"]}}`

	// The agent is away: the trigger is saved, and read back from the database.
	status, body, _ := call(t, "POST", api+"/triggers?account=acc_farm", trigger)
	if status != http.StatusOK || !strings.Contains(body["result"].(map[string]any)["watchError"].(string), "no agent") {
		t.Fatalf("save while away = %d %v", status, body)
	}
	if last := audit.last(); last.Action != agent.CmdTriggerSet || !last.OK || last.AccountID != "acc_farm" {
		t.Fatalf("audit = %+v", last)
	}
	status, body, _ = call(t, "GET", api+"/triggers?account=acc_farm", "")
	if status != http.StatusOK || len(body["triggers"].([]any)) != 1 {
		t.Fatalf("triggers while away = %d %v", status, body)
	}
	if status, _, _ = call(t, "POST", api+"/triggers?account=acc_farm", `{"event":"level_up","run":{"className":"a.B"}}`); status != http.StatusBadRequest {
		t.Fatalf("bad trigger = %d", status)
	}

	// The agent links and gets it; the watcher's state comes from the agent.
	startAgent(t, srv.URL, "", agent.Hello{Host: "zaros", Account: "acc_farm"}, "A")
	waitLinked(t, link, 1)
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, body, _ = call(t, "GET", api+"/triggers?account=acc_farm", "")
		if ts, _ := body["triggers"].([]any); status == http.StatusOK && len(ts) == 1 && ts[0].(map[string]any)["enabled"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the agent never got the trigger: %d %v", status, body)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The history comes from the database.
	now := time.Now().UTC()
	run := agent.Run{ID: "run-1", UID: "0123456789abcdef", ClassName: "a.b.CatSwapPlugin", Trigger: agent.EventKittenGrown,
		State: agent.RunDone, StartedAt: now, FinishedAt: &now, Events: []agent.RunEvent{{At: now, State: agent.RunDone, Text: "pronto"}}}
	if _, _, err := repo.RecordRun(ctx, "acc_farm", "zaros", run); err != nil {
		t.Fatal(err)
	}
	status, body, _ = call(t, "GET", api+"/runs?account=acc_farm", "")
	runs, _ := body["runs"].([]any)
	if status != http.StatusOK || len(runs) != 1 || runs[0].(map[string]any)["uid"] != run.UID {
		t.Fatalf("runs = %d %v", status, body)
	}

	// An agent from before F6 keeps its own history.
	oldAgent(t, srv.URL, "acc_old")
	waitLinked(t, link, 2)
	status, body, _ = call(t, "GET", api+"/runs?account=acc_old", "")
	if runs, _ = body["runs"].([]any); status != http.StatusOK || len(runs) != 1 || runs[0].(map[string]any)["id"] != "run-9" {
		t.Fatalf("old agent's runs = %d %v", status, body)
	}
}

// oldAgent links a bare agent that declares the protocol from before F6
// (no routines_sync) and answers run_history with one run of its own.
func oldAgent(t *testing.T, url, account string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, agent.LinkURL(url), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	hello, _ := json.Marshal(agent.Message{Type: agent.TypeHello, Hello: &agent.Hello{
		Host: "old", Account: account, Commands: []string{agent.CmdStatus, agent.CmdRunHistory, agent.CmdTriggerGet},
	}})
	if err := conn.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var m agent.Message
			if json.Unmarshal(raw, &m) != nil {
				continue
			}
			res := agent.Message{Type: agent.TypeResult, ID: m.ID, OK: true}
			if m.Command == agent.CmdRunHistory {
				res.Data = json.RawMessage(`{"runs":[{"id":"run-9","className":"a.B","state":"done","startedAt":"2026-11-13T10:00:00Z","events":[]}]}`)
			} else {
				res.OK, res.Error = false, "not here"
			}
			b, _ := json.Marshal(res)
			_ = conn.Write(ctx, websocket.MessageText, b)
		}
	}()
}
