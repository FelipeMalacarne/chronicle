package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// clientGame is a client's Agent Server that keeps its settings, its active
// Microbot plugins and its Hub installs, and records the writes.
type clientGame struct {
	stubGame
	mu        sync.Mutex
	settings  map[string]*string // "group.key"
	active    map[string]bool
	installed map[string]string
	writes    []string
}

func (g *clientGame) JSON(_ context.Context, method, path string, q url.Values, body any, _ time.Duration) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, _ := json.Marshal(body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	switch method + " " + path {
	case "GET /settings/plugin":
		return json.Marshal(map[string]any{"value": g.settings[q.Get("group")+"."+q.Get("key")]})
	case "POST /settings/plugin":
		v, _ := m["value"].(string)
		g.settings[m["group"].(string)+"."+m["key"].(string)] = &v
		g.writes = append(g.writes, "set "+m["group"].(string)+"."+m["key"].(string)+"="+v)
	case "GET /scripts":
		var s []map[string]any
		for c, a := range g.active {
			s = append(s, map[string]any{"className": c, "active": a})
		}
		return json.Marshal(map[string]any{"scripts": s})
	case "GET /plugins":
		var p []map[string]any
		for n, v := range g.installed {
			p = append(p, map[string]any{"internalName": n, "installedVersion": v, "loaded": true})
		}
		return json.Marshal(map[string]any{"plugins": p})
	case "POST /plugins/install":
		g.installed[m["name"].(string)], _ = m["version"].(string)
		g.writes = append(g.writes, "install "+m["name"].(string))
	case "POST /scripts/start":
		g.active[m["className"].(string)] = true
		g.writes = append(g.writes, "start "+m["className"].(string))
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func TestClientDriftRoutesPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_farm', 'dev', 'Farm', 'farm', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	repo := storeworkers.New(pool, testVaultKey(t), nil)
	if _, err := repo.PutProfile(ctx, "dev", "farm", "", agent.ClientProfile{
		Plugins: []agent.ProfilePlugin{
			{Source: agent.PluginHub, Name: "GotrPlugin", Version: "1.2.3", ClassName: "a.GotrPlugin", Start: true},
			{Source: agent.PluginOwn, Name: "a.BlackjackerPlugin", Start: true},
		},
		Settings: []agent.ProfileSetting{
			{Group: "entityhider", Key: "hidePlayers", Value: "true"},
			{Group: "fm-blackjacker", Key: "minWines", Value: "2"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetAccountClient(ctx, "acc_farm", "farm", nil); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	audit := &memAudit{}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet,
		Microbot: &MicrobotOptions{Link: link, Audit: audit, Sessions: repo, Profiles: repo},
	}))
	t.Cleanup(srv.Close)
	api := srv.URL + "/api/accounts/client"

	if status, _, _ := call(t, "GET", api+"/drift?account=acc_farm", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("drift without an agent = %d", status)
	}
	off := "false"
	game := &clientGame{
		settings:  map[string]*string{"entityhider.hidePlayers": &off},
		active:    map[string]bool{"a.BlackjackerPlugin": false},
		installed: map[string]string{"GotrPlugin": "1.2.0"},
	}
	agentCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a := &agent.Agent{URL: srv.URL, Log: quiet, VNCAddr: "127.0.0.1:1", Hello: agent.Hello{Host: "zaros", Account: "acc_farm"},
		Executor: &agent.Executor{Game: game, Home: t.TempDir(), ClientCmd: []string{"echo", "up"}}}
	go func() { _ = a.Run(agentCtx) }()
	waitLinked(t, link, 1)

	// The drift: both settings and both plugins differ.
	status, body, _ := call(t, "GET", api+"/drift?account=acc_farm", "")
	if status != http.StatusOK || body["profile"] != "farm" || len(body["settings"].([]any)) != 2 || len(body["plugins"].([]any)) != 2 {
		t.Fatalf("drift = %d %v", status, body)
	}

	// Saving: the client's value to the account; an unset one is skipped.
	status, body, _ = call(t, "POST", api+"/drift/save?account=acc_farm",
		`{"to":"account","keys":[{"group":"entityhider","key":"hidePlayers"},{"group":"fm-blackjacker","key":"minWines"}]}`)
	if status != http.StatusOK || len(body["skipped"].([]any)) != 1 || len(body["settings"].([]any)) != 1 {
		t.Fatalf("save to account = %d %v", status, body)
	}
	if ac, _ := repo.AccountClient(ctx, "acc_farm"); ac.Effective.Settings[0].Value != "false" {
		t.Fatalf("effective after save = %+v", ac.Effective)
	}
	// To the profile.
	if status, body, _ = call(t, "POST", api+"/drift/save?account=acc_farm", `{"to":"profile","keys":[{"group":"entityhider","key":"hidePlayers"}]}`); status != http.StatusOK {
		t.Fatalf("save to profile = %d %v", status, body)
	}
	if p, _ := repo.GetProfile(ctx, "dev", "farm"); p.Settings[0].Value != "false" {
		t.Fatalf("profile after save = %+v", p.Settings)
	}
	if status, _, _ = call(t, "POST", api+"/drift/save?account=acc_farm", `{"to":"elsewhere","keys":[{"group":"a","key":"b"}]}`); status != http.StatusBadRequest {
		t.Fatalf("bad target = %d", status)
	}
	if last := audit.last(); last.Action != "profile_save" || !last.OK || last.AccountID != "acc_farm" {
		t.Fatalf("audit = %+v", last)
	}

	// Applying the whole profile: the setting the client lacks, the Hub
	// plugin at the pinned version, the starts.
	status, body, _ = call(t, "POST", api+"/apply?account=acc_farm", "")
	if status != http.StatusOK || len(body["problems"].([]any)) != 0 {
		t.Fatalf("apply = %d %v", status, body)
	}
	game.mu.Lock()
	writes := strings.Join(game.writes, "\n")
	game.mu.Unlock()
	for _, want := range []string{"set fm-blackjacker.minWines=2", "install GotrPlugin", "start a.BlackjackerPlugin", "start a.GotrPlugin"} {
		if !strings.Contains(writes, want) {
			t.Errorf("apply did not %q:\n%s", want, writes)
		}
	}
	status, body, _ = call(t, "GET", api+"/drift?account=acc_farm", "")
	if status != http.StatusOK || len(body["settings"].([]any)) != 0 || len(body["plugins"].([]any)) != 0 {
		t.Fatalf("drift after apply = %d %v", status, body)
	}
	if status, _, _ = call(t, "POST", api+"/apply?account=acc_farm", `{"keys":[{"group":"nope","key":"x"}]}`); status != http.StatusBadRequest {
		t.Fatalf("apply a key outside the profile = %d", status)
	}

	// The Cockpit's start of an attached client applies the profile.
	game.mu.Lock()
	game.writes = nil
	game.mu.Unlock()
	status, body, _ = call(t, "POST", srv.URL+"/api/microbot/client/start?account=acc_farm", "")
	if status != http.StatusOK || body["profile"] == nil {
		t.Fatalf("start = %d %v", status, body)
	}
	game.mu.Lock()
	defer game.mu.Unlock()
	if !strings.Contains(strings.Join(game.writes, "\n"), "set entityhider.hidePlayers=false") {
		t.Fatalf("start did not apply the profile: %v", game.writes)
	}
}
