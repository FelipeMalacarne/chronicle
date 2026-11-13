package progress

import (
	"context"
	"errors"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

func TestObjectiveMarksPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-objective-marks', 'dev-objective-marks');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'g1', 'classic'),
		       ('account-2', 'user-1', 'Other', 'other', 'ironman', 'g1', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, loadContent(t), catalog)
	if _, err = repo.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReevaluateAll(ctx); err != nil {
		t.Fatal(err)
	}
	// The owner did the checkbox steps before the agility one.
	for _, step := range []string{"a1", "a2"} {
		if _, err = repo.Mark(ctx, "account-1", "g1", step, coreprogress.ActionManualDone, nil); err != nil {
			t.Fatal(err)
		}
	}
	assertCursor(ctx, t, repo, "g1", "a3", coreprogress.CursorAuto)

	// Something that is not an objective of any step is refused.
	for _, key := range []string{"nope", "skill:cooking>=5", ""} {
		if _, err = repo.MarkObjective(ctx, "account-1", key, ""); !errors.Is(err, ErrInvalidObjective) {
			t.Fatalf("mark %q: %v", key, err)
		}
	}
	if _, err = repo.MarkObjective(ctx, "nobody", "skill:agility>=10", ""); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("mark of an unknown account: %v", err)
	}

	// The mark closes a3 (g1) and c2 (g2) and moves the cursor on.
	res, err := repo.MarkObjective(ctx, "account-1", "skill:agility>=10", "did it long ago")
	if err != nil || !res.Marked || res.StepsChanged != 2 {
		t.Fatalf("mark = %+v, %v", res, err)
	}
	assertCursor(ctx, t, repo, "g1", "a4", coreprogress.CursorAuto)
	assertCursor(ctx, t, repo, "g2", "c1", coreprogress.CursorAuto)
	board, err := repo.Board(ctx, "account-1", "g1")
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, s := range board.Steps {
		if s.ID != "a3" {
			continue
		}
		seen = true
		o := s.Objectives[0]
		if s.Status != coreprogress.AutoDone || o.Verdict != rules.Met || !o.OwnerMarked {
			t.Fatalf("a3 = %s, objective %+v", s.Status, o)
		}
	}
	if !seen {
		t.Fatal("a3 not on the board")
	}
	// Not evidence: no stored verdict, no implied marks, events are the owner's.
	if n := count(ctx, t, pool, `SELECT count(*) FROM objective_progress`); n != 0 {
		t.Fatalf("the mark was written as a verdict: %d rows", n)
	}
	assertMarks(ctx, t, pool, "g1", map[string]string{"a1": "manual_done", "a2": "manual_done"})
	if n := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE cause = 'user' AND to_status = 'auto_done'`); n != 2 {
		t.Fatalf("user auto_done events = %d, want 2", n)
	}
	list, err := repo.Objectives(ctx, "account-1")
	if err != nil || len(list) != 1 || list[0].Key != "skill:agility>=10" || list[0].Verdict != "met" || !list[0].OwnerMarked || list[0].Evidence != nil {
		t.Fatalf("objectives = %+v, %v", list, err)
	}
	// Another account is untouched.
	assertCursorOf(ctx, t, repo, "account-2", "g1", "a1")
	assertConsistent(ctx, t, repo, "account-1")

	// Marking again changes nothing; unmarking restores the derived state.
	if res, err = repo.MarkObjective(ctx, "account-1", "skill:agility>=10", "again"); err != nil || res.StepsChanged != 0 {
		t.Fatalf("second mark = %+v, %v", res, err)
	}
	if res, err = repo.UnmarkObjective(ctx, "account-1", "skill:agility>=10"); err != nil || res.Marked || res.StepsChanged != 2 {
		t.Fatalf("unmark = %+v, %v", res, err)
	}
	assertCursor(ctx, t, repo, "g1", "a3", coreprogress.CursorAuto)
	if res, err = repo.UnmarkObjective(ctx, "account-1", "skill:agility>=10"); err != nil || res.StepsChanged != 0 {
		t.Fatalf("second unmark = %+v, %v", res, err)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM objective_marks`); n != 0 {
		t.Fatalf("marks left: %d", n)
	}
	assertConsistent(ctx, t, repo, "account-1")
	assertConsistent(ctx, t, repo, "account-2")
}

func assertCursorOf(ctx context.Context, t *testing.T, repo *Repository, account, guide, step string) {
	t.Helper()
	board, err := repo.Board(ctx, account, guide)
	if err != nil {
		t.Fatal(err)
	}
	if board.Cursor == nil || board.Cursor.StepID != step {
		t.Fatalf("%s %s cursor = %+v, want %s", account, guide, board.Cursor, step)
	}
}
