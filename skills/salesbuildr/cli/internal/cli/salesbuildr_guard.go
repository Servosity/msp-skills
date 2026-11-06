// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// salesbuildr handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: the api-key header and every API request go
//     only to the scheme, host and port of the API base URL as resolved
//     (SALESBUILDR_BASE_URL or the config file; default
//     https://{tenant}.salesbuildr.com/public-api with SALESBUILDR_TENANT).
//     Every redirect hop must stay on that origin. Nothing given later (a
//     Location header, an MCP argument, a path) can move the credential to
//     another host.
//   - tenant-is-one-label: the tenant is spliced into the host as text, so a
//     value carrying a dot, slash, '@', ':', '?' or '#' would name another
//     host (evil.example#.salesbuildr.com). A base URL with {tenant} pins
//     nothing (every request refused) unless the tenant is one DNS label,
//     and the shipped placeholder tenant is refused the same way.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files. A credential a response carries under a declared
//     name is held by the shared credential kit's transport (credkit-wiring),
//     which this pin wraps.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"salesbuildr-pp-cli/internal/client"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Salesbuildr host")

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
	pin, refusal := pinnedOrigin(c)
	installPin(c.HTTPClient, pin, refusal, c.DryRun)
}

// ErrTenantInvalid marks a request refused because SALESBUILDR_TENANT is not
// one DNS label (or is still the shipped placeholder).
var ErrTenantInvalid = errors.New("request refused: SALESBUILDR_TENANT must be your Salesbuildr subdomain, one DNS label such as acme (from https://acme.salesbuildr.com)")

// shippedPlaceholderTenant is the default the generated config fills in when
// no tenant is set; it names no customer.
const shippedPlaceholderTenant = "your-tenant"

// pinnedOrigin is the origin of the base URL with its template variables
// resolved, or "" with the reason when it cannot be pinned safely.
func pinnedOrigin(c *client.Client) (string, error) {
	var vars map[string]string
	if c.Config != nil {
		vars = c.Config.TemplateVars
	}
	if strings.Contains(c.BaseURL, "{tenant}") && !TenantPinnable(vars["tenant"]) {
		return "", ErrTenantInvalid
	}
	resolved := client.ResolvedBaseURL(c.BaseURL, vars)
	if strings.ContainsAny(resolved, "{}") {
		return "", nil // an unresolved placeholder: buildURL names the variable first
	}
	u, err := url.Parse(resolved)
	if err != nil {
		return "", nil
	}
	return normalizedOrigin(u), nil
}

// TenantPinnable reports whether tenant may fill {tenant} in the base URL: one
// DNS label that is not the shipped placeholder. The credential kit's wiring
// (internal/credwire showBaseURL) shows the kit a tenant host only on this
// same test, so the kit's allowed origin and this pin never differ.
func TenantPinnable(tenant string) bool {
	return isDNSLabel(tenant) && !strings.EqualFold(tenant, shippedPlaceholderTenant)
}

// isDNSLabel reports whether s is one hostname label: 1 to 63 ASCII letters,
// digits or hyphens, not starting or ending with a hyphen.
func isDNSLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')) {
			return false
		}
	}
	return true
}

func installPin(hc *http.Client, pin string, refusal error, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	inner := next
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pin: pin, refusal: refusal, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set SALESBUILDR_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
		}
		if refusal != nil {
			return refusal
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
		return fmt.Errorf("%w (%s); set SALESBUILDR_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
	refusal error // set when the base URL cannot be pinned (a bad tenant): every request refused
	next    http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	err := t.refusal
	if err == nil {
		err = checkPin(t.pin, req)
	}
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
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
