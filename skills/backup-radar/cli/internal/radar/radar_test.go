// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package radar

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

// Fixtures follow the v3 ClientsDevicesPage/ClientsHealthPage and v2
// ApiBackup/ApiBackupInactive examples in the official specs, with synthetic
// names.
const devicesJSON = `[
 {"device_id":"dev-1","br_device_id":"Q2c4N1RhM06xY1k9","client":{"name":"Corp","br_company_name":"Corp BR","client_id":"cli-1"},
  "device_name":"Server-01","br_device_name":"SERVER-01","br_job_name":"Nightly Backup","br_backup_classification":"Primary Backup",
  "last_result":[{"status":"error","date":"2026-10-10T02:00:00Z"}],
  "history":[{"date":"2026-10-13T00:00:00Z","sla_status":"Out Of Compliance - No Results"},{"date":"2026-10-12T00:00:00Z","sla_status":"Out Of Compliance"},{"date":"2026-10-11T00:00:00Z","sla_status":"In Compliance"}]},
 {"device_id":null,"br_device_id":"Zz9_unmapped","client":{"name":null,"br_company_name":"Fabrik BR","client_id":null},
  "device_name":null,"br_device_name":"fab-nas.fabrik.local","br_job_name":"NAS Sync","last_result":[],
  "history":[{"date":"2026-10-13T00:00:00Z","sla_status":"At Risk"}]},
 {"device_id":null,"br_device_id":"Mm1_devonly","client":{"name":"Corp","br_company_name":"Corp BR","client_id":"cli-1"},
  "device_name":null,"br_device_name":"LAPTOP-7","br_job_name":"Daily","last_result":[{"status":"success","date":"2026-10-13T01:00:00Z"}],
  "history":[{"date":"2026-10-13T00:00:00Z","sla_status":"In Compliance"}]}
]`

const backupsJSON = `[
 {"ticketingCompany":"CW Company","status":{"id":3,"name":"Failure"},"daysInStatus":2.24,"lastResult":"2026-10-10T02:00:00","lastSuccess":"2026-10-09T02:00:00",
  "ticketCount":0,"treatWarningAsSuccess":false,"tags":["Veeam"],"note":"Primary DC","backupId":1000,"companyName":"Corp BR","deviceName":"SERVER-01","jobName":"Nightly Backup","methodName":"Veeam",
  "history":[{"status":{"id":3,"name":"Failure"},"date":"2026-10-13T00:00:00","countFailure":1,"countWarning":0,"countSuccess":0,"countNoResult":0}]},
 {"ticketingCompany":"[Undefined]","status":{"id":2,"name":"Warning"},"daysInStatus":0.3,"lastResult":"2026-10-14T02:00:00","lastSuccess":"2026-10-14T02:00:00",
  "ticketCount":1,"treatWarningAsSuccess":true,"backupId":1003,"companyName":"XYZ Company","deviceName":"BR-DVS01","jobName":"BR Daily Report","methodName":"Veeam",
  "history":[{"status":{"id":2,"name":"Warning"},"date":"2026-10-13T00:00:00","countFailure":0,"countWarning":2,"countSuccess":0,"countNoResult":0}]},
 {"ticketingCompany":"CW Company","status":{"id":1,"name":"Success"},"daysInStatus":4,"lastResult":"2026-08-01T02:00:00","lastSuccess":"2026-08-01T02:00:00",
  "ticketCount":0,"backupId":1004,"companyName":"Old Co","deviceName":"OLD-SRV","jobName":"Weekly","methodName":"Datto"}
]`

const inactiveJSON = `[{"emailFrom":"[Undefined]","lastReceived":"2026-10-14T04:35:47","backupId":12001,"companyName":"[Undefined]","deviceName":"SERVER09","jobName":"OneTwo - (dead0000beef)","methodName":"Datto API"}]`

const healthJSON = `[
 {"client":{"name":"Corp","client_id":"cli-1"},"history":[
   {"date":"2026-10-13T00:00:00Z","compliance":{"pending_count":5,"at_risk_count":1,"in_compliance_count":7,"out_of_compliance_count":1,"out_of_compliance_no_results_count":1}},
   {"date":"2026-10-06T00:00:00Z","compliance":{"pending_count":0,"at_risk_count":0,"in_compliance_count":10,"out_of_compliance_count":0,"out_of_compliance_no_results_count":0}},
   {"date":"2026-05-01T00:00:00Z","compliance":{"pending_count":0,"at_risk_count":0,"in_compliance_count":0,"out_of_compliance_count":50,"out_of_compliance_no_results_count":0}}]},
 {"client":{"name":"Contoso","client_id":"cli-2"},"history":[
   {"date":"2026-10-13T00:00:00Z","compliance":{"pending_count":0,"at_risk_count":0,"in_compliance_count":4,"out_of_compliance_count":0,"out_of_compliance_no_results_count":0}}]},
 {"client":{"name":"Contoso East","client_id":"cli-3"},"history":[]}
]`

func snapshot(t *testing.T) Snapshot {
	t.Helper()
	var s Snapshot
	for _, f := range []struct {
		src string
		dst any
	}{{devicesJSON, &s.Devices}, {backupsJSON, &s.Backups}, {inactiveJSON, &s.Inactive}, {healthJSON, &s.Health}} {
		if err := json.Unmarshal([]byte(f.src), f.dst); err != nil {
			t.Fatal(err)
		}
	}
	s.Synced = map[string]bool{"devices": true, "health": true, "backups": true, "backups-inactive": true}
	return s
}

func TestStateMapping(t *testing.T) {
	for in, want := range map[string]string{"Out Of Compliance - No Results": StateNoResults, "out of compliance": StateFailing, "At Risk": StateAtRisk, "In Compliance": StateOK, "Pending": StateOK, "": StateOK} {
		if got := StateFromSLA(in); got != want {
			t.Errorf("StateFromSLA(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"No Result": StateNoResults, "Failure": StateFailing, "Warning": StateWarning, "Success": StateOK, "Pending": StateOK} {
		if got := StateFromV2(in); got != want {
			t.Errorf("StateFromV2(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckMergesAndRanks(t *testing.T) {
	s := snapshot(t)
	rows := Check(s, false, "", now)
	if len(rows) != 1 {
		t.Fatalf("want 1 failing row without at-risk, got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	// v3 sets the more severe state; no results has lasted one day (the day
	// before was plain out of compliance), so it is new with 1 day.
	if r.Source != "both" || r.State != StateNoResults || r.Trend != "new" || r.DaysInState != 1 || r.TicketCount == nil || *r.TicketCount != 0 || r.BackupID != 1000 || r.BRDeviceID != "Q2c4N1RhM06xY1k9" {
		t.Errorf("merged row wrong: %+v", r)
	}
	all := Check(s, true, "", now)
	if len(all) != 3 || all[0].State != StateNoResults || all[1].State != StateAtRisk && all[1].State != StateWarning {
		t.Fatalf("include-at-risk ordering wrong: %+v", all)
	}
	if got := Check(s, true, "xyz", now); len(got) != 1 || got[0].Device != "BR-DVS01" {
		t.Errorf("client filter: %+v", got)
	}
	if got := Check(Snapshot{}, true, "", now); got == nil || len(got) != 0 {
		t.Errorf("empty snapshot must give empty non-nil slice, got %#v", got)
	}
}

func TestUnwatchedReasons(t *testing.T) {
	rows := Unwatched(snapshot(t), AllReasons, now)
	got := map[string][]string{}
	for _, r := range rows {
		got[r.Reason] = append(got[r.Reason], r.Device)
	}
	want := map[string][]string{
		ReasonInactive:   {"SERVER09"},
		ReasonUnmapped:   {"LAPTOP-7", "fab-nas.fabrik.local"}, // sorted by client: Corp, Fabrik BR
		ReasonTicketless: {"SERVER-01"},
		ReasonNoPSA:      {"BR-DVS01"},
		ReasonMasked:     {"BR-DVS01"},
	}
	for reason, devices := range want {
		if strings.Join(got[reason], ",") != strings.Join(devices, ",") {
			t.Errorf("%s: got %v want %v", reason, got[reason], devices)
		}
	}
	// Server-01 is mapped (client and device ids present), so never "unmapped".
	for _, d := range got[ReasonUnmapped] {
		if d == "Server-01" {
			t.Error("mapped device reported as unmapped")
		}
	}
	if only := Unwatched(snapshot(t), []string{ReasonInactive}, now); len(only) != 1 || only[0].Client != "(unassigned)" {
		t.Errorf("reason filter / unassigned display: %+v", only)
	}
}

func TestCoverageDiff(t *testing.T) {
	res := Coverage(snapshot(t), []string{"server-01", "fab-nas", "SERVER09.corp.example", "ghost-1", "ghost-1", " "}, "")
	if res.Counts["vendor_devices"] != 4 || res.Counts["matched"] != 3 || res.Counts["vendor_only"] != 1 {
		t.Fatalf("counts: %+v", res.Counts)
	}
	methods := map[string]string{}
	for _, m := range res.Matched {
		methods[m.Name] = m.MatchMethod
		if m.Name == "SERVER09.corp.example" && m.Monitored {
			t.Error("inactive-only device must be monitored=false")
		}
	}
	if methods["server-01"] != "exact" || methods["fab-nas"] != "short-name" || methods["SERVER09.corp.example"] != "short-name" {
		t.Errorf("match methods: %v", methods)
	}
	if res.VendorOnly[0].Name != "ghost-1" {
		t.Errorf("vendor only: %+v", res.VendorOnly)
	}
	for _, b := range res.BackupRadarOnly {
		if strings.EqualFold(b.Device, "server-01") {
			t.Error("matched device listed as backup-radar-only")
		}
	}
	if len(res.BackupRadarOnly) != 3 { // LAPTOP-7, BR-DVS01, OLD-SRV; the v2 SERVER-01 row merges with v3
		t.Errorf("backup radar only: %+v", res.BackupRadarOnly)
	}
	// --client: a device monitored under another client is matched (other_client), never a gap.
	if f := Coverage(snapshot(t), []string{"server-01"}, "xyz"); f.Counts["matched_other_client"] != 1 || f.Counts["vendor_only"] != 0 || f.Counts["backup_radar_only"] != 1 {
		t.Errorf("client filter: %+v", f.Counts)
	}
	// ScalePad client names scope too (Corp is the ScalePad name of "Corp BR").
	if f := Coverage(snapshot(t), []string{"server-01"}, "Corp"); f.Counts["matched"] != 1 || f.Counts["matched_other_client"] != 0 {
		t.Errorf("ScalePad-name client filter: %+v", f.Counts)
	}
}

func TestStale(t *testing.T) {
	rows := Stale(snapshot(t), 30, "", false, now)
	got := map[string]StaleRow{}
	for _, r := range rows {
		got[r.Device] = r
	}
	if len(rows) != 2 {
		t.Fatalf("want OLD-SRV (74 days) and fab-nas (never), got %+v", rows)
	}
	// OLD-SRV is v2-only: no v3 row tells whether it is mapped, so no body.
	if r := got["OLD-SRV"]; r.DaysSilent == nil || *r.DaysSilent != 74 || r.RetireRequest != nil || r.RetireNote == "" {
		t.Errorf("OLD-SRV: %+v", r)
	}
	if r := got["fab-nas.fabrik.local"]; !r.NeverReported || r.DaysSilent != nil || r.RetireRequest == nil || r.RetireRequest.ClientName != "Fabrik BR" {
		t.Errorf("never-reported row: %+v", r)
	}
	if rows := Stale(snapshot(t), 3, "corp", true, now); len(rows) != 1 || rows[0].Device != "Server-01" || rows[0].RetireRequest == nil || rows[0].RetireRequest.DeviceID != "dev-1" || rows[0].RetireRequest.DeviceName != "" || !rows[0].RetireRequest.ClosePSATickets {
		t.Errorf("mapped device retires by ScalePad device id: %+v", rows)
	}
	if rows := Stale(snapshot(t), 400, "old co", false, now); len(rows) != 0 {
		t.Errorf("nothing silent 400 days: %+v", rows)
	}
}

func TestScorecard(t *testing.T) {
	scores := Scorecard(snapshot(t), 90, true, now)
	if len(scores) != 3 {
		t.Fatalf("scores: %+v", scores)
	}
	if scores[0].Client != "Corp" || scores[1].Client != "Contoso" || scores[2].Rate != nil {
		t.Errorf("ranking worst-first with nil last: %v %v %v", scores[0].Client, scores[1].Client, scores[2].Client)
	}
	corp := scores[0]
	// May entry is outside 90 days; pending excluded: 17/(17+1+1+1) = 85%.
	if corp.DaysCovered != 2 || corp.Rate == nil || *corp.Rate != 85 || corp.Counts.Pending != 5 {
		t.Errorf("corp: %+v rate=%v", corp, corp.Rate)
	}
	if len(corp.Weekly) != 2 || corp.Weekly[0].WeekStart != "2026-10-05" || corp.Weekly[1].WeekStart != "2026-10-12" {
		t.Errorf("weekly: %+v", corp.Weekly)
	}
	if len(corp.WorstDevices) != 1 || corp.WorstDevices[0].Device != "Server-01" || corp.WorstDevices[0].BadDays != 2 {
		t.Errorf("worst devices: %+v", corp.WorstDevices)
	}
	if _, err := FindClient(scores, "contoso"); err != nil {
		t.Errorf("exact name should win over substring: %v", err)
	}
	if _, err := FindClient(scores, "conto"); err == nil || !strings.Contains(err.Error(), "matches 2 clients") {
		t.Errorf("ambiguous substring must error, got %v", err)
	}
	if c, err := FindClient(scores, "cli-1"); err != nil || c.Client != "Corp" {
		t.Errorf("client id lookup: %v %v", c, err)
	}
}

func TestTimeline(t *testing.T) {
	jobs, days := Timeline(snapshot(t), "server-01", "", "", 30, now)
	if len(jobs) != 1 || jobs[0].BackupID != 1000 || jobs[0].BRDeviceID != "Q2c4N1RhM06xY1k9" || jobs[0].TicketCount == nil {
		t.Fatalf("jobs: %+v", jobs)
	}
	if len(days) != 3 || days[0].Date != "2026-10-13" || days[0].SLAStatus != "Out Of Compliance - No Results" || days[0].V2Status != "Failure" || days[0].Failure == nil || *days[0].Failure != 1 {
		t.Errorf("days: %+v", days)
	}
	if _, d := Timeline(snapshot(t), "server", "Nightly Backup", "", 1, now); len(d) != 1 || d[0].Date != "2026-10-13" {
		t.Errorf("substring + job + 1-day window (the newest synced day): %+v", d)
	}
	if j, d := Timeline(snapshot(t), "no-such-host", "", "", 30, now); len(j) != 0 || len(d) != 0 {
		t.Errorf("unknown device must be empty: %v %v", j, d)
	}
}

func TestParseFlatRowsAndChunks(t *testing.T) {
	csvIn := "client,device,job,type,time,status,notes,data_selected\n" +
		"Corp,SRV1,Nightly,backup,2026-10-13T02:00:00Z,success,ok;fast,100\n" +
		"Corp,SRV1,Nightly,replication,2026-10-13 03:00:00,warning,,\n" +
		"Corp,SRV2,Nightly,,2026-10-13T02:00:00Z,failure,,\n"
	rows, errs := ParseFlatRows([]byte(csvIn))
	if len(errs) != 0 || len(rows) != 3 {
		t.Fatalf("rows=%d errs=%v", len(rows), errs)
	}
	if rows[1].Kind != "replications" || rows[1].Time != "2026-10-13T03:00:00Z" || len(rows[0].Notes) != 2 || *rows[0].DataSelected != 100 {
		t.Errorf("row parse: %+v", rows)
	}
	chunks := BuildChunks(rows, 2)
	if len(chunks) != 2 || chunks[0].Results != 2 || chunks[1].Results != 1 {
		t.Fatalf("chunks: %+v", chunks)
	}
	if e := chunks[0].Entries[0]; e.DeviceName != "SRV1" || len(e.Backups) != 1 || len(e.Replications) != 1 || e.Alerts == nil {
		t.Errorf("entry grouping: %+v", e)
	}
	split := BuildChunks([]FlatRow{rows[0], rows[0], rows[0]}, 2)
	if len(split) != 2 || split[1].Entries[0].DeviceName != "SRV1" {
		t.Errorf("an oversize entry must split with the same identity: %+v", split)
	}
	if one := BuildChunks(rows, 0); len(one) != 1 || one[0].Results != 3 {
		t.Errorf("limit 0 defaults to the API cap: %+v", one)
	}

	_, errs = ParseFlatRows([]byte("client,device,job,time,status\n ,SRV,J,yesterday,failed\n"))
	joined := ""
	for _, e := range errs {
		joined += e.Error() + "|"
	}
	for _, want := range []string{"client_name is required", "time \"yesterday\"", "status \"failed\""} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing error %q in %s", want, joined)
		}
	}

	jsonl := `{"client_name":"Corp","device_name":"SRV1","job_name":"J","time":"2026-10-13T02:00:00Z","status":"success","notes":["a","b"],"data_transferred":5}
{"client_name":"Corp","device_name":"SRV1","job_name":"J","type":"alert","time":"2026-10-13T02:00:00Z","status":"failure"}`
	rows, errs = ParseFlatRows([]byte(jsonl))
	if len(errs) != 0 || len(rows) != 2 || rows[1].Kind != "alerts" || len(rows[0].Notes) != 2 || *rows[0].DataTransferred != 5 {
		t.Errorf("jsonl: %+v %v", rows, errs)
	}
	if rows, errs = ParseFlatRows([]byte(`[{"client":"C","device":"D","job":"J","time":"2026-10-13","status":"SUCCESS"}]`)); len(errs) != 0 || rows[0].Status != "success" {
		t.Errorf("json array: %+v %v", rows, errs)
	}
}

func TestNormalizeHost(t *testing.T) {
	for in, want := range map[string]string{"SRV01.corp.local": "srv01", " Laptop ": "laptop", "10.0.0.5": "10.0.0.5"} {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q want %q", in, got, want)
		}
	}
}

// Regression tests for review findings.

func TestJoinIdentity(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[
 {"device_id":"d1","br_device_id":"A","client":{"name":"Acme","br_company_name":"Acme BR","client_id":"c1"},"device_name":"Prod-DC","br_device_name":"SRV01","br_job_name":"Nightly","history":[{"date":"2026-10-13T00:00:00Z","sla_status":"Out Of Compliance"}]},
 {"device_id":null,"br_device_id":"N1","client":{"name":null,"br_company_name":null,"client_id":null},"device_name":null,"br_device_name":null,"br_job_name":"Dup","history":[{"date":"2026-10-13T00:00:00Z","sla_status":"Out Of Compliance"}]},
 {"device_id":null,"br_device_id":"N2","client":{"name":null,"br_company_name":null,"client_id":null},"device_name":null,"br_device_name":null,"br_job_name":"Dup","history":[{"date":"2026-10-13T00:00:00Z","sla_status":"In Compliance"}],"last_result":[]}]`), &s.Devices)
	_ = json.Unmarshal([]byte(`[
 {"status":{"name":"Success"},"daysInStatus":100,"backupId":7,"companyName":"Acme BR","deviceName":"SRV01","jobName":"Nightly","ticketCount":3,"lastResult":"2026-10-13T01:00:00",
  "history":[{"status":{"name":"Success"},"date":"2026-10-12T00:00:00","countSuccess":1}]},
 {"status":{"name":"Failure"},"daysInStatus":2,"backupId":8,"companyName":"Acme BR","deviceName":"SRV01","jobName":"Weekly","ticketCount":0},
 {"status":{"name":"Failure"},"daysInStatus":2,"backupId":9,"companyName":"[Undefined]","deviceName":"","jobName":"Dup","ticketCount":0}]`), &s.Backups)

	rows := Check(s, false, "", now)
	byID := map[string]CheckRow{}
	for _, r := range rows {
		byID[r.BRDeviceID+"/"+fmt.Sprint(r.BackupID)] = r
	}
	// v3 A joins only the first v2 backup with its names (7); the healthy v2
	// side must not lend its 100 days to the v3 problem.
	if r, ok := byID["A/7"]; !ok || r.Source != "both" || r.DaysInState != 1 || r.Trend != "new" || r.Client != "Acme" {
		t.Errorf("joined row: %+v (all %+v)", r, rows)
	}
	// a different job on the same device keeps its own row
	if r, ok := byID["/8"]; !ok || r.Source != "v2" || r.Client != "Acme" {
		t.Errorf("duplicate-name v2 backup must stay separate and show the ScalePad client: %+v", rows)
	}
	// two v3 devices with null names keep separate identities, and the
	// unassigned v2 backup with the same empty names does not join either.
	if _, ok := byID["N1/0"]; !ok {
		t.Errorf("N1 must be its own row: %+v", rows)
	}
	if r, ok := byID["/9"]; !ok || r.Source != "v2" {
		t.Errorf("ambiguous cross-generation names must not join: %+v", rows)
	}

	stale := Stale(s, 1, "", false, now)
	unscoped := 0
	for _, r := range stale {
		if r.Device == "Prod-DC" && (r.RetireRequest == nil || r.RetireRequest.DeviceID != "d1") {
			t.Errorf("mapped device retires by device id: %+v", r.RetireRequest)
		}
		if r.RetireRequest != nil && !r.RetireRequest.scoped() {
			t.Errorf("unscoped retire body emitted: %+v", r.RetireRequest)
		}
		if r.RetireRequest == nil {
			if r.RetireNote == "" {
				t.Errorf("missing body needs a note: %+v", r)
			}
			unscoped++
		}
	}
	// N1, N2 (no company, no device id) and the two v2-only backups (9, 8) get no body.
	if unscoped != 4 {
		t.Errorf("want 4 rows without bodies, got %d: %+v", unscoped, stale)
	}
	unmapped := Device{DeviceID: "", BRDeviceName: "BR-NAME", DeviceName: "", BRJobName: "J"}
	unmapped.Client.Name, unmapped.Client.BRCompanyName = "Mapped Co", "BR Co"
	if r := retireFor(unmapped); r.ClientName != "BR Co" || r.DeviceName != "BR-NAME" {
		t.Errorf("unmapped device retires by Backup Radar names: %+v", r)
	}

	jobs, days := Timeline(s, "Prod-DC", "", "", 30, now)
	if len(jobs) != 1 || jobs[0].BackupID != 7 || jobs[0].TicketCount == nil {
		t.Fatalf("ScalePad name must pull in the joined v2 backup: %+v", jobs)
	}
	for _, d := range days {
		if d.BackupID != 7 || d.Client != "Acme" || d.Device != "SRV01" {
			t.Errorf("every day carries the job's backup id and identity: %+v", d)
		}
	}
}

func TestAmbiguousKeyNeverJoins(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"device_id":"d1","br_device_id":"A","client":{"name":"Acme","br_company_name":"Acme BR","client_id":"c1"},"br_device_name":"SRV01","br_job_name":"Nightly","history":[{"date":"2026-10-13T00:00:00Z","sla_status":"Out Of Compliance"}]}]`), &s.Devices)
	_ = json.Unmarshal([]byte(`[
 {"status":{"name":"Failure"},"daysInStatus":5,"backupId":1,"companyName":"Acme BR","deviceName":"SRV01","jobName":"Nightly","ticketCount":0},
 {"status":{"name":"No Result"},"daysInStatus":9,"backupId":2,"companyName":"Acme BR","deviceName":"SRV01","jobName":"Nightly","ticketCount":4}]`), &s.Backups)
	rows := Check(s, false, "", now)
	if len(rows) != 3 {
		t.Fatalf("one v3 + two same-named v2 backups must stay three rows: %+v", rows)
	}
	for _, r := range rows {
		if r.Source == "both" {
			t.Errorf("ambiguous key joined: %+v", r)
		}
	}
	for _, r := range Stale(s, 1, "", false, now) {
		if r.RetireRequest != nil && r.RetireRequest.NoResultIn != 1 {
			t.Errorf("retire body must carry no_result_in: %+v", r.RetireRequest)
		}
	}
}

func TestCoverageAmbiguousShortName(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"br_device_id":"1","client":{"br_company_name":"A"},"br_device_name":"srv.alpha.example","br_job_name":"J"}]`), &s.Devices)
	_ = json.Unmarshal([]byte(`[{"backupId":2,"companyName":"B","deviceName":"srv.beta.example","jobName":"J"}]`), &s.Inactive)
	res := Coverage(s, []string{"srv"}, "")
	if len(res.Matched) != 1 || res.Matched[0].MatchMethod != "short-name-ambiguous" || len(res.Matched[0].Candidates) != 2 || res.Matched[0].Monitored || res.Counts["ambiguous"] != 1 {
		t.Errorf("ambiguous: %+v", res)
	}
}

func TestParseFlatRowsReviewCases(t *testing.T) {
	rows, errs := ParseFlatRows([]byte(`[{"client":"C","device":"D","job":"J","time":"2026-10-13T00:00:00Z","status":"success","notes":["a;b"," c "],"data_selected":9007199254740993}]`))
	if len(errs) != 0 || len(rows[0].Notes) != 2 || rows[0].Notes[0] != "a;b" || *rows[0].DataSelected != 9007199254740993 {
		t.Errorf("JSON arrays and big integers must survive: %+v %v", rows, errs)
	}
	if _, errs = ParseFlatRows([]byte(`{"client":"Good","company":"Other","device":"D","job":"J","time":"2026-10-13T00:00:00Z","status":"success"}`)); len(errs) != 1 || !strings.Contains(errs[0].Error(), "disagree") {
		t.Errorf("conflicting aliases must be rejected: %v", errs)
	}
	bom := append([]byte("\xef\xbb\xbf"), []byte("client,device,job,time,status\nC,D,J,2026-10-13T00:00:00Z,success\n")...)
	if rows, errs = ParseFlatRows(bom); len(errs) != 0 || len(rows) != 1 {
		t.Errorf("BOM CSV: %v", errs)
	}
	if _, errs = ParseFlatRows([]byte("Client Name,device,job,time,status\nC,D,J,2026-10-13T00:00:00Z,success\n")); len(errs) != 1 || !strings.Contains(errs[0].Error(), "missing required column") {
		t.Errorf("unknown header must fail once: %v", errs)
	}
	long := strings.Repeat("x", 513)
	if _, errs = ParseFlatRows([]byte("client,device,job,time,status\nC," + long + ",J,2026-10-13T00:00:00Z,success\n")); len(errs) != 1 || !strings.Contains(errs[0].Error(), "at most 512") {
		t.Errorf("device name limit: %v", errs)
	}
}

func TestCoverageClientScopeNoBorrowing(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"backupId":1,"companyName":"Alpha","deviceName":"DC01","jobName":"J"}]`), &s.Inactive)
	_ = json.Unmarshal([]byte(`[{"backupId":2,"companyName":"Beta","deviceName":"DC01","jobName":"J","status":{"name":"Success"}}]`), &s.Backups)
	res := Coverage(s, []string{"DC01"}, "Alpha")
	if len(res.Matched) != 1 || res.Matched[0].Monitored || res.Matched[0].OtherClient {
		t.Errorf("Alpha's inactive DC01 must not borrow Beta's monitored status: %+v", res.Matched)
	}
	res = Coverage(s, []string{"DC01"}, "Gamma")
	if len(res.Matched) != 1 || !res.Matched[0].OtherClient || res.Counts["vendor_only"] != 0 {
		t.Errorf("device only under other clients is other_client: %+v", res)
	}
}

func TestTimelineExactMatchRespectsClient(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[
 {"br_device_id":"a","client":{"br_company_name":"Alpha"},"br_device_name":"SRV","br_job_name":"J","history":[{"date":"2026-10-13T00:00:00Z","sla_status":"In Compliance"}]},
 {"br_device_id":"b","client":{"br_company_name":"Beta"},"br_device_name":"SRV01","br_job_name":"J","history":[{"date":"2026-10-13T00:00:00Z","sla_status":"In Compliance"}]}]`), &s.Devices)
	jobs, _ := Timeline(s, "SRV", "", "Beta", 30, now)
	if len(jobs) != 1 || jobs[0].Device != "SRV01" {
		t.Errorf("--client Beta must fall back to Beta's substring match: %+v", jobs)
	}
}

func TestCSVExtraCellsRejected(t *testing.T) {
	_, errs := ParseFlatRows([]byte("client,device,job,time,status,notes\nC,D,J,2026-10-13T00:00:00Z,success,a, b\n"))
	if len(errs) != 1 {
		t.Errorf("an unquoted comma must be an error, not a truncated note: %v", errs)
	}
}

func TestCoverageExactBeatsScopedShortName(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"status":{"name":"Success"},"backupId":1,"companyName":"Alpha","deviceName":"dc.alpha.example","jobName":"J"},
 {"status":{"name":"Success"},"backupId":2,"companyName":"Beta","deviceName":"dc.beta.example","jobName":"J"}]`), &s.Backups)
	res := Coverage(s, []string{"dc.beta.example"}, "Alpha")
	if len(res.Matched) != 1 || !res.Matched[0].OtherClient || res.Matched[0].MatchMethod != "exact" || res.Counts["backup_radar_only"] != 1 {
		t.Errorf("Beta's exact match must win and Alpha's device stay a gap: %+v", res)
	}
}

func TestReviewRound5(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"status":{"name":"Success"},"backupId":5,"companyName":"Acme BR","deviceName":"BR-SRV","jobName":"J","lastResult":"2026-01-01T00:00:00","ticketingCompany":"PSA","treatWarningAsSuccess":true,
  "history":[{"date":"2026-08-01T00:00:00","countWarning":4},{"date":"2026-10-12T00:00:00","countWarning":1}]}]`), &s.Backups)
	for _, r := range Stale(s, 30, "", false, now) {
		if r.RetireRequest != nil || r.RetireNote == "" {
			t.Errorf("a v2-only row must get a note, not a name-only retire body: %+v", r)
		}
	}
	rows := Unwatched(s, []string{ReasonMasked}, now)
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Detail, "1 warning") {
		t.Errorf("masked must count only the last 31 days up to today: %+v", rows)
	}
}

func TestCheckOngoingAndUnassignedV3(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"br_device_id":"u1","client":{"name":null,"br_company_name":"[Undefined]","client_id":null},"br_device_name":"LOST","br_job_name":"J",
  "history":[{"date":"2026-10-13T00:00:00Z","sla_status":"Out Of Compliance"},{"date":"2026-10-12T00:00:00Z","sla_status":"Out Of Compliance"},{"date":"2026-10-11T00:00:00Z","sla_status":"Out Of Compliance - No Results"}]}]`), &s.Devices)
	rows := Check(s, false, "", now)
	if len(rows) != 1 || rows[0].Trend != "ongoing" || rows[0].DaysInState != 2 || rows[0].Client != "(unassigned)" {
		t.Errorf("two days out of compliance (then a different state) is ongoing/2 and shows (unassigned): %+v", rows)
	}
}

func TestReviewRound6(t *testing.T) {
	if _, errs := ParseFlatRows([]byte(`{"client":"Alpha","Client":"Beta","device":"D","job":"J","time":"2026-10-13T00:00:00Z","status":"success"}`)); len(errs) == 0 || !strings.Contains(errs[0].Error(), "differ only in case") {
		t.Errorf("case-only duplicate keys must be rejected: %v", errs)
	}
	if _, errs := ParseFlatRows([]byte("client,device,job,time,status,Client\nA,D,J,2026-10-13T00:00:00Z,success,B\n")); len(errs) != 1 || !strings.Contains(errs[0].Error(), "repeats column") {
		t.Errorf("repeated CSV header must be rejected: %v", errs)
	}
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"br_device_id":"g","client":{"br_company_name":"C"},"br_device_name":"D","br_job_name":"J",
  "history":[{"date":"2026-10-13T00:00:00Z","sla_status":"Out Of Compliance"},{"date":"2026-10-01T00:00:00Z","sla_status":"Out Of Compliance"}]}]`), &s.Devices)
	if rows := Check(s, false, "", now); len(rows) != 1 || rows[0].DaysInState != 1 || rows[0].Trend != "new" {
		t.Errorf("a gap in stored days must end the streak: %+v", rows)
	}
}

func TestReviewRound9(t *testing.T) {
	if _, errs := ParseFlatRows([]byte(`{"client":"A","client":"B","device":"D","job":"J","time":"2026-10-13T00:00:00Z","status":"failure","status":"success"}`)); len(errs) == 0 || !strings.Contains(errs[0].Error(), "repeats key") {
		t.Errorf("identically repeated keys must be rejected: %v", errs)
	}
	if rows, errs := ParseFlatRows([]byte(`[{"client":"A","device":"D","job":"J","time":"2026-10-13T00:00:00Z","status":"success","notes":["a","a"]}]`)); len(errs) != 0 || len(rows) != 1 {
		t.Errorf("repeated array values are not repeated keys: %v", errs)
	}
	d := Device{BRJobName: "Daily"}
	d.Client.BRCompanyName = "Acme"
	if r := retireFor(d); r.scoped() {
		t.Errorf("an unmapped device with no device name must not get a body: %+v", r)
	}
}

func TestDuplicateKeysAfterHugeNumber(t *testing.T) {
	if _, errs := ParseFlatRows([]byte(`{"unused":1e400,"client":"A","device":"D","job":"J","time":"2026-10-13T00:00:00Z","status":"failure","status":"success"}`)); len(errs) == 0 || !strings.Contains(errs[0].Error(), "repeats key") {
		t.Errorf("a huge number must not hide a repeated key: %v", errs)
	}
}

func TestCoverageSameNameTwoClientsInScope(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"backupId":1,"companyName":"Alpha Dental","deviceName":"DC01","jobName":"J"}]`), &s.Inactive)
	_ = json.Unmarshal([]byte(`[{"status":{"name":"Success"},"backupId":2,"companyName":"Beta Dental","deviceName":"DC01","jobName":"J"}]`), &s.Backups)
	res := Coverage(s, []string{"DC01"}, "Dental")
	if len(res.Matched) != 1 || res.Matched[0].Monitored || res.Matched[0].MatchMethod != "exact-ambiguous" || len(res.Matched[0].Candidates) != 2 || res.Counts["ambiguous"] != 1 {
		t.Errorf("two clients' DC01 must stay apart and report ambiguity: %+v", res.Matched)
	}
}

func TestCoverageSameNamedClientsStaySeparate(t *testing.T) {
	// Backup Radar companies Alpha and Beta both map to ScalePad clients named
	// Contoso (different client ids). Alpha's active DC01 must not hide Beta's
	// inactive DC01.
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"br_device_id":"1","client":{"name":"Contoso","br_company_name":"Alpha","client_id":"c-1"},"br_device_name":"DC01","br_job_name":"J"},
		{"br_device_id":"2","client":{"name":"Contoso","br_company_name":"Beta","client_id":"c-2"},"br_device_name":"FS01","br_job_name":"J"}]`), &s.Devices)
	_ = json.Unmarshal([]byte(`[{"backupId":7,"companyName":"Beta","deviceName":"DC01","jobName":"Image"}]`), &s.Inactive)
	res := Coverage(s, []string{"DC01"}, "")
	if len(res.Matched) != 1 || res.Matched[0].Monitored || res.Matched[0].MatchMethod != "exact-ambiguous" || len(res.Matched[0].Candidates) != 2 {
		t.Fatalf("same-named clients merged: %+v", res.Matched)
	}
	if res.Matched[0].Candidates[0] == res.Matched[0].Candidates[1] {
		t.Errorf("candidates must name the Backup Radar company: %v", res.Matched[0].Candidates)
	}
}

func TestFindClientDuplicateExactName(t *testing.T) {
	scores := []ClientScore{{Client: "Contoso", ClientID: "c-1"}, {Client: "Contoso", ClientID: "c-2"}}
	if _, err := FindClient(scores, "contoso"); err == nil || !strings.Contains(err.Error(), "c-2") {
		t.Errorf("an exact name shared by two client ids must be ambiguous, got %v", err)
	}
	if c, err := FindClient(scores, "c-2"); err != nil || c.ClientID != "c-2" {
		t.Errorf("client id lookup: %v %v", c, err)
	}
}

func TestDistinctDevicesSameNamedClients(t *testing.T) {
	jobs := []TimelineJob{{Client: "Contoso", BRCompany: "Alpha", Device: "DC01"}, {Client: "Contoso", BRCompany: "Alpha", Device: "DC01", Job: "2"}, {Client: "Contoso", BRCompany: "Beta", Device: "DC01"}}
	got := DistinctDevices(jobs)
	if len(got) != 2 || got[0] != "Contoso / DC01 (Backup Radar: Alpha)" {
		t.Errorf("two companies' same-named machines must stay distinct and labeled: %v", got)
	}
	if got := DistinctDevices(jobs[:2]); len(got) != 1 || got[0] != "Contoso / DC01" {
		t.Errorf("one machine keeps the plain label: %v", got)
	}
}

func TestDayWindowMatchesHistoryDays(t *testing.T) {
	// history_days N = N calendar days ending on the anchor day.
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	if w := newDayWindow(now, 1); !w.has(day(14)) || w.has(day(13)) || w.has(day(15)) {
		t.Errorf("1-day window = %+v", w)
	}
	if w := newDayWindow(now, 7); !w.has(day(8)) || w.has(day(7)) || !w.has(day(14).Add(23*time.Hour)) {
		t.Errorf("7-day window = %+v", w)
	}
}

func TestMergeCheckKeepsDisplayedStateReading(t *testing.T) {
	dst := CheckRow{State: StateAtRisk, DaysInState: 1, Trend: "new"}
	mergeCheck(&dst, CheckRow{State: StateWarning, DaysInState: 5, Trend: "ongoing"})
	if dst.State != StateAtRisk || dst.DaysInState != 1 || dst.Trend != "new" {
		t.Errorf("warning's duration leaked into at_risk: %+v", dst)
	}
	mergeCheck(&dst, CheckRow{State: StateAtRisk, DaysInState: 3, Trend: "ongoing"})
	if dst.DaysInState != 3 || dst.Trend != "ongoing" {
		t.Errorf("same state: longer, ongoing reading wins: %+v", dst)
	}
}

func TestWindowEndsOnNewestSyncedDay(t *testing.T) {
	// 02:00Z on Oct 14 is still Oct 13 for a New York tenant: the API's
	// history ends Oct 13, and --days 1 must score that day.
	at := time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"client":{"name":"NY","client_id":"c-ny"},"history":[
		{"date":"2026-10-13","compliance":{"in_compliance_count":3,"out_of_compliance_count":1}},
		{"date":"2026-10-12","compliance":{"in_compliance_count":0,"out_of_compliance_count":4}}]}]`), &s.Health)
	scores := Scorecard(s, 1, false, at)
	if len(scores) != 1 || scores[0].DaysCovered != 1 || scores[0].Rate == nil || *scores[0].Rate != 75 {
		t.Errorf("--days 1 must score the tenant's current day: %+v", scores)
	}
	// A bogus far-future date is neither the anchor nor inside the window.
	s.Health[0].History = append(s.Health[0].History, s.Health[0].History[1])
	s.Health[0].History[2].Date = "2027-01-01"
	if got := s.newestDay(at, histHealth); got.Format("2006-01-02") != "2026-10-13" {
		t.Errorf("anchor = %v", got)
	}
	if sc := Scorecard(s, 1, false, at); sc[0].DaysCovered != 1 || *sc[0].Rate != 75 {
		t.Errorf("a future-dated day must not be scored: %+v", sc[0])
	}
	if got := (Snapshot{}).newestDay(at, histHealth); !got.Equal(at) {
		t.Errorf("no history falls back to now: %v", got)
	}
}

func TestCheckIgnoresFutureDatedHistory(t *testing.T) {
	var s Snapshot
	_ = json.Unmarshal([]byte(`[{"br_device_id":"1","client":{"br_company_name":"A"},"br_device_name":"DC01","br_job_name":"J","history":[
		{"date":"2027-01-01","sla_status":"Out Of Compliance"},
		{"date":"2026-10-13","sla_status":"In Compliance"}]}]`), &s.Devices)
	if rows := Check(s, true, "", now); len(rows) != 0 {
		t.Errorf("a future-dated day must not become the latest state: %+v", rows)
	}
}

// TestStaleCarriesClientIDForSameNamedClients: two ScalePad clients named Acme
// keep distinct client_id values on their stale rows (round 4, 2026-10-19).
func TestStaleCarriesClientIDForSameNamedClients(t *testing.T) {
	var s Snapshot
	for _, id := range []string{"c1", "c2"} {
		d := Device{DeviceID: "dev-" + id, DeviceName: "SRV-01", BRJobName: "Nightly"}
		d.Client.Name, d.Client.ClientID, d.Client.BRCompanyName = "Acme", id, "Acme "+id
		s.Devices = append(s.Devices, d)
	}
	rows := Stale(s, 1, "", false, now)
	if len(rows) != 2 || rows[0].ClientID == rows[1].ClientID || rows[0].ClientID == "" || rows[1].ClientID == "" {
		t.Fatalf("same-named clients must keep distinct client ids: %+v", rows)
	}
}

// Two unmapped backups sharing company, device and job names get one body
// that retires both; each row says so and carries its own br_device_id.
// Timeline day rows carry the Backup Radar company and device id.
func TestStaleBodyCollisionAndTimelineDayIdentity(t *testing.T) {
	s := snapshot(t)
	twin := s.Devices[1]
	twin.BRDeviceID = "Zz9_twin"
	twin.BRDeviceName = "FAB-NAS.fabrik.local" // the endpoint matches case-insensitively
	s.Devices = append(s.Devices, twin)
	var fab []StaleRow
	for _, r := range Stale(s, 30, "fabrik", false, now) {
		fab = append(fab, r)
	}
	if len(fab) != 2 || fab[0].BRDeviceID == fab[1].BRDeviceID || !strings.Contains(fab[0].RetireNote, "all 2 stale backups") || !strings.Contains(fab[1].RetireNote, "all 2 stale backups") {
		t.Fatalf("colliding bodies must be flagged on both rows: %+v", fab)
	}
	if rows := Stale(snapshot(t), 30, "fabrik", false, now); len(rows) != 1 || rows[0].RetireNote != "" {
		t.Errorf("a unique body carries no note: %+v", rows)
	}
	if _, days := Timeline(snapshot(t), "server-01", "", "", 30, now); len(days) == 0 || days[0].BRCompany != "Corp BR" || days[0].BRDeviceID != "Q2c4N1RhM06xY1k9" {
		t.Errorf("day rows carry br_company and br_device_id: %+v", days)
	}
}
