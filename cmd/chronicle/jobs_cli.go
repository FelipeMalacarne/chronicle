package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// newJobsCmd is `chronicle jobs`: the account's jobs over the API (docs/12
// section 13): what the account was made to do, with its state and measured
// progress, and the runs and events behind each.
func newJobsCmd() *cobra.Command {
	var (
		f     clientFlags
		state string
	)
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "The account's jobs: state and measured progress (show <id> for its runs and events, create -f, run <id>, cancel <id>)",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			jobs, err := c.Jobs(ctx, state)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), jobs)
			}
			printJobs(cmd.OutOrStdout(), jobs)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.Flags().StringVar(&state, "state", "", "only jobs in this state (queued, running, stalled, succeeded, failed, canceled)")
	cmd.AddCommand(newJobsShowCmd(), newJobsCancelCmd(), newJobsCreateCmd(), newJobsRunCmd())
	return cmd
}

func newJobsShowCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "One job with the runs that attempted it and its events",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			d, err := c.Job(ctx, args[0])
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), d)
			}
			out := cmd.OutOrStdout()
			printJobs(out, []client.Job{d.Job})
			fmt.Fprintf(out, "meta: %s\n", d.Job.Goal)
			for _, r := range d.Runs {
				fmt.Fprintf(out, "run %s  %s  %s\n", r.UID, r.State, r.ClassName)
			}
			for _, e := range d.Events {
				fmt.Fprintf(out, "%s  %s\n", e.At.Local().Format("15:04:05"), e.Type)
			}
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func newJobsCancelCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel an active job (and its run, when it is the agent's current one)",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			j, err := c.CancelJob(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", j.ID, j.State)
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}

func newJobsCreateCmd() *cobra.Command {
	var (
		f    clientFlags
		file string
	)
	cmd := &cobra.Command{
		Use:   "create -f <job.json>",
		Short: "Queue a job from its JSON (kind, spec, goal; POST /api/jobs); nothing runs until `jobs run`",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				return errors.New("-f <job.json> is required")
			}
			raw, err := os.ReadFile(file) //nolint:gosec // the owner's own file
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			j, err := c.CreateJob(ctx, raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", j.ID, j.State)
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVarP(&file, "file", "f", "", "the job, as JSON")
	return cmd
}

func newJobsRunCmd() *cobra.Command {
	var (
		f          clientFlags
		yes        bool
		supervised bool
		timeout    time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run <id> --yes",
		Short: "Start a queued plugin job on the account's agent: it turns the plugin on in the game client",
		Long: `Sends the job to the account's agent, which turns the job's plugin on in the
game client with the job's settings and keeps it on until the ingest proves the
job's goal, then turns it off. It acts in game on the account, so it needs --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("o job age no jogo, na conta: confirme com --yes")
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			if err := c.RunJob(ctx, args[0], supervised, int(timeout/time.Second)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: enviado ao agent; acompanhe com `chronicle jobs show %s`\n", args[0], args[0])
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm: the job acts in game")
	cmd.Flags().BoolVar(&supervised, "supervised", false, "the owner is watching (an HCIM runs a plugin job only so)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "bound the run (1m to 12h; the agent's default, 2h, when unset)")
	return cmd
}

func printJobs(w io.Writer, jobs []client.Job) {
	if len(jobs) == 0 {
		fmt.Fprintln(w, "nenhum job")
		return
	}
	for _, j := range jobs {
		line := fmt.Sprintf("%s  %-9s %-7s", j.ID, j.State, j.Kind)
		if p := j.Progress; p != nil {
			line += fmt.Sprintf("  %d/%d (de %d, %s)", p.Current, p.Target, p.From, p.At.Local().Format("15:04:05"))
		}
		if j.Reason != "" {
			line += "  " + j.Reason
		}
		fmt.Fprintln(w, line)
	}
}
