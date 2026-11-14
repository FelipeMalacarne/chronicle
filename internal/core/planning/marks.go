package planning

import "github.com/FelipeMalacarne/chronicle/internal/core/content"

// ItemMarks are the owner's marks on the requirements of one segment, by
// RequirementKey: "I already have this item for this leg". A marked
// requirement reads as met (Item.OwnerMarked) in EvaluateSegment and LookAhead.
//
// A mark is the owner's assertion, never evidence (CLAUDE.md rules 2 and 8):
// it is a separate input overlaid when readiness is computed, and nothing is
// written to item holdings, snapshots or player_state. If the plugin later
// proves the item is held the mark is harmless; if it proves the item absent
// the mark still wins until the owner takes it back. It satisfies the whole
// requirement (all of Required), never a part of it. A nil map means no marks.
type ItemMarks map[string]bool

// OwnerMarks are the marks of an account, per (guide, segment): a mark never
// leaks to another segment or guide.
type OwnerMarks map[[2]string]ItemMarks

// For is the marks of one segment (nil for none).
func (m OwnerMarks) For(guideID, segmentID string) ItemMarks {
	return m[[2]string{guideID, segmentID}]
}

// Add records a mark.
func (m OwnerMarks) Add(guideID, segmentID, key string) {
	k := [2]string{guideID, segmentID}
	if m[k] == nil {
		m[k] = ItemMarks{}
	}
	m[k][key] = true
}

// SegmentRequirementKeys is every requirement key of seg an owner may mark:
// the items of its loadout and, for a leg without one, the requirements of its
// steps (what LookAhead previews).
func SegmentRequirementKeys(seg *content.Segment) map[string]bool {
	out := map[string]bool{}
	if lo := seg.Loadout; lo != nil {
		for _, li := range lo.Equipment {
			out[RequirementKey(li)] = true
		}
		for _, li := range lo.Inventory {
			out[RequirementKey(li)] = true
		}
	}
	for _, st := range seg.Steps {
		for _, li := range st.Requirements {
			out[RequirementKey(li)] = true
		}
	}
	return out
}
