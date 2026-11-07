// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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

// clientRef identifies the client a record belongs to. Every report groups by
// key (the client's Id when one is known, else the name the record carries)
// and turns keys into display labels only at output (clientLabels), so two
// clients that share a name never merge and a label never decides identity.
type clientRef struct {
	key  string // "id:<Id>", "name:<Name>", or "" (unassigned)
	id   string
	name string
}

// clientIndex is the synced clients: Id to name, and each name that exactly
// one client carries to that client's Id (a record that names only its client
// joins that client; an ambiguous name stays on its own).
type clientIndex struct {
	names    map[string]string
	uniqueID map[string]string
}

func clientNameIndex(clients []map[string]any) clientIndex {
	idx := clientIndex{names: map[string]string{}, uniqueID: map[string]string{}}
	owners := map[string]map[string]bool{}
	for _, c := range clients {
		id := Str(c, "Id", "id", "ClientId", "clientId")
		if id == "" {
			continue
		}
		name := strings.TrimSpace(Str(c, "Name", "name", "ClientName"))
		idx.names[id] = name
		if owners[name] == nil {
			owners[name] = map[string]bool{}
		}
		owners[name][id] = true
	}
	for name, ids := range owners {
		if name == "" || len(ids) != 1 {
			continue
		}
		for id := range ids {
			idx.uniqueID[name] = id
		}
	}
	return idx
}

// byID is the client with that Id, named from the clients index when it has
// a name, else from the name the record carries.
func (x clientIndex) byID(id, carried string) clientRef {
	name := x.names[id]
	if name == "" {
		name = carried
	}
	return clientRef{key: "id:" + id, id: id, name: name}
}

// byName is a client a record names without an Id: the one synced client of
// that name, else that name on its own.
func (x clientIndex) byName(name string) clientRef {
	if name == "" {
		return clientRef{}
	}
	if id, ok := x.uniqueID[name]; ok {
		return x.byID(id, name)
	}
	return clientRef{key: "name:" + name, name: name}
}

// computerClient resolves the owning client of a computer: its ClientId (or a
// nested Client object's Id), else the client name it carries.
func computerClient(c map[string]any, x clientIndex) clientRef {
	carried := strings.TrimSpace(Str(c, "ClientName", "clientName"))
	if carried == "" {
		carried = strings.TrimSpace(nestedName(c, "Client"))
	}
	id := Str(c, "ClientId", "clientId")
	if id == "" {
		if nested, ok := c["Client"].(map[string]any); ok {
			id = Str(nested, "Id", "id")
		}
	}
	if id != "" {
		return x.byID(id, carried)
	}
	return x.byName(carried)
}

// computerClientIndex maps each synced computer's Id to its client.
func computerClientIndex(computers []map[string]any, x clientIndex) map[string]clientRef {
	idx := map[string]clientRef{}
	for _, c := range computers {
		if id := Str(c, "Id", "id", "ComputerId", "computerId"); id != "" {
			idx[id] = computerClient(c, x)
		}
	}
	return idx
}

// recordClient resolves the client of a record that points at a computer or a
// client (an alert, a patch event, a location): the computer join first, then
// its ClientId, then the ClientName it carries. Every report attributes
// records through here so they all agree.
func recordClient(r map[string]any, compClient map[string]clientRef, x clientIndex) clientRef {
	if cid := Str(r, "ComputerId", "computerId"); cid != "" {
		if ref, ok := compClient[cid]; ok {
			return ref
		}
	}
	carried := strings.TrimSpace(Str(r, "ClientName", "clientName"))
	if id := Str(r, "ClientId", "clientId"); id != "" {
		return x.byID(id, carried)
	}
	return x.byName(carried)
}

// clientLabels renders one display label per client key, unique across the
// keys given and every synced client (so a client's label is the same in
// every report): a client's name, "Client <Id>" for a nameless one,
// "(unassigned)" for none. While labels collide, each colliding client with
// an Id gets " (#<Id>)" appended, so a client really named "Acme (#1)" can
// never take the label of client 1.
func clientLabels(refs map[string]clientRef, x clientIndex) map[string]string {
	all := make(map[string]clientRef, len(refs)+len(x.names))
	for id := range x.names {
		ref := x.byID(id, "")
		all[ref.key] = ref
	}
	for k, r := range refs {
		all[k] = r
	}
	label := make(map[string]string, len(all))
	for k, r := range all {
		switch {
		case r.key == "":
			label[k] = "(unassigned)"
		case r.name != "":
			label[k] = r.name
		default:
			label[k] = "Client " + r.id
		}
	}
	// ponytail: bounded rounds; each round lengthens every colliding Id label.
	for round := 0; round < 8; round++ {
		groups := map[string][]string{}
		for k, l := range label {
			groups[l] = append(groups[l], k)
		}
		changed := false
		for _, ks := range groups {
			if len(ks) < 2 {
				continue
			}
			for _, k := range ks {
				if id := all[k].id; id != "" {
					label[k] += " (#" + id + ")"
					changed = true
				} else if all[k].key != "" {
					label[k] += " (no Id)"
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return label
}

// openAlert reports whether an alert is still open: a Closed alert
// (Alert.Status) is not, whatever else the record says.
func openAlert(a map[string]any) bool {
	switch strings.ToLower(strings.TrimSpace(Str(a, "Status", "status"))) {
	case "closed", "resolved":
		return false
	}
	return true
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
	refs := map[string]clientRef{}
	staleCutoff := now.AddDate(0, 0, -staleDays)
	for _, c := range computers {
		ref := computerClient(c, clientIdx)
		refs[ref.key] = ref
		h := agg[ref.key]
		if h == nil {
			h = &ClientHealth{}
			agg[ref.key] = h
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
	labels := clientLabels(refs, clientIdx)
	out := make([]ClientHealth, 0, len(agg))
	for key, h := range agg {
		h.Client = labels[key]
		h.OpenAlerts = alertsByClient[key]
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

// alertCountByClient counts open alerts per client key (recordClient).
func alertCountByClient(alerts, computers []map[string]any, clientIdx clientIndex) map[string]int {
	compClient := computerClientIndex(computers, clientIdx)
	counts := map[string]int{}
	for _, a := range alerts {
		if openAlert(a) {
			counts[recordClient(a, compClient, clientIdx).key]++
		}
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
	refs := map[string]clientRef{}
	var keys []string
	for _, c := range computers {
		last := Str(c, "LastContact", "lastContact")
		t, ok := ParseTime(last)
		switch {
		case !ok:
			out = append(out, StaleAgent{
				Computer:    Str(c, "ComputerName", "computerName", "Name", "name"),
				LastContact: "never",
				DaysStale:   -1,
			})
		case t.Before(cutoff):
			out = append(out, StaleAgent{
				Computer:    Str(c, "ComputerName", "computerName", "Name", "name"),
				LastContact: last,
				DaysStale:   int(now.Sub(t).Hours() / 24),
			})
		default:
			continue
		}
		ref := computerClient(c, clientIdx)
		refs[ref.key] = ref
		keys = append(keys, ref.key)
	}
	labels := clientLabels(refs, clientIdx)
	for i := range out {
		out[i].Client = labels[keys[i]]
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
	compClient := computerClientIndex(computers, clientIdx)
	agg := map[string]*ClientPatch{}
	refs := map[string]clientRef{}
	for _, p := range patchHistory {
		ref := recordClient(p, compClient, clientIdx)
		refs[ref.key] = ref
		cp := agg[ref.key]
		if cp == nil {
			cp = &ClientPatch{}
			agg[ref.key] = cp
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
	labels := clientLabels(refs, clientIdx)
	out := make([]ClientPatch, 0, len(agg))
	for key, cp := range agg {
		cp.Client = labels[key]
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
	refs := map[string]clientRef{}
	ensure := func(ref clientRef) *ClientRollup {
		refs[ref.key] = ref
		r := agg[ref.key]
		if r == nil {
			r = &ClientRollup{}
			agg[ref.key] = r
		}
		return r
	}
	// Seed from the clients list so clients with zero computers still appear.
	for id := range clientIdx.names {
		ensure(clientIdx.byID(id, ""))
	}
	for _, c := range computers {
		r := ensure(computerClient(c, clientIdx))
		r.Computers++
		if !isOnline(c) {
			r.Offline++
		}
	}
	for _, l := range locations {
		ensure(recordClient(l, nil, clientIdx)).Locations++
	}
	labels := clientLabels(refs, clientIdx)
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]ClientRollup, 0, len(agg))
	for key, r := range agg {
		r.Client = labels[key]
		if filter != "" && !strings.Contains(strings.ToLower(r.Client), filter) {
			continue
		}
		r.OpenAlerts = alertsByClient[key]
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
	type comp struct {
		client         clientRef
		location, name string
	}
	compClients := computerClientIndex(computers, clientIdx)
	compIdx := map[string]comp{}
	for _, c := range computers {
		if id := Str(c, "Id", "id", "ComputerId", "computerId"); id != "" {
			compIdx[id] = comp{
				client:   computerClient(c, clientIdx),
				location: Str(c, "LocationName", "locationName"),
				name:     Str(c, "ComputerName", "computerName", "Name", "name"),
			}
		}
	}
	dedup := map[string]TriagedAlert{}
	refs := map[string]clientRef{}
	for _, a := range alerts {
		prio := Int(a, "Priority", "priority")
		if prio < minPriority || !openAlert(a) {
			continue
		}
		cid := Str(a, "ComputerId", "computerId")
		client := recordClient(a, compClients, clientIdx)
		location := Str(a, "LocationName", "locationName")
		computer := Str(a, "ComputerName", "computerName")
		if ci, ok := compIdx[cid]; ok {
			if location == "" {
				location = ci.location
			}
			if computer == "" {
				computer = ci.name
			}
		}
		msg := Str(a, "Message", "message")
		refs[client.key] = client
		ta := TriagedAlert{
			Priority: prio,
			Client:   client.key,
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
	labels := clientLabels(refs, clientIdx)
	out := make([]TriagedAlert, 0, len(dedup))
	for _, ta := range dedup {
		ta.Client = labels[ta.Client]
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
// Windows 10 left support on 2025-10-14; its LTSC 2019 and 2021 editions are
// the only ones still supported (the LTSB editions have ended).
func IsEOL(os string) bool {
	l := strings.ToLower(os)
	if strings.Contains(l, "windows 10") {
		return !strings.Contains(l, "ltsc")
	}
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
	compIdx := map[string]clientRef{}
	refs := map[string]clientRef{}
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
			client := recordClient(a, compIdx, clientIdx)
			refs[client.key] = client
			rep.NewAlerts = append(rep.NewAlerts, TriagedAlert{
				Priority: Int(a, "Priority", "priority"),
				Client:   client.key,
				Computer: Str(a, "ComputerName", "computerName"),
				Message:  Str(a, "Message", "message"),
				AlertID:  Str(a, "Id", "id", "AlertId", "alertId"),
			})
		}
	}
	labels := clientLabels(refs, clientIdx)
	for i := range rep.NewAlerts {
		rep.NewAlerts[i].Client = labels[rep.NewAlerts[i].Client]
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
