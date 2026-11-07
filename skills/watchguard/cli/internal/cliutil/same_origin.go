// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"net/netip"
	"net/url"
	"strings"
)

// SameOrigin is the one origin comparison every credential decision uses
// (hand-fix, handfixes.json: scoped-bearer-stays-on-origin): scheme, host and
// canonical port, with hosts compared the way the network resolves them. An IP
// literal must be the same address with the IPv6 zone compared exactly
// (fe80::1%lan0 and fe80::1%LAN0 are different links on Linux), a DNS name
// matches ASCII case-insensitively, and a name with any non-ASCII byte must
// match exactly. The token client's redirect check, the API client's
// write-replay check and the audience-scope transport all call it, so no path
// folds a host the network keeps apart.
func SameOrigin(target, origin *url.URL) bool {
	return oauthTokenSameOrigin(target, origin)
}

// SameHost is SameOrigin's host rule for callers that hold bare hostnames (a
// dial address, a redirect hop, a proxy). Hand-written (handfixes.json
// stored-credential-origin-exact-host).
func SameHost(a, b string) bool {
	return oauthTokenSameHost(a, b)
}

// CanonicalHost returns a key for hostname h such that CanonicalHost(a) ==
// CanonicalHost(b) exactly when SameHost(a, b): an IP literal in netip form
// with its zone as written, an ASCII DNS name lowercased, and a name with any
// non-ASCII byte unchanged. Hand-written (handfixes.json
// stored-credential-origin-exact-host); NormalizeCredentialOrigin keys stored
// credentials with it.
func CanonicalHost(h string) string {
	if ip, err := netip.ParseAddr(h); err == nil {
		return ip.String()
	}
	for i := 0; i < len(h); i++ {
		if h[i] >= 0x80 {
			return h
		}
	}
	return strings.ToLower(h)
}
