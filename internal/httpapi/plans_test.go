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

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// memPlans is PlanStore in memory: acc-1 follows its route.
type memPlans struct{ created []storeplans.NewPlan }

func (m *memPlans) List(_ context.Context, account string) (storeplans.List, error) {
	if account != "acc-1" {
		return storeplans.List{}, storeplans.ErrAccountNotFound
	}
	return storeplans.List{Account: account, Active: plan.RouteID("oziris-main"), Plans: []plan.Plan{}}, nil
}

func (m *memPlans) Active(_ context.Context, account string) (plan.Plan, error) {
	if account != "acc-1" {
		return plan.Plan{}, storeplans.ErrAccountNotFound
	}
	return plan.Plan{ID: plan.RouteID("oziris-main"), Kind: plan.KindRoute, Guide: "oziris-main"}, nil
}

func (m *memPlans) Create(_ context.Context, _ string, in storeplans.NewPlan) (plan.Plan, error) {
	m.created = append(m.created, in)
	return plan.Plan{ID: "pln_mem00001", Kind: in.Kind, Title: in.Title}, nil
}

func (m *memPlans) Delete(context.Context, string, string) error { return storeplans.ErrPlanNotFound }

func (m *memPlans) SetActive(ctx context.Context, account, _ string) (plan.Plan, error) {
	return m.Active(ctx, account)
}

// In oidc the plan routes need a session, reach only the session user's
// accounts, and the writes need the CSRF header (rule 10).
func TestPlanRoutesOIDC(t *testing.T) {
	store := &memPlans{}
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) { o.Plans = store })
	c := e.client()
	if status, _ := e.do(c, "GET", "/api/plan/next?account=acc-1", "", false); status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", status)
	}
	e.login(c, "/", owner)
	for _, path := range []string{"/api/plan/next?account=acc-2", "/api/plans?account=acc-2"} {
		if status, _ := e.do(c, "GET", path, "", false); status != http.StatusNotFound {
			t.Fatalf("%s: another user's account = %d", path, status)
		}
	}
	status, body := e.do(c, "GET", "/api/plan/next?account=acc-1", "", false)
	if status != http.StatusOK || body["account"] != "acc-1" || body["plan"].(map[string]any)["kind"] != "route" {
		t.Fatalf("own account = %d %v", status, body)
	}
	newPlan := `{"kind":"custom","title":"P","items":[]}`
	if status, _ := e.do(c, "POST", "/api/plans?account=acc-1", newPlan, false); status != http.StatusForbidden {
		t.Fatalf("create without CSRF = %d", status)
	}
	if status, _ := e.do(c, "POST", "/api/plans?account=acc-2", newPlan, true); status != http.StatusNotFound || len(store.created) != 0 {
		t.Fatalf("create on another user's account = %d", status)
	}
	if status, _ := e.do(c, "POST", "/api/plans?account=acc-1", newPlan, true); status != http.StatusCreated || len(store.created) != 1 {
		t.Fatalf("create = %d", status)
	}
}

func TestPlanRoutesAreDevOnly(t *testing.T) {
	srv := httptest.NewServer(New(Options{Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Plans: &memPlans{}, Progress: &fakeProgress{}}))
	t.Cleanup(srv.Close)
	if status, _, _ := call(t, "GET", srv.URL+"/api/plan/next?account=acc-1", ""); status != http.StatusNotFound {
		t.Fatalf("outside dev mode and oidc = %d", status)
	}
}

// TestPlanNextPostgres runs the plan routes on the real stores and content:
// the route plan of an account, a custom plan it follows, and the travel
// the plugin's position proves.
func TestPlanNextPostgres(t *testing.T) {
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
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_plan', 'dev', 'ironman260', 'ironman260', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, cnt, catalog)
	if _, err = progress.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).WithReevaluator(progress)
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, Progress: progress, Holdings: ingest, Catalog: catalog, DevMode: true,
		Plans: storeplans.New(pool, cnt, catalog), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	t.Cleanup(srv.Close)
	const acc = "?account=acc_plan"
	next := func() map[string]any {
		t.Helper()
		status, body, _ := call(t, "GET", srv.URL+"/api/plan/next"+acc, "")
		if status != http.StatusOK {
			t.Fatalf("next = %d %v", status, body)
		}
		return body
	}

	// The route: the content has no typed task yet, so the cursor step is
	// the owner's, with the guide's credit line (rule 6).
	body := next()
	item, _ := body["item"].(map[string]any)
	if body["status"] != plan.StatusManual || body["task"] != nil || item == nil || item["guide"] != "oziris-main" ||
		!strings.Contains(item["attribution"].(string), "OzirisLoL") || body["plan"].(map[string]any)["id"] != "route:oziris-main" {
		t.Fatalf("route = %v", body)
	}

	// A custom plan: Lumbridge Castle, then Draynor.
	status, created, _ := call(t, "POST", srv.URL+"/api/plans"+acc, `{"kind":"custom","title":"Logística","items":[
		{"kind":"task","task":{"type":"travel","location":"location:lumbridge_castle"}},
		{"kind":"task","task":{"type":"travel","location":"location:draynor_village"}}]}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v", status, created)
	}
	id := created["id"].(string)
	if status, refused, _ := call(t, "POST", srv.URL+"/api/plans"+acc, `{"kind":"custom","title":"x","items":[{"kind":"task","task":{"type":"quest"}}]}`); status != http.StatusBadRequest || refused["problems"] == nil {
		t.Fatalf("invalid = %d %v", status, refused)
	}
	if status, _, _ := call(t, "PUT", srv.URL+"/api/plans/active"+acc, `{"plan":"pln_nothere0"}`); status != http.StatusNotFound {
		t.Fatalf("unknown plan = %d", status)
	}
	if status, _, _ := call(t, "PUT", srv.URL+"/api/plans/active"+acc, `{"plan":"`+id+`"}`); status != http.StatusOK {
		t.Fatalf("activate = %d", status)
	}
	body = next()
	task, _ := body["task"].(map[string]any)
	if body["status"] != plan.StatusTask || task["key"] != "task:0#0" || task["state"] != "unknown" || task["executable"] != true {
		t.Fatalf("before a position = %v", body)
	}

	// The plugin sends a position inside the castle: the first travel is
	// met (proven by the ingest) and skipped.
	token, err := ingest.CreateToken(ctx, "acc_plan", "plan", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
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
	body = next()
	task, _ = body["task"].(map[string]any)
	skipped, _ := body["skipped"].([]any)
	if body["status"] != plan.StatusTask || task["key"] != "task:1#0" || task["state"] != "unmet" || len(skipped) != 1 {
		t.Fatalf("after the position = %v", body)
	}

	// Deleting the plan puts the account back on its route.
	if status, _, _ := call(t, "DELETE", srv.URL+"/api/plans/"+id+acc, ""); status != http.StatusNoContent {
		t.Fatalf("delete = %d", status)
	}
	if body = next(); body["plan"].(map[string]any)["kind"] != plan.KindRoute {
		t.Fatalf("after the delete = %v", body)
	}
	if status, _, _ := call(t, "GET", srv.URL+"/api/plan/next?account=acc_nobody", ""); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}
}
