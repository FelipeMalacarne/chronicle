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
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/appenv"
	"github.com/FelipeMalacarne/chronicle/internal/buildinfo"
	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/microbot"
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
//	MICROBOT_HARNESS_DIR   the chronicle checkout; mb-client (microbot/bin) runs through
//	                       its `microbot` Nix dev shell (Xvfb, x11vnc and the JDK)
//	MICROBOT_CLIENT_CMD, MICROBOT_ACCOUNT_CMD
//	                       instead of the above, the commands that run mb-client
//	                       and mb-account
//	MB_VNC_PORT            the harness's x11vnc port (default 5999), always on loopback
//	MICROBOT_AGENT_ACCOUNT the Chronicle account this agent declares (optional)
//	MICROBOT_AGENT_CLIENTS instead of MICROBOT_AGENT_ACCOUNT and MICROBOT_HOME, several
//	                       clients, each its own dev home and account (docs/10 section
//	                       10): see agentClients
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
				store, serr := clientStore()
				if serr != nil {
					return serr
				}
				if url, _, serr = (&clientFlags{}).resolveURL(store); serr != nil {
					return serr
				}
			}
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			a, err := attachedAgent(url, os.Getenv, log)
			if err != nil {
				return err
			}
			a.Token = (&client.AgentAuth{URL: url, ClientID: os.Getenv("MICROBOT_AGENT_CLIENT_ID"), ClientSecret: secret}).Token
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
//	WORKER_GAME_CACHE           the shared game cache volume, linked as <MB_DEV_HOME>/.runelite/jagexcache
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
		// Hello.Bridge stays empty: the hello goes out before the client and its bridge come up;
		// the bridge's version is read while the client runs (Status.Bridge).
		Hello: agent.Hello{Host: name, Version: buildinfo.String(), Account: account, Kind: agent.KindManaged, Worker: name},
		Executor: &agent.Executor{
			Game: &microbot.Client{Home: home}, Home: home, Log: log,
			Bridge:    &microbot.Client{Home: home, Socket: microbot.BridgeSocket},
			ClientCmd: clientCmd,
			Worker:    &agent.WorkerConfig{Account: account, HWIDStoreDir: hwid, GameCache: strings.TrimSpace(getenv("WORKER_GAME_CACHE"))},
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
// in env, else the tool through the checkout's `microbot` Nix dev shell. A
// directory without microbot/bin is a checkout of the old microbot-plugins
// repository (its flake has only the default shell, its tools in bin/).
func harnessCmd(env, tool string) []string {
	if c := strings.Fields(os.Getenv(env)); len(c) > 0 {
		return c
	}
	dir := os.Getenv("MICROBOT_HARNESS_DIR")
	if dir == "" {
		return nil
	}
	if bin := filepath.Join(dir, "microbot", "bin", tool); fileExists(bin) {
		return []string{"nix", "develop", dir + "#microbot", "--command", bin}
	}
	return []string{"nix", "develop", dir, "--command", filepath.Join(dir, "bin", tool)}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// attachedAgent builds the agent of the machine with the game (zaros) from
// its environment: one client (MICROBOT_HOME, MICROBOT_AGENT_ACCOUNT,
// MB_VNC_PORT) or several (MICROBOT_AGENT_CLIENTS, agentClients). The harness
// tools come from MICROBOT_CLIENT_CMD, MICROBOT_ACCOUNT_CMD or
// MICROBOT_HARNESS_DIR (harnessCmd); the caller sets the Token.
func attachedAgent(url string, getenv func(string) string, log *slog.Logger) (*agent.Agent, error) {
	host, _ := os.Hostname()
	clientCmd, accountCmd := harnessCmd("MICROBOT_CLIENT_CMD", "mb-client"), harnessCmd("MICROBOT_ACCOUNT_CMD", "mb-account")
	a := &agent.Agent{
		URL: url, Log: log,
		// Hello.Bridge stays empty: the bridge's version is in the status (Status.Bridge).
		Hello: agent.Hello{Host: host, Version: buildinfo.String(), Kind: agent.KindAttached},
	}
	specs, err := agentClients(getenv("MICROBOT_AGENT_CLIENTS"))
	if err != nil {
		return nil, err
	}
	port, err := basePort(getenv("MB_VNC_PORT"))
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		home := microbot.DefaultHome()
		a.VNCAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		a.Hello.Account = strings.TrimSpace(getenv("MICROBOT_AGENT_ACCOUNT"))
		a.Executor = &agent.Executor{
			Game: &microbot.Client{Home: home}, Home: home,
			Bridge:        &microbot.Client{Home: home, Socket: microbot.BridgeSocket},
			TriggerFile:   agent.DefaultTriggerFile(),
			RecurringFile: agent.DefaultRecurringFile(),
			RunFile:       agent.DefaultRunFile(),
			ClientCmd:     clientCmd, AccountCmd: accountCmd,
		}
		log.Info("agent: starting", "server", url, "microbot_home", home, "client_cmd", strings.Join(clientCmd, " "))
		return a, nil
	}
	if strings.TrimSpace(getenv("MICROBOT_AGENT_ACCOUNT")) != "" {
		return nil, errors.New("set MICROBOT_AGENT_CLIENTS or MICROBOT_AGENT_ACCOUNT, not both: each client names its account")
	}
	display, err := baseDisplay(getenv("MB_HEADLESS_DISPLAY"))
	if err != nil {
		return nil, err
	}
	for i, s := range specs {
		vnc := strconv.Itoa(port + i)
		a.Clients = append(a.Clients, agent.Client{
			Account: s.account, VNCAddr: net.JoinHostPort("127.0.0.1", vnc),
			Executor: &agent.Executor{
				Game: &microbot.Client{Home: s.home}, Home: s.home, Log: log.With("account", s.account),
				Bridge: &microbot.Client{Home: s.home, Socket: microbot.BridgeSocket},
				// Each client its own files: the runs, triggers and recurring
				// routines are the account's.
				TriggerFile:   clientFile(agent.DefaultTriggerFile(), s.account),
				RecurringFile: clientFile(agent.DefaultRecurringFile(), s.account),
				RunFile:       clientFile(agent.DefaultRunFile(), s.account),
				ClientCmd:     clientCmd, AccountCmd: accountCmd,
				Env: []string{"MB_DEV_HOME=" + s.home, "MB_HEADLESS_DISPLAY=:" + strconv.Itoa(display+i), "MB_VNC_PORT=" + vnc},
			},
		})
		log.Info("agent: client", "account", s.account, "microbot_home", s.home, "display", ":"+strconv.Itoa(display+i), "vnc_port", vnc)
	}
	log.Info("agent: starting", "server", url, "clients", len(specs), "client_cmd", strings.Join(clientCmd, " "))
	return a, nil
}

// clientSpec is one client of MICROBOT_AGENT_CLIENTS.
type clientSpec struct{ account, home string }

// agentClients parses MICROBOT_AGENT_CLIENTS (docs/10 section 10): the game
// clients of an agent that drives several, as "account=dev home" entries
// separated by commas, semicolons or blanks, e.g.
//
//	acc_main=~/.local/share/microbot-dev,acc_alt=~/.local/share/microbot-dev-alt
//
// Each account (acc_<rsn>) and each dev home once, at most agent.MaxClients;
// a home starting with ~/ is under the user's home, any other must be
// absolute. In this order, the clients take the headless displays and VNC
// ports from MB_HEADLESS_DISPLAY (:99) and MB_VNC_PORT (5999) up: the first
// :99 and 5999, the second :100 and 6000. Empty: nil (one client).
func agentClients(spec string) ([]clientSpec, error) {
	fields := strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ';' || unicode.IsSpace(r) })
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields) > agent.MaxClients {
		return nil, fmt.Errorf("MICROBOT_AGENT_CLIENTS: at most %d clients", agent.MaxClients)
	}
	userHome, _ := os.UserHomeDir()
	var out []clientSpec
	accounts, homes := map[string]bool{}, map[string]bool{}
	for _, f := range fields {
		account, home, ok := strings.Cut(f, "=")
		if !ok || !agent.ValidAccount(account) || home == "" {
			return nil, fmt.Errorf("MICROBOT_AGENT_CLIENTS: %q is not account=dev home, with the account as acc_<rsn>", f)
		}
		if rest, ok := strings.CutPrefix(home, "~/"); ok && userHome != "" {
			home = filepath.Join(userHome, rest)
		}
		if !filepath.IsAbs(home) {
			return nil, fmt.Errorf("MICROBOT_AGENT_CLIENTS: the dev home of %s must be an absolute path (or ~/...)", account)
		}
		home = filepath.Clean(home)
		if accounts[account] || homes[home] {
			return nil, fmt.Errorf("MICROBOT_AGENT_CLIENTS: %s or its dev home appears twice", account)
		}
		accounts[account], homes[home] = true, true
		out = append(out, clientSpec{account: account, home: home})
	}
	return out, nil
}

// basePort is MB_VNC_PORT, the x11vnc port of the (first) client: 5999 when
// unset.
func basePort(v string) (int, error) {
	if v = strings.TrimSpace(v); v == "" {
		return 5999, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1024 || p+agent.MaxClients > 65535 {
		return 0, fmt.Errorf("MB_VNC_PORT: %q is not a port between 1024 and %d", v, 65535-agent.MaxClients)
	}
	return p, nil
}

// baseDisplay is the number of MB_HEADLESS_DISPLAY, the Xvfb display of the
// first client: 99 when unset.
func baseDisplay(v string) (int, error) {
	if v = strings.TrimSpace(v); v == "" {
		return 99, nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(v, ":"))
	if err != nil || n < 1 || n > 9999 {
		return 0, fmt.Errorf("MB_HEADLESS_DISPLAY: %q is not an X display like :99", v)
	}
	return n, nil
}

// clientFile is file with the client's account before its extension
// (agent-runs.json → agent-runs.acc_x.json); "" stays "" (memory only).
func clientFile(file, account string) string {
	if file == "" {
		return ""
	}
	ext := filepath.Ext(file)
	return strings.TrimSuffix(file, ext) + "." + account + ext
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
