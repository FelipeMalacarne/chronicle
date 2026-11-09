package main

import (
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
	"github.com/FelipeMalacarne/ironman-planner/internal/microbot"
)

// newAgentCmd is `ironman agent` (docs/08): it runs on the machine with the
// game, links OUTBOUND to the planner server and executes only the closed
// list of control commands against the Microbot dev client of the harness.
//
//	IRONMAN_URL            the planner server (default http://localhost:8080)
//	MICROBOT_AGENT_TOKEN   the machine token the server expects (required)
//	MICROBOT_HOME          the harness dev home (default ~/.local/share/microbot-dev)
//	MICROBOT_HARNESS_DIR   the microbot-plugins checkout; mb-client runs through its
//	                       Nix dev shell (Xvfb, x11vnc and the JDK come from it)
//	MICROBOT_CLIENT_CMD    instead of the above, the command that runs mb-client
func newAgentCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Link this machine's Microbot client to the planner server (docs/08)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			token := os.Getenv("MICROBOT_AGENT_TOKEN")
			if token == "" {
				return errors.New("MICROBOT_AGENT_TOKEN is required (the same value as the server's)")
			}
			if url == "" {
				url = os.Getenv("IRONMAN_URL")
			}
			if url == "" {
				url = client.DefaultURL
			}
			home := microbot.DefaultHome()
			host, _ := os.Hostname()
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			a := &agent.Agent{
				URL: url, Token: token, Log: log,
				Hello: agent.Hello{Host: host, Version: buildinfo.String()},
				Executor: &agent.Executor{
					Game: &microbot.Client{Home: home}, Home: home, ClientCmd: clientCmd(),
				},
			}
			log.Info("agent: starting", "server", url, "microbot_home", home, "client_cmd", strings.Join(a.Executor.ClientCmd, " "))
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return a.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "planner server (default: IRONMAN_URL, "+client.DefaultURL+")")
	return cmd
}

// clientCmd is the argv that runs the harness's mb-client.
func clientCmd() []string {
	if c := strings.Fields(os.Getenv("MICROBOT_CLIENT_CMD")); len(c) > 0 {
		return c
	}
	if dir := os.Getenv("MICROBOT_HARNESS_DIR"); dir != "" {
		return []string{"nix", "develop", dir, "--command", filepath.Join(dir, "bin", "mb-client")}
	}
	return nil
}
