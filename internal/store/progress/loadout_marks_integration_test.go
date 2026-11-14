package progress

import (
	"context"
	"errors"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func TestLoadoutMarksPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO guides (id) VALUES ('lg') ON CONFLICT DO NOTHING;
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-loadout-marks', 'dev-loadout-marks');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'lg', 'classic'),
		       ('account-2', 'user-1', 'Other', 'other', 'ironman', 'lg', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	hammer, spade := 2347, 952
	seg := func(id string, inv ...content.ItemRequirement) *content.Segment {
		return &content.Segment{
			ID: id, Kind: content.SegmentBankLeg, Steps: []*content.Step{{ID: id + "-1", Text: "x"}},
			Loadout: &content.Loadout{Status: content.LoadoutPartial, Inventory: inv},
		}
	}
	hammerReq := content.ItemRequirement{Name: "Hammer", ItemID: &hammer}
	spadeReq := content.ItemRequirement{Name: "Spade", ItemID: &spade}
	// A leg without a loadout lists its items in its steps.
	bare := &content.Segment{ID: "seg-c", Kind: content.SegmentBankLeg, Steps: []*content.Step{
		{ID: "c1", Text: "dig", Requirements: []content.ItemRequirement{spadeReq}},
	}}
	mk := func(id string, segs ...*content.Segment) *content.Guide {
		g := &content.Guide{ID: id, Sections: []*content.Section{{ID: "s1", Slug: "s1", Segments: segs}}}
		g.Index()
		return g
	}
	cnt := &content.Content{Guides: []*content.Guide{
		mk("lg", seg("seg-a", hammerReq, spadeReq), seg("seg-b", spadeReq), bare),
		mk("lg2", seg("seg-a", spadeReq)),
	}}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, cnt, catalog)

	// Invalid targets are refused.
	for _, c := range []struct{ guide, segment, item string }{
		{"nope", "seg-a", "id:952"},
		{"lg", "nope", "id:952"},
		{"lg", "seg-a", "id:1"},
		{"lg", "seg-a", ""},
		{"lg2", "seg-b", "id:952"},
	} {
		if _, err = repo.MarkLoadoutItem(ctx, "account-1", c.guide, c.segment, c.item, ""); !errors.Is(err, ErrInvalidLoadoutItem) {
			t.Fatalf("mark %+v: %v", c, err)
		}
	}
	if _, err = repo.MarkLoadoutItem(ctx, "nobody", "lg", "seg-a", "id:952", ""); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("mark of an unknown account: %v", err)
	}

	// The mark is per (guide, segment): the spade of seg-a is not the spade of seg-b or of lg2.
	res, err := repo.MarkLoadoutItem(ctx, "account-1", "lg", "seg-a", "id:952", "already have it")
	if err != nil || !res.Marked || res.Item != "id:952" {
		t.Fatalf("mark = %+v, %v", res, err)
	}
	marks, err := repo.LoadoutMarks(ctx, "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if !marks.For("lg", "seg-a")["id:952"] || marks.For("lg", "seg-b") != nil || marks.For("lg2", "seg-a") != nil || len(marks.For("lg", "seg-a")) != 1 {
		t.Fatalf("marks = %+v", marks)
	}
	// A leg with no loadout takes marks on its steps' requirements.
	if _, err = repo.MarkLoadoutItem(ctx, "account-1", "lg", "seg-c", "id:952", ""); err != nil {
		t.Fatal(err)
	}
	// Another account is untouched.
	if other, oErr := repo.LoadoutMarks(ctx, "account-2"); oErr != nil || len(other) != 0 {
		t.Fatalf("other account marks = %+v, %v", other, oErr)
	}
	// Not evidence: no verdicts, no holdings.
	for _, table := range []string{"objective_progress", "player_state", "observations", "step_marks"} {
		if n := count(ctx, t, pool, `SELECT count(*) FROM `+table); n != 0 {
			t.Fatalf("%s has %d rows after a loadout mark", table, n)
		}
	}
	// Marking again keeps one row and updates the note.
	if _, err = repo.MarkLoadoutItem(ctx, "account-1", "lg", "seg-a", "id:952", "again"); err != nil {
		t.Fatal(err)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM loadout_marks WHERE note = 'again'`); n != 1 {
		t.Fatalf("notes = %d", n)
	}

	// Unmarking takes it back; unmarking twice, or what was never marked, changes nothing.
	if res, err = repo.UnmarkLoadoutItem(ctx, "account-1", "lg", "seg-a", "id:952"); err != nil || res.Marked {
		t.Fatalf("unmark = %+v, %v", res, err)
	}
	if _, err = repo.UnmarkLoadoutItem(ctx, "account-1", "lg", "seg-a", "id:952"); err != nil {
		t.Fatalf("second unmark: %v", err)
	}
	if marks, err = repo.LoadoutMarks(ctx, "account-1"); err != nil || marks.For("lg", "seg-a") != nil || !marks.For("lg", "seg-c")["id:952"] {
		t.Fatalf("marks after unmark = %+v, %v", marks, err)
	}
	if _, err = repo.UnmarkLoadoutItem(ctx, "nobody", "lg", "seg-a", "id:952"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("unmark of an unknown account: %v", err)
	}
}
