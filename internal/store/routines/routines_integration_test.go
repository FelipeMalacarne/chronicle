package routines

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

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

const catSwap, fisher = "a.b.CatSwapPlugin", "a.b.FisherPlugin"

func cat(enabled bool) agent.Trigger {
	return agent.Trigger{Event: agent.EventKittenGrown, Enabled: enabled, Run: agent.RunParams{ClassName: catSwap, Stop: []string{fisher}}}
}

func TestTriggersPostgres(t *testing.T) {
	ctx := context.Background()
	repo := setup(ctx, t)
	if ts, err := repo.Triggers(ctx, "acc_a"); err != nil || len(ts) != 0 {
		t.Fatalf("no triggers = %+v, %v", ts, err)
	}
	if _, err := repo.PutTrigger(ctx, "acc_missing", cat(true)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	ts, err := repo.PutTrigger(ctx, "acc_a", cat(false))
	if err != nil || len(ts) != 1 || ts[0].Enabled {
		t.Fatalf("put = %+v, %v", ts, err)
	}
	// One trigger per event: a put replaces it.
	if ts, err = repo.PutTrigger(ctx, "acc_a", cat(true)); err != nil || !reflect.DeepEqual(ts, []agent.Trigger{cat(true)}) {
		t.Fatalf("replace = %+v, %v", ts, err)
	}
	// Import only seeds an account without triggers.
	if ok, err := repo.ImportTriggers(ctx, "acc_a", []agent.Trigger{cat(false)}); ok || err != nil {
		t.Fatalf("import over triggers = %v, %v", ok, err)
	}
	if ok, err := repo.ImportTriggers(ctx, "acc_b", []agent.Trigger{cat(false)}); !ok || err != nil {
		t.Fatalf("import = %v, %v", ok, err)
	}
	if ts, _ = repo.Triggers(ctx, "acc_b"); !reflect.DeepEqual(ts, []agent.Trigger{cat(false)}) {
		t.Fatalf("imported = %+v", ts)
	}
	if ts, _ = repo.Triggers(ctx, "acc_a"); !reflect.DeepEqual(ts, []agent.Trigger{cat(true)}) {
		t.Fatalf("acc_a after imports = %+v", ts)
	}
}

func TestRecordRunPostgres(t *testing.T) {
	ctx := context.Background()
	repo := setup(ctx, t)
	if _, err := repo.PutTrigger(ctx, "acc_a", cat(true)); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	line := func(s int, state, text string) agent.RunEvent {
		return agent.RunEvent{At: t0.Add(time.Duration(s) * time.Second), State: state, Text: text}
	}
	run := agent.Run{
		ID: "run-1", UID: "0123456789abcdef", ClassName: catSwap, Stop: []string{fisher}, Trigger: agent.EventKittenGrown,
		State: agent.RunWaiting, StartedAt: t0, Events: []agent.RunEvent{line(0, agent.RunWaiting, "esperando um ponto seguro")},
	}
	newRun, finished, err := repo.RecordRun(ctx, "acc_a", "worker-acc-a", run)
	if err != nil || !newRun || finished {
		t.Fatalf("first report = %v %v %v", newRun, finished, err)
	}
	// The same report again changes nothing.
	if newRun, finished, err = repo.RecordRun(ctx, "acc_a", "worker-acc-a", run); err != nil || newRun || finished {
		t.Fatalf("repeated report = %v %v %v", newRun, finished, err)
	}
	done := run
	end := t0.Add(90 * time.Second)
	done.State, done.FinishedAt, done.Result = agent.RunDone, &end, json.RawMessage(`{"outcome":"done"}`)
	done.From = &agent.WalkParams{X: 2600, Y: 3400}
	done.Events = append(append([]agent.RunEvent{}, run.Events...), line(30, agent.RunRunning, "Cat Swap ligado"), line(90, agent.RunDone, "pronto"))
	if newRun, finished, err = repo.RecordRun(ctx, "acc_a", "worker-acc-a", done); err != nil || newRun || !finished {
		t.Fatalf("final report = %v %v %v", newRun, finished, err)
	}
	// An older report (shorter trail) arriving late is ignored.
	if _, finished, err = repo.RecordRun(ctx, "acc_a", "worker-acc-a", run); err != nil || finished {
		t.Fatalf("stale report = %v %v", finished, err)
	}
	// The final state again: not finished a second time.
	if _, finished, err = repo.RecordRun(ctx, "acc_a", "worker-acc-a", done); err != nil || finished {
		t.Fatalf("final again = %v %v", finished, err)
	}
	// A run never moves to another account; an unknown account is ErrNotFound;
	// a malformed report never reaches the database.
	if _, _, err = repo.RecordRun(ctx, "acc_b", "zaros", done); !errors.Is(err, ErrForeignRun) {
		t.Fatalf("foreign run = %v", err)
	}
	other := done
	other.UID = "fedcba9876543210"
	if _, _, err = repo.RecordRun(ctx, "acc_missing", "zaros", other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown account = %v", err)
	}
	bad := done
	bad.UID = "nope"
	if _, _, err = repo.RecordRun(ctx, "acc_a", "zaros", bad); err == nil {
		t.Fatal("malformed report stored")
	}

	owner := agent.Run{
		ID: "run-2", UID: "00000000000000bb", ClassName: catSwap, State: agent.RunRunning, StartedAt: t0.Add(time.Hour),
		Events: []agent.RunEvent{line(3600, agent.RunRunning, "rodando")},
	}
	if _, _, err = repo.RecordRun(ctx, "acc_a", "zaros", owner); err != nil {
		t.Fatal(err)
	}
	runs, err := repo.Runs(ctx, "acc_a", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].UID != owner.UID || runs[1].UID != done.UID {
		t.Fatalf("runs = %+v", runs)
	}
	got := runs[1]
	if got.ID != "run-1" || got.State != agent.RunDone || got.Trigger != agent.EventKittenGrown || !got.FinishedAt.Equal(end) ||
		!reflect.DeepEqual(got.Stop, []string{fisher}) || got.From == nil || got.From.X != 2600 || len(got.Events) != 3 ||
		got.Events[2].Text != "pronto" || !sameJSON(got.Result, done.Result) {
		t.Fatalf("stored run = %+v", got)
	}
	if runs[0].Stop == nil || runs[0].FinishedAt != nil || len(runs[0].Events) != 1 {
		t.Fatalf("owner's run = %+v", runs[0])
	}
	if runs, _ = repo.Runs(ctx, "acc_b", 30); len(runs) != 0 {
		t.Fatalf("acc_b runs = %+v", runs)
	}
}

// TestTaskRunPostgres: a task run (docs/10 phase 3) keeps its task in the
// history, and a plugin's run has none.
func TestTaskRunPostgres(t *testing.T) {
	ctx := context.Background()
	repo := setup(ctx, t)
	start := time.Now().UTC().Truncate(time.Second)
	task := agent.Run{
		ID: "run-1", UID: "aa11bb22cc33dd44", ClassName: agent.ChronicleLinkClass, Stop: []string{}, State: agent.RunDone,
		StartedAt: start, FinishedAt: &start,
		Task:   &agent.RunTask{Key: "step:oziris-main::1.1.3#0", Plan: "route:oziris-main", Type: "travel", Label: "Ir até Lumbridge"},
		Events: []agent.RunEvent{{At: start, State: agent.RunDone, Text: "o executor terminou"}},
	}
	if _, _, err := repo.RecordRun(ctx, "acc_a", "zaros", task); err != nil {
		t.Fatal(err)
	}
	plugin := agent.Run{ID: "run-2", UID: "aa11bb22cc33dd45", ClassName: catSwap, Stop: []string{}, State: agent.RunWaiting, StartedAt: start.Add(time.Second), Events: []agent.RunEvent{}}
	if _, _, err := repo.RecordRun(ctx, "acc_a", "zaros", plugin); err != nil {
		t.Fatal(err)
	}
	runs, err := repo.Runs(ctx, "acc_a", 10)
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if runs[0].Task != nil || runs[1].Task == nil || *runs[1].Task != *task.Task {
		t.Fatalf("tasks = %+v, %+v", runs[0].Task, runs[1].Task)
	}
}

func recurringRoutine(name, clock string, interval, priority int) agent.Recurring {
	return agent.Recurring{
		Name: name, Enabled: true, Clock: clock, IntervalMin: interval, Priority: priority,
		Run: agent.RunParams{ClassName: "a.b.HerbrunPlugin", Stop: []string{fisher}, TimeoutS: 900},
	}
}

func TestRecurringPostgres(t *testing.T) {
	ctx := context.Background()
	repo := setup(ctx, t)
	if list, err := repo.Recurring(ctx, "acc_a"); err != nil || len(list) != 0 {
		t.Fatalf("none = %+v, %v", list, err)
	}
	if _, _, err := repo.PutRecurring(ctx, "acc_missing", recurringRoutine("Herb run", "HERB", 0, 0)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	if _, _, err := repo.PutRecurring(ctx, "acc_a", recurringRoutine("Nothing", "", 0, 0)); err == nil {
		t.Fatal("a routine without clock or interval was saved")
	}
	herbID, _, err := repo.PutRecurring(ctx, "acc_a", recurringRoutine("Herb run", "HERB", 80, 10))
	if err != nil || !strings.HasPrefix(herbID, "rtn_") {
		t.Fatalf("herb = %q, %v", herbID, err)
	}
	treeID, list, err := repo.PutRecurring(ctx, "acc_a", recurringRoutine("Tree run", "TREE", 0, 50))
	if err != nil || len(list) != 2 || list[0].ID != treeID || list[1].ID != herbID {
		t.Fatalf("by priority = %+v, %v", list, err)
	}
	herb := list[1]
	if herb.Name != "Herb run" || herb.Clock != "HERB" || herb.IntervalMin != 80 || herb.Run.TimeoutS != 900 ||
		!reflect.DeepEqual(herb.Run.Stop, []string{fisher}) || herb.UpdatedAt == nil || herb.LastRun != nil {
		t.Fatalf("stored herb = %+v", herb)
	}
	if _, _, err = repo.PutRecurring(ctx, "acc_a", recurringRoutine("Herb run", "", 60, 0)); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("same name = %v", err)
	}
	// An update by id: another account's routine is not found.
	edit := herb
	edit.Enabled, edit.Priority, edit.LastRun = false, 60, nil
	if _, _, err = repo.PutRecurring(ctx, "acc_b", edit); !errors.Is(err, ErrRoutineNotFound) {
		t.Fatalf("foreign update = %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, list, err = repo.PutRecurring(ctx, "acc_a", edit); err != nil || list[0].ID != herbID || list[0].Enabled || !list[0].UpdatedAt.After(*herb.UpdatedAt) {
		t.Fatalf("update = %+v, %v", list, err)
	}
	// Triggers and recurring routines live side by side.
	if _, err = repo.PutTrigger(ctx, "acc_a", cat(true)); err != nil {
		t.Fatal(err)
	}
	if ts, _ := repo.Triggers(ctx, "acc_a"); len(ts) != 1 {
		t.Fatalf("triggers = %+v", ts)
	}

	// A run of the routine is its last run, and reads back with it.
	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	end := t0.Add(12 * time.Minute)
	run := agent.Run{
		ID: "run-3", UID: "00000000000000cc", ClassName: "a.b.HerbrunPlugin", Routine: herbID, State: agent.RunDone,
		StartedAt: t0, FinishedAt: &end, Events: []agent.RunEvent{{At: t0, State: agent.RunWaiting, Text: "rotina Herb run: relógio Herb Patches pronto"}},
	}
	if _, _, err = repo.RecordRun(ctx, "acc_a", "zaros", run); err != nil {
		t.Fatal(err)
	}
	// A run naming a routine of another account keeps none.
	stray := run
	stray.UID, stray.ID, stray.StartedAt = "00000000000000dd", "run-4", t0.Add(time.Hour)
	if _, _, err = repo.RecordRun(ctx, "acc_b", "box", stray); err != nil {
		t.Fatal(err)
	}
	list, _ = repo.Recurring(ctx, "acc_a")
	if l := list[0].LastRun; l == nil || l.State != agent.RunDone || !l.StartedAt.Equal(t0) || !l.FinishedAt.Equal(end) {
		t.Fatalf("last run = %+v", l)
	}
	if list[1].LastRun != nil {
		t.Fatalf("tree's last run = %+v", list[1].LastRun)
	}
	if runs, _ := repo.Runs(ctx, "acc_a", 30); len(runs) != 1 || runs[0].Routine != herbID || runs[0].Trigger != "" {
		t.Fatalf("runs = %+v", runs)
	}
	if runs, _ := repo.Runs(ctx, "acc_b", 30); len(runs) != 1 || runs[0].Routine != "" {
		t.Fatalf("stray run = %+v", runs)
	}

	// Deleting keeps the run, without the routine.
	if list, err = repo.DeleteRecurring(ctx, "acc_a", herbID); err != nil || len(list) != 1 || list[0].ID != treeID {
		t.Fatalf("delete = %+v, %v", list, err)
	}
	if _, err = repo.DeleteRecurring(ctx, "acc_a", herbID); !errors.Is(err, ErrRoutineNotFound) {
		t.Fatalf("delete again = %v", err)
	}
	if runs, _ := repo.Runs(ctx, "acc_a", 30); len(runs) != 1 || runs[0].Routine != "" {
		t.Fatalf("run after the delete = %+v", runs)
	}

	// At most agent.RecurringMax per account.
	for i := range agent.RecurringMax {
		if _, _, err = repo.PutRecurring(ctx, "acc_b", recurringRoutine(fmt.Sprintf("Run %d", i), "", 60, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = repo.PutRecurring(ctx, "acc_b", recurringRoutine("One more", "", 60, 0)); !errors.Is(err, ErrTooMany) {
		t.Fatalf("too many = %v", err)
	}
	// The table holds the shape of each kind.
	for _, bad := range []string{
		`INSERT INTO routines (id, account_id, kind, enabled, spec) VALUES ('rtn_x1', 'acc_a', 'recurring', true, '{}')`,
		`INSERT INTO routines (id, account_id, kind, name, interval_min, spec) VALUES ('rtn_x2', 'acc_a', 'trigger', 'x', 60, '{}')`,
		`INSERT INTO routines (id, account_id, kind, name, interval_min, spec) VALUES ('rtn_x3', 'acc_a', 'recurring', 'x', 5, '{}')`,
	} {
		if _, err = repo.pool.Exec(ctx, bad); err == nil {
			t.Errorf("accepted: %s", bad)
		}
	}
}

// sameJSON compares two JSON values (jsonb normalizes the spacing).
func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func setup(ctx context.Context, t *testing.T) *Repository {
	t.Helper()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_a', 'u1', 'A', 'a', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_b', 'u1', 'B', 'b', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	return New(pool)
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL routines integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("routines_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
