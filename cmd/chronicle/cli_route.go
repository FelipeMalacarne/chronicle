package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/chronicle/internal/core/progress"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// verdictMark is how the CLI shows a verdict; unknown is never shown as
// unmet (docs/ui decisions).
func verdictMark(v rules.Verdict) string {
	switch v {
	case rules.Met:
		return "✓"
	case rules.Unmet:
		return "✗"
	}
	return "?"
}

func newNowCmd() *cobra.Command {
	var f clientFlags
	var limit int
	cmd := &cobra.Command{
		Use:   "now",
		Short: "Show the current step of the active route, what to bring and what comes next",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			w, err := c.Route(ctx, limit)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), w)
			}
			printWindow(cmd.OutOrStdout(), w)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.Flags().IntVar(&limit, "limit", 5, "steps shown after the current one (1 to 50)")
	return cmd
}

func printWindow(out io.Writer, w panel.Window) {
	fmt.Fprintf(out, "%s · %d de %d feitos", w.Title, w.Counts.Done, w.Counts.Total)
	if w.Counts.Blocked > 0 {
		fmt.Fprintf(out, " · %d bloqueados", w.Counts.Blocked)
	}
	fmt.Fprintln(out)
	if w.Cursor == nil {
		fmt.Fprintln(out, "Rota concluída.")
		fmt.Fprintln(out, w.Attribution)
		return
	}
	if w.Section != nil {
		fmt.Fprintf(out, "Seção %s\n", w.Section.Title)
	}
	for _, s := range w.Steps {
		if !s.Current {
			continue
		}
		fmt.Fprintf(out, "\n▶ %s  %s\n", s.Ref, s.Title)
		if s.Text != "" && s.Text != s.Title {
			fmt.Fprintf(out, "  %s\n", strings.ReplaceAll(strings.TrimSpace(s.Text), "\n", "\n  "))
		}
		for _, n := range s.Notes {
			fmt.Fprintf(out, "  [%s] %s\n", n.Kind, n.Text)
		}
		if len(s.Objectives) > 0 {
			parts := make([]string, len(s.Objectives))
			for i, o := range s.Objectives {
				parts[i] = verdictMark(o.Verdict) + " " + o.Label
				if o.OwnerMarked {
					parts[i] += " (marcado por você)"
				}
			}
			fmt.Fprintf(out, "  Objetivos: %s\n", strings.Join(parts, " · "))
		}
		if len(s.Tasks) > 0 {
			// the assisted mode's checklist (docs/10 phase 2)
			fmt.Fprintln(out, "  Tarefas:")
			for _, tk := range s.Tasks {
				mark := verdictMark(tk.State)
				if tk.State == rules.Unmet {
					mark = "○" // still to do, not a failure
				}
				line := "  " + mark + " " + tk.Label
				if tk.Until != "" {
					line += " (até: " + tk.Until + ")"
				}
				fmt.Fprintln(out, line)
			}
		}
		if len(s.BlockedBy) > 0 {
			labels := make([]string, len(s.BlockedBy))
			for i, b := range s.BlockedBy {
				labels[i] = b.Label
			}
			fmt.Fprintf(out, "  Falta: %s\n", strings.Join(labels, ", "))
		}
	}
	if w.Carry != nil && (len(w.Carry.Items) > 0 || len(w.Carry.BankActions) > 0) {
		title := "Levar"
		if w.Carry.Derived {
			title = "Os próximos passos usam"
		}
		fmt.Fprintf(out, "\n%s:\n", title)
		for _, it := range w.Carry.Items {
			qty := ""
			if it.QtyStated || it.Qty > 1 {
				qty = fmt.Sprintf(" x%d", it.Qty)
			}
			have := ""
			if it.Verdict != rules.Unknown {
				have = fmt.Sprintf(" (com você: %d)", it.Carried)
			}
			fmt.Fprintf(out, "  %s %s%s%s\n", verdictMark(it.Verdict), it.Name, qty, have)
		}
		for _, b := range w.Carry.BankActions {
			fmt.Fprintf(out, "  banco: %s\n", b)
		}
	}
	var next []panel.Step
	for _, s := range w.Steps {
		if !s.Current {
			next = append(next, s)
		}
	}
	if len(next) > 0 {
		fmt.Fprintln(out, "\nDepois:")
		for _, s := range next {
			lock := " "
			if len(s.BlockedBy) > 0 {
				lock = "⊘"
			}
			fmt.Fprintf(out, "  %s %-14s %s\n", lock, s.Ref, s.Title)
		}
	}
	fmt.Fprintf(out, "\n%s\n", w.Attribution)
}

// markCmd builds done, skip and undo: the mark of one step of the active
// route, the current one by default.
func markCmd(use, short string, action coreprogress.MarkAction, verb string) *cobra.Command {
	var f clientFlags
	var note string
	cmd := &cobra.Command{
		Use:   use + " [step]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			w, err := c.Route(ctx, panel.MaxLimit)
			if err != nil {
				return err
			}
			step, err := pickStep(w, args)
			if err != nil {
				return err
			}
			var notePtr *string
			if cmd.Flags().Changed("note") {
				notePtr = &note
			}
			if err := c.Mark(ctx, w.Guide, step.ID, action, notePtr); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s: %s  %s\n", verb, firstNonEmpty(step.Ref, step.ID), step.Title)
			return printCurrent(ctx, out, c)
		}),
	}
	f.add(cmd, false)
	if action != coreprogress.ActionClear {
		cmd.Flags().StringVar(&note, "note", "", "note on the step (\"\" clears it)")
	}
	return cmd
}

// pickStep is the step named by args (its ref, as `now` shows it, or its
// id) within the window, else the window's current step. A step outside the
// window is addressed by id as is.
func pickStep(w panel.Window, args []string) (panel.Step, error) {
	if len(args) == 0 {
		for _, s := range w.Steps {
			if s.Current {
				return s, nil
			}
		}
		return panel.Step{}, fmt.Errorf("a rota %s não tem passo atual", w.Title)
	}
	want := strings.TrimSpace(args[0])
	for _, s := range w.Steps {
		if strings.EqualFold(s.Ref, want) || s.ID == want {
			return s, nil
		}
	}
	return panel.Step{ID: want}, nil
}

func printCurrent(ctx context.Context, out io.Writer, c *client.Client) error {
	w, err := c.Route(ctx, 1)
	if err != nil {
		return err
	}
	for _, s := range w.Steps {
		if s.Current {
			fmt.Fprintf(out, "Agora: %s  %s\n", s.Ref, s.Title)
			return nil
		}
	}
	fmt.Fprintln(out, "Rota concluída.")
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func newPlanCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "plan [goal]",
		Short: "List your goals, or show the ordered plan of one (by id or name)",
		Args:  cobra.MaximumNArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			goals, err := c.Goals(ctx)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				if f.json {
					return printJSON(out, goals)
				}
				if len(goals) == 0 {
					fmt.Fprintln(out, "Nenhuma meta: fixe uma no Mundo, na web.")
				}
				for _, g := range goals {
					fmt.Fprintf(out, "%s %-22s %s (%s)\n", verdictMark(rules.Verdict(g.Verdict)), g.ID, firstNonEmpty(g.Label, g.Name), g.Status)
				}
				return nil
			}
			goal, err := pickGoal(goals, args[0])
			if err != nil {
				return err
			}
			plan, err := c.GoalPlan(ctx, goal.ID)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(out, plan)
			}
			guides, err := c.Guides(ctx)
			if err != nil {
				return err
			}
			printPlan(out, plan, guides)
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

// pickGoal finds a goal by id, else by a unique piece of its label or name.
func pickGoal(goals []client.Goal, want string) (client.Goal, error) {
	var hits []client.Goal
	for _, g := range goals {
		if g.ID == want {
			return g, nil
		}
		name := strings.ToLower(g.Label + " " + g.Name)
		if strings.Contains(name, strings.ToLower(want)) {
			hits = append(hits, g)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return client.Goal{}, fmt.Errorf("nenhuma meta com %q (veja `chronicle plan`)", want)
	}
	ids := make([]string, len(hits))
	for i, g := range hits {
		ids[i] = g.ID + " " + firstNonEmpty(g.Label, g.Name)
	}
	return client.Goal{}, fmt.Errorf("%q casa com várias metas: %s", want, strings.Join(ids, "; "))
}

func printPlan(out io.Writer, gp client.GoalPlan, guides []client.GuideInfo) {
	p := gp.Plan
	fmt.Fprintf(out, "%s %s", verdictMark(rules.Verdict(gp.Goal.Verdict)), firstNonEmpty(gp.Goal.Label, gp.Goal.Name))
	if p.Requirements {
		fmt.Fprintf(out, " · %d de %d requisitos", p.Met, p.Total)
		if p.Presumed > 0 {
			fmt.Fprintf(out, " (%d presumidos)", p.Presumed)
		}
	}
	fmt.Fprintln(out)
	if len(p.Bottlenecks) > 0 {
		fmt.Fprintln(out, "\nNíveis:")
		for _, b := range p.Bottlenecks {
			have := "?"
			if b.Level != nil {
				have = fmt.Sprint(*b.Level)
			}
			mark := "✗"
			if b.Met {
				mark = "✓"
			}
			if b.Level == nil && !b.Met {
				mark = "?"
			}
			fmt.Fprintf(out, "  %s %-12s %s/%d%s\n", mark, b.Skill, have, b.Target, placeSuffix(b.Step))
		}
	}
	if len(p.Nodes) > 1 {
		fmt.Fprintln(out, "\nOrdem sugerida:")
		for _, n := range p.Nodes {
			if n.Depth == 0 {
				continue
			}
			mark := verdictMark(n.Verdict)
			if n.Presumed {
				mark = "~"
			}
			fmt.Fprintf(out, "  %s%s %s%s\n", strings.Repeat("  ", n.Depth-1), mark, n.Label, placeSuffix(n.Step))
		}
	}
	used := map[string]bool{}
	if len(p.Routes) > 0 {
		fmt.Fprintln(out, "\nOnde as rotas fazem:")
		for _, r := range p.Routes {
			used[r.GuideID] = true
			done := ""
			if r.Done {
				done = " ✓"
			}
			fmt.Fprintf(out, "  %-16s %-14s %s%s\n", r.GuideID, r.Ref, r.Title, done)
		}
	}
	for _, n := range p.Nodes {
		if n.Step != nil {
			used[n.Step.GuideID] = true
		}
	}
	var credits []string
	for _, g := range guides {
		if used[g.ID] {
			credits = append(credits, panel.Attribution(&content.Guide{Title: g.Title, Source: g.Source}))
		}
	}
	sort.Strings(credits)
	if len(credits) > 0 {
		fmt.Fprintf(out, "\n%s\n", strings.Join(credits, " · "))
	}
}

func placeSuffix(s *coreprogress.StepPlace) string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("  → %s %s", s.Ref, s.Title)
}

func newSyncCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync the account from the hiscores now",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			run, err := c.Sync(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(out, run)
			}
			fmt.Fprintf(out, "Sync %s: %s, %d passo(s) mudaram.\n", run.Source, run.Status, run.StepsChanged)
			if run.Error != "" {
				fmt.Fprintf(out, "Erro: %s\n", run.Error)
			}
			if run.NextAllowedAt != nil {
				fmt.Fprintf(out, "Próximo sync a partir de %s.\n", run.NextAllowedAt.Local().Format("15:04"))
			}
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func newStateCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:       "state [skills|quests|diaries]",
		Short:     "Show the account's state as the planner knows it",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"skills", "quests", "diaries"},
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			st, err := c.AccountState(ctx)
			if err != nil {
				return err
			}
			what := "skills"
			if len(args) == 1 {
				what = args[0]
			}
			switch {
			case f.json && what == "skills":
				return printJSON(out, st.Skills)
			case f.json && what == "quests":
				return printJSON(out, st.Quests)
			case f.json:
				return printJSON(out, st.Diaries)
			}
			fmt.Fprintf(out, "%s (%s)", st.Account.RSN, st.Account.ID)
			if st.TotalLevel > 0 {
				fmt.Fprintf(out, " · total %d", st.TotalLevel)
			}
			if st.QuestPoints != nil {
				fmt.Fprintf(out, " · %d QP", *st.QuestPoints)
			}
			fmt.Fprintln(out)
			switch what {
			case "skills":
				for _, s := range st.Skills {
					fmt.Fprintf(out, "  %-14s %3d  %12d xp\n", s.Skill, s.Level, s.XP)
				}
			case "quests":
				for _, q := range st.Quests {
					fmt.Fprintf(out, "  %-12s %s\n", q.State, q.Name)
				}
			case "diaries":
				for _, d := range st.Diaries {
					mark := "✗"
					if d.Complete {
						mark = "✓"
					}
					fmt.Fprintf(out, "  %s %s %s\n", mark, d.Area, d.Tier)
				}
			}
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}
