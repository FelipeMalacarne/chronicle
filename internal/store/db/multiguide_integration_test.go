package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
	if _, err := pool.Exec(ctx, `
		INSERT INTO step_progress (account_id, guide_id, step_id, status, manual_locked, user_note)
		VALUES ('account-1', 'bruhsailer-main', 'shared-step', 'skipped', true, 'later'),
		       ('account-1', 'oziris-main', 'auto-step', 'auto_done', false, NULL);
	`); err != nil {
		t.Fatalf("insert multi-guide progress: %v", err)
	}
	applyMigration(t, ctx, pool, "0004_objective_progress.sql")
	queries := New(pool)

	for _, tc := range []struct {
		guide, kind, note string
	}{
		{guide: "oziris-main", kind: "manual_done"},
		{guide: "bruhsailer-main", kind: "skipped", note: "later"},
	} {
		marks, err := queries.ListStepMarksByGuide(ctx, ListStepMarksByGuideParams{AccountID: "account-1", GuideID: tc.guide})
		if err != nil || len(marks) != 1 || marks[0].StepID != "shared-step" || marks[0].Kind != tc.kind || marks[0].Note.String != tc.note {
			t.Fatalf("%s marks = %+v, %v", tc.guide, marks, err)
		}
	}
	last, err := queries.ListLastStepStatusesByGuide(ctx, ListLastStepStatusesByGuideParams{AccountID: "account-1", GuideID: "oziris-main"})
	if err != nil || len(last) != 1 || last[0].StepID != "shared-step" || last[0].ToStatus != "manual_done" {
		t.Fatalf("legacy events = %+v, %v", last, err)
	}
	legacyAlias, err := queries.ResolveStepIDAlias(ctx, ResolveStepIDAliasParams{GuideID: "oziris-main", OldStepID: "old-shared"})
	if err != nil || legacyAlias.NewStepID != "shared-step" {
		t.Fatalf("legacy alias = %+v, %v", legacyAlias, err)
	}

	version, err := queries.UpsertContentVersion(ctx, UpsertContentVersionParams{ID: "version-b", ContentHash: "hash-b", Summary: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertContentSteps(ctx, []InsertContentStepsParams{
		{GuideID: "oziris-main", StepID: "shared-step", SectionID: "shared-section", Position: 1, TextHash: "sha256:o", ContentVersionID: version.ID},
		{GuideID: "bruhsailer-main", StepID: "shared-step", SectionID: "shared-section", Position: 1, TextHash: "sha256:b", ContentVersionID: version.ID},
	}); err != nil {
		t.Fatalf("insert colliding content steps: %v", err)
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
	for _, guide := range []string{"oziris-main", "bruhsailer-main"} {
		orphans, err := queries.ListOrphanStepMarks(ctx, ListOrphanStepMarksParams{AccountID: "account-1", GuideID: guide})
		if err != nil || len(orphans) != 0 {
			t.Fatalf("%s orphans = %+v, %v", guide, orphans, err)
		}
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
