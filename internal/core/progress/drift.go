package progress

import "fmt"

// Drift lists what this plan would still write. A reevaluation with no new
// facts right after another must write nothing: every line is a stored
// projection (verdict, implied mark, status event, cursor) out of step with
// the engine (docs/10 section 6, `chronicle progress check`).
func (p Plan) Drift() []string {
	var out []string
	for _, u := range p.Objectives {
		out = append(out, fmt.Sprintf("objective %s: stored verdict would become %s", u.Key, u.Verdict))
	}
	for _, m := range p.Marks {
		out = append(out, fmt.Sprintf("%s/%s: missing implied mark (by %s)", m.Ref.GuideID, m.Ref.StepID, m.Mark.ImpliedBy))
	}
	for _, e := range p.StepEvents {
		out = append(out, fmt.Sprintf("%s/%s: last recorded status %s, engine says %s", e.Ref.GuideID, e.Ref.StepID, statusText(e.From), e.To))
	}
	for _, c := range p.Cursors {
		out = append(out, fmt.Sprintf("%s: cursor would move to %s (%s)", c.GuideID, c.Cursor.StepID, c.Cursor.Cause))
	}
	return out
}

func statusText(s Status) string {
	if s == "" {
		return "none"
	}
	return string(s)
}
