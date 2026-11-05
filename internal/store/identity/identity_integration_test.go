package identity

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
)

const issuer = "https://auth.example"

func TestLoginOIDCClaimsDevAccountsOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_a', 'dev', 'A', 'a', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_b', 'dev', 'B', 'b', 'hcim', 'bruhsailer-main', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	repo := New(pool)

	// The dev user's subject never logs in through OIDC.
	if _, err := repo.LoginOIDC(ctx, Login{Issuer: issuer, Subject: "dev"}); !errors.Is(err, ErrSubjectConflict) {
		t.Fatalf("dev subject = %v", err)
	}

	first, err := repo.LoginOIDC(ctx, Login{Issuer: issuer, Subject: "100", Username: "dev", Email: "o@example.org", DisplayName: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	// "dev" is taken by the dev user: the subject disambiguates.
	if !first.Created || first.ClaimedAccounts != 2 || first.User.Username != "dev#100" || first.User.Role != "owner" || first.User.Email != "o@example.org" {
		t.Fatalf("first login = %+v", first)
	}
	accounts, err := repo.Accounts(ctx, first.User.ID)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("owner accounts = %+v, %v", accounts, err)
	}
	if devAccounts, _ := repo.Accounts(ctx, "dev"); len(devAccounts) != 0 {
		t.Fatalf("dev kept accounts: %+v", devAccounts)
	}
	if ok, ownErr := repo.OwnsAccount(ctx, first.User.ID, "acc_a"); ownErr != nil || !ok {
		t.Fatalf("owns acc_a = %v, %v", ok, ownErr)
	}

	// Second login of the same subject: same user, profile updated, no claim.
	again, err := repo.LoginOIDC(ctx, Login{Issuer: issuer, Subject: "100", Username: "renamed", DisplayName: "Owner 2"})
	if err != nil || again.Created || again.ClaimedAccounts != 0 || again.User.ID != first.User.ID ||
		again.User.Username != "dev#100" || again.User.DisplayName != "Owner 2" || again.User.Email != "" {
		t.Fatalf("second login = %+v, %v", again, err)
	}

	// A dev account created later (dev mode on the same database) is not
	// claimed by a second OIDC user: only the first OIDC user ever claims.
	if _, err = pool.Exec(ctx, `INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES ('acc_c', 'dev', 'C', 'c', 'ironman', 'oziris-main', 'enhanced')`); err != nil {
		t.Fatal(err)
	}
	other, err := repo.LoginOIDC(ctx, Login{Issuer: issuer, Subject: "200", Username: "friend"})
	if err != nil || !other.Created || other.ClaimedAccounts != 0 || other.User.Role != "member" {
		t.Fatalf("second user = %+v, %v", other, err)
	}
	if ok, _ := repo.OwnsAccount(ctx, other.User.ID, "acc_a"); ok {
		t.Fatal("second user owns the owner's account")
	}
	if ok, _ := repo.OwnsAccount(ctx, other.User.ID, "acc_c"); ok {
		t.Fatal("second user claimed a dev account")
	}
	if ok, _ := repo.OwnsAccount(ctx, first.User.ID, "acc_missing"); ok {
		t.Fatal("missing account owned")
	}

	// The same subject from another issuer is refused, not merged.
	if _, err = repo.LoginOIDC(ctx, Login{Issuer: "https://other.example", Subject: "100"}); !errors.Is(err, ErrSubjectConflict) {
		t.Fatalf("other issuer = %v", err)
	}

	u, err := repo.User(ctx, first.User.ID)
	if err != nil || u.ID != first.User.ID {
		t.Fatalf("user = %+v, %v", u, err)
	}
	if _, err = repo.User(ctx, "nope"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user = %v", err)
	}
	dev, found, err := repo.DevUser(ctx)
	if err != nil || !found || dev.ID != "dev" {
		t.Fatalf("dev user = %+v %v, %v", dev, found, err)
	}
}

func TestLoginOIDCWithoutDevUser(t *testing.T) {
	ctx := context.Background()
	repo := New(testPool(ctx, t))
	res, err := repo.LoginOIDC(ctx, Login{Issuer: issuer, Subject: "1", Email: "a@example.org"})
	if err != nil || !res.Created || res.ClaimedAccounts != 0 || res.User.Username != "a@example.org" {
		t.Fatalf("login = %+v, %v", res, err)
	}
	if _, found, err := repo.DevUser(ctx); err != nil || found {
		t.Fatalf("dev user = %v, %v", found, err)
	}
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL identity integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("identity_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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
