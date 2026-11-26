package job

import (
	"errors"
	"fmt"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// Metric kinds.
const (
	MetricVarp    = "varp"
	MetricVarbit  = "varbit"
	MetricSkillXP = "skill_xp"
)

// Metric is the number that measures a job's progress and the value that
// ends it, read from the account's facts: a varp or varbit by id, a skill's
// experience by name.
type Metric struct {
	Kind   string `json:"kind"`
	ID     int    `json:"id,omitempty"`
	Skill  string `json:"skill,omitempty"`
	Target int64  `json:"target"`
}

// Check validates the metric.
func (m Metric) Check() error {
	switch m.Kind {
	case MetricVarp, MetricVarbit:
		if m.ID < 0 || m.Skill != "" {
			return errors.New("metric: a varp or varbit has an id and no skill")
		}
	case MetricSkillXP:
		if m.Skill == "" || m.ID != 0 {
			return errors.New("metric: skill_xp has a skill and no id")
		}
	default:
		return fmt.Errorf("metric: unknown kind %q", m.Kind)
	}
	return nil
}

// Read is the metric's value on the account's facts; false when the fact was
// never observed (unknown is never zero).
func (m Metric) Read(ps rules.PlayerState) (int64, bool) {
	switch m.Kind {
	case MetricVarp:
		v, ok := ps.Varps[m.ID]
		return v, ok
	case MetricVarbit:
		v, ok := ps.Varbits[m.ID]
		return v, ok
	case MetricSkillXP:
		s, ok := ps.Skills[m.Skill]
		return s.XP, ok
	}
	return 0, false
}

// Progress is a job's measured progress: the metric's value when the job
// started, the last one observed, the target, and when the value last
// changed.
type Progress struct {
	From    int64     `json:"from"`
	Current int64     `json:"current"`
	Target  int64     `json:"target"`
	At      time.Time `json:"at"`
}

// Observe reads the metric on the facts and returns the progress that
// follows: the same progress when the value did not change (changed false),
// a new one with the time of the change otherwise. The first reading starts
// the progress. Without the fact it changes nothing.
func Observe(m Metric, prev *Progress, ps rules.PlayerState, now time.Time) (p *Progress, changed bool) {
	v, ok := m.Read(ps)
	switch {
	case !ok:
		return prev, false
	case prev == nil:
		return &Progress{From: v, Current: v, Target: m.Target, At: now}, true
	case prev.Current == v:
		return prev, false
	}
	next := *prev
	next.Current, next.At = v, now
	return &next, true
}
