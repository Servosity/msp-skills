// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package fleet holds the pure aggregation logic behind the ConnectWise Automate
// CLI's transcendence commands (fleet-health, stale-agents, patch-compliance,
// client-rollup, alert-triage, os-inventory, since). Each function takes already
// decoded JSON objects (as returned by the Automate API and stored verbatim in
// the local SQLite mirror) and returns typed, sorted results. Keeping the logic
// here — free of cobra, the store, and IO — makes it directly table-testable.
//
// Field access is deliberately tolerant. The Automate API's exact field shapes
// vary by server version (a computer may carry a flat "ClientName" or a nested
// "Client":{"Id","Name"} object), so accessors try several candidate keys and
// fall back to joining on IDs against the synced clients/locations tables.
package fleet

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---- field accessors (tolerant of PascalCase / camelCase / json.Number) ----

// getRaw returns the first present value among candidate keys (exact match).
func getRaw(obj map[string]any, candidates ...string) any {
	for _, k := range candidates {
		if v, ok := obj[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// Str returns a string value for the first present candidate key.
func Str(obj map[string]any, candidates ...string) string {
	switch v := getRaw(obj, candidates...).(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// Int returns an int value for the first present candidate key, parsing
// json.Number, float64, int, and numeric strings. Returns 0 when absent or
// unparseable (the documented "absence reads as zero" contract).
func Int(obj map[string]any, candidates ...string) int {
	switch v := getRaw(obj, candidates...).(type) {
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
		if f, err := v.Float64(); err == nil {
			return int(f)
		}
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int(f)
		}
	}
	return 0
}

// nestedName extracts a "Name" from a nested object value (e.g. a computer's
// "Client":{"Id":1,"Name":"Acme"}). Returns "" when the value is not an object
// or carries no name.
func nestedName(obj map[string]any, key string) string {
	if v, ok := obj[key].(map[string]any); ok {
		return Str(v, "Name", "name")
	}
	return ""
}

var msDateRe = regexp.MustCompile(`/Date\((\d+)`)

// ParseTime parses the date shapes ConnectWise Automate emits: ISO 8601 (with
// or without a timezone), space-separated SQL datetimes, and the legacy
// Microsoft `/Date(ms)/` form. Returns ok=false when the value is empty or
// unrecognized so callers never treat a parse miss as a real timestamp.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if m := msDateRe.FindStringSubmatch(s); m != nil {
		if ms, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return time.UnixMilli(ms).UTC(), true
		}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

const onlineStatus = "online"

func isOnline(obj map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(Str(obj, "Status", "status")), onlineStatus)
}

// clientNameIndex maps a client Id (as string) to its Name from the clients list.
func clientNameIndex(clients []map[string]any) map[string]string {
	idx := make(map[string]string, len(clients))
	for _, c := range clients {
		id := Str(c, "Id", "id", "ClientId", "clientId")
		name := Str(c, "Name", "name", "ClientName")
		if id != "" {
			idx[id] = name
		}
	}
	return idx
}

// computerClient resolves the owning client name for a computer, trying a flat
// field, a nested Client object, then a join against the clients index.
func computerClient(c map[string]any, clientIdx map[string]string) string {
	if n := Str(c, "ClientName", "clientName"); n != "" {
		return n
	}
	if n := nestedName(c, "Client"); n != "" {
		return n
	}
	if id := Str(c, "ClientId", "clientId"); id != "" {
		if n, ok := clientIdx[id]; ok && n != "" {
			return n
		}
		return "Client " + id
	}
	return "(unassigned)"
}

// ---- fleet-health ----

type ClientHealth struct {
	Client     string `json:"client"`
	Computers  int    `json:"computers"`
	Online     int    `json:"online"`
	Offline    int    `json:"offline"`
	Stale      int    `json:"stale"`
	OpenAlerts int    `json:"open_alerts"`
}

// FleetHealth rolls computers up per client: total, online, offline, stale
// (last-contact older than staleDays), and open-alert count. Rows are sorted by
// offline desc, then client name. now anchors the staleness math (injected for
// testability).
func FleetHealth(computers, clients, alerts []map[string]any, staleDays int, now time.Time) []ClientHealth {
	clientIdx := clientNameIndex(clients)
	alertsByClient := alertCountByClient(alerts, computers, clientIdx)

	agg := map[string]*ClientHealth{}
	staleCutoff := now.AddDate(0, 0, -staleDays)
	for _, c := range computers {
		name := computerClient(c, clientIdx)
		h := agg[name]
		if h == nil {
			h = &ClientHealth{Client: name}
			agg[name] = h
		}
		h.Computers++
		if isOnline(c) {
			h.Online++
		} else {
			h.Offline++
		}
		if t, ok := ParseTime(Str(c, "LastContact", "lastContact")); ok && t.Before(staleCutoff) {
			h.Stale++
		}
	}
	out := make([]ClientHealth, 0, len(agg))
	for name, h := range agg {
		h.OpenAlerts = alertsByClient[name]
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Offline != out[j].Offline {
			return out[i].Offline > out[j].Offline
		}
		return out[i].Client < out[j].Client
	})
	return out
}

// alertCountByClient buckets open alerts by client, preferring an explicit
// ClientName on the alert and falling back to a computer join.
func alertCountByClient(alerts, computers []map[string]any, clientIdx map[string]string) map[string]int {
	compClient := map[string]string{}
	for _, c := range computers {
		if id := Str(c, "Id", "id", "ComputerId", "computerId"); id != "" {
			compClient[id] = computerClient(c, clientIdx)
		}
	}
	counts := map[string]int{}
	for _, a := range alerts {
		name := Str(a, "ClientName", "clientName")
		if name == "" {
			if cid := Str(a, "ComputerId", "computerId"); cid != "" {
				name = compClient[cid]
			}
		}
		if name == "" {
			name = "(unassigned)"
		}
		counts[name]++
	}
	return counts
}

// ---- stale-agents ----

type StaleAgent struct {
	Computer    string `json:"computer"`
	Client      string `json:"client"`
	LastContact string `json:"last_contact"`
	DaysStale   int    `json:"days_stale"`
}

// StaleAgents lists computers whose last contact is older than days (or which
// have no parseable last-contact at all), sorted most-stale first. now anchors
// the age math.
func StaleAgents(computers, clients []map[string]any, days int, now time.Time) []StaleAgent {
	clientIdx := clientNameIndex(clients)
	cutoff := now.AddDate(0, 0, -days)
	out := []StaleAgent{}
	for _, c := range computers {
		last := Str(c, "LastContact", "lastContact")
		t, ok := ParseTime(last)
		switch {
		case !ok:
			out = append(out, StaleAgent{
				Computer:    Str(c, "ComputerName", "computerName", "Name", "name"),
				Client:      computerClient(c, clientIdx),
				LastContact: "never",
				DaysStale:   -1,
			})
		case t.Before(cutoff):
			out = append(out, StaleAgent{
				Computer:    Str(c, "ComputerName", "computerName", "Name", "name"),
				Client:      computerClient(c, clientIdx),
				LastContact: last,
				DaysStale:   int(now.Sub(t).Hours() / 24),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysStale != out[j].DaysStale {
			return out[i].DaysStale > out[j].DaysStale
		}
		return out[i].Computer < out[j].Computer
	})
	return out
}

// ---- patch-compliance ----

type ClientPatch struct {
	Client        string  `json:"client"`
	Installed     int     `json:"installed"`
	Failed        int     `json:"failed"`
	Pending       int     `json:"pending"`
	Total         int     `json:"total"`
	CompliancePct float64 `json:"compliance_pct"`
}

// PatchCompliance groups patch-history events by client (joined computer ->
// client) and computes a per-client compliance percentage (installed / total),
// worst compliance first. Patch status strings are matched case-insensitively
// against installed / failed / pending buckets; anything else still counts
// toward the total denominator.
func PatchCompliance(patchHistory, computers, clients []map[string]any) []ClientPatch {
	clientIdx := clientNameIndex(clients)
	compClient := map[string]string{}
	for _, c := range computers {
		if id := Str(c, "Id", "id", "ComputerId", "computerId"); id != "" {
			compClient[id] = computerClient(c, clientIdx)
		}
	}
	agg := map[string]*ClientPatch{}
	for _, p := range patchHistory {
		name := Str(p, "ClientName", "clientName")
		if name == "" {
			if cid := Str(p, "ComputerId", "computerId"); cid != "" {
				name = compClient[cid]
			}
		}
		if name == "" {
			name = "(unassigned)"
		}
		cp := agg[name]
		if cp == nil {
			cp = &ClientPatch{Client: name}
			agg[name] = cp
		}
		cp.Total++
		switch strings.ToLower(strings.TrimSpace(Str(p, "Status", "status"))) {
		case "installed", "success", "succeeded", "complete", "completed":
			cp.Installed++
		case "failed", "error", "failure":
			cp.Failed++
		case "pending", "notinstalled", "not installed", "approved", "downloading":
			cp.Pending++
		}
	}
	out := make([]ClientPatch, 0, len(agg))
	for _, cp := range agg {
		if cp.Total > 0 {
			cp.CompliancePct = round1(float64(cp.Installed) / float64(cp.Total) * 100)
		}
		out = append(out, *cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CompliancePct != out[j].CompliancePct {
			return out[i].CompliancePct < out[j].CompliancePct
		}
		return out[i].Client < out[j].Client
	})
	return out
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}

// ---- client-rollup ----

type ClientRollup struct {
	Client     string `json:"client"`
	Computers  int    `json:"computers"`
	Locations  int    `json:"locations"`
	Offline    int    `json:"offline"`
	OpenAlerts int    `json:"open_alerts"`
}

// ClientRollups produces a one-row-per-client snapshot of computers, locations,
// offline agents, and open alerts. When filter is non-empty, only clients whose
// name contains it (case-insensitive) are returned. Sorted by client name.
func ClientRollups(computers, clients, locations, alerts []map[string]any, filter string) []ClientRollup {
	clientIdx := clientNameIndex(clients)
	alertsByClient := alertCountByClient(alerts, computers, clientIdx)

	agg := map[string]*ClientRollup{}
	ensure := func(name string) *ClientRollup {
		r := agg[name]
		if r == nil {
			r = &ClientRollup{Client: name}
			agg[name] = r
		}
		return r
	}
	// Seed from the clients list so clients with zero computers still appear.
	for _, c := range clients {
		if n := Str(c, "Name", "name"); n != "" {
			ensure(n)
		}
	}
	for _, c := range computers {
		r := ensure(computerClient(c, clientIdx))
		r.Computers++
		if !isOnline(c) {
			r.Offline++
		}
	}
	for _, l := range locations {
		name := Str(l, "ClientName", "clientName")
		if name == "" {
			if id := Str(l, "ClientId", "clientId"); id != "" {
				if n, ok := clientIdx[id]; ok {
					name = n
				}
			}
		}
		if name == "" {
			name = "(unassigned)"
		}
		ensure(name).Locations++
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]ClientRollup, 0, len(agg))
	for name, r := range agg {
		if filter != "" && !strings.Contains(strings.ToLower(name), filter) {
			continue
		}
		r.OpenAlerts = alertsByClient[name]
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Client < out[j].Client })
	return out
}

// ---- alert-triage ----

type TriagedAlert struct {
	Priority int    `json:"priority"`
	Client   string `json:"client"`
	Location string `json:"location"`
	Computer string `json:"computer"`
	Message  string `json:"message"`
	AlertID  string `json:"alert_id"`
}

// AlertTriage filters open alerts to those at or above minPriority, resolves
// client/location/computer context (explicit fields, else a computer join),
// collapses duplicate (computer, message) pairs keeping the highest priority,
// and sorts by priority desc then client.
func AlertTriage(alerts, computers, clients []map[string]any, minPriority int) []TriagedAlert {
	clientIdx := clientNameIndex(clients)
	type comp struct{ client, location string }
	compIdx := map[string]comp{}
	for _, c := range computers {
		if id := Str(c, "Id", "id", "ComputerId", "computerId"); id != "" {
			compIdx[id] = comp{
				client:   computerClient(c, clientIdx),
				location: Str(c, "LocationName", "locationName"),
			}
		}
	}
	dedup := map[string]TriagedAlert{}
	for _, a := range alerts {
		prio := Int(a, "Priority", "priority")
		if prio < minPriority {
			continue
		}
		cid := Str(a, "ComputerId", "computerId")
		client := Str(a, "ClientName", "clientName")
		location := Str(a, "LocationName", "locationName")
		computer := Str(a, "ComputerName", "computerName")
		if ci, ok := compIdx[cid]; ok {
			if client == "" {
				client = ci.client
			}
			if location == "" {
				location = ci.location
			}
		}
		msg := Str(a, "Message", "message")
		ta := TriagedAlert{
			Priority: prio,
			Client:   client,
			Location: location,
			Computer: computer,
			Message:  msg,
			AlertID:  Str(a, "Id", "id", "AlertId", "alertId"),
		}
		key := cid + "\x00" + msg
		if prev, ok := dedup[key]; !ok || ta.Priority > prev.Priority {
			dedup[key] = ta
		}
	}
	out := make([]TriagedAlert, 0, len(dedup))
	for _, ta := range dedup {
		out = append(out, ta)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if out[i].Client != out[j].Client {
			return out[i].Client < out[j].Client
		}
		return out[i].Computer < out[j].Computer
	})
	return out
}

// ---- os-inventory ----

type OSGroup struct {
	OS    string `json:"os"`
	Count int    `json:"count"`
	EOL   bool   `json:"eol"`
}

var eolNeedles = []string{
	"windows 7", "windows xp", "windows vista", "windows 8", "windows 2000",
	"server 2003", "server 2008", "server 2012",
}

// IsEOL reports whether an OS name names a known end-of-life Windows release.
func IsEOL(os string) bool {
	l := strings.ToLower(os)
	for _, n := range eolNeedles {
		if strings.Contains(l, n) {
			return true
		}
	}
	return false
}

// OSInventory groups computers by operating-system name with counts and an EOL
// flag, sorted by count desc. When eolOnly is set, only EOL groups are returned.
func OSInventory(computers []map[string]any, eolOnly bool) []OSGroup {
	counts := map[string]int{}
	for _, c := range computers {
		os := strings.TrimSpace(Str(c, "OperatingSystemName", "operatingSystemName", "OS", "os"))
		if os == "" {
			os = "(unknown)"
		}
		counts[os]++
	}
	out := make([]OSGroup, 0, len(counts))
	for os, n := range counts {
		eol := IsEOL(os)
		if eolOnly && !eol {
			continue
		}
		out = append(out, OSGroup{OS: os, Count: n, EOL: eol})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].OS < out[j].OS
	})
	return out
}

// ---- since ----

type SinceReport struct {
	Hours            int            `json:"hours"`
	NewAlerts        []TriagedAlert `json:"new_alerts"`
	AgentsCheckedIn  int            `json:"agents_checked_in"`
	PatchesInstalled int            `json:"patches_installed"`
}

// Since reports fleet activity within the last `hours`, derived from the
// records' own timestamps (no fabricated drift): alerts created in the window,
// the count of agents that checked in (LastContact within the window), and the
// count of patches installed in the window. now anchors the window.
func Since(alerts, computers, patchHistory, clients []map[string]any, hours int, now time.Time) SinceReport {
	cutoff := now.Add(-time.Duration(hours) * time.Hour)
	rep := SinceReport{Hours: hours, NewAlerts: []TriagedAlert{}}

	clientIdx := clientNameIndex(clients)
	compIdx := map[string]string{}
	for _, c := range computers {
		if id := Str(c, "Id", "id", "ComputerId", "computerId"); id != "" {
			compIdx[id] = computerClient(c, clientIdx)
		}
		if t, ok := ParseTime(Str(c, "LastContact", "lastContact")); ok && !t.Before(cutoff) {
			rep.AgentsCheckedIn++
		}
	}
	for _, a := range alerts {
		if t, ok := ParseTime(Str(a, "CreatedDate", "createdDate", "DateAdded", "dateAdded")); ok && !t.Before(cutoff) {
			client := Str(a, "ClientName", "clientName")
			cid := Str(a, "ComputerId", "computerId")
			if client == "" {
				client = compIdx[cid]
			}
			rep.NewAlerts = append(rep.NewAlerts, TriagedAlert{
				Priority: Int(a, "Priority", "priority"),
				Client:   client,
				Computer: Str(a, "ComputerName", "computerName"),
				Message:  Str(a, "Message", "message"),
				AlertID:  Str(a, "Id", "id", "AlertId", "alertId"),
			})
		}
	}
	sort.Slice(rep.NewAlerts, func(i, j int) bool {
		return rep.NewAlerts[i].Priority > rep.NewAlerts[j].Priority
	})
	for _, p := range patchHistory {
		if t, ok := ParseTime(Str(p, "InstallDate", "installDate")); ok && !t.Before(cutoff) {
			switch strings.ToLower(strings.TrimSpace(Str(p, "Status", "status"))) {
			case "installed", "success", "succeeded", "complete", "completed":
				rep.PatchesInstalled++
			}
		}
	}
	return rep
}
