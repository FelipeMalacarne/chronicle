package progress

import (
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func TestCATiersAndCollectionLog(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	none := func(string, string) FactSource { return FactSource{Source: "plugin"} }
	if got := caTiers(reg, nil, nil, none); len(got) != 0 {
		t.Fatalf("without varps = %+v", got)
	}
	if collectionLog(reg, nil, none) != nil {
		t.Fatal("collection log without varps")
	}
	// Every CA varp observed; tasks 0 (easy, Noxious Foe) and 574 (medium,
	// You're a wizard) done; the game says 1 easy.
	varps := map[int]int64{}
	for _, c := range reg.CombatAchievements() {
		varps[c.Proof.ID] |= 0
	}
	varps[3116] = 1
	varps[4204] = 1 << 30
	varps[2943], varps[2944] = 312, 1692
	got := caTiers(reg, varps, map[int]int64{12885: 1}, none)
	if len(got) != 6 || got[0].Tier != "easy" || got[5].Tier != "grandmaster" {
		t.Fatalf("tiers = %+v", got)
	}
	if got[0].Done != 1 || got[0].GameDone == nil || *got[0].GameDone != 1 || got[1].Done != 1 || got[1].GameDone != nil {
		t.Fatalf("easy %+v, medium %+v", got[0], got[1])
	}
	sum := 0
	for _, x := range got {
		sum += x.Total
	}
	if sum != len(reg.CombatAchievements()) {
		t.Fatalf("totals add to %d", sum)
	}
	// A tier whose varp is missing is not shown.
	delete(varps, 3116)
	if again := caTiers(reg, varps, nil, none); len(again) >= 6 {
		t.Fatalf("tiers with a varp missing = %d", len(again))
	}
	cl := collectionLog(reg, varps, none)
	if cl == nil || cl.Obtained != 312 || cl.Total == nil || *cl.Total != 1692 {
		t.Fatalf("collection log = %+v", cl)
	}
}
