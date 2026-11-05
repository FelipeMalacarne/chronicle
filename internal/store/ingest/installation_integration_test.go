package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

// testIngestPool opens a pool on a throwaway schema of DATABASE_URL with every
// migration applied, or skips the test.
func testIngestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL ingest integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("ingest_inst_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})
	applyTestMigrations(ctx, t, pool)
	return pool
}

func TestDevInstallationPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testIngestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-1', 'user-1', 'Iron Example', 'iron_example', 'ironman', 'guide-1');
	`); err != nil {
		t.Fatalf("insert account fixture: %v", err)
	}
	def := Definition{Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10}
	const installation = "3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c"
	dev := Credential{Installation: installation}

	// Without WithDevInstallations (AUTH_MODE=oidc) nothing tokenless passes.
	closed := New(pool, def)
	if _, err := closed.ManifestFor(ctx, dev); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("tokenless manifest when disabled = %v", err)
	}
	if _, err := closed.Principal(ctx, dev); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("tokenless principal when disabled = %v", err)
	}
	pushOf := func(seq int64, username string) (coreingest.Push, json.RawMessage) {
		p := repositoryFullPush(seq, repositoryRestored(1, map[string]int64{"995": 10}))
		p.Username = username
		raw, _ := json.Marshal(p)
		return p, raw
	}
	p1, raw1 := pushOf(1, "Iron Example")
	if _, err := closed.AcceptFor(ctx, dev, "127.0.0.1", raw1, p1); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("tokenless push when disabled = %v", err)
	}

	repo := New(pool, def).WithDevInstallations(true)
	if _, err := repo.ManifestFor(ctx, Credential{Installation: "not-a-uuid"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("malformed installation = %v", err)
	}
	m, err := repo.ManifestFor(ctx, dev)
	if err != nil || !m.Resync || !m.Items.Bank || !m.Items.Inventory || !m.Items.Equipment {
		t.Fatalf("first tokenless manifest = %+v, %v", m, err)
	}
	resp, err := repo.AcceptFor(ctx, dev, "127.0.0.1", raw1, p1)
	if err != nil || !resp.OK || resp.AcceptedSeq != 1 {
		t.Fatalf("first tokenless push = %+v, %v", resp, err)
	}
	var (
		account, tokenHash string
		hasHash            bool
		lastSeq            int64
	)
	if err = pool.QueryRow(ctx, `
		SELECT account_id, token_hash IS NOT NULL, coalesce(token_hash, ''), last_seq FROM plugin_ingest_tokens WHERE installation_id = $1
	`, installation).Scan(&account, &hasHash, &tokenHash, &lastSeq); err != nil {
		t.Fatal(err)
	}
	if account != "account-1" || hasHash || lastSeq != 1 {
		t.Fatalf("installation record = %s hash=%v seq=%d", account, hasHash, lastSeq)
	}
	if m, err = repo.ManifestFor(ctx, dev); err != nil || m.Resync {
		t.Fatalf("manifest after a full = %+v, %v", m, err)
	}
	// Same seq again: 409 semantics as with a token.
	if resp, err = repo.AcceptFor(ctx, dev, "127.0.0.1", raw1, p1); !errors.Is(err, ErrStaleSequence) || resp.AcceptedSeq != 1 || !resp.Resync {
		t.Fatalf("replayed tokenless push = %+v, %v", resp, err)
	}
	p2, raw2 := pushOf(2, "Iron Example")
	if resp, err = repo.AcceptFor(ctx, dev, "127.0.0.1", raw2, p2); err != nil || resp.AcceptedSeq != 2 {
		t.Fatalf("second tokenless push = %+v, %v", resp, err)
	}
	if got := countRows(ctx, t, pool, `SELECT count(*) FROM plugin_ingest_tokens WHERE installation_id = '`+installation+`'`); got != 1 {
		t.Fatalf("installation records = %d, want 1", got)
	}
	if p, perr := repo.Principal(ctx, dev); perr != nil || p.AccountID != "account-1" || !p.Tokenless || !p.Allows(coreingest.ScopeMarksWrite) {
		t.Fatalf("principal = %+v, %v", p, perr)
	}

	// The only account takes an unknown character's push (owner decision 28);
	// with a second account it becomes ambiguous and Conta must name one.
	p3, raw3 := pushOf(3, "Somebody Else")
	if resp, err = repo.AcceptFor(ctx, dev, "127.0.0.1", raw3, p3); err != nil || resp.AcceptedSeq != 3 {
		t.Fatalf("unknown character with one account = %+v, %v", resp, err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-2', 'user-1', 'Alt Two', 'alt_two', 'ironman', 'guide-1');
	`); err != nil {
		t.Fatal(err)
	}
	p4, raw4 := pushOf(4, "Somebody Else")
	if _, err = repo.AcceptFor(ctx, dev, "127.0.0.1", raw4, p4); !errors.Is(err, ErrAccountAmbiguous) {
		t.Fatalf("unknown character with two accounts = %v", err)
	}
	if _, err = repo.AcceptFor(ctx, Credential{Installation: installation, Account: "account-9"}, "127.0.0.1", raw4, p4); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("unknown explicit account = %v", err)
	}
	// The alt's own name picks it: a second record, with its own sequence.
	p5, raw5 := pushOf(5, "alt two")
	if resp, err = repo.AcceptFor(ctx, dev, "127.0.0.1", raw5, p5); err != nil || resp.AcceptedSeq != 5 {
		t.Fatalf("alt push = %+v, %v", resp, err)
	}
	if got := countRows(ctx, t, pool, `SELECT count(*) FROM plugin_ingest_tokens WHERE installation_id = '`+installation+`'`); got != 2 {
		t.Fatalf("installation records = %d, want one per account", got)
	}
	// The panel follows the account the installation pushed to last.
	if p, perr := repo.Principal(ctx, dev); perr != nil || p.AccountID != "account-2" {
		t.Fatalf("principal after the alt's push = %+v, %v", p, perr)
	}
	if p, perr := repo.Principal(ctx, Credential{Installation: installation, Account: "account-1"}); perr != nil || p.AccountID != "account-1" {
		t.Fatalf("principal with Conta = %+v, %v", p, perr)
	}
	if _, perr := repo.Principal(ctx, Credential{Installation: "11111111-2222-4333-8444-555555555555"}); !errors.Is(perr, ErrAccountAmbiguous) {
		t.Fatalf("a new installation with two accounts = %v", perr)
	}

	// Tokens keep working next to the installation records.
	created, err := repo.CreateToken(ctx, "account-1", "desktop", []coreingest.TokenScope{coreingest.ScopeMarksWrite})
	if err != nil {
		t.Fatal(err)
	}
	if p, perr := repo.Principal(ctx, Credential{Token: created.Token}); perr != nil || p.AccountID != "account-1" || p.Tokenless || !p.Allows(coreingest.ScopeMarksWrite) {
		t.Fatalf("token principal = %+v, %v", p, perr)
	}
	if m, err := repo.Manifest(ctx, created.Token); err != nil || m.Items.Bank || !m.Resync {
		t.Fatalf("token manifest = %+v, %v", m, err)
	}
	if _, err := repo.Principal(ctx, Credential{Token: "ipl_wrong"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong token = %v", err)
	}
}

func countRows(ctx context.Context, t *testing.T, pool *pgxpool.Pool, query string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
