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
	"os"
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
scope api://{client-id}/.default and caches the access token to expiry; the
client re-mints it automatically when it nears expiry. Pass --client-secret -
to read the secret from stdin so it stays out of process arguments and shell
history. To use a static bearer token instead, set CIPP_API_KEY or run
'auth set-token'.`, "\n"),
		Example: strings.Trim(`
  cipp-cli auth login \
    --client-id 00000000-0000-0000-0000-000000000000 \
    --client-secret - \
    --tenant-id 11111111-1111-1111-1111-111111111111 \
    --base-url https://cipp.yourmsp.com/api < secret-file`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// issue #270: CIPP_NO_CONFIG_WRITE makes the automatic token
			// cache a no-op. A command whose whole purpose is to write a
			// credential to disk must not silently do nothing and report
			// success, so it refuses here and names the variable.
			if config.NoConfigWrite() {
				return configErr(fmt.Errorf("%s is set, so no credential may be written to disk; unset it and re-run to save one", config.NoConfigWriteEnv))
			}
			// Bare invocation prints help rather than a terse flag error.
			if clientID == "" && clientSecret == "" && tenantID == "" && baseURL == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "auth login")
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
				return usageErr(fmt.Errorf("missing required flag(s): %s", strings.Join(missing, ", ")))
			}
			if clientSecret == "-" {
				s, err := readSecretFromStdin(cmd.InOrStdin())
				if err != nil {
					return authErr(err)
				}
				clientSecret = s
			}

			if scope == "" {
				scope = "api://" + clientID + "/.default"
			}
			if authority == "" {
				authority = config.DefaultAuthority
			}
			// The token POST carries client_secret, so the authority must be
			// HTTPS (loopback http only, for a local mock).
			if !config.AuthorityIsSecure(authority) {
				return usageErr(fmt.Errorf("--authority %q must be an https URL", authority))
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

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			token, expiry, lifetime, err := cippClientCredentialsToken(cmd.Context(), cfg, authority, tenantID, clientID, clientSecret, scope)
			if err != nil {
				return authErr(err)
			}
			cfg.BaseURL = baseURL
			// The minted token must be what gets sent. A stored auth_header or
			// static api_key outranks access_token in AuthHeader(), so a stale
			// one would keep being sent, to the new base URL too (Codex r1).
			cfg.AuthHeaderVal = ""
			cfg.CippApiKey = ""
			if strings.TrimSpace(os.Getenv("CIPP_API_KEY")) != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: CIPP_API_KEY is set in the environment and outranks the token this login caches; unset it to use the auto-refreshing token")
			}
			// Persist the non-secret client-credentials context alongside the
			// token. Without tenant_id the client has no Azure AD token
			// endpoint to POST to, so an expired token could only be replaced
			// by a human re-running this command; authority and scope are
			// persisted too so a sovereign-cloud or custom-scope login is not
			// silently downgraded to the defaults on the first re-mint.
			cfg.SetOAuthContext(tenantID, authority, scope)
			// Record the granted expires_in so the client can clamp its refresh
			// safety skew to half the lifetime.
			cfg.SetTokenLifetime(lifetime)
			cfg.MarkCredentialsExplicit(true, true)
			if err := cfg.SaveTokens(clientID, clientSecret, token, "", expiry); err != nil {
				return configErr(fmt.Errorf("caching token: %w", err))
			}

			fmt.Fprintf(w, "Authenticated. Token cached to %s, expires %s.\n", credentialSavePath(cfg), expiry.Format(time.RFC3339))
			fmt.Fprintln(w, "The token refreshes automatically when it nears expiry; no need to re-run this command until the client secret rotates.")
			fmt.Fprintf(w, "Base URL set to %s. Run 'cipp-cli doctor' to verify connectivity.\n", baseURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&clientID, "client-id", "", "Azure AD application (client) ID of the CIPP API client")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "Client secret for the CIPP API client; '-' reads it from stdin")
	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "Azure AD tenant (directory) ID that hosts the CIPP API app registration")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "CIPP API base URL, must include /api (e.g. https://cipp.yourmsp.com/api)")
	cmd.Flags().StringVar(&scope, "scope", "", "OAuth2 scope (default: api://<client-id>/.default)")
	cmd.Flags().StringVar(&authority, "authority", "", "Token authority host (default: https://login.microsoftonline.com)")
	return cmd
}

// cippClientCredentialsToken performs the Azure AD client-credentials grant and
// returns the access token, its computed expiry time, and the expires_in the
// endpoint granted (0 when it did not say).
func cippClientCredentialsToken(ctx context.Context, cfg *config.Config, authority, tenantID, clientID, clientSecret, scope string) (string, time.Time, int, error) {
	tokenURL := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimRight(authority, "/"), url.PathEscape(tenantID))
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("grant_type", "client_credentials")
	form.Set("scope", scope)
	encoded := form.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, tokenURL, strings.NewReader(encoded))
	if err != nil {
		return "", time.Time{}, 0, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// The guarded probe client (dial guard, TLS settings) with the token
	// redirect policy. NOT http.DefaultClient: its zero CheckRedirect follows
	// up to 10 hops and REPLAYS THE REQUEST BODY on 307/308, and this body is
	// the client-credentials form carrying client_secret.
	plain, err := client.NewPlainHTTPClient(30*time.Second, cfg)
	if err != nil {
		return "", time.Time{}, 0, fmt.Errorf("building token client: %w", err)
	}
	httpClient := &http.Client{
		Transport:     plain.Transport,
		Jar:           plain.Jar,
		Timeout:       30 * time.Second,
		CheckRedirect: client.TokenExchangeRedirectPolicy,
	}
	if req, err = client.GuardTokenRequest(httpClient, req, []byte(encoded)); err != nil {
		return "", time.Time{}, 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Hand-fix (handfixes.json secret-echo-decoded-funnel): a refused redirect's error carries the Location URL, which a token endpoint controls, so the transport error passes the funnel too.
		return "", time.Time{}, 0, fmt.Errorf("requesting token from %s: %s", tokenURL, cliutil.FunnelWithhold(maskSecret(err.Error(), clientSecret), []string{clientSecret}))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// A non-success body may still carry a token (an authority answering
		// 201 or 4xx with one): redact token members before anything prints.
		body = []byte(client.RedactTokenResponse(string(body)))
		// Hand-fix (handfixes.json secret-echo-decoded-funnel): the complete raw body passes the funnel once, before any branch formats a field out of it.
		if w := cliutil.FunnelWithhold(maskSecret(string(body), clientSecret), []string{clientSecret}); cliutil.FunnelWithheld(w) {
			return "", time.Time{}, 0, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, w)
		}
		// Hand-fix (handfixes.json token-error-text-allowlist): only the Azure
		// AD error code and a plain-prose description are shown; anything else
		// the authority sent is withheld, whatever its encoding.
		return "", time.Time{}, 0, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode,
			cliutil.FunnelWithhold(maskSecret(client.TokenErrorText(body), clientSecret), []string{clientSecret}))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	// Hand-fix (handfixes.json token-error-text-allowlist): a 2xx answer that
	// cannot be used is described by kind only, never by the decoder's text.
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", time.Time{}, 0, client.TokenResponseError("token response could not be decoded", body)
	}
	if tok.AccessToken == "" {
		return "", time.Time{}, 0, client.TokenResponseError("token response contained no access_token", body)
	}
	if tok.ExpiresIn < 0 {
		return "", time.Time{}, 0, client.ErrExpiredOnArrival
	}
	// A huge expires_in overflows time.Duration and wraps NEGATIVE, which
	// would stamp an expiry in the past. Clamp instead of trusting it.
	expiry := time.Now().Add(client.TokenLifetimeToDuration(tok.ExpiresIn))
	lifetime := tok.ExpiresIn
	if lifetime > client.MaxTokenLifetimeSeconds {
		lifetime = client.MaxTokenLifetimeSeconds
	}
	if tok.ExpiresIn == 0 {
		// Unknown lifetime: trust it briefly, then let the client re-mint.
		expiry = client.UnknownLifetimeExpiry(time.Now())
		lifetime = 0
	}
	return tok.AccessToken, expiry, lifetime, nil
}

// maskSecret redacts an exact credential value (as written and URL-encoded)
// inside a server-supplied string. A misconfigured or hostile token endpoint
// can echo the submitted form back, and cliutil.SanitizeErrorBody only knows
// generic token shapes. The replacement reveals nothing of the secret.
func maskSecret(text, secret string) string {
	secret = strings.TrimSpace(secret)
	if text == "" || secret == "" {
		return text
	}
	text = strings.ReplaceAll(text, secret, "****")
	if escaped := url.QueryEscape(secret); escaped != secret {
		text = strings.ReplaceAll(text, escaped, "****")
	}
	if escaped := url.PathEscape(secret); escaped != secret {
		text = strings.ReplaceAll(text, escaped, "****")
	}
	return text
}
