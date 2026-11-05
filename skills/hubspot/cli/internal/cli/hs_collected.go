// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json hubspot-reports-read-only-collected-properties):
// HubSpot answers a list or read with only the properties it names, or the
// object's defaults when it names none (press/tools-manifest.json, the
// "properties" parameter of every /crm/v3/objects list and read). Sync and the
// live timeline therefore request each object's defaults (never narrowed) plus
// every property a report reads (hsReportProperties). A requested property with
// no value comes back null, so a stored record that LACKS one was never
// collected (an older build's sync, a narrow live read): the report refuses and
// names the full sync instead of reading the absence as empty.

import (
	"context"
	"fmt"
	"strings"

	"hubspot-pp-cli/internal/store"
)

// hsEngagementDefaultProperties is HubSpot's default property set for calls,
// emails, meetings, notes and tasks.
const hsEngagementDefaultProperties = "hs_createdate,hs_lastmodifieddate,hs_object_id"

// hsSyncProperties is each object's default property set (probed live) plus
// what deal state needs; hsCollectedProperties adds hsReportProperties.
var hsSyncProperties = map[string]string{
	"hubspot-deals-crm":     hsDealSyncProperties,
	"hubspot-contacts-crm":  "createdate,email,firstname,hs_object_id,lastmodifieddate,lastname",
	"hubspot-companies-crm": "createdate,domain,hs_lastmodifieddate,hs_object_id,name",
	"hubspot-calls-crm":     hsEngagementDefaultProperties,
	"hubspot-emails-crm":    hsEngagementDefaultProperties,
	"hubspot-meetings-crm":  hsEngagementDefaultProperties,
	"hubspot-notes-crm":     hsEngagementDefaultProperties,
	"hubspot-tasks-crm":     hsEngagementDefaultProperties,
}

// hsReportProperties are the properties outside HubSpot's defaults that a
// report reads, per object. hsRequireCollected refuses when a stored record
// lacks one.
var hsReportProperties = map[string][]string{
	"hubspot-deals-crm":     {"hubspot_owner_id", "notes_last_contacted", "notes_last_updated"},
	"hubspot-contacts-crm":  {"hs_lastmodifieddate", "hubspot_owner_id", "lifecyclestage", "notes_last_contacted", "notes_last_updated"},
	"hubspot-companies-crm": {"hubspot_owner_id"},
	"hubspot-calls-crm":     {"hs_call_disposition", "hs_call_title", "hubspot_owner_id"},
	"hubspot-emails-crm":    {"hs_email_subject", "hubspot_owner_id"},
	"hubspot-meetings-crm":  {"hs_meeting_outcome", "hs_meeting_title", "hubspot_owner_id"},
	"hubspot-notes-crm":     {"hs_note_body", "hubspot_owner_id"},
	"hubspot-tasks-crm":     {"hs_task_status", "hs_task_subject", "hubspot_owner_id"},
}

// hsCollectedProperties is the properties= value a read of resource sends:
// the defaults, the report properties, then whatever the user asked for. A
// user list adds to it and never narrows it. Other resources keep user as is.
func hsCollectedProperties(resource, user string) string {
	base, ok := hsSyncProperties[resource]
	if !ok {
		return user
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range append(append(strings.Split(base, ","), hsReportProperties[resource]...), strings.Split(user, ",")...) {
		if p = strings.TrimSpace(p); p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// hsRequireCollected fails command when any unarchived stored record of
// resource lacks a property sync collects for it (the defaults included: a
// projected live read stores a record without them) or a property its
// --filter names (filterFields). A table not created yet has nothing to check. A
// filter property sync does not request by default is named in the remedy's
// --param properties=, which adds it to what sync collects.
func hsRequireCollected(ctx context.Context, db *store.Store, command, resource string, filterFields ...string) error {
	table := strings.ReplaceAll(resource, "-", "_")
	if n, err := hsTableCount(ctx, db, table); err != nil || n == 0 {
		return err
	}
	collected := "," + hsCollectedProperties(resource, "") + ","
	seen := map[string]bool{}
	var missing, extra []string
	for _, p := range append(strings.Split(strings.Trim(collected, ","), ","), filterFields...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		var n int
		if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE COALESCE(archived, 0) = 0 AND json_type(data, '$.properties."' || ? || '"') IS NULL`, p).Scan(&n); err != nil {
			return fmt.Errorf("%s: checking which %s properties were collected: %w", command, resource, err)
		}
		if n > 0 {
			missing = append(missing, fmt.Sprintf("%s (%d records)", p, n))
			if !strings.Contains(collected, ","+p+",") {
				extra = append(extra, p)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	syncCmd := "hubspot-cli sync --resources " + resource + " --full"
	mcpArgs := "resources " + resource + " and full"
	note := ""
	if len(extra) > 0 {
		syncCmd += " --param properties=" + strings.Join(extra, ",")
		mcpArgs += " and param properties=" + strings.Join(extra, ",")
		note = " (HubSpot never returns a property it does not define for this object)"
	}
	return fmt.Errorf("%s: stored %s records were synced without %s, so a value never collected cannot be told from an empty one. No figure is reported on a guess. Run '%s' (over MCP: the sync tool with %s)%s, then rerun",
		command, resource, strings.Join(missing, ", "), syncCmd, mcpArgs, note)
}
