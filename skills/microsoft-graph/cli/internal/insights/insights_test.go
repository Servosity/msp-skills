// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import (
	"encoding/json"
	"testing"
	"time"
)

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return b
}

func TestParseGraphTime(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-05T08:30:00.0000000Z", true}, // 7 fractional digits
		{"2026-05-05T08:30:00Z", true},
		{"2026-05-05T08:30:00.123Z", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, c := range cases {
		_, ok := ParseGraphTime(c.in)
		if ok != c.ok {
			t.Errorf("ParseGraphTime(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestLicenseWaste(t *testing.T) {
	skus := []json.RawMessage{
		raw(t, map[string]any{"skuPartNumber": "ENTERPRISEPACK", "skuId": "a", "consumedUnits": 80, "prepaidUnits": map[string]any{"enabled": 100, "suspended": 0, "warning": 0}}),
		raw(t, map[string]any{"skuPartNumber": "EMS", "skuId": "b", "consumedUnits": 5, "prepaidUnits": map[string]any{"enabled": 50, "warning": 2}}),
		raw(t, map[string]any{"skuPartNumber": "FULLYUSED", "skuId": "c", "consumedUnits": 10, "prepaidUnits": map[string]any{"enabled": 10}}),
	}
	got := LicenseWaste(skus)
	if len(got) != 2 {
		t.Fatalf("want 2 wasteful SKUs (fully-used dropped), got %d: %+v", len(got), got)
	}
	// Ranked by unused desc: EMS (45) before ENTERPRISEPACK (20).
	if got[0].SkuPartNumber != "EMS" || got[0].Unused != 45 {
		t.Errorf("rank[0] = %+v, want EMS unused=45", got[0])
	}
	if got[1].SkuPartNumber != "ENTERPRISEPACK" || got[1].Unused != 20 {
		t.Errorf("rank[1] = %+v, want ENTERPRISEPACK unused=20", got[1])
	}
	if got[0].Warning != 2 {
		t.Errorf("EMS warning seats = %d, want 2", got[0].Warning)
	}
}

func TestLicenseWasteEmpty(t *testing.T) {
	if got := LicenseWaste(nil); got == nil || len(got) != 0 {
		t.Errorf("LicenseWaste(nil) = %v, want non-nil empty slice", got)
	}
}

func TestSkuNameMap(t *testing.T) {
	skus := []json.RawMessage{
		raw(t, map[string]any{"skuId": "id-1", "skuPartNumber": "ENTERPRISEPACK"}),
		raw(t, map[string]any{"skuId": "id-2", "skuPartNumber": "EMS"}),
	}
	m := SkuNameMap(skus)
	if m["id-1"] != "ENTERPRISEPACK" || m["id-2"] != "EMS" {
		t.Errorf("SkuNameMap = %v", m)
	}
}

func boolp(b bool) *bool { return &b }

func TestLicensedOrphans(t *testing.T) {
	users := []json.RawMessage{
		// disabled + licensed -> orphan
		raw(t, map[string]any{"userPrincipalName": "ex@contoso.com", "displayName": "Ex Employee", "accountEnabled": false, "userType": "Member", "assignedLicenses": []map[string]any{{"skuId": "id-1"}}}),
		// guest + licensed -> orphan
		raw(t, map[string]any{"userPrincipalName": "guest@partner.com", "displayName": "Guest", "accountEnabled": true, "userType": "Guest", "assignedLicenses": []map[string]any{{"skuId": "id-2"}}}),
		// active member + licensed -> NOT orphan
		raw(t, map[string]any{"userPrincipalName": "active@contoso.com", "accountEnabled": true, "userType": "Member", "assignedLicenses": []map[string]any{{"skuId": "id-1"}}}),
		// disabled but unlicensed -> NOT orphan
		raw(t, map[string]any{"userPrincipalName": "noidle@contoso.com", "accountEnabled": false, "userType": "Member"}),
	}
	names := map[string]string{"id-1": "ENTERPRISEPACK", "id-2": "EMS"}
	got := LicensedOrphans(users, names)
	if len(got) != 2 {
		t.Fatalf("want 2 orphans, got %d: %+v", len(got), got)
	}
	reasons := map[string]string{}
	for _, o := range got {
		reasons[o.UserPrincipalName] = o.Reason
	}
	if reasons["ex@contoso.com"] != "disabled" {
		t.Errorf("ex reason = %q, want disabled", reasons["ex@contoso.com"])
	}
	if reasons["guest@partner.com"] != "guest" {
		t.Errorf("guest reason = %q, want guest", reasons["guest@partner.com"])
	}
	// SKU name resolution
	for _, o := range got {
		if o.UserPrincipalName == "ex@contoso.com" && (len(o.Skus) != 1 || o.Skus[0] != "ENTERPRISEPACK") {
			t.Errorf("ex skus = %v, want [ENTERPRISEPACK]", o.Skus)
		}
	}
}

func TestAdminsAudit(t *testing.T) {
	roles := []json.RawMessage{
		raw(t, map[string]any{
			"id": "r1", "displayName": "Global Administrator",
			"members": []map[string]any{
				{"displayName": "Admin One", "userPrincipalName": "admin1@contoso.com", "userType": "Member", "accountEnabled": true},
				{"displayName": "Guest Admin", "userPrincipalName": "g@partner.com", "userType": "Guest", "accountEnabled": true},
				{"displayName": "Ex Admin", "userPrincipalName": "ex@contoso.com", "userType": "Member", "accountEnabled": false},
			},
		}),
		raw(t, map[string]any{"id": "r2", "displayName": "Helpdesk Administrator", "members": []map[string]any{}}),
	}
	got := AdminsAudit(roles)
	if len(got) != 3 {
		t.Fatalf("want 3 assignments, got %d: %+v", len(got), got)
	}
	risks := map[string]string{}
	for _, a := range got {
		risks[a.UserPrincipalName] = a.Risk
		if a.Role != "Global Administrator" {
			t.Errorf("unexpected role %q for %q", a.Role, a.UserPrincipalName)
		}
	}
	if risks["admin1@contoso.com"] != "" {
		t.Errorf("clean admin risk = %q, want empty", risks["admin1@contoso.com"])
	}
	if risks["g@partner.com"] != "guest-admin" {
		t.Errorf("guest admin risk = %q, want guest-admin", risks["g@partner.com"])
	}
	if risks["ex@contoso.com"] != "disabled-admin" {
		t.Errorf("disabled admin risk = %q, want disabled-admin", risks["ex@contoso.com"])
	}
}

func TestSecurityTriage(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	since := now.Add(-24 * time.Hour)
	alerts := []json.RawMessage{
		raw(t, map[string]any{"id": "1", "title": "Fresh high", "severity": "high", "status": "new", "serviceSource": "microsoftDefenderForEndpoint", "createdDateTime": now.Add(-2 * time.Hour).Format(time.RFC3339)}),
		raw(t, map[string]any{"id": "2", "title": "Fresh medium", "severity": "medium", "status": "inProgress", "serviceSource": "microsoftSentinel", "createdDateTime": now.Add(-5 * time.Hour).Format(time.RFC3339)}),
		// resolved -> excluded
		raw(t, map[string]any{"id": "3", "title": "Resolved", "severity": "high", "status": "resolved", "serviceSource": "microsoftDefenderForEndpoint", "createdDateTime": now.Add(-1 * time.Hour).Format(time.RFC3339)}),
		// old -> excluded by window
		raw(t, map[string]any{"id": "4", "title": "Old", "severity": "high", "status": "new", "serviceSource": "microsoftSentinel", "createdDateTime": now.Add(-72 * time.Hour).Format(time.RFC3339)}),
	}
	res := SecurityTriage(alerts, since)
	if res.TotalOpen != 2 {
		t.Fatalf("TotalOpen = %d, want 2", res.TotalOpen)
	}
	// high sorts before medium
	if res.Alerts[0].Severity != "high" {
		t.Errorf("first alert severity = %q, want high", res.Alerts[0].Severity)
	}
	if len(res.BySeverity) != 2 || res.BySeverity[0].Key != "high" {
		t.Errorf("bySeverity = %+v, want high first", res.BySeverity)
	}
}

func TestDeviceDrift(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	staleBefore := now.Add(-30 * 24 * time.Hour)
	devices := []json.RawMessage{
		// noncompliant + unencrypted, recently synced -> 2 reasons
		raw(t, map[string]any{"deviceName": "LAPTOP-1", "userPrincipalName": "u1@contoso.com", "operatingSystem": "Windows", "complianceState": "noncompliant", "isEncrypted": false, "lastSyncDateTime": now.Add(-1 * time.Hour).Format(time.RFC3339)}),
		// compliant + encrypted but stale -> 1 reason
		raw(t, map[string]any{"deviceName": "LAPTOP-2", "complianceState": "compliant", "isEncrypted": true, "lastSyncDateTime": now.Add(-90 * 24 * time.Hour).Format(time.RFC3339)}),
		// fully healthy -> excluded
		raw(t, map[string]any{"deviceName": "LAPTOP-3", "complianceState": "compliant", "isEncrypted": true, "lastSyncDateTime": now.Format(time.RFC3339)}),
	}
	got := DeviceDrift(devices, staleBefore)
	if len(got) != 2 {
		t.Fatalf("want 2 drifted devices, got %d: %+v", len(got), got)
	}
	// LAPTOP-1 has 2 reasons, sorts first
	if got[0].DeviceName != "LAPTOP-1" || len(got[0].Reasons) != 2 {
		t.Errorf("first drift = %+v, want LAPTOP-1 with 2 reasons", got[0])
	}
	if got[1].DeviceName != "LAPTOP-2" || got[1].Reasons[0] != "stale" {
		t.Errorf("second drift = %+v, want LAPTOP-2 stale", got[1])
	}
}

func TestTenantSnapshot(t *testing.T) {
	now := time.Now().UTC()
	users := []json.RawMessage{
		raw(t, map[string]any{"userPrincipalName": "a@contoso.com", "accountEnabled": true, "userType": "Member"}),
		raw(t, map[string]any{"userPrincipalName": "g@partner.com", "accountEnabled": true, "userType": "Guest"}),
		raw(t, map[string]any{"userPrincipalName": "x@contoso.com", "accountEnabled": false, "userType": "Member"}),
	}
	groups := []json.RawMessage{raw(t, map[string]any{"id": "g1"})}
	licenses := []json.RawMessage{
		raw(t, map[string]any{"skuPartNumber": "EMS", "consumedUnits": 5, "prepaidUnits": map[string]any{"enabled": 20}}),
	}
	roles := []json.RawMessage{
		raw(t, map[string]any{"displayName": "Global Administrator", "members": []map[string]any{
			{"userPrincipalName": "a@contoso.com", "accountEnabled": true, "userType": "Member"},
			{"userPrincipalName": "g@partner.com", "accountEnabled": true, "userType": "Guest"},
		}}),
	}
	alerts := []json.RawMessage{
		raw(t, map[string]any{"id": "1", "title": "h", "severity": "high", "status": "new", "createdDateTime": now.Format(time.RFC3339)}),
		raw(t, map[string]any{"id": "2", "title": "m", "severity": "medium", "status": "inProgress", "createdDateTime": now.Format(time.RFC3339)}),
	}
	devices := []json.RawMessage{
		raw(t, map[string]any{"deviceName": "d1", "complianceState": "noncompliant"}),
		raw(t, map[string]any{"deviceName": "d2", "complianceState": "compliant"}),
	}
	snap := TenantSnapshot(users, groups, licenses, roles, alerts, devices)
	if snap.Users != 3 || snap.EnabledUsers != 2 || snap.GuestUsers != 1 {
		t.Errorf("user counts = %+v", snap)
	}
	if snap.UnusedLicenseSeats != 15 {
		t.Errorf("unused seats = %d, want 15", snap.UnusedLicenseSeats)
	}
	if snap.PrivilegedAssignments != 2 || snap.RiskyAdminAssignments != 1 {
		t.Errorf("admin counts = priv %d risky %d, want 2/1", snap.PrivilegedAssignments, snap.RiskyAdminAssignments)
	}
	if snap.OpenAlerts != 2 || snap.HighSeverityOpenAlerts != 1 {
		t.Errorf("alert counts = open %d high %d, want 2/1", snap.OpenAlerts, snap.HighSeverityOpenAlerts)
	}
	if snap.ManagedDevices != 2 || snap.NonCompliantDevices != 1 {
		t.Errorf("device counts = %d/%d, want 2/1", snap.ManagedDevices, snap.NonCompliantDevices)
	}
}
