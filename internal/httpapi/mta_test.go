package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/mta"
)

func newMTAServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMTARewards(t *testing.T) {
	srv := newMTAServer(t)
	resp, err := http.Get(srv.URL + "/api/mta/rewards")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var got mta.Table
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Rewards) != 12 || len(got.Rooms) != 4 {
		t.Errorf("rewards %d rooms %d", len(got.Rewards), len(got.Rooms))
	}
}

func TestMTACalculate(t *testing.T) {
	srv := newMTAServer(t)
	post := func(body string) (int, string) {
		resp, err := http.Post(srv.URL+"/api/mta/calculate", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	code, body := post(`{"rewards":["bones_to_peaches"],"magic":99}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var res mta.Result
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if want := (mta.Runes{Law: 350, Nature: 1060, Cosmic: 304}); res.Runes != want || !slices.Equal(res.Buy, []string{"bones_to_peaches"}) {
		t.Errorf("got %+v", res)
	}

	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"unknown reward":      {`{"rewards":["dragon_scimitar"],"magic":99}`, http.StatusBadRequest},
		"negative points":     {`{"rewards":["infinity_boots"],"points":{"telekinetic":-1},"magic":99}`, http.StatusBadRequest},
		"unknown field":       {`{"rewards":[],"wat":1}`, http.StatusBadRequest},
		"not json":            {`rewards`, http.StatusBadRequest},
		"no Enchant castable": {`{"rewards":["infinity_boots"],"magic":5}`, http.StatusUnprocessableEntity},
	} {
		if code, body := post(tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d: %s", name, code, tc.want, body)
		}
	}
}
