// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeTagAudit(t *testing.T) {
	tags := []lvlTag{
		{ID: "t1", Name: "prod", DeviceCount: 2},
		{ID: "t2", Name: "Prod", DeviceCount: 0},   // duplicate of t1 (case), but name used by devices -> dupe not orphan
		{ID: "t3", Name: "legacy", DeviceCount: 0}, // applied to nothing -> orphan
		{ID: "t4", Name: "db", DeviceCount: 1},
	}
	res := lvlComputeTagAudit(tags, lvlFxDevices(), true, true, true)
	// dev3 and dev4 carry no tags.
	if len(res.UntaggedDevices) != 2 {
		t.Fatalf("untagged = %v, want charlie+delta", res.UntaggedDevices)
	}
	if len(res.OrphanTags) != 1 || res.OrphanTags[0] != "legacy" {
		t.Fatalf("orphans = %v, want [legacy]", res.OrphanTags)
	}
	if len(res.DuplicateNames) != 1 || res.DuplicateNames[0].Name != "prod" || len(res.DuplicateNames[0].TagIDs) != 2 {
		t.Fatalf("duplicates = %+v, want prod x2", res.DuplicateNames)
	}
}

func TestLvlComputeTagAuditSectionToggles(t *testing.T) {
	res := lvlComputeTagAudit([]lvlTag{{ID: "t3", Name: "legacy"}}, lvlFxDevices(), false, true, false)
	if res.UntaggedDevices != nil || res.DuplicateNames != nil {
		t.Fatalf("disabled sections leaked: %+v", res)
	}
	if len(res.OrphanTags) != 1 {
		t.Fatalf("orphans = %v, want 1", res.OrphanTags)
	}
}
