// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: turn per-PBX XAPI bodies into fleet report rows.

package fleet

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Level is a check outcome. Unknown means the PBX did not report what the
// check needs (a null or missing field): not known healthy, so never a pass.
// Error means the PBX could not be read at all.
type Level string

const (
	Pass    Level = "pass"
	Warn    Level = "warn"
	Unknown Level = "unknown"
	Fail    Level = "fail"
	Error   Level = "error"
)

var levelRank = map[Level]int{Pass: 0, Warn: 1, Unknown: 2, Fail: 3, Error: 4}

// Worse reports whether a is more severe than b.
func Worse(a, b Level) bool { return levelRank[a] > levelRank[b] }

// Check is one health test on one PBX.
type Check struct {
	Name   string `json:"name"`
	Level  Level  `json:"level"`
	Detail string `json:"detail"`
}

// HealthOptions tunes the health thresholds.
type HealthOptions struct {
	Now         time.Time
	WarnDays    int           // licence/maintenance expiry inside this window warns
	BackupStale time.Duration // last backup older than this fails
}

// HealthRow is one PBX on the health board.
type HealthRow struct {
	PBX                  string   `json:"pbx"`
	FQDN                 string   `json:"fqdn"`
	Status               Level    `json:"status"`
	Problems             string   `json:"problems"`
	Version              string   `json:"version,omitempty"`
	Edition              string   `json:"edition,omitempty"`
	LicenseExpires       string   `json:"license_expires,omitempty"`
	LicenseDaysLeft      *int     `json:"license_days_left,omitempty"`
	MaintenanceExpires   string   `json:"maintenance_expires,omitempty"`
	MaintenanceDaysLeft  *int     `json:"maintenance_days_left,omitempty"`
	LastBackup           string   `json:"last_backup,omitempty"`
	BackupAgeHours       *int     `json:"backup_age_hours,omitempty"`
	TrunksRegistered     *int     `json:"trunks_registered,omitempty"`
	TrunksTotal          *int     `json:"trunks_total,omitempty"`
	ExtensionsRegistered *int     `json:"extensions_registered,omitempty"`
	ExtensionsTotal      *int     `json:"extensions_total,omitempty"`
	CallsActive          *int     `json:"calls_active,omitempty"`
	MaxSimCalls          *int     `json:"max_sim_calls,omitempty"`
	DiskFreePct          *float64 `json:"disk_free_pct,omitempty"`
	Checks               []Check  `json:"checks"`
	Error                string   `json:"error,omitempty"`
}

// ErrorHealthRow is a PBX that could not be read.
func ErrorHealthRow(p PBX, err error) HealthRow {
	return HealthRow{PBX: p.Name, FQDN: p.FQDN, Status: Error, Problems: "unreachable", Checks: []Check{}, Error: err.Error()}
}

// EvaluateHealth scores one PBX's SystemStatus.
func EvaluateHealth(p PBX, s SystemStatus, o HealthOptions) HealthRow {
	row := HealthRow{
		PBX: p.Name, FQDN: p.FQDN, Version: s.Version, Edition: s.ProductCode,
		TrunksRegistered: s.TrunksRegistered, TrunksTotal: s.TrunksTotal,
		ExtensionsRegistered: s.ExtensionsRegistered, ExtensionsTotal: s.ExtensionsTotal,
		CallsActive: s.CallsActive, MaxSimCalls: s.MaxSimCalls,
	}
	add := func(name string, level Level, format string, a ...any) {
		row.Checks = append(row.Checks, Check{Name: name, Level: level, Detail: fmt.Sprintf(format, a...)})
	}
	// settled adds a check under the every-input rule (Settle): missing
	// names the inputs the check uses that the PBX did not report.
	settled := func(name string, level Level, detail string, missing []string) {
		level, detail = Settle(level, detail, missing...)
		row.Checks = append(row.Checks, Check{Name: name, Level: level, Detail: detail})
	}

	// Licence: inactive or expired is an outage risk; expiring soon warns.
	// The expiry date is recorded whatever the verdict, so a renewal of an
	// inactive licence still shows up in 'fleet changes'.
	exp, ok := ParseTime(s.ExpirationDate)
	if ok {
		days := DaysUntil(exp, o.Now)
		row.LicenseExpires, row.LicenseDaysLeft = exp.UTC().Format(time.RFC3339), &days
	}
	{
		level, detail := Pass, "active"
		switch {
		case s.LicenseActive != nil && !*s.LicenseActive:
			level, detail = Fail, "licence is not active"
		case ok && *row.LicenseDaysLeft < 0:
			level, detail = Fail, fmt.Sprintf("expired %d days ago", -*row.LicenseDaysLeft)
		case ok && *row.LicenseDaysLeft <= o.WarnDays:
			level, detail = Warn, fmt.Sprintf("expires in %d days", *row.LicenseDaysLeft)
		case ok:
			detail = fmt.Sprintf("%d days left", *row.LicenseDaysLeft)
		}
		settled("license", level, detail, unreported("licence activation", s.LicenseActive != nil, "licence expiry", ok))
	}

	// Maintenance: lapsed maintenance blocks 3CX updates but not calls.
	if exp, ok := ParseTime(s.MaintenanceExpiresAt); ok {
		days := DaysUntil(exp, o.Now)
		row.MaintenanceExpires, row.MaintenanceDaysLeft = exp.UTC().Format(time.RFC3339), &days
		switch {
		case days < 0:
			add("maintenance", Warn, "expired %d days ago (updates blocked)", -days)
		case days <= o.WarnDays:
			add("maintenance", Warn, "expires in %d days", days)
		default:
			add("maintenance", Pass, "%d days left", days)
		}
	} else {
		settled("maintenance", Pass, "", []string{"maintenance expiry"})
	}

	// Backup: never or stale fails; unscheduled warns.
	if last, ok := ParseTime(s.LastBackupDateTime); ok {
		row.LastBackup = last.UTC().Format(time.RFC3339)
		age := int(o.Now.Sub(last).Hours())
		row.BackupAgeHours = &age
		level, detail := Pass, "last backup "+humanAge(o.Now.Sub(last))+" ago"
		switch {
		case o.Now.Sub(last) > o.BackupStale:
			level = Fail
		case s.BackupScheduled != nil && !*s.BackupScheduled:
			level, detail = Warn, detail+" but no backup is scheduled"
		}
		settled("backup", level, detail, unreported("backup schedule", s.BackupScheduled != nil))
	} else {
		// No completed backup reported: never healthy (fail, which Settle
		// keeps), whether the PBX never backed up or did not say.
		settled("backup", Fail, "no completed backup reported", []string{"last backup"})
	}

	// Trunks: any configured trunk not registered fails.
	if s.TrunksTotal != nil && s.TrunksRegistered != nil {
		switch {
		case *s.TrunksTotal == 0:
			add("trunks", Warn, "no trunks configured")
		case *s.TrunksRegistered < *s.TrunksTotal:
			add("trunks", Fail, "%d of %d trunks registered", *s.TrunksRegistered, *s.TrunksTotal)
		default:
			add("trunks", Pass, "%d of %d trunks registered", *s.TrunksRegistered, *s.TrunksTotal)
		}
	} else {
		settled("trunks", Pass, "", unreported("trunks registered", s.TrunksRegistered != nil, "trunks total", s.TrunksTotal != nil))
	}

	// Extensions: phones go off at night, so a partial count is ordinary; none or
	// under half registered is not.
	// ponytail: fixed one-half line, a flag when an MSP needs another.
	if s.ExtensionsTotal != nil && s.ExtensionsRegistered != nil {
		reg, total := *s.ExtensionsRegistered, *s.ExtensionsTotal
		level := Pass
		switch {
		case total > 0 && reg == 0:
			level = Fail
		case reg*2 < total:
			level = Warn
		}
		add("extensions", level, "%d of %d extensions registered", reg, total)
	} else {
		settled("extensions", Pass, "", unreported("extensions registered", s.ExtensionsRegistered != nil, "extensions total", s.ExtensionsTotal != nil))
	}
	settled("version", Pass, s.Version, unreported("version", s.Version != ""))

	// Every check is always emitted, pass included, so 'fleet changes' sees
	// it move; a null flag is unknown, never healthy.
	// flag turns one reported boolean into a check.
	flag := func(name string, v *bool, bad Level, badMsg, okMsg string) {
		switch {
		case v == nil:
			settled(name, Pass, "", []string{name})
		case *v:
			add(name, bad, "%s", badMsg)
		default:
			add(name, Pass, "%s", okMsg)
		}
	}
	flag("services", s.HasNotRunningServices, Fail, "one or more 3CX services are not running", "all services running")
	flag("system_extensions", s.HasUnregisteredSystemExtensions, Warn, "system extensions are unregistered", "all registered")

	// Disk: under 10% free fails, under 20% warns. Both values must be
	// reported; a null free-space value is unknown, not 0%.
	if s.TotalDiskSpace != nil && s.FreeDiskSpace != nil && *s.TotalDiskSpace > 0 {
		pct := float64(*s.FreeDiskSpace) * 100 / float64(*s.TotalDiskSpace)
		pct = float64(int(pct*10)) / 10
		row.DiskFreePct = &pct
		switch {
		case pct < 10:
			add("disk", Fail, "%.1f%% free", pct)
		case pct < 20:
			add("disk", Warn, "%.1f%% free", pct)
		default:
			add("disk", Pass, "%.1f%% free", pct)
		}
	} else {
		settled("disk", Pass, "", unreported("total disk space", s.TotalDiskSpace != nil && *s.TotalDiskSpace > 0, "free disk space", s.FreeDiskSpace != nil))
	}

	// Recording and voicemail silently stop when quotas fill.
	// Warn on any observed stop; pass only when all four flags were
	// reported false. Partial telemetry gives no verdict.
	var stopped []string
	quotaMissing := unreported("recording stopped", s.RecordingStopped != nil, "recording quota", s.RecordingQuotaReached != nil,
		"voicemail stopped", s.VoicemailStopped != nil, "voicemail quota", s.VoicemailQuotaReached != nil)
	for _, f := range []struct {
		name string
		vals []*bool
	}{{"recordings", []*bool{s.RecordingStopped, s.RecordingQuotaReached}}, {"voicemail", []*bool{s.VoicemailStopped, s.VoicemailQuotaReached}}} {
		for _, v := range f.vals {
			if v != nil && *v {
				stopped = append(stopped, f.name)
				break
			}
		}
	}
	if len(stopped) > 0 {
		settled("storage_quota", Warn, strings.Join(stopped, " and ")+" stopped or at quota", quotaMissing)
	} else {
		settled("storage_quota", Pass, "recordings and voicemail running", quotaMissing)
	}

	flag("phone_firmware", s.Firmware3CXOutdated, Warn, "3CX-supplied phone firmware is outdated", "current")
	if s.MaxSimCalls != nil && s.CallsActive != nil && *s.MaxSimCalls > 0 {
		if *s.CallsActive >= *s.MaxSimCalls {
			add("capacity", Warn, "%d active calls at the %d simultaneous-call licence limit", *s.CallsActive, *s.MaxSimCalls)
		} else {
			add("capacity", Pass, "%d of %d simultaneous calls in use", *s.CallsActive, *s.MaxSimCalls)
		}
	} else {
		// A limit of 0 says nothing about capacity: not reported.
		settled("capacity", Pass, "", unreported("simultaneous-call limit", s.MaxSimCalls != nil && *s.MaxSimCalls > 0, "active calls", s.CallsActive != nil))
	}

	row.Status, row.Problems = summarize(row.Checks)
	return row
}

func summarize(checks []Check) (Level, string) {
	status := Pass
	var problems []string
	for _, c := range checks {
		if Worse(c.Level, status) {
			status = c.Level
		}
		if c.Level != Pass {
			problems = append(problems, c.Name+":"+string(c.Level))
		}
	}
	return status, strings.Join(problems, ", ")
}

func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// SortHealth puts the worst PBXes first, then by name.
func SortHealth(rows []HealthRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Status != rows[j].Status {
			return Worse(rows[i].Status, rows[j].Status)
		}
		return rows[i].PBX < rows[j].PBX
	})
}

// TrunkRow is one trunk on the fleet trunk board.
type TrunkRow struct {
	PBX                string `json:"pbx"`
	State              string `json:"state"` // online | offline | issue | unknown | error
	Trunk              string `json:"trunk,omitempty"`
	Name               string `json:"name,omitempty"`
	Host               string `json:"host,omitempty"`
	Online             *bool  `json:"online,omitempty"`
	ConfigurationIssue string `json:"configuration_issue,omitempty"`
	// IssueUnknown: the phone system did not report whether the trunk has a
	// configuration issue, so the trunk is not known to be healthy.
	IssueUnknown      bool   `json:"configuration_issue_unknown,omitempty"`
	Direction         string `json:"direction,omitempty"`
	SimultaneousCalls *int   `json:"simultaneous_calls,omitempty"`
	CertExpires       string `json:"cert_expires,omitempty"`
	CertDaysLeft      *int   `json:"cert_days_left,omitempty"`
	Error             string `json:"error,omitempty"`
}

// NeedsAttention is any trunk not known to be healthy: down, unknown
// registration state, or a PBX that could not be read.
func (t TrunkRow) NeedsAttention() bool { return trunkState(t) != "online" }

func trunkState(t TrunkRow) string {
	switch {
	case t.Error != "":
		return "error"
	case t.ConfigurationIssue != "":
		return "issue"
	}
	state := registration(t)
	if t.IssueUnknown && state == "online" {
		return "unknown"
	}
	return state
}

// registration is online, offline, or unknown, ignoring configuration issues.
func registration(t TrunkRow) string {
	switch {
	case t.Online == nil:
		return "unknown"
	case *t.Online:
		return "online"
	}
	return "offline"
}

// ErrorTrunkRow is a PBX whose trunks could not be read.
func ErrorTrunkRow(p PBX, err error) TrunkRow {
	return TrunkRow{PBX: p.Name, State: "error", Error: err.Error()}
}

// TrunkRows builds the board rows for one PBX.
func TrunkRows(p PBX, trunks []Trunk, now time.Time) []TrunkRow {
	out := make([]TrunkRow, 0, len(trunks))
	for _, t := range trunks {
		row := TrunkRow{PBX: p.Name, Trunk: t.Number, Online: t.IsOnline, IssueUnknown: t.ConfigurationIssue == nil,
			Direction: t.Direction, SimultaneousCalls: t.SimultaneousCalls}
		if t.ConfigurationIssue != nil {
			row.ConfigurationIssue = strings.TrimSpace(*t.ConfigurationIssue)
		}
		if t.Gateway != nil {
			row.Name, row.Host = t.Gateway.Name, t.Gateway.Host
		}
		if exp, ok := ParseTime(t.CertificateExpirationDate); ok {
			days := DaysUntil(exp, now)
			row.CertExpires = exp.UTC().Format("2006-01-02")
			row.CertDaysLeft = &days
		}
		row.State = trunkState(row)
		out = append(out, row)
	}
	return out
}

// SortTrunks puts errors and down trunks first, then the nearest cert expiry.
func SortTrunks(rows []TrunkRow) {
	rank := func(r TrunkRow) int {
		switch trunkState(r) {
		case "error":
			return 0
		case "offline", "issue":
			return 1
		case "unknown":
			return 2
		}
		return 3
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if ri, rj := rank(rows[i]), rank(rows[j]); ri != rj {
			return ri < rj
		}
		ci, cj := rows[i].CertDaysLeft, rows[j].CertDaysLeft
		if (ci == nil) != (cj == nil) {
			return ci != nil
		}
		if ci != nil && *ci != *cj {
			return *ci < *cj
		}
		if rows[i].PBX != rows[j].PBX {
			return rows[i].PBX < rows[j].PBX
		}
		return rows[i].Trunk < rows[j].Trunk
	})
}

// LicenceRow is one PBX in the renewals pipeline.
type LicenceRow struct {
	PBX                 string `json:"pbx"`
	Company             string `json:"company,omitempty"`
	Edition             string `json:"edition,omitempty"`
	LicenseActive       *bool  `json:"license_active,omitempty"`
	LicenseExpires      string `json:"license_expires,omitempty"`
	LicenseDaysLeft     *int   `json:"license_days_left,omitempty"`
	MaintenanceExpires  string `json:"maintenance_expires,omitempty"`
	MaintenanceDaysLeft *int   `json:"maintenance_days_left,omitempty"`
	MaxSimCalls         *int   `json:"max_sim_calls,omitempty"`
	UserExtensions      *int   `json:"user_extensions,omitempty"`
	MaxUserExtensions   *int   `json:"max_user_extensions,omitempty"`
	SeatHeadroom        *int   `json:"seat_headroom,omitempty"`
	Reseller            string `json:"reseller,omitempty"`
	ResellerMismatch    bool   `json:"reseller_mismatch,omitempty"`
	// Unknown names the renewal inputs the PBX did not report
	// (license_active, license_expires, maintenance_expires).
	Unknown []string `json:"unknown,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// LicenceRowFor joins licence status and seat usage for one PBX. Licence
// activation is false when either source says so (any failure signal wins).
func LicenceRowFor(p PBX, l LicenseStatus, s SystemStatus, reseller string, now time.Time) LicenceRow {
	active := l.LicenseActive
	if s.LicenseActive != nil && (active == nil || !*s.LicenseActive) {
		active = s.LicenseActive
	}
	row := LicenceRow{PBX: p.Name, Company: l.CompanyName, Edition: FirstNonEmpty(l.ProductCode, s.ProductCode),
		LicenseActive: active, MaxSimCalls: l.MaxSimCalls, Reseller: FirstNonEmpty(l.ResellerName, s.ResellerName),
		UserExtensions: s.UserExtensions, MaxUserExtensions: s.MaxUserExtensions}
	if exp, ok := ParseTime(FirstNonEmpty(l.ExpirationDate, s.ExpirationDate)); ok {
		d := DaysUntil(exp, now)
		row.LicenseExpires, row.LicenseDaysLeft = exp.UTC().Format("2006-01-02"), &d
	}
	if exp, ok := ParseTime(FirstNonEmpty(l.MaintenanceExpiresAt, s.MaintenanceExpiresAt)); ok {
		d := DaysUntil(exp, now)
		row.MaintenanceExpires, row.MaintenanceDaysLeft = exp.UTC().Format("2006-01-02"), &d
	}
	row.Unknown = unreported("license_active", row.LicenseActive != nil, "license_expires", row.LicenseDaysLeft != nil,
		"maintenance_expires", row.MaintenanceDaysLeft != nil)
	// MaxUserExtensions 0 means the edition has no user cap.
	if s.MaxUserExtensions != nil && s.UserExtensions != nil && *s.MaxUserExtensions > 0 {
		h := *s.MaxUserExtensions - *s.UserExtensions
		row.SeatHeadroom = &h
	}
	if reseller != "" {
		row.ResellerMismatch = !strings.EqualFold(strings.TrimSpace(row.Reseller), strings.TrimSpace(reseller))
	}
	return row
}

// OutsideWindow reports a PBX known to need no renewal within window days:
// readable, every renewal input reported (Unknown empty), the licence active,
// and both expiries beyond the window. Anything else stays in a --within
// report (the every-input rule, Settle).
func (r LicenceRow) OutsideWindow(window int) bool {
	return r.Error == "" && len(r.Unknown) == 0 && r.LicenseActive != nil && *r.LicenseActive &&
		r.LicenseDaysLeft != nil && *r.LicenseDaysLeft > window &&
		r.MaintenanceDaysLeft != nil && *r.MaintenanceDaysLeft > window
}

// SoonestDays is the nearer of the licence and maintenance expiries.
func (r LicenceRow) SoonestDays() (int, bool) {
	switch {
	case r.LicenseDaysLeft != nil && r.MaintenanceDaysLeft != nil:
		return min(*r.LicenseDaysLeft, *r.MaintenanceDaysLeft), true
	case r.LicenseDaysLeft != nil:
		return *r.LicenseDaysLeft, true
	case r.MaintenanceDaysLeft != nil:
		return *r.MaintenanceDaysLeft, true
	}
	return 0, false
}

// SortLicences puts errors first, then the soonest expiry.
func SortLicences(rows []LicenceRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		ei, ej := rows[i].Error != "", rows[j].Error != ""
		if ei != ej {
			return ei
		}
		di, oki := rows[i].SoonestDays()
		dj, okj := rows[j].SoonestDays()
		if oki != okj {
			return oki
		}
		if oki && di != dj {
			return di < dj
		}
		return rows[i].PBX < rows[j].PBX
	})
}

// VersionRow is one PBX on the version drift report.
//
// A verdict the PBX did not report enough to make is null (behind,
// maintenance_expired) and named in unknown; unknown_updates counts update
// entries whose out-of-date state was not reported. None of them reads as
// current.
type VersionRow struct {
	PBX                string   `json:"pbx"`
	Version            string   `json:"version,omitempty"`
	NewestInFleet      string   `json:"newest_in_fleet,omitempty"`
	Behind             *bool    `json:"behind"`
	PendingUpdates     int      `json:"pending_updates"`
	UnknownUpdates     int      `json:"unknown_updates,omitempty"`
	AutoUpdate         *bool    `json:"auto_update,omitempty"`
	MaintenanceExpired *bool    `json:"maintenance_expired"`
	UpdatesRunning     *bool    `json:"updates_running,omitempty"`
	PhoneFirmwareOld   bool     `json:"phone_firmware_outdated,omitempty"`
	LastUpdate         string   `json:"last_update,omitempty"`
	LastCheck          string   `json:"last_check,omitempty"`
	Unknown            []string `json:"unknown,omitempty"`
	Error              string   `json:"error,omitempty"`
}

// NeedsAttention is a PBX not known to be current: unreadable, behind,
// with pending or unknown updates, expired maintenance, or any verdict it
// did not report enough to make.
func (r VersionRow) NeedsAttention() bool {
	return r.Error != "" || (r.Behind != nil && *r.Behind) || r.PendingUpdates > 0 || r.UnknownUpdates > 0 ||
		(r.MaintenanceExpired != nil && *r.MaintenanceExpired) || len(r.Unknown) > 0
}

// MarshalJSON leaves the drift verdicts out of an error row: a PBX that
// could not be read is not known to be current, so behind, pending_updates
// and maintenance_expired must not read as false or 0.
func (r VersionRow) MarshalJSON() ([]byte, error) {
	type plain VersionRow
	if r.Error == "" {
		return json.Marshal(plain(r))
	}
	return json.Marshal(struct {
		PBX   string `json:"pbx"`
		Error string `json:"error"`
	}{r.PBX, r.Error})
}

// VersionRowFor reads one PBX; NewestInFleet/Behind are filled by MarkDrift.
// Maintenance is expired when GetUpdates says so or SystemStatus's
// maintenance expiry has passed (any failure signal wins); it is current
// only when both sources reported it current, and unknown otherwise (the
// every-input rule, Settle). An update entry counts as pending unless it is
// reported current or ignored, and as unknown when its state is null; a
// PBX that reported no update list at all is unknown, never "0 pending".
func VersionRowFor(p PBX, s SystemStatus, u UpdateList, now time.Time) VersionRow {
	row := VersionRow{PBX: p.Name, Version: s.Version, AutoUpdate: s.AutoUpdateEnabled,
		UpdatesRunning: u.UpdatesRunning, PhoneFirmwareOld: s.Firmware3CXOutdated != nil && *s.Firmware3CXOutdated}
	var byDate *bool
	if exp, ok := ParseTime(s.MaintenanceExpiresAt); ok {
		expired := !exp.After(now)
		byDate = &expired
	}
	switch {
	case (u.IsMaintananceExpired != nil && *u.IsMaintananceExpired) || (byDate != nil && *byDate):
		row.MaintenanceExpired = boolPtr(true)
	case u.IsMaintananceExpired != nil && byDate != nil:
		row.MaintenanceExpired = boolPtr(false)
	default:
		row.Unknown = append(row.Unknown, "maintenance_expired")
	}
	if strings.TrimSpace(s.Version) == "" {
		row.Unknown = append(row.Unknown, "version")
	}
	for _, e := range u.Entries {
		switch {
		case e.Ignore != nil && *e.Ignore:
		case e.OutOfDate == nil:
			row.UnknownUpdates++
		case *e.OutOfDate:
			row.PendingUpdates++
		}
	}
	if row.UnknownUpdates > 0 {
		row.Unknown = append(row.Unknown, "update_entries")
	}
	if u.Entries == nil {
		row.Unknown = append(row.Unknown, "update_list")
	}
	if t, ok := ParseTime(FirstNonEmpty(u.LastSuccessfulUpdate, s.LastSuccessfulUpdate)); ok {
		row.LastUpdate = t.UTC().Format(time.RFC3339)
	}
	if t, ok := ParseTime(s.LastCheckForUpdates); ok {
		row.LastCheck = t.UTC().Format(time.RFC3339)
	}
	return row
}

// MarkDrift sets the fleet-relative newest build and who is behind it.
func MarkDrift(rows []VersionRow) {
	newest := ""
	for _, r := range rows {
		if r.Error == "" && r.Version != "" && (newest == "" || CompareVersions(r.Version, newest) > 0) {
			newest = r.Version
		}
	}
	for i := range rows {
		if rows[i].Error != "" || rows[i].Version == "" {
			continue
		}
		rows[i].NewestInFleet = newest
		rows[i].Behind = boolPtr(CompareVersions(rows[i].Version, newest) < 0)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ei, ej := rows[i].Error != "", rows[j].Error != ""
		if ei != ej {
			return ei
		}
		if ai, aj := rows[i].NeedsAttention(), rows[j].NeedsAttention(); ai != aj {
			return ai
		}
		if c := CompareVersions(rows[i].Version, rows[j].Version); c != 0 {
			return c < 0
		}
		return rows[i].PBX < rows[j].PBX
	})
}

// BackupRow is one PBX on the backup assurance report.
type BackupRow struct {
	PBX             string   `json:"pbx"`
	Status          string   `json:"status"`
	Flags           []string `json:"flags"`
	LastBackup      string   `json:"last_backup,omitempty"`
	AgeHours        *int     `json:"age_hours,omitempty"`
	ScheduleEnabled *bool    `json:"schedule_enabled,omitempty"`
	Schedule        string   `json:"schedule,omitempty"`
	Rotation        *int     `json:"rotation,omitempty"`
	IncludesRecords *bool    `json:"includes_recordings,omitempty"`
	Encrypted       *bool    `json:"encrypted,omitempty"`
	NewestFile      string   `json:"newest_file,omitempty"`
	NewestSizeMB    *float64 `json:"newest_size_mb,omitempty"`
	PreviousSizeMB  *float64 `json:"previous_size_mb,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// BackupRowFor joins status, settings, and the backup file list.
func BackupRowFor(p PBX, s SystemStatus, set BackupSettings, files []Backup, stale time.Duration, now time.Time) BackupRow {
	row := BackupRow{PBX: p.Name, Flags: []string{}, ScheduleEnabled: set.ScheduleEnabled, Rotation: set.Rotation}
	if set.Schedule != nil {
		row.Schedule = strings.TrimSpace(strings.Join([]string{set.Schedule.ScheduleType, set.Schedule.Day, set.Schedule.Time}, " "))
	}
	if set.Contents != nil {
		row.IncludesRecords, row.Encrypted = set.Contents.Recordings, set.Contents.EncryptBackup
	}
	sort.SliceStable(files, func(i, j int) bool {
		ti, _ := ParseTime(files[i].CreationTime)
		tj, _ := ParseTime(files[j].CreationTime)
		return ti.After(tj)
	})
	last, ok := ParseTime(s.LastBackupDateTime)
	if !ok && len(files) > 0 {
		last, ok = ParseTime(files[0].CreationTime)
	}
	if ok {
		age := now.Sub(last)
		h := int(age.Hours())
		row.LastBackup, row.AgeHours = last.UTC().Format(time.RFC3339), &h
		if age > stale {
			row.Flags = append(row.Flags, "STALE")
		}
	} else {
		row.Flags = append(row.Flags, "NEVER")
	}
	switch {
	case (set.ScheduleEnabled != nil && !*set.ScheduleEnabled) || (set.ScheduleEnabled == nil && s.BackupScheduled != nil && !*s.BackupScheduled):
		row.Flags = append(row.Flags, "UNSCHEDULED")
	case set.ScheduleEnabled == nil && s.BackupScheduled == nil:
		// Neither the backup settings nor the status say a backup is
		// scheduled: not known to run again.
		row.Flags = append(row.Flags, "SCHEDULE_UNKNOWN")
	}
	if len(files) == 0 && ok {
		// The PBX reports a completed backup but lists no backup file to
		// restore from (deleted, or rotated away).
		row.Flags = append(row.Flags, "NO_FILES")
	}
	if len(files) > 0 {
		row.NewestFile = files[0].FileName
		// Judge the newest file that can actually be restored, not only the
		// last completion timestamp (the newest file may have been deleted).
		if newest, ok := ParseTime(files[0].CreationTime); !ok || now.Sub(newest) > stale {
			row.Flags = append(row.Flags, "FILE_STALE")
		}
		if files[0].Size == nil {
			// The newest file's size is not reported: it cannot be judged
			// complete (an empty or shrunk backup looks the same).
			row.Flags = append(row.Flags, "SIZE_UNKNOWN")
		} else {
			mb := float64(*files[0].Size) / 1e6
			row.NewestSizeMB = &mb
		}
		if len(files) > 1 && files[1].Size == nil && files[0].Size != nil {
			// The previous file's size is not reported: whether the newest
			// one shrank cannot be judged (the every-input rule, Settle).
			row.Flags = append(row.Flags, "SIZE_UNKNOWN")
		}
		if len(files) > 1 && files[1].Size != nil && *files[1].Size > 0 {
			prev := float64(*files[1].Size) / 1e6
			row.PreviousSizeMB = &prev
			// A newest backup under half the previous one usually means
			// recordings or voicemail were silently dropped from it.
			if files[0].Size != nil && *files[0].Size*2 < *files[1].Size {
				row.Flags = append(row.Flags, "SHRUNK")
			}
		}
	}
	row.Status = "ok"
	if len(row.Flags) > 0 {
		row.Status = strings.ToLower(row.Flags[0])
	}
	return row
}

func boolPtr(b bool) *bool { return &b }

// FirstNonEmpty returns the first non-blank string.
func FirstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
