// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import (
	"encoding/json"
	"testing"
)

const msFirstParty = "f8cdef31-a31e-4b4a-93e4-5f571e91255a"

func TestConsentAudit(t *testing.T) {
	homeTenant := "11111111-1111-1111-1111-111111111111"

	sps := []json.RawMessage{
		// Microsoft first-party — counted, never listed.
		raw(t, map[string]any{"id": "sp-ms", "displayName": "Microsoft Teams", "appOwnerOrganizationId": msFirstParty}),
		// External vendor, admin-consented, holds an escalation application permission.
		raw(t, map[string]any{"id": "sp-backup", "appId": "app-backup", "displayName": "AcmeBackup", "appOwnerOrganizationId": "vendor-tenant", "accountEnabled": true}),
		// External vendor, user-consented delegated only, low privilege.
		raw(t, map[string]any{"id": "sp-notes", "appId": "app-notes", "displayName": "NoteTaker", "appOwnerOrganizationId": "vendor-tenant", "accountEnabled": true}),
		// Homegrown app registered in this tenant, disabled but still consented.
		raw(t, map[string]any{"id": "sp-internal", "appId": "app-internal", "displayName": "InternalTool", "appOwnerOrganizationId": homeTenant, "accountEnabled": false}),
		// External SP with no consent at all — must be skipped.
		raw(t, map[string]any{"id": "sp-empty", "displayName": "Dormant", "appOwnerOrganizationId": "vendor-tenant", "accountEnabled": true}),
	}
	grants := []json.RawMessage{
		// AcmeBackup: admin-consented, high-privilege delegated scope.
		raw(t, map[string]any{"clientId": "sp-backup", "consentType": "AllPrincipals", "scope": "User.Read Directory.Read.All"}),
		// NoteTaker: per-user consent, benign scopes.
		raw(t, map[string]any{"clientId": "sp-notes", "consentType": "Principal", "principalId": "u1", "scope": "User.Read offline_access"}),
		raw(t, map[string]any{"clientId": "sp-notes", "consentType": "Principal", "principalId": "u2", "scope": "User.Read"}),
		// InternalTool: user-consented, high-privilege.
		raw(t, map[string]any{"clientId": "sp-internal", "consentType": "Principal", "principalId": "u3", "scope": "Mail.ReadWrite"}),
	}
	roles := []json.RawMessage{
		// AcmeBackup holds Application.ReadWrite.All (escalation) app-only.
		raw(t, map[string]any{"principalId": "sp-backup", "resourceDisplayName": "Microsoft Graph", "appRoleId": "1bfefb4e-e0b5-418b-a88f-73c46d2cc8e9"}),
		// AcmeBackup also holds Files.Read.All app-only (non-escalation).
		raw(t, map[string]any{"principalId": "sp-backup", "resourceDisplayName": "Microsoft Graph", "appRoleId": "01d4889c-1287-42c6-ac1f-5d1e02578ef6"}),
	}

	res := ConsentAudit(sps, grants, roles, homeTenant)

	if res.Summary.TotalServicePrincipals != 5 {
		t.Errorf("total SPs = %d, want 5", res.Summary.TotalServicePrincipals)
	}
	if res.Summary.MicrosoftFirstParty != 1 {
		t.Errorf("microsoft first-party = %d, want 1", res.Summary.MicrosoftFirstParty)
	}
	// Reported apps: backup, notes, internal (empty + microsoft excluded).
	if len(res.Apps) != 3 {
		t.Fatalf("reported apps = %d, want 3 (%+v)", len(res.Apps), res.Apps)
	}
	if res.Summary.ThirdPartyApps != 3 {
		t.Errorf("third-party apps = %d, want 3", res.Summary.ThirdPartyApps)
	}
	if res.Summary.ExternalApps != 2 || res.Summary.InternalApps != 1 {
		t.Errorf("external/internal = %d/%d, want 2/1", res.Summary.ExternalApps, res.Summary.InternalApps)
	}
	if res.Summary.AppsWithApplicationPerms != 1 {
		t.Errorf("apps with application perms = %d, want 1", res.Summary.AppsWithApplicationPerms)
	}
	if res.Summary.AppsWithEscalationPerms != 1 {
		t.Errorf("apps with escalation perms = %d, want 1", res.Summary.AppsWithEscalationPerms)
	}
	if res.Summary.DisabledAppsWithConsent != 1 {
		t.Errorf("disabled-but-consented = %d, want 1", res.Summary.DisabledAppsWithConsent)
	}

	// Highest risk first: AcmeBackup (app-perms + escalation + high-priv + admin).
	top := res.Apps[0]
	if top.DisplayName != "AcmeBackup" {
		t.Fatalf("top app = %q, want AcmeBackup (%+v)", top.DisplayName, res.Apps)
	}
	if !top.AdminConsented {
		t.Error("AcmeBackup should be admin-consented (has application permissions)")
	}
	if !hasFlag(top.RiskFlags, "privilege-escalation") {
		t.Errorf("AcmeBackup missing privilege-escalation flag: %v", top.RiskFlags)
	}
	if !hasFlag(top.RiskFlags, "high-privilege-delegated") {
		t.Errorf("AcmeBackup missing high-privilege-delegated flag: %v", top.RiskFlags)
	}
	if len(top.HighPrivilegeScopes) != 1 || top.HighPrivilegeScopes[0] != "Directory.Read.All" {
		t.Errorf("AcmeBackup high-priv scopes = %v, want [Directory.Read.All]", top.HighPrivilegeScopes)
	}
	// Application permission names resolved from the well-known map.
	var names []string
	for _, p := range top.ApplicationPermissions {
		names = append(names, p.Permission)
	}
	if len(names) != 2 || names[0] != "Application.ReadWrite.All" || names[1] != "Files.Read.All" {
		t.Errorf("AcmeBackup app-perm names = %v, want [Application.ReadWrite.All Files.Read.All]", names)
	}

	// NoteTaker: user-consented, two per-user grants, no high-privilege.
	note := findApp(res.Apps, "NoteTaker")
	if note == nil {
		t.Fatal("NoteTaker not reported")
	}
	if note.UserConsentGrants != 2 {
		t.Errorf("NoteTaker user grants = %d, want 2", note.UserConsentGrants)
	}
	if note.AdminConsented {
		t.Error("NoteTaker must not be admin-consented")
	}
	if len(note.HighPrivilegeScopes) != 0 {
		t.Errorf("NoteTaker should have no high-priv scopes, got %v", note.HighPrivilegeScopes)
	}
	if hasFlag(note.RiskFlags, "application-permissions") {
		t.Error("NoteTaker should not carry application-permissions flag")
	}

	// InternalTool: disabled, user-consented, high-privilege delegated (Mail.ReadWrite).
	internal := findApp(res.Apps, "InternalTool")
	if internal == nil {
		t.Fatal("InternalTool not reported")
	}
	if internal.Origin != "internal" {
		t.Errorf("InternalTool origin = %q, want internal", internal.Origin)
	}
	if internal.AccountEnabled {
		t.Error("InternalTool should be disabled")
	}
	if !hasFlag(internal.RiskFlags, "disabled-but-consented") {
		t.Errorf("InternalTool missing disabled-but-consented flag: %v", internal.RiskFlags)
	}
	if !hasFlag(internal.RiskFlags, "high-privilege-delegated") {
		t.Errorf("InternalTool missing high-privilege-delegated flag: %v", internal.RiskFlags)
	}
}

func TestConsentAuditEmpty(t *testing.T) {
	res := ConsentAudit(nil, nil, nil, "")
	if len(res.Apps) != 0 || res.Summary.ThirdPartyApps != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
	if res.Note == "" {
		t.Error("expected a note explaining zero third-party apps")
	}
}

func TestConsentAuditNoHomeTenantAllExternal(t *testing.T) {
	sps := []json.RawMessage{
		raw(t, map[string]any{"id": "sp1", "displayName": "Vendor", "appOwnerOrganizationId": "some-tenant", "accountEnabled": true}),
	}
	grants := []json.RawMessage{
		raw(t, map[string]any{"clientId": "sp1", "consentType": "AllPrincipals", "scope": "User.Read"}),
	}
	res := ConsentAudit(sps, grants, nil, "")
	if res.Summary.ExternalApps != 1 || res.Summary.InternalApps != 0 {
		t.Errorf("without home tenant everything is external, got ext=%d int=%d", res.Summary.ExternalApps, res.Summary.InternalApps)
	}
	if res.Apps[0].Origin != "external" {
		t.Errorf("origin = %q, want external", res.Apps[0].Origin)
	}
}

func TestIsHighPrivilegeScope(t *testing.T) {
	high := []string{"Directory.Read.All", "Mail.ReadWrite", "Sites.FullControl.All", "Group.ReadWrite.All", "Application.ReadWrite.All", "full_access_as_user"}
	for _, s := range high {
		if !isHighPrivilegeScope(s) {
			t.Errorf("%q should be high privilege", s)
		}
	}
	low := []string{"User.Read", "offline_access", "openid", "profile", "email", ""}
	for _, s := range low {
		if isHighPrivilegeScope(s) {
			t.Errorf("%q should not be high privilege", s)
		}
	}
}

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

func findApp(apps []ConsentApp, name string) *ConsentApp {
	for i := range apps {
		if apps[i].DisplayName == name {
			return &apps[i]
		}
	}
	return nil
}
