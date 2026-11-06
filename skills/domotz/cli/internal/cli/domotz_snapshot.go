// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// domotz handfix (class incomplete-snapshot-read-as-complete, pre-closed from
// the shared layer's betterstack and atera reviews): every store-backed novel
// opens the store through novelOpenStoreChecked, which calls
// requireFinishedSync for each resource it reads, and a resource is only the
// whole account when its last sync finished unfiltered. A sync that stopped
// part way (last_attempt_complete = 0: a Collector's device list answered an
// error, --max-pages), rows written with no sync of that resource, a resource
// never synced while others were, and a filtered walk (domotz_sync_scope.go)
// are a partial set; reporting on them ("no devices offline", "every device
// monitored") is a wrong answer an MSP acts on. A store with nothing synced
// and nothing stored keeps the documented empty result with its sync hint.

import (
	"fmt"

	"domotz-pp-cli/internal/store"
)

// requireFinishedSync refuses a resource whose stored rows are not backed by
// a finished, unfiltered sync of it.
func requireFinishedSync(s *store.Store, resourceType string) error {
	var complete int
	var synced *string
	found, err := storeQueryOne(s, `SELECT last_attempt_complete, CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, []any{resourceType}, &complete, &synced)
	if err != nil {
		return err
	}
	if found {
		if complete != 1 || synced == nil || *synced == "" {
			return fmt.Errorf("the last sync of %s did not finish, so the local store holds only part of it; run 'domotz-cli sync --resources %s' again before trusting this report", resourceType, syncNameFor(resourceType))
		}
		params, filtered, err := syncWalkFiltered(s, resourceType)
		if err != nil {
			return err
		}
		if filtered {
			return fmt.Errorf("the last sync of %s was not a whole, reconciled walk (%s), so the local store may hold only the records it matched, or records Domotz no longer has; run 'domotz-cli sync --resources %s' without --param, --resource-param, --global-param, --since, --no-prune, --latest-only or --full=false before trusting this report", resourceType, params, syncNameFor(resourceType))
		}
		return nil
	}
	var n int
	if _, err := storeQueryOne(s, `SELECT COUNT(*) FROM resources WHERE resource_type = ?`, []any{resourceType}, &n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%s has %d record(s) in the local store but was never synced, so the store holds only part of it; run 'domotz-cli sync --resources %s' before trusting this report", resourceType, n, syncNameFor(resourceType))
	}
	var synced0 int
	if _, err := storeQueryOne(s, `SELECT COUNT(*) FROM sync_state`, nil, &synced0); err != nil {
		return err
	}
	if synced0 > 0 {
		return fmt.Errorf("%s was never synced, so this report would read it as empty; run 'domotz-cli sync --resources %s' before trusting this report", resourceType, syncNameFor(resourceType))
	}
	return nil // nothing synced yet, nothing stored: the empty result and its sync hint
}

// syncNameFor is the --resources name that re-syncs resourceType: a
// dependent is walked by naming its parent (naming agent walks agent_device).
func syncNameFor(resourceType string) string {
	for _, dep := range dependentResourceDefs() {
		if dep.Name == resourceType && !optInDependents[dep.Name] {
			return dep.ParentTable
		}
	}
	return resourceType
}

// storeQueryOne scans the first row of a query; found is false when it has
// none.
func storeQueryOne(s *store.Store, query string, args []any, dest ...any) (bool, error) {
	rows, err := s.DB().Query(query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	return true, rows.Scan(dest...)
}
