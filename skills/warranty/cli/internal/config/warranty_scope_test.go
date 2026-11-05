// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json warranty-store): the generated store is scoped by
// every vendor's identity, not Dell's alone.

package config

import "testing"

// Apple's Get Organization Devices answers for the authenticated organization.
// Input: Dell unset, two Apple organizations (or two Lenovo keys) in turn. Each
// must get its own store scope; the same identity keeps its scope.
func TestStoreScopeFollowsEveryVendorIdentity(t *testing.T) {
	scope := func(env map[string]string) string {
		for _, k := range []string{"WARRANTY_DELL_CLIENT_ID", "WARRANTY_DELL_CLIENT_SECRET", "WARRANTY_APPLE_CLIENT_ID", "WARRANTY_LENOVO_CLIENT_KEY"} {
			t.Setenv(k, env[k])
		}
		return (&Config{}).StoreScopeCredential()
	}
	a := scope(map[string]string{"WARRANTY_APPLE_CLIENT_ID": "BUSINESSAPI.org-a"})
	b := scope(map[string]string{"WARRANTY_APPLE_CLIENT_ID": "BUSINESSAPI.org-b"})
	if a == "" || a == b {
		t.Errorf("two Apple organizations share a store scope: %q %q", a, b)
	}
	if again := scope(map[string]string{"WARRANTY_APPLE_CLIENT_ID": "BUSINESSAPI.org-a"}); again != a {
		t.Errorf("the same organization moved scope: %q %q", a, again)
	}
	l1 := scope(map[string]string{"WARRANTY_LENOVO_CLIENT_KEY": "key-one"})
	l2 := scope(map[string]string{"WARRANTY_LENOVO_CLIENT_KEY": "key-two"})
	if l1 == "" || l1 == l2 {
		t.Errorf("two Lenovo keys share a store scope: %q %q", l1, l2)
	}
	d := map[string]string{"WARRANTY_DELL_CLIENT_ID": "dell-id", "WARRANTY_DELL_CLIENT_SECRET": "dell-secret"}
	da := scope(map[string]string{"WARRANTY_DELL_CLIENT_ID": d["WARRANTY_DELL_CLIENT_ID"], "WARRANTY_DELL_CLIENT_SECRET": d["WARRANTY_DELL_CLIENT_SECRET"], "WARRANTY_APPLE_CLIENT_ID": "BUSINESSAPI.org-a"})
	db := scope(map[string]string{"WARRANTY_DELL_CLIENT_ID": d["WARRANTY_DELL_CLIENT_ID"], "WARRANTY_DELL_CLIENT_SECRET": d["WARRANTY_DELL_CLIENT_SECRET"], "WARRANTY_APPLE_CLIENT_ID": "BUSINESSAPI.org-b"})
	if da == db {
		t.Errorf("with Dell set, two Apple organizations share a store scope")
	}
}
