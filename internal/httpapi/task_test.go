package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// standingGame is the Agent Server of a logged-in character standing in
// Draynor, out of combat and dialogue.
type standingGame struct{ stubGame }

func (standingGame) State(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"loggedIn":true,"player":{"position":{"x":3103,"y":3265,"plane":0}}}`), nil
}

func (standingGame) JSON(_ context.Context, _, path string, _ url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	if path == "/dialogue" {
		return json.RawMessage(`{"inDialogue":false}`), nil
	}
	return json.RawMessage(`{}`), nil
}

// doneBridge is Chronicle Link's bridge whose machine ends right after the
// start, claiming success: the plugin's word, which proves nothing.
type doneBridge struct {
	mu    sync.Mutex
	id    string
	start map[string]any
}

func (b *doneBridge) JSON(_ context.Context, method, path string, _ url.Values, body any, _ time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch method + " " + path {
	case "GET /bridge/version":
		return json.RawMessage(`{"version":"1.15.0"}`), nil
	case "POST /bridge/task/start":
		raw, _ := json.Marshal(body)
		_ = json.Unmarshal(raw, &b.start)
		b.id, _ = b.start["id"].(string)
		return json.RawMessage(`{"success":true}`), nil
	case "GET /bridge/task":
		return json.Marshal(map[string]any{"id": b.id, "type": "travel", "state": "done", "events": []map[string]any{{"seq": 1, "text": "cheguei, diz o plugin"}}})
	}
	return json.RawMessage(`{}`), nil
}

// TestTaskRoutesPostgres: the plan's next task runs on the account's agent,
// audited; the run's "done" is not the proof, the ingest's position is.
func TestTaskRoutesPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	cnt, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev') ON CONFLICT DO NOTHING;
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_task', 'dev', 'Task', 'task', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_hc', 'dev', 'Hc', 'hc', 'hcim', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, cnt, catalog)
	if _, err = progress.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).WithReevaluator(progress)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	audit := &memAudit{}
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, Progress: progress, Holdings: ingest, Catalog: catalog, DevMode: true, Logger: quiet,
		Plans: storeplans.New(pool, cnt, catalog), Microbot: &MicrobotOptions{Link: link, Audit: audit},
	}))
	t.Cleanup(srv.Close)
	bridge := &doneBridge{}
	actx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_task"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: standingGame{}, Bridge: bridge, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(actx) }()
	waitLinked(t, link, 1)

	follow := func(account, items string) {
		t.Helper()
		status, body, _ := call(t, "POST", srv.URL+"/api/plans?account="+account, `{"kind":"custom","title":"Logística","items":[`+items+`]}`)
		if status != http.StatusCreated {
			t.Fatalf("plan = %d %v", status, body)
		}
		if status, _, _ := call(t, "PUT", srv.URL+"/api/plans/active?account="+account, `{"plan":"`+body["id"].(string)+`"}`); status != http.StatusOK {
			t.Fatalf("activate = %d", status)
		}
	}
	follow("acc_task", `{"kind":"task","task":{"type":"travel","location":"location:lumbridge_castle"}}`)
	follow("acc_hc", `{"kind":"task","task":{"type":"travel","location":"location:mage_bank"}},
		{"kind":"task","task":{"type":"talk","npc":"npc:hans","until":{"objective":"quest:cooks_assistant@started"}}}`)

	// Refusals that never reach an agent.
	if status, _, _ := call(t, "POST", srv.URL+"/api/microbot/task", `{}`); status != http.StatusBadRequest {
		t.Fatalf("no account = %d", status)
	}
	if status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_task", `{"key":"task:9#0"}`); status != http.StatusConflict || !strings.Contains(body["error"].(string), "mudou") {
		t.Fatalf("stale key = %d %v", status, body)
	}
	if status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_hc", `{}`); status != http.StatusConflict || !strings.Contains(body["error"].(string), "E9") {
		t.Fatalf("hcim alone in the wilderness = %d %v", status, body)
	}
	if n := len(audit.entries); n != 0 {
		t.Fatalf("refusals audited: %d", n)
	}

	// The run: started on the agent, audited with the resolved task.
	status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_task", `{"key":"task:0#0"}`)
	if status != http.StatusOK || body["task"].(map[string]any)["key"] != "task:0#0" {
		t.Fatalf("run = %d %v", status, body)
	}
	if e := audit.last(); e.Action != agent.CmdTaskRun || !e.OK || e.AccountID != "acc_task" || !strings.Contains(string(e.Params), "location:lumbridge_castle") {
		t.Fatalf("audit = %+v", e)
	}
	var st taskStatus
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, raw := call(t, "GET", srv.URL+"/api/microbot/task?account=acc_task", "")
		if uerr := json.Unmarshal(raw, &st); uerr != nil {
			t.Fatal(uerr)
		}
		if st.Run != nil && st.Run.Final() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run never ended: %+v", st.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st.Run.State != agent.RunDone || st.Run.Task == nil || st.Proof == nil || st.Proven || st.Proof.State != "unknown" {
		t.Fatalf("after the plugin's done, before the ingest = %+v %+v", st.Run, st.Proof)
	}
	bridge.mu.Lock()
	travel, _ := bridge.start["task"].(map[string]any)["travel"].(map[string]any)
	bridge.mu.Unlock()
	if travel == nil || travel["location"] != "location:lumbridge_castle" || travel["wilderness"] != false {
		t.Fatalf("the bridge got %v", bridge.start)
	}

	// The ingest's position inside the castle proves it.
	token, err := ingest.CreateToken(ctx, "acc_task", "task", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	push := contractPush(manifest.ManifestVersion)
	push.SentAt = time.Now().UTC().Truncate(time.Second)
	push.Data.Position = &coreingest.Position{World: 330, X: 3222, Y: 3218}
	raw, _ := json.Marshal(push)
	if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); aerr != nil || !resp.OK {
		t.Fatalf("push = %+v, %v", resp, aerr)
	}
	_, _, raw = call(t, "GET", srv.URL+"/api/microbot/task?account=acc_task", "")
	if err := json.Unmarshal(raw, &st); err != nil || !st.Proven || st.Proof.State != "met" {
		t.Fatalf("after the ingest = %+v %v", st.Proof, err)
	}
	// Proven, the plan has nothing left to run.
	if status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_task", `{}`); status != http.StatusConflict || !strings.Contains(body["error"].(string), "done") {
		t.Fatalf("after the proof = %d %v", status, body)
	}
}
