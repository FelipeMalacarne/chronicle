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

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
	storerecording "github.com/FelipeMalacarne/chronicle/internal/store/recording"
)

// memRecording keeps events per account and records what it was asked.
type memRecording struct {
	by           map[string][]recording.Event
	since, until time.Time
	limit        int
	deleted      string
}

func (m *memRecording) Events(_ context.Context, account string, since, until time.Time, limit int) ([]recording.Event, error) {
	m.since, m.until, m.limit = since, until, limit
	events, ok := m.by[account]
	if !ok {
		return nil, storerecording.ErrNotFound
	}
	var out []recording.Event
	for _, e := range events {
		if !e.At.Before(since) && e.At.Before(until) && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memRecording) Delete(_ context.Context, account, id string) (int64, error) {
	events, ok := m.by[account]
	if !ok {
		return 0, storerecording.ErrNotFound
	}
	m.deleted = account + "/" + id
	kept := events[:0]
	n := 0
	for _, e := range events {
		if id == "" || e.Recording == id {
			n++
			continue
		}
		kept = append(kept, e)
	}
	m.by[account] = kept
	return int64(n), nil
}

func newMemRecording(now time.Time) *memRecording {
	at := func(seq int64, ago time.Duration, rec string) recording.Event {
		return recording.Event{Recording: rec, Seq: seq, At: now.Add(-ago), Kind: recording.KindMove, World: 330, X: 3222, Y: 3218}
	}
	return &memRecording{by: map[string][]recording.Event{
		"acc-1": {at(1, 3*time.Hour, "rec-a"), at(2, 2*time.Hour, "rec-a"), at(1, time.Hour, "rec-b")},
		"acc-2": {at(1, time.Hour, "rec-c")}, // another user's
	}}
}

func TestRecordingRoutes(t *testing.T) {
	now := time.Now().UTC()
	store := newMemRecording(now)
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Recording: store,
	}))
	t.Cleanup(srv.Close)
	base := srv.URL + "/api/account/recording?account="

	for name, query := range map[string]string{
		"since not a time": "&since=yesterday",
		"until not a time": "&until=later",
		"until before":     "&since=" + url.QueryEscape(now.Format(time.RFC3339)) + "&until=" + url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339)),
		"limit zero":       "&limit=0",
		"limit too big":    "&limit=5001",
	} {
		if status, _, _ := call(t, "GET", base+"acc-1"+query, ""); status != http.StatusBadRequest {
			t.Errorf("%s = %d", name, status)
		}
	}
	if status, _, _ := call(t, "GET", srv.URL+"/api/account/recording", ""); status != http.StatusBadRequest {
		t.Fatalf("no account = %d", status)
	}
	if status, _, _ := call(t, "GET", base+"acc-9", ""); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}

	// Without since, the last 24 hours up to now; one more than the limit
	// is asked to tell a cut page.
	status, body, _ := call(t, "GET", base+"acc-1&limit=2", "")
	if status != http.StatusOK || body["account"] != "acc-1" || body["truncated"] != true || len(body["events"].([]any)) != 2 {
		t.Fatalf("recording = %d %v", status, body)
	}
	if store.limit != 3 || store.since.Before(now.Add(-24*time.Hour-time.Minute)) || store.until.Before(now) {
		t.Fatalf("asked since=%v until=%v limit=%d", store.since, store.until, store.limit)
	}
	first := body["events"].([]any)[0].(map[string]any)
	if first["recording"] != "rec-a" || first["seq"] != float64(1) || first["kind"] != "move" {
		t.Fatalf("first event = %v", first)
	}
	since := now.Add(-90 * time.Minute).Format(time.RFC3339Nano)
	status, body, _ = call(t, "GET", base+"acc-1&since="+url.QueryEscape(since), "")
	if status != http.StatusOK || body["truncated"] != false || len(body["events"].([]any)) != 1 {
		t.Fatalf("since = %d %v", status, body)
	}

	// Delete on request: one recording, then the rest.
	if status, _, _ = call(t, "DELETE", base+"acc-1&recording=Not%20an%20id", ""); status != http.StatusBadRequest {
		t.Fatalf("bad recording id = %d", status)
	}
	status, body, _ = call(t, "DELETE", base+"acc-1&recording=rec-b", "")
	if status != http.StatusOK || body["deleted"] != float64(1) || store.deleted != "acc-1/rec-b" {
		t.Fatalf("delete one = %d %v", status, body)
	}
	status, body, _ = call(t, "DELETE", base+"acc-1", "")
	if status != http.StatusOK || body["deleted"] != float64(2) || len(store.by["acc-1"]) != 0 {
		t.Fatalf("delete all = %d %v", status, body)
	}
	if status, _, _ := call(t, "DELETE", base+"acc-9", ""); status != http.StatusNotFound {
		t.Fatalf("delete unknown = %d", status)
	}
}

// In oidc the routes need a session, only reach the session user's
// accounts, and the delete needs the CSRF header (rule 10).
func TestRecordingRoutesOIDC(t *testing.T) {
	store := newMemRecording(time.Now().UTC())
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) { o.Recording = store })
	c := e.client()
	if status, _ := e.do(c, "GET", "/api/account/recording?account=acc-1", "", false); status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", status)
	}
	e.login(c, "/", owner)
	if status, _ := e.do(c, "GET", "/api/account/recording?account=acc-2", "", false); status != http.StatusNotFound {
		t.Fatalf("another user's account = %d", status)
	}
	if status, body := e.do(c, "GET", "/api/account/recording?account=acc-1", "", false); status != http.StatusOK || len(body["events"].([]any)) != 3 {
		t.Fatalf("own account = %d %v", status, body)
	}
	if status, _ := e.do(c, "DELETE", "/api/account/recording?account=acc-1", "", false); status != http.StatusForbidden {
		t.Fatalf("delete without CSRF = %d", status)
	}
	if status, body := e.do(c, "DELETE", "/api/account/recording?account=acc-1", "", true); status != http.StatusOK || body["deleted"] != float64(3) {
		t.Fatalf("delete = %d %v", status, body)
	}
}

func TestRecordingRoutesAreDevOnly(t *testing.T) {
	srv := httptest.NewServer(New(Options{Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Recording: newMemRecording(time.Now())}))
	t.Cleanup(srv.Close)
	for _, method := range []string{"GET", "DELETE"} {
		if status, _, _ := call(t, method, srv.URL+"/api/account/recording?account=acc-1", ""); status != http.StatusNotFound {
			t.Fatalf("%s outside dev mode and oidc = %d", method, status)
		}
	}
}
