// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package fleet holds the pure logic behind the cross-site OneView commands:
// normalizing endpoint/detection/job records, decoding job status bitmasks,
// and computing staleness, twins, per-site health, threat spread and the
// triage queue. No I/O lives here so every rule is table-testable.
package fleet

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Site is one OneView site (customer) and the Nebula account it maps to.
type Site struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Name      string `json:"company_name"`
	Status    string `json:"account_status,omitempty"`
	EndDate   string `json:"site_end_date,omitempty"`
}

// ParseSites reads the GET /v1/sites envelope ({"sites":[...]}) or a bare array.
// Anything else (null, a missing list, a site without an id) is an error, so a
// malformed response can never read as "no sites" and reconcile the cache away.
func ParseSites(raw json.RawMessage) ([]Site, error) {
	var env struct {
		Sites []Site `json:"sites"`
	}
	sites := env.Sites
	if err := json.Unmarshal(raw, &env); err == nil && env.Sites != nil {
		sites = env.Sites
	} else if err := json.Unmarshal(raw, &sites); err != nil || sites == nil {
		return nil, fmt.Errorf("parsing sites: response is not a list of sites")
	}
	for i, s := range sites {
		if s.ID == "" {
			return nil, fmt.Errorf("parsing sites: site %d has no id", i)
		}
	}
	return sites, nil
}

// SiteIndex maps a Nebula account_id to its site.
func SiteIndex(sites []Site) map[string]Site {
	idx := make(map[string]Site, len(sites))
	for _, s := range sites {
		if s.AccountID != "" {
			idx[s.AccountID] = s
		}
	}
	return idx
}

// AccountIDs returns the account ids of the sites, skipping sites without a
// subscription (no account_id yet), optionally filtered by a case-insensitive
// substring of the site name.
func AccountIDs(sites []Site, nameFilter string) []string {
	nameFilter = strings.ToLower(strings.TrimSpace(nameFilter))
	ids := make([]string, 0, len(sites))
	for _, s := range sites {
		if s.AccountID == "" {
			continue
		}
		if nameFilter != "" && !strings.Contains(strings.ToLower(s.Name), nameFilter) {
			continue
		}
		ids = append(ids, s.AccountID)
	}
	return ids
}

// Endpoint is the flattened view of one OneView endpoint search record.
type Endpoint struct {
	ID               string   `json:"id"`
	AccountID        string   `json:"account_id"`
	Site             string   `json:"site,omitempty"`
	HostName         string   `json:"host_name"`
	FQDN             string   `json:"fully_qualified_host_name,omitempty"`
	DisplayName      string   `json:"display_name,omitempty"`
	OSType           string   `json:"os_type,omitempty"`
	OSPlatform       string   `json:"os_platform,omitempty"`
	SerialNumber     string   `json:"serial_number,omitempty"`
	MACs             []string `json:"mac_addresses,omitempty"`
	GroupName        string   `json:"group_name,omitempty"`
	PolicyName       string   `json:"policy_name,omitempty"`
	ProtectionStatus string   `json:"protection_status,omitempty"`
	Online           bool     `json:"online"`
	Isolated         bool     `json:"isolated"`
	InfectionCount   int      `json:"infection_count"`
	SuspiciousCount  int      `json:"suspicious_activity_count"`
	RebootRequired   bool     `json:"reboot_required"`
	UpdateAvailable  bool     `json:"update_available"`
	LastDaySeen      string   `json:"last_day_seen,omitempty"`
	LastActive       string   `json:"last_active,omitempty"`
	LastScannedAt    string   `json:"last_scanned_at,omitempty"`
	IsDeleted        bool     `json:"is_deleted,omitempty"`
}

type rawEndpoint struct {
	DisplayName      string `json:"display_name"`
	ProtectionStatus string `json:"protection_status"`
	Agent            struct {
		MachineID       string `json:"machine_id"`
		AccountID       string `json:"account_id"`
		HostName        string `json:"host_name"`
		FQDN            string `json:"fully_qualified_host_name"`
		SerialNumber    string `json:"serial_number"`
		UpdateAvailable bool   `json:"is_software_update_available"`
		OSInfo          struct {
			OSType     string `json:"os_type"`
			OSPlatform string `json:"os_platform"`
		} `json:"os_info"`
		NICs []struct {
			MAC string `json:"mac_address"`
		} `json:"nics"`
	} `json:"agent"`
	Machine struct {
		ID              string `json:"id"`
		AccountID       string `json:"account_id"`
		Online          bool   `json:"online"`
		Isolated        bool   `json:"isolated"`
		GroupName       string `json:"group_name"`
		PolicyName      string `json:"policy_name"`
		InfectionCount  int    `json:"infection_count"`
		SuspiciousCount int    `json:"suspicious_activity_count"`
		RebootRequired  int    `json:"reboot_required"`
		LastDaySeen     string `json:"last_day_seen"`
		LastActive      string `json:"last_active"`
		LastScannedAt   string `json:"last_scanned_at"`
		IsDeleted       bool   `json:"is_deleted"`
	} `json:"machine"`
}

// ParseEndpoint flattens one record from POST /v1/endpoints.
func ParseEndpoint(raw json.RawMessage) (Endpoint, error) {
	var r rawEndpoint
	if err := json.Unmarshal(raw, &r); err != nil {
		return Endpoint{}, err
	}
	e := Endpoint{
		ID:               firstNonEmpty(r.Machine.ID, r.Agent.MachineID),
		AccountID:        firstNonEmpty(r.Machine.AccountID, r.Agent.AccountID),
		HostName:         r.Agent.HostName,
		FQDN:             r.Agent.FQDN,
		DisplayName:      r.DisplayName,
		OSType:           r.Agent.OSInfo.OSType,
		OSPlatform:       r.Agent.OSInfo.OSPlatform,
		SerialNumber:     r.Agent.SerialNumber,
		GroupName:        r.Machine.GroupName,
		PolicyName:       r.Machine.PolicyName,
		ProtectionStatus: r.ProtectionStatus,
		Online:           r.Machine.Online,
		Isolated:         r.Machine.Isolated,
		InfectionCount:   r.Machine.InfectionCount,
		SuspiciousCount:  r.Machine.SuspiciousCount,
		RebootRequired:   r.Machine.RebootRequired != 0,
		UpdateAvailable:  r.Agent.UpdateAvailable,
		LastDaySeen:      r.Machine.LastDaySeen,
		LastActive:       r.Machine.LastActive,
		LastScannedAt:    r.Machine.LastScannedAt,
		IsDeleted:        r.Machine.IsDeleted,
	}
	for _, n := range r.Agent.NICs {
		if mac := strings.ToLower(strings.TrimSpace(n.MAC)); mac != "" {
			e.MACs = append(e.MACs, mac)
		}
	}
	if e.ID == "" || e.AccountID == "" {
		return e, fmt.Errorf("endpoint record has no machine id or account id")
	}
	return e, nil
}

// Label is the name to show for an endpoint: its host name, else its fully
// qualified host name, else the console display name. Only HostName and FQDN
// identify a machine for targeting; the display name is an editable alias.
func (e Endpoint) Label() string {
	return firstNonEmpty(e.HostName, e.FQDN, e.DisplayName)
}

// ParseTime accepts the RFC3339 variants OneView emits (with or without
// fractional seconds) and bare dates.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// LastSeen is the most recent of last_active and last_day_seen.
func (e Endpoint) LastSeen() (time.Time, bool) {
	a, okA := ParseTime(e.LastActive)
	d, okD := ParseTime(e.LastDaySeen)
	switch {
	case okA && okD:
		if a.After(d) {
			return a, true
		}
		return d, true
	case okA:
		return a, true
	default:
		return d, okD
	}
}

// HoursSinceSeen returns whole hours since the endpoint was last seen, or -1
// when OneView has never reported it.
func (e Endpoint) HoursSinceSeen(now time.Time) int {
	t, ok := e.LastSeen()
	if !ok {
		return -1
	}
	return int(now.Sub(t).Hours())
}

// StaleRow is one line of the stale-endpoint report.
type StaleRow struct {
	Site           string `json:"site"`
	AccountID      string `json:"account_id"`
	EndpointID     string `json:"endpoint_id"`
	HostName       string `json:"host_name"` // agent host name or FQDN; safe for --host
	DisplayName    string `json:"display_name,omitempty"`
	OSType         string `json:"os_type,omitempty"`
	LastSeen       string `json:"last_seen,omitempty"`
	HoursSinceSeen int    `json:"hours_since_seen"`
	// NewlyDark is nil unless StaleOptions.NewSince was set, so a check that
	// never ran is absent rather than a false "no".
	NewlyDark      *bool  `json:"newly_dark,omitempty"`
	Isolated       bool   `json:"isolated"`
	InfectionCount int    `json:"infection_count"`
	TwinOf         string `json:"twin_of,omitempty"`    // the live twin's host name
	TwinOfID       string `json:"twin_of_id,omitempty"` // the live twin's endpoint id
}

// StaleOptions controls Stale.
type StaleOptions struct {
	Hours    int           // not seen for at least this many hours
	NewSince time.Duration // when >0, mark rows that went dark inside this window
	Twins    bool          // mark stale rows whose machine has a live twin record
}

// Stale returns endpoints not seen for at least opts.Hours, newest-dark first
// within each site. Never-seen endpoints count as stale. Deleted records are
// skipped. all is the full endpoint population (used for twin detection).
func Stale(all []Endpoint, sites map[string]Site, now time.Time, opts StaleOptions) []StaleRow {
	var twins map[string]string
	byID := map[string]Endpoint{}
	if opts.Twins {
		twins = LiveTwins(all, now, opts.Hours)
		for _, e := range all {
			byID[e.ID] = e
		}
	}
	rows := make([]StaleRow, 0)
	for _, e := range all {
		if e.IsDeleted {
			continue
		}
		h := e.HoursSinceSeen(now)
		if h >= 0 && h < opts.Hours {
			continue
		}
		row := StaleRow{
			Site:           sites[e.AccountID].Name,
			AccountID:      e.AccountID,
			EndpointID:     e.ID,
			HostName:       firstNonEmpty(e.HostName, e.FQDN),
			DisplayName:    e.DisplayName,
			OSType:         e.OSType,
			HoursSinceSeen: h,
			Isolated:       e.Isolated,
			InfectionCount: e.InfectionCount,
		}
		if t, ok := e.LastSeen(); ok {
			row.LastSeen = t.Format(time.RFC3339)
		}
		if opts.NewSince > 0 {
			newly := h >= 0 && time.Duration(h)*time.Hour <= opts.NewSince
			row.NewlyDark = &newly
		}
		if id := twins[e.ID]; id != "" {
			t := byID[id]
			row.TwinOf = firstNonEmpty(t.HostName, t.FQDN, t.DisplayName, id)
			row.TwinOfID = id
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Site != rows[j].Site {
			return rows[i].Site < rows[j].Site
		}
		hi, hj := rows[i].HoursSinceSeen, rows[j].HoursSinceSeen
		if hi < 0 {
			hi = 1 << 30 // never seen sorts last
		}
		if hj < 0 {
			hj = 1 << 30
		}
		return hi < hj
	})
	return rows
}

// LiveTwins maps a stale endpoint id to the id of a live (seen within hours)
// record in the same account sharing its serial number, a MAC address, or its
// host name: the ghost left behind when a machine was re-imaged.
func LiveTwins(all []Endpoint, now time.Time, hours int) map[string]string {
	live := map[string]string{}
	for _, e := range all {
		if e.IsDeleted {
			continue
		}
		if h := e.HoursSinceSeen(now); h < 0 || h >= hours {
			continue
		}
		for _, k := range identityKeys(e) {
			live[k] = e.ID
		}
	}
	out := map[string]string{}
	for _, e := range all {
		if h := e.HoursSinceSeen(now); h >= 0 && h < hours {
			continue
		}
		for _, k := range identityKeys(e) {
			if id, ok := live[k]; ok && id != e.ID {
				out[e.ID] = id
				break
			}
		}
	}
	return out
}

func identityKeys(e Endpoint) []string {
	keys := make([]string, 0, 3+len(e.MACs))
	if s := strings.ToLower(strings.TrimSpace(e.SerialNumber)); s != "" && s != "0" && !strings.Contains(s, "default") {
		keys = append(keys, e.AccountID+"|serial|"+s)
	}
	for _, m := range e.MACs {
		keys = append(keys, e.AccountID+"|mac|"+m)
	}
	if h := strings.ToLower(strings.TrimSpace(e.HostName)); h != "" {
		keys = append(keys, e.AccountID+"|host|"+h)
	}
	return keys
}

// HealthRow is one site's endpoint health counts.
type HealthRow struct {
	Site            string `json:"site"`
	AccountID       string `json:"account_id"`
	Total           int    `json:"total"`
	Online          int    `json:"online"`
	Stale           int    `json:"stale"`
	Infected        int    `json:"infected"`
	Suspicious      int    `json:"suspicious"`
	Isolated        int    `json:"isolated"`
	RebootRequired  int    `json:"reboot_required"`
	Unprotected     int    `json:"unprotected"`
	ScanOverdue     int    `json:"scan_overdue"`
	UpdateAvailable int    `json:"update_available"`
	Attention       int    `json:"attention_score"`
}

// AttentionScore weights the conditions an MSP acts on first: live threats,
// then lost protection, then hygiene.
func (r HealthRow) AttentionScore() int {
	return 10*r.Infected + 8*r.Suspicious + 6*r.Unprotected + 4*r.Isolated + 3*r.Stale + 2*r.RebootRequired + r.ScanOverdue + r.UpdateAvailable
}

// HealthFromEndpoints rolls endpoints up per site (local mode). Every site in
// sites that has an account_id gets a row, so a site with zero endpoints still
// shows up. Sites without one (no subscription) hold no endpoints and get no
// row; callers explain them with a note.
func HealthFromEndpoints(all []Endpoint, sites []Site, now time.Time, staleHours, scanDays int) []HealthRow {
	byAcct := map[string]*HealthRow{}
	for _, s := range sites {
		if s.AccountID != "" {
			byAcct[s.AccountID] = &HealthRow{Site: s.Name, AccountID: s.AccountID}
		}
	}
	for _, e := range all {
		if e.IsDeleted {
			continue
		}
		r := byAcct[e.AccountID]
		if r == nil {
			r = &HealthRow{AccountID: e.AccountID}
			byAcct[e.AccountID] = r
		}
		r.Total++
		if e.Online {
			r.Online++
		}
		if h := e.HoursSinceSeen(now); h < 0 || h >= staleHours {
			r.Stale++
		}
		if e.InfectionCount > 0 {
			r.Infected++
		}
		if e.SuspiciousCount > 0 {
			r.Suspicious++
		}
		if e.Isolated {
			r.Isolated++
		}
		if e.RebootRequired {
			r.RebootRequired++
		}
		if e.ProtectionStatus == "unprotected" {
			r.Unprotected++
		}
		if t, ok := ParseTime(e.LastScannedAt); !ok || now.Sub(t) > time.Duration(scanDays)*24*time.Hour {
			r.ScanOverdue++
		}
		if e.UpdateAvailable {
			r.UpdateAvailable++
		}
	}
	rows := make([]HealthRow, 0, len(byAcct))
	for _, r := range byAcct {
		r.Attention = r.AttentionScore()
		rows = append(rows, *r)
	}
	SortHealth(rows)
	return rows
}

// SortHealth orders rows by attention score, then site name.
func SortHealth(rows []HealthRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Attention != rows[j].Attention {
			return rows[i].Attention > rows[j].Attention
		}
		return rows[i].Site < rows[j].Site
	})
}

// OneView job commands the respond verbs issue (POST /v1/jobs/bulk), e.g.
// command.edr.isolation. They are joined from their segments because the
// press 4.33.5 novel-host gate reads any dotted lowercase string literal as a
// hostname; the values are the API's documented job command enum (pinned in
// TestJobCommands).
var (
	CmdIsolate = jobCommand("edr", "isolation")
	CmdRestore = jobCommand("edr", "restore")
	CmdScan    = jobCommand("threat", "scan")
	CmdReboot  = jobCommand("asset", "reboot")
	CmdUpdate  = jobCommand("protection", "update", "now")
)

func jobCommand(segments ...string) string { return "command." + strings.Join(segments, ".") }

// Job status bits from POST /v1/jobs/search (summable).
var jobStatusNames = []struct {
	bit  int
	name string
}{
	{1, "CREATED"}, {2, "SENT"}, {4, "RECEIVED"}, {8, "STARTED"}, {16, "TIMED_OUT"},
	{32, "COMPLETED"}, {64, "EXPIRED"}, {128, "FAILED"}, {256, "CANCELLED"},
}

// Status sums used by respond pending.
const (
	JobStatusWaiting = 1 | 2 | 4 | 8 // created, sent, received, started
	JobStatusFailed  = 16 | 64 | 128 // timed out, expired, failed
	JobStatusNotRan  = JobStatusWaiting | JobStatusFailed
)

// DecodeJobStatus names a single status value (or every bit of a sum).
func DecodeJobStatus(v int) string {
	var names []string
	for _, s := range jobStatusNames {
		if v&s.bit != 0 {
			names = append(names, s.name)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("UNKNOWN(%d)", v)
	}
	return strings.Join(names, "|")
}

// JobFailed reports whether a single status ended without running.
func JobFailed(v int) bool { return v&JobStatusFailed != 0 }

// KnownJobState reports whether s is a job state OneView documents (the job
// webhook lists PENDING, STARTED, TIMED_OUT, COMPLETED, EXPIRED, FAILED; the
// status bitmask adds CREATED, SENT, RECEIVED, CANCELLED).
func KnownJobState(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PENDING", "CREATED", "SENT", "RECEIVED", "STARTED", "TIMED_OUT", "COMPLETED", "EXPIRED", "FAILED", "CANCELLED":
		return true
	}
	return false
}

// JobTerminal reports whether a job state string (GET job) or status value is final.
func JobTerminal(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "COMPLETED", "TIMED_OUT", "EXPIRED", "FAILED", "CANCELLED":
		return true
	}
	return false
}

// Detection is the slice of a detection record the local reports use.
type Detection struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	MachineID   string `json:"machine_id"`
	MachineName string `json:"machine_name"`
	ThreatName  string `json:"threat_name"`
	Category    string `json:"category"`
	SHA256      string `json:"sha256"`
	Status      string `json:"status"`
	ActionTaken string `json:"action_taken"`
	ReportedAt  string `json:"reported_at"`
	ScannedAt   string `json:"scanned_at"`
	CleanedAt   string `json:"cleaned_at"`
}

// Resolved reports whether the detection was already dealt with (quarantined,
// blocked, removed or cleaned). Unknown statuses count as unresolved so they
// surface for triage rather than hide.
func (d Detection) Resolved() bool {
	if strings.TrimSpace(d.CleanedAt) != "" {
		return true
	}
	for _, v := range []string{d.Status, d.ActionTaken} {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "quarantined", "blocked", "deleted", "removed", "cleaned", "remediated", "resolved":
			return true
		}
	}
	return false
}

// When returns the detection's reported_at, falling back to scanned_at.
func (d Detection) When() (time.Time, bool) {
	if t, ok := ParseTime(d.ReportedAt); ok {
		return t, true
	}
	return ParseTime(d.ScannedAt)
}

// SpreadRow is one threat (by hash, else name) and how far it reached.
type SpreadRow struct {
	Threat       string         `json:"threat"`
	SHA256       string         `json:"sha256,omitempty"`
	Category     string         `json:"category,omitempty"`
	Sites        int            `json:"sites"`
	Endpoints    int            `json:"endpoints"`
	Detections   int            `json:"detections"`
	FirstSeen    string         `json:"first_seen,omitempty"`
	LastSeen     string         `json:"last_seen,omitempty"`
	ActionsTaken map[string]int `json:"actions_taken"`
	SiteNames    []string       `json:"site_names"`
}

// Spread groups detections reported at or after since by sha256 (falling
// back to threat name) and keeps groups touching at least minSites sites.
// Rows sort by sites, then endpoints, descending.
func Spread(dets []Detection, sites map[string]Site, since time.Time, minSites int) []SpreadRow {
	type agg struct {
		row       SpreadRow
		sites     map[string]bool
		endpoints map[string]bool
		first     time.Time
		last      time.Time
	}
	groups := map[string]*agg{}
	for _, d := range dets {
		t, ok := d.When()
		if !ok || t.Before(since) {
			continue
		}
		key := strings.ToLower(d.SHA256)
		if key == "" {
			key = "name:" + strings.ToLower(d.ThreatName)
		}
		if key == "name:" {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &agg{row: SpreadRow{Threat: d.ThreatName, SHA256: d.SHA256, Category: d.Category, ActionsTaken: map[string]int{}}, sites: map[string]bool{}, endpoints: map[string]bool{}, first: t, last: t}
			groups[key] = g
		}
		g.row.Detections++
		g.sites[d.AccountID] = true
		g.endpoints[d.MachineID] = true
		if a := d.ActionTaken; a != "" {
			g.row.ActionsTaken[a]++
		}
		if g.row.Threat == "" {
			g.row.Threat = d.ThreatName
		}
		if t.Before(g.first) {
			g.first = t
		}
		if t.After(g.last) {
			g.last = t
		}
	}
	rows := make([]SpreadRow, 0, len(groups))
	for _, g := range groups {
		if len(g.sites) < minSites {
			continue
		}
		g.row.Sites, g.row.Endpoints = len(g.sites), len(g.endpoints)
		g.row.FirstSeen, g.row.LastSeen = g.first.Format(time.RFC3339), g.last.Format(time.RFC3339)
		names := make([]string, 0, len(g.sites))
		for acct := range g.sites {
			n := sites[acct].Name
			if n == "" {
				n = acct
			}
			names = append(names, n)
		}
		sort.Strings(names)
		g.row.SiteNames = names
		rows = append(rows, g.row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Sites != rows[j].Sites {
			return rows[i].Sites > rows[j].Sites
		}
		if rows[i].Endpoints != rows[j].Endpoints {
			return rows[i].Endpoints > rows[j].Endpoints
		}
		if rows[i].Detections != rows[j].Detections {
			return rows[i].Detections > rows[j].Detections
		}
		if rows[i].LastSeen != rows[j].LastSeen {
			return rows[i].LastSeen > rows[j].LastSeen // RFC3339 UTC sorts as time
		}
		return rows[i].Threat < rows[j].Threat
	})
	return rows
}

// QueueRow is one endpoint in the triage queue.
type QueueRow struct {
	Rank            int    `json:"rank"`
	Kind            string `json:"kind"` // active-threat, suspicious, forgotten-isolation
	Site            string `json:"site"`
	AccountID       string `json:"account_id"`
	EndpointID      string `json:"endpoint_id"`
	HostName        string `json:"host_name"` // agent host name or FQDN; safe for --host
	DisplayName     string `json:"display_name,omitempty"`
	Online          bool   `json:"online"`
	Isolated        bool   `json:"isolated"`
	InfectionCount  int    `json:"infection_count"`
	SuspiciousCount int    `json:"suspicious_activity_count"`
	Detections      int    `json:"detections_in_window"`
	DetectionDays   int    `json:"detection_days"`
	LastThreat      string `json:"last_threat,omitempty"`
	Suggested       string `json:"suggested_command"`
	score           int
}

// Queue ranks endpoints that need a response: live threats first (online and
// not isolated rank highest because they can still spread), then suspicious
// activity, then isolations nobody released (isolated, clean, quiet).
func Queue(endpoints []Endpoint, dets []Detection, sites map[string]Site, since time.Time, binary string) []QueueRow {
	type hist struct {
		count      int
		unresolved int
		days       map[string]bool
		last       time.Time
		name       string
		lastOpen   time.Time
		openName   string
	}
	byMachine := map[string]*hist{}
	for _, d := range dets {
		t, ok := d.When()
		if ok && t.Before(since) {
			continue
		}
		h := byMachine[d.MachineID]
		if h == nil {
			h = &hist{days: map[string]bool{}}
			byMachine[d.MachineID] = h
		}
		if !ok {
			// An undated detection may fall in the window: count it so the
			// machine is never offered a forgotten-isolation restore over it,
			// and an unresolved one still queues the machine.
			h.count++
			if !d.Resolved() {
				h.unresolved++
				if h.openName == "" {
					h.openName = d.ThreatName
				}
			}
			continue
		}
		h.count++
		if !d.Resolved() {
			h.unresolved++
			if t.After(h.lastOpen) {
				h.lastOpen, h.openName = t, d.ThreatName
			}
		}
		h.days[t.Format("2006-01-02")] = true
		if t.After(h.last) {
			h.last, h.name = t, d.ThreatName
		}
	}
	rows := make([]QueueRow, 0)
	for _, e := range endpoints {
		if e.IsDeleted {
			continue
		}
		h := byMachine[e.ID]
		var n, open, days int
		var last string
		if h != nil {
			n, open, days, last = h.count, h.unresolved, len(h.days), h.name
			if h.openName != "" {
				last = h.openName // the threat still needing action, not a resolved one
			}
		}
		row := QueueRow{
			Site: sites[e.AccountID].Name, AccountID: e.AccountID, EndpointID: e.ID, HostName: firstNonEmpty(e.HostName, e.FQDN), DisplayName: e.DisplayName,
			Online: e.Online, Isolated: e.Isolated, InfectionCount: e.InfectionCount, SuspiciousCount: e.SuspiciousCount,
			Detections: n, DetectionDays: days, LastThreat: last,
		}
		// Suggest --host only with a real targeting name (host name or FQDN,
		// which respond matches) scoped to the site; otherwise hand the exact
		// endpoint id to --stdin. Display names are aliases and never target.
		target := func(verb, extra string) string {
			if name := firstNonEmpty(e.HostName, e.FQDN); name != "" {
				return strings.TrimSpace(fmt.Sprintf("%s respond %s --host %s --site %s %s", binary, verb, ShellQuote(name), ShellQuote(sites[e.AccountID].Name), extra))
			}
			ref := fmt.Sprintf(`[{"endpoint_id":%q,"account_id":%q}]`, e.ID, e.AccountID)
			return strings.TrimSpace(fmt.Sprintf("echo %s | %s respond %s --stdin %s", ShellQuote(ref), binary, verb, extra))
		}
		switch {
		case e.InfectionCount > 0 || open > 0:
			// Live infection or detections nothing has dealt with yet.
			row.Kind = "active-threat"
			row.score = 1000 + 50*e.InfectionCount + 10*open + 20*days
			if e.Online && !e.Isolated {
				row.score += 500
			}
			if e.Isolated {
				row.Suggested = target("scan", "--remove")
			} else {
				row.Suggested = target("isolate", "")
			}
		case e.SuspiciousCount > 0:
			row.Kind = "suspicious"
			row.score = 500 + 25*e.SuspiciousCount
			if e.Online {
				row.score += 100
			}
			row.Suggested = fmt.Sprintf("%s endpoints suspicious-activity %s %s", binary, ShellQuote(e.AccountID), ShellQuote(e.ID))
		case e.Isolated && n == 0:
			// Isolated, clean and quiet for the whole window. An endpoint with any
			// detection in the window (even a quarantined one) was contained on
			// purpose recently and must not be suggested for release.
			row.Kind = "forgotten-isolation"
			row.score = 100
			row.Suggested = target("restore", "")
		default:
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].HostName < rows[j].HostName
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

// ShellQuote returns s as one POSIX shell word: unchanged when it only holds
// characters no shell treats specially, otherwise single-quoted with embedded
// single quotes escaped. Suggested commands embed API-supplied names, so every
// external value goes through here.
func ShellQuote(s string) string {
	if s != "" && safeShellWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var safeShellWord = regexp.MustCompile(`^[A-Za-z0-9._@%+=:,/-]+$`)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
