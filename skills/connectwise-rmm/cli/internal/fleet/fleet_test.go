// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func TestParseEndpointCasing(t *testing.T) {
	for _, raw := range []string{
		`{"endpointId":"e1","clientId":"c1","siteId":"s1","endpointType":"Server","osType":"Windows"}`,
		`{"endpointID":"e1","clientID":"c1","siteID":"s1","endpointType":"Server","osType":"Windows"}`,
	} {
		e := ParseEndpoint(Object(json.RawMessage(raw)))
		if e.ID != "e1" || e.CompanyID != "c1" || e.SiteID != "s1" || e.Type != "Server" {
			t.Errorf("%s -> %+v", raw, e)
		}
	}
	if got := Objects(json.RawMessage(`{"endpoints":[{"endpointId":"a"},{"endpointId":"b"}]}`), "endpoints"); len(got) != 2 {
		t.Errorf("wrapped list = %d items, want 2", len(got))
	}
	if got := Objects(json.RawMessage(`[{"id":"x"}]`)); len(got) != 1 {
		t.Errorf("bare list = %d items, want 1", len(got))
	}
}

func TestParseHeartbeatsBothCasings(t *testing.T) {
	raw := json.RawMessage(`{"status":"partial_success","successfulRecords":[{"clientID":"c","siteID":"s","endpoints":[
		{"endpointID":"e1","DcDateTimeUTC":"2026-10-13T11:00:00.000Z","Availability":true},
		{"EndpointID":"e2","DcDateTimeUTC":"2026-10-10T12:00:00Z","Availability":false}]}],
		"failedRecords":[{"resourceType":"Endpoint","resource":"e3","error":"Not Found"}]}`)
	hb, failed, ok := ParseHeartbeats(raw)
	if !ok {
		t.Fatal("valid heartbeat envelope reported unreadable")
	}
	if _, _, ok := ParseHeartbeats(json.RawMessage(`{"error":"x"}`)); ok {
		t.Error("an envelope without records must be unreadable")
	}
	if len(hb) != 2 || !hb["e1"].Available || hb["e2"].Available {
		t.Fatalf("heartbeats = %+v", hb)
	}
	if failed["e3"] != "Not Found" || len(failed) != 1 {
		t.Fatalf("failedRecords = %+v", failed)
	}
	if hb["e2"].LastSeen.IsZero() {
		t.Fatal("e2 last seen not parsed")
	}
}

func TestStale(t *testing.T) {
	eps := []Endpoint{{ID: "fresh", CompanyName: "A"}, {ID: "old", CompanyName: "A"}, {ID: "down", CompanyName: "B"}, {ID: "gone", CompanyName: "B"}}
	hb := map[string]Heartbeat{
		"fresh": {Available: true, LastSeen: now.Add(-time.Hour)},
		"old":   {Available: true, LastSeen: now.Add(-72 * time.Hour)},
		"down":  {Available: false, LastSeen: now.Add(-2 * time.Hour)},
	}
	rows := Stale(eps, hb, now, 24*time.Hour)
	got := map[string]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	if got["fresh"] || !got["old"] || !got["down"] || !got["gone"] || len(rows) != 3 {
		t.Fatalf("stale rows = %+v", rows)
	}
	for _, r := range rows {
		if r.ID == "gone" && !r.NoHeartbeat {
			t.Error("endpoint missing from heartbeat must be flagged no_heartbeat")
		}
	}
	if len(Stale(eps[:1], hb, now, 24*time.Hour)) != 0 {
		t.Error("a fresh online endpoint must not be stale")
	}
}

func TestParsePatchSummaryAndPosture(t *testing.T) {
	raw := json.RawMessage(`{"summary":{"assessedAt":"2026-10-12T00:00:00Z","missingCount":4,"pendingRebootCount":1,"installedCount":90,"compliance":82.5,"outOfSupport":false,"policyName":"Workstations"},"patches":[]}`)
	s, ok := ParsePatchSummary(raw)
	if !ok || s.Missing != 4 || s.PendingReboot != 1 || s.Compliance == nil || *s.Compliance != 82.5 || s.Policy != "Workstations" {
		t.Fatalf("summary = %+v ok=%v", s, ok)
	}
	if _, ok := ParsePatchSummary(json.RawMessage(`{"patches":[]}`)); ok {
		t.Error("payload without summary must report !ok")
	}
	good := 99.0
	stale := s
	stale.AssessedAt = now.Add(-30 * 24 * time.Hour)
	eps := []Endpoint{{ID: "e1", CompanyID: "c1", CompanyName: "Risky"}, {ID: "e2", CompanyID: "c2", CompanyName: "Clean"}, {ID: "e3", CompanyID: "c2", CompanyName: "Clean"}, {ID: "e4", CompanyID: "c1", CompanyName: "Risky"}}
	sums := map[string]PatchSummary{"e1": s, "e2": {Compliance: &good, AssessedAt: now}, "e4": stale}
	rows, detail := Posture(eps, sums, 90, 14*24*time.Hour, now)
	if rows[0].Company != "Risky" || rows[0].BelowCompliance != 2 || rows[0].PendingRebootHosts != 2 || rows[0].StaleAssessments != 1 || rows[0].MissingPatches != 8 {
		t.Fatalf("risky row = %+v", rows[0])
	}
	if rows[1].Company != "Clean" || rows[1].Endpoints != 2 || rows[1].Assessed != 1 || rows[1].RiskScore != 0 {
		t.Fatalf("clean row = %+v (unassessed endpoint must not count as assessed)", rows[1])
	}
	if len(detail) != 3 || detail[0].Compliance == nil || *detail[0].Compliance != 82.5 {
		t.Fatalf("detail sorted worst-first = %+v", detail)
	}
}

func TestSystemStateWhoisAndReboot(t *testing.T) {
	raw := json.RawMessage(`{"name":"ACME-WS01","endpointID":"e1","startupStatus":{"lastBootUpTimeUTC":"2026-10-12T08:00:00Z"},
		"lastLoggedOnUser":{"username":"ACME\\jsmith","logonTime":"2026-10-13T08:00:00Z","status":"Active"},
		"loggedOnUsers":[{"sessionID":"0","sessionName":"Services","status":"Disconnected"},
		{"username":"ACME\\jsmith","sessionID":"1","status":"Active","logonTime":"2026-10-13T08:00:00Z"}]}`)
	st, ok := ParseSystemState(raw)
	if !ok || st.Name != "ACME-WS01" || st.LastBoot == "" || st.LastUser == nil || len(st.LoggedOn) != 1 {
		t.Fatalf("state = %+v", st)
	}
	idle, _ := ParseSystemState(json.RawMessage(`{"loggedOnUsers":[{"sessionID":"0","status":"Disconnected"}],"lastLoggedOnUser":{"username":"ACME\\mlee"}}`))
	if _, ok := ParseSystemState(json.RawMessage(`null`)); ok {
		t.Error("null system state must not parse as ok")
	}
	if empty, ok := ParseSystemState(json.RawMessage(`{}`)); !ok || empty.SessionsKnown {
		t.Error("{} is an object but its sessions are unknown")
	}
	if blank, _ := ParseSystemState(json.RawMessage(`{"loggedOnUsers":[]}`)); !blank.SessionsKnown {
		t.Error("an empty loggedOnUsers array means sessions are known and empty")
	}
	noSessions, _ := ParseSystemState(json.RawMessage(`{}`))
	if got := RebootTriage([]Endpoint{{ID: "z"}}, map[string]PatchSummary{"z": {PendingReboot: 1}}, map[string]Heartbeat{"z": {Available: true}}, nil, map[string]SystemState{"z": noSessions}); got[0].Bucket != Coordinate {
		t.Errorf("unknown sessions bucket = %s, want coordinate", got[0].Bucket)
	}
	eps := []Endpoint{{ID: "e1"}, {ID: "e2"}, {ID: "e3"}, {ID: "e4"}}
	states := map[string]SystemState{"e1": st, "e2": idle}

	who := Whois(eps, states, "JSMITH")
	if len(who) != 1 || who[0].ID != "e1" || who[0].Match != "current" {
		t.Fatalf("whois jsmith = %+v", who)
	}
	if last := Whois(eps, states, "mlee"); len(last) != 1 || last[0].Match != "last" {
		t.Fatalf("whois mlee = %+v", last)
	}
	if none := Whois(eps, states, "nobody"); len(none) != 0 {
		t.Fatalf("whois nobody = %+v", none)
	}

	sums := map[string]PatchSummary{"e1": {PendingReboot: 2}, "e2": {PendingReboot: 1}, "e3": {PendingReboot: 1}, "e4": {PendingReboot: 0}}
	hb := map[string]Heartbeat{"e1": {Available: true}, "e2": {Available: true}, "e3": {Available: false}}
	rows := RebootTriage(eps, sums, hb, nil, states)
	want := map[string]string{"e1": Coordinate, "e2": RebootNow, "e3": Offline}
	if len(rows) != 3 {
		t.Fatalf("reboot rows = %+v (e4 has nothing pending)", rows)
	}
	for _, r := range rows {
		if want[r.ID] != r.Bucket {
			t.Errorf("%s bucket = %s, want %s", r.ID, r.Bucket, want[r.ID])
		}
	}
	if rows[0].Bucket != RebootNow {
		t.Error("reboot_now must sort first")
	}
	unknown := RebootTriage([]Endpoint{{ID: "x"}}, map[string]PatchSummary{"x": {PendingReboot: 1}}, map[string]Heartbeat{"x": {Available: true}}, nil, nil)
	if unknown[0].Bucket != Coordinate {
		t.Error("online endpoint with unknown sessions must not be reboot_now")
	}
	failed := RebootTriage([]Endpoint{{ID: "y"}}, map[string]PatchSummary{"y": {PendingReboot: 1}}, nil, map[string]bool{"y": true}, nil)
	if failed[0].Bucket != Unknown {
		t.Errorf("unreadable availability bucket = %s, want %s", failed[0].Bucket, Unknown)
	}
}

func TestTaskInstancesAndFailures(t *testing.T) {
	inst, _ := ParseTaskInstances(json.RawMessage(`{"Results":[
		{"Id":"i-old","OverallStatus":"FAILURE","ExecutedOn":"2026-10-01T00:00:00Z","FailedCount":1},
		{"Id":"i-new","OverallStatus":"RUNNING","ExecutedOn":"2026-10-13T00:00:00Z","RunningCount":2}],"Count":2}`))
	if len(inst) != 2 || inst[0].ID != "i-new" || inst[0].Done() || !inst[1].Done() {
		t.Fatalf("instances = %+v", inst)
	}
	if _, ok := ParseTaskResults(json.RawMessage(`{"message":"pending"}`)); ok {
		t.Error("a payload without Result must be unreadable")
	}
	if _, ok := ParseTaskResults(json.RawMessage(`{"Result":[42],"Count":1}`)); ok {
		t.Error("an array of non-objects must be unreadable")
	}
	if _, ok := ParseTaskResults(json.RawMessage(`{"Result":[{"completionStatus":"FAILED"}]}`)); ok {
		t.Error("a result row without endpointId must be unreadable")
	}
	if empty, ok := ParseTaskResults(json.RawMessage(`{"Result":[],"Count":0}`)); !ok || len(empty) != 0 {
		t.Error("an empty Result array is a valid empty read")
	}
	res, _ := ParseTaskResults(json.RawMessage(`{"Result":[
		{"endpointId":"e1","completionStatus":"FAILED","errorDetails":"Access denied"},
		{"endpointId":"e2","completionStatus":"FAILED","errorDetails":"Access denied"},
		{"endpointId":"e3","completionStatus":"FAILED","errorDetails":"Timeout waiting for agent"},
		{"endpointId":"e4","completionStatus":"SUCCESS","resultDetails":"ok"}],"Count":4}`))
	groups := GroupFailures(res, map[string]Endpoint{"e1": {ID: "e1", Name: "ACME-WS01"}})
	if len(groups) != 2 || groups[0].Error != "Access denied" || groups[0].Count != 2 || groups[0].Endpoints[0].Name != "ACME-WS01" {
		t.Fatalf("groups = %+v", groups)
	}
	if len(GroupFailures(res[3:], nil)) != 0 {
		t.Error("successful results must not form a failure group")
	}
}
