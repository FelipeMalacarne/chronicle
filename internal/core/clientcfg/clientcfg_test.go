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
		{"quest bot never fetches", CheckSettings([]Setting{{QuestHelperGroup, QuestObtainMissingItems, QuestObtainNo}}), ""},
		{"quest bot to the GE", CheckSettings([]Setting{{QuestHelperGroup, QuestObtainMissingItems, "YES"}}), "only ever NO"},
		{"quest bot asks", CheckSetting(Setting{"QuestHelper", QuestObtainMissingItems, "ASK"}), "Grand Exchange"},
		{"pinned key alone", CheckPinned(QuestHelperGroup, QuestObtainMissingItems, ""), ""},
		{"quest bot switch", CheckSetting(Setting{QuestHelperGroup, QuestTurnOn, "true"}), ""},
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

func TestCheckPluginSettings(t *testing.T) {
	ok := []Setting{{"MicroTutIsland", "toggleMusic", "false"}, {"microtutisland", "toggleRoofs", "true"}}
	for _, c := range []struct {
		name  string
		class string
		list  []Setting
		want  string
	}{
		{"the tutorial's toggles", TutorialIslandClass, ok, ""},
		{"none", TutorialIslandClass, nil, ""},
		{"a key the plugin lacks", TutorialIslandClass, []Setting{{"MicroTutIsland", "ironman", "true"}}, "not a setting"},
		{"another group", TutorialIslandClass, []Setting{{"other", "toggleMusic", "true"}}, "not a setting"},
		{"a value off the toggle", TutorialIslandClass, []Setting{{"MicroTutIsland", "toggleMusic", "1"}}, "true, false"},
		{"the general rules still apply", TutorialIslandClass, []Setting{{"runelite", "toggleMusic", "true"}}, "not edited"},
		{"an unknown plugin takes any valid setting", "a.b.OtherPlugin", []Setting{{"other", "mode", "FAST"}}, ""},
		{"and not a secret", "a.b.OtherPlugin", []Setting{{"other", "token", "x"}}, "secret"},
	} {
		err := CheckPluginSettings(c.class, c.list)
		if (c.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	// Whatever the tutorial plugin may be set to, none of it is a pinned or
	// ironman setting: its keys are quality-of-life toggles only.
	for k := range KnownPlugins[TutorialIslandClass].Keys {
		if !strings.HasPrefix(k, "toggle") {
			t.Errorf("the tutorial plugin may set %q, which is not a toggle", k)
		}
	}
}
