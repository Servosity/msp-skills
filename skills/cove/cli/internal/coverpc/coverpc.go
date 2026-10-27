// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
	"strconv"
	"strings"
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
	// ErrDataSecurity ("Operation failed because of security reasons") is
	// returned when a visa is presented but the method is refused — typically
	// the API User's role does not permit it.
	ErrDataSecurity = 13501
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
	// codesWithheld is set when Code or Data repeats a credential this client
	// holds: the numbers are server text too, so Error and DataText leave them
	// out (Data still drives the hint and IsVisaError).
	codesWithheld bool
}

// DataText is the vendor error number as it may be shown.
func (e *RPCError) DataText() string {
	if e.codesWithheld {
		return "number withheld"
	}
	return strconv.Itoa(e.Data)
}

func (e *RPCError) Error() string {
	hint := ""
	switch e.Data {
	case ErrDataVisa:
		hint = " (session expired or missing — run `cove-cli auth login`)"
	case ErrDataBadCredentials:
		hint = " (for an API User set COVE_PARTNER to the full customer/partner string shown in the Cove console customer dropdown — usually including the account email in parentheses, e.g. \"Acme Corp (admin@example.com)\", not just the company name — COVE_USERNAME to its login name, and COVE_PASSWORD to its API token; Cove returns this same error for a wrong COVE_PARTNER format as for a bad password, and an empty COVE_PARTNER is the usual cause)"
	case ErrDataSecurity:
		hint = " (the visa was accepted but this method was refused for security reasons — the API User's role may not permit it; verify the API User's role in the Cove console, or run the method through `cove-cli call <Method>`, which manages the session visa automatically)"
	}
	if e.codesWithheld {
		return fmt.Sprintf("cove api error (numbers withheld) on %s: %s%s", e.Method, e.Message, hint)
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
	// Username, Partner and Endpoint bind the visa to the login and the API
	// endpoint it was issued for: a cache whose binding differs from the
	// current credentials (or that predates the binding) is never reused, so
	// another tenant's session cannot answer for this one.
	Username string `json:"username,omitempty"`
	Partner  string `json:"partner,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
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
	// refuseCrossOriginRedirect (redirect.go): every call's body carries the
	// password or the session visa, so no hop may leave the configured origin.
	c := &Client{
		BaseURL:     baseURL,
		HTTP:        &http.Client{Timeout: 60 * time.Second, CheckRedirect: refuseCrossOriginRedirect},
		Creds:       CredentialsFromEnv(),
		SessionPath: DefaultSessionPath(),
		limiter:     cliutil.NewAdaptiveLimiter(4),
	}
	// The process-wide error backstops (CLI and MCP) withhold the API token
	// from the moment a client exists.
	cliutil.FunnelHold(c.Creds.Password)
	return c
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
		time.Since(s.SavedAt) < sessionMaxAge && c.sessionBound(s) {
		c.setVisa(s.Visa)
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
	c.setVisa(visa)
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
		c.setVisa(newVisa)
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
	// Every request carries the password (Login) or the visa: refuse plaintext
	// HTTP before anything is dialled.
	if err := refusePlaintextOrigin(c.BaseURL); err != nil {
		return nil, "", err
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, "", err
	}
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
		if w := c.withhold(err.Error(), c.heldSecrets(visa)); cliutil.FunnelWithheld(w) {
			return nil, "", fmt.Errorf("calling %s: %s", method, w)
		}
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
		return nil, "", fmt.Errorf("calling %s: HTTP %d: %s", method, resp.StatusCode, c.shownBody(body, visa))
	}
	var env responseEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		// The decoder's message quotes server text (a number it could not
		// store), so it passes the funnel too.
		err = c.shownErr(err, c.heldSecrets(append([]string{visa}, responseVisas(body)...)...))
		return nil, "", fmt.Errorf("parsing %s response: %w: %s", method, err, c.shownBody(body, visa))
	}
	// Every credential this answer could repeat: the token, the visa this
	// client holds, the visa it sent, and every visa the answer returned.
	held := c.heldSecrets(append([]string{visa, env.Visa}, responseVisas(body)...)...)
	if env.Error != nil {
		env.Error.Method = method
		// The message is the only server text an RPCError shows.
		if w := c.withhold(env.Error.Message, held); cliutil.FunnelWithheld(w) {
			env.Error.Message = w
		}
		for _, n := range []int{env.Error.Code, env.Error.Data} {
			if cliutil.FunnelWithheld(c.withhold(strconv.Itoa(n), held)) {
				env.Error.codesWithheld = true
			}
		}
		return nil, env.Visa, env.Error
	}
	// Success data is server-controlled too: a result that repeats a held
	// credential is refused before any command prints or stores it.
	if cliutil.FunnelJSONEchoes(env.Result, held) {
		return nil, env.Visa, fmt.Errorf("the %s answer repeated a credential this program holds, so it was withheld", method)
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
	// UseNumber keeps every number exact (ColumnText renders it); a float64
	// would round ids past 2^53 and print large values in e-notation.
	dec := json.NewDecoder(bytes.NewReader(inner))
	dec.UseNumber()
	rows := make([]map[string]any, 0)
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("decoding enumeration rows: %w", err)
	}
	return rows, nil
}

// FlattenSettings converts an EnumerateAccountStatistics "Settings" value —
// a list of single-key objects like [{"I1":"name"},{"I14":"123"}] — into one
// flat map, every value rendered by ColumnText (the one shared parser): a
// multi-valued column keeps every value, joined by ",", and a number keeps its
// exact decimal form. A null value is left out (not reported).
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
			if text, ok := ColumnText(v); ok {
				flat[k] = text
			}
		}
	}
	return flat
}

// CachedVisa is the visa in the default session cache ("" when there is
// none), whatever login or endpoint it is bound to: for a process that must
// withhold a visa it never logged in with (the MCP server's error backstop,
// whose CLI children write that cache).
func CachedVisa() string {
	s, err := (&Client{SessionPath: DefaultSessionPath()}).loadSession()
	if err != nil {
		return ""
	}
	return s.Visa
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
	data, err := json.Marshal(session{Visa: c.visa, SavedAt: time.Now().UTC(), Username: c.Creds.Username, Partner: c.Creds.Partner, Endpoint: c.Endpoint(), PartnerID: c.partnerID})
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
	if err != nil || s.Visa == "" || !c.sessionBound(s) {
		return 0, false
	}
	return time.Since(s.SavedAt), true
}

// heldSecrets is every credential this client holds or sent with the request:
// the API token (Password) and the session visa. Hand-fix: handfixes.json
// secret-echo-decoded-funnel.
func (c *Client) heldSecrets(extra ...string) []string {
	return append([]string{c.Creds.Password, c.visa}, extra...)
}

// shownBody is the text an error shows of the server's answer: the WHOLE
// answer is checked for an echoed credential, in any encoding the shared
// funnel decodes, before the 300-byte cut, and the cut text again as shown. A
// held value too short for the decoded check is masked as written.
func (c *Client) shownBody(body []byte, extra ...string) string {
	held := c.heldSecrets(append(extra, responseVisas(body)...)...)
	if w := c.withhold(string(body), held); cliutil.FunnelWithheld(w) {
		return w
	}
	text := truncate(body, 300)
	for _, v := range held {
		if v = strings.TrimSpace(v); v != "" {
			text = strings.ReplaceAll(text, v, "****")
		}
	}
	return c.withhold(text, held)
}

// shownErr is err, or an error carrying only the funnel's withheld notice
// when err's text repeats one of held.
func (c *Client) shownErr(err error, held []string) error {
	if w := c.withhold(err.Error(), held); cliutil.FunnelWithheld(w) {
		return errors.New(w)
	}
	return err
}

// withhold is the one funnel every piece of server text passes before it is
// shown: cliutil.FunnelWithhold's decoded views plus every short credential
// as written or encoded (cliutil.FunnelWithholdAll).
func (c *Client) withhold(text string, held []string) string {
	return cliutil.FunnelWithholdAll(text, held)
}

// setVisa records a visa this client now holds, for the process-wide error
// backstops too.
func (c *Client) setVisa(visa string) {
	c.visa = visa
	cliutil.FunnelHold(visa)
}

// sessionBound reports whether a cached session was issued for this client's
// login (partner and username) at this client's endpoint. A cache written
// before the binding existed carries none and is never reused.
func (c *Client) sessionBound(s session) bool {
	return s.Endpoint != "" && s.Endpoint == c.Endpoint() &&
		s.Username == c.Creds.Username && s.Partner == c.Creds.Partner
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
