package ingest

import (
	"context"
	"encoding/json"
	"testing"

	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
)

// The membership (docs/06, adendo 2026-11-24): one fact, replaced by the
// value of a full push and of a delta; a push without it leaves it as it was.
func TestMembershipPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testIngestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-membership-test', 'dev-membership-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'guide-1');
	`); err != nil {
		t.Fatal(err)
	}
	repository := New(pool, Definition{Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10})
	created, err := repository.CreateToken(ctx, "account-1", "desktop", []coreingest.TokenScope{coreingest.ScopeItemsBank})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := repository.Manifest(ctx, created.Token)
	if err != nil || !manifest.Membership {
		t.Fatalf("manifest = %+v, %v", manifest, err)
	}
	accept := func(p coreingest.Push) {
		t.Helper()
		raw, _ := json.Marshal(p)
		if r, err := repository.Accept(ctx, created.Token, "127.0.0.1", raw, p); err != nil || !r.OK {
			t.Fatalf("accept seq %d = %+v, %v", p.ClientSeq, r, err)
		}
	}
	state := func() string {
		t.Helper()
		var v string
		err := pool.QueryRow(ctx, `SELECT value::text FROM player_state WHERE account_id = 'account-1' AND facet = 'membership' AND key = 'membership'`).Scan(&v)
		if err != nil {
			return ""
		}
		return v
	}
	days := func(d int64, member bool) *coreingest.Membership {
		return &coreingest.Membership{Days: &d, Member: &member}
	}

	full := repositoryFullPush(1, repositoryRestored(1, map[string]int64{}))
	full.Data.Membership = days(120, true)
	accept(full)
	if got := state(); got != `{"days": 120, "member": true}` {
		t.Fatalf("after the full push = %q", got)
	}

	delta := repositoryDeltaPush(2, repositoryRestored(2, map[string]int64{}))
	delta.Data.Items = nil
	delta.Data.Membership = days(119, true)
	accept(delta)
	if got := state(); got != `{"days": 119, "member": true}` {
		t.Fatalf("after the delta = %q", got)
	}

	// A full push without it (the login has not settled) leaves it as it was.
	accept(repositoryFullPush(3, repositoryRestored(3, map[string]int64{})))
	if got := state(); got != `{"days": 119, "member": true}` {
		t.Fatalf("after a full push without it = %q", got)
	}

	// The membership ran out.
	lapsed := repositoryDeltaPush(4, repositoryRestored(4, map[string]int64{}))
	lapsed.Data.Items = nil
	lapsed.Data.Membership = days(0, false)
	accept(lapsed)
	if got := state(); got != `{"days": 0, "member": false}` {
		t.Fatalf("after it ran out = %q", got)
	}
	if n := countRows(ctx, t, pool, `SELECT count(*) FROM snapshot_facts WHERE facet = 'membership'`); n != 3 {
		t.Fatalf("membership history rows = %d, want 3", n)
	}
}
