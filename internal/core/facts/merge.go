// Package facts merges the last observation of each source into the current
// fact of an account (docs/10 section 5, decision E1). It is pure: the store
// keeps one observation per (account, facet, key, source) and writes the
// merged facts to player_state, a projection.
//
// Facts that only grow (XP and level, quest state, diary completion, activity
// score) take the greatest value among the sources, so neither the arrival
// order nor a client clock decides. Every other facet (varbits, varps, items,
// collection log, combat achievements) has a single source today and takes
// the newest observation.
package facts

import (
	"encoding/json"
	"sort"
	"time"
)

// Observation is the last value one source reported for a fact.
type Observation struct {
	Key        string
	Source     string
	Value      json.RawMessage
	SnapshotID string
	ObservedAt time.Time
}

// Fact is the merged fact of one key. Source, SnapshotID and ObservedAt are
// the provenance of the winning observation (the evidence of a verdict cites
// them).
type Fact struct {
	Key        string
	Value      json.RawMessage
	Source     string
	SnapshotID string
	ObservedAt time.Time
}

// sourceRank breaks exact ties: plugin > wikisync > hiscores > wom > import.
var sourceRank = map[string]int{"plugin": 5, "wikisync": 4, "hiscores": 3, "wom": 2, "import": 1}

// Merge returns one fact per key of the observations of one facet, sorted by
// key. The result does not depend on the order of obs.
func Merge(facet string, obs []Observation) []Fact {
	byKey := map[string][]Observation{}
	for _, o := range obs {
		byKey[o.Key] = append(byKey[o.Key], o)
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Fact, 0, len(keys))
	for _, k := range keys {
		group := byKey[k]
		sort.Slice(group, func(i, j int) bool { return newer(group[i], group[j]) })
		out = append(out, mergeKey(facet, group))
	}
	return out
}

// mergeKey merges the observations of one key, given newest first.
func mergeKey(facet string, group []Observation) Fact {
	switch facet {
	case "skill":
		return mergeSkill(group)
	case "activity":
		return greatest(group, func(o Observation) []int64 { return []int64{field(o.Value, "score")} })
	case "quest":
		return greatest(group, func(o Observation) []int64 { return []int64{questRank(o.Value)} })
	case "diary":
		return greatest(group, func(o Observation) []int64 { return []int64{boolRank(o.Value, "complete")} })
	default:
		return factOf(group[0])
	}
}

// greatest picks the observation with the greatest rank (compared field by
// field); among equal ranks the newest wins, since group is newest first.
func greatest(group []Observation, rank func(Observation) []int64) Fact {
	best, bestRank := group[0], rank(group[0])
	for _, o := range group[1:] {
		if r := rank(o); less(bestRank, r) {
			best, bestRank = o, r
		}
	}
	return factOf(best)
}

// mergeSkill takes the observation with the most XP (then level). The level
// is the greatest seen, and boosted, which only the plugin reports, comes from
// the plugin's observation: a hiscores update must not erase it.
func mergeSkill(group []Observation) Fact {
	f := greatest(group, func(o Observation) []int64 { return []int64{field(o.Value, "xp"), field(o.Value, "level")} })
	var value map[string]json.RawMessage
	if json.Unmarshal(f.Value, &value) != nil {
		return f
	}
	level := field(f.Value, "level")
	for _, o := range group {
		if l := field(o.Value, "level"); l > level {
			level = l
		}
	}
	if level >= 0 {
		value["level"] = json.RawMessage(itoa(level))
	}
	if _, ok := value["boosted"]; !ok {
		for _, o := range group {
			var v map[string]json.RawMessage
			if o.Source == "plugin" && json.Unmarshal(o.Value, &v) == nil && v["boosted"] != nil {
				value["boosted"] = v["boosted"]
				break
			}
		}
	}
	f.Value, _ = json.Marshal(value)
	return f
}

func factOf(o Observation) Fact {
	return Fact{Key: o.Key, Value: o.Value, Source: o.Source, SnapshotID: o.SnapshotID, ObservedAt: o.ObservedAt}
}

// newer orders observations: later observed_at, then the higher source rank.
func newer(a, b Observation) bool {
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.After(b.ObservedAt)
	}
	if sourceRank[a.Source] != sourceRank[b.Source] {
		return sourceRank[a.Source] > sourceRank[b.Source]
	}
	return a.SnapshotID > b.SnapshotID
}

func less(a, b []int64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// field reads an integer field of a JSON object; -1 when absent or invalid.
func field(raw json.RawMessage, name string) int64 {
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil || v[name] == nil {
		return -1
	}
	var n int64
	if json.Unmarshal(v[name], &n) != nil {
		return -1
	}
	return n
}

// Quest states as RuneLite names them; a quest never goes back.
var questStates = map[string]int64{"NOT_STARTED": 0, "IN_PROGRESS": 1, "FINISHED": 2}

func questRank(raw json.RawMessage) int64 {
	var v struct {
		State string `json:"state"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return -1
	}
	if r, ok := questStates[v.State]; ok {
		return r
	}
	return -1
}

func boolRank(raw json.RawMessage, name string) int64 {
	var v map[string]bool
	if json.Unmarshal(raw, &v) != nil {
		return -1
	}
	if b, ok := v[name]; ok && b {
		return 1
	}
	return 0
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
