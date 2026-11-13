package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
)

// newPlansCmd is `chronicle plans`: the account's plans over the API
// (docs/10 section 9). The screen of the custom plans is phase 5; until
// then a plan is a JSON file.
func newPlansCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "The account's plans: its route, goal and custom plans, and the one it follows",
		Long: `Lists the account's plans (GET /api/plans). "create -f plan.json" stores a goal or
custom plan, "use <id>" follows it ("use route" goes back to the route) and
"delete <id> --yes" removes it. A plan file is {"kind": "custom", "title": "...",
"items": [...]}, each item a step {"kind": "step", "guide": "oziris-main", "step":
"1.1.3"}, a recipe {"kind": "recipe", "recipe": "item:1351x1"} or a task {"kind":
"task", "task": {"type": "travel", "location": "location:lumbridge_castle"}}.`,
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			list, err := c.Plans(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), list)
			}
			printPlans(cmd.OutOrStdout(), list)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.AddCommand(newPlansCreateCmd(), newPlansUseCmd(), newPlansDeleteCmd())
	return cmd
}

func newPlansCreateCmd() *cobra.Command {
	var (
		f    clientFlags
		file string
	)
	cmd := &cobra.Command{
		Use:   "create -f <plan.json>",
		Short: "Store a goal or custom plan from a JSON file (the server verifies every item)",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				return errors.New("-f <plan.json> is required")
			}
			raw, err := os.ReadFile(file) //nolint:gosec // the owner's own file
			if err != nil {
				return err
			}
			var p client.NewPlan
			if uerr := json.Unmarshal(raw, &p); uerr != nil {
				return fmt.Errorf("%s: %w", file, uerr)
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			created, err := c.CreatePlan(ctx, p)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (%d itens); siga com chronicle plans use %s\n", created.ID, created.Title, len(created.Items), created.ID)
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVarP(&file, "file", "f", "", "the plan, as JSON")
	return cmd
}

func newPlansUseCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "use <id|route>",
		Short: "Follow a plan (route: back to the active guide's route)",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if id == "route" {
				id = ""
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			p, err := c.UsePlan(ctx, id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Seguindo %s (%s)\n", p.Title, p.ID)
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}

func newPlansDeleteCmd() *cobra.Command {
	var (
		f   clientFlags
		yes bool
	)
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a stored plan (the account goes back to its route if it followed it)",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("apagar um plano não tem volta: confirme com --yes")
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			if err := c.DeletePlan(ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "apagado")
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the deletion")
	return cmd
}

func printPlans(w io.Writer, list client.Plans) {
	for _, p := range list.Plans {
		mark := " "
		if p.ID == list.Active {
			mark = "*"
		}
		detail := fmt.Sprintf("%d itens", len(p.Items))
		if p.Kind == plan.KindRoute {
			detail = "o guia " + p.Guide + " e o cursor"
		}
		fmt.Fprintf(w, "%s %-22s %-6s %s (%s)\n", mark, p.ID, p.Kind, p.Title, detail)
	}
}
