// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// domotz handfix (classes deleted-rows-read-as-current and
// incomplete-snapshot-read-as-complete, filtered walk; ported from the
// betterstack 4.34.2 reprint): the store-backed novels read the store as the
// whole account, so a sync records what it walked.
//
//   - deleted-rows-read-as-current: the generated sync prunes only on --full
//     (prune := full && !noPrune), so after a plain sync a Collector removed
//     from the account (DELETE /agent/{agent_id}) and a device a Collector no
//     longer reports stayed in the store, and fleet offline and fleet health
//     kept reporting them. Every plain sync is now a full walk: the agent list
//     reconciles flat_global and each Collector's device list per_parent.
//   - a filtered walk read as complete: a walk is the account's whole set only
//     when it is a full walk that reconciles deletions, with no --since, no
//     --latest-only and no --param, --resource-param or --global-param.
//     Anything else is recorded (the parameter names, never their values)
//     before the walk starts, and cleared only when an unfiltered walk of that
//     resource then finishes: its checkpoint is set incomplete before that
//     walk and read back after it. requireFinishedSync (domotz_snapshot.go)
//     refuses a resource with a record. A filtered sync that fails, or an
//     unfiltered one that does not finish, leaves the record or an incomplete
//     checkpoint: fail closed.

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"domotz-pp-cli/internal/store"
)

// syncFilteredTable is the store table of filtered walks (hand-owned; the
// generated schema never names it).
const syncFilteredTable = "domotz_sync_filtered"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "sync" && c.RunE != nil {
				wrapSyncScope(c, flags)
			}
		}
	})
}

// syncScope is what one sync run walks: every resource it may walk, and the
// filter parameter names each of them is sent.
type syncScope struct {
	resources []string
	filters   map[string][]string
}

func wrapSyncScope(cmd *cobra.Command, flags *rootFlags) {
	inner := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return inner(c, args)
		}
		// Every plain sync is a full walk: it reconciles deletions.
		// --full=false keeps the resumable walk, which is then recorded as
		// not a snapshot.
		if f := c.Flags().Lookup("full"); f != nil && !f.Changed {
			_ = f.Value.Set("true")
		}
		scope, ok := syncScopeOf(c)
		if !ok {
			return inner(c, args) // a usage error: the sync reports it and walks nothing
		}
		dbPath, _ := c.Flags().GetString("db")
		if dbPath == "" {
			dbPath = defaultDBPath("domotz-cli")
		}
		pending, err := markFilteredWalks(dbPath, scope)
		if err != nil {
			return err
		}
		runErr := inner(c, args)
		clearUnfilteredWalks(dbPath, pending)
		return runErr
	}
}

// syncScopeOf reads the sync flags the way the generated sync does
// (parseSyncUserParams, applyTo; syncDependentResources): --param reaches
// every flat walk, --global-param every walk, --resource-param its
// resource's walk; a dependent runs when nothing is named, or its parent or
// itself is named (an opt-in dependent only when named itself).
func syncScopeOf(c *cobra.Command) (syncScope, bool) {
	get := func(name string) []string {
		v, _ := c.Flags().GetStringArray(name)
		return v
	}
	params, err := parseSyncUserParams(get("param"), get("resource-param"), get("global-param"))
	if err != nil {
		return syncScope{}, false
	}
	since, _ := c.Flags().GetString("since")
	full, _ := c.Flags().GetBool("full")
	noPrune, _ := c.Flags().GetBool("no-prune")
	latestOnly, _ := c.Flags().GetBool("latest-only")
	named, _ := c.Flags().GetStringSlice("resources")
	inScope := map[string]bool{}
	allow := map[string]bool{}
	for _, r := range named {
		allow[strings.TrimSpace(r)] = true
	}
	if len(allow) == 0 {
		for _, r := range defaultSyncResources() {
			inScope[r] = true
		}
	} else {
		for r := range allow {
			inScope[r] = true
		}
	}
	deps := map[string]dependentResourceDef{}
	for _, dep := range dependentResourceDefs() {
		deps[dep.Name] = dep
		if optInDependents[dep.Name] && !allow[dep.Name] {
			continue
		}
		if len(allow) == 0 || allow[dep.ParentTable] || allow[dep.Name] {
			inScope[dep.Name] = true
		}
	}
	scope := syncScope{filters: map[string][]string{}}
	for r := range inScope {
		if r == "" {
			continue
		}
		scope.resources = append(scope.resources, r)
		dep, isDependent := deps[r]
		sent := map[string]string{}
		params.applyTo(r, sent, isDependent)
		for k := range sent {
			scope.filters[r] = append(scope.filters[r], k)
		}
		sort.Strings(scope.filters[r])
		if p := syncResourceSinceParam(r); p != "" && since != "" {
			scope.filters[r] = append(scope.filters[r], "--since ("+p+")")
		}
		if !full {
			scope.filters[r] = append(scope.filters[r], "--full=false (a resumed walk that keeps deleted records)")
		}
		if noPrune {
			scope.filters[r] = append(scope.filters[r], "--no-prune (deleted records kept)")
		}
		if latestOnly {
			scope.filters[r] = append(scope.filters[r], "--latest-only")
		}
		reconciles := resourceIsFlatReconcilable(resourceReconcileMode(r))
		if isDependent {
			reconciles = dep.ReconcileMode == "per_parent"
		}
		if !reconciles {
			scope.filters[r] = append(scope.filters[r], "no deletion reconcile for "+r)
		}
	}
	sort.Strings(scope.resources)
	return scope, true
}

// markFilteredWalks records every filtered walk of scope, and returns every
// unfiltered walk of a resource whose last walk was filtered: its checkpoint
// is set incomplete now (a walk that does not finish leaves it so, and the
// novels refuse it), and clearUnfilteredWalks clears its record only when this
// run completes it.
func markFilteredWalks(dbPath string, scope syncScope) ([]string, error) {
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	db := s.DB()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + syncFilteredTable + ` (resource_type TEXT PRIMARY KEY, params TEXT NOT NULL)`); err != nil {
		return nil, err
	}
	var pending []string
	for _, r := range scope.resources {
		if names := scope.filters[r]; len(names) > 0 {
			if _, err := db.Exec(`INSERT OR REPLACE INTO `+syncFilteredTable+` (resource_type, params) VALUES (?, ?)`, r, strings.Join(names, ",")); err != nil {
				return nil, err
			}
			continue
		}
		var params string
		if err := db.QueryRow(`SELECT params FROM `+syncFilteredTable+` WHERE resource_type = ?`, r).Scan(&params); err != nil {
			continue // not filtered before: nothing to clear
		}
		if _, err := db.Exec(`UPDATE sync_state SET last_attempt_complete = 0 WHERE resource_type = ?`, r); err != nil {
			return nil, err
		}
		pending = append(pending, r)
	}
	return pending, nil
}

// clearUnfilteredWalks clears the record of every pending unfiltered walk this
// run completed.
func clearUnfilteredWalks(dbPath string, pending []string) {
	if len(pending) == 0 {
		return
	}
	s, err := store.Open(dbPath)
	if err != nil {
		return
	}
	defer s.Close()
	db := s.DB()
	for _, r := range pending {
		var complete int
		var synced sql.NullString
		if err := db.QueryRow(`SELECT last_attempt_complete, CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, r).Scan(&complete, &synced); err != nil {
			continue
		}
		if complete == 1 && synced.Valid && synced.String != "" {
			_, _ = db.Exec(`DELETE FROM `+syncFilteredTable+` WHERE resource_type = ?`, r)
		}
	}
}

// syncWalkFiltered reports the filter parameter names the last walk of
// resourceType sent, when it was filtered and no unfiltered walk finished
// since.
func syncWalkFiltered(s *store.Store, resourceType string) (string, bool, error) {
	var n int
	if _, err := storeQueryOne(s, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, []any{syncFilteredTable}, &n); err != nil || n == 0 {
		return "", false, err
	}
	var params string
	found, err := storeQueryOne(s, `SELECT params FROM `+syncFilteredTable+` WHERE resource_type = ?`, []any{resourceType}, &params)
	return params, found, err
}

// withoutAccountCell drops the {region} line from a checkpoint scope key for
// the prune decision only (domotz handfix region-is-the-account; the
// one-line call in syncPruneSkipReason is re-applied by tools/apply_edits.py).
// The press treats any URL template variable as a tenant partition and skips
// every flat_global and per_parent prune under one; Domotz's {region} is the
// account's cell, one per API key, so a deleted Collector or device was never
// removed from the store.
func withoutAccountCell(scopeKey string) string {
	var kept []string
	for _, line := range strings.Split(scopeKey, "\n") {
		if !strings.HasPrefix(line, "template:region=") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
