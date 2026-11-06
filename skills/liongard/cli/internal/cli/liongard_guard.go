// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// liongard handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: the X-ROAR-API-KEY header and every API request go
//     only to the scheme, host and port of the resolved API base URL
//     (LIONGARD_BASE_URL or the config file, default
//     https://{instance}.app.liongard.com/api/v1 with LIONGARD_INSTANCE). Every
//     redirect hop must stay on that origin. Nothing given later (a Location
//     header, an MCP argument, a path) can move the credential to another host.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files; the local store keeps credential fields out
//     (store/liongard_secrets.go).
//   - credkit-wiring: under the pin, every request passes the shared
//     credential kit's transport (client.OutboundGuard), which refuses a held
//     credential to any origin but the base URL's, over plain http or across
//     a cross-origin redirect, and holds every credential a response carries
//     (declared response fields, every Set-Cookie value) before the body
//     reaches any command. The pin stays: the kit lets an uncredentialed
//     request reach another host, the pin does not.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"liongard-pp-cli/internal/client"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Liongard host")

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
	// The shipped base URL is per instance (https://{instance}.app.liongard.com/api/v1):
	// pin the RESOLVED origin. An unresolved template does not parse to an
	// origin, so the pin is "" and every request is refused (fail closed).
	pin := ""
	if u, err := url.Parse(client.ResolvedBaseURL(c.BaseURL, c.CurrentTemplateVars())); err == nil {
		pin = normalizedOrigin(u)
	}
	installPin(c.HTTPClient, pin, c.DryRun)
}

func installPin(hc *http.Client, pin string, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	inner := client.OutboundGuard(next)
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pin: pin, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set LIONGARD_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
		return fmt.Errorf("%w (%s); set LIONGARD_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
	pin  string
	next http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkPin(t.pin, req); err != nil {
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
