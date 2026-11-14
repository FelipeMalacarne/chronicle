package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/core/mta"
)

// newMTACmd is `chronicle mta`: the Mage Training Arena calculator over the API. Which rewards to
// earn pizazz points for, and how many points and runes that takes.
func newMTACmd() *cobra.Command {
	var (
		f       clientFlags
		want    []string
		own     []string
		points  []int
		magic   int
		enchant int
	)
	cmd := &cobra.Command{
		Use:   "mta",
		Short: "Pizazz points and runes the chosen Mage Training Arena rewards take",
		Long: `Prices Mage Training Arena rewards: what to buy (a wand upgrade also takes the wand below
it, unless --own has it), the pizazz points still missing in each room and the law, nature and
cosmic runes that earning them takes. --points are the four totals you hold, in the order
telekinetic, graveyard, enchantment, alchemist. Without --enchant the best Enchant --magic casts
is assumed. Estimates: staves cover the elemental runes. "chronicle mta rewards" lists the ids.`,
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			in := mta.Input{Rewards: want, Owned: own, Magic: magic, EnchantLevel: enchant}
			switch len(points) {
			case 0:
			case 4:
				in.Points = mta.Points{Telekinetic: points[0], Graveyard: points[1], Enchantment: points[2], Alchemist: points[3]}
			default:
				return fmt.Errorf("--points takes four numbers (telekinetic, graveyard, enchantment, alchemist), got %d", len(points))
			}
			if len(want) == 0 {
				return fmt.Errorf("--want lists no reward: see `chronicle mta rewards` for the ids")
			}
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			res, err := c.MTACalculate(ctx, in)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(out, res)
			}
			printMTA(out, res)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.Flags().StringSliceVar(&want, "want", nil, "reward ids to earn points for (comma separated)")
	cmd.Flags().StringSliceVar(&own, "own", nil, "reward ids you already have")
	cmd.Flags().IntSliceVar(&points, "points", nil, "pizazz points you hold: telekinetic,graveyard,enchantment,alchemist")
	cmd.Flags().IntVar(&magic, "magic", 0, "Magic level, to pick the Enchant of the Enchanting Chamber")
	cmd.Flags().IntVar(&enchant, "enchant", 0, "Lvl-N Enchant (1 to 6) the cosmic runes assume; default the best --magic casts")
	cmd.AddCommand(newMTARewardsCmd())
	return cmd
}

func newMTARewardsCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "rewards",
		Short: "The Rewards Guardian's stock: ids and the pizazz points each reward costs",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			t, err := c.MTAData(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(out, t)
			}
			fmt.Fprintf(out, "%-18s %6s %6s %6s %6s\n", "id", "tele", "grave", "ench", "alch")
			for _, r := range t.Rewards {
				note := ""
				if r.Requires != "" {
					note = "  (upgrade de " + r.Requires + ")"
				}
				fmt.Fprintf(out, "%-18s %6d %6d %6d %6d%s\n", r.ID, r.Cost.Telekinetic, r.Cost.Graveyard, r.Cost.Enchantment, r.Cost.Alchemist, note)
			}
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func printMTA(w io.Writer, r mta.Result) {
	if len(r.Buy) == 0 {
		fmt.Fprintln(w, "Nada a comprar: você já tem tudo o que escolheu.")
		return
	}
	fmt.Fprintf(w, "Comprar, nesta ordem: %s\n\n", strings.Join(r.Buy, ", "))
	fmt.Fprintf(w, "%-12s %8s %8s\n", "sala", "precisa", "falta")
	row := func(name string, needed, missing int) {
		fmt.Fprintf(w, "%-12s %8d %8d\n", name, needed, missing)
	}
	row("Telekinetic", r.Needed.Telekinetic, r.Missing.Telekinetic)
	row("Graveyard", r.Needed.Graveyard, r.Missing.Graveyard)
	row("Enchantment", r.Needed.Enchantment, r.Missing.Enchantment)
	row("Alchemist", r.Needed.Alchemist, r.Missing.Alchemist)
	fmt.Fprintf(w, "\nRunas: law %d, nature %d, cosmic %d", r.Runes.Law, r.Runes.Nature, r.Runes.Cosmic)
	if r.EnchantLevel > 0 {
		fmt.Fprintf(w, " (Lvl-%d Enchant)", r.EnchantLevel)
	}
	fmt.Fprintln(w, "\nEstimativa: os staves cobrem as runas elementais.")
}
