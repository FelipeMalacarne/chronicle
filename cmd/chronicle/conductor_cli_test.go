package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

func TestConductorCLI(t *testing.T) {
	var posted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/me":
			_ = json.NewEncoder(w).Encode(client.Me{AuthMode: "dev", Accounts: []client.Account{{ID: "acc_x", RSN: "X"}}})
		case r.URL.Path == "/api/conductor" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"account":"acc_x","state":"blocked","job":"job_00000000000000a1","reason":"o job job_00000000000000a1 terminou failed: run ended: failed"}`))
		case r.URL.Path == "/api/conductor" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			posted = append(posted, strings.TrimSpace(string(body)))
			state := "off"
			if strings.Contains(string(body), "true") {
				state = "idle"
			}
			_, _ = w.Write([]byte(`{"account":"acc_x","state":"` + state + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	out, err := runCLI(t, "conductor", "--url", srv.URL)
	if err != nil || !strings.Contains(out, "acc_x  blocked  job_00000000000000a1  o job") {
		t.Fatalf("status = %q %v", out, err)
	}
	if _, err = runCLI(t, "conductor", "on", "--url", srv.URL); err == nil || !strings.Contains(err.Error(), "--yes") || len(posted) != 0 {
		t.Fatalf("on without --yes = %v, posted %v", err, posted)
	}
	if out, err = runCLI(t, "conductor", "on", "--yes", "--url", srv.URL); err != nil || !strings.Contains(out, "idle") {
		t.Fatalf("on = %q %v", out, err)
	}
	if out, err = runCLI(t, "conductor", "off", "--url", srv.URL); err != nil || !strings.Contains(out, "off") {
		t.Fatalf("off = %q %v", out, err)
	}
	if len(posted) != 2 || posted[0] != `{"on":true}` || posted[1] != `{"on":false}` {
		t.Fatalf("posted = %v", posted)
	}
}
