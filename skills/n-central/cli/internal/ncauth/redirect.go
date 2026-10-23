// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ncauth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// refuseCrossOriginRedirect keeps every request of this client on the
// configured origin. Every request it sends carries a credential:
// the User-API JWT, the access token, or the refresh token (in the header and
// in the body).
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
