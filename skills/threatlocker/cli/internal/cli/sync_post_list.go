// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// ThreatLocker's real list endpoints are POST-with-body. The generated sync
// loop only issues GET with query params, so the profiler wired sync to the
// only GET-able list-ish paths available: the portal's dropdown helpers
// (OrganizationGetForMoveComputers, ComputerGetForNewComputer,
// ...GetForDropdownList). Those return UI picker payloads - a handful of
// {label, value} rows carrying no primary key - which is why every resource
// logged all_items_failed_id_extraction and stored zero rows while sync still
// exited 0.
//
// This file adds a POST path for the resources the offline store is built
// around. `devices health` joins the computers table and scopes with
// --all-tenants, so it needs the real inventory rather than one dropdown page.
//
// Tenancy note: these endpoints are tenant-scoped via the
// ManagedOrganizationId header, but a request carrying childOrganizations=true
// already returns the authenticating organization's entire tree. Verified
// live on a 5-organization tenant: the top-level request returns 58 computers,
// which is exactly that organization's own 5 plus 1, 29 and 23 from its three
// populated children. Fanning out per organization and merging returns the
// same 58 rows (every child row arrives as a duplicate) for six extra API
// calls, so this deliberately does not fan out.
//
// Kept in a novel file so regen-merge preserves it; the generated sync.go
// carries only a small dispatch hook. See skills/threatlocker/handfixes.json
// (sync-post-list-endpoints).

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"threatlocker-pp-cli/internal/store"
)

// tenantPostClient is the subset of the API client this path needs.
type tenantPostClient interface {
	PostWithHeaders(context.Context, string, any, map[string]string) (json.RawMessage, int, error)
}

// tenantSyncSpec describes how to page a POST list endpoint.
type tenantSyncSpec struct {
	path string
	// body returns the request body for a 1-based page.
	body func(page int) map[string]any
	// idField is the primary key on the returned rows. ThreatLocker names
	// these per entity (computerId, organizationId) rather than a bare id,
	// which the generic fallback chain does not match.
	idField string
}

const tenantSyncPageSize = 500

// tenantSyncSpecs covers the resources whose real records are only reachable
// by POST. Resources absent here keep the generated GET path.
var tenantSyncSpecs = map[string]tenantSyncSpec{
	"organizations": {
		path: "/Organization/OrganizationGetChildOrganizationsByParameters",
		body: func(page int) map[string]any {
			return map[string]any{
				"searchText": "", "includeAllChildren": true,
				"orderBy": "name", "isAscending": true,
				"pageNumber": page, "pageSize": tenantSyncPageSize,
			}
		},
		idField: "organizationId",
	},
	"computers": {
		path: "/Computer/ComputerGetByAllParameters",
		body: func(page int) map[string]any {
			return map[string]any{
				"pageNumber": page, "pageSize": tenantSyncPageSize,
				"searchText": "", "searchBy": 1,
				"orderBy": "computername", "isAscending": true,
				// Covers the whole organization tree in one request; see the
				// tenancy note at the top of this file.
				"childOrganizations": true,
			}
		},
		idField: "computerId",
	},
}

// tenantSyncRows pulls every page of one POST list endpoint.
func tenantSyncRows(ctx context.Context, c tenantPostClient, spec tenantSyncSpec, maxPages int) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for page := 1; maxPages <= 0 || page <= maxPages; page++ {
		raw, _, err := c.PostWithHeaders(ctx, spec.path, spec.body(page), map[string]string{})
		if err != nil {
			return out, err
		}
		rows, ok := tenantExtractRows(raw)
		if !ok || len(rows) == 0 {
			return out, nil
		}
		out = append(out, rows...)
		if len(rows) < tenantSyncPageSize {
			return out, nil
		}
	}
	return out, nil
}

// tenantExtractRows unwraps ThreatLocker's {"action":..,"data":[...]} envelope,
// tolerating a bare array and the {"results":[...]} shape.
func tenantExtractRows(raw json.RawMessage) ([]json.RawMessage, bool) {
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return arr, true
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) != nil {
		return nil, false
	}
	for _, key := range []string{"data", "Data", "results", "Results"} {
		if v, ok := env[key]; ok {
			if json.Unmarshal(v, &arr) == nil {
				return arr, true
			}
		}
	}
	return nil, false
}

// syncTenantPostResource is the hand-wired replacement for the generated GET
// walk on POST-only resources, storing rows keyed by the entity's real id
// field. Rows whose id field is absent are dropped rather than stored under an
// empty key, and counted so the caller can surface them.
func syncTenantPostResource(
	ctx context.Context,
	c tenantPostClient,
	db *store.Store,
	resource string,
	spec tenantSyncSpec,
	maxPages int,
	syncEvents io.Writer,
	started time.Time,
) syncResult {
	if syncEvents == nil {
		syncEvents = io.Discard
	}

	rows, err := tenantSyncRows(ctx, c, spec, maxPages)
	if err != nil {
		if !humanFriendly {
			fmt.Fprintln(syncEvents, syncErrorJSON(resource, "", err))
		}
		return syncResult{Resource: resource, Err: fmt.Errorf("fetching %s: %w", resource, err), Duration: time.Since(started)}
	}

	seen := map[string]bool{}
	items := make([]json.RawMessage, 0, len(rows))
	unkeyed := 0
	for _, r := range rows {
		var obj map[string]any
		if json.Unmarshal(r, &obj) != nil {
			unkeyed++
			continue
		}
		id := store.ResourceIDString(store.LookupFieldValue(obj, spec.idField))
		if id == "" || id == "<nil>" {
			unkeyed++
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		items = append(items, r)
	}

	stored, extractFailures, err := db.UpsertBatch(resource, items)
	if err != nil {
		return syncResult{Resource: resource, Count: stored, Err: fmt.Errorf("storing %s: %w", resource, err), Duration: time.Since(started)}
	}
	if (unkeyed > 0 || extractFailures > 0) && !humanFriendly {
		fmt.Fprintf(syncEvents, `{"event":"sync_anomaly","resource":"%s","consumed":%d,"stored":%d,"unkeyed":%d,"extract_failures":%d,"reason":"primary_key_unresolved"}`+"\n", resource, len(rows), stored, unkeyed, extractFailures)
	}
	if !humanFriendly {
		fmt.Fprintf(syncEvents, `{"event":"sync_complete","resource":"%s","total":%d,"duration_ms":%d}`+"\n", resource, stored, time.Since(started).Milliseconds())
	}
	_ = db.SaveSyncState(resource, "", stored)
	return syncResult{Resource: resource, Count: stored, Duration: time.Since(started)}
}
