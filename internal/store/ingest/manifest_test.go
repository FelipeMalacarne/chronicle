package ingest

import (
	"slices"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
)

func TestBuildDefinitionAsksForReferenceVars(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	without := BuildDefinition(nil, nil)
	with := BuildDefinition(nil, reg)
	if len(without.Varbits) != 0 || len(without.Varps) != 0 {
		t.Fatalf("without a catalog = %+v", without)
	}
	// Ardougne easy and Karamja elite diary tiers, and QUEST_POINTS.
	for _, id := range []int64{4458, 3611} {
		if !slices.Contains(with.Varbits, id) {
			t.Errorf("varbits %v miss diary varbit %d", with.Varbits, id)
		}
	}
	// QUEST_POINTS, COLLECTION_COUNT and _MAX, and the 21 CA_TASK_COMPLETED_<n>
	// varps that hold the combat achievements (0, 12, 13 and 20 checked).
	if len(with.Varps) != 24 || !slices.IsSorted(with.Varps) {
		t.Errorf("varps = %v, want 24 sorted", with.Varps)
	}
	for _, id := range []int64{101, 2943, 2944, 3116, 3128, 3387, 5673} {
		if !slices.Contains(with.Varps, id) {
			t.Errorf("varps %v miss %d", with.Varps, id)
		}
	}
	// The 48 diary tiers and the six CA_TOTAL_TASKS_COMPLETED_<tier>.
	if len(with.Varbits) != 54 || !slices.IsSorted(with.Varbits) || !slices.Contains(with.Varbits, 12890) {
		t.Errorf("varbits = %v, want the 48 diary tiers and 6 CA totals sorted", with.Varbits)
	}
	if with.Version == without.Version {
		t.Error("the reference vars must change the manifest version")
	}
	if again := BuildDefinition(nil, reg); again.Version != with.Version {
		t.Errorf("version is not stable: %d then %d", with.Version, again.Version)
	}

	manifest := with.manifest([]coreingest.TokenScope{coreingest.ScopeStateWrite}, false)
	if !manifest.Quests || manifest.Validate() != nil {
		t.Fatalf("manifest = %+v, %v", manifest, manifest.Validate())
	}
	if without.manifest([]coreingest.TokenScope{coreingest.ScopeStateWrite}, false).Quests {
		t.Fatal("a definition without a catalog cannot resolve quests and must not offer them")
	}
}
