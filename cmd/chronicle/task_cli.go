package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
)

// newTaskCmd is `chronicle task`: the next typed task of the account's
// active plan (docs/10 section 9) and its run by the account's agent
// (docs/10 phase 3), over the API.
func newTaskCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "task",
		Short: "The next typed task of the active plan, and its run by the agent",
		Long: `Without a subcommand, the next task of the account's active plan
(GET /api/plan/next): what it is, what ends it and whether the ingest already
proves it. "run" asks the account's agent to execute it in game; "status"
shows the run and the proof; "cancel" stops it.`,
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			next, err := c.PlanNext(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), next)
			}
			printPlanNext(cmd.OutOrStdout(), next)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.AddCommand(newTaskRunCmd(), newTaskStatusCmd(), newTaskCancelCmd())
	return cmd
}

func newTaskRunCmd() *cobra.Command {
	var (
		f          clientFlags
		key        string
		supervised bool
		yes        bool
		timeoutS   int
		wait       time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute the plan's next task in game, on the account's agent (acts in game)",
		Long: `The account's agent runs the next task of the active plan, after a safe point:
travel, bank or buy through Chronicle Link's executor, or a train delegated to
the plugin the task names (it must be installed in the client), which the
agent stops when the server proves the level from the ingest. It acts in game
on the account: confirm with --yes. --key guards against the plan moving on
(it defaults to the task the command reads first). An HCIM task with a death
risk (a train that fights included) runs only with --supervised (docs/10, E9).
The run ending "done" is the plugin's machine finishing (for a train, the
server's proof); the task is done only when the ingest proves its
termination, which --wait follows (a training takes hours: --wait 6h).`,
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			if key == "" {
				next, nerr := c.PlanNext(ctx)
				if nerr != nil {
					return nerr
				}
				if next.Task == nil {
					printPlanNext(out, next)
					return errors.New("o plano não tem tarefa a executar agora")
				}
				key = next.Task.Key
				fmt.Fprintf(out, "Próxima tarefa: %s (%s)\n", next.Task.Label, key)
			}
			if !yes {
				return errors.New("a tarefa age no jogo, na conta: confirme com --yes")
			}
			started, err := c.RunTask(ctx, key, supervised, timeoutS)
			if err != nil {
				return err
			}
			if f.json && wait == 0 {
				return printJSON(out, started)
			}
			if r := started.Result.Run; r != nil {
				fmt.Fprintf(out, "%s começou: %s\n", r.ID, started.Task.Label)
			}
			if wait <= 0 {
				return nil
			}
			return followTask(ctx, out, c, wait)
		}),
	}
	f.add(cmd, true)
	cmd.Flags().StringVar(&key, "key", "", "the task's key (default: the plan's next task, read first)")
	cmd.Flags().BoolVar(&supervised, "supervised", false, "the owner is watching (required for an HCIM task with a death risk)")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm: the task acts in game")
	cmd.Flags().IntVar(&timeoutS, "timeout", 0, "seconds the plugin's machine may take (30 to 3600; default 900 travel, 300 otherwise; a train 60 to 43200, default 7200)")
	cmd.Flags().DurationVar(&wait, "wait", 0, "follow the run and then the ingest's proof for up to this long (e.g. 10m)")
	return cmd
}

func newTaskStatusCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "The agent's last run and the ingest's proof of its task",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			st, err := c.TaskStatus(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), st)
			}
			printTaskStatus(cmd.OutOrStdout(), st, 0)
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func newTaskCancelCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Stop the agent's run (a task stops where it is)",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			if err := c.CancelRun(ctx); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cancelado")
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}

// followTask prints the run's trail as it grows, then waits for the
// ingest's proof, up to wait.
func followTask(ctx context.Context, out io.Writer, c *client.Client, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	shown := 0
	for {
		st, err := c.TaskStatus(ctx)
		if err != nil {
			return err
		}
		shown = printTaskStatus(out, st, shown)
		if st.Run != nil && st.Run.Final() {
			if st.Proven {
				fmt.Fprintln(out, "Término provado pelo ingest.")
				return nil
			}
			if st.Run.State != "done" {
				return fmt.Errorf("o run terminou como %s", st.Run.State)
			}
		}
		select {
		case <-ctx.Done():
			if st.Proof != nil && st.Proof.Reason != "" {
				return fmt.Errorf("sem prova do ingest em %s: %s", wait, st.Proof.Reason)
			}
			return fmt.Errorf("sem prova do ingest em %s", wait)
		case <-time.After(3 * time.Second):
		}
	}
}

// printTaskStatus prints the run's trail from line from, and the proof once
// the run ended; it returns how many lines are shown.
func printTaskStatus(w io.Writer, st client.TaskStatus, from int) int {
	r := st.Run
	if r == nil {
		fmt.Fprintln(w, "Nenhum run.")
		return 0
	}
	if from == 0 {
		label := r.ClassName
		if r.Task != nil {
			label = r.Task.Label + " [" + r.Task.Key + "]"
		}
		fmt.Fprintf(w, "%s %s: %s\n", r.ID, r.State, label)
	}
	for _, ev := range r.Events[min(from, len(r.Events)):] {
		fmt.Fprintf(w, "  %s %-10s %s\n", ev.At.Local().Format("15:04:05"), ev.State, ev.Text)
	}
	if r.Final() || from == 0 {
		switch {
		case st.Proof == nil:
			fmt.Fprintf(w, "Prova: %s\n", st.Reason)
		case st.Proven:
			fmt.Fprintf(w, "Prova: %s, %s (pelo ingest)\n", st.Proof.Until, st.Proof.State)
		default:
			fmt.Fprintf(w, "Prova: %s, %s %s\n", st.Proof.Until, st.Proof.State, st.Proof.Reason)
		}
	}
	return len(r.Events)
}

// printPlanNext prints the next task of the plan, what it skipped and the
// step's credit line (rule 6).
func printPlanNext(w io.Writer, n client.PlanNext) {
	fmt.Fprintf(w, "Plano: %s (%s)\n", n.Plan.Title, n.Plan.Kind)
	if it := n.Item; it != nil {
		switch it.Kind {
		case plan.ItemStep:
			fmt.Fprintf(w, "Passo %s · %s\n  %s\n", it.Ref, it.Title, it.Attribution)
		case plan.ItemRecipe:
			fmt.Fprintf(w, "Receita: %s\n", it.Label)
		default:
			fmt.Fprintf(w, "Item %d do plano\n", it.Position+1)
		}
	}
	for _, s := range n.Skipped {
		fmt.Fprintf(w, "  ✓ %s (%s)\n", s.Label, s.Until)
	}
	if t := n.Task; t != nil {
		state := map[string]string{"unmet": "a fazer", "unknown": "sem dado"}[string(t.State)]
		fmt.Fprintf(w, "→ %s\n  termina: %s (%s", t.Label, t.Until, state)
		if t.Reason != "" {
			fmt.Fprintf(w, ": %s", t.Reason)
		}
		fmt.Fprintln(w, ")")
		if len(t.Risks) > 0 {
			fmt.Fprintf(w, "  riscos: %s", strings.Join(t.Risks, ", "))
			if t.Supervised {
				fmt.Fprint(w, " (HCIM: só com --supervised)")
			}
			fmt.Fprintln(w)
		}
		if t.Executable {
			fmt.Fprintf(w, "  executável: chronicle task run --key '%s' --yes\n", t.Key)
		} else {
			fmt.Fprintf(w, "  não executável: %s\n", t.NotExecutable)
		}
		return
	}
	fmt.Fprintf(w, "%s: %s\n", n.Status, n.Reason)
}
