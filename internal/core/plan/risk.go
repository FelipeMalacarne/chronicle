package plan

import (
	"slices"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// The wilderness, as Microbot's web walker bounds it
// (shortestpath/pathfinder/PathfinderConfig: WILDERNESS_ABOVE_GROUND and
// WILDERNESS_UNDERGROUND), on any plane: a travel task whose destination
// touches it counts as a wilderness risk (docs/10, E9), and the executor
// stops a walk that enters it unasked. The plugin keeps the same bounds
// (TaskSpec.inWilderness), pinned by tests on both sides.
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

// AreaInWilderness reports whether any tile of the area's box is in the
// wilderness: the box bounds the polygon, so this errs on the side of risk.
func AreaInWilderness(a entities.Area) bool {
	overlaps := func(minX, maxX, minY, maxY int) bool {
		return a.MaxX >= minX && a.MinX <= maxX && a.MaxY >= minY && a.MinY <= maxY
	}
	return overlaps(WildMinX, WildMaxX, WildMinY, WildMaxY) || overlaps(WildUnderMinX, WildUnderMaxX, WildUnderMinY, WildUnderMaxY)
}

// Risks returns the risk kinds of one task: its recipe variant's (nil for a
// step's or a plan's own task) and, for a travel whose destination touches
// the wilderness, wilderness. Sorted, without repeats.
func Risks(t content.Task, v *content.RecipeVariant, reg *entities.Registry) []string {
	out := []string{}
	if v != nil {
		for _, r := range v.Risks {
			out = append(out, r.Kind)
		}
	}
	if t.Type == content.TaskTravel && t.Location != nil && reg != nil {
		if a, ok := reg.LocationArea(t.Location.ID); ok && AreaInWilderness(a) {
			out = append(out, content.RiskWilderness)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

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

// Executable says whether the phase 3 executor runs a task (docs/10 section
// 13: travel, bank and buy), and why not: the other types, a travel with a
// via (the executor has no teleport primitive yet: the web walker would pick
// its own way), a destination without an area, a bank task for an Ultimate
// Ironman (no bank) or without items to bank for.
func Executable(t content.Task, bankItems bool, mode string, reg *entities.Registry) (bool, string) {
	switch t.Type {
	case content.TaskTravel:
		if t.Via != nil {
			return false, "travel com via ainda não é executado (o executor não tem teleporte): faça à mão"
		}
		if t.Location == nil || reg == nil {
			return false, "a tarefa não tem local"
		}
		if _, ok := reg.LocationArea(t.Location.ID); !ok {
			return false, "o local não tem área no mapa"
		}
		return true, ""
	case content.TaskBank:
		if mode == content.ModeUIM {
			return false, "Ultimate Ironman não usa o banco"
		}
		if !bankItems {
			return false, "o trecho não tem setup para o banco montar"
		}
		return true, ""
	case content.TaskBuy:
		if len(ShopKeepers(t, reg)) == 0 {
			return false, "a loja não tem lojista com id de jogo no catálogo"
		}
		return true, ""
	}
	return false, "o executor da fase 3 só roda travel, bank e buy"
}

// ShopKeepers returns the game ids of the NPCs that own the shop of a buy
// task (every version of every owner, as the catalog lists them), in order,
// without repeats: what the executor looks for in game.
func ShopKeepers(t content.Task, reg *entities.Registry) []int {
	if t.Shop == nil || reg == nil {
		return nil
	}
	shop, ok := reg.Shop(t.Shop.ID)
	if !ok {
		return nil
	}
	var out []int
	for _, owner := range shop.Owners {
		npc, ok := reg.NPC(owner)
		if !ok {
			continue
		}
		for _, id := range npc.GameIDs {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}
