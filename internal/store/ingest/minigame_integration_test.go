package ingest

import (
	"context"
	"encoding/json"
	"testing"

	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
)

// The minigame counters (docs/06, adendo 2026-11-16): each key present
// replaces its value, in a full push and in a delta; a key absent stays,
// since the client reads a counter only when its interface is on screen.
func TestMinigamePostgres(t *testing.T) {
	ctx := context.Background()
	pool := testIngestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-minigame-test', 'dev-minigame-test');
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
	if err != nil || !manifest.Minigame {
		t.Fatalf("manifest = %+v, %v", manifest, err)
	}
	accept := func(p coreingest.Push) {
		t.Helper()
		raw, _ := json.Marshal(p)
		if r, err := repository.Accept(ctx, created.Token, "127.0.0.1", raw, p); err != nil || !r.OK {
			t.Fatalf("accept seq %d = %+v, %v", p.ClientSeq, r, err)
		}
	}
	state := func() map[string]string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT key, value::text FROM player_state WHERE account_id = 'account-1' AND facet = 'minigame'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatal(err)
			}
			out[k] = v
		}
		return out
	}

	full := repositoryFullPush(1, repositoryRestored(1, map[string]int64{}))
	full.Data.Minigame = &coreingest.IntMap{"mta.telekinetic": 204, "mta.graveyard": 152, "mta.enchantment": 1342, "mta.alchemist": 140}
	accept(full)
	if got := state(); len(got) != 4 || got["mta.graveyard"] != `{"value": 152}` {
		t.Fatalf("after the full push = %v", got)
	}

	delta := repositoryDeltaPush(2, repositoryRestored(2, map[string]int64{}))
	delta.Data.Items = nil
	delta.Data.Minigame = &coreingest.IntMap{"mta.graveyard": 170}
	accept(delta)
	if got := state(); len(got) != 4 || got["mta.graveyard"] != `{"value": 170}` || got["mta.telekinetic"] != `{"value": 204}` {
		t.Fatalf("after the delta = %v", got)
	}

	// A full push without the counters (the client restarted and has not
	// seen the arena yet) leaves them as they were.
	accept(repositoryFullPush(3, repositoryRestored(3, map[string]int64{})))
	if got := state(); len(got) != 4 || got["mta.alchemist"] != `{"value": 140}` {
		t.Fatalf("after a full push without them = %v", got)
	}
	if n := countRows(ctx, t, pool, `SELECT count(*) FROM snapshot_facts WHERE facet = 'minigame'`); n != 5 {
		t.Fatalf("minigame history rows = %d, want 5", n)
	}
}
