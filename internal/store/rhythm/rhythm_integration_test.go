package rhythm

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

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
)

// The rhythm from real plugin pushes: a full one in the morning, a long
// pause, then a session of deltas with Fishing XP going up and coins coming
// into the inventory.
func TestRhythmPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_r', 'u1', 'Rhythm', 'rhythm', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	cnt, err := content.Load(filepath.Join("..", "..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog))
	token, err := ingest.CreateToken(ctx, "acc_r", "test", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-6 * time.Hour)
	xp0 := entities.XPForLevel(70)
	xps := []int64{int64(xp0), int64(xp0) + 4000, int64(xp0) + 9000, int64(xp0) + 14000, int64(xp0) + 23000}
	pushes := []struct {
		ago  time.Duration
		full bool
		xp   int64
		inv  map[string]int64
	}{
		{8 * time.Hour, true, xps[0], map[string]int64{}},
		{40 * time.Minute, false, xps[1], map[string]int64{"995": 2000}},
		{20 * time.Minute, false, xps[2], nil},
		{10 * time.Minute, false, xps[3], nil},
		{2 * time.Minute, false, xps[4], nil},
	}
	for i, p := range pushes {
		sentAt := now.Add(-p.ago)
		rev := int64(i + 1)
		live := func(q map[string]int64) *coreingest.ItemObservation {
			m := coreingest.QuantityMap(q)
			return &coreingest.ItemObservation{State: coreingest.ItemLive, ObservedAt: sentAt.Add(-time.Second), Revision: &rev, LiveEpoch: "login-1", Quantities: &m}
		}
		levels := coreingest.IntMap{"fishing": int64(entities.LevelForXP(int(p.xp)))}
		xp := coreingest.IntMap{"fishing": p.xp}
		push := coreingest.Push{
			ContractVersion: coreingest.ContractVersion, Username: "Rhythm", Profile: "STANDARD",
			ManifestVersion: manifest.ManifestVersion, ClientSeq: int64(i + 1), Kind: coreingest.PushDelta, SentAt: sentAt,
			Data: coreingest.PushData{Levels: &levels, XP: &xp},
		}
		if p.full {
			empty := coreingest.IntMap{}
			push.Kind = coreingest.PushFull
			push.Data.Varbits, push.Data.Varps, push.Data.Boosted = &empty, &empty, &empty
			push.Data.Items = &coreingest.ItemContainers{
				Bank: live(map[string]int64{"995": 100_000, "4151": 1}), Inventory: live(p.inv), Equipment: live(map[string]int64{}),
			}
		} else if p.inv != nil {
			push.Data.Items = &coreingest.ItemContainers{Inventory: live(p.inv)}
		}
		raw, _ := json.Marshal(push)
		if resp, perr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); perr != nil || !resp.OK {
			t.Fatalf("push %d: %+v, %v", i, resp, perr)
		}
	}

	repo := New(pool, catalog)
	if _, err = repo.Rhythm(ctx, "acc_missing", now, since); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	r, err := repo.Rhythm(ctx, "acc_r", now, since)
	if err != nil {
		t.Fatal(err)
	}
	if r.Today.XP != int(xps[4]-xps[0]) || len(r.Today.Skills) != 1 || r.Today.Skills[0].Skill != "fishing" {
		t.Fatalf("today = %+v", r.Today)
	}
	if r.Session == nil || !r.Session.StartedAt.Equal(now.Add(-40*time.Minute)) || r.Session.XP != int(xps[4]-xps[1]) {
		t.Fatalf("session = %+v", r.Session)
	}
	// From the first gain in the hour (20 min ago) to the last (2 min ago).
	wantRate := int(float64(xps[4]-xps[2]) / (18 * time.Minute).Hours())
	if tr := r.Training; tr == nil || tr.Skill != "fishing" || tr.XP != int(xps[4]) || tr.XPPerHour != wantRate || tr.NextLevelInS == nil {
		t.Fatalf("training = %+v, want rate %d", r.Training, wantRate)
	}
	if r.GP == nil || r.GP.Value != 102_000 || r.GP.Today == nil || *r.GP.Today != 2000 {
		t.Fatalf("gp = %+v", r.GP)
	}

	// Two hours later the session is over and nothing is being trained.
	later, err := repo.Rhythm(ctx, "acc_r", now.Add(2*time.Hour), since)
	if err != nil {
		t.Fatal(err)
	}
	if later.Session != nil || later.Training != nil || later.Today.XP != r.Today.XP {
		t.Fatalf("later = %+v", later)
	}
	// An account without snapshots has an empty rhythm.
	if _, err = pool.Exec(ctx, `INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_new', 'u1', 'New', 'new', 'ironman', 'oziris-main', 'enhanced')`); err != nil {
		t.Fatal(err)
	}
	empty, err := repo.Rhythm(ctx, "acc_new", now, since)
	if err != nil || empty.Training != nil || empty.Session != nil || empty.GP != nil || empty.Today.XP != 0 {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL rhythm integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("rhythm_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
