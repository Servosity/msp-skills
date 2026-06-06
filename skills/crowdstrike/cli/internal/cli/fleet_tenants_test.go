// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet tenants fabric join.

package cli

import (
	"encoding/json"
	"testing"
)

func fabricEnt(kind, id, name string, raw map[string]any) fleetEntity {
	e := fleetEntity{CID: "self", Kind: kind, ID: id, Name: name}
	if raw != nil {
		b, _ := json.Marshal(raw)
		e.Raw = b
	}
	return e
}

func TestBuildTenantFabricJoinsGroupsAndRoles(t *testing.T) {
	ents := []fleetEntity{
		fabricEnt(kindChildCID, "cid-a", "Acme Corp", nil),
		fabricEnt(kindChildCID, "cid-b", "Beta LLC", nil),
		fabricEnt(kindCIDGroup, "cg-1", "East", map[string]any{"cid_group_id": "cg-1", "name": "East"}),
		fabricEnt(kindCIDGroupMember, "cg-1", "", map[string]any{"cid_group_id": "cg-1", "cids": []string{"cid-a"}}),
		fabricEnt(kindUserGroup, "ug-1", "Analysts", map[string]any{"user_group_id": "ug-1", "name": "Analysts"}),
		fabricEnt(kindMSSPRole, "ug-1:cg-1:remote_responder", "", map[string]any{
			"user_group_id": "ug-1", "cid_group_id": "cg-1", "role_id": "remote_responder",
		}),
	}
	view := buildTenantFabric(ents)
	if len(view.Tenants) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(view.Tenants))
	}
	a := view.Tenants[0] // sorted by CID: cid-a first
	if a.CID != "cid-a" {
		t.Fatalf("expected cid-a first, got %s", a.CID)
	}
	if len(a.CIDGroups) != 1 || a.CIDGroups[0] != "East" {
		t.Errorf("cid-a groups = %v, want [East]", a.CIDGroups)
	}
	if len(a.UserGroups) != 1 || a.UserGroups[0].UserGroup != "Analysts" {
		t.Fatalf("cid-a user groups = %v, want Analysts", a.UserGroups)
	}
	if len(a.UserGroups[0].Roles) != 1 || a.UserGroups[0].Roles[0] != "remote_responder" {
		t.Errorf("cid-a roles = %v, want [remote_responder]", a.UserGroups[0].Roles)
	}
	// cid-b is in no group: no memberships, no roles.
	b := view.Tenants[1]
	if len(b.CIDGroups) != 0 || len(b.UserGroups) != 0 {
		t.Errorf("cid-b should have no fabric memberships, got groups=%v ugs=%v", b.CIDGroups, b.UserGroups)
	}
}

func TestBuildTenantFabricEmptyStoreNote(t *testing.T) {
	view := buildTenantFabric(nil)
	if len(view.Tenants) != 0 {
		t.Fatalf("expected no tenants, got %d", len(view.Tenants))
	}
	if view.Note == "" {
		t.Error("expected an honest empty-store note, got none")
	}
}

func TestBuildTenantFabricIgnoresNonFabricKinds(t *testing.T) {
	ents := []fleetEntity{
		{CID: "cid-a", Kind: kindHost, ID: "h1", Name: "host-1"},
		{CID: "cid-a", Kind: kindVuln, ID: "v1", Name: "CVE-2026-0001"},
		fabricEnt(kindChildCID, "cid-a", "Acme Corp", nil),
	}
	view := buildTenantFabric(ents)
	if len(view.Tenants) != 1 {
		t.Fatalf("expected 1 tenant, got %d", len(view.Tenants))
	}
	if view.Counts[kindHost] != 0 || view.Counts[kindVuln] != 0 {
		t.Errorf("non-fabric kinds must not be counted: %v", view.Counts)
	}
}

func TestDiffExpected(t *testing.T) {
	rows := []fleetTenantRow{{CID: "cid-a"}, {CID: "cid-b"}}
	diff := diffExpected(rows, []string{"cid-a", "cid-c"})
	if len(diff.MissingFromFalcon) != 1 || diff.MissingFromFalcon[0] != "cid-c" {
		t.Errorf("missing_from_falcon = %v, want [cid-c]", diff.MissingFromFalcon)
	}
	if len(diff.NotInExpected) != 1 || diff.NotInExpected[0] != "cid-b" {
		t.Errorf("not_in_expected = %v, want [cid-b]", diff.NotInExpected)
	}
}

func TestParseExpectedCIDs(t *testing.T) {
	got := parseExpectedCIDs([]byte("# tenants\ncid-a\n\n  cid-b  \n#skip\n"))
	if len(got) != 2 || got[0] != "cid-a" || got[1] != "cid-b" {
		t.Errorf("parseExpectedCIDs = %v, want [cid-a cid-b]", got)
	}
}
