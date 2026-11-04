// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: auth login - exchanges the BMS username,
// password and tenant for a JWT via POST /v2/security/authenticate and saves
// it to the 0600 credential store, bound to the host and login it was minted
// for, so every other command sends Authorization: Bearer <jwt>. API commands
// also mint on demand from the same login (kbms_guard.go mintForClient).
//
// kaseya-bms handfixes (4.34.2 reprint): the token client is pinned to the API
// host and follows no redirect (mint-pinned-to-api-host); --dry-run sends
// nothing (auth-login-dry-run-sends-nothing); the token goes to the credential
// store, never config.toml, and is never printed.

// pp:data-source live

package cli

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"kaseya-bms-pp-cli/internal/client"
	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/config"

	"kaseya-bms-pp-cli/internal/credkit"
)

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var username string
	var tenant string
	var mfaCode string
	var grantType string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Exchange BMS username, password, and tenant for a JWT and save it",
		Long: strings.Trim(`
Authenticates against POST /v2/security/authenticate using KASEYA_BMS_USERNAME,
KASEYA_BMS_PASSWORD, and KASEYA_BMS_TENANT (or the matching flags; the password
is env-only) and saves the returned JWT + refresh token to the credential store.

With the same three variables set, every API command also mints a token on
first use and again when it expires, so this command is needed only for an API
user with MFA enabled (pass --mfa-code) or to check the login.`, "\n"),
		Example: strings.Trim(`
  # Mint and store a token from env credentials
  kaseya-bms-cli auth login

  # Explicit tenant + MFA
  kaseya-bms-cli auth login --tenant mymsp --mfa-code 123456`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			login := config.KbmsEnvLogin()
			if username != "" {
				login.Username = strings.TrimSpace(username)
			}
			if tenant != "" {
				login.Tenant = strings.TrimSpace(tenant)
			}
			login.MFACode = mfaCode
			login.GrantType = grantType
			credkit.Hold("flag:--mfa-code", mfaCode)

			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				cfg, err := config.Load(flags.configPath)
				if err != nil {
					return configErr(err)
				}
				u, _ := url.Parse(cfg.BaseURL)
				fmt.Fprintf(cmd.OutOrStdout(), "would POST %s/v2/security/authenticate as %q on tenant %q and save the returned JWT; nothing sent (--dry-run)\n",
					displayOrigin(u), login.Username, login.Tenant)
				return nil
			}
			if login.Username == "" || login.Password == "" || login.Tenant == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("missing credentials: set %s, %s, and %s (or pass --username/--tenant)",
					config.KbmsEnvUsername, config.KbmsEnvLoginVar, config.KbmsEnvTenant))
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if cfg.CredentialRouteBlocked() {
				return configErr(cfg.CredentialRefusalError())
			}
			binding := config.KbmsTokenBinding(cfg.BaseURL, login)
			if binding == "" {
				return configErr(fmt.Errorf("the Kaseya BMS base URL is not an http(s) URL with a plain host; set KASEYA_BMS_BASE_URL"))
			}
			// The token client honors the configured TLS settings and the dial
			// guard, as the API client does, and is pinned to the API host.
			hc, err := client.NewPlainHTTPClient(30*time.Second, cfg)
			if err != nil {
				return configErr(err)
			}
			guardTokenClient(hc, cfg, flags.dryRun)
			tok, err := config.MintKbmsToken(cmd.Context(), hc, cfg.BaseURL, login)
			if err != nil {
				return authErr(err)
			}

			// Clear any legacy auth_header so AuthHeader() uses the new JWT.
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(binding, "", tok.Access, tok.Refresh, tok.Expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}
			savePath := credentialSavePath(cfg)
			if flags.asJSON || flags.agent {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"logged_in":        true,
					"tenant":           login.Tenant,
					"expires":          tok.Expiry.Format(time.RFC3339),
					"credentials_path": savePath,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in to tenant %s. Token saved to %s (expires %s)\n",
				login.Tenant, savePath, tok.Expiry.Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "BMS username (default $"+config.KbmsEnvUsername+")")
	cmd.Flags().StringVar(&tenant, "tenant", "", "BMS company/tenant name from My Settings (default $"+config.KbmsEnvTenant+")")
	cmd.Flags().StringVar(&mfaCode, "mfa-code", "", "One-time MFA code, when the API user has MFA enabled")
	cmd.Flags().StringVar(&grantType, "grant-type", "password", "OAuth grant type sent to /v2/security/authenticate")
	return cmd
}
