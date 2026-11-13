package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
	storerecording "github.com/FelipeMalacarne/chronicle/internal/store/recording"
)

// The demonstration recording (docs/10 section 11): the ingest stores each
// event once, even when a push resends it; the raw snapshot does not keep
// it; the owner reads it in order and deletes it on request.
func TestRecordingPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testIngestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-recording-test', 'dev-recording-test');
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
	if manifest, merr := repository.Manifest(ctx, created.Token); merr != nil || !manifest.Recording {
		t.Fatalf("manifest = %+v, %v", manifest, merr)
	}
	accept := func(p coreingest.Push) {
		t.Helper()
		raw, _ := json.Marshal(p)
		if r, aerr := repository.Accept(ctx, created.Token, "127.0.0.1", raw, p); aerr != nil || !r.OK {
			t.Fatalf("accept seq %d = %+v, %v", p.ClientSeq, r, aerr)
		}
	}
	event := func(seq int64, at time.Time, kind string, edit func(*recording.Event)) recording.Event {
		e := recording.Event{Recording: "rec-test", Seq: seq, At: at, Kind: kind, World: 330, X: 3222, Y: 3218}
		if edit != nil {
			edit(&e)
		}
		return e
	}

	full := repositoryFullPush(1, repositoryRestored(1, map[string]int64{}))
	t1 := full.SentAt.Add(-30 * time.Second)
	first := []recording.Event{
		event(1, t1, recording.KindMove, nil),
		event(2, t1.Add(time.Second), recording.KindNPC, func(e *recording.Event) { e.NPC, e.Option, e.Name = recording.Int(3080), "Talk-to", "Duke Horacio" }),
	}
	full.Data.Recording = &first
	accept(full)

	// The next push resends event 2 (its ack was lost) with a new one.
	delta := repositoryDeltaPush(2, repositoryRestored(2, map[string]int64{}))
	delta.Data.Items = nil
	second := []recording.Event{
		first[1],
		event(3, t1.Add(2*time.Second), recording.KindDialogue, func(e *recording.Event) { e.Option, e.NPC = "Have you any quests for me?", recording.Int(3080) }),
	}
	delta.Data.Recording = &second
	accept(delta)

	var stored, inRaw int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM recorded_events WHERE account_id = 'account-1'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM snapshots WHERE account_id = 'account-1' AND raw->'data' ? 'recording'`).Scan(&inRaw); err != nil {
		t.Fatal(err)
	}
	if stored != 3 || inRaw != 0 {
		t.Fatalf("stored %d events, %d raw snapshots keep the recording", stored, inRaw)
	}

	reader := storerecording.New(pool)
	got, err := reader.Events(ctx, "account-1", t1, t1.Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Seq != 1 || got[1].Kind != recording.KindNPC || *got[1].NPC != 3080 || got[2].Option != "Have you any quests for me?" {
		t.Fatalf("events = %+v", got)
	}
	if page, _ := reader.Events(ctx, "account-1", t1.Add(time.Second), t1.Add(time.Hour), 1); len(page) != 1 || page[0].Seq != 2 {
		t.Fatalf("page = %+v", page)
	}
	if _, err := reader.Events(ctx, "account-9", t1, t1.Add(time.Hour), 10); err != storerecording.ErrNotFound {
		t.Fatalf("unknown account = %v", err)
	}
	if n, err := reader.Delete(ctx, "account-1", "rec-other"); err != nil || n != 0 {
		t.Fatalf("delete another recording = %d, %v", n, err)
	}
	if n, err := reader.Delete(ctx, "account-1", ""); err != nil || n != 3 {
		t.Fatalf("delete = %d, %v", n, err)
	}
}
