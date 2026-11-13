package agent

import (
	"encoding/json"
	"reflect"
	"testing"
)

func str(s string) *string { return &s }

func TestProfileDrift(t *testing.T) {
	p := ClientProfile{
		Plugins: []ProfilePlugin{
			{Source: PluginHub, Name: "GotrPlugin", Version: "1.2.3", ClassName: "a.GotrPlugin", Start: true},
			{Source: PluginHub, Name: "QuestHelperPlugin"},
			{Source: PluginHub, Name: "BankTagsPlugin", ClassName: "a.BankTagsPlugin", Start: true},
			{Source: PluginOwn, Name: "a.BlackjackerPlugin", Start: true},
			{Source: PluginOwn, Name: "a.CatSwapPlugin"},
			{Source: PluginCore, Name: "net.runelite.client.plugins.fps.FpsPlugin", Start: true},
		},
		Settings: []ProfileSetting{
			{Group: "entityhider", Key: "hidePlayers", Value: "true"},
			{Group: "fm-blackjacker", Key: "minWines", Value: "2"},
			{Group: "fm-blackjacker", Key: "target", Value: "AUTO"},
			{Group: "unread", Key: "k", Value: "v"},
		},
	}
	c := ClientState{
		Values: map[string]map[string]*string{
			"entityhider":    {"hidePlayers": str("false")},
			"fm-blackjacker": {"minWines": nil, "target": str("AUTO")},
		},
		Active:    map[string]bool{"a.BlackjackerPlugin": false, "a.GotrPlugin": true, "a.BankTagsPlugin": true},
		Installed: map[string]string{"GotrPlugin": "1.2.0", "BankTagsPlugin": ""},
	}
	got := ProfileDrift(p, c)
	want := Drift{
		Settings: []SettingDrift{
			{Group: "entityhider", Key: "hidePlayers", Profile: "true", Client: str("false")},
			{Group: "fm-blackjacker", Key: "minWines", Profile: "2"},
		},
		Plugins: []PluginDrift{
			{Source: PluginHub, Name: "GotrPlugin", ClassName: "a.GotrPlugin", Problem: DriftVersion, Version: "1.2.0"},
			{Source: PluginHub, Name: "QuestHelperPlugin", Problem: DriftNotInstalled},
			{Source: PluginOwn, Name: "a.BlackjackerPlugin", Problem: DriftNotStarted},
		},
		Unchecked: []string{"net.runelite.client.plugins.fps.FpsPlugin"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("drift =\n%+v\nwant\n%+v", got, want)
	}
	if d := ProfileDrift(ClientProfile{}, ClientState{}); d.Settings == nil || d.Plugins == nil || d.Unchecked == nil {
		t.Fatalf("an empty drift has nil lists: %+v", d)
	}
}

func TestReadAgentServerLists(t *testing.T) {
	scripts := json.RawMessage(`{"count":2,"scripts":[{"name":"A","className":"a.A","active":true,"enabled":true},{"name":"B","className":"a.B","active":false}]}`)
	if got := ActiveScripts(scripts); !reflect.DeepEqual(got, map[string]bool{"a.A": true, "a.B": false}) {
		t.Fatalf("active = %v", got)
	}
	installed := json.RawMessage(`{"plugins":[{"internalName":"GotrPlugin","installedVersion":"1.2.0","loaded":true},{"internalName":"Old","installedVersion":null,"loaded":false},{"internalName":"NoVersion","installedVersion":null,"loaded":true}],"total":3}`)
	if got := InstalledPlugins(installed); !reflect.DeepEqual(got, map[string]string{"GotrPlugin": "1.2.0", "NoVersion": ""}) {
		t.Fatalf("installed = %v", got)
	}
	if got := ActiveScripts(json.RawMessage(`not json`)); len(got) != 0 {
		t.Fatalf("garbage = %v", got)
	}
}

func TestSettingGroups(t *testing.T) {
	var settings []ProfileSetting
	for _, k := range []string{"a", "b", "c"} {
		settings = append(settings, ProfileSetting{Group: "g", Key: k})
	}
	settings = append(settings, ProfileSetting{Group: "h", Key: "x"})
	got := SettingGroups(settings, 2)
	// A full batch starts another for its group, in the order the keys come.
	want := []ConfigGetParams{{Group: "g", Keys: []string{"a", "b"}}, {Group: "g", Keys: []string{"c"}}, {Group: "h", Keys: []string{"x"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %+v", got)
	}
}
