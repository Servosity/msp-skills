// Hand-written for saas-alerts (not generated): Respond rule-trigger pages prove
// their end only with a literal pagination.hasNextPage:false (Respond 2.0.0
// example). Sync's completion branches consult this before declaring an end,
// because a declared end authorizes --full pruning of stored rows.

package cli

import (
	"encoding/json"
	"strings"
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
