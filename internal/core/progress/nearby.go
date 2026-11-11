package progress

import (
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// The steps near the character (item 16 of docs/04 section 9): the open steps
// of the route whose location's wiki map point is within reach of the
// position the plugin last sent. A location without a point (a region, an
// instance) never counts as near.

// NearRadius is how far, in tiles (Chebyshev), a step's location counts as
// near: about a town's size.
const NearRadius = 64

// Point is a tile.
type Point struct {
	X, Y, Plane int
}

// NearStep is one open step near the character.
type NearStep struct {
	ID       string `json:"id"`
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Section  string `json:"section_id"`
	Location string `json:"location"`
	Name     string `json:"location_name"`
	Distance int    `json:"distance"`
}

// Nearby lists the open, visible steps of the main route whose location is
// within radius of at, in route order, at most limit of them. Planes count
// apart only for a location placed above ground: the map point of a town is
// on plane 0 and the character climbing a tower is still in town.
func Nearby(b Board, g *content.Guide, reg *entities.Registry, at Point, radius, limit int) []NearStep {
	if g == nil || reg == nil {
		return nil
	}
	out := []NearStep{}
	for _, s := range b.Steps {
		if !s.Visible || !s.Main || !open(s.Status) {
			continue
		}
		st, ok := g.Step(s.ID)
		if !ok || st.LocationRef == nil || st.LocationRef.Kind != entities.KindLocation {
			continue
		}
		m, ok := reg.LocationMap(st.LocationRef.ID)
		if !ok || (m.Plane > 0 && m.Plane != at.Plane) {
			continue
		}
		d := max(abs(m.X-at.X), abs(m.Y-at.Y))
		if d > radius {
			continue
		}
		name := st.LocationRef.ID
		if l, ok := reg.Location(st.LocationRef.ID); ok {
			name = l.Name
		}
		out = append(out, NearStep{ID: s.ID, Ref: s.Ref, Title: s.Title, Section: s.SectionID, Location: st.LocationRef.String(), Name: name, Distance: d})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func open(s Status) bool { return s == Pending || s == PinnedOpen }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
