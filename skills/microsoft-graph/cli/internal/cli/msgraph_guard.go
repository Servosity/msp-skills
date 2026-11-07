// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// microsoft-graph handfixes applied to every API client (CLI, novel
// fan-out and MCP) through registerClientHook:
//
//   - api-origin-pinned: every request, redirect hops and @odata.nextLink
//     pages included, must go to the scheme, host and port the configuration
//     resolved when the client was built: the API base URL
//     (MICROSOFT_GRAPH_BASE_URL or the config file, default
//     https://graph.microsoft.com/v1.0). Nothing given later (a Location
//     header, a nextLink, an MCP argument, a path) can move the bearer token
//     to another host. 'auth login' sends the client secret only to the Entra
//     token endpoint and follows no redirect (auth_login.go).
//   - no-response-cache: the on-disk response cache is off. Raw Graph
//     bodies must not sit in cache files.
//   - credkit-wiring: under the pin, every request leaves through the
//     credential kit's outbound guard (client.OutboundGuard), which holds the
//     declared request and response fields (internal/credwire) before any
//     command reads them and refuses a held value off its allowed origin.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"microsoft-graph-pp-cli/internal/client"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Microsoft Graph API host")

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
	next := c.HTTPClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	next = client.OutboundGuard(next)
	c.HTTPClient.Transport = originPinTripper{origins: origins, next: next}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkPinned(origins, req.URL); err != nil {
			return err
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
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
	if o := normalizedOrigin(u); o == "" || !origins[o] {
		return fmt.Errorf("%w (%s); set MICROSOFT_GRAPH_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(u))
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
