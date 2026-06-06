// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import (
	"encoding/json"
	"testing"
)

func TestLicenseMap(t *testing.T) {
	skus := []json.RawMessage{
		raw(t, map[string]any{"skuId": "ent", "skuPartNumber": "ENTERPRISEPACK", "consumedUnits": 3, "prepaidUnits": map[string]any{"enabled": 10}}),
		raw(t, map[string]any{"skuId": "ems", "skuPartNumber": "EMS", "consumedUnits": 5, "prepaidUnits": map[string]any{"enabled": 5}}),
	}
	users := []json.RawMessage{
		raw(t, map[string]any{"userPrincipalName": "active@contoso.com", "displayName": "Active", "accountEnabled": true, "userType": "Member", "assignedLicenses": []map[string]any{{"skuId": "ent"}}}),
		raw(t, map[string]any{"userPrincipalName": "ex@contoso.com", "displayName": "Ex", "accountEnabled": false, "userType": "Member", "assignedLicenses": []map[string]any{{"skuId": "ent"}}}),
		raw(t, map[string]any{"userPrincipalName": "guest@partner.com", "displayName": "Guest", "accountEnabled": true, "userType": "Guest", "assignedLicenses": []map[string]any{{"skuId": "ent"}}}),
		raw(t, map[string]any{"userPrincipalName": "unlicensed@contoso.com", "accountEnabled": true, "userType": "Member"}),
		raw(t, map[string]any{"userPrincipalName": "other@contoso.com", "accountEnabled": true, "userType": "Member", "assignedLicenses": []map[string]any{{"skuId": "ems"}}}),
	}

	cases := []struct {
		name        string
		query       string
		wantOK      bool
		wantHolders int
		wantReclaim int
	}{
		{"by part number", "ENTERPRISEPACK", true, 3, 2},
		{"case-insensitive part number", "enterprisepack", true, 3, 2},
		{"by skuId", "ent", true, 3, 2},
		{"other sku isolated", "EMS", true, 1, 0},
		{"no match", "NONEXISTENT", false, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, ok := LicenseMap(users, skus, tc.query)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if len(res.Consumers) != tc.wantHolders {
				t.Errorf("consumers = %d, want %d (%+v)", len(res.Consumers), tc.wantHolders, res.Consumers)
			}
			if res.ReclaimableSeats != tc.wantReclaim {
				t.Errorf("reclaimable = %d, want %d", res.ReclaimableSeats, tc.wantReclaim)
			}
		})
	}

	// Flagged consumers sort first.
	res, _ := LicenseMap(users, skus, "ENTERPRISEPACK")
	if len(res.Consumers) > 0 && len(res.Consumers[0].Flags) == 0 {
		t.Errorf("expected flagged consumers first, got %+v", res.Consumers)
	}
}

func TestLicenseMapEmptyInputs(t *testing.T) {
	if _, ok := LicenseMap(nil, nil, "ANY"); ok {
		t.Error("expected ok=false with no SKUs")
	}
	res, ok := LicenseMap(nil, []json.RawMessage{
		raw(t, map[string]any{"skuId": "ent", "skuPartNumber": "ENTERPRISEPACK", "prepaidUnits": map[string]any{"enabled": 10}}),
	}, "ENTERPRISEPACK")
	if !ok {
		t.Fatal("expected SKU match with empty users")
	}
	if len(res.Consumers) != 0 {
		t.Errorf("expected zero consumers, got %d", len(res.Consumers))
	}
}

func TestGroupsRisk(t *testing.T) {
	groups := []json.RawMessage{
		// healthy: owner + member majority
		raw(t, map[string]any{"id": "g1", "displayName": "Healthy", "members": []map[string]any{{"userPrincipalName": "a@x.com", "userType": "Member"}, {"userPrincipalName": "b@x.com", "userType": "Member"}}, "owners": []map[string]any{{"id": "o1"}}}),
		// ownerless
		raw(t, map[string]any{"id": "g2", "displayName": "NoOwner", "members": []map[string]any{{"userPrincipalName": "a@x.com", "userType": "Member"}}, "owners": []map[string]any{}}),
		// empty + ownerless
		raw(t, map[string]any{"id": "g3", "displayName": "Empty", "members": []map[string]any{}, "owners": []map[string]any{}}),
		// guest-heavy (2 of 3 = 0.66 >= 0.5)
		raw(t, map[string]any{"id": "g4", "displayName": "GuestHeavy", "members": []map[string]any{{"userType": "Guest"}, {"userType": "Guest"}, {"userType": "Member"}}, "owners": []map[string]any{{"id": "o2"}}}),
		// no association data — must not be flagged
		raw(t, map[string]any{"id": "g5", "displayName": "Unknown"}),
	}

	res := GroupsRisk(groups, 0.5)
	if res.ScannedGroups != 5 {
		t.Errorf("scanned = %d, want 5", res.ScannedGroups)
	}
	if res.MissingAssociations != 1 {
		t.Errorf("missing associations = %d, want 1", res.MissingAssociations)
	}
	if res.Note == "" {
		t.Error("expected note about missing association data")
	}
	if len(res.Groups) != 3 {
		t.Fatalf("flagged = %d, want 3 (%+v)", len(res.Groups), res.Groups)
	}
	// Most reasons first: Empty (empty+ownerless) leads.
	if res.Groups[0].DisplayName != "Empty" || len(res.Groups[0].Reasons) != 2 {
		t.Errorf("expected Empty with 2 reasons first, got %+v", res.Groups[0])
	}
	byName := map[string][]string{}
	for _, g := range res.Groups {
		byName[g.DisplayName] = g.Reasons
	}
	if got := byName["NoOwner"]; len(got) != 1 || got[0] != "ownerless" {
		t.Errorf("NoOwner reasons = %v", got)
	}
	if got := byName["GuestHeavy"]; len(got) != 1 || got[0] != "guest-heavy" {
		t.Errorf("GuestHeavy reasons = %v", got)
	}
	if _, flagged := byName["Healthy"]; flagged {
		t.Error("Healthy group must not be flagged")
	}
	if _, flagged := byName["Unknown"]; flagged {
		t.Error("group without association data must not be flagged")
	}
}

func TestGroupsRiskEmpty(t *testing.T) {
	res := GroupsRisk(nil, 0.5)
	if res.ScannedGroups != 0 || len(res.Groups) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

// Literal JSON null for the association keys means "never synced", not
// "zero" — it must land in MissingAssociations, never be flagged. The pull
// path guarantees synced groups always carry [] (marshalJSONArray), so null
// only appears for rows written by other sync paths. This locks the contract
// from both sides: null → missing, [] → flaggable zero.
func TestGroupsRiskNullVsEmptyAssociations(t *testing.T) {
	groups := []json.RawMessage{
		json.RawMessage(`{"id":"g1","displayName":"NullAssoc","members":null,"owners":null}`),
		json.RawMessage(`{"id":"g2","displayName":"ZeroAssoc","members":[],"owners":[]}`),
	}
	res := GroupsRisk(groups, 0.5)
	if res.MissingAssociations != 1 {
		t.Errorf("null associations should count as missing, got %d", res.MissingAssociations)
	}
	if len(res.Groups) != 1 || res.Groups[0].DisplayName != "ZeroAssoc" {
		t.Fatalf("expected only ZeroAssoc flagged, got %+v", res.Groups)
	}
	reasons := res.Groups[0].Reasons
	if len(reasons) != 2 || reasons[0] != "empty" || reasons[1] != "ownerless" {
		t.Errorf("ZeroAssoc reasons = %v, want [empty ownerless]", reasons)
	}
}

func TestGroupsRiskZeroRatioDisablesGuestCheck(t *testing.T) {
	groups := []json.RawMessage{
		raw(t, map[string]any{"id": "g1", "displayName": "AllGuests", "members": []map[string]any{{"userType": "Guest"}}, "owners": []map[string]any{{"id": "o1"}}}),
	}
	res := GroupsRisk(groups, 0)
	if len(res.Groups) != 0 {
		t.Errorf("guest-ratio 0 should disable the guest-heavy check, got %+v", res.Groups)
	}
}
