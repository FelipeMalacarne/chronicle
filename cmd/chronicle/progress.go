package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// newProgressCmd groups the account progress commands. They talk to a running
// server over HTTP and never open the database (CLAUDE.md rule 1).
func newProgressCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "progress", Short: "Account progress, over the API of a running server"}
	cmd.AddCommand(newProgressCheckCmd())
	return cmd
}

// newProgressCheckCmd is `chronicle progress check` (docs/10 section 6,
// decision E3): every projection of the account recomputed from its sources
// by the server, read only. It exits 1 when any differs.
func newProgressCheckCmd() *cobra.Command {
	var f clientFlags
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Recompute the account's projections (marks, facts, verdicts, cursors) and report differences",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(f.account) == "" {
				return errors.New("--account is required (e.g. acc_iron_man)")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			return progressCheck(ctx, cmd.OutOrStdout(), c, f.account)
		}),
	}
	f.add(cmd, false)
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "request timeout")
	return cmd
}

// progressCheck asks the server for GET /api/progress/check of one account
// and prints the differences; any difference is an error (exit 1).
func progressCheck(ctx context.Context, out io.Writer, c *client.Client, account string) error {
	raw, err := c.Get(ctx, "/api/progress/check", url.Values{"account": {account}}, false)
	if err != nil {
		return fmt.Errorf("progress check: %w", err)
	}
	var rep struct {
		Account     string   `json:"account"`
		OK          bool     `json:"ok"`
		Differences []string `json:"differences"`
	}
	if err := json.Unmarshal(raw, &rep); err != nil {
		return fmt.Errorf("progress check: unreadable answer: %w", err)
	}
	if rep.OK {
		_, _ = fmt.Fprintf(out, "%s: every projection matches its sources\n", rep.Account)
		return nil
	}
	_, _ = fmt.Fprintf(out, "%s: %d differences\n", rep.Account, len(rep.Differences))
	for _, d := range rep.Differences {
		_, _ = fmt.Fprintln(out, "  "+d)
	}
	return fmt.Errorf("progress check: %d differences", len(rep.Differences))
}
