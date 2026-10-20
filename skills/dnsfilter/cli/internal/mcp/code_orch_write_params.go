// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-execute-writes-refuse-unknown-args" entry.

package mcp

import (
	"fmt"
	"sort"
	"strings"
)

// codeOrchWriteBodyKeys lists, per "METHOD /path/template", the top-level
// JSON body fields of every POST/PUT/PATCH endpoint (a Rails-style wrapper
// such as "network", or the flat fields). It is derived from the generated
// companion CLI (the body each internal/cli command builds from its flags);
// TestCodeOrchWriteBodyKeysMatchCLI re-derives it from internal/cli and fails
// on drift, so a regeneration that changes a body cannot leave this stale.
var codeOrchWriteBodyKeys = map[string][]string{
	"PATCH /v1/agent_local_users/{id}":                                  {"agent_local_user"},
	"PATCH /v1/billing/address/{organization_id}":                       {"billing_address"},
	"PATCH /v1/block_pages/{id}":                                        {"block_email_addr", "block_logo_uuid", "block_org_name", "created_at", "name", "organization_id", "updated_at"},
	"PATCH /v1/current_user":                                            {"user"},
	"PATCH /v1/distributors/distributors":                               {"distributor"},
	"PATCH /v1/distributors/msps/{id}":                                  {"msp"},
	"PATCH /v1/distributors/organizations/{id}":                         {"msp"},
	"PATCH /v1/enterprise_connections/{id}":                             {"authorized_domains", "default_organization_id", "display_name", "idp", "options", "organization_id", "role_default", "role_map"},
	"PATCH /v1/ip_addresses/{id}":                                       {"ip_address"},
	"PATCH /v1/mac_addresses/{id}":                                      {"mac_address"},
	"PATCH /v1/networks/{id}":                                           {"network"},
	"PATCH /v1/networks/{id}/lan_ips/{lan_ip_id}":                       {"network_lan_ip"},
	"PATCH /v1/networks/{id}/secret_key":                                {},
	"PATCH /v1/networks/{id}/subnets/{subnet_id}":                       {"network_subnet"},
	"PATCH /v1/notes/{resource}/{id}/batch_update":                      {},
	"PATCH /v1/notes/{resource}/{id}/{domain}":                          {"domain", "note", "type"},
	"PATCH /v1/organizations/bulk_update":                               {"exclude_organization_ids", "msp_id", "organization_ids", "send_uninstall_notifications_to_admin_users", "user_agent_uninstall_notification", "user_agent_uninstall_notification_recipient_emails", "user_agents_auto_update", "vpn_settings_organization_attributes"},
	"PATCH /v1/organizations/{id}":                                      {"organization"},
	"PATCH /v1/organizations/{organization_id}/users/{id}":              {"user"},
	"PATCH /v1/policies/{id}":                                           {"append_domains", "include_relationships", "policy"},
	"PATCH /v1/scheduled_policies/{id}":                                 {"name", "organization_id", "policy_ids", "timezone"},
	"PATCH /v1/scheduled_reports/{id}":                                  {"scheduled_report"},
	"PATCH /v1/user_agents/{id}":                                        {"user_agent"},
	"PATCH /v1/users/change_password":                                   {"new_password"},
	"PATCH /v2/current_user/ui_settings":                                {"disable_license_warnings", "theme_mode", "user_uuid"},
	"PATCH /v2/dashboards/{id}":                                         {"dashboard"},
	"PATCH /v2/scheduled_reports/{id}":                                  {"scheduled_report"},
	"PATCH /v2/user_agents/{id}/update_settings":                        {"user_agent"},
	"POST /v1/agent_local_user_bulk_deletes":                            {"exclude_ids", "ids"},
	"POST /v1/api_keys":                                                 {"expiry", "name"},
	"POST /v1/api_keys/{id}/revoke":                                     {},
	"POST /v1/billing":                                                  {"organization_id", "payment_token"},
	"POST /v1/block_pages":                                              {"block_email_addr", "block_logo_uuid", "block_org_name", "created_at", "name", "organization_id", "updated_at"},
	"POST /v1/collections/{collection_id}/users":                        {"collection_id", "id"},
	"POST /v1/distributors/msps":                                        {"msp"},
	"POST /v1/distributors/msps/{id}/cancel":                            {"distributor_status"},
	"POST /v1/distributors/msps/{id}/reactivate":                        {"distributor_status", "reactivate_sub_organizations"},
	"POST /v1/distributors/msps/{id}/upsert_skus":                       {"add", "remove"},
	"POST /v1/distributors/organizations":                               {"msp"},
	"POST /v1/distributors/organizations/{id}/add_sku":                  {"sku"},
	"POST /v1/distributors/organizations/{id}/cancel":                   {"distributor_status"},
	"POST /v1/distributors/organizations/{id}/reactivate":               {"distributor_status"},
	"POST /v1/distributors/organizations/{id}/remove_sku":               {"sku"},
	"POST /v1/distributors/users":                                       {"membership", "return-reset_password_url", "send_reset_password_email", "user"},
	"POST /v1/distributors/users/{id}/add_membership":                   {"membership"},
	"POST /v1/distributors/users/{id}/remove_membership":                {"organization_id"},
	"POST /v1/distributors/users/{id}/reset_password_url":               {},
	"POST /v1/distributors/users/{id}/send_reset_password_email":        {},
	"POST /v1/distributors/users/{id}/update_membership":                {"membership"},
	"POST /v1/domains/suggest_threat":                                   {},
	"POST /v1/enterprise_connections":                                   {"authorized_domains", "default_organization_id", "display_name", "idp", "options", "organization_id", "role_default", "role_map", "strategy"},
	"POST /v1/ip_addresses":                                             {"ip_address"},
	"POST /v1/mac_addresses":                                            {"mac_address"},
	"POST /v1/networks":                                                 {"network"},
	"POST /v1/networks/bulk_create":                                     {"networks"},
	"POST /v1/networks/bulk_update":                                     {"block_page_id", "ids", "is_legacy_vpn_active", "organization_id", "policy_id", "scheduled_policy_id"},
	"POST /v1/networks/{id}/secret_key":                                 {},
	"POST /v1/networks/{id}/subnets":                                    {"network_subnet"},
	"POST /v1/organizations":                                            {"organization"},
	"POST /v1/organizations/promote_to_msp":                             {},
	"POST /v1/organizations/{id}/cancel":                                {},
	"POST /v1/organizations/{organization_id}/users":                    {"user"},
	"POST /v1/organizations/{organization_id}/users/{id}/resend_invite": {},
	"POST /v1/policies":                                                 {"append_domains", "include_relationships", "policy"},
	"POST /v1/policies/application_update":                              {},
	"POST /v1/policies/bulk/add_allowlist_domains":                      {"domains", "notes", "policy_ids"},
	"POST /v1/policies/bulk/add_blocklist_domains":                      {"domains", "notes", "policy_ids"},
	"POST /v1/policies/bulk/remove_allowlist_domains":                   {"domains", "notes", "policy_ids"},
	"POST /v1/policies/bulk/remove_blocklist_domains":                   {"domains", "notes", "policy_ids"},
	"POST /v1/policies/{id}/add_allowed_application":                    {"include_relationships", "name"},
	"POST /v1/policies/{id}/add_blacklist_category":                     {"category_id", "include_relationships"},
	"POST /v1/policies/{id}/add_blacklist_domain":                       {"domain", "include_relationships", "note"},
	"POST /v1/policies/{id}/add_blocked_application":                    {"include_relationships", "name"},
	"POST /v1/policies/{id}/add_whitelist_domain":                       {"domain", "include_relationships", "note"},
	"POST /v1/policies/{id}/permissive_mode":                            {},
	"POST /v1/policies/{id}/remove_allowed_application":                 {"include_relationships", "name"},
	"POST /v1/policies/{id}/remove_blacklist_category":                  {"category_id", "include_relationships"},
	"POST /v1/policies/{id}/remove_blacklist_domain":                    {"domain", "include_relationships", "note"},
	"POST /v1/policies/{id}/remove_blocked_application":                 {"include_relationships", "name"},
	"POST /v1/policies/{id}/remove_whitelist_domain":                    {"domain", "include_relationships", "note"},
	"POST /v1/scheduled_policies":                                       {"name", "organization_id", "policy_ids", "timezone"},
	"POST /v1/scheduled_report_previews":                                {"scheduled_report_preview"},
	"POST /v1/scheduled_reports":                                        {"scheduled_report"},
	"POST /v1/trials":                                                   {},
	"POST /v1/user_agent_bulk_deletes":                                  {"exclude_ids", "ids"},
	"POST /v1/user_agent_bulk_updates":                                  {"changeset", "exclude_ids", "ids"},
	"POST /v1/user_agent_bulk_updates/has_mixed":                        {"ids"},
	"POST /v1/user_agent_cleanups":                                      {"inactive_for", "organization_id", "organization_ids"},
	"POST /v1/user_agent_csv_exports":                                   {"agent_state", "name_search", "network_ids", "search", "state", "status", "tags", "traffic_received_last_15_mins", "type", "user_agent_csv_export"},
	"POST /v1/user_agents/dequeue_uninstall":                            {},
	"POST /v2/agent_local_users_csv_export":                             {"agent_local_users_csv_export"},
	"POST /v2/current_user/suppress_license_warning":                    {"organization_uuid"},
	"POST /v2/cyber_sight/csv_exports/create":                           {"activity_type_ids", "agent_local_user_uuids", "ai_tool", "app_executable_path", "app_name", "app_window_title", "category_ids", "cyber_sight_csv_export", "end_at", "excluded_org_uuids", "msp_uuid", "org_uuids", "report_type", "search_id", "size", "sort_by", "sort_direction", "start_at", "threats_only", "user_agent_uuids", "web_categories", "web_full_url", "web_host", "weekdays_only"},
	"POST /v2/dashboards":                                               {"dashboard"},
	"POST /v2/dashboards/{id}/link":                                     {},
	"POST /v2/dashboards/{id}/make_private":                             {},
	"POST /v2/dashboards/{id}/make_public":                              {},
	"POST /v2/dashboards/{id}/unlink":                                   {},
	"POST /v2/networks_csv_export":                                      {"networks_csv_export"},
	"POST /v2/scheduled_reports":                                        {"scheduled_report"},
	"POST /v2/scheduled_reports/bulk_update":                            {"active", "ids"},
	"POST /v2/unblock_requests/{id}/decision":                           {"decision", "domains", "internal_notes", "message"},
	"POST /v2/unblock_requests_csv_export":                              {"unblock_requests_csv_export"},
	"PUT /v1/user_agent_cleanups/{id}":                                  {"inactive_for", "start"},
}

// codeOrchRefuseUnknownWriteParams refuses a write whose params carry a key
// outside the endpoint's schema. dnsfilter_execute has no dry-run: the
// generated handler forwarded every unrecognized POST/PUT/PATCH param into the
// JSON body and every unrecognized DELETE param into the query string, so an
// agent passing a CLI spelling such as dry-run or dry_run to preview a change
// ran it live. Path placeholders, declared query and header params are
// accepted under their public or wire names; on POST/PUT/PATCH the remaining
// keys must be the endpoint's top-level body fields; on DELETE nothing else is
// accepted. GET is unchanged (extra params stay query params). An endpoint
// missing from codeOrchWriteBodyKeys accepts no body field (fail closed).
func codeOrchRefuseUnknownWriteParams(ep *codeOrchEndpoint, params map[string]any) error {
	method := strings.ToUpper(ep.Method)
	if method == "GET" || len(params) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, p := range ep.Positional {
		allowed[p] = true
	}
	for _, group := range [][]codeOrchParamBinding{ep.QueryParams, ep.HeaderParams, ep.TemplateParams} {
		for _, b := range group {
			allowed[b.PublicName] = true
			allowed[b.WireName] = true
		}
	}
	if method != "DELETE" {
		if ep.BodyIsArray {
			allowed["body"] = true
		}
		for _, k := range codeOrchWriteBodyKeys[method+" "+ep.Path] {
			allowed[k] = true
		}
	}
	var unknown []string
	for k := range params {
		if !allowed[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	accepted := make([]string, 0, len(allowed))
	for k := range allowed {
		if k != "" {
			accepted = append(accepted, k)
		}
	}
	sort.Strings(accepted)
	hint := "none"
	if len(accepted) > 0 {
		hint = strings.Join(accepted, ", ")
	}
	return fmt.Errorf("unknown MCP parameter %q on a write tool; use the tool schema's named parameters (%s %s accepts: %s). dnsfilter_execute has no dry-run and nothing was sent", unknown[0], method, ep.ID, hint)
}
