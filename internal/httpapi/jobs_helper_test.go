package httpapi

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/jobtracker"
	"github.com/FelipeMalacarne/chronicle/internal/routinesync"
	storeeventlog "github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
)

// jobsWiring is what chronicle serve wires around the jobs: the store the
// handlers use, and the routines' sync that stores each run an agent reports
// with its job, which the tracker then follows on the event log.
type jobsWiring struct {
	Jobs     *storejobs.Repository
	Events   *storeeventlog.Repository
	Routines *storeroutines.Repository
}

// startJobs wires the jobs to the link and starts the tracker until the test
// ends; the hub wakes it, the tick is only a safety net.
func startJobs(t *testing.T, pool *pgxpool.Pool, progress *storeprogress.Repository, catalog *entities.Registry, link *agent.Link, audit *memAudit, hub *events.Hub, quiet *slog.Logger) jobsWiring {
	t.Helper()
	w := jobsWiring{Jobs: storejobs.New(pool).WithPublisher(hub), Events: storeeventlog.New(pool), Routines: storeroutines.New(pool)}
	sync := &routinesync.Sync{Store: w.Routines, Hub: link, Audit: audit, Log: quiet, Wake: hub}
	link.OnEvent = sync.OnEvent
	tr := &jobtracker.Tracker{
		Jobs: w.Jobs, Log: w.Events, Facts: progress, Registry: catalog, Hub: link, Events: hub, Audit: audit, Logr: quiet,
		Interval: 100 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go tr.Run(ctx)
	return w
}

// eventually waits for a condition the tracker makes true.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
	}
}
