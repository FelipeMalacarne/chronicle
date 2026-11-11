package progress

import (
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

func TestNearby(t *testing.T) {
	reg := registry(t)
	alk, ok := reg.LocationMap("al_kharid")
	if !ok {
		t.Fatal("al_kharid has no map point")
	}
	loc := func(id string) *entities.Ref { r := entities.Ref{Kind: entities.KindLocation, ID: id}; return &r }
	g := &content.Guide{ID: "g", Sections: []*content.Section{{ID: "s", Steps: []*content.Step{
		{ID: "a", LocationRef: loc("al_kharid")},
		{ID: "b", LocationRef: loc("al_kharid")},
		{ID: "c", LocationRef: loc("al_kharid")},
		{ID: "d", LocationRef: loc("ape_atoll")},
		{ID: "e"},
		{ID: "f", LocationRef: loc("wilderness")},
		{ID: "h", LocationRef: loc("al_kharid")},
	}}}}
	g.Index()
	b := Board{Steps: []StepState{
		{ID: "a", Ref: "1.1", Title: "Open, near", Status: Pending, Main: true, Visible: true},
		{ID: "b", Ref: "1.2", Title: "Done", Status: AutoDone, Main: true, Visible: true},
		{ID: "c", Ref: "1.3", Title: "Other layer", Status: Pending, Main: true},
		{ID: "d", Ref: "1.4", Title: "Far away", Status: Pending, Main: true, Visible: true},
		{ID: "e", Ref: "1.5", Title: "No location", Status: Pending, Main: true, Visible: true},
		{ID: "f", Ref: "1.6", Title: "A region, no point", Status: Pending, Main: true, Visible: true},
		{ID: "h", Ref: "1.7", Title: "Pinned open", Status: PinnedOpen, Main: true, Visible: true},
	}}
	at := Point{X: alk.X + 10, Y: alk.Y - 20}
	got := Nearby(b, g, reg, at, NearRadius, 5)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "h" || got[0].Distance != 20 || got[0].Name != "Al Kharid" || got[0].Location != "location:al_kharid" {
		t.Fatalf("near = %+v", got)
	}
	if got := Nearby(b, g, reg, at, NearRadius, 1); len(got) != 1 {
		t.Fatalf("limit = %+v", got)
	}
	if got := Nearby(b, g, reg, Point{X: 3222, Y: 3218 + 200}, NearRadius, 5); len(got) != 0 {
		t.Fatalf("far from everything = %+v", got)
	}
}
