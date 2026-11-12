package progress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// TestIntegrityPostgres checks the invariants migration 0007 moved into the
// database (docs/10 sections 4 and 6): they must hold even when Go is wrong.
func TestIntegrityPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-integrity', 'dev-integrity');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc-a', 'user-1', 'A', 'a', 'ironman', 'g1', 'classic'),
		  ('acc-b', 'user-1', 'B', 'b', 'ironman', 'g1', 'classic');
		INSERT INTO snapshots (id, account_id, source, fetched_at, raw, raw_hash, adapter_version) VALUES
		  ('snap-a', 'acc-a', 'hiscores', now(), '{}', 'ha', 1),
		  ('snap-b', 'acc-b', 'hiscores', now(), '{}', 'hb', 1);
	`); err != nil {
		t.Fatalf("insert fixtures: %v", err)
	}

	const evidence = `[{"rule":"skill_level","verdict":"met"}]`
	rejected := []struct {
		name, code, sql string
	}{
		{"met with empty evidence", "23514", `INSERT INTO objective_progress (account_id, objective_key, verdict, evidence_snapshot_id, evidence, first_met_at)
			VALUES ('acc-a', 'skill:attack>=10', 'met', 'snap-a', '[]', now())`},
		{"evidence snapshot of another account", "23503", `INSERT INTO objective_progress (account_id, objective_key, verdict, evidence_snapshot_id, evidence, first_met_at)
			VALUES ('acc-a', 'skill:attack>=10', 'met', 'snap-b', '` + evidence + `', now())`},
		{"objective key without a kind", "23514", `INSERT INTO objective_progress (account_id, objective_key, verdict) VALUES ('acc-a', 'attack 10', 'unknown')`},
		{"goal key without a kind", "23514", `INSERT INTO goals (id, account_id, objective_key, label) VALUES ('goal-1', 'acc-a', '', 'x')`},
		{"fact snapshot of another account", "23503", `INSERT INTO player_state (account_id, facet, key, value, source, snapshot_id, observed_at)
			VALUES ('acc-a', 'skill', 'attack', '{"level":10}', 'hiscores', 'snap-b', now())`},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, tt.sql)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tt.code {
				t.Fatalf("err = %v, want SQLSTATE %s", err, tt.code)
			}
		})
	}

	// A met verdict is latched: only regressed_at may move.
	if _, err := pool.Exec(ctx, `INSERT INTO objective_progress (account_id, objective_key, verdict, evidence_snapshot_id, evidence, first_met_at)
		VALUES ('acc-a', 'qp>=1', 'met', 'snap-a', '`+evidence+`', now())`); err != nil {
		t.Fatalf("valid met verdict: %v", err)
	}
	q := storedb.New(pool)
	if err := q.SetObjectiveRegressed(ctx, storedb.SetObjectiveRegressedParams{
		AccountID: "acc-a", ObjectiveKey: "qp>=1", RegressedAt: timestamptz(ptrNow()),
	}); err != nil {
		t.Fatalf("regressed_at must stay writable: %v", err)
	}
	for _, sql := range []string{
		`UPDATE objective_progress SET verdict = 'unmet' WHERE objective_key = 'qp>=1'`,
		`UPDATE objective_progress SET evidence = '[{"rule":"other"}]' WHERE objective_key = 'qp>=1'`,
		`UPDATE objective_progress SET first_met_at = now() - interval '1 day' WHERE objective_key = 'qp>=1'`,
	} {
		var pgErr *pgconn.PgError
		if _, err := pool.Exec(ctx, sql); !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("%s: err = %v, want the latch to refuse it", sql, err)
		}
	}

	// The cascade never replaces the owner's mark.
	if err := q.UpsertStepMark(ctx, storedb.UpsertStepMarkParams{AccountID: "acc-a", GuideID: "g1", StepID: "a1", Kind: "manual_done"}); err != nil {
		t.Fatal(err)
	}
	n, err := q.InsertImpliedMark(ctx, storedb.InsertImpliedMarkParams{
		AccountID: "acc-a", GuideID: "g1", StepID: "a1", ImpliedByStepID: text("a3"), ImpliedAt: timestamptz(ptrNow()),
	})
	if err != nil || n != 0 {
		t.Fatalf("implied over an owner mark: rows %d, %v", n, err)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM step_marks WHERE step_id = 'a1' AND kind = 'manual_done'`); got != 1 {
		t.Fatal("the owner's mark was replaced")
	}
	n, err = q.InsertImpliedMark(ctx, storedb.InsertImpliedMarkParams{
		AccountID: "acc-a", GuideID: "g1", StepID: "a2", ImpliedByStepID: text("a3"), ImpliedAt: timestamptz(ptrNow()),
	})
	if err != nil || n != 1 {
		t.Fatalf("implied on a free step: rows %d, %v", n, err)
	}
}

func ptrNow() *time.Time {
	now := time.Now()
	return &now
}
