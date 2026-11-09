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

	"github.com/alexedwards/scs/pgxstore"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/auth"
	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/config"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/events"
	"github.com/FelipeMalacarne/ironman-planner/internal/httpapi"
	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/identity"
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

	// One hub per process: the store publishes after each commit, GET
	// /api/events streams to the browser (docs/04 section 6.5).
	guideIDs := make([]string, 0, len(cnt.Guides))
	for _, g := range cnt.Guides {
		guideIDs = append(guideIDs, g.ID)
	}
	hub := events.NewHub(guideIDs)

	var db httpapi.DBPinger
	var ingestService storeingest.Service
	var holdings httpapi.HoldingsSource
	var progressService httpapi.ProgressService
	var syncService httpapi.SyncService
	var accountService httpapi.AccountService
	var hiscoresDetector httpapi.ModeDetector
	var identityRepo httpapi.Identity
	var oidcOpts *httpapi.OIDCOptions
	if !cfg.IsDev() && cfg.DatabaseURL == "" {
		return fmt.Errorf("AUTH_MODE=oidc requires DATABASE_URL (users and sessions live in Postgres)")
	}
	if cfg.DatabaseURL != "" {
		pool, dbErr := connectDB(ctx, cfg, logger)
		if dbErr != nil {
			return dbErr
		}
		if pool != nil {
			defer pool.Close()
			db = pool
			identityRepo = identity.New(pool)
			if !cfg.IsDev() {
				if oidcOpts, err = newOIDC(ctx, cfg, pool); err != nil {
					return err
				}
				logger.Info("auth: oidc", "issuer", cfg.OIDCIssuer, "allowed_subjects", len(cfg.OIDCAllowedSubjects), "required_role", cfg.OIDCRequiredRole)
			}
			progressRepo := storeprogress.New(pool, cnt, catalog).WithPublisher(hub)
			if err = bootProgress(ctx, progressRepo, logger); err != nil {
				if !cfg.IsDev() {
					return err
				}
				logger.Error("progress: boot indexing failed; continuing", "err", err)
			}
			// Tokenless plugin installations only in dev mode (docs/04 section 2.5);
			// the HTTP layer also requires a loopback peer.
			repo := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).
				WithReevaluator(progressRepo).WithPublisher(hub).WithDevInstallations(cfg.IsDev())
			ingestService, holdings, progressService = repo, repo, progressRepo
			hiscoresClient := hiscores.New()
			hiscoresDetector, accountService = hiscoresClient, progressRepo
			syncRepo := syncrun.New(pool, hiscoresClient, progressRepo).WithPublisher(hub)
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
		Accounts:       accountService,
		Hiscores:       hiscoresDetector,
		Catalog:        catalog,
		Events:         hub,
		DevMode:        cfg.IsDev(),
		OIDC:           oidcOpts,
		Identity:       identityRepo,
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

// newOIDC discovers the issuer and builds the session manager over the
// sessions table (pgxstore, expired rows swept every 5 minutes).
func newOIDC(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) (*httpapi.OIDCOptions, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	provider, err := auth.NewProvider(discoverCtx, auth.Config{
		Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID, RedirectURL: cfg.OIDCRedirectURL, ProjectID: cfg.OIDCProjectID,
		NativeClientID: cfg.OIDCNativeClientID,
		HTTPClient:     &http.Client{Timeout: 15 * time.Second},
	})
	if err != nil {
		return nil, err
	}
	sealer, err := auth.NewSealer(cfg.SessionSecret)
	if err != nil {
		return nil, err
	}
	return &httpapi.OIDCOptions{
		Provider: provider,
		Policy:   auth.Policy{AllowedSubjects: cfg.OIDCAllowedSubjects, RequiredRole: cfg.OIDCRequiredRole},
		Sessions: httpapi.NewSessionManager(pgxstore.New(pool), cfg.SecureCookies()),
		Flow:     sealer, PublicBaseURL: cfg.PublicBaseURL, SecureCookies: cfg.SecureCookies(),
	}, nil
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
	checkProgress(ctx, repo, logger)
	accounts, err := repo.ReevaluateAll(ctx)
	if err != nil {
		return fmt.Errorf("reevaluate progress: %w", err)
	}
	logger.Info("progress: content indexed", "content_version", report.ContentVersionID,
		"steps", report.Steps, "aliases_applied", report.AliasesApplied, "accounts_reevaluated", accounts)
	return nil
}

// checkProgress runs `ironman progress check` on every account before the
// boot reevaluation and only logs (docs/10 decision E3): a difference is a bug
// to look at, and the reevaluation then fixes verdicts, statuses and cursors;
// the facts and the mark log are reported, never rewritten by it.
func checkProgress(ctx context.Context, repo *storeprogress.Repository, logger *slog.Logger) {
	reports, err := repo.CheckAll(ctx)
	if err != nil {
		logger.Warn("progress check failed", "err", err)
		return
	}
	for _, rep := range reports {
		if rep.OK {
			continue
		}
		shown := rep.Differences
		if len(shown) > 10 {
			shown = shown[:10]
		}
		logger.Warn("progress check: projections out of step with their sources",
			"account", rep.Account, "differences", len(rep.Differences), "first", shown)
	}
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
