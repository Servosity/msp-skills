// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package fleet holds the pure logic behind the `fleet` commands: parsing
// ConnectWise Asio payloads and computing cross-endpoint rollups. No IO.
//
// Asio is inconsistent about key casing across services (endpointId vs
// endpointID vs EndpointID, Availability vs availability), so every lookup
// here is case-insensitive.
package fleet

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Endpoint is one managed endpoint joined to its company and site names.
type Endpoint struct {
	ID          string `json:"endpoint_id"`
	Name        string `json:"name,omitempty"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company,omitempty"`
	SiteID      string `json:"site_id"`
	SiteName    string `json:"site,omitempty"`
	Type        string `json:"endpoint_type,omitempty"`
	OS          string `json:"os_type,omitempty"`
}

// Heartbeat is the agent availability record from the v2 heartbeat API.
type Heartbeat struct {
	Available bool      `json:"available"`
	LastSeen  time.Time `json:"last_seen"`
}

// PatchSummary is the `summary` block of GET /api/platform/v1/patching/{id}/patches.
type PatchSummary struct {
	AssessedAt    time.Time `json:"assessed_at"`
	Missing       int       `json:"missing"`
	PendingReboot int       `json:"pending_reboot"`
	Installed     int       `json:"installed"`
	Compliance    *float64  `json:"compliance,omitempty"`
	OutOfSupport  bool      `json:"out_of_support"`
	Policy        string    `json:"policy,omitempty"`
}

// User is one logged-on session from system-state-info.
type User struct {
	Username  string `json:"username"`
	Status    string `json:"status,omitempty"`
	LogonTime string `json:"logon_time,omitempty"`
	IsAdmin   bool   `json:"is_admin,omitempty"`
}

// SystemState is the subset of system-state-info the fleet commands use.
type SystemState struct {
	Name       string `json:"name,omitempty"`
	LastBoot   string `json:"last_boot,omitempty"`
	LastUser   *User  `json:"last_logged_on_user,omitempty"`
	LoggedOn   []User `json:"logged_on_users"`
	EndpointID string `json:"endpoint_id,omitempty"`
	// SessionsKnown is true only when the payload carried a loggedOnUsers
	// array (possibly empty); absent or malformed means sessions are unknown.
	SessionsKnown bool `json:"sessions_known"`
}

// --- case-insensitive JSON access ---

func field(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	for mk, v := range m {
		for _, k := range keys {
			if strings.EqualFold(mk, k) {
				return v
			}
		}
	}
	return nil
}

// Str returns a string field, case-insensitively, trying keys in order.
func Str(m map[string]any, keys ...string) string {
	switch v := field(m, keys...).(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func num(m map[string]any, keys ...string) (float64, bool) {
	switch v := field(m, keys...).(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

func boolean(m map[string]any, keys ...string) bool {
	switch v := field(m, keys...).(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	}
	return false
}

func obj(m map[string]any, keys ...string) map[string]any {
	o, _ := field(m, keys...).(map[string]any)
	return o
}

func list(m map[string]any, keys ...string) []any {
	l, _ := field(m, keys...).([]any)
	return l
}

// ParseTime accepts RFC3339 with or without fractional seconds.
func ParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// ObjectsOK is Objects plus whether the payload had the expected shape (a
// bare array, or an object carrying one of keys as an array).
func ObjectsOK(raw json.RawMessage, keys ...string) ([]map[string]any, bool) {
	var items []any
	if json.Unmarshal(raw, &items) != nil || items == nil {
		m := Object(raw)
		if m == nil {
			return nil, false
		}
		var ok bool
		if items, ok = field(m, keys...).([]any); !ok {
			return nil, false
		}
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		o, ok := it.(map[string]any)
		if !ok {
			return nil, false // an array of non-objects is not a readable payload
		}
		out = append(out, o)
	}
	return out, true
}

// Objects decodes a payload that is either a bare array or an object that
// wraps the array under one of the given keys (e.g. {"endpoints": [...]}).
func Objects(raw json.RawMessage, keys ...string) []map[string]any {
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := make([]map[string]any, 0)
	for _, item := range list(m, keys...) {
		if o, ok := item.(map[string]any); ok {
			out = append(out, o)
		}
	}
	return out
}

// Object decodes a JSON object, returning nil on anything else.
func Object(raw json.RawMessage) map[string]any {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// --- parsers ---

// ParseEndpoint reads one endpoint list item or detail payload.
func ParseEndpoint(m map[string]any) Endpoint {
	return Endpoint{
		ID:        Str(m, "endpointId", "endpointID"),
		Name:      Str(m, "friendlyName", "deviceName", "name"),
		CompanyID: Str(m, "clientId", "clientID", "companyId"),
		SiteID:    Str(m, "siteId", "siteID"),
		Type:      Str(m, "endpointType"),
		OS:        Str(m, "osType"),
	}
}

// ParseHeartbeats reads the v2 heartbeat response into availability keyed by
// endpoint ID, plus the per-endpoint errors from failedRecords (Not Found,
// Forbidden). A failed record means availability is unknown, not offline.
func ParseHeartbeats(raw json.RawMessage) (map[string]Heartbeat, map[string]string, bool) {
	out, failed := map[string]Heartbeat{}, map[string]string{}
	m := Object(raw)
	_, okS := field(m, "successfulRecords").([]any)
	_, okF := field(m, "failedRecords").([]any)
	if m == nil || (!okS && !okF) {
		return out, failed, false
	}
	for _, rec := range list(m, "failedRecords") {
		r, _ := rec.(map[string]any)
		if id := Str(r, "resource"); id != "" {
			failed[id] = firstNonEmpty(Str(r, "error"), "failed")
		}
	}
	for _, rec := range list(m, "successfulRecords") {
		r, _ := rec.(map[string]any)
		for _, e := range list(r, "endpoints") {
			ep, _ := e.(map[string]any)
			id := Str(ep, "endpointID", "endpointId")
			if id == "" {
				continue
			}
			out[id] = Heartbeat{
				Available: boolean(ep, "Availability", "availability"),
				LastSeen:  ParseTime(Str(ep, "DcDateTimeUTC", "dcDateTimeUTC")),
			}
		}
	}
	return out, failed, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ParsePatchSummary reads the `summary` block of the endpoint patches response.
func ParsePatchSummary(raw json.RawMessage) (PatchSummary, bool) {
	m := Object(raw)
	s := obj(m, "summary")
	if s == nil {
		return PatchSummary{}, false
	}
	ps := PatchSummary{
		AssessedAt:   ParseTime(Str(s, "assessedAt")),
		OutOfSupport: boolean(s, "outOfSupport"),
		Policy:       Str(s, "policyName"),
	}
	if v, ok := num(s, "missingCount"); ok {
		ps.Missing = int(v)
	}
	if v, ok := num(s, "pendingRebootCount"); ok {
		ps.PendingReboot = int(v)
	}
	if v, ok := num(s, "installedCount"); ok {
		ps.Installed = int(v)
	}
	if v, ok := num(s, "compliance"); ok {
		ps.Compliance = &v
	}
	return ps, true
}

func parseUser(m map[string]any) User {
	return User{
		Username:  Str(m, "username", "userName"),
		Status:    Str(m, "status"),
		LogonTime: Str(m, "logonTime"),
		IsAdmin:   boolean(m, "isAdmin"),
	}
}

// ParseSystemState reads system-state-info. ok is false when the payload is
// not a JSON object (null, error text), so callers never mistake an unreadable
// state for "nobody logged on".
func ParseSystemState(raw json.RawMessage) (st SystemState, ok bool) {
	m := Object(raw)
	st = SystemState{LoggedOn: make([]User, 0)}
	if m == nil {
		return st, false
	}
	st.Name = Str(m, "name")
	st.EndpointID = Str(m, "endpointID", "endpointId")
	if b := obj(m, "startupStatus"); b != nil {
		st.LastBoot = Str(b, "lastBootUpTimeUTC")
	} else {
		st.LastBoot = Str(m, "startupStatus")
	}
	if u := obj(m, "lastLoggedOnUser"); u != nil {
		lu := parseUser(u)
		if lu.Username != "" {
			st.LastUser = &lu
		}
	}
	_, st.SessionsKnown = field(m, "loggedOnUsers").([]any)
	for _, it := range list(m, "loggedOnUsers") {
		if u, ok := it.(map[string]any); ok {
			if pu := parseUser(u); pu.Username != "" {
				st.LoggedOn = append(st.LoggedOn, pu)
			}
		}
	}
	return st, true
}

// --- rollups ---

// StaleRow is one silent or offline endpoint.
type StaleRow struct {
	Endpoint
	Available   bool    `json:"available"`
	LastSeen    string  `json:"last_seen,omitempty"`
	SilentHours float64 `json:"silent_hours"`
	NoHeartbeat bool    `json:"no_heartbeat,omitempty"`
}

// Stale returns endpoints that are offline, silent longer than olderThan, or
// missing from the heartbeat response, ordered by company then silence.
func Stale(eps []Endpoint, hb map[string]Heartbeat, now time.Time, olderThan time.Duration) []StaleRow {
	rows := make([]StaleRow, 0)
	for _, e := range eps {
		h, ok := hb[e.ID]
		r := StaleRow{Endpoint: e, Available: h.Available, NoHeartbeat: !ok}
		if !h.LastSeen.IsZero() {
			r.LastSeen = h.LastSeen.Format(time.RFC3339)
			r.SilentHours = roundHours(now.Sub(h.LastSeen))
		}
		silent := !h.LastSeen.IsZero() && now.Sub(h.LastSeen) > olderThan
		if !ok || !h.Available || silent {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CompanyName != rows[j].CompanyName {
			return rows[i].CompanyName < rows[j].CompanyName
		}
		return rows[i].SilentHours > rows[j].SilentHours
	})
	return rows
}

func roundHours(d time.Duration) float64 {
	return float64(int64(d.Hours()*10)) / 10
}

// PostureRow is one company's patch-risk rollup.
type PostureRow struct {
	CompanyID          string   `json:"company_id"`
	Company            string   `json:"company,omitempty"`
	Endpoints          int      `json:"endpoints"`
	Assessed           int      `json:"assessed"`
	BelowCompliance    int      `json:"below_compliance"`
	MissingPatches     int      `json:"missing_patches"`
	PendingRebootHosts int      `json:"pending_reboot_endpoints"`
	OutOfSupport       int      `json:"out_of_support"`
	StaleAssessments   int      `json:"stale_assessments"`
	LowestCompliance   *float64 `json:"lowest_compliance,omitempty"`
	RiskScore          int      `json:"risk_score"`
}

// EndpointPosture is one endpoint's patch row for the --company drilldown.
type EndpointPosture struct {
	Endpoint
	PatchSummary
	BelowCompliance bool `json:"below_compliance"`
	StaleAssessment bool `json:"stale_assessment"`
}

// Posture rolls patch summaries up per company. Endpoints without a summary
// count toward Endpoints but not Assessed, so a fetch failure never looks
// like a clean endpoint.
func Posture(eps []Endpoint, sums map[string]PatchSummary, minCompliance float64, staleAfter time.Duration, now time.Time) ([]PostureRow, []EndpointPosture) {
	byCo := map[string]*PostureRow{}
	order := make([]string, 0)
	detail := make([]EndpointPosture, 0)
	for _, e := range eps {
		row, ok := byCo[e.CompanyID]
		if !ok {
			row = &PostureRow{CompanyID: e.CompanyID, Company: e.CompanyName}
			byCo[e.CompanyID] = row
			order = append(order, e.CompanyID)
		}
		row.Endpoints++
		s, ok := sums[e.ID]
		if !ok {
			continue
		}
		row.Assessed++
		ep := EndpointPosture{Endpoint: e, PatchSummary: s}
		if s.Compliance != nil {
			c := *s.Compliance
			if c < minCompliance {
				ep.BelowCompliance = true
				row.BelowCompliance++
			}
			if row.LowestCompliance == nil || c < *row.LowestCompliance {
				row.LowestCompliance = &c
			}
		}
		row.MissingPatches += s.Missing
		if s.PendingReboot > 0 {
			row.PendingRebootHosts++
		}
		if s.OutOfSupport {
			row.OutOfSupport++
		}
		if s.AssessedAt.IsZero() || now.Sub(s.AssessedAt) > staleAfter {
			ep.StaleAssessment = true
			row.StaleAssessments++
		}
		detail = append(detail, ep)
	}
	rows := make([]PostureRow, 0, len(order))
	for _, id := range order {
		r := byCo[id]
		r.RiskScore = r.BelowCompliance + r.PendingRebootHosts + r.OutOfSupport + r.StaleAssessments
		rows = append(rows, *r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].RiskScore != rows[j].RiskScore {
			return rows[i].RiskScore > rows[j].RiskScore
		}
		return rows[i].Company < rows[j].Company
	})
	sort.SliceStable(detail, func(i, j int) bool {
		return complianceOf(detail[i].Compliance) < complianceOf(detail[j].Compliance)
	})
	return rows, detail
}

func complianceOf(c *float64) float64 {
	if c == nil {
		return 101
	}
	return *c
}

// Reboot buckets.
const (
	RebootNow  = "reboot_now"
	Coordinate = "coordinate"
	Offline    = "offline"
	Unknown    = "availability_unknown"
)

// RebootRow is one endpoint with pending reboots.
type RebootRow struct {
	Endpoint
	Bucket        string `json:"bucket"`
	PendingReboot int    `json:"pending_reboot"`
	LoggedOn      []User `json:"logged_on_users"`
	LastSeen      string `json:"last_seen,omitempty"`
}

// RebootTriage buckets endpoints with pending reboots. An endpoint whose
// availability could not be read (unknown) is availability_unknown; one
// missing from a successful heartbeat counts as offline; an online endpoint
// whose system state could not be read is "coordinate", never "reboot_now".
func RebootTriage(eps []Endpoint, sums map[string]PatchSummary, hb map[string]Heartbeat, unknown map[string]bool, states map[string]SystemState) []RebootRow {
	rows := make([]RebootRow, 0)
	for _, e := range eps {
		s, ok := sums[e.ID]
		if !ok || s.PendingReboot <= 0 {
			continue
		}
		r := RebootRow{Endpoint: e, PendingReboot: s.PendingReboot, LoggedOn: make([]User, 0)}
		h, hok := hb[e.ID]
		if !h.LastSeen.IsZero() {
			r.LastSeen = h.LastSeen.Format(time.RFC3339)
		}
		st, sok := states[e.ID]
		sok = sok && st.SessionsKnown
		switch {
		case unknown[e.ID]:
			r.Bucket = Unknown
		case !hok || !h.Available:
			r.Bucket = Offline
		case !sok:
			r.Bucket = Coordinate
		default:
			r.LoggedOn = st.LoggedOn
			if active := activeUsers(st.LoggedOn); len(active) > 0 {
				r.Bucket = Coordinate
				r.LoggedOn = active
			} else {
				r.Bucket = RebootNow
			}
		}
		rows = append(rows, r)
	}
	rank := map[string]int{RebootNow: 0, Coordinate: 1, Offline: 2, Unknown: 3}
	sort.SliceStable(rows, func(i, j int) bool { return rank[rows[i].Bucket] < rank[rows[j].Bucket] })
	return rows
}

// activeUsers drops service sessions with no username and sessions that are
// disconnected, so a console-less server does not block a reboot.
func activeUsers(us []User) []User {
	out := make([]User, 0)
	for _, u := range us {
		if u.Username == "" || strings.EqualFold(u.Status, "Disconnected") {
			continue
		}
		out = append(out, u)
	}
	return out
}

// WhoisRow is one endpoint where the user is or was last logged on.
type WhoisRow struct {
	Endpoint
	Match     string `json:"match"` // current | last
	Username  string `json:"username"`
	Status    string `json:"status,omitempty"`
	LogonTime string `json:"logon_time,omitempty"`
}

// Whois finds endpoints whose current or last user contains query
// (case-insensitive). Current sessions sort before last-logon matches.
func Whois(eps []Endpoint, states map[string]SystemState, query string) []WhoisRow {
	q := strings.ToLower(strings.TrimSpace(query))
	rows := make([]WhoisRow, 0)
	if q == "" {
		return rows
	}
	for _, e := range eps {
		st, ok := states[e.ID]
		if !ok {
			continue
		}
		matched := false
		for _, u := range st.LoggedOn {
			if strings.Contains(strings.ToLower(u.Username), q) {
				rows = append(rows, WhoisRow{Endpoint: e, Match: "current", Username: u.Username, Status: u.Status, LogonTime: u.LogonTime})
				matched = true
			}
		}
		if !matched && st.LastUser != nil && strings.Contains(strings.ToLower(st.LastUser.Username), q) {
			rows = append(rows, WhoisRow{Endpoint: e, Match: "last", Username: st.LastUser.Username, Status: st.LastUser.Status, LogonTime: st.LastUser.LogonTime})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Match != rows[j].Match {
			return rows[i].Match == "current"
		}
		return rows[i].LogonTime > rows[j].LogonTime
	})
	return rows
}

// TaskResult is one per-endpoint row of a task instance's results.
type TaskResult struct {
	EndpointID string `json:"endpoint_id"`
	Status     string `json:"completion_status"`
	Error      string `json:"error_details,omitempty"`
	Result     string `json:"result_details,omitempty"`
	InstanceID string `json:"task_instance_id,omitempty"`
}

// ParseTaskResults reads GET .../instances/{id}/results ({Result: [...], Count}).
// ok is false when the payload carries no result array.
func ParseTaskResults(raw json.RawMessage) ([]TaskResult, bool) {
	out := make([]TaskResult, 0)
	items, ok := ObjectsOK(raw, "Result", "results")
	for _, m := range items {
		if Str(m, "endpointId", "endpointID") == "" {
			ok = false // a result row that names no endpoint is unusable
		}
		out = append(out, TaskResult{
			EndpointID: Str(m, "endpointId", "endpointID"),
			Status:     Str(m, "completionStatus"),
			Error:      Str(m, "errorDetails"),
			Result:     Str(m, "resultDetails"),
			InstanceID: Str(m, "taskInstanceId", "taskInstanceID"),
		})
	}
	return out, ok
}

// TaskInstance is one row of GET .../instances/summary ({Results: [...], Count}).
type TaskInstance struct {
	ID            string `json:"id"`
	OverallStatus string `json:"overall_status,omitempty"`
	ExecutedOn    string `json:"executed_on,omitempty"`
	CreatedOn     string `json:"created_on,omitempty"`
	FailedCount   int    `json:"failed_count"`
	SuccessCount  int    `json:"success_count"`
	DeviceCount   int    `json:"device_count"`
	Pending       int    `json:"pending_count"`
}

// ParseTaskInstances reads the instance summary, newest first. ok is false
// when the payload carries no instance array.
func ParseTaskInstances(raw json.RawMessage) ([]TaskInstance, bool) {
	out := make([]TaskInstance, 0)
	items, ok := ObjectsOK(raw, "Results", "results", "TaskInstances")
	for _, m := range items {
		if Str(m, "Id", "taskInstanceId", "instanceId") == "" {
			ok = false // an instance without an ID cannot be followed to its results
		}
		ti := TaskInstance{
			ID:            Str(m, "Id", "taskInstanceId", "instanceId"),
			OverallStatus: Str(m, "OverallStatus", "Status"),
			ExecutedOn:    Str(m, "ExecutedOn"),
			CreatedOn:     Str(m, "CreatedOn"),
		}
		if v, ok := num(m, "FailedCount"); ok {
			ti.FailedCount = int(v)
		}
		if v, ok := num(m, "SuccessCount"); ok {
			ti.SuccessCount = int(v)
		}
		if v, ok := num(m, "DeviceCount", "Count"); ok {
			ti.DeviceCount = int(v)
		}
		for _, k := range []string{"ScheduledCount", "RunningCount", "WaitingCount", "PostponedCount"} {
			if v, ok := num(m, k); ok {
				ti.Pending += int(v)
			}
		}
		if ti.ID != "" {
			out = append(out, ti)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return instanceTime(out[i]).After(instanceTime(out[j])) })
	return out, ok
}

func instanceTime(t TaskInstance) time.Time {
	if ts := ParseTime(t.ExecutedOn); !ts.IsZero() {
		return ts
	}
	return ParseTime(t.CreatedOn)
}

// Done reports whether an instance has no devices left to run.
func (t TaskInstance) Done() bool {
	s := strings.ToLower(t.OverallStatus)
	for _, p := range []string{"running", "waiting", "queued", "pending", "in_progress", "scheduled"} {
		if s == p {
			return false
		}
	}
	return t.Pending == 0
}

// Failed reports whether a per-endpoint result is a failure.
func (r TaskResult) Failed() bool {
	s := strings.ToUpper(r.Status)
	return strings.Contains(s, "FAIL") || strings.Contains(s, "ERROR") || strings.Contains(s, "TIMEOUT")
}

// FailureGroup is a set of endpoints that failed with identical error text.
type FailureGroup struct {
	Error     string     `json:"error"`
	Count     int        `json:"count"`
	Endpoints []Endpoint `json:"endpoints"`
}

// GroupFailures groups failed results by exact errorDetails text, largest
// group first. names resolves endpoint IDs; unknown IDs keep just the ID.
func GroupFailures(results []TaskResult, names map[string]Endpoint) []FailureGroup {
	idx := map[string]int{}
	groups := make([]FailureGroup, 0)
	for _, r := range results {
		if !r.Failed() {
			continue
		}
		key := strings.TrimSpace(r.Error)
		if key == "" {
			key = "(no error details; status " + r.Status + ")"
		}
		i, ok := idx[key]
		if !ok {
			i = len(groups)
			idx[key] = i
			groups = append(groups, FailureGroup{Error: key, Endpoints: make([]Endpoint, 0)})
		}
		e, ok := names[r.EndpointID]
		if !ok {
			e = Endpoint{ID: r.EndpointID}
		}
		groups[i].Endpoints = append(groups[i].Endpoints, e)
		groups[i].Count++
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	return groups
}
