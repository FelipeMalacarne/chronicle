package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newObjectiveCmd is `chronicle objective`: the owner's marks on objectives.
func newObjectiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objective",
		Short: "Mark an objective as already done (or take the mark back)",
	}
	cmd.AddCommand(newObjectiveMarkCmd(), newObjectiveUnmarkCmd())
	return cmd
}

func newObjectiveMarkCmd() *cobra.Command {
	var (
		f    clientFlags
		note string
	)
	cmd := &cobra.Command{
		Use:   "mark <key>",
		Short: "Record that an objective is already satisfied, for the whole account",
		Long: `Marks an objective by its canonical key (as "chronicle now" lists them, e.g.
item:1351x1). It counts as met in every guide and step, but it is your mark, not
evidence: the stored verdicts stay as the facts left them. "unmark" takes it back.`,
		Args: cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			res, err := c.MarkObjective(ctx, args[0], note)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marcado: %s (%d passos mudaram)\n", res.Key, res.StepsChanged)
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVar(&note, "note", "", "note on the mark")
	return cmd
}

func newObjectiveUnmarkCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "unmark <key>",
		Short: "Take your mark off an objective",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			res, err := c.UnmarkObjective(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Desmarcado: %s (%d passos mudaram)\n", res.Key, res.StepsChanged)
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}
