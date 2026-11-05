// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/cliutil/testenv"
	"netsapiens-pp-cli/internal/store"
)

func fv(p *float64) float64 {
	if p == nil {
		return -999
	}
	return *p
}

func rows(t *testing.T, js string) []fleetRow {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(js))
	dec.UseNumber()
	var out []fleetRow
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return out
}

func TestSeatRows(t *testing.T) {
	domains := rows(t, `[{"domain":"acme.example","limits-max-users":10},{"domain":"beta.example"},{"domain":"gamma.example","limits-max-users":0}]`)
	billing := rows(t, `[{"domain":"acme.example","count-users-total":11,"limits-max-call-queues":4,"count-call-queues":4,"count-devices-registered":7,"count-devices-total":9},
		{"domain":"beta.example","sub_count":5,"max_user":5}]`)
	users := rows(t, `[{"domain":"gamma.example","user":"100"},{"domain":"gamma.example","user":"101"}]`)
	got := buildSeatRows(domains, billing, users, nil, nil, 0.9, seatCoverage{users: true, callqueues: true, autoattendants: true})
	if len(got) != 3 {
		t.Fatalf("rows = %d, want 3", len(got))
	}
	by := map[string]seatRow{}
	for _, r := range got {
		by[r.Domain] = r
	}
	if r := by["acme.example"]; r.Status != "over" || fv(r.Metrics["users"].Count) != 11 || fv(r.Metrics["users"].Limit) != 10 || r.Metrics["users"].Source != "billing" {
		t.Errorf("acme: %+v", r.Metrics["users"])
	}
	if r := by["acme.example"]; r.Metrics["call_queues"].Status != "near" || r.DevicesRegistered == nil || *r.DevicesRegistered != 7 {
		t.Errorf("acme queues: %+v devices %v", r.Metrics["call_queues"], r.DevicesRegistered)
	}
	// legacy snake_case billing shape still counts
	if r := by["beta.example"]; fv(r.Metrics["users"].Count) != 5 || fv(r.Metrics["users"].Limit) != 5 || r.Metrics["users"].Status != "near" {
		t.Errorf("beta legacy: %+v", r.Metrics["users"])
	}
	// no billing row: local count, and a 0 limit is unlimited, never over
	if r := by["gamma.example"]; fv(r.Metrics["users"].Count) != 2 || r.Metrics["users"].Source != "local" || r.Status != "unlimited" {
		t.Errorf("gamma: %+v status %s", r.Metrics["users"], r.Status)
	}
	if got[0].Domain != "acme.example" || fv(by["acme.example"].Metrics["users"].Pct) != 110 {
		t.Errorf("worst status must sort first with a rounded pct, got %s %v", got[0].Domain, fv(by["acme.example"].Metrics["users"].Pct))
	}
	// billing without device/number fields: unknown (null), never zero
	if r := by["beta.example"]; r.DevicesTotal != nil || r.PhoneNumbers != nil {
		t.Errorf("absent billing fields must be nil: %+v", r)
	}
	// users never synced and no billing count: unknown, never a reassuring zero
	unsynced := buildSeatRows(rows(t, `[{"domain":"gamma.example","limits-max-users":5}]`), nil, nil, nil, nil, 0.9, seatCoverage{})
	if m := unsynced[0].Metrics["users"]; m.Status != "unknown" || m.Source != "unavailable" || m.Count != nil || m.Pct != nil || unsynced[0].Status != "unknown" {
		t.Errorf("uncovered users must be unknown: %+v row %s", m, unsynced[0].Status)
	}
}

func TestOfflineIssues(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	devices := rows(t, `[
		{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"unregistered"},
		{"domain":"acme.example","device":"101a","user":"101","device-sip-registration-state":"registered","device-sip-registration-expires-datetime":"2026-10-14T13:00:00Z","device-sip-latency-seconds-average":0.05},
		{"domain":"acme.example","device":"102a","user":"102","device-sip-registration-state":"registered","device-sip-registration-expires-datetime":"2026-10-14T13:00:00Z","device-sip-latency-seconds-average":0.9},
		{"domain":"acme.example","device":"103a","user":"103","device-sip-registration-state":"registered","device-sip-registration-expires-datetime":"2026-10-14 10:00:00","_synced_at":"2026-10-14 11:00:00"},
		{"domain":"acme.example","device":"104a","user":"104","device-sip-registration-state":"unregistered","device-sip-registration-ignore-report-enabled":"yes"},
		{"domain":"acme.example","device":"105a","user":"105","device-sip-registration-state":"unregistered","device-sip-registration-password":"s3cret"}]`)
	users := rows(t, `[{"domain":"acme.example","user":"100","name-first-name":"Ada","name-last-name":"Lovelace"}]`)
	phones := rows(t, `[{"domain":"acme.example","device-provisioning-mac-address":"00:15:65:AA:BB:CC","device-provisioning-last-retrieved-datetime":""},
		{"domain":"acme.example","device-provisioning-mac-address":"001565aabbcc"},
		{"domain":"acme.example","device-provisioning-mac-address":"001565000001","device-provisioning-last-retrieved-datetime":"2026-10-13 09:00:00"},
		{"domain":"acme.example","device-provisioning-mac-address":"001565000002","device-provisioning-last-retrieved-datetime":"2026-08-01 09:00:00"},
		{"domain":"acme.example","device-provisioning-mac-address":"001565000003"}]`)
	got := offlineIssues(devices, users, phones, 0.3, 14*24*time.Hour, now)
	ids := map[string][]string{}
	for _, i := range got {
		ids[i.ID] = i.Reasons
	}
	want := map[string]string{"100a": "unregistered", "102a": "latency", "103a": "registration-expired", "105a": "unregistered", "001565aabbcc": "never-provisioned", "001565000002": "provisioning-stale"}
	for id, reason := range want {
		if !strings.Contains(strings.Join(ids[id], ","), reason) {
			t.Errorf("%s reasons = %v, want %s", id, ids[id], reason)
		}
	}
	for _, healthy := range []string{"101a", "104a", "001565000001", "001565000003"} {
		if _, bad := ids[healthy]; bad {
			t.Errorf("%s must not be flagged: %v", healthy, ids[healthy])
		}
	}
	if len(got) != len(want) {
		t.Errorf("issues = %d, want %d (MAC duplicates across /phones views must collapse)", len(got), len(want))
	}
	out, _ := json.Marshal(got)
	if strings.Contains(string(out), "s3cret") || strings.Contains(string(out), "password") {
		t.Fatal("secret field leaked into fleet offline output")
	}
	for _, i := range got {
		if i.ID == "100a" && i.UserName != "Ada Lovelace" {
			t.Errorf("user join failed: %q", i.UserName)
		}
	}
}

func TestAuditDIDs(t *testing.T) {
	domains := rows(t, `[{"domain":"acme.example"}]`)
	numbers := rows(t, `[
		{"phonenumber":"16025550100","domain":"acme.example","enabled":"yes","dial-rule-application":"to-user","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"acme.example"},
		{"phonenumber":"16025550101","domain":"acme.example","enabled":"yes","dial-rule-application":"to-user","dial-rule-translation-destination-user":"999","dial-rule-translation-destination-host":"acme.example"},
		{"phonenumber":"16025550102","domain":"acme.example","enabled":"yes","dial-rule-application":"to-callqueue","dial-rule-translation-destination-user":"700","dial-rule-translation-destination-host":"[*]"},
		{"phonenumber":"16025550103","domain":"acme.example","enabled":"no","dial-rule-application":"available-number"},
		{"phonenumber":"16025550104","domain":"acme.example","enabled":"yes","dial-rule-application":"to-user","dial-rule-translation-destination-user":"101","dial-rule-translation-destination-host":"acme.example"},
		{"phonenumber":"16025550105","domain":"acme.example","enabled":"yes","dial-rule-application":"to-connection","dial-rule-translation-destination-user":"14805550000","dial-rule-translation-destination-host":"carrier1"},
		{"phonenumber":"16025550106","domain":"acme.example","enabled":"yes","dial-rule-application":"to-user","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"other.example"}]`)
	users := rows(t, `[{"domain":"acme.example","user":"100","voicemail-enabled":"yes"},{"domain":"acme.example","user":"101","voicemail-enabled":"no"}]`)
	queues := rows(t, `[{"domain":"acme.example","callqueue":"700","description":"Sales"}]`)
	devices := rows(t, `[{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"registered"}]`)
	got := auditDIDs(numbers, domains, users, queues, nil, devices, didCoverage{users: true, callqueues: true, autoattendants: true, devices: true})
	by := map[string]didRoute{}
	for _, r := range got {
		by[r.PhoneNumber] = r
	}
	check := func(num, target, problem string) {
		t.Helper()
		r := by[num]
		if r.TargetType != target {
			t.Errorf("%s target = %s, want %s", num, r.TargetType, target)
		}
		joined := strings.Join(r.Problems, ",")
		if problem == "" && joined != "" {
			t.Errorf("%s problems = %s, want none", num, joined)
		}
		if problem != "" && !strings.Contains(joined, problem) {
			t.Errorf("%s problems = %s, want %s", num, joined, problem)
		}
	}
	check("16025550100", "user", "")
	check("16025550101", "missing", "dangling")
	check("16025550102", "callqueue", "")
	check("16025550103", "parked", "disabled")
	check("16025550104", "user", "possible-dead-end")
	check("16025550105", "external", "")
	check("16025550106", "unverified", "destination-domain-not-synced")

	// application decides the table: user 100 exists, queue 100 does not
	wrongType := rows(t, `[{"phonenumber":"16025550199","domain":"acme.example","enabled":"yes","dial-rule-application":"to-callqueue","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"acme.example"}]`)
	if r := auditDIDs(wrongType, domains, users, queues, nil, devices, didCoverage{users: true, callqueues: true, autoattendants: true, devices: true}); r[0].TargetType != "missing" {
		t.Errorf("to-callqueue at a user extension must be missing, got %+v", r[0])
	}
	// a completed domains sync without the destination domain: dangling, not unverified
	full := didCoverage{domains: true, users: true, callqueues: true, autoattendants: true, devices: true}
	for _, r := range auditDIDs(numbers, domains, users, queues, nil, devices, full) {
		if r.PhoneNumber == "16025550106" && (r.TargetType != "missing" || !strings.Contains(strings.Join(r.Problems, ","), "domain-other.example-not-found")) {
			t.Errorf("unknown destination domain under full coverage = %+v", r)
		}
	}
	// an omitted destination field is unverified, not "no destination"
	if r := auditDIDs(rows(t, `[{"phonenumber":"16025550197","domain":"acme.example","dial-rule-application":"to-user"}]`), domains, users, queues, nil, devices, didCoverage{users: true, callqueues: true, autoattendants: true, devices: true}); r[0].TargetType != "unverified" {
		t.Errorf("absent destination must be unverified: %+v", r[0])
	}
	// a device whose record omits its registration state blocks the dead-end claim
	unknownState := append(rows(t, `[{"domain":"acme.example","device":"101a","user":"101"}]`), devices...)
	for _, r := range auditDIDs(numbers, domains, users, queues, nil, unknownState, didCoverage{users: true, callqueues: true, autoattendants: true, devices: true}) {
		if r.PhoneNumber == "16025550104" && len(r.Problems) != 0 {
			t.Errorf("unknown registration must not yield a dead end: %+v", r)
		}
	}
	// an unsynced queue table makes the target unverifiable, not missing
	noQueues := didCoverage{users: true, autoattendants: true, devices: true}
	if r := auditDIDs(rows(t, `[{"phonenumber":"16025550198","domain":"acme.example","enabled":"yes","dial-rule-application":"to-callqueue","dial-rule-translation-destination-user":"701","dial-rule-translation-destination-host":"acme.example"}]`), domains, users, nil, nil, devices, noQueues); r[0].TargetType != "unverified" {
		t.Errorf("unsynced callqueues must be unverified, got %+v", r[0])
	}
	if len(got[0].Problems) == 0 {
		t.Error("numbers with problems must sort first")
	}
}

func TestE911Gaps(t *testing.T) {
	domains := rows(t, `[{"domain":"acme.example","caller-id-number-emergency":"6025550100","emergency-address-id":"A1"},{"domain":"bare.example","caller-id-number-emergency":""}]`)
	users := rows(t, `[{"domain":"acme.example","user":"100"},{"domain":"bare.example","user":"200","caller-id-number-emergency":""},{"domain":"bare.example","user":"201","caller-id-number-emergency":"4805550000","emergency-address-id":"GONE"}]`)
	devices := rows(t, `[{"domain":"bare.example","device":"201a","user":"201"},{"domain":"bare.example","device":"200a","user":"200","caller-id-number-emergency":"[*]"}]`)
	addresses := rows(t, `[{"domain":"acme.example","emergency-address-id":"A1"},{"domain":"bare.example","emergency-address-id":"B1"}]`)
	rep := e911Gaps(domains, users, devices, addresses, e911Coverage{domains: true, users: true, addresses: true})
	found := map[string]string{}
	for _, g := range rep.Gaps {
		found[g.Entity+":"+g.ID] = strings.Join(g.Gaps, ",")
	}
	if _, ok := found["user:100"]; ok {
		t.Error("user 100 inherits the domain emergency caller ID; must not be flagged")
	}
	if _, ok := found["domain:acme.example"]; ok {
		t.Error("acme has caller ID and a known address; must not be flagged")
	}
	for k, want := range map[string]string{"domain:bare.example": "no-domain-emergency-caller-id", "user:200": "no-emergency-caller-id", "user:201": "emergency-address-id-not-found", "device:200a": "no-emergency-caller-id"} {
		if !strings.Contains(found[k], want) {
			t.Errorf("%s = %q, want %s", k, found[k], want)
		}
	}
	if _, ok := found["device:201a"]; ok {
		t.Error("device 201a inherits user 201's caller ID; must not be flagged")
	}
	// "[*]" on a device means inherit; with nothing to inherit it is a gap
	inherit := e911Gaps(rows(t, `[{"domain":"bare.example","caller-id-number-emergency":""}]`), nil, rows(t, `[{"domain":"bare.example","device":"300a","user":"300","caller-id-number-emergency":"[*]"}]`), nil, e911Coverage{domains: true, users: true, addresses: true})
	if len(inherit.Gaps) != 2 || inherit.Gaps[0].Entity != "domain" || !strings.Contains(strings.Join(inherit.Gaps[1].Gaps, ","), "no-emergency-caller-id") {
		t.Errorf("[*] must not count as a caller ID: %+v", inherit.Gaps)
	}
	// a synced tenant with zero addresses still flags references to a missing address
	zero := e911Gaps(rows(t, `[{"domain":"solo.example","caller-id-number-emergency":"6025550100"}]`), rows(t, `[{"domain":"solo.example","user":"1","emergency-address-id":"DELETED"}]`), nil, nil, e911Coverage{domains: true, users: true, addresses: true})
	// users not synced: a device may inherit a user caller ID we never saw -> unverified, not a gap
	unver := e911Gaps(rows(t, `[{"domain":"bare.example"}]`), nil, rows(t, `[{"domain":"bare.example","device":"300a","user":"300"}]`), nil, e911Coverage{domains: true, addresses: true})
	if g := strings.Join(unver.Gaps[len(unver.Gaps)-1].Gaps, ","); !strings.Contains(g, "unverified") {
		t.Errorf("unsynced users must make device inheritance unverified, got %s", g)
	}
	if len(zero.Gaps) != 1 || !strings.Contains(strings.Join(zero.Gaps[0].Gaps, ","), "emergency-address-id-not-found") {
		t.Errorf("synced empty address set must still flag dangling ids: %+v", zero.Gaps)
	}
	// a user whose domain row is absent and domains did not complete: unverified
	orphan := e911Gaps(nil, rows(t, `[{"domain":"ghost.example","user":"1"}]`), nil, nil, e911Coverage{users: true, addresses: true})
	if g := strings.Join(orphan.Gaps[0].Gaps, ","); !strings.Contains(g, "unverified") {
		t.Errorf("missing domain level must be unverified, got %s", g)
	}
	// omitted caller-ID fields are unknown, never a confirmed gap
	omitted := e911Gaps(rows(t, `[{"domain":"acme.example"}]`), rows(t, `[{"domain":"acme.example","user":"100"}]`), nil, nil, e911Coverage{domains: true, users: true, addresses: true})
	for _, g := range omitted.Gaps {
		if j := strings.Join(g.Gaps, ","); strings.Contains(j, "no-") || !strings.Contains(j, "unverified") {
			t.Errorf("omitted fields must be unverified, got %s", j)
		}
	}
	// a known domain caller ID guarantees the device has one, even with users unsynced
	guaranteed := e911Gaps(rows(t, `[{"domain":"acme.example","caller-id-number-emergency":"6025550100"}]`), nil, rows(t, `[{"domain":"acme.example","device":"100a","user":"100","caller-id-number-emergency":"[*]"}]`), nil, e911Coverage{domains: true, addresses: true})
	if len(guaranteed.Gaps) != 0 {
		t.Errorf("domain caller ID must cover the device: %+v", guaranteed.Gaps)
	}
	// without synced addresses, ids are not judged and the note says so
	if rep := e911Gaps(domains, users, nil, nil, e911Coverage{domains: true, users: true}); rep.Note == "" {
		t.Error("missing addresses must produce a note")
	} else {
		for _, g := range rep.Gaps {
			if strings.Contains(strings.Join(g.Gaps, ","), "address") {
				t.Errorf("address gap reported without synced addresses: %+v", g)
			}
		}
	}
}

func TestTraceIdentifier(t *testing.T) {
	td := traceData{
		numbers: rows(t, `[{"phonenumber":"16025550142","domain":"acme.example","enabled":"yes","dial-rule-application":"to-user","dial-rule-translation-destination-user":"204","dial-rule-translation-destination-host":"acme.example"}]`),
		users: rows(t, `[{"domain":"acme.example","user":"204","name-first-name":"Grace","name-last-name":"Hopper","email":"grace@acme.example","voicemail-login-pin":"1234"},
			{"domain":"beta.example","user":"204","name-first-name":"Other","email":"o@beta.example"}]`),
		devices: rows(t, `[{"domain":"acme.example","device":"204a","user":"204","device-sip-registration-state":"registered","device-provisioning-mac-address":"00:15:65:AA:BB:CC","device-provisioning-password":"pw"}]`),
		phones:  rows(t, `[{"domain":"acme.example","device-provisioning-mac-address":"001565aabbcc","device-models-brand-and-model":"Yealink T54W"}]`),
	}
	td.synced = map[string]bool{rtPhonenumbers: true, rtSmsnumbers: true, rtUsers: true, rtDevices: true, rtCallqueues: true, rtAutoattendants: true, "phones": true}
	res := traceIdentifier("+1 (602) 555-0142", "", td)
	if res.Kind != "number" || len(res.Matches) != 1 {
		t.Fatalf("number trace = %+v", res)
	}
	m := res.Matches[0]
	if m.User == nil || m.User.Name != "Grace Hopper" || len(m.Devices) != 1 || m.Devices[0].Model != "Yealink T54W" || len(m.Numbers) != 1 {
		t.Fatalf("number chain incomplete: %+v", m)
	}
	if r := traceIdentifier("204", "", td); len(r.Matches) != 2 || r.Note == "" {
		t.Errorf("ambiguous extension must return both domains with a note: %+v", r)
	}
	if r := traceIdentifier("204", "beta.example", td); len(r.Matches) != 1 || r.Matches[0].Domain != "beta.example" {
		t.Errorf("--domain must scope extension: %+v", r)
	}
	if r := traceIdentifier("00-15-65-aa-bb-cc", "", td); r.Kind != "mac" || len(r.Matches) != 1 || r.Matches[0].User == nil {
		t.Errorf("mac trace: %+v", r)
	}
	if r := traceIdentifier("GRACE@acme.example", "", td); len(r.Matches) != 1 {
		t.Errorf("email trace must be case-insensitive: %+v", r)
	}
	// a to-callqueue DID resolves to the queue, never to a same-numbered user and its phones
	td.numbers = append(td.numbers, rows(t, `[{"phonenumber":"16025550150","domain":"acme.example","enabled":"yes","dial-rule-application":"to-callqueue","dial-rule-translation-destination-user":"204","dial-rule-translation-destination-host":"acme.example"}]`)...)
	td.queues = rows(t, `[{"domain":"acme.example","callqueue":"204","description":"Support"}]`)
	if r := traceIdentifier("6025550150", "", td); len(r.Matches) != 1 || r.Matches[0].TargetType != "callqueue" || r.Matches[0].User != nil || len(r.Matches[0].Devices) != 0 || r.Matches[0].TargetName != "Support" {
		t.Errorf("queue route trace = %+v", r.Matches)
	}
	if k := traceKind("442071234567"); k != "number" {
		t.Errorf("an all-digit 12-digit query is a DID, got %s", k)
	}
	td.synced[rtSmsnumbers] = false
	if r := traceIdentifier("4805551234", "", td); r.Complete || !strings.Contains(r.Note, "unverified") {
		t.Errorf("an unsynced smsnumbers table must keep a negative number lookup unverified: %+v", r)
	}
	td.synced[rtSmsnumbers] = true
	// devices not synced: the user's phone list is unknown (null), and the trace incomplete
	td.synced[rtDevices] = false
	if r := traceIdentifier("204", "acme.example", td); r.Complete || r.Matches[0].Devices != nil {
		t.Errorf("uncovered devices must be null and incomplete: %+v", r)
	}
	td.synced[rtDevices] = true
	// a DID without an application is unverified routing: nothing attached
	td.numbers = append(td.numbers, rows(t, `[{"phonenumber":"16025550160","domain":"acme.example","dial-rule-translation-destination-user":"204","dial-rule-translation-destination-host":"acme.example"}]`)...)
	if r := traceIdentifier("6025550160", "", td); r.Matches[0].TargetType != "unverified" || r.Matches[0].User != nil || r.Complete {
		t.Errorf("absent application must be unverified: %+v", r.Matches)
	}
	if r := traceIdentifier("4805551234", "", td); len(r.Matches) != 0 || !r.Complete || r.Note == "" {
		t.Errorf("unknown number must return no matches plus a note: %+v", r)
	}
	out, _ := json.Marshal(res)
	if strings.Contains(string(out), "1234\"") || strings.Contains(string(out), "\"pw\"") {
		t.Fatal("secret leaked into trace output")
	}
}

func TestGroupMissed(t *testing.T) {
	cdrs := rows(t, `[
		{"id":"1","call-orig-call-id":"c1","call-term-domain":"acme.example","call-term-user":"100","call-start-datetime":"2026-10-13 09:00:00"},
		{"id":"2","call-orig-call-id":"c1","call-term-domain":"acme.example","call-term-user":"100","call-start-datetime":"2026-10-13 09:00:01"},
		{"id":"3","call-orig-call-id":"c2","call-term-domain":"acme.example","call-term-user":"100","call-start-datetime":"2026-10-14 08:00:00"},
		{"id":"4","call-orig-call-id":"c3","call-term-domain":"acme.example","call-term-user":"101","call-answer-datetime":"2026-10-14 08:00:05"},
		{"id":"5","call-orig-call-id":"c4","domain":"beta.example","call-orig-to-user":"300"}]`)
	users := indexBy(rows(t, `[{"domain":"acme.example","user":"100","name-first-name":"Ada"}]`), "user")
	got := groupMissed(cdrs, users, regMap{key("acme.example", "100"): {}}, true, true, "")
	if len(got) != 2 {
		t.Fatalf("groups = %+v, want 2 (answered leg excluded)", got)
	}
	if got[0].User != "100" || got[0].Missed != 2 || got[0].UserName != "Ada" || got[0].RegisteredDevices == nil || *got[0].RegisteredDevices != 0 || got[0].LastMissed != "2026-10-14 08:00:00" {
		t.Errorf("acme 100 = %+v (legs of one call must count once)", got[0])
	}
	if got[1].Domain != "beta.example" || got[1].User != "300" {
		t.Errorf("fallback fields = %+v", got[1])
	}
	if g := groupMissed(cdrs, nil, nil, true, false, ""); g[0].RegisteredDevices != nil {
		t.Error("registered device count must be omitted when devices are not synced")
	}
	// one ring-group call (same call id) missed by two users counts for both
	ring := rows(t, `[{"call-orig-call-id":"r1","call-term-domain":"acme.example","call-term-user":"100"},{"call-orig-call-id":"r1","call-term-domain":"acme.example","call-term-user":"101"}]`)
	if g := groupMissed(ring, nil, nil, false, false, ""); len(g) != 2 || g[0].Missed != 1 || g[1].Missed != 1 {
		t.Errorf("ring-group legs must count once per target: %+v", g)
	}
}

func TestBuildQueueRows(t *testing.T) {
	stats := map[string]fleetRow{"700": {"name": "Sales", "calls-total-count": json.Number("100"), "calls-abandoned-count": json.Number("12"), "calls-abandoned-rate": json.Number("12")}}
	queues := rows(t, `[{"domain":"acme.example","callqueue":"700","callqueue-count-agents-available":0,"callqueue-count-agents-total":3},{"domain":"acme.example","callqueue":"701","callqueue-count-agents-available":2,"callqueue-count-agents-total":2}]`)
	agents := rows(t, `[{"callqueue":"701","callqueue-agent-has-registered-devices":"no"},{"callqueue":"701","callqueue-agent-has-registered-devices":"yes"}]`)
	got := buildQueueRows("acme.example", stats, queues, agents)
	if len(got) != 2 {
		t.Fatalf("rows = %+v", got)
	}
	if got[0].Callqueue != "700" || got[0].AbandonedRate == nil || *got[0].AbandonedRate != 12 || got[0].Staffing != "none-available" {
		t.Errorf("700 = %+v", got[0])
	}
	if got[1].Callqueue != "701" || got[1].AgentsNoPhone == nil || *got[1].AgentsNoPhone != 1 || got[1].Staffing != "agents-unreachable" || got[1].CallsTotal != nil || got[1].HasStats {
		t.Errorf("701 = %+v", got[1])
	}
}

func TestQueueUnknownReachability(t *testing.T) {
	queues := rows(t, `[{"domain":"acme.example","callqueue":"700","callqueue-count-agents-available":1,"callqueue-count-agents-total":1}]`)
	got := buildQueueRows("acme.example", nil, queues, rows(t, `[{"callqueue":"700"}]`))
	if got[0].AgentsNoPhone != nil || got[0].Staffing != "unknown" {
		t.Errorf("agent rows without the registration field must be unknown: %+v", got[0])
	}
}

func TestDiffSnapshots(t *testing.T) {
	base := []snapEntry{
		{Entity: "user", Key: "acme.example/100", Domain: "acme.example", Fields: map[string]string{"email": "a@x"}},
		{Entity: "phonenumber", Key: "16025550100", Domain: "acme.example", Fields: map[string]string{"destination_user": "100"}},
		{Entity: "device", Key: "acme.example/100a", Domain: "acme.example", Fields: map[string]string{"mac": "001565aabbcc"}},
	}
	cur := []snapEntry{
		{Entity: "user", Key: "acme.example/100", Domain: "acme.example", Fields: map[string]string{"email": "a@x"}},
		{Entity: "phonenumber", Key: "16025550100", Domain: "acme.example", Fields: map[string]string{"destination_user": "700"}},
		{Entity: "user", Key: "acme.example/101", Domain: "acme.example", Fields: map[string]string{"email": "b@x"}},
	}
	got := diffSnapshots(base, cur)
	kinds := map[string]string{}
	for _, c := range got {
		kinds[c.Entity+":"+c.Key] = c.Change
	}
	if len(got) != 3 || kinds["phonenumber:16025550100"] != "changed" || kinds["user:acme.example/101"] != "added" || kinds["device:acme.example/100a"] != "removed" {
		t.Fatalf("diff = %+v", got)
	}
	// a field the current record omits is not a clearing, and a merge keeps its old value
	omits := []snapEntry{{Entity: "user", Key: "acme.example/100", Domain: "acme.example", Fields: map[string]string{}}}
	if d := diffSnapshots(base[:1], omits); len(d) != 0 {
		t.Fatalf("an omitted field must not be reported as changed: %+v", d)
	}
	if m := mergeSnapshot(base[:1], omits); m[0].Fields["email"] != "a@x" {
		t.Fatalf("merge must keep the baseline value of an omitted field: %+v", m)
	}
	if d := diffSnapshots(cur, cur); len(d) != 0 {
		t.Fatalf("identical snapshots must not fabricate changes: %+v", d)
	}
}

// TestSyncPagesOnStart proves the start-offset hand-fix: NetSapiens caps a
// page at limit (default 100, max 1000) and advances with a 0-based start.
// Without the fix sync stops at one page or re-reads start=0.
func TestSyncPagesOnStart(t *testing.T) {
	testenv.Isolate(t)
	const total = 1150
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/domains") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
			return
		}
		calls++
		start, _ := strconv.Atoi(r.URL.Query().Get("start"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 100
		}
		var page []map[string]string
		for i := start; i < total && i < start+limit; i++ {
			page = append(page, map[string]string{"domain": fmt.Sprintf("d%04d.example", i)})
		}
		if page == nil {
			page = []map[string]string{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()
	t.Setenv("NETSAPIENS_BASE_URL", srv.URL)
	t.Setenv("NETSAPIENS_API_KEY", "test-key")
	dbPath := filepath.Join(t.TempDir(), "data.db")

	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--resources", "domains", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type='domains'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != total {
		t.Fatalf("synced %d domains, want %d (calls=%d)\n%s", n, total, calls, out.String())
	}

	cmd = RootCmd()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"domains", "list", "--all", "--json", "--data-source", "live"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("domains list --all: %v", err)
	}
	if got := strings.Count(out.String(), `"domain"`); got != total {
		t.Fatalf("domains list --all returned %d domains, want %d", got, total)
	}
}

// TestSyncStoresSingleObjectBilling proves per-domain billing (one JSON object
// per parent, not a list) lands as one row per domain.
func TestSyncStoresSingleObjectBilling(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			_, _ = w.Write([]byte(`[{"domain":"acme.example"},{"domain":"beta.example"}]`))
		case strings.HasSuffix(r.URL.Path, "/billing"):
			d := strings.Split(strings.TrimSuffix(r.URL.Path, "/billing"), "/")
			_, _ = fmt.Fprintf(w, `{"domain":%q,"count-users-total":3,"limits-max-users":2}`, d[len(d)-1])
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("NETSAPIENS_BASE_URL", srv.URL)
	t.Setenv("NETSAPIENS_API_KEY", "test-key")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--resources", "domains,billing", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type='billing'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("billing rows = %d, want 2\n%s", n, out.String())
	}
}

// TestQueueDiscoveryAndTotalFailure: /domains discovery pages past 1000 on
// start, --max-domains is reported as a cap, and all-domains-failed exits non-zero.
func TestQueueDiscoveryAndTotalFailure(t *testing.T) {
	testenv.Isolate(t)
	const total = 1500
	failQueues := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/domains") {
			start, _ := strconv.Atoi(r.URL.Query().Get("start"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			var page []map[string]string
			for i := start; i < total && i < start+limit; i++ {
				page = append(page, map[string]string{"domain": fmt.Sprintf("d%04d.example", i)})
			}
			if page == nil {
				page = []map[string]string{}
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		if failQueues {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"500","message":"boom"}`))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/per-queue"):
			_, _ = w.Write([]byte(`{"700":{"name":"Sales","calls-abandoned-rate":5}}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("NETSAPIENS_BASE_URL", srv.URL)
	t.Setenv("NETSAPIENS_API_KEY", "test-key")
	t.Setenv("NETSAPIENS_RETRY_MAX", "0")

	c, err := (&rootFlags{}).newClient()
	if err != nil {
		t.Fatal(err)
	}
	names, _, err := queueTargetDomains(t.Context(), c, filepath.Join(t.TempDir(), "none.db"))
	if err != nil || len(names) != total {
		t.Fatalf("discovered %d domains (err %v), want %d", len(names), err, total)
	}

	failQueues = false
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"fleet", "queues", "--max-domains", "3", "--db", filepath.Join(t.TempDir(), "none.db"), "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet queues: %v\n%s", err, errb.String())
	}
	var rep queueReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if !rep.DomainsCapped || rep.DomainsAvailable != total || rep.DomainsChecked != 3 || len(rep.Queues) != 3 {
		t.Fatalf("cap not reported: capped=%v available=%d checked=%d queues=%d", rep.DomainsCapped, rep.DomainsAvailable, rep.DomainsChecked, len(rep.Queues))
	}

	failQueues = true
	cmd = RootCmd()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"fleet", "queues", "--max-domains", "2", "--db", filepath.Join(t.TempDir(), "none.db"), "--json"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("every domain failed, want a non-zero exit; stdout=%s", out.String())
	}
}

// TestSyncExplicitResourcesAreExact: listing resources must not cascade into
// unlisted dependents (per-user addresses / message sessions).
func TestSyncExplicitResourcesAreExact(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			_, _ = w.Write([]byte(`[{"domain":"acme.example"}]`))
		case strings.HasSuffix(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`[{"domain":"acme.example","user":"100"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("NETSAPIENS_BASE_URL", srv.URL)
	t.Setenv("NETSAPIENS_API_KEY", "test-key")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--resources", "domains,users", "--db", filepath.Join(t.TempDir(), "data.db")})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	if hits["/domains/acme.example/users"] == 0 {
		t.Fatalf("users not synced: %v", hits)
	}
	for p := range hits {
		if strings.Contains(p, "/users/100/") || strings.Contains(p, "/devices") || strings.Contains(p, "/presence") || strings.Contains(p, "/messagesessions") {
			t.Errorf("unlisted dependent fetched: %s (%v)", p, hits)
		}
	}
}

// seedMirror writes resources and their sync_state the way sync does.
func seedMirror(t *testing.T, path string, complete map[string]bool, data map[string][]string) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for rt, docs := range data {
		for i, d := range docs {
			if _, err := db.DB().Exec(`INSERT OR REPLACE INTO resources (id, resource_type, data) VALUES (?, ?, ?)`, fmt.Sprintf("%s-%d", rt, i), rt, d); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A complete dependent (users, devices, ... walked per domain) implies its
	// parent walk completed too, unless the test states the parent.
	states := map[string]bool{}
	for rt, ok := range complete {
		states[rt] = ok
		for p := fleetDependentParent[rt]; ok && p != ""; p = fleetDependentParent[p] {
			if _, stated := complete[p]; !stated {
				states[p] = true
			}
		}
	}
	if _, err := db.DB().Exec(fleetProvenanceDDLForTest); err != nil {
		t.Fatal(err)
	}
	walk := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339) // one unfiltered sync run a minute ago
	for rt, ok := range states {
		c := 0
		if ok {
			c = 1
		}
		if _, err := db.DB().Exec(`INSERT OR REPLACE INTO sync_state (resource_type, last_synced_at, total_count, last_attempt_complete) VALUES (?, CURRENT_TIMESTAMP, 0, ?)`, rt, c); err != nil {
			t.Fatal(err)
		}
		if ok {
			if _, err := db.DB().Exec(`INSERT OR REPLACE INTO fleet_sync_provenance (resource_type, run_started, state_synced_at, authoritative) SELECT resource_type, ?, CAST(last_synced_at AS TEXT), 1 FROM sync_state WHERE resource_type = ?`, walk, rt); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestFleetSyncedRequiresCompletedSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"users": true, "devices": false}, nil)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if !fleetSynced(db, rtUsers) || fleetSynced(db, rtDevices) || fleetSynced(db, rtCallqueues) {
		t.Fatal("coverage must require a completed latest sync")
	}
}

// TestFleetChangesReadOnlyAndEmptyBaseline: no writes without --rebaseline
// (including --dry-run), an empty recorded baseline still reports additions,
// and an incompletely synced type is skipped instead of reported as removed.
func TestFleetChangesReadOnlyAndEmptyBaseline(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"domains": true, "users": true}, map[string][]string{"domains": {`{"domain":"acme.example"}`}})
	run := func(args ...string) changesReport {
		t.Helper()
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"fleet", "changes", "--db", path, "--json"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("fleet changes %v: %v", args, err)
		}
		var rep changesReport
		if strings.Contains(strings.Join(args, " "), "--dry-run") {
			return rep
		}
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("decode %v: %v\n%s", args, err, out.String())
		}
		return rep
	}
	tables := func() int {
		db, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var n int
		_ = db.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'fleet_baseline%'`).Scan(&n)
		return n
	}
	if r := run(); !r.FirstRun || r.Rebaselined {
		t.Fatalf("first run must only report no baseline: %+v", r)
	}
	run("--rebaseline", "--dry-run")
	if tables() != 0 {
		t.Fatal("read-only and --dry-run runs must not create baseline tables")
	}
	// record a baseline while users is still empty, then a user appears
	if r := run("--rebaseline"); !r.Rebaselined {
		t.Fatalf("rebaseline: %+v", r)
	}
	seedMirror(t, path, map[string]bool{"users": true, "devices": false}, map[string][]string{"users": {`{"domain":"acme.example","user":"100"}`}})
	r := run()
	if r.FirstRun || r.Added != 1 || r.Changes[0].Key != "acme.example/100" {
		t.Fatalf("addition after an empty baseline must be reported: %+v", r)
	}
	if !strings.Contains(strings.Join(r.SkippedEntities, ","), "device") {
		t.Fatalf("incompletely synced devices must be skipped: %+v", r.SkippedEntities)
	}
	// a device exists in the baseline only if devices were complete; record one, then
	// break the devices sync: --rebaseline must keep the old device entry
	seedMirror(t, path, map[string]bool{"devices": true}, map[string][]string{"devices": {`{"domain":"acme.example","device":"100a","user":"100"}`}})
	run("--rebaseline")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.DB().Exec(`DELETE FROM resources WHERE resource_type='devices'`)
	_, _ = db.DB().Exec(`UPDATE sync_state SET last_attempt_complete=0 WHERE resource_type='devices'`)
	db.Close()
	run("--rebaseline")
	seedMirror(t, path, map[string]bool{"devices": true}, nil) // next full devices sync: device really gone
	if r := run(); r.Removed != 1 || r.Changes[0].Entity != "device" {
		t.Fatalf("a removal hidden by a partial sync must surface after the next full sync: %+v", r)
	}
}

func TestOfflineProvisioningObservations(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	phones := rows(t, `[
		{"domain":"a.example","device-provisioning-mac-address":"0015650000a1","device-provisioning-last-retrieved-datetime":null},
		{"domain":"a.example","device-provisioning-mac-address":"0015650000a2","device-provisioning-last-retrieved-datetime":"not-a-date"},
		{"domain":"a.example","device-provisioning-mac-address":"0015650000a3"},
		{"domain":"a.example","device-provisioning-mac-address":"0015650000a3","device-provisioning-last-retrieved-datetime":""},
		{"domain":"a.example","device-provisioning-mac-address":"0015650000a4","device-provisioning-last-retrieved-datetime":""},
		{"domain":"a.example","device-provisioning-mac-address":"0015650000a4","device-provisioning-last-retrieved-datetime":"2026-10-13 09:00:00"}]`)
	got := map[string]string{}
	for _, i := range offlineIssues(nil, nil, phones, 0.3, 14*24*time.Hour, now) {
		got[i.ID] = strings.Join(i.Reasons, ",")
	}
	if _, ok := got["0015650000a1"]; ok {
		t.Error("a NULL timestamp is unknown, not never-provisioned")
	}
	if _, ok := got["0015650000a2"]; ok {
		t.Error("an unparseable timestamp is unknown, not never-provisioned")
	}
	if got["0015650000a3"] != "never-provisioned" {
		t.Errorf("an absent duplicate must not hide an explicit empty timestamp: %v", got)
	}
	if _, ok := got["0015650000a4"]; ok {
		t.Error("a valid recent timestamp in any duplicate wins over an empty one")
	}
}

func TestFleetChangesFirstBaselineSkipsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"domains": true, "users": false}, map[string][]string{
		"domains": {`{"domain":"acme.example"}`}, "users": {`{"domain":"acme.example","user":"100","email":"a@x"}`}})
	run := func(args ...string) changesReport {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"fleet", "changes", "--db", path, "--json"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var rep changesReport
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatal(err)
		}
		return rep
	}
	run("--rebaseline")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.DB().Exec(`DELETE FROM resources WHERE resource_type='users'`)
	db.Close()
	seedMirror(t, path, map[string]bool{"users": true}, nil) // now a complete, empty users sync
	if r := run(); r.Removed != 0 {
		t.Fatalf("a user seen only in an incomplete sync must not be in the baseline: %+v", r)
	}
}

func TestTraceRound6(t *testing.T) {
	all := map[string]bool{rtPhonenumbers: true, rtSmsnumbers: true, rtUsers: true, rtDevices: true, rtCallqueues: true, rtAutoattendants: true, "phones": true}
	// domain names are case-sensitive: two tenants that differ only by case stay separate
	td := traceData{synced: all, users: rows(t, `[{"domain":"Acme.example","user":"100","name-first-name":"Alice"},{"domain":"acme.example","user":"100","name-first-name":"Bob"}]`)}
	if r := traceIdentifier("100", "", td); len(r.Matches) != 2 || r.Matches[0].User.Name == r.Matches[1].User.Name {
		t.Errorf("case-distinct tenants must not merge: %+v", r.Matches)
	}
	// an SMS number routed to a call queue resolves to the queue
	td = traceData{synced: all,
		sms:    rows(t, `[{"number":"16025550142","domain":"acme.example","application":"callqueue","dest":"700"}]`),
		users:  rows(t, `[{"domain":"acme.example","user":"700","name-first-name":"Not The Queue"}]`),
		queues: rows(t, `[{"domain":"acme.example","callqueue":"700","description":"Sales"}]`)}
	if r := traceIdentifier("6025550142", "", td); len(r.Matches) != 1 || r.Matches[0].TargetType != "callqueue" || r.Matches[0].User != nil {
		t.Errorf("SMS queue route = %+v", r.Matches)
	}
	// a provisioned MAC is only "unassigned" when devices completed a sync
	td = traceData{synced: map[string]bool{"phones": true}, phones: rows(t, `[{"domain":"acme.example","device-provisioning-mac-address":"001565aabbcc"}]`)}
	if r := traceIdentifier("00:15:65:aa:bb:cc", "", td); r.Matches[0].MatchedOn == "mac-unassigned" || r.Complete {
		t.Errorf("unassigned claim needs completed devices: %+v", r.Matches)
	}
}

func TestRound7(t *testing.T) {
	all := map[string]bool{rtPhonenumbers: true, rtSmsnumbers: true, rtUsers: true, rtDevices: true, rtCallqueues: true, rtAutoattendants: true, "phones": true}
	td := traceData{synced: all, sms: rows(t, `[{"number":"16025550142","domain":"acme.example","dest":"700"}]`), users: rows(t, `[{"domain":"acme.example","user":"700"}]`)}
	if r := traceIdentifier("6025550142", "", td); r.Matches[0].TargetType != "unverified" || r.Matches[0].User != nil || r.Complete {
		t.Errorf("SMS without an application must be unverified: %+v", r)
	}
	got := buildSeatRows(rows(t, `[{"domain":"acme.example","limits-max-users":10}]`), rows(t, `[{"domain":"acme.example","count-users-total":11,"limits-max-users-toal":0}]`), nil, nil, nil, 0.9, seatCoverage{billing: true, users: true, callqueues: true, autoattendants: true})
	if m := got[0].Metrics["users"]; m.Status != "unlimited" || fv(m.Limit) != 0 {
		t.Errorf("an explicit billing limit of 0 is unlimited: %+v", m)
	}
}

func TestRound8(t *testing.T) {
	all := map[string]bool{rtPhonenumbers: true, rtSmsnumbers: true, rtUsers: true, rtDevices: true, rtCallqueues: true, rtAutoattendants: true, "phones": true}
	td := traceData{synced: all, users: rows(t, `[{"domain":"acme.example","user":"204","email":"alice@acme.example; backup@acme.example"},{"domain":"acme.example","user":"1234567"}]`)}
	if r := traceIdentifier("BACKUP@acme.example", "", td); len(r.Matches) != 1 {
		t.Errorf("an address inside an email list must match: %+v", r)
	}
	if r := traceIdentifier("1234567", "acme.example", td); len(r.Matches) != 1 || r.Matches[0].MatchedOn != "extension" {
		t.Errorf("a long numeric extension must match: %+v", r)
	}

	// a first baseline recorded while users were incomplete must not turn existing users into additions
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"domains": true, "users": false}, map[string][]string{
		"domains": {`{"domain":"acme.example"}`}, "users": {`{"domain":"acme.example","user":"100"}`}})
	run := func(args ...string) changesReport {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"fleet", "changes", "--db", path, "--json"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var rep changesReport
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatal(err)
		}
		return rep
	}
	run("--rebaseline")
	seedMirror(t, path, map[string]bool{"users": true}, nil)
	if r := run(); r.Added != 0 || !strings.Contains(strings.Join(r.SkippedEntities, ","), "user") {
		t.Fatalf("users unknown at baseline time must be skipped, not added: %+v", r)
	}
	run("--rebaseline") // now users are covered in the baseline too
	if r := run(); r.Added != 0 || strings.Contains(strings.Join(r.SkippedEntities, ","), "user") {
		t.Fatalf("after a covered rebaseline users compare normally: %+v", r)
	}
}

func TestE911DeviceWithoutOwner(t *testing.T) {
	rep := e911Gaps(rows(t, `[{"domain":"acme.example","caller-id-number-emergency":""}]`),
		rows(t, `[{"domain":"acme.example","user":"100","caller-id-number-emergency":"16025550100"}]`),
		rows(t, `[{"domain":"acme.example","device":"100a","caller-id-number-emergency":"[*]"}]`), nil,
		e911Coverage{domains: true, users: true, devices: true, addresses: true})
	for _, g := range rep.Gaps {
		if g.Entity == "device" && !strings.Contains(strings.Join(g.Gaps, ","), "unverified") {
			t.Errorf("a device without an owner field must be unverified, got %v", g.Gaps)
		}
	}
}

func TestUnownedDevices(t *testing.T) {
	all := didCoverage{domains: true, users: true, callqueues: true, autoattendants: true, devices: true}
	users := rows(t, `[{"domain":"acme.example","user":"100","voicemail-enabled":"no"}]`)
	unowned := rows(t, `[{"domain":"acme.example","device":"100a","device-sip-registration-state":"registered"}]`)
	num := rows(t, `[{"phonenumber":"16025550100","domain":"acme.example","dial-rule-application":"to-user","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"acme.example"}]`)
	if r := auditDIDs(num, rows(t, `[{"domain":"acme.example"}]`), users, nil, nil, unowned, all); len(r[0].Problems) != 0 {
		t.Errorf("an unowned registered device makes the dead-end claim unverifiable: %+v", r[0])
	}
	cdr := rows(t, `[{"call-orig-call-id":"c1","call-term-domain":"acme.example","call-term-user":"100"}]`)
	if g := groupMissed(cdr, indexBy(users, "user"), registeredDevices(unowned), true, true, ""); g[0].RegisteredDevices != nil {
		t.Errorf("registered phones must be unknown when the domain has unowned devices: %+v", g[0])
	}
	td := traceData{synced: map[string]bool{rtUsers: true, rtDevices: true, "phones": true},
		devices: rows(t, `[{"domain":"acme.example","device":"a1","device-provisioning-mac-address":"001565aabbcc"},{"domain":"acme.example","device":"a2","device-provisioning-mac-address":"001565ddeeff"}]`)}
	r := traceIdentifier("00:15:65:aa:bb:cc", "", td)
	if len(r.Matches) != 1 || len(r.Matches[0].Devices) != 1 || r.Matches[0].Devices[0].Device != "a1" || r.Complete {
		t.Errorf("an unowned MAC must attach only its own device, owner unverified: %+v", r.Matches)
	}
}

func TestNumberWithoutDomain(t *testing.T) {
	all := didCoverage{domains: true, users: true, callqueues: true, autoattendants: true, devices: true}
	num := rows(t, `[{"phonenumber":"16025550100","dial-rule-application":"to-user","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"[*]"}]`)
	if r := auditDIDs(num, rows(t, `[{"domain":"acme.example"}]`), nil, nil, nil, nil, all); r[0].TargetType != "unverified" {
		t.Errorf("a number without a domain must be unverified, not dangling: %+v", r[0])
	}
	td := traceData{synced: map[string]bool{rtPhonenumbers: true, rtSmsnumbers: true, rtUsers: true, rtDevices: true, rtCallqueues: true, rtAutoattendants: true, "phones": true}, numbers: num}
	if r := traceIdentifier("6025550100", "", td); r.Matches[0].TargetType != "unverified" || r.Complete {
		t.Errorf("a trace through an absent domain must be unverified: %+v", r.Matches)
	}
}

func TestRound12(t *testing.T) {
	all := map[string]bool{rtPhonenumbers: true, rtSmsnumbers: true, rtUsers: true, rtDevices: true, rtCallqueues: true, rtAutoattendants: true, "phones": true}
	td := traceData{synced: all, users: rows(t, `[{"domain":"acme.example","user":"100"}]`),
		devices: rows(t, `[{"domain":"acme.example","device":"100a","device-provisioning-mac-address":"001565000001"}]`)}
	if r := traceIdentifier("100", "acme.example", td); r.Complete {
		t.Errorf("an unowned device in the domain must leave the user's device list unverified: %+v", r)
	}
	td = traceData{synced: all, phones: rows(t, `[{"domain":"acme.example","device-provisioning-mac-address":"001565aabbcc"}]`),
		devices: rows(t, `[{"domain":"acme.example","device":"100a","user":"100"}]`)}
	if r := traceIdentifier("00:15:65:aa:bb:cc", "", td); r.Matches[0].MatchedOn == "mac-unassigned" || r.Complete {
		t.Errorf("a device without a MAC field blocks the unassigned claim: %+v", r.Matches)
	}
	reg := registeredDevices(rows(t, `[{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"registered"}]`))
	g := groupMissed(rows(t, `[{"id":"c1","call-term-user":"100"}]`), nil, reg, false, true, "")
	if g[0].RegisteredDevices != nil {
		t.Errorf("a CDR without a domain cannot join registration: %+v", g[0])
	}
	if g := groupMissed(rows(t, `[{"id":"c1","call-term-user":"100"}]`), nil, reg, false, true, "acme.example"); g[0].RegisteredDevices == nil || *g[0].RegisteredDevices != 1 {
		t.Errorf("a --domain request supplies the tenant: %+v", g[0])
	}
}

func TestNoTenantRows(t *testing.T) {
	cov := e911Coverage{domains: true, users: true, devices: true, addresses: true}
	rep := e911Gaps(rows(t, `[{"domain":"acme.example","caller-id-number-emergency":""}]`), nil,
		rows(t, `[{"device":"x1","user":"100"}]`), nil, cov)
	for _, g := range rep.Gaps {
		if g.Entity == "device" && !strings.Contains(strings.Join(g.Gaps, ","), "unverified") {
			t.Errorf("a device without a domain cannot be a confirmed gap: %v", g.Gaps)
		}
	}
	reg := registeredDevices(rows(t, `[{"device":"x1","user":"100","device-sip-registration-state":"registered"}]`))
	if reg.forUser("acme.example", "100").knownZero() {
		t.Error("a device with no domain could be this user's; zero is unverifiable")
	}
}

func TestLoadCoveredRejectsTenantlessTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"users": true, "devices": true}, map[string][]string{
		"users":   {`{"domain":"acme.example","user":"100"}`, `{"user":"100","email":"b@b.example"}`},
		"devices": {`{"domain":"acme.example","device":"100a","user":"100"}`}})
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cov := fleetCov{}
	rows, err := loadCovered(t.Context(), db, cov, rtUsers, rtDevices)
	if err != nil {
		t.Fatal(err)
	}
	if cov[rtUsers] || !cov[rtDevices] || len(rows) != 1 {
		t.Fatalf("a table with a tenantless row must be incomplete and excluded: cov=%v rows=%d", cov, len(rows))
	}
}
