package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/planning"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
)

type fakeHoldings map[string]rules.ItemHoldings

func (f fakeHoldings) ItemHoldings(_ context.Context, accountID string) (rules.ItemHoldings, error) {
	h, ok := f[accountID]
	if !ok {
		return nil, storeingest.ErrAccountNotFound
	}
	return h, nil
}

// readinessGuide builds a route whose local ids ("1", "2", "leg-1", "leg-2")
// are shared by both test guides; only the loadout item differs.
func readinessGuide(id string, itemID int) *content.Guide {
	lo := &content.Loadout{Status: content.LoadoutPartial, Inventory: []content.LoadoutItem{{Name: "tool", ItemID: &itemID}}}
	g := &content.Guide{ID: id, Title: id, Sections: []*content.Section{{ID: "s", Slug: "s", Segments: []*content.Segment{
		{ID: "leg-1", Kind: content.SegmentBankLeg, Steps: []*content.Step{{ID: "1", Text: "bank"}}},
		{ID: "leg-2", Kind: content.SegmentBankLeg, Loadout: lo,
			BankActions: []content.BankAction{{Type: content.BankWithdraw, Position: content.PositionStart, Text: "withdraw the tool"}},
			Steps:       []*content.Step{{ID: "2", Text: "use the tool"}}},
	}}}}
	g.Index()
	return g
}

func newReadinessServer(t *testing.T, devMode bool) *httptest.Server {
	t.Helper()
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	holdings := fakeHoldings{"acc-1": {
		rules.ContainerBank:      {State: coreingest.ItemRestored, ObservedAt: at, Complete: true, Quantities: map[int]int64{1511: 5}},
		rules.ContainerInventory: {State: coreingest.ItemLive, ObservedAt: at, Complete: true, Quantities: map[int]int64{2347: 1}},
		rules.ContainerEquipment: {State: coreingest.ItemLive, ObservedAt: at, Complete: true, Quantities: map[int]int64{}},
	}}
	c := &content.Content{Guides: []*content.Guide{readinessGuide("oziris-main", 2347), readinessGuide("bruhsailer-main", 1511)}}
	server := httptest.NewServer(New(Options{
		Version: "test", Content: c, Holdings: holdings, DevMode: devMode,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	t.Cleanup(server.Close)
	return server
}

func getStatus(t *testing.T, url string, into any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if into != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

func TestReadinessRoutesAreDevOnly(t *testing.T) {
	server := newReadinessServer(t, false)
	if got := getStatus(t, server.URL+"/api/guides/oziris-main/readiness?account=acc-1", nil); got != http.StatusNotFound {
		t.Fatalf("readiness without dev mode = %d", got)
	}
}

func TestReadinessGuideScoped(t *testing.T) {
	server := newReadinessServer(t, true)

	var oz, bruh readinessResponse
	if got := getStatus(t, server.URL+"/api/guides/oziris-main/readiness?account=acc-1&step=1", &oz); got != http.StatusOK {
		t.Fatalf("oziris status = %d", got)
	}
	if got := getStatus(t, server.URL+"/api/guides/bruhsailer-main/readiness?account=acc-1&step=1", &bruh); got != http.StatusOK {
		t.Fatalf("bruhsailer status = %d", got)
	}
	if oz.Current == nil || oz.Current.SegmentID != "leg-1" || oz.Current.Status != planning.NoLoadout {
		t.Fatalf("oziris current = %+v", oz.Current)
	}
	// Same local ids, different routes: each next leg is evaluated against its
	// own loadout. The hammer is carried; the logs are only in a restored bank.
	if oz.Next == nil || oz.Next.GuideID != "oziris-main" || oz.Next.Status != planning.ListedReady {
		t.Fatalf("oziris next = %+v", oz.Next)
	}
	if bruh.Next == nil || bruh.Next.GuideID != "bruhsailer-main" || bruh.Next.Status != planning.NotReady {
		t.Fatalf("bruhsailer next = %+v", bruh.Next)
	}
	owned := bruh.Next.Items[0].Owned
	if owned == nil || owned.Verdict != rules.Met || !owned.Stale {
		t.Fatalf("restored-bank ownership must be stale: %+v", owned)
	}
	if bruh.Next.Freshness[rules.ContainerBank].Live {
		t.Fatal("restored bank reported live")
	}
	if len(bruh.Next.BankActions) != 1 {
		t.Fatalf("bank actions = %+v", bruh.Next.BankActions)
	}

	var seg planning.Segment
	if got := getStatus(t, server.URL+"/api/guides/bruhsailer-main/segments/leg-2/readiness?account=acc-1", &seg); got != http.StatusOK {
		t.Fatalf("segment status = %d", got)
	}
	if seg.GuideID != "bruhsailer-main" || seg.Status != planning.NotReady {
		t.Fatalf("segment = %+v", seg)
	}
}

func TestReadinessErrors(t *testing.T) {
	server := newReadinessServer(t, true)
	cases := []struct {
		path string
		want int
	}{
		{"/api/guides/oziris-main/readiness", http.StatusBadRequest},
		{"/api/guides/oziris-main/readiness?account=nobody", http.StatusNotFound},
		{"/api/guides/nope/readiness?account=acc-1", http.StatusNotFound},
		{"/api/guides/oziris-main/readiness?account=acc-1&step=missing", http.StatusNotFound},
		{"/api/guides/oziris-main/segments/missing/readiness?account=acc-1", http.StatusNotFound},
	}
	for _, tc := range cases {
		if got := getStatus(t, server.URL+tc.path, nil); got != tc.want {
			t.Errorf("GET %s = %d, want %d", tc.path, got, tc.want)
		}
	}
}
