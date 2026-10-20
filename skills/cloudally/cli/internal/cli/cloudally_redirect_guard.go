// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: keep request bodies on the API origin across redirects.
//
// The generated client's redirect policy drops Authorization, client-id and
// client-secret when a hop leaves the origin, but a 307 or 308 still makes
// net/http re-send the request body to the new host. Several CloudAlly
// bodies are secrets in their own right: `session login` and
// `session partner-login` post the email and password, `session refresh`
// posts the refresh token, `user update-password` posts a password. The
// generated commands and the MCP tools reach these endpoints through that
// client, so a redirect to another host would hand it the secret. This hook
// wraps the generated policy (on CLI and MCP clients alike, through
// ApplyClientHooks) and refuses any hop that would carry a body, or keep a
// non-GET method, to a different origin. A same-origin redirect is
// unaffected, and so is a GET or HEAD, which carries no body.

package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cloudally-pp-cli/internal/client"
)

// errBodyRedirectRefused is returned when a redirect would re-send a request
// body to another origin.
var errBodyRedirectRefused = errors.New("refusing to re-send a request body to another origin after a redirect")

func init() {
	registerClientHook(cloudallyRedirectGuardHook)
}

// cloudallyRedirectGuardHook installs the body-replay guard on c.
func cloudallyRedirectGuardHook(c *client.Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	c.HTTPClient.CheckRedirect = guardBodyRedirects(c.HTTPClient.CheckRedirect)
	return nil
}

// guardBodyRedirects runs the existing policy, then refuses a cross-origin
// hop that carries a body or keeps a method other than GET or HEAD.
func guardBodyRedirects(inner func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if inner != nil {
			if err := inner(req, via); err != nil {
				return err
			}
		}
		if len(via) == 0 || req.URL == nil {
			return nil
		}
		carriesBody := req.Body != nil && req.Body != http.NoBody
		if !carriesBody && (req.Method == http.MethodGet || req.Method == http.MethodHead) {
			return nil
		}
		for _, hop := range via {
			if hop != nil && !sameRequestOrigin(hop.URL, req.URL) {
				return fmt.Errorf("%w: %s %s redirected to %s", errBodyRedirectRefused, req.Method, requestOrigin(via[0].URL), requestOrigin(req.URL))
			}
		}
		return nil
	}
}

// sameRequestOrigin compares scheme, host and effective port.
func sameRequestOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		requestPort(a) == requestPort(b)
}

func requestPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// requestOrigin renders scheme://host only; a path or query can carry a
// token.
func requestOrigin(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}
