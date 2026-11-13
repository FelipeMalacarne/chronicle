package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

// newRecordingCmd is `chronicle recording`: the account's demonstration
// recording over the API (docs/10 section 11), printed or written to a file
// for the content MCP's propose_tasks.
func newRecordingCmd() *cobra.Command {
	var (
		f            clientFlags
		since, until string
		output       string
		maxEvents    int
	)
	cmd := &cobra.Command{
		Use:   "recording",
		Short: "Show or export the demonstration the plugin recorded",
		Long: `Read the events Chronicle Link recorded while "Record demonstration" was on
(docs/10 section 11), from --since to --until. -o writes them as JSON, the file
the content MCP's propose_tasks reads to propose a step's tasks; without it, one
line per event. --since and --until take an RFC 3339 time or a duration back
from now (2h, 30m); --since defaults to 24h.`,
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			now := time.Now().UTC()
			from, err := timeFlag(since, now, now.Add(-24*time.Hour))
			if err != nil {
				return fmt.Errorf("--since: %w", err)
			}
			to, err := timeFlag(until, now, time.Time{})
			if err != nil {
				return fmt.Errorf("--until: %w", err)
			}
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			rec, err := c.Recording(ctx, from, to, maxEvents)
			if err != nil {
				return err
			}
			if output != "" {
				if err := writeRecording(output, rec); err != nil {
					return err
				}
				fmt.Fprintf(out, "%d eventos de %s em %s\n", len(rec.Events), rec.Account, output)
				return nil
			}
			if f.json {
				return printJSON(out, rec)
			}
			printRecording(out, rec)
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.Flags().StringVar(&since, "since", "", "start: RFC 3339 time or a duration back from now (default 24h)")
	cmd.Flags().StringVar(&until, "until", "", "end: RFC 3339 time or a duration back from now (default now)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write the recording as JSON to this file")
	cmd.Flags().IntVar(&maxEvents, "max", 0, "stop after this many events (0: all)")
	cmd.AddCommand(newRecordingDeleteCmd())
	return cmd
}

func newRecordingDeleteCmd() *cobra.Command {
	var (
		f   clientFlags
		id  string
		yes bool
	)
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete the account's recording on the server (all of it, or one --recording)",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return errors.New("apagar a gravação não tem volta: confirme com --yes")
			}
			if id != "" && !recording.ValidRecordingID(id) {
				return fmt.Errorf("--recording %q não é um id de gravação", id)
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			n, err := c.DeleteRecording(ctx, id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d eventos apagados\n", n)
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVar(&id, "recording", "", "only this recording (its id, rec-...)")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the deletion")
	return cmd
}

// timeFlag reads an RFC 3339 time or a duration back from now; empty is def.
func timeFlag(s string, now, def time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d < 0 {
			return time.Time{}, errors.New("a duration back from now is positive")
		}
		return now.Add(-d), nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, errors.New("not an RFC 3339 time nor a duration (2h, 30m)")
	}
	return t.UTC(), nil
}

func writeRecording(path string, rec client.Recording) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := printJSON(file, rec); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// printRecording prints one line per event: time, kind, where and what.
func printRecording(w io.Writer, rec client.Recording) {
	if len(rec.Events) == 0 {
		fmt.Fprintln(w, "Nenhum evento gravado nesse intervalo.")
		return
	}
	for _, e := range rec.Events {
		fmt.Fprintf(w, "%s %-13s %5d,%5d,%d", e.At.Local().Format("15:04:05"), e.Kind, e.X, e.Y, e.Plane)
		for _, part := range []struct {
			label string
			v     *int
		}{{"npc", e.NPC}, {"object", e.Object}, {"item", e.Item}, {"on item", e.TargetItem}, {"x", e.Qty}} {
			if part.v != nil {
				fmt.Fprintf(w, " %s %d", part.label, *part.v)
			}
		}
		if e.Option != "" {
			fmt.Fprintf(w, " %q", e.Option)
		}
		if e.Name != "" {
			fmt.Fprintf(w, " (%s)", e.Name)
		}
		fmt.Fprintf(w, "  [%s#%d]\n", e.Recording, e.Seq)
	}
	if rec.Truncated {
		fmt.Fprintln(w, "… cortado em --max")
	}
}
