package ingest

import (
	"context"
	"encoding/json"
	"testing"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

// The Time Tracking clocks and the position (docs/06, adendo 2026-11-10): a
// full push replaces the clocks, a delta patches them, the position is the
// last one and leaves no snapshot_facts history.
func TestTimersAndPositionPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testIngestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-timer-test', 'dev-timer-test');
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
	if err != nil || !manifest.Timers || !manifest.Position {
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
		rows, err := pool.Query(ctx, `SELECT facet || '/' || key, value::text FROM player_state WHERE account_id = 'account-1' AND facet IN ('timer', 'position')`)
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
	ready := int64(1794361000)

	full := repositoryFullPush(1, repositoryRestored(1, map[string]int64{}))
	full.Data.Timers = &coreingest.TimerMap{
		"HERB":       {State: "IN_PROGRESS", ReadyAt: &ready, Label: "Herbs"},
		"BIRD_HOUSE": {State: "EMPTY", Label: "Bird Houses"},
	}
	full.Data.Position = &coreingest.Position{World: 330, X: 3222, Y: 3218}
	accept(full)
	got := state()
	if len(got) != 3 || got["timer/HERB"] != `{"label": "Herbs", "state": "IN_PROGRESS", "ready_at": 1794361000}` ||
		got["position/player"] != `{"x": 3222, "y": 3218, "plane": 0, "world": 330, "region": 12850}` {
		t.Fatalf("after the full push = %v", got)
	}

	delta := repositoryDeltaPush(2, repositoryRestored(2, map[string]int64{}))
	delta.Data.Items = nil
	delta.Data.Timers = &coreingest.TimerMap{"HERB": {State: "COMPLETED", ReadyAt: &ready, Label: "Herbs"}}
	delta.Data.Position = &coreingest.Position{World: 330, X: 2662, Y: 3305}
	accept(delta)
	got = state()
	if len(got) != 3 || got["timer/BIRD_HOUSE"] == "" || got["timer/HERB"] != `{"label": "Herbs", "state": "COMPLETED", "ready_at": 1794361000}` ||
		got["position/player"] != `{"x": 2662, "y": 3305, "plane": 0, "world": 330, "region": 10547}` {
		t.Fatalf("after the delta = %v", got)
	}

	// A full push replaces the clocks: a tab that is gone is gone.
	full = repositoryFullPush(3, repositoryRestored(3, map[string]int64{}))
	full.Data.Timers = &coreingest.TimerMap{"HERB": {State: "EMPTY"}}
	accept(full)
	if got = state(); got["timer/BIRD_HOUSE"] != "" || got["timer/HERB"] != `{"state": "EMPTY"}` || got["position/player"] == "" {
		t.Fatalf("after the second full = %v", got)
	}
	if n := countRows(ctx, t, pool, `SELECT count(*) FROM snapshot_facts WHERE facet = 'position'`); n != 0 {
		t.Fatalf("position history rows = %d", n)
	}
}
