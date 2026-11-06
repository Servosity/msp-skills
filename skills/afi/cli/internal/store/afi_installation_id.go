// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// afi handfix installation-identity: an application installation
// (GET /api/v1/applications/me/installations, v1Installation) carries no id,
// only org_id and tenant_id, and the spec documents that one of them is always
// empty (tenant_id for an organization-wide installation, org_id for a
// tenant-level one), so neither a single-field nor a composite x-resource-id
// names it. Without an identity every installation failed id extraction and
// `sync --resources applications` (and `workflow archive`) stored nothing.
// The key is org_id:tenant_id, the same one fleet-sync stores installations
// under.
func afiInstallationID(resourceType string, obj map[string]any) (string, bool) {
	if resourceType != "applications" {
		return "", false
	}
	org, _ := obj["org_id"].(string)
	tenant, _ := obj["tenant_id"].(string)
	if org == "" && tenant == "" {
		return "", false
	}
	return org + ":" + tenant, true
}
