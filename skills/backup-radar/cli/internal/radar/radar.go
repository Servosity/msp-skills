// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package radar holds the pure logic behind the hand-written Backup Radar
// commands (check, unwatched, coverage diff, stale, scorecard, timeline,
// push). It joins Backup Radar API v3 rows (devices, client health) with
// API v2 rows (backups, inactive) on Backup Radar company, device and job
// names, which both generations carry verbatim.
package radar

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"strings"
	"time"
)

// IDName is the v2 {id, name} pair used for status and backupType.
type IDName struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Device is one v3 /clients/devices item (BackupDeviceData). Nullable
// strings decode to "".
type Device struct {
	DeviceID   string `json:"device_id"`
	BRDeviceID string `json:"br_device_id"`
	Client     struct {
		Name          string `json:"name"`
		BRCompanyName string `json:"br_company_name"`
		ClientID      string `json:"client_id"`
	} `json:"client"`
	DeviceName     string `json:"device_name"`
	BRDeviceName   string `json:"br_device_name"`
	BRJobName      string `json:"br_job_name"`
	Classification string `json:"br_backup_classification"`
	LastResult     []struct {
		Status string `json:"status"`
		Date   string `json:"date"`
	} `json:"last_result"`
	// History is most recent first (index 0 = yesterday, per the v3 spec).
	History []struct {
		Date      string `json:"date"`
		SLAStatus string `json:"sla_status"`
	} `json:"history"`
}

// ClientName prefers the ScalePad client name, falling back to the Backup
// Radar company name for unmapped companies.
func (d Device) ClientName() string { return firstNonEmpty(d.Client.Name, d.Client.BRCompanyName) }

// Name prefers the ScalePad device name, falling back to the Backup Radar name.
func (d Device) Name() string { return firstNonEmpty(d.DeviceName, d.BRDeviceName) }

// LastResultAt is the newest last_result date, or zero when none was received.
func (d Device) LastResultAt() time.Time {
	var newest time.Time
	for _, r := range d.LastResult {
		if t, ok := ParseTime(r.Date); ok && t.After(newest) {
			newest = t
		}
	}
	return newest
}

// Backup is one v2 /backups item (ApiBackup).
type Backup struct {
	BackupID              int64       `json:"backupId"`
	CompanyName           string      `json:"companyName"`
	DeviceName            string      `json:"deviceName"`
	DeviceType            string      `json:"deviceType"`
	JobName               string      `json:"jobName"`
	MethodName            string      `json:"methodName"`
	BackupType            IDName      `json:"backupType"`
	Status                IDName      `json:"status"`
	DaysInStatus          float64     `json:"daysInStatus"`
	LastResult            string      `json:"lastResult"`
	LastSuccess           string      `json:"lastSuccess"`
	TicketCount           int         `json:"ticketCount"`
	TreatWarningAsSuccess bool        `json:"treatWarningAsSuccess"`
	Note                  string      `json:"note"`
	Tags                  []string    `json:"tags"`
	TicketingCompany      string      `json:"ticketingCompany"`
	History               []BackupDay `json:"history"`
}

// BackupDay is one v2 history entry (ApiBackupsHistory).
type BackupDay struct {
	Status        IDName `json:"status"`
	Date          string `json:"date"`
	CountFailure  int    `json:"countFailure"`
	CountWarning  int    `json:"countWarning"`
	CountSuccess  int    `json:"countSuccess"`
	CountNoResult int    `json:"countNoResult"`
}

// Inactive is one v2 /backups/inactive item: results arriving for a backup
// nobody has activated.
type Inactive struct {
	BackupID     int64  `json:"backupId"`
	EmailFrom    string `json:"emailFrom"`
	LastReceived string `json:"lastReceived"`
	CompanyName  string `json:"companyName"`
	DeviceName   string `json:"deviceName"`
	JobName      string `json:"jobName"`
	MethodName   string `json:"methodName"`
}

// ClientHealth is one v3 /clients/health item.
type ClientHealth struct {
	Client struct {
		Name     string `json:"name"`
		ClientID string `json:"client_id"`
	} `json:"client"`
	History []struct {
		Date       string     `json:"date"`
		Compliance Compliance `json:"compliance"`
	} `json:"history"`
}

// Compliance is the v3 per-day BackupCompliance counts.
type Compliance struct {
	Pending                  int `json:"pending_count"`
	AtRisk                   int `json:"at_risk_count"`
	InCompliance             int `json:"in_compliance_count"`
	OutOfCompliance          int `json:"out_of_compliance_count"`
	OutOfComplianceNoResults int `json:"out_of_compliance_no_results_count"`
}

// Snapshot is everything synced locally.
type Snapshot struct {
	Devices  []Device
	Health   []ClientHealth
	Backups  []Backup
	Inactive []Inactive
	// Synced records which collections completed a sync attempt (or kept
	// rows from one), so reports can say "skipped" instead of "nothing found".
	Synced map[string]bool
}

// AnySynced reports whether any report collection (v3 or v2) was synced.
func (s Snapshot) AnySynced() bool {
	return s.Synced["devices"] || s.Synced["health"] || s.Synced["backups"] || s.Synced["backups-inactive"]
}

// ReasonCollection names the synced collection each unwatched reason reads.
func ReasonCollection(reason string) string {
	switch reason {
	case ReasonInactive:
		return "backups-inactive"
	case ReasonUnmapped:
		return "devices"
	default:
		return "backups"
	}
}

// Problem states, ordered by severity.
const (
	StateOK        = "ok"
	StateWarning   = "warning"
	StateAtRisk    = "at_risk"
	StateFailing   = "failing"
	StateNoResults = "no_results"
)

var severity = map[string]int{StateOK: 0, StateWarning: 1, StateAtRisk: 1, StateFailing: 2, StateNoResults: 3}

// StateFromSLA maps a v3 sla_status to a problem state.
func StateFromSLA(sla string) string {
	switch strings.ToLower(strings.TrimSpace(sla)) {
	case "out of compliance - no results":
		return StateNoResults
	case "out of compliance":
		return StateFailing
	case "at risk":
		return StateAtRisk
	default: // In Compliance, Pending, unrecorded
		return StateOK
	}
}

// StateFromV2 maps a v2 status name (Success, Warning, Failure, No Result,
// Pending per GET /backups/filters) to a problem state.
func StateFromV2(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "no result", "no results":
		return StateNoResults
	case "failure":
		return StateFailing
	case "warning":
		return StateWarning
	default:
		return StateOK
	}
}

// Key joins Backup Radar company/device/job names case-insensitively.
func Key(company, device, job string) string {
	if IsUnassigned(company) {
		company = ""
	}
	return norm(company) + "\x1f" + norm(device) + "\x1f" + norm(job)
}

// joiner assigns row identities across the two API generations. A v3 device
// and a v2 backup join only when their Backup Radar company/device/job names
// identify exactly one backup on each side; otherwise each keeps its own
// identity (br_device_id or backupId) rather than being merged with a
// different backup.
type joiner struct {
	v3Count map[string]int
	v2Count map[string]int
}

func newJoiner(s Snapshot) *joiner {
	j := &joiner{v3Count: map[string]int{}, v2Count: map[string]int{}}
	for _, d := range s.Devices {
		j.v3Count[Key(d.Client.BRCompanyName, d.BRDeviceName, d.BRJobName)]++
	}
	for _, b := range s.Backups {
		j.v2Count[Key(b.CompanyName, b.DeviceName, b.JobName)]++
	}
	return j
}

func (j *joiner) v3(d Device) string {
	k := Key(d.Client.BRCompanyName, d.BRDeviceName, d.BRJobName)
	if j.v3Count[k] > 1 {
		return k + "\x1fv3:" + d.BRDeviceID
	}
	return k
}

func (j *joiner) v2(b Backup) string {
	k := Key(b.CompanyName, b.DeviceName, b.JobName)
	if j.v3Count[k] > 1 || j.v2Count[k] > 1 {
		return k + "\x1fv2:" + fmt.Sprint(b.BackupID)
	}
	return k
}

// joined reports whether join key k pairs exactly one v3 device with exactly
// one v2 backup (the same backup seen by both generations).
func (j *joiner) joined(k string) bool { return j.v3Count[k] == 1 && j.v2Count[k] == 1 }

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// IsUnassigned reports Backup Radar's placeholder for "no company/ticketing".
func IsUnassigned(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.EqualFold(s, "[Undefined]")
}

// ParseTime accepts the v3 (RFC 3339) and v2 (zone-less) timestamp shapes;
// zone-less values are read as UTC.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// NormTime renders any parseable timestamp as RFC 3339 UTC so v2 (zone-less)
// and v3 values compare and read the same; unparseable values pass through.
func NormTime(s string) string {
	if t, ok := ParseTime(s); ok {
		return t.Format(time.RFC3339)
	}
	return s
}

// ClientNames maps Backup Radar company names to the ScalePad client name for
// companies that are mapped, learned from v3 devices.
func (s Snapshot) ClientNames() map[string]string {
	m := map[string]string{}
	for _, d := range s.Devices {
		if d.Client.Name != "" && d.Client.BRCompanyName != "" {
			m[norm(d.Client.BRCompanyName)] = d.Client.Name
		}
	}
	return m
}

// displayClient shows the ScalePad client name when the Backup Radar company
// is mapped, so one backup carries one client name across every report.
func displayClient(names map[string]string, brCompany string) string {
	if IsUnassigned(brCompany) {
		return "(unassigned)"
	}
	if n, ok := names[norm(brCompany)]; ok {
		return n
	}
	return brCompany
}

// ClientMatches reports whether a --client filter matches any of the names
// (ScalePad or Backup Radar) a backup is known by.
func ClientMatches(filter string, names ...string) bool {
	if filter == "" {
		return true
	}
	for _, n := range names {
		if strings.Contains(norm(n), norm(filter)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// dayWindow is days calendar days ending on anchor's date, [from, to),
// matching the API's history_days ("days of history ending on the current
// date"): days=1 is anchor's day only. Later dates are outside it too.
type dayWindow struct{ from, to time.Time }

func newDayWindow(anchor time.Time, days int) dayWindow {
	y, m, d := anchor.UTC().Date()
	to := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return dayWindow{from: to.AddDate(0, 0, -days), to: to}
}

func (w dayWindow) has(t time.Time) bool { return !t.Before(w.from) && t.Before(w.to) }

// History collections newestDay can anchor on.
const (
	histHealth = 1 << iota
	histDevices
	histBackups
)

// implausible reports a history date more than a day past now (no tenant's
// local date is that far ahead); reports ignore such entries.
func implausible(t, now time.Time) bool { return t.After(now.Add(24 * time.Hour)) }

// newestDay is the newest synced history date in the chosen collections.
// API history windows end on the tenant's local date, which the CLI cannot
// know, so report windows end on the newest date the API returned (a date
// more than a day past now is ignored); with no history they end today (UTC).
func (s Snapshot) newestDay(now time.Time, which int) time.Time {
	var newest time.Time
	seen := func(date string) {
		if t, ok := ParseTime(date); ok && t.After(newest) && !implausible(t, now) {
			newest = t
		}
	}
	if which&histHealth != 0 {
		for _, h := range s.Health {
			for _, d := range h.History {
				seen(d.Date)
			}
		}
	}
	if which&histDevices != 0 {
		for _, dev := range s.Devices {
			for _, d := range dev.History {
				seen(d.Date)
			}
		}
	}
	if which&histBackups != 0 {
		for _, b := range s.Backups {
			for _, d := range b.History {
				seen(d.Date)
			}
		}
	}
	if newest.IsZero() {
		return now
	}
	return newest
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// ---------------------------------------------------------------- check

// CheckRow is one backup needing attention today.
type CheckRow struct {
	Client      string  `json:"client"`
	Device      string  `json:"device"`
	Job         string  `json:"job"`
	State       string  `json:"state"`
	Trend       string  `json:"trend"`
	DaysInState float64 `json:"days_in_state"`
	SLAStatus   string  `json:"sla_status,omitempty"`
	V2Status    string  `json:"v2_status,omitempty"`
	TicketCount *int    `json:"ticket_count,omitempty"`
	LastResult  string  `json:"last_result,omitempty"`
	LastSuccess string  `json:"last_success,omitempty"`
	Source      string  `json:"source"`
	BRDeviceID  string  `json:"br_device_id,omitempty"`
	BackupID    int64   `json:"backup_id,omitempty"`
	ClientID    string  `json:"client_id,omitempty"`
}

// Check ranks today's problem backups: no results, then failing, then (when
// includeAtRisk) at-risk and warning. A backup present in both generations
// becomes one row with source "both".
func Check(s Snapshot, includeAtRisk bool, clientFilter string, now time.Time) []CheckRow {
	rows := map[string]*CheckRow{}
	order := []string{}
	names := s.ClientNames()
	brCompany := map[string]string{}
	join := newJoiner(s)
	upsert := func(key string, r CheckRow) {
		if existing, ok := rows[key]; ok {
			mergeCheck(existing, r)
			return
		}
		cp := r
		rows[key] = &cp
		order = append(order, key)
	}
	for _, d := range s.Devices {
		hist := d.History[:0:0]
		for _, h := range d.History {
			if t, ok := ParseTime(h.Date); !ok || !implausible(t, now) {
				hist = append(hist, h)
			}
		}
		if len(hist) == 0 {
			continue
		}
		state := StateFromSLA(hist[0].SLAStatus)
		days := 0 // consecutive calendar days in this same state
		var prev time.Time
		for _, h := range hist {
			t, ok := ParseTime(h.Date)
			if StateFromSLA(h.SLAStatus) != state || (days > 0 && (!ok || prev.Sub(t) > 36*time.Hour)) {
				break // a different state, or a gap in the stored days
			}
			prev = t
			days++
		}
		trend := "new"
		if days > 1 {
			trend = "ongoing"
		}
		r := CheckRow{Client: displayCompany(d.ClientName()), Device: d.Name(), Job: d.BRJobName, State: state, Trend: trend,
			DaysInState: float64(days), SLAStatus: hist[0].SLAStatus, Source: "v3", BRDeviceID: d.BRDeviceID, ClientID: d.Client.ClientID}
		if t := d.LastResultAt(); !t.IsZero() {
			r.LastResult = t.Format(time.RFC3339)
		}
		key := join.v3(d)
		brCompany[key] = d.Client.BRCompanyName
		upsert(key, r)
	}
	for _, b := range s.Backups {
		tc := b.TicketCount
		trend := "new"
		if b.DaysInStatus >= 1 {
			trend = "ongoing"
		}
		r := CheckRow{Client: displayClient(names, b.CompanyName), Device: b.DeviceName, Job: b.JobName, State: StateFromV2(b.Status.Name), Trend: trend,
			DaysInState: round1(b.DaysInStatus), V2Status: b.Status.Name, TicketCount: &tc, LastResult: NormTime(b.LastResult),
			LastSuccess: NormTime(b.LastSuccess), Source: "v2", BackupID: b.BackupID}
		key := join.v2(b)
		brCompany[key] = b.CompanyName
		upsert(key, r)
	}
	out := make([]CheckRow, 0, len(order))
	for _, k := range order {
		r := *rows[k]
		if r.State == StateOK || (!includeAtRisk && severity[r.State] < severity[StateFailing]) {
			continue
		}
		if !ClientMatches(clientFilter, r.Client, brCompany[k]) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if severity[out[i].State] != severity[out[j].State] {
			return severity[out[i].State] > severity[out[j].State]
		}
		if out[i].DaysInState != out[j].DaysInState {
			return out[i].DaysInState > out[j].DaysInState
		}
		return norm(out[i].Client+out[i].Device+out[i].Job) < norm(out[j].Client+out[j].Device+out[j].Job)
	})
	return out
}

func mergeCheck(dst *CheckRow, src CheckRow) {
	// days_in_state and trend describe the reported state, so they come from
	// whichever side sets it; when both report that same state the longer,
	// ongoing reading wins. Equal severity with different states (at_risk vs
	// warning) keeps the displayed state's own reading.
	switch {
	case severity[src.State] > severity[dst.State]:
		dst.State, dst.Trend, dst.DaysInState = src.State, src.Trend, src.DaysInState
	case src.State == dst.State && src.State != StateOK:
		if src.Trend == "ongoing" {
			dst.Trend = "ongoing"
		}
		if src.DaysInState > dst.DaysInState {
			dst.DaysInState = src.DaysInState
		}
	}
	dst.SLAStatus = firstNonEmpty(dst.SLAStatus, src.SLAStatus)
	dst.V2Status = firstNonEmpty(dst.V2Status, src.V2Status)
	if dst.TicketCount == nil {
		dst.TicketCount = src.TicketCount
	}
	if src.LastResult > dst.LastResult {
		dst.LastResult = src.LastResult
	}
	dst.LastSuccess = firstNonEmpty(dst.LastSuccess, src.LastSuccess)
	dst.BRDeviceID = firstNonEmpty(dst.BRDeviceID, src.BRDeviceID)
	dst.ClientID = firstNonEmpty(dst.ClientID, src.ClientID)
	if dst.BackupID == 0 {
		dst.BackupID = src.BackupID
	}
	if dst.Source != src.Source {
		dst.Source = "both"
	}
}

// ---------------------------------------------------------------- unwatched

// Unwatched reason codes.
const (
	ReasonInactive   = "inactive"
	ReasonUnmapped   = "unmapped"
	ReasonTicketless = "ticketless"
	ReasonNoPSA      = "no-psa"
	ReasonMasked     = "masked"
)

// AllReasons lists every unwatched reason in report order.
var AllReasons = []string{ReasonInactive, ReasonUnmapped, ReasonTicketless, ReasonNoPSA, ReasonMasked}

// UnwatchedRow is one backup with no person or ticket behind it.
type UnwatchedRow struct {
	Reason       string `json:"reason"`
	Client       string `json:"client"`
	BRCompany    string `json:"br_company,omitempty"`
	Device       string `json:"device"`
	Job          string `json:"job"`
	Detail       string `json:"detail"`
	Source       string `json:"source"`
	BackupID     int64  `json:"backup_id,omitempty"`
	BRDeviceID   string `json:"br_device_id,omitempty"`
	LastReceived string `json:"last_received,omitempty"`
}

// Unwatched returns blind spots for the requested reasons.
func Unwatched(s Snapshot, reasons []string, now time.Time) []UnwatchedRow {
	maskedWin := newDayWindow(s.newestDay(now, histBackups), 31)
	want := map[string]bool{}
	for _, r := range reasons {
		want[r] = true
	}
	out := make([]UnwatchedRow, 0)
	names := s.ClientNames()
	if want[ReasonInactive] {
		for _, in := range s.Inactive {
			out = append(out, UnwatchedRow{Reason: ReasonInactive, Client: displayClient(names, in.CompanyName), BRCompany: in.CompanyName, Device: in.DeviceName, Job: in.JobName,
				Detail: "results arriving (" + firstNonEmpty(in.MethodName, "unknown method") + ") but the backup was never activated", Source: "v2",
				BackupID: in.BackupID, LastReceived: NormTime(in.LastReceived)})
		}
	}
	if want[ReasonUnmapped] {
		for _, d := range s.Devices {
			detail := ""
			switch {
			case d.Client.ClientID == "":
				detail = "Backup Radar company is not mapped to a ScalePad client"
			case d.DeviceID == "":
				detail = "device is not mapped to a ScalePad device"
			default:
				continue
			}
			row := UnwatchedRow{Reason: ReasonUnmapped, Client: displayCompany(d.ClientName()), BRCompany: d.Client.BRCompanyName, Device: d.Name(), Job: d.BRJobName, Detail: detail, Source: "v3", BRDeviceID: d.BRDeviceID}
			if t := d.LastResultAt(); !t.IsZero() {
				row.LastReceived = t.Format(time.RFC3339)
			}
			out = append(out, row)
		}
	}
	for _, b := range s.Backups {
		base := UnwatchedRow{Client: displayClient(names, b.CompanyName), BRCompany: b.CompanyName, Device: b.DeviceName, Job: b.JobName, Source: "v2", BackupID: b.BackupID, LastReceived: NormTime(b.LastResult)}
		state := StateFromV2(b.Status.Name)
		if want[ReasonTicketless] && severity[state] >= severity[StateFailing] && b.TicketCount == 0 {
			r := base
			r.Reason, r.Detail = ReasonTicketless, fmt.Sprintf("%s for %.1f days with no open ticket", b.Status.Name, b.DaysInStatus)
			out = append(out, r)
		}
		if want[ReasonNoPSA] && IsUnassigned(b.TicketingCompany) {
			r := base
			r.Reason, r.Detail = ReasonNoPSA, "no PSA ticketing company, so failures never create tickets"
			out = append(out, r)
		}
		if want[ReasonMasked] && b.TreatWarningAsSuccess {
			// Count the last 31 days (v2's maximum window) up to the newest
			// synced day; the store keeps older days, which must not read as recent.
			warnings := 0
			for _, h := range b.History {
				if t, ok := ParseTime(h.Date); ok && maskedWin.has(t) {
					warnings += h.CountWarning
				}
			}
			if warnings > 0 {
				r := base
				r.Reason, r.Detail = ReasonMasked, fmt.Sprintf("%d warning result(s) in the last 31 days counted as success (treatWarningAsSuccess)", warnings)
				out = append(out, r)
			}
		}
	}
	rank := map[string]int{}
	for i, r := range AllReasons {
		rank[r] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Reason != out[j].Reason {
			return rank[out[i].Reason] < rank[out[j].Reason]
		}
		return norm(out[i].Client+"\x1f"+out[i].Device+"\x1f"+out[i].Job) < norm(out[j].Client+"\x1f"+out[j].Device+"\x1f"+out[j].Job)
	})
	return out
}

func displayCompany(s string) string {
	if IsUnassigned(s) {
		return "(unassigned)"
	}
	return s
}

// ---------------------------------------------------------------- coverage diff

// VendorDevice is one name from an external backup vendor's device list.
type VendorDevice struct {
	Name       string `json:"name"`
	Normalized string `json:"normalized"`
}

// CoverageMatch pairs a vendor device with Backup Radar backups.
type CoverageMatch struct {
	Name        string   `json:"name"`
	BRDevice    string   `json:"br_device"`
	Clients     []string `json:"clients"`
	Jobs        int      `json:"jobs"`
	MatchMethod string   `json:"match_method"`
	Monitored   bool     `json:"monitored"`
	// OtherClient is true when the device is monitored under a client the
	// --client filter excludes, so it is neither a gap nor in scope.
	OtherClient bool `json:"other_client,omitempty"`
	// Candidates lists every Backup Radar device a short name could mean
	// when more than one shares it (match_method short-name-ambiguous).
	Candidates []string `json:"candidates,omitempty"`
}

// CoverageBROnly is a Backup Radar device the vendor list does not contain.
type CoverageBROnly struct {
	Device  string   `json:"device"`
	Clients []string `json:"clients"`
	Jobs    int      `json:"jobs"`
}

// CoverageResult is the two-way diff.
type CoverageResult struct {
	VendorOnly      []VendorDevice   `json:"vendor_only"`
	BackupRadarOnly []CoverageBROnly `json:"backup_radar_only"`
	Matched         []CoverageMatch  `json:"matched"`
	Counts          map[string]int   `json:"counts"`
}

// NormalizeHost lowercases, trims, and strips a DNS suffix (keeping IPs).
func NormalizeHost(s string) string {
	s = norm(s)
	if net.ParseIP(s) == nil {
		if i := strings.IndexByte(s, '.'); i > 0 {
			s = s[:i]
		}
	}
	return s
}

type brDev struct {
	display   string
	company   string // Backup Radar company: the entry's client identity
	clients   map[string]bool
	jobs      int
	monitored bool
}

// coverIndex finds Backup Radar devices by exact name and by short host name.
// Each entry is one device name under one Backup Radar company, so two
// companies' same-named devices never merge (and never lend each other
// monitored status), even when both map to same-named ScalePad clients.
type coverIndex struct {
	byKey map[string]*brDev   // name + company
	exact map[string][]*brDev // name -> one entry per Backup Radar company
	short map[string][]*brDev
	order []*brDev
}

func newCoverIndex() *coverIndex {
	return &coverIndex{byKey: map[string]*brDev{}, exact: map[string][]*brDev{}, short: map[string][]*brDev{}}
}

func (ix *coverIndex) add(name, client, company string, monitored bool) {
	if strings.TrimSpace(name) == "" {
		return
	}
	k := norm(name) + "\x1f" + norm(company)
	d := ix.byKey[k]
	if d == nil {
		d = &brDev{display: strings.TrimSpace(name), company: strings.TrimSpace(company), clients: map[string]bool{client: true}}
		ix.byKey[k] = d
		ix.exact[norm(name)] = append(ix.exact[norm(name)], d)
		sk := NormalizeHost(name)
		ix.short[sk] = append(ix.short[sk], d)
		ix.order = append(ix.order, d)
	}
	d.jobs++
	d.monitored = d.monitored || monitored
}

// Coverage diffs vendor device names against Backup Radar devices
// (v3 devices, v2 backups as monitored; v2 inactive as received-only).
// clientFilter scopes the Backup Radar side (ScalePad or Backup Radar name):
// matching and monitored status use in-scope devices only, only in-scope
// devices can be backup_radar_only, and a vendor device found only under
// another client is matched with other_client=true, never vendor_only.
func Coverage(s Snapshot, vendor []string, clientFilter string) CoverageResult {
	scoped, other := newCoverIndex(), newCoverIndex()
	names := s.ClientNames()
	add := func(name, company string, monitored bool) {
		client := displayClient(names, company)
		if ClientMatches(clientFilter, client, company) {
			scoped.add(name, client, company, monitored)
		} else {
			other.add(name, client, company, monitored)
		}
	}
	join := newJoiner(s)
	for _, d := range s.Devices {
		add(firstNonEmpty(d.BRDeviceName, d.DeviceName), firstNonEmpty(d.Client.BRCompanyName, d.Client.Name), true)
	}
	for _, b := range s.Backups {
		if join.joined(Key(b.CompanyName, b.DeviceName, b.JobName)) {
			continue // the same backup, already counted from v3
		}
		add(b.DeviceName, b.CompanyName, true)
	}
	for _, in := range s.Inactive {
		add(in.DeviceName, in.CompanyName, false)
	}
	res := CoverageResult{VendorOnly: make([]VendorDevice, 0), BackupRadarOnly: make([]CoverageBROnly, 0), Matched: make([]CoverageMatch, 0)}
	used := map[*brDev]bool{}
	seenVendor := map[string]bool{}
	for _, raw := range vendor {
		name := strings.TrimSpace(raw)
		if name == "" || seenVendor[norm(name)] {
			continue
		}
		seenVendor[norm(name)] = true
		// Exact names win over short names in either scope, so another
		// client's exact match is never shadowed by a scoped short-name one.
		var devs []*brDev
		var method string
		var otherClient bool
		for _, try := range []struct {
			found  []*brDev
			method string
			other  bool
		}{
			{scoped.exact[norm(name)], "exact", false},
			{other.exact[norm(name)], "exact", true},
			{scoped.short[NormalizeHost(name)], "short-name", false},
			{other.short[NormalizeHost(name)], "short-name", true},
		} {
			if len(try.found) > 0 {
				devs, method, otherClient = try.found, try.method, try.other
				break
			}
		}
		if len(devs) == 0 {
			res.VendorOnly = append(res.VendorOnly, VendorDevice{Name: name, Normalized: NormalizeHost(name)})
			continue
		}
		if len(devs) > 1 {
			method += "-ambiguous" // several devices (or clients) answer to this name
		}
		m := CoverageMatch{Name: name, MatchMethod: method, Monitored: true, OtherClient: otherClient}
		clients := map[string]bool{}
		for _, d := range devs {
			used[d] = true
			m.Jobs += d.jobs
			m.Monitored = m.Monitored && d.monitored // monitored only if every candidate is
			for c := range d.clients {
				clients[c] = true
				if len(devs) > 1 {
					label := c
					if d.company != "" && norm(d.company) != norm(c) {
						label += "; Backup Radar: " + d.company // tells same-named clients apart
					}
					m.Candidates = append(m.Candidates, d.display+" ("+label+")")
				}
			}
		}
		sort.Strings(m.Candidates)
		m.Clients = sortedKeys(clients)
		m.BRDevice = devs[0].display
		if len(devs) > 1 {
			m.BRDevice = strings.Join(m.Candidates, " | ")
		}
		res.Matched = append(res.Matched, m)
	}
	for _, d := range scoped.order {
		if !used[d] {
			res.BackupRadarOnly = append(res.BackupRadarOnly, CoverageBROnly{Device: d.display, Clients: sortedKeys(d.clients), Jobs: d.jobs})
		}
	}
	sort.SliceStable(res.BackupRadarOnly, func(i, j int) bool {
		a, b := res.BackupRadarOnly[i], res.BackupRadarOnly[j]
		return norm(a.Device+"\x1f"+strings.Join(a.Clients, ",")) < norm(b.Device+"\x1f"+strings.Join(b.Clients, ","))
	})
	unmonitored, ambiguous, otherCount := 0, 0, 0
	for _, m := range res.Matched {
		if m.OtherClient {
			otherCount++
		}
		if !m.Monitored {
			unmonitored++
		}
		if len(m.Candidates) > 0 {
			ambiguous++
		}
	}
	res.Counts = map[string]int{"vendor_devices": len(seenVendor), "matched": len(res.Matched), "vendor_only": len(res.VendorOnly),
		"backup_radar_only": len(res.BackupRadarOnly), "matched_inactive_only": unmonitored, "ambiguous": ambiguous, "matched_other_client": otherCount}
	return res
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- stale

// RetireRequest is a POST /backups/retire body (RetireBackupsRequest).
type RetireRequest struct {
	Reason          string `json:"reason"`
	DryRun          bool   `json:"dry_run"`
	ClosePSATickets bool   `json:"close_psa_tickets,omitempty"`
	JobName         string `json:"job_name,omitempty"`
	DeviceName      string `json:"device_name,omitempty"`
	ClientName      string `json:"client_name,omitempty"`
	DeviceID        string `json:"device_id,omitempty"`
	NoResultIn      int    `json:"no_result_in,omitempty"`
}

// scoped reports whether the filters pin one backup: a ScalePad device id, or
// both the device and client names (job is always set).
func (r RetireRequest) scoped() bool {
	return r.DeviceID != "" || (r.ClientName != "" && r.DeviceName != "")
}

// retireFor builds the retire filters for a v3 device. A mapped device is
// pinned by its ScalePad device id; an unmapped one by its Backup Radar names,
// which the endpoint matches when the device is not mapped.
func retireFor(d Device) RetireRequest {
	if d.DeviceID != "" {
		return RetireRequest{JobName: d.BRJobName, DeviceID: d.DeviceID}
	}
	return RetireRequest{JobName: d.BRJobName, DeviceName: d.BRDeviceName, ClientName: retireName(d.Client.BRCompanyName)}
}

// retireName drops placeholder or empty client names: the retire endpoint
// requires a non-empty exact name, and "[Undefined]" matches nothing.
func retireName(s string) string {
	if IsUnassigned(s) {
		return ""
	}
	return s
}

// RetireBodyKey is the identity the retire endpoint matches on: names compare
// case-insensitively, so two bodies with the same key retire the same backups.
func RetireBodyKey(q RetireRequest) RetireRequest {
	q.JobName, q.DeviceName, q.ClientName, q.DeviceID = norm(q.JobName), norm(q.DeviceName), norm(q.ClientName), norm(q.DeviceID)
	return q
}

// StaleRow is a backup silent for at least the threshold.
type StaleRow struct {
	Client string `json:"client"`
	// ClientID is the ScalePad client id when the backup's device is mapped;
	// two clients can share a display name, so by_client groups on it.
	ClientID      string         `json:"client_id,omitempty"`
	BRDeviceID    string         `json:"br_device_id,omitempty"`
	BackupID      int64          `json:"backup_id,omitempty"`
	BRCompany     string         `json:"br_company,omitempty"`
	Device        string         `json:"device"`
	Job           string         `json:"job"`
	DaysSilent    *int           `json:"days_silent"`
	NeverReported bool           `json:"never_reported"`
	LastResult    string         `json:"last_result,omitempty"`
	LastSuccess   string         `json:"last_success,omitempty"`
	TicketCount   *int           `json:"ticket_count,omitempty"`
	Source        string         `json:"source"`
	RetireRequest *RetireRequest `json:"retire_request,omitempty"`
	// RetireNote explains a missing retire_request: without a client or a
	// ScalePad device id a name-only body could retire other clients' backups.
	RetireNote string `json:"retire_note,omitempty"`
}

// Stale lists backups with no result in at least days days (or ever).
// Retire bodies pin a mapped device by ScalePad device id and an unmapped one
// by Backup Radar names; a backup with neither a client nor a device id gets
// no body (retire_note says why) rather than an unscoped one.
func Stale(s Snapshot, days int, clientFilter string, closePSA bool, now time.Time) []StaleRow {
	type acc struct {
		row  StaleRow
		req  RetireRequest
		last time.Time
	}
	rows := map[string]*acc{}
	order := []string{}
	names := s.ClientNames()
	brCompany := map[string]string{}
	join := newJoiner(s)
	reason := fmt.Sprintf("No backup result in %d+ days (backup-radar-cli stale)", days)
	get := func(key string, mk func() StaleRow, req RetireRequest) *acc {
		if a, ok := rows[key]; ok {
			return a
		}
		a := &acc{row: mk(), req: req}
		rows[key] = a
		order = append(order, key)
		return a
	}
	for _, d := range s.Devices {
		key := join.v3(d)
		brCompany[key] = d.Client.BRCompanyName
		a := get(key, func() StaleRow {
			return StaleRow{Client: displayCompany(d.ClientName()), ClientID: d.Client.ClientID, BRDeviceID: d.BRDeviceID, BRCompany: d.Client.BRCompanyName, Device: d.Name(), Job: d.BRJobName, Source: "v3"}
		}, retireFor(d))
		if t := d.LastResultAt(); t.After(a.last) {
			a.last = t
		}
	}
	for _, b := range s.Backups {
		tc := b.TicketCount
		key := join.v2(b)
		brCompany[key] = b.CompanyName
		a := get(key, func() StaleRow {
			return StaleRow{Client: displayClient(names, b.CompanyName), BRCompany: b.CompanyName, Device: b.DeviceName, Job: b.JobName, Source: "v2"}
		}, RetireRequest{}) // see below: a v2-only row gets no retire body
		if a.row.Source == "v3" {
			a.row.Source = "both"
		}
		a.row.TicketCount = &tc
		a.row.BackupID = b.BackupID // HAND-FIX(stale-retire-body-collisions): tells same-named v2 backups apart
		a.row.LastSuccess = NormTime(b.LastSuccess)
		if t, ok := ParseTime(b.LastResult); ok && t.After(a.last) {
			a.last = t
		}
	}
	out := make([]StaleRow, 0)
	for _, k := range order {
		a := rows[k]
		r := a.row
		if !ClientMatches(clientFilter, r.Client, brCompany[k]) {
			continue
		}
		if a.last.IsZero() {
			r.NeverReported = true
		} else {
			silent := int(now.Sub(a.last).Hours() / 24)
			if silent < days {
				continue
			}
			r.DaysSilent = &silent
			r.LastResult = a.last.Format(time.RFC3339)
		}
		if req := a.req; req.scoped() {
			req.Reason, req.DryRun, req.ClosePSATickets = reason, true, closePSA
			// no_result_in keeps the filter to silent backups only (the API caps it at 3650).
			req.NoResultIn = min(max(days, 1), 3650)
			r.RetireRequest = &req
		} else {
			r.RetireNote = "no retire body: the backup has no client and no ScalePad device id, so name-only filters could retire other clients' backups; retire it in the Backup Radar UI"
			if r.Source == "v2" {
				// The retire endpoint matches ScalePad names for mapped devices and
				// Backup Radar names otherwise; without a v3 row we cannot tell which.
				r.RetireNote = "no retire body: this backup has no single matching v3 device, so whether it is mapped to ScalePad (which decides the names retire matches) is unknown; sync devices with BACKUP_RADAR_API_KEY or retire it in the Backup Radar UI"
			}
		}
		out = append(out, r)
	}
	// The retire endpoint matches names case-insensitively, so two backups
	// that share company, device and job names get one body that retires
	// both; say so on each row. HAND-FIX(stale-retire-body-collisions)
	bodyKey := RetireBodyKey
	bodies := map[RetireRequest]int{}
	for _, r := range out {
		if r.RetireRequest != nil {
			bodies[bodyKey(*r.RetireRequest)]++
		}
	}
	for i, r := range out {
		if r.RetireRequest != nil {
			if n := bodies[bodyKey(*r.RetireRequest)]; n > 1 {
				out[i].RetireNote = fmt.Sprintf("this retire body matches all %d stale backups that share these names; retire acts on every one of them", n)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if norm(out[i].Client) != norm(out[j].Client) {
			return norm(out[i].Client) < norm(out[j].Client)
		}
		return silentDays(out[i]) > silentDays(out[j])
	})
	return out
}

func silentDays(r StaleRow) int {
	if r.DaysSilent == nil {
		return math.MaxInt
	}
	return *r.DaysSilent
}

// ---------------------------------------------------------------- scorecard

// Counts sums per-day compliance counts across a window.
type Counts struct {
	InCompliance             int `json:"in_compliance"`
	AtRisk                   int `json:"at_risk"`
	OutOfCompliance          int `json:"out_of_compliance"`
	OutOfComplianceNoResults int `json:"out_of_compliance_no_results"`
	Pending                  int `json:"pending"`
}

func (c *Counts) add(x Compliance) {
	c.InCompliance += x.InCompliance
	c.AtRisk += x.AtRisk
	c.OutOfCompliance += x.OutOfCompliance
	c.OutOfComplianceNoResults += x.OutOfComplianceNoResults
	c.Pending += x.Pending
}

// Rate is the in-compliance percentage, excluding pending; nil when nothing was scored.
func (c Counts) Rate() *float64 {
	den := c.InCompliance + c.AtRisk + c.OutOfCompliance + c.OutOfComplianceNoResults
	if den == 0 {
		return nil
	}
	r := round1(100 * float64(c.InCompliance) / float64(den))
	return &r
}

// WeekRate is one week of a client's trend.
type WeekRate struct {
	WeekStart string   `json:"week_start"`
	Rate      *float64 `json:"rate"`
	Days      int      `json:"days"`
}

// WorstDevice ranks devices by out-of-compliance days.
type WorstDevice struct {
	Device  string `json:"device"`
	Job     string `json:"job"`
	BadDays int    `json:"bad_days"`
	Days    int    `json:"days"`
}

// ClientScore is one client's compliance over the window.
type ClientScore struct {
	Client       string        `json:"client"`
	ClientID     string        `json:"client_id"`
	Rate         *float64      `json:"rate"`
	DaysCovered  int           `json:"days_covered"`
	Counts       Counts        `json:"counts"`
	Weekly       []WeekRate    `json:"weekly,omitempty"`
	WorstDevices []WorstDevice `json:"worst_devices,omitempty"`
}

// Scorecard scores every client over the last days days. When detail is
// true it adds the weekly trend and the five worst devices per client.
func Scorecard(s Snapshot, days int, detail bool, now time.Time) []ClientScore {
	win := newDayWindow(s.newestDay(now, histHealth), days)
	out := make([]ClientScore, 0, len(s.Health))
	for _, h := range s.Health {
		cs := ClientScore{Client: h.Client.Name, ClientID: h.Client.ClientID}
		weeks := map[string]*Counts{}
		weekDays := map[string]int{}
		for _, day := range h.History {
			t, ok := ParseTime(day.Date)
			if ok && !win.has(t) {
				continue
			}
			cs.DaysCovered++
			cs.Counts.add(day.Compliance)
			if detail && ok {
				ws := weekStart(t)
				if weeks[ws] == nil {
					weeks[ws] = &Counts{}
				}
				weeks[ws].add(day.Compliance)
				weekDays[ws]++
			}
		}
		cs.Rate = cs.Counts.Rate()
		if detail {
			cs.Weekly = make([]WeekRate, 0, len(weeks))
			for ws, c := range weeks {
				cs.Weekly = append(cs.Weekly, WeekRate{WeekStart: ws, Rate: c.Rate(), Days: weekDays[ws]})
			}
			sort.Slice(cs.Weekly, func(i, j int) bool { return cs.Weekly[i].WeekStart < cs.Weekly[j].WeekStart })
			cs.WorstDevices = worstDevices(s.Devices, h.Client.ClientID, h.Client.Name, win)
		}
		out = append(out, cs)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := out[i].Rate, out[j].Rate
		switch {
		case ri == nil && rj == nil:
			return norm(out[i].Client) < norm(out[j].Client)
		case ri == nil:
			return false
		case rj == nil:
			return true
		case *ri != *rj:
			return *ri < *rj
		}
		return norm(out[i].Client) < norm(out[j].Client)
	})
	return out
}

func weekStart(t time.Time) string {
	offset := (int(t.Weekday()) + 6) % 7 // Monday = 0
	return t.AddDate(0, 0, -offset).Format("2006-01-02")
}

func worstDevices(devices []Device, clientID, clientName string, win dayWindow) []WorstDevice {
	out := make([]WorstDevice, 0)
	for _, d := range devices {
		if clientID != "" && d.Client.ClientID != clientID {
			continue
		}
		if clientID == "" && !strings.EqualFold(d.ClientName(), clientName) {
			continue
		}
		w := WorstDevice{Device: d.Name(), Job: d.BRJobName}
		for _, h := range d.History {
			if t, ok := ParseTime(h.Date); ok && !win.has(t) {
				continue
			}
			w.Days++
			if s := StateFromSLA(h.SLAStatus); s == StateFailing || s == StateNoResults {
				w.BadDays++
			}
		}
		if w.BadDays > 0 {
			out = append(out, w)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].BadDays != out[j].BadDays {
			return out[i].BadDays > out[j].BadDays
		}
		return norm(out[i].Device+out[i].Job) < norm(out[j].Device+out[j].Job)
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// FindClient picks the scorecard for a client by id, exact name, or a unique
// name substring. A name shared by several client ids is ambiguous.
func FindClient(scores []ClientScore, query string) (ClientScore, error) {
	q := norm(query)
	var exact, partial []ClientScore
	for _, c := range scores {
		if c.ClientID != "" && c.ClientID == query {
			return c, nil
		}
		if norm(c.Client) == q {
			exact = append(exact, c)
		}
		if strings.Contains(norm(c.Client), q) {
			partial = append(partial, c)
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return ClientScore{}, fmt.Errorf("no synced client matches %q", query)
	}
	names := make([]string, 0, len(matches))
	for _, c := range matches {
		names = append(names, fmt.Sprintf("%q (client id %s)", c.Client, c.ClientID)) // %q escapes tenant control characters
	}
	return ClientScore{}, fmt.Errorf("%q matches %d clients (%s); use the client id", query, len(matches), strings.Join(names, ", "))
}

// ---------------------------------------------------------------- timeline

// TimelineJob summarizes one backup job on the device.
type TimelineJob struct {
	Client      string   `json:"client"`
	BRCompany   string   `json:"br_company,omitempty"`
	Device      string   `json:"device"`
	Job         string   `json:"job"`
	BackupID    int64    `json:"backup_id,omitempty"`
	BRDeviceID  string   `json:"br_device_id,omitempty"`
	V2Status    string   `json:"v2_status,omitempty"`
	TicketCount *int     `json:"ticket_count,omitempty"`
	LastSuccess string   `json:"last_success,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Note        string   `json:"note,omitempty"`
}

// TimelineDay merges both generations for one job on one date.
type TimelineDay struct {
	Date       string          `json:"date"`
	Client     string          `json:"client"`
	BRCompany  string          `json:"br_company,omitempty"` // HAND-FIX(timeline-day-identity)
	BRDeviceID string          `json:"br_device_id,omitempty"`
	Device     string          `json:"device"`
	Job        string          `json:"job"`
	SLAStatus  string          `json:"sla_status,omitempty"`
	V2Status   string          `json:"v2_status,omitempty"`
	Success    *int            `json:"success,omitempty"`
	Warning    *int            `json:"warning,omitempty"`
	Failure    *int            `json:"failure,omitempty"`
	NoResult   *int            `json:"no_result,omitempty"`
	BackupID   int64           `json:"backup_id,omitempty"`
	Results    json.RawMessage `json:"results,omitempty"`
	jobKey     string
}

// Timeline merges per-day history for one device (exact name match first,
// then substring), optionally limited to one job and one client. Every row
// carries its client and device, because two clients can share a host name.
func Timeline(s Snapshot, device, job, clientFilter string, days int, now time.Time) ([]TimelineJob, []TimelineDay) {
	names := s.ClientNames()
	join := newJoiner(s)
	exact := func(name string) bool { return norm(name) == norm(device) }
	match := exact
	if !anyDeviceMatches(s, exact, clientFilter, names) {
		match = func(name string) bool { return strings.Contains(norm(name), norm(device)) }
	}
	jobOK := func(j string) bool { return job == "" || norm(j) == norm(job) }
	win := newDayWindow(s.newestDay(now, histDevices|histBackups), days)
	jobs := map[string]*TimelineJob{}
	jobOrder := []string{}
	dayRows := map[string]*TimelineDay{}
	getJob := func(key string, mk func() TimelineJob) *TimelineJob {
		if j, ok := jobs[key]; ok {
			return j
		}
		j := mk()
		jobs[key] = &j
		jobOrder = append(jobOrder, key)
		return &j
	}
	getDay := func(jobKey, date string, j *TimelineJob) *TimelineDay {
		k := jobKey + "\x1f" + date
		if d, ok := dayRows[k]; ok {
			return d
		}
		d := &TimelineDay{Date: date, Client: j.Client, BRCompany: j.BRCompany, BRDeviceID: j.BRDeviceID, Device: j.Device, Job: j.Job, jobKey: jobKey}
		dayRows[k] = d
		return d
	}
	for _, d := range s.Devices {
		if !(match(d.BRDeviceName) || match(d.DeviceName)) || !jobOK(d.BRJobName) || !ClientMatches(clientFilter, d.ClientName(), d.Client.BRCompanyName) {
			continue
		}
		key := join.v3(d)
		j := getJob(key, func() TimelineJob {
			return TimelineJob{Client: displayCompany(d.ClientName()), BRCompany: d.Client.BRCompanyName, Device: firstNonEmpty(d.BRDeviceName, d.DeviceName), Job: d.BRJobName, BRDeviceID: d.BRDeviceID}
		})
		for _, h := range d.History {
			t, ok := ParseTime(h.Date)
			if !ok || !win.has(t) {
				continue
			}
			getDay(key, t.Format("2006-01-02"), j).SLAStatus = h.SLAStatus
		}
	}
	for _, b := range s.Backups {
		client := displayClient(names, b.CompanyName)
		key := join.v2(b)
		_, v3Matched := jobs[key]
		if !(match(b.DeviceName) || v3Matched) || !jobOK(b.JobName) || !ClientMatches(clientFilter, client, b.CompanyName) {
			continue
		}
		j := getJob(key, func() TimelineJob {
			return TimelineJob{Client: client, BRCompany: b.CompanyName, Device: b.DeviceName, Job: b.JobName}
		})
		tc := b.TicketCount
		j.BackupID, j.V2Status, j.TicketCount, j.LastSuccess, j.Tags, j.Note = b.BackupID, b.Status.Name, &tc, NormTime(b.LastSuccess), b.Tags, b.Note
		for _, h := range b.History {
			t, ok := ParseTime(h.Date)
			if !ok || !win.has(t) {
				continue
			}
			day := getDay(key, t.Format("2006-01-02"), j)
			sc, wc, fc, nc := h.CountSuccess, h.CountWarning, h.CountFailure, h.CountNoResult
			day.V2Status, day.Success, day.Warning, day.Failure, day.NoResult, day.BackupID = h.Status.Name, &sc, &wc, &fc, &nc, b.BackupID
		}
	}
	outJobs := make([]TimelineJob, 0, len(jobOrder))
	for _, k := range jobOrder {
		outJobs = append(outJobs, *jobs[k])
	}
	outDays := make([]TimelineDay, 0, len(dayRows))
	for _, d := range dayRows {
		if d.BackupID == 0 {
			d.BackupID = jobs[d.jobKey].BackupID
		}
		outDays = append(outDays, *d)
	}
	sort.Slice(outDays, func(i, j int) bool {
		if outDays[i].Date != outDays[j].Date {
			return outDays[i].Date > outDays[j].Date
		}
		return norm(outDays[i].Client+"\x1f"+outDays[i].Device+"\x1f"+outDays[i].Job) < norm(outDays[j].Client+"\x1f"+outDays[j].Device+"\x1f"+outDays[j].Job)
	})
	return outJobs, outDays
}

func anyDeviceMatches(s Snapshot, match func(string) bool, clientFilter string, names map[string]string) bool {
	for _, d := range s.Devices {
		if (match(d.BRDeviceName) || match(d.DeviceName)) && ClientMatches(clientFilter, d.ClientName(), d.Client.BRCompanyName) {
			return true
		}
	}
	for _, b := range s.Backups {
		if match(b.DeviceName) && ClientMatches(clientFilter, displayClient(names, b.CompanyName), b.CompanyName) {
			return true
		}
	}
	return false
}

// DistinctDevices lists the client/device pairs a timeline matched, so callers
// can warn when one name resolved to more than one machine.
func DistinctDevices(jobs []TimelineJob) []string {
	// A machine is one device under one Backup Radar company; two companies
	// can map to same-named ScalePad clients, so those labels name the company.
	first := map[string]TimelineJob{} // machine key -> its first job
	labels := map[string]int{}        // label -> machine count
	for _, j := range jobs {
		k := norm(firstNonEmpty(j.BRCompany, j.Client)) + "\x1f" + norm(j.Device)
		if _, ok := first[k]; !ok {
			first[k] = j
			labels[norm(j.Client+" / "+j.Device)]++
		}
	}
	out := make([]string, 0, len(first))
	for _, j := range first {
		l := j.Client + " / " + j.Device
		if labels[norm(l)] > 1 {
			l += " (Backup Radar: " + j.BRCompany + ")"
		}
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
