package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/importer/bruhsailer"
)

func newContentImportBruhsailerCmd() *cobra.Command {
	var sourceDir, outputPath string
	cmd := &cobra.Command{
		Use:   "import-bruhsailer",
		Short: "Import the pinned BRUHsailer DOCX snapshots",
		Long: `Read the three checksummed, pinned BRUHsailer DOCX snapshots without
network access and deterministically regenerate the independent
content/guides/bruhsailer-main/guide.json route.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := bruhsailer.Run(sourceDir, outputPath)
			if err != nil {
				return err
			}
			guide, err := content.LoadGuideFile(outputPath)
			if err != nil {
				return reportIssues(cmd, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d pinned sources, %d sections, %d segments, %d bank legs, %d steps; schema-valid guide %q)\n",
				result.GuideFile, result.SourceFiles, result.Sections, result.Segments, result.BankLegs, result.Steps, guide.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceDir, "source", "data/sources/bruhsailer", "directory containing pinned Chapter1/2/3.docx")
	cmd.Flags().StringVar(&outputPath, "out", "content/guides/bruhsailer-main/guide.json", "canonical guide output path")
	return cmd
}
