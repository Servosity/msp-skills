// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature (printing-press scaffold filled in). Implements the
// pp:data-source live
// OAuth2 client_credentials handshake against Blumira's auth service.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"blumira-pp-cli/internal/cliutil"
	"blumira-pp-cli/internal/config"

	"github.com/spf13/cobra"
)

const defaultBlumiraAuthURL = "https://auth.blumira.com/oauth/token"

// tokenResponse is the Auth0-style client_credentials grant response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

// newNovelAuthLoginCmd mints a Blumira JWT from a Client ID + Client Secret via
// the OAuth2 client_credentials grant, then caches it (with expiry) in the
// CLI config so every other command authenticates without a manual exchange.
// The incumbent MCP makes the operator bring and refresh their own JWT; this
// mints and rotates it.
func newNovelAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var flagClientID string
	var flagClientSecret string
	var flagAudience string
	var flagAuthURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Mint and cache a Blumira JWT from a Client ID + Client Secret (OAuth2 client_credentials)",
		Long: strings.Trim(`
Exchange a Blumira API Client ID + Client Secret for a JWT bearer token and
cache it (with its ~30-day expiry) in the CLI config, so every other command
authenticates automatically. Generate credentials in the Blumira UI under
Settings > Organization > Generate API Credentials.

Credentials are read from --client-id/--client-secret or, if those are unset,
from the BLUMIRA_CLIENT_ID / BLUMIRA_CLIENT_SECRET environment variables.
`, "\n"),
		Example: strings.Trim(`
  blumira-cli auth login --client-id "$BLUMIRA_CLIENT_ID" --client-secret "$BLUMIRA_CLIENT_SECRET"
  BLUMIRA_CLIENT_ID=... BLUMIRA_CLIENT_SECRET=... blumira-cli auth login
  blumira-cli auth login --dry-run   # show the request without sending
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			clientID := flagClientID
			if clientID == "" {
				clientID = os.Getenv("BLUMIRA_CLIENT_ID")
			}
			clientSecret := flagClientSecret
			if clientSecret == "" {
				clientSecret = os.Getenv("BLUMIRA_CLIENT_SECRET")
			}
			authURL := flagAuthURL
			if authURL == "" {
				authURL = defaultBlumiraAuthURL
			}
			audience := flagAudience
			if audience == "" {
				audience = "public-api"
			}

			// Verify/dry-run probes must never hit the network or write real
			// credentials. Short-circuit before any IO and report what would
			// happen (secret redacted). Covers both `--dry-run` and the
			// printing-press verifier's PRINTING_PRESS_VERIFY=1 subprocess.
			if cliutil.IsVerifyEnv() || dryRunOK(flags) {
				out := map[string]any{
					"would_authenticate": true,
					"auth_url":           authURL,
					"audience":           audience,
					"grant_type":         "client_credentials",
					"client_id":          maskCredential(clientID),
					"client_secret":      maskCredential(clientSecret),
				}
				if flags.asJSON {
					return printJSONFiltered(cmd.OutOrStdout(), out, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Would POST %s (grant_type=client_credentials, audience=%s)\n", authURL, audience)
				return nil
			}

			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if clientID == "" || clientSecret == "" {
				return usageErr(fmt.Errorf("missing credentials: pass --client-id and --client-secret, or set BLUMIRA_CLIENT_ID and BLUMIRA_CLIENT_SECRET (generate them in Settings > Organization > Generate API Credentials)"))
			}

			tok, err := mintBlumiraToken(cmd.Context(), authURL, clientID, clientSecret, audience, flags.timeout)
			if err != nil {
				return authErr(err)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			expiry := time.Time{}
			if tok.ExpiresIn > 0 {
				expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
			}
			if err := cfg.SaveTokens(clientID, clientSecret, tok.AccessToken, "", expiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			if flags.asJSON {
				out := map[string]any{
					"authenticated": true,
					"config":        cfg.Path,
					"token_type":    tok.TokenType,
				}
				if !expiry.IsZero() {
					out["expires_at"] = expiry.UTC().Format(time.RFC3339)
					out["expires_in_seconds"] = tok.ExpiresIn
				}
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, green("Authenticated. JWT minted and cached."))
			fmt.Fprintf(w, "  Config:  %s\n", cfg.Path)
			if !expiry.IsZero() {
				fmt.Fprintf(w, "  Expires: %s (re-run 'auth login' to refresh)\n", expiry.UTC().Format(time.RFC3339))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagClientID, "client-id", "", "Blumira API Client ID (or set BLUMIRA_CLIENT_ID)")
	cmd.Flags().StringVar(&flagClientSecret, "client-secret", "", "Blumira API Client Secret (or set BLUMIRA_CLIENT_SECRET)")
	cmd.Flags().StringVar(&flagAudience, "audience", "public-api", "OAuth2 audience for the token request")
	cmd.Flags().StringVar(&flagAuthURL, "auth-url", defaultBlumiraAuthURL, "OAuth2 token endpoint")
	return cmd
}

// mintBlumiraToken performs the client_credentials grant and returns the parsed
// token response. It posts form-encoded values, which Blumira's auth endpoint
// accepts alongside JSON.
func mintBlumiraToken(ctx context.Context, authURL, clientID, clientSecret, audience string, timeout time.Duration) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("audience", audience)

	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request to %s failed: %w", authURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, summarizeAuthError(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token")
	}
	return &tok, nil
}

// maskCredential keeps only a short prefix so dry-run/verbose output can
// confirm which credential was used without leaking it.
func maskCredential(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "…(redacted)"
}

// summarizeAuthError extracts a human message from an Auth0-style error body
// ({"error":"...","error_description":"..."}) without echoing the whole blob.
func summarizeAuthError(body []byte) string {
	var e struct {
		Error   string `json:"error"`
		ErrDesc string `json:"error_description"`
	}
	if json.Unmarshal(body, &e) == nil {
		switch {
		case e.ErrDesc != "":
			return e.ErrDesc
		case e.Error != "":
			return e.Error
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "no response body"
	}
	return s
}
