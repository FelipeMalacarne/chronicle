package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// Mark command metadata (docs/10 section 6): every change to step_marks is a
// row of mark_commands with its effects, written in the same transaction.

// Where an owner command came from.
const (
	ViaWeb         = "web"
	ViaPluginPanel = "plugin_panel"
)

// Kinds and actors of mark_commands.
const (
	kindMark           = "mark"
	kindConfirmImplied = "confirm_implied"
	kindMarkBefore     = "mark_before"
	kindRestore        = "restore"
	kindPin            = "pin"
	kindImplied        = "implied"
	kindAlias          = "alias"

	actorOwner  = "owner"
	actorEngine = "engine"
	actorSystem = "system"
)

// ErrInvalidRevokes: the command an undo names is not one of this account and
// guide.
var ErrInvalidRevokes = errors.New("revokes does not name a command of this account and guide")

// CommandMeta describes an owner write: where it came from and, for an undo,
// the command it takes back.
type CommandMeta struct {
	Via     string
	Revokes string
}

type commandMetaKey struct{}

// WithCommand attaches the metadata of an owner write to its request context.
func WithCommand(ctx context.Context, m CommandMeta) context.Context {
	return context.WithValue(ctx, commandMetaKey{}, m)
}

func commandMeta(ctx context.Context) CommandMeta {
	m, _ := ctx.Value(commandMetaKey{}).(CommandMeta)
	if m.Via == "" {
		m.Via = ViaWeb
	}
	return m
}

// logCommand appends one command and returns its id.
func logCommand(ctx context.Context, q *storedb.Queries, accountID, guideID, kind, actor, stepID string, meta CommandMeta, effects []coreprogress.MarkEffect) (string, error) {
	if effects == nil {
		effects = []coreprogress.MarkEffect{}
	}
	raw, err := json.Marshal(effects)
	if err != nil {
		return "", fmt.Errorf("encode effects: %w", err)
	}
	if meta.Revokes != "" {
		prev, err := q.GetMarkCommand(ctx, meta.Revokes)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (prev.AccountID != accountID || prev.GuideID != guideID)) {
			return "", ErrInvalidRevokes
		}
		if err != nil {
			return "", fmt.Errorf("read revoked command: %w", err)
		}
	}
	id := newID("mc_")
	params := storedb.InsertMarkCommandParams{
		ID: id, AccountID: accountID, GuideID: guideID, Kind: kind, Actor: actor,
		StepID: text(stepID), RevokesID: text(meta.Revokes), Effects: raw,
	}
	if actor == actorOwner {
		params.Via = text(meta.Via)
	}
	if err := q.InsertMarkCommand(ctx, params); err != nil {
		return "", fmt.Errorf("log %s command: %w", kind, err)
	}
	return id, nil
}

// VerifyMarks replays the account's mark commands and compares the result with
// step_marks (docs/10 section 6). It returns one line per difference; none
// means step_marks is exactly the projection of the log.
func (r *Repository) VerifyMarks(ctx context.Context, accountID string) ([]string, error) {
	return verifyMarks(ctx, r.queries, accountID)
}

func verifyMarks(ctx context.Context, q *storedb.Queries, accountID string) ([]string, error) {
	cmds, err := q.ListMarkCommands(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list mark commands: %w", err)
	}
	byGuide := map[string][][]coreprogress.MarkEffect{}
	for _, c := range cmds {
		var effects []coreprogress.MarkEffect
		if err = json.Unmarshal(c.Effects, &effects); err != nil {
			return nil, fmt.Errorf("decode effects of %s: %w", c.ID, err)
		}
		byGuide[c.GuideID] = append(byGuide[c.GuideID], effects)
	}
	rows, err := q.ListStepMarksByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list step marks: %w", err)
	}
	stored := map[string]map[string]coreprogress.Mark{}
	for _, row := range rows {
		if stored[row.GuideID] == nil {
			stored[row.GuideID] = map[string]coreprogress.Mark{}
		}
		stored[row.GuideID][row.StepID] = markOf(row)
	}
	guides := map[string]bool{}
	for g := range byGuide {
		guides[g] = true
	}
	for g := range stored {
		guides[g] = true
	}
	var out []string
	for _, g := range sortedKeys(guides) {
		replayed, gaps := coreprogress.ReplayMarks(byGuide[g])
		for _, gap := range gaps {
			out = append(out, g+": "+gap)
		}
		steps := map[string]bool{}
		for s := range replayed {
			steps[s] = true
		}
		for s := range stored[g] {
			steps[s] = true
		}
		for _, s := range sortedKeys(steps) {
			want, inLog := replayed[s]
			have, inTable := stored[g][s]
			if inLog != inTable || want != have {
				out = append(out, fmt.Sprintf("%s/%s: log gives %s, step_marks has %s", g, s, markText(want, inLog), markText(have, inTable)))
			}
		}
	}
	return out, nil
}

func markText(m coreprogress.Mark, ok bool) string {
	if !ok {
		return "no mark"
	}
	return fmt.Sprintf("%+v", m)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
