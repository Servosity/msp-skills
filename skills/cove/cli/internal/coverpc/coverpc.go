// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written JSON-RPC client for the Cove Data Protection Management
// Service. The generated REST client cannot carry the session visa, which
// lives inside the JSON-RPC request body, so every authenticated hand-built
// command goes through this package instead.
package coverpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"cove-pp-cli/internal/cliutil"
)

// DefaultBaseURL is the public Cove Management Service origin.
const DefaultBaseURL = "https://api.backup.management"

// rpcPath is the single JSON-RPC endpoint every method POSTs to.
const rpcPath = "/jsonapi"

// Vendor error data codes the client gives special treatment.
const (
	// ErrDataVisa is returned when the visa is missing, expired, or corrupt.
	ErrDataVisa = 1701
	// ErrDataBadCredentials is returned by Login for unknown partner/user/password.
	ErrDataBadCredentials = 2100
)

// Credentials carries the partner-scoped login triple. Username/Password are
// required; Partner is optional (the API accepts an empty partner for users
// whose name is globally unique).
type Credentials struct {
	Partner  string
	Username string
	Password string
}

// CredentialsFromEnv reads COVE_PARTNER, COVE_USERNAME, and COVE_PASSWORD.
func CredentialsFromEnv() Credentials {
	return Credentials{
		Partner:  os.Getenv("COVE_PARTNER"),
		Username: os.Getenv("COVE_USERNAME"),
		Password: os.Getenv("COVE_PASSWORD"),
	}
}

// Present reports whether the minimum login material is set.
func (c Credentials) Present() bool {
	return c.Username != "" && c.Password != ""
}

// RPCError is a JSON-RPC error envelope from the Cove API. Code is the
// JSON-RPC error code (usually -32603); Data is the vendor-specific error
// number (1701 visa, 2100 bad credentials, ...).
type RPCError struct {
	Code    int    `json:"code"`
	Data    int    `json:"data"`
	Message string `json:"message"`
	Method  string `json:"-"`
}

func (e *RPCError) Error() string {
	hint := ""
	switch e.Data {
	case ErrDataVisa:
		hint = " (session expired or missing — run `cove-cli auth login`)"
	case ErrDataBadCredentials:
		hint = " (for an API User set COVE_PARTNER to the customer name it was created for, COVE_USERNAME to its login name, and COVE_PASSWORD to its API token; an empty COVE_PARTNER is the usual cause)"
	}
	return fmt.Sprintf("cove api error %d (data %d) on %s: %s%s", e.Code, e.Data, e.Method, e.Message, hint)
}

// IsVisaError reports whether err is an RPCError carrying the expired/missing
// visa data code.
func IsVisaError(err error) bool {
	var rpcErr *RPCError
	return errors.As(err, &rpcErr) && rpcErr.Data == ErrDataVisa
}

// envelope is the JSON-RPC 2.0 request shape Cove expects. Visa rides inside
// the body — not a header — which is why the generated REST client cannot
// authenticate these calls.
type envelope struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Visa    string `json:"visa,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type responseEnvelope struct {
	Visa   string          `json:"visa"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

// session is the on-disk cached visa.
type session struct {
	Visa    string    `json:"visa"`
	SavedAt time.Time `json:"saved_at"`
	// Username records which login the visa belongs to so a credential swap
	// invalidates the cache instead of reusing another account's session.
	Username string `json:"username,omitempty"`
	// PartnerID is the logged-in user's root partner, cached so commands can
	// default their query scope without an extra login round-trip.
	PartnerID int64 `json:"partner_id,omitempty"`
}

// sessionMaxAge is how long a cached visa is trusted before forcing a fresh
// login. Cove rotates the visa on every response and expires idle sessions
// quickly, so this is deliberately short; an expired visa is also healed at
// call time via the 1701 retry path.
const sessionMaxAge = 15 * time.Minute

// Client is a minimal JSON-RPC 2.0 client with visa lifecycle handling:
// cached visa → login on demand → single retry after a 1701 visa error →
// visa rotation captured from every response.
type Client struct {
	BaseURL     string
	HTTP        *http.Client
	Creds       Credentials
	SessionPath string

	limiter   *cliutil.AdaptiveLimiter
	visa      string
	partnerID int64
}

// New builds a client for baseURL (empty selects DefaultBaseURL, with
// COVE_BASE_URL honored for verify/mock runs) using env credentials and the
// default session cache path.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = os.Getenv("COVE_BASE_URL")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:     baseURL,
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		Creds:       CredentialsFromEnv(),
		SessionPath: DefaultSessionPath(),
		limiter:     cliutil.NewAdaptiveLimiter(4),
	}
}

// DefaultSessionPath returns ~/.config/cove-cli/session.json.
func DefaultSessionPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "cove-cli", "session.json")
}

// Visa returns a usable session token, loading the disk cache or logging in
// when needed. The returned visa may be served stale; callers going through
// Call get the 1701 retry for free.
func (c *Client) Visa(ctx context.Context) (string, error) {
	if c.visa != "" {
		return c.visa, nil
	}
	if s, err := c.loadSession(); err == nil && s.Visa != "" &&
		time.Since(s.SavedAt) < sessionMaxAge && (s.Username == "" || s.Username == c.Creds.Username) {
		c.visa = s.Visa
		if c.partnerID == 0 {
			c.partnerID = s.PartnerID
		}
		return c.visa, nil
	}
	if _, err := c.Login(ctx); err != nil {
		return "", err
	}
	return c.visa, nil
}

// Login performs the JSON-RPC Login method with the configured credentials,
// caches the returned visa, and returns the raw result (UserInfo under the
// inner "result" key).
func (c *Client) Login(ctx context.Context) (json.RawMessage, error) {
	if !c.Creds.Present() {
		return nil, fmt.Errorf("missing credentials: set COVE_USERNAME (API user login name), COVE_PASSWORD (API token), and COVE_PARTNER (the customer the API user was created for), then run `cove-cli auth login`")
	}
	params := map[string]string{
		"partner":  c.Creds.Partner,
		"username": c.Creds.Username,
		"password": c.Creds.Password,
	}
	result, visa, err := c.do(ctx, "Login", params, "")
	if err != nil {
		return nil, err
	}
	if visa == "" {
		return nil, fmt.Errorf("login succeeded but the response carried no visa — cannot establish a session")
	}
	c.visa = visa
	if inner, ierr := InnerResult(result); ierr == nil {
		var info struct {
			PartnerID int64 `json:"PartnerId"`
		}
		if json.Unmarshal(inner, &info) == nil && info.PartnerID > 0 {
			c.partnerID = info.PartnerID
		}
	}
	c.saveSession()
	return result, nil
}

// RootPartnerID returns the logged-in user's partner id, logging in when the
// cached session does not carry one.
func (c *Client) RootPartnerID(ctx context.Context) (int64, error) {
	if _, err := c.Visa(ctx); err != nil {
		return 0, err
	}
	if c.partnerID > 0 {
		return c.partnerID, nil
	}
	if _, err := c.Login(ctx); err != nil {
		return 0, err
	}
	if c.partnerID == 0 {
		return 0, fmt.Errorf("login response did not include a partner id; pass --partner-id explicitly")
	}
	return c.partnerID, nil
}

// Call invokes a JSON-RPC method with the session visa injected, healing an
// expired session once via re-login. It returns the raw JSON-RPC "result"
// member (method return value under its inner "result" key, OUT params as
// siblings).
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	visa, err := c.Visa(ctx)
	if err != nil {
		return nil, err
	}
	result, newVisa, err := c.do(ctx, method, params, visa)
	if IsVisaError(err) && c.Creds.Present() {
		// Session died between cache and call — login once and retry.
		c.visa = ""
		if _, lerr := c.Login(ctx); lerr != nil {
			return nil, lerr
		}
		result, newVisa, err = c.do(ctx, method, params, c.visa)
	}
	if err != nil {
		return nil, err
	}
	if newVisa != "" && newVisa != c.visa {
		// Cove rotates the visa on every response; keep the freshest.
		c.visa = newVisa
		c.saveSession()
	}
	return result, nil
}

// CallAnonymous invokes a method without injecting a visa. Useful for probes
// and for methods the operator explicitly wants to run visa-less.
func (c *Client) CallAnonymous(ctx context.Context, method string, params any) (json.RawMessage, error) {
	result, _, err := c.do(ctx, method, params, "")
	return result, err
}

func (c *Client) do(ctx context.Context, method string, params any, visa string) (json.RawMessage, string, error) {
	c.limiter.Wait()
	reqBody, err := json.Marshal(envelope{
		JSONRPC: "2.0",
		ID:      "jsonrpc",
		Visa:    visa,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, "", fmt.Errorf("encoding %s request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+rpcPath, bytes.NewReader(reqBody))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("calling %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		c.limiter.OnRateLimit()
		return nil, "", &cliutil.RateLimitError{URL: c.BaseURL + rpcPath, Body: fmt.Sprintf("JSON-RPC method %s", method)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", fmt.Errorf("reading %s response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("calling %s: HTTP %d: %s", method, resp.StatusCode, truncate(body, 300))
	}
	var env responseEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", fmt.Errorf("parsing %s response: %w: %s", method, err, truncate(body, 300))
	}
	if env.Error != nil {
		env.Error.Method = method
		return nil, env.Visa, env.Error
	}
	c.limiter.OnSuccess()
	return env.Result, env.Visa, nil
}

// InnerResult unwraps the JSON-RPC result member's inner "result" key — the
// method's return value. Cove nests it so OUT params can sit alongside.
func InnerResult(result json.RawMessage) (json.RawMessage, error) {
	if len(result) == 0 {
		return nil, fmt.Errorf("empty result")
	}
	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(result, &wrapper); err != nil {
		return nil, fmt.Errorf("unwrapping result: %w", err)
	}
	if len(wrapper.Result) == 0 {
		// Some methods (void returns) legitimately omit the inner key.
		return result, nil
	}
	return wrapper.Result, nil
}

// Rows decodes an enumeration result into a slice of generic objects.
func Rows(result json.RawMessage) ([]map[string]any, error) {
	inner, err := InnerResult(result)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0)
	if err := json.Unmarshal(inner, &rows); err != nil {
		return nil, fmt.Errorf("decoding enumeration rows: %w", err)
	}
	return rows, nil
}

// FlattenSettings converts an EnumerateAccountStatistics "Settings" value —
// a list of single-key objects like [{"I1":"name"},{"I14":"123"}] — into one
// flat map. Values arrive as strings or arrays of strings; arrays keep their
// first element (Cove uses arrays for multi-valued columns).
func FlattenSettings(settings any) map[string]string {
	flat := map[string]string{}
	list, ok := settings.([]any)
	if !ok {
		return flat
	}
	for _, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for k, v := range obj {
			switch val := v.(type) {
			case string:
				flat[k] = val
			case float64:
				flat[k] = fmt.Sprintf("%v", val)
			case []any:
				if len(val) > 0 {
					flat[k] = fmt.Sprintf("%v", val[0])
				}
			default:
				flat[k] = fmt.Sprintf("%v", val)
			}
		}
	}
	return flat
}

func (c *Client) loadSession() (session, error) {
	var s session
	if c.SessionPath == "" {
		return s, errors.New("no session path")
	}
	data, err := os.ReadFile(c.SessionPath)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func (c *Client) saveSession() {
	if c.SessionPath == "" || c.visa == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.SessionPath), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(session{Visa: c.visa, SavedAt: time.Now().UTC(), Username: c.Creds.Username, PartnerID: c.partnerID})
	if err != nil {
		return
	}
	_ = os.WriteFile(c.SessionPath, data, 0o600)
}

// ClearSession removes the cached visa from memory and disk.
func (c *Client) ClearSession() error {
	c.visa = ""
	if c.SessionPath == "" {
		return nil
	}
	err := os.Remove(c.SessionPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// SessionAge returns how old the cached visa is, or false when no session is
// cached on disk.
func (c *Client) SessionAge() (time.Duration, bool) {
	s, err := c.loadSession()
	if err != nil || s.Visa == "" {
		return 0, false
	}
	return time.Since(s.SavedAt), true
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
