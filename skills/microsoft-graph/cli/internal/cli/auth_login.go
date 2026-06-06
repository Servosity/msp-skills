// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/cliutil"
	"microsoft-graph-pp-cli/internal/config"
)

// newAuthLoginCmd implements app-only authentication via the OAuth2
// client-credentials flow. It mints a token from the Entra token endpoint and
// caches it (plus the client id/secret) in the config file, so subsequent
// commands authenticate as the registered application without an interactive
// sign-in or an external token-minting tool.
func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var tenant, clientID, clientSecret, scope string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate app-only (client credentials) and cache the token",
		Long: strings.Trim(`
Mint an app-only access token using the OAuth2 client-credentials flow and
cache it for subsequent commands. Use this when running unattended as an Entra
app registration (the typical MSP pattern); the registration must have the
required Graph application permissions granted with admin consent.

Alternatively, skip this and export a pre-minted token as MICROSOFT_GRAPH_TOKEN
(for example from 'az account get-access-token --scope https://graph.microsoft.com/.default').`, "\n"),
		Example: "  microsoft-graph-cli auth login --tenant <tenant-id> --client-id <app-id> --client-secret <secret>",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Side-effect command: never hit the network under the verifier or
			// a dry-run probe.
			if cliutil.IsVerifyEnv() || dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "auth login: verify/dry-run mode — skipping token mint")
				return nil
			}
			if tenant == "" || clientID == "" || clientSecret == "" {
				return usageErr(fmt.Errorf("--tenant, --client-id, and --client-secret are all required"))
			}
			if scope == "" {
				scope = "https://graph.microsoft.com/.default"
			}

			token, expiry, err := mintClientCredentialsToken(cmd.Context(), flags.timeout, tenant, clientID, clientSecret, scope)
			if err != nil {
				return authErr(err)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			// Clear any legacy auth_header so AuthHeader() uses the new token.
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(clientID, clientSecret, token, "", expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"authenticated": true,
					"auth_mode":     "app-only",
					"expires_at":    expiry.UTC().Format(time.RFC3339),
					"config_path":   cfg.Path,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Authenticated (app-only). Token cached to %s (expires %s).\n",
				cfg.Path, expiry.UTC().Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Entra tenant id or domain (required)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "App registration (client) id (required)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "App registration client secret (required)")
	cmd.Flags().StringVar(&scope, "scope", "https://graph.microsoft.com/.default", "OAuth2 scope")
	return cmd
}

// mintClientCredentialsToken exchanges client credentials for an access token
// at the Entra v2.0 token endpoint. It applies a 60-second safety margin to the
// returned expiry so callers re-auth before the token actually lapses.
func mintClientCredentialsToken(ctx context.Context, timeout time.Duration, tenant, clientID, clientSecret, scope string) (string, time.Time, error) {
	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenant))
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("requesting token from Entra: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed struct {
		AccessToken      string `json:"access_token"`
		ExpiresIn        int    `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode != http.StatusOK || parsed.AccessToken == "" {
		msg := parsed.ErrorDescription
		if msg == "" {
			msg = parsed.Error
		}
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		// The error body from Entra may echo the client_id but never the
		// secret; still, keep the surfaced message bounded.
		if len(msg) > 400 {
			msg = msg[:400] + "..."
		}
		return "", time.Time{}, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	margin := parsed.ExpiresIn - 60
	if margin < 0 {
		margin = parsed.ExpiresIn
	}
	expiry := time.Now().Add(time.Duration(margin) * time.Second)
	return parsed.AccessToken, expiry, nil
}
