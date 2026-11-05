// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// connectwise-control handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: the Basic credential and every API request
//     go only to the scheme, host and port of the configured API base URL
//     (CONNECTWISE_CONTROL_BASE_URL or the config file; there is no usable
//     default). Every
//     redirect hop must stay on that origin. Nothing given later (a Location
//     header, an MCP argument, a path) can move the credential to another host.
//   - no-credential-to-placeholder-host: a request to the shipped placeholder
//     host (config.ShippedPlaceholderHost) is refused here as well as in
//     client.doInternal, in any spelling, with the remedy named.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files. The shared credential kit (handfixes.json
//     credkit-wiring) holds a response's credential fields at the outbound
//     guard, around this pin.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"connectwise-control-pp-cli/internal/client"
	"connectwise-control-pp-cli/internal/config"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured ConnectWise Control host")

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
	// ONE origin: the configured base URL's. A base URL that does not parse to
	// an origin pins "" and every request is refused (fail closed).
	pin := ""
	if u, err := url.Parse(c.BaseURL); err == nil {
		pin = normalizedOrigin(u)
	}
	installPin(c.HTTPClient, pin, c.DryRun)
}

func installPin(hc *http.Client, pin string, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
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
			return fmt.Errorf("%w (a redirect to %s); set CONNECTWISE_CONTROL_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
	if err := config.CheckRequestURL(req.URL); err != nil { // no-credential-to-placeholder-host
		return err
	}
	if o := normalizedOrigin(req.URL); o == "" || o != pin {
		return fmt.Errorf("%w (%s); set CONNECTWISE_CONTROL_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
