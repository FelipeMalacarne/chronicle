package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValidateParams(t *testing.T) {
	cases := []struct {
		command, params string
		bad             string // substring of the error; empty means valid
	}{
		{CmdPlugins, ``, ""},
		{CmdHubSearch, `{"q":"birdhouse"}`, ""},
		{CmdHubSearch, `{"q":"a\nb"}`, "one line"},
		{CmdPluginInstall, `{"name":"GotrPlugin","version":"1.5.15"}`, ""},
		{CmdPluginInstall, `{"name":"../etc"}`, "internal name"},
		{CmdPluginInstall, `{"name":"GotrPlugin","version":"1.0; rm"}`, "bad version"},
		{CmdPluginRemove, `{"name":"GotrPlugin","version":"1.0"}`, "bad version"},
		{CmdPluginInstall, `{"name":"GotrPlugin","path":"/x"}`, "unknown field"},
		{CmdScriptStart, `{"className":"net.runelite.client.plugins.microbot.gotr.GotrPlugin"}`, ""},
		{CmdScriptStart, `{"className":"GotrPlugin"}`, "fully qualified"},
		{CmdScriptStop, `{"className":"net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin"}`, "not controlled"},
		{CmdScriptStop, `{"className":"net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin"}`, "not controlled"},
		// Reading a protected plugin's status is fine.
		{CmdScriptDetail, `{"className":"net.runelite.client.plugins.microbot.fm.ironmanplanner.IronmanPlannerPlugin"}`, ""},
		{CmdConfigGet, `{"group":"birdhouserunner","keys":["logs","seeds"]}`, ""},
		{CmdConfigGet, `{"group":"birdhouserunner","keys":[]}`, "between 1 and 50"},
		{CmdConfigGet, `{"group":"agentServer","keys":["port"]}`, "not edited"},
		{CmdConfigGet, `{"group":"AGENTSERVER","keys":["port"]}`, "not edited"},
		{CmdConfigGet, `{"group":"fm-ironmanplanner","keys":["url"]}`, "not edited"},
		{CmdConfigGet, `{"group":"someplugin","keys":["authToken"]}`, "secret"},
		{CmdConfigSet, `{"group":"birdhouserunner","key":"logs","value":"teak"}`, ""},
		{CmdConfigSet, `{"group":"birdhouserunner","key":"logs","value":null}`, ""},
		{CmdConfigSet, `{"group":"bank","key":"bankPin","value":"1234"}`, "secret"},
		{CmdConfigSet, `{"group":"runelite","key":"qolplugin","value":"true"}`, "not edited"},
		{CmdConfigSet, `{"group":"a b","key":"k","value":"v"}`, "group must be"},
		{CmdConfigSet, `{"group":"g","key":"k","value":"` + strings.Repeat("x", 2001) + `"}`, "2000"},
		{"scripts_deploy", `{}`, "unknown command"},
	}
	for _, c := range cases {
		_, err := ValidateParams(c.command, json.RawMessage(c.params))
		switch {
		case c.bad == "" && err != nil:
			t.Errorf("%s %s: unexpected error %v", c.command, c.params, err)
		case c.bad != "" && (err == nil || !strings.Contains(err.Error(), c.bad)):
			t.Errorf("%s %s: error = %v, want %q", c.command, c.params, err, c.bad)
		}
	}
}

func TestPluginCommandsThroughTheLink(t *testing.T) {
	game := &fakeGame{}
	link := linked(t, game)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var plugins Plugins
	if err := link.Call(ctx, CmdPlugins, nil, &plugins); err != nil || !strings.Contains(string(plugins.Scripts), "QoLPlugin") {
		t.Fatalf("plugins = %s %v", plugins.Scripts, err)
	}
	if err := link.Call(ctx, CmdPluginInstall, PluginParams{Name: "GotrPlugin"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := link.Call(ctx, CmdScriptStart, ScriptParams{ClassName: "a.b.GotrPlugin"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := link.Call(ctx, CmdScriptsPause, nil, nil); err != nil {
		t.Fatal(err)
	}
	var values ConfigValues
	if err := link.Call(ctx, CmdConfigGet, ConfigGetParams{Group: "qol", Keys: []string{"speed", "unset"}}, &values); err != nil {
		t.Fatal(err)
	}
	if v := values.Values["speed"]; v == nil || *v != "v" || values.Values["unset"] != nil {
		t.Fatalf("config values = %+v", values.Values)
	}
	if err := link.Call(ctx, CmdConfigSet, ConfigSetParams{Group: "qol", Key: "speed"}, nil); err != nil {
		t.Fatal(err)
	}
	var detail ScriptDetail
	if err := link.Call(ctx, CmdScriptDetail, ScriptParams{ClassName: "a.b.GotrPlugin"}, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Status == nil || detail.Errors["health"] == "" {
		t.Fatalf("detail = %+v", detail)
	}

	// The agent refuses on its own too, whatever the server let through.
	var cmd *CommandError
	err := link.Call(ctx, CmdScriptStop, json.RawMessage(`{"className":"net.runelite.client.plugins.microbot.MicrobotPlugin"}`), nil)
	if !errors.As(err, &cmd) || !strings.Contains(cmd.Message, "not controlled") {
		t.Fatalf("stop the core = %v", err)
	}

	want := []string{
		`GET /scripts`,
		`GET /plugins?installed=true`,
		`POST /plugins/install {"name":"GotrPlugin","timeout":120}`,
		`POST /scripts/start {"className":"a.b.GotrPlugin"}`,
		`POST /scripts/pause {}`,
		`GET /settings/plugin?group=qol&key=speed`,
		`GET /settings/plugin?group=qol&key=unset`,
		`POST /settings/plugin {"group":"qol","key":"speed","value":null}`,
		`GET /scripts/status?className=a.b.GotrPlugin`,
		`GET /scripts/health?className=a.b.GotrPlugin`,
		`GET /scripts/results?className=a.b.GotrPlugin`,
		`GET /debug/snapshot`,
	}
	game.mu.Lock()
	defer game.mu.Unlock()
	if !slices.Equal(game.requests, want) {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(game.requests, "\n"), strings.Join(want, "\n"))
	}
}

func TestGameView(t *testing.T) {
	ctx := context.Background()
	// Logged out: only /observe.
	out := &fakeGame{}
	view := (&Executor{Game: out}).gameView(ctx)
	if view.Observe == nil || view.Inventory != nil || len(out.requests) != 1 || out.requests[0] != "GET /observe?nearby=15&nearbyLimit=12" {
		t.Fatalf("logged out view = %+v, requests %v", view, out.requests)
	}
	// Logged in: every part, and a failing part carries its error.
	in := &loggedInGame{}
	view = (&Executor{Game: in}).gameView(ctx)
	if view.Inventory == nil || view.Skills == nil || view.Bank == nil || view.Ground == nil || view.Errors["machines"] == "" {
		t.Fatalf("logged in view = %+v", view)
	}
	if _, err := ValidateParams(CmdGameView, nil); err != nil {
		t.Fatal(err)
	}
}

type loggedInGame struct{ fakeGame }

func (g *loggedInGame) JSON(_ context.Context, _, path string, _ url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	switch path {
	case "/observe":
		return json.RawMessage(`{"loggedIn":true}`), nil
	case "/debug/snapshot":
		return nil, errors.New("HTTP 500")
	}
	return json.RawMessage(`{}`), nil
}

func TestGameActions(t *testing.T) {
	cases := []struct {
		command, params string
		bad             string
		want            string // the Agent Server request
	}{
		{CmdWalk, `{"x":3222,"y":3218,"plane":0}`, "", `POST /walk {"plane":0,"wait":false,"x":3222,"y":3218}`},
		{CmdWalk, `{"x":0,"y":3218}`, "between 1 and 16383", ""},
		{CmdWalk, `{"x":3222,"y":3218,"plane":4}`, "plane", ""},
		{CmdInventoryUse, `{"name":"Lobster","action":"Eat"}`, "", `POST /inventory/interact {"action":"Eat","name":"Lobster"}`},
		{CmdInventoryUse, `{"name":"Lobster"}`, "action is required", ""},
		{CmdInventoryUse, `{"name":"Lobster","id":379,"action":"Eat"}`, "not both", ""},
		{CmdInventoryUse, `{"name":"Lob\nster","action":"Eat"}`, "printable", ""},
		{CmdNPCInteract, `{"id":3010,"action":"Bank"}`, "", `POST /npcs/interact {"action":"Bank","id":3010}`},
		{CmdObjectInteract, `{"name":"Oak tree","action":"Chop down"}`, "", `POST /objects/interact {"action":"Chop down","name":"Oak tree"}`},
		{CmdGroundPickup, `{"name":"Bones"}`, "", `POST /ground-items/pickup {"name":"Bones"}`},
		{CmdGroundPickup, `{"name":"Bones","action":"Take"}`, "no action", ""},
		{CmdInventoryDrop, `{"name":"Logs","all":true}`, "", `POST /inventory/drop {"all":true,"name":"Logs"}`},
		{CmdInventoryDrop, `{"all":true}`, "name is required", ""},
		{CmdBankDeposit, `{"all":true}`, "", `POST /bank/deposit {"all":true}`},
		{CmdBankDeposit, `{"name":"Logs","all":true}`, "not both", ""},
		{CmdBankWithdraw, `{"name":"Pure essence","quantity":28,"noted":false}`, "", `POST /bank/withdraw {"name":"Pure essence","noted":false,"quantity":28}`},
		{CmdBankWithdraw, `{"name":"Pure essence"}`, "quantity", ""},
		{CmdBankOpen, ``, "", `POST /bank/open {}`},
		{CmdDialogueContinue, ``, "", `POST /dialogue/continue {}`},
		{CmdDialogueSelect, `{"index":2}`, "", `POST /dialogue/select {"index":2}`},
		{CmdDialogueSelect, `{"option":"Yes","index":2}`, "not both", ""},
		{CmdWalk, `{"x":1,"y":1,"keys":"F1"}`, "unknown field", ""},
	}
	for _, c := range cases {
		g := &fakeGame{}
		_, err := (&Executor{Game: g}).act(context.Background(), c.command, json.RawMessage(c.params))
		switch {
		case c.bad != "":
			if err == nil || !strings.Contains(err.Error(), c.bad) || len(g.requests) != 0 {
				t.Errorf("%s %s: error %v (want %q), requests %v", c.command, c.params, err, c.bad, g.requests)
			}
		case err != nil:
			t.Errorf("%s %s: %v", c.command, c.params, err)
		case len(g.requests) != 1 || g.requests[0] != c.want:
			t.Errorf("%s %s: requests %v, want %q", c.command, c.params, g.requests, c.want)
		}
	}
	for _, cmd := range GameActions {
		if !Known(cmd) {
			t.Errorf("%s is not in the closed list", cmd)
		}
	}
}

func TestConfigSchema(t *testing.T) {
	home := t.TempDir()
	e := &Executor{Home: home}
	if _, err := e.configSchema("a.b.GotrPlugin"); !errors.Is(err, ErrNoSchema) {
		t.Fatalf("no export = %v", err)
	}
	export := `{"version":1,"plugins":[
		{"className":"a.b.GotrPlugin","name":"GOTR","group":"gotr","sections":[{"key":"general","name":"General","position":0}],
		 "items":[{"key":"mode","name":"Mode","position":1,"type":"enum","values":[{"value":"FAST","label":"Fast"}]},
		          {"key":"apiToken","name":"Token","position":2,"type":"string"},
		          {"key":"eat","name":"Eat at","position":3,"type":"int","min":1,"max":99}]},
		{"className":"a.b.AgentServerPlugin","name":"Agent Server","group":"agentServer","items":[]}]}`
	if err := os.MkdirAll(filepath.Join(home, ".runelite", "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, SchemaFile), []byte(export), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := e.configSchema("a.b.GotrPlugin")
	if err != nil || s.Group != "gotr" || len(s.Items) != 2 || s.Items[0].Values[0].Label != "Fast" || *s.Items[1].Max != 99 {
		t.Fatalf("schema = %+v %v", s, err)
	}
	if _, err = e.configSchema("a.b.AgentServerPlugin"); err == nil || !strings.Contains(err.Error(), "not edited") {
		t.Fatalf("protected group = %v", err)
	}
	if _, err = e.configSchema("a.b.Other"); err == nil {
		t.Fatal("a plugin without options returned a schema")
	}
}
