package clientcfg

import (
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"class", CheckClass("net.runelite.client.plugins.microbot.tickfisher.TickFisherPlugin"), ""},
		{"bare class", CheckClass("TickFisherPlugin"), "fully qualified"},
		{"long class", CheckClass("a." + strings.Repeat("b", MaxClass)), "fully qualified"},
		{"agent server", CheckClass("net.runelite.client.plugins.microbot.agentserver.AgentServerPlugin"), "not controlled"},
		{"group", CheckGroup("tickfisher"), ""},
		{"protected group", CheckGroup("AgentServer"), "not edited"},
		{"group shape", CheckGroup("a b"), "letters"},
		{"key", CheckKey("fishSpot"), ""},
		{"secret key", CheckKey("apiKey"), "secret"},
		{"pin", CheckKey("pin"), "secret"},
		{"value", CheckValue(strings.Repeat("x", MaxValue)), ""},
		{"long value", CheckValue(strings.Repeat("x", MaxValue+1)), "at most"},
		{"nul", CheckValue("a\x00b"), "at most"},
		{"settings", CheckSettings([]Setting{{"t", "a", "1"}, {"t", "b", "2"}}), ""},
		{"twice", CheckSettings([]Setting{{"t", "a", "1"}, {"T", "a", "2"}}), "twice"},
		{"too many", CheckSettings(make([]Setting, MaxSettings+1)), "at most"},
		{"a bad one", CheckSettings([]Setting{{"t", "token", "1"}}), "config/0"},
	} {
		got := ""
		if c.err != nil {
			got = c.err.Error()
		}
		if (c.want == "") != (c.err == nil) || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
