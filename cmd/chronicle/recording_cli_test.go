package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

// recordingPlanner serves a recording in pages of two, like the server cuts
// one at ?limit=.
type recordingPlanner struct {
	events  []recording.Event
	deleted []string
}

func (p *recordingPlanner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/me":
		_ = json.NewEncoder(w).Encode(client.Me{AuthMode: "dev", Accounts: []client.Account{{ID: "acc_x", RSN: "X"}}})
	case r.URL.Path == "/api/account/recording" && r.Method == http.MethodGet:
		since, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("since"))
		var page []recording.Event
		for _, e := range p.events {
			if !e.At.Before(since) {
				page = append(page, e)
			}
		}
		out := client.Recording{Account: r.URL.Query().Get("account"), Events: page}
		if len(page) > 2 {
			out.Events, out.Truncated = page[:2], true
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.URL.Path == "/api/account/recording" && r.Method == http.MethodDelete:
		p.deleted = append(p.deleted, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"deleted":` + strconv.Itoa(len(p.events)) + `}`))
	default:
		http.NotFound(w, r)
	}
}

func TestRecordingExportFollowsPages(t *testing.T) {
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	p := &recordingPlanner{}
	for i := range 5 {
		p.events = append(p.events, recording.Event{Recording: "rec-a", Seq: int64(i + 1), At: at.Add(time.Duration(i) * time.Second), Kind: recording.KindMove, World: 330, X: 3222, Y: 3218})
	}
	srv := httptest.NewServer(p)
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "rec.json")
	out, err := runCLI(t, "recording", "--url", srv.URL, "--since", "2h", "-o", file)
	if err != nil || !strings.Contains(out, "5 eventos de acc_x") {
		t.Fatalf("export = %q %v", out, err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var rec client.Recording
	if err = json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Account != "acc_x" || len(rec.Events) != 5 || rec.Events[4].Seq != 5 || rec.Truncated {
		t.Fatalf("file = %+v", rec)
	}

	out, err = runCLI(t, "recording", "--url", srv.URL, "--max", "3")
	if err != nil || strings.Count(out, "rec-a#") != 3 || !strings.Contains(out, "cortado") {
		t.Fatalf("print = %q %v", out, err)
	}
	if _, err := runCLI(t, "recording", "--url", srv.URL, "--since", "ontem"); err == nil {
		t.Fatal("a bad --since must fail")
	}
}

func TestRecordingDeleteNeedsYes(t *testing.T) {
	p := &recordingPlanner{}
	srv := httptest.NewServer(p)
	defer srv.Close()
	if _, err := runCLI(t, "recording", "delete", "--url", srv.URL); err == nil || len(p.deleted) != 0 {
		t.Fatalf("delete without --yes = %v, %v", err, p.deleted)
	}
	if out, err := runCLI(t, "recording", "delete", "--url", srv.URL, "--recording", "rec-a", "--yes"); err != nil || !strings.Contains(out, "apagados") {
		t.Fatalf("delete = %q %v", out, err)
	}
	if len(p.deleted) != 1 || !strings.Contains(p.deleted[0], "recording=rec-a") || !strings.Contains(p.deleted[0], "account=acc_x") {
		t.Fatalf("asked = %v", p.deleted)
	}
}
