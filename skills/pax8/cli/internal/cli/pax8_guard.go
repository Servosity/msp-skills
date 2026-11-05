// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pax8 handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: a credential goes only to the scheme, host and port
//     it is configured for. The bearer token and every API request go to the
//     origin of the API base URL (PAX8_BASE_URL or the config file, default
//     https://api.pax8.com/v1). The client ID and secret go only to the token
//     URL (PAX8_TOKEN_URL or the config file, default
//     https://token-manager.pax8.com/oauth/token), and that origin receives
//     nothing but a POST to that exact path carrying no Authorization header.
//     Every redirect hop must stay on the origin of the request it follows.
//     Nothing given later (a Location header, an MCP argument, a path) can
//     move a credential to another host. 'auth login' builds its own token
//     client; guardTokenClient pins it to the token URL the same way.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.
//
// The pins sit outside the shared credential kit's outbound guard
// (client.OutboundGuard, credkit-wiring), which the client already wraps
// around its transport and which holds what a response carries: a request
// leaves only when both pass. The kit binds each credential to its origin; the
// pins also refuse a request carrying no credential at all, and narrow the
// token host to the one token POST.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"pax8-pp-cli/internal/client"
	"pax8-pp-cli/internal/config"
)

// pax8DefaultMintEndpoint is the token URL the client mints against when none is
// configured (client.go's mintClientCredentials default).
const pax8DefaultMintEndpoint = "https://token-manager.pax8.com/oauth/token"

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Pax8 host")

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return nil
	})
}

// pax8Pins is what one client may reach: any request to api, and to token only
// the token POST itself.
type pax8Pins struct {
	api       string // normalized origin of the API base URL ("" pins nothing)
	token     string // normalized origin of the token URL ("" pins nothing)
	tokenPath string
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(originPinTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	tokenURL := ""
	if c.Config != nil {
		tokenURL = c.Config.TokenURL
	}
	pins := newPax8Pins(c.BaseURL, tokenURL)
	installPins(c.HTTPClient, pins, c.DryRun)
}

// guardTokenClient pins the token-endpoint client 'auth login' builds to the
// token URL alone (it sends no API request) and refuses every request under
// --dry-run.
func guardTokenClient(hc *http.Client, cfg *config.Config, dryRun bool) {
	if hc == nil {
		return
	}
	if _, ok := hc.Transport.(originPinTripper); ok {
		return
	}
	tokenURL := ""
	if cfg != nil {
		tokenURL = cfg.TokenURL
	}
	pins := newPax8Pins("", tokenURL)
	installPins(hc, pins, dryRun)
}

func installPins(hc *http.Client, pins pax8Pins, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = client.OutboundGuard(http.DefaultTransport) // never bare: the kit's guard is under every pin
	}
	inner := next
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pins: pins, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set PAX8_BASE_URL or PAX8_TOKEN_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
		}
		if err := pins.check(req); err != nil {
			return err
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
	}
}

func newPax8Pins(baseURL, tokenURL string) pax8Pins {
	if strings.TrimSpace(tokenURL) == "" {
		tokenURL = pax8DefaultMintEndpoint
	}
	var p pax8Pins
	if baseURL != "" {
		if u, err := url.Parse(baseURL); err == nil {
			p.api = normalizedOrigin(u)
		}
	}
	if u, err := url.Parse(tokenURL); err == nil {
		if p.token = normalizedOrigin(u); p.token != "" {
			p.tokenPath = u.EscapedPath()
		}
	}
	return p
}

func (p pax8Pins) check(req *http.Request) error {
	o := normalizedOrigin(req.URL)
	if o == "" {
		return fmt.Errorf("%w (%s); set PAX8_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
	}
	if p.api != "" && o == p.api {
		return nil
	}
	if p.token != "" && o == p.token {
		if req.Method == http.MethodPost && req.URL.EscapedPath() == p.tokenPath && req.Header.Get("Authorization") == "" {
			return nil
		}
		return fmt.Errorf("%w (%s %s on the token host takes only the token request); set PAX8_TOKEN_URL to change it", ErrOriginChanged, req.Method, displayOrigin(req.URL))
	}
	return fmt.Errorf("%w (%s); set PAX8_BASE_URL or PAX8_TOKEN_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
	pins pax8Pins
	next http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.pins.check(req); err != nil {
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
