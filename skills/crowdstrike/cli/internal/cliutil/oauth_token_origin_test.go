// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"net/url"
	"testing"
)

// TestOAuthTokenSameOriginComparesHostsLikeTheNetwork: an IP literal must be
// the same address with an exact IPv6 zone (Linux interface names are
// case-sensitive, so fe80::1%lan0 and fe80::1%LAN0 can be different peers); a
// DNS name folds ASCII case only; a default port equals none; the scheme must
// match. Hand-owned (handfixes.json: token-request-stays-on-origin).
func TestOAuthTokenSameOriginComparesHostsLikeTheNetwork(t *testing.T) {
	for _, tc := range []struct {
		origin, target string
		same           bool
	}{
		{"https://ID.Example.TEST/token", "https://id.example.test/token2", true},
		{"https://id.example.test/token", "https://id.example.test:443/token", true},
		{"https://id.example.test/token", "http://id.example.test/token", false},
		{"https://id.example.test/token", "https://id.example.test:8443/token", false},
		{"https://id.example.test/token", "https://other.example.test/token", false},
		{"http://[FE80::1%25lan0]:8080/token", "http://[fe80::1%25lan0]:8080/token", true},
		{"http://[fe80::1%25lan0]:8080/token", "http://[fe80::1%25LAN0]:8080/token", false},
		{"http://[fe80::1%25lan0]:8080/token", "http://[fe80::1]:8080/token", false},
		{"https://k.example.test/token", "https://\u212a.example.test/token", false},
	} {
		origin, err := url.Parse(tc.origin)
		if err != nil {
			t.Fatal(err)
		}
		target, err := url.Parse(tc.target)
		if err != nil {
			t.Fatal(err)
		}
		if got := oauthTokenSameOrigin(target, origin); got != tc.same {
			t.Errorf("%s -> %s: same origin = %v, want %v", tc.origin, tc.target, got, tc.same)
		}
	}
}
