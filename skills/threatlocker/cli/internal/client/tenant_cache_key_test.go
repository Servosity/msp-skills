// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: guards the tenant partitioning of the response cache.
// See skills/threatlocker/handfixes.json (multi-tenant-org-header).

package client

import (
	"net/http"
	"testing"
	"time"

	"threatlocker-pp-cli/internal/config"
)

func tenantClient(t *testing.T, org string) *Client {
	t.Helper()
	cfg := &config.Config{
		BaseURL:    "https://portalapi.g.threatlocker.com/portalapi",
		AuthSource: "env:THREATLOCKER_API_KEY",
	}
	if org != "" {
		cfg.Headers = map[string]string{
			"ManagedOrganizationId":         org,
			"OverrideManagedOrganizationId": org,
		}
	}
	return New(cfg, 5*time.Second, 0)
}

// TestCacheKeyPartitionsByTenant is the regression guard for a cross-tenant
// data leak: one MSP API token addresses many customer organizations, and the
// tenant is selected by a request HEADER rather than by the path or query. If
// that header is missing from the cache key, `--org A` then `--org B` hit the
// same entry and B is served A's rows for the life of the cache TTL.
func TestCacheKeyPartitionsByTenant(t *testing.T) {
	const path = "/Computer/ComputerGetByAllParameters"
	params := map[string]string{"pageNumber": "1"}

	orgA := tenantClient(t, "11111111-1111-1111-1111-111111111111")
	orgB := tenantClient(t, "22222222-2222-2222-2222-222222222222")

	keyA := orgA.cacheKeyFor(http.MethodGet, path, params, nil, nil)
	keyB := orgB.cacheKeyFor(http.MethodGet, path, params, nil, nil)

	if keyA == keyB {
		t.Fatalf("cache key is identical across tenants (%s); one org would serve another org's cached rows", keyA)
	}

	// Same tenant must still hit the same entry, or the cache is useless.
	if again := tenantClient(t, "11111111-1111-1111-1111-111111111111").cacheKeyFor(http.MethodGet, path, params, nil, nil); again != keyA {
		t.Errorf("cache key is not stable for one tenant: %q vs %q", again, keyA)
	}

	// An unscoped client (token's own organization) must not collide with a
	// tenant-scoped one either.
	keyNone := tenantClient(t, "").cacheKeyFor(http.MethodGet, path, params, nil, nil)
	if keyNone == keyA || keyNone == keyB {
		t.Errorf("unscoped cache key collides with a tenant-scoped key")
	}
}
