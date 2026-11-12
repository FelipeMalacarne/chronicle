package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/ironman-planner/internal/appenv"
	"github.com/FelipeMalacarne/ironman-planner/internal/client"
)

// clientFlags are the connection flags of every API client command. The
// server is --url, else CHRONICLE_URL, else config.json, else
// client.DefaultURL; the account is --account, else CHRONICLE_ACCOUNT, else
// config.json, else the user's only account. CHRONICLE_TOKEN is a bearer used
// as is, in place of the `chronicle login` credential.
type clientFlags struct {
	url     string
	account string
	json    bool
}

func (f *clientFlags) add(cmd *cobra.Command, withJSON bool) {
	cmd.Flags().StringVar(&f.url, "url", "", "planner server (default: CHRONICLE_URL, config, "+client.DefaultURL+")")
	cmd.Flags().StringVar(&f.account, "account", "", "account id (default: CHRONICLE_ACCOUNT, config, the only account)")
	if withJSON {
		cmd.Flags().BoolVar(&f.json, "json", false, "print the API's JSON")
	}
}

func clientStore() (*client.Store, error) {
	dir, err := client.DefaultDir()
	if err != nil {
		return nil, err
	}
	return &client.Store{Dir: dir}, nil
}

// resolveURL applies the precedence of the server URL.
func (f *clientFlags) resolveURL(store *client.Store) (string, client.Settings, error) {
	settings, err := store.Settings()
	if err != nil {
		return "", settings, err
	}
	for _, u := range []string{f.url, appenv.Get("URL"), settings.URL, client.DefaultURL} {
		if u = client.NormalizeURL(u); u != "" {
			return u, settings, nil
		}
	}
	return client.DefaultURL, settings, nil
}

// connect builds the client of a command.
func (f *clientFlags) connect(ctx context.Context) (*client.Client, error) {
	store, err := clientStore()
	if err != nil {
		return nil, err
	}
	url, settings, err := f.resolveURL(store)
	if err != nil {
		return nil, err
	}
	account := f.account
	if account == "" {
		account = appenv.Get("ACCOUNT")
	}
	if account == "" {
		account = settings.Account
	}
	return client.New(ctx, client.Options{URL: url, Account: account, Token: appenv.Get("TOKEN"), Store: store})
}

// explain turns the client's errors into what the user should do (PT-BR,
// like the rest of the UI).
func explain(err error) error {
	var many *client.ManyAccountsError
	var api *client.APIError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, client.ErrLoginRequired):
		return errors.New("o servidor pede login: rode `chronicle login`")
	case errors.Is(err, client.ErrLoginExpired):
		return errors.New("o login expirou: rode `chronicle login` de novo")
	case errors.Is(err, client.ErrNoAccount):
		return errors.New("nenhuma conta neste servidor: crie a conta pela web")
	case errors.As(err, &many):
		ids := make([]string, len(many.Accounts))
		for i, a := range many.Accounts {
			ids[i] = a.ID
		}
		return fmt.Errorf("várias contas (%s): use --account <id> ou `chronicle use <id>`", strings.Join(ids, ", "))
	case errors.As(err, &api) && api.Status == http.StatusUnauthorized:
		return fmt.Errorf("login recusado pelo servidor (%s): rode `chronicle login`", api.Message)
	case errors.As(err, &api):
		return fmt.Errorf("%s (HTTP %d)", api.Message, api.Status)
	}
	return err
}

// runClient wraps a command body with explain.
func runClient(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error { return explain(fn(cmd, args)) }
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newLoginCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to a planner server (device flow in oidc; nothing in dev) and make it the default",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			store, err := clientStore()
			if err != nil {
				return err
			}
			url, settings, err := f.resolveURL(store)
			if err != nil {
				return err
			}
			cred, err := client.Login(ctx, nil, url, func(p client.DevicePrompt) {
				fmt.Fprintf(out, "Abra %s\ne confirme o código %s.\nAguardando a confirmação…\n", p.URL, p.UserCode)
			})
			switch {
			case errors.Is(err, client.ErrDevMode):
				fmt.Fprintf(out, "%s está em modo dev: não precisa de login.\n", url)
			case errors.Is(err, client.ErrNoNativeClient):
				return fmt.Errorf("%s não aceita login pela CLI: falta OIDC_NATIVE_CLIENT_ID no servidor", url)
			case err != nil:
				return err
			default:
				if serr := store.SaveCredential(url, cred); serr != nil {
					return serr
				}
			}
			if settings.URL != url {
				settings.URL = url
				if serr := store.SaveSettings(settings); serr != nil {
					return serr
				}
			}
			c, err := client.New(ctx, client.Options{URL: url, Store: store})
			if err != nil {
				return err
			}
			me, err := c.Me(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Conectado a %s como %s (%d conta(s)).\n", url, me.User.Username, len(me.Accounts))
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}

func newLogoutCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Forget the login of a planner server",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			store, err := clientStore()
			if err != nil {
				return err
			}
			url, _, err := f.resolveURL(store)
			if err != nil {
				return err
			}
			ok, err := store.DeleteCredential(url)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintf(cmd.OutOrStdout(), "Nenhum login guardado para %s.\n", url)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Login de %s removido deste computador.\n", url)
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}

func newAccountsCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List your accounts on the server",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			ctx, out := cmd.Context(), cmd.OutOrStdout()
			c, err := f.connect(ctx)
			if err != nil {
				return err
			}
			me, err := c.Me(ctx)
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(out, me)
			}
			current, _ := c.Account(ctx)
			for _, a := range me.Accounts {
				mark := " "
				if a.ID == current {
					mark = "*"
				}
				fmt.Fprintf(out, "%s %-24s %-14s %s\n", mark, a.ID, a.RSN, a.GuideID)
			}
			if len(me.Accounts) == 0 {
				fmt.Fprintln(out, "Nenhuma conta: crie pela web.")
			}
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func newUseCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "use <account>",
		Short: "Make an account the default of the CLI and MCP",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := clientStore()
			if err != nil {
				return err
			}
			url, settings, err := f.resolveURL(store)
			if err != nil {
				return err
			}
			c, err := client.New(ctx, client.Options{URL: url, Token: appenv.Get("TOKEN"), Store: store})
			if err != nil {
				return err
			}
			me, err := c.Me(ctx)
			if err != nil {
				return err
			}
			for _, a := range me.Accounts {
				if a.ID == args[0] {
					settings.URL, settings.Account = url, a.ID
					if err := store.SaveSettings(settings); err != nil {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Conta padrão: %s (%s) em %s.\n", a.ID, a.RSN, url)
					return nil
				}
			}
			return fmt.Errorf("conta %q não é sua em %s (veja `chronicle accounts`)", args[0], url)
		}),
	}
	cmd.Flags().StringVar(&f.url, "url", "", "planner server (default: CHRONICLE_URL, config, "+client.DefaultURL+")")
	return cmd
}
