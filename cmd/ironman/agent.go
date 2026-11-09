package main

import (
	"fmt"
	"log/slog"
	"net"
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
//	MICROBOT_AGENT_CLIENT_ID, MICROBOT_AGENT_CLIENT_SECRET (or _SECRET_FILE)
//	                       the Zitadel machine user (role microbot:agent) when the
//	                       server runs AUTH_MODE=oidc; a dev server needs nothing and
//	                       takes the link only from loopback
//	MICROBOT_HOME          the harness dev home (default ~/.local/share/microbot-dev)
//	MICROBOT_HARNESS_DIR   the microbot-plugins checkout; mb-client runs through its
//	                       Nix dev shell (Xvfb, x11vnc and the JDK come from it)
//	MICROBOT_CLIENT_CMD, MICROBOT_ACCOUNT_CMD
//	                       instead of the above, the commands that run mb-client
//	                       and mb-account
//	MB_VNC_PORT            the harness's x11vnc port (default 5999), always on loopback
func newAgentCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Link this machine's Microbot client to the planner server (docs/08)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret, err := agentSecret()
			if err != nil {
				return err
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
				URL: url, Log: log, VNCAddr: vncAddr(),
				Token: (&client.AgentAuth{URL: url, ClientID: os.Getenv("MICROBOT_AGENT_CLIENT_ID"), ClientSecret: secret}).Token,
				Hello: agent.Hello{Host: host, Version: buildinfo.String()},
				Executor: &agent.Executor{
					Game: &microbot.Client{Home: home}, Home: home,
					Bridge:    &microbot.Client{Home: home, Socket: microbot.BridgeSocket},
					ClientCmd: harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"), AccountCmd: harnessCmd("MICROBOT_ACCOUNT_CMD", "mb-account"),
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

// harnessCmd is the argv that runs one of the harness's tools: the command
// in env, else the tool through the harness's Nix dev shell.
func harnessCmd(env, tool string) []string {
	if c := strings.Fields(os.Getenv(env)); len(c) > 0 {
		return c
	}
	if dir := os.Getenv("MICROBOT_HARNESS_DIR"); dir != "" {
		return []string{"nix", "develop", dir, "--command", filepath.Join(dir, "bin", tool)}
	}
	return nil
}

// vncAddr is the harness's x11vnc: loopback only, on MB_VNC_PORT like
// mb-client.
func vncAddr() string {
	port := os.Getenv("MB_VNC_PORT")
	if port == "" {
		port = "5999"
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// agentSecret is the machine user's secret: MICROBOT_AGENT_CLIENT_SECRET, or
// the file MICROBOT_AGENT_CLIENT_SECRET_FILE names (a sops secret).
func agentSecret() (string, error) {
	if f := os.Getenv("MICROBOT_AGENT_CLIENT_SECRET_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("MICROBOT_AGENT_CLIENT_SECRET_FILE: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return os.Getenv("MICROBOT_AGENT_CLIENT_SECRET"), nil
}
