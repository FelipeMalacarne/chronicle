package plan

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/execspec"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// ErrNotATask: the key names no task of the plan.
var ErrNotATask = errors.New("plan: the key names no task of the plan")

// Resolve turns the task a key names into what the phase 3 executor runs
// (docs/10 section 10: the server decides what): the destination's area
// and a target tile inside it, the bank items as candidate game ids with
// quantities, the shop's keepers and how many to buy now. It refuses what
// the executor does not run, a task already met (nothing to do) and what
// the catalog cannot resolve; it never guesses an id.
func Resolve(in Input, key string) (execspec.Task, TaskView, error) {
	s, ok := in.locate(key)
	if !ok {
		return execspec.Task{}, TaskView{}, ErrNotATask
	}
	item, _, _ := SplitTaskKey(key)
	v := in.view(s.t, item, s.index, s.sc)
	if !v.Executable {
		return execspec.Task{}, v, errors.New(v.NotExecutable)
	}
	if v.State == rules.Met {
		return execspec.Task{}, v, errors.New("a tarefa já está cumprida: nada a fazer")
	}
	var (
		out execspec.Task
		err error
	)
	switch s.t.Type {
	case content.TaskTravel:
		out, err = in.resolveTravel(s.t)
	case content.TaskBank:
		reqs, equip := s.t.Items, []content.ItemRequirement(nil)
		if len(reqs) == 0 && s.sc.segment != nil && s.sc.segment.Loadout != nil {
			reqs, equip = s.sc.segment.Loadout.Inventory, s.sc.segment.Loadout.Equipment
		}
		out, err = in.resolveBank(reqs, equip)
	case content.TaskBuy:
		out, err = in.resolveBuy(s.t)
	case content.TaskTrain:
		out, err = in.resolveTrain(s.t)
	default:
		err = fmt.Errorf("o executor não roda %s", s.t.Type)
	}
	if err != nil {
		return execspec.Task{}, v, err
	}
	if err := out.Check(); err != nil {
		return execspec.Task{}, v, fmt.Errorf("a tarefa resolvida não passa na checagem: %w", err)
	}
	return out, v, nil
}

func (in Input) resolveTravel(t content.Task) (execspec.Task, error) {
	area, ok := in.Registry.LocationArea(t.Location.ID)
	if !ok {
		return execspec.Task{}, errors.New("o local não tem área no mapa")
	}
	a := execspec.Area{Plane: area.Plane, MinX: area.MinX, MinY: area.MinY, MaxX: area.MaxX, MaxY: area.MaxY, Polygon: area.Polygon}
	m, _ := in.Registry.LocationMap(t.Location.ID)
	target, ok := Target(a, execspec.Tile{X: m.X, Y: m.Y, Plane: m.Plane})
	if !ok {
		return execspec.Task{}, errors.New("a área do local não tem um tile para onde andar")
	}
	return execspec.Task{Type: execspec.TypeTravel, Travel: &execspec.Travel{
		Location: t.Location.String(), Name: in.name(*t.Location), Target: target, Area: a,
		Wilderness: execspec.AreaInWilderness(a),
	}}, nil
}

// Target is the tile a walk to an area heads to: the wiki map's point when
// it is inside, else the box's center, else the inside tile nearest to it.
func Target(a execspec.Area, point execspec.Tile) (execspec.Tile, bool) {
	if a.Contains(point.X, point.Y, point.Plane) {
		return point, true
	}
	cx, cy := (a.MinX+a.MaxX)/2, (a.MinY+a.MaxY)/2
	best, found, bestD := execspec.Tile{}, false, 0
	for x := a.MinX; x <= a.MaxX; x++ {
		for y := a.MinY; y <= a.MaxY; y++ {
			if !a.Contains(x, y, a.Plane) {
				continue
			}
			d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			if !found || d < bestD {
				best, found, bestD = execspec.Tile{X: x, Y: y, Plane: a.Plane}, true, d
			}
		}
	}
	return best, found
}

func (in Input) resolveBank(inventory, equipment []content.ItemRequirement) (execspec.Task, error) {
	var items []execspec.BankItem
	add := func(reqs []content.ItemRequirement, equip bool) error {
		for _, r := range reqs {
			ids, err := in.candidates(r)
			if err != nil {
				return err
			}
			qty := 1
			if r.Qty != nil && *r.Qty > 0 {
				qty = *r.Qty
			}
			name := strings.TrimSpace(r.Name)
			if name == "" {
				name = in.name(entities.ItemRef(ids[0]))
			}
			items = append(items, execspec.BankItem{
				Name: name, IDs: ids, Qty: qty, Noted: r.Noted && !equip, Equip: equip, Optional: r.Optional,
			})
		}
		return nil
	}
	if err := add(inventory, false); err != nil {
		return execspec.Task{}, err
	}
	if err := add(equipment, true); err != nil {
		return execspec.Task{}, err
	}
	if len(items) == 0 {
		return execspec.Task{}, errors.New("o banco não tem itens para montar")
	}
	return execspec.Task{Type: execspec.TypeBank, Bank: &execspec.Bank{Items: items}}, nil
}

// candidates are the unnoted ids a requirement accepts, preferred first:
// its id, its any_of, or its family (as the catalog lists it).
func (in Input) candidates(r content.ItemRequirement) ([]int, error) {
	var raw []int
	switch r.Variant() {
	case content.RequireExact:
		raw = []int{*r.ItemID}
	case content.RequireAnyOf:
		raw = r.AnyOf
	case content.RequireFamily:
		ids, ok := in.Registry.Family(r.Family)
		if !ok || len(ids) == 0 {
			return nil, fmt.Errorf("a família %q não está no catálogo", r.Family)
		}
		raw = ids
	default:
		return nil, fmt.Errorf("o item %q não está resolvido para um id", r.Name)
	}
	var out []int
	for _, id := range raw {
		if f, ok := in.Registry.Item(id); ok && f.Noted {
			id = f.BaseID // the bank holds the unnoted form
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if len(out) > execspec.MaxIDs {
		out = out[:execspec.MaxIDs]
	}
	return out, nil
}

func (in Input) resolveBuy(t content.Task) (execspec.Task, error) {
	npcs := ShopKeepers(t, in.Registry)
	if len(npcs) == 0 {
		return execspec.Task{}, errors.New("a loja não tem lojista com id de jogo no catálogo")
	}
	if len(npcs) > execspec.MaxNPCs {
		npcs = npcs[:execspec.MaxNPCs]
	}
	id, ok := t.Item.Int()
	if !ok {
		return execspec.Task{}, errors.New("o item da compra não é um id")
	}
	// Buy what is missing: the holdings the server reads (live and
	// restored containers) count, so a stale bank buys less, never more;
	// the termination (item:<id>x<qty>) is what proves it.
	o := rules.Owns(in.Holdings, in.Registry, id, int64(t.Qty), nil)
	qty := t.Qty - int(min(o.Held, int64(t.Qty)))
	if qty < 1 {
		return execspec.Task{}, errors.New("a conta já tem a quantidade; falta o ingest provar")
	}
	return execspec.Task{Type: execspec.TypeBuy, Buy: &execspec.Buy{
		Shop: t.Shop.String(), ShopName: in.name(*t.Shop), NPCs: npcs, Item: id, ItemName: in.name(*t.Item), Qty: qty,
	}}, nil
}

// resolveTrain is the delegation of a train (docs/10 phase 4): the skill,
// the level its termination names, the plugin and its settings as the task
// gives them, and whether the method fights.
func (in Input) resolveTrain(t content.Task) (execspec.Task, error) {
	u, ok := t.Ends()
	if !ok || u.Objective == nil || t.Skill == nil || u.Objective.Ref != *t.Skill || u.Objective.Op != entities.OpAtLeast {
		return execspec.Task{}, errors.New("o treino não termina num nível da própria skill")
	}
	return execspec.Task{Type: execspec.TypeTrain, Train: &execspec.Train{
		Skill: t.Skill.String(), SkillName: in.name(*t.Skill), Level: u.Objective.N,
		Plugin: t.Plugin, Config: slices.Clone(t.Config), Combat: TrainFights(t, in.Registry),
	}}, nil
}

// name is a ref's display name, or its id.
func (in Input) name(r entities.Ref) string {
	if in.Registry != nil {
		if d, ok := in.Registry.Display(r); ok {
			return strings.TrimSuffix(strings.TrimSpace(d), ".")
		}
	}
	return r.ID
}
