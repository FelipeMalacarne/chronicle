package entities

import "testing"

func TestXPTable(t *testing.T) {
	for _, c := range []struct{ level, xp int }{
		{1, 0}, {2, 83}, {10, 1154}, {27, 9730}, {29, 12031}, {30, 13363}, {92, 6517253}, {99, 13034431},
	} {
		if got := XPForLevel(c.level); got != c.xp {
			t.Errorf("XPForLevel(%d) = %d, want %d", c.level, got, c.xp)
		}
	}
	for _, c := range []struct{ xp, level int }{
		{0, 1}, {82, 1}, {83, 2}, {12725, 29}, {13034430, 98}, {13034431, 99}, {200000000, 99},
	} {
		if got := LevelForXP(c.xp); got != c.level {
			t.Errorf("LevelForXP(%d) = %d, want %d", c.xp, got, c.level)
		}
	}
	if StartingXP("hitpoints") != 1154 || StartingXP("smithing") != 0 {
		t.Error("starting xp: hitpoints is level 10, the rest level 1")
	}
}
