// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func routingDecodeAudit(t *testing.T, out string) routingAuditResult {
	t.Helper()
	var r routingAuditResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("decode audit output: %v\n%s", err, out)
	}
	return r
}

func routingFindings(r routingAuditResult, code string) []routingAuditFinding {
	var out []routingAuditFinding
	for _, f := range r.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func routingDay(offsetDays int) string {
	return time.Now().UTC().Add(time.Duration(offsetDays) * 24 * time.Hour).Format(time.RFC3339)
}

// routingAuditBaseRoutes is a healthy single-site account: one main auto
// receptionist (numbers, upcoming holiday) whose IVR sends key 1 to a call
// queue with a receiving member and no-input to an active user.
func routingAuditBaseRoutes() map[string]routingFakeHandler {
	return map[string]routingFakeHandler{
		"/phone/auto_receptionists": routingStatic(fmt.Sprintf(`{"next_page_token":"","auto_receptionists":[
			{"id":"arMain","name":"Main Line","extension_id":"extMain","extension_number":1000,
			 "phone_numbers":[{"id":"pn1","number":"+14155550123"}],
			 "holiday_hours":[{"id":"h1","name":"Thanksgiving","from":%q,"to":%q}],
			 "site":{"id":"s1","name":"HQ"}}]}`, routingDay(30), routingDay(31))),
		"/phone/call_queues": routingStatic(`{"next_page_token":"","call_queues":[
			{"id":"cqSales","name":"Sales Queue","extension_id":"extSales","extension_number":1001,"status":"active","phone_numbers":[]}]}`),
		"/phone/users": routingStatic(`{"next_page_token":"","users":[
			{"id":"u1","name":"Jane Doe","email":"jane@example.com","extension_id":"extU1","extension_number":1002,"status":"activate","activation_status":"activated"}]}`),
		"/phone/common_areas":       routingStatic(`{"next_page_token":"","common_areas":[]}`),
		"/phone/shared_line_groups": routingStatic(`{"next_page_token":"","shared_line_groups":[]}`),
		"/phone/sites": routingStatic(`{"next_page_token":"","sites":[
			{"id":"s1","name":"HQ","main_auto_receptionist":{"id":"arMain","extension_id":"extMain","extension_number":1000,"name":"Main Line"}}]}`),
		"/phone/auto_receptionists/arMain/ivr": func(q url.Values) (int, any) {
			if q.Get("hours_type") == "closed_hours" {
				return http.StatusOK, `{"key_actions":[{"key":"1","action":300,"target":{"id":"arMain","extension_id":"extMain","extension_number":"1000","display_name":"Main Line"}}]}`
			}
			return http.StatusOK, `{"key_actions":[
				{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}}],
				"caller_enters_no_action":{"action":2,"forward_to":{"id":"u1","extension_id":"extU1","extension_number":"1002","display_name":"Jane Doe"}}}`
		},
		"/phone/call_queues/cqSales/members": routingStatic(`{"call_queue_members":[
			{"id":"u1","level":"user","name":"Jane Doe","receive_call":true,"extension_id":"extU1"}]}`),
	}
}

func TestRoutingAuditFlagsBrokenRoutingNotHealthyQueue(t *testing.T) {
	routes := routingAuditBaseRoutes()
	routes["/phone/auto_receptionists"] = routingStatic(fmt.Sprintf(`{"next_page_token":"","auto_receptionists":[
		{"id":"arMain","name":"Main Line","extension_id":"extMain","extension_number":1000,
		 "phone_numbers":[{"id":"pn1","number":"+14155550123"}],
		 "holiday_hours":[{"id":"h1","name":"Thanksgiving","from":%q,"to":%q}]},
		{"id":"arOld","name":"Old Menu","extension_id":"extOld","extension_number":1009,"phone_numbers":[],"holiday_hours":[]},
		{"id":"arBranch","name":"Branch Line","extension_id":"extBranch","extension_number":1010,
		 "phone_numbers":[{"id":"pn2","number":"+14155550199"}],
		 "holiday_hours":[{"id":"h2","name":"Labor Day","from":%q,"to":%q}]}]}`,
		routingDay(30), routingDay(31), routingDay(-60), routingDay(-59)))
	routes["/phone/call_queues"] = routingStatic(`{"next_page_token":"","call_queues":[
		{"id":"cqSales","name":"Sales Queue","extension_id":"extSales","extension_number":1001,"status":"active"},
		{"id":"cqSupport","name":"Support Queue","extension_id":"extSupport","extension_number":1003,"status":"active"}]}`)
	routes["/phone/users"] = routingStatic(`{"next_page_token":"","users":[
		{"id":"u1","name":"Jane Doe","extension_id":"extU1","extension_number":1002,"status":"activate","activation_status":"activated"},
		{"id":"u3","name":"Gone Person","extension_id":"extU3","extension_number":1005,"status":"deactivate","activation_status":"activated"}]}`)
	routes["/phone/auto_receptionists/arMain/ivr"] = func(q url.Values) (int, any) {
		if q.Get("hours_type") == "closed_hours" {
			return http.StatusOK, `{"key_actions":[]}`
		}
		return http.StatusOK, `{"key_actions":[
			{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}},
			{"key":"2","action":7,"target":{"id":"cqSupport","extension_id":"extSupport","extension_number":"1003","display_name":"Support Queue"}},
			{"key":"3","action":2,"target":{"id":"uMissing","extension_id":"extMissing","extension_number":"1099","display_name":"Former Employee"}},
			{"key":"4","action":2,"target":{"id":"u3","extension_id":"extU3","extension_number":"1005","display_name":"Gone Person"}}],
			"caller_enters_no_action":{"action":2,"forward_to":{"id":"u1","extension_id":"extU1","extension_number":"1002","display_name":"Jane Doe"}}}`
	}
	routes["/phone/auto_receptionists/arOld/ivr"] = routingStatic(`{"key_actions":[]}`)
	routes["/phone/auto_receptionists/arBranch/ivr"] = func(q url.Values) (int, any) {
		if q.Get("hours_type") == "closed_hours" {
			return http.StatusOK, `{}`
		}
		return http.StatusOK, `{"key_actions":[{"key":"0","action":6,"target":{"id":"arMain","extension_id":"extMain","extension_number":"1000","display_name":"Main Line"}}]}`
	}
	routes["/phone/call_queues/cqSupport/members"] = routingStatic(`{"call_queue_members":[]}`)
	fake := routingStartFake(t, routes)

	out, stderr, err := routingRun(t, newNovelRoutingAuditCmd)
	if err != nil {
		t.Fatalf("routing audit: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeAudit(t, out)

	if r.ScannedAutoReceptionists != 3 || r.ScannedCallQueues != 2 || r.ScannedUsers != 2 || r.ScannedSites != 1 || r.ScannedIVRs != 6 || r.ScannedQueueMemberLists != 2 {
		t.Fatalf("scanned counts = ars %d cqs %d users %d sites %d ivrs %d member lists %d",
			r.ScannedAutoReceptionists, r.ScannedCallQueues, r.ScannedUsers, r.ScannedSites, r.ScannedIVRs, r.ScannedQueueMemberLists)
	}
	if len(r.FetchFailures) != 0 || len(r.SkippedChecks) != 0 {
		t.Fatalf("fetch_failures=%v skipped=%v; want none", r.FetchFailures, r.SkippedChecks)
	}

	dead := routingFindings(r, routingCodeDeadTarget)
	if len(dead) != 2 {
		t.Fatalf("DEAD_TARGET = %d findings %+v; want 2 (missing user, inactive user)", len(dead), dead)
	}
	byKey := map[string]routingAuditFinding{}
	for _, f := range dead {
		byKey[f.Key] = f
		if f.EntityID != "arMain" || f.HoursType != "business_hours" {
			t.Fatalf("dead target on wrong entity/hours: %+v", f)
		}
	}
	if f, ok := byKey["3"]; !ok || f.TargetID != "uMissing" || !strings.Contains(f.Detail, "does not exist") {
		t.Fatalf("key 3 finding = %+v; want missing user uMissing", f)
	}
	if f, ok := byKey["4"]; !ok || f.TargetID != "u3" || !strings.Contains(f.Detail, "not active") {
		t.Fatalf("key 4 finding = %+v; want inactive user u3", f)
	}

	empty := routingFindings(r, routingCodeEmptyQueue)
	if len(empty) != 1 || empty[0].EntityID != "cqSupport" || empty[0].Detail != "call queue has no members" {
		t.Fatalf("EMPTY_QUEUE = %+v; want only cqSupport", empty)
	}
	for _, f := range r.Findings {
		if f.EntityID == "cqSales" || f.TargetID == "cqSales" || f.TargetID == "cqSupport" || f.TargetID == "u1" {
			t.Fatalf("healthy entity flagged: %+v", f)
		}
	}

	orphans := routingFindings(r, routingCodeOrphanAR)
	if len(orphans) != 1 || orphans[0].EntityID != "arOld" {
		t.Fatalf("ORPHAN_AR = %+v; want only arOld (arMain is site main, arBranch has numbers)", orphans)
	}
	holidays := routingFindings(r, routingCodeNoUpcomingHoliday)
	if len(holidays) != 1 || holidays[0].EntityID != "arBranch" {
		t.Fatalf("NO_UPCOMING_HOLIDAY = %+v; want only arBranch (past holiday only)", holidays)
	}
	if r.Summary[routingCodeDeadTarget] != 2 || r.Summary[routingCodeEmptyQueue] != 1 || r.Summary[routingCodeOrphanAR] != 1 || r.Summary[routingCodeNoUpcomingHoliday] != 1 {
		t.Fatalf("summary = %v", r.Summary)
	}
	if n := fake.countPrefix("/phone/auto_receptionists/arMain/ivr?hours_type=closed_hours"); n != 1 {
		t.Fatalf("closed IVR fetched %d times; want 1", n)
	}
}

func TestRoutingAuditCleanAccountHasNoFindings(t *testing.T) {
	routingStartFake(t, routingAuditBaseRoutes())
	out, stderr, err := routingRun(t, newNovelRoutingAuditCmd)
	if err != nil {
		t.Fatalf("routing audit: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeAudit(t, out)
	if len(r.Findings) != 0 {
		t.Fatalf("clean account produced findings: %+v", r.Findings)
	}
	if !strings.Contains(out, `"findings": []`) {
		t.Fatalf("findings must serialize as [] not null:\n%s", out)
	}
	for code, n := range r.Summary {
		if n != 0 {
			t.Fatalf("summary[%s] = %d; want 0", code, n)
		}
	}
	if r.ScannedIVRs != 2 || r.ScannedQueueMemberLists != 1 {
		t.Fatalf("scanned ivrs=%d member lists=%d; want 2 and 1", r.ScannedIVRs, r.ScannedQueueMemberLists)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

// Some fetches fail, others succeed: the report is still printed, failures
// are listed, and the command exits 6 (0 with --allow-partial-failure).
func TestRoutingAuditPartialFailureIsReportedNotFatal(t *testing.T) {
	routes := routingAuditBaseRoutes()
	routes["/phone/auto_receptionists/arMain/ivr"] = func(q url.Values) (int, any) {
		if q.Get("hours_type") == "closed_hours" {
			return http.StatusForbidden, `{"code":124,"message":"Invalid access token, does not contain scopes."}`
		}
		return http.StatusOK, `{"key_actions":[{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}}]}`
	}
	routingStartFake(t, routes)
	out, stderr, err := routingRun(t, newNovelRoutingAuditCmd)
	if code := routingExitCode(err); code != 6 {
		t.Fatalf("partial failure exit = %d (%v); want 6 (partialFailureErr)", code, err)
	}
	r := routingDecodeAudit(t, out)
	if len(r.FetchFailures) != 1 || !strings.Contains(r.FetchFailures[0].Path, "closed_hours") || !strings.Contains(r.FetchFailures[0].Error, "HTTP 403") {
		t.Fatalf("fetch_failures = %+v; want the closed-hours IVR 403", r.FetchFailures)
	}
	if !strings.Contains(stderr, "warning: 1 fetch(es) failed") {
		t.Fatalf("stderr = %q; want a fetch-failure warning", stderr)
	}
	skippedOrphan := false
	for _, s := range r.SkippedChecks {
		if s.Check == routingCodeOrphanAR {
			skippedOrphan = true
		}
	}
	if !skippedOrphan || len(routingFindings(r, routingCodeOrphanAR)) != 0 {
		t.Fatalf("ORPHAN_AR must be skipped (not guessed) when an IVR fetch failed: skipped=%+v", r.SkippedChecks)
	}
	if r.ScannedIVRs != 1 {
		t.Fatalf("scanned_ivrs = %d; want 1 (the failed fetch is excluded)", r.ScannedIVRs)
	}

	out, _, err = routingRunFlags(t, &rootFlags{asJSON: true, allowPartialFailure: true}, newNovelRoutingAuditCmd)
	if err != nil {
		t.Fatalf("--allow-partial-failure should exit 0, got %v", err)
	}
	if r := routingDecodeAudit(t, out); len(r.FetchFailures) != 1 {
		t.Fatalf("--allow-partial-failure still reports fetch_failures; got %+v", r.FetchFailures)
	}
}

// The first API call runs before any goroutine starts, so a first-use token
// mint is never raced: no sibling list request may arrive while the first
// auto receptionist page is still being served.
func TestRoutingAuditFirstCallIsSequential(t *testing.T) {
	routes := routingAuditBaseRoutes()
	var firstDone atomic.Bool
	var early atomic.Int32
	arList := routes["/phone/auto_receptionists"]
	routes["/phone/auto_receptionists"] = func(q url.Values) (int, any) {
		// Hold the first page: a concurrently started sibling would land now.
		time.Sleep(60 * time.Millisecond)
		firstDone.Store(true)
		return arList(q)
	}
	for _, p := range []string{"/phone/call_queues", "/phone/users", "/phone/common_areas", "/phone/shared_line_groups", "/phone/sites"} {
		h := routes[p]
		routes[p] = func(q url.Values) (int, any) {
			if !firstDone.Load() {
				early.Add(1)
			}
			return h(q)
		}
	}
	fake := routingStartFake(t, routes)
	if _, _, err := routingRun(t, newNovelRoutingAuditCmd); err != nil {
		t.Fatalf("routing audit: %v", err)
	}
	if n := early.Load(); n != 0 {
		t.Fatalf("%d sibling list request(s) started before the first list call finished", n)
	}
	if hits := fake.hitList(); len(hits) == 0 || !strings.HasPrefix(hits[0], "/phone/auto_receptionists?") {
		t.Fatalf("first request = %v; want the auto receptionist list", hits)
	}
}

// NoCache: a queue fixed between two runs is no longer flagged on the second
// run, even in the same home (same response cache directory).
func TestRoutingAuditSeesFixImmediately(t *testing.T) {
	routes := routingAuditBaseRoutes()
	var members atomic.Value
	members.Store(`{"call_queue_members":[]}`)
	routes["/phone/call_queues/cqSales/members"] = func(url.Values) (int, any) { return http.StatusOK, members.Load().(string) }
	routingStartFake(t, routes)
	out, _, err := routingRun(t, newNovelRoutingAuditCmd)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if n := len(routingFindings(routingDecodeAudit(t, out), routingCodeEmptyQueue)); n != 1 {
		t.Fatalf("run 1 EMPTY_QUEUE = %d; want 1", n)
	}
	members.Store(`{"call_queue_members":[{"id":"u1","level":"user","name":"Jane Doe","receive_call":true,"extension_id":"extU1"}]}`)
	out, _, err = routingRun(t, newNovelRoutingAuditCmd)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if n := len(routingFindings(routingDecodeAudit(t, out), routingCodeEmptyQueue)); n != 0 {
		t.Fatalf("run 2 EMPTY_QUEUE = %d; want 0 (a cached response hid the fix)", n)
	}
}

func TestRoutingAuditNegative(t *testing.T) {
	unauthorized := func(url.Values) (int, any) {
		return http.StatusUnauthorized, `{"code":124,"message":"Invalid access token."}`
	}
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/auto_receptionists": unauthorized, "/phone/call_queues": unauthorized, "/phone/users": unauthorized,
		"/phone/common_areas": unauthorized, "/phone/shared_line_groups": unauthorized, "/phone/sites": unauthorized,
	})
	_, _, err := routingRun(t, newNovelRoutingAuditCmd)
	if code := routingExitCode(err); code != 4 {
		t.Fatalf("all lists 401 exit = %d (%v); want 4 (auth)", code, err)
	}
	before := len(fake.hitList())
	_, _, err = routingRun(t, newNovelRoutingAuditCmd, "--max-pages", "0")
	if code := routingExitCode(err); code != 2 {
		t.Fatalf("--max-pages 0 exit = %d; want 2", code)
	}
	_, _, err = routingRun(t, newNovelRoutingAuditCmd, "--holiday-window", "soon")
	if code := routingExitCode(err); code != 2 {
		t.Fatalf("--holiday-window soon exit = %d; want 2", code)
	}
	if len(fake.hitList()) != before {
		t.Fatalf("usage errors made API calls")
	}
}

// A member list cut by --max-pages must not produce EMPTY_QUEUE when the
// receiving member is on a page that was not fetched: the gap is reported
// under skipped_checks (with the queue id) and truncated_lists instead.
func TestRoutingAuditTruncatedMemberListIsNotEmptyQueue(t *testing.T) {
	routes := routingAuditBaseRoutes()
	var complete atomic.Bool
	routes["/phone/call_queues/cqSales/members"] = func(q url.Values) (int, any) {
		if complete.Load() {
			// Control: a complete list where nobody receives calls.
			return http.StatusOK, `{"call_queue_members":[{"id":"u2","level":"user","name":"John Roe","receive_call":false}],"next_page_token":""}`
		}
		if q.Get("next_page_token") == "m2" {
			return http.StatusOK, `{"call_queue_members":[{"id":"u1","level":"user","name":"Jane Doe","receive_call":true}],"next_page_token":""}`
		}
		return http.StatusOK, `{"call_queue_members":[{"id":"u2","level":"user","name":"John Roe","receive_call":false}],"next_page_token":"m2"}`
	}
	routingStartFake(t, routes)

	out, stderr, err := routingRun(t, newNovelRoutingAuditCmd, "--max-pages", "1")
	if err != nil {
		t.Fatalf("routing audit --max-pages 1: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeAudit(t, out)
	if n := len(routingFindings(r, routingCodeEmptyQueue)); n != 0 {
		t.Fatalf("EMPTY_QUEUE = %d on a truncated member list; want 0 (receiving member is on page 2): %+v", n, r.Findings)
	}
	skipped := false
	for _, s := range r.SkippedChecks {
		if s.Check == routingCodeEmptyQueue && s.EntityID == "cqSales" && strings.Contains(s.Reason, "--max-pages 1") {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("skipped_checks = %+v; want EMPTY_QUEUE skipped for cqSales", r.SkippedChecks)
	}
	if !slices.Contains(r.TruncatedLists, "call_queue_members:cqSales") {
		t.Fatalf("truncated_lists = %v; want call_queue_members:cqSales", r.TruncatedLists)
	}

	// Enough pages: the receiving member is found, nothing skipped or flagged.
	out, _, err = routingRun(t, newNovelRoutingAuditCmd, "--max-pages", "2")
	if err != nil {
		t.Fatalf("routing audit --max-pages 2: %v", err)
	}
	r = routingDecodeAudit(t, out)
	if len(routingFindings(r, routingCodeEmptyQueue)) != 0 || len(r.SkippedChecks) != 0 || len(r.TruncatedLists) != 0 {
		t.Fatalf("full paging: findings=%+v skipped=%+v truncated=%v; want none", r.Findings, r.SkippedChecks, r.TruncatedLists)
	}

	// Control: a complete list with no receiving member is still flagged.
	complete.Store(true)
	out, _, err = routingRun(t, newNovelRoutingAuditCmd, "--max-pages", "1")
	if err != nil {
		t.Fatalf("routing audit control: %v", err)
	}
	empty := routingFindings(routingDecodeAudit(t, out), routingCodeEmptyQueue)
	if len(empty) != 1 || empty[0].EntityID != "cqSales" || !strings.Contains(empty[0].Detail, "receive_call=true") {
		t.Fatalf("control EMPTY_QUEUE = %+v; want cqSales flagged (no member receiving)", empty)
	}
}

// zoomFlexString migration: extension numbers arriving as JSON numbers, as
// padded strings, or as an unexpected object all index and compare correctly.
func TestRoutingFlexExtensionNumbers(t *testing.T) {
	var v struct {
		A zoomFlexString `json:"a"`
		B zoomFlexString `json:"b"`
		C zoomFlexString `json:"c"`
		D zoomFlexString `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"a":1001,"b":" 1001 ","c":{"x":1},"d":null}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A.String() != "1001" || v.B.String() != "1001" || v.C.String() != "" || v.D.String() != "" {
		t.Fatalf("decoded a=%q b=%q c=%q d=%q", v.A.String(), v.B.String(), v.C.String(), v.D.String())
	}
	ix := routingEntityIndex{}
	ix.add(routingEntityInfo{typ: routingTypeUser, id: "u1", ext: v.A.String()}, "")
	if _, ok, _ := ix.lookup(routingIVRTarget{ExtensionNumber: v.B}); !ok {
		t.Fatalf("padded string extension did not match the numeric one")
	}
	if _, ok, _ := ix.lookup(routingIVRTarget{ExtensionNumber: v.C}); ok {
		t.Fatalf("object-valued extension must not match anything")
	}
}

// Stable identifiers are authoritative for DEAD_TARGET: a deleted user's
// extension number now belonging to an active user must not hide the dead
// target, and an id / extension_id pair naming two different users is a
// DEAD_TARGET with the reason. A number-only target still resolves by number,
// and a common area (whose list rows carry no extension_id) is not a conflict.
func TestRoutingAuditStableIDsAreAuthoritative(t *testing.T) {
	routes := routingAuditBaseRoutes()
	routes["/phone/users"] = routingStatic(`{"next_page_token":"","users":[
		{"id":"u1","name":"Jane Doe","extension_id":"extU1","extension_number":1002,"status":"activate","activation_status":"activated"},
		{"id":"u9","name":"Bob Other","extension_id":"extU9","extension_number":1009,"status":"activate","activation_status":"activated"}]}`)
	routes["/phone/common_areas"] = routingStatic(`{"next_page_token":"","common_areas":[{"id":"ca1","display_name":"Lobby Phone","extension_number":1050}]}`)
	routes["/phone/auto_receptionists/arMain/ivr"] = func(q url.Values) (int, any) {
		if q.Get("hours_type") == "closed_hours" {
			return http.StatusOK, `{"key_actions":[]}`
		}
		return http.StatusOK, `{"key_actions":[
			{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}},
			{"key":"3","action":2,"target":{"id":"uGone","extension_id":"extGone","extension_number":"1002","display_name":"Former Employee"}},
			{"key":"4","action":2,"target":{"id":"u1","extension_id":"extU9","extension_number":"1002","display_name":"Jane Doe"}},
			{"key":"5","action":2,"target":{"extension_number":"1002"}},
			{"key":"6","action":4,"target":{"id":"ca1","extension_id":"extCA1","extension_number":"1050","display_name":"Lobby Phone"}}],
			"caller_enters_no_action":{"action":2,"forward_to":{"id":"u1","extension_id":"extU1","extension_number":"1002","display_name":"Jane Doe"}}}`
	}
	routingStartFake(t, routes)
	out, stderr, err := routingRun(t, newNovelRoutingAuditCmd)
	if err != nil {
		t.Fatalf("routing audit: %v\nstderr=%s", err, stderr)
	}
	dead := routingFindings(routingDecodeAudit(t, out), routingCodeDeadTarget)
	byKey := map[string]routingAuditFinding{}
	for _, f := range dead {
		byKey[f.Key] = f
	}
	if len(dead) != 2 {
		t.Fatalf("DEAD_TARGET = %+v; want exactly keys 3 and 4", dead)
	}
	if f, ok := byKey["3"]; !ok || f.TargetID != "uGone" || !strings.Contains(f.Detail, "does not exist") {
		t.Fatalf("key 3 = %+v; want uGone dead even though extension 1002 now belongs to Jane", f)
	}
	if f, ok := byKey["4"]; !ok || !strings.Contains(f.Detail, "disagree") || !strings.Contains(f.Detail, "extU9") {
		t.Fatalf("key 4 = %+v; want an id/extension_id conflict DEAD_TARGET naming extU9", f)
	}
}

func routingHasFailure(r routingAuditResult, pathPrefix string) bool {
	for _, f := range r.FetchFailures {
		if strings.HasPrefix(f.Path, pathPrefix) && strings.Contains(f.Error, "pagination loop") {
			return true
		}
	}
	return false
}

func routingHasSkip(r routingAuditResult, check, entityID string) bool {
	for _, s := range r.SkippedChecks {
		if s.Check == check && s.EntityID == entityID {
			return true
		}
	}
	return false
}

// A page that hands back the same next_page_token it was requested with is a
// failed list, not a complete one: dependent checks move to skipped_checks
// instead of reporting a dead target / empty queue from half a list.
func TestRoutingAuditRepeatedCursorIsAFailure(t *testing.T) {
	routes := routingAuditBaseRoutes()
	routes["/phone/users"] = func(q url.Values) (int, any) {
		if q.Get("next_page_token") == "uTok" {
			return http.StatusOK, `{"users":[],"next_page_token":"uTok"}`
		}
		return http.StatusOK, `{"users":[{"id":"u1","name":"Jane Doe","extension_id":"extU1","extension_number":1002,"status":"activate"}],"next_page_token":"uTok"}`
	}
	routes["/phone/call_queues/cqSales/members"] = routingStatic(`{"call_queue_members":[{"id":"u2","level":"user","name":"John Roe","receive_call":false}],"next_page_token":"m1"}`)
	routes["/phone/auto_receptionists/arMain/ivr"] = func(q url.Values) (int, any) {
		if q.Get("hours_type") == "closed_hours" {
			return http.StatusOK, `{"key_actions":[]}`
		}
		return http.StatusOK, `{"key_actions":[
			{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}},
			{"key":"3","action":2,"target":{"id":"uMissing","extension_id":"extMissing","extension_number":"1099","display_name":"Former Employee"}}]}`
	}
	routingStartFake(t, routes)
	out, _, err := routingRun(t, newNovelRoutingAuditCmd)
	if code := routingExitCode(err); code != 6 {
		t.Fatalf("exit = %d (%v); want 6", code, err)
	}
	r := routingDecodeAudit(t, out)
	if !routingHasFailure(r, "/phone/users") || !routingHasFailure(r, "/phone/call_queues/cqSales/members") {
		t.Fatalf("fetch_failures = %+v; want pagination-loop failures for users and cqSales members", r.FetchFailures)
	}
	if n := len(routingFindings(r, routingCodeDeadTarget)) + len(routingFindings(r, routingCodeEmptyQueue)); n != 0 {
		t.Fatalf("findings from half-read lists: %+v", r.Findings)
	}
	if !routingHasSkip(r, routingCodeDeadTarget+" (user targets not found)", "") || !routingHasSkip(r, routingCodeEmptyQueue, "cqSales") {
		t.Fatalf("skipped_checks = %+v; want DEAD_TARGET (user) and EMPTY_QUEUE cqSales skipped", r.SkippedChecks)
	}
}

// An A -> B -> A cursor cycle is caught on the first repeat (3 requests),
// not walked until --max-pages and reported as merely truncated.
func TestRoutingAuditCursorTwoCycleIsAFailure(t *testing.T) {
	routes := routingAuditBaseRoutes()
	routes["/phone/call_queues"] = func(q url.Values) (int, any) {
		switch q.Get("next_page_token") {
		case "qA":
			return http.StatusOK, `{"call_queues":[],"next_page_token":"qB"}`
		case "qB":
			return http.StatusOK, `{"call_queues":[],"next_page_token":"qA"}`
		}
		return http.StatusOK, `{"call_queues":[{"id":"cqSales","name":"Sales Queue","extension_id":"extSales","extension_number":1001,"status":"active"}],"next_page_token":"qA"}`
	}
	routes["/phone/auto_receptionists/arMain/ivr"] = func(q url.Values) (int, any) {
		if q.Get("hours_type") == "closed_hours" {
			return http.StatusOK, `{"key_actions":[]}`
		}
		return http.StatusOK, `{"key_actions":[
			{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}},
			{"key":"2","action":7,"target":{"id":"cqMissing","extension_id":"extMissing","extension_number":"1098","display_name":"Old Queue"}}]}`
	}
	fake := routingStartFake(t, routes)
	out, _, err := routingRun(t, newNovelRoutingAuditCmd)
	if code := routingExitCode(err); code != 6 {
		t.Fatalf("exit = %d (%v); want 6", code, err)
	}
	r := routingDecodeAudit(t, out)
	if !routingHasFailure(r, "/phone/call_queues") || slices.Contains(r.TruncatedLists, "call_queues") {
		t.Fatalf("fetch_failures=%+v truncated=%v; want a pagination-loop failure, not a page-cap truncation", r.FetchFailures, r.TruncatedLists)
	}
	if n := fake.countPrefix("/phone/call_queues?"); n != 3 {
		t.Fatalf("call queue list requests = %d; want 3 (stop at the first repeated cursor)", n)
	}
	if len(routingFindings(r, routingCodeDeadTarget)) != 0 || !routingHasSkip(r, routingCodeDeadTarget+" (callQueue targets not found)", "") {
		t.Fatalf("findings=%+v skipped=%+v; want the missing queue skipped, not flagged", r.Findings, r.SkippedChecks)
	}
}
