package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/buildinfo"
	"github.com/FelipeMalacarne/chronicle/internal/config"
	"github.com/FelipeMalacarne/chronicle/internal/events"
	"github.com/FelipeMalacarne/chronicle/internal/httpapi"
	"github.com/FelipeMalacarne/chronicle/internal/jobtracker"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
	"github.com/FelipeMalacarne/chronicle/internal/routinesync"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
	"github.com/FelipeMalacarne/chronicle/internal/vault"
	"github.com/FelipeMalacarne/chronicle/internal/workerctl"
)

// microbotDeps is what the control module reads besides the database: the
// job tracker, which tells the agent to stop the runs whose job it proved
// (internal/jobtracker), and the hub that wakes the event log's consumers.
type microbotDeps struct {
	tracker *jobtracker.Tracker
	events  *events.Hub
}

// newMicrobot builds the Microbot control module (docs/08) and, with a
// vault key, the accounts' sessions and the reconciler of the workers
// (docs/11), started on ctx: it fills the slots of the attached agents, and
// with WORKERS=on also runs the pods in the cluster.
func newMicrobot(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, ingest *storeingest.Repository, deps microbotDeps, logger *slog.Logger) (*httpapi.MicrobotOptions, error) {
	link := &agent.Link{Log: logger}
	audit := storemicrobot.New(pool)
	routines := storeroutines.New(pool)
	opts := &httpapi.MicrobotOptions{Link: link, Audit: audit, WorkerImage: cfg.WorkerImage, Routines: routines}
	logger.Warn("microbot: control module ON (docs/08): /api/microbot routes and the agent link are enabled")
	// The triggers and the runs' history live in the database (docs/11
	// section 4.4): every agent that links gets its account's triggers, and
	// every run it reports is stored, with its job. A run that waits for an
	// objective (wait_until, docs/10 phase 4) is a job the tracker proves on
	// the account's facts, and it tells the agent through this link.
	sync := &routinesync.Sync{Store: routines, Hub: link, Audit: audit, Log: logger, Wake: deps.events}
	deps.tracker.Hub, deps.tracker.Audit = link, audit
	link.OnConnect = sync.OnConnect
	link.OnEvent = sync.OnEvent
	if cfg.VaultKey == "" {
		logger.Info("workers: no CHRONICLE_VAULT_KEY; game sessions and workers are off")
		return opts, nil
	}
	key, err := vault.ParseKey(cfg.VaultKey)
	if err != nil {
		return nil, fmt.Errorf("CHRONICLE_VAULT_KEY: %w", err)
	}
	repo := storeworkers.New(pool, key, ingest)
	opts.Sessions, opts.Profiles = repo, repo
	logger.Info("workers: game sessions on", "vault_key", key.ID())
	// The bootstrap of a linking worker agent needs only the database and the
	// link, so it runs with WORKERS=off too: a worker image started by hand
	// against a dev server (loopback) gets its session the same way.
	rec := &workerctl.Reconciler{
		Store: workerctl.RepoStore{Repo: repo}, Hub: link, Audit: audit, Log: logger,
		Image: cfg.WorkerImage, ServerURL: cfg.WorkerServerURL, PublicURL: cfg.PublicBaseURL, Version: buildinfo.String(),
	}
	link.OnConnect = func(info agent.Info) {
		// The triggers first: the bootstrap starts the client and takes minutes.
		sync.OnConnect(info)
		rec.OnConnect(info)
	}
	opts.Control = rec
	if !cfg.Workers {
		if rec.ServerURL == "" {
			rec.ServerURL = cfg.PublicBaseURL
		}
		go rec.Run(ctx)
		logger.Warn("workers: reconciler ON for attached agents only (WORKERS=off, docs/11)", "public_url", rec.PublicURL)
		return opts, nil
	}
	cluster, err := kube.InCluster(cfg.WorkerNamespace)
	if err != nil {
		return nil, fmt.Errorf("WORKERS=on: %w", err)
	}
	if rec.ServerURL == "" {
		rec.ServerURL = "http://chronicle." + cluster.Namespace + ".svc"
	}
	rec.Cluster, rec.Namespace = cluster, cluster.Namespace
	opts.Workers, opts.Cluster = podVerifier{cluster}, true
	go rec.Run(ctx)
	logger.Warn("workers: reconciler ON, cluster and attached agents (docs/11)", "namespace", cluster.Namespace, "image", cfg.WorkerImage, "server_url", rec.ServerURL)
	return opts, nil
}

// podVerifier checks a worker pod's projected token by TokenReview.
type podVerifier struct{ c *kube.Client }

func (v podVerifier) VerifyWorkerToken(ctx context.Context, token string) (httpapi.WorkerIdentity, error) {
	id, err := v.c.VerifyWorkerToken(ctx, token)
	return httpapi.WorkerIdentity{Pod: id.Pod, Account: id.Account}, err
}
