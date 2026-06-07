// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored helpers shared by the Afi novel commands (coverage-gaps,
// fleet-health, backup-stale, resolve, reconcile-licenses, tenant-scorecard,
// offboard, fleet-sync). All local reads go through the generic `resources`
// table written by `fleet-sync` (resource_type keyed rows with parent IDs
// injected into the JSON), never the typed per-entity tables.
package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"afi-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// errNoLiveEquivalent rejects --data-source live on local-only novel commands.
func errNoLiveEquivalent(flags *rootFlags, name string) error {
	if flags.dataSource == "live" {
		return usageErr(fmt.Errorf("%s has no live equivalent: it answers a fleet-wide join the rate-limited Afi API cannot serve; run 'afi-cli fleet-sync' then re-run with --data-source local or auto", name))
	}
	return nil
}

// errNoLocalSource rejects --data-source local on live-only novel commands.
func errNoLocalSource(flags *rootFlags, name string) error {
	if flags.dataSource == "local" {
		return usageErr(fmt.Errorf("%s has no local data source: it must call the live Afi API", name))
	}
	return nil
}

// openAfiStore resolves the database path and opens the local store.
func openAfiStore(dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("afi-cli")
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w", err)
	}
	return db, nil
}

// fleetHint emits the generated sync hints plus a fleet-sync pointer when the
// fleet tables have never been populated (the generic hint names the framework
// `sync` command, which only covers installations).
func fleetHint(cmd *cobra.Command, db *store.Store, resourceType string, flags *rootFlags) {
	if hintIfUnsynced(cmd, db, resourceType) {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: fleet entities (orgs, tenants, resources, protections, archives, quotas) are populated by 'afi-cli fleet-sync', not the flat 'sync' command.")
		return
	}
	hintIfStale(cmd, db, resourceType, flags.maxAge)
}

// jsonInt64 parses Afi's string-encoded int64 fields ("25") as well as plain
// JSON numbers. Returns 0,false on null/missing/garbage.
func jsonInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case nil:
		return 0, false
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

// csvSet splits a comma-separated flag value into a membership set; an empty
// input returns an empty (match-everything) set.
func csvSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}

// taskStatusStats mirrors v1TaskStatusStats.
type taskStatusStats struct {
	Done     int64 `json:"done"`
	Failed   int64 `json:"failed"`
	Warnings int64 `json:"warnings"`
}

// parseTaskStats extracts the total counters from a stored task_stats
// snapshot (lenient: tolerates string-encoded ints and missing keys).
func parseTaskStats(data []byte) (total taskStatusStats, byAction map[string]taskStatusStats, ok bool) {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return total, nil, false
	}
	read := func(m any) taskStatusStats {
		var s taskStatusStats
		mm, _ := m.(map[string]any)
		if mm == nil {
			return s
		}
		s.Done, _ = jsonInt64(mm["done"])
		s.Failed, _ = jsonInt64(mm["failed"])
		s.Warnings, _ = jsonInt64(mm["warnings"])
		return s
	}
	total = read(obj["total"])
	if ba, okBA := obj["by_action"].(map[string]any); okBA {
		byAction = map[string]taskStatusStats{}
		for k, v := range ba {
			byAction[k] = read(v)
		}
	}
	return total, byAction, true
}
