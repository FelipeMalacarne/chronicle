package progress

import (
	"slices"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// TestBoostableRequirementsEvaluatedNotBlocking: a boostable catalog
// requirement (25 Agility for The Grand Tree) gets a verdict, so the entity
// panel can show it, but never blocks the step (docs/09 section 3.5).
func TestBoostableRequirementsEvaluatedNotBlocking(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	q, ok := reg.Quest("the_grand_tree")
	if !ok {
		t.Skip("no The Grand Tree in the catalog")
	}
	var boostable string
	for _, req := range q.Requires {
		if req.Boostable && !req.DuringQuest {
			boostable = req.Key.String()
		}
	}
	if boostable == "" {
		t.Skip("The Grand Tree has no boostable requirement in the catalog")
	}
	key, err := entities.ParseObjectiveKey("quest:the_grand_tree@finished")
	if err != nil {
		t.Fatal(err)
	}
	st := &content.Step{ID: "s1", Objectives: []content.Objective{{Key: key, Role: content.RoleCompletes}}}
	if slices.Contains(CatalogRequires(st, reg), boostable) {
		t.Fatalf("%s blocks the step", boostable)
	}
	if !slices.Contains(catalogRequires(st, reg, true), boostable) {
		t.Fatalf("%s is not evaluated", boostable)
	}
}
