package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	coreactivity "github.com/FelipeMalacarne/chronicle/internal/core/activity"
	storeactivity "github.com/FelipeMalacarne/chronicle/internal/store/activity"
)

// memLevels answers fixed level-ups per account and records the since it got.
type memLevels struct {
	by    map[string]coreactivity.Levels
	since []time.Time
}

func (m *memLevels) Levels(_ context.Context, account string, since time.Time) (coreactivity.Levels, error) {
	m.since = append(m.since, since)
	lv, ok := m.by[account]
	if !ok {
		return lv, storeactivity.ErrNotFound
	}
	return lv, nil
}

func newMemLevels(now time.Time) *memLevels {
	up := func(ago time.Duration, skill string, level int) coreactivity.LevelUp {
		return coreactivity.LevelUp{Skill: skill, Level: level, At: now.Add(-ago)}
	}
	return &memLevels{by: map[string]coreactivity.Levels{
		"acc-1": {
			Events: []coreactivity.LevelUp{up(time.Hour, "fishing", 76), up(3*time.Hour, "farming", 39), up(3*time.Hour, "farming", 38)},
			Skills: []coreactivity.SkillGain{{Skill: "fishing", FromLevel: 75, ToLevel: 76, XP: 90_000}, {Skill: "farming", FromLevel: 37, ToLevel: 39, XP: 25_000}},
		},
		"acc-3": {Events: []coreactivity.LevelUp{up(2*time.Hour, "woodcutting", 68), up(3*time.Hour, "hunter", 34)}, Skills: []coreactivity.SkillGain{}},
		"acc-2": {Events: []coreactivity.LevelUp{up(time.Minute, "attack", 99)}}, // another user's: never in the fleet
	}}
}

func TestLevelRoutes(t *testing.T) {
	now := time.Now().UTC()
	store := newMemLevels(now)
	ident := newMemIdentity()
	ident.owners["acc-3"] = "dev"
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Identity: ident, Levels: store,
	}))
	t.Cleanup(srv.Close)

	for name, query := range map[string]string{
		"not a time":    "since=yesterday",
		"in the future": "since=" + url.QueryEscape(now.Add(time.Hour).Format(time.RFC3339)),
		"too old":       "since=" + url.QueryEscape(now.Add(-8*24*time.Hour).Format(time.RFC3339)),
		"limit zero":    "limit=0",
		"limit too big": "limit=501",
		"limit text":    "limit=x",
	} {
		if status, _, _ := call(t, "GET", srv.URL+"/api/account/levels?account=acc-1&"+query, ""); status != http.StatusBadRequest {
			t.Errorf("account %s = %d", name, status)
		}
		if status, _, _ := call(t, "GET", srv.URL+"/api/levels?"+query, ""); status != http.StatusBadRequest {
			t.Errorf("fleet %s = %d", name, status)
		}
	}
	if status, _, _ := call(t, "GET", srv.URL+"/api/account/levels?account=acc-9", ""); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}
	if status, _, _ := call(t, "GET", srv.URL+"/api/account/levels", ""); status != http.StatusBadRequest {
		t.Fatalf("no account = %d", status)
	}

	// Without since, the last 24 hours; limit cuts the events, not the total.
	status, body, _ := call(t, "GET", srv.URL+"/api/account/levels?account=acc-1&limit=2", "")
	if status != http.StatusOK || body["account"] != "acc-1" || body["total"] != float64(3) {
		t.Fatalf("levels = %d %v", status, body)
	}
	events := body["events"].([]any)
	first := events[0].(map[string]any)
	if len(events) != 2 || first["account"] != "acc-1" || first["skill"] != "fishing" || first["level"] != float64(76) {
		t.Fatalf("events = %v", events)
	}
	if skills := body["skills"].([]any); len(skills) != 2 || skills[0].(map[string]any)["from_level"] != float64(75) {
		t.Fatalf("skills = %v", skills)
	}
	if got := store.since[len(store.since)-1]; got.Before(now.Add(-24*time.Hour-time.Minute)) || got.After(time.Now().Add(-24*time.Hour)) {
		t.Fatalf("default since = %v", got)
	}

	// The fleet: the dev user's accounts merged newest first, by account
	// within an instant.
	since := now.Add(-7 * 24 * time.Hour).Add(time.Minute).Truncate(time.Second)
	status, body, _ = call(t, "GET", srv.URL+"/api/levels?limit=4&since="+url.QueryEscape(since.Format(time.RFC3339)), "")
	if status != http.StatusOK || body["total"] != float64(5) {
		t.Fatalf("fleet = %d %v", status, body)
	}
	var got []string
	for _, e := range body["events"].([]any) {
		ev := e.(map[string]any)
		got = append(got, ev["account"].(string)+":"+ev["skill"].(string))
	}
	if want := []string{"acc-1:fishing", "acc-3:woodcutting", "acc-1:farming", "acc-1:farming"}; !slices.Equal(got, want) {
		t.Fatalf("fleet events = %v, want %v", got, want)
	}
	if got := store.since[len(store.since)-1]; !got.Equal(since) {
		t.Fatalf("fleet since = %v", got)
	}
}

// In oidc both routes need a session, and the fleet is the session user's.
func TestLevelRoutesOIDC(t *testing.T) {
	store := newMemLevels(time.Now().UTC())
	ident := newMemIdentity()
	ident.owners["acc-3"] = "dev"
	e := newOIDCEnv(t, ident, memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) { o.Levels = store })
	c := e.client()
	for _, path := range []string{"/api/account/levels?account=acc-1", "/api/levels"} {
		if status, _ := e.do(c, "GET", path, "", false); status != http.StatusUnauthorized {
			t.Fatalf("anonymous %s = %d", path, status)
		}
	}
	e.login(c, "/", owner) // the first OIDC user claims the dev user's accounts
	if status, _ := e.do(c, "GET", "/api/account/levels?account=acc-2", "", false); status != http.StatusNotFound {
		t.Fatalf("another user's account = %d", status)
	}
	status, body := e.do(c, "GET", "/api/levels", "", false)
	if status != http.StatusOK || body["total"] != float64(5) {
		t.Fatalf("fleet = %d %v", status, body)
	}
	for _, ev := range body["events"].([]any) {
		if acc := ev.(map[string]any)["account"]; acc != "acc-1" && acc != "acc-3" {
			t.Fatalf("fleet has %v", acc)
		}
	}
}
