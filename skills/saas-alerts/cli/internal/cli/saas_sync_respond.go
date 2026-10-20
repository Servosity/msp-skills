// Hand-written for saas-alerts (not generated): Respond rule-trigger pages prove
// their end only with a literal pagination.hasNextPage:false (Respond 2.0.0
// example). Sync's completion branches consult this before declaring an end,
// because a declared end authorizes --full pruning of stored rows.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"saas-alerts-pp-cli/internal/store"
)

// saasRespondEndUnproven reports whether a rule-trigger page would end the walk
// without the documented terminal flag. Other resources are unaffected.
func saasRespondEndUnproven(resource string, data json.RawMessage) bool {
	if resource != "rule-triggers" {
		return false
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(data, &env) != nil {
		return true
	}
	raw, ok := rawAtPath(env, "pagination.hasNextPage")
	return !ok || strings.TrimSpace(string(raw)) != "false"
}

// saasCollectionOnly lists resources whose list endpoint always returns a
// collection; a non-collection 200 body (an error object) must fail the sync,
// never be stored as a single row and never prove the walk complete.
func saasCollectionOnly(resource string) bool {
	return resource == "rule-triggers" || resource == "events"
}

// saasRespondEnvelopeInvalid reports a page that is not the documented collection:
// for rule-triggers the {"ruleTriggers": [...], ...} envelope (not an error object,
// a null collection, or a bare array); for events a bare array. Checked right after each fetch, before generic extraction or empty-page
// detection, so a malformed body can never look like an empty final page.
func saasRespondEnvelopeInvalid(resource string, data json.RawMessage) bool {
	if resource == "events" {
		// GET /reports/events is documented as a bare array of events.
		return !strings.HasPrefix(strings.TrimSpace(string(data)), "[")
	}
	if resource != "rule-triggers" {
		return false
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(data, &env) != nil {
		return true
	}
	raw, ok := env["ruleTriggers"]
	if !ok || !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return true
	}
	if saasEnvelopeDeclaresFailure(env) {
		return true
	}
	// Every member must be a readable trigger: generic storage would keep a
	// failure-bearing row by its id, and the page would still prove the end.
	var members []json.RawMessage
	if json.Unmarshal(raw, &members) != nil {
		return true
	}
	for _, m := range members {
		if saasTriggerMemberInvalid(m) {
			return true
		}
	}
	return false
}

// saasTriggerStatuses is the RuleTrigger.status enum (Respond 2.0.0).
var saasTriggerStatuses = map[string]bool{
	"APPROVED": true, "PENDING": true, "REJECTED": true, "REMEDIATION_FAILED": true,
	"REMEDIATION_SUCCESSFUL": true, "AUTO_IGNORED": true, "IGNORED": true,
	"MANUALLY_REMEDIATED": true, "REMEDIATION_PARTIAL": true,
}

// saasTriggerMemberInvalid reports a ruleTriggers member that is not an object with
// a non-empty string id and a documented status, or that carries a failure
// indicator. `status` is the trigger's own enum field, so it is checked against
// the enum instead of as a failure indicator.
func saasTriggerMemberInvalid(data json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil || m == nil {
		return true
	}
	var id, status string
	if json.Unmarshal(m["id"], &id) != nil || strings.TrimSpace(id) == "" {
		return true
	}
	if json.Unmarshal(m["status"], &status) != nil || !saasTriggerStatuses[status] {
		return true
	}
	delete(m, "status")
	return saasEnvelopeDeclaresFailure(m)
}

// saasEnvelopeDeclaresFailure reports explicit failure indicators next to the
// collection, OR'd so a positive field never overrides a negative one: a
// non-empty error, errors, message, or status (the documented envelope carries
// none of these), success or ok set to false, or a code other than 0 or 200.
func saasEnvelopeDeclaresFailure(env map[string]json.RawMessage) bool {
	nonEmpty := func(key string) bool {
		v := strings.TrimSpace(string(env[key]))
		return v != "" && v != "null" && v != `""` && v != "[]" && v != "{}"
	}
	for _, key := range []string{"error", "errors", "message", "status"} {
		if nonEmpty(key) {
			return true
		}
	}
	for _, key := range []string{"success", "ok"} {
		if strings.TrimSpace(string(env[key])) == "false" {
			return true
		}
	}
	code := strings.Trim(strings.TrimSpace(string(env["code"])), `"`)
	return code != "" && code != "null" && code != "0" && code != "200"
}

// unifySortCursorPath is where Unify v2 (GET /manageApi/api/v1/unify/devices
// and its mapped/unmapped/ignored siblings) returns the searchAfter value for
// the next page, per the Manage 1.2.0 spec's searchAfter description.
const unifySortCursorPath = "meta.sort"

// unifyDeviceResource reports the synced Unify v2 device lists.
func unifyDeviceResource(resource string) bool {
	switch resource {
	case "devices", "devices-ignored", "devices-mapped", "devices-unmapped":
		return true
	}
	return false
}

// unifySortCursor keeps a non-empty meta.sort array of scalars as compact JSON
// (appendArrayQueryParam re-expands it into repeated searchAfter keys); any
// other shape is no cursor, so a full page without one is reported as
// truncated rather than complete.
func unifySortCursor(raw json.RawMessage) string {
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) == 0 {
		return ""
	}
	vals := make([]string, 0, len(parts))
	for _, part := range parts {
		v := cursorValue(part)
		if v == "" {
			return ""
		}
		vals = append(vals, v)
	}
	out, err := json.Marshal(vals)
	if err != nil {
		return ""
	}
	return string(out)
}

// saasEventsCoverageFloor computes the new events coverage floor after a
// completed events sync. sinceTS is the explicit --since start (RFC3339, or
// empty), lastSynced the watermark the sync started from (zero for a first or
// --full sync). A sync with no start covers the full history; an incremental
// sync from the watermark keeps the previous floor; an explicit --since start
// at or before the watermark extends the range, and one after it leaves a hole
// between the watermark and that start, so the floor restarts there.
func saasEventsCoverageFloor(sinceTS string, lastSynced time.Time, full bool, prev time.Time, havePrev bool) time.Time {
	var start time.Time
	if sinceTS != "" {
		if t, err := time.Parse(time.RFC3339, sinceTS); err == nil {
			start = t
		}
	} else if !lastSynced.IsZero() && !full {
		if havePrev {
			return prev
		}
		return lastSynced
	}
	if start.IsZero() {
		return time.Time{}
	}
	if havePrev && !lastSynced.IsZero() && !full && !start.After(lastSynced) {
		if prev.IsZero() || prev.Before(start) {
			return prev
		}
	}
	return start
}

// recordSaasEventsCoverage stores the floor after a completed events sync and,
// for an explicit sync, the watermark the next explicit sync's late-event
// look-back is anchored to.
func recordSaasEventsCoverage(db *store.Store, sinceTS string, lastSynced time.Time, full, latestOnly bool, watermark time.Time) error {
	prev, havePrev, err := db.EventsCoverage()
	if err != nil {
		return err
	}
	if err := db.SaveEventsCoverage(saasEventsCoverageFloor(sinceTS, lastSynced, full, prev, havePrev)); err != nil {
		return err
	}
	if latestOnly {
		return nil
	}
	return db.SaveEventsExplicitWatermark(watermark)
}

// saasSyncFiltered reports whether user query params (--param, --global-param,
// --resource-param <resource>:...) changed what a sync of resource fetched,
// mirroring syncUserParams.applyTo (--param skips dependents). A narrowed sync
// (organizationIds, customerId) or one with an overridden page size cannot
// prove the whole resource was enumerated, so it stores what it fetched but
// must not prune other rows, advance the watermark or record an events
// coverage floor.
//
// A parameter the resource requires (events-fileshare's startDate/endDate, the
// report resources' organizationIds) is not narrowing: the resource cannot be
// synced without it, so counting it would make that resource impossible to
// complete.
func saasSyncFiltered(p *syncUserParams, resource string, isDependent bool) bool {
	if p == nil {
		return false
	}
	required := map[string]bool{}
	for _, k := range syncResourceRequiredQueryParams(resource) {
		required[k] = true
	}
	narrows := func(m map[string]string) bool {
		for k := range m {
			if !required[k] {
				return true
			}
		}
		return false
	}
	return (!isDependent && narrows(p.flatGlobal)) || narrows(p.trueGlobal) || narrows(p.perResource[resource])
}

// emitSaasFilteredSyncWarning reports a filtered sync that does not count as
// complete.
func emitSaasFilteredSyncWarning(w io.Writer, resource string) {
	if humanFriendly {
		fmt.Fprintf(os.Stderr, "  %s: query params changed what this sync fetched, so it does not count as a complete sync\n", resource)
		return
	}
	fmt.Fprintf(w, `{"event":"sync_warning","resource":%q,"reason":"filtered_sync_not_complete","message":"query params changed what this sync fetched, so its rows are stored but it does not count as a complete sync: nothing was pruned and the incremental watermark did not move; local reads that check completeness report synced:false until an unfiltered sync finishes"}`+"\n", resource)
}

// saasResumeCursor returns the saved cursor a sync may resume from. Events
// page by offset over timeSort=desc, so a saved offset shifts as new events
// arrive at the top and resuming it would skip them; an events walk always
// restarts from the top of its window.
func saasResumeCursor(resource, saved string) string {
	if resource == "events" {
		return ""
	}
	return saved
}

// saasEventsLateAllowance is how far before the previous watermark an explicit
// incremental events sync starts. Events are filtered by their event time, and
// SaaS Alerts can pull an event after that time (the API's isLatent flag marks
// a long pull lag), so a sync starting exactly at the watermark never fetches
// an event that happened before it but was pulled after it. Rows dedupe by
// _id, so the overlap only costs requests.
const saasEventsLateAllowance = 6 * time.Hour

// saasIncrementalSince is the start of an incremental sync. The page-capped
// automatic refresh (latestOnly) keeps the plain watermark: its one page holds
// the newest events, and widening it would mark busy stores unfinished on
// every refresh. Because that refresh also moves the watermark, an explicit
// events sync anchors its look-back to the last explicit sync's watermark
// when that is older, so a late event behind it is still fetched.
func saasIncrementalSince(db *store.Store, resource string, watermark time.Time, latestOnly bool) time.Time {
	if resource != "events" || latestOnly {
		return watermark
	}
	anchor := watermark
	if db != nil {
		if explicit, ok := db.EventsExplicitWatermark(); ok && explicit.Before(anchor) {
			anchor = explicit
		}
	}
	return anchor.Add(-saasEventsLateAllowance)
}

// saasRefuseParentKeyOverride rejects a --global-param or --resource-param that
// replaces the parent key a dependent sync sets for each parent (users'
// customerId). The sync stamps every returned row with the iterated parent,
// so an override would fetch one customer's users and store copies of them
// under every other customer.
func saasRefuseParentKeyOverride(dep dependentResourceDef, queryParams []dependentPathParamDef, parentRow, params map[string]string) error {
	for _, qp := range queryParams {
		want := parentRow[qp.Field]
		if want == "" {
			continue
		}
		want = dependentParentKeyValue(dep.ParentTable, qp.Field, want)
		if got := params[qp.Param]; got != want {
			return fmt.Errorf("refusing to sync %s: a query param sets %s=%s, but the %s sync sets %s to each %s row's id itself (here %s); remove the override", dep.Name, qp.Param, got, dep.Name, qp.Param, dep.ParentTable, want)
		}
	}
	return nil
}
