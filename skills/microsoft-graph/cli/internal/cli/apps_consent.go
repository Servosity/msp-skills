// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/client"
	"microsoft-graph-pp-cli/internal/cliutil"
	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelAppsConsentCmd(flags *rootFlags) *cobra.Command {
	var appPermissions bool

	cmd := &cobra.Command{
		Use:   "consent",
		Short: "Inventory third-party app consents (delegated + application permissions) with least-privilege risk flags",
		Long: strings.Trim(`
Inventories every non-Microsoft enterprise application (service principal)
consented into the tenant and the access it holds — the third-party app
attack surface Entra spreads across three blades and no single Graph call
returns. Joins servicePrincipals, oauth2PermissionGrants (delegated consent),
and appRoleAssignments (application/app-only permissions) into one risk-ranked
table so an MSP can spot over-privileged, admin-consented, and shadow-IT
(user-consented) apps in a single pass.

Read-only. Microsoft's own first-party service principals are counted but not
listed — the report is about the consent you granted or inherited, not
Microsoft's plumbing. Apps registered in this tenant (homegrown) read as
"internal"; apps owned by another tenant read as "external".

Least-privilege flags: application-permissions (app-only, runs with no signed-in
user), privilege-escalation (can grant itself more access), high-privilege-delegated,
admin-consented (tenant-wide), user-consented (per-user, unmanaged), and
disabled-but-consented.

Live Graph access only — these surfaces are not part of the local sync. Needs a
token with Application.Read.All + Directory.Read.All + DelegatedPermissionGrant.Read.All
(all read-only).`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli apps consent --agent
  microsoft-graph-cli apps consent --json --select displayName,riskScore,riskFlags
  microsoft-graph-cli apps consent --app-permissions=false --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("apps consent needs live Graph access (servicePrincipals/oauth2PermissionGrants/appRoleAssignments are not part of the local sync); use --data-source live or --data-source auto"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			stderr := cmd.ErrOrStderr()

			// 1. Service principals (the enterprise-app inventory).
			spParams := map[string]string{
				"$select": "id,appId,displayName,appOwnerOrganizationId,servicePrincipalType,accountEnabled,signInAudience,tags",
				"$top":    "999",
			}
			sps, spTruncated, err := fetchGraphList(ctx, c, "/servicePrincipals", spParams)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			// --dry-run already printed the primary request; stop before the
			// dependent fan-out so a dry run shows one representative call.
			if dryRunOK(flags) {
				return nil
			}

			// 2. Delegated permission grants (oauth2PermissionGrants).
			grants, grantsTruncated, err := fetchGraphList(ctx, c, "/oauth2PermissionGrants", map[string]string{"$top": "999"})
			if err != nil {
				return classifyAPIError(err, flags)
			}

			// Home tenant id (best-effort) so we can split internal vs external.
			homeTenant := fetchHomeTenantID(ctx, c)

			// 3. Application (app-only) permissions per third-party SP — fanned
			//    out over non-Microsoft principals only to bound the call count.
			var roles []json.RawMessage
			if appPermissions {
				targets := thirdPartyServicePrincipalIDs(sps)
				results, errs := cliutil.FanoutRun(ctx, targets,
					func(id string) string { return id },
					func(ctx context.Context, id string) ([]json.RawMessage, error) {
						items, _, e := fetchGraphList(ctx, c, "/servicePrincipals/"+id+"/appRoleAssignments", map[string]string{"$top": "999"})
						return items, e
					})
				cliutil.FanoutReportErrors(stderr, errs)
				for _, r := range results {
					roles = append(roles, r.Value...)
				}
			}

			result := insights.ConsentAudit(sps, grants, roles, homeTenant)
			if spTruncated || grantsTruncated {
				result.Note = strings.TrimSpace(result.Note + " WARNING: a collection returned the 999-row page ceiling and more pages exist; counts are a floor. Large-tenant paging is the upgrade path.")
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().BoolVar(&appPermissions, "app-permissions", true, "Fetch each third-party app's application (app-only) permissions (one call per app); disable to skip the fan-out")
	return cmd
}

// fetchGraphList GETs a Graph collection and returns its items. Graph wraps
// lists in {"value":[...]}; a bare array is also accepted. truncated is true
// when the response carries an @odata.nextLink (more pages exist beyond the
// single request this helper makes).
func fetchGraphList(ctx context.Context, c *client.Client, path string, params map[string]string) (items []json.RawMessage, truncated bool, err error) {
	data, err := c.GetWithHeaders(ctx, path, params, nil)
	if err != nil {
		return nil, false, err
	}
	// Bare array response.
	if json.Unmarshal(data, &items) == nil {
		return items, false, nil
	}
	// OData envelope {"value":[...], "@odata.nextLink": "..."}.
	var env struct {
		Value    []json.RawMessage `json:"value"`
		NextLink string            `json:"@odata.nextLink"`
	}
	if json.Unmarshal(data, &env) == nil {
		return env.Value, env.NextLink != "", nil
	}
	return nil, false, nil
}

// thirdPartyServicePrincipalIDs returns the ids of every non-Microsoft service
// principal — the set worth fanning out over for application permissions.
func thirdPartyServicePrincipalIDs(sps []json.RawMessage) []string {
	ids := make([]string, 0, len(sps))
	for _, raw := range sps {
		var sp struct {
			ID                     string `json:"id"`
			AppOwnerOrganizationID string `json:"appOwnerOrganizationId"`
		}
		if json.Unmarshal(raw, &sp) != nil || sp.ID == "" {
			continue
		}
		if insights.IsMicrosoftFirstParty(sp.AppOwnerOrganizationID) {
			continue
		}
		ids = append(ids, sp.ID)
	}
	return ids
}

// fetchHomeTenantID resolves the tenant's own directory id so the audit can
// tell homegrown apps from external vendors. Best-effort: returns "" on any
// error (the audit then reads all non-Microsoft apps as external).
func fetchHomeTenantID(ctx context.Context, c *client.Client) string {
	orgs, _, err := fetchGraphList(ctx, c, "/organization", map[string]string{"$select": "id"})
	if err != nil || len(orgs) == 0 {
		return ""
	}
	var org struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(orgs[0], &org) != nil {
		return ""
	}
	return org.ID
}
