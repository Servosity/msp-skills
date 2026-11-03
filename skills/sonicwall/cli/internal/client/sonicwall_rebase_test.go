// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "testing"

// A SONICWALL_BASE_URL override moves NSM only; MySonicWall keeps its own host
// unless the override is a loopback test server (handfix credential-host-pin).
func TestBaseOverrideNeverMovesMySonicWall(t *testing.T) {
	const msw = "https://api.mysonicwall.com/api/hgms/get-cloud-tenants"
	for _, base := range []string{"https://nsm-uswest.sonicwall.com/api/manager", "https://nsm.example.net/api/manager"} {
		got, err := rebaseAbsoluteURLOntoOverride(msw, base, nil)
		if err != nil || got != msw {
			t.Errorf("base %s: got %q %v, want MySonicWall untouched", base, got, err)
		}
	}
	got, err := rebaseAbsoluteURLOntoOverride(msw, "http://127.0.0.1:8080", nil)
	if err != nil || got != "http://127.0.0.1:8080/api/hgms/get-cloud-tenants" {
		t.Errorf("loopback base: got %q %v, want rebased onto the test server", got, err)
	}
}
