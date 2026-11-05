// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/config"
)

// The on-disk response cache is a local store too. Its key folds the base URL,
// the credential and the tenant-selecting headers, but CustomerSubDomain, the
// header that picks the SuperOps tenant, was not one of them: one token at two
// subdomains shared every cached GET body.
func TestResponseCacheKeyIsPerTenant(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-cache-token-1")
	key := func(subdomain string) string {
		t.Setenv("SUPEROPS_SUBDOMAIN", subdomain)
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		return New(cfg, time.Second, 0).cacheKeyFor(http.MethodGet, "/graphql", map[string]string{"first": "50"}, nil, nil)
	}
	a, b := key("acme-a"), key("acme-b")
	if a == b {
		t.Fatalf("tenants acme-a and acme-b share the response cache key %s", a)
	}
	if again := key("acme-a"); again != a {
		t.Fatalf("one tenant's key is not stable: %s then %s", a, again)
	}
}
