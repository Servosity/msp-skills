// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: StreamOne ION refresh-token exchange (auth mint) and token check (auth check).
// pp:data-source live

package cli

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"
	"td-synnex-pp-cli/internal/config"
)

// ION refresh tokens are single-use and last 32 days (docs.streamone.cloud#authentication).
const ionRefreshTokenLifetime = 32 * 24 * time.Hour

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		authCmd, _, err := root.Find([]string{"auth"})
		if err != nil || authCmd == nil || authCmd == root {
			return
		}
		addNovelCommandIfAbsent(authCmd, newAuthMintCmd(flags))
		addNovelCommandIfAbsent(authCmd, newAuthCheckCmd(flags))
		if setCmd, _, err := authCmd.Find([]string{"set-token"}); err == nil && setCmd != nil && setCmd != authCmd {
			setCmd.RunE = ionSetTokenRunE(flags)
		}
		if logoutCmd, _, err := authCmd.Find([]string{"logout"}); err == nil && logoutCmd != nil && logoutCmd != authCmd {
			logoutCmd.RunE = ionLogoutRunE(flags)
		}
	})
}

// ionSetTokenRunE replaces the generated set-token, which saved the access
// token with an empty refresh token. ION refresh tokens are single-use and
// already rotated, so dropping the stored one locks the user out until they
// request new credentials in the portal.
func ionSetTokenRunE(flags *rootFlags) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return writeDryRun(cmd.OutOrStdout(), flags, "auth set-token (would save the access token and keep the stored refresh token)")
		}
		token, err := readSecretFromStdin(cmd.InOrStdin())
		if err != nil {
			return authErr(err)
		}
		credsPath, restoreHome, err := ionCredentialsTarget(flags)
		if err != nil {
			return configErr(err)
		}
		defer restoreHome()
		if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
			return configErr(err)
		}
		var cfg *config.Config
		// Same lock as auth mint: loading the refresh token outside it could save a
		// token a concurrent mint just rotated away.
		err = cliutil.WithFileLock(credsPath+".mint", func() error {
			var lerr error
			if cfg, lerr = config.Load(flags.configPath); lerr != nil {
				return configErr(lerr)
			}
			// A credentials file refused for unsafe permissions was not read, so
			// saving now would overwrite its single-use refresh token with nothing.
			if cfg.HasCredentialRefusals() {
				return authErr(cfg.CredentialRefusalError())
			}
			keep, serr := refreshTokenToKeep(cfg, credsPath)
			if serr != nil {
				return authErr(serr)
			}
			cfg.AuthHeaderVal = "" // a legacy auth_header or file-stored token would shadow the saved one
			cfg.DropFileAccessToken()
			cfg.KeepFileEndpoint()
			// Expiry is unknown for a pasted token, so auth mint --if-expiring refreshes it.
			if serr := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, token, keep, time.Time{}); serr != nil {
				return configErr(fmt.Errorf("saving token: %w", serr))
			}
			return nil
		})
		if err != nil {
			return err
		}
		savePath := credentialSavePath(cfg)
		if flags.asJSON {
			out := map[string]any{"saved": true, "config_path": cfg.Path, "refresh_token_kept": cfg.RefreshToken != ""}
			if !cfg.AgentcookieManagedByExternalStore() {
				out["credentials_path"] = savePath
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Token saved to %s\n", savePath)
		return nil
	}
}

// ionCredentialsTarget returns the credentials file config.Load reads for this
// invocation, and points the save path at it until restore runs. With an
// explicit config file (--config or TD_SYNNEX_CONFIG) Load reads
// <config dir>/data/credentials.toml when it holds credentials and otherwise
// falls back to the default store, while the generated save path is always the
// default store. Only the first case needs the save path moved; if a per-kind
// data-dir override still wins over the home override, the command refuses.
// refreshTokenToKeep is the stored refresh token a save must carry forward.
// Token fields in config.toml make Load skip credentials.toml, so the token is
// then read from credsPath (the file the save replaces) under Load's own
// permission check. A refresh_token in config.toml is refused: it may be one
// ION already spent, and saving it would replace the newer one.
func refreshTokenToKeep(cfg *config.Config, credsPath string) (string, error) {
	// Only config.toml (cfg.Path) is checked; under agentcookie it is the
	// credential store, so it is skipped. ponytail: the pre-paths legacy file is
	// not checked; this connector never shipped with that layout.
	if !cfg.AgentcookieManagedByExternalStore() {
		if rt, err := tomlRefreshToken(cfg.Path, false); err == nil && rt != "" {
			return "", fmt.Errorf("%s holds a refresh_token, which hides the one in the credentials file and may already be spent; delete that line (keep a copy), then run this again", cfg.Path)
		}
	}
	if cfg.RefreshToken != "" {
		return cfg.RefreshToken, nil
	}
	return tomlRefreshToken(credsPath, true)
}

// tomlRefreshToken reads refresh_token from a TOML file; a missing file holds
// none. checkPerms applies the credentials-file permission guard first.
func tomlRefreshToken(path string, checkPerms bool) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if checkPerms {
		if err := cliutil.VerifyCredsPerms(real); err != nil {
			return "", fmt.Errorf("refusing to replace %s: %w", path, err)
		}
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return "", err
	}
	var f struct {
		RefreshToken string `toml:"refresh_token"`
	}
	if err := toml.Unmarshal(data, &f); err != nil {
		return "", fmt.Errorf("%s is not valid TOML, so its refresh token cannot be kept: %w", path, err)
	}
	return f.RefreshToken, nil
}

func ionCredentialsTarget(flags *rootFlags) (string, func(), error) {
	noop := func() {}
	def, err := cliutil.CredentialsFilePath()
	if err != nil {
		return "", noop, err
	}
	cfgPath := strings.TrimSpace(flags.configPath)
	if cfgPath == "" {
		cfgPath = strings.TrimSpace(os.Getenv("TD_SYNNEX_CONFIG"))
	}
	if cfgPath == "" {
		return def, noop, nil
	}
	if cfgPath, err = filepath.Abs(cfgPath); err != nil {
		return "", noop, err
	}
	sibling, err := cliutil.CredentialsFilePathForConfig(cfgPath)
	if err != nil {
		return "", noop, err
	}
	if sibling == def {
		return def, noop, nil
	}
	creds, status, lerr := cliutil.LoadCredentialsForConfigWithStatus(cfgPath)
	if status.Refusal != nil && status.Refusal.CredentialsPresent {
		// Load refuses these credentials and does not fall back to the default
		// store. Even when config.toml itself holds the tokens today, a save moves
		// them out of config.toml, after which Load reads this refused file. So
		// any save or clear now would act on a store this config cannot use.
		return "", noop, fmt.Errorf("credentials at %s are refused (%v); fix them (for example chmod 600) before changing tokens for this config", sibling, status.Refusal.Err)
	}
	if lerr != nil {
		// An unreadable or malformed sibling makes later loads fail, so a save
		// anywhere would strand the rotated refresh token.
		return "", noop, fmt.Errorf("reading credentials at %s: %w; fix or remove that file before changing tokens for this config", sibling, lerr)
	}
	if creds == nil || !creds.HasValues() {
		return def, noop, nil // Load falls back to the default store, and so do saves
	}
	restore, err := cliutil.SetHomeOverride(filepath.Dir(filepath.Dir(sibling)))
	if err != nil {
		return "", noop, err
	}
	if got, err := cliutil.CredentialsFilePath(); err != nil || got != sibling {
		restore()
		return "", noop, fmt.Errorf("cannot keep credentials next to %s: the credentials store resolves to %s instead; unset the data-dir override or use --home", cfgPath, got)
	}
	return sibling, restore, nil
}

// ionLogoutRunE replaces the generated logout so clearing tokens does not
// persist this run's TD_SYNNEX_BASE_URL or account ID, and does not race a mint.
func ionLogoutRunE(flags *rootFlags) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return writeDryRun(cmd.OutOrStdout(), flags, "auth logout (would clear the stored access and refresh tokens)")
		}
		credsPath, restoreHome, err := ionCredentialsTarget(flags)
		if err != nil {
			return configErr(err)
		}
		defer restoreHome()
		if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
			return configErr(err)
		}
		err = cliutil.WithFileLock(credsPath+".mint", func() error {
			cfg, lerr := config.Load(flags.configPath)
			if lerr != nil {
				return configErr(lerr)
			}
			cfg.KeepFileEndpoint()
			if cerr := cfg.ClearTokens(); cerr != nil {
				return configErr(fmt.Errorf("clearing tokens: %w", cerr))
			}
			return nil
		})
		if err != nil {
			return err
		}
		envStillSet := ""
		if os.Getenv("TD_SYNNEX_ACCESS_TOKEN") != "" {
			envStillSet = "TD_SYNNEX_ACCESS_TOKEN"
		}
		restoreHome() // check what the next run will load, with normal path resolution
		fallback := ""
		envCleared := envStillSet == ""
		if after, lerr := config.Load(flags.configPath); lerr == nil && envCleared && after.AuthHeader() != "" {
			if def, derr := cliutil.CredentialsFilePath(); derr == nil {
				fallback = def
			}
		}
		if fallback != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: credentials in %s are still loaded as a fallback for this config; run auth logout without --config to clear them\n", fallback)
		}
		if flags.asJSON {
			out := map[string]any{"cleared": true}
			if envStillSet != "" {
				out["note"] = envStillSet + " env var is still set"
			}
			if fallback != "" {
				out["fallback_credentials_still_loaded"] = fallback
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		}
		if envStillSet != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Config cleared. Note: %s env var is still set.\n", envStillSet)
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Logged out. Credentials cleared.")
		return nil
	}
}

// ionOAuthURL derives https://<host>/oauth/<endpoint> from the configured API base URL,
// so TD_SYNNEX_BASE_URL also redirects token calls (for example to a test server).
func ionOAuthURL(baseURL, endpoint string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://ion.tdsynnex.com"
	}
	if i := strings.Index(base, "://"); i >= 0 {
		if j := strings.Index(base[i+3:], "/"); j >= 0 {
			base = base[:i+3+j]
		}
	}
	return strings.TrimRight(base, "/") + "/oauth/" + endpoint
}

// postIONForm posts a form to the ION OAuth host and decodes a JSON object.
// Redirects are refused so the form (which carries a token) is never resent to
// another host. An error body is reduced to its OAuth error code;
// secrets in the request are never echoed. status is 0 when no response came back.
func postIONForm(ctx context.Context, hc *http.Client, endpoint string, form url.Values, timeout time.Duration) (map[string]json.RawMessage, int, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(context.WithValue(ctx, ionOAuthExchange{}, true), http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	nc := http.Client{}
	if hc != nil {
		nc = *hc
	}
	nc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := nc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("calling %s: %s", endpoint, ionTransportErrorKind(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode != http.StatusOK {
		// Only the OAuth error code is echoed: error_description and message
		// are free text a server can fill with the token it was sent.
		msg := http.StatusText(resp.StatusCode)
		var code string
		if json.Unmarshal(out["error"], &code) == nil && oauthErrorCodes[code] {
			msg = code
		}
		return out, resp.StatusCode, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, endpoint, msg)
	}
	return out, resp.StatusCode, nil
}

// ionTransportErrorKind names a token-exchange transport failure by kind,
// never by text: net/http puts bytes the server sent (a malformed header
// line, for example) into its error, and the request carried the refresh or
// access token the server could echo. Only fixed strings and OS errno text
// reach the message.
func ionTransportErrorKind(err error) string {
	var ne net.Error
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var errno syscall.Errno
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return "timed out"
	case errors.As(err, &dnsErr):
		return "DNS lookup failed"
	case errors.As(err, &certErr):
		return "TLS certificate verification failed"
	case errors.As(err, &errno):
		return "connection failed: " + errno.Error()
	}
	return "transport failure (connection interrupted or response malformed)"
}

// oauthErrorCodes are the RFC 6749 section 5.2 and RFC 6750 section 3.1
// error codes. Only a listed code is echoed: any other "error" value is free
// text a server can fill with the token it was sent (a shape rule such as
// lowercase-and-underscores let "testsecrettoken" through).
var oauthErrorCodes = map[string]bool{
	"invalid_request": true, "invalid_client": true, "invalid_grant": true,
	"unauthorized_client": true, "unsupported_grant_type": true, "invalid_scope": true,
	"access_denied": true, "server_error": true, "temporarily_unavailable": true,
	"invalid_token": true, "insufficient_scope": true, "unsupported_token_type": true,
}

// ionTokenHostAllowed refuses to send a token to a host other than
// ion.tdsynnex.com unless TD_SYNNEX_BASE_URL points there in this run. A
// base_url persisted into config.toml (for example from an earlier test run)
// must not silently receive the single-use refresh token.
func ionTokenHostAllowed(endpoint, configPath string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parsing token URL %q: %w", endpoint, err)
	}
	// Same rule as redactingTransport, checked here so the refusal says the
	// token was never sent instead of surfacing as a failed exchange.
	if !strings.EqualFold(u.Scheme, "https") && !(strings.EqualFold(u.Scheme, "http") && isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("refusing to send a token to %q (the token was not sent): ION tokens travel only over https, and only a loopback test server may use http", endpoint)
	}
	if ionHostAllowed(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("refusing to send a token to %s: base_url in %s points away from ion.tdsynnex.com and TD_SYNNEX_BASE_URL is not set; fix base_url or set TD_SYNNEX_BASE_URL for this run", u.Host, configPath)
}

// ionHTTPErr types an OAuth endpoint failure: no response or a 5xx is an API
// failure, 429 is rate limiting, 400/401/403 are ION rejecting the credential,
// and any other status is an API failure.
func ionHTTPErr(status int, err error, rejected string) error {
	switch {
	case status == 429:
		return rateLimitErr(err)
	case status == 400 || status == 401 || status == 403:
		return authErr(fmt.Errorf("%s: %w", rejected, err))
	default:
		return apiErr(err)
	}
}

func rawString(m map[string]json.RawMessage, key string) string {
	var s string
	if json.Unmarshal(m[key], &s) == nil {
		return s
	}
	var f flexFloat
	if json.Unmarshal(m[key], &f) == nil && m[key] != nil {
		return fmt.Sprint(float64(f))
	}
	return ""
}

func rawSeconds(m map[string]json.RawMessage, key string) time.Duration {
	var f flexFloat
	if raw, ok := m[key]; ok && json.Unmarshal(raw, &f) == nil {
		return time.Duration(float64(f)) * time.Second
	}
	return 0
}

func newAuthMintCmd(flags *rootFlags) *cobra.Command {
	var fromStdin, printAccess bool
	var ifExpiring string

	cmd := &cobra.Command{
		Use:   "mint",
		Short: "Exchange a StreamOne ION refresh token for an access token and store the rotated refresh token",
		Long: "Exchange a StreamOne ION refresh token for a 2-hour access token at https://ion.tdsynnex.com/oauth/token. " +
			"ION refresh tokens are single-use: every exchange returns a NEW refresh token (valid 32 days) and kills the " +
			"old one. mint saves the new access token and refresh token to the private credentials file (0600, file " +
			"locked, atomic) before it prints anything; if that save fails it writes the new refresh token to a 0600 " +
			"rescue file next to the config so you are never locked out.\n\n" +
			"The refresh token is read from stdin (--refresh-token-stdin), else the token stored by the previous mint, " +
			"else TD_SYNNEX_REFRESH_TOKEN (used only before the first mint; the stored token is always newer). Get the first one in the ION portal: Settings > Users > Edit > OAuth credentials " +
			"> Request New Credentials. Token values are never printed unless --print-access-token is set. " +
			"Use --if-expiring from cron to refresh only when the stored access token is about to expire.",
		Example: strings.Trim(`
  pbpaste | td-synnex-cli auth mint --refresh-token-stdin
  td-synnex-cli auth mint --if-expiring 15m --json
  OTHER_TOOL_TOKEN=$(td-synnex-cli auth mint --print-access-token --json | jq -r .access_token)`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			tokenURL := ionOAuthURL(cfg.BaseURL, "token")
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "auth mint (POST "+tokenURL+" grant_type=refresh_token)")
			}
			var margin time.Duration
			if ifExpiring != "" {
				margin, err = cliutil.ParseDurationLoose(ifExpiring)
				if err != nil || margin < 0 {
					return usageErr(fmt.Errorf("--if-expiring must be a duration like 15m or 1h, got %q", ifExpiring))
				}
			}
			if err := ionTokenHostAllowed(tokenURL, cfg.Path); err != nil {
				return configErr(err)
			}
			stdinToken := ""
			if fromStdin {
				if stdinToken, err = readSecretFromStdin(cmd.InOrStdin()); err != nil {
					return usageErr(fmt.Errorf("reading refresh token from stdin: %w", err))
				}
			}
			credsPath, restoreHome, err := ionCredentialsTarget(flags)
			if err != nil {
				return configErr(err)
			}
			defer restoreHome()
			if err := os.MkdirAll(filepath.Dir(credsPath), 0o700); err != nil {
				return configErr(err)
			}
			// One mint at a time across processes: refresh tokens are single-use, so two
			// overlapping runs (cron plus a manual run) must not both spend the stored one.
			// The lock is a separate file; SaveTokens locks the credentials file itself.
			return cliutil.WithFileLock(credsPath+".mint", func() error {
				return runIONMint(cmd, flags, stdinToken, ifExpiring != "", margin, printAccess, credsPath)
			})
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "refresh-token-stdin", false, "Read the refresh token from stdin (keeps it out of shell history)")
	cmd.Flags().StringVar(&ifExpiring, "if-expiring", "", "Only mint when the stored access token expires within this window, for example 15m")
	cmd.Flags().BoolVar(&printAccess, "print-access-token", false, "Also print the new access token (for exporting to other tools)")
	return cmd
}

// runIONMint runs under the mint lock. It re-reads the config so a token a
// peer rotated while this run waited for the lock is the one used.
func runIONMint(cmd *cobra.Command, flags *rootFlags, stdinToken string, checkExpiry bool, margin time.Duration, printAccess bool, credsPath string) error {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return configErr(err)
	}
	// Refuse rather than mint over a credentials file that was not read: the
	// save would replace the stored refresh token the user may still need.
	if cfg.HasCredentialRefusals() {
		return authErr(cfg.CredentialRefusalError())
	}
	stored := ""
	if stdinToken == "" { // a piped token replaces whatever is stored
		if stored, err = refreshTokenToKeep(cfg, credsPath); err != nil {
			return authErr(err)
		}
	}
	if checkExpiry && cfg.AccessToken != "" && !cfg.TokenExpiry.IsZero() && time.Until(cfg.TokenExpiry) > margin {
		return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
			"minted": false, "reason": "stored access token is still valid",
			"access_token_expires_at": cfg.TokenExpiry.UTC().Format(time.RFC3339),
		}, flags)
	}
	refresh, source := "", ""
	switch {
	case stdinToken != "":
		refresh, source = stdinToken, "stdin"
	case stored != "":
		// The stored token is the newest rotation; an env value goes stale after the first mint.
		refresh, source = stored, "stored"
	case strings.TrimSpace(os.Getenv("TD_SYNNEX_REFRESH_TOKEN")) != "":
		refresh, source = strings.TrimSpace(os.Getenv("TD_SYNNEX_REFRESH_TOKEN")), "env:TD_SYNNEX_REFRESH_TOKEN"
	}
	if strings.TrimSpace(refresh) == "" {
		return authErr(fmt.Errorf("no refresh token: pipe one in with --refresh-token-stdin (ION portal: Settings > Users > Edit > OAuth credentials > Request New Credentials)"))
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	c, err := flags.newClient()
	if err != nil {
		return configErr(err)
	}
	tokenURL := ionOAuthURL(cfg.BaseURL, "token")
	// Re-check: base_url may have changed while this run waited for the lock.
	if err := ionTokenHostAllowed(tokenURL, cfg.Path); err != nil {
		return configErr(err)
	}
	resp, status, err := postIONForm(ctx, c.HTTPClient, tokenURL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "redirect_uri": {"http://localhost/"}}, flags.timeout)
	if err != nil {
		if status == 0 || status >= 500 {
			return apiErr(fmt.Errorf("token exchange failed before ION answered cleanly (%w); the refresh token may already be spent, so run 'auth check' before retrying it", err))
		}
		return ionHTTPErr(status, err, "ION rejected the refresh token (it may be used already or older than 32 days)")
	}
	// A legacy auth_header outranks the token about to be saved, and the saved
	// config must keep the file's endpoint, not this run's env overrides.
	cfg.AuthHeaderVal = ""
	cfg.DropFileAccessToken()
	cfg.KeepFileEndpoint()
	access := rawString(resp, "access_token")
	newRefresh := rawString(resp, "refresh_token")
	ttl := rawSeconds(resp, "expires_in")
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	now := time.Now().UTC()
	expiry := now.Add(ttl)
	switch {
	case access == "" && newRefresh == "":
		return authErr(fmt.Errorf("token exchange returned neither access_token nor refresh_token; the refresh token sent may now be spent"))
	case access == "":
		// The token just sent is spent; keep the replacement ION returned.
		if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "", newRefresh, time.Time{}); err != nil {
			return rescueRefreshToken(newRefresh, err)
		}
		return authErr(fmt.Errorf("token exchange returned no access_token; the new refresh token was saved, so run auth mint again"))
	case newRefresh == "":
		// Never keep the spent token as if it were valid: the next mint would fail opaquely.
		if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, access, "", expiry); err != nil {
			return configErr(fmt.Errorf("saving the access token: %w", err))
		}
		return authErr(fmt.Errorf("token exchange returned no new refresh_token: the access token was saved (valid until %s), but the next mint needs a new refresh token from the ION portal (Settings > Users > Edit > OAuth credentials > Request New Credentials)", expiry.Format(time.RFC3339)))
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, access, newRefresh, expiry); err != nil {
		return rescueRefreshToken(newRefresh, err)
	}
	if dir, err := cliutil.ConfigDir(); err == nil {
		_ = os.Remove(filepath.Join(dir, ionRescueFile)) // superseded by the token just saved
	}
	out := map[string]any{
		"minted":                           true,
		"access_token_expires_at":          expiry.Format(time.RFC3339),
		"refresh_token_rotated":            newRefresh != refresh,
		"refresh_token_valid_until_est":    now.Add(ionRefreshTokenLifetime).Format(time.RFC3339),
		"refresh_token_source":             source,
		"saved_to":                         credsPath,
		"env_access_token_overrides_saved": strings.TrimSpace(os.Getenv("TD_SYNNEX_ACCESS_TOKEN")) != "",
	}
	if printAccess {
		out["access_token"] = access
	}
	if out["env_access_token_overrides_saved"] == true {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: TD_SYNNEX_ACCESS_TOKEN is set and overrides the token just saved; unset it to use the stored token")
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Minted an access token (expires %s) and saved the rotated refresh token to %s.\n", expiry.Format(time.RFC3339), credsPath)
	if printAccess {
		fmt.Fprintln(cmd.OutOrStdout(), access)
	}
	return nil
}

const ionRescueFile = "refresh-token.rescue"

// rescueRefreshToken keeps a just-rotated refresh token reachable when the
// credentials save failed: the old token is already spent. It writes a 0600
// file (config dir, else the temp dir) and never prints the token itself.
func rescueRefreshToken(refresh string, saveErr error) error {
	write := func() (string, error) {
		if dir, err := cliutil.ConfigDir(); err == nil {
			path := filepath.Join(dir, ionRescueFile)
			if err := cliutil.AtomicWritePrivateFile(path, []byte(refresh+"\n"), 0o600, 0o700); err == nil {
				return path, nil
			}
		}
		f, err := os.CreateTemp("", "td-synnex-refresh-*.rescue") // CreateTemp opens 0600
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := f.WriteString(refresh + "\n"); err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		return f.Name(), nil
	}
	path, err := write()
	if err != nil {
		return configErr(fmt.Errorf("saving credentials failed (%v) and no rescue file could be written (%v); the new refresh token is lost, so request new credentials in the ION portal", saveErr, err))
	}
	return configErr(fmt.Errorf("saving credentials failed (%v); the new refresh token was written to %s (0600): pipe it to 'auth mint --refresh-token-stdin' once the config is writable, then delete the file", saveErr, path))
}

func newAuthCheckCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Ask StreamOne ION how long the current access and refresh tokens stay valid",
		Long: "Validate the access token in use (TD_SYNNEX_ACCESS_TOKEN, else the one stored by auth mint) against " +
			"https://ion.tdsynnex.com/oauth/validateAccess and report the remaining lifetime of the access token and its " +
			"refresh token, so a 32-day refresh token never lapses unnoticed. Exits 4 when ION rejects the token.",
		Example: strings.Trim(`
  td-synnex-cli auth check
  td-synnex-cli auth check --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			checkURL := ionOAuthURL(cfg.BaseURL, "validateAccess")
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "auth check (POST "+checkURL+")")
			}
			token := strings.TrimSpace(strings.TrimPrefix(cfg.AuthHeader(), "Bearer "))
			if token == "" {
				return authErr(fmt.Errorf("no access token: run 'td-synnex-cli auth mint --refresh-token-stdin'"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return configErr(err)
			}
			if err := ionTokenHostAllowed(checkURL, cfg.Path); err != nil {
				return configErr(err)
			}
			resp, status, err := postIONForm(ctx, c.HTTPClient, checkURL, url.Values{"access_token": {token}}, flags.timeout)
			now := time.Now().UTC()
			storedRT := cfg.RefreshToken
			if storedRT == "" { // config.toml token fields make Load skip credentials.toml
				if credsPath, restore, cerr := ionCredentialsTarget(flags); cerr == nil {
					storedRT, _ = tomlRefreshToken(credsPath, false)
					restore()
				}
			}
			out := map[string]any{"token_source": cfg.AuthSource, "refresh_token_stored": storedRT != ""}
			if !cfg.TokenExpiry.IsZero() {
				out["stored_access_token_expires_at"] = cfg.TokenExpiry.UTC().Format(time.RFC3339)
			}
			if err != nil {
				out["valid"] = false
				out["error"] = err.Error()
				_ = printJSONFiltered(cmd.OutOrStdout(), out, flags)
				if status == 0 || status >= 500 {
					return apiErr(fmt.Errorf("could not validate the access token: %w", err))
				}
				return ionHTTPErr(status, err, "ION rejected the access token")
			}
			out["valid"] = true
			if d := rawSeconds(resp, "access_expires_in"); d > 0 {
				out["access_expires_in_seconds"] = int(d.Seconds())
				out["access_expires_at"] = now.Add(d).Format(time.RFC3339)
			}
			if d := rawSeconds(resp, "refresh_expires_in"); d > 0 {
				out["refresh_expires_in_seconds"] = int(d.Seconds())
				out["refresh_expires_at"] = now.Add(d).Format(time.RFC3339)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			when := func(k string) string {
				if v, ok := out[k].(string); ok {
					return v
				}
				return "unknown (ION did not say)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Access token valid (source %v). Access expires %s; refresh token expires %s.\n", out["token_source"], when("access_expires_at"), when("refresh_expires_at"))
			return nil
		},
	}
	return cmd
}
