package ingest

import (
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func registry(t *testing.T) *entities.Registry {
	t.Helper()
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestQuestFacts(t *testing.T) {
	reg := registry(t)
	tests := []struct {
		name           string
		quests         QuestMap
		wantFacts      []Fact
		wantUnresolved []string
	}{
		{name: "empty", quests: QuestMap{}},
		{
			name:   "canonical names",
			quests: QuestMap{"Cook's Assistant": QuestFinished, "X Marks the Spot": QuestInProgress},
			wantFacts: []Fact{
				{Facet: FacetQuest, Key: "Cook's Assistant", Value: map[string]any{"state": QuestFinished}},
				{Facet: FacetQuest, Key: "X Marks the Spot", Value: map[string]any{"state": QuestInProgress}},
			},
		},
		{
			name:   "RuneLite names that differ from the wiki resolve by alias",
			quests: QuestMap{"Recipe for Disaster - Mountain Dwarf": QuestNotStarted, "Vale Totems": QuestFinished},
			wantFacts: []Fact{
				{Facet: FacetQuest, Key: "Recipe for Disaster/Freeing the Mountain Dwarf", Value: map[string]any{"state": QuestNotStarted}},
				{Facet: FacetQuest, Key: "Vale Totems (miniquest)", Value: map[string]any{"state": QuestFinished}},
			},
		},
		{
			name:   "two client names for one quest keep the furthest state",
			quests: QuestMap{"Desert Treasure": QuestInProgress, "Desert Treasure I": QuestFinished},
			wantFacts: []Fact{
				{Facet: FacetQuest, Key: "Desert Treasure I", Value: map[string]any{"state": QuestFinished}},
			},
		},
		{
			name:           "unknown names never become facts",
			quests:         QuestMap{"A Quest From The Future": QuestFinished, "Cook's Assistant": QuestNotStarted},
			wantFacts:      []Fact{{Facet: FacetQuest, Key: "Cook's Assistant", Value: map[string]any{"state": QuestNotStarted}}},
			wantUnresolved: []string{"A Quest From The Future"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, unresolved := QuestFacts(reg, tt.quests)
			if !reflect.DeepEqual(facts, tt.wantFacts) || !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Fatalf("QuestFacts() = %v, %v\nwant %v, %v", facts, unresolved, tt.wantFacts, tt.wantUnresolved)
			}
		})
	}
	if facts, unresolved := QuestFacts(nil, QuestMap{"Cook's Assistant": QuestFinished}); facts != nil || len(unresolved) != 1 {
		t.Fatalf("without a registry = %v, %v", facts, unresolved)
	}
}

// Every quest name RuneLite reports (the pinned Quest.java sample that the
// plugin's client is built against) must resolve to exactly one catalog
// quest, or its state would be dropped by the ingest.
func TestRuneLiteQuestNamesResolve(t *testing.T) {
	reg := registry(t)
	raw, err := os.ReadFile("../../../data/samples/runelite/Quest.java")
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`(?m)^\s+[A-Z0-9_]+\(\d+, "([^"]+)"\)`).FindAllStringSubmatch(string(raw), -1)
	if len(matches) < 200 {
		t.Fatalf("parsed only %d quests from Quest.java", len(matches))
	}
	quests := QuestMap{}
	for _, m := range matches {
		quests[m[1]] = QuestFinished
	}
	facts, unresolved := QuestFacts(reg, quests)
	if len(unresolved) != 0 {
		t.Fatalf("RuneLite quest names without a catalog quest: %q", unresolved)
	}
	if len(facts) != len(quests) {
		t.Fatalf("%d RuneLite names became %d facts: two names share a catalog quest", len(quests), len(facts))
	}
}

func TestDiaryFacts(t *testing.T) {
	reg := registry(t)
	varbits := func(m map[string]int64) *IntMap { v := IntMap(m); return &v }
	tests := []struct {
		name    string
		varbits *IntMap
		varps   *IntMap
		want    []Fact
	}{
		{name: "no varbits"},
		{name: "varbits without reference vars", varbits: varbits(map[string]int64{"1": 1})},
		{
			name:    "complete and incomplete tiers",
			varbits: varbits(map[string]int64{"4458": 1, "4459": 0, "3578": 1}),
			varps:   varbits(map[string]int64{"101": 33}),
			want: []Fact{
				{Facet: FacetDiary, Key: "ardougne:easy", Value: map[string]any{"complete": true}},
				{Facet: FacetDiary, Key: "ardougne:medium", Value: map[string]any{"complete": false}},
				{Facet: FacetDiary, Key: "karamja:easy", Value: map[string]any{"complete": true}},
			},
		},
		{
			name:    "a value other than the flag is not complete",
			varbits: varbits(map[string]int64{"4458": 2}),
			want:    []Fact{{Facet: FacetDiary, Key: "ardougne:easy", Value: map[string]any{"complete": false}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DiaryFacts(reg, tt.varbits, tt.varps); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DiaryFacts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTestHolds(t *testing.T) {
	one, three := 1, 3
	tests := []struct {
		test  entities.VarTest
		value int64
		want  bool
	}{
		{entities.VarTest{Var: entities.Varbit, ID: 1, Eq: &one}, 1, true},
		{entities.VarTest{Var: entities.Varbit, ID: 1, Eq: &one}, 2, false},
		{entities.VarTest{Var: entities.Varp, ID: 1, Min: &three}, 3, true},
		{entities.VarTest{Var: entities.Varp, ID: 1, Min: &three}, 2, false},
		{entities.VarTest{Var: entities.Varp, ID: 1}, 2, false},
	}
	for _, tt := range tests {
		if got := TestHolds(tt.test, tt.value); got != tt.want {
			t.Errorf("TestHolds(%+v, %d) = %v, want %v", tt.test, tt.value, got, tt.want)
		}
	}
}
