package httpapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/auth/authtest"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func TestFleetGoals(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cnt := &content.Content{Guides: []*content.Guide{crossGuide("oziris-main", "1.1.40", "", 3), crossGuide("bruhsailer-main", "bruh-1.1-x", "CH1.6", 2)}}
	fake := &fakeProgress{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, Catalog: catalog, Progress: fake, Identity: newMemIdentity(), DevMode: true, Logger: quiet,
	}))
	t.Cleanup(server.Close)
	base := server.URL + "/api/fleet/goals"

	// The dev user's accounts only (acc-2 is someone else's), with the
	// names and routes of each objective.
	status, body := do(t, "GET", base, "")
	goals, _ := body["goals"].([]any)
	accounts, _ := body["accounts"].([]any)
	if status != http.StatusOK || len(goals) != 1 || len(accounts) != 1 || fmt.Sprint(fake.fleet) != "[acc-1]" {
		t.Fatalf("fleet goals = %d %v (store got %v)", status, body, fake.fleet)
	}
	g := goals[0].(map[string]any)
	if g["name"] != "Monk's Friend" || g["ref"] != "quest:monks_friend" || g["kind"] != "quest" || len(g["routes"].([]any)) != 2 || len(g["accounts"].([]any)) != 1 {
		t.Fatalf("goal = %v", g)
	}

	// keys= is repeatable and comma-separated.
	q := url.Values{"keys": {"skill:agility>=10, quest:dragon_slayer_i@finished", "item:1755x1"}}
	if status, _ = do(t, "GET", base+"?"+q.Encode(), ""); status != http.StatusOK || fake.step != "skill:agility>=10 quest:dragon_slayer_i@finished item:1755x1" {
		t.Fatalf("keys = %d, store got %q", status, fake.step)
	}
	if status, _ = do(t, "GET", base+"?keys=nope", ""); status != http.StatusBadRequest {
		t.Fatalf("invalid key = %d", status)
	}
	many := strings.TrimSuffix(strings.Repeat("skill:agility>=10,", maxFleetKeys+1), ",")
	if status, _ = do(t, "GET", base+"?"+url.Values{"keys": {many}}.Encode(), ""); status != http.StatusBadRequest {
		t.Fatalf("too many keys = %d", status)
	}
}

// Without Identity there is nobody whose fleet to read, and outside dev and
// oidc no account route exists (CLAUDE.md rule 10).
func TestFleetGoalsNeedsIdentityAndMode(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	for name, opts := range map[string]Options{
		"no identity": {DevMode: true},
		"no mode":     {Identity: newMemIdentity()},
	} {
		opts.Version, opts.Content, opts.Progress, opts.Logger = "test", &content.Content{}, &fakeProgress{}, quiet
		server := httptest.NewServer(New(opts))
		if status, _ := do(t, "GET", server.URL+"/api/fleet/goals", ""); status != http.StatusNotFound {
			t.Errorf("%s: fleet goals = %d", name, status)
		}
		server.Close()
	}
}

// In oidc the fleet is the session user's: never another user's account.
func TestFleetGoalsOIDC(t *testing.T) {
	fake := &fakeProgress{}
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) { o.Progress = fake })
	c := e.client()
	if status, _ := e.do(c, "GET", "/api/fleet/goals", "", false); status != http.StatusUnauthorized || fake.call == "fleet" {
		t.Fatalf("fleet goals without a session = %d (store called: %v)", status, fake.call == "fleet")
	}
	// The first login claims the dev accounts (acc-1); acc-2 stays someone else's.
	e.login(c, "/", owner)
	status, body := e.do(c, "GET", "/api/fleet/goals", "", false)
	if status != http.StatusOK || fmt.Sprint(fake.fleet) != "[acc-1]" || len(body["accounts"].([]any)) != 1 {
		t.Fatalf("fleet goals = %d %v (store got %v)", status, body, fake.fleet)
	}
	// Another user, with no accounts, reads an empty fleet.
	other := e.client()
	e.login(other, "/", authtest.User{Subject: "200", Username: "other", Roles: []string{"owner"}})
	if status, _ = e.do(other, "GET", "/api/fleet/goals", "", false); status != http.StatusOK || len(fake.fleet) != 0 {
		t.Fatalf("other user's fleet = %d (store got %v)", status, fake.fleet)
	}
}
