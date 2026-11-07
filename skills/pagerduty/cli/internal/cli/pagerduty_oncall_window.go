// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pagerduty handfix r1-stale-oncall-rows-read-as-current (Codex r1, pagerduty
// 4.34.2 reprint): GET /oncalls answers for a time window (since and until,
// each defaulting to now: every on-call period overlapping the range; List
// all of the on-calls), and sync only upserted what it returned, so a
// responder PagerDuty no longer returns (a permanent on-call replaced, a
// shift reassigned) stayed "on call now" after a fresh, complete sync. The
// generated --full prune had the opposite fault: it deleted every stored
// entry the walk did not return, history and future shifts outside the
// window included.
//
// After a complete walk of /oncalls (full or incremental; --full --no-prune
// deletes nothing), the stored entries that overlap the walked window for
// every instant of the walk and were not returned are deleted; every other
// entry is kept. A walk narrowed to some users, policies or schedules, or to
// the earliest entry each (any parameter but the window, time zone, paging
// and include[]), deletes nothing, and neither does an incomplete walk or a
// window bound it cannot read. Called from syncResource (sync.go, anchored by
// tools/apply_credential_rules.py) in place of the generated prune.
//
// pagerduty handfix partial-pagination-not-deletion-evidence (Codex r2):
// reaching the last page proves the window was enumerated only when this run
// read from the first page on. A walk resumed from a saved position, or one
// the operator paged (--param offset or limit), deletes nothing.

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"pagerduty-pp-cli/internal/store"
)

// oncallWindowParams are the /oncalls query parameters that do not narrow
// which entries of the window are returned. offset and limit are not among
// them: an operator's offset skips entries, and an operator's limit makes the
// walk's own page step skip them.
var oncallWindowParams = map[string]bool{
	"since": true, "until": true, "time_zone": true, "total": true,
	"include[]": true, "include": true,
}

func reconcileOncallWindow(db *store.Store, complete bool, reason, startCursor, effectiveSince, dates string, userParams *syncUserParams,
	requestedAt time.Time, seenIDs []string, events io.Writer) error {
	const resource = "oncalls"
	skip := func(why string) error {
		fmt.Fprintf(events, `{"event":"reconcile_skipped","resource":%q,"scope":"window","reason":%q}`+"\n", resource, why)
		return nil
	}
	if !complete {
		if reason == "" {
			reason = "incomplete"
		}
		return skip(reason)
	}
	if n, err := strconv.Atoi(startCursor); startCursor != "" && (err != nil || n != 0) {
		return skip("resumed-walk")
	}
	if dates != "" {
		return skip("date-range")
	}
	params := map[string]string{}
	for k, v := range syncResourceParamDefaults(resource) {
		params[k] = v
	}
	if effectiveSince != "" {
		params["since"] = effectiveSince
	}
	userParams.applyTo(resource, params, false)
	for k := range params {
		if !oncallWindowParams[k] {
			return skip("narrowed-walk")
		}
	}
	// The window at an instant t of the walk is [since or t, until or t]; an
	// entry was in it for every instant when it starts before the earliest
	// window end and ends after the latest window start.
	untilLo, sinceHi := requestedAt, time.Now().UTC()
	for name, bound := range map[string]*time.Time{"until": &untilLo, "since": &sinceHi} {
		if v := params[name]; v != "" {
			t, ok := pdParseTime(v)
			if !ok {
				return skip("window-unread")
			}
			*bound = t
		}
	}
	rows, err := db.DB().Query(`SELECT id, data FROM resources WHERE resource_type = ?`, resource)
	if err != nil {
		return fmt.Errorf("reconciling %s: %w", resource, err)
	}
	keep := append([]string(nil), seenIDs...)
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			_ = rows.Close()
			return fmt.Errorf("reconciling %s: %w", resource, err)
		}
		if !oncallInWindow(data, untilLo, sinceHi) {
			keep = append(keep, id)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reconciling %s: %w", resource, err)
	}
	deleted, err := db.ReconcileAll(resource, keep, reconcileTypedTable(resource), store.CascadeJunctionsFor(resource))
	if err != nil {
		fmt.Fprintf(events, `{"event":"reconcile_error","resource":%q,"scope":"window","error":%q}`+"\n", resource, err.Error())
		return fmt.Errorf("reconciling %s: %w", resource, err)
	}
	fmt.Fprintf(events, `{"event":"reconcile","resource":%q,"scope":"window","deleted":%d}`+"\n", resource, deleted)
	return nil
}

// oncallInWindow reports whether a stored entry overlapped the window for the
// whole walk: it starts before untilLo (or has no start) and ends after
// sinceHi (or has no end). An entry it cannot read is never in it.
func oncallInWindow(data string, untilLo, sinceHi time.Time) bool {
	obj, err := store.DecodeJSONObject([]byte(data))
	if err != nil {
		return false
	}
	for _, f := range []string{"start", "end"} {
		if v, present := obj[f]; present && v != nil {
			if _, ok := v.(string); !ok {
				return false
			}
			if _, ok := pdParseTime(v.(string)); !ok {
				return false
			}
		}
	}
	start, hasStart := pdParseTime(pdString(obj, "start"))
	end, hasEnd := pdParseTime(pdString(obj, "end"))
	return (!hasStart || start.Before(untilLo)) && (!hasEnd || end.After(sinceHi))
}
