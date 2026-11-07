// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
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
			sps, err := graphGetAllPages(ctx, c, "/servicePrincipals", spParams)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			// --dry-run already printed the primary request; stop before the
			// dependent fan-out so a dry run shows one representative call.
			if dryRunOK(flags) {
				return nil
			}

			// 2. Delegated permission grants (oauth2PermissionGrants).
			grants, err := graphGetAllPages(ctx, c, "/oauth2PermissionGrants", map[string]string{"$top": "999"})
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}

			// Home tenant id (best-effort) so we can split internal vs external.
			homeTenant := fetchHomeTenantID(ctx, c)

			// 3. Application (app-only) permissions per third-party SP — fanned
			//    out over non-Microsoft principals only to bound the call count.
			var roles []json.RawMessage
			var unread []string
			if appPermissions {
				targets := thirdPartyServicePrincipalIDs(sps)
				results, errs := cliutil.FanoutRun(ctx, targets,
					func(id string) string { return id },
					func(ctx context.Context, id string) ([]json.RawMessage, error) {
						return graphGetAllPages(ctx, c, "/servicePrincipals/"+id+"/appRoleAssignments", map[string]string{"$top": "999"})
					})
				cliutil.FanoutReportErrors(stderr, errs)
				for _, r := range results {
					roles = append(roles, r.Value...)
				}
				for _, e := range errs {
					unread = append(unread, e.Source)
				}
			}

			// Every collection is read through @odata.nextLink to the end; an app
			// whose application permissions could not be read is named, so it
			// never reads as holding none (Codex r1 class incomplete-read-as-complete).
			result := insights.ConsentAudit(sps, grants, roles, homeTenant)
			if len(unread) > 0 {
				sort.Strings(unread)
				result.Note = strings.TrimSpace(result.Note + fmt.Sprintf(" WARNING: the application permissions of %d app(s) could not be read, so their flags are incomplete: %s", len(unread), strings.Join(unread, ", ")))
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().BoolVar(&appPermissions, "app-permissions", true, "Fetch each third-party app's application (app-only) permissions (one call per app); disable to skip the fan-out")
	return cmd
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
	orgs, err := graphGetAllPages(ctx, c, "/organization", map[string]string{"$select": "id"})
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
