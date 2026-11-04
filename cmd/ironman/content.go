package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/config"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content/migrate"
)

func newContentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "content",
		Short: "Validate and migrate guide content",
	}
	cmd.AddCommand(newContentValidateCmd(), newContentMigrateCmd(), newContentImportBruhsailerCmd())
	return cmd
}

func newContentValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate content against the canonical schema",
		Long: `Validate guide content against content/schema/guide.schema.json plus the
semantic checks of the loader (unique ids, resolvable step references).

path may be a content directory (default: $CONTENT_DIR or "content"), which
is loaded exactly as the server loads it (guides plus annotation sidecars; a
stale annotation is an error here and only a warning in serve), or a single
guide.json file.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				path = cfg.ContentDir
			}
			out := cmd.OutOrStdout()
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				g, loadErr := content.LoadGuideFile(path)
				if loadErr != nil {
					return reportIssues(cmd, loadErr)
				}
				fmt.Fprintf(out, "ok: %s: guide %q, %d sections, %d steps, 0 errors\n", path, g.ID, len(g.Sections), g.StepCount())
				return nil
			}
			c, err := content.Load(path)
			if err != nil {
				return reportIssues(cmd, err)
			}
			if len(c.Warnings) > 0 {
				// The server skips stale annotations; committed content must not have any.
				return reportIssues(cmd, c.Warnings)
			}
			for _, g := range c.Guides {
				n := 0
				if a := c.Annotations[g.ID]; a != nil {
					n = len(a.Sections)
				}
				fmt.Fprintf(out, "ok: %s: guide %q, %d sections, %d steps, %d annotated sections\n", g.File, g.ID, len(g.Sections), g.StepCount(), n)
			}
			fmt.Fprintf(out, "ok: %d guide(s), %d steps, 0 errors\n", len(c.Guides), c.StepCount())
			return nil
		},
	}
}

// reportIssues prints each validation issue on its own line and returns a
// summary error so the process exits 1.
func reportIssues(cmd *cobra.Command, err error) error {
	issues, ok := content.AsErrors(err)
	if !ok {
		return err
	}
	w := cmd.ErrOrStderr()
	for _, is := range issues {
		fmt.Fprintln(w, is.Error())
	}
	return fmt.Errorf("content validation failed with %d error(s)", len(issues))
}

func newContentMigrateCmd() *cobra.Command {
	var in, out, guideID, version string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Convert the normalized ironman.guide JSON into canonical content",
		Long: `Read data/guide/normalized/guide.json (schema v0.1 from tools/normalize_guide.py),
convert it to the canonical schema, validate the result and write
content/guides/<id>/guide.json plus README.md (attribution and provenance).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				out = "content/guides/" + guideID
			}
			res, err := migrate.Run(in, out, migrate.Options{GuideID: guideID, ContentVersion: version})
			if err != nil {
				return reportIssues(cmd, err)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "wrote %s (%d variants, %d sections, %d segments, %d bank legs, %d steps: %d with rules, %d manual)\n",
				res.GuideFile, res.Variants, res.Sections, res.Segments, res.BankLegs, res.Steps, res.WithRules, res.Manual)
			fmt.Fprintf(w, "wrote %s\n", res.ReadmeFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&in, "in", "data/guide/normalized/guide.json", "normalized guide (schema v0.1)")
	cmd.Flags().StringVar(&out, "out", "", "output directory (default content/guides/<guide-id>)")
	cmd.Flags().StringVar(&guideID, "guide-id", "oziris-main", "canonical guide id")
	cmd.Flags().StringVar(&version, "content-version", "2026.11.0", "value for guide.version")
	return cmd
}
