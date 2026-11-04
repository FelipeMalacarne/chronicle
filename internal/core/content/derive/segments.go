// Package derive enriches imported guide steps with deterministic structural
// metadata. It only promotes explicit source facts; it never treats absent
// loadout information as an empty loadout.
package derive

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
)

type Stats struct {
	Variants        int
	Segments        int
	BankLegs        int
	KnownLoadouts   int
	PartialLoadouts int
	UnknownLoadouts int
}

var (
	depositAllPattern = regexp.MustCompile(`(?i)\bdeposit\s+all(?:\s+of)?(?:\s+your)?\s+(?:items|inventory)\b`)
	bankWordPattern   = regexp.MustCompile(`(?i)\b(?:deposit(?:ing)?|withdraw(?:ing)?|rebank(?:ing)?|bank(?:ing)?)\b`)
	emptySlotsPattern = regexp.MustCompile(`(?i)\b(?:at\s+least\s+)?(\d+)\s+empty(?:\s+inventory)?\s+slots?\b`)
	oneEmptyPattern   = regexp.MustCompile(`(?i)\b(?:an|one|1)\s+empty(?:\s+inventory)?\s+slot\b`)
	quantityPattern   = regexp.MustCompile(`^\s*(\d+)\s+(.+\S)\s*$`)
	equipPattern      = regexp.MustCompile(`(?i)\s*\((?:equip|equipped)\)\s*$`)
	optionalPattern   = regexp.MustCompile(`(?i)^\s*optional\s*:\s*|\s*\(optional\)\s*$`)
)

// EnrichGuide moves legacy root sections under a main variant and derives
// segments from explicit bank/deposit/withdraw boundaries. Calling it twice is
// idempotent: existing variants and segments are preserved.
func EnrichGuide(g *content.Guide) Stats {
	if len(g.Variants) == 0 {
		g.DefaultVariant = "main"
		g.Variants = []*content.Variant{{
			ID:       "main",
			Slug:     "main",
			Title:    "Main route",
			Modes:    append([]string(nil), g.DefaultModes...),
			Layers:   append([]string(nil), g.DefaultLayers...),
			Sections: g.Sections,
		}}
	}
	if g.DefaultVariant == "" && len(g.Variants) > 0 {
		g.DefaultVariant = g.Variants[0].ID
	}

	stats := Stats{Variants: len(g.Variants)}
	for _, variant := range g.Variants {
		for _, section := range variant.Sections {
			if len(section.Segments) == 0 {
				section.Segments = deriveSection(section)
			}
			for _, segment := range section.Segments {
				stats.Segments++
				if segment.Kind == content.SegmentBankLeg {
					stats.BankLegs++
				}
				if segment.Loadout == nil {
					continue
				}
				switch segment.Loadout.Status {
				case content.LoadoutKnown:
					stats.KnownLoadouts++
				case content.LoadoutPartial:
					stats.PartialLoadouts++
				case content.LoadoutUnknown:
					stats.UnknownLoadouts++
				}
			}
		}
	}
	g.Index()
	return stats
}

func deriveSection(section *content.Section) []*content.Segment {
	if len(section.Steps) == 0 {
		return nil
	}
	var segments []*content.Segment
	for _, step := range section.Steps {
		actions := bankActions(step)
		if len(segments) == 0 || len(actions) > 0 {
			segment := newSegment(section.ID, step, actions)
			segments = append(segments, segment)
		}
		segments[len(segments)-1].Steps = append(segments[len(segments)-1].Steps, step)
	}
	return segments
}

func newSegment(sectionID string, first *content.Step, actions []content.BankAction) *content.Segment {
	material := sectionID + "\x00" + first.ID
	sum := sha256.Sum256([]byte(material))
	segment := &content.Segment{
		ID:    "seg-" + hex.EncodeToString(sum[:6]),
		Kind:  content.SegmentSequence,
		Title: "Sequence from " + first.ID,
		Source: content.Provenance{
			Method:     content.ProvenanceDerived,
			Confidence: content.ConfidenceHigh,
			StepIDs:    []string{first.ID},
			Evidence:   "section start; no bank boundary asserted",
		},
		Loadout: &content.Loadout{Status: content.LoadoutUnknown},
	}
	if len(actions) == 0 {
		return segment
	}
	segment.Kind = content.SegmentBankLeg
	segment.Title = "Bank leg from " + first.ID
	segment.BankActions = actions
	segment.Source = content.Provenance{
		Method:     content.ProvenanceDerived,
		Confidence: content.ConfidenceHigh,
		StepIDs:    []string{first.ID},
		Evidence:   "segment begins at an explicit bank action",
	}
	segment.Loadout = loadoutFromStep(first)
	return segment
}

type actionMatch struct {
	start int
	end   int
	kind  string
	text  string
}

func bankActions(step *content.Step) []content.BankAction {
	text := step.Text
	var matches []actionMatch
	occupied := [][2]int{}
	for _, loc := range depositAllPattern.FindAllStringIndex(text, -1) {
		matches = append(matches, actionMatch{start: loc[0], end: loc[1], kind: content.BankDepositAll, text: text[loc[0]:loc[1]]})
		occupied = append(occupied, [2]int{loc[0], loc[1]})
	}
	for _, loc := range bankWordPattern.FindAllStringIndex(text, -1) {
		if overlaps(loc, occupied) {
			continue
		}
		word := strings.ToLower(text[loc[0]:loc[1]])
		actionType := content.BankOpen
		switch {
		case strings.HasPrefix(word, "deposit"):
			actionType = content.BankDeposit
		case strings.HasPrefix(word, "withdraw"):
			actionType = content.BankWithdraw
		case strings.HasPrefix(word, "rebank"):
			actionType = content.BankSetup
		case len(step.Items) > 0:
			actionType = content.BankSetup
		}
		matches = append(matches, actionMatch{start: loc[0], end: loc[1], kind: actionType, text: text[loc[0]:loc[1]]})
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })
	actions := make([]content.BankAction, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		position := content.PositionWithin
		if match.start <= 12 {
			position = content.PositionStart
		} else if match.end >= len(text)-12 {
			position = content.PositionEnd
		}
		key := match.kind + "\x00" + position + "\x00" + strings.ToLower(match.text)
		if seen[key] {
			continue
		}
		seen[key] = true
		evidence := strings.TrimSpace(match.text)
		actions = append(actions, content.BankAction{
			Type:     match.kind,
			Position: position,
			Text:     evidence,
			Source: content.Provenance{
				Method:     content.ProvenanceExplicit,
				Confidence: content.ConfidenceHigh,
				StepIDs:    []string{step.ID},
				Evidence:   evidence,
			},
		})
	}
	return actions
}

func overlaps(loc []int, ranges [][2]int) bool {
	for _, value := range ranges {
		if loc[0] < value[1] && loc[1] > value[0] {
			return true
		}
	}
	return false
}

func loadoutFromStep(step *content.Step) *content.Loadout {
	loadout := &content.Loadout{Status: content.LoadoutUnknown}
	var evidence []string
	for _, item := range step.Items {
		parsed, equipped := loadoutItem(item)
		if equipped {
			loadout.Equipment = append(loadout.Equipment, parsed)
		} else {
			loadout.Inventory = append(loadout.Inventory, parsed)
		}
	}
	if len(step.Items) > 0 {
		evidence = append(evidence, "step.items")
	}
	if emptySlots, match := explicitEmptySlots(step.Text); emptySlots != nil {
		loadout.EmptySlots = emptySlots
		evidence = append(evidence, match)
	}
	if len(loadout.Inventory) == 0 && len(loadout.Equipment) == 0 && loadout.EmptySlots == nil {
		return loadout
	}
	loadout.Status = content.LoadoutPartial
	loadout.Source = &content.Provenance{
		Method:     content.ProvenanceExplicit,
		Confidence: content.ConfidenceHigh,
		StepIDs:    []string{step.ID},
		Evidence:   strings.Join(evidence, "; "),
	}
	return loadout
}

func loadoutItem(item content.Item) (content.LoadoutItem, bool) {
	sourceText := strings.TrimSpace(item.Name)
	name := sourceText
	equipped := item.Equip || equipPattern.MatchString(name)
	name = equipPattern.ReplaceAllString(name, "")
	optional := optionalPattern.MatchString(name)
	name = optionalPattern.ReplaceAllString(name, "")
	qty := item.Qty
	if qty == nil {
		if match := quantityPattern.FindStringSubmatch(name); match != nil {
			value, err := strconv.Atoi(match[1])
			if err == nil && value > 0 {
				qty = &value
				name = match[2]
			}
		}
	}
	return content.LoadoutItem{
		Name:       strings.TrimSpace(name),
		ItemID:     item.ItemID,
		Qty:        qty,
		Optional:   optional,
		SourceText: sourceText,
	}, equipped
}

func explicitEmptySlots(text string) (*int, string) {
	if match := emptySlotsPattern.FindStringSubmatch(text); match != nil {
		value, err := strconv.Atoi(match[1])
		if err == nil && value >= 0 && value <= 28 {
			return &value, match[0]
		}
	}
	if match := oneEmptyPattern.FindString(text); match != "" {
		value := 1
		return &value, match
	}
	return nil, ""
}
