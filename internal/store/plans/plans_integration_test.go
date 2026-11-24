package plans

import (
	"context"
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
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

func TestPlansPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	c, err := content.Load(filepath.Join("..", "..", "core", "content", "testdata", "execution", "content"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO guides (id, title) VALUES ('t', 'Execution fixture'), ('other', 'Other');
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_a', 'u1', 'A', 'a', 'ironman', 't', 'enhanced'),
		  ('acc_b', 'u1', 'B', 'b', 'hcim', 't', 'enhanced');
		INSERT INTO goals (id, account_id, objective_key, label) VALUES
		  ('goal_a', 'acc_a', 'item:1351x1', 'Bronze axe'), ('goal_b', 'acc_b', 'item:1351x1', 'Bronze axe');
	`); err != nil {
		t.Fatal(err)
	}
	repo := New(pool, c, reg)

	// Before any plan, the account follows its route.
	active, err := repo.Active(ctx, "acc_a")
	if err != nil || active.ID != "route:t" || active.Kind != plan.KindRoute || active.Guide != "t" || active.Title != "Execution fixture" {
		t.Fatalf("active = %+v, %v", active, err)
	}
	if _, err = repo.Active(ctx, "acc_none"); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("unknown account: %v", err)
	}

	axe := entities.MustParseObjectiveKey("item:1351x1")
	lumbridge := entities.MustParseRef("location:lumbridge")
	created, err := repo.Create(ctx, "acc_a", NewPlan{Kind: plan.KindCustom, Title: " Logística ", Items: []plan.Item{
		{Kind: plan.ItemStep, Guide: "t", Step: "b"},
		{Kind: plan.ItemRecipe, Recipe: &axe},
		{Kind: plan.ItemTask, Task: &content.Task{Type: content.TaskTravel, Location: &lumbridge}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ID, "pln_") || created.Title != "Logística" || len(created.Items) != 3 ||
		created.Items[0].Step != "b" || created.Items[1].Recipe.String() != "item:1351x1" || created.Items[2].Task.Location.String() != "location:lumbridge" {
		t.Fatalf("created = %+v", created)
	}
	goalPlan, err := repo.Create(ctx, "acc_a", NewPlan{Kind: plan.KindGoal, Title: "Machado", Goal: "goal_a", Items: []plan.Item{{Kind: plan.ItemRecipe, Recipe: &axe}}})
	if err != nil || goalPlan.Goal != "goal_a" {
		t.Fatalf("goal plan = %+v, %v", goalPlan, err)
	}

	// Refused before anything is written.
	for name, in := range map[string]NewPlan{
		"route":         {Kind: plan.KindRoute, Title: "x"},
		"kind":          {Kind: "wish", Title: "x"},
		"title":         {Kind: plan.KindCustom, Title: ""},
		"goal missing":  {Kind: plan.KindGoal, Title: "x"},
		"custom goal":   {Kind: plan.KindCustom, Title: "x", Goal: "goal_a"},
		"other's goal":  {Kind: plan.KindGoal, Title: "x", Goal: "goal_b"},
		"step":          {Kind: plan.KindCustom, Title: "x", Items: []plan.Item{{Kind: plan.ItemStep, Guide: "t", Step: "zz"}}},
		"bank no items": {Kind: plan.KindCustom, Title: "x", Items: []plan.Item{{Kind: plan.ItemTask, Task: &content.Task{Type: content.TaskBank}}}},
	} {
		var invalid *InvalidError
		if _, err = repo.Create(ctx, "acc_a", in); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err = repo.Create(ctx, "acc_none", NewPlan{Kind: plan.KindCustom, Title: "x"}); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("unknown account: %v", err)
	}

	// The database refuses what the store never writes.
	if _, err = pool.Exec(ctx, `INSERT INTO plans (id, account_id, kind, title, guide_id) VALUES ('pln_route0001', 'acc_a', 'route', 'R', 't')`); err != nil {
		t.Fatal(err)
	}
	for name, sql := range map[string]string{
		"route items":     `INSERT INTO plan_items (plan_id, position, kind, guide_id, step_id) VALUES ('pln_route0001', 0, 'step', 't', 'a')`,
		"two refs":        fmt.Sprintf(`INSERT INTO plan_items (plan_id, position, kind, guide_id, step_id, recipe_key) VALUES ('%s', 9, 'step', 't', 'a', 'item:1351x1')`, created.ID),
		"task not object": fmt.Sprintf(`INSERT INTO plan_items (plan_id, position, kind, task) VALUES ('%s', 9, 'task', '[1]')`, created.ID),
		"second route":    `INSERT INTO plans (id, account_id, kind, title, guide_id) VALUES ('pln_route0002', 'acc_a', 'route', 'R', 't')`,
		"custom guide":    `INSERT INTO plans (id, account_id, kind, title, guide_id) VALUES ('pln_custom001', 'acc_a', 'custom', 'R', 't')`,
		"goal of another": `INSERT INTO plans (id, account_id, kind, title, goal_id) VALUES ('pln_goal00001', 'acc_a', 'goal', 'R', 'goal_b')`,
		"bad id":          `INSERT INTO plans (id, account_id, kind, title) VALUES ('plan-1', 'acc_a', 'custom', 'R')`,
		"other's plan":    fmt.Sprintf(`UPDATE accounts SET active_plan_id = '%s' WHERE id = 'acc_b'`, created.ID),
	} {
		if _, err = pool.Exec(ctx, sql); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err = pool.Exec(ctx, `DELETE FROM plans WHERE id = 'pln_route0001'`); err != nil {
		t.Fatal(err)
	}

	// Following a plan, and back to the route.
	if active, err = repo.SetActive(ctx, "acc_a", created.ID); err != nil || active.ID != created.ID || len(active.Items) != 3 {
		t.Fatalf("set active = %+v, %v", active, err)
	}
	list, err := repo.List(ctx, "acc_a")
	if err != nil || list.Active != created.ID || len(list.Plans) != 3 || list.Plans[0].ID != "route:t" || list.Plans[1].ID != created.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if _, err = repo.SetActive(ctx, "acc_b", created.ID); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("another account's plan: %v", err)
	}
	if _, err = repo.SetActive(ctx, "acc_a", "route:other"); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("another guide's route: %v", err)
	}
	if active, err = repo.SetActive(ctx, "acc_a", "route:t"); err != nil || active.Kind != plan.KindRoute {
		t.Fatalf("back to the route = %+v, %v", active, err)
	}
	if _, err = repo.SetActive(ctx, "acc_a", goalPlan.ID); err != nil {
		t.Fatal(err)
	}
	// Deleting the followed plan (here by deleting its goal) puts the account back on its route.
	if _, err = pool.Exec(ctx, `DELETE FROM goals WHERE id = 'goal_a'`); err != nil {
		t.Fatal(err)
	}
	if active, err = repo.Active(ctx, "acc_a"); err != nil || active.Kind != plan.KindRoute {
		t.Fatalf("after the goal went = %+v, %v", active, err)
	}
	if err = repo.Delete(ctx, "acc_b", created.ID); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("delete another account's plan: %v", err)
	}
	if err = repo.Delete(ctx, "acc_a", "route:t"); err == nil {
		t.Error("the route plan is not deleted")
	}
	if err = repo.Delete(ctx, "acc_a", created.ID); err != nil {
		t.Fatal(err)
	}
	var items int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM plan_items`).Scan(&items); err != nil || items != 0 {
		t.Errorf("items left = %d, %v", items, err)
	}
}

// TestPlanItemsFollowARenamedStep: a step id alias moves the plans' step
// items (store/progress applyAlias runs the same query at boot).
func TestPlanItemsFollowARenamedStep(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO guides (id) VALUES ('t');
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES ('acc_a', 'u1', 'A', 'a', 'ironman', 't', 'enhanced');
		INSERT INTO plans (id, account_id, kind, title) VALUES ('pln_custom001', 'acc_a', 'custom', 'P');
		INSERT INTO plan_items (plan_id, position, kind, guide_id, step_id) VALUES ('pln_custom001', 0, 'step', 't', 'old');
	`); err != nil {
		t.Fatal(err)
	}
	repo := New(pool, nil, nil)
	if err := repo.queries.MovePlanItemSteps(ctx, storedb.MovePlanItemStepsParams{NewStepID: "new", GuideID: "t", OldStepID: "old"}); err != nil {
		t.Fatal(err)
	}
	var step string
	if err := pool.QueryRow(ctx, `SELECT step_id FROM plan_items`).Scan(&step); err != nil || step != "new" {
		t.Errorf("step = %s, %v", step, err)
	}
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL plans integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("plans_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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

// TestSaleProofsPostgres: a proven sale is recorded per (account, plan,
// task key), read back by plan, kept at its first record for the same run,
// moved to a later run's proof and forgotten when a new run starts; a proof
// below the run's coins is refused.
func TestSaleProofsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO guides (id, title) VALUES ('t', 'Execution fixture');
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_a', 'u1', 'A', 'a', 'regular', 't', 'enhanced'),
		  ('acc_b', 'u1', 'B', 'b', 'regular', 't', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	repo := New(pool, nil, nil)
	proof := SaleProof{Plan: "pln_abcdefgh", TaskKey: "task:0#0", RunUID: "0123abcd", Item: 1775, CoinsAtLeast: 170_000, CoinsSeen: 170_500}
	if err := repo.ProveSale(ctx, "acc_a", proof); err != nil {
		t.Fatal(err)
	}
	first := func() (string, time.Time) {
		var uid string
		var at time.Time
		if err := pool.QueryRow(ctx, `SELECT run_uid, proven_at FROM sale_proofs WHERE account_id = 'acc_a'`).Scan(&uid, &at); err != nil {
			t.Fatal(err)
		}
		return uid, at
	}
	_, at := first()
	if err := repo.ProveSale(ctx, "acc_a", proof); err != nil {
		t.Fatal(err)
	}
	if uid, again := first(); uid != "0123abcd" || !again.Equal(at) {
		t.Fatalf("the same run's proof moved: %s %v -> %v", uid, at, again)
	}
	later := proof
	later.RunUID = "4567abcd"
	if err := repo.ProveSale(ctx, "acc_a", later); err != nil {
		t.Fatal(err)
	}
	if uid, _ := first(); uid != "4567abcd" {
		t.Fatalf("a later run's proof = %s", uid)
	}
	short := proof
	short.CoinsSeen = short.CoinsAtLeast - 1
	if err := repo.ProveSale(ctx, "acc_a", short); err == nil {
		t.Fatal("a proof below the run's coins was recorded")
	}
	for _, tc := range []struct {
		account, plan string
		want          map[string]string
	}{
		{"acc_a", "pln_abcdefgh", map[string]string{"task:0#0": "4567abcd"}},
		{"acc_a", "pln_other000", map[string]string{}},
		{"acc_b", "pln_abcdefgh", map[string]string{}},
	} {
		got, err := repo.ProvenSales(ctx, tc.account, tc.plan)
		if err != nil || fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s %s = %v, %v; want %v", tc.account, tc.plan, got, err, tc.want)
		}
	}
	if err := repo.ForgetSale(ctx, "acc_a", "pln_abcdefgh", "task:0#0"); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ProvenSales(ctx, "acc_a", "pln_abcdefgh"); err != nil || len(got) != 0 {
		t.Fatalf("after forgetting = %v, %v", got, err)
	}
}
