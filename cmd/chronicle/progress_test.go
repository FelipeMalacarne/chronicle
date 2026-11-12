package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

func TestProgressCheck(t *testing.T) {
	answers := map[string]string{
		"acc_ok":  `{"account":"acc_ok","ok":true,"differences":[]}`,
		"acc_bad": `{"account":"acc_bad","ok":false,"differences":["marks: g1/a1: log gives no mark, step_marks has {Kind:skipped}"]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/progress/check" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		body, ok := answers[r.URL.Query().Get("account")]
		if !ok {
			http.Error(w, `{"error":"account not found"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tests := []struct {
		account, wantOut string
		wantErr          bool
	}{
		{account: "acc_ok", wantOut: "every projection matches"},
		{account: "acc_bad", wantOut: "1 differences", wantErr: true},
		{account: "acc_nope", wantErr: true},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		c, err := client.New(context.Background(), client.Options{URL: srv.URL + "/", Token: "tok"})
		if err != nil {
			t.Fatal(err)
		}
		err = progressCheck(context.Background(), &out, c, tt.account)
		if (err != nil) != tt.wantErr || !strings.Contains(out.String(), tt.wantOut) {
			t.Fatalf("%s: out %q, err %v", tt.account, out.String(), err)
		}
	}
}
