package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidateProfile(t *testing.T) {
	ok := ClientProfile{
		Plugins: []ProfilePlugin{
			{Source: PluginHub, Name: "GotrPlugin", Version: "1.2.3", ClassName: "net.runelite.client.plugins.microbot.gotr.GotrPlugin", Start: true},
			{Source: PluginHub, Name: "QuestHelperPlugin"},
			{Source: PluginOwn, Name: "net.runelite.client.plugins.microbot.fm.blackjacker.BlackjackerPlugin", Start: true},
			{Source: PluginCore, Name: "net.runelite.client.plugins.entityhider.EntityHiderPlugin", Start: true},
		},
		Settings: []ProfileSetting{
			{Group: "entityhider", Key: "hidePlayers", Value: "true"},
			{Group: "fm-blackjacker", Key: "room", Value: "THUG_TENT"},
			{Group: "fpscontrol", Key: "maxFps", Value: ""},
		},
	}
	if err := ValidateProfile(ok); err != nil {
		t.Fatalf("valid profile: %v", err)
	}
	if err := ValidateProfile(ClientProfile{}); err != nil {
		t.Fatalf("empty profile: %v", err)
	}
	many := ClientProfile{}
	for range maxProfileSettings + 1 {
		many.Settings = append(many.Settings, ProfileSetting{Group: "g", Key: "k", Value: "v"})
	}
	for name, p := range map[string]ClientProfile{
		"unknown source":      {Plugins: []ProfilePlugin{{Source: "jar", Name: "a.B"}}},
		"hub with a class":    {Plugins: []ProfilePlugin{{Source: PluginHub, Name: "a.BPlugin"}}},
		"hub start, no class": {Plugins: []ProfilePlugin{{Source: PluginHub, Name: "GotrPlugin", Start: true}}},
		"hub bad version":     {Plugins: []ProfilePlugin{{Source: PluginHub, Name: "GotrPlugin", Version: "1 2"}}},
		"hub bad class":       {Plugins: []ProfilePlugin{{Source: PluginHub, Name: "GotrPlugin", ClassName: "Gotr"}}},
		"own by a short name": {Plugins: []ProfilePlugin{{Source: PluginOwn, Name: "Blackjacker"}}},
		"core with a version": {Plugins: []ProfilePlugin{{Source: PluginCore, Name: "a.BPlugin", Version: "1"}}},
		"own with class_name": {Plugins: []ProfilePlugin{{Source: PluginOwn, Name: "a.BPlugin", ClassName: "a.CPlugin"}}},
		"protected class":     {Plugins: []ProfilePlugin{{Source: PluginOwn, Name: "net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin"}}},
		"protected hub class": {Plugins: []ProfilePlugin{{Source: PluginHub, Name: "Agent", ClassName: "net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin", Start: true}}},
		"plugin twice":        {Plugins: []ProfilePlugin{{Source: PluginCore, Name: "a.BPlugin"}, {Source: PluginCore, Name: "a.BPlugin", Start: true}}},
		"protected group":     {Settings: []ProfileSetting{{Group: "runelite", Key: "fpsplugin", Value: "false"}}},
		"agent server group":  {Settings: []ProfileSetting{{Group: "agentServer", Key: "port", Value: "1"}}},
		"tracking group":      {Settings: []ProfileSetting{{Group: "fm-ironmanplanner", Key: "serverUrl", Value: "x"}}},
		"secret key":          {Settings: []ProfileSetting{{Group: "autologin", Key: "password", Value: "x"}}},
		"bank pin":            {Settings: []ProfileSetting{{Group: "bank", Key: "bankPin", Value: "1234"}}},
		"bad group":           {Settings: []ProfileSetting{{Group: "a b", Key: "k", Value: "v"}}},
		"long value":          {Settings: []ProfileSetting{{Group: "g", Key: "k", Value: strings.Repeat("x", maxSettingValue+1)}}},
		"nul in value":        {Settings: []ProfileSetting{{Group: "g", Key: "k", Value: "a\x00b"}}},
		"setting twice":       {Settings: []ProfileSetting{{Group: "g", Key: "k", Value: "1"}, {Group: "g", Key: "k", Value: "2"}}},
		"too many settings":   many,
	} {
		if err := ValidateProfile(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMergeProfiles(t *testing.T) {
	model := ClientProfile{
		Plugins: []ProfilePlugin{
			{Source: PluginCore, Name: "a.EntityHiderPlugin", Start: true},
			{Source: PluginHub, Name: "GotrPlugin", Version: "1.0.0"},
		},
		Settings: []ProfileSetting{{Group: "entityhider", Key: "hidePlayers", Value: "true"}, {Group: "gotr", Key: "mode", Value: "fast"}},
	}
	profile := ClientProfile{
		Plugins:  []ProfilePlugin{{Source: PluginHub, Name: "GotrPlugin", ClassName: "a.GotrPlugin", Start: true}, {Source: PluginOwn, Name: "a.CatSwapPlugin"}},
		Settings: []ProfileSetting{{Group: "gotr", Key: "mode", Value: "safe"}},
	}
	overrides := ClientProfile{Settings: []ProfileSetting{{Group: "entityhider", Key: "hidePlayers", Value: "false"}, {Group: "catswap", Key: "world", Value: "330"}}}
	got := MergeProfiles(model, profile, overrides)
	want := ClientProfile{
		Plugins: []ProfilePlugin{
			{Source: PluginCore, Name: "a.EntityHiderPlugin", Start: true},
			{Source: PluginHub, Name: "GotrPlugin", ClassName: "a.GotrPlugin", Start: true},
			{Source: PluginOwn, Name: "a.CatSwapPlugin"},
		},
		Settings: []ProfileSetting{
			{Group: "entityhider", Key: "hidePlayers", Value: "false"},
			{Group: "gotr", Key: "mode", Value: "safe"},
			{Group: "catswap", Key: "world", Value: "330"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge =\n%+v\nwant\n%+v", got, want)
	}
	if empty := MergeProfiles(); empty.Plugins == nil || empty.Settings == nil {
		t.Fatalf("an empty merge has nil lists: %+v", empty)
	}
}

// profileGame fails the Agent Server requests whose path and body contain
// one of fail.
type profileGame struct {
	*podGame
	fail []string
}

func (g *profileGame) JSON(ctx context.Context, method, path string, query url.Values, body any, timeout time.Duration) (json.RawMessage, error) {
	raw, err := g.podGame.JSON(ctx, method, path, query, body, timeout)
	b, _ := json.Marshal(body)
	for _, f := range g.fail {
		if strings.Contains(path+" "+string(b), f) {
			return nil, errors.New("HTTP 409: refused")
		}
	}
	return raw, err
}

func TestBootstrapAppliesTheProfile(t *testing.T) {
	e, pod, _ := podExecutor(t)
	game := &profileGame{podGame: pod, fail: []string{`"name":"BrokenPlugin"`, `"key":"badKey"`}}
	e.Game = game
	profile := &ClientProfile{
		Plugins: []ProfilePlugin{
			{Source: PluginHub, Name: "GotrPlugin", Version: "1.2.3", ClassName: "a.GotrPlugin", Start: true},
			{Source: PluginHub, Name: "BrokenPlugin", ClassName: "a.BrokenPlugin", Start: true},
			{Source: PluginOwn, Name: "a.BlackjackerPlugin", Start: true},
			{Source: PluginCore, Name: "a.IdlePlugin"},
		},
		Settings: []ProfileSetting{
			{Group: "fpscontrol", Key: "maxFps", Value: "15"},
			{Group: "gotr", Key: "badKey", Value: "x"},
			{Group: "gotr", Key: "mode", Value: "fast"},
		},
	}
	ctx := context.Background()
	res, err := e.Execute(ctx, CmdWorkerBootstrap, bootstrapParams(func(p *BootstrapParams) { p.Profile = profile }))
	if err != nil {
		t.Fatal(err)
	}
	r := res.(BootstrapResult)
	if !r.Started {
		t.Fatalf("bootstrap = %+v", r)
	}
	wantProblems := []string{"setting gotr.badKey: HTTP 409: refused", "install BrokenPlugin: HTTP 409: refused"}
	if !reflect.DeepEqual(r.Problems, wantProblems) {
		t.Fatalf("problems = %q, want %q", r.Problems, wantProblems)
	}

	// Settings, then installs, then starts; the profile's FPS over the
	// system's; a failed install is not started; a plugin without start is
	// left alone.
	pod.fakeGame.mu.Lock()
	var got []string
	for _, req := range pod.requests {
		if strings.HasPrefix(req, "POST ") {
			got = append(got, req)
		}
	}
	pod.fakeGame.mu.Unlock()
	want := []string{
		`POST /settings/plugin {"group":"fpscontrol","key":"limitFps","value":"true"}`,
		`POST /settings/plugin {"group":"fpscontrol","key":"maxFps","value":"15"}`,
		`POST /settings/plugin {"group":"gotr","key":"badKey","value":"x"}`,
		`POST /settings/plugin {"group":"gotr","key":"mode","value":"fast"}`,
		`POST /plugins/install {"name":"GotrPlugin","timeout":120,"version":"1.2.3"}`,
		`POST /plugins/install {"name":"BrokenPlugin","timeout":120}`,
		`POST /scripts/start {"className":"net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin"}`,
		`POST /scripts/start {"className":"net.runelite.client.plugins.fps.FpsPlugin"}`,
		`POST /scripts/start {"className":"a.GotrPlugin"}`,
		`POST /scripts/start {"className":"a.BlackjackerPlugin"}`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A later client_start applies the same profile and says what failed.
	if _, err = e.Execute(ctx, CmdClientStop, nil); err != nil {
		t.Fatal(err)
	}
	out, err := e.Execute(ctx, CmdClientStart, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o := out.(ClientOutput).Output; !strings.Contains(o, "\nprofile: install BrokenPlugin: HTTP 409: refused") {
		t.Fatalf("client_start output = %q", o)
	}
}

func TestBootstrapRefusesABadProfile(t *testing.T) {
	e, pod, _ := podExecutor(t)
	bad := &ClientProfile{Settings: []ProfileSetting{{Group: "agentServer", Key: "port", Value: "1"}}}
	_, err := e.Execute(context.Background(), CmdWorkerBootstrap, bootstrapParams(func(p *BootstrapParams) { p.Profile = bad }))
	if err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("bad profile = %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("the error carries a secret: %v", err)
	}
	pod.fakeGame.mu.Lock()
	defer pod.fakeGame.mu.Unlock()
	if len(pod.requests) != 0 {
		t.Fatalf("a refused bootstrap reached the Agent Server: %q", pod.requests)
	}
}
