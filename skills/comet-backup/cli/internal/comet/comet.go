// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package comet holds the hand-written fleet logic behind the novel commands:
// decoding Comet Server admin responses into credential-free projections and
// computing health, usage, device and change views from them.
//
// Field names and constants come from the official Comet Go SDK
// (github.com/CometBackup/comet-go-sdk, cometsdk.go) and the API constants page
// (https://docs.cometbackup.com/latest/api/api-constants). Profiles are decoded
// into narrow structs on purpose: PasswordHash, TOTPKey, PasswordRecovery and
// Storage Vault credentials have no field here, so they can never be printed
// or stored by these commands.
package comet

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Job is the subset of BackupJobDetail the reports use.
type Job struct {
	GUID                    string
	Username                string
	Classification          int64
	Status                  int64
	StartTime               int64
	EndTime                 int64
	SourceGUID              string
	DestinationGUID         string
	DeviceID                string
	BackupRuleGUID          string
	ClientVersion           string
	TotalSize               int64
	UploadSize              int64
	TotalVmCount            int64
	TotalAccountsCount      int64
	TotalLicensedMailsCount int64
}

// present reports whether j carries a real job record (Comet sends a zero
// BackupJobDetail when an item has never run).
func (j Job) present() bool { return j.GUID != "" || j.StartTime > 0 }

type Schedule struct {
	FrequencyType   uint64
	SecondsPast     uint64
	Offset          int64
	RestrictRuntime bool
	FromTime        HourMinute
	ToTime          HourMinute
	RestrictDays    bool
	RandomDelaySecs uint64
	DaysSelect      struct {
		Sunday, Monday, Tuesday, Wednesday, Thursday, Friday, Saturday bool
	}
}

// HourMinute is Comet's HourSchedConfig.
type HourMinute struct {
	Hour    uint64
	Minutes uint64
}

type Rule struct {
	Description string
	Source      string
	Destination string
	Schedules   []Schedule
}

type Source struct {
	Engine      string
	Description string
	OwnerDevice string
	CreateTime  int64
	Statistics  struct {
		LastStartTime           int64
		LastBackupJob           Job
		LastSuccessfulBackupJob Job
	}
}

type Vault struct {
	Description         string
	DestinationType     uint64
	StorageLimitEnabled bool
	StorageLimitBytes   int64
	StorageLimitID      string
	Statistics          struct {
		ClientProvidedSize struct {
			Size             int64
			MeasureCompleted int64
		}
		LastSuccessfulDeepVerify_EndTime int64
	}
}

type Device struct {
	DeviceTimezone   string
	FriendlyName     string
	RegistrationTime int64
	ClientVersion    string
	PlatformVersion  struct {
		Distribution string `json:"distribution"`
		OS           string `json:"os"`
	}
}

// Profile is a credential-free projection of UserProfileConfig.
type Profile struct {
	Username                        string
	AccountName                     string
	LocalTimezone                   string
	OrganizationID                  string
	IsSuspended                     bool
	Sources                         map[string]Source
	Destinations                    map[string]Vault
	Devices                         map[string]Device
	BackupRules                     map[string]Rule
	AllProtectedItemsQuotaEnabled   bool
	AllProtectedItemsQuotaBytes     int64
	MaximumDevices                  int64
	QuotaOffice365ProtectedAccounts int64
	QuotaHyperVGuests               int64
	QuotaVMwareGuests               int64
}

// LiveConn is the subset of LiveUserConnection used to tell who is online.
type LiveConn struct {
	DeviceTimeZone  string
	Username        string
	DeviceID        string
	ReportedVersion string
	ConnectionTime  int64
}

// DecodeProfiles decodes AdminListUsersFull (Dictionary<string, UserProfileConfig>).
func DecodeProfiles(data []byte) (map[string]Profile, error) {
	out := map[string]Profile{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding user profiles: %w", err)
	}
	for name, p := range out {
		if p.Username == "" {
			p.Username = name
			out[name] = p
		}
	}
	return out, nil
}

// DecodeLive decodes AdminDispatcherListActive (Dictionary<string, LiveUserConnection>).
func DecodeLive(data []byte) (map[string]LiveConn, error) {
	out := map[string]LiveConn{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding live connections: %w", err)
	}
	return out, nil
}

// DecodeJobs decodes List<BackupJobDetail>.
func DecodeJobs(data []byte) ([]Job, error) {
	out := []Job{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding jobs: %w", err)
	}
	return out, nil
}

// ---- status and classification names (api-constants) ----------------------------

var statusNames = map[int64]string{
	5000: "success",
	6000: "running_indeterminate", 6001: "running", 6002: "running_revived", 6003: "running_retrying", 6004: "not_yet_started",
	7000: "timeout", 7001: "warning", 7002: "error", 7003: "quota_exceeded", 7004: "missed",
	7005: "cancelled", 7006: "skipped_already_running", 7007: "abandoned",
}

var classificationNames = map[int64]string{
	4000: "unknown", 4001: "backup", 4002: "restore", 4003: "retention", 4004: "unlock", 4005: "delete_custom",
	4006: "remeasure", 4007: "update", 4008: "import", 4009: "reindex", 4010: "deep_verify", 4011: "uninstall",
}

func StatusName(s int64) string {
	if n, ok := statusNames[s]; ok {
		return n
	}
	return fmt.Sprintf("status_%d", s)
}

func ClassificationName(c int64) string {
	if n, ok := classificationNames[c]; ok {
		return n
	}
	return fmt.Sprintf("classification_%d", c)
}

// Family groups a JOB_STATUS code: success (5xxx), running (6xxx), warning
// (7001, "completed with warnings"), failed (other 7xxx), unknown.
func Family(s int64) string {
	switch {
	case s >= 5000 && s <= 5999:
		return "success"
	case s >= 6000 && s <= 6999:
		return "running"
	case s == 7001:
		return "warning"
	case s >= 7000 && s <= 7999:
		return "failed"
	}
	return "unknown"
}

// ---- schedules ------------------------------------------------------------------

const (
	freqOnceOnly = 8010
	freqDaily    = 8011
	freqHourly   = 8012
	freqWeekly   = 8013
	freqMonthly  = 8014
	freqPeriodic = 8015
	freqYearly   = 8016
)

// scheduleWalkLimit is a safety valve only: the walk jumps across excluded
// days and windows, so reaching it means the evaluation is incomplete, which
// is reported rather than treated as "no run due".
const scheduleWalkLimit = 200000

// maxScheduleField bounds the unsigned schedule fields Comet sends (seconds
// past, a periodic interval, a window hour or minute, a random delay). Every
// real value is far below it, and it keeps the walk's signed arithmetic from
// wrapping: a periodic SecondsPast of 1<<63 used to never terminate.
const maxScheduleField = math.MaxInt32

// scheduleField converts an unsigned schedule field for the walk; ok is false
// when the value is out of range.
func scheduleField(v uint64) (n int, ok bool) {
	if v > maxScheduleField {
		return 0, false
	}
	return int(v), true
}

// NextRun returns the first eligible occurrence of s strictly after `after`
// and no later than limit, in the device's local timezone loc, plus its random
// delay. found is false when no eligible occurrence exists in that span or the
// schedule has no walkable recurrence (once-only, yearly, unknown frequency);
// incomplete is true when the safety valve stopped the walk early, or when a
// field is too large to be a real schedule (see maxScheduleField).
//
// Occurrences follow the SDK definitions and are built from local calendar
// fields, so wall-clock times hold across daylight-saving changes: hourly =
// SecondsPast into every local hour, daily = SecondsPast after local midnight,
// weekly = after local Sunday 00:00, monthly = after local 00:00 on the 1st,
// periodic = Offset + k*SecondsPast on the Unix clock. A wall-clock time that
// does not exist on a daylight-saving day is placed where Go normalizes it,
// and the day/window restrictions are then checked on that local time.
func NextRun(s Schedule, after, limit time.Time, loc *time.Location) (run time.Time, found, incomplete bool) {
	switch s.FrequencyType {
	case freqHourly, freqDaily, freqWeekly, freqMonthly:
	case freqPeriodic:
		if s.SecondsPast == 0 {
			return time.Time{}, false, false
		}
	default:
		return time.Time{}, false, false
	}
	days := []bool{s.DaysSelect.Sunday, s.DaysSelect.Monday, s.DaysSelect.Tuesday, s.DaysSelect.Wednesday,
		s.DaysSelect.Thursday, s.DaysSelect.Friday, s.DaysSelect.Saturday}
	if s.RestrictDays && !slicesContainsTrue(days) {
		return time.Time{}, false, false // no day selected: never runs
	}
	sp, okSP := scheduleField(s.SecondsPast)
	delay, okDelay := scheduleField(s.RandomDelaySecs)
	if !okSP || !okDelay {
		return time.Time{}, false, true // out of range: unresolved, never a wrapped value
	}
	var from, to int
	if s.RestrictRuntime {
		fh, ok1 := scheduleField(s.FromTime.Hour)
		fm, ok2 := scheduleField(s.FromTime.Minutes)
		th, ok3 := scheduleField(s.ToTime.Hour)
		tm, ok4 := scheduleField(s.ToTime.Minutes)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return time.Time{}, false, true
		}
		from, to = fh*60+fm, th*60+tm
	}
	inWindow := func(lt time.Time) bool {
		if !s.RestrictRuntime {
			return true
		}
		tod := lt.Hour()*60 + lt.Minute()
		if from <= to {
			return tod >= from && tod <= to
		}
		return tod >= from || tod <= to // window wraps midnight
	}
	dayOK := func(lt time.Time) bool { return !s.RestrictDays || days[lt.Weekday()] }
	// firstAtOrAfter is the first occurrence at or after x. Each calendar
	// candidate is built from local fields and advanced until it is not before
	// x, because Go may normalize a nonexistent wall-clock time backwards.
	firstAtOrAfter := func(x time.Time) time.Time {
		lx := x.In(loc)
		y, m, d := lx.Date()
		var cand func(i int) time.Time
		switch s.FrequencyType {
		case freqHourly:
			mi, sec := (sp%3600)/60, sp%60
			hr := lx.Hour()
			cand = func(i int) time.Time { return time.Date(y, m, d, hr+i, mi, sec, 0, loc) }
		case freqDaily:
			h, mi, sec := (sp/3600)%24, (sp%3600)/60, sp%60
			cand = func(i int) time.Time { return time.Date(y, m, d+i, h, mi, sec, 0, loc) }
		case freqWeekly:
			dd, h, mi, sec := sp/86400, (sp%86400)/3600, (sp%3600)/60, sp%60
			sunday := d - int(lx.Weekday())
			cand = func(i int) time.Time { return time.Date(y, m, sunday+7*i+dd, h, mi, sec, 0, loc) }
		case freqMonthly:
			dd, h, mi, sec := sp/86400, (sp%86400)/3600, (sp%3600)/60, sp%60
			cand = func(i int) time.Time { return time.Date(y, m+time.Month(i), 1+dd, h, mi, sec, 0, loc) }
		case freqPeriodic:
			period := int64(sp)
			base := x.Unix() - s.Offset
			k := base / period
			if base < 0 && base%period != 0 {
				k-- // floor division
			}
			cand = func(i int) time.Time { return time.Unix(s.Offset+(k+int64(i))*period, 0) }
		}
		// Start one period back: a monthly offset past the end of a short month
		// (Feb 1 + 30 days = Mar 3) normalizes into the following month.
		for i := -1; ; i++ {
			if c := cand(i); !c.Before(x) {
				return c
			}
		}
	}
	// nextAllowed is the earliest instant at or after x on an allowed day and
	// inside the runtime window. Weekday and window eligibility can only change
	// on minute boundaries (local midnight, window edges and daylight-saving
	// transitions all fall on whole minutes), so inside a day range the first
	// eligible instant is found by stepping minute by minute: exact, no matter
	// how DST reshapes the day. Consecutive ranges are contiguous ([day, next)
	// with next the following day's start), and a range is skipped only when
	// none of the weekdays it can contain is allowed. It scans two weeks and
	// otherwise resumes the walk two weeks on.
	nextAllowed := func(x time.Time) time.Time {
		lx := x.In(loc)
		ok := func(t time.Time) bool { lt := t.In(loc); return dayOK(lt) && inWindow(lt) }
		for dd := 0; dd <= 14; dd++ {
			day := time.Date(lx.Year(), lx.Month(), lx.Day()+dd, 0, 0, 0, 0, loc)
			next := time.Date(lx.Year(), lx.Month(), lx.Day()+dd+1, 0, 0, 0, 0, loc)
			start := day
			if dd == 0 {
				start = x
			}
			// Midnight can be normalized across a DST transition, so the range
			// [day, next) may hold instants of the neighbouring weekdays. Skip it
			// only when none of the weekdays it can contain is allowed.
			noon := time.Date(lx.Year(), lx.Month(), lx.Day()+dd, 12, 0, 0, 0, loc)
			if !dayOK(start.In(loc)) && !dayOK(noon.In(loc)) && !dayOK(next.Add(-time.Second).In(loc)) {
				continue
			}
			if ok(start) {
				return start
			}
			for t := start.Truncate(time.Minute).Add(time.Minute); t.Before(next); t = t.Add(time.Minute) {
				if ok(t) {
					return t
				}
			}
		}
		return time.Date(lx.Year(), lx.Month(), lx.Day()+15, 0, 0, 0, 0, loc)
	}
	x := after.Add(time.Second)
	for i := 0; i < scheduleWalkLimit; i++ {
		c := firstAtOrAfter(x)
		if c.After(limit) {
			return time.Time{}, false, false
		}
		lc := c.In(loc)
		if dayOK(lc) && inWindow(lc) {
			return c.Add(time.Duration(delay) * time.Second), true, false
		}
		next := nextAllowed(c.Add(time.Second))
		if !next.After(c) {
			next = c.Add(time.Second)
		}
		x = next
	}
	return time.Time{}, false, true
}

func slicesContainsTrue(v []bool) bool {
	for _, b := range v {
		if b {
			return true
		}
	}
	return false
}

// ExpectedRun is the earliest eligible scheduled run, across every backup rule
// that targets sourceID, after `after` and no later than limit. scheduled
// reports whether any rule has a recurring schedule; incomplete reports that a
// walk hit its safety valve, so the answer is unknown.
func ExpectedRun(p Profile, sourceID string, after, limit time.Time, loc *time.Location) (run time.Time, scheduled, found, incomplete bool) {
	for _, r := range p.BackupRules {
		if r.Source != sourceID {
			continue
		}
		for _, s := range r.Schedules {
			switch s.FrequencyType {
			case freqHourly, freqDaily, freqWeekly, freqMonthly, freqPeriodic, freqYearly:
				scheduled = true
			}
			t, ok, inc := NextRun(s, after, limit, loc)
			incomplete = incomplete || inc
			if ok && (!found || t.Before(run)) {
				run, found = t, true
			}
		}
	}
	return run, scheduled, found, incomplete
}

// zoneSensitive reports whether any schedule targeting sourceID restricts
// weekdays or a time window on a local-clock frequency or a periodic one;
// without the device's zone such eligibility cannot be decided.
func zoneSensitive(p Profile, sourceID string) bool {
	for _, r := range p.BackupRules {
		if r.Source != sourceID {
			continue
		}
		for _, s := range r.Schedules {
			if s.RestrictDays || s.RestrictRuntime {
				return true
			}
		}
	}
	return false
}

// unknownZones spans every UTC offset from -12:00 to +14:00 in 15-minute
// steps; used when a device reports no timezone.
var unknownZones = func() []*time.Location {
	var out []*time.Location
	for m := -12 * 60; m <= 14*60; m += 15 {
		out = append(out, time.FixedZone(fmt.Sprintf("UTC%+d", m), m*60))
	}
	return out
}()

// expectedRunAnyZone is ExpectedRun for a device whose timezone is unknown:
// it returns the latest of the per-zone expected runs (so an item is only
// called overdue when it would be overdue in every possible zone) and found
// only when every zone has a run.
func expectedRunAnyZone(p Profile, sourceID string, after, limit time.Time) (run time.Time, scheduled, found, incomplete bool) {
	found = true
	for _, z := range unknownZones {
		t, sch, ok, inc := ExpectedRun(p, sourceID, after, limit, z)
		scheduled = scheduled || sch
		incomplete = incomplete || inc
		if !ok {
			found = false
			continue
		}
		if t.After(run) {
			run = t
		}
	}
	if !found {
		run = time.Time{}
	}
	return run, scheduled, found, incomplete
}

// ---- health -----------------------------------------------------------------------

type HealthOptions struct {
	Now    time.Time
	Grace  time.Duration // how late a scheduled run may be before the item is overdue
	MaxAge time.Duration // expected interval for items with no recurring schedule
}

type HealthRow struct {
	Tenant             string   `json:"tenant"`
	TenantName         string   `json:"tenant_name,omitempty"`
	ItemID             string   `json:"item_id"`
	Item               string   `json:"item"`
	Engine             string   `json:"engine"`
	DeviceID           string   `json:"device_id,omitempty"`
	Device             string   `json:"device,omitempty"`
	State              string   `json:"state"`
	Overdue            bool     `json:"overdue"`
	NeverSucceeded     bool     `json:"never_succeeded"`
	LastStatus         int64    `json:"last_status,omitempty"`
	LastStatusName     string   `json:"last_status_name,omitempty"`
	LastJobID          string   `json:"last_job_id,omitempty"`
	LastJobEnd         string   `json:"last_job_end,omitempty"`
	LastSuccessAt      string   `json:"last_success_at,omitempty"`
	HoursSinceSuccess  *float64 `json:"hours_since_success"`
	ExpectedRunAt      string   `json:"expected_run_at,omitempty"`
	Scheduled          bool     `json:"scheduled"`
	ScheduleUnresolved bool     `json:"schedule_unresolved,omitempty"`
	Online             bool     `json:"online"`
	ConnectionID       string   `json:"connection_id,omitempty"`
	Suspended          bool     `json:"suspended,omitempty"`
}

// IsProblem reports whether a health state needs attention.
func IsProblem(state string) bool {
	switch state {
	case "failed", "warning", "never", "overdue":
		return true
	}
	return false
}

// OnlineIndex maps "username\x00deviceID" to a live connection id.
func OnlineIndex(live map[string]LiveConn) map[string]string {
	idx := map[string]string{}
	ids := make([]string, 0, len(live))
	for id := range live {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic pick when a device has two connections
	for _, id := range ids {
		c := live[id]
		k := c.Username + "\x00" + c.DeviceID
		if _, dup := idx[k]; !dup {
			idx[k] = id
		}
	}
	return idx
}

func ts(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

// Health returns one row per protected item across every profile, sorted by
// tenant then item.
func Health(profiles map[string]Profile, live map[string]LiveConn, o HealthOptions) []HealthRow {
	online := OnlineIndex(live)
	rows := make([]HealthRow, 0)
	for _, user := range sortedKeys(profiles) {
		p := profiles[user]
		for _, sid := range sortedKeys(p.Sources) {
			src := p.Sources[sid]
			last := src.Statistics.LastBackupJob
			succ := src.Statistics.LastSuccessfulBackupJob
			r := HealthRow{
				Tenant: p.Username, TenantName: p.AccountName, ItemID: sid, Item: src.Description,
				Engine: src.Engine, DeviceID: src.OwnerDevice, Suspended: p.IsSuspended,
			}
			if d, ok := p.Devices[src.OwnerDevice]; ok {
				r.Device = d.FriendlyName
			}
			if cid, ok := online[p.Username+"\x00"+src.OwnerDevice]; ok && src.OwnerDevice != "" {
				r.Online, r.ConnectionID = true, cid
			}
			if last.present() {
				r.LastStatus, r.LastStatusName, r.LastJobID, r.LastJobEnd = last.Status, StatusName(last.Status), last.GUID, ts(last.EndTime)
			}
			// Overdue = a run the schedule called for is more than Grace in the
			// past and no success has landed since. Runs are walked in the
			// device's own timezone; an unknown zone uses UTC plus a day of slack.
			// Device timezone: the profile's DeviceTimezone, else the zone the
			// live connection reports, else the account's LocalTimezone
			// (UserProfileConfig: "Individual devices may declare a more
			// specific timezone"); unknown zones are evaluated across every
			// UTC offset (plus 1h for daylight saving) and only flag an item
			// when it would be overdue in all of them.
			var loc *time.Location
			zone := ""
			if d, ok := p.Devices[src.OwnerDevice]; ok {
				zone = d.DeviceTimezone
			}
			if zone == "" {
				for _, c := range live {
					if c.Username == p.Username && c.DeviceID == src.OwnerDevice && c.DeviceTimeZone != "" {
						zone = c.DeviceTimeZone
						break
					}
				}
			}
			if zone == "" {
				zone = p.LocalTimezone
			}
			if zone != "" {
				if l, err := time.LoadLocation(zone); err == nil {
					loc = l
				}
			}
			ref := time.Unix(src.CreateTime, 0)
			if succ.present() && succ.EndTime > 0 {
				ref = time.Unix(succ.EndTime, 0)
				r.LastSuccessAt = ts(succ.EndTime)
				h := o.Now.Sub(ref).Hours()
				r.HoursSinceSuccess = &h
			} else {
				r.NeverSucceeded = true
			}
			knownStart := !r.NeverSucceeded || src.CreateTime > 0
			limit := o.Now.Add(366 * 24 * time.Hour)
			var run time.Time
			var scheduled, found, incomplete bool
			slack := time.Duration(0)
			switch {
			case loc != nil:
				run, scheduled, found, incomplete = ExpectedRun(p, sid, ref, limit, loc)
			case zoneSensitive(p, sid):
				// Day or time-window eligibility depends on the device's zone and
				// its daylight-saving rules, which are unknown here: no claim.
				scheduled, incomplete = true, true
			default:
				run, scheduled, found, incomplete = expectedRunAnyZone(p, sid, ref, limit)
				slack = time.Hour
			}
			r.Scheduled = scheduled
			r.ScheduleUnresolved = incomplete
			switch {
			case incomplete:
				// The schedule could not be resolved (unknown zone with day or
				// window restrictions, or the walk's safety valve): reported as
				// schedule_unresolved, never asserted overdue.
			case !knownStart:
				// Never succeeded and no creation time: it has existed for an
				// unknown time without a good backup.
				r.Overdue = true
			case found:
				r.ExpectedRunAt = run.UTC().Format(time.RFC3339)
				r.Overdue = o.Now.Sub(run) > o.Grace+slack
			case !scheduled:
				// No recurring schedule: fall back to the unscheduled interval.
				r.Overdue = o.Now.Sub(ref) > o.MaxAge+o.Grace
			}
			switch {
			case last.present() && Family(last.Status) == "failed":
				r.State = "failed"
			case last.present() && Family(last.Status) == "warning":
				r.State = "warning"
			case r.NeverSucceeded && r.Overdue:
				r.State = "never"
			case r.Overdue:
				r.State = "overdue"
			case last.present() && Family(last.Status) == "running":
				r.State = "running"
			case r.NeverSucceeded:
				r.State = "pending"
			default:
				r.State = "ok"
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// MatchState reports whether a row matches any requested state. "overdue" and
// "never" also match rows whose primary state is more severe (a failed item
// that is also overdue matches --state overdue).
func MatchState(r HealthRow, want map[string]bool) bool {
	if len(want) == 0 {
		return true
	}
	if want[r.State] {
		return true
	}
	if want["overdue"] && r.Overdue {
		return true
	}
	if want["never"] && r.NeverSucceeded {
		return true
	}
	if want["problem"] && IsProblem(r.State) {
		return true
	}
	if want["unresolved"] && r.ScheduleUnresolved {
		return true
	}
	return false
}

// ---- triage ------------------------------------------------------------------------

type TriageOptions struct {
	Now             time.Time
	IncludeWarnings bool
	StuckAfter      time.Duration
}

// ProblemJobs keeps the newest job per (user, protected item, vault) and
// returns it when it failed, warned (if requested), or has been running longer
// than StuckAfter. A later success therefore suppresses an earlier failure.
func ProblemJobs(jobs []Job, o TriageOptions) []Job {
	latest := map[string]Job{}
	for _, j := range jobs {
		if j.Classification != 0 && j.Classification != 4001 {
			continue // backup jobs only; restores and maintenance have their own commands
		}
		k := j.Username + "\x00" + j.SourceGUID + "\x00" + j.DestinationGUID
		if cur, ok := latest[k]; !ok || newerJob(j, cur) {
			latest[k] = j
		}
	}
	out := make([]Job, 0)
	for _, j := range latest {
		switch Family(j.Status) {
		case "failed":
			out = append(out, j)
		case "warning":
			if o.IncludeWarnings {
				out = append(out, j)
			}
		case "running":
			if o.StuckAfter > 0 && j.StartTime > 0 && o.Now.Sub(time.Unix(j.StartTime, 0)) > o.StuckAfter {
				out = append(out, j)
			}
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Username != out[b].Username {
			return out[a].Username < out[b].Username
		}
		return out[a].StartTime > out[b].StartTime
	})
	return out
}

// newerJob orders jobs for one protected item and vault: later start wins; on
// the same start second a success beats a running job, which beats a warning,
// which beats a failure (so a tie never triggers a rerun next to an active or
// successful contender), then later end, then the GUID decides.
func newerJob(a, b Job) bool {
	if a.StartTime != b.StartTime {
		return a.StartTime > b.StartTime
	}
	rank := func(j Job) int {
		switch Family(j.Status) {
		case "success":
			return 3
		case "running":
			return 2
		case "warning":
			return 1
		}
		return 0
	}
	if rank(a) != rank(b) {
		return rank(a) > rank(b)
	}
	if a.EndTime != b.EndTime {
		return a.EndTime > b.EndTime
	}
	return a.GUID < b.GUID
}

// VersionAtLeast compares a Comet "MAJOR.MINOR.PATCH" version string.
func VersionAtLeast(v string, major, minor, patch int) bool {
	var a, b, c int
	if n, _ := fmt.Sscanf(strings.TrimSpace(v), "%d.%d.%d", &a, &b, &c); n < 2 {
		return false
	}
	switch {
	case a != major:
		return a > major
	case b != minor:
		return b > minor
	}
	return c >= patch
}

var (
	reGUID   = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	rePath   = regexp.MustCompile(`([A-Za-z]:\\|/)[^\s'":]*`)
	reNumber = regexp.MustCompile(`\d+`)
	reSpace  = regexp.MustCompile(`\s+`)
)

// NormalizeError strips GUIDs, paths and numbers so the same failure on two
// machines groups together.
func NormalizeError(msg string) string {
	s := reGUID.ReplaceAllString(msg, "<id>")
	s = rePath.ReplaceAllString(s, "<path>")
	s = reNumber.ReplaceAllString(s, "<n>")
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}

// ---- usage ---------------------------------------------------------------------------

// BoosterEngines are the paid "booster" engines as counted by the official
// comet-munin-plugin (countBoosters): one booster per distinct device+engine
// pair, plus one per Synology device that owns a protected item. Newer
// engines (vmware, proxmox) are not in that list and are not flagged here.
var BoosterEngines = map[string]bool{
	"engine1/exchangeedb": true, "engine1/mssql": true, "engine1/vsswriter": true, "engine1/hyperv": true,
	"engine1/windisk": true, "engine1/mongodb": true, "engine1/winmsofficemail": true,
}

type UsageRow struct {
	Tenant              string         `json:"tenant"`
	TenantName          string         `json:"tenant_name,omitempty"`
	Vault               string         `json:"vault,omitempty"`
	VaultID             string         `json:"vault_id,omitempty"`
	SharedQuotaID       string         `json:"shared_quota_id,omitempty"`
	Suspended           bool           `json:"suspended"`
	StorageBytes        int64          `json:"storage_bytes"`
	StorageLimitBytes   int64          `json:"storage_limit_bytes"`
	SharedQuotaVaults   int            `json:"shared_quota_vaults,omitempty"`
	MaxVaultPct         *float64       `json:"max_vault_pct"`
	ProtectedBytes      int64          `json:"protected_bytes"`
	ProtectedQuotaBytes int64          `json:"protected_quota_bytes"`
	ProtectedPct        *float64       `json:"protected_pct"`
	Devices             int            `json:"devices"`
	BillableDevices     int            `json:"billable_devices"`
	MaximumDevices      int64          `json:"maximum_devices"`
	DevicesPct          *float64       `json:"devices_pct"`
	ProtectedItems      int            `json:"protected_items"`
	Boosters            int            `json:"boosters"`
	SynologyDevices     int            `json:"synology_devices"`
	Engines             map[string]int `json:"engines,omitempty"`
	M365Accounts        int64          `json:"m365_accounts"`
	M365LicensedMails   int64          `json:"m365_licensed_mailboxes"`
	M365Quota           int64          `json:"m365_quota"`
	M365Pct             *float64       `json:"m365_pct"`
	VMGuests            int64          `json:"vm_guests"`
	LastDeepVerifyAt    string         `json:"last_deep_verify_at,omitempty"`
	MaxQuotaPct         float64        `json:"max_quota_pct"`
}

func pct(used, limit int64) *float64 {
	if limit <= 0 {
		return nil
	}
	v := float64(used) * 100 / float64(limit)
	return &v
}

// vaultLimit is a vault's own enforced storage limit; a vault on a shared
// quota (StorageLimitID set) ignores StorageLimitBytes, so it reports none.
func vaultLimit(v Vault) int64 {
	if !v.StorageLimitEnabled || v.StorageLimitID != "" {
		return 0
	}
	return v.StorageLimitBytes
}

// lastSizedJob is the job a protected item's size and seat counts come from.
func lastSizedJob(s Source) Job {
	if j := s.Statistics.LastSuccessfulBackupJob; j.present() {
		return j
	}
	return s.Statistics.LastBackupJob
}

// Usage returns per-tenant rows, or per-vault rows when byVault is set.
// Counts are what this server reports, not an invoice: Comet bills across
// replicated servers and time windows a single server cannot see.
//
// Two different quotas are kept apart: Storage Vault limits compare stored
// bytes (Statistics.ClientProvidedSize), while AllProtectedItemsQuotaBytes
// limits "the total Size of all Protected Items" (SDK), compared against the
// summed TotalSize of each item's last backup. Vaults on a shared quota are
// counted (shared_quota_vaults) but contribute no limit.
func Usage(profiles map[string]Profile, byVault bool) []UsageRow {
	rows := make([]UsageRow, 0)
	for _, user := range sortedKeys(profiles) {
		p := profiles[user]
		if byVault {
			for _, vid := range sortedKeys(p.Destinations) {
				v := p.Destinations[vid]
				r := UsageRow{Tenant: p.Username, TenantName: p.AccountName, Vault: v.Description, VaultID: vid,
					SharedQuotaID: v.StorageLimitID, Suspended: p.IsSuspended, StorageBytes: v.Statistics.ClientProvidedSize.Size,
					StorageLimitBytes: vaultLimit(v), LastDeepVerifyAt: ts(v.Statistics.LastSuccessfulDeepVerify_EndTime)}
				r.MaxVaultPct = pct(r.StorageBytes, r.StorageLimitBytes)
				r.MaxQuotaPct = maxPct(r.MaxVaultPct)
				rows = append(rows, r)
			}
			continue
		}
		r := UsageRow{Tenant: p.Username, TenantName: p.AccountName, Suspended: p.IsSuspended,
			Devices: len(p.Devices), MaximumDevices: p.MaximumDevices, ProtectedItems: len(p.Sources),
			Engines: map[string]int{}, M365Quota: p.QuotaOffice365ProtectedAccounts}
		for _, v := range p.Destinations {
			r.StorageBytes += v.Statistics.ClientProvidedSize.Size
			if v.StorageLimitID != "" {
				r.SharedQuotaVaults++
			}
			if lim := vaultLimit(v); lim > 0 {
				r.StorageLimitBytes += lim
				r.MaxVaultPct = maxOf(r.MaxVaultPct, pct(v.Statistics.ClientProvidedSize.Size, lim))
			}
		}
		if p.AllProtectedItemsQuotaEnabled {
			r.ProtectedQuotaBytes = p.AllProtectedItemsQuotaBytes
		}
		owners := map[string]bool{}
		boosters := map[string]bool{} // device+engine pairs, as comet-munin-plugin counts them
		for _, s := range p.Sources {
			r.Engines[s.Engine]++
			if BoosterEngines[s.Engine] {
				boosters[s.OwnerDevice+"\x00"+s.Engine] = true
			}
			if s.OwnerDevice != "" {
				owners[s.OwnerDevice] = true
			}
			j := lastSizedJob(s)
			r.ProtectedBytes += j.TotalSize
			switch s.Engine {
			case "engine1/winmsofficemail":
				r.M365Accounts += j.TotalAccountsCount
				r.M365LicensedMails += j.TotalLicensedMailsCount
			case "engine1/hyperv", "engine1/vmware", "engine1/proxmox":
				r.VMGuests += j.TotalVmCount
			}
		}
		for id, d := range p.Devices {
			if !owners[id] {
				continue
			}
			r.BillableDevices++
			if d.PlatformVersion.Distribution == "Synology DSM" {
				r.SynologyDevices++
				boosters[id+"\x00synology"] = true
			}
		}
		r.Boosters = len(boosters)
		r.ProtectedPct = pct(r.ProtectedBytes, r.ProtectedQuotaBytes)
		r.DevicesPct = pct(int64(len(p.Devices)), p.MaximumDevices)
		r.M365Pct = pct(r.M365Accounts, p.QuotaOffice365ProtectedAccounts)
		r.MaxQuotaPct = maxPct(r.MaxVaultPct, r.ProtectedPct, r.DevicesPct, r.M365Pct)
		rows = append(rows, r)
	}
	return rows
}

func maxOf(a, b *float64) *float64 {
	if a == nil || (b != nil && *b > *a) {
		return b
	}
	return a
}

func maxPct(vs ...*float64) float64 {
	m := 0.0
	for _, v := range vs {
		if v != nil && *v > m {
			m = *v
		}
	}
	return m
}

// ---- devices ---------------------------------------------------------------------------

type DeviceRow struct {
	Tenant           string   `json:"tenant"`
	DeviceID         string   `json:"device_id"`
	Device           string   `json:"device"`
	Online           bool     `json:"online"`
	ConnectionID     string   `json:"connection_id,omitempty"`
	ClientVersion    string   `json:"client_version,omitempty"`
	ServerVersion    string   `json:"server_version,omitempty"`
	OS               string   `json:"os,omitempty"`
	RegisteredAt     string   `json:"registered_at,omitempty"`
	LastJobEnd       string   `json:"last_job_end,omitempty"`
	DaysSinceLastJob *float64 `json:"days_since_last_job"`
	ProtectedItems   int      `json:"protected_items"`
	StillBillable    bool     `json:"still_billable"`
}

// Devices returns one row per registered device with its online state and
// the newest job end time across the protected items it owns (Comet keeps no
// last-seen time, so that is the best available proxy).
func Devices(profiles map[string]Profile, live map[string]LiveConn, now time.Time) []DeviceRow {
	online := OnlineIndex(live)
	rows := make([]DeviceRow, 0)
	for _, user := range sortedKeys(profiles) {
		p := profiles[user]
		for _, did := range sortedKeys(p.Devices) {
			d := p.Devices[did]
			r := DeviceRow{Tenant: p.Username, DeviceID: did, Device: d.FriendlyName, ClientVersion: d.ClientVersion,
				OS: strings.TrimSpace(d.PlatformVersion.Distribution + " " + d.PlatformVersion.OS), RegisteredAt: ts(d.RegistrationTime)}
			if cid, ok := online[p.Username+"\x00"+did]; ok {
				r.Online, r.ConnectionID = true, cid
				if v := live[cid].ReportedVersion; v != "" {
					r.ClientVersion = v
				}
			}
			var newest int64
			for _, s := range p.Sources {
				if s.OwnerDevice != did {
					continue
				}
				r.ProtectedItems++
				for _, j := range []Job{s.Statistics.LastBackupJob, s.Statistics.LastSuccessfulBackupJob} {
					if j.EndTime > newest {
						newest = j.EndTime
					}
				}
			}
			r.StillBillable = r.ProtectedItems > 0
			if newest > 0 {
				r.LastJobEnd = ts(newest)
				days := now.Sub(time.Unix(newest, 0)).Hours() / 24
				r.DaysSinceLastJob = &days
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// Stale keeps offline devices whose newest job is older than olderThan (or
// that never ran a job), sorted longest-silent first.
func Stale(rows []DeviceRow, olderThan time.Duration) []DeviceRow {
	out := make([]DeviceRow, 0)
	for _, r := range rows {
		if r.Online {
			continue
		}
		if r.DaysSinceLastJob != nil && *r.DaysSinceLastJob*24 < olderThan.Hours() {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(a, b int) bool {
		da, db := out[a].DaysSinceLastJob, out[b].DaysSinceLastJob
		if da == nil || db == nil {
			return da == nil && db != nil
		}
		return *da > *db
	})
	return out
}

// Outdated keeps devices whose client version is known and differs from the
// server version, and counts devices per client version.
func Outdated(rows []DeviceRow, serverVersion string) ([]DeviceRow, map[string]int) {
	out := make([]DeviceRow, 0)
	counts := map[string]int{}
	for _, r := range rows {
		v := r.ClientVersion
		if v == "" {
			v = "unknown"
		}
		counts[v]++
		if r.ClientVersion == "" || r.ClientVersion == serverVersion {
			continue
		}
		r.ServerVersion = serverVersion
		out = append(out, r)
	}
	return out, counts
}

// ---- snapshots and changes ---------------------------------------------------------------

type Snapshot struct {
	TakenAt       int64       `json:"taken_at"`
	ServerVersion string      `json:"server_version"`
	Items         []HealthRow `json:"items"`
	Devices       []DeviceRow `json:"devices"`
	Tenants       []UsageRow  `json:"tenants"`
	Vaults        []UsageRow  `json:"vaults"`
}

type Change struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Key      string `json:"change_id"` // not "key": SecretFieldName would redact it
	Tenant   string `json:"tenant"`
	Device   string `json:"device,omitempty"`
	Item     string `json:"item,omitempty"`
	Vault    string `json:"vault,omitempty"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Diff compares two snapshots. Keys are stable (tenant/item-id,
// tenant/device-id, tenant/vault-id) so a PSA integration can open a ticket on
// problem_opened and close it on problem_resolved.
func Diff(prev, cur Snapshot) []Change {
	out := make([]Change, 0)
	pi, ci := map[string]HealthRow{}, map[string]HealthRow{}
	for _, r := range prev.Items {
		pi[r.Tenant+"/"+r.ItemID] = r
	}
	for _, r := range cur.Items {
		ci[r.Tenant+"/"+r.ItemID] = r
	}
	for _, k := range sortedKeys(ci) {
		c := ci[k]
		p, existed := pi[k]
		if !existed {
			ch := Change{Kind: "item_added", Category: "inventory", Key: k, Tenant: c.Tenant, Device: c.Device, Item: c.Item, After: c.Engine}
			if BoosterEngines[c.Engine] {
				ch.Detail = "booster engine"
			}
			out = append(out, ch)
			if IsProblem(c.State) {
				out = append(out, Change{Kind: "problem_opened", Category: "health", Key: k, Tenant: c.Tenant, Device: c.Device, Item: c.Item, After: c.State})
			}
			continue
		}
		switch {
		case !IsProblem(p.State) && IsProblem(c.State):
			out = append(out, Change{Kind: "problem_opened", Category: "health", Key: k, Tenant: c.Tenant, Device: c.Device, Item: c.Item, Before: p.State, After: c.State})
		case IsProblem(p.State) && !IsProblem(c.State):
			out = append(out, Change{Kind: "problem_resolved", Category: "health", Key: k, Tenant: c.Tenant, Device: c.Device, Item: c.Item, Before: p.State, After: c.State})
		case IsProblem(p.State) && IsProblem(c.State) && p.State != c.State:
			out = append(out, Change{Kind: "problem_changed", Category: "health", Key: k, Tenant: c.Tenant, Device: c.Device, Item: c.Item, Before: p.State, After: c.State})
		}
	}
	for _, k := range sortedKeys(pi) {
		if _, still := ci[k]; !still {
			p := pi[k]
			out = append(out, Change{Kind: "item_removed", Category: "inventory", Key: k, Tenant: p.Tenant, Device: p.Device, Item: p.Item, Before: p.Engine})
			// A problem item that disappears (deleted, or its device or tenant
			// removed) closes its ticket too, so a feed filtered to the health
			// category never leaves it open.
			if IsProblem(p.State) {
				out = append(out, Change{Kind: "problem_resolved", Category: "health", Key: k, Tenant: p.Tenant, Device: p.Device, Item: p.Item, Before: p.State, After: "removed"})
			}
		}
	}

	pd, cd := map[string]DeviceRow{}, map[string]DeviceRow{}
	for _, r := range prev.Devices {
		pd[r.Tenant+"/"+r.DeviceID] = r
	}
	for _, r := range cur.Devices {
		cd[r.Tenant+"/"+r.DeviceID] = r
	}
	for _, k := range sortedKeys(cd) {
		c := cd[k]
		p, existed := pd[k]
		switch {
		case !existed:
			out = append(out, Change{Kind: "device_added", Category: "inventory", Key: k, Tenant: c.Tenant, Device: c.Device})
		case p.Online && !c.Online:
			out = append(out, Change{Kind: "device_offline", Category: "health", Key: k, Tenant: c.Tenant, Device: c.Device})
		case !p.Online && c.Online:
			out = append(out, Change{Kind: "device_online", Category: "health", Key: k, Tenant: c.Tenant, Device: c.Device})
		}
		if existed && p.ClientVersion != "" && c.ClientVersion != "" && p.ClientVersion != c.ClientVersion {
			out = append(out, Change{Kind: "client_version_changed", Category: "inventory", Key: k, Tenant: c.Tenant, Device: c.Device, Before: p.ClientVersion, After: c.ClientVersion})
		}
	}
	for _, k := range sortedKeys(pd) {
		if _, still := cd[k]; !still {
			out = append(out, Change{Kind: "device_removed", Category: "inventory", Key: k, Tenant: pd[k].Tenant, Device: pd[k].Device})
		}
	}

	pt, ct := map[string]UsageRow{}, map[string]UsageRow{}
	for _, r := range prev.Tenants {
		pt[r.Tenant] = r
	}
	for _, r := range cur.Tenants {
		ct[r.Tenant] = r
	}
	for _, k := range sortedKeys(ct) {
		c := ct[k]
		p, existed := pt[k]
		if !existed {
			out = append(out, Change{Kind: "tenant_added", Category: "inventory", Key: k, Tenant: k})
			continue
		}
		if p.Suspended != c.Suspended {
			out = append(out, Change{Kind: "tenant_suspension_changed", Category: "inventory", Key: k, Tenant: k,
				Before: fmt.Sprint(p.Suspended), After: fmt.Sprint(c.Suspended)})
		}
		if p.StorageLimitBytes != c.StorageLimitBytes || p.ProtectedQuotaBytes != c.ProtectedQuotaBytes {
			out = append(out, Change{Kind: "quota_changed", Category: "inventory", Key: k, Tenant: k,
				Before: fmt.Sprintf("vault limits %d, protected-item quota %d", p.StorageLimitBytes, p.ProtectedQuotaBytes),
				After:  fmt.Sprintf("vault limits %d, protected-item quota %d", c.StorageLimitBytes, c.ProtectedQuotaBytes)})
		}
	}
	for _, k := range sortedKeys(pt) {
		if _, still := ct[k]; !still {
			out = append(out, Change{Kind: "tenant_removed", Category: "inventory", Key: k, Tenant: k})
		}
	}

	pv := map[string]UsageRow{}
	for _, r := range prev.Vaults {
		pv[r.Tenant+"/"+r.VaultID] = r
	}
	for _, r := range cur.Vaults {
		k := r.Tenant + "/" + r.VaultID
		if p, ok := pv[k]; ok && p.StorageBytes != r.StorageBytes {
			out = append(out, Change{Kind: "vault_size_changed", Category: "inventory", Key: k, Tenant: r.Tenant, Vault: r.Vault,
				Before: fmt.Sprint(p.StorageBytes), After: fmt.Sprint(r.StorageBytes),
				Detail: fmt.Sprintf("%+d bytes", r.StorageBytes-p.StorageBytes)})
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
