package derive

import (
	"reflect"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
)

func TestEnrichGuideDerivesOnlyExplicitBankLegs(t *testing.T) {
	guide := &content.Guide{
		SchemaVersion: 1,
		ID:            "test",
		Title:         "Test",
		Sections: []*content.Section{{
			ID: "one", Slug: "one", Title: "One",
			Steps: []*content.Step{
				{ID: "travel", Text: "Travel to Falador."},
				{ID: "setup", Text: "Bank", Items: []content.Item{{Name: "GP"}, {Name: "18 cakes"}, {Name: "Gloves (equip)"}}},
				{ID: "quest", Text: "Complete the quest."},
				{ID: "reset", Text: "Deposit all your items and withdraw a rope."},
			},
		}},
	}

	stats := EnrichGuide(guide)
	if stats.Variants != 1 || stats.Segments != 3 || stats.BankLegs != 2 || stats.PartialLoadouts != 1 || stats.UnknownLoadouts != 2 {
		t.Fatalf("stats = %+v", stats)
	}
	if guide.DefaultVariant != "main" || len(guide.Variants) != 1 || len(guide.Sections) != 1 {
		t.Fatalf("variant hierarchy not built: %+v", guide)
	}
	segments := guide.Sections[0].Segments
	if segments[0].Kind != content.SegmentSequence || segments[0].Loadout.Status != content.LoadoutUnknown {
		t.Fatalf("initial sequence = %+v", segments[0])
	}
	setup := segments[1]
	if setup.Kind != content.SegmentBankLeg || len(setup.Steps) != 2 || setup.Steps[0].ID != "setup" {
		t.Fatalf("setup segment = %+v", setup)
	}
	if setup.Loadout.Status != content.LoadoutPartial || len(setup.Loadout.Inventory) != 2 || len(setup.Loadout.Equipment) != 1 {
		t.Fatalf("setup loadout = %+v", setup.Loadout)
	}
	if got := setup.Loadout.Inventory[1]; got.Name != "cakes" || got.Qty == nil || *got.Qty != 18 || got.SourceText != "18 cakes" {
		t.Fatalf("quantity item = %+v", got)
	}
	if got := setup.Loadout.Equipment[0]; got.Name != "Gloves" || got.SourceText != "Gloves (equip)" {
		t.Fatalf("equipment item = %+v", got)
	}
	reset := segments[2]
	if reset.Loadout.Status != content.LoadoutUnknown || len(reset.BankActions) != 2 {
		t.Fatalf("reset segment = %+v", reset)
	}
	if reset.BankActions[0].Type != content.BankDepositAll || reset.BankActions[1].Type != content.BankWithdraw {
		t.Fatalf("reset actions = %+v", reset.BankActions)
	}
	if setup.Steps[0].VariantID != "main" || setup.Steps[0].SegmentID != setup.ID {
		t.Fatalf("step backrefs = variant %q segment %q", setup.Steps[0].VariantID, setup.Steps[0].SegmentID)
	}

	ids := []string{segments[0].ID, segments[1].ID, segments[2].ID}
	secondStats := EnrichGuide(guide)
	gotIDs := []string{guide.Sections[0].Segments[0].ID, guide.Sections[0].Segments[1].ID, guide.Sections[0].Segments[2].ID}
	if secondStats != stats || !reflect.DeepEqual(gotIDs, ids) {
		t.Fatalf("second enrichment changed output: stats=%+v ids=%v", secondStats, gotIDs)
	}
}

func TestExplicitEmptySlotRequirementMakesPartialLoadout(t *testing.T) {
	guide := &content.Guide{SchemaVersion: 1, ID: "test", Title: "Test", Sections: []*content.Section{{
		ID: "one", Slug: "one", Title: "One", Steps: []*content.Step{{ID: "bank", Text: "Bank and leave at least 3 empty inventory slots."}},
	}}}
	EnrichGuide(guide)
	loadout := guide.Sections[0].Segments[0].Loadout
	if loadout.Status != content.LoadoutPartial || loadout.EmptySlots == nil || *loadout.EmptySlots != 3 {
		t.Fatalf("loadout = %+v", loadout)
	}
}

func TestBankSegmentIDDoesNotDependOnArrayPosition(t *testing.T) {
	makeGuide := func(prefix ...*content.Step) *content.Guide {
		steps := append([]*content.Step(nil), prefix...)
		steps = append(steps,
			&content.Step{ID: "bank", Text: "Bank", Items: []content.Item{{Name: "Rope"}}},
			&content.Step{ID: "leave", Text: "Leave for the quest."},
		)
		return &content.Guide{SchemaVersion: 1, ID: "test", Title: "Test", Sections: []*content.Section{{
			ID: "one", Slug: "one", Title: "One", Steps: steps,
		}}}
	}

	first := makeGuide(&content.Step{ID: "travel", Text: "Travel to Falador."})
	second := makeGuide(
		&content.Step{ID: "new-intro", Text: "New introductory instruction."},
		&content.Step{ID: "travel", Text: "Travel to Falador."},
	)
	EnrichGuide(first)
	EnrichGuide(second)
	firstBank := first.Sections[0].Segments[1]
	secondBank := second.Sections[0].Segments[1]
	if firstBank.Steps[0].ID != "bank" || secondBank.Steps[0].ID != "bank" || firstBank.ID != secondBank.ID {
		t.Fatalf("bank segment ids changed with unrelated insertion: %q vs %q", firstBank.ID, secondBank.ID)
	}
}
