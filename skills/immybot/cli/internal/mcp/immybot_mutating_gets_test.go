// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"immybot-pp-cli/internal/cliutil"
)

// reviewedReadGETs are the GETs (and their HEAD twins) that only read, each reviewed by its name and
// its spec.json 200 schema. Every GET endpoint is either here or declared in
// cliutil.MutatingGETPaths: a GET whose response the spec leaves undocumented
// and whose name suggests an action is declared mutating, failing closed. A
// reprint or a spec refresh that adds a GET fails until someone decides which
// it is.
var reviewedReadGETs = []string{
	"/api/v1/application-locks",
	"/api/v1/application-locks/realtime-event-stream",
	"/api/v1/application-logs/source-contexts",
	"/api/v1/audits/global/dx",
	"/api/v1/audits/local/dx",
	"/api/v1/auth",
	"/api/v1/auth/get-azure-tenant-auth-details/{azureTenantPrincipalId}",
	"/api/v1/auth/get-ip-addresses",
	"/api/v1/auth/me/permissions/{permissionType}/tenants",
	"/api/v1/azure-errors/dx",
	"/api/v1/azure-errors/for-tenant/{tenantPrincipalId}/dx",
	"/api/v1/azure/app-registration-options",
	"/api/v1/azure/partner-tenant-customers/{partnerPrincipalId}",
	"/api/v1/azure/partner-tenant-infos",
	"/api/v1/billing/billing-platform-details",
	"/api/v1/billing/credit-cards",
	"/api/v1/billing/download-invoice",
	"/api/v1/billing/feature-usage-counts",
	"/api/v1/billing/information",
	"/api/v1/billing/product-catalog",
	"/api/v1/billing/product-catalog-items",
	"/api/v1/billing/subscription-details",
	"/api/v1/brandings",
	"/api/v1/brandings/support-branding",
	"/api/v1/brandings/{id}",
	"/api/v1/change-requests/dx",
	"/api/v1/change-requests/open-count",
	"/api/v1/chocolatey/find-packages-by-id",
	"/api/v1/chocolatey/search",
	"/api/v1/computers",
	"/api/v1/computers/agent-status",
	"/api/v1/computers/dx",
	"/api/v1/computers/export",
	"/api/v1/computers/inventory",
	"/api/v1/computers/inventory-software/search-by-name",
	"/api/v1/computers/inventory-software/search-by-upgrade-code",
	"/api/v1/computers/inventory/export",
	"/api/v1/computers/my",
	"/api/v1/computers/onboarding",
	"/api/v1/computers/paged",
	"/api/v1/computers/user-affinities",
	"/api/v1/computers/user-affinities/export",
	"/api/v1/computers/{computerId}",
	"/api/v1/computers/{computerId}/detected-computer-software",
	"/api/v1/computers/{computerId}/device-update-form-data",
	"/api/v1/computers/{computerId}/ephemeral-agent",
	"/api/v1/computers/{computerId}/ephemeral-agent-circuit-breaker",
	"/api/v1/computers/{computerId}/events",
	"/api/v1/computers/{computerId}/inventory-script-results/{inventoryKey}",
	"/api/v1/computers/{computerId}/parent-tenant-info",
	"/api/v1/computers/{computerId}/provider-links/{providerLinkId}/screen-share-url",
	"/api/v1/computers/{computerId}/resolve-onboarding-overridable-target-assignments",
	"/api/v1/computers/{computerId}/status",
	"/api/v1/dynamic-provider-types",
	"/api/v1/dynamic-provider-types/global/{id}",
	"/api/v1/dynamic-provider-types/local/{id}",
	"/api/v1/ephemeral-session/development/latest-ephemeral-binary",
	"/api/v1/getting-started/checklist",
	"/api/v1/groups",
	"/api/v1/groups/{groupId}",
	"/api/v1/groups/{groupId}/members",
	"/api/v1/groups/{groupId}/role-assignments",
	"/api/v1/immy-agent-metadata/agent-hash",
	"/api/v1/inventory-tasks",
	"/api/v1/licenses",
	"/api/v1/licenses/dx",
	"/api/v1/licenses/{licenseId}",
	"/api/v1/licenses/{licenseId}/request-download",
	"/api/v1/maintenance-actions/computer/{computerId}/needs-attention",
	"/api/v1/maintenance-actions/dx",
	"/api/v1/maintenance-actions/dx-for-computer/{computerId}",
	"/api/v1/maintenance-actions/latest-for-computer/{computerId}",
	"/api/v1/maintenance-actions/latest-for-tenant/{tenantId}",
	"/api/v1/maintenance-actions/latest-non-compliant-actions-for-tenant/{tenantId}",
	"/api/v1/maintenance-actions/maintenance-item",
	"/api/v1/maintenance-actions/version",
	"/api/v1/maintenance-actions/{actionId}/logs",
	"/api/v1/maintenance-sessions/dx",
	"/api/v1/maintenance-sessions/status-counts",
	"/api/v1/maintenance-sessions/{sessionId}",
	"/api/v1/maintenance-sessions/{sessionId}/last-log",
	"/api/v1/maintenance-sessions/{sessionId}/logs",
	"/api/v1/maintenance-sessions/{sessionId}/old-logs",
	"/api/v1/maintenance-sessions/{sessionId}/phases",
	"/api/v1/maintenance-tasks/global",
	"/api/v1/maintenance-tasks/global/{id}",
	"/api/v1/maintenance-tasks/local",
	"/api/v1/maintenance-tasks/local/{id}",
	"/api/v1/maintenance-tasks/local/{id}/migrate-local-to-global-what-if",
	"/api/v1/maintenance-tasks/reference-count",
	"/api/v1/maintenance-tasks/search",
	"/api/v1/maintenance-tasks/{id}/authorization",
	"/api/v1/me/permissions",
	"/api/v1/media/global",
	"/api/v1/media/global/{id}",
	"/api/v1/media/global/{id}/download-url",
	"/api/v1/media/local",
	"/api/v1/media/local/{id}",
	"/api/v1/media/local/{id}/authorization",
	"/api/v1/media/local/{id}/download-url",
	"/api/v1/media/search",
	"/api/v1/metrics/circuit-breakers",
	"/api/v1/metrics/provider-links",
	"/api/v1/metrics/provider-links/{providerLinkId}/rate-limit-statistics",
	"/api/v1/notifications",
	"/api/v1/notifications/dx",
	"/api/v1/notifications/unacknowledged",
	"/api/v1/oauth/oauth-access-tokens",
	"/api/v1/oauth/oauth-access-tokens/{id}/{accessTokenId}",
	"/api/v1/persons",
	"/api/v1/persons/dx",
	"/api/v1/persons/requesting-access",
	"/api/v1/persons/{id}",
	"/api/v1/persons/{id}/self-service",
	"/api/v1/persons/{personId}/detected-software",
	"/api/v1/preferences",
	"/api/v1/preferences/tenants/{tenantId}",
	"/api/v1/provider-agents/pending",
	"/api/v1/provider-agents/pending-counts",
	"/api/v1/provider-agents/{agentId}/identification-logs",
	"/api/v1/provider-agents/{computerId}/pending-conflicts",
	"/api/v1/provider-links",
	"/api/v1/provider-links/{id}",
	"/api/v1/provider-links/{id}/asset-field-mappings",
	"/api/v1/provider-links/{id}/asset-fields",
	"/api/v1/provider-links/{id}/asset-fields/{externalFieldId}/samples",
	"/api/v1/provider-links/{id}/asset-match-fields",
	"/api/v1/provider-links/{id}/asset-match-fields/recommended",
	"/api/v1/provider-links/{id}/assets/dx",
	"/api/v1/provider-links/{id}/assets/job-state",
	"/api/v1/provider-links/{id}/assets/schedule",
	"/api/v1/provider-links/{id}/assets/{externalAssetId}/url",
	"/api/v1/provider-links/{id}/audits",
	"/api/v1/provider-links/{id}/clients",
	"/api/v1/provider-links/{id}/clients/statuses",
	"/api/v1/provider-links/{id}/clients/types",
	"/api/v1/provider-links/{id}/immy-field-samples",
	"/api/v1/provider-links/{id}/tickets/{ticketId}",
	"/api/v1/provider-types",
	"/api/v1/provider-types/client-group-types/{clientGroupTypeId}/client-groups",
	"/api/v1/provider-types/device-group-types/{deviceGroupTypeId}/device-groups",
	"/api/v1/provider-types/form-dropdown-options/{key}",
	"/api/v1/rmm-links",
	"/api/v1/rmm-links/{id}",
	"/api/v1/roles",
	"/api/v1/roles/permissions",
	"/api/v1/roles/{roleId}",
	"/api/v1/roles/{roleId}/assignments",
	"/api/v1/schedules",
	"/api/v1/schedules/running-schedule-ids",
	"/api/v1/schedules/{scheduleId}",
	"/api/v1/scripts/disabled-preflight-scripts",
	"/api/v1/scripts/dx",
	"/api/v1/scripts/functions",
	"/api/v1/scripts/global",
	"/api/v1/scripts/global/names",
	"/api/v1/scripts/global/{scriptId}",
	"/api/v1/scripts/global/{scriptId}/audit",
	"/api/v1/scripts/global/{scriptId}/references",
	"/api/v1/scripts/local",
	"/api/v1/scripts/local/names",
	"/api/v1/scripts/local/{scriptId}",
	"/api/v1/scripts/local/{scriptId}/audit",
	"/api/v1/scripts/local/{scriptId}/authorization",
	"/api/v1/scripts/local/{scriptId}/migrate-local-to-global-what-if",
	"/api/v1/scripts/local/{scriptId}/references",
	"/api/v1/scripts/references/count",
	"/api/v1/scripts/search",
	"/api/v1/smtp-configs",
	"/api/v1/smtp-configs/{tenantId}",
	"/api/v1/software/global",
	"/api/v1/software/global/{softwareIdentifier}",
	"/api/v1/software/global/{softwareIdentifier}/latest",
	"/api/v1/software/global/{softwareIdentifier}/versions",
	"/api/v1/software/global/{softwareIdentifier}/versions/{semanticVersion}",
	"/api/v1/software/global/{softwareIdentifier}/versions/{semanticVersion}/request-download",
	"/api/v1/software/local",
	"/api/v1/software/local/{softwareIdentifier}",
	"/api/v1/software/local/{softwareIdentifier}/authorization",
	"/api/v1/software/local/{softwareIdentifier}/latest",
	"/api/v1/software/local/{softwareIdentifier}/migrate-local-to-global-what-if",
	"/api/v1/software/local/{softwareIdentifier}/versions",
	"/api/v1/software/local/{softwareIdentifier}/versions/{semanticVersion}",
	"/api/v1/software/local/{softwareIdentifier}/versions/{semanticVersion}/request-download",
	"/api/v1/system/immy-support-access-grant-details",
	"/api/v1/system/releases",
	"/api/v1/system/timezones",
	"/api/v1/tags",
	"/api/v1/tags/{tagId}",
	"/api/v1/tags/{tagId}/authorization",
	"/api/v1/target-assignments",
	"/api/v1/target-assignments/change-request/{changeRequestId}",
	"/api/v1/target-assignments/change-request/{changeRequestId}/diff",
	"/api/v1/target-assignments/change-requests",
	"/api/v1/target-assignments/global",
	"/api/v1/target-assignments/global/{id}",
	"/api/v1/target-assignments/global/{id}/type",
	"/api/v1/target-assignments/maintenance-item-orders",
	"/api/v1/target-assignments/optional-target-assignment-approvals/computer/{computerId}",
	"/api/v1/target-assignments/recommended-approvals",
	"/api/v1/target-assignments/{deploymentId}/change-requests",
	"/api/v1/target-assignments/{id}",
	"/api/v1/target-assignments/{id}/type",
	"/api/v1/tenants",
	"/api/v1/tenants/computer-counts",
	"/api/v1/tenants/excluded-from-cross-tenant-deployments",
	"/api/v1/tenants/software-from-inventory/dx",
	"/api/v1/tenants/software-from-inventory/export",
	"/api/v1/tenants/software-from-inventory/{id}",
	"/api/v1/tenants/{id}",
	"/api/v1/tenants/{id}/azure-groups",
	"/api/v1/tenants/{id}/azure-groups/{groupId}",
	"/api/v1/tenants/{id}/computers/excluded-from-maintenance",
	"/api/v1/user-role-assignments",
	"/api/v1/user-role-assignments/users/{userId}",
	"/api/v1/user-role-assignments/users/{userId}/count",
	"/api/v1/users",
	"/api/v1/users/claims",
	"/api/v1/users/{userId}",
	"/api/v1/users/{userId}/groups",
	"/auth/me",
}

func TestEveryGETIsClassified(t *testing.T) {
	reads := map[string]bool{}
	for _, p := range reviewedReadGETs {
		reads[p] = true
	}
	endpoints := map[string]bool{}
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if m := codeOrchNormalizeMethod(ep.Method); m != "GET" && m != "HEAD" {
			continue
		}
		endpoints[ep.Path] = true
		mutating := cliutil.IsMutatingGETPath(ep.Path)
		if mutating != ep.Mutating || mutating != ep.NeedsConfirm {
			t.Errorf("%s: declared mutating %v, but Mutating %v and NeedsConfirm %v", ep.ID, mutating, ep.Mutating, ep.NeedsConfirm)
		}
		if mutating == reads[ep.Path] {
			t.Errorf("%s (%s): classify it once, in cliutil.MutatingGETPaths or reviewedReadGETs", ep.ID, ep.Path)
		}
	}
	for _, p := range append(cliutil.MutatingGETPaths(), reviewedReadGETs...) {
		if !endpoints[p] {
			t.Errorf("classified GET %s is no endpoint", p)
		}
	}
}

// A GET that changes state is a write over MCP (round-1 next-batch review):
// immybot_read refuses it, the read-only switch refuses it, and
// immybot_execute previews it until confirm is true.
func TestAMutatingGETNeverRunsWithoutConfirm(t *testing.T) {
	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"ok"`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	call := func(handler func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error), args map[string]any) *mcplib.CallToolResult {
		var req mcplib.CallToolRequest
		req.Params.Arguments = args
		res, err := handler(context.Background(), req)
		if err != nil || res == nil {
			t.Fatalf("%v: %v", args, err)
		}
		return res
	}
	cancel := map[string]any{"endpoint_id": "maintenance-sessions.get-cancel-for-schedule-by-schedule-id", "params": map[string]any{"scheduleId": 4421}}
	if res := call(handleCodeOrchRead, cancel); !res.IsError || hits.Load() != 0 {
		t.Fatalf("immybot_read ran the schedule cancellation (%d requests)", hits.Load())
	}
	if res := call(handleCodeOrchExecute, cancel); hits.Load() != 0 {
		t.Fatalf("immybot_execute ran it without confirm: %#v", res)
	}
	t.Setenv("IMMYBOT_MCP_READ_ONLY", "1")
	confirmed := map[string]any{"endpoint_id": cancel["endpoint_id"], "params": cancel["params"], "confirm": true}
	if res := call(handleCodeOrchExecute, confirmed); !res.IsError || hits.Load() != 0 {
		t.Fatalf("the read-only switch let it run (%d requests)", hits.Load())
	}
	t.Setenv("IMMYBOT_MCP_READ_ONLY", "")
	if res := call(handleCodeOrchExecute, confirmed); res.IsError || hits.Load() != 1 {
		t.Fatalf("a confirmed call did not run once (%d requests): %#v", hits.Load(), res)
	}
}
