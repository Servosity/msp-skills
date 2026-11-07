// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import "testing"

// The fielded permission lists hold every name the map literals did.
func TestPermissionTablesKeptTheirNames(t *testing.T) {
	if len(highPrivilegeScopeNames) != 19 || !highPrivilegeScopeNames["Mail.Read"] || !highPrivilegeScopeNames["full_access_as_user"] {
		t.Errorf("highPrivilegeScopeNames: %v", highPrivilegeScopeNames)
	}
	if len(wellKnownGraphAppRoles) != 18 || wellKnownGraphAppRoles["9e3f62cf-ca93-4989-b6ce-bf83c28f9fe8"] != "RoleManagement.ReadWrite.Directory" {
		t.Errorf("wellKnownGraphAppRoles: %v", wellKnownGraphAppRoles)
	}
	if len(escalationAppRoles) != 4 || !escalationAppRoles["AppRoleAssignment.ReadWrite.All"] {
		t.Errorf("escalationAppRoles: %v", escalationAppRoles)
	}
}
