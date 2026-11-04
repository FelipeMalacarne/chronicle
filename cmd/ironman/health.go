package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// newHealthCmd checks a running server over HTTP (the container healthcheck:
// the distroless image has no shell or curl). It never opens the database
// (CLAUDE.md rule 1).
func newHealthCmd() *cobra.Command {
	var url string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check a running server's /api/health (exit 1 unless it answers ok)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return checkHealth(cmd.Context(), url, timeout)
		},
	}
	cmd.Flags().StringVar(&url, "url", "http://127.0.0.1:8080/api/health", "health endpoint")
	cmd.Flags().DurationVar(&timeout, "timeout", 3*time.Second, "request timeout")
	return cmd
}

func checkHealth(ctx context.Context, url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Status string `json:"status"`
		DB     string `json:"db"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("health: HTTP %d, unreadable body: %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || body.Status != "ok" || body.DB == "error" {
		return fmt.Errorf("health: HTTP %d, status %q, db %q", resp.StatusCode, body.Status, body.DB)
	}
	return nil
}
