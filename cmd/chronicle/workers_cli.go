package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// launcherAccount is one entry of the Microbot launcher's accounts.json (the
// file `mb-account` reads).
type launcherAccount struct {
	DisplayName *string `json:"displayName"`
	SessionID   string  `json:"sessionId"`
	AccountID   string  `json:"accountId"`
}

// notSet is how the launcher shows a character whose display name it does not
// have (accounts.json keeps null).
const notSet = "Not set"

// launcherPick says which character of accounts.json to connect: by its
// 1-based number in the launcher's list (Index, like `mb-account use 6`) or by
// the name the launcher shows (Character); As names a character the launcher
// still lists as "Not set" (like `mb-account use --as`).
type launcherPick struct {
	Character string
	Index     int
	As        string
}

// readLauncherAccount finds one character in accounts.json.
func readLauncherAccount(path string, pick launcherPick) (agent.GameSession, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the launcher's file, chosen by the user
	if err != nil {
		return agent.GameSession{}, fmt.Errorf("contas do launcher em %s: %w (abra o launcher do Microbot uma vez)", path, err)
	}
	var list []launcherAccount
	if err := json.Unmarshal(b, &list); err != nil {
		return agent.GameSession{}, fmt.Errorf("contas do launcher em %s: %w", path, err)
	}
	names := make([]string, len(list))
	listed := make([]string, len(list))
	for i, a := range list {
		names[i] = notSet
		if a.DisplayName != nil && strings.TrimSpace(*a.DisplayName) != "" {
			names[i] = strings.TrimSpace(*a.DisplayName)
		}
		listed[i] = fmt.Sprintf("%d %s", i+1, names[i])
	}
	choice := -1
	switch {
	case pick.Index > 0:
		if pick.Index > len(list) {
			return agent.GameSession{}, fmt.Errorf("o launcher tem %d personagens (%s)", len(list), strings.Join(listed, ", "))
		}
		choice = pick.Index - 1
	default:
		for i, n := range names {
			if !strings.EqualFold(n, pick.Character) {
				continue
			}
			if choice >= 0 {
				return agent.GameSession{}, fmt.Errorf("mais de um personagem %q no launcher: escolha pelo número com --index (%s)", pick.Character, strings.Join(listed, ", "))
			}
			choice = i
		}
	}
	if choice < 0 {
		hint := ""
		if slices.Contains(names, notSet) {
			hint = `; um personagem que o launcher mostra como "Not set" se conecta com --index <número> --as <nome no jogo>`
		}
		return agent.GameSession{}, fmt.Errorf("nenhum personagem %q no launcher (há: %s)%s", pick.Character, strings.Join(listed, ", "), hint)
	}
	a, name := list[choice], names[choice]
	if as := strings.TrimSpace(pick.As); as != "" {
		name = as
	}
	if name == notSet {
		return agent.GameSession{}, errors.New(`o launcher não tem o nome desse personagem ("Not set"): passe --as <nome no jogo>`)
	}
	if a.SessionID == "" || a.AccountID == "" {
		return agent.GameSession{}, fmt.Errorf("o launcher não tem sessão para %s: abra o launcher e entre de novo", name)
	}
	return agent.GameSession{Character: name, SessionID: a.SessionID, CharacterID: a.AccountID}, nil
}

// hwidKey is the key the Microbot HWID agent (pinned hwid-agent jar) gives a
// Jagex Account character in both of its stores: the lower-case hex SHA-256 of
// "jx/<JX_CHARACTER_ID>" (DeviceIdStore.keyFor, RandomDatStore.keyFor and
// HwidAgent.currentIdentifier, read from the 2.6.52 jar).
func hwidKey(characterID string) string {
	sum := sha256.Sum256([]byte("jx/" + strings.TrimSpace(characterID)))
	return hex.EncodeToString(sum[:])
}

// hwidLines keeps the line of an HWID agent store (a Java properties file)
// whose key is key: one character's spoofed identity. The rest of the file
// (the other characters') never leaves this machine.
func hwidLines(path, key string) (string, error) {
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
		k, _, ok := strings.Cut(line, "=")
		if !ok {
			k, _, ok = strings.Cut(line, ":")
		}
		if ok && strings.TrimSpace(k) == key {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return strings.Join(out, "\n"), sc.Err()
}

func newAccountsConnectCmd() *cobra.Command {
	var f clientFlags
	var character, as, accountsFile, runeliteDir string
	var index int
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Store an account's game session (from the Microbot launcher) on the server, for its worker (docs/11)",
		Long: "Reads one character from the Microbot launcher's accounts.json and, from the HWID agent's stores " +
			"in ~/.runelite, only that character's lines, and sends them to the server, which encrypts them. " +
			"Run it on the machine with the launcher; run it again when the session expires.",
		Args: cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			if strings.TrimSpace(character) == "" && index <= 0 {
				return errors.New("diga o personagem: --character <nome no launcher> ou --index <número na lista do launcher>")
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
			session, err := readLauncherAccount(accountsFile, launcherPick{Character: character, Index: index, As: as})
			if err != nil {
				return err
			}
			if session.DeviceIDs, err = hwidLines(filepath.Join(runeliteDir, "spoofed-device-ids.properties"), hwidKey(session.CharacterID)); err != nil {
				return err
			}
			if session.RandomDats, err = hwidLines(filepath.Join(runeliteDir, "spoofed-random-dats.properties"), hwidKey(session.CharacterID)); err != nil {
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
			found := func(v string) string {
				if v == "" {
					return "não achado"
				}
				return "levado"
			}
			fmt.Fprintf(out, "Identidade de aparelho em %s: device id %s, random.dat %s.\n", runeliteDir, found(session.DeviceIDs), found(session.RandomDats))
			if session.DeviceIDs == "" || session.RandomDats == "" {
				fmt.Fprintln(out, "Aviso: o que faltou o HWID agent cria quando o personagem entra pela primeira vez num cliente do launcher; entre uma vez e rode o connect de novo, senão o worker gera uma identidade nova.")
			}
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVar(&character, "character", "", "character name as the launcher lists it")
	cmd.Flags().IntVar(&index, "index", 0, "the character's number in the launcher's list (1-based), instead of --character")
	cmd.Flags().StringVar(&as, "as", "", `the character's name in game, for one the launcher lists as "Not set"`)
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
	cmd.AddCommand(newWorkersListCmd(), newWorkerAutoLoginCmd())
	for _, action := range []struct{ name, short string }{
		{"start", "Start the account's worker in the cluster (it stops at the login screen)"},
		{"stop", "Stop the account's worker (logs out and deletes its pod)"},
		{"restart", "Restart the account's worker now on the server's current image (logs out; an outdated worker also restarts by itself at its next pause)"},
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
			state += " · versão antiga (reinicia sozinho na pausa)"
		}
		if w.AutoLogin {
			state += " · login automático"
		}
		session := "sem sessão"
		if w.Session != nil {
			session = "sessão de " + w.Session.Character
			if w.Session.LoginFailed {
				session += " · último login falhou: " + w.Session.LastLoginResult
			}
		}
		if u := w.Usage; u != nil {
			session += fmt.Sprintf(" · uso %s núcleo, %d MiB", strings.Replace(fmt.Sprintf("%.2f", u.CPUCores), ".", ",", 1), u.MemoryBytes>>20)
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
	if c := ws.Capacity; c != nil {
		fmt.Fprintf(out, "Cabe mais %d worker(s) na quota do namespace (limite: %s).\n", c.Fits, c.LimitedBy)
	}
}

func newWorkerAutoLoginCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "auto-login on|off",
		Short: "Log the worker in again after a new pod, when it was logged in (docs/11 section 15.5)",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			if args[0] != "on" && args[0] != "off" {
				return fmt.Errorf("auto-login: use on ou off, não %q", args[0])
			}
			ctx := cmd.Context()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			account, err := c.Account(ctx)
			if err != nil {
				return err
			}
			if err := c.SetAutoLogin(ctx, account, args[0] == "on"); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: login automático %s.\n", account, args[0])
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
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
