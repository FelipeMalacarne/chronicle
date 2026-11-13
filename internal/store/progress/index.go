package progress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// IndexReport is what IndexContent did.
type IndexReport struct {
	ContentVersionID string
	Steps            int
	AliasesApplied   int
	// Warnings are aliases that were not applied (content problems).
	Warnings []string
}

type guideSummary struct {
	Version        string `json:"version,omitempty"`
	Steps          int    `json:"steps"`
	Objectives     int    `json:"objectives"`
	AnnotatedSteps int    `json:"annotated_steps"`
}

// IndexContent records the loaded bundle in content_versions, rewrites
// content_steps for every loaded guide and applies the step id aliases the
// steps declare in `replaces` (docs/04 section 3.2) to marks, step status
// events and cursors, once each. It runs in one transaction at boot, before
// the reevaluation.
func (r *Repository) IndexContent(ctx context.Context, gitSHA string) (IndexReport, error) {
	var rep IndexReport
	hash, summary, bundleErr := bundle(r.content)
	if bundleErr != nil {
		return rep, bundleErr
	}
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		version, err := q.UpsertContentVersion(ctx, storedb.UpsertContentVersionParams{
			ID: "cv_" + hash[:16], ContentHash: hash, GitSha: text(gitSHA), Summary: summary,
		})
		if err != nil {
			return fmt.Errorf("upsert content version: %w", err)
		}
		rep.ContentVersionID = version.ID
		return r.indexGuides(ctx, q, version.ID, &rep)
	})
	return rep, err
}

func (r *Repository) indexGuides(ctx context.Context, q *storedb.Queries, versionID string, rep *IndexReport) error {
	for _, g := range r.content.Guides {
		if err := q.UpsertGuide(ctx, storedb.UpsertGuideParams{ID: g.ID, Title: g.Title}); err != nil {
			return fmt.Errorf("upsert guide %s: %w", g.ID, err)
		}
	}
	for _, g := range r.content.Guides {
		n, err := indexGuide(ctx, q, g, versionID)
		if err != nil {
			return err
		}
		rep.Steps += n
		applied, warnings, err := applyAliases(ctx, q, g)
		if err != nil {
			return err
		}
		rep.AliasesApplied += applied
		rep.Warnings = append(rep.Warnings, warnings...)
	}
	return nil
}

// bundle hashes the composed content (guides with their annotations) and
// summarizes each guide.
func bundle(cnt *content.Content) (string, json.RawMessage, error) {
	guides := append([]*content.Guide(nil), cnt.Guides...)
	sort.Slice(guides, func(i, j int) bool { return guides[i].ID < guides[j].ID })
	h := sha256.New()
	summary := map[string]guideSummary{}
	for _, g := range guides {
		raw, err := json.Marshal(g)
		if err != nil {
			return "", nil, fmt.Errorf("encode guide %s: %w", g.ID, err)
		}
		h.Write([]byte(g.ID))
		h.Write([]byte{0})
		h.Write(raw)
		s := guideSummary{Version: g.Version}
		for _, st := range g.Steps() {
			s.Steps++
			s.Objectives += len(st.Objectives)
		}
		if ann := cnt.Annotations[g.ID]; ann != nil {
			for _, f := range ann.Sections {
				s.AnnotatedSteps += len(f.Steps)
			}
		}
		summary[g.ID] = s
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		return "", nil, fmt.Errorf("encode content summary: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), raw, nil
}

func indexGuide(ctx context.Context, q *storedb.Queries, g *content.Guide, versionID string) (int, error) {
	if _, err := q.DeleteContentStepsByGuide(ctx, g.ID); err != nil {
		return 0, fmt.Errorf("clear content steps of %s: %w", g.ID, err)
	}
	seen := map[string]bool{}
	var rows []storedb.InsertContentStepsParams
	for _, st := range g.Steps() {
		if seen[st.ID] {
			continue
		}
		seen[st.ID] = true
		rows = append(rows, storedb.InsertContentStepsParams{
			GuideID: g.ID, StepID: st.ID, SectionID: st.SectionID, Position: int32(len(rows) + 1),
			TextHash: content.StepTextHash(st), ContentVersionID: versionID,
		})
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if _, err := q.InsertContentSteps(ctx, rows); err != nil {
		return 0, fmt.Errorf("insert content steps of %s: %w", g.ID, err)
	}
	return len(rows), nil
}

// applyAliases records the guide's `replaces` declarations and applies the
// pending ones. An alias whose old id is still a step of the guide would move
// progress away from a live step; it is reported and left pending.
func applyAliases(ctx context.Context, q *storedb.Queries, g *content.Guide) (int, []string, error) {
	for _, st := range g.Steps() {
		for _, old := range st.Replaces {
			if old == "" || old == st.ID {
				continue
			}
			if _, err := q.UpsertStepIDAlias(ctx, storedb.UpsertStepIDAliasParams{GuideID: g.ID, OldStepID: old, NewStepID: st.ID}); err != nil {
				return 0, nil, fmt.Errorf("record alias %s/%s: %w", g.ID, old, err)
			}
		}
	}
	pending, err := q.ListPendingStepIDAliases(ctx, g.ID)
	if err != nil {
		return 0, nil, fmt.Errorf("list pending aliases of %s: %w", g.ID, err)
	}
	var (
		applied  int
		warnings []string
	)
	for _, a := range pending {
		if _, live := g.Step(a.OldStepID); live {
			warnings = append(warnings, fmt.Sprintf("%s: step %s replaces %s, which is still a step of the guide", g.ID, a.NewStepID, a.OldStepID))
			continue
		}
		if err := applyAlias(ctx, q, a); err != nil {
			return 0, nil, err
		}
		applied++
	}
	return applied, warnings, nil
}

func applyAlias(ctx context.Context, q *storedb.Queries, a storedb.StepIDAlias) error {
	moved, err := q.ListMovedStepMarks(ctx, storedb.ListMovedStepMarksParams{GuideID: a.GuideID, StepID: a.OldStepID})
	if err != nil {
		return fmt.Errorf("list marks of %s/%s: %w", a.GuideID, a.OldStepID, err)
	}
	present, err := q.ListMovedStepMarks(ctx, storedb.ListMovedStepMarksParams{GuideID: a.GuideID, StepID: a.NewStepID})
	if err != nil {
		return fmt.Errorf("list marks of %s/%s: %w", a.GuideID, a.NewStepID, err)
	}
	kept := map[string]bool{}
	for _, m := range present {
		kept[m.AccountID] = true
	}
	if err := q.MoveStepStatusEvents(ctx, storedb.MoveStepStatusEventsParams{GuideID: a.GuideID, OldStepID: a.OldStepID, NewStepID: a.NewStepID}); err != nil {
		return fmt.Errorf("move events of %s/%s: %w", a.GuideID, a.OldStepID, err)
	}
	if err := q.MoveRouteCursors(ctx, storedb.MoveRouteCursorsParams{GuideID: a.GuideID, OldStepID: a.OldStepID, NewStepID: a.NewStepID}); err != nil {
		return fmt.Errorf("move cursors of %s/%s: %w", a.GuideID, a.OldStepID, err)
	}
	// The plans' step items follow the step (docs/10 section 9).
	if err := q.MovePlanItemSteps(ctx, storedb.MovePlanItemStepsParams{GuideID: a.GuideID, OldStepID: a.OldStepID, NewStepID: a.NewStepID}); err != nil {
		return fmt.Errorf("move plan items of %s/%s: %w", a.GuideID, a.OldStepID, err)
	}
	if _, err := q.MoveStepMark(ctx, storedb.MoveStepMarkParams{GuideID: a.GuideID, OldStepID: a.OldStepID, NewStepID: a.NewStepID}); err != nil {
		return fmt.Errorf("move marks of %s/%s: %w", a.GuideID, a.OldStepID, err)
	}
	if err := q.DeleteStepMarksByStep(ctx, storedb.DeleteStepMarksByStepParams{GuideID: a.GuideID, StepID: a.OldStepID}); err != nil {
		return fmt.Errorf("drop superseded marks of %s/%s: %w", a.GuideID, a.OldStepID, err)
	}
	ref := coreprogress.StepRef{GuideID: a.GuideID, StepID: a.NewStepID}
	for _, m := range moved {
		// The move enters the mark log (docs/10 section 6): the old id loses
		// its mark, and the new id gets it unless it already had its own.
		mark := coreprogress.Mark{Kind: coreprogress.MarkKind(m.Kind), ImpliedBy: m.ImpliedByStepID.String, Note: m.Note.String}
		effects := []coreprogress.MarkEffect{{StepID: a.OldStepID, From: &mark}}
		if !kept[m.AccountID] {
			effects = append(effects, coreprogress.MarkEffect{StepID: a.NewStepID, To: &mark})
		}
		commandID, err := logCommand(ctx, q, m.AccountID, a.GuideID, kindAlias, actorSystem, a.NewStepID, CommandMeta{}, effects)
		if err != nil {
			return err
		}
		if kept[m.AccountID] {
			continue
		}
		if err := insertEvents(ctx, q, m.AccountID, "", []coreprogress.StepEvent{{
			Ref: ref, To: coreprogress.Status(m.Kind), Cause: coreprogress.CauseMigration,
		}}, func(coreprogress.StepEvent) string { return commandID }); err != nil {
			return err
		}
	}
	if err := q.MarkStepIDAliasApplied(ctx, storedb.MarkStepIDAliasAppliedParams{GuideID: a.GuideID, OldStepID: a.OldStepID}); err != nil {
		return fmt.Errorf("mark alias %s/%s applied: %w", a.GuideID, a.OldStepID, err)
	}
	return nil
}
