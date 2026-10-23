// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ncauth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// redirectLeavesOrigin reports whether the next hop is on a different
// host:port than the first request, or downgrades from https to http.
func redirectLeavesOrigin(next *url.URL, via []*http.Request) bool {
	if next.Host != via[0].URL.Host {
		return true
	}
	if next.Scheme == "https" {
		return false
	}
	for _, hop := range via {
		if hop.URL.Scheme == "https" {
			return true
		}
	}
	return false
}
