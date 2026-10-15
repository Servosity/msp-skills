// Package fgtcloud is a read-only FortiGate Cloud API client.
//
// Public sources: FortiGate Cloud Administration Guide "API access" and the
// FortiCloud IAM guide "Accessing FortiAPIs" (docs.fortinet.com). Tokens come
// from POST https://customerapiauth.fortinet.com/api/v1/oauth/token/ with an
// IAM API user (password grant, client_id "fortigatecloud"); refresh tokens
// are single-use. Repeated failed password grants lock the FortiCloud account
// for 15-30 minutes, so this client caches the token on disk and performs at
// most one password grant per process. Hosts are per region:
// {api,usapi,euapi,jpapi}.fortigate.forticloud.com/forticloudapi/v1.
package fgtcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fortinet-pp-cli/internal/cliutil"
)

// DefaultTokenURL is the FortiCloud IAM token endpoint (trailing slash required).
const DefaultTokenURL = "https://customerapiauth.fortinet.com/api/v1/oauth/token/" // #nosec G101 -- public OAuth token endpoint URL, not a credential

var regionHosts = map[string]string{
	"": "api", "global": "api", "us": "usapi", "eu": "euapi", "jp": "jpapi",
}

// BaseURL returns the API root for a region (global, us, eu, jp).
func BaseURL(region string) (string, error) {
	h, ok := regionHosts[strings.ToLower(strings.TrimSpace(region))]
	if !ok {
		return "", fmt.Errorf("unknown FortiGate Cloud region %q (want global, us, eu or jp)", region)
	}
	return "https://" + h + ".fortigate.forticloud.com/forticloudapi/v1", nil
}

// Client is a FortiGate Cloud API client for one IAM API user.
type Client struct {
	BaseURL   string
	TokenURL  string
	User      string
	Password  string
	AccountID string
	CachePath string
	HTTP      *http.Client

	// CacheWarning is set when the token cache could not be written; the
	// caller should surface it (a stale cache could replay a used refresh token).
	CacheWarning string

	limiter *cliutil.AdaptiveLimiter
	mu      sync.Mutex
	tok     *cachedToken
}

// passwordGrants records, per IAM user, that this process already ran a
// password grant. It is process-wide so a second Client cannot bypass the
// account-lockout guard.
var passwordGrants = struct {
	sync.Mutex
	used map[string]bool
}{used: map[string]bool{}}

func passwordGrantUsed(user string) bool {
	passwordGrants.Lock()
	defer passwordGrants.Unlock()
	return passwordGrants.used[user]
}

// takePasswordGrant reserves this process's single password grant for user.
func takePasswordGrant(user string) bool {
	passwordGrants.Lock()
	defer passwordGrants.Unlock()
	if passwordGrants.used[user] {
		return false
	}
	passwordGrants.used[user] = true
	return true
}

// refuseRedirect stops redirects: a 307/308 would replay the password-grant
// body to another origin, and any redirect could move the bearer token.
func refuseRedirect(req *http.Request, _ []*http.Request) error {
	return fmt.Errorf("FortiCloud redirected to %s; refusing to follow", req.URL.Redacted())
}

type cachedToken struct {
	User         string    `json:"user"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// New builds a client. cachePath is where the access token is cached (0600).
func New(baseURL, user, password, accountID, cachePath string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(user) == "" || strings.TrimSpace(password) == "" {
		return nil, errors.New("FortiGate Cloud needs FORTINET_CLOUD_API_USER and FORTINET_CLOUD_API_PASSWORD (a FortiCloud IAM API user)")
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), TokenURL: DefaultTokenURL,
		User: strings.TrimSpace(user), Password: password, AccountID: strings.TrimSpace(accountID), CachePath: cachePath,
		HTTP: &http.Client{Timeout: timeout, CheckRedirect: refuseRedirect}, limiter: cliutil.NewAdaptiveLimiter(5),
	}, nil
}

func (c *Client) loadCache() *cachedToken {
	if c.CachePath == "" {
		return nil
	}
	// A cache other users can read (restored from a backup, copied by hand)
	// or that is not a plain file is ignored; the next save replaces it with
	// a fresh 0600 file. Windows reports 0666 for every file, so the mode
	// check is Unix-only.
	info, err := os.Lstat(c.CachePath)
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return nil
	}
	b, err := os.ReadFile(c.CachePath)
	if err != nil {
		return nil
	}
	var t cachedToken
	if json.Unmarshal(b, &t) != nil || t.User != c.User || t.AccessToken == "" {
		return nil
	}
	return &t
}

// saveCache writes the token through a fresh 0600 temp file and renames it
// into place, so it never inherits loose permissions or follows a symlink.
// On failure the old cache is removed: it may hold a refresh token that was
// just consumed.
func (c *Client) saveCache(t *cachedToken) {
	if c.CachePath == "" {
		return
	}
	if err := writeCache(c.CachePath, t); err != nil {
		_ = os.Remove(c.CachePath)
		c.CacheWarning = "FortiGate Cloud token cache not saved (" + err.Error() + "); the next run will log in again"
	}
}

func writeCache(path string, t *cachedToken) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(t) // #nosec G117 -- persisting the token is the cache's purpose; the file is 0600 (temp file + chmod + rename)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".fortigate-cloud-token-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Client) grant(ctx context.Context, body map[string]string) (*cachedToken, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FortiCloud token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusOK || out.AccessToken == "" {
		msg := out.Error
		if out.Description != "" {
			msg += ": " + out.Description
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, &TokenError{Message: maskSecrets(msg, body["password"], body["refresh_token"])}
	}
	exp := out.ExpiresIn
	if exp <= 0 {
		exp = 3600
	}
	// Renew a minute early so a token never expires mid-request.
	return &cachedToken{User: c.User, AccessToken: out.AccessToken, RefreshToken: out.RefreshToken,
		ExpiresAt: time.Now().Add(time.Duration(exp)*time.Second - time.Minute)}, nil
}

// token returns a valid access token: cache, then one refresh grant, then at
// most one password grant per process (account-lockout protection).
func (c *Client) token(ctx context.Context, forceNew bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !forceNew {
		if c.tok == nil {
			c.tok = c.loadCache()
		}
		if c.tok != nil && time.Now().Before(c.tok.ExpiresAt) {
			return c.tok.AccessToken, nil
		}
		if c.tok != nil && c.tok.RefreshToken != "" {
			t, err := c.grant(ctx, map[string]string{"refresh_token": c.tok.RefreshToken, "client_id": "fortigatecloud", "grant_type": "refresh_token"})
			if err == nil {
				c.tok = t
				c.saveCache(t)
				return t.AccessToken, nil
			}
		}
	}
	if !takePasswordGrant(c.User) {
		return "", errors.New("FortiCloud rejected the token and a password login already ran in this invocation; not retrying (repeated failed logins lock the account for 15-30 minutes)")
	}
	t, err := c.grant(ctx, map[string]string{"username": c.User, "password": c.Password, "client_id": "fortigatecloud", "grant_type": "password"})
	if err != nil {
		return "", err
	}
	c.tok = t
	c.saveCache(t)
	return t.AccessToken, nil
}

// TokenError is a rejected FortiCloud IAM login or refresh.
type TokenError struct{ Message string }

func (e *TokenError) Error() string { return "FortiCloud token request failed (" + e.Message + ")" }

// IsAuth reports whether err is a rejected credential (token grant failure,
// or an API 401/403).
func IsAuth(err error) bool {
	var te *TokenError
	var he *HTTPError
	return errors.As(err, &te) || (errors.As(err, &he) && (he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden))
}

// maskSecrets removes credential values from response-derived text, in case
// a server echoes them back in an error body.
func maskSecrets(text string, secrets ...string) string {
	for _, s := range secrets {
		if len(s) >= 4 {
			text = strings.ReplaceAll(text, s, "****")
		}
	}
	return text
}

func (c *Client) refreshToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok == nil {
		return ""
	}
	return c.tok.RefreshToken
}

// HTTPError is a non-2xx API answer.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("FortiGate Cloud API returned HTTP %d", e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	if e.Status == http.StatusUnauthorized {
		msg += " (the IAM API user needs FortiGate Cloud portal permission; with FortiCloud Organizations set FORTINET_CLOUD_ACCOUNT_ID)"
	}
	return msg
}

// Get calls GET <BaseURL><path>. A 401 with a cached token triggers one
// fresh login and one retry.
func (c *Client) Get(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	if q == nil {
		q = url.Values{}
	}
	if c.AccountID != "" && q.Get("account_id") == "" {
		q.Set("account_id", c.AccountID)
	}
	target := c.BaseURL + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := c.token(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("FortiGate Cloud request: %w", err)
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if rerr != nil {
			return nil, rerr
		}
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			c.limiter.OnRateLimit()
			return nil, &cliutil.RateLimitError{URL: target, RetryAfter: cliutil.RetryAfter(resp)}
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0 && !passwordGrantUsed(c.User):
			continue
		case resp.StatusCode >= 300:
			// Mask the whole body first so a secret crossing the cut is not half-shown.
			body := maskSecrets(strings.TrimSpace(string(raw)), tok, c.Password, c.refreshToken())
			if len(body) > 300 {
				body = body[:300]
			}
			return nil, &HTTPError{Status: resp.StatusCode, Body: body}
		}
		c.limiter.OnSuccess()
		return raw, nil
	}
	return nil, &HTTPError{Status: http.StatusUnauthorized}
}

// ListItems accepts a bare array or an object wrapping one under
// devices/data/results/items.
func ListItems(raw json.RawMessage) ([]map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unexpected FortiGate Cloud response: %w", err)
	}
	for _, k := range []string{"devices", "data", "results", "items"} {
		if v, ok := obj[k]; ok {
			if err := json.Unmarshal(v, &arr); err == nil {
				return arr, nil
			}
		}
	}
	return nil, errors.New("unexpected FortiGate Cloud response: no device array")
}

// Devices pages GET /devices with limit/offset until a short page or maxPages.
func (c *Client) Devices(ctx context.Context, pageSize, maxPages int) ([]map[string]any, bool, error) {
	out := []map[string]any{}
	for page := 0; page < maxPages; page++ {
		q := url.Values{"limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(page * pageSize)}}
		raw, err := c.Get(ctx, "/devices", q)
		if err != nil {
			return out, false, err
		}
		items, err := ListItems(raw)
		if err != nil {
			return out, false, err
		}
		out = append(out, items...)
		if len(items) < pageSize {
			return out, false, nil
		}
	}
	return out, true, nil
}
