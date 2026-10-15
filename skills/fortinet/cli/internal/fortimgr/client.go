// Package fortimgr is a read-only FortiManager JSON-RPC client.
//
// Wire format (public sources: Fortinet's ansible-galaxy-fortimanager-collection,
// pyFMG, the Fortinet "How-to FortiManager API" guide): every call is
// POST https://<fmg>/jsonrpc with {"id","method","params":[{"url",...}],"verbose":1};
// success is result[i].status.code == 0 (the HTTP status says nothing).
// Auth is an API key sent as "Authorization: Bearer <key>" (FortiManager
// 7.2.2+), so there is no login, logout or session.
//
// Only the "get" method is ever sent: this client cannot change a
// FortiManager, by construction.
package fortimgr

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"fortinet-pp-cli/internal/cliutil"
)

// Client talks to one FortiManager.
type Client struct {
	endpoint string
	apiKey   string
	http     *http.Client
	limiter  *cliutil.AdaptiveLimiter
	id       atomic.Int64
}

// StatusError is a non-zero JSON-RPC status.
type StatusError struct {
	URL     string
	Code    int
	Message string
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("FortiManager %s: status %d %s", e.URL, e.Code, e.Message)
	if e.Code == -11 {
		msg += " (no permission: check the API key, its admin profile, its ADOM scope and the trusted hosts)"
	}
	return msg
}

// AuthError is an HTTP 401/403 from FortiManager.
type AuthError struct{ Status int }

func (e *AuthError) Error() string {
	return fmt.Sprintf("FortiManager returned HTTP %d: check FORTINET_FMG_API_KEY and that this host is a trusted host of the API admin", e.Status)
}

// IsAuth reports whether err is a rejected credential (HTTP 401/403 or
// JSON-RPC -11, which FortiManager uses for both no-permission and a bad key).
func IsAuth(err error) bool {
	var ae *AuthError
	var se *StatusError
	return errors.As(err, &ae) || (errors.As(err, &se) && se.Code == -11)
}

// New validates baseURL (https only) and builds a client.
func New(baseURL, apiKey string, insecure bool, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid FortiManager URL (want https://<fortimanager>[:port])")
	}
	if u.User != nil {
		return nil, fmt.Errorf("FortiManager URL must not embed a user or password; use FORTINET_FMG_API_KEY")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("FortiManager URL %s must use https; the API key would otherwise travel in clear text", u.Redacted())
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("FortiManager API key is empty")
	}
	u.Path, u.RawQuery, u.Fragment = "/jsonrpc", "", ""
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		// #nosec G402 -- explicit --insecure / FORTINET_FMG_INSECURE opt-in for self-signed FortiManager certs
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit --insecure opt-in for self-signed FortiManager certs
	}
	return &Client{
		endpoint: u.String(),
		apiKey:   strings.TrimSpace(apiKey),
		http:     &http.Client{Timeout: timeout, Transport: tr, CheckRedirect: refuseRedirect},
		limiter:  cliutil.NewAdaptiveLimiter(5),
	}, nil
}

// refuseRedirect stops the client from following redirects: the JSON-RPC
// endpoint never redirects, and following one could carry the API key to
// another origin or downgrade to http.
func refuseRedirect(req *http.Request, _ []*http.Request) error {
	return fmt.Errorf("FortiManager redirected to %s; refusing to follow (check FORTINET_FMG_URL)", req.URL.Redacted())
}

type request struct {
	ID      int64            `json:"id"`
	Method  string           `json:"method"`
	Params  []map[string]any `json:"params"`
	Verbose int              `json:"verbose"`
}

type result struct {
	Status *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	URL  string          `json:"url"`
	Data json.RawMessage `json:"data"`
}

// Get runs one JSON-RPC "get" on url (e.g. /dvmdb/adom) and returns data.
// extra carries documented params such as fields, filter, option, loadsub.
func (c *Client) Get(ctx context.Context, rpcURL string, extra map[string]any) (json.RawMessage, error) {
	param := map[string]any{"url": rpcURL}
	for k, v := range extra {
		param[k] = v
	}
	body, err := json.Marshal(request{ID: c.id.Add(1), Method: "get", Params: []map[string]any{param}, Verbose: 1})
	if err != nil {
		return nil, err
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FortiManager request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		c.limiter.OnRateLimit()
		return nil, &cliutil.RateLimitError{URL: c.endpoint, RetryAfter: cliutil.RetryAfter(resp)}
	}
	c.limiter.OnSuccess()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &AuthError{Status: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("FortiManager returned HTTP %d", resp.StatusCode)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("FortiManager response is not JSON-RPC: %w", err)
	}
	// result is a list for single-param calls, but some builds return an object.
	var results []result
	if err := json.Unmarshal(env.Result, &results); err != nil {
		var one result
		if err2 := json.Unmarshal(env.Result, &one); err2 != nil {
			return nil, fmt.Errorf("FortiManager result has an unexpected shape: %w", err)
		}
		results = []result{one}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("FortiManager returned an empty result for %s", rpcURL)
	}
	r := results[0]
	if r.Status == nil {
		return nil, fmt.Errorf("FortiManager result for %s has no status", rpcURL)
	}
	if r.Status.Code != 0 {
		return nil, &StatusError{URL: rpcURL, Code: r.Status.Code, Message: r.Status.Message}
	}
	if len(r.Data) == 0 {
		return json.RawMessage("null"), nil
	}
	return r.Data, nil
}

// DeviceFields are the read fields requested for managed devices. Secrets
// such as adm_pass are never requested.
var DeviceFields = []string{"name", "hostname", "sn", "platform_str", "os_ver", "mr", "patch", "build",
	"conn_status", "conf_status", "db_status", "dev_status", "ha_mode", "ip", "desc", "last_checked"}

// Firmware renders os_ver/mr/patch/build (os_ver is "7.0" with verbose:1,
// or a bare major number) as "7.4.3 build2573".
func Firmware(d map[string]any) string {
	major := strings.TrimSpace(fmt.Sprint(d["os_ver"]))
	if i := strings.Index(major, "."); i >= 0 {
		major = major[:i]
	}
	if major == "" || major == "<nil>" || major == "unknown" {
		return ""
	}
	out := major + "." + num(d["mr"])
	// patch is -1 for model (not yet provisioned) devices.
	if p, ok := d["patch"].(float64); !ok || p >= 0 {
		out += "." + num(d["patch"])
	}
	if b := num(d["build"]); b != "" && b != "0" {
		out += " build" + b
	}
	return out
}

func num(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%d", int64(x))
	case nil:
		return "0"
	default:
		return fmt.Sprint(x)
	}
}
