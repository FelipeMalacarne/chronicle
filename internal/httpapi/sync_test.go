package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/syncrun"
)

type fakeSync struct{ calls int }

func (f *fakeSync) Run(_ context.Context, accountID, trigger string) (syncrun.RunView, error) {
	f.calls++
	switch accountID {
	case "acc-1":
		if trigger != syncrun.TriggerManual {
			return syncrun.RunView{}, nil
		}
		if f.calls > 1 {
			return syncrun.RunView{}, &syncrun.TooSoonError{RetryAt: time.Now().Add(time.Minute), Last: syncrun.RunView{ID: "sync_1", Status: "ok"}}
		}
		return syncrun.RunView{ID: "sync_1", Status: "ok", StepsChanged: 3}, nil
	case "no-name":
		return syncrun.RunView{}, syncrun.ErrNoName
	}
	return syncrun.RunView{}, syncrun.ErrAccountNotFound
}

func (f *fakeSync) Runs(_ context.Context, accountID string, limit int) ([]syncrun.RunView, error) {
	if accountID != "acc-1" {
		return nil, syncrun.ErrAccountNotFound
	}
	return make([]syncrun.RunView, limit), nil
}

func TestSyncRoutes(t *testing.T) {
	newServer := func(dev bool) *httptest.Server {
		srv := httptest.NewServer(New(Options{
			Version: "test", Content: &content.Content{}, Sync: &fakeSync{}, DevMode: dev,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	if status, _ := do(t, "POST", newServer(false).URL+"/api/sync?account=acc-1", ""); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("sync outside dev mode = %d", status)
	}
	base := newServer(true).URL + "/api"
	if status, body := do(t, "POST", base+"/sync?account=acc-1", ""); status != http.StatusOK || body["steps_changed"] != float64(3) {
		t.Fatalf("sync = %d %v", status, body)
	}
	status, body := do(t, "POST", base+"/sync?account=acc-1", "")
	if status != http.StatusTooManyRequests || body["retry_at"] == nil || body["last"] == nil {
		t.Fatalf("debounced sync = %d %v", status, body)
	}
	if status, _ = do(t, "POST", base+"/sync?account=no-name", ""); status != http.StatusConflict {
		t.Fatalf("account without a name = %d", status)
	}
	if status, _ = do(t, "POST", base+"/sync?account=nobody", ""); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}
	if status, body = do(t, "GET", base+"/sync/runs?account=acc-1&limit=2", ""); status != http.StatusOK || len(body["runs"].([]any)) != 2 {
		t.Fatalf("runs = %d %v", status, body)
	}
}
