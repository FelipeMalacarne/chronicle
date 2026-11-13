package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeidentity "github.com/FelipeMalacarne/chronicle/internal/store/identity"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// The fleet's plugins (V8): the model, each account's own profile, the
// groups by use and the live state of the linked client.
func TestFleetPluginsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		  ('acc_farm', 'dev', 'Farm', 'farm', 'ironman', 'oziris-main', 'enhanced'),
		  ('acc_two', 'dev', 'Two', 'two', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	const (
		blackjacker = "net.runelite.client.plugins.microbot.fm.blackjacker.BlackjackerPlugin"
		catSwap     = "net.runelite.client.plugins.microbot.fm.catswap.CatSwapPlugin"
		fps         = "net.runelite.client.plugins.fps.FpsPlugin"
	)
	repo := storeworkers.New(pool, testVaultKey(t), nil)
	if _, err := repo.PutProfile(ctx, "dev", FleetModel, "", agent.ClientProfile{Plugins: []agent.ProfilePlugin{
		{Source: agent.PluginHub, Name: "GotrPlugin", Version: "1.2.3", ClassName: "a.GotrPlugin", Start: true},
		{Source: agent.PluginOwn, Name: blackjacker},
		{Source: agent.PluginCore, Name: fps, Start: true},
	}}); err != nil {
		t.Fatal(err)
	}
	own := []agent.ProfileSetting{{Group: "fm-blackjacker", Key: "minWines", Value: "3"}}
	if _, err := repo.PutProfile(ctx, "dev", "acc_farm", FleetModel, agent.ClientProfile{Settings: own}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetAccountClient(ctx, "acc_farm", "acc_farm", nil); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: quiet, Identity: storeidentity.New(pool),
		Microbot: &MicrobotOptions{Link: link, Audit: &memAudit{}, Sessions: repo, Profiles: repo},
	}))
	t.Cleanup(srv.Close)

	triggers := filepath.Join(t.TempDir(), "agent-triggers.json")
	if err := os.WriteFile(triggers, []byte(`[{"event":"kitten_grown","enabled":true,"run":{"className":"`+catSwap+`"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	game := &clientGame{
		settings:  map[string]*string{},
		active:    map[string]bool{blackjacker: true, catSwap: false, "net.runelite.client.plugins.microbot.MicrobotPlugin": true},
		installed: map[string]string{"GotrPlugin": "1.2.0"},
	}
	agentCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a := &agent.Agent{URL: srv.URL, Log: quiet, VNCAddr: "127.0.0.1:1", Hello: agent.Hello{Host: "zaros", Account: "acc_farm"},
		Executor: &agent.Executor{Game: game, Home: t.TempDir(), ClientCmd: []string{"echo"}, TriggerFile: triggers}}
	go func() { _ = a.Run(agentCtx) }()
	waitLinked(t, link, 1)

	status, body, _ := call(t, "GET", srv.URL+"/api/fleet/plugins", "")
	if status != http.StatusOK || body["model"].(map[string]any)["name"] != FleetModel {
		t.Fatalf("fleet plugins = %d %v", status, body)
	}
	accounts := map[string]map[string]any{}
	for _, x := range body["accounts"].([]any) {
		acc := x.(map[string]any)
		accounts[acc["account"].(string)] = acc
	}
	if f := accounts["acc_farm"]; f["profile"] != "acc_farm" || f["parent"] != FleetModel || f["connected"] != true || len(f["own"].([]any)) != 1 {
		t.Fatalf("acc_farm = %v", f)
	}
	if two := accounts["acc_two"]; two["connected"] != false || two["profile"] != nil {
		t.Fatalf("acc_two = %v", two)
	}
	type row struct{ group, latest string }
	got := map[string]row{}
	states := map[string][]any{}
	for _, x := range body["plugins"].([]any) {
		p := x.(map[string]any)
		latest, _ := p["latest"].(string)
		got[p["name"].(string)] = row{p["group"].(string), latest}
		states[p["name"].(string)] = p["accounts"].([]any)
	}
	want := map[string]row{
		"GotrPlugin": {GroupAlways, "9.9.9"}, fps: {GroupAlways, ""}, blackjacker: {GroupPlays, ""}, catSwap: {GroupRoutines, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("plugins = %v", got)
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %+v, want %+v", name, got[name], w)
		}
	}
	// The linked client's live state; the other account has none.
	for _, st := range states["GotrPlugin"] {
		s := st.(map[string]any)
		switch s["account"] {
		case "acc_farm":
			if s["installed"] != "1.2.0" || s["in_profile"] != true {
				t.Errorf("gotr on acc_farm = %v", s)
			}
		case "acc_two":
			if s["installed"] != nil || s["in_profile"] != false {
				t.Errorf("gotr on acc_two = %v", s)
			}
		}
	}
	for _, st := range states[blackjacker] {
		if s := st.(map[string]any); s["account"] == "acc_farm" && s["active"] != true {
			t.Errorf("blackjacker on acc_farm = %v", s)
		}
	}
}
