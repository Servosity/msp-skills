// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// knowbe4 handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: the X-Api-Key credential and every API request
//     go only to the scheme, host and port of the configured API base URL
//     (KNOWBE4_BASE_URL or the config file, default
//     https://api-{region}.knowbe4.com/v1 with KNOWBE4_REGION,
//     default us-east-1-cell-1, resolved when the client is built). Every
//     redirect hop must stay on that origin. Nothing given later (a Location
//     header, an MCP argument, a path) can move the credential to another host.
//   - no-response-cache: the on-disk response cache is off, so raw bodies do
//     not sit in cache files.
//   - r1/r2-credential-to-other-api-host: neither API's key goes to the other
//     API's host. The credential kit binds each key to its own origin; this
//     check covers the one case the kit cannot see, a value held from two
//     sources (a config file's api_key equal to the User Event key), which the
//     kit allows to both origins.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"knowbe4-pp-cli/internal/client"
	"knowbe4-pp-cli/internal/cliutil"
	"knowbe4-pp-cli/internal/config"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured KnowBe4 host")

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return nil
	})
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(originPinTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	installPin(c.HTTPClient, pinnedOrigin(c.BaseURL, c.CurrentTemplateVars()), c.DryRun)
	// r2-credential-to-other-api-host: the User Event key is foreign to the
	// Reporting host unless it is the very credential this client sends
	// (config.SentReportingCredentials), never because some configuration
	// layer an environment value overrode also holds it.
	refuseOtherAPICredentials(c.HTTPClient, otherThan(userEventCredentials(), config.SentReportingCredentials(c.Config)))
}

// ErrOtherAPICredential marks a request refused because it carries the other
// KnowBe4 API's key.
var ErrOtherAPICredential = errors.New("request refused: it would carry the other KnowBe4 API's key to this host")

// refuseOtherAPICredentials makes hc refuse, before anything is sent, a
// request that repeats any of foreign (the other KnowBe4 API's credentials)
// whole in its URL, any header or its body, as written or percent-decoded
// (knowbe4 handfix
// r1-credential-to-other-api-host, Codex r1 knowbe4 4.34.2: events create
// --description <the Reporting key> posted the Reporting key to the User
// Event host). Each client is built with the other API's keys from every
// source: the Reporting client with KNOWBE4_USER_EVENT_API_KEY, the User Event
// client with the Reporting key from the environment, the config file and the
// credential store. Redirect hops pass through the transport again. The check
// is part of the origin pin, which installPin has made the outermost
// transport (it runs right after the origin check, before the dry-run
// refusal and the send); a client without the pin gets the pin's refusal
// for every request.
func refuseOtherAPICredentials(hc *http.Client, foreign []string) {
	p, ok := hc.Transport.(originPinTripper)
	if !ok {
		p = originPinTripper{next: hc.Transport} // pin "" refuses every request: fail closed
	}
	p.foreign = foreign
	hc.Transport = p
}

// refuseForeign is the r1-credential-to-other-api-host check of one request:
// the request, with its body read and restored, or the refusal.
func (t originPinTripper) refuseForeign(req *http.Request) (*http.Request, error) {
	if len(t.foreign) == 0 {
		return req, nil
	}
	var body []byte
	if req.Body != nil && req.Body != http.NoBody {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = b
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(b))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
	}
	if repeatsAny(req, body, t.foreign) {
		return nil, fmt.Errorf("%w (%s)", ErrOtherAPICredential, displayOrigin(req.URL))
	}
	return req, nil
}

// repeatsAny reports whether any of values appears whole in the request's URL,
// a header or its body, as written or percent-decoded. These are the effective
// keys of the other API (a short, known list), not a scan for credentials.
func repeatsAny(req *http.Request, body []byte, values []string) bool {
	parts := []string{req.URL.String(), string(body)}
	for name, vs := range req.Header {
		parts = append(parts, name+": "+strings.Join(vs, ", "))
	}
	for _, p := range parts {
		forms := []string{p}
		if u, err := url.PathUnescape(p); err == nil {
			forms = append(forms, u)
		}
		if u, err := url.QueryUnescape(p); err == nil {
			forms = append(forms, u)
		}
		for _, f := range forms {
			for _, v := range values {
				if v != "" && strings.Contains(f, v) {
					return true
				}
			}
		}
	}
	return false
}

// userEventCredentials is the User Event API key (environment only).
func userEventCredentials() []string {
	if k := strings.TrimSpace(cliutil.EnvOverride("KNOWBE4_USER_EVENT_API_KEY")); k != "" {
		return []string{k}
	}
	return nil
}

// otherThan is values without the ones that are also the client's own
// credentials (an operator who set both keys to one value): a request always
// carries its own key in its Authorization header. own is only what the client
// sends as its own credential (config.SentReportingCredentials for the
// Reporting client, the effective KNOWBE4_USER_EVENT_API_KEY for the User
// Event client), never every value a configuration layer holds.
func otherThan(values, own []string) []string {
	mine := map[string]bool{}
	for _, v := range own {
		mine[strings.TrimSpace(v)] = true
	}
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !mine[v] {
			out = append(out, v)
		}
	}
	return out
}

// regionLabel is one DNS label: the only form a KnowBe4 region
// (us-east-1-cell-1, eu-west-1-cell-1) takes in the API host.
var regionLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

var basePlaceholder = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// pinnedOrigin is the ONE origin a client may send to: the configured base
// URL's, with its {region} resolved from KNOWBE4_REGION or the config file at
// the moment the client is built (the generated base URL is the template
// https://api-{region}.knowbe4.com/v1, which does not parse as a
// URL, so pinning it unresolved refused every request). A template value
// that is not one DNS label ("evil.example/x?" would move the host), an
// unresolved placeholder, or a base URL that does not parse pins "" and
// every request is refused (fail closed). A value changed after the client
// is built (sync --path-context region=..., reachable from an MCP tool
// argument) cannot move the credential: requests keep this pin.
func pinnedOrigin(base string, vars map[string]string) string {
	for _, m := range basePlaceholder.FindAllStringSubmatch(base, -1) {
		if !regionLabel.MatchString(vars[m[1]]) {
			return ""
		}
	}
	u, err := url.Parse(client.ResolvedBaseURL(base, vars))
	if err != nil {
		return ""
	}
	return normalizedOrigin(u)
}

func installPin(hc *http.Client, pin string, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = cliutil.OutboundGuard(nil) // Hand-fix (handfixes.json credkit-wiring): never a transport the kit does not wrap
	}
	inner := next
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pin: pin, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set KNOWBE4_BASE_URL or KNOWBE4_REGION to change hosts", ErrOriginChanged, displayOrigin(req.URL))
		}
		if err := checkPin(pin, req); err != nil {
			return err
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
	}
}

// checkPin refuses a request to any origin but pin. An empty pin (a base URL
// that does not parse to an origin) matches nothing: fail closed.
func checkPin(pin string, req *http.Request) error {
	if o := normalizedOrigin(req.URL); o == "" || o != pin {
		return fmt.Errorf("%w (%s); set KNOWBE4_BASE_URL or KNOWBE4_REGION to change hosts", ErrOriginChanged, displayOrigin(req.URL))
	}
	return nil
}

// normalizedOrigin is scheme://host:port with ASCII letters lowercased and
// the default port filled in. It is "" (never equal to a pinned origin) for a
// host holding any non-ASCII byte: Unicode case folding and IDNA can make two
// spellings compare equal while the transport dials another name.
func normalizedOrigin(u *url.URL) string {
	if u == nil || u.Host == "" || !isASCII(u.Scheme) || !isASCII(u.Host) {
		return ""
	}
	scheme := asciiLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(asciiLower(u.Hostname()), port)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

type originPinTripper struct {
	pin     string
	foreign []string // the other KnowBe4 API's credentials (refuseOtherAPICredentials)
	next    http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkPin(t.pin, req); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	req, err := t.refuseForeign(req)
	if err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s)", ErrDryRunSend, req.Method, displayOrigin(req.URL))
}

func displayOrigin(u *url.URL) string {
	if u == nil {
		return "no URL"
	}
	if o := normalizedOrigin(&url.URL{Scheme: u.Scheme, Host: u.Host}); o != "" {
		return o
	}
	return "a non-ASCII host"
}
