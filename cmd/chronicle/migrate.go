package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/db"
)

// newMigrateCmd is `chronicle migrate`: the goose migrations embedded in the
// binary, against DATABASE_URL. Besides `serve`, it is the only command that
// opens the database (CLAUDE.md rule 1); the deploy runs it before the server
// (the initContainer on saradomin, the one-shot service of compose.yaml).
func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "migrate [up|status]",
		Short:     "Apply the database migrations (up, the default) or list them (status)",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"up", "status"},
		RunE: func(cmd *cobra.Command, args []string) error {
			action := "up"
			if len(args) == 1 {
				action = args[0]
			}
			dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
			if dsn == "" {
				return errors.New("DATABASE_URL is not set")
			}
			return migrateDatabase(cmd.Context(), cmd.OutOrStdout(), dsn, action)
		},
	}
}

func migrateDatabase(ctx context.Context, out io.Writer, dsn, action string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	switch action {
	case "up":
		results, err := provider.Up(ctx)
		for _, r := range results {
			fmt.Fprintf(out, "applied %s (%s)\n", r.Source.Path, r.Duration.Round(1e6))
		}
		if err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		version, err := provider.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("database version: %w", err)
		}
		fmt.Fprintf(out, "database at version %d\n", version)
		return nil
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
		for _, s := range statuses {
			fmt.Fprintf(out, "%-8s %s\n", s.State, s.Source.Path)
		}
		return nil
	default:
		return fmt.Errorf("unknown action %q (up or status)", action)
	}
}
