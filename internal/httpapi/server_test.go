package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func newTestServer(t *testing.T, db DBPinger) *httptest.Server {
	t.Helper()
	c, err := content.Load("../../content")
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	static := fstest.MapFS{
		"index.html":   {Data: []byte("<html>spa</html>")},
		"assets/a.css": {Data: []byte("body{}")},
	}
	srv := httptest.NewServer(New(Options{Version: "test", Content: c, DB: db, Static: static}))
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, url string, into any) *http.Response {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // test URL
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%s: content-type = %q, body %s", url, ct, body)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("%s: decode %s: %v", url, body, err)
	}
	return resp
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t, nil)
	var h struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Content struct {
			Guides int `json:"guides"`
			Steps  int `json:"steps"`
		} `json:"content"`
		DB string `json:"db"`
	}
	resp := getJSON(t, srv.URL+"/api/health", &h)
	if resp.StatusCode != 200 || h.Status != "ok" || h.Version != "test" {
		t.Errorf("health = %d %+v", resp.StatusCode, h)
	}
	if h.Content.Guides != 1 || h.Content.Steps != 576 {
		t.Errorf("content counts = %+v", h.Content)
	}
	if h.DB != DBNotConfigured {
		t.Errorf("db = %q", h.DB)
	}
}

func TestHealthDBStatus(t *testing.T) {
	for _, tc := range []struct {
		db   DBPinger
		want string
	}{
		{fakeDB{}, DBOK},
		{fakeDB{err: errors.New("boom")}, DBError},
	} {
		srv := newTestServer(t, tc.db)
		var h struct {
			DB string `json:"db"`
		}
		getJSON(t, srv.URL+"/api/health", &h)
		if h.DB != tc.want {
			t.Errorf("db = %q want %q", h.DB, tc.want)
		}
	}
}

func TestNow(t *testing.T) {
	srv := newTestServer(t, nil)
	var now struct {
		Current struct {
			ID      string `json:"id"`
			Section string `json:"section"`
			Manual  bool   `json:"manual"`
		} `json:"current"`
		Next []struct {
			ID string `json:"id"`
		} `json:"next"`
		Progress struct {
			Done  int `json:"done"`
			Total int `json:"total"`
		} `json:"progress"`
	}
	getJSON(t, srv.URL+"/api/now", &now)
	if now.Current.ID == "" || now.Current.Section != "1.1" {
		t.Errorf("current = %+v", now.Current)
	}
	if len(now.Next) != 5 {
		t.Errorf("next = %d steps, want 5", len(now.Next))
	}
	// Total excludes the Sailing track (12 steps).
	if now.Progress.Done != 0 || now.Progress.Total != 576-12 {
		t.Errorf("progress = %+v", now.Progress)
	}
}

func TestGuideSectionStep(t *testing.T) {
	srv := newTestServer(t, nil)
	var guide struct {
		Guides []struct {
			ID       string `json:"id"`
			Sections []struct {
				ID    string `json:"id"`
				Slug  string `json:"slug"`
				Steps int    `json:"steps"`
				Track bool   `json:"track"`
			} `json:"sections"`
		} `json:"guides"`
	}
	getJSON(t, srv.URL+"/api/guide", &guide)
	if len(guide.Guides) != 1 || len(guide.Guides[0].Sections) != 7 {
		t.Fatalf("guide = %+v", guide)
	}
	if s := guide.Guides[0].Sections[6]; s.ID != "S" || !s.Track || s.Steps != 12 {
		t.Errorf("sailing summary = %+v", s)
	}

	var sec struct {
		ID    string           `json:"id"`
		Guide string           `json:"guide"`
		Steps []map[string]any `json:"steps"`
	}
	getJSON(t, srv.URL+"/api/guide/1.1", &sec)
	if sec.ID != "1.1" || sec.Guide != "oziris-main" || len(sec.Steps) != 286 {
		t.Errorf("section 1.1 = %s %s %d steps", sec.ID, sec.Guide, len(sec.Steps))
	}
	getJSON(t, srv.URL+"/api/guide/sailing", &sec)
	if sec.ID != "S" {
		t.Errorf("section by slug = %q", sec.ID)
	}

	var step struct {
		ID           string           `json:"id"`
		Section      string           `json:"section"`
		CompleteWhen []map[string]any `json:"complete_when"`
	}
	getJSON(t, srv.URL+"/api/steps/1.1.258a", &step)
	if step.ID != "1.1.258a" || step.Section != "1.1" || len(step.CompleteWhen) != 2 {
		t.Errorf("step = %+v", step)
	}

	var e struct {
		Error string `json:"error"`
	}
	if resp := getJSON(t, srv.URL+"/api/steps/nope", &e); resp.StatusCode != 404 || e.Error == "" {
		t.Errorf("missing step = %d %+v", resp.StatusCode, e)
	}
	if resp := getJSON(t, srv.URL+"/api/whatever", &e); resp.StatusCode != 404 {
		t.Errorf("unknown api route = %d", resp.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	srv := newTestServer(t, nil)
	for _, tc := range []struct {
		path, want, ct string
	}{
		{"/", "<html>spa</html>", "text/html"},
		{"/account/123", "<html>spa</html>", "text/html"},
		{"/assets/a.css", "body{}", "text/css"},
	} {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(body) != tc.want || !strings.HasPrefix(resp.Header.Get("Content-Type"), tc.ct) {
			t.Errorf("%s: %d %q %q", tc.path, resp.StatusCode, body, resp.Header.Get("Content-Type"))
		}
	}
}
