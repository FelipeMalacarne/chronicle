// Command ironman is the single binary of the Ironman planner: web server
// (serve), content tooling (content validate|migrate|mcp), and the CLI (now,
// done, plan, graph, state, sync, progress check) and MCP (mcp) clients of
// the API.
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
		newHealthCmd(),
		newProgressCmd(),
		newNowCmd(),
		newDoneCmd(),
		newPlanCmd(),
		newGraphCmd(),
		newStateCmd(),
		newSyncCmd(),
		newAccountMCPCmd(),
	)
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
