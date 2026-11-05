// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (netsapiens; handfixes.json netsapiens-credential-goes-only-to-the-configured-server):
// the credential-routing chokepoint. A client is pinned, when it is built, to
// the scheme, host and port of the server it was configured for (the base URL
// with its template vars resolved: NETSAPIENS_SERVER, NETSAPIENS_BASE_URL or
// the config). Every request it sends, and every redirect hop, must stay on
// that origin; anything else is refused before a connection is made. A
// template var or base URL changed afterwards (--path-context server=, any
// argument) therefore cannot carry the credential, the config headers or the
// request body to another host.

package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"netsapiens-pp-cli/internal/config"
)

// ErrOffServerRequest refuses a request or redirect hop whose scheme, host or
// port is not the configured server's. It is terminal: a retry cannot change it.
var ErrOffServerRequest = errors.New("refusing request: its scheme, host or port is not the configured NetSapiens server (set NETSAPIENS_SERVER or NETSAPIENS_BASE_URL to change the server)")

// pinnedOrigin returns "scheme://host:port" for the server cfg names, or ""
// when it cannot be resolved (every request is then refused).
func pinnedOrigin(baseURL string, cfg *config.Config) string {
	var vars map[string]string
	if cfg != nil {
		vars = cfg.TemplateVars
	}
	resolved, err := buildURL(strings.TrimRight(baseURL, "/"), "", vars)
	if err != nil {
		return ""
	}
	return originKey(resolved)
}

func originKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	scheme, host, port, ok := redirectEffectiveOrigin(u)
	if !ok {
		return ""
	}
	return scheme + "://" + strings.ToLower(strings.TrimSuffix(host, ".")) + ":" + port
}

// refuseOffServer is nil when raw is on the pinned origin.
func (c *Client) refuseOffServer(raw string) error {
	if c == nil {
		return ErrOffServerRequest
	}
	if c.serverOrigin == "" {
		return fmt.Errorf("%w: the client has no resolved server", ErrOffServerRequest)
	}
	if got := originKey(raw); got == "" || got != c.serverOrigin {
		return ErrOffServerRequest
	}
	return nil
}

// doPinned sends req only when it, and every redirect hop after it, stays on
// the pinned origin. The check wraps whichever http.Client is in use, so a
// replaced or cloned client (streaming, upload) is pinned too.
func (c *Client) doPinned(hc *http.Client, req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, ErrOffServerRequest
	}
	if err := c.refuseOffServer(req.URL.String()); err != nil {
		return nil, err
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	// Hand-fix (handfixes.json credkit-wiring): every request leaves through the
	// credential kit's outbound guard, inside this origin pin.
	pinned := *hc
	pinned.Transport = OutboundGuard(hc.Transport)
	inner := hc.CheckRedirect
	pinned.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next == nil || next.URL == nil {
			return ErrOffServerRequest
		}
		if err := c.refuseOffServer(next.URL.String()); err != nil {
			return err
		}
		if inner != nil {
			return inner(next, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return pinned.Do(req)
}
