// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/client"
	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/config"
)

// intuitTokenURL is the QuickBooks OAuth 2.0 token endpoint. Override with
// QUICKBOOKS_TOKEN_URL for testing.
// #nosec G101 -- public Intuit OAuth endpoint URL, not a credential; the "bearer" path segment trips the heuristic. Real secrets come from env/config.
const intuitTokenURL = "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"

// The client refreshes on its own (before a request whose access token is
// about to expire, and once after a 401) with this same grant, so `auth
// refresh`, every command and the MCP server share one refresh path
// (client.QBORefreshTokens). Hand-fix (handfixes.json qbo-token-refresh).
func init() { client.QBOTokenGrant = qboTokenGrant }

func qboTokenGrant(ctx context.Context, clientID, clientSecret, refreshToken string) (string, string, int, error) {
	return refreshAccessToken(ctx, qboTokenURL(), clientID, clientSecret, refreshToken)
}

// qboTokenURL is the token endpoint: QUICKBOOKS_TOKEN_URL (a test hook), else
// Intuit's.
func qboTokenURL() string {
	return firstNonEmpty(cliutil.EnvOverride("QUICKBOOKS_TOKEN_URL"), intuitTokenURL)
}

func newAuthRefreshCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Mint a fresh access token from your OAuth2 refresh token",
		Long: "Exchange a QuickBooks OAuth 2.0 refresh token for a new access token and save\n" +
			"both to the credential store. Access tokens expire after about an hour; refresh\n" +
			"tokens last ~100 days and rotate on refresh. Every command already refreshes on\n" +
			"its own when the access token is about to expire or the API answers 401; run\n" +
			"this to refresh now. Reads QUICKBOOKS_CLIENT_ID and QUICKBOOKS_CLIENT_SECRET from\n" +
			"the environment or the stored config, and uses the stored refresh token (where\n" +
			"each refresh saves the rotated one) before QUICKBOOKS_REFRESH_TOKEN.",
		Example:     "  quickbooks-cli auth refresh\n  quickbooks-cli auth refresh --json",
		Annotations: map[string]string{"mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			clientID, clientSecret, refreshTokens := client.QBORefreshCredentials(cfg)
			if clientID == "" || clientSecret == "" || len(refreshTokens) == 0 {
				return authErr(fmt.Errorf("auth refresh needs QUICKBOOKS_CLIENT_ID, QUICKBOOKS_CLIENT_SECRET, and QUICKBOOKS_REFRESH_TOKEN (set in env or saved via `auth set-token`)"))
			}

			tokenURL := qboTokenURL()

			// Verify/dry-run short-circuit: never make the network call during a
			// printing-press verify pass or a --dry-run, so the credential
			// requirement and the token endpoint stay untouched.
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				if flags.asJSON {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
						"would_refresh": true,
						"token_url":     tokenURL,
						"config_path":   cfg.Path,
					}, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "would refresh access token via %s\n", tokenURL)
				return nil
			}

			// The same locked re-read, grant and save every command uses;
			// asked for by name, it always calls the token endpoint.
			res, err := client.QBORefreshTokens(cmd.Context(), cfg, "", true)
			if err != nil {
				var saveErr *client.QBOTokenSaveError
				if errors.As(err, &saveErr) {
					return configErr(err)
				}
				return authErr(err)
			}
			expiresIn, expiry := res.ExpiresIn, res.Expiry

			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"refreshed":   true,
					"expires_in":  expiresIn,
					"expires_at":  expiry.UTC().Format(time.RFC3339),
					"config_path": cfg.Path,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Access token refreshed (expires in %ds); saved to %s\n", expiresIn, cfg.Path)
			return nil
		},
	}
	return cmd
}

// refreshAccessToken performs the OAuth2 refresh_token grant against the Intuit
// token endpoint and returns the new access token, rotated refresh token, and
// access-token lifetime in seconds.
func refreshAccessToken(ctx context.Context, tokenURL, clientID, clientSecret, refreshToken string) (string, string, int, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	// #nosec G704 -- tokenURL defaults to the hardcoded Intuit endpoint; the only override (QUICKBOOKS_TOKEN_URL) is a documented operator-set test hook, not attacker-controlled request input.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, err
	}
	if err := refuseInsecureTokenURL(tokenURL); err != nil {
		return "", "", 0, err
	}
	basic := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Never follow a redirect: a 307/308 would replay the form body (the
	// refresh token) and Go would rebuild the request for the new host.
	hc := &http.Client{Timeout: 30 * time.Second, Transport: cliutil.OutboundGuard(client.QBOTokenTransport(ctx)), CheckRedirect: func(*http.Request, []*http.Request) error { // Hand-fix (handfixes.json credkit-wiring)
		return http.ErrUseLastResponse
	}}
	// #nosec G704 -- see tokenURL note above; the request target is the trusted Intuit endpoint by default and only operator-overridable for tests.
	resp, err := hc.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("calling token endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		// Hand-fix (handfixes.json qbo-token-refresh): a refused answer can
		// still carry a token; it is held before anything can print it.
		cliutil.QBOHoldTokenErrorAnswer(body, oauthTokenErrorCodes)
		return "", "", 0, tokenEndpointError(resp.StatusCode, body)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	// Hand-fix (handfixes.json qbo-token-refresh): the one
	// credential-answer decode holds every value as it is read (an overwritten
	// duplicate, a token before a syntax error) and its error quotes nothing.
	if err := cliutil.QBODecodeCredentialJSON(body, &out); err != nil {
		return "", "", 0, fmt.Errorf("parsing token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", "", 0, fmt.Errorf("token endpoint returned no access_token")
	}
	if out.ExpiresIn == 0 {
		out.ExpiresIn = 3600
	}
	return out.AccessToken, out.RefreshToken, out.ExpiresIn, nil
}

// oauthTokenErrorCodes are the error codes RFC 6749 section 5.2 lets a token
// endpoint return (Intuit's answers use them: invalid_grant for an expired or
// already-rotated refresh token, invalid_client for a wrong client secret).
var oauthTokenErrorCodes = map[string]bool{
	"invalid_request":        true,
	"invalid_client":         true,
	"invalid_grant":          true,
	"unauthorized_client":    true,
	"unsupported_grant_type": true,
	"invalid_scope":          true,
}

// tokenEndpointError is the error for a non-200 token answer. It shows the
// status and, when the answer is an OAuth error object, its RFC 6749 error
// code, and nothing else the endpoint wrote: the request carried the client
// secret and refresh token, and an echo of either can arrive in an encoding
// no detector decodes, so the answer is allowlisted, not scanned.
func tokenEndpointError(status int, body []byte) error {
	var answer struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(body, &answer) == nil {
		if code, ok := answer.Error.(string); ok && oauthTokenErrorCodes[code] {
			return &tokenEndpointRefusal{status: status, code: code}
		}
	}
	return &tokenEndpointRefusal{status: status}
}

// tokenEndpointRefusal is a non-200 token answer: the status and, at most,
// an allowlisted RFC 6749 error code. invalid_grant matches
// client.ErrRefreshTokenRejected, so the refresh can try the next refresh
// token it knows.
type tokenEndpointRefusal struct {
	status int
	code   string
}

func (e *tokenEndpointRefusal) Error() string {
	if e.code == "" {
		return fmt.Sprintf("token endpoint returned %d (response body withheld)", e.status)
	}
	return fmt.Sprintf("token endpoint returned %d: %s", e.status, e.code)
}

func (e *tokenEndpointRefusal) Is(target error) bool {
	return target == client.ErrRefreshTokenRejected && e.code == "invalid_grant"
}

// refuseInsecureTokenURL keeps the client secret and refresh token off
// plaintext HTTP: the token endpoint must be https, except a loopback host
// (a local test server).
func refuseInsecureTokenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("token URL %q is not an absolute URL", raw)
	}
	if strings.EqualFold(u.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(u.Scheme, "http") {
		host := u.Hostname()
		if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || strings.EqualFold(host, "localhost") {
			return nil
		}
	}
	return fmt.Errorf("refusing to send OAuth credentials to %s: the token URL must use https", u.Redacted())
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
