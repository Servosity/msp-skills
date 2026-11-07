// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: Datto RMM's primary auth path. The API issues ~100h bearer
// tokens via an OAuth2 password grant from an API key + secret key. `auth
// login` performs that exchange explicitly and saves the token to the
// credential store, bound to the host and API key it was minted for; normal
// API commands also mint on demand (credkit_client.go mintForClient).
package cli

import (
	"fmt"
	"time"

	"datto-rmm-pp-cli/internal/client"
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
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Dials out to the OAuth endpoint, so short-circuit under the
			// verifier (it sets PRINTING_PRESS_VERIFY=1) per the side-effect rule.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would mint an OAuth token from DATTO_RMM_API_KEY/DATTO_RMM_API_SECRET_KEY")
				return nil
			}

			// issue #270: DATTO_RMM_NO_CONFIG_WRITE makes the automatic token
			// cache a no-op. A command whose whole purpose is to write a
			// credential to disk must not silently do nothing and report
			// success, so it refuses here and names the variable.
			if config.NoConfigWrite() {
				return configErr(fmt.Errorf("%s is set, so no credential may be written to disk; unset it and re-run to save one", config.NoConfigWriteEnv))
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if cfg.CredentialRouteBlocked() {
				return configErr(cfg.CredentialRefusalError())
			}
			if cfg.DattoRmmApiKey == "" || cfg.DattoRmmApiSecretKey == "" {
				return authErr(fmt.Errorf("set DATTO_RMM_API_KEY and DATTO_RMM_API_SECRET_KEY (and DATTO_RMM_PLATFORM or DATTO_RMM_API_URL) before running auth login"))
			}
			binding := config.DattoTokenBinding(cfg.BaseURL, cfg.DattoRmmApiKey)
			if binding == "" {
				return configErr(fmt.Errorf("the Datto RMM base URL is not an http(s) URL with a plain host; set DATTO_RMM_PLATFORM or DATTO_RMM_BASE_URL"))
			}
			// datto-rmm handfix dry-run-never-mints: --dry-run names the
			// request and sends nothing.
			if flags.dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "would POST %s/auth/oauth/token (password grant) and save the token; nothing sent (--dry-run)\n", config.DattoOrigin(cfg.BaseURL))
				return nil
			}

			// The token client honors the configured TLS settings
			// (DATTO_RMM_CA_CERT, DATTO_RMM_TLS_SERVER_NAME) and the dial guard,
			// as the API client does (tests-helper finding
			// auth-login-ignores-tls-settings: a bare http.Client refused a
			// host the API commands reach through the configured CA).
			hc, err := client.NewPlainHTTPClient(30*time.Second, cfg)
			if err != nil {
				return configErr(err)
			}
			token, expiresIn, err := config.MintDattoToken(cmd.Context(), config.NewDattoTokenClient(cfg.BaseURL, hc.Transport), cfg.BaseURL, cfg.DattoRmmApiKey, cfg.DattoRmmApiSecretKey)
			if err != nil {
				return authErr(err)
			}

			expiry := time.Time{}
			if expiresIn > 0 {
				expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
			}
			// Clear any legacy auth_header so AuthHeader() uses the fresh token.
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(binding, "", token, "", expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}
			savePath := credentialSavePath(cfg)
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"authenticated":    true,
					"expires_in":       expiresIn,
					"credentials_path": savePath,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Authenticated. Token saved to %s (expires in ~%dh).\n", savePath, expiresIn/3600)
			return nil
		},
	}
}
