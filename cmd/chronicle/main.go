// Command chronicle is the single binary of Chronicle, the Ironman planner: web server
// (serve), migrations (migrate), content tooling (content validate|migrate|mcp) and the CLI and MCP
// clients of the API (login, now, done, plan, sync, state, mcp).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/buildinfo"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "chronicle:", err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "chronicle",
		Short:         "Self-hosted OSRS Ironman planner",
		Version:       buildinfo.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newServeCmd(),
		newMigrateCmd(),
		newContentCmd(),
		newVersionCmd(),
		newHealthCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newAccountsCmd(),
		newUseCmd(),
		newNowCmd(),
		markCmd("done", "Mark a step of the active route done (the current one by default)", coreprogress.ActionManualDone, "Feito"),
		markCmd("skip", "Skip a step of the active route (the current one by default)", coreprogress.ActionSkipped, "Pulado"),
		markCmd("undo", "Clear your mark on a step of the active route", coreprogress.ActionClear, "Desfeito"),
		newPlanCmd(),
		newSyncCmd(),
		newStateCmd(),
		newRecordingCmd(),
		newTaskCmd(),
		newPlansCmd(),
		newMCPCmd(),
		newProgressCmd(),
		newAgentCmd(),
		newWorkersCmd(),
		newProfilesCmd(),
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
