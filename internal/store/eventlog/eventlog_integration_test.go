package eventlog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// A reader never reads past an uncommitted event of its own log, and a writer
// in another schema (another test package's log) never holds it back.
func TestUncommittedEventHoldsOnlyItsOwnLogPostgres(t *testing.T) {
	ctx := context.Background()
	mine, other := testPool(ctx, t), testPool(ctx, t)

	tx, err := mine.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = Append(ctx, storedb.New(tx), "acc_log", "fact.changed", "skill:magic", "", nil); err != nil {
		t.Fatal(err)
	}

	if _, err = New(mine).After(ctx, 0, 10); !errors.Is(err, ErrBusy) {
		t.Fatalf("After on the writer's log = %v, want ErrBusy", err)
	}
	var evs []Event
	if evs, err = New(other).After(ctx, 0, 10); err != nil || len(evs) != 0 {
		t.Fatalf("After on another schema's log = %v, %v: its writer must not hold it", evs, err)
	}

	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if evs, err = New(mine).After(ctx, 0, 10); err != nil || len(evs) != 1 {
		t.Fatalf("After the commit = %v, %v", evs, err)
	}
}

func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL event log integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("eventlog_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
		raw, readErr := os.ReadFile(f)
		if readErr != nil {
			t.Fatal(readErr)
		}
		up, _, found := strings.Cut(string(raw), "-- +goose Down")
		if !found {
			t.Fatalf("%s has no goose Down marker", f)
		}
		if _, err = pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_log', 'u1', 'Log', 'log', 'regular', NULL, 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	return pool
}
