// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// liongard handfix (r2-narrowed-sync-is-not-a-snapshot): the sync walk's
// verdict on whether it fetched a full, unnarrowed snapshot of a resource.
// sync.go records it in sync_state (store.SaveSyncSnapshotAtInScope) and the
// store-backed commands refuse a resource whose last finished sync was not
// one (requireCompleteSnapshot).

import (
	"sort"
	"strings"
)

// syncWalkNarrowing lists the user query parameters a flat sync walk of
// resource sends (--param, --global-param, --resource-param). Liongard's
// list endpoints document only narrowing parameters (fields[] projects the
// fields, conditions[] filters the rows) and no paging, and the walk adds
// none of its own, so any one of them narrows the walk. A closed rule: no
// parameter is judged harmless by name.
func syncWalkNarrowing(resource string, userParams *syncUserParams) []string {
	if userParams == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, src := range []map[string]string{userParams.flatGlobal, userParams.trueGlobal, userParams.perResource[resource]} {
		for k := range src {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// syncWalkIsFullSnapshot is the walk's verdict. Any one signal makes it not
// full: the walk did not reach a proven natural end (an error, a --max-pages
// or --latest-only cap with more pages, a stuck cursor); a user query
// parameter narrowed it; it resumed from a cursor an earlier attempt left,
// so it did not fetch the earlier pages itself; or it sent a since window
// over a checkpoint that was not itself a full snapshot (a window refreshes
// a full snapshot; on its own it is only the rows that changed).
func syncWalkIsFullSnapshot(resource string, userParams *syncUserParams, walkComplete bool, resumedCursor, effectiveSince string, priorFull bool) bool {
	if !walkComplete {
		return false
	}
	if len(syncWalkNarrowing(resource, userParams)) > 0 {
		return false
	}
	if strings.TrimSpace(resumedCursor) != "" {
		return false
	}
	if strings.TrimSpace(effectiveSince) != "" && !priorFull {
		return false
	}
	return true
}
