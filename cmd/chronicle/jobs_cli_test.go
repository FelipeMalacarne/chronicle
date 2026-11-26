package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

func TestJobsCLI(t *testing.T) {
	canceled := false
	var posted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const job = `{"id":"job_00000000000000a1","kind":"plugin","state":"running","source":"owner","goal":{"type":"varp","id":281,"min":1000},
			"progress":{"from":10,"current":400,"target":1000,"at":"2026-11-24T12:00:00Z"},"stall_after_s":1800,"created_at":"2026-11-24T11:00:00Z"}`
		switch {
		case r.URL.Path == "/api/me":
			_ = json.NewEncoder(w).Encode(client.Me{AuthMode: "dev", Accounts: []client.Account{{ID: "acc_x", RSN: "X"}}})
		case r.URL.Path == "/api/jobs" && r.Method == http.MethodGet:
			if r.URL.Query().Get("state") == "failed" {
				_, _ = w.Write([]byte(`{"account":"acc_x","jobs":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"account":"acc_x","jobs":[` + job + `]}`))
		case r.URL.Path == "/api/jobs/job_00000000000000a1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"job":` + job + `,"runs":[{"uid":"aa11bb22cc33dd44","className":"a.b.Tutorial","state":"running"}],
				"events":[{"id":1,"type":"job.queued","at":"2026-11-24T11:00:00Z"},{"id":2,"type":"job.started","at":"2026-11-24T11:01:00Z"}]}`))
		case r.URL.Path == "/api/jobs/job_00000000000000a1/cancel" && r.Method == http.MethodPost:
			canceled = true
			_, _ = w.Write([]byte(`{"job":{"id":"job_00000000000000a1","state":"canceled","kind":"plugin","source":"owner","goal":"x","stall_after_s":60,"created_at":"2026-11-24T11:00:00Z"},"run_canceled":false}`))
		case r.URL.Path == "/api/jobs" && r.Method == http.MethodPost:
			posted = append(posted, "create")
			_, _ = w.Write([]byte(`{"id":"job_00000000000000a2","state":"queued","kind":"plugin","source":"owner","goal":"x","stall_after_s":60,"created_at":"2026-11-24T11:00:00Z"}`))
		case r.URL.Path == "/api/jobs/job_00000000000000a1/run" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			posted = append(posted, "run "+string(body))
			_, _ = w.Write([]byte(`{"ok":true,"job":"job_00000000000000a1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	out, err := runCLI(t, "jobs", "--url", srv.URL)
	if err != nil || !strings.Contains(out, "job_00000000000000a1") || !strings.Contains(out, "running") || !strings.Contains(out, "400/1000") {
		t.Fatalf("list = %q %v", out, err)
	}
	if out, err = runCLI(t, "jobs", "--url", srv.URL, "--state", "failed"); err != nil || !strings.Contains(out, "nenhum job") {
		t.Fatalf("empty = %q %v", out, err)
	}
	out, err = runCLI(t, "jobs", "show", "--url", srv.URL, "job_00000000000000a1")
	if err != nil || !strings.Contains(out, "run aa11bb22cc33dd44") || !strings.Contains(out, "job.started") || !strings.Contains(out, `"min":1000`) {
		t.Fatalf("show = %q %v", out, err)
	}
	// Starting a job acts in game: never without --yes.
	if out, err = runCLI(t, "jobs", "run", "--url", srv.URL, "job_00000000000000a1"); err == nil || !strings.Contains(err.Error(), "--yes") || len(posted) != 0 {
		t.Fatalf("run without --yes = %q %v (%v)", out, err, posted)
	}
	if out, err = runCLI(t, "jobs", "run", "--url", srv.URL, "--yes", "--timeout", "1h", "job_00000000000000a1"); err != nil ||
		len(posted) != 1 || posted[0] != `run {"supervised":false,"timeoutS":3600}` || !strings.Contains(out, "jobs show") {
		t.Fatalf("run = %q %v (%q)", out, err, posted)
	}
	file := filepath.Join(t.TempDir(), "job.json")
	if err = os.WriteFile(file, []byte(`{"kind":"plugin"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err = runCLI(t, "jobs", "create", "--url", srv.URL, "-f", file); err != nil || !strings.Contains(out, "job_00000000000000a2: queued") {
		t.Fatalf("create = %q %v", out, err)
	}
	if out, err = runCLI(t, "jobs", "cancel", "--url", srv.URL, "job_00000000000000a1"); err != nil || !canceled || !strings.Contains(out, "canceled") {
		t.Fatalf("cancel = %q %v", out, err)
	}
}
