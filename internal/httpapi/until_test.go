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

	"github.com/coder/websocket"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/rununtil"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

const untilTrainer = "a.b.FishingTrainerPlugin"

// trainingGame is a logged-in character with a plugin that trains until it
// is stopped: it never switches itself off.
type trainingGame struct {
	standingGame
	mu     sync.Mutex
	active map[string]bool
	posts  []string
}

func newTrainingGame() *trainingGame { return &trainingGame{active: map[string]bool{}} }

func (g *trainingGame) JSON(ctx context.Context, method, path string, q url.Values, body any, d time.Duration) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	class := ""
	if m, ok := body.(map[string]any); ok {
		class, _ = m["className"].(string)
	}
	switch method + " " + path {
	case "POST /scripts/start":
		g.active[class] = true
		g.posts = append(g.posts, "start "+class)
	case "POST /scripts/stop":
		g.active[class] = false
		g.posts = append(g.posts, "stop "+class)
	case "POST /settings/plugin":
		m := body.(map[string]any)
		g.posts = append(g.posts, "set "+m["group"].(string)+"."+m["key"].(string)+"="+m["value"].(string))
	case "GET /scripts":
		var s []map[string]any
		for c, a := range g.active {
			s = append(s, map[string]any{"className": c, "active": a})
		}
		return json.Marshal(map[string]any{"scripts": s})
	}
	return g.standingGame.JSON(ctx, method, path, q, body, d)
}

func (g *trainingGame) on(class string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active[class]
}

// bareAgent links an agent that answers every command OK, with the closed
// list minus drop in its hello (an agent from before them).
func bareAgent(t *testing.T, url, account string, drop ...string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, agent.LinkURL(url), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	commands := slices.DeleteFunc(slices.Clone(agent.Commands), func(c string) bool { return slices.Contains(drop, c) })
	hello, _ := json.Marshal(agent.Message{Type: agent.TypeHello, Hello: &agent.Hello{Host: "old", Account: account, Commands: commands}})
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
			b, _ := json.Marshal(agent.Message{Type: agent.TypeResult, ID: m.ID, OK: true, Data: json.RawMessage(`{}`)})
			_ = conn.Write(ctx, websocket.MessageText, b)
		}
	}()
}

// TestRunUntilRoutes: the server writes the account into a run's
// wait_until, refuses a key without an account or unknown to the catalog,
// and an agent that cannot end a run on an objective.
func TestRunUntilRoutes(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	audit := &memAudit{}
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet, Catalog: catalog,
		Microbot: &MicrobotOptions{Link: link, Audit: audit},
	}))
	t.Cleanup(srv.Close)
	game := newTrainingGame()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: game, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(ctx) }()
	waitLinked(t, link, 1)

	run := `{"className":"` + untilTrainer + `","stay":true,"until":{"objective":"%s","account":"acc_someone_else"}}`
	for _, c := range []struct{ query, objective, want string }{
		{"", "skill:fishing>=76", "account"},
		{"?account=acc_zezima", "quest:no_such_quest@finished", "unknown"},
		{"?account=acc_zezima", "skill:fishing>=100", "skills take"},
	} {
		status, body, _ := call(t, "POST", srv.URL+"/api/microbot/run"+c.query, strings.Replace(run, "%s", c.objective, 1))
		if status != http.StatusBadRequest || !strings.Contains(body["error"].(string), c.want) {
			t.Fatalf("%s %s = %d %v", c.query, c.objective, status, body)
		}
	}
	if len(audit.entries) != 0 {
		t.Fatalf("refusals audited: %+v", audit.entries)
	}
	status, body, _ := call(t, "POST", srv.URL+"/api/microbot/run?account=acc_zezima", strings.Replace(run, "%s", "skill:fishing>=76", 1))
	if status != http.StatusOK {
		t.Fatalf("run = %d %v", status, body)
	}
	if e := audit.last(); !e.OK || !strings.Contains(string(e.Params), `"account":"acc_zezima"`) {
		t.Fatalf("audit = %+v", e)
	}
	_, _, raw := call(t, "GET", srv.URL+"/api/microbot/run?account=acc_zezima", "")
	var st agent.RunStatus
	if err := json.Unmarshal(raw, &st); err != nil || st.Run == nil || st.Run.Until == nil || st.Run.Until.Account != "acc_zezima" {
		t.Fatalf("the agent's run = %s", raw)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/run/cancel?account=acc_zezima", ""); status != http.StatusOK {
		t.Fatalf("cancel = %d", status)
	}

	// An agent from before run_until: 409, the run never reaches it.
	bareAgent(t, srv.URL, "acc_old", agent.CmdRunUntil)
	waitLinked(t, link, 2)
	before := len(audit.entries)
	status, body, _ = call(t, "POST", srv.URL+"/api/microbot/run?account=acc_old", strings.Replace(run, "%s", "skill:fishing>=76", 1))
	if status != http.StatusConflict || !strings.Contains(body["error"].(string), "predates wait_until") {
		t.Fatalf("old agent = %d %v", status, body)
	}
	if e := audit.entries[before]; e.OK || e.AccountID != "acc_old" {
		t.Fatalf("old agent audit = %+v", e)
	}
	// A plain run still goes to it.
	if status, _, _ = call(t, "POST", srv.URL+"/api/microbot/run?account=acc_old", `{"className":"`+untilTrainer+`"}`); status != http.StatusOK {
		t.Fatalf("plain run on the old agent = %d", status)
	}
}

// TestRunUntilPostgres: a run waits for skill:fishing>=76; the ingest's
// level 75 leaves it running, 76 makes the server tell the agent, which
// stops the plugin and ends the run done. Nobody else acts.
func TestRunUntilPostgres(t *testing.T) {
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
		  ('acc_until', 'dev', 'Until', 'until', 'ironman', 'oziris-main', 'enhanced');
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
		Microbot: &MicrobotOptions{Link: link, Audit: audit},
	}))
	t.Cleanup(srv.Close)
	game := newTrainingGame()
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_until"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: game, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(wctx) }()
	waitLinked(t, link, 1)

	token, err := ingest.CreateToken(ctx, "acc_until", "until", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
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
	pushLevel(1, 75)

	status, body, _ := call(t, "POST", srv.URL+"/api/microbot/run?account=acc_until",
		`{"className":"`+untilTrainer+`","stay":true,"until":{"objective":"skill:fishing>=76"}}`)
	if status != http.StatusOK {
		t.Fatalf("run = %d %v", status, body)
	}
	runStatus := func() *agent.Run {
		_, _, raw := call(t, "GET", srv.URL+"/api/microbot/run?account=acc_until", "")
		var st agent.RunStatus
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}
		return st.Run
	}
	deadline := time.Now().Add(5 * time.Second)
	for !game.on(untilTrainer) {
		if time.Now().After(deadline) {
			t.Fatalf("the trainer never started: %+v", runStatus())
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if r := runStatus(); r.Final() || !game.on(untilTrainer) {
		t.Fatalf("at level 75 = %+v", r)
	}

	pushLevel(2, 76)
	var r *agent.Run
	for deadline = time.Now().Add(5 * time.Second); ; {
		if r = runStatus(); r.Final() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run never ended: %+v", r)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.State != agent.RunDone || game.on(untilTrainer) {
		t.Fatalf("after level 76 = %+v, trainer on %v", r, game.on(untilTrainer))
	}
	proof := false
	for _, ev := range r.Events {
		proof = proof || strings.Contains(ev.Text, "provado pelo ingest: skill:fishing>=76 (nível 76)")
	}
	if !proof {
		t.Fatalf("trail = %+v", r.Events)
	}
	var sent bool
	audit.mu.Lock()
	for _, e := range audit.entries {
		sent = sent || (e.Action == agent.CmdRunUntil && e.Actor == rununtil.Actor && e.Origin == "server" && e.OK && e.AccountID == "acc_until")
	}
	audit.mu.Unlock()
	if !sent {
		t.Fatalf("audit = %+v", audit.entries)
	}
	for deadline = time.Now().Add(5 * time.Second); len(watcher.Watching()) != 0; {
		if time.Now().After(deadline) {
			t.Fatalf("still watching %v", watcher.Watching())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
