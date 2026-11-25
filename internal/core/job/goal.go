package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	"github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Goal is the rule that proves a job ended, evaluated by the rule engine on
// the account's facts (docs/12 section 13.2). It has three JSON forms:
//
//   - a string, an objective key ("skill:mining>=15", "quest:x@finished");
//   - an object with a "type" (or all/any/not): a content rule
//     ({"type": "varp", "id": 281, "min": 1000});
//   - {"task": {...}}: a typed task whose termination (a position in an area,
//     a loadout ready) the panel evaluates, for the tasks no rule states.
type Goal struct {
	raw  json.RawMessage
	key  *entities.ObjectiveKey
	rule content.Rule
	task *content.Task
}

// ParseGoal reads and checks a goal.
func ParseGoal(raw []byte) (Goal, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return Goal{}, errors.New("goal: empty")
	}
	g := Goal{raw: append(json.RawMessage(nil), raw...)}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return Goal{}, fmt.Errorf("goal: %w", err)
		}
		k, err := entities.ParseObjectiveKey(s)
		if err != nil {
			return Goal{}, fmt.Errorf("goal: %w", err)
		}
		if k.String() != s {
			return Goal{}, fmt.Errorf("goal: write the key as %s", k)
		}
		g.key = &k
		return g, nil
	}
	var probe struct {
		Task json.RawMessage `json:"task"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return Goal{}, fmt.Errorf("goal: %w", err)
	}
	if probe.Task != nil {
		var t content.Task
		if err := json.Unmarshal(probe.Task, &t); err != nil {
			return Goal{}, fmt.Errorf("goal: task: %w", err)
		}
		if _, ok := t.Ends(); !ok {
			return Goal{}, errors.New("goal: the task says no termination")
		}
		g.task = &t
		return g, nil
	}
	r, err := content.DecodeRule(raw)
	if err != nil {
		return Goal{}, fmt.Errorf("goal: %w", err)
	}
	g.rule = r
	return g, nil
}

// ObjectiveGoal is the goal "the objective key is met".
func ObjectiveGoal(k entities.ObjectiveKey) Goal {
	raw, _ := json.Marshal(k.String())
	return Goal{raw: raw, key: &k}
}

// TaskGoal is the goal "the task's termination is met".
func TaskGoal(t content.Task) (Goal, error) {
	raw, err := json.Marshal(map[string]any{"task": t})
	if err != nil {
		return Goal{}, err
	}
	return ParseGoal(raw)
}

// SaleGoal is a Grand Exchange sale's goal (docs/12 section 6.2): the live
// inventory holds none of the item and at least coinsAtLeast coins. The
// rules prove absence only on a live container with a full baseline, so
// anything less than that is unknown, never met.
func SaleGoal(itemID int, coinsAtLeast int64) (Goal, error) {
	if itemID < 1 || coinsAtLeast < 1 {
		return Goal{}, errors.New("goal: a sale has an item and the coins it pays")
	}
	inv := []string{rules.ContainerInventory}
	coins := entities.CoinsID
	raw, err := json.Marshal(content.AllRule{All: content.RuleList{
		content.NotRule{Not: content.ItemOwnedRule{RuleHeader: content.RuleHeader{Type: content.TypeItemOwned}, ItemID: &itemID, Qty: 1, Containers: inv}},
		content.ItemOwnedRule{RuleHeader: content.RuleHeader{Type: content.TypeItemOwned}, ItemID: &coins, Qty: int(coinsAtLeast), Containers: inv},
	}})
	if err != nil {
		return Goal{}, err
	}
	return ParseGoal(raw)
}

// MarshalJSON is the goal as it was parsed.
func (g Goal) MarshalJSON() ([]byte, error) {
	if g.IsZero() {
		return []byte("null"), nil
	}
	return g.raw, nil
}

// UnmarshalJSON parses and checks the goal.
func (g *Goal) UnmarshalJSON(data []byte) error {
	p, err := ParseGoal(data)
	if err != nil {
		return err
	}
	*g = p
	return nil
}

// JSON is the goal as it is stored.
func (g Goal) JSON() json.RawMessage { return g.raw }

// IsZero reports whether the goal was never parsed.
func (g Goal) IsZero() bool { return len(g.raw) == 0 }

// ObjectiveKey is the goal's key when it is one: the form the agent's
// run_until carries.
func (g Goal) ObjectiveKey() (entities.ObjectiveKey, bool) {
	if g.key == nil {
		return entities.ObjectiveKey{}, false
	}
	return *g.key, true
}

// Metric is the metric a goal implies: a skill's XP up to the level, a varp
// or varbit up to its minimum or value. Nil for a goal no single number
// measures.
func (g Goal) Metric() *Metric {
	switch {
	case g.key != nil && g.key.Ref.Kind == entities.KindSkill && !g.key.IsQuestPoints():
		return &Metric{Kind: MetricSkillXP, Skill: g.key.Ref.ID, Target: int64(entities.XPForLevel(g.key.N))}
	case g.rule != nil:
		switch v := g.rule.(type) {
		case content.VarpRule:
			return varMetric(MetricVarp, v.ID, v.Eq, v.Min)
		case content.VarbitRule:
			return varMetric(MetricVarbit, v.ID, v.Eq, v.Min)
		}
	}
	return nil
}

func varMetric(kind string, id int, eq, atLeast *int) *Metric {
	switch {
	case atLeast != nil:
		return &Metric{Kind: kind, ID: id, Target: int64(*atLeast)}
	case eq != nil:
		return &Metric{Kind: kind, ID: id, Target: int64(*eq)}
	}
	return nil
}

// Env is what a goal is evaluated on.
type Env struct {
	Facts    panel.Facts
	Registry *entities.Registry
	Now      time.Time
}

// Result is a goal evaluated: the verdict, whether it leans on a restored
// (not live) mirror, and the evidence for the trail.
type Result struct {
	Verdict rules.Verdict
	// Stale: the verdict is met only through a restored container (CLAUDE.md
	// rule 8).
	Stale    bool
	Evidence string
}

// Proven reports whether the result proves the goal: met, on live facts only.
// It is the one test the tracker uses to end a job.
func (r Result) Proven() bool { return r.Verdict == rules.Met && !r.Stale }

// Evaluate evaluates the goal on the account's facts. Without facts the
// verdict is unknown, never unmet.
func (g Goal) Evaluate(env Env) Result {
	if env.Facts.State == nil {
		return Result{Verdict: rules.Unknown}
	}
	ps := *env.Facts.State
	switch {
	case g.key != nil:
		return fromEvidence(progressVerdict(progress.EvaluateObjective(*g.key, ps, env.Registry)))
	case g.rule != nil:
		v, evs := rules.EvaluateRule(g.rule, ps)
		return fromEvidence(v, evs)
	case g.task != nil:
		u, ok := g.task.Ends()
		if !ok {
			return Result{Verdict: rules.Unknown}
		}
		v, _ := panel.EvaluateUntil(*g.task, u, progress.StepState{}, nil, nil, ps.Items, env.Facts, env.Registry, env.Now)
		return Result{Verdict: v}
	}
	return Result{Verdict: rules.Unknown}
}

func progressVerdict(ev progress.Evaluation) (rules.Verdict, []rules.Evidence) {
	return ev.Verdict, ev.Evidence
}

func fromEvidence(v rules.Verdict, evs []rules.Evidence) Result {
	r := Result{Verdict: v, Evidence: evidenceText(evs)}
	for _, e := range evs {
		if e.Stale {
			r.Stale = true
		}
	}
	return r
}

// evidenceText is what the evaluation observed, short, for the run's trail
// ("nível 76"); empty when it says nothing short.
func evidenceText(evs []rules.Evidence) string {
	var parts []string
	for _, e := range evs {
		switch v := e.Observed.(type) {
		case nil:
		case rules.SkillState:
			parts = append(parts, fmt.Sprintf("nível %d", v.Level))
		case string:
			parts = append(parts, "observado "+v)
		default:
			if b, err := json.Marshal(v); err == nil {
				parts = append(parts, "observado "+string(b))
			}
		}
	}
	out := strings.Join(parts, "; ")
	if len(out) > 200 {
		out = out[:200]
		for !utf8.ValidString(out) {
			out = out[:len(out)-1]
		}
	}
	return out
}

// Proof is a goal met on live facts: the only thing that ends a job as
// succeeded. Only Result.Prove makes one, so no code path can report a
// success that the evaluation did not give.
type Proof struct {
	evidence string
	valid    bool
}

// Prove returns the proof a result gives, or false when it proves nothing (not
// met, or met only through a restored mirror).
func (r Result) Prove() (Proof, bool) {
	if !r.Proven() {
		return Proof{}, false
	}
	return Proof{evidence: r.Evidence, valid: true}, true
}

// Valid reports whether the proof came from Prove.
func (p Proof) Valid() bool { return p.valid }

// Evidence is what the evaluation observed, short.
func (p Proof) Evidence() string { return p.evidence }
