package prices

import (
	"context"
	"log/slog"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/ge"
)

// Fetcher reads the current prices from the source (wikiprices.Client).
type Fetcher interface {
	Fetch(ctx context.Context) ([]ge.Price, error)
}

// Scheduler refreshes the prices on a cadence: once at start, then every
// Interval. A failed refresh is logged and retried at the next tick.
type Scheduler struct {
	Repo     *Repository
	Source   Fetcher
	Interval time.Duration
	Log      *slog.Logger
}

// Run blocks until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		s.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Refresh fetches and stores the prices once and returns how many it stored.
func (s *Scheduler) Refresh(ctx context.Context) int {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	batch, err := s.Source.Fetch(ctx)
	if err != nil {
		log.Error("sync: item prices fetch", "err", err)
		return 0
	}
	if err := s.Repo.Replace(ctx, batch); err != nil {
		log.Error("sync: item prices store", "err", err)
		return 0
	}
	log.Info("sync: item prices", "stored", len(batch))
	return len(batch)
}
