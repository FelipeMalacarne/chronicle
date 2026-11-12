package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// StepTextHash is the hash the annotations of a step are pinned to: the
// source fields an annotation may quote (text, location, notes, items).
// Editing any of them invalidates the step's annotation.
func StepTextHash(st *Step) string {
	data, _ := json.Marshal(struct {
		Text     string `json:"text"`
		Location string `json:"location,omitempty"`
		Notes    []Note `json:"notes,omitempty"`
		Items    []Item `json:"items,omitempty"`
	}{st.Text, st.Location, st.Notes, st.Items})
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// RuleObjectives converts a step's complete_when and prerequisites into
// objectives: complete_when rules complete (or start) the step, prerequisites
// are requires. Rules that have no objective target (manual) are skipped;
// rules that cannot be expressed as one (any/not, eq on a quest state,
// varbits...) are reported, never approximated.
func RuleObjectives(st *Step, reg *entities.Registry) ([]Objective, []Issue) {
	var (
		out    []Objective
		issues []Issue
	)
	for _, field := range []struct {
		name  string
		rules RuleList
	}{{"complete_when", st.CompleteWhen}, {"prerequisites", st.Prerequisites}} {
		for i, r := range field.rules {
			objs, errs := ruleObjectives(r, fmt.Sprintf("/%s/%d", field.name, i), field.name == "prerequisites", reg)
			out = append(out, objs...)
			issues = append(issues, errs...)
		}
	}
	return out, issues
}

func ruleObjectives(r Rule, ptr string, prerequisite bool, reg *entities.Registry) ([]Objective, []Issue) {
	fail := func(format string, args ...any) ([]Objective, []Issue) {
		return nil, []Issue{{Pointer: ptr, Message: fmt.Sprintf(format, args...)}}
	}
	role := RoleCompletes
	if prerequisite {
		role = RoleRequires
	}
	one := func(key entities.ObjectiveKey, role string, meta *RuleMeta) ([]Objective, []Issue) {
		if msg := reg.KnownObjective(key); msg != "" {
			return fail("%s", msg)
		}
		return []Objective{{Key: key, Role: role, Evidence: &ObjectiveEvidence{Field: ptr}, Source: ruleSource(meta)}}, nil
	}
	switch v := r.(type) {
	case AllRule:
		var (
			out    []Objective
			issues []Issue
		)
		for i, c := range v.All {
			objs, errs := ruleObjectives(c, fmt.Sprintf("%s/all/%d", ptr, i), prerequisite, reg)
			out = append(out, objs...)
			issues = append(issues, errs...)
		}
		return out, issues
	case ManualRule:
		return nil, nil
	case QuestStateRule:
		res := reg.ResolveQuest(v.Quest)
		if res.Status != entities.Resolved {
			return fail("quest %q does not resolve (%s)", v.Quest, res.Reason)
		}
		if v.Op != "" && v.Op != "gte" {
			return fail("quest_state with op %q has no objective target", v.Op)
		}
		key := entities.ObjectiveKey{Ref: res.Ref, Op: entities.OpState}
		switch v.State {
		case QuestInProgress:
			key.State = entities.StateStarted
			if !prerequisite {
				role = RoleStarts
			}
		case QuestFinished:
			key.State = entities.StateFinished
		default:
			return fail("quest_state %q has no objective target", v.State)
		}
		return one(key, role, v.Meta)
	case SkillLevelRule:
		res := reg.ResolveSkill(v.Skill)
		if res.Status != entities.Resolved {
			return fail("skill %q does not resolve (%s)", v.Skill, res.Reason)
		}
		if v.Boosted {
			return fail("boosted skill_level has no objective target")
		}
		return one(entities.ObjectiveKey{Ref: res.Ref, Op: entities.OpAtLeast, N: v.Level}, role, v.Meta)
	case DiaryTierRule:
		var areas []string
		if v.Area == "all" {
			areas = reg.DiaryAreaIDs()
		} else {
			res := reg.ResolveDiary(v.Area, v.Tier)
			if res.Status != entities.Resolved {
				return fail("diary %q %q does not resolve (%s)", v.Area, v.Tier, res.Reason)
			}
			areas = []string{res.Ref.ID}
		}
		var out []Objective
		for _, a := range areas {
			id := a
			if v.Area == "all" {
				id = a + ":" + v.Tier
			}
			objs, errs := one(entities.ObjectiveKey{Ref: entities.Ref{Kind: entities.KindDiary, ID: id}, Op: entities.OpState, State: entities.StateComplete}, role, v.Meta)
			if len(errs) > 0 {
				return nil, errs
			}
			out = append(out, objs...)
		}
		return out, nil
	case BossKCRule:
		ref, err := entities.NewRef(entities.KindBoss, entities.Slug(v.Boss))
		if err != nil {
			return fail("%v", err)
		}
		return one(entities.ObjectiveKey{Ref: ref, Op: entities.OpAtLeast, N: v.KC}, role, v.Meta)
	case ItemOwnedRule:
		if v.ItemID == nil {
			return fail("item_owned without item_id has no objective target")
		}
		if len(v.Containers) > 0 {
			return fail("item_owned limited to containers %v has no objective target", v.Containers)
		}
		return one(entities.ObjectiveKey{Ref: entities.ItemRef(*v.ItemID), Op: entities.OpQuantity, N: max(v.Qty, 1)}, role, v.Meta)
	}
	return fail("rule %q has no objective target", r.RuleType())
}

// ruleSource maps rule provenance to objective provenance: structured source
// fields (questStatus, skillGoal) are explicit, text-inferred rules derived.
func ruleSource(meta *RuleMeta) ObjectiveSource {
	src := ObjectiveSource{Method: ProvenanceExplicit, Confidence: ConfidenceHigh}
	if meta == nil {
		return src
	}
	if meta.Inferred || meta.Origin == "text" {
		src.Method = ProvenanceDerived
	}
	if meta.Confidence != "" {
		src.Confidence = meta.Confidence
	}
	return src
}

// checkObjective validates one objective of a step: a known key, a role that
// fits the key, and verifiable evidence when it quotes the step.
func checkObjective(st *Step, doc any, o Objective, reg *entities.Registry) []string {
	var msgs []string
	if msg := reg.KnownObjective(o.Key); msg != "" {
		msgs = append(msgs, msg)
	}
	switch o.Role {
	case RoleCompletes, RoleRequires:
		if o.Key.Ref.Kind == entities.KindQuest && o.Key.State == entities.StateStarted && o.Role == RoleCompletes {
			msgs = append(msgs, "a step that starts a quest has role starts, not completes")
		}
	case RoleStarts:
		if o.Key.Ref.Kind != entities.KindQuest || o.Key.State != entities.StateStarted {
			msgs = append(msgs, fmt.Sprintf("role starts needs a quest@started key, got %s", o.Key))
		}
	case RoleUses:
		if o.Key.Ref.Kind != entities.KindItem {
			msgs = append(msgs, fmt.Sprintf("role uses needs an item key, got %s", o.Key))
		}
	default:
		msgs = append(msgs, fmt.Sprintf("unknown role %q", o.Role))
	}
	if o.Evidence != nil && o.Evidence.Quote != "" {
		if msg := verifyQuote(doc, *o.Evidence); msg != "" {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// verifyQuote checks that ev.Field is a string of the step and ev.Span selects
// ev.Quote in it, up to case, whitespace and typographic punctuation.
func verifyQuote(doc any, ev ObjectiveEvidence) string {
	s, msg := fieldText(doc, ev.Field)
	if msg != "" {
		return msg
	}
	if len(ev.Span) != 2 {
		return "quoted evidence needs a span"
	}
	if !entities.QuoteAt(s, ev.Span[0], ev.Span[1], ev.Quote) {
		start, end, found := entities.FindQuote(s, ev.Quote)
		if found {
			return fmt.Sprintf("span %v does not select %q in %s (it is at [%d, %d])", ev.Span, ev.Quote, ev.Field, start, end)
		}
		return fmt.Sprintf("quote %q is not in %s", ev.Quote, ev.Field)
	}
	return ""
}

// fieldText returns the text a pointer addresses in a step document: a
// string, or the name of an object (an item).
func fieldText(doc any, field string) (string, string) {
	val, ok := jsonPointer(doc, field)
	if !ok {
		return "", fmt.Sprintf("field %q does not exist in the step", field)
	}
	s, isString := val.(string)
	if obj, isObj := val.(map[string]any); isObj {
		s, isString = obj["name"].(string)
	}
	if !isString {
		return "", fmt.Sprintf("field %q is not text", field)
	}
	return s, ""
}

// QuoteSpan locates quote in the step field addressed by field (/text,
// /location, /notes/0/text, /items/2) the way the verifier compares them and
// returns the exact [start, end) of the original, so evidence can be built
// from a quote alone. The first occurrence wins.
func QuoteSpan(st *Step, field, quote string) ([]int, error) {
	s, msg := fieldText(stepDoc(st), field)
	if msg != "" {
		return nil, errors.New(msg)
	}
	start, end, found := entities.FindQuote(s, quote)
	if !found {
		return nil, fmt.Errorf("quote %q is not in %s", quote, field)
	}
	return []int{start, end}, nil
}

// stepDoc is the step as decoded JSON, the document evidence pointers address.
func stepDoc(st *Step) any {
	var doc any
	if raw, err := json.Marshal(st); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	return doc
}

// mergeObjectives appends add to base; an objective with the same key and
// role replaces the earlier one in place.
func mergeObjectives(base, add []Objective) []Objective {
	at := make(map[string]int, len(base))
	for i, o := range base {
		at[o.Key.String()+"|"+o.Role] = i
	}
	for _, o := range add {
		k := o.Key.String() + "|" + o.Role
		if i, ok := at[k]; ok {
			base[i] = o
			continue
		}
		at[k] = len(base)
		base = append(base, o)
	}
	return base
}

// checkStepObjectives validates the objectives a step states in guide.json,
// its location ref and the conversion of its rules into objectives. It does
// not change the step: composition is Load's job (composeRuleObjectives), so
// a guide parsed on its own re-encodes to the same guide.json.
func (g *Guide) checkStepObjectives(name, ptr string, st *Step, reg *entities.Registry) Errors {
	var issues Errors
	if len(st.Objectives) > 0 {
		doc := stepDoc(st)
		for i, o := range st.Objectives {
			for _, msg := range checkObjective(st, doc, o, reg) {
				issues = append(issues, Issue{File: name, Pointer: fmt.Sprintf("%s/objectives/%d", ptr, i), Message: msg})
			}
		}
	}
	if st.LocationRef != nil && (st.LocationRef.Kind != entities.KindLocation || !reg.Known(*st.LocationRef)) {
		issues = append(issues, Issue{File: name, Pointer: ptr + "/location_ref", Message: fmt.Sprintf("unknown location %s", st.LocationRef)})
	}
	_, errs := RuleObjectives(st, reg)
	for _, e := range errs {
		issues = append(issues, Issue{File: name, Pointer: ptr + e.Pointer, Message: e.Message})
	}
	return issues
}

// composeRuleObjectives appends to every step the objectives converted from
// its complete_when and prerequisites (after those guide.json states). The
// guide was checked by ParseGuide, so conversion problems cannot occur here.
func (g *Guide) composeRuleObjectives(reg *entities.Registry) {
	for _, st := range g.Steps() {
		converted, _ := RuleObjectives(st, reg)
		st.Objectives = mergeObjectives(st.Objectives, converted)
	}
}
