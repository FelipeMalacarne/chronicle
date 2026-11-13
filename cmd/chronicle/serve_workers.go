package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/buildinfo"
	"github.com/FelipeMalacarne/chronicle/internal/config"
	"github.com/FelipeMalacarne/chronicle/internal/httpapi"
	"github.com/FelipeMalacarne/chronicle/internal/kube"
	"github.com/FelipeMalacarne/chronicle/internal/routinesync"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storemicrobot "github.com/FelipeMalacarne/chronicle/internal/store/microbot"
	storeroutines "github.com/FelipeMalacarne/chronicle/internal/store/routines"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
	"github.com/FelipeMalacarne/chronicle/internal/vault"
	"github.com/FelipeMalacarne/chronicle/internal/workerctl"
)

// newMicrobot builds the Microbot control module (docs/08) and, with a
// vault key, the accounts' sessions; with WORKERS=on also the reconciler of
// the managed workers in the cluster (docs/11), started on ctx.
func newMicrobot(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, ingest *storeingest.Repository, logger *slog.Logger) (*httpapi.MicrobotOptions, error) {
	link := &agent.Link{Log: logger}
	audit := storemicrobot.New(pool)
	routines := storeroutines.New(pool)
	opts := &httpapi.MicrobotOptions{Link: link, Audit: audit, WorkerImage: cfg.WorkerImage, Routines: routines}
	logger.Warn("microbot: control module ON (docs/08): /api/microbot routes and the agent link are enabled")
	// The triggers and the runs' history live in the database (docs/11
	// section 4.4): every agent that links gets its account's triggers, and
	// every run it reports is stored.
	sync := &routinesync.Sync{Store: routines, Hub: link, Audit: audit, Log: logger}
	link.OnConnect, link.OnEvent = sync.OnConnect, sync.OnEvent
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
		Store: workerctl.RepoStore{Repo: repo}, Hub: link, Log: logger,
		Image: cfg.WorkerImage, ServerURL: cfg.WorkerServerURL, Version: buildinfo.String(),
	}
	link.OnConnect = func(info agent.Info) {
		// The triggers first: the bootstrap starts the client and takes minutes.
		sync.OnConnect(info)
		rec.OnConnect(info)
	}
	if !cfg.Workers {
		if rec.ServerURL == "" {
			rec.ServerURL = cfg.PublicBaseURL
		}
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
	opts.Control = rec
	opts.Workers = podVerifier{cluster}
	go rec.Run(ctx)
	logger.Warn("workers: reconciler ON (docs/11)", "namespace", cluster.Namespace, "image", cfg.WorkerImage, "server_url", rec.ServerURL)
	return opts, nil
}

// podVerifier checks a worker pod's projected token by TokenReview.
type podVerifier struct{ c *kube.Client }

func (v podVerifier) VerifyWorkerToken(ctx context.Context, token string) (httpapi.WorkerIdentity, error) {
	id, err := v.c.VerifyWorkerToken(ctx, token)
	return httpapi.WorkerIdentity{Pod: id.Pod, Account: id.Account}, err
}
