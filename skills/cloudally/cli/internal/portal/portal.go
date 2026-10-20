// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package portal reads the CloudAlly customer portal's own service layer.
//
// The documented V2 API at api.cloudally.com exposes only the latest backup
// date per mailbox and per sub-source, so it cannot answer "which backup was
// taken on or before this past date". The customer portal's restore wizard
// can: it loads a per-mailbox backup calendar from a service layer mounted at
// /application/service/<operation> on the portal host.
//
// That service layer authenticates with a portal session cookie plus a CSRF
// header, not with a partner bearer token. A partner reaches it the same way
// the partner portal does when an operator clicks a customer account name:
//
//  1. POST /v2/{distributors|resellers}/accounts/{id}/partner-access-token on
//     the documented API mints a short-lived, single-use, opaque handoff token
//     for that customer. It is not a JWT and carries no readable claims.
//  2. GET <portal>/newapp?accessToken=<token> spends the token and returns a
//     session cookie scoped to that customer. The token is consumed by this
//     one request; a second use fails.
//  3. The served page carries the session's CSRF token and the name of the
//     header to send it in, as <meta name="_csrf"> and <meta name="_csrf_header">.
//  4. POST <portal>/application/service/<operation> with that cookie and that
//     header reads customer data.
//
// Everything this package calls is read-only.
package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloudally-pp-cli/internal/cliutil"
)

// DefaultBaseURL is the customer portal origin. www.cloudally.com/app and the
// legacy white-label hosts redirect here.
const DefaultBaseURL = "https://app.cloudally.com"

// Portal paths. servicePath is the RPC mount: the operation name is appended
// to it directly.
const (
	bootstrapPath = "/newapp"
	servicePath   = "/application/service/"
)

// handoffParam is the query parameter that spends a partner handoff token.
const handoffParam = "accessToken"

// CSRF meta tag names in the served page. The page names its own header
// rather than hard-coding one, so both are read.
const (
	csrfMetaName       = "_csrf"
	csrfHeaderMetaName = "_csrf_header"
)

// fallbackCSRFHeader is used only if the page omits _csrf_header.
const fallbackCSRFHeader = "X-CSRF-TOKEN"

// deferredResult is the envelope marker for "not ready, poll by reqId".
const deferredResult = "WAIT"

// Deferred-result polling bounds. The portal front-end waits 1500ms between
// polls; the attempt cap keeps a stuck job from hanging a CLI run.
const (
	deferredPollInterval = 1500 * time.Millisecond
	maxDeferredPolls     = 20
)

// defaultTimeout bounds one portal request.
const defaultTimeout = 60 * time.Second

// defaultRatePerSec paces service calls. The portal is a customer-facing web
// app rather than a metered API and sends no rate-limit headers, so a
// resolution that walks many mailbox-months stays deliberately gentle and
// capped: four calls a second from one session, never raised by the server.
const defaultRatePerSec = 4.0

// Throttle handling for a 429 or a 503. Retry-After is honoured when the
// server sends one, clamped so a bad header cannot stall a whole run.
const (
	maxThrottleRetries = 3
	defaultRetryAfter  = 2 * time.Second
	maxRetryAfter      = 30 * time.Second
)

// ErrThrottled is returned when the portal keeps refusing on rate grounds.
var ErrThrottled = errors.New("portal rate-limited the session")

// userAgent is sent because the portal serves its SPA shell, not an API.
const userAgent = "cloudally-cli"

// maxBootstrapBody caps how much of the SPA shell is read while looking for
// the CSRF meta tags.
const maxBootstrapBody = 1 << 20

// ErrNoCSRF is returned when the portal page carries no CSRF token, which is
// what an unauthenticated or redirected response looks like.
var ErrNoCSRF = errors.New("portal page carried no _csrf token: the handoff token was refused, already spent, or expired")

// ErrNoSession is returned when the service layer rejects the session.
var ErrNoSession = errors.New("portal session is not authenticated")

// operationPattern constrains an operation name to a bare identifier so a
// caller can never steer the request off the service mount.
var operationPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// metaTagPattern matches a whole <meta ...> tag.
var metaTagPattern = regexp.MustCompile(`<meta\s+[^>]*>`)

// metaNameAttr and metaContentAttr read those attributes off a single tag.
var (
	metaNameAttr    = regexp.MustCompile(`(?i)\bname\s*=\s*"([^"]*)"`)
	metaContentAttr = regexp.MustCompile(`(?i)\bcontent\s*=\s*"([^"]*)"`)
)

// Config configures a Session. The zero value uses the public portal.
type Config struct {
	// BaseURL overrides the portal origin.
	BaseURL string
	// HTTPClient supplies the transport. A cookie jar is installed on a
	// copy of it, so the caller's client is never mutated.
	HTTPClient *http.Client
	// Timeout bounds one request; zero means defaultTimeout.
	Timeout time.Duration
	// RatePerSec caps outbound service calls; zero means
	// defaultRatePerSec. A negative value disables pacing, which only
	// tests have a reason to do.
	RatePerSec float64
}

// Session is an authenticated customer-portal session. It is safe for
// concurrent use: the rate limiter serialises pacing and the call counter is
// guarded, so several entities can be resolved at once over one session.
type Session struct {
	base       *url.URL
	hc         *http.Client
	csrfToken  string
	csrfHeader string
	// mu guards calls.
	mu sync.Mutex
	// calls counts service requests, for tests and for run summaries.
	calls int
	// limiter paces outbound service calls. It is the same adaptive
	// limiter the generated API client uses, with a hard ceiling because
	// the portal advertises no budget of its own.
	limiter *cliutil.AdaptiveLimiter
}

// pace waits for the rate limiter so a long resolution never bursts at the
// portal. A nil limiter (pacing disabled) returns at once.
func (s *Session) pace(ctx context.Context) error {
	return s.limiter.Wait(ctx)
}

// retryAfter reads a Retry-After delay from a throttled response, clamped
// to a sane range.
func retryAfter(resp *http.Response) time.Duration {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return defaultRetryAfter
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		d := time.Duration(secs) * time.Second
		if d <= 0 {
			return defaultRetryAfter
		}
		return min(d, maxRetryAfter)
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return min(d, maxRetryAfter)
		}
	}
	return defaultRetryAfter
}

// envelope is the service layer's response wrapper. A deferred response
// carries result "WAIT" and an id to poll with.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Result  string          `json:"result"`
	ID      string          `json:"id"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
	// Status and StatusCode carry the failure text on the GET services
	// (getDownloadUrl answers an unknown job with HTTP 200 and
	// success:false, so the body is the only signal).
	Status     string `json:"status"`
	StatusCode int    `json:"statusCode"`
}

// Open spends a partner handoff token and returns a session for the customer
// account that token was minted for. The token is single-use: mint a fresh
// one per session.
func Open(ctx context.Context, cfg Config, handoffToken string) (*Session, error) {
	if strings.TrimSpace(handoffToken) == "" {
		return nil, errors.New("portal: handoff token is empty")
	}
	raw := cfg.BaseURL
	if strings.TrimSpace(raw) == "" {
		raw = DefaultBaseURL
	}
	base, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("portal: base URL %q: %w", raw, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("portal: base URL %q needs a scheme and host", raw)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("portal: cookie jar: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	hc := &http.Client{Jar: jar, Timeout: timeout}
	var callerRedirect func(*http.Request, []*http.Request) error
	if cfg.HTTPClient != nil {
		// Copy so the caller's client keeps its own (absent) jar.
		hc.Transport = cfg.HTTPClient.Transport
		callerRedirect = cfg.HTTPClient.CheckRedirect
		if cfg.HTTPClient.Timeout > 0 && cfg.Timeout <= 0 {
			hc.Timeout = cfg.HTTPClient.Timeout
		}
	}
	// The origin rule runs first, so a caller's own policy can only narrow it.
	hc.CheckRedirect = redirectPolicy(base, callerRedirect)
	rate := cfg.RatePerSec
	if rate == 0 {
		rate = defaultRatePerSec
	}
	// NewAdaptiveLimiter caps at the rate given, unlike the Auto variant:
	// the portal publishes no budget, so it must never be ramped up.
	s := &Session{base: base, hc: hc, limiter: cliutil.NewAdaptiveLimiter(rate)}
	// Spending the token sets the session cookie and redirects to the SPA
	// shell; following the redirect lands on a page that carries the CSRF
	// token for the session just created.
	body, err := s.getPage(ctx, bootstrapPath, url.Values{handoffParam: {handoffToken}})
	if err != nil {
		return nil, err
	}
	token, header := parseCSRFMeta(body)
	if token == "" {
		// Some deployments answer the exchange with a bare redirect body;
		// the shell itself still carries the token for this session.
		body, err = s.getPage(ctx, bootstrapPath, nil)
		if err != nil {
			return nil, err
		}
		token, header = parseCSRFMeta(body)
	}
	if token == "" {
		return nil, ErrNoCSRF
	}
	if header == "" {
		header = fallbackCSRFHeader
	}
	s.csrfToken, s.csrfHeader = token, header
	return s, nil
}

// Calls reports how many service requests this session has made.
func (s *Session) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// countCall records one outbound service request.
func (s *Session) countCall() {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
}

// getPage fetches an HTML page, following redirects, and returns its body.
func (s *Session) getPage(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := *s.base
	u.Path = path
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("portal: building request for %s: %w", path, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.hc.Do(req)
	if err != nil {
		// Never echo the URL: it may carry the handoff token.
		return nil, fmt.Errorf("portal: GET %s: %w", path, scrubURLError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBootstrapBody))
	if err != nil {
		return nil, fmt.Errorf("portal: reading %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("portal: GET %s: unexpected status %d", path, resp.StatusCode)
	}
	return body, nil
}

// ErrOutcomeUnknown marks a failed call that may still have been acted on:
// a throttling or server-error answer, a transport failure, or a deferred
// result that never resolved, on an operation that creates something. A
// caller must reconcile (list the jobs) before sending such a request again,
// and must never record it as a plain failure that a resume would resend.
var ErrOutcomeUnknown = errors.New("portal: the request may have been acted on")

// readOperations are the service operations known only to read. Call treats
// them like CallRead (a throttling answer is retried), so a caller that still
// uses Call for one of them keeps that resilience. Every other operation,
// startRestoreOrDownload above all, is treated as one that creates something.
var readOperations = map[string]bool{
	"getBackupTaskInfo":        true,
	"getDownloadInfo":          true,
	"getDownloadUrl":           true,
	"getJobsDasboard":          true,
	"getLoggedUser":            true,
	"getRestoreDownloadStatus": true,
	"getSnapshotsCalendar":     true,
}

// Call invokes one service-layer operation and returns its data field.
// Deferred responses are polled until they resolve.
//
// Unless the operation is a known read (readOperations), Call treats it as
// one that creates something (startRestoreOrDownload creates a job): a
// throttling answer is returned as an error rather than retried, because a
// 503 does not say whether the server acted and a retry could create a
// second job. Such failures wrap ErrOutcomeUnknown. Use CallRead for an
// operation that only reads.
func (s *Session) Call(ctx context.Context, operation string, body any) (json.RawMessage, error) {
	return s.call(ctx, operation, body, readOperations[operation])
}

// CallRead invokes a service-layer operation that only reads, such as
// getSnapshotsCalendar or getRestoreDownloadStatus. Because replaying a read
// is harmless, a throttling answer is retried after Retry-After.
func (s *Session) CallRead(ctx context.Context, operation string, body any) (json.RawMessage, error) {
	return s.call(ctx, operation, body, true)
}

// call implements Call and CallRead. idempotent says whether the operation
// may be sent again after a throttling answer.
func (s *Session) call(ctx context.Context, operation string, body any, idempotent bool) (json.RawMessage, error) {
	if !operationPattern.MatchString(operation) {
		return nil, fmt.Errorf("portal: operation %q is not a bare identifier", operation)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("portal: encoding %s body: %w", operation, err)
	}
	env, err := s.post(ctx, operation, payload, nil, idempotent)
	if err != nil {
		return nil, err
	}
	// Once a request is deferred the server has it, so for a create every
	// failure from here on leaves the outcome unknown.
	pending := func(err error) error {
		if idempotent || errors.Is(err, ErrOutcomeUnknown) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrOutcomeUnknown, err)
	}
	for polls := 0; env.Result == deferredResult; polls++ {
		if polls >= maxDeferredPolls {
			return nil, pending(fmt.Errorf("portal: %s still deferred after %d polls", operation, maxDeferredPolls))
		}
		if env.ID == "" {
			return nil, pending(fmt.Errorf("portal: %s deferred without a request id", operation))
		}
		select {
		case <-ctx.Done():
			return nil, pending(ctx.Err())
		case <-time.After(deferredPollInterval):
		}
		env, err = s.post(ctx, operation, payload, url.Values{"reqId": {env.ID}}, idempotent)
		if err != nil {
			return nil, pending(err)
		}
	}
	if !env.Success {
		return nil, fmt.Errorf("portal: %s failed: %s", operation, env.failureText())
	}
	return env.Data, nil
}

// Get invokes one service-layer operation as a GET with query parameters
// and returns its data field. Several portal services (getDownloadUrl,
// getDownloadInfo) are GETs rather than POSTs.
func (s *Session) Get(ctx context.Context, operation string, params url.Values) (json.RawMessage, error) {
	if !operationPattern.MatchString(operation) {
		return nil, fmt.Errorf("portal: operation %q is not a bare identifier", operation)
	}
	// A GET only reads, so a throttling answer is retried.
	env, err := s.send(ctx, http.MethodGet, operation, nil, params, true)
	if err != nil {
		return nil, err
	}
	if !env.Success {
		return nil, fmt.Errorf("portal: %s failed: %s", operation, env.failureText())
	}
	return env.Data, nil
}

// post sends one service request and decodes its envelope, pacing outbound
// calls and, for an idempotent operation, backing off on a throttling
// response.
func (s *Session) post(ctx context.Context, operation string, payload []byte, query url.Values, idempotent bool) (*envelope, error) {
	return s.send(ctx, http.MethodPost, operation, payload, query, idempotent)
}

// send issues one service request with the given method, pacing outbound
// calls. A throttling response (429 or 503) is retried after Retry-After
// only when idempotent is true; otherwise it is returned at once, because
// the server may already have acted on the request.
func (s *Session) send(ctx context.Context, method, operation string, payload []byte, query url.Values, idempotent bool) (*envelope, error) {
	u := *s.base
	u.Path = servicePath + operation
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	for attempt := 0; ; attempt++ {
		if err := s.pace(ctx); err != nil {
			return nil, err
		}
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
		if err != nil {
			return nil, fmt.Errorf("portal: building %s request: %w", operation, err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set(s.csrfHeader, s.csrfToken)
		s.countCall()
		resp, err := s.hc.Do(req)
		if err != nil {
			// Even a refused redirect follows a request the server received,
			// so for a create every transport failure leaves the outcome
			// unknown.
			return nil, unknownUnless(idempotent, fmt.Errorf("portal: %s %s: %w", method, operation, scrubURLError(err)))
		}
		// A throttling response to a read is retried after Retry-After
		// rather than surfaced as a failure, up to a bounded number of
		// attempts. One that may have created something is never re-sent.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			s.limiter.OnRateLimit()
			wait := retryAfter(resp)
			_ = resp.Body.Close()
			if !idempotent {
				return nil, fmt.Errorf("portal: %s: %w (status %d); not retried because this call can create a job (%w): check the job list before sending it again",
					operation, ErrThrottled, resp.StatusCode, ErrOutcomeUnknown)
			}
			if attempt >= maxThrottleRetries {
				return nil, fmt.Errorf("portal: %s: %w (status %d after %d attempts)",
					operation, ErrThrottled, resp.StatusCode, attempt+1)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, unknownUnless(idempotent, fmt.Errorf("portal: reading %s response: %w", operation, readErr))
		}
		if closeErr != nil {
			return nil, unknownUnless(idempotent, fmt.Errorf("portal: closing %s response: %w", operation, closeErr))
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("portal: %s: %w (status %d)", operation, ErrNoSession, resp.StatusCode)
		case resp.StatusCode >= 500:
			// A server error does not say whether the work was done.
			return nil, unknownUnless(idempotent, fmt.Errorf("portal: %s: status %d", operation, resp.StatusCode))
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return nil, fmt.Errorf("portal: %s: status %d", operation, resp.StatusCode)
		}
		s.limiter.OnSuccess()
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			// A 2xx the client cannot read may still mean the job exists.
			return nil, unknownUnless(idempotent, fmt.Errorf("portal: decoding %s response: %w", operation, err))
		}
		return &env, nil
	}
}

// unknownUnless wraps err in ErrOutcomeUnknown for an operation that may
// create something.
func unknownUnless(idempotent bool, err error) error {
	if idempotent {
		return err
	}
	return fmt.Errorf("%w: %w", ErrOutcomeUnknown, err)
}

// failureText picks the most useful message an envelope carries.
func (e *envelope) failureText() string {
	for _, s := range []string{e.Message, e.Error, e.Status, e.Result} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "no message"
}

// parseCSRFMeta pulls the CSRF token and its header name out of a page.
func parseCSRFMeta(body []byte) (token, header string) {
	for _, tag := range metaTagPattern.FindAll(body, -1) {
		name := submatch(metaNameAttr, tag)
		switch name {
		case csrfMetaName:
			token = submatch(metaContentAttr, tag)
		case csrfHeaderMetaName:
			header = submatch(metaContentAttr, tag)
		}
	}
	return token, header
}

// submatch returns the first capture group of pat in tag, or "".
func submatch(pat *regexp.Regexp, tag []byte) string {
	m := pat.FindSubmatch(tag)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// scrubURLError drops the URL from a *url.Error so a handoff token in a
// query string never reaches a log or an error message.
func scrubURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// ErrRedirectRefused is returned when a portal request is redirected off
// the portal origin, or from https to plain http.
var ErrRedirectRefused = errors.New("portal: redirect refused")

// maxRedirects matches net/http's own limit.
const maxRedirects = 10

// redirectPolicy is the session client's CheckRedirect. Go re-sends custom
// headers on every hop (it strips only Authorization, Cookie and the like
// when the host changes) and replays the body on a 307 or 308, so following
// a portal redirect to another host would hand that host the session's CSRF
// token and the request body. The rules:
//
//   - a chain that starts on the portal origin must stay on it. The partner
//     handoff (/newapp?accessToken= redirecting to /newapp) stays on one
//     origin and keeps working;
//   - no hop may go from https to plain http;
//   - a chain that starts elsewhere (a presigned object-storage link fetched
//     through a copy of this policy) may change host, because it carries no
//     session header, but still may not downgrade.
//
// next, when set, is the caller's own policy and runs after these rules.
func redirectPolicy(base *url.URL, next func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		if len(via) >= maxRedirects {
			return fmt.Errorf("%w: stopped after %d redirects", ErrRedirectRefused, maxRedirects)
		}
		prev := via[len(via)-1].URL
		if strings.EqualFold(prev.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("%w: %s redirected to %s, which drops https", ErrRedirectRefused, originOf(prev), originOf(req.URL))
		}
		if sameOrigin(via[0].URL, base) && !sameOrigin(req.URL, base) {
			return fmt.Errorf("%w: the portal at %s redirected to %s; the session is never sent to another origin (if that host is the customer portal, set CLOUDALLY_PORTAL_URL to it)",
				ErrRedirectRefused, originOf(base), originOf(req.URL))
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
}

// originOf renders scheme://host[:port] and nothing else: a redirect target
// can carry a token in its path or query.
func originOf(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// sameOrigin compares scheme, host and effective port.
func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

// effectivePort fills in the scheme's default port.
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}
