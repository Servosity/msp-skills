// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// autotask handfix (class incomplete-snapshot-read-as-complete, filtered walk;
// pre-closed from the atera 4.34.2 reprint's Codex r1): a sync that sent a
// filter (--param, --resource-param or --global-param: an id-walk `filter`
// condition list, `includeFields`, or any other body or query parameter)
// finishes as a complete walk of what the filter matched, and its sync_state
// says so: the press keeps ordinary filters out of the checkpoint scope by
// design. The novels read that state as the whole account, so
// `sync --resources tickets --param 'filter=[{"op":"eq","field":"status","value":5}]'`
// then `triage` would report every other ticket as absent.
//
// The sync command records, per resource, that its walk was filtered (the
// parameter names, never their values) before the walk starts, and clears the
// record only when an unfiltered walk of that resource then finishes: the
// checkpoint is set incomplete before that walk and read back after it.
// requireFinishedSync (novel_shared.go) refuses a resource with a record. A
// filtered sync that fails, or an unfiltered one that does not finish, leaves
// the record or an incomplete checkpoint: fail closed. MaxRecords alone (the
// page size; the id walk still reads every page) is not a filter.
//
// Codex r1, same class (resumed cursor): the checkpoint scope also leaves the
// resume cursor and watermark shared, so an unfiltered sync after a filtered
// walk stopped by --max-pages resumed after the filtered walk's last id and
// finished "complete" without every lower record. A filtered walk starts from
// no cursor; the first unfiltered walk after a recorded filtered walk starts
// from no cursor too, and only then marks its own cursor resumable, so neither
// walk resumes the other's position (Codex r2: the filtered record, not a mark
// a filtered walk writes, decides, so a store a build without the mark filled
// starts over too), while a chunked unfiltered walk (--max-pages) still resumes.

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"autotask-pp-cli/internal/store"
)

// syncFilteredTable is the store table of filtered walks (hand-owned; the
// generated schema never names it).
const syncFilteredTable = "autotask_sync_filtered"

// syncUnfilteredResumeTable names the resources whose sync_state cursor an
// unfiltered walk wrote since their last filtered walk (hand-owned, like the
// above): only those resume it.
const syncUnfilteredResumeTable = "autotask_sync_unfiltered_resume"

// syncFiltersUnrecorded is the filtered-walk record of a checkpoint written
// without one: by a build older than this layer (the published 4.24 build) or
// by `workflow archive`, which walks outside the sync command. Its walk may
// have been filtered, so it is treated as one (Codex r2 class
// incomplete-snapshot-read-as-complete, older store; fixed after the stop rule).
const syncFiltersUnrecorded = "unrecorded"

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
		scope, ok := syncScopeOf(c)
		if !ok {
			return inner(c, args) // a usage error: the sync reports it and walks nothing
		}
		dbPath, _ := c.Flags().GetString("db")
		if dbPath == "" {
			dbPath = defaultDBPath("autotask-cli")
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
// (parseSyncUserParams, applyTo): --param and --global-param reach every walk,
// --resource-param its resource's walk.
func syncScopeOf(c *cobra.Command) (syncScope, bool) {
	get := func(name string) []string {
		v, _ := c.Flags().GetStringArray(name)
		return v
	}
	params, err := parseSyncUserParams(get("param"), get("resource-param"), get("global-param"))
	if err != nil {
		return syncScope{}, false
	}
	named, _ := c.Flags().GetStringSlice("resources")
	inScope := map[string]bool{}
	if len(named) == 0 {
		for _, r := range defaultSyncResources() {
			inScope[r] = true
		}
	} else {
		for _, r := range named {
			inScope[strings.TrimSpace(r)] = true
		}
	}
	scope := syncScope{filters: map[string][]string{}}
	for r := range inScope {
		if r == "" {
			continue
		}
		scope.resources = append(scope.resources, r)
		sent := map[string]string{}
		params.applyTo(r, sent, false) // autotask sync walks no dependent resources
		for k := range sent {
			if strings.EqualFold(k, "MaxRecords") {
				continue // the page size, not a filter
			}
			scope.filters[r] = append(scope.filters[r], k)
		}
		sort.Strings(scope.filters[r])
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
	var recorded int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, syncFilteredTable).Scan(&recorded); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + syncFilteredTable + ` (resource_type TEXT PRIMARY KEY, params TEXT NOT NULL)`); err != nil {
		return nil, err
	}
	if recorded == 0 {
		if _, err := db.Exec(`INSERT OR IGNORE INTO `+syncFilteredTable+` (resource_type, params) SELECT resource_type, ? FROM sync_state`, syncFiltersUnrecorded); err != nil {
			return nil, err
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + syncUnfilteredResumeTable + ` (resource_type TEXT PRIMARY KEY)`); err != nil {
		return nil, err
	}
	restart := func(r string) error {
		_, err := db.Exec(`UPDATE sync_state SET last_cursor = '', last_synced_at = NULL, last_attempt_complete = 0 WHERE resource_type = ?`, r)
		return err
	}
	var pending []string
	for _, r := range scope.resources {
		if names := scope.filters[r]; len(names) > 0 {
			if err := restart(r); err != nil {
				return nil, err
			}
			if _, err := db.Exec(`DELETE FROM `+syncUnfilteredResumeTable+` WHERE resource_type = ?`, r); err != nil {
				return nil, err
			}
			if _, err := db.Exec(`INSERT OR REPLACE INTO `+syncFilteredTable+` (resource_type, params) VALUES (?, ?)`, r, strings.Join(names, ",")); err != nil {
				return nil, err
			}
			continue
		}
		var params string
		if err := db.QueryRow(`SELECT params FROM `+syncFilteredTable+` WHERE resource_type = ?`, r).Scan(&params); err != nil {
			continue // not filtered before: nothing to clear
		}
		var resumable int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+syncUnfilteredResumeTable+` WHERE resource_type = ?`, r).Scan(&resumable); err != nil {
			return nil, err
		}
		if resumable == 0 {
			if err := restart(r); err != nil {
				return nil, err
			}
			if _, err := db.Exec(`INSERT OR REPLACE INTO `+syncUnfilteredResumeTable+` (resource_type) VALUES (?)`, r); err != nil {
				return nil, err
			}
		} else if _, err := db.Exec(`UPDATE sync_state SET last_attempt_complete = 0 WHERE resource_type = ?`, r); err != nil {
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
			_, _ = db.Exec(`DELETE FROM `+syncUnfilteredResumeTable+` WHERE resource_type = ?`, r)
		}
	}
}

// syncWalkFiltered reports the filter parameter names the last walk of
// resourceType sent, when it was filtered and no unfiltered walk finished
// since.
func syncWalkFiltered(s *store.Store, resourceType string) (string, bool, error) {
	var n int
	if _, err := novelQueryOne(s, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, []any{syncFilteredTable}, &n); err != nil {
		return "", false, err
	}
	if n == 0 { // a checkpoint no sync command of this build wrote
		return syncFiltersUnrecorded, true, nil
	}
	var params string
	found, err := novelQueryOne(s, `SELECT params FROM `+syncFilteredTable+` WHERE resource_type = ?`, []any{resourceType}, &params)
	return params, found, err
}
