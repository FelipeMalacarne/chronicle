package prices

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/core/ge"
)

func TestReplacePostgres(t *testing.T) {
	ctx := context.Background()
	repo := New(testPool(ctx, t))
	t0 := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	lim, members := 100, true
	price := func(id int, high, low int64, at time.Time) ge.Price {
		p := ge.Price{ItemID: id, Source: ge.SourceWiki, FetchedAt: at}
		if high > 0 {
			p.High, p.HighAt = high, at
		}
		if low > 0 {
			p.Low, p.LowAt = low, at
		}
		return p
	}

	first := price(1, 7_949_999_997, 7_800_000_000, t0)
	first.Limit, first.Members = &lim, &members
	if err := repo.Replace(ctx, []ge.Price{first, price(2, 0, 50, t0)}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2 (3 has no price)", len(got))
	}
	if !reflect.DeepEqual(got[1], first) {
		t.Errorf("item 1 round trip:\n got %+v\nwant %+v", got[1], first)
	}
	if got[2].High != 0 || !got[2].HighAt.IsZero() || got[2].Low != 50 || got[2].Limit != nil || got[2].Members != nil {
		t.Errorf("item 2: %+v", got[2])
	}

	// A newer batch without item 1 keeps it; item 2 gains a high side and
	// keeps its low; a batch that knows no limit keeps the stored one.
	t1 := t0.Add(time.Hour)
	second := price(2, 60, 0, t1)
	if err = repo.Replace(ctx, []ge.Price{second, price(3, 9, 8, t1)}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, 1, 2, 3)
	if len(got) != 3 || !reflect.DeepEqual(got[1], first) {
		t.Fatalf("item 1 must stay: %+v", got)
	}
	if got[2].High != 60 || !got[2].HighAt.Equal(t1) || got[2].Low != 50 || !got[2].LowAt.Equal(t0) || !got[2].FetchedAt.Equal(t1) {
		t.Errorf("item 2 merge: %+v", got[2])
	}

	// Never back in time: an older trade on either side is ignored, a newer
	// one wins, and the limit survives a batch that lacks it.
	older := price(1, 1, 1, t0.Add(-time.Hour))
	older.FetchedAt = t0.Add(2 * time.Hour)
	newer := price(2, 0, 55, t1.Add(time.Minute))
	if err = repo.Replace(ctx, []ge.Price{older, newer}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, 1, 2)
	if got[1].High != first.High || got[1].Low != first.Low || !got[1].HighAt.Equal(t0) || got[1].Limit == nil || *got[1].Limit != 100 {
		t.Errorf("older trade overwrote: %+v", got[1])
	}
	if got[2].Low != 55 || got[2].High != 60 {
		t.Errorf("newer low: %+v", got[2])
	}

	// A bad row rolls the whole batch back.
	if err = repo.Replace(ctx, []ge.Price{price(4, 5, 5, t1), {ItemID: 5, Source: ge.SourceWiki, FetchedAt: t1}}); err == nil {
		t.Error("batch with an empty price committed")
	}
	if got, _ = repo.Get(ctx, 4, 5); len(got) != 0 {
		t.Errorf("rolled back batch left %+v", got)
	}
	if got, err = repo.Get(ctx); err != nil || len(got) != 0 {
		t.Errorf("Get() = %v, %v", got, err)
	}
}

func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL prices integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("prices_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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

type fakeSource struct {
	batch []ge.Price
	err   error
}

func (f fakeSource) Fetch(context.Context) ([]ge.Price, error) { return f.batch, f.err }

func TestRefreshPostgres(t *testing.T) {
	ctx := context.Background()
	repo := New(testPool(ctx, t))
	at := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	batch := []ge.Price{{ItemID: 7, High: 5, HighAt: at, Source: ge.SourceWiki, FetchedAt: at}}
	if n := (&Scheduler{Repo: repo, Source: fakeSource{batch: batch}}).Refresh(ctx); n != 1 {
		t.Fatalf("stored %d, want 1", n)
	}
	if n := (&Scheduler{Repo: repo, Source: fakeSource{err: context.DeadlineExceeded}}).Refresh(ctx); n != 0 {
		t.Fatalf("a failed fetch stored %d", n)
	}
	if got, _ := repo.Get(ctx, 7); got[7].High != 5 {
		t.Errorf("price lost after a failed refresh: %+v", got)
	}
}
