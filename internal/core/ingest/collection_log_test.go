package ingest

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func barrowsPage() CollectionLogPage {
	return CollectionLogPage{Tab: "BOSSES", ObservedAt: testTime.Add(-time.Minute), Items: QuantityMap{"4708": 1, "4710": 0, "4712": 2}}
}

func TestCollectionLogValidation(t *testing.T) {
	page := func(edit func(*CollectionLogPage)) CollectionLogPage {
		p := barrowsPage()
		edit(&p)
		return p
	}
	tooMany := CollectionLogMap{}
	for i := range MaxCollectionLogPages + 1 {
		tooMany["Page "+strconv.Itoa(i)] = barrowsPage()
	}
	bigPage := barrowsPage()
	bigPage.Items = QuantityMap{}
	for i := 1; i <= MaxCollectionLogPageItems+1; i++ {
		bigPage.Items[strconv.Itoa(i)] = 0
	}
	cases := []struct {
		name    string
		pages   *CollectionLogMap
		wantErr string
	}{
		{"a page", &CollectionLogMap{"Barrows Chests": barrowsPage()}, ""},
		{"no tab", &CollectionLogMap{"All Pets": page(func(p *CollectionLogPage) { p.Tab = "" })}, ""},
		{"every tab", &CollectionLogMap{
			"Chambers of Xeric":        page(func(p *CollectionLogPage) { p.Tab = "RAIDS" }),
			"Beginner Treasure Trails": page(func(p *CollectionLogPage) { p.Tab = "CLUES" }),
			"Barbarian Assault":        page(func(p *CollectionLogPage) { p.Tab = "MINIGAMES" }),
			"All Pets":                 page(func(p *CollectionLogPage) { p.Tab = "OTHER" }),
		}, ""},
		{"empty object", &CollectionLogMap{}, ""},
		{"null object", func() *CollectionLogMap { var m CollectionLogMap; return &m }(), "data.collectionLog must be a JSON object"},
		{"too many pages", &tooMany, "more than 500 pages"},
		{"untrimmed title", &CollectionLogMap{" Barrows Chests": barrowsPage()}, "non-empty trimmed page title"},
		{"empty title", &CollectionLogMap{"": barrowsPage()}, "non-empty trimmed page title"},
		{"long title", &CollectionLogMap{strings.Repeat("x", MaxCollectionLogPageLength+1): barrowsPage()}, "longer than 80 bytes"},
		{"unknown tab", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.Tab = "Bosses" })}, `tab "Bosses" is invalid`},
		{"no read time", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.ObservedAt = time.Time{} })}, "observed_at is required"},
		{"read after sent", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.ObservedAt = testTime.Add(time.Second) })}, "observed_at must not be after sent_at"},
		{"no slots", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.Items = QuantityMap{} })}, "items must hold the page's slots"},
		{"too many slots", &CollectionLogMap{"Barrows Chests": bigPage}, "more than 1000 slots"},
		{"bad item id", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.Items = QuantityMap{"04708": 1} })}, "positive canonical item id"},
		{"zero item id", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.Items = QuantityMap{"0": 1} })}, "positive canonical item id"},
		{"negative quantity", &CollectionLogMap{"Barrows Chests": page(func(p *CollectionLogPage) { p.Items = QuantityMap{"4708": -1} })}, "must not be negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The same rules in a full push and in a delta: a page replaces
			// itself in both.
			for _, push := range []Push{fullPush(), deltaPush(nil)} {
				push.Data.CollectionLog = tc.pages
				err := push.Validate()
				if tc.wantErr == "" && err != nil {
					t.Fatalf("%s: Validate() = %v", push.Kind, err)
				}
				if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
					t.Fatalf("%s: Validate() = %v, want %q", push.Kind, err, tc.wantErr)
				}
			}
		})
	}
}

func TestCollectionLogNeedsManifest(t *testing.T) {
	manifest := Manifest{
		ContractVersion: ContractVersion, ManifestVersion: 7, TokenScopes: []TokenScope{ScopeStateWrite},
		Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10,
	}
	push := deltaPush(nil)
	push.Data.Items = nil
	push.Data.CollectionLog = &CollectionLogMap{"Barrows Chests": barrowsPage()}
	if err := push.ValidateAgainst(manifest); err == nil || !strings.Contains(err.Error(), "data.collectionLog is not enabled") {
		t.Fatalf("ValidateAgainst() = %v, want the gate", err)
	}
	manifest.CollectionLog = true
	if err := push.ValidateAgainst(manifest); err != nil {
		t.Fatalf("ValidateAgainst() with the flag = %v", err)
	}

	// The wire shape the plugin sends (Map<String, CollectionLogPage>), read
	// back strictly like the ingest handler does.
	raw := `{"collectionLog":{"Barrows Chests":{"tab":"BOSSES","observed_at":"2026-11-04T11:59:00Z","items":{"4708":1,"4710":0}}}}`
	var data PushData
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&data); err != nil {
		t.Fatal(err)
	}
	want := CollectionLogMap{"Barrows Chests": {Tab: "BOSSES", ObservedAt: testTime.Add(-time.Minute), Items: QuantityMap{"4708": 1, "4710": 0}}}
	if data.CollectionLog == nil || !reflect.DeepEqual(*data.CollectionLog, want) {
		t.Fatalf("decoded = %+v", data.CollectionLog)
	}
	if encoded, _ := json.Marshal(data); string(encoded) != raw {
		t.Fatalf("round trip = %s", encoded)
	}
	var m map[string]any
	encoded, _ := json.Marshal(manifest)
	if json.Unmarshal(encoded, &m) != nil || m["collection_log"] != true {
		t.Fatalf("manifest wire = %s", encoded)
	}
}

func TestCollectionLogFacts(t *testing.T) {
	if got := CollectionLogFacts(nil); len(got) != 0 {
		t.Fatalf("no pages = %+v", got)
	}
	read := testTime.Add(-time.Hour)
	got := CollectionLogFacts(CollectionLogMap{
		"Barrows Chests": barrowsPage(),
		"All Pets":       {ObservedAt: read, Items: QuantityMap{"12646": 0}},
	})
	want := []PageFact{
		{Fact: Fact{Facet: FacetCollectionLog, Key: "page:All Pets", Value: map[string]any{"items": map[string]int64{"12646": 0}}}, ObservedAt: read},
		{Fact: Fact{Facet: FacetCollectionLog, Key: "page:Barrows Chests", Value: map[string]any{
			"tab": "BOSSES", "items": map[string]int64{"4708": 1, "4710": 0, "4712": 2},
		}}, ObservedAt: testTime.Add(-time.Minute)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts = %+v\nwant %+v", got, want)
	}
	if CollectionLogPageKey("Barrows Chests") != "page:Barrows Chests" {
		t.Fatal("page key")
	}
}

func TestClampTimesBoundsCollectionLogPages(t *testing.T) {
	now := testTime
	ahead, behind := now.Add(time.Minute), now.Add(-time.Minute)
	pages := CollectionLogMap{
		"Barrows Chests": {ObservedAt: ahead, Items: QuantityMap{"4708": 1}},
		"All Pets":       {ObservedAt: behind, Items: QuantityMap{"12646": 0}},
	}
	p := Push{SentAt: ahead, Data: PushData{CollectionLog: &pages}}
	got := p.ClampTimes(now)
	if c := *got.Data.CollectionLog; !c["Barrows Chests"].ObservedAt.Equal(now) || !c["All Pets"].ObservedAt.Equal(behind) {
		t.Fatalf("clamped pages = %+v", c)
	}
	if !pages["Barrows Chests"].ObservedAt.Equal(ahead) {
		t.Fatal("the caller's pages were modified")
	}
}
