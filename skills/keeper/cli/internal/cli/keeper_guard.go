// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// keeper handfixes applied to every API client (CLI, novel fan-out and
// MCP) through registerClientHook:
//
//   - api-origin-pinned: every request must go to the scheme, host and port
//     the configuration resolved when the client was built: the Service Mode
//     base URL (KEEPER_BASE_URL or the config file, default
//     https://localhost:9090). Nothing given later (a Location header, an MCP
//     argument, a path) can move the api-key to another host. No redirect is
//     followed at all, same host included: Service Mode never redirects.
//   - no-plaintext-key: a plain http base URL is refused before a request, on
//     every host, loopback included: the credential kit sends no held value over
//     plain http, and this says why in words an operator can act on.
//   - no-response-cache: the on-disk response cache is off. Raw Service Mode
//     bodies must not sit in cache files.
//
// Beneath the pin, every request passes the credential kit's outbound guard
// (handfixes.json credkit-wiring).

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"keeper-pp-cli/internal/client"
	"keeper-pp-cli/internal/cliutil"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Keeper Service Mode host")

// ErrRedirect marks a redirect answer; none is ever followed.
var ErrRedirect = errors.New("request refused: Service Mode answered with a redirect, which this CLI never follows")

// ErrPlaintextKey marks a plain-http base URL that is not loopback.
var ErrPlaintextKey = errors.New("request refused: KEEPER_BASE_URL uses plain http; run Service Mode with TLS and use https (KEEPER_CA_CERT trusts a private certificate), so the api-key is never sent in clear")

// loopbackPlainHTTP lets this package's in-process tests talk to a plain httptest
// server. Production code never sets it: the built binaries refuse plain http
// (internal/keeperblackbox), and the kit would refuse the key regardless.
var loopbackPlainHTTP = false

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
	origins := pinnedOrigins(c.BaseURL)
	c.HTTPClient.Transport = originPinTripper{origins: origins, next: cliutil.OutboundGuard(c.HTTPClient.Transport)}
	// No redirect is followed, same origin included; the pin runs first so
	// an off-origin hop's host is never even resolved.
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkPinned(origins, req.URL); err != nil {
			return err
		}
		return ErrRedirect
	}
}

// pinnedOrigins returns the normalized origin of each configured URL; an
// unparseable one pins nothing (its requests are refused).
func pinnedOrigins(urls ...string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range urls {
		if u, err := url.Parse(raw); err == nil {
			if o := normalizedOrigin(u); o != "" {
				out[o] = true
			}
		}
	}
	return out
}

func checkPinned(origins map[string]bool, u *url.URL) error {
	if u != nil && asciiLower(u.Scheme) == "http" && !(loopbackPlainHTTP && net.ParseIP(u.Hostname()).IsLoopback()) {
		return ErrPlaintextKey
	}
	if o := normalizedOrigin(u); o == "" || !origins[o] {
		return fmt.Errorf("%w (%s); set KEEPER_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(u))
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
	origins map[string]bool
	next    http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkPinned(t.origins, req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
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
