package syncrun

import (
	"context"
	"encoding/json"
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

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/events"
	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// recorder keeps the published events as "type:source:status".
type recorder struct{ got []string }

func (r *recorder) Publish(e events.Event) {
	r.got = append(r.got, e.Type+":"+e.Source+":"+e.Status)
}

// fakeFetcher answers with a sample file or an error, and records the calls.
type fakeFetcher struct {
	file  string
	err   error
	calls []string
}

func (f *fakeFetcher) Fetch(_ context.Context, rsn, mode string) (hiscores.Result, error) {
	f.calls = append(f.calls, rsn+"/"+mode)
	if f.err != nil {
		return hiscores.Result{}, f.err
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "data", "samples", "hiscores", f.file))
	return hiscores.Result{Raw: raw, Table: "hiscore_oldschool_ironman"}, err
}

func TestSyncPostgresLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-sync-test', 'dev-sync-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'ironman260', 'ironman260', 'ironman', 'g1', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, loadContent(t), catalog)
	fetch := &fakeFetcher{file: "ironman260.json"}
	rec := &recorder{}
	repo := New(pool, fetch, progress).WithPublisher(rec)
	clock := time.Now()
	repo.now = func() time.Time { return clock }

	// First manual sync: snapshot, facts and reevaluation in one go.
	run, err := repo.Run(ctx, "account-1", TriggerManual)
	if err != nil || run.Status != "ok" || run.SnapshotID == "" || run.StepsChanged == 0 {
		t.Fatalf("first run = %+v, %v", run, err)
	}
	if len(fetch.calls) != 1 || fetch.calls[0] != "ironman260/ironman" {
		t.Fatalf("calls = %v", fetch.calls)
	}
	var source string
	if err = pool.QueryRow(ctx, `SELECT source FROM player_state WHERE account_id = 'account-1' AND facet = 'skill' AND key = 'attack'`).Scan(&source); err != nil || source != "hiscores" {
		t.Fatalf("attack fact = %q, %v", source, err)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM player_state WHERE facet = 'skill' AND key = 'fletching'`); n != 0 {
		t.Fatal("an unranked skill was written as level 1")
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM snapshot_facts WHERE facet = 'skill'`); n == 0 {
		t.Fatal("no skill snapshot_facts rows")
	}
	board, err := progress.Board(ctx, "account-1", "g1")
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, s := range board.Steps {
		status[s.ID] = string(s.Status)
	}
	if status["a2"] != "auto_done" || status["a3"] != "pending" {
		t.Fatalf("statuses = %v (a3 must stay pending: fletching is unranked, so unknown)", status)
	}

	// A manual sync right away is refused by the debounce.
	var tooSoon *TooSoonError
	if _, err = repo.Run(ctx, "account-1", TriggerManual); !errors.As(err, &tooSoon) {
		t.Fatalf("debounce: %v", err)
	}
	// The same facts again (ranks may move) are "unchanged".
	clock = clock.Add(2 * time.Minute)
	if run, err = repo.Run(ctx, "account-1", TriggerScheduled); err != nil || run.Status != "unchanged" || run.SnapshotID != "" {
		t.Fatalf("unchanged run = %+v, %v", run, err)
	}

	// Three 404s in a row: errors with growing backoff, then the rename alert.
	fetch.err = &hiscores.StatusError{Status: 404, Err: hiscores.ErrNotFound}
	var last RunView
	for i := 1; i <= 3; i++ {
		clock = clock.Add(7 * time.Hour)
		if last, err = repo.Run(ctx, "account-1", TriggerScheduled); err != nil || last.Status != "error" || last.HTTPStatus != 404 {
			t.Fatalf("404 #%d = %+v, %v", i, last, err)
		}
		if want := clock.Add(Backoff(i)); last.NextAllowedAt == nil || last.NextAllowedAt.Sub(want).Abs() > time.Millisecond {
			t.Fatalf("404 #%d backoff = %v, want %v", i, last.NextAllowedAt, want)
		}
	}
	if !strings.Contains(last.Error, "renomeou") {
		t.Fatalf("third 404 error = %q", last.Error)
	}
	if _, err = repo.Run(ctx, "account-1", TriggerManual); !errors.As(err, &tooSoon) || !tooSoon.RetryAt.Equal(*last.NextAllowedAt) {
		t.Fatalf("manual run during backoff: %v", err)
	}
	sched := &Scheduler{Repo: repo, Interval: 30 * time.Minute, Jitter: 5 * time.Minute}
	if n := sched.RunDue(ctx); n != 0 {
		t.Fatalf("scheduler ran %d during the backoff", n)
	}

	// After the backoff the scheduler runs; a 429 is rate_limited.
	clock = clock.Add(7 * time.Hour)
	fetch.err = &hiscores.StatusError{Status: 429, Err: hiscores.ErrRateLimited}
	if n := sched.RunDue(ctx); n != 1 {
		t.Fatalf("scheduler ran %d after the backoff", n)
	}
	runs, err := repo.Runs(ctx, "account-1", 10)
	if err != nil || len(runs) != 6 || runs[0].Status != "rate_limited" || runs[0].Trigger != TriggerScheduled {
		t.Fatalf("runs = %+v, %v", runs, err)
	}

	if _, err = repo.Run(ctx, "nobody", TriggerManual); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("unknown account: %v", err)
	}

	// Events: progress only for the run that stored a snapshot, a sync event
	// per finished run, nothing for refused runs.
	want := []string{
		"progress:hiscores:", "sync:hiscores:ok", "sync:hiscores:unchanged",
		"sync:hiscores:error", "sync:hiscores:error", "sync:hiscores:error", "sync:hiscores:rate_limited",
	}
	if strings.Join(rec.got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", rec.got, want)
	}
}

func loadContent(t *testing.T) *content.Content {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "guides.json"))
	if err != nil {
		t.Fatal(err)
	}
	var guides []*content.Guide
	if err := json.Unmarshal(raw, &guides); err != nil {
		t.Fatal(err)
	}
	for _, g := range guides {
		g.Index()
	}
	return &content.Content{Guides: guides}
}

func count(ctx context.Context, t *testing.T, pool *pgxpool.Pool, query string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// testPool opens a schema of its own with every migration applied.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL sync integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err = admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping admin database: %v", err)
	}
	schema := fmt.Sprintf("sync_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, dropErr := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); dropErr != nil {
			t.Errorf("drop test schema: %v", dropErr)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open test database: %v", err)
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
		if _, execErr := pool.Exec(ctx, up); execErr != nil {
			t.Fatalf("apply %s: %v", f, execErr)
		}
	}
	return pool
}
