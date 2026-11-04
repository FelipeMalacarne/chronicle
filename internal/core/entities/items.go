package entities

// ItemForm describes one concrete item id as the game (and the plugin) sees
// it. Noted and unnoted forms have different ids; variants (doses, charges)
// are different ids of one family.
type ItemForm struct {
	ID      int
	BaseID  int // unnoted id; equals ID for unnoted forms
	Noted   bool
	Name    string
	Family  string // empty when the item has no variants
	Variant string
	WikiKey WikiKey
	// Stackable is true when one inventory slot holds any quantity; noted
	// forms are always stackable.
	Stackable bool
}

// Item describes an item id listed in the catalog, unnoted or noted.
func (r *Registry) Item(id int) (ItemForm, bool) {
	if it, ok := r.items[id]; ok {
		return ItemForm{ID: id, BaseID: id, Name: it.Name, Family: it.Family, Variant: it.Variant, WikiKey: it.WikiKey, Stackable: it.Stackable}, true
	}
	if base, ok := r.notedBase[id]; ok {
		it := r.items[base]
		return ItemForm{ID: id, BaseID: base, Noted: true, Name: it.Name, Family: it.Family, Variant: it.Variant, WikiKey: it.WikiKey, Stackable: true}, true
	}
	return ItemForm{}, false
}

// NotedID returns the noted form of an unnoted id, when the item can be noted.
func (r *Registry) NotedID(id int) (int, bool) {
	it, ok := r.items[id]
	if !ok || it.NotedID == nil {
		return 0, false
	}
	return *it.NotedID, true
}

// ItemComparison selects how two item ids are compared. Comparisons are
// explicit so that a caller matching plugin inventory against a loadout
// decides whether a noted stack satisfies an unnoted requirement.
type ItemComparison int

const (
	// CompareExact: same id (noted ≠ unnoted, Prayer potion(4) ≠ (3)).
	CompareExact ItemComparison = iota
	// CompareIgnoreNoted: noted and unnoted forms of one id are equal.
	CompareIgnoreNoted
	// CompareFamily: any form of any variant of one family is equal
	// (Prayer potion(4) = Prayer potion(1) = noted Prayer potion(3)).
	CompareFamily
)

// SameItem compares two item ids. Ids missing from the catalog only ever
// compare equal to themselves: equivalence is never assumed for unknown ids.
func (r *Registry) SameItem(a, b int, cmp ItemComparison) bool {
	if a == b {
		return true
	}
	if cmp == CompareExact {
		return false
	}
	fa, okA := r.Item(a)
	fb, okB := r.Item(b)
	if !okA || !okB {
		return false
	}
	if fa.BaseID == fb.BaseID {
		return true
	}
	return cmp == CompareFamily && fa.Family != "" && fa.Family == fb.Family
}
