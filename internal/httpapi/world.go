package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// worldObjective is one objective of an entity with the account's verdict.
type worldObjective struct {
	Key       string `json:"key"`
	Verdict   string `json:"verdict"`
	Regressed bool   `json:"regressed,omitempty"`
}

// worldEntity is an entity the routes touch (the Mundo, V4Mundo): its
// objectives and where each route does them.
type worldEntity struct {
	Ref        string           `json:"ref"`
	Kind       string           `json:"kind"`
	Name       string           `json:"name"`
	Wiki       string           `json:"wiki,omitempty"`
	Objectives []worldObjective `json:"objectives"`
	Routes     []occurrence     `json:"routes"`
}

type worldResponse struct {
	Account  string        `json:"account"`
	Entities []worldEntity `json:"entities"`
}

// maxRouteOccurrences caps "onde cada rota faz isso" per guide: items such as
// coins appear in dozens of steps.
const maxRouteOccurrences = 6

func (s *Server) handleWorld(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	verdicts, err := s.verdicts(r, account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	out := worldResponse{Account: account, Entities: make([]worldEntity, 0, len(s.index.entities))}
	for _, entity := range s.index.entities {
		out.Entities = append(out.Entities, s.worldEntity(entity, verdicts))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) worldEntity(entity string, verdicts map[string]storeprogress.ObjectiveView) worldEntity {
	e := worldEntity{Ref: entity, Kind: entities.QuestPoints, Name: "quest points", Objectives: []worldObjective{}, Routes: []occurrence{}}
	if entity != entities.QuestPoints {
		if ref, err := entities.ParseObjectiveRef(entity); err == nil {
			e.Kind, e.Name = string(ref.Kind), ref.ID
			if s.opts.Catalog != nil {
				if name, ok := s.opts.Catalog.Display(ref); ok {
					e.Name = name
				}
				if url, ok := s.opts.Catalog.WikiURL(ref); ok {
					e.Wiki = url
				}
			}
		}
	}
	keys := map[string]bool{}
	perGuide := map[string]int{}
	for _, occ := range s.index.touched[entity] {
		keys[occ.Key] = true
		if perGuide[occ.Guide] < maxRouteOccurrences {
			perGuide[occ.Guide]++
			e.Routes = append(e.Routes, occ)
		}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		v := verdicts[k]
		verdict := v.Verdict
		if verdict == "" {
			verdict = "unknown"
		}
		e.Objectives = append(e.Objectives, worldObjective{Key: k, Verdict: verdict, Regressed: v.RegressedAt != nil})
	}
	return e
}

func (s *Server) verdicts(r *http.Request, account string) (map[string]storeprogress.ObjectiveView, error) {
	list, err := s.opts.Progress.Objectives(r.Context(), account)
	if err != nil {
		return nil, err
	}
	out := make(map[string]storeprogress.ObjectiveView, len(list))
	for _, o := range list {
		out[o.Key] = o
	}
	return out, nil
}

// goalOut is a goal with its display name and where each route completes it.
type goalOut struct {
	storeprogress.GoalView
	Name   string       `json:"name"`
	Ref    string       `json:"ref"`
	Kind   string       `json:"kind"`
	Routes []occurrence `json:"routes"`
}

type goalsResponse struct {
	Account string    `json:"account"`
	Goals   []goalOut `json:"goals"`
}

type addGoalRequest struct {
	ObjectiveKey string `json:"objective_key"`
	Label        string `json:"label,omitempty"`
}

func (s *Server) goalOut(g storeprogress.GoalView) goalOut {
	out := goalOut{GoalView: g, Routes: []occurrence{}}
	k, err := entities.ParseObjectiveKey(g.Key)
	if err != nil {
		return out
	}
	entity := entityOf(k)
	e := s.worldEntity(entity, nil)
	out.Name, out.Ref, out.Kind = e.Name, entity, e.Kind
	if out.Label == "" {
		out.Label = e.Name
	}
	out.Routes = append(out.Routes, s.index.byEntity[entity]...)
	return out
}

func (s *Server) handleGoals(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	goals, err := s.opts.Progress.Goals(r.Context(), account)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	out := goalsResponse{Account: account, Goals: make([]goalOut, 0, len(goals))}
	for _, g := range goals {
		out.Goals = append(out.Goals, s.goalOut(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAddGoal(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	var req addGoalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	g, err := s.opts.Progress.AddGoal(r.Context(), account, strings.TrimSpace(req.ObjectiveKey), strings.TrimSpace(req.Label))
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.goalOut(g))
}

func (s *Server) handleRemoveGoal(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	if err := s.opts.Progress.RemoveGoal(r.Context(), account, r.PathValue("id")); err != nil {
		s.writeProgressError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// goalPlanResponse is a goal (with its names and routes) and its plan.
type goalPlanResponse struct {
	Account string                `json:"account"`
	Goal    goalOut               `json:"goal"`
	Plan    coreprogress.GoalPlan `json:"plan"`
}

func (s *Server) handleGoalPlan(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	v, err := s.opts.Progress.GoalPlan(r.Context(), account, r.PathValue("id"))
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, goalPlanResponse{Account: account, Goal: s.goalOut(v.Goal), Plan: v.Plan})
}

func isGoalError(err error) (int, bool) {
	switch {
	case errors.Is(err, storeprogress.ErrInvalidGoal):
		return http.StatusBadRequest, true
	case errors.Is(err, storeprogress.ErrGoalNotFound):
		return http.StatusNotFound, true
	}
	return 0, false
}

// handleGraph is GET /api/graph?account=&focus=<objective key>&depth=1..3:
// the focus graph of Mundo › Grafo (V6Grafo1440/640, F10).
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	account, ok := s.accountParam(w, r)
	if !ok {
		return
	}
	focus, err := entities.ParseObjectiveKey(strings.TrimSpace(r.URL.Query().Get("focus")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "focus must be an objective key (quest:dragon_slayer_i@finished, skill:agility>=50, ...)")
		return
	}
	depth := coreprogress.DefaultGraphDepth
	if raw := r.URL.Query().Get("depth"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 1 || n > coreprogress.MaxGraphDepth {
			writeError(w, http.StatusBadRequest, "depth must be between 1 and "+strconv.Itoa(coreprogress.MaxGraphDepth))
			return
		}
		depth = n
	}
	g, err := s.opts.Progress.Graph(r.Context(), account, focus, depth)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// maxFleetKeys caps the keys= of GET /api/fleet/goals.
const maxFleetKeys = 50

// fleetGoalOut is one objective of the Mundo da frota with its display name
// and where each route completes it (the first step of each, as goalOut).
type fleetGoalOut struct {
	coreprogress.FleetGoal
	Name   string       `json:"name"`
	Ref    string       `json:"ref"`
	Kind   string       `json:"kind"`
	Wiki   string       `json:"wiki,omitempty"`
	Routes []occurrence `json:"routes"`
}

// fleetGoalsResponse is GET /api/fleet/goals: the user's accounts, in the
// order of GET /api/me, and the objectives with each account's standing in
// that order.
type fleetGoalsResponse struct {
	Accounts []storeprogress.AccountView `json:"accounts"`
	Goals    []fleetGoalOut              `json:"goals"`
}

// handleFleetGoals is GET /api/fleet/goals[?keys=<objective key>,...]: the
// Mundo da frota (V7Mundo1440, slice V10 of docs/ui/decisoes-canvas.md
// section 7.5) in one request. Every goal one of the user's accounts pinned,
// then the asked keys, each with every account's stored verdict, its goal
// and a summary of its plan (internal/core/progress.BuildFleetGoals). The
// accounts are only the ones the user owns, as /api/me lists them
// (CLAUDE.md rule 10); keys= is repeatable and comma-separated.
func (s *Server) handleFleetGoals(w http.ResponseWriter, r *http.Request) {
	var keys []string
	for _, raw := range r.URL.Query()["keys"] {
		for k := range strings.SplitSeq(raw, ",") {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
	}
	if len(keys) > maxFleetKeys {
		writeError(w, http.StatusBadRequest, "at most "+strconv.Itoa(maxFleetKeys)+" keys")
		return
	}
	user, ok := s.requestUser(w, r)
	if !ok {
		return
	}
	accounts, ok := s.userAccounts(w, r, user)
	if !ok {
		return
	}
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	goals, err := s.opts.Progress.FleetGoals(r.Context(), ids, keys)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}
	out := fleetGoalsResponse{Accounts: accounts, Goals: make([]fleetGoalOut, 0, len(goals))}
	for _, g := range goals {
		o := fleetGoalOut{FleetGoal: g, Routes: []occurrence{}}
		if k, parseErr := entities.ParseObjectiveKey(g.Key); parseErr == nil {
			entity := entityOf(k)
			e := s.worldEntity(entity, nil)
			o.Name, o.Ref, o.Kind, o.Wiki = e.Name, entity, e.Kind, e.Wiki
			o.Routes = append(o.Routes, s.index.byEntity[entity]...)
		}
		out.Goals = append(out.Goals, o)
	}
	writeJSON(w, http.StatusOK, out)
}
