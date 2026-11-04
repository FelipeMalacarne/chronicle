package progress

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// StepEventView is one recent step status change.
type StepEventView struct {
	StepID string    `json:"step_id"`
	From   string    `json:"from,omitempty"`
	To     string    `json:"to"`
	Cause  string    `json:"cause"`
	Source string    `json:"source,omitempty"`
	At     time.Time `json:"at"`
}

// ObjectiveEventView is one recent objective verdict change.
type ObjectiveEventView struct {
	Key    string    `json:"key"`
	From   string    `json:"from,omitempty"`
	To     string    `json:"to"`
	Source string    `json:"source,omitempty"`
	At     time.Time `json:"at"`
}

// EventsView is what changed lately: the step changes of one guide and the
// objective changes of the account, newest first.
type EventsView struct {
	Guide      string               `json:"guide"`
	Account    string               `json:"account"`
	Steps      []StepEventView      `json:"steps"`
	Objectives []ObjectiveEventView `json:"objectives"`
}

// SourceView is the newest observation of one fact source.
type SourceView struct {
	Source         string    `json:"source"`
	LastObservedAt time.Time `json:"last_observed_at"`
	Facts          int64     `json:"facts"`
}

// Sources returns, per source, the newest fact it reported for the account.
func (r *Repository) Sources(ctx context.Context, accountID string) ([]SourceView, error) {
	if _, err := getAccount(ctx, r.queries, accountID); err != nil {
		return nil, err
	}
	rows, err := r.queries.GetPlayerStateFreshness(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("player state freshness: %w", err)
	}
	out := make([]SourceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, SourceView{Source: row.Source, LastObservedAt: row.LastObservedAt.Time, Facts: row.Facts})
	}
	return out, nil
}

// Events returns the latest changes, at most limit of each kind.
func (r *Repository) Events(ctx context.Context, accountID, guideID string, limit int) (EventsView, error) {
	out := EventsView{Guide: guideID, Account: accountID, Steps: []StepEventView{}, Objectives: []ObjectiveEventView{}}
	if _, ok := r.content.Guide(guideID); !ok {
		return out, ErrGuideNotFound
	}
	if _, err := getAccount(ctx, r.queries, accountID); err != nil {
		return out, err
	}
	steps, err := r.queries.ListRecentStepStatusEvents(ctx, storedb.ListRecentStepStatusEventsParams{AccountID: accountID, GuideID: guideID, Limit: int32(limit)})
	if err != nil {
		return out, fmt.Errorf("list step events: %w", err)
	}
	for _, e := range steps {
		out.Steps = append(out.Steps, StepEventView{StepID: e.StepID, From: e.FromStatus.String, To: e.ToStatus, Cause: e.Cause, Source: e.Source, At: e.At.Time})
	}
	objectives, err := r.queries.ListRecentObjectiveProgressEvents(ctx, storedb.ListRecentObjectiveProgressEventsParams{AccountID: accountID, Limit: int32(limit)})
	if err != nil {
		return out, fmt.Errorf("list objective events: %w", err)
	}
	for _, e := range objectives {
		out.Objectives = append(out.Objectives, ObjectiveEventView{Key: e.ObjectiveKey, From: e.FromVerdict.String, To: e.ToVerdict, Source: e.Source, At: e.At.Time})
	}
	return out, nil
}

// SetActiveGuide makes guideID the route the account follows and reevaluates
// it in the same transaction. Objectives met from now on feed the cascade of
// the new route only (docs/09 decision 9.1).
func (r *Repository) SetActiveGuide(ctx context.Context, accountID, guideID string) error {
	if _, ok := r.content.Guide(guideID); !ok {
		return ErrGuideNotFound
	}
	return r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		if _, err := lockAccount(ctx, q, accountID); err != nil {
			return err
		}
		if err := q.SetAccountGuide(ctx, storedb.SetAccountGuideParams{ID: accountID, GuideID: guideID}); err != nil {
			return fmt.Errorf("set active guide: %w", err)
		}
		_, err := r.Reevaluate(ctx, tx, accountID, "")
		return err
	})
}
