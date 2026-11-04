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
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
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

	var db httpapi.DBPinger
	var ingestService storeingest.Service
	var holdings httpapi.HoldingsSource
	if cfg.DatabaseURL != "" {
		pool, dbErr := connectDB(ctx, cfg, logger)
		if dbErr != nil {
			return dbErr
		}
		if pool != nil {
			defer pool.Close()
			db = pool
			repo := storeingest.New(pool, storeingest.BuildDefinition(cnt))
			ingestService, holdings = repo, repo
		}
	} else {
		logger.Info("no DATABASE_URL set; running without a database")
	}

	static, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embedded web/dist: %w", err)
	}

	catalog, err := entities.Default()
	if err != nil {
		return fmt.Errorf("entity catalog: %w", err)
	}

	api := httpapi.New(httpapi.Options{
		Version:        buildinfo.String(),
		Content:        cnt,
		DefaultGuideID: "oziris-main",
		DB:             db,
		Ingest:         ingestService,
		Holdings:       holdings,
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
