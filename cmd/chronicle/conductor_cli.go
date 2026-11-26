package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// newConductorCmd is `chronicle conductor`: the account's conductor over the
// API (docs/12 section 12.5). Turned on, it dispatches the next task of the
// account's plan as a job, and the next one when the job succeeds; a failed
// job or a task it cannot run blocks it until it is turned on again.
func newConductorCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "conductor",
		Short: "The account's conductor: off, idle, running (its job) or blocked (on --yes, off)",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			st, err := c.Conductor(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), st)
			}
			printConductor(cmd.OutOrStdout(), st)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.AddCommand(newConductorSetCmd(true), newConductorSetCmd(false))
	return cmd
}

func newConductorSetCmd(on bool) *cobra.Command {
	var (
		f   clientFlags
		yes bool
	)
	cmd := &cobra.Command{
		Use:   "off",
		Short: "Turn the conductor off: it dispatches nothing more; a job it started goes on",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			if on && !yes {
				return errors.New("o condutor age no jogo, na conta: confirme com --yes")
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			st, err := c.SetConductor(ctx, on)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), st)
			}
			printConductor(cmd.OutOrStdout(), st)
			return nil
		}),
	}
	f.add(cmd, true)
	if on {
		cmd.Use = "on --yes"
		cmd.Short = "Turn the conductor on: it runs the plan job by job, in game, on the account"
		cmd.Flags().BoolVar(&yes, "yes", false, "confirm: the conductor acts in game")
	}
	return cmd
}

func printConductor(w io.Writer, st client.Conductor) {
	fmt.Fprintf(w, "%s  %s", st.Account, st.State)
	if st.Job != "" {
		fmt.Fprintf(w, "  %s", st.Job)
	}
	if st.Reason != "" {
		fmt.Fprintf(w, "  %s", st.Reason)
	}
	fmt.Fprintln(w)
}
