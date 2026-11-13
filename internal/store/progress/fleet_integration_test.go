package progress

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
)

// TestFleetGoalsPostgres reads the Mundo da frota over two accounts of one
// user: the goals of both, the stored verdicts and the plan summary of each
// account, in one call.
func TestFleetGoalsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-fleet-test', 'dev-fleet-test'), ('user-2', 'other', 'other');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('account-1', 'user-1', 'Example', 'example', 'ironman', 'g1', 'classic'),
		  ('account-2', 'user-1', 'Second', 'second', 'ironman', 'g2', 'classic'),
		  ('account-3', 'user-2', 'Third', 'third', 'ironman', 'g1', 'classic');
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
	pins := []struct{ account, key string }{
		{"account-1", "skill:agility>=10"},
		{"account-2", "skill:agility>=10"},
		{"account-2", "quest:shilo_village@finished"},
		{"account-3", "skill:attack>=5"},
		{"account-1", "skill:attack>=5"},
		{"account-1", "skill:agility>=50"},
		{"account-2", "skill:agility>=50"},
	}
	ids := map[string]string{}
	for _, p := range pins {
		g, pinErr := repo.AddGoal(ctx, p.account, p.key, "")
		if pinErr != nil {
			t.Fatal(pinErr)
		}
		ids[p.account+" "+p.key] = g.ID
	}
	// An abandoned goal is out of the fleet.
	if _, err = pool.Exec(ctx, `UPDATE goals SET status = 'abandoned' WHERE id = $1`, ids["account-1 skill:attack>=5"]); err != nil {
		t.Fatal(err)
	}
	// Then account 1 has 12 Agility (met and achieved, with evidence; 50
	// unmet) and a bank without the feathers; account 2 has no facts at all.
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
	push(ctx, t, ingest, token.Token, fullPush(1, map[string]int64{"agility": 12}, map[string]int64{"995": 10}))

	goals, err := repo.FleetGoals(ctx, []string{"account-1", "account-2"}, []string{"item:314x120", "skill:agility>=10"})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	byKey := map[string]coreprogress.FleetGoal{}
	for _, g := range goals {
		order = append(order, g.Key)
		byKey[g.Key] = g
	}
	// Same priority: the earliest pin first; account 3 is not in this fleet.
	if fmt.Sprint(order) != "[skill:agility>=10 quest:shilo_village@finished skill:agility>=50 item:314x120]" {
		t.Fatalf("order = %v", order)
	}
	at := func(key, account string) coreprogress.FleetStanding {
		for _, s := range byKey[key].Accounts {
			if s.Account == account {
				return s
			}
		}
		t.Fatalf("%s: no %s in %+v", key, account, byKey[key])
		return coreprogress.FleetStanding{}
	}

	agility := byKey["skill:agility>=10"]
	if agility.Pinned != 2 || agility.Met != 1 || agility.Unknown != 1 || agility.Closest != "" || len(agility.Accounts) != 2 {
		t.Fatalf("agility = %+v", agility)
	}
	if s := at("skill:agility>=10", "account-1"); s.Verdict != "met" || s.Goal == nil || s.Goal.Status != "achieved" || s.Goal.ID != ids["account-1 skill:agility>=10"] || s.Level == nil || *s.Level != 12 {
		t.Fatalf("agility of account 1 = %+v", s)
	}
	if s := at("skill:agility>=10", "account-2"); s.Verdict != "unknown" || s.Goal == nil || s.Goal.Status != "active" || s.Level != nil {
		t.Fatalf("agility of account 2 = %+v", s)
	}

	// 50 Agility: known unmet for account 1, the only one with a level.
	fifty := byKey["skill:agility>=50"]
	if fifty.Pinned != 2 || fifty.Met != 0 || fifty.Unknown != 1 || fifty.Closest != "account-1" {
		t.Fatalf("50 agility = %+v", fifty)
	}
	if s := at("skill:agility>=50", "account-1"); s.Verdict != "unmet" || len(s.Short) != 1 || s.Short[0].Skill != "agility" || *s.Short[0].Level != 12 || s.Short[0].Target != 50 {
		t.Fatalf("50 agility of account 1 = %+v", s)
	}

	// Shilo Village: never a guide objective of account 1, so unknown there.
	shilo := byKey["quest:shilo_village@finished"]
	if shilo.Pinned != 1 || shilo.Met != 0 || shilo.Unknown != 2 {
		t.Fatalf("shilo = %+v", shilo)
	}
	if s := at("quest:shilo_village@finished", "account-1"); s.Goal != nil || s.Requirements.Total != 5 {
		t.Fatalf("shilo of account 1 = %+v", s)
	}

	// The feathers: nobody pins them, and the bank of one push proves no
	// absence (unknown, never unmet); the active route of account 1 does
	// them at a5 and its cursor is at a1 (no marks).
	feathers := at("item:314x120", "account-1")
	if feathers.Verdict != "unknown" || feathers.Step == nil || feathers.Step.StepID != "a5" || feathers.StepsLeft == nil || *feathers.StepsLeft != 4 ||
		feathers.Cursor == nil || feathers.Cursor.StepID != "a1" {
		t.Fatalf("feathers of account 1 = %+v", feathers)
	}
	if s := at("item:314x120", "account-2"); s.Verdict != "unknown" || s.Step != nil || s.StepsLeft != nil || s.Cursor == nil || s.Cursor.GuideID != "g2" {
		t.Fatalf("feathers of account 2 = %+v", s)
	}

	// Keys must be canonical objectives of the catalog.
	for _, bad := range []string{"skill:agility:70", "quest:nope@finished"} {
		if _, err = repo.FleetGoals(ctx, []string{"account-1"}, []string{bad}); !errors.Is(err, ErrInvalidGoal) {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	// No accounts: the asked keys, with nobody's standing.
	empty, err := repo.FleetGoals(ctx, nil, []string{"skill:agility>=10"})
	if err != nil || len(empty) != 1 || len(empty[0].Accounts) != 0 {
		t.Fatalf("no accounts = %+v, %v", empty, err)
	}
	if _, err = repo.FleetGoals(ctx, []string{"account-missing"}, nil); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("missing account: %v", err)
	}
}
