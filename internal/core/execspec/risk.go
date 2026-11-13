package execspec

import (
	"slices"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
)

// The wilderness, as Microbot's web walker bounds it
// (shortestpath/pathfinder/PathfinderConfig: WILDERNESS_ABOVE_GROUND and
// WILDERNESS_UNDERGROUND), on any plane. A travel whose destination touches
// it counts as a wilderness risk (docs/10, E9), and the executor stops a
// walk that enters it unasked. The plugin keeps the same bounds
// (TaskSpec.inWilderness), pinned by the wire fixture on both sides.
const (
	WildMinX      = 2944
	WildMaxX      = 3391
	WildMinY      = 3525
	WildMaxY      = 3972
	WildUnderMinX = 2944
	WildUnderMaxX = 3461
	WildUnderMinY = 9918
	WildUnderMaxY = 10375
)

// InWilderness reports whether the tile is in the wilderness.
func InWilderness(x, y int) bool {
	return (x >= WildMinX && x <= WildMaxX && y >= WildMinY && y <= WildMaxY) ||
		(x >= WildUnderMinX && x <= WildUnderMaxX && y >= WildUnderMinY && y <= WildUnderMaxY)
}

// BoxInWilderness reports whether any tile of the box is in the
// wilderness; for an area, its box bounds the polygon, so this errs on the
// side of risk.
func BoxInWilderness(minX, minY, maxX, maxY int) bool {
	overlaps := func(x0, x1, y0, y1 int) bool { return maxX >= x0 && minX <= x1 && maxY >= y0 && minY <= y1 }
	return overlaps(WildMinX, WildMaxX, WildMinY, WildMaxY) || overlaps(WildUnderMinX, WildUnderMaxX, WildUnderMinY, WildUnderMaxY)
}

// AreaInWilderness reports whether the area's box touches the wilderness.
func AreaInWilderness(a Area) bool { return BoxInWilderness(a.MinX, a.MinY, a.MaxX, a.MaxY) }

// KnownRisk reports whether kind is one of the closed list (E9).
func KnownRisk(kind string) bool { return slices.Contains(content.RiskKinds, kind) }

// Deadly reports whether one of the risk kinds can kill the character
// (combat, wilderness, hazard; item loss cannot).
func Deadly(risks []string) bool {
	return slices.ContainsFunc(risks, func(k string) bool { return content.Risk{Kind: k}.Deadly() })
}

// NeedsSupervision is the E9 policy (docs/10 section 14): in HCIM nothing
// with a death risk runs without the owner watching.
func NeedsSupervision(mode string, risks []string) bool {
	return mode == content.ModeHCIM && Deadly(risks)
}
