// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// datto-rmm handfixes users-keyed-on-username and sync-partition-names-pinned:
// a Datto user record carries no id/uid/name (lastName, firstName, username,
// email, ...), so the spec keys /v2/account/users on username (x-resource-id)
// and sync stores every user; the fleet analytics read sites as `account`, so
// the 4.34.2 re-point (account -> components) is pinned back.
func TestUsersKeyOnUsername(t *testing.T) {
	if got := resourceIDFieldOverrides["account-users"]; got != "username" {
		t.Fatalf("sync keys account-users on %q, want username", got)
	}
	id := extractID("account-users", map[string]any{"firstName": "Ann", "lastName": "Lee", "username": "ann.lee", "email": "ann@example.com"})
	if id != "ann.lee" {
		t.Fatalf("a Datto user record extracts id %q, want its username", id)
	}
}

func TestShippedPartitionNamesHold(t *testing.T) {
	want := map[string]string{
		"account":            "/v2/account/sites",
		"account-components": "/v2/account/components",
		"account-devices":    "/v2/account/devices",
		"activity-logs":      "/v2/activity-logs",
	}
	for name, path := range want {
		got, err := syncResourcePath(name)
		if err != nil || got != path {
			t.Errorf("sync partition %q = %q (%v), want %q", name, got, err, path)
		}
	}
	defaults := map[string]bool{}
	for _, r := range defaultSyncResources() {
		defaults[r] = true
	}
	for _, r := range []string{fleetSitesResource, fleetDevicesResource, fleetAlertsOpenResource, fleetAlertsResolvedResource, fleetActivityLogsResource} {
		if !defaults[r] {
			t.Errorf("a fleet analytic reads %q, which the default sync no longer fills", r)
		}
	}
}
