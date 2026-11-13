package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
)

// fakePlanner is a dev-mode API with one account and a two-step window; it
// records the marks it receives.
type fakePlanner struct {
	mu    sync.Mutex
	marks []string
}

func (f *fakePlanner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/me":
		_ = json.NewEncoder(w).Encode(client.Me{AuthMode: "dev", Accounts: []client.Account{{ID: "acc_x", RSN: "X"}}})
	case r.URL.Path == "/api/route" && r.URL.Query().Get("account") == "acc_x":
		_ = json.NewEncoder(w).Encode(panel.Window{
			Guide: "bruhsailer-main", Title: "BRUHsailer", Attribution: "Guia por So Iron BRUH e ParasailerOSRS",
			Cursor: &panel.Cursor{StepID: "bruh-1.2-a"},
			Steps: []panel.Step{
				{ID: "bruh-1.2-a", Ref: "CH1.31", Title: "Barbarian fishing", Current: true, Tasks: []panel.Task{
					{Type: "travel", Label: "Ir até Otto's Grotto", Until: "na área do local", State: "met"},
					{Type: "talk", Label: "Falar com Otto", Until: "objetivos do passo", State: "unmet"},
				}},
				{ID: "bruh-1.2-b", Ref: "CH1.32", Title: "Next"},
			},
		})
	case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/mark"):
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.marks = append(f.marks, r.URL.Path+"?"+r.URL.RawQuery+" "+string(body))
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	default:
		http.NotFound(w, r)
	}
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CHRONICLE_CONFIG_DIR", t.TempDir())
	t.Setenv("CHRONICLE_TOKEN", "")
	t.Setenv("CHRONICLE_ACCOUNT", "")
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestDoneMarksCurrentOrNamedStep(t *testing.T) {
	f := &fakePlanner{}
	srv := httptest.NewServer(f)
	defer srv.Close()

	out, err := runCLI(t, "done", "--url", srv.URL)
	if err != nil || !strings.Contains(out, "Feito: CH1.31") {
		t.Fatalf("done = %q %v", out, err)
	}
	if _, err := runCLI(t, "skip", "ch1.32", "--url", srv.URL, "--note", "depois"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "undo", "bruh-9.9-z", "--url", srv.URL); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`/api/guides/bruhsailer-main/steps/bruh-1.2-a/mark?account=acc_x {"kind":"manual_done"}`,
		`/api/guides/bruhsailer-main/steps/bruh-1.2-b/mark?account=acc_x {"kind":"skipped","note":"depois"}`,
		`/api/guides/bruhsailer-main/steps/bruh-9.9-z/mark?account=acc_x {"kind":"clear"}`,
	}
	if strings.Join(f.marks, "\n") != strings.Join(want, "\n") {
		t.Fatalf("marks:\n%s\nwant:\n%s", strings.Join(f.marks, "\n"), strings.Join(want, "\n"))
	}
}

func TestNowShowsAttribution(t *testing.T) {
	srv := httptest.NewServer(&fakePlanner{})
	defer srv.Close()
	out, err := runCLI(t, "now", "--url", srv.URL)
	if err != nil || !strings.Contains(out, "▶ CH1.31") || !strings.Contains(out, "CH1.32") || !strings.Contains(out, "So Iron BRUH e ParasailerOSRS") {
		t.Fatalf("now = %q %v", out, err)
	}
	// The assisted mode's checklist of the current step.
	if !strings.Contains(out, "✓ Ir até Otto's Grotto (até: na área do local)") || !strings.Contains(out, "○ Falar com Otto") {
		t.Fatalf("now without the tasks = %q", out)
	}
}

func TestPickGoal(t *testing.T) {
	goals := []client.Goal{
		{ID: "g1", Label: "Barrows gloves"},
		{ID: "g2", Label: "Fire cape"},
		{ID: "g3", Name: "Dragon Slayer II"},
		{ID: "g4", Name: "Dragon Slayer I"},
	}
	for want, id := range map[string]string{"g2": "g2", "barrows": "g1", "dragon slayer ii": "g3"} {
		if g, err := pickGoal(goals, want); err != nil || g.ID != id {
			t.Errorf("pickGoal(%q) = %v %v, want %s", want, g.ID, err, id)
		}
	}
	if _, err := pickGoal(goals, "dragon"); err == nil {
		t.Error("ambiguous name picked a goal")
	}
	if _, err := pickGoal(goals, "quest cape"); err == nil {
		t.Error("unknown name picked a goal")
	}
}

func TestExplainLoginRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"login required"}`))
	}))
	defer srv.Close()
	if _, err := runCLI(t, "now", "--url", srv.URL); err == nil || !strings.Contains(err.Error(), "chronicle login") {
		t.Fatalf("now without login = %v", err)
	}
}

func TestAccountStateParts(t *testing.T) {
	var st client.AccountState
	if err := json.Unmarshal([]byte(`{"account":{"id":"acc_x"},"skills":[{"skill":"agility","level":45}],
		"quests":[{"name":"Waterfall Quest","state":"FINISHED"},{"name":"Dragon Slayer I","state":"NOT_STARTED"}],
		"diaries":[{"area":"ardougne","tier":"easy","complete":true},{"area":"ardougne","tier":"medium"}],
		"containers":[{"container":"bank","state":"live","items":[{"id":1,"name":"Rune pickaxe","qty":1},{"id":2,"name":"Coins","qty":5}]}]}`), &st); err != nil {
		t.Fatal(err)
	}
	sum, err := accountStatePart(st, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(sum)
	for _, want := range []string{`"agility":45`, `"FINISHED":1`, `"diaries_complete":["ardougne easy"]`, `"items":2`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("summary %s lacks %s", b, want)
		}
	}
	items, _ := accountStatePart(st, "items", "PICKAXE")
	if b, _ := json.Marshal(items); string(b) != `[{"container":"bank","name":"Rune pickaxe","qty":1}]` {
		t.Errorf("items = %s", b)
	}
	quests, _ := accountStatePart(st, "quests", "not_started")
	if b, _ := json.Marshal(quests); string(b) != `[{"name":"Dragon Slayer I","state":"NOT_STARTED"}]` {
		t.Errorf("quests = %s", b)
	}
	if _, err := accountStatePart(st, "bank", ""); err == nil {
		t.Error("unknown part accepted")
	}
}
