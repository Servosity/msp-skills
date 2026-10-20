// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (not generated): shared Respond rule-trigger model, live/local
// loading, and decision-deadline math for 'rule-triggers queue' and
// 'rule-triggers decide'.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"saas-alerts-pp-cli/internal/client"
	"saas-alerts-pp-cli/internal/cliutil"
	"saas-alerts-pp-cli/internal/store"
)

// rqNow is the clock behind every Respond deadline computation. Logic never
// calls time.Now directly so tests can pin it.
var rqNow = time.Now

const (
	// Respond spec 2.0.0, rule ruleTriggerType MANUAL: "Manual approval will auto
	// expire when the rule triggers again or 12 hours whichever is earlier".
	rqManualApprovalWindow = 12 * time.Hour
	// SaaS Alerts help docs: a failed remediation gets a reminder every 15
	// minutes and is auto-ignored after the 3rd reminder.
	rqFailedReminderInterval = 15 * time.Minute
	rqFailedReminderLimit    = 3

	rqRuleTriggersPath = "/respondApi/api/v1/rule-triggers"
	rqRulesPath        = "/respondApi/api/v1/rules"
	// rqLivePageLimit is sent as `limit`. The spec only says it limits the number
	// of rule triggers; with cursor pagination it acts as the page size (INFERRED).
	rqLivePageLimit   = 100
	rqDefaultMaxPages = 5

	rqKindExpiresAt    = "expires_at"
	rqKindAutoIgnoreAt = "auto_ignore_at"
	rqKindNone         = "none"
)

// rqValidStatuses is the rule-trigger status enum from the Respond spec.
var rqValidStatuses = []string{
	"APPROVED", "PENDING", "REJECTED", "REMEDIATION_FAILED", "REMEDIATION_SUCCESSFUL",
	"IGNORED", "AUTO_IGNORED", "MANUALLY_REMEDIATED", "REMEDIATION_PARTIAL",
}

// rqDestructiveActions are rule response actions that cannot be walked back
// from Respond: a reset password is gone and a deleted user is deleted.
var rqDestructiveActions = map[string]bool{
	"Reset User Password": true,
	"Delete User":         true,
}

// rqMillis decodes a Respond epoch-milliseconds field. The spec types these as
// integer/number; a numeric string or RFC3339 string is accepted too, and any
// other shape decodes as absent (0) instead of failing the whole trigger.
type rqMillis int64

func (m *rqMillis) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	*m = 0
	if s == "" || s == "null" {
		return nil
	}
	s = strings.Trim(s, `"`)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*m = rqMillis(int64(f))
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		*m = rqMillis(t.UnixMilli())
	}
	return nil
}

func (m rqMillis) time() time.Time { return time.UnixMilli(int64(m)).UTC() }

type rqAccount struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Product  struct {
		Type string `json:"type"`
	} `json:"product"`
}

type rqIDName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type rqResponseAction struct {
	ApplicationType string `json:"applicationType"`
	ResponseAction  string `json:"responseAction"`
}

type rqRule struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	AlertSeverity   string             `json:"alertSeverity"`
	ResponseActions []rqResponseAction `json:"responseActions"`
	RuleTriggerType string             `json:"ruleTriggerType"`
}

type rqHistory struct {
	Type      string   `json:"type"`
	Timestamp rqMillis `json:"timestamp"`
}

// rqTrigger is the subset of the Respond RuleTrigger schema these commands use.
// actionsKnown records whether rule.responseActions was present and every
// entry named a known action, i.e. whether wouldFire() can be trusted.
type rqTrigger struct {
	ID           string      `json:"id"`
	Status       string      `json:"status"`
	TriggeredAt  rqMillis    `json:"triggeredAt"`
	CreatedAt    rqMillis    `json:"createdAt"`
	UpdatedAt    rqMillis    `json:"updatedAt"`
	Accounts     []rqAccount `json:"accounts"`
	Organization rqIDName    `json:"organization"`
	Rule         rqRule      `json:"rule"`
	History      []rqHistory `json:"history"`

	actionsKnown bool
}

// UnmarshalJSON decodes a trigger and records whether its rule's action list
// is complete. A null rule, a missing responseActions, or an entry without a
// known responseAction leaves the actions unknown: an empty wouldFire() then
// means "unknown", not "fires nothing".
func (t *rqTrigger) UnmarshalJSON(b []byte) error {
	type plain rqTrigger
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*t = rqTrigger(p)
	var raw struct {
		Rule json.RawMessage `json:"rule"`
	}
	if json.Unmarshal(b, &raw) == nil {
		t.actionsKnown = rqRuleActionsComplete(raw.Rule)
	}
	return nil
}

// rqKnownResponseActions is the responseAction enum from the Respond spec.
var rqKnownResponseActions = map[string]bool{
	"Block Sign In": true, "Force User to Change Password on Next Sign In": true, "Alert Only": true,
	"Expire Account Logins": true, "Reset User Password": true, "Setup User MFA": true, "Delete User": true,
}

// rqRuleActionsComplete reports whether a raw Rule object carries a
// responseActions array whose every entry names a known action. An explicit
// empty array is complete (the rule fires nothing).
func rqRuleActionsComplete(rawRule json.RawMessage) bool {
	if !rqIsJSONObject(rawRule) {
		return false
	}
	var rule map[string]json.RawMessage
	if json.Unmarshal(rawRule, &rule) != nil || !rqIsJSONArray(rule["responseActions"]) {
		return false
	}
	// A rule object that carries a failure indicator is not evidence of
	// anything, so its action list stays unknown. A rule's status (ACTIVE,
	// ...) is its own field, not a failure signal.
	if rqFailureIndicator(rule, true) != "" {
		return false
	}
	var entries []map[string]json.RawMessage
	if json.Unmarshal(rule["responseActions"], &entries) != nil {
		return false
	}
	for _, e := range entries {
		var action string
		if json.Unmarshal(e["responseAction"], &action) != nil || !rqKnownResponseActions[action] {
			return false
		}
	}
	return true
}

// rqFetchRule reads GET /rules/{id} (a bare Rule per the spec) and returns its
// response actions only when the list is complete and the id matches.
func rqFetchRule(ctx context.Context, c *client.Client, id string) ([]rqResponseAction, error) {
	path := rqRulesPath + "/" + url.PathEscape(id)
	data, err := c.GetNoCache(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var rule rqRule
	if !rqIsJSONObject(data) || json.Unmarshal(data, &rule) != nil {
		return nil, fmt.Errorf("unexpected GET %s response: expected a rule object, got %s", path, rqDescribeJSON(data))
	}
	if rule.ID != id {
		return nil, fmt.Errorf("unexpected GET %s response: it is not rule %s", path, id)
	}
	if !rqRuleActionsComplete(data) {
		return nil, fmt.Errorf("GET %s did not list complete responseActions (or carried a failure indicator)", path)
	}
	return rule.ResponseActions, nil
}

// rqDescribeJSON describes a response body by shape only (type, object key
// names that look like identifiers, sizes), never by value: bodies can echo
// credentials, and only HTTP/transport errors pass through the client's masker.
func rqDescribeJSON(raw json.RawMessage) string {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return "an empty body"
	}
	var v any
	if json.Unmarshal(t, &v) != nil {
		return fmt.Sprintf("invalid JSON (%d bytes)", len(t))
	}
	switch tv := v.(type) {
	case nil:
		return "null"
	case bool:
		return "a boolean"
	case float64:
		return "a number"
	case string:
		return fmt.Sprintf("a string of %d characters", len(tv))
	case []any:
		return fmt.Sprintf("an array of %d elements", len(tv))
	case map[string]any:
		keys := make([]string, 0, len(tv))
		other := 0
		for k := range tv {
			if rqDescribableKeys[k] {
				keys = append(keys, k)
			} else {
				other++
			}
		}
		sort.Strings(keys)
		desc := "an object with keys [" + strings.Join(keys, " ") + "]"
		if other > 0 {
			desc += fmt.Sprintf(" and %d other keys", other)
		}
		return desc
	}
	return fmt.Sprintf("%d bytes", len(t))
}

// rqDescribableKeys are the schema key names a shape description may show;
// any other key, however ordinary it looks, is only counted, because a key
// name can itself be a value (a token used as a map key, ...).
var rqDescribableKeys = map[string]bool{
	"ruleTriggers": true, "pagination": true, "hasNextPage": true, "cursor": true,
	"ruleTrigger": true, "events": true, "lastRuleTriggerTime": true,
	"message": true, "error": true, "errors": true, "status": true, "id": true,
	"executedAt": true, "executedBy": true, "executedByIp": true, "eventIds": true,
	"createdAt": true, "updatedAt": true, "total": true, "success": true, "failed": true,
	"missed": true, "completed": true, "action": true, "createdBy": true, "partnerId": true,
	"channelId": true, "url": true, "enabled": true, "domain": true, "unsentChannelIds": true,
	"rule": true, "responseActions": true, "jobs": true,
}

// effectiveTriggeredAt returns triggeredAt, falling back to createdAt when the
// trigger carries no triggeredAt (every spec example has one; the fallback
// keeps a trigger with a missing field orderable instead of dropping it).
func (t rqTrigger) effectiveTriggeredAt() (time.Time, string) {
	if t.TriggeredAt > 0 {
		return t.TriggeredAt.time(), "triggeredAt"
	}
	if t.CreatedAt > 0 {
		return t.CreatedAt.time(), "createdAt"
	}
	return time.Time{}, "missing"
}

func (t rqTrigger) wouldFire() []string {
	out := make([]string, 0, len(t.Rule.ResponseActions))
	seen := map[string]bool{}
	for _, ra := range t.Rule.ResponseActions {
		if ra.ResponseAction == "" || seen[ra.ResponseAction] {
			continue
		}
		seen[ra.ResponseAction] = true
		out = append(out, ra.ResponseAction)
	}
	return out
}

func (t rqTrigger) destructiveActions() []string {
	out := make([]string, 0)
	for _, a := range t.wouldFire() {
		if rqDestructiveActions[a] {
			out = append(out, a)
		}
	}
	return out
}

func (t rqTrigger) accountEmails() []string {
	out := make([]string, 0, len(t.Accounts))
	for _, a := range t.Accounts {
		if a.Email != "" {
			out = append(out, a.Email)
		}
	}
	return out
}

// rqQueueItem is one trigger with its decision deadline computed. Every field
// is always emitted (no omitempty) so --agent compaction keeps the columns.
type rqQueueItem struct {
	TriggerID         string   `json:"trigger_id"`
	Status            string   `json:"status"`
	RuleID            string   `json:"rule_id"`
	RuleName          string   `json:"rule_name"`
	Severity          string   `json:"severity"`
	RuleTriggerType   string   `json:"rule_trigger_type"`
	OrganizationID    string   `json:"organization_id"`
	OrganizationName  string   `json:"organization_name"`
	Accounts          []string `json:"accounts"`
	WouldFire         []string `json:"would_fire"`
	WouldFireKnown    bool     `json:"would_fire_known"`
	Destructive       bool     `json:"destructive"`
	TriggeredAt       string   `json:"triggered_at"`
	TriggeredAtSource string   `json:"triggered_at_source"`
	DeadlineKind      string   `json:"deadline_kind"`
	Deadline          *string  `json:"deadline"`
	ExpiresAt         *string  `json:"expires_at"`
	ExpiryReason      string   `json:"expiry_reason"`
	Superseded        bool     `json:"superseded"`
	SupersededBy      string   `json:"superseded_by"`
	RemindersSent     *int     `json:"reminders_sent"`
	AutoIgnoreAt      *string  `json:"auto_ignore_at"`
	TimeLeft          *int64   `json:"time_left_seconds"`
	Expired           bool     `json:"expired"`

	deadline  time.Time
	triggered time.Time
}

func rqFormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func rqStringPtr(s string) *string { return &s }

// rqAnnotate computes per-trigger deadlines against now. Supersession looks
// only at triggers in the given set: a newer trigger for the same key that
// was filtered out (other status, other org, beyond --max-pages) is invisible.
// UNVERIFIED scope, keyed conservatively (saas-alerts hand-fix, adversarial
// review 2026-10-20): the Respond spec says only "auto expire when the rule
// triggers again", without saying whether that is rule-wide or per customer.
// Keying by rule.id alone let one customer's firing hide another customer's
// live approval (expired_count, skipped by decide). The key is rule.id plus
// organization.id plus the trigger's accounts, so an unverified reading can
// only err toward showing a trigger Respond already expired, whose approve the
// API then refuses, never toward hiding a live one. Widen it only against a
// live tenant.
func rqAnnotate(triggers []rqTrigger, now time.Time) []rqQueueItem {
	type stamp struct {
		at time.Time
		id string
	}
	byRule := map[string][]stamp{}
	for _, t := range triggers {
		if t.Rule.ID == "" {
			continue
		}
		at, _ := t.effectiveTriggeredAt()
		if at.IsZero() {
			continue
		}
		byRule[rqSupersessionKey(t)] = append(byRule[rqSupersessionKey(t)], stamp{at: at, id: t.ID})
	}

	items := make([]rqQueueItem, 0, len(triggers))
	for _, t := range triggers {
		at, atSource := t.effectiveTriggeredAt()
		destructive := len(t.destructiveActions()) > 0
		item := rqQueueItem{
			TriggerID:         t.ID,
			Status:            t.Status,
			RuleID:            t.Rule.ID,
			RuleName:          t.Rule.Name,
			Severity:          t.Rule.AlertSeverity,
			RuleTriggerType:   t.Rule.RuleTriggerType,
			OrganizationID:    t.Organization.ID,
			OrganizationName:  t.Organization.Name,
			Accounts:          t.accountEmails(),
			WouldFire:         t.wouldFire(),
			WouldFireKnown:    t.actionsKnown,
			Destructive:       destructive,
			TriggeredAtSource: atSource,
			DeadlineKind:      rqKindNone,
			triggered:         at,
		}
		if !at.IsZero() {
			item.TriggeredAt = rqFormatTime(at)
		}

		switch strings.ToUpper(t.Status) {
		case "PENDING":
			if at.IsZero() {
				break
			}
			expires := at.Add(rqManualApprovalWindow)
			reason := "12h"
			var newer stamp
			for _, s := range byRule[rqSupersessionKey(t)] {
				if s.id == t.ID || !s.at.After(at) {
					continue
				}
				if newer.id == "" || s.at.Before(newer.at) {
					newer = s
				}
			}
			if newer.id != "" {
				item.Superseded = true
				item.SupersededBy = newer.id
				if !newer.at.After(expires) {
					expires = newer.at
					reason = "superseded"
				}
			}
			item.DeadlineKind = rqKindExpiresAt
			item.deadline = expires
			item.ExpiresAt = rqStringPtr(rqFormatTime(expires))
			item.ExpiryReason = reason
		case "REMEDIATION_FAILED":
			failedAt := time.Time{}
			reminders := 0
			for _, h := range t.History {
				switch strings.ToUpper(h.Type) {
				case "REMEDIATION_FAILED":
					if h.Timestamp > 0 && h.Timestamp.time().After(failedAt) {
						failedAt = h.Timestamp.time()
					}
				case "REMEDIATION_FAILED_REMINDER":
					reminders++
				}
			}
			// No REMEDIATION_FAILED history entry: fall back to updatedAt (the
			// status change), then the trigger time, so the item stays ranked.
			if failedAt.IsZero() && t.UpdatedAt > 0 {
				failedAt = t.UpdatedAt.time()
			}
			if failedAt.IsZero() {
				failedAt = at
			}
			if failedAt.IsZero() {
				break
			}
			autoIgnore := failedAt.Add(rqFailedReminderLimit * rqFailedReminderInterval)
			r := reminders
			item.RemindersSent = &r
			item.DeadlineKind = rqKindAutoIgnoreAt
			item.deadline = autoIgnore
			item.AutoIgnoreAt = rqStringPtr(rqFormatTime(autoIgnore))
		}

		if !item.deadline.IsZero() {
			left := int64(item.deadline.Sub(now) / time.Second)
			item.TimeLeft = &left
			item.Deadline = rqStringPtr(rqFormatTime(item.deadline))
			item.Expired = !item.deadline.After(now)
		}
		items = append(items, item)
	}
	return items
}

// rqSortQueue orders by deadline ascending: live deadlines first, then
// triggers with no computable deadline, then expired ones (still ascending).
func rqSortQueue(items []rqQueueItem) {
	class := func(it rqQueueItem) int {
		switch {
		case it.deadline.IsZero():
			return 1
		case it.Expired:
			return 2
		default:
			return 0
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		ca, cb := class(a), class(b)
		if ca != cb {
			return ca < cb
		}
		if !a.deadline.Equal(b.deadline) {
			return a.deadline.Before(b.deadline)
		}
		if !a.triggered.Equal(b.triggered) {
			return a.triggered.Before(b.triggered)
		}
		return a.TriggerID < b.TriggerID
	})
}

// rqSplitCSV splits a comma-separated flag value, trimming blanks.
func rqSplitCSV(raw string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// rqParseStatuses upper-cases and validates a comma-separated status list.
func rqParseStatuses(raw, flagName string) ([]string, error) {
	statuses := make([]string, 0)
	for _, s := range rqSplitCSV(raw) {
		up := strings.ToUpper(s)
		valid := false
		for _, v := range rqValidStatuses {
			if v == up {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("--%s: unknown status %q (valid: %s)", flagName, s, strings.Join(rqValidStatuses, ", "))
		}
		statuses = append(statuses, up)
	}
	if len(statuses) == 0 {
		return nil, fmt.Errorf("--%s needs at least one status (valid: %s)", flagName, strings.Join(rqValidStatuses, ", "))
	}
	return statuses, nil
}

// rqFilter is the selection applied to live and local trigger sets alike.
type rqFilter struct {
	Statuses []string
	OrgIDs   []string
	RuleIDs  []string
}

// rqContainsExact is for opaque ids (trigger, organization, rule), which are
// compared byte for byte; rqContainsFold is only for statuses, decisions and
// emails.
func rqContainsExact(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func rqContainsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func (f rqFilter) match(t rqTrigger) bool {
	if len(f.Statuses) > 0 && !rqContainsFold(f.Statuses, t.Status) {
		return false
	}
	// Organization and rule ids are opaque: compared exactly, never by case.
	if len(f.OrgIDs) > 0 && !rqContainsExact(f.OrgIDs, t.Organization.ID) {
		return false
	}
	if len(f.RuleIDs) > 0 && !rqContainsExact(f.RuleIDs, t.Rule.ID) {
		return false
	}
	return true
}

func (f rqFilter) apply(triggers []rqTrigger) []rqTrigger {
	out := make([]rqTrigger, 0, len(triggers))
	for _, t := range triggers {
		if f.match(t) {
			out = append(out, t)
		}
	}
	return out
}

// rqListPath builds the GET /rule-triggers path with the array filters the
// spec declares as form/explode query params.
func rqListPath(f rqFilter) string {
	path := rqRuleTriggersPath
	if len(f.Statuses) > 0 {
		path = appendArrayQueryParam(path, "statuses[]", strings.Join(f.Statuses, ","), "form", true)
	}
	if len(f.OrgIDs) > 0 {
		path = appendArrayQueryParam(path, "organizationIds[]", strings.Join(f.OrgIDs, ","), "form", true)
	}
	if len(f.RuleIDs) > 0 {
		path = appendArrayQueryParam(path, "ruleIds[]", strings.Join(f.RuleIDs, ","), "form", true)
	}
	return path
}

// rqPage is one decoded GET /rule-triggers page. HasNextPage is nil unless
// pagination.hasNextPage was a literal JSON boolean.
type rqPage struct {
	RuleTriggers []json.RawMessage
	HasNextPage  *bool
	Cursor       json.RawMessage
}

// rqTruncPaginationFlag marks a page whose pagination.hasNextPage was absent,
// null or not a boolean: whether more triggers exist is unknown.
const rqTruncPaginationFlag = "pagination_flag_missing"

// rqCursorValue renders one startAfter cursor element: the spec cursor is
// [triggeredAt number, trigger id string].
func rqCursorValue(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	switch tv := v.(type) {
	case string:
		return tv
	case float64:
		return strconv.FormatFloat(tv, 'f', -1, 64)
	default:
		return ""
	}
}

type rqLiveResult struct {
	Triggers  []rqTrigger
	Pages     int
	Truncated bool
	// TruncatedReason says why paging stopped while the API still reported
	// more triggers (max pages reached, unusable or repeated cursor, ...).
	TruncatedReason string
	Undecoded       int
	MaxPages        int
	FilterPath      string
}

// rqDecodeTriggers decodes raw trigger objects, counting the ones that do
// not decode instead of failing the whole page.
func rqDecodeTriggers(raws []json.RawMessage) ([]rqTrigger, int) {
	out := make([]rqTrigger, 0, len(raws))
	bad := 0
	for _, raw := range raws {
		var t rqTrigger
		// A row needs an id and a RuleTrigger status: a null, missing or
		// unknown status is a rejected row (counted), not a quietly
		// filtered-out one, so a page of such rows is never a complete empty
		// queue.
		if err := json.Unmarshal(raw, &t); err != nil || t.ID == "" || !rqContainsFold(rqValidStatuses, t.Status) || rqRowFailure(raw) {
			bad++
			continue
		}
		t.Status = strings.ToUpper(t.Status)
		out = append(out, t)
	}
	return out, bad
}

// rqRowFailure applies the failure rule to one bulk trigger row (its status
// is the trigger's own field): a flagged row is rejected, not trusted.
func rqRowFailure(raw json.RawMessage) bool {
	var row map[string]json.RawMessage
	return json.Unmarshal(raw, &row) == nil && rqFailureIndicator(row, true) != ""
}

// rqEnvelopeFailure applies the shared failure rule to a page envelope
// (whose documented keys are only ruleTriggers and pagination).
func rqEnvelopeFailure(obj map[string]json.RawMessage) string {
	return rqFailureIndicator(obj, false)
}

// rqFailureIndicator is the one failure rule for response objects (the same
// rule the sync path uses, so both agree). The object is a failure if ANY of:
// a non-empty error; a non-empty errors; a non-empty message; a non-null,
// non-empty status (skipped when statusIsData: on a RuleTrigger, status is
// the trigger's own field); success:false; ok:false; a non-null code that is
// not 0 or 200. Positive fields (success:true, ok:true) never override a
// failure field. It returns a value-free description, or "" for none.
func rqFailureIndicator(obj map[string]json.RawMessage, statusIsData bool) string {
	for _, field := range []struct{ key, desc string }{{"error", "an error field"}, {"errors", "an errors field"}, {"message", "a message field"}} {
		if v, ok := obj[field.key]; ok && !containJSONEmpty(v) {
			return field.desc
		}
	}
	if !statusIsData {
		if v, ok := obj["status"]; ok && rqNonEmptyValue(v) {
			return "a status field"
		}
	}
	for _, key := range []string{"success", "ok"} {
		if string(bytes.TrimSpace(obj[key])) == "false" {
			return key + ":false"
		}
	}
	if v, ok := obj["code"]; ok && rqNonEmptyValue(v) {
		switch string(bytes.TrimSpace(v)) {
		case "0", "200", `"0"`, `"200"`:
		default:
			return "a non-success code"
		}
	}
	return ""
}

// rqNonEmptyValue reports a value other than null, "", [] or {}.
func rqNonEmptyValue(v json.RawMessage) bool {
	switch string(bytes.TrimSpace(v)) {
	case "", "null", `""`, "[]", "{}":
		return false
	}
	return true
}

// rqFetchLive pages GET /rule-triggers up to maxPages, passing the response
// cursor back as a two-element startAfter array. Responses bypass the HTTP
// cache: deadlines are minutes-sensitive and decide must act on fresh state.
func rqFetchLive(ctx context.Context, c *client.Client, f rqFilter, maxPages int) (rqLiveResult, error) {
	if cliutil.IsDogfoodEnv() {
		maxPages = 1
	}
	if maxPages < 1 {
		maxPages = 1
	}
	base := rqListPath(f)
	res := rqLiveResult{Triggers: make([]rqTrigger, 0), MaxPages: maxPages, FilterPath: base}
	params := map[string]string{"limit": strconv.Itoa(rqLivePageLimit)}
	cursor := ""
	seenCursors := map[string]bool{}
	seenIDs := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		path := base
		if cursor != "" {
			path = appendArrayQueryParam(base, "startAfter", cursor, "form", true)
		}
		data, err := c.GetNoCache(ctx, path, params)
		if err != nil {
			return res, err
		}
		res.Pages++
		pg, err := rqDecodePage(data)
		if err != nil {
			return res, err
		}
		triggers, bad := rqDecodeTriggers(pg.RuleTriggers)
		for _, t := range triggers {
			// A page served twice must not double-count its triggers.
			if seenIDs[t.ID] {
				continue
			}
			seenIDs[t.ID] = true
			res.Triggers = append(res.Triggers, t)
		}
		res.Undecoded += bad
		// Only a literal boolean is authoritative. Without one the API has not
		// said this is the last page, so the result is marked truncated.
		if pg.HasNextPage == nil {
			res.Truncated, res.TruncatedReason = true, rqTruncPaginationFlag
			return res, nil
		}
		if !*pg.HasNextPage {
			return res, nil
		}
		// The API says more triggers exist: every way of not following that
		// is reported as truncation, never a silent stop.
		if len(pg.RuleTriggers) == 0 {
			res.Truncated, res.TruncatedReason = true, "the API returned an empty page with hasNextPage=true"
			return res, nil
		}
		var cursorParts []json.RawMessage
		if json.Unmarshal(pg.Cursor, &cursorParts) != nil || len(cursorParts) != 2 {
			res.Truncated, res.TruncatedReason = true, "hasNextPage=true but the pagination cursor is missing or not a [triggeredAt, id] pair"
			return res, nil
		}
		ts, id := rqCursorValue(cursorParts[0]), rqCursorValue(cursorParts[1])
		if ts == "" || id == "" {
			res.Truncated, res.TruncatedReason = true, "hasNextPage=true but the pagination cursor is not a usable [triggeredAt, id] pair"
			return res, nil
		}
		next := ts + "," + id
		if seenCursors[next] {
			// The cursor values are not copied: they come from the response.
			res.Truncated, res.TruncatedReason = true, "the API repeated a pagination cursor it had already returned; stopped to avoid looping"
			return res, nil
		}
		seenCursors[next] = true
		cursor = next
		if page == maxPages-1 {
			res.Truncated, res.TruncatedReason = true, fmt.Sprintf("stopped after --max-pages %d with more triggers waiting", maxPages)
		}
	}
	return res, nil
}

// rqDecodePage decodes one GET /rule-triggers body. A bare JSON array is
// accepted as a single page; an object must carry a ruleTriggers array, so an
// error-shaped 200 (e.g. {"message":"backend failure"}) is an error, never an
// empty queue.
func rqDecodePage(data json.RawMessage) (rqPage, error) {
	var pg rqPage
	if rqIsJSONArray(data) {
		// A bare array carries no pagination, so HasNextPage stays nil and
		// the caller reports the result as possibly truncated.
		if err := json.Unmarshal(data, &pg.RuleTriggers); err != nil {
			return pg, fmt.Errorf("unexpected GET %s response: the trigger array did not decode (%s)", rqRuleTriggersPath, rqDescribeJSON(data))
		}
		return pg, nil
	}
	var obj map[string]json.RawMessage
	// json.Unmarshal accepts a null body into a map or slice as "empty";
	// require an actual object so null is an error, not an empty queue.
	if !rqIsJSONObject(data) || json.Unmarshal(data, &obj) != nil {
		return pg, fmt.Errorf("unexpected GET %s response: expected an object with ruleTriggers, got %s", rqRuleTriggersPath, rqDescribeJSON(data))
	}
	// The documented envelope carries only ruleTriggers and pagination: an
	// explicit failure indicator beside them is an error, never an empty
	// (complete) page.
	if why := rqEnvelopeFailure(obj); why != "" {
		return pg, fmt.Errorf("GET %s answered with %s; the queue could not be read", rqRuleTriggersPath, why)
	}
	rawTriggers, ok := obj["ruleTriggers"]
	if !ok {
		return pg, fmt.Errorf("unexpected GET %s response without ruleTriggers: expected an object with ruleTriggers, got %s", rqRuleTriggersPath, rqDescribeJSON(data))
	}
	if !rqIsJSONArray(rawTriggers) || json.Unmarshal(rawTriggers, &pg.RuleTriggers) != nil {
		return pg, fmt.Errorf("unexpected GET %s response: ruleTriggers is not an array, got %s", rqRuleTriggersPath, rqDescribeJSON(rawTriggers))
	}
	// pagination is read field by field: a missing, null or non-object
	// pagination, or a hasNextPage that is not a boolean, leaves HasNextPage
	// nil instead of failing or reading as false.
	if rawPagination := obj["pagination"]; rqIsJSONObject(rawPagination) {
		var pag map[string]json.RawMessage
		if json.Unmarshal(rawPagination, &pag) == nil {
			switch string(bytes.TrimSpace(pag["hasNextPage"])) {
			case "true":
				v := true
				pg.HasNextPage = &v
			case "false":
				v := false
				pg.HasNextPage = &v
			}
			pg.Cursor = pag["cursor"]
		}
	}
	return pg, nil
}

// rqIsJSONArray and rqIsJSONObject check the raw token, because
// json.Unmarshal turns null into an empty slice or map without an error.
func rqIsJSONArray(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '['
}

func rqIsJSONObject(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// rqFetchOne reads GET /rule-triggers/{id}. The spec documents two 200
// shapes: the schema is a bare RuleTrigger, and the example wraps it as
// {"ruleTrigger": {...}, "events": [...]}. Either is accepted, but only when
// it holds a trigger object whose id is the one requested; null, an
// error-shaped object, {"ruleTrigger": null} or another trigger's id is an
// error. A 404 comes back as the client's *client.APIError.
func rqFetchOne(ctx context.Context, c *client.Client, id string) (rqTrigger, error) {
	path := rqRuleTriggersPath + "/" + url.PathEscape(id)
	data, err := c.GetNoCache(ctx, path, nil)
	if err != nil {
		return rqTrigger{}, err
	}
	t, why := rqParseTriggerBody(data, id)
	if why != "" {
		return rqTrigger{}, fmt.Errorf("unexpected GET %s response: %s; got %s", path, why, rqDescribeJSON(data))
	}
	return t, nil
}

// rqParseTriggerBody reads a RuleTrigger response body (bare, or wrapped as
// {"ruleTrigger": {...}}) and checks it is the trigger requested. It returns
// why not, without copying any value from the body (the returned id included).
func rqParseTriggerBody(data json.RawMessage, id string) (rqTrigger, string) {
	var obj map[string]json.RawMessage
	if !rqIsJSONObject(data) || json.Unmarshal(data, &obj) != nil {
		return rqTrigger{}, "not a rule trigger object"
	}
	raw := json.RawMessage(data)
	if wrapped, ok := obj["ruleTrigger"]; ok {
		if !rqIsJSONObject(wrapped) {
			return rqTrigger{}, "ruleTrigger is not an object"
		}
		// The wrapper is an envelope: every clause of the failure rule applies.
		if why := rqFailureIndicator(obj, false); why != "" {
			return rqTrigger{}, "the response carries " + why
		}
		raw = wrapped
	}
	// On the trigger object itself status is data; the rest of the rule holds.
	var inner map[string]json.RawMessage
	if json.Unmarshal(raw, &inner) == nil {
		if why := rqFailureIndicator(inner, true); why != "" {
			return rqTrigger{}, "the rule trigger carries " + why
		}
	}
	var t rqTrigger
	if err := json.Unmarshal(raw, &t); err != nil {
		return rqTrigger{}, "the rule trigger did not decode"
	}
	if t.ID == "" {
		return rqTrigger{}, "no rule trigger id"
	}
	if t.ID != id {
		return rqTrigger{}, "trigger id mismatch (the body is a different rule trigger)"
	}
	// The status is shown and compared downstream, so only an enum member is
	// accepted; anything else (missing, or a value that is not a status) makes
	// the body invalid and is never printed.
	if !rqContainsFold(rqValidStatuses, t.Status) {
		return rqTrigger{}, "the rule trigger status is missing or not a RuleTrigger status"
	}
	t.Status = strings.ToUpper(t.Status)
	return t, ""
}

// rqLoadLocal reads every synced rule trigger from the generic resources table.
func rqLoadLocal(ctx context.Context, db *store.Store) ([]rqTrigger, int, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = 'rule-triggers'`)
	if err != nil {
		return nil, 0, err
	}
	raws := make([]json.RawMessage, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		raws = append(raws, json.RawMessage(data))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	_ = rows.Close()
	triggers, bad := rqDecodeTriggers(raws)
	return triggers, bad, nil
}

// rqLiveUnavailable reports whether a live read failed in a way that makes the
// local mirror the honest answer: no network, or the API itself is down (5xx).
// Auth and request errors (4xx) are surfaced, not papered over.
func rqLiveUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if isNetworkError(err) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 500 {
		return true
	}
	return false
}

// rqHumanLeft renders a time_left value for tables.
func rqHumanLeft(left *int64) string {
	if left == nil {
		return "-"
	}
	d := time.Duration(*left) * time.Second
	neg := d < 0
	if neg {
		d = -d
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := fmt.Sprintf("%dh%02dm", h, m)
	if h == 0 {
		s = fmt.Sprintf("%dm", m)
	}
	if neg {
		return "expired " + s + " ago"
	}
	return s
}

// rqSupersessionKey scopes "the rule triggers again" to one rule firing for one
// organization and the same accounts (see rqAnnotate).
func rqSupersessionKey(t rqTrigger) string {
	accounts := append([]string(nil), t.accountEmails()...)
	sort.Strings(accounts)
	return t.Rule.ID + "\x00" + t.Organization.ID + "\x00" + strings.Join(accounts, ",")
}
