package progress

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

func TestCollectionLogPages(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	row := func(key, value string) storedb.PlayerState {
		return storedb.PlayerState{
			Facet: "collection_log", Key: key, Value: json.RawMessage(value), Source: "plugin",
			ObservedAt: pgtype.Timestamptz{Time: at, Valid: true},
		}
	}
	if got := collectionLogPages(reg, nil); got == nil || len(got) != 0 {
		t.Fatalf("no rows = %#v, want an empty list", got)
	}
	got := collectionLogPages(reg, []storedb.PlayerState{
		row("page:All Pets", `{"items": {"12646": 0, "13262": 1}}`),
		row("page:Barrows Chests", `{"tab": "BOSSES", "items": {"4712": 2, "4708": 1, "4710": 0}}`),
		row("page:Chambers of Xeric", `{"tab": "RAIDS", "items": {"999999999": 3}}`),
		// A slot of the collectionLogSlots bitset, keyed by item id: not a page.
		row("4151", `{"qty": 1}`),
		row("page:Broken", `[1, 2]`),
	})
	src := FactSource{Source: "plugin", ObservedAt: at}
	want := []CollectionLogPageView{
		{
			Page: "Barrows Chests", Tab: "BOSSES", Obtained: 2, Total: 3,
			Items:      []CollectionLogItemView{{ID: 4708, Name: "Ahrim's hood", Qty: 1}, {ID: 4712, Name: "Ahrim's robetop", Qty: 2}},
			Missing:    []CollectionLogItemView{{ID: 4710, Name: "Ahrim's staff"}},
			FactSource: src,
		},
		{
			// An id the catalog does not know keeps its number as the name.
			Page: "Chambers of Xeric", Tab: "RAIDS", Obtained: 1, Total: 1,
			Items:      []CollectionLogItemView{{ID: 999999999, Name: "999999999", Qty: 3}},
			Missing:    []CollectionLogItemView{},
			FactSource: src,
		},
		{
			// No tab: after the five tabs.
			Page: "All Pets", Obtained: 1, Total: 2,
			Items:      []CollectionLogItemView{{ID: 13262, Name: "Abyssal orphan", Qty: 1}},
			Missing:    []CollectionLogItemView{{ID: 12646, Name: "Baby Mole"}},
			FactSource: src,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pages = %+v\nwant %+v", got, want)
	}
	raw, _ := json.Marshal(want[0].Missing[0])
	if string(raw) != `{"id":4710,"name":"Ahrim's staff"}` {
		t.Fatalf("a missing slot has no quantity on the wire: %s", raw)
	}
}
