package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/config"
	"github.com/FelipeMalacarne/ironman-planner/internal/httpapi"
	"github.com/FelipeMalacarne/ironman-planner/internal/kube"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
	storemicrobot "github.com/FelipeMalacarne/ironman-planner/internal/store/microbot"
	storeworkers "github.com/FelipeMalacarne/ironman-planner/internal/store/workers"
	"github.com/FelipeMalacarne/ironman-planner/internal/vault"
	"github.com/FelipeMalacarne/ironman-planner/internal/workerctl"
)

// newMicrobot builds the Microbot control module (docs/08) and, with a
// vault key, the accounts' sessions; with WORKERS=on also the reconciler of
// the managed workers in the cluster (docs/11), started on ctx.
func newMicrobot(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, ingest *storeingest.Repository, logger *slog.Logger) (*httpapi.MicrobotOptions, error) {
	link := &agent.Link{Log: logger}
	opts := &httpapi.MicrobotOptions{Link: link, Audit: storemicrobot.New(pool), WorkerImage: cfg.WorkerImage}
	logger.Warn("microbot: control module ON (docs/08): /api/microbot routes and the agent link are enabled")
	if cfg.VaultKey == "" {
		logger.Info("workers: no CHRONICLE_VAULT_KEY; game sessions and workers are off")
		return opts, nil
	}
	key, err := vault.ParseKey(cfg.VaultKey)
	if err != nil {
		return nil, fmt.Errorf("CHRONICLE_VAULT_KEY: %w", err)
	}
	repo := storeworkers.New(pool, key, ingest)
	opts.Sessions = repo
	logger.Info("workers: game sessions on", "vault_key", key.ID())
	if !cfg.Workers {
		return opts, nil
	}
	cluster, err := kube.InCluster(cfg.WorkerNamespace)
	if err != nil {
		return nil, fmt.Errorf("WORKERS=on: %w", err)
	}
	serverURL := cfg.WorkerServerURL
	if serverURL == "" {
		serverURL = "http://chronicle." + cluster.Namespace + ".svc"
	}
	rec := &workerctl.Reconciler{
		Store: workerctl.RepoStore{Repo: repo}, Cluster: cluster, Hub: link, Log: logger,
		Namespace: cluster.Namespace, Image: cfg.WorkerImage, ServerURL: serverURL, Version: buildinfo.String(),
	}
	link.OnConnect = rec.OnConnect
	opts.Control = rec
	opts.Workers = podVerifier{cluster}
	go rec.Run(ctx)
	logger.Warn("workers: reconciler ON (docs/11)", "namespace", cluster.Namespace, "image", cfg.WorkerImage, "server_url", serverURL)
	return opts, nil
}

// podVerifier checks a worker pod's projected token by TokenReview.
type podVerifier struct{ c *kube.Client }

func (v podVerifier) VerifyWorkerToken(ctx context.Context, token string) (httpapi.WorkerIdentity, error) {
	id, err := v.c.VerifyWorkerToken(ctx, token)
	return httpapi.WorkerIdentity{Pod: id.Pod, Account: id.Account}, err
}
