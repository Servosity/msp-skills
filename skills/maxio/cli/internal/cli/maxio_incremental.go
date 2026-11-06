// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json incremental-window-site-zone).

package cli

import (
	"strings"
	"time"
)

// siteZoneSinceMargin moves a date-only incremental bound one day earlier.
// Maxio reads start_date as midnight in the site's time zone (spec.json
// /subscriptions.json start_date), and the bound is the UTC date of the last
// sync: west of UTC that date can be a day past the site's own, which skipped
// the evening's changes. One day covers every zone (UTC-12 to UTC+14); the
// records fetched twice are upserted again.
func siteZoneSinceMargin(value, format string) string {
	if !strings.EqualFold(format, "date") {
		return value
	}
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return d.AddDate(0, 0, -1).Format("2006-01-02")
}

// syncUpdatedAtDateField are the list endpoints whose date_field takes
// updated_at (spec.json SubscriptionDateField, InvoiceDateField, BasicDateField,
// ListSubscriptionComponentsDateField). Their default field is a creation or
// issue date, so an incremental sync without it never saw a cancellation, a
// void or any other change to an older record. A --param date_field still wins.
var syncUpdatedAtDateField = map[string]bool{
	"components-json":               true,
	"customers-json":                true,
	"invoices-json":                 true,
	"product-families-json":         true,
	"products-json":                 true,
	"subscriptions-components-json": true,
	"subscriptions-json":            true,
}

// userSyncParam reports whether the user supplied any of keys for resource
// through --param, --global-param or --resource-param.
func userSyncParam(p *syncUserParams, resource string, keys ...string) bool {
	if p == nil {
		return false
	}
	for _, src := range []map[string]string{p.flatGlobal, p.trueGlobal, p.perResource[resource]} {
		for _, k := range keys {
			if _, ok := src[k]; ok {
				return true
			}
		}
	}
	return false
}

// userDateWindowKeys are the bounds of a user's own date window.
var userDateWindowKeys = []string{"start_date", "end_date", "start_datetime", "end_datetime"}

// dropUpdatedAtUnderUserWindow removes the incremental date_field=updated_at
// when the user's own start bound replaced the since bound it rides with (and
// the user set no date_field): their window keeps its run-1 meaning.
func dropUpdatedAtUnderUserWindow(p *syncUserParams, resource string, params map[string]string) {
	if userSyncParam(p, resource, "start_date", "start_datetime") && !userSyncParam(p, resource, "date_field") {
		delete(params, "date_field")
	}
}

// syncRunReach says how much of a run's query matches a plain sync. The stored
// sync time asserts that every change since it was seen, and the checkpoint is
// shared across filters (syncCheckpointScope), so:
//
//   - plain: no user param and no --since bound, so the query is the one a plain
//     sync sends and a resume cursor means the same page in both.
//   - covers: the run saw everything since the stored time, so it may move it.
//     A --since no later than a stored time covers it; a user param of any kind
//     may narrow the result and never does.
//
// ponytail: any user param counts as narrowing, even per_page. The cost is one
// wider next run, never a lost change; add a non-narrowing list if it matters.
func syncRunReach(p *syncUserParams, resource, sinceTS, sinceParam string, stored time.Time) (plain, covers bool) {
	if p != nil && (len(p.flatGlobal) > 0 || len(p.trueGlobal) > 0 || len(p.perResource[resource]) > 0) {
		return false, false
	}
	if sinceTS == "" || sinceParam == "" {
		return true, true
	}
	since, err := time.Parse(time.RFC3339, sinceTS)
	return false, err == nil && !stored.IsZero() && !since.After(stored)
}
