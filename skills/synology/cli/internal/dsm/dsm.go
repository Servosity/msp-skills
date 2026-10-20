// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Not emitted by CLI Printing Press; survives regeneration.
//
// Package dsm holds the Synology DSM wire dialect the generic session-handshake
// transport cannot express on its own:
//
//   - Login is a credential exchange (SYNO.API.Auth login, version 6 - the
//     version Synology's DSM Login Web API Guide recommends - on entry.cgi with
//     format=sid, enable_syno_token, enable_device_token, otp_code, device_id),
//     sent as a form POST so the password never lands in a URL, a log line or a
//     proxy access log. The guide allows GET or POST; N4S4/synology-api,
//     laupernoe/dumbmonit and the ABB Prometheus exporter all log in by POST.
//   - Failures arrive as HTTP 200 {"success":false,"error":{"code":N}}; success
//     wraps the payload as {"success":true,"data":{...}}.
//   - Codes 106/107/119/150 mean the session id is no longer usable: relogin once.
//   - DSM 7.3.2+ rejects calls without the X-SYNO-TOKEN header.
//
// Calls are addressed by the same /webapi/entry.cgi?api=...&method=...&version=...
// paths spec.yaml declares, so every request traces to a spec operation.
//
// The generated client calls into this package at a few marked HAND-FIX points;
// the fleet commands use Client directly, one per NAS.
package dsm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"synology-pp-cli/internal/cliutil"
)

// Spec paths used by this package (spec.yaml session.* and the login shape in
// auth.session_token_url).
const (
	loginPath  = "/webapi/entry.cgi"
	loginQuery = "api=SYNO.API.Auth&method=login&version=6&session=webui&format=sid&enable_syno_token=yes&enable_device_token=yes"
	LogoutPath = "/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6"
)

// authAPI is the login namespace, read from loginQuery so the name lives in one place.
var authAPI = mustQuery(loginQuery).Get("api")

func mustQuery(q string) url.Values {
	v, err := url.ParseQuery(q)
	if err != nil {
		panic(err)
	}
	return v
}

// Credentials carries what a login needs. It is read from env vars or the CLI
// config at call time and never written to disk by this package.
type Credentials struct {
	Account    string
	Password   string
	OTPCode    string
	DeviceID   string
	DeviceName string
}

// Complete reports whether a login can be attempted.
func (c Credentials) Complete() bool { return c.Account != "" && c.Password != "" }

// Session is the useful part of a successful login.
type Session struct {
	SID       string
	SynoToken string
	DeviceID  string // trusted-device token, returned when enable_device_token=yes
}

// Error is a DSM failure envelope.
type Error struct {
	Code int
	API  string
}

func (e *Error) Error() string { return ErrorText(e.Code, e.API) }

// HTTPStatus maps a DSM code to the HTTP status the generated CLI uses for exit
// codes: auth problems look like 401/403, a missing API like 404.
func (e *Error) HTTPStatus() int { return HTTPStatus(e.Code, e.API) }

// IsAuth reports whether the failure came from the login API itself.
func (e *Error) IsAuth() bool { return e.API == authAPI }

var errorMessages = map[int]string{
	100: "unknown error",
	101: "invalid parameter",
	102: "the requested API does not exist on this NAS - the package that provides it (for example Active Backup for Business) may not be installed",
	103: "the requested method does not exist",
	104: "this DSM does not support the requested API version",
	105: "insufficient permission - the account may not be an administrator or may not be granted this application",
	106: "session timeout",
	107: "session interrupted by a duplicate login",
	114: "a required parameter is missing",
	117: "invalid parameter value (SMART calls need a device path such as /dev/sata1)",
	119: "session expired",
	120: "a required parameter is missing",
	150: "the request source IP does not match the login IP",
	160: "insufficient application privilege - grant this account the application in DSM",
}

// authErrorMessages apply only to SYNO.API.Auth; other namespaces reuse 4xx
// numbers for their own meanings.
var authErrorMessages = map[int]string{
	400: "invalid account or password",
	401: "the account is disabled",
	402: "permission denied",
	403: "a one-time code is required - log in once with a fresh code in SYNOLOGY_OTP_CODE (or <prefix>_OTP_CODE for a fleet NAS); the CLI caches the trusted-device token DSM returns for later logins",
	404: "the one-time code was rejected",
	406: "two-factor authentication is enforced but this account has no OTP enrolled",
	407: "this IP is blocked by the DSM auto-block list",
	408: "the account password has expired and must be changed in DSM",
	409: "the account password has expired",
	410: "the account password must be changed in DSM before API use",
}

// ErrorText renders a DSM error code as an actionable sentence.
func ErrorText(code int, api string) string {
	if msg, ok := errorMessages[code]; ok {
		return fmt.Sprintf("DSM error %d: %s", code, msg)
	}
	if api == authAPI {
		if msg, ok := authErrorMessages[code]; ok {
			return fmt.Sprintf("DSM error %d: %s", code, msg)
		}
	}
	return fmt.Sprintf("DSM error %d", code)
}

// HTTPStatus maps a DSM code to an HTTP-like status for exit-code classification.
func HTTPStatus(code int, api string) int {
	switch {
	case SessionExpired(code):
		return http.StatusUnauthorized
	case code == 105 || code == 160:
		return http.StatusForbidden
	case code == 102 || code == 103 || code == 104:
		return http.StatusNotFound
	case api == authAPI && code >= 400 && code < 500:
		return http.StatusUnauthorized
	}
	return http.StatusBadRequest
}

// SessionExpired reports whether a code means "log in again and retry".
func SessionExpired(code int) bool { return code == 106 || code == 107 || code == 119 || code == 150 }

type envelope struct {
	Success json.RawMessage `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   json.RawMessage `json:"error"`
}

// Envelope inspects a response body. failed is true for a success:false
// envelope (code carries the DSM code). For success:true with a non-null data
// member, data is the unwrapped payload. Anything else (non-JSON, arrays,
// binary, success with no data) returns data=nil, failed=false: pass the body
// through unchanged.
func Envelope(body []byte) (data json.RawMessage, code int, failed bool) {
	trimmed := strings.TrimLeft(string(body), " \t\r\n")
	if !strings.HasPrefix(trimmed, "{") {
		return nil, 0, false
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil || len(env.Success) == 0 {
		return nil, 0, false
	}
	// The success flag decides, decoded on its own: a malformed error code
	// must never turn a failure into data. Anything but true is a failure.
	switch strings.Trim(strings.ToLower(string(env.Success)), `"`) {
	case "true":
	default:
		var e struct {
			Code json.RawMessage `json:"code"`
		}
		code := 100 // unknown error
		if json.Unmarshal(env.Error, &e) == nil {
			if n, err := strconv.Atoi(strings.Trim(string(e.Code), `" `)); err == nil {
				code = n
			}
		}
		return nil, code, true
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, 0, false
	}
	return env.Data, 0, false
}

// APIName extracts the api= value from a request path or URL for error text.
func APIName(pathOrURL string) string {
	if u, err := url.Parse(pathOrURL); err == nil {
		return u.Query().Get("api")
	}
	return ""
}

// operationKeys select which DSM operation a request runs (api, method,
// version) or carry its session (_sid, SynoToken). On DSM every operation is
// a GET on one CGI path selected by these keys, so a parameter that set one
// would turn a read into a write: `sync --param api=SYNO.ActiveBackup.Task
// --global-param method=backup` (or the same three arguments on the MCP sync
// tool) started a backup.
var operationKeys = map[string]bool{"api": true, "method": true, "version": true, "_sid": true, "synotoken": true}

// CheckOperationParams refuses a request whose extra parameters would change
// the operation its spec path selects: a key that selects the operation or
// session (compared case-insensitively), a key the path itself fixes, or an
// operation key the path carries twice (a query appended to the path). Every
// request the CLI and the fleet layer send passes this one rule; nothing is
// sent when it fails.
func CheckOperationParams(path string, params map[string]string) error {
	fixed := url.Values{}
	if _, rawQuery, ok := strings.Cut(path, "?"); ok {
		fixed, _ = url.ParseQuery(rawQuery)
	}
	for k, vs := range fixed {
		if operationKeys[strings.ToLower(k)] && len(vs) > 1 {
			return fmt.Errorf("refused: the request names its DSM %q more than once; a parameter may not re-select the operation (nothing was sent)", k)
		}
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if operationKeys[strings.ToLower(k)] {
			return fmt.Errorf("refused: parameter %q selects the DSM operation or session, which the command fixes; it cannot be set by a flag, --param or an MCP argument (nothing was sent)", k)
		}
		if _, ok := fixed[k]; ok {
			return fmt.Errorf("refused: parameter %q is fixed by this command's DSM call and cannot be overridden (nothing was sent)", k)
		}
	}
	return nil
}

// secretPattern matches a credential-bearing parameter with a plain or
// URL-encoded "=" (an SSO return URL carries "%26_sid%3D..."), up to the next
// plain or encoded "&". It over-masks rather than under-masks.
var secretPattern = regexp.MustCompile(`(?i)(passwd|password|otp_code|device_id|_sid|synotoken)(=|%3d|%253d)(?:[^&\s"'%]|%(?:[013-9a-f][0-9a-f]|2[0-57-9a-f]))*`)

// Scrub masks credential-bearing query parameters in free text (error strings
// from net/url embed the request URL).
func Scrub(text string) string {
	return secretPattern.ReplaceAllStringFunc(text, func(m string) string {
		loc := secretPattern.FindStringSubmatchIndex(m)
		return m[:loc[5]] + "***"
	})
}

var pctEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)

// errRedirectRefused is what errors.Is matches for a refused redirect
// (*RedirectError).
var errRedirectRefused = errors.New("refusing to follow a redirect")

// verifyBeforeSending is VerifyDSM for a request that carries the session:
// a host that is not DSM is a SessionRefusedError, one that did not answer
// keeps its transport kind.
func (c *Client) verifyBeforeSending(ctx context.Context, what string) error {
	err := VerifyDSM(ctx, c.HTTP, c.BaseURL)
	if err == nil {
		return nil
	}
	var notDSM *NotDSMError
	if errors.As(err, &notDSM) {
		return fmt.Errorf("%s: %w", what, SessionNotSent(err))
	}
	return c.transportErr(what, err)
}

// transportErr reports a failed request by kind only (describeTransport),
// keeping the kind for errors.Is / errors.As (SanitizeTransport).
func (c *Client) transportErr(what string, err error, _ ...string) error {
	return fmt.Errorf("%s: %w", what, SanitizeTransport(err))
}

// Transport failure kinds (TransportKind): what went wrong with a request
// that got no usable answer. Every surface that classifies one (the error
// text, doctor's API and Credentials rows) reads this one function.
const (
	KindPinMismatch = "pin-mismatch" // the NAS presented a certificate other than the pinned one
	KindCertificate = "certificate"  // the certificate failed verification (self-signed, wrong host, unknown CA)
	KindRedirect    = "redirect"     // the NAS answered with a redirect (never followed)
	KindTimeout     = "timeout"
	KindCanceled    = "canceled"
	KindUnreachable = "unreachable" // no connection (refused, no route, DNS)
	KindOther       = "other"
)

// TransportKind classifies a transport error by its type, never its text.
func TransportKind(err error) string {
	var (
		netErr  *net.OpError
		certErr *tls.CertificateVerificationError
		hostErr x509.HostnameError
		authErr x509.UnknownAuthorityError
		pinErr  *PinMismatchError
		redir   *RedirectError
	)
	switch {
	case errors.As(err, &pinErr):
		return KindPinMismatch
	case errors.Is(err, context.DeadlineExceeded):
		return KindTimeout
	case errors.Is(err, context.Canceled):
		return KindCanceled
	case errors.As(err, &redir), errors.Is(err, errRedirectRefused):
		return KindRedirect
	case errors.As(err, &certErr), errors.As(err, &hostErr), errors.As(err, &authErr):
		return KindCertificate
	case errors.As(err, &netErr):
		return KindUnreachable
	}
	return KindOther
}

// describeTransport names what went wrong with a request without quoting
// Go's error text, which can carry server-chosen bytes (a redirect URL, a
// malformed status line, a certificate field).
func describeTransport(err error) string {
	switch TransportKind(err) {
	case KindPinMismatch:
		var pinErr *PinMismatchError
		errors.As(err, &pinErr)
		return fmt.Sprintf("the NAS certificate does not match the pinned SHA-256 fingerprint (the NAS presented %s); %s", pinErr.Presented, PinMismatchHint)
	case KindTimeout:
		return "the request timed out"
	case KindCanceled:
		return "the request was canceled"
	case KindRedirect:
		redir := &RedirectError{}
		errors.As(err, &redir)
		return redir.Error()
	case KindCertificate:
		return "the NAS certificate failed verification (check SYNOLOGY_TLS_FINGERPRINT or fleet add --pin-cert)"
	case KindUnreachable:
		var netErr *net.OpError
		errors.As(err, &netErr)
		return "cannot reach the NAS (" + netErr.Op + " failed)"
	}
	return "the request failed (connection error or malformed response)"
}

// errReadBody reports a failed body read without Go's error text, which can
// quote response-controlled bytes (a malformed chunk trailer, say).
func errReadBody(what string) error {
	return fmt.Errorf("%s: reading the response failed (connection closed or malformed body)", what)
}

// Decoded repeatedly decodes HTML entities and percent escapes (either case)
// until the text stops changing, so a secret cannot hide behind an encoding
// in a diagnostic. Masking runs on what a reader would see.
func Decoded(s string) string {
	// Each pass that changes the text shortens it, so this reaches a fixed point.
	for {
		d := pctEscape.ReplaceAllStringFunc(html.UnescapeString(s), func(m string) string {
			b, _ := hex.DecodeString(m[1:])
			return string(b)
		})
		if d == s {
			return s
		}
		s = d
	}
}

// DescribeTransport is describeTransport for the generated client.
func DescribeTransport(err error) string { return describeTransport(err) }

// ErrRedirectRefused is what errors.Is matches for a refused redirect.
var ErrRedirectRefused = errRedirectRefused

// ScrubSecrets is scrubSecrets for the generated session manager.
func ScrubSecrets(text string, secrets ...string) string { return scrubSecrets(text, secrets...) }

// scrubSecrets is Scrub plus masking of known secret values, raw or escaped
// (a proxy may echo a sid anywhere in a redirect URL).
func scrubSecrets(text string, secrets ...string) string {
	text = Decoded(text)
	// Whole known values first, longest variant first: parameter scrubbing
	// stops at an encoded "&", and masking "alpha" before "alphaOMEGA" would
	// leave "OMEGA" behind.
	var forms []string
	for _, v := range secrets {
		if v == "" {
			continue
		}
		j, _ := json.Marshal(v)
		forms = append(forms, v, Decoded(v), url.QueryEscape(v), url.PathEscape(v), url.QueryEscape(url.QueryEscape(v)), url.PathEscape(url.PathEscape(v)),
			strings.Trim(string(j), `"`), html.EscapeString(v))
	}
	sort.Slice(forms, func(i, k int) bool { return len(forms[i]) > len(forms[k]) })
	for _, f := range forms {
		text = strings.ReplaceAll(text, f, "***")
	}
	return Scrub(text)
}

// CheckOrigin rejects a NAS address that is not a plain scheme://host:port.
// The error never quotes the address: it may carry a password.
func CheckOrigin(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return errors.New("the NAS address must be scheme://host:port with no user or password, for example https://<nas>:5001")
	}
	return nil
}

// PlaceholderOrigin reports an address on a reserved example host (RFC 2606:
// example.com/.net/.org and their subdomains, .example, .invalid), such as the
// CLI's default https://nas.example.com:5001. No NAS lives there, so nothing
// (a password least of all) is sent to it.
func PlaceholderOrigin(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	labels := strings.Split(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), ".")
	last := labels[len(labels)-1]
	if len(labels) > 1 && (last == "example" || last == "invalid") {
		return true
	}
	return len(labels) > 1 && labels[len(labels)-2] == "example" && (last == "com" || last == "net" || last == "org")
}

// Login performs SYNO.API.Auth login against baseURL (scheme://host:port).
//
// No credential leaves the process until the host proves it is a DSM WebAPI:
// VerifyDSM asks it for SYNO.API.Info without credentials first (once per
// address per process), so a mistyped address (the client's router on :5001,
// a proxy, a web server) gets an unauthenticated GET and never the account
// and password. Every login in this CLI (auth login, every live command,
// fleet add --check, fleet sync, doctor) comes through here.
func Login(ctx context.Context, hc *http.Client, baseURL string, creds Credentials) (Session, error) {
	if err := CheckOrigin(baseURL); err != nil {
		return Session{}, err
	}
	if PlaceholderOrigin(baseURL) {
		return Session{}, errors.New("the NAS address is an example placeholder; set the real NAS address")
	}
	if !creds.Complete() {
		return Session{}, errors.New("DSM login needs SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD (or the fleet entry's account/password env vars)")
	}
	if err := VerifyDSM(ctx, hc, baseURL); err != nil {
		var notDSM *NotDSMError
		if errors.As(err, &notDSM) {
			return Session{}, fmt.Errorf("DSM login refused: %w; the password was not sent - check SYNOLOGY_BASE_URL (or the fleet entry's --url)", err)
		}
		return Session{}, fmt.Errorf("DSM login: %w; the password was not sent", err)
	}
	// The host is DSM; over http:// the password is about to cross the
	// network in clear text. Say so once per NAS, on every login path.
	WarnPlainHTTP(baseURL)
	return login(ctx, hc, baseURL, creds)
}

// login sends the credential form to a host ProbeDSM accepted.
func login(ctx context.Context, hc *http.Client, baseURL string, creds Credentials) (Session, error) {
	if creds.DeviceID != "" && creds.OTPCode != "" {
		// Prefer the trusted-device token; fall back to the one-time code only
		// when DSM no longer accepts the token (403/404: code required/rejected).
		withToken := creds
		withToken.OTPCode = ""
		s, err := login(ctx, hc, baseURL, withToken)
		var de *Error
		if err == nil || !errors.As(err, &de) || (de.Code != 403 && de.Code != 404) {
			return s, err
		}
		creds.DeviceID = ""
	}
	form := mustQuery(loginQuery)
	form.Set("account", creds.Account)
	form.Set("passwd", creds.Password)
	if creds.OTPCode != "" {
		form.Set("otp_code", creds.OTPCode)
	}
	if creds.DeviceID != "" {
		form.Set("device_id", creds.DeviceID)
	}
	// The guide's OTP + enable_device_token example names the device; send it
	// on every login so DSM can mint (and later match) the trusted-device token.
	name := creds.DeviceName
	if name == "" {
		name = "synology-cli"
	}
	form.Set("device_name", name)
	// Never follow a redirect with the password form: a 307/308 would re-send it.
	hc = NoRedirect(hc)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+loginPath, strings.NewReader(form.Encode()))
	if err != nil {
		return Session{}, errors.New(scrubSecrets(err.Error(), creds.Password, creds.OTPCode, creds.DeviceID))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("DSM login: %w", SanitizeTransport(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Session{}, errReadBody("DSM login")
	}
	if rerr := RedirectStatus(resp.StatusCode); rerr != nil {
		return Session{}, fmt.Errorf("DSM login: %w", rerr)
	}
	if resp.StatusCode >= 400 {
		return Session{}, fmt.Errorf("DSM login: HTTP %d", resp.StatusCode)
	}
	return parseLogin(body)
}

func parseLogin(body []byte) (Session, error) {
	data, code, failed := Envelope(body)
	if failed {
		return Session{}, &Error{Code: code, API: authAPI}
	}
	if data == nil {
		return Session{}, errors.New("DSM login: response was not a DSM success envelope")
	}
	var out struct {
		SID       string `json:"sid"`
		SynoToken string `json:"synotoken"`
		DID       string `json:"did"`
		DeviceID  string `json:"device_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Session{}, fmt.Errorf("DSM login: parsing payload: %w", err)
	}
	if out.SID == "" {
		return Session{}, errors.New("DSM login: response carried no session id")
	}
	did := out.DID
	if did == "" {
		did = out.DeviceID
	}
	return Session{SID: out.SID, SynoToken: out.SynoToken, DeviceID: did}, nil
}

// Client is a minimal DSM RPC client for one NAS, used by the fleet commands.
// It logs in lazily, attaches _sid and X-SYNO-TOKEN, unwraps the envelope,
// relogs in once on an expired session and paces requests with an adaptive
// limiter (a NAS on a client's WAN link is easy to overload).
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Creds   Credentials
	// RatePerSec caps requests per second to this NAS (default 5).
	RatePerSec float64
	// OnDeviceID receives a new trusted-device token DSM issued at login (a
	// two-factor login with a one-time code), so the caller can cache it. A
	// returned error is kept in DeviceIDErr for the caller to surface.
	OnDeviceID  func(deviceID string) error
	DeviceIDErr error

	mu      sync.Mutex
	sess    Session
	seen    []string // every credential and session value used, for Redact
	limiter *cliutil.AdaptiveLimiter
}

// Redact masks every credential and session value this client has used
// (password, one-time codes, device tokens, sids, SynoTokens) in text, in
// any encoding. Fleet and triage errors pass through it before they are
// printed or stored.
func (c *Client) Redact(text string) string {
	return scrubSecrets(text, c.secrets()...)
}

// secrets lists every credential and session value this client has used.
func (c *Client) secrets() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{c.Creds.Password, c.Creds.OTPCode, c.Creds.DeviceID}, c.seen...)
}

// Get calls a spec path (/webapi/entry.cgi?api=...&method=...&version=...)
// with extra params and returns the unwrapped data payload.
func (c *Client) Get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("invalid DSM path %q: %w", path, err)
	}
	api := u.Query().Get("api")
	flat := map[string]string{}
	for k, vs := range params {
		flat[k] = strings.Join(vs, ",")
	}
	if err := CheckOperationParams(path, flat); err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		sess, err := c.session(ctx)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, vs := range params {
			q[k] = vs
		}
		q.Set("_sid", sess.SID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+u.Path+"?"+q.Encode(), nil)
		if err != nil {
			return nil, errors.New(scrubSecrets(err.Error(), sess.SID, sess.SynoToken))
		}
		if sess.SynoToken != "" {
			req.Header.Set("X-SYNO-TOKEN", sess.SynoToken)
		}
		body, err := c.do(ctx, req, api)
		if err != nil {
			return nil, err
		}
		data, code, failed := Envelope(body)
		if failed {
			if SessionExpired(code) && attempt == 0 {
				c.reset()
				continue
			}
			return nil, &Error{Code: code, API: api}
		}
		if data == nil {
			// An HTML page (proxy, captive portal, DSM maintenance) or a bare
			// {"success":true} is not an answer to a read; never treat it as
			// "nothing there".
			return nil, fmt.Errorf("%s: the NAS did not return a DSM data envelope (%s)", api, snippet(body))
		}
		// Every successful answer is masked for every secret value this
		// client holds before the fleet layer prints, stores or mirrors it.
		return MaskResponse(data, c.secrets()...), nil
	}
}

// snippet describes an unexpected body without quoting it (a proxy page can
// echo the request URL, session id included).
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	switch {
	case s == "":
		return "empty body"
	case strings.HasPrefix(s, "<"):
		return fmt.Sprintf("%d-byte HTML page", len(body))
	case strings.HasPrefix(s, "{") || strings.HasPrefix(s, "["):
		return fmt.Sprintf("%d-byte JSON without DSM data", len(body))
	}
	return fmt.Sprintf("%d-byte non-JSON body", len(body))
}

// do sends one request through the limiter, retrying HTTP 429 with backoff and
// returning *cliutil.RateLimitError when the NAS keeps throttling.
func (c *Client) do(ctx context.Context, req *http.Request, api string) ([]byte, error) {
	lim := c.rateLimiter()
	for attempt := 0; ; attempt++ {
		if err := lim.Wait(ctx); err != nil {
			return nil, err
		}
		// The request carries the sid and SynoToken: it goes only to an address
		// that answered SYNO.API.Info as DSM in this process (the login checked
		// it; this keeps the rule true for every send).
		if err := c.verifyBeforeSending(ctx, api); err != nil {
			return nil, err
		}
		// No DSM request follows a redirect, whatever client was handed in.
		resp, err := NoRedirect(c.HTTP).Do(req.Clone(ctx))
		if err != nil {
			return nil, c.transportErr(api, err, req.URL.Query().Get("_sid"), req.Header.Get("X-SYNO-TOKEN"))
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, errReadBody(api)
		}
		if rerr := RedirectStatus(resp.StatusCode); rerr != nil {
			return nil, fmt.Errorf("%s: %w", api, rerr)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			lim.OnRateLimit()
			wait := cliutil.RetryAfter(resp)
			if attempt >= 2 {
				return nil, &cliutil.RateLimitError{URL: scrubSecrets(req.URL.String(), req.URL.Query().Get("_sid")), RetryAfter: wait}
			}
			if wait <= 0 {
				wait = cliutil.Backoff(attempt)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("%s: HTTP %d", api, resp.StatusCode)
		}
		lim.OnSuccess()
		return body, nil
	}
}

func (c *Client) rateLimiter() *cliutil.AdaptiveLimiter {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiter == nil {
		rate := c.RatePerSec
		if rate <= 0 {
			rate = 5
		}
		c.limiter = cliutil.NewAdaptiveLimiter(rate)
	}
	return c.limiter
}

// Logout ends the session if one is open. Errors are ignored by callers that
// only want to avoid leaving sessions behind.
func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	sess := c.sess
	c.sess = Session{}
	c.mu.Unlock()
	if sess.SID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+LogoutPath+"&_sid="+url.QueryEscape(sess.SID), nil)
	if err != nil {
		return errors.New(scrubSecrets(err.Error(), sess.SID, sess.SynoToken))
	}
	if sess.SynoToken != "" {
		req.Header.Set("X-SYNO-TOKEN", sess.SynoToken)
	}
	if err := c.verifyBeforeSending(ctx, "DSM logout"); err != nil {
		return err
	}
	resp, err := NoRedirect(c.HTTP).Do(req)
	if err != nil {
		return c.transportErr("DSM logout", err, sess.SID, sess.SynoToken)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	if err != nil {
		return errReadBody("DSM logout")
	}
	if rerr := RedirectStatus(resp.StatusCode); rerr != nil {
		return fmt.Errorf("DSM logout: %w", rerr)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("DSM logout: HTTP %d", resp.StatusCode)
	}
	if _, code, failed := Envelope(body); failed {
		return &Error{Code: code, API: authAPI}
	}
	if !IsSuccess(body) {
		return fmt.Errorf("DSM logout: %s", snippet(body))
	}
	return nil
}

// IsSuccess reports whether body is a DSM envelope with success:true (with or
// without data).
func IsSuccess(body []byte) bool {
	var env envelope
	return json.Unmarshal(body, &env) == nil && strings.TrimSpace(string(env.Success)) == "true"
}

func (c *Client) session(ctx context.Context) (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sess.SID != "" {
		return c.sess, nil
	}
	c.seen = append(c.seen, c.Creds.Password, c.Creds.OTPCode, c.Creds.DeviceID)
	s, err := Login(ctx, c.HTTP, c.BaseURL, c.Creds)
	if err != nil {
		return Session{}, err
	}
	c.sess = s
	c.seen = append(c.seen, s.SID, s.SynoToken, s.DeviceID)
	if s.DeviceID != "" && s.DeviceID != c.Creds.DeviceID && c.OnDeviceID != nil {
		c.DeviceIDErr = c.OnDeviceID(s.DeviceID)
	}
	if s.DeviceID != "" {
		// A relogin after an expired session must use the device token, not
		// the one-time code that was just spent.
		c.Creds.DeviceID, c.Creds.OTPCode = s.DeviceID, ""
	}
	return s, nil
}

func (c *Client) reset() {
	c.mu.Lock()
	c.sess = Session{}
	c.mu.Unlock()
}
