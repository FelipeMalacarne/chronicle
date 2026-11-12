package planning

import "github.com/FelipeMalacarne/chronicle/internal/core/content"

// Legs returns the segment that contains stepID and the next bank leg after
// it in the same variant's main route (parallel tracks are skipped). An empty
// stepID means the start of the guide's first main section. next is nil when
// no bank leg follows; current is nil when stepID is not in a main section of
// g or the guide has no segments.
func Legs(g *content.Guide, stepID string) (current, next *content.Segment) {
	variant := ""
	for _, sec := range g.Sections {
		if sec.IsTrack() || (variant != "" && sec.VariantID != variant) {
			continue
		}
		for _, seg := range sec.Segments {
			if current == nil {
				if !containsStep(seg, stepID) {
					continue
				}
				current, variant = seg, sec.VariantID
				continue
			}
			if seg.Kind == content.SegmentBankLeg {
				return current, seg
			}
		}
	}
	return current, nil
}

func containsStep(seg *content.Segment, stepID string) bool {
	if stepID == "" {
		return len(seg.Steps) > 0
	}
	for _, st := range seg.Steps {
		if st.ID == stepID {
			return true
		}
	}
	return false
}
