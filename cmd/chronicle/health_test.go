package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckHealth(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"ok with db", http.StatusOK, `{"status":"ok","db":"ok"}`, false},
		{"ok without db", http.StatusOK, `{"status":"ok","db":"not_configured"}`, false},
		{"db down", http.StatusOK, `{"status":"ok","db":"error"}`, true},
		{"server error", http.StatusInternalServerError, `{"status":"error"}`, true},
		{"not json", http.StatusOK, `<html>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			if err := checkHealth(context.Background(), srv.URL, time.Second); (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
		})
	}
	if err := checkHealth(context.Background(), "http://127.0.0.1:1/api/health", 200*time.Millisecond); err == nil {
		t.Fatal("no server, no error")
	}
}
