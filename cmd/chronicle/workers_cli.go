package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/agent"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
)

// launcherAccount is one entry of the Microbot launcher's accounts.json (the
// file `mb-account` reads).
type launcherAccount struct {
	DisplayName *string `json:"displayName"`
	SessionID   string  `json:"sessionId"`
	AccountID   string  `json:"accountId"`
}

// readLauncherAccount finds one character in accounts.json by display name
// (the launcher shows a character without one as "Not set").
func readLauncherAccount(path, character string) (agent.GameSession, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the launcher's file, chosen by the user
	if err != nil {
		return agent.GameSession{}, fmt.Errorf("contas do launcher em %s: %w (abra o launcher do Microbot uma vez)", path, err)
	}
	var list []launcherAccount
	if err := json.Unmarshal(b, &list); err != nil {
		return agent.GameSession{}, fmt.Errorf("contas do launcher em %s: %w", path, err)
	}
	var names []string
	for _, a := range list {
		name := "Not set"
		if a.DisplayName != nil && *a.DisplayName != "" {
			name = *a.DisplayName
		}
		names = append(names, name)
		if strings.EqualFold(name, character) {
			if a.SessionID == "" || a.AccountID == "" {
				return agent.GameSession{}, fmt.Errorf("o launcher não tem sessão para %s: abra o launcher e entre de novo", name)
			}
			return agent.GameSession{Character: name, SessionID: a.SessionID, CharacterID: a.AccountID}, nil
		}
	}
	return agent.GameSession{}, fmt.Errorf("nenhum personagem %q no launcher (há: %s)", character, strings.Join(names, ", "))
}

// hwidLines keeps the lines of an HWID agent store that belong to one
// character: those that mention its character id or its name. The rest of
// the file (the other characters' spoofed identities) never leaves this
// machine.
func hwidLines(path string, needles ...string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // a fixed file of ~/.runelite
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, n := range needles {
			if n != "" && strings.Contains(strings.ToLower(line), strings.ToLower(n)) {
				out = append(out, line)
				break
			}
		}
	}
	return strings.Join(out, "\n"), sc.Err()
}

func newAccountsConnectCmd() *cobra.Command {
	var f clientFlags
	var character, accountsFile, runeliteDir string
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Store an account's game session (from the Microbot launcher) on the server, for its worker (docs/11)",
		Long: "Reads one character from the Microbot launcher's accounts.json and, from the HWID agent's stores " +
			"in ~/.runelite, only that character's lines, and sends them to the server, which encrypts them. " +
			"Run it on the machine with the launcher; run it again when the session expires.",
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			if strings.TrimSpace(character) == "" {
				return errors.New("--character é obrigatório (o nome do personagem no launcher)")
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if accountsFile == "" {
				accountsFile = filepath.Join(home, ".microbot", "accounts.json")
			}
			if runeliteDir == "" {
				runeliteDir = filepath.Join(home, ".runelite")
			}
			session, err := readLauncherAccount(accountsFile, character)
			if err != nil {
				return err
			}
			if session.DeviceIDs, err = hwidLines(filepath.Join(runeliteDir, "spoofed-device-ids.properties"), session.CharacterID, session.Character); err != nil {
				return err
			}
			if session.RandomDats, err = hwidLines(filepath.Join(runeliteDir, "spoofed-random-dats.properties"), session.CharacterID, session.Character); err != nil {
				return err
			}
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			account, err := connectAccount(cmd, c, f.account, session.Character)
			if err != nil {
				return err
			}
			info, err := c.PutSession(ctx, account, session)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Sessão de %s guardada em %s (chave %s).\n", info.Character, account, info.KeyID)
			if session.DeviceIDs == "" && session.RandomDats == "" {
				fmt.Fprintln(out, "Aviso: nenhuma identidade de aparelho deste personagem em "+runeliteDir+"; o worker vai gerar outra.")
			}
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVar(&character, "character", "", "character name as the launcher lists it")
	cmd.Flags().StringVar(&accountsFile, "accounts-file", "", "launcher accounts (default ~/.microbot/accounts.json)")
	cmd.Flags().StringVar(&runeliteDir, "runelite-dir", "", "where the HWID agent keeps its stores (default ~/.runelite)")
	return cmd
}

// connectAccount picks the Chronicle account: --account, or the user's
// account whose RSN is the character's name.
func connectAccount(cmd *cobra.Command, c *client.Client, flag, character string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	me, err := c.Me(cmd.Context())
	if err != nil {
		return "", err
	}
	norm := func(s string) string { return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "_") }
	for _, a := range me.Accounts {
		if norm(a.RSN) == norm(character) {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("nenhuma conta sua com o RSN %q: crie pela web ou passe --account", character)
}

func newWorkersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "workers", Short: "The accounts' Microbot workers (docs/11)"}
	cmd.AddCommand(newWorkersListCmd())
	for _, action := range []struct{ name, short string }{
		{"start", "Start the account's worker in the cluster (it stops at the login screen)"},
		{"stop", "Stop the account's worker (logs out and deletes its pod)"},
		{"restart", "Restart the account's worker on the server's current image (logs out)"},
	} {
		cmd.AddCommand(newWorkerActionCmd(action.name, action.short))
	}
	return cmd
}

func newWorkersListCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the workers and the attached agents",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			c, err := f.connect(cmd.Context())
			if err != nil {
				return err
			}
			ws, err := c.ListWorkers(cmd.Context())
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), ws)
			}
			printWorkers(cmd.OutOrStdout(), ws)
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func printWorkers(out io.Writer, ws client.Workers) {
	if !ws.Enabled {
		fmt.Fprintln(out, "Workers desligados neste servidor (WORKERS=off): só as sessões ficam guardadas.")
	}
	for _, w := range ws.Workers {
		state := w.Desired
		if w.Pod != nil {
			state += " · pod " + w.Pod.State
			if w.Pod.Reason != "" {
				state += " (" + w.Pod.Reason + ")"
			}
		}
		if w.Agent != nil {
			state += " · agent ligado"
		}
		if w.Outdated {
			state += " · versão antiga"
		}
		session := "sem sessão"
		if w.Session != nil {
			session = "sessão de " + w.Session.Character
		}
		fmt.Fprintf(out, "%-24s %-14s %-40s %s\n", w.Account, w.RSN, state, session)
		if w.LastError != "" {
			fmt.Fprintf(out, "  erro: %s\n", w.LastError)
		}
	}
	for _, a := range ws.Attached {
		fmt.Fprintf(out, "%-24s %-14s anexado em %s\n", a.Account, "", a.Host)
	}
	if len(ws.Workers) == 0 && len(ws.Attached) == 0 {
		fmt.Fprintln(out, "Nenhum worker: conecte uma conta com `chronicle accounts connect`.")
	}
}

func newWorkerActionCmd(action, short string) *cobra.Command {
	var f clientFlags
	var world int
	cmd := &cobra.Command{
		Use:   action,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			account, err := c.Account(ctx)
			if err != nil {
				return err
			}
			if err := c.WorkerAction(ctx, account, action, world); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s pedido; acompanhe com `chronicle workers list`.\n", account, action)
			return nil
		}),
	}
	f.add(cmd, false)
	if action == "start" {
		cmd.Flags().IntVar(&world, "world", 0, "world to log in to later (301-999); 0 keeps the profile's")
	}
	return cmd
}
