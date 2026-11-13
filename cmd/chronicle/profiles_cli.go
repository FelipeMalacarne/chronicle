package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// The workers' client profiles (docs/11 section 4.3, phase F5): `show`
// prints a profile in the format `put` reads, so a profile is edited as a
// file. A change reaches a worker on its next Iniciar or Reiniciar.

const profileApplies = "Vale no próximo Iniciar ou Reiniciar do worker."

func newProfilesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "profiles", Short: "The workers' client profiles: plugins and settings (docs/11 section 4.3)"}
	cmd.AddCommand(newProfilesListCmd(), newProfilesShowCmd(), newProfilesPutCmd(), newProfilesDeleteCmd(),
		newProfilesUseCmd(), newProfilesAccountCmd())
	return cmd
}

func newProfilesListCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your client profiles",
		Args:  cobra.NoArgs,
		RunE: runClient(func(cmd *cobra.Command, _ []string) error {
			c, err := f.connect(cmd.Context())
			if err != nil {
				return err
			}
			list, err := c.ListProfiles(cmd.Context())
			if err != nil {
				return err
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), list)
			}
			out := cmd.OutOrStdout()
			for _, p := range list {
				line := fmt.Sprintf("%-24s %d plugins, %d configurações", p.Name, len(p.Plugins), len(p.Settings))
				if p.Parent != "" {
					line += " · modelo " + p.Parent
				}
				if len(p.Accounts) > 0 {
					line += " · contas " + strings.Join(p.Accounts, ", ")
				}
				fmt.Fprintln(out, line)
			}
			if len(list) == 0 {
				fmt.Fprintln(out, "Nenhum perfil: crie com `chronicle profiles put <nome> -f perfil.json`.")
			}
			return nil
		}),
	}
	f.add(cmd, true)
	return cmd
}

func newProfilesShowCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a profile as the JSON `profiles put` reads",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			c, err := f.connect(cmd.Context())
			if err != nil {
				return err
			}
			p, err := c.GetProfile(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), p.ProfileDef)
		}),
	}
	f.add(cmd, false)
	return cmd
}

func newProfilesPutCmd() *cobra.Command {
	var f clientFlags
	var file string
	cmd := &cobra.Command{
		Use:   "put <name> -f <file>",
		Short: "Create or replace a profile from a JSON file (- for stdin)",
		Long: `Create or replace a profile from a JSON file (- for stdin):

  {
    "parent": "model-name",
    "plugins": [
      {"source": "hub", "name": "GotrPlugin", "version": "1.2.3", "class_name": "net.runelite...GotrPlugin", "start": true},
      {"source": "own", "name": "net.runelite.client.plugins.microbot.fm.catswap.CatSwapPlugin"},
      {"source": "core", "name": "net.runelite.client.plugins.entityhider.EntityHiderPlugin", "start": true}
    ],
    "settings": [{"group": "entityhider", "key": "hidePlayers", "value": "true"}]
  }

parent is optional (a model has no model of its own). The settings follow the
rules of the Cockpit's config: no runelite, microbot, agentServer or
fm-ironmanplanner group, no key that looks like a secret.`,
		Args: cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			var def client.ProfileDef
			if err := readJSONFile(cmd, file, &def); err != nil {
				return err
			}
			c, err := f.connect(cmd.Context())
			if err != nil {
				return err
			}
			p, err := c.PutProfile(cmd.Context(), args[0], def)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Perfil %s gravado: %d plugins, %d configurações. %s\n", p.Name, len(p.Plugins), len(p.Settings), profileApplies)
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().StringVarP(&file, "file", "f", "", "the profile's JSON (- for stdin)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func newProfilesDeleteCmd() *cobra.Command {
	var f clientFlags
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a profile no account or profile uses",
		Args:  cobra.ExactArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			c, err := f.connect(cmd.Context())
			if err != nil {
				return err
			}
			if err := c.DeleteProfile(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Perfil %s apagado.\n", args[0])
			return nil
		}),
	}
	f.add(cmd, false)
	return cmd
}

func newProfilesUseCmd() *cobra.Command {
	var f clientFlags
	var none bool
	cmd := &cobra.Command{
		Use:   "use [<name>]",
		Short: "Set the account's profile (--none for none); the account's own settings stay",
		Args:  cobra.MaximumNArgs(1),
		RunE: runClient(func(cmd *cobra.Command, args []string) error {
			if none == (len(args) == 1) {
				return errors.New("diga o perfil ou --none")
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
			cur, err := c.AccountClient(ctx, account)
			if err != nil {
				return err
			}
			profile := ""
			if len(args) == 1 {
				profile = args[0]
			}
			ac, err := c.SetAccountClient(ctx, account, profile, cur.Settings)
			if err != nil {
				return err
			}
			if ac.Profile == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: sem perfil. %s\n", account, profileApplies)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: perfil %s (%d plugins, %d configurações no efetivo). %s\n",
				account, ac.Profile, len(ac.Effective.Plugins), len(ac.Effective.Settings), profileApplies)
			return nil
		}),
	}
	f.add(cmd, false)
	cmd.Flags().BoolVar(&none, "none", false, "no profile: the worker applies its fixed settings only")
	return cmd
}

func newProfilesAccountCmd() *cobra.Command {
	var f clientFlags
	var file string
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Show the account's profile, its own settings and the effective profile; -f replaces the own settings",
		Long: `Show the account's profile, its own settings and what its worker gets
(the model, the profile and the own settings, merged). With -f, replace the
account's own settings from a JSON list (- for stdin):

  [{"group": "blackjacker", "key": "room", "value": "THUG_TENT"}]`,
		Args: cobra.NoArgs,
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
			ac, err := c.AccountClient(ctx, account)
			if err != nil {
				return err
			}
			if file != "" {
				var settings []agent.ProfileSetting
				if readErr := readJSONFile(cmd, file, &settings); readErr != nil {
					return readErr
				}
				if ac, err = c.SetAccountClient(ctx, account, ac.Profile, settings); err != nil {
					return err
				}
			}
			if f.json {
				return printJSON(cmd.OutOrStdout(), ac)
			}
			printAccountClient(cmd.OutOrStdout(), ac)
			if file != "" {
				fmt.Fprintln(cmd.OutOrStdout(), profileApplies)
			}
			return nil
		}),
	}
	f.add(cmd, true)
	cmd.Flags().StringVarP(&file, "file", "f", "", "replace the account's own settings with this JSON list (- for stdin)")
	return cmd
}

func printAccountClient(out io.Writer, ac client.AccountClient) {
	profile := ac.Profile
	if profile == "" {
		profile = "nenhum"
	}
	fmt.Fprintf(out, "%s · perfil %s · %d configurações próprias\n", ac.AccountID, profile, len(ac.Settings))
	fmt.Fprintln(out, "Efetivo:")
	for _, p := range ac.Effective.Plugins {
		start := ""
		if p.Start {
			start = " · liga no boot"
		}
		version := ""
		if p.Version != "" {
			version = " " + p.Version
		}
		fmt.Fprintf(out, "  plugin %s %s%s%s\n", p.Source, p.Name, version, start)
	}
	for _, s := range ac.Effective.Settings {
		fmt.Fprintf(out, "  %s.%s = %s\n", s.Group, s.Key, s.Value)
	}
	if len(ac.Effective.Plugins)+len(ac.Effective.Settings) == 0 {
		fmt.Fprintln(out, "  nada além do fixo do worker (Chronicle Link, FPS Control a 20)")
	}
}

// readJSONFile decodes file ("-" is stdin) into v, refusing unknown fields.
func readJSONFile(cmd *cobra.Command, file string, v any) error {
	var (
		raw []byte
		err error
	)
	if file == "-" {
		raw, err = io.ReadAll(cmd.InOrStdin())
	} else {
		raw, err = os.ReadFile(file) //nolint:gosec // a file the user names
	}
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}
