// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "net/url"

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
