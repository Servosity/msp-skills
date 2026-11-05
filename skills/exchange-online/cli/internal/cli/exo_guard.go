// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// exchange-online handfixes applied to every API client (CLI, novel
// fan-out and MCP) through registerClientHook:
//
//   - api-origin-pinned: every request, redirect hops and @odata.nextLink
//     pages included, must go to the scheme, host and port the configuration
//     resolved when the client was built: the API base URL
//     (EXCHANGE_ONLINE_BASE_URL or the config file, default
//     https://outlook.office365.com). Nothing given later (a Location header,
//     a nextLink, an MCP argument, a path) can move the bearer token to
//     another host. The token is minted outside this program, so there is no
//     token endpoint to pin.
//   - no-response-cache: the on-disk response cache is off. Raw cmdlet
//     bodies must not sit in cache files; the local store keeps credential
//     fields out (store/exo_secrets.go).
//   - response-credentials-held: every credential a request body or a
//     response carries is held with the credential kit before the request is
//     sent or the body reaches any command. A response then has those members
//     replaced by "[redacted]", so a cmdlet listing stays readable rather than
//     being withheld whole by the kit.
//   - credkit-wiring: the kit's outbound guard is the transport under all of
//     this, so it judges exactly what reaches the wire.
//   - request-read-as-the-kit-reads-it: every request reaches that guard in the
//     one form it reads (cliutil.KitReadableRequest), or fails aloud here.
//
// The guard is installed under the cmdlet transport (exo_transport.go): the
// envelope transport wraps the pinned one, so each page it sends is pinned.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Exchange Online API host")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return installEXOTransport(c)
	})
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(*exoTransport); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	origins := pinnedOrigins(c.BaseURL)
	next := cliutil.OutboundGuard(c.HTTPClient.Transport)
	c.HTTPClient.Transport = originPinTripper{origins: origins, next: responseHoldTripper{next: next}}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkPinned(origins, req.URL); err != nil {
			return err
		}
		// The kit holds a URL's user name and password on sight, so a Location could
		// choose what every later unit may say.
		if len(via) > 0 && req.URL.User.String() != via[0].URL.User.String() {
			return fmt.Errorf("refusing a redirect that carries a user name or password: %w", cliutil.ErrUnreadableRequest)
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
		return fmt.Errorf("%w (%s); set EXCHANGE_ONLINE_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(u))
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
	// Every API request passes here, redirect hops included: a URL a server wrote
	// or an operator set never reaches the kit in a form it shuts down on.
	out, err := cliutil.KitReadableRequest(req)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(out)
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

// responseHoldTripper reads each response (under the client's size cap),
// decodes a copy as the client will (Content-Encoding, then BOM and JSONP/XSSI
// prefixes), holds its credential values with the one shared walker
// (cliutil.HoldCredentialFields: any length, masks excluded) and replaces them
// by name before any command reads it. Request bodies and params are held in the
// client itself (doInternal), before a --dry-run preview or a send.
type responseHoldTripper struct{ next http.RoundTripper }

func (t responseHoldTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	body, rerr := client.ReadBody(resp.Body)
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	plain, derr := client.DecodeBody(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		return resp, nil
	}
	cliutil.HoldCredentialFields(plain)
	if redacted := cliutil.RedactBody(plain); !bytes.Equal(redacted, plain) {
		resp.Body = io.NopCloser(bytes.NewReader(redacted))
		resp.ContentLength = int64(len(redacted))
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
	}
	return resp, nil
}
