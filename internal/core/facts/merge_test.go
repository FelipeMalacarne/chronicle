package facts

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func ob(key, source, value string, minutes int) Observation {
	return Observation{Key: key, Source: source, Value: json.RawMessage(value), SnapshotID: source + "-snap", ObservedAt: t0.Add(time.Duration(minutes) * time.Minute)}
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name       string
		facet      string
		obs        []Observation
		wantValue  string
		wantSource string
	}{
		{
			name:  "skill: more xp wins even when older",
			facet: "skill",
			obs: []Observation{
				ob("attack", "plugin", `{"level":60,"xp":300000,"boosted":63}`, 0),
				ob("attack", "hiscores", `{"level":59,"xp":290000}`, 30),
			},
			wantValue: `{"boosted":63,"level":60,"xp":300000}`, wantSource: "plugin",
		},
		{
			name:  "skill: hiscores ahead keeps the plugin's boosted",
			facet: "skill",
			obs: []Observation{
				ob("attack", "plugin", `{"level":60,"xp":300000,"boosted":63}`, 0),
				ob("attack", "hiscores", `{"level":61,"xp":310000}`, 30),
			},
			wantValue: `{"boosted":63,"level":61,"xp":310000}`, wantSource: "hiscores",
		},
		{
			name:  "skill: a plugin clock ahead does not hide newer hiscores xp",
			facet: "skill",
			obs: []Observation{
				ob("magic", "plugin", `{"level":40,"xp":37000}`, 600),
				ob("magic", "hiscores", `{"level":43,"xp":50000}`, 1),
			},
			wantValue: `{"level":43,"xp":50000}`, wantSource: "hiscores",
		},
		{
			name:  "skill: delta without level keeps the greatest level seen",
			facet: "skill",
			obs: []Observation{
				ob("prayer", "plugin", `{"xp":50000}`, 5),
				ob("prayer", "hiscores", `{"level":43,"xp":49000}`, 0),
			},
			wantValue: `{"level":43,"xp":50000}`, wantSource: "plugin",
		},
		{
			name:  "activity: the greatest score",
			facet: "activity",
			obs: []Observation{
				ob("Zulrah", "hiscores", `{"score":50,"rank":10}`, 0),
				ob("Zulrah", "wom", `{"score":48}`, 10),
			},
			wantValue: `{"score":50,"rank":10}`, wantSource: "hiscores",
		},
		{
			name:  "quest: a quest never goes back",
			facet: "quest",
			obs: []Observation{
				ob("cooks_assistant", "plugin", `{"state":"IN_PROGRESS"}`, 10),
				ob("cooks_assistant", "wikisync", `{"state":"FINISHED"}`, 0),
			},
			wantValue: `{"state":"FINISHED"}`, wantSource: "wikisync",
		},
		{
			name:  "diary: complete wins",
			facet: "diary",
			obs: []Observation{
				ob("ardougne:easy", "plugin", `{"complete":false}`, 10),
				ob("ardougne:easy", "wikisync", `{"complete":true}`, 0),
			},
			wantValue: `{"complete":true}`, wantSource: "wikisync",
		},
		{
			name:  "varbit: the newest observation",
			facet: "varbit",
			obs: []Observation{
				ob("4458", "plugin", `{"value":1}`, 0),
				ob("4458", "wikisync", `{"value":0}`, 5),
			},
			wantValue: `{"value":0}`, wantSource: "wikisync",
		},
		{
			name:  "equal time and rank: the source rank breaks the tie",
			facet: "varp",
			obs: []Observation{
				ob("101", "hiscores", `{"value":1}`, 0),
				ob("101", "plugin", `{"value":2}`, 0),
			},
			wantValue: `{"value":2}`, wantSource: "plugin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Merge(tt.facet, tt.obs)
			if len(got) != 1 {
				t.Fatalf("got %d facts", len(got))
			}
			if !jsonEqual(t, got[0].Value, tt.wantValue) || got[0].Source != tt.wantSource {
				t.Fatalf("got %s from %s, want %s from %s", got[0].Value, got[0].Source, tt.wantValue, tt.wantSource)
			}
			if got[0].SnapshotID != tt.wantSource+"-snap" {
				t.Fatalf("provenance %s does not match the winner", got[0].SnapshotID)
			}
		})
	}
}

// TestMergeOrderIndependent: the merged facts never depend on the order the
// observations arrive in, which is what makes a replay deterministic.
func TestMergeOrderIndependent(t *testing.T) {
	obs := []Observation{
		ob("attack", "plugin", `{"level":60,"xp":300000,"boosted":63}`, 0),
		ob("attack", "hiscores", `{"level":61,"xp":310000}`, 30),
		ob("magic", "plugin", `{"level":40,"xp":37000}`, 600),
		ob("magic", "hiscores", `{"level":43,"xp":50000}`, 1),
		ob("magic", "wom", `{"level":43,"xp":50000}`, 1),
	}
	want, _ := json.Marshal(Merge("skill", obs))
	r := rand.New(rand.NewSource(1))
	for range 50 {
		r.Shuffle(len(obs), func(i, j int) { obs[i], obs[j] = obs[j], obs[i] })
		if got, _ := json.Marshal(Merge("skill", obs)); string(got) != string(want) {
			t.Fatalf("order changed the result:\n%s\n%s", got, want)
		}
	}
}

func TestMergeKeysSorted(t *testing.T) {
	got := Merge("varbit", []Observation{ob("b", "plugin", `{"value":1}`, 0), ob("a", "plugin", `{"value":1}`, 0)})
	if len(got) != 2 || got[0].Key != "a" || got[1].Key != "b" {
		t.Fatalf("keys = %+v", got)
	}
}

func jsonEqual(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	var a, b any
	if json.Unmarshal(got, &a) != nil || json.Unmarshal([]byte(want), &b) != nil {
		t.Fatalf("invalid json %s / %s", got, want)
	}
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	return string(ga) == string(gb)
}
