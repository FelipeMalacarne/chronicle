package plan

import (
	"slices"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
)

// AreaInWilderness reports whether any tile of a location's area is in
// the wilderness (execspec, the web walker's bounds): the box bounds the
// polygon, so this errs on the side of risk.
func AreaInWilderness(a entities.Area) bool {
	return execspec.BoxInWilderness(a.MinX, a.MinY, a.MaxX, a.MaxY)
}

// Risks returns the risk kinds of one task: its recipe variant's (nil for a
// step's or a plan's own task); for a travel, or a train at a location,
// whose area touches the wilderness, wilderness; for a train that fights
// (TrainFights), combat. Sorted, without repeats.
func Risks(t content.Task, v *content.RecipeVariant, reg *entities.Registry) []string {
	out := []string{}
	if v != nil {
		for _, r := range v.Risks {
			out = append(out, r.Kind)
		}
	}
	if (t.Type == content.TaskTravel || t.Type == content.TaskTrain) && t.Location != nil && reg != nil {
		if a, ok := reg.LocationArea(t.Location.ID); ok && AreaInWilderness(a) {
			out = append(out, content.RiskWilderness)
		}
	}
	if TrainFights(t, reg) {
		out = append(out, content.RiskCombat)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// TrainFights reports whether a train task fights: its NPC is a monster
// (the wiki's monster infobox), or its skill is trained only by fighting
// (execspec.CombatSkills), whatever the NPC.
func TrainFights(t content.Task, reg *entities.Registry) bool {
	if t.Type != content.TaskTrain {
		return false
	}
	if t.Skill != nil && slices.Contains(execspec.CombatSkills, t.Skill.ID) {
		return true
	}
	if t.NPC != nil && reg != nil {
		if n, ok := reg.NPC(t.NPC.ID); ok && n.Monster {
			return true
		}
	}
	return false
}

// NeedsSupervision is the E9 policy (docs/10 section 14): in HCIM nothing
// with a death risk runs without the owner watching.
func NeedsSupervision(mode string, risks []string) bool {
	return execspec.NeedsSupervision(mode, risks)
}

// Executable says whether the executor runs a task (docs/10 section 13:
// travel, bank and buy on Chronicle Link, phase 3; train delegated to a
// plugin, phase 4), and why not: the other types, a travel with a via (the
// executor has no teleport primitive yet: the web walker would pick its own
// way), a destination without an area, a bank task for an Ultimate Ironman
// (no bank) or without items to bank for, a train that names no plugin.
// Whether the plugin is installed only the account's client knows: the
// server asks it before the run, the agent again when it starts.
func Executable(t content.Task, bankItems bool, mode string, reg *entities.Registry) (bool, string) {
	switch t.Type {
	case content.TaskTrain:
		if t.Plugin == "" {
			return false, "o treino não diz qual plugin o faz (plugin na tarefa): faça à mão"
		}
		if t.Skill == nil {
			return false, "a tarefa não tem skill"
		}
		return true, ""
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
	return false, "o executor só roda travel, bank, buy e train"
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
