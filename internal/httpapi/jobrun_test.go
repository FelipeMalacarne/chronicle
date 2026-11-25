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
	"github.com/FelipeMalacarne/chronicle/internal/core/clientcfg"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

const tutorialJob = `{"kind":"plugin","goal":{"type":"varp","id":281,"min":1000},
	"spec":{"type":"plugin","plugin":{"plugin":"` + clientcfg.TutorialIslandClass + `","config":[{"group":"MicroTutIsland","key":"toggleMusic","value":"false"}]}}}`

// A plugin job is created with the plugin and its settings, started on the
// account's agent by POST /api/jobs/{id}/run, shows the varp 281 progress as
// the ingest brings it and ends when it proves 1000, with the plugin off.
func TestPluginJobPostgres(t *testing.T) {
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
		  ('acc_island', 'dev', 'Island', 'island', 'regular', 'oziris-main', 'enhanced'),
		  ('acc_old', 'dev', 'Old', 'old', 'regular', 'oziris-main', 'enhanced'),
		  ('acc_hc', 'dev', 'Hc', 'hc', 'hcim', 'oziris-main', 'enhanced');
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
	jw := startJobs(t, pool, progress, catalog, link, audit, hub, quiet)
	wctx, wcancel := context.WithCancel(ctx)
	t.Cleanup(wcancel)
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, Progress: progress, Holdings: ingest, Catalog: catalog, DevMode: true, Logger: quiet,
		Jobs: jw.Jobs, JobEvents: jw.Events, Microbot: &MicrobotOptions{Link: link, Audit: audit, Routines: jw.Routines},
	}))
	t.Cleanup(srv.Close)
	game := newTrainingGame()
	game.active[clientcfg.TutorialIslandClass] = false // installed, off
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_island"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: game, Bridge: &doneBridge{}, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(wctx) }()
	bareAgent(t, srv.URL, "acc_old") // declares no features: it predates plugin jobs
	waitLinked(t, link, 2)

	// The plugin and its settings are validated when the job is created.
	for name, body := range map[string]string{
		"a setting the plugin lacks": strings.Replace(tutorialJob, "toggleMusic", "ironman", 1),
		"no plugin task":             `{"kind":"plugin","goal":{"type":"varp","id":281,"min":1000}}`,
		"a goal the agent cannot wait for": strings.Replace(tutorialJob, `{"type":"varp","id":281,"min":1000}`,
			`{"type":"varp","id":281,"eq":1000}`, 1),
		"an until that is not the goal's": strings.Replace(tutorialJob, `"plugin":{`, `"plugin":{"until":"varp:281>=999",`, 1),
	} {
		if status, _, _ := call(t, "POST", srv.URL+"/api/jobs?account=acc_island", body); status != http.StatusBadRequest {
			t.Errorf("%s = %d", name, status)
		}
	}
	status, created, _ := call(t, "POST", srv.URL+"/api/jobs?account=acc_island", tutorialJob)
	if status != http.StatusCreated || created["state"] != "queued" {
		t.Fatalf("create = %d %v", status, created)
	}
	id := created["id"].(string)
	spec := created["spec"].(map[string]any)["plugin"].(map[string]any)
	if spec["until"] != "varp:281>=1000" || spec["name"] != "TutorialIslandPlugin" {
		t.Fatalf("the spec is completed from the goal: %v", spec)
	}

	// An agent without the feature is refused clearly, and nothing is dispatched.
	_, oldJob, _ := call(t, "POST", srv.URL+"/api/jobs?account=acc_old", tutorialJob)
	status, refused, _ := call(t, "POST", srv.URL+"/api/jobs/"+oldJob["id"].(string)+"/run?account=acc_old", "")
	if status != http.StatusConflict || !strings.Contains(refused["error"].(string), "plugin_task") {
		t.Fatalf("old agent = %d %v", status, refused)
	}
	if e := audit.last(); e.OK || e.Action != agent.CmdTaskRun || e.AccountID != "acc_old" {
		t.Fatalf("refusal audit = %+v", e)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/jobs/"+id+"/run?account=acc_old", ""); status != http.StatusNotFound {
		t.Fatalf("another account's job = %d", status)
	}

	// Starting or canceling a job acts in game: Microbot control, which in
	// dev answers only this machine.
	for _, action := range []string{"run", "cancel"} {
		req := httptest.NewRequest("POST", "/api/jobs/"+id+"/"+action+"?account=acc_island", nil)
		req.RemoteAddr = "192.168.1.50:40000"
		rec := httptest.NewRecorder()
		srv.Config.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s from the LAN = %d", action, rec.Code)
		}
	}

	// A plugin's risks are unknown: an HCIM runs one only supervised (E9).
	_, hcJob, _ := call(t, "POST", srv.URL+"/api/jobs?account=acc_hc", tutorialJob)
	status, refused, _ = call(t, "POST", srv.URL+"/api/jobs/"+hcJob["id"].(string)+"/run?account=acc_hc", "")
	if status != http.StatusConflict || !strings.Contains(refused["error"].(string), "supervised") {
		t.Fatalf("unsupervised HCIM = %d %v", status, refused)
	}

	token, err := ingest.CreateToken(ctx, "acc_island", "island", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	pushVarp := func(seq int64, value int64) {
		t.Helper()
		push := contractPush(manifest.ManifestVersion)
		push.ClientSeq = seq
		push.SentAt = time.Now().Add(time.Duration(seq-10) * time.Second).UTC().Truncate(time.Second)
		varps := coreingest.IntMap{"281": value}
		push.Data.Varps = &varps
		raw, _ := json.Marshal(push)
		if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); aerr != nil || !resp.OK {
			t.Fatalf("push = %+v, %v", resp, aerr)
		}
	}
	jobState := func() (state string, current float64) {
		_, body, _ := call(t, "GET", srv.URL+"/api/jobs/"+id+"?account=acc_island", "")
		j := body["job"].(map[string]any)
		if p, ok := j["progress"].(map[string]any); ok {
			current, _ = p["current"].(float64)
		}
		return j["state"].(string), current
	}
	pushVarp(1, 20)

	// The job starts at the varp it finds, with the plugin on and set.
	status, ran, _ := call(t, "POST", srv.URL+"/api/jobs/"+id+"/run?account=acc_island", "")
	if status != http.StatusOK || ran["job"] != id {
		t.Fatalf("run = %d %v", status, ran)
	}
	if e := audit.last(); !e.OK || e.Action != agent.CmdTaskRun || !strings.Contains(string(e.Params), `"jobId":"`+id+`"`) {
		t.Fatalf("audit = %+v", e)
	}
	eventually(t, "the job running on the plugin", func() bool {
		state, _ := jobState()
		return state == "running" && game.on(clientcfg.TutorialIslandClass)
	})
	game.mu.Lock()
	posts := strings.Join(game.posts, "|")
	game.mu.Unlock()
	if posts != "set MicroTutIsland.toggleMusic=false|start "+clientcfg.TutorialIslandClass {
		t.Fatalf("actions = %s", posts)
	}
	if status, _, _ = call(t, "POST", srv.URL+"/api/jobs/"+id+"/run?account=acc_island", ""); status != http.StatusConflict {
		t.Fatalf("a second run of a running job = %d", status)
	}

	pushVarp(2, 640)
	eventually(t, "progress 640", func() bool { _, cur := jobState(); return cur == 640 })
	if !game.on(clientcfg.TutorialIslandClass) {
		t.Fatal("the plugin stopped before the proof")
	}

	pushVarp(3, 1000)
	eventually(t, "the job succeeded", func() bool { s, _ := jobState(); return s == "succeeded" })
	eventually(t, "the plugin off", func() bool { return !game.on(clientcfg.TutorialIslandClass) })
	_, body, _ := call(t, "GET", srv.URL+"/api/jobs/"+id+"?account=acc_island", "")
	runs := body["runs"].([]any)
	if _, cur := jobState(); cur != 1000 || len(runs) != 1 {
		t.Fatalf("progress %v, runs %v", cur, runs)
	}
	eventually(t, "the run done", func() bool {
		_, b, _ := call(t, "GET", srv.URL+"/api/jobs/"+id+"?account=acc_island", "")
		return b["runs"].([]any)[0].(map[string]any)["state"] == agent.RunDone
	})
	if status, _, _ = call(t, "POST", srv.URL+"/api/jobs/"+id+"/run?account=acc_island", ""); status != http.StatusConflict {
		t.Fatalf("running a succeeded job = %d", status)
	}
}
