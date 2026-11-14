package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// newLoadoutCmd is `chronicle loadout`: the owner's marks on the items of the
// current leg ("Levar do banco").
func newLoadoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loadout",
		Short: "Mark an item to carry as already yours (or take the mark back)",
	}
	cmd.AddCommand(newLoadoutMarkCmd(false), newLoadoutMarkCmd(true))
	return cmd
}

func newLoadoutMarkCmd(unmark bool) *cobra.Command {
	var (
		f    clientFlags
		note string
	)
	use, short := "mark <item>", "Record that you already have an item of the current leg"
	long := `Marks an item of the current leg's "Levar" list by its name or key (as
"chronicle now" lists them). The leg is the cursor step's: the mark counts only
there, not in another leg or route, and it is your word, not evidence: the
plugin's facts stay as they are. "unmark" takes it back.`
	if unmark {
		use, short, long = "unmark <item>", "Take your mark off an item of the current leg", ""
	}
	cmd := &cobra.Command{
		Use: use, Short: short, Long: long, Args: cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			guide, segment, item, name, err := resolveLoadoutItem(ctx, c, args[0])
			if err != nil {
				return err
			}
			if unmark {
				if _, err = c.UnmarkLoadoutItem(ctx, guide, segment, item); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Desmarcado: %s\n", name)
				return nil
			}
			if _, err = c.MarkLoadoutItem(ctx, guide, segment, item, note); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marcado: %s\n", name)
			return nil
		}),
	}
	f.add(cmd, false)
	if !unmark {
		cmd.Flags().StringVar(&note, "note", "", "note on the mark")
	}
	return cmd
}

// resolveLoadoutItem finds, in the current leg of the account's route, the
// item that arg names: its requirement key or its name (case-insensitive).
func resolveLoadoutItem(ctx context.Context, c *client.Client, arg string) (guide, segment, item, name string, err error) {
	w, err := c.Route(ctx, 1)
	if err != nil {
		return "", "", "", "", err
	}
	if w.Carry == nil || len(w.Carry.Items) == 0 {
		return "", "", "", "", fmt.Errorf("the current leg lists no item to carry")
	}
	var hits []string
	var found string
	for _, it := range w.Carry.Items {
		if it.Key == arg || strings.EqualFold(it.Name, arg) {
			hits = append(hits, it.Name)
			found, name = it.Key, it.Name
		}
	}
	switch len(hits) {
	case 0:
		return "", "", "", "", fmt.Errorf("no item %q in the current leg (use the name or key chronicle now shows)", arg)
	case 1:
		return w.Guide, w.Carry.SegmentID, found, name, nil
	}
	return "", "", "", "", fmt.Errorf("%q matches %d items of the current leg; use the key", arg, len(hits))
}
