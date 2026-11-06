// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// dropsuite handfixes applied to every API client (CLI, novel fan-out and
// MCP) through ONE registerClientHook, so their order is fixed rather than
// left to file init order:
//
//   - api-origin-pinned: every request and redirect hop must go to the
//     scheme, host and port the configuration resolved when the client was
//     built: the partner API base URL (DROPSUITE_BASE_URL, or
//     https://<DROPSUITE_HOST>/api, default host dropsuite.us). Nothing given
//     later (a Location header, an MCP argument, a path) can move the reseller
//     or an organization token to another host.
//   - no-response-cache: the on-disk response cache is off. Raw bodies (GET
//     /users carries every organization's authentication_token) must not sit
//     in cache files. The shared credential kit (handfixes.json
//     credkit-wiring) holds that token as a declared response field beneath
//     the pin, and its SQLite driver refuses a statement that repeats it.
//   - mcp-write-confirm (Codex review round 1 class
//     read-call-reaches-a-mint-route): only a call the client issued as a
//     mutation (cliutil.MarkMutation: GetMutating) reaches a route that mints
//     a portal sign-in link, in whatever spelling a path argument gives it
//     (organizations get generate_redirect_link, a format suffix, another
//     case, a redirect hop); and no request goes out whose path the API could
//     route other than it reads here (a path argument holding a slash,
//     backslash, percent sign, semicolon, dot segment or empty segment).
//
// The pin sits directly above the kit's transport (client.OutboundGuard).
// Above the pin, in this order: the --org
// token override (dropsuite_org.go), the response redaction transport
// (dropsuite_org.go) and the organization-token routing transport
// (dropsuite_routing.go), whose directory loads reuse the pinned transport.

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = fmt.Errorf("%w: it would leave the configured Dropsuite API host", cliutil.ErrRequestRefused)

func init() {
	registerClientHook(func(c *client.Client) error {
		if c == nil || c.HTTPClient == nil || pinnedChain(c.HTTPClient.Transport) {
			return nil // already guarded (a hook ran twice on one client)
		}
		guardClient(c)
		applyOrgOverride(c)
		installRedaction(c)
		installOrgRouting(c)
		return nil
	})
}

// pinnedChain reports whether the origin pin is already in a transport chain
// this file builds (routing over redaction over the pin).
func pinnedChain(rt http.RoundTripper) bool {
	for {
		switch t := rt.(type) {
		case originPinTripper:
			return true
		case orgRoutingTransport:
			rt = t.next
		case redactingTransport:
			rt = t.next
		default:
			return false
		}
	}
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(originPinTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	var vars map[string]string
	if c.Config != nil {
		vars = c.Config.TemplateVars
	}
	// The default base URL is a template (https://{host}/api): pin the host it
	// resolves to. An unresolved template pins nothing, so every request fails.
	resolved := client.ResolvedBaseURL(c.BaseURL, vars)
	origins := pinnedOrigins(resolved)
	next := c.HTTPClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	c.HTTPClient.Transport = originPinTripper{origins: origins, base: basePathOfURL(resolved), next: next}
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
		return fmt.Errorf("%w (%s); set DROPSUITE_HOST or DROPSUITE_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(u))
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
	base    string // the API base URL's escaped path, without a trailing slash
	next    http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	err := checkPinned(t.origins, req.URL)
	if err == nil {
		err = checkRoute(t.base, req)
	}
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
}

// ErrMintRoute marks a request refused because it would reach a route that
// mints a portal sign-in link without being issued as a mutation.
var ErrMintRoute = fmt.Errorf("%w: it would reach a Dropsuite route that mints a portal sign-in link, and it was not issued as that call (use portal get-login-link or organizations get-support-sso-link)", cliutil.ErrRequestRefused)

// ErrAmbiguousRoute marks a request refused because a path segment could be
// routed by the API other than it reads here.
var ErrAmbiguousRoute = fmt.Errorf("%w: a path argument holds a slash, backslash, percent sign, semicolon or dot segment the Dropsuite API could route to another endpoint", cliutil.ErrRequestRefused)

// basePathOfURL is the escaped path of a resolved base URL, without a
// trailing slash ("" when it does not parse: such a base pins no origin).
func basePathOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimRight(u.EscapedPath(), "/")
}

// checkRoute refuses a request whose path the API could route other than it
// reads (routeSegments), and one that would reach a sign-in link route
// (mintRoute) unless the client issued it as a mutation.
func checkRoute(base string, req *http.Request) error {
	segs, err := routeSegments(base, req.URL)
	if err != nil {
		return err
	}
	if mintRoute(segs) && !cliutil.MutationIntent(req.Context()) {
		return ErrMintRoute
	}
	return nil
}

// routeSegments returns the request path below the API base path, one
// percent-decoded segment per element. It fails closed on anything a server
// may normalize into another route: a path outside the base path, and a
// segment that is empty (a doubled slash), "." or "..", or that holds a
// slash, backslash, percent sign (a second encoding), semicolon (a path
// parameter), or a control or space character. One trailing slash is
// allowed; it routes the same.
func routeSegments(base string, u *url.URL) ([]string, error) {
	raw := u.EscapedPath()
	if base != "" {
		if raw != base && !strings.HasPrefix(raw, base+"/") {
			return nil, ErrAmbiguousRoute
		}
		raw = raw[len(base):]
	}
	raw = strings.TrimPrefix(raw, "/")
	raw = strings.TrimSuffix(raw, "/")
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "/")
	segs := make([]string, 0, len(parts))
	for _, p := range parts {
		seg, err := url.PathUnescape(p)
		if err != nil || seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "/\\%;") {
			return nil, ErrAmbiguousRoute
		}
		for _, r := range seg {
			if r <= ' ' || r == 0x7f || unicode.IsControl(r) || unicode.IsSpace(r) {
				return nil, ErrAmbiguousRoute
			}
		}
		segs = append(segs, seg)
	}
	return segs, nil
}

// mintRoute reports a path that names a sign-in link route: any segment
// that, case-folded and without a format suffix (generate_redirect_link.json)
// or its separators, starts with "generate". Dropsuite's two such routes are
// /users/generate_redirect_link and /users/generate_support_sso_link; any
// other generate action is treated the same (fail closed).
func mintRoute(segs []string) bool {
	for _, seg := range segs {
		if i := strings.IndexByte(seg, '.'); i >= 0 {
			seg = seg[:i]
		}
		norm := strings.Map(func(r rune) rune {
			if r == '_' || r == '-' {
				return -1
			}
			return unicode.ToLower(r)
		}, seg)
		if strings.HasPrefix(norm, "generate") {
			return true
		}
	}
	return false
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
