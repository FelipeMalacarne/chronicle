package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMultiGuideMigrationAndQueryIsolation(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL multi-guide integration test skipped")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping admin database: %v", err)
	}
	schema := "multiguide_test_" + strings.ReplaceAll(time.Now().UTC().Format("20060102_150405.000000000"), ".", "_")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
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

	applyMigration(t, ctx, pool, "0001_init.sql")
	applyMigration(t, ctx, pool, "0002_ingest_container_freshness.sql")
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-multiguide-test', 'dev-multiguide-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'oziris-main');
		INSERT INTO content_versions (id, content_hash, guide_version, step_count)
		VALUES ('version-o', 'hash-o', 'legacy', 1);
		INSERT INTO content_steps
		  (step_id, guide_id, section_id, position, rule_hash, is_manual, layers, modes, content_version_id)
		VALUES ('shared-step', 'oziris-main', 'shared-section', 1, 'rule-o', true, '[]', '[]', 'version-o');
		INSERT INTO step_id_aliases (old_step_id, new_step_id)
		VALUES ('old-shared', 'shared-step');
		INSERT INTO step_progress (account_id, step_id, status)
		VALUES ('account-1', 'shared-step', 'manual_done');
		INSERT INTO step_progress_events (id, account_id, step_id, to_status, cause)
		VALUES ('event-o', 'account-1', 'shared-step', 'manual_done', 'user');
	`); err != nil {
		t.Fatalf("insert legacy fixtures: %v", err)
	}

	applyMigration(t, ctx, pool, "0003_multiguide_identity.sql")
	queries := New(pool)
	legacy, err := queries.GetStepProgress(ctx, GetStepProgressParams{
		AccountID: "account-1", GuideID: "oziris-main", StepID: "shared-step",
	})
	if err != nil || legacy.Status != "manual_done" {
		t.Fatalf("legacy progress = %+v, %v", legacy, err)
	}
	legacyAlias, err := queries.ResolveStepIDAlias(ctx, ResolveStepIDAliasParams{GuideID: "oziris-main", OldStepID: "old-shared"})
	if err != nil || legacyAlias.NewStepID != "shared-step" {
		t.Fatalf("legacy alias = %+v, %v", legacyAlias, err)
	}
	legacyEvents, err := queries.ListRecentStepProgressEvents(ctx, ListRecentStepProgressEventsParams{
		AccountID: "account-1", GuideID: "oziris-main",
		CreatedAt: pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, Limit: 10,
	})
	if err != nil || len(legacyEvents) != 1 || legacyEvents[0].GuideID != "oziris-main" {
		t.Fatalf("legacy events = %+v, %v", legacyEvents, err)
	}

	if _, err := queries.InsertContentVersion(ctx, InsertContentVersionParams{
		ID: "version-b", ContentHash: "hash-b", GuideVersion: pgtype.Text{String: "imported", Valid: true}, StepCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.UpsertContentStep(ctx, UpsertContentStepParams{
		GuideID: "bruhsailer-main", StepID: "shared-step", SectionID: "shared-section", Position: 1,
		RuleHash: "rule-b", IsManual: true, Layers: []byte(`[]`), Modes: []byte(`[]`), ContentVersionID: "version-b",
	}); err != nil {
		t.Fatalf("insert colliding content step: %v", err)
	}
	if _, err := queries.SetStepManualStatus(ctx, SetStepManualStatusParams{
		AccountID: "account-1", GuideID: "bruhsailer-main", StepID: "shared-step", Status: "skipped", ManualLocked: true,
	}); err != nil {
		t.Fatalf("insert BruhSailer progress: %v", err)
	}

	oziris, err := queries.GetStepProgress(ctx, GetStepProgressParams{AccountID: "account-1", GuideID: "oziris-main", StepID: "shared-step"})
	if err != nil {
		t.Fatal(err)
	}
	bruh, err := queries.GetStepProgress(ctx, GetStepProgressParams{AccountID: "account-1", GuideID: "bruhsailer-main", StepID: "shared-step"})
	if err != nil {
		t.Fatal(err)
	}
	if oziris.Status != "manual_done" || bruh.Status != "skipped" {
		t.Fatalf("cross-guide progress leaked: oziris=%q bruh=%q", oziris.Status, bruh.Status)
	}
	for _, tc := range []struct {
		guide, want string
	}{
		{guide: "oziris-main", want: "manual_done"},
		{guide: "bruhsailer-main", want: "skipped"},
	} {
		board, err := queries.ListStepBoard(ctx, ListStepBoardParams{AccountID: "account-1", GuideID: tc.guide})
		if err != nil || len(board) != 1 || board[0].StepID != "shared-step" || board[0].Status != tc.want {
			t.Fatalf("%s board = %+v, %v", tc.guide, board, err)
		}
	}

	if _, err := queries.UpsertStepIDAlias(ctx, UpsertStepIDAliasParams{
		GuideID: "bruhsailer-main", OldStepID: "old-shared", NewStepID: "shared-step",
	}); err != nil {
		t.Fatalf("insert colliding alias: %v", err)
	}
	aliases := 0
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM step_id_aliases WHERE old_step_id = 'old-shared'`).Scan(&aliases); err != nil || aliases != 2 {
		t.Fatalf("colliding aliases = %d, %v", aliases, err)
	}
}

func applyMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "db", "migrations", name)
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
