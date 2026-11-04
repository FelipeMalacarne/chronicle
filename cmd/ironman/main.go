// Command ironman is the single binary of the Ironman planner: web server
// (serve), content tooling (content validate|migrate) and, later, the CLI and
// MCP clients of the API.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ironman:", err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "ironman",
		Short:         "Self-hosted OSRS Ironman planner",
		Version:       buildinfo.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newServeCmd(),
		newContentCmd(),
		newVersionCmd(),
	)
	root.AddCommand(stubCommands()...)
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), buildinfo.String())
		},
	}
}

// exitCodeNotImplemented is returned by stub subcommands.
const exitCodeNotImplemented = 2

func stubCommands() []*cobra.Command {
	stubs := []struct{ use, short string }{
		{"now", "Show the next step, items to bring and blockers (client of the API)"},
		{"done", "Mark a step done, skipped or add a note"},
		{"plan", "Ordered plan to reach a goal"},
		{"sync", "Force a sync from a data source"},
		{"state", "Show player state (skills, quests, diaries)"},
		{"mcp", "Run the MCP server over stdio"},
	}
	out := make([]*cobra.Command, 0, len(stubs))
	for _, s := range stubs {
		use := s.use
		out = append(out, &cobra.Command{
			Use:                use,
			Short:              s.short + " [not implemented yet]",
			DisableFlagParsing: true,
			Run: func(cmd *cobra.Command, _ []string) {
				fmt.Fprintf(cmd.ErrOrStderr(), "ironman %s: not implemented yet\n", use)
				os.Exit(exitCodeNotImplemented)
			},
		})
	}
	return out
}
