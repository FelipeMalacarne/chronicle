package httpapi

import (
	"net/http"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
)

// worldLink is one requirement of an entity, or one entity it unlocks, with
// the account's verdict on that objective.
type worldLink struct {
	Key         string `json:"key"`
	Ref         string `json:"ref,omitempty"`
	Label       string `json:"label"`
	Verdict     string `json:"verdict"`
	Boostable   bool   `json:"boostable,omitempty"`
	DuringQuest bool   `json:"during_quest,omitempty"`
}

// worldEntityDetail is one entity for the entity panel (V6Entidade): the
// world entity plus its direct requirements and what directly requires it.
// Only quests carry requirements in the catalog today; unlocks are the quests
// whose requirements name this entity.
type worldEntityDetail struct {
	worldEntity
	Requires    []worldLink `json:"requires"`
	CombatLevel int         `json:"combat_level,omitempty"`
	Unlocks     []worldLink `json:"unlocks"`
}

// maxUnlocks caps "destrava": a skill such as Agility unlocks dozens of quests.
const maxUnlocks = 12

func (s *Server) handleWorldEntity(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		writeError(w, http.StatusBadRequest, "ref is required")
		return
	}
	var parsed entities.Ref
	if ref != entities.QuestPoints {
		p, err := entities.ParseObjectiveRef(ref)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ref: "+err.Error())
			return
		}
		parsed = p
	}
	verdicts, err := s.verdicts(r, account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	out := worldEntityDetail{worldEntity: s.worldEntity(ref, verdicts), Requires: []worldLink{}, Unlocks: []worldLink{}}
	catalog := s.opts.Catalog
	if catalog == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if parsed.Kind == entities.KindQuest {
		if q, ok := catalog.Quest(parsed.ID); ok {
			out.CombatLevel = q.CombatLevel
			for _, req := range q.Requires {
				out.Requires = append(out.Requires, s.worldLink(req.Key, verdicts, req.Boostable, req.DuringQuest))
			}
		}
	}
	for _, id := range catalog.QuestIDs() {
		if len(out.Unlocks) == maxUnlocks {
			break
		}
		q, _ := catalog.Quest(id)
		for _, req := range q.Requires {
			if entityOf(req.Key) != ref {
				continue
			}
			key := entities.ObjectiveKey{Ref: entities.Ref{Kind: entities.KindQuest, ID: id}, Op: entities.OpState, State: "finished"}
			out.Unlocks = append(out.Unlocks, s.worldLink(key, verdicts, false, false))
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) worldLink(k entities.ObjectiveKey, verdicts map[string]storeprogress.ObjectiveView, boostable, during bool) worldLink {
	key := k.String()
	l := worldLink{Key: key, Ref: entityOf(k), Label: k.Ref.ID, Verdict: "unknown", Boostable: boostable, DuringQuest: during}
	if k.IsQuestPoints() {
		l.Label = "quest points"
	} else if name, ok := s.opts.Catalog.Display(k.Ref); ok {
		l.Label = name
	}
	if v, ok := verdicts[key]; ok && v.Verdict != "" {
		l.Verdict = v.Verdict
	}
	return l
}
