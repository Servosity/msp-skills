// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Hand-owned (handfixes.json: redirect-strips-every-credential). The generated
// CheckRedirect deleted only the primary auth header on a hop to another host.
// sherweb therefore forwarded the Ocp-Apim-Subscription-Key header
// (SHERWEB_SUBSCRIPTION_KEY) to a server the operator never configured.
// Go's own redirect handling strips Authorization and Cookie only when the
// hostname changes (a port change keeps them), never strips a custom header,
// and replays the request body on a 307 or 308.

// redirectCredentialHeaders names every header this client can fill with a
// credential: the standard auth headers plus this connector's own.
var redirectCredentialHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie", "Cookie2", "Ocp-Apim-Subscription-Key"}

// redirectLeavesOrigin reports whether the next hop is on a different host or
// port than the first request, or downgrades from https to http. Hostnames
// compare case-insensitively and an explicit default port equals none, so
// https://API.example.com and https://api.example.com:443 stay on the origin.
func redirectLeavesOrigin(next *url.URL, via []*http.Request) bool {
	first := via[0].URL
	if !strings.EqualFold(next.Hostname(), first.Hostname()) || explicitPort(next) != explicitPort(first) {
		return true
	}
	if strings.EqualFold(next.Scheme, "https") {
		return false
	}
	for _, hop := range via {
		if strings.EqualFold(hop.URL.Scheme, "https") {
			return true
		}
	}
	return false
}

// explicitPort is the URL's port, with the scheme's default port treated as
// absent.
func explicitPort(u *url.URL) string {
	p := u.Port()
	if (p == "443" && strings.EqualFold(u.Scheme, "https")) || (p == "80" && strings.EqualFold(u.Scheme, "http")) {
		return ""
	}
	return p
}

// stripRedirectCredentials removes every credential header, and every header
// the config file added, before a hop that leaves the origin. Config-file
// headers exist for the configured origin only, whatever their name: a
// name-keyword rule would not recognise "clientId" as a secret.
func (c *Client) stripRedirectCredentials(h http.Header) {
	for _, k := range redirectCredentialHeaders {
		h.Del(k)
	}
	if c.Config != nil {
		for k := range c.Config.Headers {
			h.Del(k)
		}
	}
}

// refuseCrossOriginBodyReplay refuses a hop that leaves the origin while
// keeping the method: Go then replays the body, and a token request or a write
// body can carry a secret. (A 301, 302 or 303 turns the request into a GET
// with no body.)
func refuseCrossOriginBodyReplay(req *http.Request) error {
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		return nil
	}
	return fmt.Errorf("refusing to replay a %s request body to another origin (%s)", req.Method, req.URL.Host)
}

// RefuseCrossOriginRedirect is a CheckRedirect for a client that carries a
// credential and never needs to leave the configured origin, such as the token
// request in `auth login`.
func RefuseCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if redirectLeavesOrigin(req.URL, via) {
		return fmt.Errorf("refusing a redirect to another origin (%s)", req.URL.Host)
	}
	return nil
}
