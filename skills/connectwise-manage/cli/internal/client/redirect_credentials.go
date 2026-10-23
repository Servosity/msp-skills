// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// Hand-owned (handfixes.json: redirect-strips-every-credential). The generated
// CheckRedirect deleted only the primary auth header on a hop to another host.
// connectwise-manage therefore forwarded the Basic Authorization header built
// from the public and private keys (carried in Config.Headers) to a server the
// operator never configured.
// Go's own redirect handling strips Authorization and Cookie only when the
// hostname changes (a port change keeps them), never strips a custom header,
// and replays the request body on a 307 or 308.

// redirectCredentialHeaders names every header this client can fill with a
// credential: the standard auth headers plus this connector's own.
var redirectCredentialHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie", "Cookie2", "clientId"}

// redirectLeavesOrigin reports whether the next hop is on a different host or
// port than the first request, or downgrades from https to http. Hosts compare
// the way the network resolves them (sameHost) and an explicit default port
// equals none, so https://API.example.com and https://api.example.com:443 stay
// on the origin.
func redirectLeavesOrigin(next *url.URL, via []*http.Request) bool {
	first := via[0].URL
	if !sameHost(next.Hostname(), first.Hostname()) || explicitPort(next) != explicitPort(first) {
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

// sameHost reports whether two URL hostnames name the same host. An IP literal
// must be the same address, IPv6 zone included and compared exactly (interface
// names are case-sensitive on Linux, so fe80::1%lan0 and fe80::1%LAN0 can be
// different peers). A DNS name matches case-insensitively in ASCII only; a
// name with any non-ASCII byte must match exactly, since Unicode case folding
// is not how a resolver compares names.
func sameHost(a, b string) bool {
	ipA, errA := netip.ParseAddr(a)
	ipB, errB := netip.ParseAddr(b)
	if errA == nil || errB == nil {
		return errA == nil && errB == nil && ipA == ipB
	}
	if !isASCII(a) || !isASCII(b) {
		return a == b
	}
	return strings.EqualFold(a, b)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
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
