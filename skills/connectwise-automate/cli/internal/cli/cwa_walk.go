// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Walk membership for the fleet reports. A sync records, per resource the
// reports read, which record IDs its last finished, UNFILTERED walk returned
// (store table cwa_walk). The reports read only those records, so the fleet is
// what the API last returned in full, never inferred from row timestamps:
//   - a walk capped by --max-pages and resumed by later syncs is one walk: each
//     run adds its IDs, and the walk finishes when a run reaches the natural end
//     starting exactly where the previous run stopped;
//   - a filtered walk (--param, --resource-param, a since window) never touches
//     the record, so it is never taken for the whole fleet;
//   - a live read's write-through is not in the record, so it is not current;
//   - anything else (an interrupted run, a resume from an unexpected cursor)
//     breaks the walk, and the reports refuse until a walk from the first page
//     finishes.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"connectwise-automate-pp-cli/internal/store"
)

// cwaWalkResources are the resources the fleet reports read (loadResource).
var cwaWalkResources = map[string]bool{
	"alerts": true, "clients": true, "computers": true, "locations": true, "patching": true,
}

func cwaWalkTracked(resource string) bool { return cwaWalkResources[resource] }

const (
	cwaWalkWalking  = "walking"
	cwaWalkFinished = "finished"
	cwaWalkBroken   = "broken"
)

// cwaWalkUnfiltered reports whether this walk asks the API for the whole
// resource: no --param / --global-param / --resource-param for it, no since
// window and no required date window.
func cwaWalkUnfiltered(resource, effectiveSince string, userParams *syncUserParams) bool {
	if userParams != nil && (len(userParams.flatGlobal) > 0 || len(userParams.trueGlobal) > 0 || len(userParams.perResource[resource]) > 0) {
		return false
	}
	return strings.TrimSpace(effectiveSince) == "" && !syncResourceHasDateWindow(resource)
}

type cwaWalkRow struct {
	state, nextCursor string
	pending, members  []string
}

func cwaWalkRead(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, resource string) (cwaWalkRow, bool, error) {
	var r cwaWalkRow
	var pending, members string
	err := q.QueryRowContext(ctx, `SELECT state, next_cursor, pending, members FROM cwa_walk WHERE resource_type = ?`, resource).
		Scan(&r.state, &r.nextCursor, &pending, &members)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	if json.Unmarshal([]byte(pending), &r.pending) != nil || json.Unmarshal([]byte(members), &r.members) != nil {
		// An unreadable record proves nothing: treat the walk as broken.
		return cwaWalkRow{state: cwaWalkBroken}, true, nil
	}
	return r, true, nil
}

// cwaWalkUpdate applies f to the resource's walk record in one transaction.
// ponytail: ID sets are JSON text columns; a membership table if fleets reach
// hundreds of thousands of records.
func cwaWalkUpdate(db *store.Store, resource string, f func(r cwaWalkRow, ok bool) cwaWalkRow) error {
	ctx := context.Background()
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	r, ok, err := cwaWalkRead(ctx, tx, resource)
	if err != nil {
		return err
	}
	r = f(r, ok)
	if r.pending == nil {
		r.pending = []string{}
	}
	if r.members == nil {
		r.members = []string{}
	}
	pending, _ := json.Marshal(r.pending)
	members, _ := json.Marshal(r.members)
	if _, err := tx.ExecContext(ctx, `INSERT INTO cwa_walk (resource_type, state, next_cursor, pending, members) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(resource_type) DO UPDATE SET state = excluded.state, next_cursor = excluded.next_cursor, pending = excluded.pending, members = excluded.members`,
		resource, r.state, r.nextCursor, string(pending), string(members)); err != nil {
		return err
	}
	return tx.Commit()
}

// cwaWalkBegin runs when a sync run of resource starts sending requests from
// startCursor ("" = the first page).
func cwaWalkBegin(db *store.Store, resource string, unfiltered bool, startCursor string) error {
	if !cwaWalkTracked(resource) || !unfiltered {
		return nil
	}
	err := cwaWalkUpdate(db, resource, func(r cwaWalkRow, ok bool) cwaWalkRow {
		switch {
		case startCursor == "":
			return cwaWalkRow{state: cwaWalkWalking, members: r.members}
		case ok && r.state == cwaWalkWalking && r.nextCursor == startCursor:
			return r
		default:
			return cwaWalkRow{state: cwaWalkBroken, members: r.members}
		}
	})
	if err != nil {
		return fmt.Errorf("recording the %s walk: %w", resource, err)
	}
	return nil
}

// cwaWalkEnd runs when a sync run of resource has saved its state: ids are the
// record IDs this run stored, natural is a proven end of the list, nextCursor
// is where a later run resumes ("" = none).
func cwaWalkEnd(db *store.Store, resource string, unfiltered bool, ids []string, natural bool, nextCursor string) error {
	if !cwaWalkTracked(resource) || !unfiltered {
		return nil
	}
	err := cwaWalkUpdate(db, resource, func(r cwaWalkRow, ok bool) cwaWalkRow {
		if !ok || r.state != cwaWalkWalking {
			return r
		}
		r.pending = append(r.pending, ids...)
		switch {
		case natural:
			return cwaWalkRow{state: cwaWalkFinished, members: r.pending}
		case nextCursor != "":
			r.nextCursor = nextCursor
			return r
		default:
			return cwaWalkRow{state: cwaWalkBroken, members: r.members}
		}
	})
	if err != nil {
		return fmt.Errorf("recording the %s walk: %w", resource, err)
	}
	return nil
}
