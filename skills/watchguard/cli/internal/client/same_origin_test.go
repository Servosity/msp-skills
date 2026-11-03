// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/url"
	"testing"
)

// hand-fix guard (handfixes.json redirect-refuses-cross-origin-body): a write
// is replayed only to the exact origin; an http -> https upgrade or a port
// change on the same host is another origin.
func TestSameOriginIsStrict(t *testing.T) {
	u := func(s string) *url.URL { v, _ := url.Parse(s); return v }
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"https://api.example/x", "https://api.example/y", true},
		{"https://api.example/x", "https://API.example:443/y", true},
		{"http://api.example/x", "https://api.example/x", false},
		{"https://api.example/x", "https://api.example:8443/x", false},
		{"https://api.example/x", "https://evil.example/x", false},
	} {
		if got := sameOrigin(u(tc.a), u(tc.b)); got != tc.same {
			t.Errorf("sameOrigin(%s, %s) = %v, want %v", tc.a, tc.b, got, tc.same)
		}
	}
}
