// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "secret-responses-never-persisted" entry.

package mcp

import "testing"

// TestSecretReturningEndpointsAreCLIOnly: neither execute nor get can reach an
// operation whose response carries a tamper password or migration token, and
// the filter leaves the rest of the catalog alone.
func TestSecretReturningEndpointsAreCLIOnly(t *testing.T) {
	for id := range cliOnlySecretEndpoints {
		if findCodeOrchEndpoint(id) != nil {
			t.Errorf("%s is still reachable through the MCP execute catalog", id)
		}
	}
	for _, id := range []string{"endpoints.list", "endpoints.get-isolation", "migrations.get-endpoints"} {
		if findCodeOrchEndpoint(id) == nil {
			t.Errorf("%s was dropped; only secret-returning operations may be filtered", id)
		}
	}
}
