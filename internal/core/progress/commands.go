package progress

import "fmt"

// MarkEffect is what one command did to one step's mark: From is the mark
// before (nil: none), To the mark after (nil: removed). A mark command stores
// its effects so the log alone gives step_marks back (docs/10 section 6).
type MarkEffect struct {
	StepID string `json:"step_id"`
	From   *Mark  `json:"from"`
	To     *Mark  `json:"to"`
}

// EffectsOf turns an owner plan's mark changes into effects, in order: the
// mark before each change is the guide's mark (current, by step id) after the
// changes before it. Changes that keep the mark are dropped.
func EffectsOf(current map[string]Mark, changes []MarkChange) []MarkEffect {
	state := make(map[string]Mark, len(changes))
	for _, c := range changes {
		if m, ok := current[c.StepID]; ok {
			state[c.StepID] = m
		}
	}
	out := make([]MarkEffect, 0, len(changes))
	for _, c := range changes {
		e := MarkEffect{StepID: c.StepID, To: c.Mark}
		if m, ok := state[c.StepID]; ok {
			e.From = &m
		}
		if equalMark(e.From, e.To) {
			continue
		}
		if c.Mark == nil {
			delete(state, c.StepID)
		} else {
			state[c.StepID] = *c.Mark
		}
		out = append(out, e)
	}
	return out
}

// ReplayMarks applies the effects of one guide's commands, in log order, and
// returns the marks they produce. An effect whose From does not match the
// replayed state is a gap in the log (a write that bypassed it): it is
// reported, and the replay goes on with the effect's To.
func ReplayMarks(commands [][]MarkEffect) (map[string]Mark, []string) {
	marks := map[string]Mark{}
	var gaps []string
	for i, effects := range commands {
		for _, e := range effects {
			var have *Mark
			if m, ok := marks[e.StepID]; ok {
				have = &m
			}
			if !equalMark(have, e.From) {
				gaps = append(gaps, fmt.Sprintf("command %d, step %s: log says %s, replay has %s", i, e.StepID, describe(e.From), describe(have)))
			}
			if e.To == nil {
				delete(marks, e.StepID)
			} else {
				marks[e.StepID] = *e.To
			}
		}
	}
	return marks, gaps
}

func equalMark(a, b *Mark) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func describe(m *Mark) string {
	if m == nil {
		return "no mark"
	}
	return fmt.Sprintf("%+v", *m)
}
