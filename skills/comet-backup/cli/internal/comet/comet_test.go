// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package comet

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var now = time.Unix(1_760_000_000, 0)

func ago(d time.Duration) int64 { return now.Add(-d).Unix() }

// fixture builds a list-users-full body with credential fields present, the
// way the real API returns it.
func fixture(t *testing.T) []byte {
	t.Helper()
	daily := map[string]any{"FrequencyType": 8011, "SecondsPast": 3600}
	body := map[string]any{
		"acme": map[string]any{
			"Username": "acme", "AccountName": "Acme Corp", "PasswordHash": "SECRET-HASH", "TOTPKey": "SECRET-TOTP",
			"PasswordRecovery": "SECRET-RECOVERY", "MaximumDevices": 4, "QuotaOffice365ProtectedAccounts": 10,
			"AllProtectedItemsQuotaEnabled": true, "AllProtectedItemsQuotaBytes": 1000,
			"Devices": map[string]any{
				"dev-a": map[string]any{"FriendlyName": "ACME-DC01", "ClientVersion": "26.8.7", "RegistrationTime": ago(400 * 24 * time.Hour)},
				"dev-b": map[string]any{"FriendlyName": "ACME-LAPTOP", "ClientVersion": "26.5.0", "PlatformVersion": map[string]any{"distribution": "Synology DSM"}},
			},
			"Destinations": map[string]any{
				"vault-1": map[string]any{"Description": "Acme S3", "DestinationType": 1000, "S3SecretKey": "SECRET-S3",
					"StorageLimitEnabled": true, "StorageLimitBytes": 2000,
					"Statistics": map[string]any{"ClientProvidedSize": map[string]any{"Size": 900}}},
			},
			"Sources": map[string]any{
				"item-ok": map[string]any{"Engine": "engine1/file", "Description": "Files", "OwnerDevice": "dev-a",
					"Statistics": map[string]any{
						"LastBackupJob":           map[string]any{"GUID": "j1", "Status": 5000, "StartTime": ago(3 * time.Hour), "EndTime": ago(2 * time.Hour)},
						"LastSuccessfulBackupJob": map[string]any{"GUID": "j1", "Status": 5000, "StartTime": ago(3 * time.Hour), "EndTime": ago(2 * time.Hour)},
					}},
				"item-failed": map[string]any{"Engine": "engine1/mssql", "Description": "SQL", "OwnerDevice": "dev-a",
					"Statistics": map[string]any{
						"LastBackupJob":           map[string]any{"GUID": "j2", "Status": 7002, "StartTime": ago(5 * time.Hour), "EndTime": ago(4 * time.Hour)},
						"LastSuccessfulBackupJob": map[string]any{"GUID": "j0", "Status": 5000, "EndTime": ago(40 * time.Hour)},
					}},
				"item-overdue": map[string]any{"Engine": "engine1/winmsofficemail", "Description": "M365", "OwnerDevice": "dev-b",
					"Statistics": map[string]any{
						"LastBackupJob":           map[string]any{"GUID": "j3", "Status": 5000, "EndTime": ago(80 * time.Hour), "TotalAccountsCount": 8, "TotalLicensedMailsCount": 6},
						"LastSuccessfulBackupJob": map[string]any{"GUID": "j3", "Status": 5000, "EndTime": ago(80 * time.Hour), "TotalAccountsCount": 8, "TotalLicensedMailsCount": 6},
					}},
				"item-never":   map[string]any{"Engine": "engine1/file", "Description": "New", "OwnerDevice": "dev-b", "CreateTime": ago(10 * 24 * time.Hour)},
				"item-pending": map[string]any{"Engine": "engine1/file", "Description": "Brand new", "OwnerDevice": "dev-b", "CreateTime": ago(time.Hour)},
			},
			"BackupRules": map[string]any{
				"r1": map[string]any{"Source": "item-ok", "Destination": "vault-1", "Schedules": []any{daily}},
				"r2": map[string]any{"Source": "item-failed", "Destination": "vault-1", "Schedules": []any{daily}},
				"r3": map[string]any{"Source": "item-overdue", "Destination": "vault-1", "Schedules": []any{daily}},
				"r4": map[string]any{"Source": "item-never", "Destination": "vault-1", "Schedules": []any{daily}},
				"r5": map[string]any{"Source": "item-pending", "Destination": "vault-1", "Schedules": []any{daily}},
			},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var live = map[string]LiveConn{"conn-1": {Username: "acme", DeviceID: "dev-a", ReportedVersion: "26.8.7"}}

func TestDecodeProfilesDropsCredentials(t *testing.T) {
	p, err := DecodeProfiles(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(p)
	for _, secret := range []string{"SECRET-HASH", "SECRET-TOTP", "SECRET-RECOVERY", "SECRET-S3", "PasswordHash", "TOTPKey"} {
		if strings.Contains(string(out), secret) {
			t.Fatalf("projection leaked %q", secret)
		}
	}
}

func TestHealthStates(t *testing.T) {
	p, _ := DecodeProfiles(fixture(t))
	rows := Health(p, live, HealthOptions{Now: now, Grace: 6 * time.Hour, MaxAge: 48 * time.Hour})
	got := map[string]HealthRow{}
	for _, r := range rows {
		got[r.ItemID] = r
	}
	want := map[string]string{"item-ok": "ok", "item-failed": "failed", "item-overdue": "overdue", "item-never": "never", "item-pending": "pending"}
	for id, state := range want {
		if got[id].State != state {
			t.Errorf("%s: state %q, want %q", id, got[id].State, state)
		}
	}
	if !got["item-failed"].Overdue {
		t.Error("failed item 40h past a daily schedule (+6h grace) should also be overdue")
	}
	if !got["item-ok"].Online || got["item-ok"].ConnectionID != "conn-1" || got["item-overdue"].Online {
		t.Error("online flag wrong")
	}
	if got["item-never"].HoursSinceSuccess != nil || !got["item-never"].NeverSucceeded {
		t.Error("never-succeeded item must have null hours_since_success")
	}
	if !MatchState(got["item-failed"], map[string]bool{"overdue": true}) || MatchState(got["item-ok"], map[string]bool{"failed": true, "overdue": true}) {
		t.Error("MatchState wrong")
	}
}

func TestNextRun(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	lordHowe, err := time.LoadLocation("Australia/Lord_Howe")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	at := func(v string) time.Time {
		tt, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t.Fatal(err)
		}
		return tt
	}
	sunday := func(s Schedule) Schedule { s.RestrictDays = true; s.DaysSelect.Sunday = true; return s }
	window := func(s Schedule, fromH, toH uint64) Schedule {
		s.RestrictRuntime, s.FromTime, s.ToTime = true, HourMinute{Hour: fromH}, HourMinute{Hour: toH}
		return s
	}
	windowHM := func(s Schedule, fh, fm, th, tm uint64) Schedule {
		s.RestrictRuntime, s.FromTime, s.ToTime = true, HourMinute{Hour: fh, Minutes: fm}, HourMinute{Hour: th, Minutes: tm}
		return s
	}
	weekdays := Schedule{FrequencyType: 8011, SecondsPast: 3600, RestrictDays: true}
	weekdays.DaysSelect.Monday, weekdays.DaysSelect.Tuesday, weekdays.DaysSelect.Wednesday = true, true, true
	weekdays.DaysSelect.Thursday, weekdays.DaysSelect.Friday = true, true
	cases := []struct {
		name  string
		s     Schedule
		after string
		loc   *time.Location
		want  string // "" = no eligible run
	}{
		{"daily 01:00 UTC", Schedule{FrequencyType: 8011, SecondsPast: 3600}, "2026-03-02T06:00:00Z", time.UTC, "2026-03-03T01:00:00Z"},
		{"daily Mon-Fri skips the weekend", weekdays, "2026-03-06T02:00:00Z", time.UTC, "2026-03-09T01:00:00Z"},
		{"hourly at :30", Schedule{FrequencyType: 8012, SecondsPast: 1800}, "2026-03-02T06:40:00Z", time.UTC, "2026-03-02T07:30:00Z"},
		{"weekly Sunday 02:00 UTC", Schedule{FrequencyType: 8013, SecondsPast: 7200}, "2026-03-02T00:00:00Z", time.UTC, "2026-03-08T02:00:00Z"},
		{"monthly 1st 00:00 UTC", Schedule{FrequencyType: 8014}, "2026-03-02T00:00:00Z", time.UTC, "2026-04-01T00:00:00Z"},
		{"random delay is added", Schedule{FrequencyType: 8011, SecondsPast: 3600, RandomDelaySecs: 1800}, "2026-03-02T06:00:00Z", time.UTC, "2026-03-03T01:30:00Z"},
		// Codex round 9 reproductions.
		{"periodic 20h in a 00:00-01:00 window", window(Schedule{FrequencyType: 8015, SecondsPast: 72000}, 0, 1), "2026-03-03T00:00:00Z", time.UTC, "2026-03-08T00:00:00Z"},
		{"periodic 24h Sunday-only across DST", sunday(Schedule{FrequencyType: 8015, SecondsPast: 86400, Offset: 16200}), "2026-03-02T04:30:00Z", ny, "2026-03-15T04:30:00Z"},
		{"hourly :30 Sunday 02:00-03:00 skips spring-forward day", window(sunday(Schedule{FrequencyType: 8012, SecondsPast: 1800}), 2, 3), "2026-03-01T07:30:00Z", ny, "2026-03-15T06:30:00Z"},
		{"daily 02:30 Sunday 02:00-03:00 skips spring-forward day", window(sunday(Schedule{FrequencyType: 8011, SecondsPast: 9000}), 2, 3), "2026-03-01T07:30:00Z", ny, "2026-03-15T06:30:00Z"},
		// Codex round 10 reproductions.
		{"daily 09:00 NY window 09:00-09:30 across spring-forward", windowHM(Schedule{FrequencyType: 8011, SecondsPast: 32400}, 9, 0, 9, 30), "2026-03-07T14:05:00Z", ny, "2026-03-08T13:00:00Z"},
		{"weekly Monday 09:00 NY across spring-forward", windowHM(Schedule{FrequencyType: 8013, SecondsPast: 86400 + 32400}, 9, 0, 9, 30), "2026-03-07T14:05:00Z", ny, "2026-03-09T13:00:00Z"},
		{"monthly 9th 09:00 NY across spring-forward", windowHM(Schedule{FrequencyType: 8014, SecondsPast: 8*86400 + 32400}, 9, 0, 9, 30), "2026-03-07T14:05:00Z", ny, "2026-03-09T13:00:00Z"},
		{"hourly on local hours in a +05:30 zone", windowHM(Schedule{FrequencyType: 8012}, 10, 0, 10, 15), "2026-03-01T04:35:00Z", kolkata, "2026-03-02T04:30:00Z"},
		{"periodic 60s Sunday 00:00-00:00 jumps a week", window(sunday(Schedule{FrequencyType: 8015, SecondsPast: 60}), 0, 0), "2026-03-01T00:01:00Z", time.UTC, "2026-03-08T00:00:00Z"},
		// Codex round 11 reproductions.
		{"hourly NY on spring-forward stays after the last success", Schedule{FrequencyType: 8012}, "2026-03-08T06:05:00Z", ny, "2026-03-08T07:00:00Z"},
		{"periodic 1h Sunday 02:00-04:00 keeps the valid part of a DST day", windowHM(sunday(Schedule{FrequencyType: 8015, SecondsPast: 3600}), 2, 0, 4, 0), "2026-03-08T05:01:00Z", ny, "2026-03-08T07:00:00Z"},
		// Codex round 12 reproductions.
		{"periodic 1h Sunday 02:30-03:15 enters the window at the DST jump", windowHM(sunday(Schedule{FrequencyType: 8015, SecondsPast: 3600}), 2, 30, 3, 15), "2026-03-08T05:01:00Z", ny, "2026-03-08T07:00:00Z"},
		{"periodic 30m Lord Howe Sunday 02:15-02:40 across its 30-minute shift", windowHM(sunday(Schedule{FrequencyType: 8015, SecondsPast: 1800}), 2, 15, 2, 40), "2026-10-03T13:31:00Z", lordHowe, "2026-10-03T15:30:00Z"},
		// Codex round 13 reproduction: Santiago's DST skips local midnight.
		{"periodic 1h Sunday 01:00-01:15 when Sunday midnight does not exist", windowHM(sunday(Schedule{FrequencyType: 8015, SecondsPast: 3600}), 1, 0, 1, 15), "2026-09-05T04:01:00Z", santiago, "2026-09-06T04:00:00Z"},
		// Codex round 14 reproduction: February's 30-day offset lands on March 3.
		{"monthly offset past a short month", Schedule{FrequencyType: 8014, SecondsPast: 30 * 86400}, "2026-03-01T00:00:00Z", time.UTC, "2026-03-03T00:00:00Z"},
		{"once only has no recurrence", Schedule{FrequencyType: 8010, SecondsPast: 1}, "2026-03-02T00:00:00Z", time.UTC, ""},
		{"no day selected never runs", Schedule{FrequencyType: 8011, RestrictDays: true}, "2026-03-02T00:00:00Z", time.UTC, ""},
	}
	for _, c := range cases {
		got, ok, _ := NextRun(c.s, at(c.after), at(c.after).Add(366*24*time.Hour), c.loc)
		gotS := ""
		if ok {
			gotS = got.UTC().Format(time.RFC3339)
		}
		if gotS != c.want {
			t.Errorf("%s: got %q, want %q", c.name, gotS, c.want)
		}
	}
}

func TestHealthStaleAndUnknownStart(t *testing.T) {
	daily := Schedule{FrequencyType: 8011, SecondsPast: 3600}
	src := func(createAgo time.Duration, succAgo time.Duration) Source {
		s := Source{Engine: "engine1/file", OwnerDevice: "d"}
		if createAgo > 0 {
			s.CreateTime = ago(createAgo)
		}
		if succAgo > 0 {
			s.Statistics.LastSuccessfulBackupJob = Job{GUID: "j", Status: 5000, EndTime: ago(succAgo)}
			s.Statistics.LastBackupJob = s.Statistics.LastSuccessfulBackupJob
		}
		return s
	}
	p := map[string]Profile{"t": {Username: "t", Devices: map[string]Device{"d": {DeviceTimezone: "UTC"}},
		Sources: map[string]Source{
			"ancient":  src(0, 900*24*time.Hour), // success long before the walk horizon
			"nocreate": src(0, 0),                // never succeeded, creation time unknown
		},
		BackupRules: map[string]Rule{
			"r1": {Source: "ancient", Schedules: []Schedule{daily}},
			"r2": {Source: "nocreate", Schedules: []Schedule{daily}},
		}}}
	got := map[string]HealthRow{}
	for _, r := range Health(p, nil, HealthOptions{Now: now, Grace: 6 * time.Hour, MaxAge: 48 * time.Hour}) {
		got[r.ItemID] = r
	}
	if got["ancient"].State != "overdue" || got["nocreate"].State != "never" {
		t.Fatalf("stale/unknown-start states wrong: ancient=%s nocreate=%s", got["ancient"].State, got["nocreate"].State)
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{{"24.6.6", true}, {"24.6.5", false}, {"26.8.7", true}, {"24.12.0", true}, {"23.9.9", false}, {"", false}, {"junk", false}}
	for _, c := range cases {
		if VersionAtLeast(c.v, 24, 6, 6) != c.want {
			t.Errorf("VersionAtLeast(%q)=%v", c.v, !c.want)
		}
	}
}

func TestProblemJobsTieBreak(t *testing.T) {
	fail := Job{GUID: "b", Username: "u", SourceGUID: "s", DestinationGUID: "v", Classification: 4001, Status: 7002, StartTime: ago(time.Hour), EndTime: ago(50 * time.Minute)}
	ok := fail
	ok.GUID, ok.Status, ok.EndTime = "a", 5000, ago(40*time.Minute)
	for _, order := range [][]Job{{fail, ok}, {ok, fail}} {
		if got := ProblemJobs(order, TriageOptions{Now: now}); len(got) != 0 {
			t.Fatalf("same start, later-ending success must win regardless of order: %+v", got)
		}
	}
	running := fail
	running.GUID, running.Status, running.EndTime = "c", 6001, 0
	for _, order := range [][]Job{{fail, running}, {running, fail}} {
		if got := ProblemJobs(order, TriageOptions{Now: now}); len(got) != 0 {
			t.Fatalf("an active job with the same start must outrank a completed failure: %+v", got)
		}
	}
	ok.EndTime = fail.EndTime // full tie: success still wins
	for _, order := range [][]Job{{fail, ok}, {ok, fail}} {
		if got := ProblemJobs(order, TriageOptions{Now: now}); len(got) != 0 {
			t.Fatalf("full tie must prefer success: %+v", got)
		}
	}
}

func TestFamilyAndNames(t *testing.T) {
	cases := map[int64]string{5000: "success", 6001: "running", 7001: "warning", 7002: "failed", 7004: "failed", 4: "unknown"}
	for s, want := range cases {
		if Family(s) != want {
			t.Errorf("Family(%d)=%s want %s", s, Family(s), want)
		}
	}
	if StatusName(7003) != "quota_exceeded" || ClassificationName(4002) != "restore" || StatusName(9) != "status_9" {
		t.Error("names wrong")
	}
}

func TestProblemJobsLatestWins(t *testing.T) {
	jobs := []Job{
		{GUID: "old-fail", Username: "acme", SourceGUID: "s1", DestinationGUID: "v1", Classification: 4001, Status: 7002, StartTime: ago(10 * time.Hour)},
		{GUID: "new-ok", Username: "acme", SourceGUID: "s1", DestinationGUID: "v1", Classification: 4001, Status: 5000, StartTime: ago(2 * time.Hour)},
		{GUID: "warn", Username: "acme", SourceGUID: "s2", DestinationGUID: "v1", Classification: 4001, Status: 7001, StartTime: ago(3 * time.Hour)},
		{GUID: "stuck", Username: "beta", SourceGUID: "s3", DestinationGUID: "v1", Classification: 4001, Status: 6001, StartTime: ago(20 * time.Hour)},
		{GUID: "running", Username: "beta", SourceGUID: "s4", DestinationGUID: "v1", Classification: 4001, Status: 6001, StartTime: ago(time.Hour)},
		{GUID: "restore-fail", Username: "beta", SourceGUID: "s5", DestinationGUID: "v1", Classification: 4002, Status: 7002, StartTime: ago(time.Hour)},
	}
	ids := func(js []Job) string {
		var s []string
		for _, j := range js {
			s = append(s, j.GUID)
		}
		return strings.Join(s, ",")
	}
	if got := ids(ProblemJobs(jobs, TriageOptions{Now: now, StuckAfter: 12 * time.Hour})); got != "stuck" {
		t.Fatalf("got %q, want only the stuck job (later success suppresses the failure)", got)
	}
	if got := ids(ProblemJobs(jobs, TriageOptions{Now: now, IncludeWarnings: true, StuckAfter: 12 * time.Hour})); got != "warn,stuck" {
		t.Fatalf("with warnings got %q", got)
	}
	if got := ProblemJobs(nil, TriageOptions{Now: now}); got == nil || len(got) != 0 {
		t.Fatal("empty input must give an empty, non-nil slice")
	}
}

func TestNormalizeError(t *testing.T) {
	a := NormalizeError(`Couldn't open C:\Users\bob\file.txt: error 32 on 3f2504e0-4f89-11d3-9a0c-0305e82c3301`)
	b := NormalizeError(`Couldn't open C:\Data\x.db: error 5 on 9a2504e0-4f89-11d3-9a0c-0305e82c3302`)
	if a != b {
		t.Fatalf("expected same bucket:\n%s\n%s", a, b)
	}
}

func TestUsage(t *testing.T) {
	p, _ := DecodeProfiles(fixture(t))
	rows := Usage(p, false)
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	r := rows[0]
	if r.StorageBytes != 900 || r.StorageLimitBytes != 2000 || r.ProtectedQuotaBytes != 1000 || r.Boosters != 3 || r.M365Accounts != 8 ||
		r.Devices != 2 || r.BillableDevices != 2 || r.SynologyDevices != 1 || r.ProtectedItems != 5 {
		t.Fatalf("usage row wrong: %+v", r)
	}
	if r.MaxVaultPct == nil || *r.MaxVaultPct != 45 {
		t.Fatalf("vault pct must compare stored bytes with the vault limit (900/2000), got %v", r.MaxVaultPct)
	}
	if r.MaxQuotaPct != 80 { // M365 8/10 is the highest ratio (vault 45%, devices 50%)
		t.Fatalf("max quota pct %v", r.MaxQuotaPct)
	}
	if v := Usage(p, true); len(v) != 1 || v[0].VaultID != "vault-1" || v[0].StorageLimitBytes != 2000 {
		t.Fatalf("by vault wrong: %+v", v)
	}
}

func TestUsageQuotaEdges(t *testing.T) {
	vault := func(size, limit int64, sharedID string) Vault {
		v := Vault{StorageLimitEnabled: limit > 0, StorageLimitBytes: limit, StorageLimitID: sharedID}
		v.Statistics.ClientProvidedSize.Size = size
		return v
	}
	p := map[string]Profile{"t": {Username: "t", Destinations: map[string]Vault{
		"hot":    vault(90, 100, ""),
		"cold":   vault(10, 900, ""),
		"shared": vault(500, 1, "squota-1"),
	}}}
	r := Usage(p, false)[0]
	if r.MaxVaultPct == nil || *r.MaxVaultPct != 90 || r.MaxQuotaPct != 90 {
		t.Fatalf("one vault at 90%% must surface even when the tenant aggregate is low: %+v", r)
	}
	if r.SharedQuotaVaults != 1 || r.StorageLimitBytes != 1000 {
		t.Fatalf("shared-quota vault must not contribute its ignored StorageLimitBytes: %+v", r)
	}
	for _, v := range Usage(p, true) {
		if v.VaultID == "shared" && (v.StorageLimitBytes != 0 || v.MaxVaultPct != nil || v.SharedQuotaID != "squota-1") {
			t.Fatalf("shared vault row wrong: %+v", v)
		}
	}
}

func TestDevicesStaleOutdated(t *testing.T) {
	p, _ := DecodeProfiles(fixture(t))
	rows := Devices(p, live, now)
	stale := Stale(rows, 24*time.Hour)
	if len(stale) != 1 || stale[0].DeviceID != "dev-b" || !stale[0].StillBillable {
		t.Fatalf("stale wrong: %+v", stale)
	}
	if len(Stale(rows, 30*24*time.Hour)) != 0 {
		t.Fatal("dev-b last ran 80h ago; a 30d threshold must exclude it")
	}
	out, counts := Outdated(rows, "26.8.7")
	if len(out) != 1 || out[0].DeviceID != "dev-b" || counts["26.8.7"] != 1 || counts["26.5.0"] != 1 {
		t.Fatalf("outdated wrong: %+v %v", out, counts)
	}
}

func TestDiff(t *testing.T) {
	p, _ := DecodeProfiles(fixture(t))
	snap := Snapshot{Items: Health(p, live, HealthOptions{Now: now, Grace: 6 * time.Hour, MaxAge: 48 * time.Hour}),
		Devices: Devices(p, live, now), Tenants: Usage(p, false), Vaults: Usage(p, true)}
	if d := Diff(snap, snap); len(d) != 0 {
		t.Fatalf("identical snapshots must not produce changes: %+v", d)
	}
	next := snap
	next.Items = append([]HealthRow(nil), snap.Items...)
	for i := range next.Items {
		if next.Items[i].ItemID == "item-failed" {
			next.Items[i].State = "ok"
		}
	}
	next.Devices = append([]DeviceRow(nil), snap.Devices...)
	next.Devices[0].Online = false
	kinds := map[string]bool{}
	for _, c := range Diff(snap, next) {
		kinds[c.Kind] = true
	}
	if !kinds["problem_resolved"] || !kinds["device_offline"] || len(kinds) != 2 {
		t.Fatalf("unexpected kinds: %v", kinds)
	}
	if d := Diff(Snapshot{}, snap); len(d) == 0 {
		t.Fatal("first snapshot vs empty should list added inventory")
	}
}

func TestBoostersCountDevicePairs(t *testing.T) {
	src := func(dev, engine string) Source { return Source{OwnerDevice: dev, Engine: engine} }
	p := map[string]Profile{"t": {Username: "t",
		Devices: map[string]Device{"d1": {}, "d2": {}, "nas": {PlatformVersion: struct {
			Distribution string `json:"distribution"`
			OS           string `json:"os"`
		}{Distribution: "Synology DSM"}}, "idle-nas": {PlatformVersion: struct {
			Distribution string `json:"distribution"`
			OS           string `json:"os"`
		}{Distribution: "Synology DSM"}}},
		Sources: map[string]Source{
			"a": src("d1", "engine1/mssql"), "b": src("d1", "engine1/mssql"), // same device+engine: one booster
			"c": src("d2", "engine1/mssql"), "d": src("d1", "engine1/file"), "e": src("nas", "engine1/file"),
		}}}
	r := Usage(p, false)[0]
	if r.Boosters != 3 || r.SynologyDevices != 1 || r.BillableDevices != 3 {
		t.Fatalf("want 2 mssql device pairs + 1 Synology with items = 3 boosters, 3 billable devices: %+v", r)
	}
}

func TestHealthZoneAndSparseSchedules(t *testing.T) {
	at := func(v string) time.Time { tt, _ := time.Parse(time.RFC3339, v); return tt }
	sundayDaily := Schedule{FrequencyType: 8011, SecondsPast: 3600, RestrictDays: true}
	sundayDaily.DaysSelect.Sunday = true
	sundayMonthly := Schedule{FrequencyType: 8014, RestrictDays: true}
	sundayMonthly.DaysSelect.Sunday = true
	item := func(succ string) Source {
		s := Source{Engine: "engine1/file", OwnerDevice: "d"}
		s.Statistics.LastSuccessfulBackupJob = Job{GUID: "j", Status: 5000, EndTime: at(succ).Unix()}
		s.Statistics.LastBackupJob = s.Statistics.LastSuccessfulBackupJob
		return s
	}
	run := func(dev Device, sch Schedule, succ, now string) HealthRow {
		p := map[string]Profile{"t": {Username: "t", Devices: map[string]Device{"d": dev},
			Sources:     map[string]Source{"i": item(succ)},
			BackupRules: map[string]Rule{"r": {Source: "i", Schedules: []Schedule{sch}}}}}
		return Health(p, nil, HealthOptions{Now: at(now), Grace: 6 * time.Hour, MaxAge: 48 * time.Hour})[0]
	}
	// Unknown zone with a day restriction: eligibility depends on the zone and
	// its DST rules, so the schedule is reported unresolved, never overdue.
	if r := run(Device{}, sundayDaily, "2026-03-07T12:00:00Z", "2026-03-10T08:00:00Z"); r.Overdue || !r.ScheduleUnresolved {
		t.Fatalf("unknown zone with restriction must be unresolved, not overdue: %+v", r)
	}
	dstWindow := Schedule{FrequencyType: 8011, SecondsPast: 9000, RestrictDays: true, RestrictRuntime: true,
		FromTime: HourMinute{Hour: 2}, ToTime: HourMinute{Hour: 3}}
	dstWindow.DaysSelect.Sunday = true
	if r := run(Device{}, dstWindow, "2026-03-07T12:00:00Z", "2026-03-10T08:00:00Z"); r.Overdue || !r.ScheduleUnresolved {
		t.Fatalf("round-11 unknown-zone DST case must be unresolved: %+v", r)
	}
	if r := run(Device{DeviceTimezone: "America/New_York"}, dstWindow, "2026-03-07T12:00:00Z", "2026-03-10T08:00:00Z"); r.Overdue || r.ExpectedRunAt != "2026-03-15T06:30:00Z" {
		t.Fatalf("known zone must skip the spring-forward window: %+v", r)
	}
	// Unresolved wins over the missing-start fallback (never asserted overdue).
	noStart := map[string]Profile{"t": {Username: "t", Devices: map[string]Device{"d": {}},
		Sources:     map[string]Source{"i": {Engine: "engine1/file", OwnerDevice: "d"}},
		BackupRules: map[string]Rule{"r": {Source: "i", Schedules: []Schedule{dstWindow}}}}}
	if r := Health(noStart, nil, HealthOptions{Now: at("2026-03-10T08:00:00Z"), Grace: 6 * time.Hour, MaxAge: 48 * time.Hour})[0]; r.Overdue || !r.ScheduleUnresolved {
		t.Fatalf("unresolved item without a start must not be overdue: %+v", r)
	}
	// Unknown zone, unrestricted daily: evaluated across every offset.
	if r := run(Device{}, Schedule{FrequencyType: 8011, SecondsPast: 3600}, "2026-03-07T12:00:00Z", "2026-03-10T08:00:00Z"); !r.Overdue || r.ScheduleUnresolved {
		t.Fatalf("unrestricted daily missed for days must be overdue in every zone: %+v", r)
	}
	// Known zone, same facts: overdue in UTC.
	if r := run(Device{DeviceTimezone: "UTC"}, sundayDaily, "2026-03-07T12:00:00Z", "2026-03-10T08:00:00Z"); !r.Overdue {
		t.Fatalf("UTC device should be overdue: %+v", r)
	}
	// Sparse monthly schedule: a missed run more than 390 days back still counts.
	if r := run(Device{DeviceTimezone: "UTC"}, sundayMonthly, "2027-07-02T00:00:00Z", "2028-09-01T00:00:00Z"); !r.Overdue || r.ExpectedRunAt != "2027-08-01T00:00:00Z" {
		t.Fatalf("missed monthly run not detected: %+v", r)
	}
}

// TestNextRunOutOfRangeFields: Comet sends schedule fields as unsigned 64-bit
// integers. Values too large for the walk's signed arithmetic used to wrap
// (a periodic SecondsPast of 1<<63 never terminated); they must come back as
// unresolved, never as a run time and never as a hang.
func TestNextRunOutOfRangeFields(t *testing.T) {
	after := now.Add(-48 * time.Hour)
	limit := now.Add(366 * 24 * time.Hour)
	huge := uint64(1) << 63
	cases := map[string]Schedule{
		"periodic SecondsPast":   {FrequencyType: freqPeriodic, SecondsPast: huge},
		"periodic SecondsPast+1": {FrequencyType: freqPeriodic, SecondsPast: huge + 1},
		"daily SecondsPast":      {FrequencyType: freqDaily, SecondsPast: huge},
		"window FromTime.Hour":   {FrequencyType: freqDaily, SecondsPast: 3600, RestrictRuntime: true, FromTime: HourMinute{Hour: huge}},
		"window ToTime.Minutes":  {FrequencyType: freqDaily, SecondsPast: 3600, RestrictRuntime: true, ToTime: HourMinute{Minutes: huge}},
		"RandomDelaySecs":        {FrequencyType: freqDaily, SecondsPast: 3600, RandomDelaySecs: huge},
	}
	for name, s := range cases {
		done := make(chan [2]bool, 1)
		go func(s Schedule) {
			_, found, incomplete := NextRun(s, after, limit, time.UTC)
			done <- [2]bool{found, incomplete}
		}(s)
		select {
		case got := <-done:
			if got[0] || !got[1] {
				t.Errorf("%s: found=%v incomplete=%v, want found=false incomplete=true", name, got[0], got[1])
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: NextRun did not return", name)
		}
	}
	// In-range values are unaffected.
	s := Schedule{FrequencyType: freqDaily, SecondsPast: 3600, RandomDelaySecs: 60}
	if _, found, incomplete := NextRun(s, after, limit, time.UTC); !found || incomplete {
		t.Errorf("in-range daily: found=%v incomplete=%v", found, incomplete)
	}
}

// TestDiffRemovedProblemResolves: a problem item that disappears must close
// its ticket with problem_resolved (health category); a healthy item that
// disappears is inventory only.
func TestDiffRemovedProblemResolves(t *testing.T) {
	prev := Snapshot{Items: []HealthRow{
		{Tenant: "acme", ItemID: "bad", Item: "Docs", State: "overdue"},
		{Tenant: "acme", ItemID: "good", Item: "Mail", State: "ok"},
	}}
	got := map[string][]string{}
	for _, c := range Diff(prev, Snapshot{}) {
		got[c.Key] = append(got[c.Key], c.Kind+"/"+c.Category+"/"+c.After)
	}
	if want := []string{"item_removed/inventory/", "problem_resolved/health/removed"}; strings.Join(got["acme/bad"], ",") != strings.Join(want, ",") {
		t.Errorf("removed problem item: got %v, want %v", got["acme/bad"], want)
	}
	if want := []string{"item_removed/inventory/"}; strings.Join(got["acme/good"], ",") != strings.Join(want, ",") {
		t.Errorf("removed healthy item: got %v, want %v", got["acme/good"], want)
	}
}

// Round 1, documented timezone fallback: a device with no timezone of its own
// and no live connection is walked in the account's LocalTimezone
// (UserProfileConfig.LocalTimezone, comet-go-sdk cometsdk.go "Individual
// devices may declare a more specific timezone").
func TestHealthFallsBackToProfileLocalTimezone(t *testing.T) {
	p, err := DecodeProfiles([]byte(`{"u":{"Username":"u","LocalTimezone":"UTC","Devices":{"d":{}},"Sources":{"s":{"OwnerDevice":"d","Statistics":{"LastBackupJob":{"GUID":"j","Status":5000,"EndTime":1609718400},"LastSuccessfulBackupJob":{"GUID":"j","Status":5000,"EndTime":1609718400}}}},"BackupRules":{"r":{"Source":"s","Schedules":[{"FrequencyType":8011,"RestrictDays":true,"DaysSelect":{"Monday":true}}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2021-01-20T12:00:00Z")
	r := Health(p, nil, HealthOptions{Now: now, Grace: 6 * time.Hour, MaxAge: 48 * time.Hour})[0]
	if !r.Overdue || r.ScheduleUnresolved {
		t.Fatalf("overdue=%v unresolved=%v", r.Overdue, r.ScheduleUnresolved)
	}
}
