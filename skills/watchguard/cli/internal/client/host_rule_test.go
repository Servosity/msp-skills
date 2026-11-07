// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/url"
	"os"
	"testing"
)

// TestClientHostComparatorsKeepZoneCase (handfixes.json
// stored-credential-origin-exact-host): the redirect origin check, the dial
// guard and the configured-proxy check compare hosts by cliutil.SameHost, so an
// IPv6 zone that differs only in case is another link (and another peer), while
// an ASCII DNS name still ignores case.
func TestClientHostComparatorsKeepZoneCase(t *testing.T) {
	mustURL := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	if redirectSameEffectiveOrigin(mustURL("https://[fe80::1%25lan0]/a"), mustURL("https://[fe80::1%25LAN0]/b")) {
		t.Error("redirect from zone lan0 to zone LAN0 treated as same origin")
	}
	if !redirectSameEffectiveOrigin(mustURL("https://API.example.com/a"), mustURL("https://api.example.com:443/b")) {
		t.Error("redirect within one ASCII host treated as leaving the origin")
	}
	if dialTargetAllowed("[fe80::1%LAN0]:443", "[fe80::1%lan0]:443") {
		t.Error("dial to zone LAN0 allowed by a guard pinned to zone lan0")
	}
	if dialTargetAllowed("[fe80::1%LAN0]:443", "fe80::1%lan0") {
		t.Error("dial to zone LAN0 allowed by a bare-host guard pinned to zone lan0")
	}
	if !dialTargetAllowed("API.example.com:443", "api.example.com:443") {
		t.Error("dial to the pinned ASCII host refused over case")
	}
	// Clear every proxy variable first and set HTTPS_PROXY last: Windows
	// environment names ignore case, so clearing https_proxy after setting
	// HTTPS_PROXY would erase the proxy this test configures.
	for _, k := range []string{"HTTP_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(k, "")
	}
	t.Setenv("HTTPS_PROXY", "http://[fe80::1%25lan0]:3128")
	if got := os.Getenv("HTTPS_PROXY"); got == "" {
		t.Fatal("test setup: HTTPS_PROXY is empty after being set")
	}
	if dialAddrIsConfiguredProxy("[fe80::1%LAN0]:3128") {
		t.Error("zone LAN0 dial treated as the proxy configured on zone lan0")
	}
	if !dialAddrIsConfiguredProxy("[fe80::1%lan0]:3128") {
		t.Error("the configured proxy itself not recognized")
	}
}
