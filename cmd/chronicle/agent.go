package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/appenv"
	"github.com/FelipeMalacarne/ironman-planner/internal/buildinfo"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
	"github.com/FelipeMalacarne/ironman-planner/internal/microbot"
)

// newAgentCmd is `chronicle agent` (docs/08): it runs on the machine with the
// game, links OUTBOUND to the planner server and executes only the closed
// list of control commands against the Microbot dev client of the harness.
//
//	CHRONICLE_URL            the planner server (default: config.json, then https://chronicle.saradomin.ftm.dev.br)
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
//	MICROBOT_AGENT_ACCOUNT the Chronicle account this agent declares (optional)
//
// With --worker it is the agent of a worker pod (docs/11): see workerAgent.
func newAgentCmd() *cobra.Command {
	var url string
	var worker bool
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Link this machine's Microbot client to the planner server (docs/08)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if worker {
				return runWorker(cmd.Context(), url)
			}
			secret, err := agentSecret()
			if err != nil {
				return err
			}
			if url == "" {
				store, err := clientStore()
				if err != nil {
					return err
				}
				if url, _, err = (&clientFlags{}).resolveURL(store); err != nil {
					return err
				}
			}
			home := microbot.DefaultHome()
			host, _ := os.Hostname()
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			a := &agent.Agent{
				URL: url, Log: log, VNCAddr: vncAddr(),
				Token: (&client.AgentAuth{URL: url, ClientID: os.Getenv("MICROBOT_AGENT_CLIENT_ID"), ClientSecret: secret}).Token,
				// Bridge stays empty: the Chronicle Link bridge has no version route yet.
				Hello: agent.Hello{Host: host, Version: buildinfo.String(), Account: strings.TrimSpace(os.Getenv("MICROBOT_AGENT_ACCOUNT")), Kind: agent.KindAttached},
				Executor: &agent.Executor{
					Game: &microbot.Client{Home: home}, Home: home,
					Bridge:      &microbot.Client{Home: home, Socket: microbot.BridgeSocket},
					TriggerFile: agent.DefaultTriggerFile(),
					RunFile:     agent.DefaultRunFile(),
					ClientCmd:   harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"), AccountCmd: harnessCmd("MICROBOT_ACCOUNT_CMD", "mb-account"),
				},
			}
			log.Info("agent: starting", "server", url, "microbot_home", home, "client_cmd", strings.Join(a.Executor.ClientCmd, " "))
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return a.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "planner server (default: CHRONICLE_URL, config, "+client.DefaultURL+")")
	cmd.Flags().BoolVar(&worker, "worker", false, "run as the agent of a worker pod (docs/11): WORKER_ACCOUNT, CHRONICLE_AGENT_TOKEN_FILE")
	return cmd
}

// workerShutdown bounds the worker's SIGTERM: logout, then mb-client stop (which waits up
// to 30 s for the java process, then for Xvfb and x11vnc); inside the pod's 90 s
// terminationGracePeriodSeconds (internal/kube).
const workerShutdown = 75 * time.Second

// runWorker is `chronicle agent --worker`: it links until SIGTERM, then logs
// the character out and stops the client.
func runWorker(ctx context.Context, url string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	a, err := workerAgent(url, os.Getenv, log)
	if err != nil {
		return err
	}
	log.Info("agent: starting as a worker", "server", a.URL, "account", a.Hello.Account, "worker", a.Hello.Worker, "microbot_home", a.Executor.Home)
	sctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	err = a.Run(sctx)
	stop()
	log.Info("agent: worker stopping: logout and mb-client stop")
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workerShutdown)
	defer cancel()
	if serr := a.Executor.Shutdown(dctx); serr != nil {
		log.Warn("agent: worker shutdown", "err", serr)
	}
	return err
}

// workerAgent builds the agent of a worker pod from its environment (docs/11
// section 13.2):
//
//	CHRONICLE_URL               the planner as the pod reaches it (or --url); required
//	CHRONICLE_AGENT_TOKEN_FILE  the pod's projected token, read again on every connection; required
//	WORKER_ACCOUNT              the only account the worker serves; required
//	HOSTNAME                    the pod's name (the hello's worker)
//	MB_DEV_HOME                 the harness dev home (default ~/.local/share/microbot-dev)
//	MB_HWID_STORE_DIR           where the HWID agent's device files go (default $HOME/.runelite)
//	MICROBOT_CLIENT_CMD         instead of mb-client from the PATH
//	MB_VNC_PORT                 the x11vnc port (default 5999), on loopback
//
// mb-account is off (account_use is refused), and the runs' history and the
// triggers live in memory only: the pod's home is an emptyDir, and they move
// to the database in a later phase (docs/11, F6).
func workerAgent(url string, getenv func(string) string, log *slog.Logger) (*agent.Agent, error) {
	chronicle := func(name string) string {
		if v := getenv(appenv.Prefix + name); v != "" {
			return v
		}
		return getenv(appenv.LegacyPrefix + name)
	}
	if url == "" {
		url = chronicle("URL")
	}
	if url = client.NormalizeURL(url); url == "" {
		return nil, errors.New("agent --worker: CHRONICLE_URL (or --url) is required")
	}
	tokenFile := chronicle("AGENT_TOKEN_FILE")
	if tokenFile == "" {
		return nil, errors.New("agent --worker: CHRONICLE_AGENT_TOKEN_FILE is required")
	}
	account := strings.TrimSpace(getenv("WORKER_ACCOUNT"))
	if account == "" {
		return nil, errors.New("agent --worker: WORKER_ACCOUNT is required")
	}
	name := strings.TrimSpace(getenv("HOSTNAME"))
	if name == "" {
		name, _ = os.Hostname()
	}
	home := getenv("MB_DEV_HOME")
	if home == "" {
		home = microbot.DefaultHome()
	}
	hwid := getenv("MB_HWID_STORE_DIR")
	if hwid == "" {
		h, _ := os.UserHomeDir()
		hwid = filepath.Join(h, ".runelite")
	}
	clientCmd := strings.Fields(getenv("MICROBOT_CLIENT_CMD"))
	if len(clientCmd) == 0 {
		clientCmd = []string{"mb-client"}
	}
	port := getenv("MB_VNC_PORT")
	if port == "" {
		port = "5999"
	}
	return &agent.Agent{
		URL: url, Log: log, VNCAddr: net.JoinHostPort("127.0.0.1", port),
		Token: tokenFromFile(tokenFile),
		// Bridge stays empty: the Chronicle Link bridge has no version route yet.
		Hello: agent.Hello{Host: name, Version: buildinfo.String(), Account: account, Kind: agent.KindManaged, Worker: name},
		Executor: &agent.Executor{
			Game: &microbot.Client{Home: home}, Home: home, Log: log,
			Bridge:    &microbot.Client{Home: home, Socket: microbot.BridgeSocket},
			ClientCmd: clientCmd,
			Worker:    &agent.WorkerConfig{Account: account, HWIDStoreDir: hwid},
		},
	}, nil
}

// tokenFromFile reads the projected token on every connection: the kubelet
// rotates it.
func tokenFromFile(file string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		b, err := os.ReadFile(file) //nolint:gosec // the pod's own token file
		if err != nil {
			return "", fmt.Errorf("CHRONICLE_AGENT_TOKEN_FILE: %w", err)
		}
		tok := strings.TrimSpace(string(b))
		if tok == "" {
			return "", errors.New("CHRONICLE_AGENT_TOKEN_FILE is empty")
		}
		return tok, nil
	}
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
