package ingest

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

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

func TestRepositoryPostgresIngestLifecycle(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL ingest integration test skipped")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err = admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping admin database: %v", err)
	}

	schema := fmt.Sprintf("ingest_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
		if _, err = admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
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

	applyTestMigrations(ctx, t, pool)
	if _, err = pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-ingest-test', 'dev-ingest-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'guide-1');
	`); err != nil {
		t.Fatalf("insert account fixture: %v", err)
	}

	repository := New(pool, Definition{
		Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{},
		IntervalSeconds: 10,
	})
	created, err := repository.CreateToken(ctx, "account-1", "desktop", []coreingest.TokenScope{coreingest.ScopeItemsBank})
	if err != nil {
		t.Fatalf("create ingest token: %v", err)
	}
	manifest, err := repository.Manifest(ctx, created.Token)
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if !manifest.Items.Bank || manifest.Items.Inventory || manifest.Items.Equipment || !manifest.Resync {
		t.Fatalf("manifest privacy/resync = %+v", manifest)
	}

	full := repositoryFullPush(1, repositoryRestored(4, map[string]int64{"4151": 1}))
	rawFull, _ := json.Marshal(full)
	response, err := repository.Accept(ctx, created.Token, "127.0.0.1", rawFull, full)
	if err != nil || !response.OK || response.AcceptedSeq != 1 || response.Resync {
		t.Fatalf("accept restored full = %+v, %v", response, err)
	}
	manifest, err = repository.Manifest(ctx, created.Token)
	if err != nil || manifest.Resync {
		t.Fatalf("manifest after baseline full = %+v, %v", manifest, err)
	}

	var state string
	var liveEpoch *string
	if err = pool.QueryRow(ctx, `
		SELECT state, live_epoch FROM ingest_container_freshness
		WHERE account_id = 'account-1' AND container = 'bank'
	`).Scan(&state, &liveEpoch); err != nil {
		t.Fatalf("read restored freshness: %v", err)
	}
	if state != "restored" || liveEpoch != nil {
		t.Fatalf("restored freshness = state %q epoch %v", state, liveEpoch)
	}
	var value []byte
	if err = pool.QueryRow(ctx, `
		SELECT value FROM player_state
		WHERE account_id = 'account-1' AND facet = 'item' AND key = 'bank:4151'
	`).Scan(&value); err != nil {
		t.Fatalf("read restored item fact: %v", err)
	}
	var restoredValue map[string]any
	if err = json.Unmarshal(value, &restoredValue); err != nil {
		t.Fatal(err)
	}
	if restoredValue["container_state"] != "restored" || restoredValue["live_epoch"] != nil {
		t.Fatalf("restored item value = %s", value)
	}
	assertHoldings(ctx, t, repository, map[string]wantContainer{
		"bank":      {state: coreingest.ItemRestored, complete: true, items: map[int]int64{4151: 1}},
		"inventory": {state: coreingest.ItemUnavailable},
		"equipment": {state: coreingest.ItemUnavailable},
	})
	var unavailableCount int
	if err = pool.QueryRow(ctx, `
		SELECT count(*) FROM snapshot_item_containers WHERE state = 'unavailable'
	`).Scan(&unavailableCount); err != nil || unavailableCount != 2 {
		t.Fatalf("unavailable observations = %d, %v", unavailableCount, err)
	}

	delta := repositoryDeltaPush(2, repositoryLive(5, "login-1", map[string]int64{"4151": 0, "995": 250}))
	rawDelta, _ := json.Marshal(delta)
	response, err = repository.Accept(ctx, created.Token, "127.0.0.1", rawDelta, delta)
	if err != nil || !response.OK || response.AcceptedSeq != 2 {
		t.Fatalf("accept live delta = %+v, %v", response, err)
	}
	if err = pool.QueryRow(ctx, `
		SELECT state, live_epoch FROM ingest_container_freshness
		WHERE account_id = 'account-1' AND container = 'bank'
	`).Scan(&state, &liveEpoch); err != nil {
		t.Fatalf("read live freshness: %v", err)
	}
	if state != "live" || liveEpoch == nil || *liveEpoch != "login-1" {
		t.Fatalf("live freshness = state %q epoch %v", state, liveEpoch)
	}
	var oldFactCount, tombstoneCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM player_state WHERE account_id = 'account-1' AND facet = 'item' AND key = 'bank:4151'`).Scan(&oldFactCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM snapshot_items WHERE container = 'bank' AND item_id = 4151 AND qty = 0`).Scan(&tombstoneCount); err != nil {
		t.Fatal(err)
	}
	if oldFactCount != 0 || tombstoneCount != 1 {
		t.Fatalf("delta tombstone: current facts=%d snapshot rows=%d", oldFactCount, tombstoneCount)
	}

	// A live delta that was not preceded by a live full in the same epoch
	// proves presence only: the restored item it tombstoned is gone, and the
	// container is not complete.
	assertHoldings(ctx, t, repository, map[string]wantContainer{
		"bank":      {state: coreingest.ItemLive, complete: false, items: map[int]int64{995: 250}},
		"inventory": {state: coreingest.ItemUnavailable},
		"equipment": {state: coreingest.ItemUnavailable},
	})

	response, err = repository.Accept(ctx, created.Token, "127.0.0.1", rawDelta, delta)
	if !errors.Is(err, ErrStaleSequence) || response.OK || response.AcceptedSeq != 2 || !response.Resync {
		t.Fatalf("duplicate response = %+v, %v", response, err)
	}
	assertSnapshotCount(ctx, t, pool, 2)

	// A second install has an independent counter. Its seq=1 and even its
	// identical raw body are valid for the same account.
	second, err := repository.CreateToken(ctx, "account-1", "laptop", []coreingest.TokenScope{coreingest.ScopeItemsBank})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Accept(ctx, second.Token, "127.0.0.2", rawFull, full); err != nil {
		t.Fatalf("second token sequence: %v", err)
	}
	assertSnapshotCount(ctx, t, pool, 3)
	// The second install's restored full was observed before the live delta,
	// so it must not regress the newer live observation or its facts.
	assertHoldings(ctx, t, repository, map[string]wantContainer{
		"bank":      {state: coreingest.ItemLive, complete: false, items: map[int]int64{995: 250}},
		"inventory": {state: coreingest.ItemUnavailable},
		"equipment": {state: coreingest.ItemUnavailable},
	})
	liveFull := repositoryFullPush(3, repositoryLive(6, "login-1", map[string]int64{"995": 300, "2347": 1}))
	rawLiveFull, _ := json.Marshal(liveFull)
	if _, err = repository.Accept(ctx, created.Token, "127.0.0.1", rawLiveFull, liveFull); err != nil {
		t.Fatalf("accept live full: %v", err)
	}
	// A live full in the current epoch is a baseline: absence is now provable.
	assertHoldings(ctx, t, repository, map[string]wantContainer{
		"bank":      {state: coreingest.ItemLive, complete: true, items: map[int]int64{995: 300, 2347: 1}},
		"inventory": {state: coreingest.ItemUnavailable},
		"equipment": {state: coreingest.ItemUnavailable},
	})
	assertSnapshotCount(ctx, t, pool, 4)
	if _, err = repository.ItemHoldings(ctx, "no-such-account"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("unknown account holdings error = %v", err)
	}

	noBank, err := repository.CreateToken(ctx, "account-1", "private", nil)
	if err != nil {
		t.Fatal(err)
	}
	privateManifest, err := repository.Manifest(ctx, noBank.Token)
	if err != nil {
		t.Fatal(err)
	}
	if privateManifest.Items.Bank || privateManifest.Items.Inventory || privateManifest.Items.Equipment {
		t.Fatalf("default token exposed item containers: %+v", privateManifest.Items)
	}
	if _, err = repository.Accept(ctx, noBank.Token, "", rawFull, full); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("disabled bank error = %v", err)
	}
	assertSnapshotCount(ctx, t, pool, 4)

	if _, err = pool.Exec(ctx, `UPDATE plugin_ingest_tokens SET revoked_at = now() WHERE id = $1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Manifest(ctx, second.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked manifest error = %v", err)
	}
}

func applyTestMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "db", "migrations", "*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("list migrations: %v", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		name := filepath.Base(path)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		up, _, found := strings.Cut(string(raw), "-- +goose Down")
		if !found {
			t.Fatalf("%s has no goose Down marker", name)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

type wantContainer struct {
	state    coreingest.ItemState
	complete bool
	items    map[int]int64
}

func assertHoldings(ctx context.Context, t *testing.T, repository *Repository, want map[string]wantContainer) {
	t.Helper()
	got, err := repository.ItemHoldings(ctx, "account-1")
	if err != nil {
		t.Fatalf("item holdings: %v", err)
	}
	{
		if len(got) != len(want) {
			t.Fatalf("holdings containers = %v, want %v", got, want)
		}
		for name, w := range want {
			c, ok := got[name]
			if !ok || c.State != w.state || c.Complete != w.complete || len(c.Quantities) != len(w.items) {
				t.Fatalf("%s = %+v, want %+v", name, c, w)
			}
			for id, qty := range w.items {
				if c.Quantities[id] != qty {
					t.Fatalf("%s[%d] = %d, want %d", name, id, c.Quantities[id], qty)
				}
			}
		}
	}
}

func assertSnapshotCount(ctx context.Context, t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM snapshots`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("snapshots = %d, want %d", got, want)
	}
}

func repositoryIntMap(values map[string]int64) *coreingest.IntMap {
	value := coreingest.IntMap(values)
	return &value
}

func repositoryQuantityMap(values map[string]int64) *coreingest.QuantityMap {
	value := coreingest.QuantityMap(values)
	return &value
}

func repositoryUnavailable(at time.Time) *coreingest.ItemObservation {
	return &coreingest.ItemObservation{State: coreingest.ItemUnavailable, ObservedAt: at}
}

func repositoryRestored(revision int64, values map[string]int64) coreingest.ItemObservation {
	return coreingest.ItemObservation{State: coreingest.ItemRestored, Revision: &revision, Quantities: repositoryQuantityMap(values)}
}

func repositoryLive(revision int64, epoch string, values map[string]int64) coreingest.ItemObservation {
	return coreingest.ItemObservation{State: coreingest.ItemLive, Revision: &revision, LiveEpoch: epoch, Quantities: repositoryQuantityMap(values)}
}

func repositoryFullPush(seq int64, bank coreingest.ItemObservation) coreingest.Push {
	sentAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
	bank.ObservedAt = sentAt.Add(-time.Second)
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion,
		Username:        "Example",
		Profile:         "STANDARD",
		ManifestVersion: 7,
		ClientSeq:       seq,
		Kind:            coreingest.PushFull,
		SentAt:          sentAt,
		Data: coreingest.PushData{
			Varbits: repositoryIntMap(map[string]int64{}), Varps: repositoryIntMap(map[string]int64{}),
			Levels: repositoryIntMap(map[string]int64{}), XP: repositoryIntMap(map[string]int64{}), Boosted: repositoryIntMap(map[string]int64{}),
			Items: &coreingest.ItemContainers{Bank: &bank, Inventory: repositoryUnavailable(sentAt.Add(-time.Second)), Equipment: repositoryUnavailable(sentAt.Add(-time.Second))},
		},
	}
}

func repositoryDeltaPush(seq int64, bank coreingest.ItemObservation) coreingest.Push {
	sentAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
	bank.ObservedAt = sentAt.Add(-time.Second)
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion,
		Username:        "Example",
		Profile:         "STANDARD",
		ManifestVersion: 7,
		ClientSeq:       seq,
		Kind:            coreingest.PushDelta,
		SentAt:          sentAt,
		Data:            coreingest.PushData{Items: &coreingest.ItemContainers{Bank: &bank}},
	}
}
