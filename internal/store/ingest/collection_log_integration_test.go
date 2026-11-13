package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
)

// The collection log pages the plugin read (docs/06, adendo 2026-11-13): a
// page present replaces that page in a full push and in a delta, an older
// read never replaces a newer one, pages a push does not carry stay, and the
// collectionLogSlots bitset replaces its slots without touching the pages.
func TestCollectionLogPagesPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testIngestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-clog-test', 'dev-clog-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'guide-1');
	`); err != nil {
		t.Fatal(err)
	}
	repository := New(pool, Definition{Version: 7, Varbits: []int64{}, Varps: []int64{}, Collections: []int64{4151}, IntervalSeconds: 10})
	created, err := repository.CreateToken(ctx, "account-1", "desktop", []coreingest.TokenScope{coreingest.ScopeItemsBank})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := repository.Manifest(ctx, created.Token)
	if err != nil || !manifest.CollectionLog {
		t.Fatalf("manifest = %+v, %v", manifest, err)
	}
	accept := func(p coreingest.Push) {
		t.Helper()
		raw, _ := json.Marshal(p)
		if r, err := repository.Accept(ctx, created.Token, "127.0.0.1", raw, p); err != nil || !r.OK {
			t.Fatalf("accept seq %d = %+v, %v", p.ClientSeq, r, err)
		}
	}
	type fact struct {
		Tab   string           `json:"tab"`
		Items map[string]int64 `json:"items"`
		Qty   int64            `json:"qty"`
		At    time.Time        `json:"-"`
	}
	state := func() map[string]fact {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT key, value, observed_at FROM player_state WHERE account_id = 'account-1' AND facet = 'collection_log'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]fact{}
		for rows.Next() {
			var key string
			var raw []byte
			var at time.Time
			if err := rows.Scan(&key, &raw, &at); err != nil {
				t.Fatal(err)
			}
			var f fact
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatal(err)
			}
			f.At = at.UTC()
			out[key] = f
		}
		return out
	}
	none := repositoryRestored(0, map[string]int64{})

	// A full push with the two pages read this session.
	full := repositoryFullPush(1, none)
	barrowsRead := full.SentAt.Add(-30 * time.Second)
	full.Data.CollectionLog = &coreingest.CollectionLogMap{
		"Barrows Chests": {Tab: "BOSSES", ObservedAt: barrowsRead, Items: coreingest.QuantityMap{"4708": 1, "4710": 0}},
		"All Pets":       {ObservedAt: full.SentAt.Add(-time.Minute), Items: coreingest.QuantityMap{"12646": 0}},
	}
	accept(full)
	got := state()
	if len(got) != 2 || !reflect.DeepEqual(got["page:Barrows Chests"].Items, map[string]int64{"4708": 1, "4710": 0}) ||
		got["page:Barrows Chests"].Tab != "BOSSES" || !got["page:Barrows Chests"].At.Equal(barrowsRead) ||
		got["page:All Pets"].Tab != "" || got["page:All Pets"].Items["12646"] != 0 {
		t.Fatalf("after the full push = %+v", got)
	}
	if n := countRows(ctx, t, pool, `SELECT count(*) FROM snapshot_facts WHERE facet = 'collection_log'`); n != 2 {
		t.Fatalf("snapshot facts = %d, want one per page", n)
	}

	// A delta: the Barrows page read again (the staff obtained) and a new page.
	delta := repositoryDeltaPush(2, none)
	delta.Data.Items = nil
	barrowsAgain := delta.SentAt.Add(-5 * time.Second)
	delta.Data.CollectionLog = &coreingest.CollectionLogMap{
		"Barrows Chests":    {Tab: "BOSSES", ObservedAt: barrowsAgain, Items: coreingest.QuantityMap{"4708": 1, "4710": 1}},
		"Chambers of Xeric": {Tab: "RAIDS", ObservedAt: barrowsAgain, Items: coreingest.QuantityMap{"20997": 0}},
	}
	accept(delta)
	got = state()
	if len(got) != 3 || got["page:Barrows Chests"].Items["4710"] != 1 || !got["page:Barrows Chests"].At.Equal(barrowsAgain) ||
		got["page:All Pets"].Items == nil || got["page:Chambers of Xeric"].Tab != "RAIDS" {
		t.Fatalf("after the delta = %+v", got)
	}

	// A full push without the facet (an older plugin, or no page read since
	// login) keeps the pages: the client only knows what it opened.
	accept(repositoryFullPush(3, none))
	if got = state(); len(got) != 3 {
		t.Fatalf("after a full push without pages = %+v", got)
	}

	// An older read of a page never replaces a newer one.
	stale := repositoryDeltaPush(4, none)
	stale.Data.Items = nil
	stale.Data.CollectionLog = &coreingest.CollectionLogMap{
		"Barrows Chests": {Tab: "BOSSES", ObservedAt: barrowsRead, Items: coreingest.QuantityMap{"4708": 1, "4710": 0}},
	}
	accept(stale)
	if got = state(); got["page:Barrows Chests"].Items["4710"] != 1 || !got["page:Barrows Chests"].At.Equal(barrowsAgain) {
		t.Fatalf("an older read replaced the page: %+v", got["page:Barrows Chests"])
	}

	// The bitset replaces its slots (keyed by item id) and leaves the pages.
	slots := base64.StdEncoding.EncodeToString([]byte{1})
	bitset := repositoryDeltaPush(5, none)
	bitset.Data.Items = nil
	bitset.Data.CollectionLogSlots = &slots
	accept(bitset)
	if got = state(); len(got) != 4 || got["4151"].Qty != 1 || got["page:All Pets"].Items == nil {
		t.Fatalf("after the bitset = %+v", got)
	}

	// An item id the database cannot hold rejects the push.
	huge := repositoryDeltaPush(6, none)
	huge.Data.Items = nil
	huge.Data.CollectionLog = &coreingest.CollectionLogMap{
		"Barrows Chests": {ObservedAt: huge.SentAt, Items: coreingest.QuantityMap{"4294967296": 1}},
	}
	raw, _ := json.Marshal(huge)
	if _, err := repository.Accept(ctx, created.Token, "127.0.0.1", raw, huge); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("an item id outside int4 = %v, want ErrInvalidPayload", err)
	}
	if got = state(); len(got) != 4 || got["page:Barrows Chests"].Items["4710"] != 1 {
		t.Fatalf("a rejected push changed the pages: %+v", got)
	}
}
