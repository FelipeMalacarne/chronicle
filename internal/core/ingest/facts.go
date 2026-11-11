package ingest

import (
	"sort"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// Facets the ingest derives from a push besides the raw maps.
const (
	FacetQuest = "quest"
	FacetDiary = "diary"
	// FacetTimer: one fact per Time Tracking tab, value {state, ready_at,
	// label}; FacetPosition: one fact, key "player", value {world, x, y,
	// plane, region}.
	FacetTimer    = "timer"
	FacetPosition = "position"
)

// Fact is one derived player_state fact: (facet, key) and its JSON value.
type Fact struct {
	Facet string
	Key   string
	Value map[string]any
}

// QuestFacts resolves the RuneLite names of data.quest through the registry
// (canonical name, WikiSync/RuneLite alias or curated alias) and returns one
// "quest" fact per catalog quest, keyed by its canonical wiki name, which is
// the key progress.EvaluateObjective reads. Names that do not resolve to
// exactly one catalog quest are returned in unresolved, sorted, and never
// become facts: a guess could mark the wrong quest. Facts are sorted by key.
func QuestFacts(reg *entities.Registry, quests QuestMap) (facts []Fact, unresolved []string) {
	if len(quests) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(quests))
	for name := range quests {
		names = append(names, name)
	}
	sort.Strings(names)
	byKey := map[string]string{}
	for _, name := range names {
		key, ok := canonicalQuest(reg, name)
		if !ok {
			unresolved = append(unresolved, name)
			continue
		}
		state := quests[name]
		// Two client names for one catalog quest keep the furthest state, so
		// the result does not depend on map order.
		if prev, seen := byKey[key]; seen && questRank(prev) >= questRank(state) {
			continue
		}
		byKey[key] = state
	}
	for key, state := range byKey {
		facts = append(facts, Fact{Facet: FacetQuest, Key: key, Value: map[string]any{"state": state}})
	}
	sortFacts(facts)
	return facts, unresolved
}

func canonicalQuest(reg *entities.Registry, name string) (string, bool) {
	if reg == nil {
		return "", false
	}
	res := reg.ResolveQuest(name)
	if res.Status != entities.Resolved {
		return "", false
	}
	q, ok := reg.Quest(res.Ref.ID)
	if !ok {
		return "", false
	}
	return q.Name, true
}

func questRank(state string) int {
	switch state {
	case QuestInProgress:
		return 1
	case QuestFinished:
		return 2
	}
	return 0
}

// DiaryFacts derives the "diary" facet (one fact per tier, key
// "area:tier", value {"complete": bool}) from the reference flag vars of the
// registry (content/reference/varbits.json, subject "diary:<area>:<tier>")
// whose value the push carries. A tier whose var is absent from the push
// yields no fact, so it stays unknown. Facts are sorted by key.
func DiaryFacts(reg *entities.Registry, varbits, varps *IntMap) []Fact {
	if reg == nil {
		return nil
	}
	var facts []Fact
	for _, v := range reg.Vars() {
		key, ok := strings.CutPrefix(v.Subject, string(entities.KindDiary)+":")
		if !ok || v.Semantics != entities.VarFlag || v.Test == nil {
			continue
		}
		value, ok := varValue(*v.Test, varbits, varps)
		if !ok {
			continue
		}
		facts = append(facts, Fact{Facet: FacetDiary, Key: key, Value: map[string]any{"complete": TestHolds(*v.Test, value)}})
	}
	sortFacts(facts)
	return facts
}

func varValue(t entities.VarTest, varbits, varps *IntMap) (int64, bool) {
	values := varbits
	if t.Var == entities.Varp {
		values = varps
	}
	if values == nil {
		return 0, false
	}
	value, ok := (*values)[strconv.Itoa(t.ID)]
	return value, ok
}

// TestHolds reports whether value satisfies a reference var test: equal to
// Eq, or at least Min.
func TestHolds(t entities.VarTest, value int64) bool {
	switch {
	case t.Eq != nil:
		return value == int64(*t.Eq)
	case t.Min != nil:
		return value >= int64(*t.Min)
	}
	return false
}

func sortFacts(facts []Fact) {
	sort.Slice(facts, func(i, j int) bool { return facts[i].Key < facts[j].Key })
}
