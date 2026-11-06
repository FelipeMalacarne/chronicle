package progress

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/state"
)

// CheckReport is the result of `ironman progress check` for one account
// (docs/10 section 6, decision E3).
type CheckReport struct {
	Account     string   `json:"account"`
	OK          bool     `json:"ok"`
	Differences []string `json:"differences"`
}

// Check recomputes every projection of one account from its sources and
// reports where the stored rows differ, without writing:
//   - step_marks against the replay of mark_commands;
//   - player_state against the merge of observations;
//   - verdicts, implied marks, last statuses and cursors against a
//     reevaluation of the current facts, which must have nothing to write.
//
// It reads one consistent snapshot of the database (repeatable read, read
// only), so it can run beside live writes.
func (r *Repository) Check(ctx context.Context, accountID string) (CheckReport, error) {
	out := CheckReport{Account: accountID, Differences: []string{}}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, fmt.Errorf("begin check transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	acc, err := getAccount(ctx, q, accountID)
	if err != nil {
		return out, err
	}
	marks, err := verifyMarks(ctx, q, accountID)
	if err != nil {
		return out, err
	}
	for _, d := range marks {
		out.Differences = append(out.Differences, "marks: "+d)
	}
	facts, err := state.Diff(ctx, q, accountID)
	if err != nil {
		return out, err
	}
	for _, d := range facts {
		out.Differences = append(out.Differences, "facts: "+d)
	}
	// The engine sees player_state as stored: when it is out of step with the
	// observations, the facts line above is the cause.
	in, err := r.engineInput(ctx, q, accountID, acc)
	if err != nil {
		return out, err
	}
	for _, d := range coreprogress.Reevaluate(in).Drift() {
		out.Differences = append(out.Differences, "engine: "+d)
	}
	out.OK = len(out.Differences) == 0
	return out, nil
}

// CheckAll checks every active account.
func (r *Repository) CheckAll(ctx context.Context) ([]CheckReport, error) {
	accounts, err := r.queries.ListActiveAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active accounts: %w", err)
	}
	out := make([]CheckReport, 0, len(accounts))
	for _, acc := range accounts {
		rep, err := r.Check(ctx, acc.ID)
		if err != nil {
			return out, fmt.Errorf("check %s: %w", acc.ID, err)
		}
		out = append(out, rep)
	}
	return out, nil
}
