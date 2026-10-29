// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-vsa handfixes applied to every API client (CLI and MCP) through
// registerClientHook:
//
//   - api-origin-pinned: every request, redirect hops included, must go to
//     the scheme, host and port the configuration resolved when the client was
//     built. Nothing given later (sync --path-context, an MCP argument, a
//     Location header) can move the operator's Basic credentials to another
//     host. To use another server, set KASEYA_VSA_SERVER.
//   - no-response-cache: the on-disk response cache is off. VSA bodies carry
//     custom-field values, webhook signing keys and Windows product keys; the
//     local store keeps them out (store/kaseya_secrets.go), and raw bodies
//     must not sit in cache files either.
//   - chrome-transport-plain-http-h1: a plain-http base URL is served over
//     HTTP/1.1 (client/kaseya_plain_http.go).

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"kaseya-vsa-pp-cli/internal/client"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured VSA 10 server")

func init() {
	registerClientHook(func(c *client.Client) error {
		if c == nil || c.HTTPClient == nil {
			return nil
		}
		c.NoCache = true
		client.ServePlainHTTPOverH1(c.HTTPClient)
		var vars map[string]string
		if c.Config != nil {
			vars = c.Config.TemplateVars
		}
		next := c.HTTPClient.Transport
		if next == nil {
			next = http.DefaultTransport
		}
		origin := pinnedOrigin(c.BaseURL, vars)
		c.HTTPClient.Transport = originPinTripper{origin: origin, next: next}
		// The generated redirect check resolves an off-origin hop's host in
		// DNS; a query is itself a disclosure, so the pin runs first.
		generated := c.HTTPClient.CheckRedirect
		c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if origin == "" || normalizedOrigin(req.URL) != origin {
				return fmt.Errorf("%w (%s, not %s); set KASEYA_VSA_SERVER to change servers", ErrOriginChanged, displayOrigin(req.URL), origin)
			}
			if generated != nil {
				return generated(req, via)
			}
			return nil
		}
		return nil
	})
}

// pinnedOrigin resolves {placeholders} in the base URL from the configured
// template vars and returns its normalized origin, or "" when it cannot be
// resolved (every request is then refused).
func pinnedOrigin(base string, vars map[string]string) string {
	for k, v := range vars {
		base = strings.ReplaceAll(base, "{"+k+"}", v)
	}
	if strings.ContainsAny(base, "{}") {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return normalizedOrigin(u)
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
	origin string
	next   http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.origin == "" || normalizedOrigin(req.URL) != t.origin {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("%w (%s, not %s); set KASEYA_VSA_SERVER to change servers", ErrOriginChanged, displayOrigin(req.URL), t.origin)
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

// bodyForError renders a response body for an error message: credentials are
// masked before truncation, so a cut can never leave part of one in clear.
func bodyForError(c *client.Client, raw []byte) string {
	text := c.MaskCredentials(string(raw))
	if len(text) > 300 {
		return text[:300] + "..."
	}
	return text
}

// failureText is the text of a per-section failure that a command prints.
func failureText(c *client.Client, err error) string {
	return c.MaskCredentials(err.Error())
}
