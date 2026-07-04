// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// consent.go holds the third-party application consent inventory: a
// least-privilege audit of every non-Microsoft service principal (enterprise
// application) in the tenant, joining the delegated permission grants
// (oauth2PermissionGrants), the service principals (servicePrincipals), and the
// application permissions (appRoleAssignments) into one risk-ranked table.
//
// Same contract as insights.go and governance.go: pure functions over
// already-fetched JSON rows so the whole classification is unit-testable
// without a live tenant. The command layer does the fetching; this file does
// the judging.

package insights

import (
	"encoding/json"
	"sort"
	"strings"
)

// microsoftFirstPartyOwnerIDs are the tenant IDs Microsoft's own first-party
// applications (Office, Teams, Intune, the Graph command-line tools, etc.) are
// owned by. Service principals owned by these tenants are Microsoft plumbing,
// not third-party consent risk, so the audit counts them but does not list
// them — they are the noise an MSP is trying to see past.
var microsoftFirstPartyOwnerIDs = map[string]bool{
	"f8cdef31-a31e-4b4a-93e4-5f571e91255a": true, // Microsoft Services
	"72f988bf-86f1-41af-91ab-2d7cd011db47": true, // Microsoft corp tenant
}

// IsMicrosoftFirstParty reports whether a service principal's
// appOwnerOrganizationId belongs to Microsoft (its own first-party apps). The
// command layer uses it to skip Microsoft SPs when fanning out for application
// permissions.
func IsMicrosoftFirstParty(appOwnerOrganizationID string) bool {
	return microsoftFirstPartyOwnerIDs[appOwnerOrganizationID]
}

// highPrivilegeScopeNames are delegated (oauth2PermissionGrant) scope names an
// MSP should review on sight: broad read-all, any write-all, mail access, and
// directory/role/application management. Matched exactly, case-insensitively.
var highPrivilegeScopeNames = map[string]bool{
	"Directory.Read.All": true, "Directory.ReadWrite.All": true,
	"User.Read.All": true, "User.ReadWrite.All": true,
	"Group.Read.All": true, "Group.ReadWrite.All": true,
	"Mail.Read": true, "Mail.ReadWrite": true, "Mail.Send": true,
	"Files.Read.All": true, "Files.ReadWrite.All": true,
	"Sites.Read.All": true, "Sites.ReadWrite.All": true, "Sites.FullControl.All": true,
	"Application.Read.All": true, "Application.ReadWrite.All": true,
	"RoleManagement.ReadWrite.Directory": true,
	"AppRoleAssignment.ReadWrite.All":    true,
	"full_access_as_user":                true,
}

// isHighPrivilegeScope flags a delegated scope by exact name or by the broad
// suffix/prefix shapes that always denote elevated access.
// ponytail: name + affix heuristic, not the full Graph permission catalog;
// upgrade path is a generated permission-risk table keyed by scope id.
func isHighPrivilegeScope(scope string) bool {
	s := strings.TrimSpace(scope)
	if s == "" {
		return false
	}
	if highPrivilegeScopeNames[s] {
		return true
	}
	if strings.HasSuffix(s, ".ReadWrite.All") || strings.HasSuffix(s, ".FullControl.All") {
		return true
	}
	return strings.HasPrefix(s, "Directory.") ||
		strings.HasPrefix(s, "RoleManagement.") ||
		strings.HasPrefix(s, "Mail.") ||
		strings.HasPrefix(s, "Application.ReadWrite")
}

// wellKnownGraphAppRoles maps the notorious Microsoft Graph application
// (app-only) role ids to their human name so the report can say
// "Mail.ReadWrite" instead of a bare GUID. Application permissions are the
// highest-privilege consent class regardless of name — this map only makes the
// dangerous ones legible; an unmapped id still counts and still flags.
// ponytail: curated high-risk subset, not every Graph app role.
var wellKnownGraphAppRoles = map[string]string{
	"7ab1d382-f21e-4acd-a863-ba3e13f7da61": "Directory.Read.All",
	"19dbc75e-c2e2-444c-a770-ec69d8559fc7": "Directory.ReadWrite.All",
	"df021288-bdef-4463-88db-98f22de89214": "User.Read.All",
	"741f803b-c850-494e-b5df-cde7c675a1ca": "User.ReadWrite.All",
	"5b567255-7703-4780-807c-7be8301ae99b": "Group.Read.All",
	"62a82d76-70ea-41e2-9197-370581804d09": "Group.ReadWrite.All",
	"810c84a8-4a9e-49e6-bf7d-12d183f40d01": "Mail.Read",
	"e2a3a72e-5f79-4c64-b1b1-878b674786c9": "Mail.ReadWrite",
	"b633e1c5-b582-4048-a93e-9f11b44c7e96": "Mail.Send",
	"01d4889c-1287-42c6-ac1f-5d1e02578ef6": "Files.Read.All",
	"75359482-378d-4052-8f01-80520e7db3cd": "Files.ReadWrite.All",
	"332a536c-c7ef-4017-ab91-336970924f0d": "Sites.Read.All",
	"9492366f-7969-46a4-8d15-ed1a20078fff": "Sites.ReadWrite.All",
	"a82116e5-55eb-4c41-a434-62fe8a61c773": "Sites.FullControl.All",
	"9a5d68dd-52b0-4cc2-bd40-abcf44ac3a30": "Application.Read.All",
	"1bfefb4e-e0b5-418b-a88f-73c46d2cc8e9": "Application.ReadWrite.All",
	"9e3f62cf-ca93-4989-b6ce-bf83c28f9fe8": "RoleManagement.ReadWrite.Directory",
	"06b708a9-e830-4db3-a914-8e69da51d44f": "AppRoleAssignment.ReadWrite.All",
}

// escalationAppRoles are the application permissions that let an app grant
// itself (or others) more access — the "one consent to rule them all" tier.
var escalationAppRoles = map[string]bool{
	"Application.ReadWrite.All":          true,
	"RoleManagement.ReadWrite.Directory": true,
	"AppRoleAssignment.ReadWrite.All":    true,
	"Directory.ReadWrite.All":            true,
}

// ---- input shapes -----------------------------------------------------------

type servicePrincipalLite struct {
	ID                     string   `json:"id"`
	AppID                  string   `json:"appId"`
	DisplayName            string   `json:"displayName"`
	AppOwnerOrganizationID string   `json:"appOwnerOrganizationId"`
	ServicePrincipalType   string   `json:"servicePrincipalType"`
	AccountEnabled         *bool    `json:"accountEnabled"`
	SignInAudience         string   `json:"signInAudience"`
	Tags                   []string `json:"tags"`
}

type oauth2GrantLite struct {
	ClientID    string `json:"clientId"`
	ConsentType string `json:"consentType"` // "AllPrincipals" (admin) | "Principal" (user)
	PrincipalID string `json:"principalId"`
	ResourceID  string `json:"resourceId"`
	Scope       string `json:"scope"` // space-delimited scope names
}

type appRoleAssignmentLite struct {
	PrincipalID         string `json:"principalId"`
	PrincipalType       string `json:"principalType"`
	ResourceID          string `json:"resourceId"`
	ResourceDisplayName string `json:"resourceDisplayName"`
	AppRoleID           string `json:"appRoleId"`
}

// ---- output shapes ----------------------------------------------------------

// AppPermission is one application (app-only) role an app has been granted on a
// resource API. These require admin consent by definition and run with no
// signed-in user, so every entry is inherently elevated.
type AppPermission struct {
	Resource   string `json:"resource"`
	Permission string `json:"permission,omitempty"` // resolved name, else empty (see appRoleId)
	AppRoleID  string `json:"appRoleId"`
	Escalation bool   `json:"escalation,omitempty"` // can grant further access
}

// ConsentApp is one third-party (non-Microsoft) enterprise application with the
// access it has been consented, and the least-privilege risk flags an MSP
// would review.
type ConsentApp struct {
	ID                     string          `json:"id"`
	AppID                  string          `json:"appId,omitempty"`
	DisplayName            string          `json:"displayName"`
	Origin                 string          `json:"origin"` // "external" | "internal" (this tenant) | "microsoft"
	AccountEnabled         bool            `json:"accountEnabled"`
	AdminConsented         bool            `json:"adminConsented"`         // any delegated grant with consentType AllPrincipals, or any application permission
	UserConsentGrants      int             `json:"userConsentGrants"`      // count of per-user delegated grants
	DelegatedScopes        []string        `json:"delegatedScopes"`        // union across all grants
	HighPrivilegeScopes    []string        `json:"highPrivilegeScopes,omitempty"`
	ApplicationPermissions []AppPermission `json:"applicationPermissions"` // app-only roles
	RiskFlags              []string        `json:"riskFlags"`
	RiskScore              int             `json:"riskScore"`
}

// ConsentSummary is the headline count block: the numbers an MSP spot-checks
// against the Entra "Enterprise applications" blade.
type ConsentSummary struct {
	TotalServicePrincipals      int `json:"totalServicePrincipals"`
	MicrosoftFirstParty         int `json:"microsoftFirstParty"`
	ThirdPartyApps              int `json:"thirdPartyApps"`
	ExternalApps                int `json:"externalApps"`
	InternalApps                int `json:"internalApps"`
	AdminConsentedApps          int `json:"adminConsentedApps"`
	UserConsentedApps           int `json:"userConsentedApps"`
	AppsWithApplicationPerms    int `json:"appsWithApplicationPermissions"`
	AppsWithEscalationPerms     int `json:"appsWithEscalationPermissions"`
	HighRiskApps                int `json:"highRiskApps"`
	DisabledAppsWithConsent     int `json:"disabledAppsWithConsent"`
}

// ConsentAuditResult is the full inventory envelope.
type ConsentAuditResult struct {
	Summary ConsentSummary `json:"summary"`
	Apps    []ConsentApp   `json:"apps"`
	Note    string         `json:"note,omitempty"`
}

// highRiskScore is the riskScore at or above which an app is counted high-risk.
const highRiskScore = 3

// ConsentAudit joins the three consent surfaces into a third-party app
// inventory. homeTenantID, when non-empty, splits non-Microsoft apps into
// "external" (owned by another tenant) and "internal" (registered in this
// tenant); pass "" if unknown and everything non-Microsoft reads as external.
//
// Microsoft first-party service principals are counted in the summary but
// never listed — the report is about consent an MSP granted or inherited, not
// Microsoft's own plumbing. Apps are returned highest-risk first.
func ConsentAudit(servicePrincipals, oauth2Grants, appRoleAssignments []json.RawMessage, homeTenantID string) ConsentAuditResult {
	res := ConsentAuditResult{Apps: []ConsentApp{}}

	// Index grants and app-role assignments by the client/principal SP id.
	grantsByClient := map[string][]oauth2GrantLite{}
	for _, raw := range oauth2Grants {
		var g oauth2GrantLite
		if err := json.Unmarshal(raw, &g); err != nil || g.ClientID == "" {
			continue
		}
		grantsByClient[g.ClientID] = append(grantsByClient[g.ClientID], g)
	}
	rolesByPrincipal := map[string][]appRoleAssignmentLite{}
	for _, raw := range appRoleAssignments {
		var a appRoleAssignmentLite
		if err := json.Unmarshal(raw, &a); err != nil || a.PrincipalID == "" {
			continue
		}
		rolesByPrincipal[a.PrincipalID] = append(rolesByPrincipal[a.PrincipalID], a)
	}

	for _, raw := range servicePrincipals {
		var sp servicePrincipalLite
		if err := json.Unmarshal(raw, &sp); err != nil || sp.ID == "" {
			continue
		}
		res.Summary.TotalServicePrincipals++

		if microsoftFirstPartyOwnerIDs[sp.AppOwnerOrganizationID] {
			res.Summary.MicrosoftFirstParty++
			continue
		}

		origin := "external"
		if homeTenantID != "" && sp.AppOwnerOrganizationID == homeTenantID {
			origin = "internal"
		}

		app := ConsentApp{
			ID:                     sp.ID,
			AppID:                  sp.AppID,
			DisplayName:            sp.DisplayName,
			Origin:                 origin,
			AccountEnabled:         sp.AccountEnabled == nil || *sp.AccountEnabled,
			DelegatedScopes:        []string{},
			ApplicationPermissions: []AppPermission{},
			RiskFlags:              []string{},
		}

		// Delegated permissions (oauth2PermissionGrants).
		scopeSet := map[string]bool{}
		highSet := map[string]bool{}
		for _, g := range grantsByClient[sp.ID] {
			if strings.EqualFold(g.ConsentType, "AllPrincipals") {
				app.AdminConsented = true
			} else {
				app.UserConsentGrants++
			}
			for _, s := range strings.Fields(g.Scope) {
				scopeSet[s] = true
				if isHighPrivilegeScope(s) {
					highSet[s] = true
				}
			}
		}
		app.DelegatedScopes = sortedKeys(scopeSet)
		app.HighPrivilegeScopes = sortedKeys(highSet)

		// Application permissions (appRoleAssignments) — always admin-consented.
		hasEscalation := false
		for _, a := range rolesByPrincipal[sp.ID] {
			name := wellKnownGraphAppRoles[a.AppRoleID]
			esc := name != "" && escalationAppRoles[name]
			if esc {
				hasEscalation = true
			}
			app.ApplicationPermissions = append(app.ApplicationPermissions, AppPermission{
				Resource:   a.ResourceDisplayName,
				Permission: name,
				AppRoleID:  a.AppRoleID,
				Escalation: esc,
			})
		}
		if len(app.ApplicationPermissions) > 0 {
			app.AdminConsented = true
		}
		sort.SliceStable(app.ApplicationPermissions, func(i, j int) bool {
			return app.ApplicationPermissions[i].Permission < app.ApplicationPermissions[j].Permission
		})

		// Skip apps with no consent at all — a bare service principal with no
		// grants and no app roles is not a consent-inventory finding.
		if len(app.DelegatedScopes) == 0 && len(app.ApplicationPermissions) == 0 {
			continue
		}

		// Risk flags + score (least-privilege framing).
		if len(app.ApplicationPermissions) > 0 {
			app.RiskFlags = append(app.RiskFlags, "application-permissions")
			app.RiskScore += 3
			res.Summary.AppsWithApplicationPerms++
		}
		if hasEscalation {
			app.RiskFlags = append(app.RiskFlags, "privilege-escalation")
			app.RiskScore += 3
			res.Summary.AppsWithEscalationPerms++
		}
		if len(app.HighPrivilegeScopes) > 0 {
			app.RiskFlags = append(app.RiskFlags, "high-privilege-delegated")
			app.RiskScore += 2
		}
		if app.AdminConsented {
			app.RiskFlags = append(app.RiskFlags, "admin-consented")
			app.RiskScore++
			res.Summary.AdminConsentedApps++
		}
		if app.UserConsentGrants > 0 {
			app.RiskFlags = append(app.RiskFlags, "user-consented")
			app.RiskScore++
			res.Summary.UserConsentedApps++
		}
		if !app.AccountEnabled {
			app.RiskFlags = append(app.RiskFlags, "disabled-but-consented")
			app.RiskScore++
			res.Summary.DisabledAppsWithConsent++
		}

		if origin == "internal" {
			res.Summary.InternalApps++
		} else {
			res.Summary.ExternalApps++
		}
		res.Summary.ThirdPartyApps++
		if app.RiskScore >= highRiskScore {
			res.Summary.HighRiskApps++
		}
		res.Apps = append(res.Apps, app)
	}

	// Highest risk first, then stable by name.
	sort.SliceStable(res.Apps, func(i, j int) bool {
		if res.Apps[i].RiskScore != res.Apps[j].RiskScore {
			return res.Apps[i].RiskScore > res.Apps[j].RiskScore
		}
		return res.Apps[i].DisplayName < res.Apps[j].DisplayName
	})

	if res.Summary.ThirdPartyApps == 0 {
		res.Note = "no third-party app consents found; if this tenant has enterprise apps, confirm the token carries Application.Read.All + Directory.Read.All (read-only) and that servicePrincipals returned rows"
	}
	return res
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
