// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// `auth set-token`: persist the Avanan application ID.
//
// Press 4.30.2 emitted newAuthSetTokenCmd unwired and avanan_auth.go wired it
// into the auth group. Press 4.34.0 emits no set-token for an API that needs
// three credential inputs, so the command lives here, unchanged in behavior:
// SaveCredential writes the value as the application ID (the field AuthHeader()
// resolves), and the signing transport mints session tokens from it plus the
// client secret. `auth login --save` persists both at once.

package cli

import (
	"avanan-pp-cli/internal/cliutil"
	"errors"
	"fmt"

	"avanan-pp-cli/internal/config"
	"github.com/spf13/cobra"
)

func newAuthSetTokenCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-token <app-id>",
		Short:   "Save the Avanan application ID to the credentials file",
		Long:    "Save the Avanan application ID to the credentials file. Session tokens are minted from it and AVANAN_CLIENT_SECRET on demand; `auth login --save` stores both.",
		Example: "  avanan-cli auth set-token YOUR_APP_ID",
		// The application ID is a credential. Args runs before every flag check
		// and pre-run, so nothing later can repeat a positional.
		Args: func(cmd *cobra.Command, args []string) error {
			for _, a := range args {
				cliutil.Hold("config:token", a)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if err := cfg.SaveCredential(args[0]); err != nil {
				return configErr(fmt.Errorf("saving application ID: %w", err))
			}
			savePath := credentialSavePath(cfg)
			if flags.asJSON {
				out := map[string]any{
					"saved":       true,
					"config_path": cfg.Path,
				}
				if !cfg.AgentcookieManagedByExternalStore() {
					out["credentials_path"] = savePath
				}
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Application ID saved to %s\n", savePath)
			return nil
		},
	}
	// A flag parse fails before Args runs and its error repeats what it read,
	// which may be the credential, so this error repeats nothing.
	cmd.SetFlagErrorFunc(func(*cobra.Command, error) error {
		return usageErr(errors.New("auth set-token: invalid flag or flag value (not repeated here: this command line carries a credential); run 'avanan-cli auth set-token --help'"))
	})
	return cmd
}
