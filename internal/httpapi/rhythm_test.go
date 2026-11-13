package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	corerhythm "github.com/FelipeMalacarne/chronicle/internal/core/rhythm"
	storerhythm "github.com/FelipeMalacarne/chronicle/internal/store/rhythm"
)

// memRhythm answers a fixed rhythm per account and records the since it got.
type memRhythm struct {
	by    map[string]corerhythm.Rhythm
	since []time.Time
}

func (m *memRhythm) Rhythm(_ context.Context, account string, _, since time.Time) (corerhythm.Rhythm, error) {
	m.since = append(m.since, since)
	r, ok := m.by[account]
	if !ok {
		return r, storerhythm.ErrNotFound
	}
	return r, nil
}

func TestRhythmRoutes(t *testing.T) {
	gp := func(v int64, today *int64) *corerhythm.GP { return &corerhythm.GP{Value: v, Today: today} }
	plus := int64(22_000)
	store := &memRhythm{by: map[string]corerhythm.Rhythm{
		"acc-1": {Today: corerhythm.Today{XP: 26_700, Levels: 1}, GP: gp(1_900_000, &plus)},
		"acc-3": {Today: corerhythm.Today{XP: 0}, GP: gp(100_000, nil)},
		"acc-2": {Today: corerhythm.Today{XP: 999_999}}, // another user's: never in the fleet
	}}
	ident := newMemIdentity()
	ident.owners["acc-3"] = "dev"
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Identity: ident, Rhythm: store,
	}))
	t.Cleanup(srv.Close)

	now := time.Now().UTC()
	for name, since := range map[string]string{
		"not a time":    "yesterday",
		"in the future": now.Add(time.Hour).Format(time.RFC3339),
		"too old":       now.Add(-49 * time.Hour).Format(time.RFC3339),
	} {
		if status, _, _ := call(t, "GET", srv.URL+"/api/account/rhythm?account=acc-1&since="+url.QueryEscape(since), ""); status != http.StatusBadRequest {
			t.Errorf("%s = %d", name, status)
		}
	}
	if status, _, _ := call(t, "GET", srv.URL+"/api/account/rhythm?account=acc-9", ""); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}
	status, body, _ := call(t, "GET", srv.URL+"/api/account/rhythm?account=acc-1", "")
	if status != http.StatusOK || body["account"] != "acc-1" || body["today"].(map[string]any)["xp"] != float64(26_700) {
		t.Fatalf("rhythm = %d %v", status, body)
	}
	// Without since, the day starts at midnight UTC.
	y, m, d := now.Date()
	if got := store.since[len(store.since)-1]; !got.Equal(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("default since = %v", got)
	}

	midnight := now.Add(-5 * time.Hour).Truncate(time.Second)
	status, body, _ = call(t, "GET", srv.URL+"/api/rhythm?since="+url.QueryEscape(midnight.Format(time.RFC3339)), "")
	if status != http.StatusOK || len(body["accounts"].([]any)) != 2 {
		t.Fatalf("fleet = %d %v", status, body)
	}
	totals := body["totals"].(map[string]any)
	if totals["xp_today"] != float64(26_700) || totals["levels_today"] != float64(1) || totals["gaining"] != float64(1) ||
		totals["gp"] != float64(2_000_000) || totals["gp_today"] != float64(22_000) {
		t.Fatalf("totals = %v", totals)
	}
	if got := store.since[len(store.since)-1]; !got.Equal(midnight) {
		t.Fatalf("fleet since = %v", got)
	}
}
