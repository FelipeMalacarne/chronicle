package activity

import (
	"context"
	"encoding/json"
	"errors"
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

	coreactivity "github.com/FelipeMalacarne/chronicle/internal/core/activity"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
)

// Level-ups from real plugin pushes: a full one before the window (the
// baseline), deltas inside it, and a hiscores snapshot that lags the plugin.
func TestLevelsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		('acc_l', 'u1', 'Levels', 'levels', 'ironman', 'oziris-main', 'enhanced'),
		('acc_new', 'u1', 'New', 'new', 'ironman', 'oziris-main', 'enhanced');
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
	token, err := ingest.CreateToken(ctx, "acc_l", "test", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}

	xp := entities.XPForLevel
	now := time.Now().UTC().Truncate(time.Second)
	at := func(ago time.Duration) time.Time { return now.Add(-ago) }
	pushes := []struct {
		ago  time.Duration
		full bool
		xp   map[string]int
	}{
		{30 * time.Hour, true, map[string]int{"fishing": xp(70) + 10, "farming": xp(33) + 5, "hunter": xp(29)}},
		{20 * time.Hour, false, map[string]int{"fishing": xp(71) + 1}},
		{3 * time.Hour, false, map[string]int{"fishing": xp(74) - 1, "farming": xp(39)}},
		{time.Hour, false, map[string]int{"fishing": xp(76) + 20}},
	}
	for i, p := range pushes {
		sentAt := at(p.ago)
		levels, xps := coreingest.IntMap{}, coreingest.IntMap{}
		for skill, v := range p.xp {
			levels[skill], xps[skill] = int64(entities.LevelForXP(v)), int64(v)
		}
		push := coreingest.Push{
			ContractVersion: coreingest.ContractVersion, Username: "Levels", Profile: "STANDARD",
			ManifestVersion: manifest.ManifestVersion, ClientSeq: int64(i + 1), Kind: coreingest.PushDelta, SentAt: sentAt,
			Data: coreingest.PushData{Levels: &levels, XP: &xps},
		}
		if p.full {
			rev := int64(1)
			live := func() *coreingest.ItemObservation {
				m := coreingest.QuantityMap{}
				return &coreingest.ItemObservation{State: coreingest.ItemLive, ObservedAt: sentAt, Revision: &rev, LiveEpoch: "login-1", Quantities: &m}
			}
			empty := coreingest.IntMap{}
			push.Kind = coreingest.PushFull
			push.Data.Varbits, push.Data.Varps, push.Data.Boosted = &empty, &empty, &empty
			push.Data.Items = &coreingest.ItemContainers{Bank: live(), Inventory: live(), Equipment: live()}
		}
		raw, _ := json.Marshal(push)
		if resp, perr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); perr != nil || !resp.OK {
			t.Fatalf("push %d: %+v, %v", i, resp, perr)
		}
	}
	// The hiscores, two hours ago, still show Fishing 72: after the plugin's
	// 73, so stale.
	if _, err = pool.Exec(ctx, `
		INSERT INTO snapshots (id, account_id, source, kind, fetched_at, raw, raw_hash, adapter_version)
		VALUES ('snap_hs', 'acc_l', 'hiscores', 'full', $1, '{}', 'hs-1', 1);
		INSERT INTO snapshot_facts (snapshot_id, facet, key, value)
		VALUES ('snap_hs', 'skill', 'fishing', jsonb_build_object('level', 72, 'xp', $2::bigint));
	`, at(2*time.Hour), xp(72)); err != nil {
		t.Fatal(err)
	}

	repo := New(pool)
	if _, err = repo.Levels(ctx, "acc_missing", at(24*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	up := func(ago time.Duration, skill string, level int) coreactivity.LevelUp {
		return coreactivity.LevelUp{Skill: skill, Level: level, At: at(ago)}
	}

	// The last day: the full push 30 hours ago is the baseline.
	day, err := repo.Levels(ctx, "acc_l", at(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []coreactivity.LevelUp{
		up(time.Hour, "fishing", 76), up(time.Hour, "fishing", 75), up(time.Hour, "fishing", 74),
		up(3*time.Hour, "farming", 39), up(3*time.Hour, "farming", 38), up(3*time.Hour, "farming", 37),
		up(3*time.Hour, "farming", 36), up(3*time.Hour, "farming", 35), up(3*time.Hour, "farming", 34),
		up(3*time.Hour, "fishing", 73), up(3*time.Hour, "fishing", 72),
		up(20*time.Hour, "fishing", 71),
	}
	if !reflect.DeepEqual(day.Events, want) {
		t.Fatalf("events = %+v\nwant     %+v", day.Events, want)
	}
	wantSkills := []coreactivity.SkillGain{
		{Skill: "fishing", FromLevel: 70, ToLevel: 76, XP: xp(76) + 20 - xp(70) - 10},
		{Skill: "farming", FromLevel: 33, ToLevel: 39, XP: xp(39) - xp(33) - 5},
	}
	if !reflect.DeepEqual(day.Skills, wantSkills) {
		t.Fatalf("skills = %+v\nwant     %+v", day.Skills, wantSkills)
	}

	// From 90 minutes ago, the baseline is the most seen before (the
	// plugin's 73), not the last observation (the hiscores' 72).
	recent, err := repo.Levels(ctx, "acc_l", at(90*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recent.Events, want[:3]) {
		t.Fatalf("recent events = %+v", recent.Events)
	}

	// From 40 hours ago there is no baseline: the full push only starts
	// each skill, so the events are the day's.
	long, err := repo.Levels(ctx, "acc_l", at(40*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(long.Events, want) || len(long.Skills) != 2 {
		t.Fatalf("long = %+v", long)
	}

	// An account without snapshots has no events.
	empty, err := repo.Levels(ctx, "acc_new", at(24*time.Hour))
	if err != nil || len(empty.Events) != 0 || len(empty.Skills) != 0 {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL activity integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("activity_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
