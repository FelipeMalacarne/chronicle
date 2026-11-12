package progress

import (
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// FactObservation is the provenance of one fact: the player_state row (or the
// item container's latest accepted observation) behind an evaluation.
type FactObservation struct {
	Fact
	Source     string    `json:"source"`
	SnapshotID string    `json:"snapshot_id"`
	ObservedAt time.Time `json:"observed_at"`
}

// EvidenceItem is one atomic verdict with the facts it read; a list of them is
// the evidence JSON of objective_progress.
type EvidenceItem struct {
	rules.Evidence
	Facts []FactObservation `json:"facts,omitempty"`
}

// ObjectiveRecord is the stored verdict of one objective (objective_progress).
// Exists is false when the objective has no row yet, which reads as unknown.
type ObjectiveRecord struct {
	Exists       bool
	Verdict      rules.Verdict
	FirstMetAt   *time.Time
	MetInGuideID string
	RegressedAt  *time.Time
}

// ObjectiveUpdate is the row to write for one objective. When ReplaceEvidence
// is false the stored evidence and snapshot stay as they are (a latched met).
type ObjectiveUpdate struct {
	Key                string
	Verdict            rules.Verdict
	ReplaceEvidence    bool
	Evidence           []EvidenceItem
	EvidenceSnapshotID string
	FirstMetAt         *time.Time
	MetInGuideID       string
	RegressedAt        *time.Time
}

// Record returns the stored form of the update.
func (u ObjectiveUpdate) Record() ObjectiveRecord {
	return ObjectiveRecord{Exists: true, Verdict: u.Verdict, FirstMetAt: u.FirstMetAt, MetInGuideID: u.MetInGuideID, RegressedAt: u.RegressedAt}
}

// VerdictChange is one objective_progress_events row. From is empty when the
// objective had no row.
type VerdictChange struct {
	Key  string
	From rules.Verdict
	To   rules.Verdict
}

// Advance moves the stored verdict of one objective forward with a fresh
// evaluation. Rules (docs/09 section 5, owner decision of 2026-11-04):
//
//   - met is latched: once proven with evidence it stays met with the evidence
//     of the snapshot that proved it, for every target, including items that
//     were later spent. When the facts stop holding, RegressedAt records it;
//     when they hold again it is cleared;
//   - met needs a snapshot: an evaluation whose facts have none stays unknown
//     (CLAUDE.md rule 2);
//   - otherwise only a verdict change is written, and an unknown objective
//     that has no row yet is not written at all.
//
// It returns nil, nil when nothing changes.
func Advance(key string, rec ObjectiveRecord, ev Evaluation, obs map[Fact]FactObservation, activeGuideID string, now time.Time) (*ObjectiveUpdate, *VerdictChange) {
	items, snapshot := attachFacts(ev, obs)
	verdict := ev.Verdict
	if verdict == rules.Met && snapshot == "" {
		verdict = rules.Unknown
		items = []EvidenceItem{{Evidence: rules.Evidence{Verdict: rules.Unknown, Reason: "met without an observed snapshot"}}}
	}

	if rec.Exists && rec.Verdict == rules.Met {
		upd := ObjectiveUpdate{Key: key, Verdict: rules.Met, FirstMetAt: rec.FirstMetAt, MetInGuideID: rec.MetInGuideID, RegressedAt: rec.RegressedAt}
		switch {
		case verdict != rules.Met && rec.RegressedAt == nil:
			upd.RegressedAt = &now
		case verdict == rules.Met && rec.RegressedAt != nil:
			upd.RegressedAt = nil
		default:
			return nil, nil
		}
		return &upd, nil
	}
	if rec.Exists && rec.Verdict == verdict {
		return nil, nil
	}
	if !rec.Exists && verdict == rules.Unknown {
		return nil, nil
	}
	upd := ObjectiveUpdate{Key: key, Verdict: verdict, ReplaceEvidence: true, Evidence: items}
	if verdict == rules.Met {
		upd.EvidenceSnapshotID = snapshot
		upd.FirstMetAt = &now
		upd.MetInGuideID = activeGuideID
	}
	change := VerdictChange{Key: key, To: verdict}
	if rec.Exists {
		change.From = rec.Verdict
	}
	return &upd, &change
}

// attachFacts returns the evidence with the provenance of the facts it read
// and the snapshot of the newest of them.
func attachFacts(ev Evaluation, obs map[Fact]FactObservation) ([]EvidenceItem, string) {
	var (
		facts    []FactObservation
		snapshot string
		newest   time.Time
	)
	for _, f := range ev.Facts {
		o, ok := obs[f]
		if !ok || o.SnapshotID == "" {
			continue
		}
		o.Fact = f
		facts = append(facts, o)
		if snapshot == "" || o.ObservedAt.After(newest) {
			snapshot, newest = o.SnapshotID, o.ObservedAt
		}
	}
	items := make([]EvidenceItem, 0, len(ev.Evidence))
	for _, e := range ev.Evidence {
		items = append(items, EvidenceItem{Evidence: e, Facts: facts})
	}
	return items, snapshot
}
