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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/microbot"
	"github.com/FelipeMalacarne/chronicle/internal/rununtil"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// questingGame is a logged-in character in a client with the Quest Helper
// the pinned Microbot client ships, played as its Agent Server answers
// (QuestHelperHandler): the helper's bot works on the selected quest until
// it is stopped; it never says the quest is done.
type questingGame struct {
	standingGame
	mu       sync.Mutex
	missing  bool // an Agent Server without /quest-helper
	listed   []string
	selected string
	posts    []string
}

func (g *questingGame) JSON(ctx context.Context, method, path string, q url.Values, body any, d time.Duration) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.missing && strings.HasPrefix(path, "/quest-helper") {
		return nil, &microbot.APIError{Path: path, Status: http.StatusNotFound, Message: "No context found for request"}
	}
	m, _ := body.(map[string]any)
	switch method + " " + path {
	case "GET /quest-helper/status":
		return json.Marshal(map[string]any{"active": true, "enabled": true, "selected": g.selected != "", "questName": g.selected, "completed": false, "stepText": "Talk to Fred the Farmer."})
	case "GET /quest-helper/list":
		var qs []map[string]any
		for _, n := range g.listed {
			qs = append(qs, map[string]any{"enum": strings.ToUpper(n), "displayName": n})
		}
		return json.Marshal(map[string]any{"count": len(qs), "quests": qs})
	case "POST /quest-helper/start":
		g.selected, _ = m["name"].(string)
		g.posts = append(g.posts, "start "+g.selected)
		return json.Marshal(map[string]any{"success": true, "questName": g.selected})
	case "POST /quest-helper/stop":
		g.selected = ""
		g.posts = append(g.posts, "stop")
		return json.RawMessage(`{"success":true}`), nil
	case "POST /settings/plugin":
		v, _ := m["value"].(*string)
		g.posts = append(g.posts, "set "+m["group"].(string)+"."+m["key"].(string)+"="+*v)
		return json.Marshal(map[string]any{"group": m["group"], "key": m["key"], "value": *v, "success": true})
	case "GET /scripts":
		return json.RawMessage(`{"scripts":[]}`), nil
	}
	if strings.HasPrefix(path, "/quest-helper") {
		return nil, &microbot.APIError{Path: path, Status: http.StatusNotFound, Message: "Unknown endpoint"}
	}
	return g.standingGame.JSON(ctx, method, path, q, body, d)
}

func (g *questingGame) set(f func(*questingGame)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	f(g)
}

func (g *questingGame) state() (string, []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.selected, slices.Clone(g.posts)
}

// TestQuestTaskPostgres: the plan's next task is a quest delegated to the
// client's Quest Helper (docs/10, E19). POST /api/microbot/task refuses,
// before anything reaches the game, an agent before quests, a client without
// the helper's endpoints and a quest the helper does not list; then the run
// sets "Obtain Missing Items" to No and selects the quest; the ingest with
// the quest in progress changes nothing, with it finished the server tells
// the agent, which stops the helper; GET /api/microbot/task proves it.
func TestQuestTaskPostgres(t *testing.T) {
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
		  ('acc_quest', 'dev', 'Quest', 'quest', 'ironman', 'oziris-main', 'enhanced'),
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
	game := &questingGame{missing: true, listed: []string{"Cook's Assistant", "X Marks the Spot"}}
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_quest"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: game, Bridge: &doneBridge{}, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(wctx) }()
	bareAgent(t, srv.URL, "acc_old", agent.CmdQuestHelper)
	waitLinked(t, link, 2)

	quest := `{"type":"quest","quest":"quest:sheep_shearer"}`
	for _, account := range []string{"acc_quest", "acc_old"} {
		status, body, _ := call(t, "POST", srv.URL+"/api/plans?account="+account, `{"kind":"custom","title":"Quest","items":[{"kind":"task","task":`+quest+`}]}`)
		if status != http.StatusCreated {
			t.Fatalf("plan = %d %v", status, body)
		}
		if status, _, _ := call(t, "PUT", srv.URL+"/api/plans/active?account="+account, `{"plan":"`+body["id"].(string)+`"}`); status != http.StatusOK {
			t.Fatalf("activate = %d", status)
		}
	}

	token, err := ingest.CreateToken(ctx, "acc_quest", "quest", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	pushQuest := func(seq int64, state string) {
		t.Helper()
		push := contractPush(manifest.ManifestVersion)
		push.ClientSeq = seq
		push.SentAt = time.Now().Add(time.Duration(seq-10) * time.Second).UTC().Truncate(time.Second)
		quests := coreingest.QuestMap{"Cook's Assistant": coreingest.QuestFinished, "Sheep Shearer": state}
		push.Data.Quests = &quests
		raw, _ := json.Marshal(push)
		if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); aerr != nil || !resp.OK {
			t.Fatalf("push = %+v, %v", resp, aerr)
		}
	}
	pushQuest(1, coreingest.QuestInProgress)

	// The plan shows the quest as executable, a death risk; the refusals
	// reach no game.
	_, body, _ := call(t, "GET", srv.URL+"/api/plan/next?account=acc_quest", "")
	task, _ := body["task"].(map[string]any)
	if task == nil || task["type"] != "quest" || task["executable"] != true || task["state"] != "unmet" || !strings.Contains(task["until"].(string), "Sheep Shearer") {
		t.Fatalf("next = %v", body)
	}
	if risks, _ := task["risks"].([]any); len(risks) != 2 || risks[0] != "combat" || risks[1] != "hazard" {
		t.Fatalf("risks = %v", task["risks"])
	}
	refused := func(account, want string) {
		t.Helper()
		status, answer, _ := call(t, "POST", srv.URL+"/api/microbot/task?account="+account, `{"key":"task:0#0"}`)
		if status != http.StatusConflict || !strings.Contains(answer["error"].(string), want) {
			t.Fatalf("%s = %d %v, want %q", account, status, answer, want)
		}
		if e := audit.last(); e.OK || e.Action != agent.CmdTaskRun || e.AccountID != account {
			t.Fatalf("refusal audit = %+v", e)
		}
	}
	refused("acc_quest", "has no Quest Helper")
	game.set(func(g *questingGame) { g.missing = false })
	refused("acc_quest", "não lista Sheep Shearer")
	refused("acc_old", "anterior às quests")
	if selected, posts := game.state(); selected != "" || len(posts) > 0 {
		t.Fatalf("a refused quest acted: %v", posts)
	}

	// Listed: the run sets the bot to never fetch items, then selects it.
	game.set(func(g *questingGame) { g.listed = append(g.listed, "Sheep Shearer") })
	status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_quest", `{"key":"task:0#0","timeoutS":10800}`)
	if status != http.StatusOK {
		t.Fatalf("run = %d %v", status, body)
	}
	if e := audit.last(); !e.OK || !strings.Contains(string(e.Params), `"helper":"Sheep Shearer"`) || !strings.Contains(string(e.Params), `"risks":["combat","hazard"]`) {
		t.Fatalf("audit = %+v", e)
	}
	taskStatusOf := func() taskStatus {
		_, _, raw := call(t, "GET", srv.URL+"/api/microbot/task?account=acc_quest", "")
		var st taskStatus
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	deadline := time.Now().Add(5 * time.Second)
	for selected, _ := game.state(); selected != "Sheep Shearer"; selected, _ = game.state() {
		if time.Now().After(deadline) {
			t.Fatalf("the quest never started: %+v", taskStatusOf().Run)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, posts := game.state(); strings.Join(posts, "|") != "set questhelper.obtainMissingItems=NO|start Sheep Shearer" {
		t.Fatalf("actions = %v", posts)
	}
	pushQuest(2, coreingest.QuestInProgress)
	time.Sleep(50 * time.Millisecond)
	if st := taskStatusOf(); st.Run.Final() || st.Proven {
		t.Fatalf("in progress = %+v %+v", st.Run, st.Proof)
	}
	if selected, _ := game.state(); selected != "Sheep Shearer" {
		t.Fatal("the helper stopped before the proof")
	}

	pushQuest(3, coreingest.QuestFinished)
	var st taskStatus
	for deadline = time.Now().Add(5 * time.Second); ; {
		if st = taskStatusOf(); st.Run.Final() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the quest run never ended: %+v", st.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	selected, posts := game.state()
	if st.Run.State != agent.RunDone || st.Run.Task == nil || st.Run.Task.Type != "quest" || !st.Proven || st.Proof.State != "met" ||
		selected != "" || posts[len(posts)-1] != "stop" {
		t.Fatalf("after finished = %+v %+v, helper on %q, %v", st.Run, st.Proof, selected, posts)
	}
	if st.Run.Until == nil || st.Run.Until.Objective != "quest:sheep_shearer@finished" {
		t.Fatalf("until = %+v", st.Run.Until)
	}
	// Proven, the plan has nothing left to run.
	if status, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_quest", `{}`); status != http.StatusConflict || !strings.Contains(body["error"].(string), "done") {
		t.Fatalf("after the proof = %d %v", status, body)
	}
}
