package syncrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// growingFetcher answers the same hiscores sample with the attack XP grown
// by one per call, so every sync writes new facts.
type growingFetcher struct {
	raw []byte
	n   atomic.Int64
}

func (f *growingFetcher) Fetch(context.Context, string, string) (hiscores.Result, error) {
	n := f.n.Add(1)
	raw := strings.Replace(string(f.raw), `"name":"Attack","rank":65103,"level":99,"xp":13082918`,
		fmt.Sprintf(`"name":"Attack","rank":65103,"level":99,"xp":%d`, 13082918+n), 1)
	return hiscores.Result{Raw: []byte(raw), Table: "hiscore_oldschool_ironman"}, nil
}

// TestPushAndSyncConcurrentPostgres interleaves plugin pushes and hiscores
// syncs of one account. Both write player_state and then reevaluate; with the
// account locked first in both (docs/10 section 6, R4) neither may fail with a
// deadlock.
func TestPushAndSyncConcurrentPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-push-sync', 'dev-push-sync');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'ironman260', 'ironman260', 'ironman', 'g1', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, loadContent(t), catalog)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "data", "samples", "hiscores", "ironman260.json"))
	if err != nil {
		t.Fatal(err)
	}
	syncs := New(pool, &growingFetcher{raw: raw}, progress)
	ingest := storeingest.New(pool, storeingest.Definition{
		Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10,
	}).WithReevaluator(progress)
	token, err := ingest.CreateToken(ctx, "account-1", "desktop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.Manifest(ctx, token.Token); err != nil {
		t.Fatal(err)
	}

	const rounds = 15
	var wg sync.WaitGroup
	errs := make(chan error, 2*rounds)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range rounds {
			run, err := syncs.Run(ctx, "account-1", TriggerScheduled)
			if err != nil || run.Status != "ok" {
				errs <- fmt.Errorf("sync %d: %+v, %v", i, run, err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := range rounds {
			p := levelsPush(int64(i+1), map[string]int64{"attack": 99, "strength": 99, "defence": 99, "hitpoints": 99})
			body, _ := json.Marshal(p)
			if resp, err := ingest.Accept(ctx, token.Token, "127.0.0.1", body, p); err != nil || !resp.OK {
				errs <- fmt.Errorf("push %d: %+v, %v", i, resp, err)
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if rep, err := progress.Check(ctx, "account-1"); err != nil || !rep.OK {
		t.Fatalf("progress check after interleaved writes: %v, %v", rep.Differences, err)
	}
}

func levelsPush(seq int64, levels map[string]int64) coreingest.Push {
	lv := coreingest.IntMap(levels)
	empty := coreingest.IntMap{}
	sentAt := time.Now().Add(-time.Second)
	unavailable := func() *coreingest.ItemObservation {
		return &coreingest.ItemObservation{State: coreingest.ItemUnavailable, ObservedAt: sentAt}
	}
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion, Username: "ironman260", Profile: "STANDARD",
		ManifestVersion: 7, ClientSeq: seq, Kind: coreingest.PushFull,
		SentAt: sentAt,
		Data: coreingest.PushData{
			Varbits: &empty, Varps: &empty, Levels: &lv, XP: &empty, Boosted: &empty,
			Items: &coreingest.ItemContainers{Bank: unavailable(), Inventory: unavailable(), Equipment: unavailable()},
		},
	}
}

// TestFactsMergePostgres: player_state is the merge of each source's last
// observation (docs/10 section 5, E1). A plugin push with less XP than the
// hiscores, sent after them, must not take the skill back, and the plugin's
// boosted level survives a hiscores update.
func TestFactsMergePostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-merge', 'dev-merge');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'ironman260', 'ironman260', 'ironman', 'g1', 'classic');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, loadContent(t), catalog)
	syncs := New(pool, &fakeFetcher{file: "ironman260.json"}, progress)
	ingest := storeingest.New(pool, storeingest.Definition{
		Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10,
	}).WithReevaluator(progress)
	token, err := ingest.CreateToken(ctx, "account-1", "desktop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ingest.Manifest(ctx, token.Token); err != nil {
		t.Fatal(err)
	}
	attack := func() (xp, boosted int64, source string) {
		t.Helper()
		var v struct{ XP, Boosted int64 }
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT value, source FROM player_state WHERE account_id = 'account-1' AND facet = 'skill' AND key = 'attack'`).Scan(&raw, &source); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return v.XP, v.Boosted, source
	}
	pluginPush := func(seq int64) {
		t.Helper()
		p := levelsPush(seq, map[string]int64{"attack": 99})
		xp, boosted := coreingest.IntMap{"attack": 13_000_000}, coreingest.IntMap{"attack": 105}
		p.Data.XP, p.Data.Boosted = &xp, &boosted
		body, _ := json.Marshal(p)
		if resp, err := ingest.Accept(ctx, token.Token, "127.0.0.1", body, p); err != nil || !resp.OK {
			t.Fatalf("push %d: %+v, %v", seq, resp, err)
		}
	}

	pluginPush(1)
	if xp, boosted, source := attack(); xp != 13_000_000 || boosted != 105 || source != "plugin" {
		t.Fatalf("after the plugin: xp %d boosted %d from %s", xp, boosted, source)
	}
	if run, err := syncs.Run(ctx, "account-1", TriggerScheduled); err != nil || run.Status != "ok" {
		t.Fatalf("sync: %+v, %v", run, err)
	}
	if xp, boosted, source := attack(); xp != 13_082_918 || boosted != 105 || source != "hiscores" {
		t.Fatalf("after the hiscores: xp %d boosted %d from %s (boosted must survive)", xp, boosted, source)
	}
	// A later full push with less XP replaces the plugin's own observation,
	// never the hiscores one: the merged skill keeps the greatest XP.
	pluginPush(2)
	if xp, boosted, source := attack(); xp != 13_082_918 || boosted != 105 || source != "hiscores" {
		t.Fatalf("after a stale plugin push: xp %d boosted %d from %s", xp, boosted, source)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM observations WHERE facet = 'skill' AND key = 'attack'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("attack observations = %d, %v (one per source)", n, err)
	}
	if rep, err := progress.Check(ctx, "account-1"); err != nil || !rep.OK {
		t.Fatalf("progress check: %v, %v", rep.Differences, err)
	}
	// A fact written around the projection is reported.
	if _, err := pool.Exec(ctx, `UPDATE player_state SET value = '{"level":1,"xp":0}' WHERE facet = 'skill' AND key = 'attack'`); err != nil {
		t.Fatal(err)
	}
	if rep, err := progress.Check(ctx, "account-1"); err != nil || rep.OK || !strings.Contains(strings.Join(rep.Differences, "\n"), "facts: skill attack") {
		t.Fatalf("check must report the stale fact: %+v, %v", rep, err)
	}
}
