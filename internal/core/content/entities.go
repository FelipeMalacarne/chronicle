package content

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// checkStepEntities validates a step's entity mentions: catalog-backed refs
// must exist in the registry, step refs must resolve in this guide, and the
// evidence must point at the step value that holds SourceText verbatim.
func (g *Guide) checkStepEntities(name, ptr string, st *Step, reg *entities.Registry) Errors {
	if len(st.Entities) == 0 {
		return nil
	}
	var issues Errors
	var doc any
	if raw, err := json.Marshal(st); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	for i, m := range st.Entities {
		mPtr := fmt.Sprintf("%s/entities/%d", ptr, i)
		if msg := g.checkRef(m.Ref, reg); msg != "" {
			issues = append(issues, Issue{File: name, Pointer: mPtr + "/ref", Message: msg})
		}
		val, ok := jsonPointer(doc, m.Field)
		if !ok {
			issues = append(issues, Issue{File: name, Pointer: mPtr + "/field", Message: fmt.Sprintf("field %q does not exist in the step", m.Field)})
			continue
		}
		s, isString := val.(string)
		if obj, isObj := val.(map[string]any); isObj {
			// Object-form items carry their text in name.
			s, isString = obj["name"].(string)
		}
		if !isString {
			continue
		}
		if len(m.Span) == 2 {
			start, end := m.Span[0], m.Span[1]
			if start < 0 || end <= start || end > len(s) || s[start:end] != m.SourceText {
				issues = append(issues, Issue{File: name, Pointer: mPtr + "/span", Message: fmt.Sprintf("span %v does not select %q in %s", m.Span, m.SourceText, m.Field)})
			}
		} else if !strings.Contains(s, m.SourceText) {
			issues = append(issues, Issue{File: name, Pointer: mPtr + "/source_text", Message: fmt.Sprintf("source_text %q is not in %s", m.SourceText, m.Field)})
		}
	}
	return issues
}

// checkLoadoutItemIDs validates the resolved loadout requirements against
// the item catalog.
func checkLoadoutItemIDs(name, ptr string, l *Loadout, reg *entities.Registry) Errors {
	if l == nil {
		return nil
	}
	var issues Errors
	for _, field := range []struct {
		name string
		list []ItemRequirement
	}{{"inventory", l.Inventory}, {"equipment", l.Equipment}} {
		for i, it := range field.list {
			for _, msg := range checkRequirement(it, reg) {
				issues = append(issues, Issue{File: name, Pointer: fmt.Sprintf("%s/loadout/%s/%d", ptr, field.name, i), Message: msg})
			}
		}
	}
	return issues
}

// checkRequirement validates one item requirement: at most one variant, and
// every id and family known to the catalog.
func checkRequirement(it ItemRequirement, reg *entities.Registry) []string {
	var msgs []string
	set := 0
	if it.ItemID != nil {
		set++
		if !reg.Known(entities.ItemRef(*it.ItemID)) {
			msgs = append(msgs, fmt.Sprintf("unknown item id %d", *it.ItemID))
		}
	}
	if len(it.AnyOf) > 0 {
		set++
		seen := map[int]bool{}
		for _, id := range it.AnyOf {
			if seen[id] {
				msgs = append(msgs, fmt.Sprintf("any_of lists item %d twice", id))
			}
			seen[id] = true
			if !reg.Known(entities.ItemRef(id)) {
				msgs = append(msgs, fmt.Sprintf("unknown item id %d in any_of", id))
			}
		}
	}
	if it.Family != "" {
		set++
		if _, ok := reg.Family(it.Family); !ok {
			msgs = append(msgs, fmt.Sprintf("unknown item family %q", it.Family))
		}
	}
	if set > 1 {
		msgs = append(msgs, "set only one of item_id, any_of and family")
	}
	return msgs
}

func (g *Guide) checkRef(ref entities.Ref, reg *entities.Registry) string {
	if err := ref.Validate(); err != nil {
		return err.Error()
	}
	if ref.Kind == entities.KindStep {
		guideID, stepID, scoped := strings.Cut(ref.ID, "/")
		if !scoped {
			stepID, guideID = guideID, g.ID
		}
		if guideID != g.ID {
			return "" // cross-guide step refs are resolved by the content set
		}
		if _, ok := g.stepsByID[stepID]; !ok {
			return fmt.Sprintf("unknown step %q", stepID)
		}
		return ""
	}
	if reg.HasCatalog(ref.Kind) && !reg.Known(ref) {
		return fmt.Sprintf("unknown entity %s", ref)
	}
	return ""
}

// jsonPointer resolves an RFC 6901 pointer against decoded JSON.
func jsonPointer(doc any, ptr string) (any, bool) {
	if ptr == "" {
		return doc, true
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, false
	}
	cur := doc
	for _, tok := range strings.Split(ptr[1:], "/") {
		tok = strings.NewReplacer("~1", "/", "~0", "~").Replace(tok)
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[tok]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(v) {
				return nil, false
			}
			cur = v[i]
		default:
			return nil, false
		}
	}
	return cur, true
}
