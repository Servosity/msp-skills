// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// prtg handfixes applied to every API client (CLI, fleet child and MCP)
// after the TLS setting, from applyPRTGTransport (registerClientHook):
//
//   - api-origin-pinned: every request, redirect hops included, must go to
//     the scheme, host and port the configuration resolved when the client was
//     built (for --core, the registered core's server). Nothing given later
//     (sync --path-context, an MCP argument, a Location header) can move the
//     API key to another host. To use another core, register it or set
//     PRTG_SERVER.
//   - no-response-cache: the on-disk response cache is off. PRTG bodies carry
//     the license key and credential settings; the local store keeps them out
//     (store/prtg_secrets.go), and raw bodies must not sit in cache files.
//   - response-credentials-held: every credential a request body or a
//     response carries is held with the credential kit before the request is
//     sent or the body reaches any command: a password sent to POST /session
//     (flags, --stdin or MCP params), the session token a login mints
//     (LoginResult.token), a new API key (CreateAPIKeyResponse.key), the license
//     key (LicenseInfo.key), a map provider key (GeoMap.apikey). A response then
//     has those members replaced by "[redacted]", so a listing stays readable
//     rather than being withheld whole by the kit.
//   - credkit-wiring: the kit's outbound guard is the transport under all of
//     this, so it judges exactly what reaches the wire.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"prtg-pp-cli/internal/client"
	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured PRTG core")

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	c.NoCache = true
	var vars map[string]string
	if c.Config != nil {
		vars = c.Config.TemplateVars
		fleet.HoldKeys("minted:authorization", c.Config.PrtgApiKey)
	}
	next := client.OutboundGuard(c.HTTPClient.Transport)
	origin := pinnedOrigin(c.BaseURL, vars)
	c.HTTPClient.Transport = originPinTripper{origin: origin, next: responseHoldTripper{next: next}}
	// No redirect is followed, same origin included: the generated redirect
	// hook re-attaches the credential on a same-host hop, and the PRTG v2 API
	// answers its own paths directly. The off-origin message runs first (the
	// generated check would resolve the other host in DNS, itself a disclosure).
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if origin == "" || normalizedOrigin(req.URL) != origin {
			return fmt.Errorf("%w (%s, not %s); register the core or set PRTG_SERVER to change servers", ErrOriginChanged, displayOrigin(req.URL), origin)
		}
		return fmt.Errorf("%w: the core answered with a redirect; point PRTG_SERVER at the address that answers the API directly", ErrRedirectRefused)
	}
}

// ErrRedirectRefused marks a request refused because the core redirected it.
var ErrRedirectRefused = errors.New("request refused: redirects are not followed")

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
		return nil, fmt.Errorf("%w (%s, not %s); register the core or set PRTG_SERVER to change servers", ErrOriginChanged, displayOrigin(req.URL), t.origin)
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

// responseHoldTripper reads each response (under the client's size cap),
// decodes a copy as the client will (Content-Encoding, then BOM and JSONP/XSSI
// prefixes), and holds its credential values with the one shared walker
// (cliutil.HoldCredentialFields: any length, masks excluded) and replaces them
// by name before any command reads it. Request bodies and params are held in the client itself
// (doInternal), before a --dry-run preview or a send. A 2xx response to a
// write whose items report any failure is recorded on the call's action
// record (cliutil.ReportActionFailure; prtg handfix: bulk-action-outcomes).
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
	if req.Method != http.MethodGet && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if _, failed, summary := fleet.ResponseFailure(plain); failed > 0 {
			cliutil.ReportActionFailure(req.Context(), req.Method+" "+req.URL.Path+": "+summary)
		}
	}
	if redacted := cliutil.RedactBody(plain); !bytes.Equal(redacted, plain) {
		resp.Body = io.NopCloser(bytes.NewReader(redacted))
		resp.ContentLength = int64(len(redacted))
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
	}
	return resp, nil
}
