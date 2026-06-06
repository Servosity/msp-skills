// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored Acronis-specific auth extension: OAuth2 client_credentials
// login against the datacenter-scoped IDP token endpoint. Kept in its own
// file (separate from the generated auth.go) so regen-merge preserves it.

package cli

import (
	"acronis-pp-cli/internal/cliutil"
	"acronis-pp-cli/internal/config"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// resolveDatacenter mirrors config.Load's normalization for the {datacenter}
// template var: env fallback + scheme/trailing-slash strip + default.
func resolveDatacenter(flag string) string {
	dc := strings.TrimSpace(flag)
	if dc == "" {
		dc = strings.TrimSpace(os.Getenv("ACRONIS_DATACENTER"))
	}
	if dc == "" {
		dc = "eu2-cloud"
	}
	dc = strings.TrimRight(dc, "/")
	if len(dc) >= 8 && strings.EqualFold(dc[:8], "https://") {
		dc = dc[8:]
	} else if len(dc) >= 7 && strings.EqualFold(dc[:7], "http://") {
		dc = dc[7:]
	}
	return dc
}

// newAuthLoginCmd performs the OAuth2 client_credentials exchange against the
// Acronis IDP token endpoint and persists the resulting access token using the
// same config save-path as `auth set-token`. It mutates stored credentials, so
// it is intentionally NOT marked mcp:read-only.
func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var clientID, clientSecret, datacenter string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Exchange client credentials for an access token (OAuth2 client_credentials)",
		Long: `Perform an OAuth2 client_credentials grant against the Acronis IDP token
endpoint and save the resulting access token to the local config.

Credentials and datacenter are read from flags, falling back to the
ACRONIS_CLIENT_ID, ACRONIS_CLIENT_SECRET, and ACRONIS_DATACENTER env vars.`,
		Example: `  acronis-cli auth login --client-id ID --client-secret SECRET --datacenter us-cloud
  ACRONIS_CLIENT_ID=ID ACRONIS_CLIENT_SECRET=SECRET acronis-cli auth login
  acronis-cli auth login --client-id ID --client-secret SECRET --dry-run`,
		// Mutates stored credentials — do NOT add mcp:read-only here.
		Annotations: map[string]string{},
		RunE: func(cmd *cobra.Command, args []string) error {
			id := clientID
			if id == "" {
				id = os.Getenv("ACRONIS_CLIENT_ID")
			}
			secret := clientSecret
			if secret == "" {
				secret = os.Getenv("ACRONIS_CLIENT_SECRET")
			}
			dc := resolveDatacenter(datacenter)
			tokenURL := fmt.Sprintf("https://%s.acronis.com/api/2/idp/token", dc)

			// No credentials and a help-like invocation: print help, exit 0,
			// rather than erroring. Avoids MarkFlagRequired (breaks verify).
			if id == "" || secret == "" {
				if dryRunOK(flags) || cliutil.IsVerifyEnv() {
					// Under dry-run/verify we still want to show the URL we
					// WOULD POST to, even without creds.
					fmt.Fprintf(cmd.OutOrStdout(), "would POST %s (no credentials provided; this is a dry run)\n", tokenURL)
					return nil
				}
				if len(args) == 0 {
					return cmd.Help()
				}
				return authErr(fmt.Errorf("missing credentials: provide --client-id/--client-secret or set ACRONIS_CLIENT_ID/ACRONIS_CLIENT_SECRET"))
			}

			// Dry-run / verify: never dial out.
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would POST %s\n", tokenURL)
				fmt.Fprintln(cmd.OutOrStdout(), "would exchange client_credentials for an access token (no network call made)")
				return nil
			}

			form := url.Values{}
			form.Set("grant_type", "client_credentials")
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
			if err != nil {
				return fmt.Errorf("building token request: %w", err)
			}
			basic := base64.StdEncoding.EncodeToString([]byte(id + ":" + secret))
			req.Header.Set("Authorization", "Basic "+basic)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Accept", "application/json")

			httpClient := &http.Client{Timeout: 30 * time.Second}
			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("calling token endpoint %s: %w", tokenURL, err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

			if resp.StatusCode != http.StatusOK {
				hint := ""
				if resp.StatusCode == http.StatusUnauthorized {
					hint = fmt.Sprintf(" (401 — check the --datacenter %q is correct and the client id/secret are valid)", dc)
				}
				return authErr(fmt.Errorf("token endpoint returned %d%s: %s", resp.StatusCode, hint, strings.TrimSpace(string(body))))
			}

			var tok struct {
				AccessToken string          `json:"access_token"`
				ExpiresIn   json.Number     `json:"expires_in"`
				ExpiresOn   json.Number     `json:"expires_on"`
				TokenType   string          `json:"token_type"`
				Scope       json.RawMessage `json:"scope"`
			}
			if err := json.Unmarshal(body, &tok); err != nil {
				return fmt.Errorf("parsing token response: %w", err)
			}
			if tok.AccessToken == "" {
				return authErr(fmt.Errorf("token endpoint returned no access_token: %s", strings.TrimSpace(string(body))))
			}

			expiry := tokenExpiry(tok.ExpiresOn, tok.ExpiresIn)

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			// Clear any legacy auth_header so AuthHeader() uses the new token.
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(id, secret, tok.AccessToken, "", expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			if flags.asJSON {
				out := map[string]any{
					"logged_in":   true,
					"datacenter":  dc,
					"config_path": cfg.Path,
				}
				if !expiry.IsZero() {
					out["expires_at"] = expiry.UTC().Format(time.RFC3339)
				}
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in. Token saved to %s\n", cfg.Path)
			if !expiry.IsZero() {
				fmt.Fprintf(cmd.OutOrStdout(), "  Expires: %s\n", expiry.UTC().Format(time.RFC3339))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth2 client id (or set ACRONIS_CLIENT_ID)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth2 client secret (or set ACRONIS_CLIENT_SECRET)")
	cmd.Flags().StringVar(&datacenter, "datacenter", "", "Acronis datacenter host prefix, e.g. us-cloud (or set ACRONIS_DATACENTER)")
	return cmd
}

// tokenExpiry derives an absolute expiry from the token response. Acronis
// returns expires_on as an absolute epoch (seconds) and/or expires_in as a
// relative lifetime (seconds). Prefer expires_on; fall back to now+expires_in.
func tokenExpiry(expiresOn, expiresIn json.Number) time.Time {
	if s := expiresOn.String(); s != "" {
		if epoch, err := strconv.ParseInt(s, 10, 64); err == nil && epoch > 0 {
			return time.Unix(epoch, 0)
		}
	}
	if s := expiresIn.String(); s != "" {
		if secs, err := strconv.ParseInt(s, 10, 64); err == nil && secs > 0 {
			return time.Now().Add(time.Duration(secs) * time.Second)
		}
	}
	return time.Time{}
}
