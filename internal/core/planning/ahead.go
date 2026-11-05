package planning

import (
	"slices"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/rules"
)

// Ahead is what the remaining steps of a leg ask for, derived when the guide
// does not list the leg's setup. It is a preview, never a loadout: it has no
// status and never makes a segment ready.
type Ahead struct {
	FromStepID  string `json:"from_step_id"`
	UntilStepID string `json:"until_step_id"`
	// Steps is how many steps follow FromStepID up to UntilStepID.
	Steps    int          `json:"steps"`
	Groups   []AheadGroup `json:"groups"`
	Obtained []Obtained   `json:"obtained"`
}

// AheadGroup is the step where items are first used. Offset counts the steps
// after FromStepID (0 is FromStepID itself).
type AheadGroup struct {
	StepID string      `json:"step_id"`
	Text   string      `json:"text"`
	Offset int         `json:"offset"`
	Items  []AheadItem `json:"items"`
}

// AheadItem is a first-use requirement evaluated like a loadout item, plus the
// later steps of the window that list it again.
type AheadItem struct {
	Item
	AlsoAt []string `json:"also_at,omitempty"`
}

// Obtained is an item an earlier step of the window acquires (an item
// objective it completes) before its first use, so it need not leave the bank.
type Obtained struct {
	Name      string `json:"name"`
	ItemID    int    `json:"item_id"`
	Qty       int    `json:"qty"`
	StepID    string `json:"step_id"`
	UseStepID string `json:"use_step_id"`
	UseOffset int    `json:"use_offset"`
}

// LookAhead collects the requirements of the steps of seg from fromStepID
// (inclusive; empty means the first step) to the end of seg. Each item is
// reported once, at its first use; an item that an earlier step of the
// window obtains moves to Obtained. It returns nil when fromStepID is not in
// seg or no step of the window lists an item.
func LookAhead(seg *content.Segment, fromStepID string, h rules.ItemHoldings, catalog *entities.Registry) *Ahead {
	start := 0
	if fromStepID != "" {
		start = slices.IndexFunc(seg.Steps, func(st *content.Step) bool { return st.ID == fromStepID })
		if start < 0 {
			return nil
		}
	}
	window := seg.Steps[start:]
	if len(window) == 0 {
		return nil
	}
	out := &Ahead{
		FromStepID: window[0].ID, UntilStepID: window[len(window)-1].ID, Steps: len(window) - 1,
		Groups: []AheadGroup{}, Obtained: []Obtained{},
	}

	type acquired struct {
		stepID string
		qty    int
	}
	got := map[int]acquired{} // item id -> first step of the window that obtains it
	// seen maps a requirement to its first use: group and item indexes, or a
	// negative group for an obtained item.
	seen := map[string][2]int{}
	for offset, st := range window {
		group := -1
		for _, li := range st.Requirements {
			key := requirementKey(li)
			if at, ok := seen[key]; ok {
				if at[0] >= 0 {
					it := &out.Groups[at[0]].Items[at[1]]
					if out.Groups[at[0]].StepID != st.ID && !slices.Contains(it.AlsoAt, st.ID) {
						it.AlsoAt = append(it.AlsoAt, st.ID)
					}
				}
				continue
			}
			if id, a, ok := obtainedBefore(li, got); ok {
				seen[key] = [2]int{-1, 0}
				out.Obtained = append(out.Obtained, Obtained{
					Name: li.Name, ItemID: id, Qty: a.qty, StepID: a.stepID, UseStepID: st.ID, UseOffset: offset,
				})
				continue
			}
			if group < 0 {
				out.Groups = append(out.Groups, AheadGroup{StepID: st.ID, Text: st.Text, Offset: offset})
				group = len(out.Groups) - 1
			}
			g := &out.Groups[group]
			g.Items = append(g.Items, AheadItem{Item: evaluateItem(li, SlotInventory, h, catalog)})
			seen[key] = [2]int{group, len(g.Items) - 1}
		}
		// Acquisitions count for later steps only: a step that obtains and
		// uses the same item still needs it listed.
		for _, o := range st.Objectives {
			if o.Role != content.RoleCompletes || o.Key.Op != entities.OpQuantity || o.Key.Ref.Kind != entities.KindItem {
				continue
			}
			id, err := strconv.Atoi(o.Key.Ref.ID)
			if err != nil {
				continue
			}
			if _, ok := got[id]; !ok {
				got[id] = acquired{stepID: st.ID, qty: o.Key.N}
			}
		}
	}
	if len(out.Groups) == 0 && len(out.Obtained) == 0 {
		return nil
	}
	return out
}

// requirementKey identifies a requirement across steps: the same item, in the
// same form, is listed once.
func requirementKey(li content.ItemRequirement) string {
	noted := ""
	if li.Noted {
		noted = "/noted"
	}
	switch li.Variant() {
	case content.RequireExact:
		return "id:" + strconv.Itoa(*li.ItemID) + noted
	case content.RequireAnyOf:
		ids := make([]string, len(li.AnyOf))
		for i, id := range li.AnyOf {
			ids[i] = strconv.Itoa(id)
		}
		slices.Sort(ids)
		return "any:" + strings.Join(ids, "|") + noted
	case content.RequireFamily:
		return "family:" + li.Family + noted
	}
	return "name:" + strings.ToLower(strings.TrimSpace(li.Name))
}

// obtainedBefore reports the id and acquisition of a requirement that an
// earlier step of the window obtains.
func obtainedBefore[T any](li content.ItemRequirement, got map[int]T) (int, T, bool) {
	var ids []int
	switch li.Variant() {
	case content.RequireExact:
		ids = []int{*li.ItemID}
	case content.RequireAnyOf:
		ids = li.AnyOf
	}
	for _, id := range ids {
		if a, ok := got[id]; ok {
			return id, a, true
		}
	}
	var zero T
	return 0, zero, false
}
