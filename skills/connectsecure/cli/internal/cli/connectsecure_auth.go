// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: ConnectSecure token exchange (POST /w/authorize).
//
// ConnectSecure does not use OAuth. A client mints a JWT by POSTing
// /w/authorize with Client-Auth-Token = base64(tenant + "+" + client_id + ":" +
// client_secret); the response carries data.access_token and data.user_id, and
// every later call sends Authorization: Bearer <token> plus X-USER-ID: <user_id>.
// The generated client already sends both headers from Config.AccessToken and
// Config.ConnectsecureUserId; this file fills them before the first request.

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	"time"

	"connectsecure-pp-cli/internal/client"
	"connectsecure-pp-cli/internal/cliutil"
	"connectsecure-pp-cli/internal/config"

	"github.com/spf13/cobra"
)

// connectsecureTokenCacheMax bounds how long a cached JWT is reused even when
// its exp claim is later, so a revoked token heals without manual cleanup.
// ponytail: fixed 30m cap; re-mint on 401 needs a client response hook the generator does not emit.
const connectsecureTokenCacheMax = 30 * time.Minute

// connectsecureFlags is the root flag set, captured so the doctor and
// auth-status checks read the same --config file the command uses.
var connectsecureFlags *rootFlags

func init() {
	registerClientHook(connectsecureClientHook)
	registerNovelCommand(func(_ *cobra.Command, flags *rootFlags) { connectsecureFlags = flags })
	config.StoreScopeIdentity = connectsecureStoreIdentity
	doctorAuthConfiguredHook = connectsecureDoctorAuthConfigured
}

// connectsecureMintConfigured reports whether tenant + client id + secret are
// all present, which is enough for the client to mint a JWT on first use.
func connectsecureMintConfigured(cfg *config.Config) bool {
	return cfg != nil && cfg.ConnectsecureTenant != "" && cfg.ClientID != "" && cfg.ClientSecret != ""
}

const connectsecureMintSource = "mint: CONNECTSECURE_TENANT + CONNECTSECURE_CLIENT_ID + CONNECTSECURE_CLIENT_SECRET"

func connectsecureDoctorAuthConfigured() (bool, string) {
	path := ""
	if connectsecureFlags != nil {
		path = connectsecureFlags.configPath
	}
	cfg, err := config.Load(path)
	if err != nil || !connectsecureMintConfigured(cfg) {
		return false, ""
	}
	return true, connectsecureMintSource
}

// connectsecureStoreIdentity pins the local store to the tenant + API client,
// not to the rotating JWT, so re-minting keeps the same database.
func connectsecureStoreIdentity(c *config.Config) string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.ConnectsecureTenant != "" && c.ClientID != "" {
		return "connectsecure:" + base + "|tenant=" + c.ConnectsecureTenant + "|client=" + c.ClientID
	}
	if c.ConnectsecureUserId != "" {
		return "connectsecure:" + base + "|user=" + c.ConnectsecureUserId
	}
	return ""
}

func connectsecureClientHook(c *client.Client) error {
	cfg := c.Config
	if cfg == nil || c.DryRun {
		return nil
	}
	if problem := connectsecureBaseURLProblem(cfg.BaseURL); problem != "" {
		return configErr(errors.New(problem))
	}
	if !connectsecureMintConfigured(cfg) {
		return nil // pre-minted token or nothing; generated auth checks report what is missing
	}
	if cfg.AccessToken != "" && !connectsecureJWTExpired(cfg.AccessToken, time.Now()) {
		return nil // an explicit, unexpired pre-minted token wins
	}
	if connectsecureFlags != nil && connectsecureFlags.dataSource == "local" {
		return nil // local-store reads never touch the network, so they must not need a token
	}
	tok, err := connectsecureMintCached(context.Background(), c)
	if errors.Is(err, errConnectsecureUnreachable) {
		// The request will fail with the same network error, which lets
		// --data-source auto fall back to the local store.
		return nil
	}
	if err != nil {
		return err
	}
	// The user id must belong to the token just minted; a stale configured
	// X-USER-ID would pair a fresh JWT with another user.
	cfg.AccessToken = tok.AccessToken
	cfg.ConnectsecureUserId = tok.UserID
	return nil
}

// connectsecureBaseURLProblem returns a user-facing message when the base URL
// cannot be the tenant pod API host, or "" when it looks usable.
func connectsecureBaseURLProblem(base string) string {
	const fix = "set CONNECTSECURE_BASE_URL to your pod API URL from the ConnectSecure portal (Profile > API Documentation), for example https://pod106.myconnectsecure.com"
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Hostname() == "" {
		return "CONNECTSECURE_BASE_URL is not a valid URL: " + fix
	}
	host := strings.ToLower(u.Hostname())
	if strings.HasPrefix(host, "your-pod.") {
		return "CONNECTSECURE_BASE_URL is not set: " + fix
	}
	// The mint header is a reversible encoding of the client secret and every
	// call carries a bearer token, so remote hosts must be HTTPS. Plain HTTP is
	// allowed only for loopback (local mocks and tests).
	if !strings.EqualFold(u.Scheme, "https") && !connectsecureLoopback(host) {
		return "CONNECTSECURE_BASE_URL must use https:// (credentials would travel in clear text): " + fix
	}
	switch host {
	case "portal.myconnectsecure.com", "portal.connectsecure.com", "connectsecure.com", "myconnectsecure.com", "www.connectsecure.com":
		return host + " is the ConnectSecure web portal, not the API: " + fix
	}
	return ""
}

func connectsecureLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// errConnectsecureUnreachable marks a mint that never reached the pod
// (DNS, refused, timeout), as opposed to one the pod answered.
var errConnectsecureUnreachable = errors.New("ConnectSecure token endpoint unreachable")

type connectsecureToken struct {
	AccessToken string    `json:"access_token"`
	UserID      string    `json:"user_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// connectsecureClientAuthToken builds the /w/authorize header value. The plus
// between tenant and client id is literal (vendor spec + n8n/MSPbots clients).
func connectsecureClientAuthToken(tenant, clientID, secret string) string {
	return base64.StdEncoding.EncodeToString([]byte(tenant + "+" + clientID + ":" + secret))
}

func connectsecureTokenCachePath(cfg *config.Config) (string, error) {
	dir, err := cliutil.CacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{cfg.BaseURL, cfg.ConnectsecureTenant, cfg.ClientID, cfg.ClientSecret}, "\x00")))
	return filepath.Join(dir, "connectsecure-token-"+hex.EncodeToString(sum[:8])+".json"), nil
}

func connectsecureMintCached(ctx context.Context, c *client.Client) (connectsecureToken, error) {
	path, pathErr := connectsecureTokenCachePath(c.Config)
	if pathErr == nil {
		if raw, err := os.ReadFile(path); err == nil { // #nosec G304 -- this CLI's own token cache: cache dir + sha256 hex name
			var tok connectsecureToken
			if json.Unmarshal(raw, &tok) == nil && tok.AccessToken != "" && time.Now().Before(tok.ExpiresAt) {
				return tok, nil
			}
		}
	}
	tok, err := connectsecureMint(ctx, c)
	if err != nil {
		return connectsecureToken{}, err
	}
	if pathErr == nil {
		if raw, err := json.Marshal(tok); err == nil { // #nosec G117 -- the JWT cache is written 0600 in a 0700 dir by design
			_ = cliutil.AtomicWritePrivateFile(path, raw, 0o600, 0o700) // cache miss next time is harmless
		}
	}
	return tok, nil
}

// connectsecureMint POSTs /w/authorize over a copy of the configured HTTP
// client that refuses redirects: Client-Auth-Token is a reversible encoding of
// the client secret, and Go forwards custom headers across redirects, so a
// redirect could hand the secret to another host. The mint also bypasses the
// response cache, so a JWT never lands in it.
func connectsecureMint(ctx context.Context, c *client.Client) (connectsecureToken, error) {
	cfg := c.Config
	endpoint := c.RequestBaseURL() + "/w/authorize"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return connectsecureToken{}, configErr(fmt.Errorf("building ConnectSecure token request: %w", err))
	}
	cat := connectsecureClientAuthToken(cfg.ConnectsecureTenant, cfg.ClientID, cfg.ClientSecret)
	req.Header.Set("Client-Auth-Token", cat)
	req.Header.Set("Accept", "application/json")
	hc := http.Client{Timeout: 30 * time.Second}
	if c.HTTPClient != nil {
		hc = *c.HTTPClient
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err // drop the URL echo; keep the transport cause
		}
		return connectsecureToken{}, configErr(fmt.Errorf("%w: could not reach %s: %v; check CONNECTSECURE_BASE_URL (Profile > API Documentation)", errConnectsecureUnreachable, cfg.BaseURL, err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	// Hand-wired (handfixes.json: error-body-redacts-secrets): an error that
	// echoes the reversible Client-Auth-Token or the client secret is withheld.
	if cliutil.ConnectsecureEchoesSecret(string(body), []string{cat, cfg.ClientSecret}) {
		body = []byte(`{"status":false,"message":"[response withheld: it echoed the client credential]"}`)
	}
	return connectsecureMintResult(resp.StatusCode, body, cfg.BaseURL)
}

const connectsecureCredHint = "check CONNECTSECURE_TENANT (portal login tenant), CONNECTSECURE_CLIENT_ID and CONNECTSECURE_CLIENT_SECRET (Settings > Users > API Key)"

// connectsecureMintResult maps the /w/authorize response to a token or a typed error.
func connectsecureMintResult(status int, body []byte, base string) (connectsecureToken, error) {
	switch {
	case status >= 300 && status < 400:
		return connectsecureToken{}, configErr(fmt.Errorf("ConnectSecure /w/authorize at %s answered with a redirect (HTTP %d); the token request is never followed across redirects: set CONNECTSECURE_BASE_URL to the pod API host itself", base, status))
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return connectsecureToken{}, authErr(fmt.Errorf("ConnectSecure rejected the credentials (HTTP %d); %s", status, connectsecureCredHint))
	case status == http.StatusNotFound:
		return connectsecureToken{}, configErr(fmt.Errorf("ConnectSecure /w/authorize not found at %s (HTTP 404); CONNECTSECURE_BASE_URL must be the pod API host", base))
	case status < 200 || status >= 300:
		return connectsecureToken{}, apiErr(fmt.Errorf("ConnectSecure token mint failed with HTTP %d: %s", status, cliutil.SanitizeErrorBody(string(bytes.TrimSpace(body)))))
	}
	tok, err := parseConnectsecureAuthorize(body)
	if err != nil {
		return connectsecureToken{}, authErr(fmt.Errorf("%w; %s", err, connectsecureCredHint))
	}
	return tok, nil
}

func parseConnectsecureAuthorize(raw []byte) (connectsecureToken, error) {
	var body struct {
		Status  *bool  `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AccessToken string          `json:"access_token"`
			UserID      json.RawMessage `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return connectsecureToken{}, fmt.Errorf("ConnectSecure /w/authorize returned non-JSON: %w", err)
	}
	if body.Status != nil && !*body.Status {
		msg := body.Message
		if msg == "" {
			msg = "status false"
		}
		return connectsecureToken{}, fmt.Errorf("ConnectSecure rejected the credentials: %s", msg)
	}
	userID := strings.Trim(strings.TrimSpace(string(body.Data.UserID)), `"`)
	if body.Data.AccessToken == "" || userID == "" || userID == "null" {
		return connectsecureToken{}, fmt.Errorf("ConnectSecure /w/authorize did not return data.access_token and data.user_id")
	}
	return connectsecureToken{
		AccessToken: body.Data.AccessToken,
		UserID:      userID,
		ExpiresAt:   connectsecureTokenExpiry(body.Data.AccessToken, time.Now()),
	}, nil
}

// connectsecureJWTExp returns a positive JWT exp claim, using the generated
// parser that auth status and doctor also use.
func connectsecureJWTExp(jwt string) (time.Time, bool) {
	exp, ok := jwtExpiry(jwt)
	return exp, ok && exp.Unix() > 0
}

// connectsecureJWTExpired: true only for a JWT whose exp has passed; opaque
// tokens are treated as live because the CLI cannot judge them.
func connectsecureJWTExpired(jwt string, now time.Time) bool {
	exp, ok := connectsecureJWTExp(jwt)
	return ok && !now.Before(exp)
}

// connectsecureTokenExpiry reads the JWT exp claim (minus a minute of skew) and
// caps it at connectsecureTokenCacheMax from now.
func connectsecureTokenExpiry(jwt string, now time.Time) time.Time {
	limit := now.Add(connectsecureTokenCacheMax)
	exp, ok := connectsecureJWTExp(jwt)
	if !ok {
		return limit
	}
	if exp = exp.Add(-time.Minute); exp.Before(limit) {
		return exp
	}
	return limit
}
