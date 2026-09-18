// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"n-central-pp-cli/internal/config"
	"n-central-pp-cli/internal/ncauth"
)

// newAuthLoginCmd performs the N-central JWT -> access-token exchange explicitly
// and caches the result. Normal commands trigger this lazily on the first live
// call; `auth login` lets a user verify credentials up front (and is handy in
// CI before a batch of calls).
func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "login",
		Short:   "Exchange your N-central JWT for an access token and cache it",
		Example: "  n-central-cli auth login",
		Annotations: map[string]string{
			"mcp:hidden": "true", // auth setup is a human action, not an agent tool
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// issue #270: N_CENTRAL_NO_CONFIG_WRITE makes the automatic token
			// cache a no-op. A command whose whole purpose is to write a
			// credential to disk must not silently do nothing and report
			// success, so it refuses here and names the variable.
			if config.NoConfigWrite() {
				return configErr(fmt.Errorf("%s is set, so no credential may be written to disk; unset it and re-run to save one", config.NoConfigWriteEnv))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if c.Config == nil || c.Config.NcentralJwt == "" {
				return fmt.Errorf("no N-central JWT configured: set NCENTRAL_JWT or run 'n-central-cli auth set-token <jwt>' first")
			}
			// Force a fresh exchange even if a cached access token still looks
			// valid: clearing AccessToken + RefreshToken makes ncauth.Ensure
			// fall through to a full JWT exchange.
			c.Config.AccessToken = ""
			c.Config.RefreshToken = ""
			if err := ncauth.Ensure(cmd.Context(), c.Config); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Authenticated to N-central. Access token cached; it will auto-refresh before each batch of calls.")
			return nil
		},
	}
}
