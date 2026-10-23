// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ncauth

import (
	"net/http"
	"testing"
)

// TestRedirectLeavesOriginComparesHostsLikeTheNetwork: an IP literal must
// match exactly, IPv6 zone included (Linux interface names are case-sensitive,
// so fe80::1%lan0 and fe80::1%LAN0 can be different peers); a DNS name matches
// case-insensitively in ASCII only (a non-ASCII host never folds); an explicit
// default port equals none; https down to http leaves the origin.
func TestRedirectLeavesOriginComparesHostsLikeTheNetwork(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		leaves   bool
	}{
		{"https://API.Example.TEST/a", "https://api.example.test/b", false},
		{"https://api.example.test/a", "https://api.example.test:443/b", false},
		{"http://api.example.test/a", "https://api.example.test/b", false},
		{"https://api.example.test/a", "http://api.example.test/b", true},
		{"https://api.example.test/a", "https://api.example.test:8443/b", true},
		{"https://api.example.test/a", "https://elsewhere.example.test/b", true},
		{"https://10.0.0.1/a", "https://10.0.0.2/b", true},
		{"http://[FE80::1%25lan0]:8080/a", "http://[fe80::1%25lan0]:8080/b", false},
		{"http://[fe80::1%25lan0]:8080/a", "http://[fe80::1%25LAN0]:8080/b", true},
		{"http://[fe80::1%25lan0]:8080/a", "http://[fe80::1]:8080/b", true},
		{"https://k.example.test/a", "https://\u212a.example.test/b", true},
	} {
		first, err := http.NewRequest(http.MethodGet, tc.from, nil)
		if err != nil {
			t.Fatal(err)
		}
		next, err := http.NewRequest(http.MethodGet, tc.to, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := redirectLeavesOrigin(next.URL, []*http.Request{first}); got != tc.leaves {
			t.Errorf("%s -> %s: leaves origin = %v, want %v", tc.from, tc.to, got, tc.leaves)
		}
	}
}
