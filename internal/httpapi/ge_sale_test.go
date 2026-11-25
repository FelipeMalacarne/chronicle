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
	"github.com/FelipeMalacarne/chronicle/internal/core/ge"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// memPrices is PriceStore in memory.
type memPrices map[int]ge.Price

func (m memPrices) Get(_ context.Context, ids ...int) (map[int]ge.Price, error) {
	out := map[int]ge.Price{}
	for _, id := range ids {
		if p, ok := m[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

// geStandingGame is a character standing at the Grand Exchange whose
// offers fill at once.
type geStandingGame struct {
	standingGame
	mu     sync.Mutex
	offers []map[string]any
}

func (g *geStandingGame) JSON(ctx context.Context, method, path string, q url.Values, body any, d time.Duration) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/ge") {
		return g.standingGame.JSON(ctx, method, path, q, body, d)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	m, _ := body.(map[string]any)
	switch method + " " + path {
	case "POST /ge/sell":
		g.offers = append(g.offers, map[string]any{"slot": 1, "item": m["name"], "itemId": 1775, "state": "SOLD", "done": m["quantity"], "total": m["quantity"]})
	case "POST /ge/collect":
		g.offers = nil
	}
	return json.Marshal(map[string]any{"open": true, "offers": g.offers, "success": true})
}

// TestGESaleProofPostgres: a sale is never met by the item missing from the
// inventory; its job is proven by the tracker only once the ingest shows the
// item gone and the run's coins in the inventory, and that proof, the job
// succeeded, keeps the sale met after the coins are spent.
func TestGESaleProofPostgres(t *testing.T) { geSaleProof(t, nil) }

// An agent that predates the job_id feature refuses the job id in task_run, so
// the server does not send it, and links the run it reports to the job it
// queued for the task.
func TestGESaleProofOldAgentPostgres(t *testing.T) {
	geSaleProof(t, []string{agent.AgentFeatureRecurringRetry})
}

func geSaleProof(t *testing.T, features []string) {
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
		  ('acc_ge', 'dev', 'Ge', 'ge', 'regular', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, cnt, catalog)
	if _, err = progress.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub(nil)
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).WithReevaluator(progress).WithPublisher(hub)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	audit := &memAudit{}
	jw := startJobs(t, pool, progress, catalog, link, audit, hub, quiet)
	now := time.Now()
	limit := 10_000
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, Progress: progress, Holdings: ingest, Catalog: catalog, DevMode: true, Logger: quiet,
		Plans: storeplans.New(pool, cnt, catalog), Jobs: jw.Jobs, JobEvents: jw.Events,
		Microbot: &MicrobotOptions{Link: link, Audit: audit, Routines: jw.Routines},
		Prices:   memPrices{1775: {ItemID: 1775, High: 140, HighAt: now, Low: 130, LowAt: now, Limit: &limit, Source: ge.SourceWiki, FetchedAt: now}},
	}))
	t.Cleanup(srv.Close)
	actx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a := &agent.Agent{URL: srv.URL, Log: quiet, Hello: agent.Hello{Host: "zaros", Account: "acc_ge", Features: features}, VNCAddr: "127.0.0.1:1",
		Executor: &agent.Executor{Game: &geStandingGame{}, Bridge: &doneBridge{}, Home: t.TempDir(), RunPoll: 5 * time.Millisecond}}
	go func() { _ = a.Run(actx) }()
	waitLinked(t, link, 1)

	token, err := ingest.CreateToken(ctx, "acc_ge", "ge", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	push := contractPush(manifest.ManifestVersion)
	seq, revision := push.ClientSeq, int64(1)
	held := *push.Data.Items.Inventory.Quantities
	// inventory pushes the live inventory as a delta of the same live
	// epoch: what left it goes as a tombstone (0).
	inventory := func(now coreingest.QuantityMap) {
		t.Helper()
		q := coreingest.QuantityMap{}
		for id := range held {
			q[id] = 0
		}
		for id, n := range now {
			q[id] = n
		}
		held = now
		seq++
		revision++
		at := time.Now().UTC().Truncate(time.Millisecond)
		rev := revision
		p := coreingest.Push{
			ContractVersion: push.ContractVersion, Username: push.Username, Profile: push.Profile,
			ManifestVersion: push.ManifestVersion, ClientSeq: seq, Kind: coreingest.PushDelta, SentAt: at,
			Data: coreingest.PushData{Items: &coreingest.ItemContainers{Inventory: &coreingest.ItemObservation{
				State: coreingest.ItemLive, ObservedAt: at, Revision: &rev, LiveEpoch: "login-1", Quantities: &q,
			}}},
		}
		raw, _ := json.Marshal(p)
		if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, p); aerr != nil || !resp.OK {
			t.Fatalf("push = %+v, %v", resp, aerr)
		}
	}
	raw, _ := json.Marshal(push)
	if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); aerr != nil || !resp.OK {
		t.Fatalf("baseline push = %+v, %v", resp, aerr)
	}
	next := func() (string, string, string) {
		t.Helper()
		_, body, _ := call(t, "GET", srv.URL+"/api/plan/next?account=acc_ge", "")
		task, _ := body["task"].(map[string]any)
		if task == nil {
			return body["status"].(string), "", ""
		}
		reason, _ := task["reason"].(string)
		return body["status"].(string), task["state"].(string), reason
	}
	status := func() taskStatus {
		t.Helper()
		var st taskStatus
		_, _, raw := call(t, "GET", srv.URL+"/api/microbot/task?account=acc_ge", "")
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}
		return st
	}

	status1, body, _ := call(t, "POST", srv.URL+"/api/plans?account=acc_ge", `{"kind":"custom","title":"Vender","items":[{"kind":"task","task":{"type":"ge_sell","item":"item:1775"}}]}`)
	if status1 != http.StatusCreated {
		t.Fatalf("plan = %d %v", status1, body)
	}
	if code, _, _ := call(t, "PUT", srv.URL+"/api/plans/active?account=acc_ge", `{"plan":"`+body["id"].(string)+`"}`); code != http.StatusOK {
		t.Fatalf("activate = %d", code)
	}

	// The glass is in the bank, the live inventory has none: no sale.
	inventory(coreingest.QuantityMap{"995": 1000})
	if st, state, reason := next(); st != "task" || state != "unknown" || !strings.Contains(reason, "nenhum run provou a venda") {
		t.Fatalf("glass in the bank = %s %s %q", st, state, reason)
	}

	// The glass in the inventory: the sale runs and fills.
	inventory(coreingest.QuantityMap{"1775": 100, "995": 1000})
	if st, state, _ := next(); st != "task" || state != "unmet" {
		t.Fatalf("glass in the inventory = %s %s", st, state)
	}
	if code, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_ge", `{"key":"task:0#0"}`); code != http.StatusOK {
		t.Fatalf("run = %d %v", code, body)
	}
	var st taskStatus
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if st = status(); st.Run != nil && st.Run.Final() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run never ended: %+v", st.Run)
		}
	}
	// 130 less 5% is 123, less the 2 gp tax: at least 121 a glass.
	if st.Run.State != agent.RunDone || st.Run.Task.CoinsAtLeast != 1000+100*121 || st.Proven {
		t.Fatalf("after the run, before the ingest = %+v %+v", st.Run, st.Proof)
	}

	// The glass left, the coins did not reach the inventory (an abort sends
	// them to the bank): not proven.
	inventory(coreingest.QuantityMap{"995": 1000})
	time.Sleep(150 * time.Millisecond) // the tracker looks at the ingest's facts
	if st = status(); st.Proven {
		t.Fatalf("coins missing = %+v", st.Proof)
	}
	if s, state, _ := next(); s != "task" || state != "unknown" {
		t.Fatalf("plan before the proof = %s %s", s, state)
	}

	// The coins arrived: the tracker proves the sale's job on the live
	// inventory, which the plan reads from then on.
	inventory(coreingest.QuantityMap{"995": 13100})
	eventually(t, "the sale proven", func() bool { st = status(); return st.Proven })
	if st.Proof.State != "met" || (features == nil) != (st.Run.Task.JobID != "") {
		t.Fatalf("coins arrived = %+v, run task %+v", st.Proof, st.Run.Task)
	}
	// The sale is the job the run attempted, proven by the tracker, and it is
	// the only job: no second one was adopted for the run.
	_, listed, _ := call(t, "GET", srv.URL+"/api/jobs?account=acc_ge", "")
	jobs, _ := listed["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("jobs = %v", listed)
	}
	if _, body, _ := call(t, "GET", srv.URL+"/api/jobs/"+jobs[0].(map[string]any)["id"].(string)+"?account=acc_ge", ""); body["job"].(map[string]any)["state"] != "succeeded" ||
		body["job"].(map[string]any)["kind"] != "ge_sell" || len(body["runs"].([]any)) != 1 {
		t.Fatalf("the sale's job = %v", body)
	}
	// Spent afterwards, the sale stays met: the plan is done.
	inventory(coreingest.QuantityMap{})
	time.Sleep(150 * time.Millisecond)
	if s, _, _ := next(); s != "done" {
		t.Fatalf("after spending = %s", s)
	}
	// The glass back in the inventory is something to sell again.
	inventory(coreingest.QuantityMap{"1775": 5, "995": 200})
	if s, state, _ := next(); s != "task" || state != "unmet" {
		t.Fatalf("glass again = %s %s", s, state)
	}
	// The second sale is proven by its own run, never by the first one's.
	first := st.Run.UID
	if code, body, _ := call(t, "POST", srv.URL+"/api/microbot/task?account=acc_ge", `{"key":"task:0#0"}`); code != http.StatusOK {
		t.Fatalf("second run = %d %v", code, body)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if st = status(); st.Run != nil && st.Run.UID != first && st.Run.Final() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the second run never ended: %+v", st.Run)
		}
	}
	inventory(coreingest.QuantityMap{"995": 200})
	time.Sleep(150 * time.Millisecond)
	if st = status(); st.Proven {
		t.Fatalf("second sale proven without its coins = %+v", st.Proof)
	}
	if s, state, _ := next(); s != "task" || state != "unknown" {
		t.Fatalf("plan after the second sale, before its proof = %s %s", s, state)
	}
	inventory(coreingest.QuantityMap{"995": 200 + 5*121})
	eventually(t, "the second sale proven", func() bool { st = status(); return st.Proven })
	if s, _, _ := next(); s != "done" {
		t.Fatalf("after the second proof = %s", s)
	}
}
