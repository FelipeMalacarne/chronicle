package content

import (
	"path/filepath"
	"testing"
)

func TestLoadoutFixtures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
	}{
		{name: "known", status: LoadoutKnown},
		{name: "partial", status: LoadoutPartial},
		{name: "unknown", status: LoadoutUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			guide, err := LoadGuideFile(filepath.Join("testdata", "loadouts", test.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			segment, ok := guide.Segment("bank-leg")
			if !ok || segment.Loadout == nil || segment.Loadout.Status != test.status {
				t.Fatalf("segment loadout = %#v, %v; want %q", segment, ok, test.status)
			}
			if test.status == LoadoutUnknown && (segment.Loadout.Inventory != nil || segment.Loadout.Equipment != nil || segment.Loadout.EmptySlots != nil) {
				t.Fatalf("unknown loadout claimed empty requirements: %#v", segment.Loadout)
			}
		})
	}
}

func TestUnknownLoadoutCannotClaimEmptyInventory(t *testing.T) {
	_, err := LoadGuideFile(filepath.Join("testdata", "loadouts", "invalid-unknown-empty.json"))
	if err == nil {
		t.Fatal("unknown loadout with an empty inventory array passed schema validation")
	}
}
