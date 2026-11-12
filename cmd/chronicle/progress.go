package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/appenv"
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
	var baseURL, account string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Recompute the account's projections (marks, facts, verdicts, cursors) and report differences",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(account) == "" {
				return errors.New("--account is required (e.g. acc_iron_man)")
			}
			return progressCheck(cmd.Context(), cmd.OutOrStdout(), baseURL, account, appenv.Get("TOKEN"), timeout)
		},
	}
	def := appenv.Get("URL")
	if def == "" {
		def = "http://127.0.0.1:8080"
	}
	cmd.Flags().StringVar(&baseURL, "url", def, "server base URL (CHRONICLE_URL)")
	cmd.Flags().StringVar(&account, "account", "", "account id")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "request timeout")
	return cmd
}

func progressCheck(ctx context.Context, out io.Writer, baseURL, account, token string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u := strings.TrimRight(baseURL, "/") + "/api/progress/check?account=" + url.QueryEscape(account)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("progress check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("progress check: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rep struct {
		Account     string   `json:"account"`
		OK          bool     `json:"ok"`
		Differences []string `json:"differences"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
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
