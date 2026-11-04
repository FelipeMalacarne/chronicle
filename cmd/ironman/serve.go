package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/config"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/httpapi"
	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/syncrun"
	"github.com/FelipeMalacarne/ironman-planner/web"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the API, the web UI and (later) the scheduler",
		Long: `Start the HTTP server. Configuration comes from the environment; see
internal/config for the variables (IRONMAN_ADDR, DATABASE_URL, AUTH_MODE, ...).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context())
		},
	}
}

func runServe(parent context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	level, _ := cfg.SlogLevel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cnt, err := content.Load(cfg.ContentDir)
	if err != nil {
		return fmt.Errorf("load content from %s: %w", cfg.ContentDir, err)
	}
	logger.Info("content loaded", "dir", cfg.ContentDir, "guides", len(cnt.Guides), "steps", cnt.StepCount())
	for _, w := range cnt.Warnings {
		logger.Warn("annotation skipped", "issue", w.Error())
	}

	catalog, err := entities.Default()
	if err != nil {
		return fmt.Errorf("entity catalog: %w", err)
	}

	var db httpapi.DBPinger
	var ingestService storeingest.Service
	var holdings httpapi.HoldingsSource
	var progressService httpapi.ProgressService
	var syncService httpapi.SyncService
	if cfg.DatabaseURL != "" {
		pool, dbErr := connectDB(ctx, cfg, logger)
		if dbErr != nil {
			return dbErr
		}
		if pool != nil {
			defer pool.Close()
			db = pool
			progressRepo := storeprogress.New(pool, cnt, catalog)
			if err = bootProgress(ctx, progressRepo, logger); err != nil {
				if !cfg.IsDev() {
					return err
				}
				logger.Error("progress: boot indexing failed; continuing", "err", err)
			}
			repo := storeingest.New(pool, storeingest.BuildDefinition(cnt)).WithReevaluator(progressRepo)
			ingestService, holdings, progressService = repo, repo, progressRepo
			syncRepo := syncrun.New(pool, hiscores.New(), progressRepo)
			syncService = syncRepo
			if cfg.SyncHiscoresMinutes > 0 {
				scheduler := &syncrun.Scheduler{
					Repo: syncRepo, Interval: time.Duration(cfg.SyncHiscoresMinutes) * time.Minute,
					Jitter: 5 * time.Minute, Log: logger,
				}
				go scheduler.Run(ctx)
				logger.Info("sync: hiscores scheduler on", "minutes", cfg.SyncHiscoresMinutes)
			}
		}
	} else {
		logger.Info("no DATABASE_URL set; running without a database")
	}

	static, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embedded web/dist: %w", err)
	}

	api := httpapi.New(httpapi.Options{
		Version:        buildinfo.String(),
		Content:        cnt,
		DefaultGuideID: "oziris-main",
		DB:             db,
		Ingest:         ingestService,
		Holdings:       holdings,
		Progress:       progressService,
		Sync:           syncService,
		Catalog:        catalog,
		DevMode:        cfg.IsDev(),
		Static:         static,
		Logger:         logger,
	})
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Addr, "auth_mode", cfg.AuthMode, "version", buildinfo.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
		close(errc)
	}()

	select {
	case err := <-errc:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// bootProgress indexes the loaded content (content_versions, content_steps),
// applies pending step id aliases and reevaluates every active account, so
// newly extracted annotations show up without any other step (docs/09
// section 6.2).
func bootProgress(ctx context.Context, repo *storeprogress.Repository, logger *slog.Logger) error {
	report, err := repo.IndexContent(ctx, buildinfo.String())
	if err != nil {
		return fmt.Errorf("index content: %w", err)
	}
	for _, w := range report.Warnings {
		logger.Warn("step id alias not applied", "issue", w)
	}
	accounts, err := repo.ReevaluateAll(ctx)
	if err != nil {
		return fmt.Errorf("reevaluate progress: %w", err)
	}
	logger.Info("progress: content indexed", "content_version", report.ContentVersionID,
		"steps", report.Steps, "aliases_applied", report.AliasesApplied, "accounts_reevaluated", accounts)
	return nil
}

// connectDB opens a pgx pool and pings it. In dev mode a failing ping is
// logged and the pool is still returned (health reports "error"); in oidc
// mode it is fatal.
func connectDB(ctx context.Context, cfg config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		if cfg.IsDev() {
			logger.Error("database: invalid DATABASE_URL; continuing without a database", "err", err)
			return nil, nil
		}
		return nil, fmt.Errorf("database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		if cfg.IsDev() {
			logger.Error("database: ping failed; continuing (health will report db=error)", "err", err)
			return pool, nil
		}
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	logger.Info("database: connected")
	return pool, nil
}
