// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// threecx handfixes applied to every API client (CLI, fleet child and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: every request, redirect hops and the OAuth token
//     exchange included (the token client reuses this transport), must go to
//     the scheme, host and port the configuration resolved when the client
//     was built (for --pbx, the registered PBX's FQDN). Nothing given later
//     (sync --path-context, an MCP argument, a Location header) can move the
//     client secret or a bearer token to another host. To use another phone
//     system, register it with 'fleet add' or set THREECX_FQDN.
//   - no-response-cache: the on-disk response cache is off. XAPI bodies carry
//     SIP passwords, provisioning links and API keys; the local store keeps
//     them out (store/threecx_secrets.go), and raw bodies must not sit in
//     cache files.
//   - response-credentials-held: every credential a request body or a
//     response carries is held with the credential kit before the request is
//     sent or the body reaches any command: the access token /connect/token
//     mints, a key GenerateApiKey returns, a password sent in a PATCH body
//     (flags, --stdin or MCP params). A response then has those members replaced
//     by "[redacted]", so a listing that carries a SIP password stays readable
//     rather than being withheld whole by the kit.
//   - credkit-wiring: the kit's outbound guard is the transport under all of
//     this, so it judges exactly what reaches the wire, each redirect hop too.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.
//
// The token-endpoint client 'auth login' builds (root.go oauthHTTPClient,
// one anchored line) gets the same pin and dry-run refusal from
// guardTokenClient: the client secret goes only to the configured phone
// system's origin, whatever token URL a variable, a config file or a
// redirect names.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"threecx-pp-cli/internal/client"
	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/config"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured 3CX phone system")

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() { registerClientHook(guardClient) }

func guardClient(c *client.Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	c.NoCache = true
	var vars map[string]string
	if c.Config != nil {
		vars = c.Config.TemplateVars
		for _, v := range []string{c.Config.ClientSecret, c.Config.AccessToken, c.Config.RefreshToken, c.Config.AuthHeader()} {
			holdKey(v)
		}
	}
	next := client.OutboundGuard(c.HTTPClient.Transport)
	origin := pinnedOrigin(c.BaseURL, vars)
	c.HTTPClient.Transport = originPinTripper{origin: origin, next: sendOrRefuse(next, c.DryRun)}
	c.HTTPClient.CheckRedirect = pinnedRedirect(origin, c.HTTPClient.CheckRedirect)
	return nil
}

// guardTokenClient pins the token-endpoint client 'auth login' builds to the
// origin of the configured base URL (the phone system THREECX_FQDN or --pbx
// names), the origin every API client is pinned to, holds what the token
// endpoint answers, and refuses every request under --dry-run.
func guardTokenClient(hc *http.Client, cfg *config.Config, dryRun bool) {
	if hc == nil {
		return
	}
	base, vars := "", map[string]string(nil)
	if cfg != nil {
		base, vars = strings.TrimRight(cfg.BaseURL, "/"), cfg.TemplateVars
	}
	next := client.OutboundGuard(hc.Transport)
	origin := pinnedOrigin(base, vars)
	hc.Transport = originPinTripper{origin: origin, next: sendOrRefuse(next, dryRun)}
	hc.CheckRedirect = pinnedRedirect(origin, hc.CheckRedirect)
}

// sendOrRefuse is the inner transport: responses held on their way in, or,
// under --dry-run, every request refused.
func sendOrRefuse(next http.RoundTripper, dryRun bool) http.RoundTripper {
	if dryRun {
		return dryRunTripper{}
	}
	return responseHoldTripper{next: next}
}

// pinnedRedirect refuses a redirect hop that leaves origin before the
// generated check runs: that check resolves an off-origin hop's host in DNS,
// and a query is itself a disclosure. It also refuses a hop to a GET that
// mints, grants or allocates (cliutil.StateChangingGET) unless the request the
// command made was one: every confirm gate reads only the first path, so a
// read must not turn into a grant on the way.
func pinnedRedirect(origin string, generated func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if origin == "" || normalizedOrigin(req.URL) != origin {
			return fmt.Errorf("%w (%s, not %s); register the phone system with fleet add or set THREECX_FQDN to change systems", ErrOriginChanged, displayOrigin(req.URL), origin)
		}
		if len(via) > 0 && cliutil.StateChangingGET(req.URL.Path) && !cliutil.StateChangingGET(via[0].URL.Path) {
			return ErrRedirectToStateChange
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
	}
}

// ErrRedirectToStateChange is a redirect from a read to a function that mints,
// grants or allocates.
var ErrRedirectToStateChange = errors.New("refusing a redirect to a function that mints, grants or allocates: run that command itself, with its confirm")

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s)", ErrDryRunSend, req.Method, displayOrigin(req.URL))
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
		return nil, fmt.Errorf("%w (%s, not %s); register the phone system with fleet add or set THREECX_FQDN to change systems", ErrOriginChanged, displayOrigin(req.URL), t.origin)
	}
	return t.next.RoundTrip(req)
}

func displayOrigin(u *url.URL) string {
	if u == nil {
		return "no URL"
	}
	if u.Host == "" {
		return "no host"
	}
	if o := normalizedOrigin(&url.URL{Scheme: u.Scheme, Host: u.Host}); o != "" {
		return o
	}
	return "a non-ASCII host"
}

// holdKey holds a configured credential as given and without a pasted
// "Bearer " prefix.
func holdKey(v string) {
	if v = strings.TrimSpace(v); v == "" {
		return
	}
	cliutil.Hold("minted:authorization", v)
	if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		cliutil.Hold("minted:authorization", strings.TrimSpace(v[7:]))
	}
}

// responseHoldTripper reads each response (under the client's size cap),
// decodes a copy as the client will (Content-Encoding, then BOM and JSONP/XSSI
// prefixes), holds its credential values with the one shared walker
// (cliutil.HoldCredentialFields: masks excluded) and replaces them by name
// before any command reads it. A token exchange is left whole: the client needs
// the token it just minted, which is held all the same. Request bodies and
// params are held in the client itself (doInternal), before a --dry-run preview
// or a send.
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
	cliutil.HoldCredentialFields(plain, cliutil.MintsCredential(req.URL.Path))
	if isTokenExchange(req) {
		return resp, nil
	}
	if redacted := cliutil.RedactBody(plain); !bytes.Equal(redacted, plain) {
		resp.Body = io.NopCloser(bytes.NewReader(redacted))
		resp.ContentLength = int64(len(redacted))
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
	}
	return resp, nil
}

// isTokenExchange reports an OAuth token request: the only form POST this
// program sends (the XAPI takes JSON), whatever path THREECX_TOKEN_URL names on
// the pinned origin.
func isTokenExchange(req *http.Request) bool {
	return req.Method == http.MethodPost && strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/x-www-form-urlencoded")
}
