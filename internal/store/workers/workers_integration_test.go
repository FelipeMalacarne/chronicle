package workers

import (
	"bytes"
	"context"
	"encoding/base64"
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

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/vault"
)

var session = agent.GameSession{
	Character: "Iron Man", SessionID: "jx-session-secret", CharacterID: "jx-character-1",
	DeviceIDs: "iron_man=abc", RandomDats: "iron_man=def",
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, repo := setup(ctx, t)

	if err := repo.PutSession(ctx, "acc_missing", "u1", session); !errors.Is(err, ErrNotFound) {
		t.Fatalf("put on missing account = %v", err)
	}
	if err := repo.PutSession(ctx, "acc_a", "u1", agent.GameSession{Character: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("put incomplete session = %v", err)
	}
	if _, err := repo.SessionInfo(ctx, "acc_a"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("info without session = %v", err)
	}
	if _, err := repo.Session(ctx, "acc_a"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("session without session = %v", err)
	}

	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}
	if !managed(ctx, t, pool, "acc_a") {
		t.Fatal("account not managed after PutSession")
	}
	// The secret is not stored in the clear.
	var ciphertext []byte
	if err := pool.QueryRow(ctx, `SELECT ciphertext FROM game_sessions WHERE account_id = 'acc_a'`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(session.SessionID)) {
		t.Fatal("session id stored in the clear")
	}
	got, err := repo.Session(ctx, "acc_a")
	if err != nil || got != session {
		t.Fatalf("session = %+v, %v", got, err)
	}
	info, err := repo.SessionInfo(ctx, "acc_a")
	if err != nil || info.Character != "Iron Man" || info.PushedBy != "u1" || info.KeyID != repo.key.ID() ||
		info.PushedAt.IsZero() || info.LastLoginAt != nil || info.LastLoginResult != "" {
		t.Fatalf("info = %+v, %v", info, err)
	}

	if err = repo.RecordLogin(ctx, "acc_a", "ok"); err != nil {
		t.Fatal(err)
	}
	if info, _ = repo.SessionInfo(ctx, "acc_a"); info.LastLoginAt == nil || info.LastLoginResult != "ok" || info.LoginFailed {
		t.Fatalf("info after login = %+v", info)
	}
	// A refused login flags the session; the next session clears it (below).
	if err = repo.RecordLogin(ctx, "acc_a", "Invalid credentials"); err != nil {
		t.Fatal(err)
	}
	if info, _ = repo.SessionInfo(ctx, "acc_a"); !info.LoginFailed || info.LastLoginResult != "Invalid credentials" {
		t.Fatalf("info after a refused login = %+v", info)
	}
	if err = repo.RecordLogin(ctx, "acc_b", "ok"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("login without session = %v", err)
	}

	// A new session replaces the old one and its last login.
	renewed := session
	renewed.SessionID = "jx-session-2"
	if err = repo.PutSession(ctx, "acc_a", "u1b", renewed); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.Session(ctx, "acc_a"); got.SessionID != "jx-session-2" {
		t.Fatalf("renewed session = %+v", got)
	}
	if info, _ = repo.SessionInfo(ctx, "acc_a"); info.LastLoginAt != nil || info.PushedBy != "u1b" || info.LoginFailed {
		t.Fatalf("info after renew = %+v", info)
	}

	if err = repo.DeleteSession(ctx, "acc_a"); err != nil {
		t.Fatal(err)
	}
	if managed(ctx, t, pool, "acc_a") {
		t.Fatal("account still managed after DeleteSession")
	}
	if _, err = repo.SessionInfo(ctx, "acc_a"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("info after delete = %v", err)
	}
	if err = repo.DeleteSession(ctx, "acc_a"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("second delete = %v", err)
	}
	if err = repo.DeleteSession(ctx, "acc_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete on missing account = %v", err)
	}
}

func TestDesiredStateNeedsSession(t *testing.T) {
	ctx := context.Background()
	_, repo := setup(ctx, t)

	if err := repo.SetDesired(ctx, "acc_b", StateRunning, 301, "img:1"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("running without session = %v", err)
	}
	if _, err := repo.Get(ctx, "acc_b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("refused start left a worker: %v", err)
	}
	for _, bad := range []struct {
		state string
		world int
	}{{"paused", 0}, {"", 0}, {StateStopped, -1}} {
		if err := repo.SetDesired(ctx, "acc_b", bad.state, bad.world, ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("SetDesired(%q, %d) = %v", bad.state, bad.world, err)
		}
	}
	if err := repo.SetDesired(ctx, "acc_missing", StateStopped, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}

	// Stopped needs no session and creates the row.
	if err := repo.SetDesired(ctx, "acc_b", StateStopped, 0, ""); err != nil {
		t.Fatal(err)
	}
	w, err := repo.Get(ctx, "acc_b")
	if err != nil || w.DesiredState != StateStopped || w.StartedAt != nil || w.StoppedAt == nil || w.UpdatedAt == nil ||
		w.Session != nil || w.RSN != "B" || w.UserID != "u1" {
		t.Fatalf("stopped worker = %+v, %v", w, err)
	}

	if err = repo.PutSession(ctx, "acc_b", "u1", session); err != nil {
		t.Fatal(err)
	}
	if err = repo.SetError(ctx, "acc_b", "pod failed"); err != nil {
		t.Fatal(err)
	}
	if err = repo.SetDesired(ctx, "acc_b", StateRunning, 301, "img:1"); err != nil {
		t.Fatal(err)
	}
	w, _ = repo.Get(ctx, "acc_b")
	if w.DesiredState != StateRunning || w.World != 301 || w.Image != "img:1" || w.StartedAt == nil || w.StoppedAt != nil ||
		w.LastError != "" || w.Session == nil || w.Session.Character != "Iron Man" {
		t.Fatalf("running worker = %+v", w)
	}
	started := *w.StartedAt

	// Running again keeps started_at; the session cannot go while running.
	if err = repo.SetDesired(ctx, "acc_b", StateRunning, 302, ""); err != nil {
		t.Fatal(err)
	}
	if w, _ = repo.Get(ctx, "acc_b"); !w.StartedAt.Equal(started) || w.World != 302 || w.Image != "img:1" {
		t.Fatalf("running twice = %+v", w)
	}
	if err = repo.DeleteSession(ctx, "acc_b"); !errors.Is(err, ErrWorkerRunning) {
		t.Fatalf("delete while running = %v", err)
	}
	// A restart on the server's image moves only the image of a running worker.
	if moved, merr := repo.SetRunningImage(ctx, "acc_b", "img:2"); merr != nil || !moved {
		t.Fatalf("image of a running worker: %v %v", moved, merr)
	}
	if w, _ = repo.Get(ctx, "acc_b"); w.Image != "img:2" || w.World != 302 || !w.StartedAt.Equal(started) || w.DesiredState != StateRunning {
		t.Fatalf("after the image moved = %+v", w)
	}
	if _, merr := repo.SetRunningImage(ctx, "acc_b", " "); !errors.Is(merr, ErrInvalid) {
		t.Fatalf("empty image = %v", merr)
	}

	if err = repo.SetDesired(ctx, "acc_b", StateStopped, 302, ""); err != nil {
		t.Fatal(err)
	}
	if w, _ = repo.Get(ctx, "acc_b"); w.DesiredState != StateStopped || w.StoppedAt == nil || !w.StartedAt.Equal(started) || w.Image != "img:2" {
		t.Fatalf("stopped again = %+v", w)
	}
	// A stopped worker, or an account without one, is never moved (nor started).
	for _, account := range []string{"acc_b", "acc_a", "acc_missing"} {
		if moved, merr := repo.SetRunningImage(ctx, account, "img:3"); merr != nil || moved {
			t.Fatalf("image of %s: %v %v", account, moved, merr)
		}
	}
	if w, _ = repo.Get(ctx, "acc_b"); w.Image != "img:2" || w.DesiredState != StateStopped {
		t.Fatalf("a stopped worker moved: %+v", w)
	}
	if err = repo.DeleteSession(ctx, "acc_b"); err != nil {
		t.Fatal(err)
	}
	// The worker row outlives the session; running is refused again.
	if w, err = repo.Get(ctx, "acc_b"); err != nil || w.Session != nil {
		t.Fatalf("worker after session delete = %+v, %v", w, err)
	}
	if err = repo.SetDesired(ctx, "acc_b", StateRunning, 0, ""); !errors.Is(err, ErrNoSession) {
		t.Fatalf("running after delete = %v", err)
	}
	if err = repo.SetError(ctx, "acc_a", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error on account without worker = %v", err)
	}
}

func TestWorkerPluginToken(t *testing.T) {
	ctx := context.Background()
	pool, repo := setup(ctx, t)
	tokens := ingest.New(pool, ingest.Definition{})

	if _, err := repo.MintPluginToken(ctx, "acc_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mint without worker = %v", err)
	}
	if err := repo.RevokePluginToken(ctx, "acc_a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke without worker = %v", err)
	}
	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDesired(ctx, "acc_a", StateRunning, 0, "img"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokePluginToken(ctx, "acc_a"); err != nil {
		t.Fatalf("revoke without token = %v", err)
	}

	first, err := repo.MintPluginToken(ctx, "acc_a")
	if err != nil || !strings.HasPrefix(first, ingest.TokenPrefix) {
		t.Fatalf("first token = %q, %v", first, err)
	}
	p, err := tokens.Principal(ctx, ingest.Credential{Token: first})
	if err != nil || p.AccountID != "acc_a" || p.Allows(coreingest.ScopeMarksWrite) ||
		!p.Allows(coreingest.ScopeStateWrite) || !p.Allows(coreingest.ScopeItemsBank) ||
		!p.Allows(coreingest.ScopeItemsInventory) || !p.Allows(coreingest.ScopeItemsEquipment) {
		t.Fatalf("first principal = %+v, %v", p, err)
	}
	w, _ := repo.Get(ctx, "acc_a")
	firstID := w.PluginTokenID
	var label string
	if err = pool.QueryRow(ctx, `SELECT label FROM plugin_ingest_tokens WHERE id = $1`, firstID).Scan(&label); err != nil || label != "worker acc_a" {
		t.Fatalf("label = %q, %v", label, err)
	}

	second, err := repo.MintPluginToken(ctx, "acc_a")
	if err != nil || second == first {
		t.Fatalf("second token = %q, %v", second, err)
	}
	if _, err = tokens.Principal(ctx, ingest.Credential{Token: first}); !errors.Is(err, ingest.ErrUnauthorized) {
		t.Fatalf("first token after replace = %v", err)
	}
	if _, err = tokens.Principal(ctx, ingest.Credential{Token: second}); err != nil {
		t.Fatalf("second token = %v", err)
	}
	if w, _ = repo.Get(ctx, "acc_a"); w.PluginTokenID == "" || w.PluginTokenID == firstID {
		t.Fatalf("token id after replace = %q (first %q)", w.PluginTokenID, firstID)
	}

	if err = repo.RevokePluginToken(ctx, "acc_a"); err != nil {
		t.Fatal(err)
	}
	if _, err = tokens.Principal(ctx, ingest.Credential{Token: second}); !errors.Is(err, ingest.ErrUnauthorized) {
		t.Fatalf("second token after revoke = %v", err)
	}
	if w, _ = repo.Get(ctx, "acc_a"); w.PluginTokenID != "" {
		t.Fatalf("token id after revoke = %q", w.PluginTokenID)
	}
	var live int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM plugin_ingest_tokens WHERE account_id = 'acc_a' AND revoked_at IS NULL`).Scan(&live); err != nil || live != 0 {
		t.Fatalf("live tokens = %d, %v", live, err)
	}
}

func TestListForUser(t *testing.T) {
	ctx := context.Background()
	_, repo := setup(ctx, t)
	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDesired(ctx, "acc_b", StateStopped, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutSession(ctx, "acc_c", "u2", session); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDesired(ctx, "acc_c", StateRunning, 420, "img"); err != nil {
		t.Fatal(err)
	}
	// acc_d has neither a session nor a worker: not listed.

	ids := func(ws []Worker, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(ws))
		for _, w := range ws {
			out = append(out, w.AccountID)
		}
		return strings.Join(out, ",")
	}
	if got := ids(repo.List(ctx)); got != "acc_a,acc_b,acc_c" {
		t.Fatalf("List = %s", got)
	}
	if got := ids(repo.ListForUser(ctx, "u1")); got != "acc_a,acc_b" {
		t.Fatalf("ListForUser(u1) = %s", got)
	}
	if got := ids(repo.ListForUser(ctx, "u2")); got != "acc_c" {
		t.Fatalf("ListForUser(u2) = %s", got)
	}
	if got := ids(repo.ListForUser(ctx, "")); got != "" {
		t.Fatalf("ListForUser('') = %s", got)
	}
	u1, _ := repo.ListForUser(ctx, "u1")
	if a := u1[0]; a.RSN != "A" || a.DesiredState != StateStopped || a.UpdatedAt != nil || a.Session == nil || a.Session.Character != "Iron Man" {
		t.Fatalf("session-only account = %+v", a)
	}
	if b := u1[1]; b.Session != nil || b.UpdatedAt == nil {
		t.Fatalf("worker-only account = %+v", b)
	}
	u2, _ := repo.ListForUser(ctx, "u2")
	if c := u2[0]; c.DesiredState != StateRunning || c.World != 420 || c.RSN != "C" || c.Session == nil {
		t.Fatalf("running account = %+v", c)
	}
}

func TestSessionKeyMismatchAndCopiedRow(t *testing.T) {
	ctx := context.Background()
	pool, repo := setup(ctx, t)
	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}

	rotated := New(pool, testKey(t, 9), nil)
	if _, err := rotated.Session(ctx, "acc_a"); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("session under another key = %v", err)
	}
	// SessionInfo needs no key.
	if info, err := rotated.SessionInfo(ctx, "acc_a"); err != nil || info.Character != "Iron Man" {
		t.Fatalf("info under another key = %+v, %v", info, err)
	}

	// A sealed row copied to another account does not open there.
	if _, err := pool.Exec(ctx, `
		INSERT INTO game_sessions (account_id, character_name, key_id, nonce, ciphertext, pushed_by)
		SELECT 'acc_c', character_name, key_id, nonce, ciphertext, pushed_by FROM game_sessions WHERE account_id = 'acc_a'`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Session(ctx, "acc_c"); !errors.Is(err, vault.ErrOpen) {
		t.Fatalf("copied row = %v", err)
	}
}

func managed(ctx context.Context, t *testing.T, pool *pgxpool.Pool, account string) bool {
	t.Helper()
	var m bool
	if err := pool.QueryRow(ctx, `SELECT managed FROM accounts WHERE id = $1`, account).Scan(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func testKey(t *testing.T, b byte) vault.Key {
	t.Helper()
	k, err := vault.ParseKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, vault.KeySize)))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// setup migrates a throwaway schema with two users: u1 owns acc_a, acc_b and
// acc_d, u2 owns acc_c.
func setup(ctx context.Context, t *testing.T) (*pgxpool.Pool, *Repository) {
	t.Helper()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('u1', 's1', 'u1'), ('u2', 's2', 'u2');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer, created_at) VALUES
		  ('acc_a', 'u1', 'A', 'a', 'ironman', 'oziris-main', 'enhanced', now() - interval '4 minutes'),
		  ('acc_b', 'u1', 'B', 'b', 'ironman', 'oziris-main', 'enhanced', now() - interval '3 minutes'),
		  ('acc_c', 'u2', 'C', 'c', 'ironman', 'oziris-main', 'enhanced', now() - interval '2 minutes'),
		  ('acc_d', 'u1', 'D', 'd', 'ironman', 'oziris-main', 'enhanced', now() - interval '1 minute');
	`); err != nil {
		t.Fatal(err)
	}
	return pool, New(pool, testKey(t, 1), ingest.New(pool, ingest.Definition{}))
}

// testPool applies the migrations in a throwaway schema of DATABASE_URL.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; PostgreSQL workers integration test skipped")
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	schema := fmt.Sprintf("workers_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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

func TestRunningPlugins(t *testing.T) {
	ctx := context.Background()
	_, repo := setup(ctx, t)
	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.RunningPlugins(ctx, "acc_a"); err != nil || len(got) != 0 {
		t.Fatalf("before a worker row: %v %v", got, err)
	}
	if err := repo.SetRunningPlugins(ctx, "acc_a", nil); !errors.Is(err, ErrNotFound) && err != nil {
		t.Fatalf("empty set without a worker row = %v", err)
	}
	if err := repo.SetDesired(ctx, "acc_a", StateRunning, 0, "img"); err != nil {
		t.Fatal(err)
	}
	on := []agent.ProfilePlugin{{Source: agent.PluginHub, Name: "BlackjackPlugin", ClassName: "a.b.BlackjackPlugin", Start: true}}
	if err := repo.SetRunningPlugins(ctx, "acc_a", on); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.RunningPlugins(ctx, "acc_a"); err != nil || len(got) != 1 || got[0] != on[0] {
		t.Fatalf("kept %+v %v", got, err)
	}
	// The same list again writes nothing; an invalid one is refused.
	if err := repo.SetRunningPlugins(ctx, "acc_a", on); err != nil {
		t.Fatal(err)
	}
	bad := []agent.ProfilePlugin{{Source: agent.PluginOwn, Name: "net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin", Start: true}}
	if err := repo.SetRunningPlugins(ctx, "acc_a", bad); err == nil {
		t.Fatal("a protected plugin was kept")
	}
	if err := repo.SetRunningPlugins(ctx, "acc_a", []agent.ProfilePlugin{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.RunningPlugins(ctx, "acc_a"); len(got) != 0 {
		t.Fatalf("after clearing: %+v", got)
	}
}

func TestAutoLoginAndLoggedIn(t *testing.T) {
	ctx := context.Background()
	_, repo := setup(ctx, t)
	if err := repo.SetAutoLogin(ctx, "acc_a", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("auto login without a worker row = %v", err)
	}
	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDesired(ctx, "acc_a", StateRunning, 0, "img"); err != nil {
		t.Fatal(err)
	}
	if w, _ := repo.Get(ctx, "acc_a"); w.AutoLogin || w.WasLoggedIn {
		t.Fatalf("defaults: %+v", w)
	}
	if err := repo.SetAutoLogin(ctx, "acc_a", true); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetLoggedIn(ctx, "acc_a", true); err != nil {
		t.Fatal(err)
	}
	// A restart (running again) and a new image keep both; a stop clears
	// that the client was logged in, never the option.
	if err := repo.SetDesired(ctx, "acc_a", StateRunning, 0, "img2"); err != nil {
		t.Fatal(err)
	}
	if w, _ := repo.Get(ctx, "acc_a"); !w.AutoLogin || !w.WasLoggedIn {
		t.Fatalf("after a running update: %+v", w)
	}
	if err := repo.SetDesired(ctx, "acc_a", StateStopped, 0, ""); err != nil {
		t.Fatal(err)
	}
	if w, _ := repo.Get(ctx, "acc_a"); !w.AutoLogin || w.WasLoggedIn {
		t.Fatalf("after a stop: %+v", w)
	}
}

func TestRunningSettings(t *testing.T) {
	ctx := context.Background()
	_, repo := setup(ctx, t)
	if err := repo.PutSession(ctx, "acc_a", "u1", session); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDesired(ctx, "acc_a", StateRunning, 0, "img"); err != nil {
		t.Fatal(err)
	}
	on := []agent.ProfileSetting{{Group: "blackjack", Key: "room", Value: "THUG_TENT"}}
	if err := repo.SetRunningSettings(ctx, "acc_a", on); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.RunningSettings(ctx, "acc_a"); err != nil || len(got) != 1 || got[0] != on[0] {
		t.Fatalf("kept %+v %v", got, err)
	}
	if err := repo.SetRunningSettings(ctx, "acc_a", []agent.ProfileSetting{{Group: "blackjack", Key: "password", Value: "x"}}); err == nil {
		t.Fatal("a secret key was kept")
	}
	if err := repo.SetRunningSettings(ctx, "acc_a", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.RunningSettings(ctx, "acc_a"); len(got) != 0 {
		t.Fatalf("after clearing: %+v", got)
	}
}
