// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (preserved across regen). CIPP-specific OAuth2
// client-credentials login: CIPP is self-hosted and authenticates via an Azure
// AD app registration (the "API client" created in CIPP > Integrations >
// CIPP-API). This command performs the client-credentials token exchange
// against login.microsoftonline.com and caches the resulting bearer token.

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

	"cipp-pp-cli/internal/client"
	"cipp-pp-cli/internal/cliutil"
	"cipp-pp-cli/internal/config"

	"github.com/spf13/cobra"
)

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var clientID, clientSecret, tenantID, baseURL, scope, authority string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a self-hosted CIPP instance via OAuth2 client credentials",
		Long: strings.Trim(`
Exchange a CIPP API client's credentials for a bearer token and cache it.

CIPP is self-hosted, so each MSP runs its own instance. Create an API client in
CIPP > Integrations > CIPP-API (use a read-only Custom Role for safe testing),
then run this command with the Client ID, Client Secret, Tenant ID, and the API
base URL it shows you. The base URL must include /api
(e.g. https://cipp.yourmsp.com/api).

This performs the Azure AD client-credentials flow against
https://login.microsoftonline.com/{tenant-id}/oauth2/v2.0/token with
scope api://{client-id}/.default and caches the access token to expiry. To use a
static bearer token instead, set CIPP_API_KEY or run 'auth set-token'.`, "\n"),
		Example: strings.Trim(`
  cipp-cli auth login \
    --client-id 00000000-0000-0000-0000-000000000000 \
    --client-secret 'your-secret' \
    --tenant-id 11111111-1111-1111-1111-111111111111 \
    --base-url https://cipp.yourmsp.com/api`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bare invocation prints help rather than a terse flag error.
			if clientID == "" && clientSecret == "" && tenantID == "" && baseURL == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}

			missing := []string{}
			if clientID == "" {
				missing = append(missing, "--client-id")
			}
			if clientSecret == "" {
				missing = append(missing, "--client-secret")
			}
			if tenantID == "" {
				missing = append(missing, "--tenant-id")
			}
			if baseURL == "" {
				missing = append(missing, "--base-url")
			}
			if len(missing) > 0 {
				return fmt.Errorf("missing required flag(s): %s", strings.Join(missing, ", "))
			}

			if scope == "" {
				scope = "api://" + clientID + "/.default"
			}
			if authority == "" {
				authority = "https://login.microsoftonline.com"
			}
			baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
			if !strings.HasSuffix(strings.ToLower(baseURL), "/api") {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: --base-url does not end in /api; CIPP requests will hit the static web UI, not the Functions backend. Got: %s\n", baseURL)
			}

			w := cmd.OutOrStdout()

			// Verify-mode: never dial out. Report what would happen and succeed.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(w, "would authenticate client %s against %s/%s/oauth2/v2.0/token (verify mode)\n", clientID, authority, tenantID)
				return nil
			}

			token, expiry, lifetime, err := cippClientCredentialsToken(cmd.Context(), authority, tenantID, clientID, clientSecret, scope)
			if err != nil {
				return err
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			cfg.BaseURL = baseURL
			// Persist the non-secret client-credentials context alongside the
			// token. Without tenant_id the client has no Azure AD token
			// endpoint to POST to, so an expired token could only be replaced
			// by a human re-running this command; authority and scope are
			// persisted too so a sovereign-cloud or custom-scope login is not
			// silently downgraded to the defaults on the first re-mint.
			cfg.SetOAuthContext(tenantID, authority, scope)
			// Record the granted expires_in so the client can clamp its refresh
			// safety skew to half the lifetime. Without it a token endpoint
			// that issues short-lived tokens puts every fresh token inside the
			// 5-minute skew immediately, and the client re-mints on every
			// request behind a package-level mutex.
			cfg.SetTokenLifetime(lifetime)
			if err := cfg.SaveTokens(clientID, clientSecret, token, "", expiry); err != nil {
				return fmt.Errorf("caching token: %w", err)
			}

			fmt.Fprintf(w, "Authenticated. Token cached to %s, expires %s.\n", cfg.Path, expiry.Format(time.RFC3339))
			fmt.Fprintln(w, "The token refreshes automatically when it nears expiry; no need to re-run this command until the client secret rotates.")
			fmt.Fprintf(w, "Base URL set to %s. Run 'cipp-cli doctor' to verify connectivity.\n", baseURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&clientID, "client-id", "", "Azure AD application (client) ID of the CIPP API client")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "Client secret for the CIPP API client")
	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "Azure AD tenant (directory) ID that hosts the CIPP API app registration")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "CIPP API base URL, must include /api (e.g. https://cipp.yourmsp.com/api)")
	cmd.Flags().StringVar(&scope, "scope", "", "OAuth2 scope (default: api://<client-id>/.default)")
	cmd.Flags().StringVar(&authority, "authority", "", "Token authority host (default: https://login.microsoftonline.com)")
	return cmd
}

// cippClientCredentialsToken performs the Azure AD client-credentials grant and
// returns the access token, its computed expiry time, and the expires_in the
// endpoint granted (0 when it did not say).
func cippClientCredentialsToken(ctx context.Context, authority, tenantID, clientID, clientSecret, scope string) (string, time.Time, int, error) {
	tokenURL := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimRight(authority, "/"), tenantID)
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("grant_type", "client_credentials")
	form.Set("scope", scope)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, 0, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// NOT http.DefaultClient. Its zero CheckRedirect is Go's default policy,
	// which follows up to 10 hops and REPLAYS THE REQUEST BODY on 307/308 -
	// and this body is the client-credentials form carrying client_secret. A
	// token endpoint answering 307 to another host would be handed the secret.
	// client.TokenExchangeRedirectPolicy refuses any cross-origin hop; the
	// re-mint path in internal/client installs the identical policy.
	httpClient := &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: client.TokenExchangeRedirectPolicy,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, 0, fmt.Errorf("requesting token from %s: %w", tokenURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		// Azure AD returns a JSON error body with error_description; surface it.
		var aadErr struct {
			Error     string `json:"error"`
			ErrorDesc string `json:"error_description"`
		}
		if json.Unmarshal(body, &aadErr) == nil && aadErr.Error != "" {
			desc := aadErr.ErrorDesc
			if i := strings.IndexByte(desc, '\n'); i > 0 {
				desc = desc[:i]
			}
			// BOTH fields are vendor-controlled free text. A custom or
			// compromised authority can echo the submitted form into either,
			// so mask the secret out of the code as well as the description -
			// masking only one of two attacker-controlled strings is not a
			// mask at all.
			return "", time.Time{}, 0, fmt.Errorf("token request failed (HTTP %d): %s: %s", resp.StatusCode, maskSecret(aadErr.Error, clientSecret), maskSecret(desc, clientSecret))
		}
		return "", time.Time{}, 0, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, cliutil.SanitizeErrorBody(maskSecret(strings.TrimSpace(string(body)), clientSecret)))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", time.Time{}, 0, fmt.Errorf("parsing token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", time.Time{}, 0, fmt.Errorf("token response contained no access_token")
	}
	// A huge expires_in overflows time.Duration (int64 nanoseconds, ~292 years)
	// and wraps NEGATIVE, which would stamp an expiry in the past and re-mint on
	// every request forever. Clamp instead of trusting the endpoint.
	expiry := time.Now().Add(client.TokenLifetimeToDuration(tok.ExpiresIn))
	lifetime := tok.ExpiresIn
	if lifetime > client.MaxTokenLifetimeSeconds {
		lifetime = client.MaxTokenLifetimeSeconds
	}
	if tok.ExpiresIn <= 0 {
		expiry = time.Now().Add(time.Hour) // conservative default
		lifetime = 0                       // unknown: the client keeps its default skew
	}
	return tok.AccessToken, expiry, lifetime, nil
}

// maskSecret redacts an exact credential value inside a server-supplied string.
// The Azure AD error path echoes an arbitrary body, and a misconfigured or
// hostile token endpoint can echo the submitted form - client_secret included -
// straight back. cliutil.SanitizeErrorBody only knows generic patterns, so it
// would let the secret through.
func maskSecret(text, secret string) string {
	secret = strings.TrimSpace(secret)
	if text == "" || secret == "" {
		return text
	}
	replacement := "****"
	if len(secret) > 4 {
		replacement = "****" + secret[len(secret)-4:]
	}
	text = strings.ReplaceAll(text, secret, replacement)
	if escaped := url.QueryEscape(secret); escaped != secret {
		text = strings.ReplaceAll(text, escaped, replacement)
	}
	return text
}
