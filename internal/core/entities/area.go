package entities

import "fmt"

// Area is where a location is, in game tiles: a box on one plane and, when
// the wiki draws one, the polygon inside it. A travel task ends when the
// character stands inside its location's area (docs/10 section 7).
//
// From names the rule that produced it from the location's wiki infobox map
// (internal/core/entities/catalogbuild):
//
//	polygon    the map's polygon (mtype=polygon or focusarea); the box bounds it
//	rectangle  the map's rectangle: rectX by rectY tiles centered on its point
//	square     the map's square: r tiles around its point
//	point      the map has only a point: AreaRadius tiles around it
type Area struct {
	Plane   int      `json:"plane"`
	MinX    int      `json:"min_x"`
	MinY    int      `json:"min_y"`
	MaxX    int      `json:"max_x"`
	MaxY    int      `json:"max_y"`
	Polygon [][2]int `json:"polygon,omitempty"`
	From    string   `json:"from"`
}

// Area rules.
const (
	AreaPolygon   = "polygon"
	AreaRectangle = "rectangle"
	AreaSquare    = "square"
	AreaPoint     = "point"
)

// AreaRadius is the half size of the area of a location whose wiki map has
// only a point: a 21 by 21 tile box, about what the game shows around the
// character. Arriving there means arriving near the point the wiki marks,
// which errs on the side of walking further rather than stopping early.
const AreaRadius = 10

// PointArea is the area of a map point without a drawn shape.
func PointArea(x, y, plane int) Area {
	return Area{Plane: plane, MinX: x - AreaRadius, MinY: y - AreaRadius, MaxX: x + AreaRadius, MaxY: y + AreaRadius, From: AreaPoint}
}

// Contains reports whether the tile (x, y, plane) is inside the area: in the
// box and, when there is a polygon, in the polygon or on its edge.
func (a Area) Contains(x, y, plane int) bool {
	if plane != a.Plane || x < a.MinX || x > a.MaxX || y < a.MinY || y > a.MaxY {
		return false
	}
	if len(a.Polygon) < 3 {
		return true
	}
	inside := false
	n := len(a.Polygon)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := a.Polygon[i][0], a.Polygon[i][1]
		xj, yj := a.Polygon[j][0], a.Polygon[j][1]
		if onSegment(x, y, xi, yi, xj, yj) {
			return true
		}
		if (yi > y) != (yj > y) {
			// x of the edge at height y, compared without division.
			lhs := (x - xi) * (yj - yi)
			rhs := (xj - xi) * (y - yi)
			if (yj > yi && lhs < rhs) || (yj < yi && lhs > rhs) {
				inside = !inside
			}
		}
	}
	return inside
}

// onSegment reports whether (x, y) lies on the segment (x1, y1)-(x2, y2).
func onSegment(x, y, x1, y1, x2, y2 int) bool {
	if (x-x1)*(y2-y1) != (x2-x1)*(y-y1) {
		return false
	}
	return min(x1, x2) <= x && x <= max(x1, x2) && min(y1, y2) <= y && y <= max(y1, y2)
}

// check reports why the area is malformed, or "".
func (a Area) check() string {
	switch a.From {
	case AreaPolygon, AreaRectangle, AreaSquare, AreaPoint:
	default:
		return fmt.Sprintf("unknown rule %q", a.From)
	}
	if a.MinX > a.MaxX || a.MinY > a.MaxY {
		return "empty box"
	}
	if a.From == AreaPolygon {
		if len(a.Polygon) < 3 {
			return "a polygon needs three points"
		}
		for _, p := range a.Polygon {
			if p[0] < a.MinX || p[0] > a.MaxX || p[1] < a.MinY || p[1] > a.MaxY {
				return fmt.Sprintf("polygon point %v is outside the box", p)
			}
		}
	} else if len(a.Polygon) > 0 {
		return "only a polygon area has points"
	}
	return ""
}
