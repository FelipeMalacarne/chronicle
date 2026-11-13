package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/rununtil"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// TestTrainTaskPostgres: the plan's next task is a train delegated to a
// plugin; POST /api/microbot/task checks the plugin is installed and starts
// it on the agent; the ingest's level 76 makes the server tell the agent,
// which stops the plugin; GET /api/microbot/task proves it. A training to
// skill:fishing>=76 with nobody acting in between (docs/10, phase 4).
func TestTrainTaskPostgres(t *testing.T) {
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
		  ('acc_train', 'dev', 'Train', 'train', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_old', 'dev', 'Old', 'old', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub(nil)
	progress := storeprogress.New(pool, cnt, catalog).WithPublisher(hub)
	if _, err = progress.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).WithReevaluator(progress).WithPublisher(hub)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	audit := &memAudit{}
	link := &agent.Link{Log: quiet}
	watcher := &rununtil.Watcher{Facts: progress, Registry: catalog, Hub: link, Events: hub, Audit: audit, Log: quiet, Interval: time.Hour}
	wctx, wcancel := context.WithCancel(ctx)
	t.Cleanup(wcancel)
	watcher.Start(wctx)
	link.OnEvent = watcher.OnEvent
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, Progress: progress, Holdings: ingest, Catalog: catalog, DevMode: true, Logger: quiet,
		Plans: storeplans.New(pool, cnt, catalog), Microbot: &MicrobotOptions{Link: link, Audit: audit},
	}))
	t.Cleanup(srv.Close)
	game := newTrainingGame()
	game.active[untilTrainer] = false // installed, off
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_train"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: game, Bridge: &doneBridge{}, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(wctx) }()
	bareAgent(t, srv.URL, "acc_old", agent.CmdRunUntil)
	waitLinked(t, link, 2)

	follow := func(account, task string) {
		t.Helper()
		status, body, _ := call(t, "POST", srv.URL+"/api/plans?account="+account, `{"kind":"custom","title":"Treino","items":[{"kind":"task","task":`+task+`}]}`)
		if status != http.StatusCreated {
			t.Fatalf("plan = %d %v", status, body)
		}
		if status, _, _ := call(t, "PUT", srv.URL+"/api/plans/active?account="+account, `{"plan":"`+body["id"].(string)+`"}`); status != http.StatusOK {
			t.Fatalf("activate = %d", status)
		}
	}
	train := func(plugin string) string {
		return `{"type":"train","skill":"skill:fishing","location":"location:lumbridge","plugin":"` + plugin +
			`","config":[{"group":"fishingtrainer","key":"spot","value":"SHRIMP"}],"until":{"objective":"skill:fishing>=76"}}`
	}
	follow("acc_train", train("a.b.NotInstalledPlugin"))
	follow("acc_old", train(untilTrainer))

	token, err := ingest.CreateToken(ctx, "acc_train", "train", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	pushLevel := func(seq int64, level int64) {
		t.Helper()
		push := contractPush(manifest.ManifestVersion)
		push.ClientSeq = seq
		push.SentAt = time.Now().Add(time.Duration(seq-10) * time.Second).UTC().Truncate(time.Second)
		(*push.Data.Levels)["fishing"] = level
		(*push.Data.XP)["fishing"] = 1_210_421 + (level-75)*126_000
		raw, _ := json.Marshal(push)
		if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); aerr != nil || !resp.OK {
			t.Fatalf("push = %+v, %v", resp, aerr)
		}
	}
	pushLevel(1, 74)

	// The plan shows the train as executable; the refusals reach no run.
	_, body, _ := call(t, "GET", srv.URL+"/api/plan/next?account=acc_train", "")
	if task, _ := body["task"].(map[string]any); task == nil || task["type"] != "train" || task["executable"] != true || task["state"] != "unmet" {
		t.Fatalf("next = %v", body)
	}
	if status, refused, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_train", `{"key":"task:0#0"}`); status != http.StatusConflict ||
		!strings.Contains(refused["error"].(string), "não está instalado") {
		t.Fatalf("missing plugin = %d %v", status, refused)
	}
	if status, refused, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_old", `{"key":"task:0#0"}`); status != http.StatusConflict ||
		!strings.Contains(refused["error"].(string), "predates wait_until") {
		t.Fatalf("old agent = %d %v", status, refused)
	}
	if e := audit.last(); e.OK || e.Action != agent.CmdTaskRun || e.AccountID != "acc_old" {
		t.Fatalf("refusal audit = %+v", e)
	}
	if game.on("a.b.NotInstalledPlugin") || game.on(untilTrainer) {
		t.Fatal("a refused train started a plugin")
	}

	// The installed one: the run starts the plugin with its settings.
	follow("acc_train", train(untilTrainer))
	status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_train", `{"key":"task:0#0","timeoutS":21600}`)
	if status != http.StatusOK {
		t.Fatalf("run = %d %v", status, body)
	}
	if e := audit.last(); !e.OK || !strings.Contains(string(e.Params), `"account":"acc_train"`) || !strings.Contains(string(e.Params), untilTrainer) {
		t.Fatalf("audit = %+v", e)
	}
	taskStatusOf := func() taskStatus {
		_, _, raw := call(t, "GET", srv.URL+"/api/microbot/task?account=acc_train", "")
		var st taskStatus
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	deadline := time.Now().Add(5 * time.Second)
	for !game.on(untilTrainer) {
		if time.Now().After(deadline) {
			t.Fatalf("the trainer never started: %+v", taskStatusOf().Run)
		}
		time.Sleep(5 * time.Millisecond)
	}
	game.mu.Lock()
	posts := strings.Join(game.posts, "|")
	game.mu.Unlock()
	if posts != "set fishingtrainer.spot=SHRIMP|start "+untilTrainer {
		t.Fatalf("actions = %s", posts)
	}
	pushLevel(2, 75)
	time.Sleep(50 * time.Millisecond)
	if st := taskStatusOf(); st.Run.Final() || st.Proven || !game.on(untilTrainer) {
		t.Fatalf("at 75 = %+v %+v", st.Run, st.Proof)
	}

	pushLevel(3, 76)
	var st taskStatus
	for deadline = time.Now().Add(5 * time.Second); ; {
		if st = taskStatusOf(); st.Run.Final() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the training never ended: %+v", st.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st.Run.State != agent.RunDone || st.Run.Task == nil || st.Run.Task.Type != "train" || !st.Proven || st.Proof.State != "met" || game.on(untilTrainer) {
		t.Fatalf("after 76 = %+v %+v, trainer on %v", st.Run, st.Proof, game.on(untilTrainer))
	}
	// Proven, the plan has nothing left to run.
	if status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_train", `{}`); status != http.StatusConflict || !strings.Contains(body["error"].(string), "done") {
		t.Fatalf("after the proof = %d %v", status, body)
	}
}
