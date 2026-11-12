package progress

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
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/events"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
)

// recorder keeps the published events.
type recorder struct{ got []events.Event }

func (r *recorder) Publish(e events.Event) { r.got = append(r.got, e) }

func TestProgressPostgresLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-progress-test', 'dev-progress-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'g1', 'classic');
		-- Progress recorded under an id the guide now declares as replaced.
		INSERT INTO step_marks (account_id, guide_id, step_id, kind, note) VALUES ('account-1', 'g1', 'a2-old', 'manual_done', 'by hand');
		-- As migration 0009 seeds it: the existing mark is the start of the log.
		INSERT INTO mark_commands (id, account_id, guide_id, kind, actor, effects) VALUES ('mc-seed', 'account-1', 'g1', 'migration', 'system',
		  '[{"step_id":"a2-old","from":null,"to":{"kind":"manual_done","note":"by hand"}}]');
		INSERT INTO step_status_events (id, account_id, guide_id, step_id, from_status, to_status, cause)
		VALUES ('sse-legacy', 'account-1', 'g1', 'a2-old', 'pending', 'manual_done', 'user');
		INSERT INTO route_cursor (account_id, guide_id, step_id, cause) VALUES ('account-1', 'g1', 'a2-old', 'pin');
	`); err != nil {
		t.Fatalf("insert fixtures: %v", err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, loadContent(t), catalog)

	// Boot: index the content and apply the alias once.
	report, err := repo.IndexContent(ctx, "test")
	if err != nil || report.Steps != 8 || report.AliasesApplied != 1 || len(report.Warnings) != 0 {
		t.Fatalf("index = %+v, %v", report, err)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM step_marks WHERE guide_id = 'g1' AND step_id = 'a2' AND note = 'by hand'`); got != 1 {
		t.Fatalf("mark not moved: %d", got)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE step_id = 'a2-old'`); got != 0 {
		t.Fatalf("events left behind: %d", got)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM route_cursor WHERE step_id = 'a2' AND cause = 'pin'`); got != 1 {
		t.Fatalf("cursor not moved: %d", got)
	}
	again, err := repo.IndexContent(ctx, "test")
	if err != nil || again.AliasesApplied != 0 || again.ContentVersionID != report.ContentVersionID {
		t.Fatalf("second index = %+v, %v", again, err)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE cause = 'migration'`); got != 1 {
		t.Fatalf("migration events = %d, want 1 (alias applied once)", got)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM content_versions`); got != 1 {
		t.Fatalf("content versions = %d", got)
	}

	// Reevaluation without facts: nothing is met; the pin on a done step drops.
	if n, reErr := repo.ReevaluateAll(ctx); reErr != nil || n != 1 {
		t.Fatalf("reevaluate all = %d, %v", n, reErr)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM objective_progress`); got != 0 {
		t.Fatalf("unknown objectives were written: %d", got)
	}
	assertCursor(ctx, t, repo, "g1", "a1", coreprogress.CursorAuto)

	// A plugin push reevaluates both guides in the ingest transaction.
	ingest := storeingest.New(pool, storeingest.Definition{
		Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10,
	}).WithReevaluator(repo)
	token, err := ingest.CreateToken(ctx, "account-1", "desktop", []coreingest.TokenScope{coreingest.ScopeItemsBank})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.Manifest(ctx, token.Token); err != nil {
		t.Fatal(err)
	}
	// Agility and the feathers were observed unmet before (an earlier source):
	// reaching them now is progress, so the cascade runs. A first observation
	// would be history and cascade nothing (core test
	// TestReevaluateFirstObservationDoesNotCascade).
	if _, err = pool.Exec(ctx, `
		INSERT INTO objective_progress (account_id, objective_key, verdict, evidence)
		VALUES ('account-1', 'skill:agility>=10', 'unmet', '[]'), ('account-1', 'item:314x120', 'unmet', '[]')
	`); err != nil {
		t.Fatal(err)
	}
	resp := push(ctx, t, ingest, token.Token, fullPush(1, map[string]int64{"agility": 10}, map[string]int64{"314": 120}))
	if resp.StepsChanged == nil || *resp.StepsChanged != 3 {
		t.Fatalf("steps_changed = %v, want 3 (a3, a5 in g1 and c2 in g2)", resp.StepsChanged)
	}
	// The response's route is the active guide's: its new cursor and every
	// step whose status changed there, implied ones included, in route order.
	if r := resp.Route; r == nil || r.Guide != "g1" || r.Cursor != "a6" || r.CursorCause != coreprogress.CursorAuto ||
		strings.Join(r.Changed, ",") != "a1,a3,a4,a5" {
		t.Fatalf("route = %+v, want g1 cursor a6 with a1,a3,a4,a5 changed", r)
	}
	firstSnapshot := latestSnapshot(ctx, t, pool)
	var (
		verdict, snapshot, metIn string
		evidence                 []byte
	)
	if err = pool.QueryRow(ctx, `
		SELECT verdict, evidence_snapshot_id, met_in_guide_id, evidence FROM objective_progress
		WHERE account_id = 'account-1' AND objective_key = 'skill:agility>=10'
	`).Scan(&verdict, &snapshot, &metIn, &evidence); err != nil {
		t.Fatal(err)
	}
	if verdict != "met" || snapshot != firstSnapshot || metIn != "g1" || !strings.Contains(string(evidence), `"facet": "skill"`) {
		t.Fatalf("agility = %s %s %s %s", verdict, snapshot, metIn, evidence)
	}
	assertMarks(ctx, t, pool, "g1", map[string]string{"a1": "implied:a3", "a2": "manual_done", "a4": "implied:a5"})
	assertMarks(ctx, t, pool, "g2", map[string]string{})
	assertCursor(ctx, t, repo, "g1", "a6", coreprogress.CursorAuto)
	assertCursor(ctx, t, repo, "g2", "c1", coreprogress.CursorAuto)
	if got := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE cause = 'engine' AND snapshot_id = '`+firstSnapshot+`'`); got != 3 {
		t.Fatalf("engine events = %d", got)
	}

	// The same facts again change nothing.
	resp = push(ctx, t, ingest, token.Token, fullPush(2, map[string]int64{"agility": 10}, map[string]int64{"314": 120}))
	if resp.StepsChanged == nil || *resp.StepsChanged != 0 {
		t.Fatalf("steps_changed on an identical push = %v", resp.StepsChanged)
	}
	if r := resp.Route; r == nil || r.Cursor != "a6" || len(r.Changed) != 0 {
		t.Fatalf("route on an identical push = %+v, want the same cursor and nothing changed", r)
	}

	// Spending the feathers keeps a5 done and marks the objective regressed.
	resp = push(ctx, t, ingest, token.Token, fullPush(3, map[string]int64{"agility": 10}, map[string]int64{"314": 3}))
	if resp.StepsChanged == nil || *resp.StepsChanged != 0 {
		t.Fatalf("steps_changed after spending items = %v", resp.StepsChanged)
	}
	var regressed *time.Time
	if err = pool.QueryRow(ctx, `
		SELECT verdict, evidence_snapshot_id, regressed_at FROM objective_progress
		WHERE account_id = 'account-1' AND objective_key = 'item:314x120'
	`).Scan(&verdict, &snapshot, &regressed); err != nil {
		t.Fatal(err)
	}
	if verdict != "met" || snapshot != firstSnapshot || regressed == nil {
		t.Fatalf("feathers = %s %s %v", verdict, snapshot, regressed)
	}

	// Owner marks; each committed one publishes an owner event for its guide.
	rec := &recorder{}
	repo.WithPublisher(rec)
	board, err := repo.Mark(ctx, "account-1", "g1", "a6", coreprogress.ActionManualDone, nil)
	if err != nil || board.Cursor != nil || board.Counts.Done != 6 {
		t.Fatalf("mark a6 = cursor %+v counts %+v, %v", board.Cursor, board.Counts, err)
	}
	if len(rec.got) != 1 || rec.got[0].Source != events.SourceOwner || rec.got[0].Account != "account-1" ||
		len(rec.got[0].Guides) != 1 || rec.got[0].Guides[0] != "g1" {
		t.Fatalf("events after a mark = %+v", rec.got)
	}
	board, err = repo.Mark(ctx, "account-1", "g1", "a6", coreprogress.ActionClear, nil)
	if err != nil || board.Cursor == nil || board.Cursor.StepID != "a6" {
		t.Fatalf("clear a6 = %+v, %v", board.Cursor, err)
	}
	published := len(rec.got)
	if _, err = repo.Mark(ctx, "account-1", "g1", "a6", coreprogress.ActionConfirmImplied, nil); !errors.Is(err, coreprogress.ErrNotImplied) {
		t.Fatalf("confirm on a checkbox: %v", err)
	}
	if len(rec.got) != published {
		t.Fatalf("a refused mark published %+v", rec.got[published:])
	}
	board, err = repo.ConfirmImplied(ctx, "account-1", "g1")
	if err != nil || board.Counts.Implied != 0 || board.Counts.Manual != 3 {
		t.Fatalf("confirm all = %+v, %v", board.Counts, err)
	}
	assertMarks(ctx, t, pool, "g1", map[string]string{"a1": "manual_done", "a2": "manual_done", "a4": "manual_done"})
	if got := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE cause = 'user' AND from_status = 'implied' AND to_status = 'manual_done'`); got != 2 {
		t.Fatalf("confirm events = %d", got)
	}
	board, err = repo.PinCursor(ctx, "account-1", "g1", "a6")
	if err != nil || board.Cursor == nil || board.Cursor.Cause != coreprogress.CursorPin {
		t.Fatalf("pin = %+v, %v", board.Cursor, err)
	}
	if _, err = repo.PinCursor(ctx, "account-1", "g1", "a3"); !errors.Is(err, coreprogress.ErrStepDone) {
		t.Fatalf("pin a done step: %v", err)
	}
	board, err = repo.PinCursor(ctx, "account-1", "g1", "")
	if err != nil || board.Cursor == nil || board.Cursor.Cause != coreprogress.CursorAuto {
		t.Fatalf("unpin = %+v, %v", board.Cursor, err)
	}

	// The other route shares the verdict but keeps its own marks and cursor.
	g2, err := repo.Board(ctx, "account-1", "g2")
	if err != nil || g2.Active || g2.Counts.AutoDone != 1 || g2.Counts.Done != 1 || g2.Cursor.StepID != "c1" {
		t.Fatalf("g2 board = %+v, %v", g2, err)
	}
	objectives, err := repo.Objectives(ctx, "account-1")
	if err != nil || len(objectives) < 2 {
		t.Fatalf("objectives = %+v, %v", objectives, err)
	}

	events, err := repo.Events(ctx, "account-1", "g1", 50)
	if err != nil || len(events.Steps) == 0 || len(events.Objectives) != 2 {
		t.Fatalf("events = %+v, %v", events, err)
	}
	if events.Objectives[0].Source != "plugin" {
		t.Fatalf("objective event source = %q", events.Objectives[0].Source)
	}

	// Goals (F9): canonical keys of catalog entries only; one per objective;
	// achieved when the objective is met, now or at a later reevaluation.
	if _, err = repo.AddGoal(ctx, "account-1", "quest:nope@finished", ""); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("unknown entity goal: %v", err)
	}
	if _, err = repo.AddGoal(ctx, "account-1", "skill:agility:70", ""); !errors.Is(err, ErrInvalidGoal) {
		t.Fatalf("old grammar goal: %v", err)
	}
	metGoal, err := repo.AddGoal(ctx, "account-1", "skill:agility>=10", "Agility 10")
	if err != nil || metGoal.Status != "achieved" || metGoal.Verdict != "met" {
		t.Fatalf("goal already met = %+v, %v", metGoal, err)
	}
	attack, err := repo.AddGoal(ctx, "account-1", "skill:attack>=5", "")
	if err != nil || attack.Status != "active" || attack.Verdict != "unknown" {
		t.Fatalf("open goal = %+v, %v", attack, err)
	}
	sameGoal, err := repo.AddGoal(ctx, "account-1", "skill:attack>=5", "")
	if err != nil || sameGoal.ID != attack.ID {
		t.Fatalf("pinning twice = %+v, %v", sameGoal, err)
	}
	pushLevels := fullPush(4, map[string]int64{"agility": 10, "attack": 6}, map[string]int64{"314": 3})
	push(ctx, t, ingest, token.Token, pushLevels)
	goals, err := repo.Goals(ctx, "account-1")
	if err != nil || len(goals) != 2 {
		t.Fatalf("goals = %+v, %v", goals, err)
	}
	for _, g := range goals {
		if g.Status != "achieved" || g.AchievedAt == nil {
			t.Fatalf("goal not achieved by the push: %+v", g)
		}
	}
	if err = repo.RemoveGoal(ctx, "account-1", attack.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.RemoveGoal(ctx, "account-1", attack.ID); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("removing twice: %v", err)
	}

	sources, err := repo.Sources(ctx, "account-1")
	if err != nil || len(sources) != 1 || sources[0].Source != "plugin" || sources[0].Facts == 0 {
		t.Fatalf("sources = %+v, %v", sources, err)
	}

	// Switching the active route keeps every cursor and reevaluates.
	if err = repo.SetActiveGuide(ctx, "account-1", "g2"); err != nil {
		t.Fatal(err)
	}
	g2, err = repo.Board(ctx, "account-1", "g2")
	if err != nil || !g2.Active || g2.Cursor.StepID != "c1" {
		t.Fatalf("g2 after switch = active %v cursor %+v, %v", g2.Active, g2.Cursor, err)
	}
	if err = repo.SetActiveGuide(ctx, "account-1", "nope"); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("switch to an unknown guide: %v", err)
	}

	for name, err := range map[string]error{
		"unknown guide":   errOf(repo.Board(ctx, "account-1", "nope")),
		"unknown account": errOf(repo.Board(ctx, "nobody", "g1")),
		"unknown step":    errOf(repo.Mark(ctx, "account-1", "g1", "zz", coreprogress.ActionManualDone, nil)),
	} {
		want := map[string]error{"unknown guide": ErrGuideNotFound, "unknown account": ErrAccountNotFound, "unknown step": coreprogress.ErrStepNotFound}[name]
		if !errors.Is(err, want) {
			t.Fatalf("%s: %v", name, err)
		}
	}

	// After aliases, owner marks, the implied cascade and its confirmation,
	// step_marks is exactly the replay of the mark log (docs/10 section 6).
	if diffs, err := repo.VerifyMarks(ctx, "account-1"); err != nil || len(diffs) != 0 {
		t.Fatalf("mark log replay: %v, %v", diffs, err)
	}
	for actor, min := range map[string]int{"owner": 1, "engine": 1, "system": 2} {
		if n := count(ctx, t, pool, `SELECT count(*) FROM mark_commands WHERE actor = '`+actor+`'`); n < min {
			t.Fatalf("%s commands = %d, want at least %d", actor, n, min)
		}
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE cause IN ('user', 'implied', 'migration') AND command_id IS NULL AND id <> 'sse-legacy'`); n != 0 {
		t.Fatalf("%d owner, implied or migration events without their command", n)
	}
	assertConsistent(ctx, t, repo, "account-1")
}

// assertConsistent runs `chronicle progress check` on one account: every
// projection must match its sources (docs/10 section 6, decision E3).
func assertConsistent(ctx context.Context, t *testing.T, repo *Repository, accountID string) {
	t.Helper()
	rep, err := repo.Check(ctx, accountID)
	if err != nil || !rep.OK {
		t.Fatalf("progress check of %s: %v, %v", accountID, rep.Differences, err)
	}
}

func errOf(_ BoardView, err error) error { return err }

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

func push(ctx context.Context, t *testing.T, repo *storeingest.Repository, token string, p coreingest.Push) coreingest.Response {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := repo.Accept(ctx, token, "127.0.0.1", raw, p)
	if err != nil || !resp.OK {
		t.Fatalf("accept seq %d = %+v, %v", p.ClientSeq, resp, err)
	}
	return resp
}

func fullPush(seq int64, levels, bank map[string]int64) coreingest.Push {
	sentAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
	revision := seq
	quantities := coreingest.QuantityMap(bank)
	empty := coreingest.IntMap{}
	lv := coreingest.IntMap(levels)
	unavailable := func() *coreingest.ItemObservation {
		return &coreingest.ItemObservation{State: coreingest.ItemUnavailable, ObservedAt: sentAt.Add(-time.Second)}
	}
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion,
		Username:        "Example",
		Profile:         "STANDARD",
		ManifestVersion: 7,
		ClientSeq:       seq,
		Kind:            coreingest.PushFull,
		SentAt:          sentAt,
		Data: coreingest.PushData{
			Varbits: &empty, Varps: &empty, Levels: &lv, XP: &empty, Boosted: &empty,
			Items: &coreingest.ItemContainers{
				Bank: &coreingest.ItemObservation{
					State: coreingest.ItemLive, Revision: &revision, LiveEpoch: "login-1",
					ObservedAt: sentAt.Add(-time.Second), Quantities: &quantities,
				},
				Inventory: unavailable(), Equipment: unavailable(),
			},
		},
	}
}

func assertMarks(ctx context.Context, t *testing.T, pool *pgxpool.Pool, guide string, want map[string]string) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT step_id, kind, COALESCE(implied_by_step_id, '') FROM step_marks WHERE account_id = 'account-1' AND guide_id = $1`, guide)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for rows.Next() {
		var step, kind, by string
		if err := rows.Scan(&step, &kind, &by); err != nil {
			t.Fatal(err)
		}
		if by != "" {
			kind += ":" + by
		}
		got[step] = kind
	}
	if fmt.Sprint(sorted(got)) != fmt.Sprint(sorted(want)) {
		t.Fatalf("%s marks = %v, want %v", guide, got, want)
	}
}

func sorted(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func assertCursor(ctx context.Context, t *testing.T, repo *Repository, guide, step, cause string) {
	t.Helper()
	board, err := repo.Board(ctx, "account-1", guide)
	if err != nil {
		t.Fatal(err)
	}
	if board.Cursor == nil || board.Cursor.StepID != step || board.Cursor.Cause != cause {
		t.Fatalf("%s cursor = %+v, want %s (%s)", guide, board.Cursor, step, cause)
	}
}

func latestSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT id FROM snapshots ORDER BY created_at DESC, fetched_at DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
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
		t.Skip("DATABASE_URL not set; PostgreSQL progress integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err = admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping admin database: %v", err)
	}
	schema := fmt.Sprintf("progress_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
	// The guides of the test content (testdata) exist as the boot's index
	// would leave them: progress rows reference guides (migration 0009).
	if _, err := pool.Exec(ctx, `INSERT INTO guides (id) VALUES ('g1'), ('g2'), ('guide-1') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert test guides: %v", err)
	}
	return pool
}

func TestAccountsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, loadContent(t), catalog)
	if _, ok, findErr := repo.FindAccount(ctx, "", "Iron Man"); findErr != nil || ok {
		t.Fatalf("find before any user = %v, %v", ok, findErr)
	}
	for _, bad := range []AccountInput{
		{RSN: "", Mode: "ironman", GuideID: "g1", Layer: "classic"},
		{RSN: "way too long a name", Mode: "ironman", GuideID: "g1", Layer: "classic"},
		{RSN: "Iron Man", Mode: "main", GuideID: "g1", Layer: "classic"},
		{RSN: "Iron Man", Mode: "ironman", GuideID: "nope", Layer: "classic"},
		{RSN: "Iron Man", Mode: "ironman", GuideID: "g1", Layer: "hcim"},
	} {
		if _, _, badErr := repo.CreateAccount(ctx, "", bad); !errors.Is(badErr, ErrInvalidAccount) {
			t.Fatalf("%+v: %v", bad, badErr)
		}
	}
	acc, created, err := repo.CreateAccount(ctx, "", AccountInput{RSN: " Iron Man ", Mode: "hcim", GuideID: "g1", Layer: "classic"})
	if err != nil || !created || acc.ID != "acc_iron_man" || acc.RSN != "Iron Man" || acc.Mode != "hcim" {
		t.Fatalf("create = %+v %v, %v", acc, created, err)
	}
	board, err := repo.Board(ctx, acc.ID, "g1")
	if err != nil || board.Cursor == nil || !board.Active {
		t.Fatalf("board after setup = %+v, %v", board.Cursor, err)
	}
	same, created, err := repo.CreateAccount(ctx, "", AccountInput{RSN: "iron man", Mode: "ironman", GuideID: "g2", Layer: "enhanced"})
	if err != nil || created || same.ID != acc.ID || same.Mode != "hcim" {
		t.Fatalf("second create = %+v %v, %v", same, created, err)
	}
	found, ok, err := repo.FindAccount(ctx, "", "IRON MAN")
	if err != nil || !ok || found.ID != acc.ID {
		t.Fatalf("find = %+v %v, %v", found, ok, err)
	}

	// An OIDC user sees and creates only its own accounts; acc_<rsn> of
	// another user is taken.
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, subject, issuer, username) VALUES ('usr_x', 'sub-x', 'https://issuer', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = repo.FindAccount(ctx, "usr_x", "Iron Man"); err != nil || ok {
		t.Fatalf("other user's find = %v, %v", ok, err)
	}
	if _, _, err = repo.CreateAccount(ctx, "usr_x", AccountInput{RSN: "Iron Man", Mode: "ironman", GuideID: "g1", Layer: "classic"}); !errors.Is(err, ErrAccountTaken) {
		t.Fatalf("taken id = %v", err)
	}
	mine, created, err := repo.CreateAccount(ctx, "usr_x", AccountInput{RSN: "Other", Mode: "ironman", GuideID: "g1", Layer: "classic"})
	if err != nil || !created || mine.ID != "acc_other" {
		t.Fatalf("oidc create = %+v %v, %v", mine, created, err)
	}
	if got := count(ctx, t, pool, `SELECT count(*) FROM accounts WHERE id = 'acc_other' AND user_id = 'usr_x'`); got != 1 {
		t.Fatalf("owner of the new account: %d", got)
	}
	if _, ok, err = repo.FindAccount(ctx, "", "Other"); err != nil || ok {
		t.Fatalf("dev sees the oidc account = %v, %v", ok, err)
	}
}

func TestMarkBeforePostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-batch-test', 'dev-batch-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'g1', 'classic');
		INSERT INTO step_marks (account_id, guide_id, step_id, kind) VALUES ('account-1', 'g1', 'a2', 'skipped');
		INSERT INTO step_marks (account_id, guide_id, step_id, kind, implied_by_step_id, implied_at) VALUES ('account-1', 'g1', 'a3', 'implied', 'a4', now());
		INSERT INTO mark_commands (id, account_id, guide_id, kind, actor, effects) VALUES ('mc-seed', 'account-1', 'g1', 'migration', 'system',
		  '[{"step_id":"a2","from":null,"to":{"kind":"skipped"}},{"step_id":"a3","from":null,"to":{"kind":"implied","implied_by_step_id":"a4"}}]');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, loadContent(t), catalog)
	// Boot: the fixture's marks get their statuses and cursors, as serve does.
	if _, err = repo.ReevaluateAll(ctx); err != nil {
		t.Fatal(err)
	}
	board, err := repo.MarkBefore(ctx, "account-1", "g1", "a5")
	if err != nil || fmt.Sprint(board.Marked) != "[a1 a3 a4]" || board.Cursor == nil || board.Cursor.StepID != "a5" {
		t.Fatalf("mark before = %v cursor %+v, %v", board.Marked, board.Cursor, err)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM step_status_events WHERE cause = 'user' AND to_status = 'manual_done'`); n != 3 {
		t.Fatalf("user events = %d", n)
	}
	batch := board.CommandID
	if batch == "" {
		t.Fatal("mark before logged no command")
	}
	// An undo names the command it takes back; a command of another guide or
	// account is refused.
	if _, err = repo.RestoreMarks(WithCommand(ctx, CommandMeta{Via: ViaWeb, Revokes: "mc-nope"}), "account-1", "g1", board.Undo); !errors.Is(err, ErrInvalidRevokes) {
		t.Fatalf("unknown revokes: %v", err)
	}
	board, err = repo.RestoreMarks(WithCommand(ctx, CommandMeta{Via: ViaPluginPanel, Revokes: batch}), "account-1", "g1", board.Undo)
	if err != nil || board.Cursor == nil || board.Cursor.StepID != "a1" {
		t.Fatalf("undo cursor = %+v, %v", board.Cursor, err)
	}
	assertMarks(ctx, t, pool, "g1", map[string]string{"a2": "skipped", "a3": "implied:a4"})
	if n := count(ctx, t, pool, `SELECT count(*) FROM mark_commands WHERE kind = 'restore' AND via = 'plugin_panel' AND revokes_id = '`+batch+`'`); n != 1 {
		t.Fatalf("restore command = %d", n)
	}
	assertConsistent(ctx, t, repo, "account-1")
	// The log is append-only, and a write that bypasses it shows in the replay.
	if _, err := pool.Exec(ctx, `UPDATE mark_commands SET kind = 'mark' WHERE id = '`+batch+`'`); err == nil {
		t.Fatal("a mark command was updated")
	}
	if _, err := pool.Exec(ctx, `UPDATE step_marks SET kind = 'manual_done' WHERE step_id = 'a2'`); err != nil {
		t.Fatal(err)
	}
	if diffs, err := repo.VerifyMarks(ctx, "account-1"); err != nil || len(diffs) != 1 {
		t.Fatalf("bypassing write: %v, %v", diffs, err)
	}
	if rep, err := repo.Check(ctx, "account-1"); err != nil || rep.OK {
		t.Fatalf("check must report the bypassing write: %+v, %v", rep, err)
	}
}

func TestGoalPlanPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-goal-test', 'dev-goal-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'g1', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, loadContent(t), catalog)
	goal, err := repo.AddGoal(ctx, "account-1", "quest:shilo_village@finished", "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := repo.GoalPlan(ctx, "account-1", goal.ID)
	if err != nil || !v.Plan.Requirements || v.Plan.Total != 5 || v.Plan.Met != 0 || len(v.Plan.Nodes) != 6 {
		t.Fatalf("plan = %+v, %v", v.Plan, err)
	}
	if _, err = repo.GoalPlan(ctx, "account-1", "goal_missing"); !errors.Is(err, ErrGoalNotFound) {
		t.Fatalf("missing goal: %v", err)
	}
}
