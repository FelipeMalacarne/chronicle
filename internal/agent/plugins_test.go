package agent

import (
	"context"
	"encoding/json"
	"errors"
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
