// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: auth login - exchanges BMS username/password/
// tenant for a JWT via POST /v2/security/authenticate and stores it in the
// config file, so every other command can send Authorization: Bearer <jwt>.

// pp:data-source live

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/config"
)

// kbmsAuthEnvUsername and friends are the env vars auth login reads when the
// matching flag is not provided. The password is env-only by design: passing
// secrets through argv leaks them into shell history and process listings.
const (
	kbmsAuthEnvUsername = "KASEYA_BMS_USERNAME"
	kbmsAuthEnvPassword = "KASEYA_BMS_PASSWORD" // #nosec G101 -- env-var NAME, not a credential; the actual secret is read from os.Getenv at runtime
	kbmsAuthEnvTenant   = "KASEYA_BMS_TENANT"
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
is env-only) and stores the returned JWT + refresh token in the config file.

BMS JWTs are short-lived: when commands start failing with 401 Security Error
(code 978001), run this again. API users with MFA enabled pass --mfa-code.`, "\n"),
		Example: strings.Trim(`
  # Mint and store a token from env credentials
  kaseya-bms-cli auth login

  # Explicit tenant + MFA
  kaseya-bms-cli auth login --tenant mymsp --mfa-code 123456`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if username == "" {
				username = os.Getenv(kbmsAuthEnvUsername)
			}
			if tenant == "" {
				tenant = os.Getenv(kbmsAuthEnvTenant)
			}
			password := os.Getenv(kbmsAuthEnvPassword)

			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would authenticate as %q against tenant %q and store the returned JWT\n", username, tenant)
				return nil
			}
			if username == "" || password == "" || tenant == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("missing credentials: set %s, %s, and %s (or pass --username/--tenant)",
					kbmsAuthEnvUsername, kbmsAuthEnvPassword, kbmsAuthEnvTenant))
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}

			body := &bytes.Buffer{}
			form := multipart.NewWriter(body)
			fields := map[string]string{
				"UserName":  username,
				"Password":  password,
				"Tenant":    tenant,
				"GrantType": grantType,
			}
			if mfaCode != "" {
				fields["MFACode"] = mfaCode
			}
			for key, value := range fields {
				if err := form.WriteField(key, value); err != nil {
					return fmt.Errorf("building login form: %w", err)
				}
			}
			if err := form.Close(); err != nil {
				return fmt.Errorf("building login form: %w", err)
			}

			url := strings.TrimRight(cfg.BaseURL, "/") + "/v2/security/authenticate"
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, url, body)
			if err != nil {
				return fmt.Errorf("building login request: %w", err)
			}
			req.Header.Set("Content-Type", form.FormDataContentType())
			req.Header.Set("Accept", "application/json")

			httpClient := &http.Client{Timeout: 30 * time.Second}
			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("calling %s: %w", url, err)
			}
			defer resp.Body.Close()
			payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if err != nil {
				return fmt.Errorf("reading login response: %w", err)
			}

			var envelope map[string]any
			if err := json.Unmarshal(payload, &envelope); err != nil {
				return fmt.Errorf("authentication failed: HTTP %d with non-JSON body", resp.StatusCode)
			}
			if resp.StatusCode != http.StatusOK || !kbmsBool(envelope, "Success") {
				return fmt.Errorf("authentication failed (HTTP %d): %s", resp.StatusCode, kbmsAuthErrorDetail(envelope))
			}
			result, _ := kbmsVal(envelope, "Result")
			resultMap, _ := result.(map[string]any)
			if resultMap == nil {
				return fmt.Errorf("authentication response missing Result payload")
			}
			accessToken := kbmsStr(resultMap, "AccessToken")
			if accessToken == "" {
				return fmt.Errorf("authentication response missing AccessToken")
			}
			refreshToken := kbmsStr(resultMap, "RefreshToken")
			expiry, hasExpiry := kbmsTime(resultMap, "AccessTokenExpireOn")
			if !hasExpiry {
				// BMS access tokens default to a short lifetime; record a
				// conservative one so auth status can warn before expiry.
				expiry = time.Now().Add(1 * time.Hour)
			}

			// Clear any legacy auth_header so AuthHeader() uses the new JWT
			// (same shadowing fix as auth set-token).
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens("", "", accessToken, refreshToken, expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			if flags.asJSON || flags.agent {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"logged_in":   true,
					"tenant":      tenant,
					"expires":     expiry.Format(time.RFC3339),
					"config_path": cfg.Path,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in to tenant %s. Token stored in %s (expires %s)\n",
				tenant, cfg.Path, expiry.Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "BMS username (default $"+kbmsAuthEnvUsername+")")
	cmd.Flags().StringVar(&tenant, "tenant", "", "BMS company/tenant name from My Settings (default $"+kbmsAuthEnvTenant+")")
	cmd.Flags().StringVar(&mfaCode, "mfa-code", "", "One-time MFA code, when the API user has MFA enabled")
	cmd.Flags().StringVar(&grantType, "grant-type", "password", "OAuth grant type sent to /v2/security/authenticate")
	return cmd
}

// kbmsAuthErrorDetail extracts the most useful message from a BMS error
// envelope: error.details, then error.message, then a generic fallback.
func kbmsAuthErrorDetail(envelope map[string]any) string {
	errVal, _ := kbmsVal(envelope, "Error")
	errMap, _ := errVal.(map[string]any)
	if errMap != nil {
		if details := kbmsStr(errMap, "Details"); details != "" {
			return strings.TrimSpace(details)
		}
		if message := kbmsStr(errMap, "Message"); message != "" {
			return strings.TrimSpace(message)
		}
	}
	return "check username, password, tenant, and KASEYA_BMS_BASE_URL region"
}
