package syncrun

import (
	"context"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Scheduler syncs every active account with a name on a cadence (docs/04
// section 6.1): a tick per minute, Interval per account with a jitter of
// ±Jitter, never before the backoff of the last run. Accounts run in series.
type Scheduler struct {
	Repo     *Repository
	Interval time.Duration
	Jitter   time.Duration
	Log      *slog.Logger
	// Tick is the period of the due check (one minute by default).
	Tick time.Duration
}

// Run blocks until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	tick := s.Tick
	if tick <= 0 {
		tick = time.Minute
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		s.RunDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunDue syncs the accounts that are due now and returns how many ran.
func (s *Scheduler) RunDue(ctx context.Context) int {
	rows, err := s.Repo.queries.ListAccountsToSync(ctx, Source)
	if err != nil {
		s.log().Error("sync: list accounts", "err", err)
		return 0
	}
	ran := 0
	now := s.Repo.now()
	for _, row := range rows {
		if ctx.Err() != nil {
			return ran
		}
		if !Due(row.ID, row.LastRunAt, row.NextAllowedAt, now, s.Interval, s.Jitter) {
			continue
		}
		run, err := s.Repo.Run(ctx, row.ID, TriggerScheduled)
		ran++
		if err != nil {
			s.log().Error("sync: hiscores", "account", row.ID, "err", err)
			continue
		}
		s.log().Info("sync: hiscores", "account", row.ID, "status", run.Status, "steps_changed", run.StepsChanged, "error", run.Error)
	}
	return ran
}

func (s *Scheduler) log() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}

// Due reports whether an account should sync now: never before its backoff;
// otherwise when it never ran, or Interval (± a jitter stable for that run)
// after its last run.
func Due(accountID string, lastRun, nextAllowed pgtype.Timestamptz, now time.Time, interval, jitter time.Duration) bool {
	if nextAllowed.Valid && now.Before(nextAllowed.Time) {
		return false
	}
	if !lastRun.Valid {
		return true
	}
	return !now.Before(lastRun.Time.Add(interval + offset(accountID, lastRun.Time, jitter)))
}

func offset(accountID string, last time.Time, jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(accountID))
	_, _ = h.Write([]byte(last.UTC().Format(time.RFC3339Nano)))
	span := uint64(2*jitter) + 1
	return time.Duration(h.Sum64()%span) - jitter
}
