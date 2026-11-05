// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// betterstack handfix (class incomplete-snapshot-read-as-complete): every
// novel reads the
// store through a loader that calls requireFinishedSync, and a resource is
// only the whole account when its last sync finished unfiltered. A sync that
// stopped part way (last_attempt_complete = 0, --max-pages included), rows a
// live read wrote through with no sync at all, a resource never synced while
// others were, and a filtered walk (betterstack_sync_scope.go) are a partial
// set; reporting on them ("no monitors down", "every monitor covered", "no
// unacked incidents") is a wrong answer an MSP acts on. A store with nothing
// synced and nothing stored keeps the documented empty result with its sync
// hint.

import (
	"fmt"

	"betterstack-pp-cli/internal/store"
)

// requireFinishedSyncSeen, set only by tests, is told every resource a loader
// asks requireFinishedSync about. TestEveryReportRefreshesEveryResourceItReads
// (class stale-dependent-resource) runs each report with it and fails when a
// report reads a resource its readCommandResources entry does not refresh.
var requireFinishedSyncSeen func(resourceType string)

// requireFinishedSync refuses a resource whose stored rows are not backed by
// a finished, unfiltered sync of it.
func requireFinishedSync(s *store.Store, resourceType string) error {
	if requireFinishedSyncSeen != nil {
		requireFinishedSyncSeen(resourceType)
	}
	var complete int
	var synced *string
	found, err := bsQueryOne(s, `SELECT last_attempt_complete, last_synced_at FROM sync_state WHERE resource_type = ?`, []any{resourceType}, &complete, &synced)
	if err != nil {
		return err
	}
	if found {
		if complete != 1 || synced == nil || *synced == "" {
			return fmt.Errorf("the last sync of %s did not finish, so the local store holds only part of it; run 'betterstack-cli sync --resources %s' again before trusting this report", resourceType, resourceType)
		}
		params, filtered, err := syncWalkFiltered(s, resourceType)
		if err != nil {
			return err
		}
		if filtered {
			return fmt.Errorf("the last sync of %s was not a whole, reconciled walk (%s), so the local store may hold only the records it matched, or records Better Stack no longer has; run 'betterstack-cli sync --resources %s' without --param, --resource-param, --global-param, --since, --no-prune, --latest-only or --full=false before trusting this report", resourceType, params, resourceType)
		}
		return nil
	}
	var n int
	if _, err := bsQueryOne(s, `SELECT COUNT(*) FROM resources WHERE resource_type = ?`, []any{resourceType}, &n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%s has %d record(s) in the local store from live reads but was never synced, so the store holds only part of it; run 'betterstack-cli sync --resources %s' before trusting this report", resourceType, n, resourceType)
	}
	var synced0 int
	if _, err := bsQueryOne(s, `SELECT COUNT(*) FROM sync_state`, nil, &synced0); err != nil {
		return err
	}
	if synced0 > 0 {
		return fmt.Errorf("%s was never synced, so this report would read it as empty; run 'betterstack-cli sync --resources %s' before trusting this report", resourceType, resourceType)
	}
	return nil // nothing synced yet, nothing stored: the empty result and its sync hint
}

// bsQueryOne scans the first row of a query; found is false when it has none.
func bsQueryOne(s *store.Store, query string, args []any, dest ...any) (bool, error) {
	rows, err := s.Query(query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	return true, rows.Scan(dest...)
}
