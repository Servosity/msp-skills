// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package coverpc

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// refuseCrossOriginRedirect keeps every request of this client on the
// configured origin. Every JSON-RPC call is a POST whose body
// carries the password (Login) or the session visa (every other method).
// Go's default redirect handling keeps Authorization on a hop that changes only
// the port, and replays the request body on a 307 or 308, so a hop to another
// host:port (or from https down to http) would hand the credential to a server
// the operator never configured. Same-origin hops still follow, up to Go's
// usual limit of 10.
func refuseCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if redirectLeavesOrigin(req.URL, via) {
		return fmt.Errorf("refusing a redirect to another origin (%s)", req.URL.Host)
	}
	return nil
}

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
