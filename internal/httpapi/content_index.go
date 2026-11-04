package httpapi

import (
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
)

// occurrence is where a route completes (or starts) an entity.
type occurrence struct {
	Guide    string `json:"guide"`
	Step     string `json:"step"`
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Section  string `json:"section"`
	Position int    `json:"position,omitempty"`
	Total    int    `json:"total"`
	Key      string `json:"key"`
	Role     string `json:"role"`
}

type guideStep struct{ guide, step string }

// contentIndex answers the cross-route questions of a step: its position in
// the main route and where the other routes complete the same entities. It
// never merges routes; it only points at them (CLAUDE.md rule 7).
type contentIndex struct {
	position map[guideStep]int
	total    map[string]int
	// byEntity lists, per entity ref (or "qp"), the first completing step of
	// each guide in route order.
	byEntity map[string][]occurrence
	// touched lists, per entity, every objective occurrence of every role in
	// route order (the Mundo's "onde cada rota faz isso"); entities keeps the
	// first-seen order.
	touched  map[string][]occurrence
	entities []string
}

func buildContentIndex(cnt *content.Content) *contentIndex {
	idx := &contentIndex{position: map[guideStep]int{}, total: map[string]int{}, byEntity: map[string][]occurrence{}, touched: map[string][]occurrence{}}
	if cnt == nil {
		return idx
	}
	for _, g := range cnt.Guides {
		n := 0
		for _, sec := range g.Sections {
			if sec.IsTrack() || (g.DefaultVariant != "" && sec.VariantID != "" && sec.VariantID != g.DefaultVariant) {
				continue
			}
			for _, st := range sec.Steps {
				if _, dup := idx.position[guideStep{g.ID, st.ID}]; dup {
					continue
				}
				n++
				idx.position[guideStep{g.ID, st.ID}] = n
			}
		}
		idx.total[g.ID] = n
	}
	for _, g := range cnt.Guides {
		seen := map[string]bool{}
		for _, st := range g.Steps() {
			for _, o := range st.Objectives {
				entity := entityOf(o.Key)
				if _, known := idx.touched[entity]; !known {
					idx.entities = append(idx.entities, entity)
				}
				idx.touched[entity] = append(idx.touched[entity], occurrence{
					Guide: g.ID, Step: st.ID, Ref: authorRef(st), Title: coreprogress.StepTitle(st.Text), Section: st.SectionID,
					Position: idx.position[guideStep{g.ID, st.ID}], Total: idx.total[g.ID],
					Key: o.Key.String(), Role: o.Role,
				})
				if !o.Completing() {
					continue
				}
				if seen[entity] {
					continue
				}
				seen[entity] = true
				idx.byEntity[entity] = append(idx.byEntity[entity], occurrence{
					Guide: g.ID, Step: st.ID, Ref: authorRef(st), Title: coreprogress.StepTitle(st.Text), Section: st.SectionID,
					Position: idx.position[guideStep{g.ID, st.ID}], Total: idx.total[g.ID],
					Key: o.Key.String(), Role: o.Role,
				})
			}
		}
	}
	return idx
}

// also returns, per completing objective of the step, where each other guide
// completes the same entity.
func (idx *contentIndex) also(st *content.Step) map[string][]occurrence {
	out := map[string][]occurrence{}
	for _, o := range st.Objectives {
		if !o.Completing() {
			continue
		}
		for _, occ := range idx.byEntity[entityOf(o.Key)] {
			if occ.Guide != st.GuideID {
				out[o.Key.String()] = append(out[o.Key.String()], occ)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func entityOf(k entities.ObjectiveKey) string {
	if k.IsQuestPoints() {
		return entities.QuestPoints
	}
	return k.Ref.String()
}

// authorRef is the step reference the author uses ("1.1.75", "CH1.6").
func authorRef(st *content.Step) string { return coreprogress.AuthorRef(st) }

// stepNames returns the display name of every canonical ref the step uses:
// objective refs, its location and its item requirements.
func stepNames(st *content.Step, catalog *entities.Registry, into map[string]string) {
	if catalog == nil {
		return
	}
	add := func(ref entities.Ref) {
		if ref.IsZero() {
			return
		}
		key := ref.String()
		if _, done := into[key]; done {
			return
		}
		if name, ok := catalog.Display(ref); ok {
			into[key] = name
		}
	}
	for _, o := range st.Objectives {
		add(o.Key.Ref)
	}
	if st.LocationRef != nil {
		add(*st.LocationRef)
	}
	for _, r := range st.Requirements {
		addRequirement(r, add)
	}
}

func addRequirement(r content.ItemRequirement, add func(entities.Ref)) {
	if r.ItemID != nil {
		add(entities.ItemRef(*r.ItemID))
	}
	for _, id := range r.AnyOf {
		add(entities.ItemRef(id))
	}
}

// sectionNames adds the names of the section's segment loadouts.
func sectionNames(sec *content.Section, catalog *entities.Registry, into map[string]string) {
	if catalog == nil {
		return
	}
	add := func(ref entities.Ref) {
		if _, done := into[ref.String()]; done {
			return
		}
		if name, ok := catalog.Display(ref); ok {
			into[ref.String()] = name
		}
	}
	for _, seg := range sec.Segments {
		if seg.Loadout == nil {
			continue
		}
		for _, r := range append(append([]content.ItemRequirement(nil), seg.Loadout.Inventory...), seg.Loadout.Equipment...) {
			addRequirement(r, add)
		}
	}
	for _, st := range sec.Steps {
		stepNames(st, catalog, into)
	}
}
