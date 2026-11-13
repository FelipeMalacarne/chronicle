package progress

import (
	"context"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/store/state"
)

// Item 16 (docs/04 section 9): a push carrying the collection log pages the
// player opened reaches the account screen as the obtained items per page,
// by tab, with the slots still missing; the game's counters stay apart.
func TestCollectionLogAccountStatePostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO guides (id) VALUES ('guide-1') ON CONFLICT DO NOTHING;
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-clog-state', 'dev-clog-state');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'guide-1');
	`); err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, nil, catalog)
	def := storeingest.BuildDefinition(nil, catalog)
	ingest := storeingest.New(pool, def)
	token, err := ingest.CreateToken(ctx, "account-1", "desktop", nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil || !manifest.CollectionLog {
		t.Fatalf("manifest = %+v, %v", manifest, err)
	}

	st, err := repo.AccountState(ctx, "account-1")
	if err != nil || st.CollectionLogPages == nil || len(st.CollectionLogPages) != 0 {
		t.Fatalf("before any page: %+v, %v", st.CollectionLogPages, err)
	}

	p := questPush(1, manifest.ManifestVersion, coreingest.PushFull, nil, map[string]int64{}, map[string]int64{"2943": 3, "2944": 1692})
	read := p.SentAt.Add(-10 * time.Second)
	p.Data.CollectionLog = &coreingest.CollectionLogMap{
		"Barrows Chests": {Tab: "BOSSES", ObservedAt: read, Items: coreingest.QuantityMap{"4708": 1, "4710": 0, "4712": 2}},
		"All Pets":       {Tab: "OTHER", ObservedAt: read, Items: coreingest.QuantityMap{"12646": 0, "13262": 1}},
	}
	push(ctx, t, ingest, token.Token, p)

	st, err = repo.AccountState(ctx, "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.CollectionLogPages) != 2 {
		t.Fatalf("pages = %+v", st.CollectionLogPages)
	}
	barrows, pets := st.CollectionLogPages[0], st.CollectionLogPages[1]
	if barrows.Page != "Barrows Chests" || barrows.Tab != "BOSSES" || barrows.Obtained != 2 || barrows.Total != 3 ||
		barrows.Source != "plugin" || !barrows.ObservedAt.Equal(read) {
		t.Fatalf("barrows = %+v", barrows)
	}
	if len(barrows.Items) != 2 || barrows.Items[0] != (CollectionLogItemView{ID: 4708, Name: "Ahrim's hood", Qty: 1}) ||
		barrows.Items[1] != (CollectionLogItemView{ID: 4712, Name: "Ahrim's robetop", Qty: 2}) ||
		len(barrows.Missing) != 1 || barrows.Missing[0] != (CollectionLogItemView{ID: 4710, Name: "Ahrim's staff"}) {
		t.Fatalf("barrows slots = %+v / %+v", barrows.Items, barrows.Missing)
	}
	if pets.Page != "All Pets" || pets.Tab != "OTHER" || pets.Obtained != 1 || pets.Items[0].Name != "Abyssal orphan" {
		t.Fatalf("pets = %+v", pets)
	}
	// The game's counters come from the varps, apart from the pages.
	if st.CollectionLog == nil || st.CollectionLog.Obtained != 3 || st.CollectionLog.Total == nil || *st.CollectionLog.Total != 1692 {
		t.Fatalf("collection log counters = %+v", st.CollectionLog)
	}

	// player_state holds the pages as the merge of their observations (the
	// facts part of `chronicle progress check`).
	if diff, err := state.Diff(ctx, storedb.New(pool), "account-1"); err != nil || len(diff) != 0 {
		t.Fatalf("facts check = %v, %v", diff, err)
	}
}
