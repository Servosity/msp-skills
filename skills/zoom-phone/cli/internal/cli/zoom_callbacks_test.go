// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func callbacksDecode(t *testing.T, out string) callbacksResult {
	t.Helper()
	var r callbacksResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("decode callbacks output: %v\n%s", err, out)
	}
	return r
}

func callbacksAgo(hours float64) string {
	return time.Now().UTC().Add(-time.Duration(hours * float64(time.Hour))).Format(time.RFC3339)
}

// callbacksRecord renders one spec-shaped call_history item.
func callbacksRecord(uuid, direction, connectType, result string, hidden bool, hoursAgo float64, callerNum, callerName, calleeNum, calleeName, calleeExt, calleeExtType string) string {
	return fmt.Sprintf(`{"call_history_uuid":%q,"call_id":"%s-call","direction":%q,"connect_type":%q,"call_result":%q,
		"hide_caller_id":%t,"start_time":%q,"caller_did_number":%q,"caller_name":%q,"caller_number_type":"external_pstn",
		"callee_did_number":%q,"callee_name":%q,"callee_ext_number":%q,"callee_ext_type":%q}`,
		uuid, uuid, direction, connectType, result, hidden, callbacksAgo(hoursAgo), callerNum, callerName, calleeNum, calleeName, calleeExt, calleeExtType)
}

// callbacksPartition reports which recording_status partition a request
// asks for. Like the API, an absent parameter means "recorded".
func callbacksPartition(q url.Values) string {
	if v := q.Get("recording_status"); v != "" {
		return v
	}
	return "recorded"
}

// A 7-day window split across the API's recording_status partitions. The
// recorded partition is two pages joined by next_page_token "p2"; the
// non_recorded partition holds calls that a recorded-only fetch never sees:
// Bob's second miss, the unrecorded callback to Alice and Dana's only miss.
func callbacksTwoPageRoutes(t *testing.T) map[string]routingFakeHandler {
	t.Helper()
	recordedPage1 := []string{
		// Bob: first miss (abandoned at the Sales Queue).
		callbacksRecord("b1", "inbound", "external", "abandoned", false, 10, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue"),
		// Alice: missed; called back later by an unrecorded call.
		callbacksRecord("a1", "inbound", "external", "no_answer", false, 5, "+14155550100", "Alice Account", "+14155550123", "Jane Doe", "1002", "user"),
		// Hidden caller ID: never listed.
		callbacksRecord("h1", "inbound", "external", "ring_timeout", true, 2, "+14155550999", "", "+14155550123", "Sales Queue", "1001", "call_queue"),
	}
	recordedPage2 := []string{
		// Carol: missed, then answered when she called again.
		callbacksRecord("c1", "inbound", "external", "voicemail", false, 6, "+14155550133", "Carol Client", "+14155550123", "Sales Queue", "1001", "call_queue"),
		callbacksRecord("c2", "inbound", "external", "answered", false, 1, "+14155550133", "Carol Client", "+14155550123", "Sales Queue", "1001", "call_queue"),
	}
	nonRecorded := []string{
		// Unrecorded outbound callback to Alice, formatted without the plus.
		callbacksRecord("a2", "outbound", "external", "answered", false, 4, "+14155550123", "Jane Doe", "14155550100", "", "", ""),
		// Bob: second miss (voicemail), unrecorded.
		callbacksRecord("b2", "inbound", "external", "voicemail", false, 3, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue"),
		// Dana: her only miss is unrecorded.
		callbacksRecord("d1", "inbound", "external", "no_answer", false, 2, "+14155550144", "Dana Dealer", "+14155550123", "Sales Queue", "1001", "call_queue"),
	}
	return map[string]routingFakeHandler{
		"/phone/call_history": func(q url.Values) (int, any) {
			if q.Get("from") == "" || q.Get("to") == "" || q.Get("page_size") != "300" {
				return http.StatusBadRequest, `{"code":300,"message":"from, to and page_size=300 expected"}`
			}
			if callbacksPartition(q) == "non_recorded" {
				return http.StatusOK, `{"next_page_token":"","page_size":300,"call_history":[` + strings.Join(nonRecorded, ",") + `]}`
			}
			switch q.Get("next_page_token") {
			case "":
				return http.StatusOK, `{"next_page_token":"p2","page_size":300,"call_history":[` + strings.Join(recordedPage1, ",") + `]}`
			case "p2":
				return http.StatusOK, `{"next_page_token":"","page_size":300,"call_history":[` + strings.Join(recordedPage2, ",") + `]}`
			}
			return http.StatusBadRequest, `{"code":300,"message":"bad token"}`
		},
	}
}

func TestCallbacksUnreturnedMissesAcrossPages(t *testing.T) {
	fake := routingStartFake(t, callbacksTwoPageRoutes(t))
	out, stderr, err := routingRun(t, newNovelCallbacksCmd, "--since", "7d")
	if err != nil {
		t.Fatalf("callbacks: %v\nstderr=%s", err, stderr)
	}
	r := callbacksDecode(t, out)

	if r.PagesFetched != 3 || r.ScannedCalls != 8 || r.Truncated || len(r.FetchFailures) != 0 {
		t.Fatalf("pages=%d scanned=%d truncated=%v failures=%v; want 3 pages, 8 calls, complete", r.PagesFetched, r.ScannedCalls, r.Truncated, r.FetchFailures)
	}
	if r.PagesByRecordingStatus["recorded"] != 2 || r.PagesByRecordingStatus["non_recorded"] != 1 {
		t.Fatalf("pages_by_recording_status = %v; want recorded 2, non_recorded 1", r.PagesByRecordingStatus)
	}
	if n := fake.countPrefix("/phone/call_history?"); n != 3 {
		t.Fatalf("call_history requests = %d; want 3. hits=%v", n, fake.hitList())
	}
	sawToken, sawRecorded, sawNonRecorded := false, false, false
	for _, h := range fake.hitList() {
		if strings.Contains(h, "next_page_token=p2") && strings.Contains(h, "recording_status=recorded") {
			sawToken = true
		}
		sawRecorded = sawRecorded || strings.Contains(h, "recording_status=recorded")
		sawNonRecorded = sawNonRecorded || strings.Contains(h, "recording_status=non_recorded")
	}
	if !sawToken || !sawRecorded || !sawNonRecorded {
		t.Fatalf("want both partitions requested explicitly and recorded page 2 via next_page_token=p2: %v", fake.hitList())
	}

	// Dana (unrecorded miss, 2h ago) then Bob (one recorded + one unrecorded miss).
	if len(r.Callers) != 2 || r.Callers[0].CallerNumber != "+14155550144" || r.Callers[0].Attempts != 1 {
		t.Fatalf("callers = %+v; want Dana then Bob (Alice called back unrecorded, Carol answered, hidden excluded)", r.Callers)
	}
	bob := r.Callers[1]
	if bob.CallerNumber != "+14155550111" || bob.CallerName != "Bob Buyer" || bob.Attempts != 2 {
		t.Fatalf("bob = %+v; want +14155550111 Bob Buyer attempts=2", bob)
	}
	if bob.LastResult != "voicemail" || bob.LastTarget.Name != "Sales Queue" || bob.LastTarget.ExtensionNumber != "1001" || bob.LastTarget.ExtensionType != "call_queue" {
		t.Fatalf("bob last miss = %+v / %+v", bob.LastResult, bob.LastTarget)
	}
	first, _ := time.Parse(time.RFC3339, bob.FirstMissedAt)
	last, _ := time.Parse(time.RFC3339, bob.LastMissedAt)
	if !first.Before(last) || last.Sub(first) < 6*time.Hour || last.Sub(first) > 8*time.Hour {
		t.Fatalf("first=%s last=%s; want ~7h apart", bob.FirstMissedAt, bob.LastMissedAt)
	}
	if bob.HoursWaiting < 9.8 || bob.HoursWaiting > 10.3 {
		t.Fatalf("hours_waiting = %v; want ~10 (since the first unreturned miss)", bob.HoursWaiting)
	}
	// Bob x2, Alice, Carol, Dana qualify as misses; the hidden-caller call does not.
	if r.MissedCalls != 5 {
		t.Fatalf("missed_calls = %d; want 5", r.MissedCalls)
	}
	for _, c := range r.Callers {
		if strings.Contains(c.CallerNumber, "555099") || strings.Contains(c.CallerNumber, "5550100") || strings.Contains(c.CallerNumber, "5550133") {
			t.Fatalf("excluded caller listed: %+v", c)
		}
	}
}

func TestCallbacksExtFilterAndCleanWindow(t *testing.T) {
	routingStartFake(t, callbacksTwoPageRoutes(t))
	out, _, err := routingRun(t, newNovelCallbacksCmd, "--ext", "1002")
	if err != nil {
		t.Fatalf("callbacks --ext: %v", err)
	}
	r := callbacksDecode(t, out)
	// Only Alice missed ext 1002 and she was called back: nothing to return.
	if len(r.Callers) != 0 || r.MissedCalls != 1 || r.Ext != "1002" {
		t.Fatalf("ext 1002: callers=%+v missed=%d ext=%q; want none, 1 miss", r.Callers, r.MissedCalls, r.Ext)
	}
	if !strings.Contains(out, `"callers": []`) {
		t.Fatalf("callers must serialize as [] not null:\n%s", out)
	}

	answeredOnly := map[string]routingFakeHandler{
		"/phone/call_history": routingStatic(`{"next_page_token":"","call_history":[` +
			callbacksRecord("x1", "inbound", "external", "answered", false, 2, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue") + `]}`),
	}
	routingStartFake(t, answeredOnly)
	out, _, err = routingRun(t, newNovelCallbacksCmd)
	if err != nil {
		t.Fatalf("callbacks answered-only: %v", err)
	}
	r = callbacksDecode(t, out)
	if len(r.Callers) != 0 || r.MissedCalls != 0 || r.ScannedCalls != 1 {
		t.Fatalf("answered-only window: callers=%+v missed=%d scanned=%d", r.Callers, r.MissedCalls, r.ScannedCalls)
	}
}

func TestCallbacksNegative(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/call_history": func(url.Values) (int, any) {
			return http.StatusUnauthorized, `{"code":124,"message":"Invalid access token."}`
		},
	})
	for _, args := range [][]string{{"--since", "soon"}, {"--since", "0d"}, {"--since", "200d"}, {"--max-pages", "0"}} {
		_, _, err := routingRun(t, newNovelCallbacksCmd, args...)
		if code := routingExitCode(err); code != 2 {
			t.Fatalf("%v exit = %d (%v); want 2", args, code, err)
		}
	}
	if len(fake.hitList()) != 0 {
		t.Fatalf("usage errors made API calls: %v", fake.hitList())
	}
	_, _, err := routingRun(t, newNovelCallbacksCmd)
	if code := routingExitCode(err); code != 4 {
		t.Fatalf("401 exit = %d (%v); want 4", code, err)
	}
	if n := fake.countPrefix("/phone/call_history?"); n != 1 {
		t.Fatalf("an auth failure made %d requests; want 1 (stop, every other walk would fail too)", n)
	}
}

func TestCallbacksComputeLaterCallbackOnlyClosesEarlierMisses(t *testing.T) {
	now := time.Now().UTC()
	at := func(h int) string { return now.Add(-time.Duration(h) * time.Hour).Format(time.RFC3339) }
	calls := []callbacksCall{
		{Direction: "inbound", ConnectType: "external", CallResult: "no_answer", StartTime: at(9), CallerDIDNumber: "+14155550111"},
		{Direction: "outbound", ConnectType: "external", CallResult: "answered", StartTime: at(8), CalleeDIDNumber: "+1 (415) 555-0111"},
		{Direction: "inbound", ConnectType: "external", CallResult: "abandoned", StartTime: at(2), CallerDIDNumber: "+14155550111"},
	}
	callers, missed := callbacksCompute(calls, "", now)
	if missed != 2 || len(callers) != 1 || callers[0].Attempts != 1 {
		t.Fatalf("callers=%+v missed=%d; want one caller with 1 open attempt (the miss after the callback)", callers, missed)
	}
	if callers[0].LastResult != "abandoned" {
		t.Fatalf("open miss = %+v; want the abandoned call after the callback", callers[0])
	}
}

// --since 45d spans two windows that share their boundary second. The API
// returns a call that starts exactly on the boundary in both windows; it is
// counted once (deduplicated by call_history_uuid), not as two attempts.
func TestCallbacksWindowBoundaryCallCountedOnce(t *testing.T) {
	var mu sync.Mutex
	newestTo := ""
	var windows [][2]string
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/call_history": func(q url.Values) (int, any) {
			mu.Lock()
			if newestTo == "" {
				newestTo = q.Get("to")
			}
			w := [2]string{q.Get("from"), q.Get("to")}
			if !slices.Contains(windows, w) {
				windows = append(windows, w)
			}
			newest := q.Get("to") == newestTo
			mu.Unlock()
			if callbacksPartition(q) == "non_recorded" {
				return http.StatusOK, `{"next_page_token":"","call_history":[]}`
			}
			// The newest window's "from" is the boundary; the older window's
			// "to" is the same second.
			boundary := q.Get("from")
			own := `{"call_history_uuid":"new-1","direction":"inbound","connect_type":"external","call_result":"answered","start_time":%q,"caller_did_number":"+14155550177","callee_did_number":"+14155550123"}`
			ownStart := callbacksAgo(24)
			if !newest {
				boundary = q.Get("to")
				own = `{"call_history_uuid":"old-1","direction":"inbound","connect_type":"external","call_result":"answered","start_time":%q,"caller_did_number":"+14155550188","callee_did_number":"+14155550123"}`
				ownStart = callbacksAgo(40 * 24)
			}
			edge := fmt.Sprintf(`{"call_history_uuid":"edge-1","call_id":"edge-call","direction":"inbound","connect_type":"external",
				"call_result":"no_answer","hide_caller_id":false,"start_time":%q,"caller_did_number":"+14155550166","caller_name":"Edge Caller",
				"callee_did_number":"+14155550123","callee_name":"Sales Queue","callee_ext_number":"1001","callee_ext_type":"call_queue"}`, boundary)
			return http.StatusOK, `{"next_page_token":"","call_history":[` + edge + `,` + fmt.Sprintf(own, ownStart) + `]}`
		},
	})
	out, stderr, err := routingRun(t, newNovelCallbacksCmd, "--since", "45d")
	if err != nil {
		t.Fatalf("callbacks: %v\nstderr=%s", err, stderr)
	}
	if n := fake.countPrefix("/phone/call_history?"); n != 4 || len(windows) != 2 {
		t.Fatalf("requests = %d over %d windows; want 2 windows x 2 recording_status partitions", n, len(windows))
	}
	if windows[1][1] != windows[0][0] {
		t.Fatalf("windows %v do not share a boundary second; the fixture is not exercising the overlap", windows)
	}
	r := callbacksDecode(t, out)
	if r.ScannedCalls != 3 {
		t.Fatalf("scanned_calls = %d; want 3 (edge-1 once + one call per window)", r.ScannedCalls)
	}
	if r.MissedCalls != 1 || len(r.Callers) != 1 || r.Callers[0].Attempts != 1 || r.Callers[0].CallerNumber != "+14155550166" {
		t.Fatalf("missed=%d callers=%+v; want the boundary caller once with attempts=1", r.MissedCalls, r.Callers)
	}
}

func TestCallbacksCallKeyUsesUniqueIDs(t *testing.T) {
	a := callbacksCall{CallHistoryUUID: "u-1", CallID: "same"}
	b := callbacksCall{CallHistoryUUID: "u-2", CallID: "same"}
	if callbacksCallKey(a) == callbacksCallKey(b) {
		t.Fatalf("two records sharing call_id but not call_history_uuid collapsed into one key")
	}
	if callbacksCallKey(callbacksCall{ID: "log-1"}) != "id:log-1" {
		t.Fatalf("call_logs items must key on id")
	}
}

// The recorded partition succeeds and the non_recorded partition fails: the
// partial result is printed with the failure in fetch_failures, and the
// command exits 6 (0 with --allow-partial-failure).
func TestCallbacksPartialFetchExitsSix(t *testing.T) {
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/call_history": func(q url.Values) (int, any) {
			if callbacksPartition(q) == "non_recorded" {
				return http.StatusBadRequest, `{"code":300,"message":"Invalid field."}`
			}
			return http.StatusOK, `{"next_page_token":"","call_history":[` +
				callbacksRecord("b1", "inbound", "external", "abandoned", false, 3, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue") + `]}`
		},
	})
	out, stderr, err := routingRun(t, newNovelCallbacksCmd)
	if code := routingExitCode(err); code != 6 {
		t.Fatalf("exit = %d (%v); want 6", code, err)
	}
	r := callbacksDecode(t, out)
	if len(r.FetchFailures) != 1 {
		t.Fatalf("fetch_failures = %+v; want the non_recorded partition", r.FetchFailures)
	}
	if f := r.FetchFailures[0]; f.RecordingStatus != "non_recorded" || !strings.Contains(f.Error, "HTTP 400") || f.From == "" || f.To == "" {
		t.Fatalf("fetch failure = %+v", f)
	}
	if r.PagesByRecordingStatus["recorded"] != 1 || r.PagesByRecordingStatus["non_recorded"] != 0 || len(r.Callers) != 1 || r.Note == "" {
		t.Fatalf("partial result = pages %v callers %d note %q", r.PagesByRecordingStatus, len(r.Callers), r.Note)
	}
	if !strings.Contains(stderr, "recording_status=non_recorded") {
		t.Fatalf("stderr = %q; want the failed partition named", stderr)
	}
	if _, _, err := routingRunFlags(t, &rootFlags{asJSON: true, allowPartialFailure: true}, newNovelCallbacksCmd); err != nil {
		t.Fatalf("--allow-partial-failure exit should be 0, got %v", err)
	}
}

// NoCache: a callback placed between two runs closes the miss on the second
// run in the same home (same response cache directory).
func TestCallbacksSeesCallbackImmediately(t *testing.T) {
	miss := callbacksRecord("b1", "inbound", "external", "abandoned", false, 3, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue")
	var body atomic.Value
	body.Store(`{"next_page_token":"","call_history":[` + miss + `]}`)
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/call_history": func(url.Values) (int, any) { return http.StatusOK, body.Load().(string) },
	})
	out, _, err := routingRun(t, newNovelCallbacksCmd)
	if err != nil || len(callbacksDecode(t, out).Callers) != 1 {
		t.Fatalf("run 1: err=%v out=%s; want Bob waiting", err, out)
	}
	callback := callbacksRecord("b2", "outbound", "external", "answered", false, 1, "+14155550123", "Jane Doe", "+14155550111", "Bob Buyer", "", "")
	body.Store(`{"next_page_token":"","call_history":[` + miss + `,` + callback + `]}`)
	out, _, err = routingRun(t, newNovelCallbacksCmd)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if n := len(callbacksDecode(t, out).Callers); n != 0 {
		t.Fatalf("run 2 callers = %d; want 0 (a cached response hid the callback)", n)
	}
}

// The page budget is shared fairly: with --max-pages 2 (the dogfood cap) each
// recording_status partition gets a page, rather than the recorded walk using
// the whole budget and the unrecorded calls never being read.
func TestCallbacksPageBudgetCoversBothPartitions(t *testing.T) {
	routingStartFake(t, callbacksTwoPageRoutes(t))
	out, _, err := routingRun(t, newNovelCallbacksCmd, "--max-pages", "2")
	if err != nil {
		t.Fatalf("callbacks --max-pages 2: %v", err)
	}
	r := callbacksDecode(t, out)
	if r.PagesByRecordingStatus["recorded"] != 1 || r.PagesByRecordingStatus["non_recorded"] != 1 || !r.Truncated || r.Note == "" {
		t.Fatalf("pages=%v truncated=%v note=%q; want one page per partition, truncated with a note", r.PagesByRecordingStatus, r.Truncated, r.Note)
	}
}

func callbacksHasLoopFailure(r callbacksResult, status string) bool {
	for _, f := range r.FetchFailures {
		if f.RecordingStatus == status && strings.Contains(f.Error, "pagination loop") {
			return true
		}
	}
	return false
}

// The recorded partition repeats the cursor it was requested with: its pages
// are kept, the partition is a fetch failure, and the command exits 6.
func TestCallbacksRepeatedCursorIsAFailure(t *testing.T) {
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/call_history": func(q url.Values) (int, any) {
			if callbacksPartition(q) == "non_recorded" {
				return http.StatusOK, `{"next_page_token":"","call_history":[]}`
			}
			if q.Get("next_page_token") == "r1" {
				return http.StatusOK, `{"next_page_token":"r1","call_history":[` +
					callbacksRecord("b2", "inbound", "external", "voicemail", false, 3, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue") + `]}`
			}
			return http.StatusOK, `{"next_page_token":"r1","call_history":[` +
				callbacksRecord("b1", "inbound", "external", "abandoned", false, 10, "+14155550111", "Bob Buyer", "+14155550123", "Sales Queue", "1001", "call_queue") + `]}`
		},
	})
	out, _, err := routingRun(t, newNovelCallbacksCmd)
	if code := routingExitCode(err); code != 6 {
		t.Fatalf("exit = %d (%v); want 6", code, err)
	}
	r := callbacksDecode(t, out)
	if !callbacksHasLoopFailure(r, "recorded") || r.PagesByRecordingStatus["recorded"] != 2 {
		t.Fatalf("failures=%+v pages=%v; want a recorded pagination-loop failure after 2 pages", r.FetchFailures, r.PagesByRecordingStatus)
	}
	if len(r.Callers) != 1 || r.Callers[0].Attempts != 2 {
		t.Fatalf("callers = %+v; want Bob with both fetched misses kept", r.Callers)
	}
}

// An A -> B -> A cursor cycle in the non_recorded partition is caught on the
// first repeat (3 requests), not walked until the page budget runs out.
func TestCallbacksCursorTwoCycleIsAFailure(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/call_history": func(q url.Values) (int, any) {
			if callbacksPartition(q) == "recorded" {
				return http.StatusOK, `{"next_page_token":"","call_history":[]}`
			}
			switch q.Get("next_page_token") {
			case "nA":
				return http.StatusOK, `{"next_page_token":"nB","call_history":[]}`
			case "nB":
				return http.StatusOK, `{"next_page_token":"nA","call_history":[]}`
			}
			return http.StatusOK, `{"next_page_token":"nA","call_history":[` +
				callbacksRecord("d1", "inbound", "external", "no_answer", false, 2, "+14155550144", "Dana Dealer", "+14155550123", "Sales Queue", "1001", "call_queue") + `]}`
		},
	})
	out, _, err := routingRun(t, newNovelCallbacksCmd)
	if code := routingExitCode(err); code != 6 {
		t.Fatalf("exit = %d (%v); want 6", code, err)
	}
	r := callbacksDecode(t, out)
	if !callbacksHasLoopFailure(r, "non_recorded") || r.Truncated {
		t.Fatalf("failures=%+v truncated=%v; want a non_recorded pagination-loop failure, not a page-cap truncation", r.FetchFailures, r.Truncated)
	}
	n := 0
	for _, h := range fake.hitList() {
		if strings.Contains(h, "recording_status=non_recorded") {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("non_recorded requests = %d; want 3 (stop at the first repeated cursor)", n)
	}
}

// Codex review r1 (class phone-identity-collision): an outbound call to a
// Canadian number must not close a missed call from the Austrian number that
// shares its national digits (call_history documents both DIDs as E.164).
func TestCallbacksCountryCodesDoNotCollide(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	calls := []callbacksCall{
		{CallHistoryUUID: "u1", Direction: "inbound", ConnectType: "external", CallResult: "no_answer", StartTime: "2026-10-26T10:00:00Z", CallerDIDNumber: "+4315550123"},
		{CallHistoryUUID: "u2", Direction: "outbound", ConnectType: "external", CallResult: "answered", StartTime: "2026-10-26T11:00:00Z", CalleeDIDNumber: "+14315550123"},
	}
	callers, missed := callbacksCompute(calls, "", now)
	if missed != 1 || len(callers) != 1 || callers[0].CallerNumber != "+4315550123" {
		t.Fatalf("callers=%+v missed=%d; the Austrian caller was never called back", callers, missed)
	}
}
