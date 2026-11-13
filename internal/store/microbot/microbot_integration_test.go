package microbot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuditAccountAndAgent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	repo := New(pool)

	// A row written without the 0012 columns (as before the migration) reads
	// with empty account and agent.
	if _, err := pool.Exec(ctx, `INSERT INTO microbot_audit (actor, origin, action, ok, at) VALUES ('dev', 'web', 'state', true, now() - interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	for _, e := range []Entry{
		{Actor: "u1", Origin: "web", Action: "client_start", OK: true, AccountID: "acc_a", AgentID: "agent-zaros"},
		{Actor: "u1", Origin: "web", Action: "login", Params: json.RawMessage(`{"world":301}`), OK: false, Error: "boom", AccountID: "acc_b", AgentID: "worker-acc-b"},
		{Actor: "u1", Origin: "routine", Action: "run_start", OK: true, AccountID: "acc_a", AgentID: "agent-zaros"},
	} {
		got, err := repo.Append(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID == 0 || got.AccountID != e.AccountID || got.AgentID != e.AgentID || got.At.IsZero() {
			t.Fatalf("appended = %+v", got)
		}
	}

	all, err := repo.List(ctx, 10, "")
	if err != nil || len(all) != 4 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if all[0].Action != "run_start" || all[3].Action != "state" || all[3].AccountID != "" || all[3].AgentID != "" {
		t.Fatalf("order or legacy row = %+v", all)
	}
	if b := all[1]; b.AccountID != "acc_b" || b.AgentID != "worker-acc-b" || b.Error != "boom" || string(b.Params) != `{"world": 301}` {
		t.Fatalf("acc_b entry = %+v (params %s)", b, b.Params)
	}

	onlyA, err := repo.List(ctx, 10, "acc_a")
	if err != nil || len(onlyA) != 2 || onlyA[0].Action != "run_start" || onlyA[1].Action != "client_start" {
		t.Fatalf("acc_a = %+v, %v", onlyA, err)
	}
	if none, _ := repo.List(ctx, 10, "acc_missing"); len(none) != 0 {
		t.Fatalf("missing account = %+v", none)
	}
	if one, _ := repo.List(ctx, 1, "acc_a"); len(one) != 1 || one[0].Action != "run_start" {
		t.Fatalf("limit 1 = %+v", one)
	}
}

// The server's own actions (the reconciler's restart on a new image, docs/11
// section 7.2) are audited as origin server; an origin outside the list is
// refused.
func TestAuditServerOrigin(t *testing.T) {
	ctx := context.Background()
	repo := New(testPool(ctx, t))
	e := Entry{
		Actor: "reconciler", Origin: "server", Action: "worker_restart", OK: true, AccountID: "acc_a", AgentID: "c1",
		Params: json.RawMessage(`{"reason":"versão nova, na pausa","pause":"logged_out"}`),
	}
	if got, err := repo.Append(ctx, e); err != nil || got.Origin != "server" || got.Actor != "reconciler" {
		t.Fatalf("server entry = %+v, %v", got, err)
	}
	if _, err := repo.Append(ctx, Entry{Actor: "x", Origin: "cluster", Action: "worker_restart", OK: true}); err == nil {
		t.Fatal("an unknown origin was stored")
	}
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL microbot audit integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("microbot_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, dropErr := admin.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE"); dropErr != nil {
			t.Errorf("drop test schema: %v", dropErr)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if pool, err = pgxpool.NewWithConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "db", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		up, _, found := strings.Cut(string(raw), "-- +goose Down")
		if !found {
			t.Fatalf("%s has no goose Down marker", f)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	return pool
}
