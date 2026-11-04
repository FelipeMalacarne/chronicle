// Package enrich attaches canonical entity refs to imported guide steps.
//
// Only exact, deterministic mappings are written into the guide
// (Step.Entities, ItemRequirement.ItemID). Everything else — ambiguous names,
// names missing from the catalogs, bare quest names without quest context —
// goes to a machine-readable Report so it can be curated, never guessed.
// Enrichment reads the source fields but never rewrites them, and it never
// merges or reorders guide routes.
package enrich

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

// ReportFileName is written next to guide.json by the importers.
const ReportFileName = "entities.report.json"

// KindLink is the report kind of a wiki link whose entity kind is not known
// until it resolves.
const KindLink = "link"

// Report summarizes one enrichment run.
type Report struct {
	GuideID      string            `json:"guide_id"`
	GuideVersion string            `json:"guide_version,omitempty"`
	Catalogs     map[string]string `json:"catalogs"`
	Counts       Counts            `json:"counts"`
	Ambiguous    []Entry           `json:"ambiguous"`
	Unresolved   []Entry           `json:"unresolved"`
}

// Counts are totals over every examined reference, overall and per kind.
// Loadouts counts loadout items separately: they repeat step items.
type Counts struct {
	Tally
	ByKind   map[string]Tally `json:"by_kind"`
	Loadouts Tally            `json:"loadout_items"`
}

// Tally counts outcomes.
type Tally struct {
	Resolved   int `json:"resolved"`
	Ambiguous  int `json:"ambiguous"`
	Unresolved int `json:"unresolved"`
}

func (t *Tally) add(s entities.Status) {
	switch s {
	case entities.Resolved:
		t.Resolved++
	case entities.Ambiguous:
		t.Ambiguous++
	default:
		t.Unresolved++
	}
}

// Entry is one ambiguous or unresolved reference with its evidence.
type Entry struct {
	StepID     string         `json:"step_id"`
	Field      string         `json:"field"`
	Span       []int          `json:"span,omitempty"`
	SourceText string         `json:"source_text"`
	Kind       string         `json:"kind"`
	Method     string         `json:"method"`
	Reason     string         `json:"reason"`
	Candidates []entities.Ref `json:"candidates,omitempty"`
}

// Guide enriches every step and loadout of g in place and returns the report.
// It is idempotent: Step.Entities is recomputed from the source fields.
func Guide(g *content.Guide, reg *entities.Registry) *Report {
	rep := &Report{
		GuideID:      g.ID,
		GuideVersion: g.Version,
		Catalogs:     map[string]string{},
		Counts:       Counts{ByKind: map[string]Tally{}},
		Ambiguous:    []Entry{},
		Unresolved:   []Entry{},
	}
	for k, v := range reg.Versions {
		rep.Catalogs[string(k)] = v
	}
	g.Index()
	for _, st := range g.Steps() {
		e := stepEnricher{reg: reg, rep: rep, step: st}
		st.Entities = nil
		e.rules("/complete_when", st.CompleteWhen)
		e.rules("/prerequisites", st.Prerequisites)
		for i, it := range st.Items {
			res, _ := reg.ResolveItem(it.Name)
			e.record(string(entities.KindItem), fmt.Sprintf("/items/%d", i), nil, it.Name, content.MentionItem, res)
		}
		for i, l := range st.Links {
			if !isWikiURL(l.URL) {
				continue
			}
			res, _ := reg.ResolveWikiURL(l.URL)
			e.record(KindLink, fmt.Sprintf("/links/%d/url", i), nil, l.URL, content.MentionLink, res)
		}
		e.text()
	}
	for _, seg := range g.Segments() {
		if seg.Loadout == nil {
			continue
		}
		for _, list := range [][]content.ItemRequirement{seg.Loadout.Inventory, seg.Loadout.Equipment} {
			for i := range list {
				it := &list[i]
				text := it.SourceText
				if text == "" {
					text = it.Name
				}
				res, _ := reg.ResolveItem(text)
				rep.Counts.Loadouts.add(res.Status)
				if res.Status == entities.Resolved && it.ItemID == nil {
					id, _ := res.Ref.Int()
					it.ItemID = &id
				}
			}
		}
	}
	return rep
}

type stepEnricher struct {
	reg  *entities.Registry
	rep  *Report
	step *content.Step
}

func (e *stepEnricher) record(kind, field string, span []int, text, method string, res entities.Resolution) {
	t := e.rep.Counts.ByKind[kind]
	if res.Status == entities.Resolved {
		kind = string(res.Ref.Kind)
		t = e.rep.Counts.ByKind[kind]
	}
	t.add(res.Status)
	e.rep.Counts.ByKind[kind] = t
	e.rep.Counts.add(res.Status)
	switch res.Status {
	case entities.Resolved:
		e.step.Entities = append(e.step.Entities, content.EntityMention{
			Ref: res.Ref, SourceText: text, Field: field, Span: span, Method: method, Match: string(res.Match),
		})
	default:
		entry := Entry{
			StepID: e.step.ID, Field: field, Span: span, SourceText: text,
			Kind: kind, Method: method, Reason: res.Reason, Candidates: res.Candidates,
		}
		if res.Status == entities.Ambiguous {
			e.rep.Ambiguous = append(e.rep.Ambiguous, entry)
		} else {
			e.rep.Unresolved = append(e.rep.Unresolved, entry)
		}
	}
}

func (e *stepEnricher) rules(ptr string, rules content.RuleList) {
	for i, r := range rules {
		e.rule(fmt.Sprintf("%s/%d", ptr, i), r)
	}
}

func (e *stepEnricher) rule(ptr string, r content.Rule) {
	reg := e.reg
	switch v := r.(type) {
	case content.AllRule:
		e.rules(ptr+"/all", v.All)
	case content.AnyRule:
		e.rules(ptr+"/any", v.Any)
	case content.NotRule:
		e.rule(ptr+"/not", v.Not)
	case content.QuestStateRule:
		e.record(string(entities.KindQuest), ptr+"/quest", nil, v.Quest, content.MentionRule, reg.ResolveQuest(v.Quest))
	case content.SkillLevelRule:
		e.record(string(entities.KindSkill), ptr+"/skill", nil, v.Skill, content.MentionRule, reg.ResolveSkill(v.Skill))
	case content.SkillXPRule:
		e.record(string(entities.KindSkill), ptr+"/skill", nil, v.Skill, content.MentionRule, reg.ResolveSkill(v.Skill))
	case content.DiaryTierRule:
		e.diary(ptr, v.Area, v.Tier)
	case content.DiaryTaskRule:
		e.diary(ptr, v.Area, v.Tier)
	case content.CombatAchievementRule:
		if v.Name != "" {
			e.record(string(entities.KindCombatAchievement), ptr+"/name", nil, v.Name, content.MentionRule, reg.ResolveCombatAchievement(v.Name))
		}
	case content.BossKCRule:
		e.record(string(entities.KindBoss), ptr+"/boss", nil, v.Boss, content.MentionRule, entities.Resolution{Status: entities.Unresolved, Reason: entities.ReasonNoCatalog})
	case content.ItemOwnedRule:
		e.ruleItem(ptr, v.ItemID, v.ItemName)
	case content.CollectionLogItemRule:
		e.ruleItem(ptr, v.ItemID, v.ItemName)
	}
}

func (e *stepEnricher) diary(ptr, area, tier string) {
	res := e.reg.ResolveDiary(area, tier)
	if res.Reason == entities.ReasonAggregate {
		return // "all areas" is a goal over many diaries, not one entity
	}
	e.record(string(entities.KindDiary), ptr, nil, area+" "+tier, content.MentionRule, res)
}

func (e *stepEnricher) ruleItem(ptr string, id *int, name string) {
	if id != nil {
		e.record(string(entities.KindItem), ptr+"/item_id", nil, strconv.Itoa(*id), content.MentionRule, e.reg.ResolveItemID(*id))
		return
	}
	if name != "" {
		res, _ := e.reg.ResolveItem(name)
		e.record(string(entities.KindItem), ptr+"/item_name", nil, name, content.MentionRule, res)
	}
}

func (e *stepEnricher) text() {
	mentions := append(e.reg.FindQuestMentions(e.step.Text), e.reg.FindDiaryMentions(e.step.Text)...)
	sort.SliceStable(mentions, func(i, j int) bool { return mentions[i].Start < mentions[j].Start })
	for _, m := range mentions {
		kind := string(entities.KindQuest)
		if len(m.Resolution.Candidates) > 0 {
			kind = string(m.Resolution.Candidates[0].Kind)
		}
		if strings.HasSuffix(strings.ToLower(m.Text), "diary") || strings.HasSuffix(strings.ToLower(m.Text), "diaries") {
			kind = string(entities.KindDiary)
		}
		e.record(kind, "/text", []int{m.Start, m.End}, m.Text, content.MentionText, m.Resolution)
	}
}

func isWikiURL(u string) bool {
	return strings.Contains(strings.ToLower(u), "://oldschool.runescape.wiki/")
}

// Encode renders the report as deterministic JSON (2-space indent, no HTML
// escaping, trailing newline).
func (r *Report) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
