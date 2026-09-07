// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// Parent-keyed sync for resources whose collection endpoint is scoped by a
// QUERY parameter rather than a path placeholder.
//
// `application-files` requires applicationId and `maintenance` requires
// computerId. The press detects parent-child relationships from paths that carry
// a {placeholder}; these take their parent key as an ordinary query param, so
// detectDependentResources never sees them and the profiler registers both as
// FLAT syncable resources. A flat walk sends only pagination, so the API rejects
// every call and the tables stay empty -- two of the four resources #223 tracks.
//
// This fans out instead: read the parent ids already in the local store, then
// walk each child collection with its parent key supplied. Rows carry their own
// parent reference (ApplicationFile.applicationId, MaintenanceEntry.computerId),
// so store.resourceStorageID composes the key without any injection here.
//
// Upstream: finding 4 of mvanhorn/cli-printing-press#4482, originally item 8 of
// #4165. Do NOT cite #4165 - it was closed as completed with this finding
// explicitly out of scope, which is how the finding lost its tracker.
//
// FIXED UPSTREAM, and #4482 is closed too: cli-printing-press#4489 merged
// 2026-09-01 and first shipped in press v4.31.5. detectDependentResources no
// longer requires a {placeholder} in the path - a child collection keyed by a
// required query param whose name matches a parent key is tracked the same way.
//
// This connector was generated on 4.30.2, so the fan-out below is still doing
// the work. RE-CHECK at the next reprint on v4.31.5 or later, and check rather
// than assume: upstream binds only when there is EXACTLY ONE such parent key
// and no leftover required scope, so whether ApplicationFileGetByApplicationId
// and MaintenanceModeGetByComputerIdV2 are picked up has to be read off the
// generated profile. Declaring x-pp-sync-walker with an explicit key_param
// remains the other route (upstream #3816 fixed sending the query key).

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"threatlocker-pp-cli/internal/store"
)

// dependentExtractRows unwraps ThreatLocker's {"action":..,"data":[...]} envelope,
// tolerating a bare array and the {"results":[...]} shape. Returns ok=false for a
// body that is neither, so an unreadable response is reported rather than being
// mistaken for an empty collection.
func dependentExtractRows(raw json.RawMessage) ([]json.RawMessage, bool) {
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

// dependentGetClient is the subset of the API client this path needs.
type dependentGetClient interface {
	Get(context.Context, string, map[string]string) (json.RawMessage, error)
}

// dependentSyncSpec describes how to walk one parent-keyed child collection.
type dependentSyncSpec struct {
	path string
	// parent is the resource whose stored ids drive the fan-out.
	parent string
	// parentParam is the query parameter carrying the parent id.
	parentParam string
	// idField is the child row's own key.
	idField string
	// identityFields, when set, synthesizes a deterministic key into idField
	// from these fields. Used for collections the API exposes no unique id for,
	// where any single field would silently collapse distinct rows on upsert.
	identityFields []string
}

// syntheticIDField is the key written by synthesizeIdentity. Prefixed so it
// cannot collide with a real API field.
const syntheticIDField = "ppSyntheticId"

// synthesizeIdentity gives a row a deterministic id derived from the fields that
// actually distinguish it. Deterministic so a re-sync upserts rather than
// duplicates; hashed so the key stays a fixed size regardless of field content.
func synthesizeIdentity(row json.RawMessage, fields []string) (json.RawMessage, bool) {
	var obj map[string]any
	if json.Unmarshal(row, &obj) != nil {
		return row, false
	}
	h := sha256.New()
	for _, f := range fields {
		fmt.Fprintf(h, "%s=%v\x00", f, obj[f])
	}
	obj[syntheticIDField] = hex.EncodeToString(h.Sum(nil)[:16])
	out, err := json.Marshal(obj)
	if err != nil {
		return row, false
	}
	return out, true
}

const dependentSyncPageSize = 100

// init registers each dependent's keying with the store, so the spec map below
// stays the single source of truth: idField becomes the row's extracted key and
// parentParam becomes the parent column resourceStorageID appends to make it
// unique within its parent.
func init() {
	for resource, spec := range dependentSyncSpecs {
		store.RegisterDependentKey(resource, spec.idField, spec.parentParam)
		resourceIDFieldOverrides[resource] = spec.idField
	}
}

var dependentSyncSpecs = map[string]dependentSyncSpec{
	"application-files": {
		path:        "/ApplicationFile/ApplicationFileGetByApplicationId",
		parent:      "applications",
		parentParam: "applicationId",
		idField:     "applicationFileId",
	},
	"maintenance": {
		path:        "/MaintenanceMode/MaintenanceModeGetByComputerIdV2",
		parent:      "computers",
		parentParam: "computerId",
		// The API exposes no unique id for a maintenance window, and keying on
		// startDateTime alone collapses two windows that share a start but
		// differ in type, creator or end. identityFields synthesizes a
		// deterministic key from all of them instead.
		idField:        syntheticIDField,
		identityFields: []string{"computerId", "startDateTime", "endDateTime", "maintenanceType", "createdBy"},
	},
}

// dependentParentIDs returns the bare parent ids to fan out over. A parent that
// was never synced yields none, which is reported rather than silently treated
// as "nothing to do".
func dependentParentIDs(db *store.Store, parent string) ([]string, error) {
	ids, err := db.ListIDs(parent)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		bare := store.BareResourceID(id)
		if bare == "" || seen[bare] {
			continue
		}
		seen[bare] = true
		out = append(out, bare)
	}
	return out, nil
}

// dependentSyncPages walks every page of one parent's child collection.
func dependentSyncPages(ctx context.Context, c dependentGetClient, spec dependentSyncSpec, parentID string, maxPages int) ([]json.RawMessage, bool, error) {
	var out []json.RawMessage
	for page := 1; maxPages <= 0 || page <= maxPages; page++ {
		params := map[string]string{
			spec.parentParam: parentID,
			"pageNumber":     strconv.Itoa(page),
			"pageSize":       strconv.Itoa(dependentSyncPageSize),
		}
		raw, err := c.Get(ctx, spec.path, params)
		if err != nil {
			return out, false, err
		}
		// The same guard the flat walk uses: {"success":false,"data":null}
		// unmarshals cleanly into an empty slice, so without this a declared
		// API failure would read as a legitimately empty child collection and
		// the resource would report a clean sync (#208's failure mode).
		if responseReportsFailure(raw) {
			return out, false, fmt.Errorf("API reported failure for %s of %s", spec.path, parentID)
		}
		rows, ok := dependentExtractRows(raw)
		if !ok {
			// A body we cannot read as rows is not an empty collection.
			return out, false, fmt.Errorf("unreadable response for %s of %s", spec.path, parentID)
		}
		if len(rows) == 0 {
			return out, true, nil
		}
		out = append(out, rows...)
		if len(rows) < dependentSyncPageSize {
			return out, true, nil
		}
	}
	// Hit the page cap: the enumeration is truncated, not complete.
	return out, false, nil
}

// syncDependentResource is the hand-wired replacement for the flat walk on
// parent-keyed resources. It reports the same honest outcomes as the generated
// path: storing nothing after consuming rows, or truncating, is a warning rather
// than a success (#208).
func syncDependentResource(
	ctx context.Context,
	c dependentGetClient,
	db *store.Store,
	resource string,
	spec dependentSyncSpec,
	maxPages int,
	syncEvents io.Writer,
	started time.Time,
) syncResult {
	if syncEvents == nil {
		syncEvents = io.Discard
	}

	parents, err := dependentParentIDs(db, spec.parent)
	if err != nil {
		return syncResult{Resource: resource, Err: fmt.Errorf("listing %s parents for %s: %w", spec.parent, resource, err), Duration: time.Since(started)}
	}
	if len(parents) == 0 {
		if !humanFriendly {
			fmt.Fprintf(syncEvents, `{"event":"sync_warning","resource":"%s","reason":"parent_table_empty","message":"%s is keyed by %s; sync %s first"}`+"\n",
				resource, resource, spec.parentParam, spec.parent)
		}
		return syncResult{
			Resource: resource,
			Count:    0,
			Warn:     fmt.Errorf("%s is keyed by %s and the %s table is empty; sync %s first", resource, spec.parentParam, spec.parent, spec.parent),
			Duration: time.Since(started),
		}
	}

	var consumed, stored, typedFailures int
	complete := true
	for _, parentID := range parents {
		rows, whole, err := dependentSyncPages(ctx, c, spec, parentID, maxPages)
		if err != nil {
			// One unreachable parent must not abort the whole fan-out, but it
			// does mean the result is incomplete.
			complete = false
			if !humanFriendly {
				fmt.Fprintln(syncEvents, syncErrorJSON(resource, parentID, err))
			}
			continue
		}
		if !whole {
			complete = false
		}
		consumed += len(rows)
		if len(rows) == 0 {
			continue
		}
		if len(spec.identityFields) > 0 {
			for i, row := range rows {
				if synthesized, ok := synthesizeIdentity(row, spec.identityFields); ok {
					rows[i] = synthesized
				}
			}
		}
		s, _, tf, err := db.UpsertBatchDetailed(resource, rows)
		if err != nil {
			return syncResult{Resource: resource, Count: stored, Err: fmt.Errorf("storing %s for %s: %w", resource, parentID, err), Duration: time.Since(started)}
		}
		stored += s
		typedFailures += tf
	}

	if !humanFriendly {
		fmt.Fprintf(syncEvents, `{"event":"sync_complete","resource":"%s","stored":%d,"parents":%d,"duration_ms":%d}`+"\n",
			resource, stored, len(parents), time.Since(started).Milliseconds())
	}
	// Only a complete fan-out earns a completion watermark; recording one after
	// a partial walk would let the next run skip parents it never reached. The
	// error is propagated rather than dropped so the scheduler's critical
	// sync-state handling applies.
	if complete {
		if err := db.SaveSyncState(resource, "", stored); err != nil {
			return syncResult{Resource: resource, Count: stored, Err: fmt.Errorf("saving sync state for %s: %w", resource, err), Duration: time.Since(started)}
		}
	}

	switch {
	case consumed > 0 && stored == 0:
		return syncResult{Resource: resource, Count: 0,
			Warn:     fmt.Errorf("%s consumed %d items across %d %s but stored 0 rows", resource, consumed, len(parents), spec.parent),
			Duration: time.Since(started)}
	case typedFailures > 0:
		return syncResult{Resource: resource, Count: stored,
			Warn:     fmt.Errorf("%s stored %d rows but %d failed their typed-table projection", resource, stored, typedFailures),
			Duration: time.Since(started)}
	case !complete:
		return syncResult{Resource: resource, Count: stored,
			Warn:     fmt.Errorf("%s enumerated only part of its parents; stored %d rows and the result may be incomplete", resource, stored),
			Duration: time.Since(started)}
	}
	return syncResult{Resource: resource, Count: stored, Duration: time.Since(started)}
}
