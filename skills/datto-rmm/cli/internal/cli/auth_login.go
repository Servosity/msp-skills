// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: Datto RMM's primary auth path. The API issues ~100h bearer
// tokens via an OAuth2 password grant from an API key + secret key. `auth
// login` performs that exchange explicitly and saves the token; normal API
// commands also mint on demand via config.EnsureDattoToken in newClient.
package cli

import (
	"fmt"
	"time"

	"datto-rmm-pp-cli/internal/cliutil"
	"datto-rmm-pp-cli/internal/config"

	"github.com/spf13/cobra"
)

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Mint and save an OAuth token from your Datto RMM API key + secret key",
		Example: "  export DATTO_RMM_API_KEY=... DATTO_RMM_API_SECRET_KEY=... DATTO_RMM_PLATFORM=merlot\n" +
			"  datto-rmm-cli auth login",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Dials out to the OAuth endpoint, so short-circuit under the
			// verifier (it sets PRINTING_PRESS_VERIFY=1) per the side-effect rule.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would mint an OAuth token from DATTO_RMM_API_KEY/DATTO_RMM_API_SECRET_KEY")
				return nil
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if err := config.ResolveDattoBaseURL(cfg); err != nil {
				return configErr(err)
			}
			if cfg.DattoRmmApiKey == "" || cfg.DattoRmmApiSecretKey == "" {
				return authErr(fmt.Errorf("set DATTO_RMM_API_KEY and DATTO_RMM_API_SECRET_KEY (and DATTO_RMM_PLATFORM or DATTO_RMM_API_URL) before running auth login"))
			}

			token, expiresIn, err := config.MintDattoToken(cmd.Context(), cfg.BaseURL, cfg.DattoRmmApiKey, cfg.DattoRmmApiSecretKey)
			if err != nil {
				return authErr(err)
			}

			expiry := cfg.TokenExpiry
			if expiresIn > 0 {
				expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
			}
			// Clear any legacy auth_header so AuthHeader() uses the fresh token.
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, token, cfg.RefreshToken, expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"authenticated": true,
					"expires_in":    expiresIn,
					"config_path":   cfg.Path,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Authenticated. Token saved to %s (expires in ~%dh).\n", cfg.Path, expiresIn/3600)
			return nil
		},
	}
}
