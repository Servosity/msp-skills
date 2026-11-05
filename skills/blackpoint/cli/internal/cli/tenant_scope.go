// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: tenant-scoped reads never touch the local store.
//
// The generated local store is keyed by resource type, not by tenant, and
// fleet-sync indexes every tenant's rows into the same tables so `search`
// spans the fleet. A read that names one tenant (x-tenant-id header or the
// Cloud Response tenantId query parameter) therefore cannot be answered from
// the store without mixing tenants, and its live result must not be written
// back into the untenanted table. Such reads always go live; an explicit
// local request is refused instead of silently widened.

package cli

import (
	"fmt"
	"strings"
)

// tenantScopedRequest reports whether a read names the tenant scope. The key's
// presence decides, not its value: an explicitly empty selector is still a
// tenant-scoped request (the vendor rejects it), never a fleet-wide one.
func tenantScopedRequest(headers, params map[string]string) bool {
	for k := range headers {
		if strings.EqualFold(k, "x-tenant-id") {
			return true
		}
	}
	_, ok := params["tenantId"]
	return ok
}

// tenantScopedStrategy forces a tenant-scoped read live: no local answer, no
// write-through, no offline fallback. An explicit local request is an error.
func tenantScopedStrategy(flags *rootFlags, strategy, resourceType string) (string, error) {
	if strategy == "local" || (strategy != "live" && flags != nil && flags.dataSource == "local") {
		return "", usageErr(fmt.Errorf("%s: a tenant-scoped read cannot use --data-source local (or %s=local): the local store mixes tenants. Read it live, or run fleet-sync and use triage/overview for the per-tenant view", resourceType, dataSourceEnvVar))
	}
	return "live", nil
}
