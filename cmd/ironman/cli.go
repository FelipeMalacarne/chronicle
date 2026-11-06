package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/client"
)

// apiFlags are the flags every API client subcommand shares. The commands
// talk to a running server over HTTP and never open the database (CLAUDE.md
// rule 1).
type apiFlags struct {
	url     string
	account string
	json    bool
	timeout time.Duration
}

func (f *apiFlags) add(cmd *cobra.Command) {
	def := os.Getenv("IRONMAN_URL")
	if def == "" {
		def = client.DefaultURL
	}
	cmd.Flags().StringVar(&f.url, "url", def, "server base URL (IRONMAN_URL)")
	cmd.Flags().StringVar(&f.account, "account", os.Getenv("IRONMAN_ACCOUNT"), "account id (IRONMAN_ACCOUNT; default: the user's only account)")
	cmd.Flags().BoolVar(&f.json, "json", false, "print the API's JSON answer instead of text")
	cmd.Flags().DurationVar(&f.timeout, "timeout", 30*time.Second, "request timeout")
}

// session opens a client with a deadline and resolves the account.
func (f *apiFlags) session(cmd *cobra.Command) (context.Context, context.CancelFunc, *client.Client, string, error) {
	ctx, cancel := context.WithTimeout(cmd.Context(), f.timeout)
	c := client.New(f.url, os.Getenv("IRONMAN_TOKEN"), client.ViaCLI)
	account, err := c.ResolveAccount(ctx, f.account)
	if err != nil {
		cancel()
		return nil, nil, nil, "", err
	}
	return ctx, cancel, c, account, nil
}

func writeJSONOut(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func newNowCmd() *cobra.Command {
	var f apiFlags
	var limit int
	cmd := &cobra.Command{
		Use:   "now",
		Short: "Show the current step of the active route, what to bring and the next steps",
		Long: `Show the active route from its cursor: the current step with its text,
objectives and notes, the items to bring for its leg and the next steps
(GET /api/route, the same window as the plugin panel).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, c, account, err := f.session(cmd)
			if err != nil {
				return err
			}
			defer cancel()
			w, err := c.Route(ctx, account, limit)
			if err != nil {
				return err
			}
			if f.json {
				return writeJSONOut(cmd.OutOrStdout(), w)
			}
			printWindow(cmd.OutOrStdout(), w)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().IntVarP(&limit, "limit", "n", 5, "steps after the current one (1 to 50)")
	return cmd
}

func newDoneCmd() *cobra.Command {
	var f apiFlags
	var guide, note string
	var skip, undo, pin bool
	cmd := &cobra.Command{
		Use:   "done [step]",
		Short: "Mark a step done (default: the current step), skip it, pin it open or undo the mark",
		Long: `Mark a step of a route as done by hand. Without a step, the current step of
the active route; a step is its id or the author's reference (1.1.219, CH1.6).
--skip skips it, --pin keeps it open, --undo clears the mark; --note sets the
step's note ("" clears it). The mark is the owner's, recorded with via=cli.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action := client.ActionManualDone
			n := 0
			for flag, a := range map[*bool]client.MarkAction{&skip: client.ActionSkipped, &undo: client.ActionClear, &pin: client.ActionPinnedOpen} {
				if *flag {
					action = a
					n++
				}
			}
			if n > 1 {
				return errors.New("pass only one of --skip, --undo and --pin")
			}
			ctx, cancel, c, account, err := f.session(cmd)
			if err != nil {
				return err
			}
			defer cancel()
			if guide == "" {
				p, perr := c.Progress(ctx, account)
				if perr != nil {
					return perr
				}
				guide = p.ActiveGuide
			}
			before, err := c.Board(ctx, account, guide)
			if err != nil {
				return err
			}
			want := ""
			if len(args) == 1 {
				want = args[0]
			}
			step, err := pickStep(before, want)
			if err != nil {
				return err
			}
			var notePtr *string
			if cmd.Flags().Changed("note") {
				notePtr = &note
			}
			after, err := c.Mark(ctx, account, guide, step.ID, action, notePtr)
			if err != nil {
				return err
			}
			if f.json {
				return writeJSONOut(cmd.OutOrStdout(), after)
			}
			printMarked(cmd.OutOrStdout(), step, action, after)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().StringVar(&guide, "guide", "", "guide id (default: the active route)")
	cmd.Flags().StringVar(&note, "note", "", `the step's note ("" clears it)`)
	cmd.Flags().BoolVar(&skip, "skip", false, "skip the step")
	cmd.Flags().BoolVar(&undo, "undo", false, "clear the step's mark")
	cmd.Flags().BoolVar(&pin, "pin", false, "keep the step open even when its objectives are met")
	return cmd
}

func newPlanCmd() *cobra.Command {
	var f apiFlags
	var add, label, remove string
	cmd := &cobra.Command{
		Use:   "plan [goal]",
		Short: "List the pinned goals, or show the plan of one (requirements, bottlenecks, order)",
		Long: `Without arguments, list the account's goals. With a goal (its id, objective
key or name), show its plan: the requirements still open in the suggested
order, the level bottlenecks and where each route completes it.
--add pins an objective key as a goal (quest:legends_quest@finished,
skill:agility>=50, qp>=175) and shows its plan; --remove unpins a goal.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, c, account, err := f.session(cmd)
			if err != nil {
				return err
			}
			defer cancel()
			out := cmd.OutOrStdout()
			switch {
			case remove != "":
				goals, gerr := c.Goals(ctx, account)
				if gerr != nil {
					return gerr
				}
				g, gerr := pickGoal(goals, remove)
				if gerr != nil {
					return gerr
				}
				if rerr := c.RemoveGoal(ctx, account, g.ID); rerr != nil {
					return rerr
				}
				fmt.Fprintf(out, "Meta removida: %s\n", goalName(g))
				return nil
			case add != "":
				g, gerr := c.AddGoal(ctx, account, add, label)
				if gerr != nil {
					return gerr
				}
				return showPlan(ctx, out, c, account, g.ID, f.json)
			case len(args) == 1:
				goals, gerr := c.Goals(ctx, account)
				if gerr != nil {
					return gerr
				}
				g, gerr := pickGoal(goals, args[0])
				if gerr != nil {
					return gerr
				}
				return showPlan(ctx, out, c, account, g.ID, f.json)
			}
			goals, err := c.Goals(ctx, account)
			if err != nil {
				return err
			}
			if f.json {
				return writeJSONOut(out, goals)
			}
			printGoals(out, goals)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().StringVar(&add, "add", "", "pin an objective key as a goal")
	cmd.Flags().StringVar(&label, "label", "", "label of the goal pinned with --add")
	cmd.Flags().StringVar(&remove, "remove", "", "unpin a goal (id, key or name)")
	return cmd
}

func showPlan(ctx context.Context, out io.Writer, c *client.Client, account, id string, asJSON bool) error {
	p, err := c.GoalPlan(ctx, account, id)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSONOut(out, p)
	}
	printPlan(out, p)
	return nil
}

func newGraphCmd() *cobra.Command {
	var f apiFlags
	var depth int
	var all bool
	cmd := &cobra.Command{
		Use:   "graph <objective key>",
		Short: "Show the requirement graph of an objective: requirements by depth, critical path, what it unlocks",
		Long: `Show the requirement graph of any objective key (Mundo › Grafo): its
requirements column by column, the critical path (the longest chain of open
requirements) and the quests it unlocks. Done requirements are collapsed
unless --all.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, c, account, err := f.session(cmd)
			if err != nil {
				return err
			}
			defer cancel()
			g, err := c.Graph(ctx, account, args[0], depth)
			if err != nil {
				return err
			}
			if f.json {
				return writeJSONOut(cmd.OutOrStdout(), g)
			}
			printGraph(cmd.OutOrStdout(), g, all)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().IntVarP(&depth, "depth", "d", 3, "requirement columns (1 to 6)")
	cmd.Flags().BoolVar(&all, "all", false, "list done requirements too")
	return cmd
}

func newStateCmd() *cobra.Command {
	var f apiFlags
	var quests bool
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Show the account's state: skills, quests, diaries, containers and sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, c, account, err := f.session(cmd)
			if err != nil {
				return err
			}
			defer cancel()
			st, err := c.AccountState(ctx, account)
			if err != nil {
				return err
			}
			p, err := c.Progress(ctx, account)
			if err != nil {
				return err
			}
			if f.json {
				return writeJSONOut(cmd.OutOrStdout(), map[string]any{"state": st, "progress": p})
			}
			printState(cmd.OutOrStdout(), st, p, quests, time.Now())
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().BoolVar(&quests, "quests", false, "list every quest with its state")
	return cmd
}

func newSyncCmd() *cobra.Command {
	var f apiFlags
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync the hiscores now and reevaluate the account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel, c, account, err := f.session(cmd)
			if err != nil {
				return err
			}
			defer cancel()
			run, err := c.Sync(ctx, account)
			if err != nil {
				return err
			}
			if f.json {
				return writeJSONOut(cmd.OutOrStdout(), run)
			}
			printSync(cmd.OutOrStdout(), run, time.Now())
			return nil
		},
	}
	f.add(cmd)
	return cmd
}

// pickStep finds a step of the board by id or author reference; empty means
// the cursor step.
func pickStep(b client.Board, want string) (stepRow, error) {
	want = strings.TrimSpace(want)
	if want == "" {
		if b.Cursor == nil {
			return stepRow{}, fmt.Errorf("%s has no current step: the route is finished", b.Guide)
		}
		want = b.Cursor.StepID
	}
	var byRef []stepRow
	for _, s := range b.Steps {
		if s.ID == want {
			return stepRow{ID: s.ID, Ref: s.Ref, Title: s.Title}, nil
		}
		if strings.EqualFold(s.Ref, want) {
			byRef = append(byRef, stepRow{ID: s.ID, Ref: s.Ref, Title: s.Title})
		}
	}
	switch len(byRef) {
	case 1:
		return byRef[0], nil
	case 0:
		return stepRow{}, fmt.Errorf("no step %q in %s", want, b.Guide)
	}
	ids := make([]string, 0, len(byRef))
	for _, s := range byRef {
		ids = append(ids, s.ID+" ("+s.Title+")")
	}
	return stepRow{}, fmt.Errorf("%q names %d steps of %s, pass the id: %s", want, len(byRef), b.Guide, strings.Join(ids, "; "))
}

type stepRow struct{ ID, Ref, Title string }

// pickGoal finds a goal by id, objective key or name (case-insensitive).
func pickGoal(goals []client.Goal, want string) (client.Goal, error) {
	want = strings.TrimSpace(want)
	var hits []client.Goal
	for _, g := range goals {
		if g.ID == want || g.Key == want {
			return g, nil
		}
		if strings.EqualFold(g.Label, want) || strings.EqualFold(g.Name, want) {
			hits = append(hits, g)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return client.Goal{}, fmt.Errorf("%q names %d goals, pass the id", want, len(hits))
	}
	return client.Goal{}, fmt.Errorf("no goal %q (pin one with --add <objective key>)", want)
}
