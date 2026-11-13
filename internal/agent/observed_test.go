package agent

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestObservedPlugins(t *testing.T) {
	scripts := `{"count":6,"scripts":[
	  {"name":"Blackjack","className":"net.runelite.client.plugins.microbot.blackjack.BlackjackPlugin","active":true,"enabled":true},
	  {"name":"Off","className":"net.runelite.client.plugins.microbot.gotr.GotrPlugin","active":false,"enabled":false},
	  {"name":"Tick","className":"net.runelite.client.plugins.microbot.fm.tickfisher.TickFisherPlugin","enabled":true},
	  {"name":"Link","className":"net.runelite.client.plugins.microbot.fm.chroniclelink.ChronicleLinkPlugin","enabled":true},
	  {"name":"Fps","className":"net.runelite.client.plugins.fps.FpsPlugin","enabled":true},
	  {"name":"Agent","className":"net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin","enabled":true}]}`
	installed := `{"count":2,"total":2,"plugins":[
	  {"internalName":"BlackjackPlugin","installedVersion":"1.2.0","loaded":true},
	  {"internalName":"GotrPlugin","installedVersion":null,"loaded":false}]}`
	got, err := ObservedPlugins(Plugins{Scripts: json.RawMessage(scripts), Installed: json.RawMessage(installed)})
	if err != nil {
		t.Fatal(err)
	}
	want := []ProfilePlugin{
		{Source: PluginHub, Name: "BlackjackPlugin", ClassName: "net.runelite.client.plugins.microbot.blackjack.BlackjackPlugin", Start: true},
		{Source: PluginOwn, Name: "net.runelite.client.plugins.microbot.fm.tickfisher.TickFisherPlugin", Start: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestObservedPluginsIncomplete(t *testing.T) {
	for name, p := range map[string]Plugins{
		"empty":        {},
		"short list":   {Scripts: json.RawMessage(`{"count":3,"scripts":[]}`), Installed: json.RawMessage(`{"plugins":[]}`)},
		"no count":     {Scripts: json.RawMessage(`{"scripts":[]}`), Installed: json.RawMessage(`{"plugins":[]}`)},
		"bad hub list": {Scripts: json.RawMessage(`{"count":0,"scripts":[]}`), Installed: json.RawMessage(`nope`)},
	} {
		if got, err := ObservedPlugins(p); err == nil {
			t.Errorf("%s: want an error, got %+v", name, got)
		}
	}
	// An empty but complete read is a real answer: nothing is on.
	got, err := ObservedPlugins(Plugins{Scripts: json.RawMessage(`{"count":0,"scripts":[]}`), Installed: json.RawMessage(`{"plugins":[]}`)})
	if err != nil || len(got) != 0 {
		t.Fatalf("empty read: %+v %v", got, err)
	}
}
