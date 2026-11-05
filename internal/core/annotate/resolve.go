package annotate

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// Kinds an objective proposal can name. They are the entity kinds plus
// "qp" (quest points), which has no entity.
const KindQuestPoints = "qp"

// EntityResult is what resolve_entity answers: the ref when the name
// resolves to one entity, the candidates when it is ambiguous, and why when
// it does not resolve.
type EntityResult struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Status     entities.Status   `json:"status"`
	Ref        string            `json:"ref,omitempty"`
	Match      entities.Match    `json:"match,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Candidates []EntityCandidate `json:"candidates,omitempty"`
	// Family is set for item names that name a whole family (all doses of a
	// potion, every charge of a jewel): usable as an item requirement family.
	Family string `json:"family,omitempty"`
	// Noted is set when the name said "noted".
	Noted bool `json:"noted,omitempty"`
}

// EntityCandidate is one candidate of an ambiguous name. Variant and Page
// tell same-name items apart: variants of one wiki page (potion doses,
// "Dyed" and "Grey" wigs), or items on pages of their own ("Message (Romeo &
// Juliet)").
type EntityCandidate struct {
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Variant string `json:"variant,omitempty"`
	Page    string `json:"page,omitempty"`
}

// ResolveEntity resolves a name of a kind the way submit will. tier is the
// diary tier (name is then the area).
func (w *Workspace) ResolveEntity(kind, name, tier string) EntityResult {
	res, noted := resolveName(w.reg, kind, name, tier)
	out := EntityResult{Kind: kind, Name: name, Status: res.Status, Match: res.Match, Reason: res.Reason, Noted: noted}
	if res.Status == entities.Resolved {
		out.Ref = res.Ref.String()
	}
	for _, c := range res.Candidates {
		disp, _ := w.reg.Display(c)
		v, page := variant(w.reg, c)
		out.Candidates = append(out.Candidates, EntityCandidate{Ref: c.String(), Name: disp, Variant: v, Page: page})
	}
	if kind == string(entities.KindItem) {
		out.Family = familySlug(w.reg, entities.ParseItemPhrase(name).Name)
	}
	return out
}

// familySlug returns the slug of the item family a name or slug denotes,
// also in the plural ("prayer potions"), or "".
func familySlug(reg *entities.Registry, name string) string {
	for _, cand := range []string{name, entities.FamilySlug(name), entities.FamilySlug(strings.TrimSuffix(strings.TrimSpace(name), "s"))} {
		if _, ok := reg.Family(cand); ok && cand != "" {
			return cand
		}
	}
	return ""
}

// resolveName dispatches to the registry resolver of kind. Boss refs have no
// catalog and resolve to their slug.
func resolveName(reg *entities.Registry, kind, name, tier string) (entities.Resolution, bool) {
	switch entities.Kind(kind) {
	case entities.KindQuest:
		return reg.ResolveQuest(name), false
	case entities.KindSkill:
		return reg.ResolveSkill(name), false
	case entities.KindDiary:
		return reg.ResolveDiary(name, tier), false
	case entities.KindCombatAchievement:
		return reg.ResolveCombatAchievement(name), false
	case entities.KindUnlock:
		return reg.ResolveUnlock(name), false
	case entities.KindLocation:
		return reg.ResolveLocation(name), false
	case entities.KindItem:
		p := entities.ParseItemPhrase(name)
		res, _ := reg.ResolveItem(p.Name)
		return res, p.Noted
	case entities.KindBoss:
		ref, err := entities.NewRef(entities.KindBoss, entities.Slug(name))
		if err != nil {
			return entities.Resolution{Status: entities.Unresolved, Reason: err.Error()}, false
		}
		return entities.Resolution{Status: entities.Resolved, Ref: ref, Match: entities.MatchName}, false
	}
	return entities.Resolution{Status: entities.Unresolved, Reason: fmt.Sprintf("unknown kind %q", kind)}, false
}

// pick turns a resolution into one ref. id chooses among the candidates of
// an ambiguous name and must agree with a resolved one; an unresolved name
// never gets an id. id is the bare id ("552") or the full ref ("item:552"),
// the form resolve_entity and the rejection reasons print.
func pick(reg *entities.Registry, res entities.Resolution, kind, name, id string) (entities.Ref, error) {
	switch res.Status {
	case entities.Resolved:
		if id != "" && !sameRef(res.Ref, id) {
			return entities.Ref{}, fmt.Errorf("%s %q resolves to %s, not id %q", kind, name, res.Ref, id)
		}
		return res.Ref, nil
	case entities.Ambiguous:
		if id == "" {
			return entities.Ref{}, fmt.Errorf("%s %q is ambiguous (%s): choose an id among %s", kind, name, res.Reason, refList(reg, res.Candidates))
		}
		for _, c := range res.Candidates {
			if sameRef(c, id) {
				return c, nil
			}
		}
		return entities.Ref{}, fmt.Errorf("id %q is not a candidate of %s %q (%s)", id, kind, name, refList(reg, res.Candidates))
	}
	hint := ""
	if kind == string(entities.KindUnlock) || kind == string(entities.KindLocation) {
		hint = "; ask the owner and propose_entity it"
	}
	return entities.Ref{}, fmt.Errorf("%s %q does not resolve (%s)%s", kind, name, res.Reason, hint)
}

// sameRef reports whether id names r, as a bare id or as the full ref.
func sameRef(r entities.Ref, id string) bool {
	return id == r.ID || id == r.String()
}

// refList prints refs for a message, with what tells same-name items apart
// ("item:2419 (Dyed), item:2421 (Grey)", "item:755 (Message (Romeo & Juliet))").
func refList(reg *entities.Registry, refs []entities.Ref) string {
	s := make([]string, len(refs))
	for i, r := range refs {
		s[i] = r.String()
		if v, page := variant(reg, r); v != "" || page != "" {
			s[i] += " (" + strings.TrimSpace(page+" "+v) + ")"
		}
	}
	return strings.Join(s, ", ")
}

// variant returns the catalog variant of an item ref and its wiki page when
// the page is not titled with the item's name; "" otherwise.
func variant(reg *entities.Registry, r entities.Ref) (v, page string) {
	if id, ok := r.Int(); ok && r.Kind == entities.KindItem {
		if it, ok := reg.Item(id); ok {
			if entities.NormalizeName(string(it.WikiKey)) != entities.NormalizeName(it.Name) {
				page = string(it.WikiKey)
			}
			return it.Variant, page
		}
	}
	return "", ""
}

// StepProposal is what the agent submits for one step: names, quotes and
// targets, never refs or spans. Submit resolves the names, locates each
// quote and builds the step annotation.
type StepProposal struct {
	StepID      string               `json:"step_id"`
	Objectives  []ObjectiveProposal  `json:"objectives,omitempty"`
	Items       []ItemProposal       `json:"items,omitempty"`
	BankActions []BankActionProposal `json:"bank_actions,omitempty"`
	Location    *LocationProposal    `json:"location,omitempty"`
	// Suppress drops rule objectives the step does not have.
	Suppress []SuppressProposal `json:"suppress,omitempty"`
	// NoBankActions drops the bank actions derived from the step's text.
	NoBankActions *NoBankActionsProposal `json:"no_bank_actions,omitempty"`
	// Comment goes to the report only: why the step has no objectives, why it
	// diverges from a rule, what was asked.
	Comment string `json:"comment,omitempty"`
}

// NoBankActionsProposal says the step has no bank action although derive
// read one from its text; it always needs the owner.
type NoBankActionsProposal struct {
	Reason      string `json:"reason"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	Decision    string `json:"decision,omitempty"`
}

// SuppressProposal names a rule objective of the step (Key and Role as
// next_batch lists it in rule_objectives) that the step does not have, with
// the reason. It always needs the owner: ConfirmedBy "owner" or a Decision.
type SuppressProposal struct {
	Key         string `json:"key"`
	Role        string `json:"role"`
	Reason      string `json:"reason"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	Decision    string `json:"decision,omitempty"`
}

// Evidence is the quote that supports a proposal. Field is a JSON pointer in
// the step (default /text): /location, /notes/<i>/text or /items/<i>.
type Evidence struct {
	Field string `json:"field,omitempty"`
	Quote string `json:"quote"`
}

// Source is how sure the agent is: high when the text says it, medium when
// it was inferred, which needs the owner's confirmation (ConfirmedBy
// "owner") or a recorded decision that covers it (Decision).
type Source struct {
	Confidence  string `json:"confidence"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	Decision    string `json:"decision,omitempty"`
}

// ObjectiveProposal names an objective target. Kind is quest, skill, diary,
// boss, item, unlock, ca or qp; Name is the entity name (the area for a
// diary); ID chooses among the candidates of an ambiguous name. State is
// "started" or "finished" for a quest; N is the level of a skill, the kill
// count of a boss, the quantity of an item and the quest points of qp.
type ObjectiveProposal struct {
	Role  string `json:"role"`
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	ID    string `json:"id,omitempty"`
	Tier  string `json:"tier,omitempty"`
	State string `json:"state,omitempty"`
	N     int    `json:"n,omitempty"`
	Evidence
	Source
}

// ItemProposal is an item requirement. Name is the item as the step says it
// (a "noted" prefix sets Noted). Exactly one way to resolve it applies: ID
// among the candidates of an ambiguous name, AnyOf (item names, or candidate
// ids of Name, in preference order; each must reach Qty alone), Family (a
// family name or slug: every dose and charge counts), or Name alone when it
// resolves to one item.
type ItemProposal struct {
	Name     string   `json:"name"`
	ID       string   `json:"id,omitempty"`
	AnyOf    []string `json:"any_of,omitempty"`
	Family   string   `json:"family,omitempty"`
	Qty      int      `json:"qty,omitempty"`
	Noted    bool     `json:"noted,omitempty"`
	Optional bool     `json:"optional,omitempty"`
	Slot     string   `json:"slot,omitempty"`
	Evidence
	Source
}

// BankActionProposal is a bank action the text asks for.
type BankActionProposal struct {
	Type     string `json:"type"`
	Position string `json:"position"`
	Evidence
	Source
}

// LocationProposal is where the step happens.
type LocationProposal struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
	Evidence
	Source
}

// build resolves a proposal into the annotation the loader reads. Every
// problem is returned; the annotation is only usable when there are none.
func build(reg *entities.Registry, st *content.Step, p StepProposal, skillVersion string) (content.StepAnnotation, []string) {
	sa := content.StepAnnotation{StepID: st.ID, TextHash: content.StepTextHash(st)}
	var errs []string
	fail := func(where string, err error) { errs = append(errs, where+": "+err.Error()) }
	for i, op := range p.Objectives {
		where := fmt.Sprintf("objectives[%d]", i)
		key, err := objectiveKey(reg, op)
		if err != nil {
			fail(where, err)
			continue
		}
		ev, src, err := provenance(st, op.Evidence, op.Source, skillVersion)
		if err != nil {
			fail(where, err)
			continue
		}
		sa.Objectives = append(sa.Objectives, content.Objective{Key: key, Role: op.Role, Evidence: &ev, Source: src})
	}
	for i, sp := range p.Suppress {
		where := fmt.Sprintf("suppress[%d]", i)
		key, err := entities.ParseObjectiveKey(sp.Key)
		if err != nil {
			fail(where, fmt.Errorf("key must be a rule objective key from next_batch: %w", err))
			continue
		}
		src, err := ownerSource(sp.ConfirmedBy, sp.Decision, skillVersion)
		if err != nil {
			fail(where, err)
			continue
		}
		sa.Suppressed = append(sa.Suppressed, content.SuppressedObjective{Key: key, Role: sp.Role, Reason: strings.TrimSpace(sp.Reason), Source: src})
	}
	if nb := p.NoBankActions; nb != nil {
		if src, err := ownerSource(nb.ConfirmedBy, nb.Decision, skillVersion); err != nil {
			fail("no_bank_actions", err)
		} else {
			sa.NoBankActions = &content.NoBankActions{Reason: strings.TrimSpace(nb.Reason), Source: src}
		}
	}
	for i, ip := range p.Items {
		where := fmt.Sprintf("items[%d]", i)
		req, err := itemRequirement(reg, ip)
		if err != nil {
			fail(where, err)
			continue
		}
		ev, src, err := provenance(st, ip.Evidence, ip.Source, skillVersion)
		if err != nil {
			fail(where, err)
			continue
		}
		req.SourceText = ev.Quote
		sa.Items = append(sa.Items, content.AnnotatedItem{ItemRequirement: req, Slot: ip.Slot, Evidence: ev, Source: src})
	}
	for i, bp := range p.BankActions {
		where := fmt.Sprintf("bank_actions[%d]", i)
		ev, src, err := provenance(st, bp.Evidence, bp.Source, skillVersion)
		if err != nil {
			fail(where, err)
			continue
		}
		sa.BankActions = append(sa.BankActions, content.AnnotatedBankAction{Type: bp.Type, Position: bp.Position, Evidence: ev, Source: src})
	}
	if lp := p.Location; lp != nil {
		ref, err := pick(reg, reg.ResolveLocation(lp.Name), string(entities.KindLocation), lp.Name, lp.ID)
		if err != nil {
			fail("location", err)
		} else if ev, src, err := provenance(st, lp.Evidence, lp.Source, skillVersion); err != nil {
			fail("location", err)
		} else {
			sa.Location = &content.AnnotatedLocation{Ref: ref, Evidence: ev, Source: src}
		}
	}
	return sa, errs
}

// objectiveKey resolves the target of an objective proposal into a key.
func objectiveKey(reg *entities.Registry, op ObjectiveProposal) (entities.ObjectiveKey, error) {
	if op.Kind == KindQuestPoints {
		return entities.ParseObjectiveKey(fmt.Sprintf("%s>=%d", entities.QuestPoints, op.N))
	}
	if op.Name == "" {
		return entities.ObjectiveKey{}, fmt.Errorf("%s objectives need a name", op.Kind)
	}
	res, noted := resolveName(reg, op.Kind, op.Name, op.Tier)
	if noted {
		return entities.ObjectiveKey{}, fmt.Errorf("item objectives count the item, not its noted form: drop %q", "noted")
	}
	ref, err := pick(reg, res, op.Kind, op.Name, op.ID)
	if err != nil {
		return entities.ObjectiveKey{}, err
	}
	var s string
	switch ref.Kind {
	case entities.KindQuest:
		switch op.State {
		case "started":
			s = ref.String() + "@" + entities.StateStarted
		case "finished":
			s = ref.String() + "@" + entities.StateFinished
		default:
			return entities.ObjectiveKey{}, fmt.Errorf("quest objectives need state started or finished, got %q", op.State)
		}
	case entities.KindSkill, entities.KindBoss:
		s = fmt.Sprintf("%s>=%d", ref, op.N)
	case entities.KindItem:
		s = fmt.Sprintf("%sx%d", ref, op.N)
	case entities.KindDiary:
		s = ref.String() + "@" + entities.StateComplete
	default:
		s = ref.String() + "@" + entities.StateDone
	}
	return entities.ParseObjectiveKey(s)
}

// itemRequirement resolves an item proposal into a requirement.
func itemRequirement(reg *entities.Registry, ip ItemProposal) (content.ItemRequirement, error) {
	phrase := entities.ParseItemPhrase(ip.Name)
	req := content.ItemRequirement{Name: ip.Name, Noted: ip.Noted || phrase.Noted, Optional: ip.Optional}
	if ip.Qty > 0 {
		q := ip.Qty
		req.Qty = &q
	} else if ip.Qty < 0 {
		return req, fmt.Errorf("qty must be positive")
	}
	ways := 0
	for _, set := range []bool{ip.ID != "", len(ip.AnyOf) > 0, ip.Family != ""} {
		if set {
			ways++
		}
	}
	if ways > 1 {
		return req, fmt.Errorf("set only one of id, any_of and family")
	}
	res, _ := reg.ResolveItem(phrase.Name)
	switch {
	case ip.Family != "":
		slug := familySlug(reg, ip.Family)
		if slug == "" {
			return req, fmt.Errorf("unknown item family %q", ip.Family)
		}
		req.Family = slug
	case len(ip.AnyOf) > 0:
		for _, alt := range ip.AnyOf {
			id, err := anyOfID(reg, res, phrase.Name, alt)
			if err != nil {
				return req, err
			}
			if slices.Contains(req.AnyOf, id) {
				return req, fmt.Errorf("any_of lists item %d twice", id)
			}
			req.AnyOf = append(req.AnyOf, id)
		}
	default:
		ref, err := pick(reg, res, string(entities.KindItem), phrase.Name, ip.ID)
		if err != nil {
			hint := ""
			if res.Status == entities.Ambiguous {
				hint = " (or use any_of / family)"
			}
			return req, fmt.Errorf("%w%s", err, hint)
		}
		id, _ := ref.Int()
		req.ItemID = &id
	}
	if req.Noted {
		ids := req.AnyOf
		if req.ItemID != nil {
			ids = []int{*req.ItemID}
		}
		for _, id := range ids {
			if _, ok := reg.NotedID(id); !ok {
				if f, known := reg.Item(id); !known || !f.Noted {
					return req, fmt.Errorf("item %d has no noted form", id)
				}
			}
		}
	}
	return req, nil
}

// anyOfID resolves one any_of entry: an item name that resolves to one item,
// or an id (bare or as an item ref) among the candidates of the proposal's own
// name.
func anyOfID(reg *entities.Registry, own entities.Resolution, ownName, alt string) (int, error) {
	if id, err := strconv.Atoi(strings.TrimPrefix(alt, string(entities.KindItem)+":")); err == nil {
		alt = strconv.Itoa(id)
		cands := own.Candidates
		if own.Status == entities.Resolved {
			cands = []entities.Ref{own.Ref}
		}
		for _, c := range cands {
			if c.ID == alt {
				return id, nil
			}
		}
		return 0, fmt.Errorf("any_of id %d is not a candidate of %q (%s); name the item instead", id, ownName, refList(reg, cands))
	}
	res, _ := reg.ResolveItem(entities.ParseItemPhrase(alt).Name)
	ref, err := pick(reg, res, string(entities.KindItem), alt, "")
	if err != nil {
		return 0, fmt.Errorf("any_of: %w", err)
	}
	id, _ := ref.Int()
	return id, nil
}

// ownerSource is the source of an entry that only the owner can decide
// (a suppressed rule objective, dropped bank actions): always medium, with
// confirmed_by owner implied by a decision.
func ownerSource(confirmedBy, decision, skillVersion string) (content.ObjectiveSource, error) {
	if confirmedBy != "" && confirmedBy != content.ConfirmedByOwner {
		return content.ObjectiveSource{}, fmt.Errorf("confirmed_by can only be %q", content.ConfirmedByOwner)
	}
	src := content.ObjectiveSource{Method: content.ProvenanceLLM, Confidence: content.ConfidenceMedium, SkillVersion: skillVersion,
		ConfirmedBy: confirmedBy, Decision: decision}
	if decision != "" {
		src.ConfirmedBy = content.ConfirmedByOwner
	}
	return src, nil
}

// provenance locates a quote and fills the source of an llm proposal.
func provenance(st *content.Step, ev Evidence, src Source, skillVersion string) (content.ObjectiveEvidence, content.ObjectiveSource, error) {
	field := ev.Field
	if field == "" {
		field = "/text"
	}
	if strings.TrimSpace(ev.Quote) == "" {
		return content.ObjectiveEvidence{}, content.ObjectiveSource{}, fmt.Errorf("a quote of the step is required")
	}
	span, err := content.QuoteSpan(st, field, ev.Quote)
	if err != nil {
		return content.ObjectiveEvidence{}, content.ObjectiveSource{}, err
	}
	switch src.Confidence {
	case content.ConfidenceHigh, content.ConfidenceMedium:
	default:
		return content.ObjectiveEvidence{}, content.ObjectiveSource{}, fmt.Errorf("confidence must be high (the text says it) or medium (inferred), got %q", src.Confidence)
	}
	if src.ConfirmedBy != "" && src.ConfirmedBy != content.ConfirmedByOwner {
		return content.ObjectiveEvidence{}, content.ObjectiveSource{}, fmt.Errorf("confirmed_by can only be %q", content.ConfirmedByOwner)
	}
	out := content.ObjectiveSource{Method: content.ProvenanceLLM, Confidence: src.Confidence, SkillVersion: skillVersion,
		ConfirmedBy: src.ConfirmedBy, Decision: src.Decision}
	if src.Decision != "" {
		out.ConfirmedBy = content.ConfirmedByOwner
	}
	return content.ObjectiveEvidence{Field: field, Quote: ev.Quote, Span: span}, out, nil
}
