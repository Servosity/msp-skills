// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Walk membership for the seven local-mirror reports (fleet-health,
// stale-backups, at-risk, alarms-triage, license-usage, company-overview,
// since). sync_state.last_attempt_complete=1 proves only that the requested
// walk ended, not that it covered the collection. So sync records, for every
// partition the reports read, which record IDs its last finished,
// UNRESTRICTED walk returned (store table veeam_walk), and veeamLoad counts
// only those records:
//   - a walk narrowed by --param, --resource-param, --global-param, a since
//     window or a required date window never touches the record, so it is
//     never taken for the whole collection;
//   - a walk capped by --max-pages and resumed by later syncs is one walk: each
//     run adds the IDs it stored, and the walk finishes when a run reaches the
//     natural end starting exactly where the previous run stopped;
//   - rows a live read wrote through, an import, a dependent sync, or an
//     earlier walk stored that the latest finished walk did not return are not
//     members (a deleted job or a cleared alarm drops out);
//   - a dry-run sync sends no request and never touches the record;
//   - anything else (an interrupted run, a resume from an unexpected cursor, a
//     run that stored a row it could not key) breaks the walk, and the reports
//     refuse until a walk from the first page finishes.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"veeam-pp-cli/internal/store"
)

// veeamWalkPaths are the documented list paths the reports read through
// veeamLoad. A path missing here is refused by veeamLoad, never read unproven.
var veeamWalkPaths = []string{
	veeamPathCompanies, veeamPathProviderCompanies, veeamPathServerJobs, veeamPathBackupServers,
	veeamPathAgentJobs, veeamPathAgents, veeamPathActiveAlarms, veeamPathAlarmTemplates,
	veeamPathWorkloadsServer, veeamPathWorkloadsConsole, veeamPathLicenseUsage,
}

// veeamWalkPartitions resolves veeamWalkPaths to their sync partitions once.
var veeamWalkPartitions = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	for _, p := range veeamWalkPaths {
		for _, name := range veeamSyncNames(p) {
			out[name] = true
		}
	}
	return out
})

// veeamWalkTracked reports whether sync records walk membership for resource.
func veeamWalkTracked(resource string) bool { return veeamWalkPartitions()[resource] }

// veeamWalkUnfiltered reports whether this walk asks the API for the whole
// collection: no --param, --global-param or --resource-param for it, no since
// window and no required date window.
func veeamWalkUnfiltered(resource, effectiveSince string, userParams *syncUserParams) bool {
	if userParams != nil && (len(userParams.flatGlobal) > 0 || len(userParams.trueGlobal) > 0 || len(userParams.perResource[resource]) > 0) {
		return false
	}
	return strings.TrimSpace(effectiveSince) == "" && !syncResourceHasDateWindow(resource)
}

// veeamWalkBegin runs when a non-dry-run sync of resource starts sending
// requests from startCursor ("" = the first page).
func veeamWalkBegin(db *store.Store, resource string, unfiltered bool, startCursor string) error {
	if !veeamWalkTracked(resource) || !unfiltered {
		return nil
	}
	err := db.UpdateVeeamWalk(context.Background(), resource, func(w store.VeeamWalk, ok bool) store.VeeamWalk {
		switch {
		case startCursor == "":
			return store.VeeamWalk{State: store.VeeamWalkWalking, Members: w.Members}
		case ok && w.State == store.VeeamWalkWalking && w.NextCursor == startCursor:
			return w
		default:
			return store.VeeamWalk{State: store.VeeamWalkBroken, Members: w.Members}
		}
	})
	if err != nil {
		return fmt.Errorf("recording the %s walk: %w", resource, err)
	}
	return nil
}

// veeamWalkEnd runs when a non-dry-run sync of resource has saved its state.
// ids are the record IDs this run stored (extractID, the key UpsertBatch
// stores them under), stored is how many rows it stored, natural is a proven
// end of the list, and nextCursor is where a later run resumes ("" = none).
func veeamWalkEnd(db *store.Store, resource string, unfiltered bool, ids []string, stored int, natural bool, nextCursor string) error {
	if !veeamWalkTracked(resource) || !unfiltered {
		return nil
	}
	err := db.UpdateVeeamWalk(context.Background(), resource, func(w store.VeeamWalk, ok bool) store.VeeamWalk {
		switch {
		case !ok || w.State != store.VeeamWalkWalking:
			return store.VeeamWalk{State: store.VeeamWalkBroken, Members: w.Members}
		case len(ids) != stored:
			// A row this run stored has no key the walk could record, so the
			// walk cannot say what it covered.
			return store.VeeamWalk{State: store.VeeamWalkBroken, Members: w.Members}
		}
		w.Pending = append(w.Pending, ids...)
		switch {
		case natural:
			slices.Sort(w.Pending)
			return store.VeeamWalk{State: store.VeeamWalkFinished, Members: slices.Compact(w.Pending)}
		case nextCursor != "":
			w.NextCursor = nextCursor
			return w
		default:
			return store.VeeamWalk{State: store.VeeamWalkBroken, Members: w.Members}
		}
	})
	if err != nil {
		return fmt.Errorf("recording the %s walk: %w", resource, err)
	}
	return nil
}

// veeamWalkMembers returns the record IDs the last finished, unrestricted sync
// walk of partition name (documented list path path) returned. veeamLoad calls
// it after veeamSyncComplete. A partition no finished unrestricted walk covers
// (its rows came from a narrowed sync, a live read or an import, or its walk
// is unfinished or broken) is an error naming the sync that fixes it.
func veeamWalkMembers(ctx context.Context, db *store.Store, path, name string) (map[string]bool, error) {
	if !veeamWalkTracked(name) {
		return nil, fmt.Errorf("%s (sync resource %s) is not a partition sync records walk coverage for in this build", path, name)
	}
	walk, walked, err := db.ReadVeeamWalk(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("reading the %s sync walk: %w", name, err)
	}
	if walked && walk.State == store.VeeamWalkFinished {
		members := make(map[string]bool, len(walk.Members))
		for _, id := range walk.Members {
			members[id] = true
		}
		return members, nil
	}
	why := "no unrestricted sync of it has finished (a sync narrowed by --param, --resource-param, --global-param or --since, a live read and an import never prove the whole list)"
	switch {
	case walked && walk.State == store.VeeamWalkWalking:
		why = "its last unrestricted sync stopped before the end of the list"
	case walked:
		why = "its last unrestricted sync walk was interrupted or resumed from an unexpected page"
	}
	return nil, fmt.Errorf("the local mirror of %s (sync resource %s) is not proven whole, so this report would be partial: %s; run `%s` with no --param, --resource-param, --global-param or --since to fix it", path, name, why, veeamWalkFix(ctx, db, name))
}

// veeamWalkFix names the sync that proves partition name whole. A plain sync
// does it in one run when it starts from the first page or continues the walk
// exactly where it stopped; otherwise --full drops the resume cursor so the
// walk restarts from the first page. Unsure (a read error) is --full.
func veeamWalkFix(ctx context.Context, db *store.Store, name string) string {
	plain, full := "veeam-cli sync --resources "+name, "veeam-cli sync --full --resources "+name
	var cursor sql.NullString
	err := db.DB().QueryRowContext(ctx, `SELECT last_cursor FROM sync_state WHERE resource_type = ?`, name).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && cursor.String == "") {
		return plain
	}
	if err != nil {
		return full
	}
	walk, walked, err := db.ReadVeeamWalk(ctx, name)
	if err == nil && walked && walk.State == store.VeeamWalkWalking && walk.NextCursor == cursor.String {
		return plain
	}
	return full
}
