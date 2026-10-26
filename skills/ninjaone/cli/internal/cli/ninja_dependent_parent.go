// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// ninjaDependentParentPartitions names the partition whose row ids a dependent resource fans
// out over, where it differs from the generated ParentTable. Hand-fix
// (reapply/dependent_parent.py).
//
// organization_devices (/v2/organization/{id}/devices) is generated under ParentTable
// "organization", which this connector pins to /v2/organization/documents, keyed on documentId
// (spec.yaml ClientDocumentWithAttributeValuesPublicApiDTO). Fanning out over it requested
// document ids as organization ids and skipped every organization without a document; the
// organization ids are the "organizations" partition's (/v2/organizations). ParentTable itself
// stays "organization", so `sync --resources organization` and the paging defaults are unchanged.
var ninjaDependentParentPartitions = map[string]string{
	"organization_devices": "organizations",
}

func ninjaDependentParentPartition(dep dependentResourceDef) string {
	if p := ninjaDependentParentPartitions[dep.Name]; p != "" {
		return p
	}
	return dep.ParentTable
}
