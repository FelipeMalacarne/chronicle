package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
)

// plansPlanner keeps the plans of acc_x in memory, refusing an empty plan
// the way the server does.
type plansPlanner struct {
	active  string
	plans   []plan.Plan
	deleted []string
}

func (p *plansPlanner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/me":
		_ = json.NewEncoder(w).Encode(client.Me{AuthMode: "dev", Accounts: []client.Account{{ID: "acc_x", RSN: "X"}}})
	case r.URL.Path == "/api/plans" && r.Method == http.MethodGet:
		all := append([]plan.Plan{{ID: "route:oziris-main", Kind: plan.KindRoute, Title: "Oziris", Guide: "oziris-main", Items: []plan.Item{}}}, p.plans...)
		_ = json.NewEncoder(w).Encode(client.Plans{Account: "acc_x", Active: p.active, Plans: all})
	case r.URL.Path == "/api/plans" && r.Method == http.MethodPost:
		var in client.NewPlan
		_ = json.NewDecoder(r.Body).Decode(&in)
		if len(in.Items) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid plan","status":400,"problems":["items/0: unknown guide \"x\""]}`))
			return
		}
		created := plan.Plan{ID: "pln_0011223344556677", Kind: in.Kind, Title: in.Title, Items: in.Items}
		p.plans = append(p.plans, created)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	case r.URL.Path == "/api/plans/active":
		var in struct {
			Plan string `json:"plan"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		p.active = in.Plan
		_ = json.NewEncoder(w).Encode(plan.Plan{ID: in.Plan, Title: "Logística"})
	case strings.HasPrefix(r.URL.Path, "/api/plans/") && r.Method == http.MethodDelete:
		p.deleted = append(p.deleted, strings.TrimPrefix(r.URL.Path, "/api/plans/"))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func TestPlansCLI(t *testing.T) {
	p := &plansPlanner{active: "route:oziris-main"}
	srv := httptest.NewServer(p)
	defer srv.Close()
	dir := t.TempDir()
	good := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(good, []byte(`{"kind":"custom","title":"Logística","items":[
		{"kind":"task","task":{"type":"travel","location":"location:lumbridge_castle"}},
		{"kind":"step","guide":"oziris-main","step":"1.1.3"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"kind":"custom","title":"x","items":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "plans", "create", "--url", srv.URL, "-f", good)
	if err != nil || !strings.Contains(out, "pln_0011223344556677: Logística (2 itens)") {
		t.Fatalf("create = %q %v", out, err)
	}
	if out, err = runCLI(t, "plans", "create", "--url", srv.URL, "-f", bad); err == nil || !strings.Contains(err.Error(), "unknown guide") {
		t.Fatalf("refused = %q %v", out, err)
	}
	if out, err = runCLI(t, "plans", "use", "--url", srv.URL, "pln_0011223344556677"); err != nil || p.active != "pln_0011223344556677" {
		t.Fatalf("use = %q %v", out, err)
	}
	out, err = runCLI(t, "plans", "--url", srv.URL)
	if err != nil || !strings.Contains(out, "* pln_0011223344556677") || !strings.Contains(out, "route:oziris-main") {
		t.Fatalf("list = %q %v", out, err)
	}
	if out, err = runCLI(t, "plans", "use", "--url", srv.URL, "route"); err != nil || p.active != "" {
		t.Fatalf("back to the route = %q %v (%q)", out, err, p.active)
	}
	if _, err = runCLI(t, "plans", "delete", "--url", srv.URL, "pln_0011223344556677"); err == nil {
		t.Fatal("delete without --yes")
	}
	if out, err = runCLI(t, "plans", "delete", "--url", srv.URL, "pln_0011223344556677", "--yes"); err != nil || len(p.deleted) != 1 {
		t.Fatalf("delete = %q %v", out, err)
	}
}
