// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"strings"

	"zammad-pp-cli/internal/store"
)

// mcpStoreStatusCompleteWithSkips means every resource this token can read
// finished, and the only unfinished ones are those the vendor refused it
// (HTTP 403). Hand-fix (handfixes.json sync-forbidden-not-partial);
// tools/maintainer/replay_handfixes.py writes this file from
// handfix_replay/sync_coverage_mcp.go.tmpl after a reprint.
const mcpStoreStatusCompleteWithSkips mcpStoreStatusKind = "complete_with_skips"

// mcpIncompleteStoreStatus settles a store where some sync_state rows did not
// finish. It needs at least one finished resource and no unfinished resource
// other than a refused one; anything else stays partial.
func mcpIncompleteStoreStatus(db *store.Store, completed int) (mcpStoreStatusKind, error) {
	coverage, err := db.SyncCoverage()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return mcpStoreStatusPartial, nil // a legacy read-only store proves nothing
		}
		return "", err
	}
	if completed > 0 && len(coverage.Incomplete) == 0 && len(coverage.SkippedForbidden) > 0 {
		return mcpStoreStatusCompleteWithSkips, nil
	}
	return mcpStoreStatusPartial, nil
}

// mcpWithSyncCoverage names, on a search or sql envelope, what the store does
// not hold, so an agent can tell "everything readable is here" from "the sync
// broke". The status on the envelope is already settled; a failed read here
// only leaves the names off.
func mcpWithSyncCoverage(db *store.Store, status mcpStoreStatusKind, out map[string]any) map[string]any {
	if status != mcpStoreStatusPartial && status != mcpStoreStatusCompleteWithSkips {
		return out
	}
	coverage, err := db.SyncCoverage()
	if err != nil {
		return out
	}
	if len(coverage.SkippedForbidden) > 0 {
		out["skipped_forbidden"] = coverage.SkippedForbidden
	}
	if len(coverage.Incomplete) > 0 {
		out["incomplete_resources"] = coverage.Incomplete
	}
	if status == mcpStoreStatusCompleteWithSkips {
		out["warning"] = "Everything this token can read is here. Not stored because the vendor refused this token (HTTP 403): " + strings.Join(coverage.SkippedForbidden, ", ") + "."
	}
	return out
}
