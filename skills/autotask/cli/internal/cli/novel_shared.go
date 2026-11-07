// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared helpers for the transcendence (novel) commands. These commands are
// cross-entity aggregations over the LOCAL SQLite store (synced via `sync`);
// no single Autotask API call returns them. They read with store.List, which
// returns `SELECT data FROM resources WHERE resource_type = ?` rows.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"autotask-pp-cli/internal/store"
)

// novelDBPath returns the DB path for a novel command: --db override or the
// default store location.
func novelDBPath(dbFlag string) string {
	if strings.TrimSpace(dbFlag) != "" {
		return dbFlag
	}
	return defaultDBPath("autotask-cli")
}

// openNovelStore opens the local store for a novel command (--db override or
// the default path), with a sync-pointing error when the open fails.
func openNovelStore(ctx context.Context, dbFlag string) (*store.Store, error) {
	db, err := store.OpenWithContext(ctx, novelDBPath(dbFlag))
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'autotask-cli sync' first.", err)
	}
	return db, nil
}

// listEntity returns every synced record of a resource_type from an open
// store as decoded maps. resourceType is the kebab key the sync layer writes
// (e.g. "tickets", "time-entries", "contract-blocks").
//
// NOTE: store.List treats limit <= 0 as limit = 200 (the generated default for
// interactive reads). Novel aggregations must see EVERY synced row — a silent
// 200-row slice understates totals and corrupts rankings on any real tenant —
// so pass an explicit effectively-unbounded limit instead of 0.
//
// A resource is read as the whole account only when its last sync finished
// unfiltered (requireFinishedSync); every novel reads through here, so every
// novel refuses a partial store, and a caller must return the error, never
// read it as an empty result.
func listEntity(db *store.Store, resourceType string) ([]map[string]any, error) {
	if err := requireFinishedSync(db, resourceType); err != nil {
		return nil, err
	}
	raws, err := db.List(resourceType, math.MaxInt32)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raws))
	for _, r := range raws {
		var m map[string]any
		if json.Unmarshal(r, &m) == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// numAt extracts a numeric field as float64 regardless of how JSON decoded it
// (float64, json.Number, int, or numeric string). Guards the store-decode
// json.Number/UseNumber int trap: a naive float64 cast yields 0 for json.Number
// and would zero every aggregation.
func numAt(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n, true
		case float32:
			return float64(n), true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		case json.Number:
			if f, err := n.Float64(); err == nil {
				return f, true
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// intAt is numAt rounded to int64.
func intAt(m map[string]any, keys ...string) (int64, bool) {
	f, ok := numAt(m, keys...)
	return int64(f), ok
}

// strAt returns the first present string-able field value.
func strAt(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch s := v.(type) {
		case string:
			if strings.TrimSpace(s) != "" {
				return s
			}
		case json.Number:
			return s.String()
		case float64:
			return strconv.FormatFloat(s, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(s)
		}
	}
	return ""
}

// boolAt returns a boolean field (handles bool, "true"/"false", and 1/0).
func boolAt(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch b := v.(type) {
		case bool:
			return b
		case string:
			if pb, err := strconv.ParseBool(strings.TrimSpace(b)); err == nil {
				return pb
			}
		case float64:
			return b != 0
		case json.Number:
			if f, err := b.Float64(); err == nil {
				return f != 0
			}
		}
	}
	return false
}

var atTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// timeAt parses a timestamp field, ok=false when absent/unparseable.
func timeAt(m map[string]any, keys ...string) (time.Time, bool) {
	s := strAt(m, keys...)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range atTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseNovelDuration accepts Autotask-friendly windows: "24h", "7d", "2w",
// "30m", "90s", or a bare integer (days). Returns the duration.
func parseNovelDuration(arg string) (time.Duration, bool) {
	arg = strings.TrimSpace(strings.ToLower(arg))
	if arg == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(arg); err == nil {
		return time.Duration(n) * 24 * time.Hour, true
	}
	if strings.HasSuffix(arg, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(arg, "d")); err == nil {
			return time.Duration(n) * 24 * time.Hour, true
		}
		return 0, false
	}
	if strings.HasSuffix(arg, "w") {
		if n, err := strconv.Atoi(strings.TrimSuffix(arg, "w")); err == nil {
			return time.Duration(n) * 7 * 24 * time.Hour, true
		}
		return 0, false
	}
	if d, err := time.ParseDuration(arg); err == nil {
		return d, true
	}
	return 0, false
}

// ageBucketLabel classifies an age in hours into a service-desk aging bucket.
func ageBucketLabel(hours float64) string {
	switch {
	case hours < 24:
		return "0-1d"
	case hours < 72:
		return "1-3d"
	case hours < 168:
		return "3-7d"
	case hours < 336:
		return "7-14d"
	case hours < 720:
		return "14-30d"
	default:
		return "30d+"
	}
}

// ticketCreated returns the most relevant timestamp for aging a ticket: prefer
// createDate, fall back to lastActivityDate.
func ticketCreated(m map[string]any) (time.Time, bool) {
	if t, ok := timeAt(m, "createDate", "CreateDate"); ok {
		return t, true
	}
	return timeAt(m, "lastActivityDate", "LastActivityDate")
}

// isTicketOpen reports whether a ticket is in an open state. Autotask status 5
// is "Complete"; everything else is open. Missing status => open (never drop).
func isTicketOpen(m map[string]any) bool {
	if s, ok := intAt(m, "status"); ok {
		return s != 5
	}
	return true
}

// accrueBlock folds one contract-block's hours into purchased/used/remaining,
// honoring Autotask's hoursLeft|hoursRemaining vs hoursUsed|hoursApproved
// split: when the block reports what's left, used is derived; when it reports
// what's used, remaining is derived. This is the single source of truth for
// the field-name contract — contract-burn, reconcile, account-brief, and
// retainer all accumulate through it.
func accrueBlock(b map[string]any) (purchased, used, remaining float64) {
	purchased, _ = numAt(b, "hours")
	if u, ok := numAt(b, "hoursApproved"); ok {
		return purchased, u, purchased - u
	}
	return purchased, 0, 0
}

// requireFinishedSync refuses a resource whose stored rows are not backed by a
// finished, unfiltered sync (autotask handfix, class
// incomplete-snapshot-read-as-complete, pre-closed from the atera 4.34.2
// reprint's Codex r1): a sync_state row whose last attempt did not complete or
// never recorded a sync time (a sync that stopped part way), a walk that sent
// a filter (autotask_sync_scope.go), rows with no sync_state at all (a live
// read's write-through), or a resource never synced in a store where other
// resources were (a report joining it would read it as empty: data-gaps would
// call every ticket's company an orphan). A store where nothing was synced
// yet keeps the documented empty result and its sync hint.
func requireFinishedSync(s *store.Store, resourceType string) error {
	if s == nil {
		return nil
	}
	var complete int
	var synced *string
	found, err := novelQueryOne(s, `SELECT last_attempt_complete, CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, []any{resourceType}, &complete, &synced)
	if err != nil {
		return err
	}
	if found {
		if complete != 1 || synced == nil || *synced == "" {
			return fmt.Errorf("the last sync of %s did not finish, so the local store holds only part of it; run 'autotask-cli sync --resources %s' again before trusting this report", resourceType, resourceType)
		}
		params, filtered, err := syncWalkFiltered(s, resourceType)
		if err != nil {
			return err
		}
		if filtered && params == syncFiltersUnrecorded {
			return fmt.Errorf("the last sync of %s was written by an older build or by 'workflow archive', which record no filters, so the local store may hold only part of it; run 'autotask-cli sync --resources %s' before trusting this report", resourceType, resourceType)
		}
		if filtered {
			return fmt.Errorf("the last sync of %s sent filters (%s), so the local store holds only the records they matched; run 'autotask-cli sync --resources %s' without --param, --resource-param or --global-param before trusting this report", resourceType, params, resourceType)
		}
		return nil
	}
	var n int
	if _, err := novelQueryOne(s, `SELECT COUNT(*) FROM resources WHERE resource_type = ?`, []any{resourceType}, &n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%s has %d record(s) in the local store from live reads but was never synced, so the store holds only part of it; run 'autotask-cli sync --resources %s' before trusting this report", resourceType, n, resourceType)
	}
	var syncedAny int
	if _, err := novelQueryOne(s, `SELECT COUNT(*) FROM sync_state`, nil, &syncedAny); err != nil {
		return err
	}
	if syncedAny > 0 {
		return fmt.Errorf("%s was never synced, so this report would read it as empty; run 'autotask-cli sync --resources %s' before trusting this report", resourceType, resourceType)
	}
	return nil
}

// novelQueryOne scans the first row of a query; found is false when it has none.
func novelQueryOne(s *store.Store, query string, args []any, dest ...any) (bool, error) {
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

// timeEntryCompanies answers what a time entry cannot say itself: TimeEntryModel
// (spec.json) carries no companyID, only ticketID, taskID and contractID, so the
// company comes from the ticket, the task's project, or the contract, in that
// order. Every source must be a finished sync (listEntity refuses otherwise).
type timeEntryCompanies struct{ ticket, task, project, contract map[string]string }

func loadTimeEntryCompanies(db *store.Store) (*timeEntryCompanies, error) {
	r := &timeEntryCompanies{}
	for _, src := range []struct {
		resource, field string
		into            *map[string]string
	}{
		{"tickets", "companyID", &r.ticket},
		{"tasks", "projectID", &r.task},
		{"projects", "companyID", &r.project},
		{"contracts", "companyID", &r.contract},
	} {
		rows, err := listEntity(db, src.resource)
		if err != nil {
			return nil, err
		}
		m := make(map[string]string, len(rows))
		for _, row := range rows {
			m[strAt(row, "id")] = strAt(row, src.field)
		}
		*src.into = m
	}
	return r, nil
}

func (r *timeEntryCompanies) company(e map[string]any) string {
	if c := r.ticket[strAt(e, "ticketID", "ticketId")]; c != "" {
		return c
	}
	if c := r.project[r.task[strAt(e, "taskID", "taskId")]]; c != "" {
		return c
	}
	if c := r.contract[strAt(e, "contractID", "contractId")]; c != "" {
		return c
	}
	return strAt(e, "companyID", "companyId")
}

// timeEntryUnbilled: billable time not yet approved and posted for billing.
// Approve and Post sets billingApprovalDateTime; spec.json documents no invoice
// link on a time entry.
func timeEntryUnbilled(e map[string]any) bool {
	return !boolAt(e, "isNonBillable") && strAt(e, "billingApprovalDateTime") == ""
}

// timeEntryBillHours is what the entry would bill: hoursToBill, else hoursWorked.
func timeEntryBillHours(e map[string]any) float64 {
	h, _ := numAt(e, "hoursToBill", "hoursWorked")
	return h
}

// isTaskDone: TaskModel (spec.json) documents no percentComplete; a task is
// done when its status is Complete (5) or it carries a completedDateTime.
func isTaskDone(tk map[string]any) bool {
	if s, ok := intAt(tk, "status"); ok && s == 5 {
		return true
	}
	return strAt(tk, "completedDateTime") != ""
}
