package wikiprices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

var fetchedAt = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNormalizeFixtures(t *testing.T) {
	got, err := Normalize(read(t, "latest.json"), read(t, "mapping.json"), fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int]int{}
	for i, p := range got {
		byID[p.ItemID] = i
		if p.Source != "wiki" || !p.FetchedAt.Equal(fetchedAt) {
			t.Errorf("%d: source/fetched_at %q %v", p.ItemID, p.Source, p.FetchedAt)
		}
	}
	if len(got) != 7 {
		t.Fatalf("got %d prices, want 7", len(got))
	}
	// Ordered by id.
	for i := 1; i < len(got); i++ {
		if got[i-1].ItemID >= got[i].ItemID {
			t.Fatalf("not ordered: %d then %d", got[i-1].ItemID, got[i].ItemID)
		}
	}
	p := got[byID[2]]
	if p.High != 243 || p.Low != 238 || p.HighAt.Unix() != 1795560401 || p.LowAt.Unix() != 1795560402 {
		t.Errorf("cannonball: %+v", p)
	}
	if p.Limit == nil || *p.Limit != 11000 || p.Members == nil || !*p.Members {
		t.Errorf("cannonball limit/members: %+v", p)
	}
	// Null high: only the low side.
	if p := got[byID[22636]]; p.High != 0 || !p.HighAt.IsZero() || p.Low != 20001 || p.LowAt.IsZero() {
		t.Errorf("no high side: %+v", p)
	}
	// Null low, and absent from /mapping: limit and members stay nil.
	if p := got[byID[2660]]; p.Low != 0 || !p.LowAt.IsZero() || p.High != 50000 || p.Limit != nil || p.Members != nil {
		t.Errorf("no low side, unmapped: %+v", p)
	}
	// Above int32.
	if p := got[byID[20014]]; p.High != 7949999997 || p.Low != 7800090000 {
		t.Errorf("big price: %+v", p)
	}
	// In /mapping without a limit.
	if p := got[byID[28226]]; p.Limit != nil || p.Members == nil {
		t.Errorf("mapped without limit: %+v", p)
	}
}

func TestNormalizeDropsNonsense(t *testing.T) {
	latest := `{"data":{
		"0":{"high":5,"highTime":1,"low":5,"lowTime":1},
		"-3":{"high":5,"highTime":1,"low":5,"lowTime":1},
		"abc":{"high":5,"highTime":1,"low":5,"lowTime":1},
		"10":{"high":null,"highTime":null,"low":null,"lowTime":null},
		"11":{"high":0,"highTime":1,"low":-4,"lowTime":1},
		"12":{"high":7,"highTime":null,"low":null,"lowTime":null},
		"13":{"high":-1,"highTime":1,"low":9,"lowTime":2},
		"14":{"high":3,"highTime":1,"low":4,"lowTime":2}}}`
	got, err := Normalize([]byte(latest), []byte(`[{"id":14,"limit":0,"members":true}]`), fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ItemID != 13 || got[1].ItemID != 14 {
		t.Fatalf("got %+v", got)
	}
	if got[0].High != 0 || got[0].Low != 9 {
		t.Errorf("item 13: %+v", got[0])
	}
	if got[1].Limit != nil || got[1].Members == nil || !*got[1].Members {
		t.Errorf("item 14: %+v", got[1])
	}
}

func TestNormalizeMalformed(t *testing.T) {
	for name, tc := range map[string]struct{ latest, mapping string }{
		"latest not json":  {`<html>`, `[]`},
		"latest no data":   {`{}`, `[]`},
		"mapping not json": {`{"data":{}}`, `{"x":1}`},
		"price not number": {`{"data":{"2":{"high":"a"}}}`, `[]`},
	} {
		if _, err := Normalize([]byte(tc.latest), []byte(tc.mapping), fetchedAt); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestFetch(t *testing.T) {
	latest, mapping := read(t, "latest.json"), read(t, "mapping.json")
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/latest":
			_, _ = w.Write(latest)
		case "/mapping":
			_, _ = w.Write(mapping)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL
	got, err := c.Fetch(context.Background())
	if err != nil || len(got) != 7 {
		t.Fatalf("%d prices, %v", len(got), err)
	}
	if hits["/latest"] != 1 || hits["/mapping"] != 1 {
		t.Errorf("hits %v", hits)
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusTooManyRequests)
	}))
	defer down.Close()
	c.BaseURL = down.URL
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Error("429 gave no error")
	}
}
