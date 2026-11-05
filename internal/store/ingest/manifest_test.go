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
	if !slices.Equal(with.Varps, []int64{101}) {
		t.Errorf("varps = %v, want [101]", with.Varps)
	}
	if len(with.Varbits) != 48 || !slices.IsSorted(with.Varbits) {
		t.Errorf("varbits = %v, want the 48 diary tiers sorted", with.Varbits)
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
