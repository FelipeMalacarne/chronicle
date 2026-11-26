package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/conductor"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	storeconductor "github.com/FelipeMalacarne/chronicle/internal/store/conductor"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// The owner turns the conductor on and off over the API, and DispatchNext
// sends the plan's next task the way POST /api/microbot/task does, audited as
// the conductor, with its job of the conductor's source; what the owner must
// act on is Refused, an absent agent is Wait.
func TestConductorRoutesAndDispatchPostgres(t *testing.T) {
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
		  ('acc_cond', 'dev', 'Cond', 'cond', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_hc', 'dev', 'Hc', 'hc', 'hcim', 'oziris-main', 'enhanced'),
		  ('acc_far', 'dev', 'Far', 'far', 'ironman', 'oziris-main', 'enhanced');
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
	jobs := storejobs.New(pool)
	api := New(Options{
		Version: "test", Content: cnt, Progress: progress, Holdings: ingest, Catalog: catalog, DevMode: true, Logger: quiet,
		Plans: storeplans.New(pool, cnt, catalog), Microbot: &MicrobotOptions{Link: link, Audit: audit},
		Jobs: jobs, Conductor: storeconductor.New(pool),
	})
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	actx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_cond"}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: standingGame{}, Bridge: &doneBridge{}, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
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
	follow("acc_cond", `{"kind":"task","task":{"type":"travel","location":"location:lumbridge_castle"}}`)
	follow("acc_hc", `{"kind":"task","task":{"type":"travel","location":"location:mage_bank"}}`)
	follow("acc_far", `{"kind":"task","task":{"type":"travel","location":"location:lumbridge_castle"}}`)

	// On and off.
	if status, body, _ := call(t, "GET", srv.URL+"/api/conductor?account=acc_cond", ""); status != http.StatusOK || body["state"] != "off" {
		t.Fatalf("before = %d %v", status, body)
	}
	if status, body, _ := call(t, "POST", srv.URL+"/api/conductor?account=acc_cond", `{"on":true}`); status != http.StatusOK || body["state"] != "idle" {
		t.Fatalf("on = %d %v", status, body)
	}
	if status, _, _ := call(t, "POST", srv.URL+"/api/conductor?account=acc_cond", `{}`); status != http.StatusBadRequest {
		t.Fatalf("without on = %d", status)
	}
	if status, body, _ := call(t, "POST", srv.URL+"/api/conductor?account=acc_cond", `{"on":false}`); status != http.StatusOK || body["state"] != "off" {
		t.Fatalf("off = %d %v", status, body)
	}

	// The dispatch: the agent takes it, as the conductor.
	out := api.DispatchNext(ctx, "acc_cond")
	if out.Kind != conductor.Dispatched || out.Job == "" {
		t.Fatalf("dispatch = %+v", out)
	}
	if e := audit.last(); e.Action != agent.CmdTaskRun || !e.OK || e.Actor != ConductorActor || e.Origin != "server" {
		t.Fatalf("audit = %+v", e)
	}
	if j, err := jobs.Get(ctx, "acc_cond", out.Job); err != nil || j.Source != job.SourceConductor || j.TaskKey != "task:0#0" {
		t.Fatalf("job = %+v %v", j, err)
	}

	// E9 is the owner's to act on; an absent agent may come back.
	if out := api.DispatchNext(ctx, "acc_hc"); out.Kind != conductor.Refused || !strings.Contains(out.Reason, "E9") {
		t.Fatalf("hcim alone in the wilderness = %+v", out)
	}
	if out := api.DispatchNext(ctx, "acc_far"); out.Kind != conductor.Wait || !strings.Contains(out.Reason, "nenhum agent") {
		t.Fatalf("no agent = %+v", out)
	}
}
