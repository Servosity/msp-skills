// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output). ESET Connect authentication.
//
// ESET Connect issues no API keys. A dedicated API user (created in ESET
// PROTECT Hub, ESET Business Account or ESET MSP Administrator 2) exchanges its
// email + password for a one-hour bearer token with an OAuth2 password grant:
//
//	POST https://{region}.business-account.iam.eset.systems/oauth/token
//	Content-Type: application/x-www-form-urlencoded
//	grant_type=password&username=<email>&password=<password>
//	-> {"access_token","refresh_token","token_type":"Bearer","expires_in":3600}
//
// and refreshes with grant_type=refresh_token. The press has no password-grant
// flow, so this file mints the token itself from ESET_PROTECT_USERNAME /
// ESET_PROTECT_PASSWORD, caches it bound to (region, user) so a token can never
// be reused against a different tenant, and registers a client hook that also
// installs the 202/401 transport (internal/client/eset_transport.go).
package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/config"
	"eset-protect-pp-cli/internal/secrets"

	"github.com/spf13/cobra"
)

// EsetAPIUserURL is ESET's guide to creating the dedicated ESET Connect API
// user. ESET Connect has no API keys, so every credential hint points here.
const EsetAPIUserURL = "https://help.eset.com/eset_connect/en-US/create_api_user_account.html"

// EsetAPIUserInstructions is the one-line credential setup shared by doctor,
// error hints and the MCP context tool (handfixes.json: auth-setup-describes-eset).
const EsetAPIUserInstructions = "In ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2, create a dedicated API user with the Integrations toggle on, then set ESET_PROTECT_USERNAME, ESET_PROTECT_PASSWORD and ESET_PROTECT_REGION and run '" + esetCLIName + " auth login'."

// esetWriteAuthSetup prints `auth setup`: how to create the API user and
// hand its credentials to the CLI, in both POSIX shell and PowerShell form.
func esetWriteAuthSetup(w io.Writer) {
	lines := []string{
		"ESET Connect has no API keys. The CLI signs in as a dedicated API user and",
		"exchanges its email and password for a one-hour access token.",
		"",
		"1. Create the API user in ESET PROTECT Hub, ESET Business Account or",
		"   ESET MSP Administrator 2, with the Integrations toggle turned on:",
		"   " + EsetAPIUserURL,
		"   Every new API user (ESET PROTECT Hub, ESET Business Account or ESET MSP",
		"   Administrator 2) receives an invitation email and must finish creating the",
		"   account before the API accepts it. A new ESET PROTECT Hub user must also",
		"   sign in to ESET PROTECT Hub once.",
		"",
		"2. Set the API user's email and password, and your tenant's region",
		"   (eu, de, us, ca or jpn):",
		"",
		"   macOS / Linux (bash, zsh):",
		"     export ESET_PROTECT_USERNAME=\"<api-user-email>\"",
		"     export ESET_PROTECT_PASSWORD=\"<api-user-password>\"",
		"     export ESET_PROTECT_REGION=\"eu\"",
		"",
		"   Windows (PowerShell):",
		"     $env:ESET_PROTECT_USERNAME = \"<api-user-email>\"",
		"     $env:ESET_PROTECT_PASSWORD = \"<api-user-password>\"",
		"     $env:ESET_PROTECT_REGION = \"eu\"",
		"",
		"3. Sign in and verify the credentials:",
		"     " + esetCLIName + " auth login",
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
}

// esetWriteAuthStatusHint is `auth status`'s guidance when no token is cached.
func esetWriteAuthStatusHint(w io.Writer, cfg *config.Config) {
	if esetHasCredentials(cfg) {
		fmt.Fprintln(w, "No access token is cached yet. ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD are set:")
		fmt.Fprintf(w, "run '%s auth login' to mint and verify one (other commands mint it on demand).\n", esetCLIName)
		return
	}
	fmt.Fprintln(w, "Set ESET_PROTECT_USERNAME, ESET_PROTECT_PASSWORD and ESET_PROTECT_REGION to the ESET Connect")
	fmt.Fprintf(w, "API user's email, password and region, then run '%s auth login'.\n", esetCLIName)
	fmt.Fprintf(w, "Run '%s auth setup' for the full steps.\n", esetCLIName)
}

// esetTokenRefreshMargin renews a cached token this long before it expires.
const esetTokenRefreshMargin = 5 * time.Minute

// esetTokenURLOverride lets tests point the mint at a local server.
var esetTokenURLOverride string

var esetMintMu sync.Mutex

func init() {
	// The generated store resolver scopes the local mirror by credential. For
	// ESET that would be the hourly bearer token, so every refresh would
	// orphan the synced mirror; scope by region + API user instead.
	config.StoreScopeIdentity = esetStoreIdentity
	registerClientHook(esetClientHook)
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "auth" {
				addNovelCommandIfAbsent(c, newEsetAuthLoginCmd(flags))
			}
		}
	})
}

func esetRegion(cfg *config.Config) (string, error) {
	region := "eu"
	if cfg != nil && cfg.TemplateVars != nil {
		if v := config.NormalizeRegion(cfg.TemplateVars["region"]); v != "" {
			region = v
		}
	}
	// Only an ESET Connect region (handfixes.json: region-normalized-and-validated);
	// the allowlist is stricter than the old [a-z]{2,4} shape check.
	source := "ESET_PROTECT_REGION"
	if cfg != nil && cfg.RegionSource == config.RegionSourceFile {
		source = "region in " + config.RegionSourceName(cfg)
	}
	if err := config.ValidateRegion(region, source); err != nil {
		return "", err
	}
	return region, nil
}

// esetServiceHostRE matches an ESET Connect service host: <region>.<service>.eset.systems.
var esetServiceHostRE = regexp.MustCompile(`^[a-z]{2,4}\.[a-z0-9-]+\.eset\.systems$`)

// esetHostAllower pins credentialed requests to ESET Connect service hosts,
// plus the host of an explicitly configured base URL (ESET_PROTECT_BASE_URL or
// config base_url: operator configuration, never a per-command flag).
func esetHostAllower(cfg *config.Config) func(host string) bool {
	configured := ""
	if cfg != nil && cfg.BaseURL != "" && !strings.Contains(cfg.BaseURL, "{region}") {
		if u, err := url.Parse(cfg.BaseURL); err == nil {
			configured = u.Host
		}
	}
	return func(host string) bool {
		return esetServiceHostRE.MatchString(host) || (configured != "" && host == configured)
	}
}

func esetTokenURL(region string) string {
	if esetTokenURLOverride != "" {
		return esetTokenURLOverride
	}
	return "https://" + region + ".business-account.iam.eset.systems/oauth/token"
}

// esetIdentity binds a cached token to one region + API user. Stored in the
// generated config's client_id slot (unused by bearer auth) so `auth logout`
// still clears it. The username is hashed so the email is not written there.
func esetIdentity(region, username string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	return "eset:" + region + ":" + hex.EncodeToString(sum[:8])
}

func esetHasCredentials(cfg *config.Config) bool {
	return cfg != nil && strings.TrimSpace(cfg.EsetProtectUsername) != "" && cfg.EsetProtectPassword != ""
}

type esetTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// esetTokenErrorCodes are the OAuth2 error codes a token endpoint may answer
// with: RFC 6749 sections 5.2 and 4.1.2.1, RFC 6750 section 3.1, RFC 8628
// section 3.5, and ESET's own invalid_username_or_password. An error code is
// shown verbatim only when it is one of these; any other value of "error" is
// server text and is never shown (handfixes.json:
// secret-registry-masks-every-boundary).
var esetTokenErrorCodes = map[string]bool{
	"invalid_request":              true,
	"invalid_client":               true,
	"invalid_grant":                true,
	"unauthorized_client":          true,
	"unsupported_grant_type":       true,
	"invalid_scope":                true,
	"access_denied":                true,
	"unsupported_response_type":    true,
	"server_error":                 true,
	"temporarily_unavailable":      true,
	"invalid_token":                true,
	"insufficient_scope":           true,
	"authorization_pending":        true,
	"slow_down":                    true,
	"expired_token":                true,
	"invalid_username_or_password": true,
	"unauthorized":                 true,
	"too_many_requests":            true,
}

// esetTokenDescriptionMax caps how much of error_description an error shows.
const esetTokenDescriptionMax = 300

// esetTokenErrorReason is what a failed token answer contributes to the
// error: the error code when it is a known OAuth2 code, then
// error_description run through the secret masker, so a token service that
// echoes the submitted password or refresh token cannot put it in an error,
// doctor row or MCP result. The description is masked before it is cut, so
// a credential is never shown in part.
func esetTokenErrorReason(tr esetTokenResponse) string {
	code := strings.ToLower(strings.TrimSpace(tr.Error))
	reason := ""
	switch {
	case esetTokenErrorCodes[code]:
		reason = code
	case code != "":
		reason = "unrecognized error code"
	}
	if desc := strings.TrimSpace(tr.ErrorDescription); desc != "" && !strings.EqualFold(desc, code) {
		desc = secrets.Mask(desc)
		if utf8.RuneCountInString(desc) > esetTokenDescriptionMax {
			desc = string([]rune(desc)[:esetTokenDescriptionMax]) + "..."
		}
		if reason != "" {
			reason += ": "
		}
		reason += desc
	}
	if reason == "" {
		reason = "no access_token in response"
	}
	return reason
}

func esetRequestToken(ctx context.Context, region string, form url.Values) (*esetTokenResponse, error) {
	// The submitted credentials are what a token service would echo.
	secrets.Register(form.Get("password"), form.Get("refresh_token"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, esetTokenURL(region), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// The token endpoint can answer 202 + response-id like any ESET Connect
	// call; the pending transport (without 401 replay) polls it through.
	// Never follow a redirect: a 307/308 would replay the password or refresh
	// token in the form body to whatever host it names.
	hc := &http.Client{
		Timeout:       2 * time.Minute,
		Transport:     &client.EsetTransport{},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Do(req)
	if err != nil {
		// No answer (connection refused, DNS, timeout, ESET kept it pending)
		// is an outage; an answer the transport could not use is not.
		var noID *client.PendingWithoutIDError
		var refused *client.CredentialHostRefusedError
		outage := !errors.As(err, &noID) && !errors.As(err, &refused)
		return nil, &esetTokenError{outage: outage, msg: secrets.Mask(fmt.Sprintf("ESET Connect token request (%s region) failed: %v (no answer from the token service, so this is not a credential rejection; check the network or proxy and retry)", region, err)), err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr esetTokenResponse
	_ = json.Unmarshal(raw, &tr)
	// Whatever the status, a token in the answer is a credential of this
	// process from now on: a later error body that echoes it is masked.
	secrets.Register(tr.AccessToken, tr.RefreshToken)
	if resp.StatusCode != http.StatusOK || tr.AccessToken == "" {
		reason := esetTokenErrorReason(tr)
		if esetTokenRejected(resp.StatusCode, tr) {
			return nil, &esetTokenError{rejected: true, msg: fmt.Sprintf("ESET Connect token request (%s region) rejected: HTTP %d %s. Check ESET_PROTECT_USERNAME / ESET_PROTECT_PASSWORD belong to an API user with ESET Connect access, and that ESET_PROTECT_REGION matches the tenant's data center", region, resp.StatusCode, reason)}
		}
		return nil, &esetTokenError{outage: esetTokenOutageStatus(resp.StatusCode), msg: fmt.Sprintf("ESET Connect token request (%s region) failed: HTTP %d %s. This is not a credential rejection (the token service is unavailable, rate limiting, or answered something other than a token), so retry later", region, resp.StatusCode, reason)}
	}
	return &tr, nil
}

// esetTokenError is a token request that returned no token. rejected is true
// only for an explicit ESET credential rejection (esetTokenRejected); a
// transport failure, a 5xx, a 429 or any other answer is not one. outage is
// true only on positive evidence that the token service is down: no answer
// at all (connection refused, DNS, timeout, a request kept pending), HTTP
// 5xx or HTTP 429 (handfixes.json: auto-falls-back-when-token-service-is-down).
// Any other answer (another 4xx, a 200 without a token, an HTML page, a
// redirect, a 202 without a response-id) is neither.
type esetTokenError struct {
	msg      string
	rejected bool
	outage   bool
	err      error
}

// esetTokenOutageStatus is the HTTP half of outage: 5xx and 429.
func esetTokenOutageStatus(status int) bool {
	return status >= 500 || status == http.StatusTooManyRequests
}

func (e *esetTokenError) Error() string { return e.msg }
func (e *esetTokenError) Unwrap() error { return e.err }

// esetTokenRejectionCodes are the OAuth2 errors ESET's token endpoint gives
// a wrong email, password or API user (error or error_description).
var esetTokenRejectionCodes = map[string]bool{"invalid_grant": true, "invalid_username_or_password": true, "unauthorized_client": true}

// esetTokenRejected is the rule behind exit 4 (handfixes.json:
// mint-exit-code-tracks-rejection): the token endpoint answered 401 or 403,
// or 400 naming one of esetTokenRejectionCodes. Everything else, a 400 with
// another error included, is not a statement about the credentials.
func esetTokenRejected(status int, tr esetTokenResponse) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return true
	case http.StatusBadRequest:
		return esetTokenRejectionCodes[strings.ToLower(strings.TrimSpace(tr.Error))] ||
			esetTokenRejectionCodes[strings.ToLower(strings.TrimSpace(tr.ErrorDescription))]
	}
	return false
}

// esetMintErr maps a failed token mint to its exit status: 4 (auth) for an
// explicit credential rejection, 5 (API) when the token request failed for
// any other reason, so a script keying on 4 never tells the MSP to reset a
// password during an ESET outage.
func esetMintErr(err error) error {
	var te *esetTokenError
	if errors.As(err, &te) && !te.rejected {
		return apiErr(err)
	}
	return authErr(err)
}

// esetMintDeferral marks the clients whose token outage may be deferred to
// the first request; newClient sets it (esetAllowMintDeferral) and the client
// hook consumes it. Keyed by client, so concurrent clients never share it.
var esetMintDeferral sync.Map

// esetAllowMintDeferral marks c when the running command is a read the local
// mirror can answer: a generated GET command, or the root search command.
// With --data-source auto it falls back like an unreachable API, local never
// needed the token, and live fails at its first request with the same error
// and exit status. Any other command (sync, the live novel views, every
// write, doctor, and every client built outside newClient, MCP included)
// keeps failing at construction.
func esetAllowMintDeferral(c *client.Client, f *rootFlags) {
	if c == nil || f == nil || f.runningCmd == nil {
		return
	}
	cmd := f.runningCmd
	search := cmd.Name() == "search" && cmd.HasParent() && !cmd.Parent().HasParent()
	if cmd.Annotations["pp:method"] == http.MethodGet || search {
		esetMintDeferral.Store(c, struct{}{})
	}
}

// esetTokenFallbackNoted records the commands that already printed the
// token-fallback line.
var esetTokenFallbackNoted sync.Map // *rootFlags -> struct{}

// esetNoteTokenFallback prints, once per command and in every output mode,
// the stderr line an auto read owes when it answers from the local mirror
// because no token could be minted (handfixes.json:
// auto-falls-back-when-token-service-is-down). It goes through the command's
// stderr writer, so the token failure's text is escaped and masked like any
// error.
func esetNoteTokenFallback(flags *rootFlags, err error) {
	if flags == nil || !client.IsTokenUnavailable(err) {
		return
	}
	if _, dup := esetTokenFallbackNoted.LoadOrStore(flags, struct{}{}); dup {
		return
	}
	w := io.Writer(os.Stderr)
	if flags.runningCmd != nil {
		w = flags.runningCmd.ErrOrStderr()
	}
	fmt.Fprintf(w, "warning: no ESET Connect token could be minted, so this answer comes from the local mirror (source local, reason api_unreachable): %s\n", strings.Join(strings.Fields(err.Error()), " "))
}

// esetStoreIdentity is the stable local-mirror scope: region plus the hashed
// API user, or the stamp of a token this CLI minted. A bare ESET_PROTECT_TOKEN
// with no username falls back to the generated per-credential scope ("").
func esetStoreIdentity(c *config.Config) string {
	if c == nil {
		return ""
	}
	region, err := esetRegion(c)
	if err != nil {
		return ""
	}
	if c.EsetProtectToken == "" {
		if u := strings.TrimSpace(c.EsetProtectUsername); u != "" {
			return esetIdentity(region, u)
		}
		if strings.HasPrefix(c.ClientID, "eset:"+region+":") {
			return c.ClientID
		}
	}
	return ""
}

// esetDropForeignToken clears a cached token this CLI minted for a different
// region or API user. It runs whether or not a password is available, so a
// token minted for one tenant never reaches another tenant's request.
// Tokens stored by `auth set-token` carry no stamp and are left alone.
func esetDropForeignToken(cfg *config.Config, region string) {
	if cfg.AccessToken == "" || !strings.HasPrefix(cfg.ClientID, "eset:") {
		return
	}
	parts := strings.SplitN(cfg.ClientID, ":", 3)
	foreign := len(parts) != 3 || parts[1] != region
	if u := strings.TrimSpace(cfg.EsetProtectUsername); u != "" && cfg.ClientID != esetIdentity(region, u) {
		foreign = true
	}
	if foreign {
		cfg.AccessToken, cfg.RefreshToken, cfg.TokenExpiry = "", "", time.Time{}
	}
}

// esetEnsureToken leaves cfg holding a usable bearer token when it can.
// force skips the cached token (used after the API answered 401); persist
// controls whether a fresh token is saved (false for concurrent re-mints,
// which work on a private copy of the config).
//
// Precedence: ESET_PROTECT_TOKEN env > cached token bound to this
// region+user > refresh grant > password grant. Without username+password it
// leaves a token the user stored (`auth set-token`) alone.
func esetEnsureToken(ctx context.Context, cfg *config.Config, force, persist bool) error {
	if cfg == nil {
		return nil
	}
	region, err := esetRegion(cfg)
	if err != nil {
		return err
	}
	if cfg.EsetProtectToken != "" {
		// An explicit token wins; keep a cached minted token from shadowing it.
		cfg.AccessToken = ""
		return nil
	}
	esetDropForeignToken(cfg, region)
	if !esetHasCredentials(cfg) {
		return nil
	}
	id := esetIdentity(region, cfg.EsetProtectUsername)
	sameIdentity := cfg.ClientID == id
	if !force && sameIdentity && cfg.AccessToken != "" &&
		(cfg.TokenExpiry.IsZero() || time.Now().Before(cfg.TokenExpiry.Add(-esetTokenRefreshMargin))) {
		return nil
	}

	var tr *esetTokenResponse
	if sameIdentity && cfg.RefreshToken != "" {
		tr, _ = esetRequestToken(ctx, region, url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {cfg.RefreshToken},
		})
	}
	if tr == nil {
		// A token minted for another tenant must never reach the wire.
		cfg.AccessToken = ""
		tr, err = esetRequestToken(ctx, region, url.Values{
			"grant_type": {"password"},
			"username":   {strings.TrimSpace(cfg.EsetProtectUsername)},
			"password":   {cfg.EsetProtectPassword},
		})
		if err != nil {
			return err
		}
	}
	expiry := time.Time{}
	if tr.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	cfg.AuthHeaderVal = ""
	saved := false
	if persist {
		// Best-effort: a read-only home (CI) must not break the command.
		saved = cfg.SaveTokens(id, "", tr.AccessToken, tr.RefreshToken, expiry) == nil
	}
	if !saved {
		cfg.ClientID, cfg.AccessToken, cfg.RefreshToken, cfg.TokenExpiry = id, tr.AccessToken, tr.RefreshToken, expiry
	}
	cfg.AuthSource = "oauth2:ESET_PROTECT_USERNAME"
	return nil
}

// esetReauth is the transport's 401 handler. It re-mints on a private copy of
// the config and publishes the new bearer through the transport, so the
// shared config the generated client reads is never written concurrently.
// When several in-flight requests hit 401 together only the first mints; the
// rest see a published token that differs from the one they were refused with.
func esetReauth(cfg *config.Config, tr *client.EsetTransport) func(ctx context.Context, rejected string) (string, error) {
	return func(ctx context.Context, rejected string) (string, error) {
		if !esetHasCredentials(cfg) || cfg.EsetProtectToken != "" {
			return "", nil
		}
		esetMintMu.Lock()
		defer esetMintMu.Unlock()
		if cur := tr.Auth(); cur != "" && cur != rejected {
			return cur, nil
		}
		fresh := *cfg
		if err := esetEnsureToken(ctx, &fresh, true, false); err != nil {
			// The transport returns this instead of the 401 (handfixes.json:
			// reauth-failure-keeps-its-class): classified like the initial
			// mint, and an outage reads as a token outage.
			var te *esetTokenError
			if errors.As(err, &te) && te.outage && !te.rejected {
				return "", &client.TokenUnavailableError{Err: esetMintErr(err)}
			}
			return "", esetMintErr(err)
		}
		header := "Bearer " + fresh.AccessToken
		tr.SetAuth(header)
		return header, nil
	}
}

// esetArmHeaderTimeout gives the base transport a ResponseHeaderTimeout.
// client.StreamingHTTPClient (binary downloads such as quarantined-objects
// download) drops the whole-call timeout and re-arms the header timeout only
// on a bare *http.Transport. Behind EsetTransport it never sees one, so a
// download whose connection opens but never answers would hang forever. The
// value is the client's own timeout, which already bounds every other call,
// so ordinary requests behave exactly as before.
func esetArmHeaderTimeout(rt http.RoundTripper, d time.Duration) http.RoundTripper {
	t, ok := rt.(*http.Transport)
	if !ok || t.ResponseHeaderTimeout > 0 || d <= 0 {
		return rt
	}
	armed := t.Clone()
	armed.ResponseHeaderTimeout = d
	return armed
}

func esetClientHook(c *client.Client) error {
	if c == nil {
		return nil
	}
	_, deferOutage := esetMintDeferral.LoadAndDelete(c)
	if c.HTTPClient != nil {
		if _, wrapped := c.HTTPClient.Transport.(*client.EsetTransport); !wrapped {
			// Arm the header timeout on the base transport before wrapping it
			// (handfixes.json: streaming-download-header-timeout).
			c.HTTPClient.Transport = esetArmHeaderTimeout(c.HTTPClient.Transport, c.ConfiguredTimeout())
			tr := &client.EsetTransport{Base: c.HTTPClient.Transport, AllowHost: esetHostAllower(c.Config)}
			tr.Reauth = esetReauth(c.Config, tr)
			c.HTTPClient.Transport = tr
		}
	}
	// A dry run sends nothing, so it only previews the URL.
	if c.DryRun {
		return nil
	}
	// The region is substituted into every service host. Validate it for every
	// client that sends (CLI and MCP, whatever the credential source) so a
	// crafted ESET_PROTECT_REGION can never steer a bearer token elsewhere.
	if _, err := esetRegion(c.Config); err != nil {
		return configErr(err)
	}
	esetMintMu.Lock()
	defer esetMintMu.Unlock()
	// Bound the initial mint by the command's --timeout, not the token
	// client's own two-minute ceiling.
	ctx, cancel := context.WithTimeout(context.Background(), c.ConfiguredTimeout())
	defer cancel()
	if err := esetEnsureToken(ctx, c.Config, false, true); err != nil {
		// For a client built for a read that --data-source auto can answer
		// from the local mirror (esetAllowMintDeferral), a mint that failed
		// because the token service is down (no answer, 5xx, 429; adversarial
		// round 6 narrowed this from "anything but a rejection") is recorded
		// on the transport instead:
		// every request fails with the same error and exit status, and the
		// auto read falls back like an unreachable API (handfixes.json:
		// auto-falls-back-when-token-service-is-down). Every other client
		// fails here as before, and a rejection always does (exit 4).
		var te *esetTokenError
		if errors.As(err, &te) && te.outage && !te.rejected && deferOutage && c.HTTPClient != nil {
			if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
				tr.SetTokenUnavailable(esetMintErr(err))
				return nil
			}
		}
		return esetMintErr(err)
	}
	// Generated AuthHeader() lazily rewrites AuthSource; settle it here, while
	// the client is still single-threaded, so concurrent requests only read.
	if c.Config != nil {
		_ = c.Config.AuthHeader()
	}
	return nil
}

// esetLoginProbeError turns a failed `auth login` probe read into the exit
// status: 4 when ESET refused the freshly minted token (HTTP 401 or 403, the
// API user lacks ESET Connect or device-management access), 5 for any other
// failure (network, HTTP 5xx, a base URL that is not the ESET API). nil when
// the probe read succeeded.
func esetLoginProbeError(err error) error {
	if err == nil {
		return nil
	}
	var refused *client.APIError
	if errors.As(err, &refused) && (refused.StatusCode == http.StatusUnauthorized || refused.StatusCode == http.StatusForbidden) {
		return authErr(fmt.Errorf("ESET Connect issued a token, but device-management refused it (HTTP %d on GET /v1/device_groups): give the API user ESET Connect access (the Integrations toggle) and permission to read devices: %w", refused.StatusCode, err))
	}
	return apiErr(fmt.Errorf("ESET Connect issued a token, but the probe read (GET /v1/device_groups) failed, so the login is not verified: %w", err))
}

func newEsetAuthLoginCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Exchange the ESET Connect API user's email and password for a bearer token and cache it",
		Long: "Mint a one-hour ESET Connect access token with the OAuth2 password grant from\n" +
			"ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD (region from ESET_PROTECT_REGION,\n" +
			"default eu), cache it for this region and user, and prove it with one read call.\n" +
			"Other commands mint and refresh on demand, so login is optional; use it to check\n" +
			"credentials. The password is never written to disk by this command.",
		Example: "  ESET_PROTECT_USERNAME=api@msp.example ESET_PROTECT_PASSWORD=... eset-protect-cli auth login\n" +
			"  ESET_PROTECT_REGION=us eset-protect-cli auth login --json",
		Annotations: map[string]string{"mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			region, err := esetRegion(cfg)
			if err != nil {
				return usageErr(err)
			}
			if flags.dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "POST %s\n  grant_type=password&username=<ESET_PROTECT_USERNAME>&password=****\n\n(dry run - no request sent)\n", esetTokenURL(region))
				return nil
			}
			if !esetHasCredentials(cfg) {
				return authErr(fmt.Errorf("set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD to the ESET Connect API user's email and password (create one in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2 with the Integrations toggle on: %s)", EsetAPIUserURL))
			}
			cfg.EsetProtectToken = ""
			ctx, cancel := context.WithTimeout(cmd.Context(), flags.timeout)
			defer cancel()
			if err := esetEnsureToken(ctx, cfg, true, true); err != nil {
				return esetMintErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)
			c.HTTPClient.Transport = &client.EsetTransport{Base: c.HTTPClient.Transport, AllowHost: esetHostAllower(cfg)}
			verified := "token accepted by device-management"
			_, probeErr := c.Get(cmd.Context(), "https://"+region+".device-management.eset.systems/v1/device_groups", map[string]string{"pageSize": "1"})
			if probeErr != nil {
				verified = "token minted, but the probe call failed: " + probeErr.Error()
			}
			out := map[string]any{
				"region":     region,
				"user":       strings.TrimSpace(cfg.EsetProtectUsername),
				"expires_at": cfg.TokenExpiry.UTC().Format(time.RFC3339),
				"verified":   verified,
				"probe_ok":   probeErr == nil,
			}
			if flags.asJSON || flags.agent {
				if err := flags.printJSON(cmd, out); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Logged in to ESET Connect (%s) as %s; token expires %s\n%s\n", region, out["user"], out["expires_at"], verified)
			}
			// A token that cannot read is not a working login
			// (handfixes.json: auth-login-fails-when-probe-fails).
			return esetLoginProbeError(probeErr)
		},
	}
	return cmd
}
