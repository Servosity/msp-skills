// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func routingDecodeTrace(t *testing.T, out string) routingTraceResult {
	t.Helper()
	var r routingTraceResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("decode trace output: %v\n%s", err, out)
	}
	return r
}

func routingByKey(t *testing.T, n *routingNode, key string) *routingNode {
	t.Helper()
	for _, ch := range n.Children {
		if ch.Key == key {
			return ch
		}
	}
	t.Fatalf("node %s/%s has no child with key %q (children=%d)", n.Type, n.ID, key, len(n.Children))
	return nil
}

// AR business IVR key 1 -> call queue -> members (+ overflow); closed hours
// call handling -> voicemail. No-input forwards to a user.
func TestRoutingTraceFollowsIVRToQueueMembersAndClosedVoicemail(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": func(q url.Values) (int, any) {
			if q.Get("phone_number") != "+14155550123" {
				return http.StatusOK, `{"phone_numbers":[],"next_page_token":""}`
			}
			return http.StatusOK, `{"next_page_token":"","page_size":30,"phone_numbers":[
				{"id":"pn1","number":"+14155550123","assignee":{"id":"arMain","name":"Main Line","extension_number":1000,"type":"autoReceptionist"}}]}`
		},
		"/phone/auto_receptionists/arMain/call_handling/settings": func(q url.Values) (int, any) {
			if q.Get("hour_type") == "closed_hours" {
				return http.StatusOK, `{"call_handling_setting":{"routing_action":"forward_to_voicemail","routing_action_setting":{
					"leave_voicemail_to":{"extension_id":"extMain","extension_number":1000,"extension_name":"Main Line","extension_type":"autoReceptionist"}}}}`
			}
			return http.StatusOK, `{"call_handling_setting":{"routing_action":"forward_to_ivr"}}`
		},
		"/phone/auto_receptionists/arMain/ivr": func(q url.Values) (int, any) {
			if q.Get("hours_type") != "business_hours" {
				return http.StatusBadRequest, `{"code":300,"message":"unexpected hours_type"}`
			}
			return http.StatusOK, `{"key_actions":[
				{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}},
				{"key":"2","action":-1}],
				"caller_enters_no_action":{"action":2,"forward_to":{"id":"u1","extension_id":"extU1","extension_number":"1002","display_name":"Jane Doe"}}}`
		},
		"/phone/call_queues/cqSales/members": routingStatic(`{"call_queue_members":[
			{"id":"u1","level":"user","name":"Jane Doe","receive_call":true,"extension_id":"extU1"},
			{"id":"u2","level":"user","name":"John Roe","receive_call":false,"extension_id":"extU2"}],"next_page_token":""}`),
		"/phone/call_queues/cqSales/call_handling/settings": routingStatic(`{"call_handling_setting":{"overflow_action":"forward_to_voicemail","overflow_action_setting":{
			"leave_voicemail_to":{"extension_id":"extSales","extension_number":1001,"extension_name":"Sales Queue","extension_type":"callQueue"}}}}`),
		// Target verification list: cqSales's id and extension_id agree.
		"/phone/call_queues": routingStatic(`{"call_queues":[{"id":"cqSales","name":"Sales Queue","extension_id":"extSales","extension_number":1001}],"next_page_token":""}`),
	})

	out, stderr, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if err != nil {
		t.Fatalf("routing trace: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeTrace(t, out)
	if r.Resolved.Type != "autoReceptionist" || r.Resolved.ID != "arMain" || r.Resolved.ExtensionNumber != "1000" {
		t.Fatalf("resolved = %+v; want autoReceptionist arMain ext 1000", r.Resolved)
	}
	if r.Business == nil || r.Closed == nil {
		t.Fatalf("missing hours trees: %s", out)
	}

	// Business: key 1 -> queue -> members + overflow; key 2 (disabled) skipped; no-input -> user.
	if len(r.Business.Children) != 2 {
		t.Fatalf("business children = %d; want 2 (key 1 + no_input)\n%s", len(r.Business.Children), out)
	}
	queue := routingByKey(t, r.Business, "1")
	if queue.Type != "callQueue" || queue.ID != "cqSales" || queue.Action != "forward_to_call_queue" || queue.Name != "Sales Queue" {
		t.Fatalf("key 1 node = %+v; want callQueue cqSales forward_to_call_queue", queue)
	}
	if len(queue.Children) != 3 {
		t.Fatalf("queue children = %d; want 2 members + overflow", len(queue.Children))
	}
	m1, m2, overflow := queue.Children[0], queue.Children[1], queue.Children[2]
	if m1.Type != "user" || m1.ID != "u1" || m1.Action != "ring_member" {
		t.Fatalf("member 1 = %+v", m1)
	}
	if m2.ID != "u2" || m2.Action != "member_not_receiving_calls" {
		t.Fatalf("member 2 = %+v; want member_not_receiving_calls", m2)
	}
	if overflow.Type != "voicemail" || overflow.Action != "overflow_forward_to_voicemail" || overflow.Name != "Sales Queue" {
		t.Fatalf("overflow = %+v", overflow)
	}
	noInput := routingByKey(t, r.Business, "no_input")
	if noInput.Type != "user" || noInput.ID != "u1" || noInput.Action != "forward_to_user" || len(noInput.Children) != 0 {
		t.Fatalf("no_input = %+v", noInput)
	}

	// Closed: call handling routes straight to voicemail; the closed IVR is never fetched.
	if len(r.Closed.Children) != 1 {
		t.Fatalf("closed children = %d; want 1", len(r.Closed.Children))
	}
	vm := r.Closed.Children[0]
	if vm.Type != "voicemail" || vm.Action != "forward_to_voicemail" || vm.Name != "Main Line" || vm.ExtensionNumber != "1000" {
		t.Fatalf("closed child = %+v; want voicemail Main Line", vm)
	}
	if n := fake.countPrefix("/phone/auto_receptionists/arMain/ivr?hours_type=closed_hours"); n != 0 {
		t.Fatalf("closed IVR fetched %d times although closed routing is voicemail", n)
	}
	if n := fake.countPrefix("/phone/auto_receptionists/arMain/ivr?hours_type=business_hours"); n != 1 {
		t.Fatalf("business IVR fetched %d times; want 1. hits=%v", n, fake.hitList())
	}
	if r.Truncated || queue.Truncated {
		t.Fatalf("complete trace reported truncated: top=%v queue=%+v", r.Truncated, queue)
	}
	for _, n := range []*routingNode{r.Business, queue, r.Closed} {
		if n.Cycle || n.Error != "" {
			t.Fatalf("unexpected cycle/error on %s %s: %+v", n.Type, n.ID, n)
		}
	}
}

// AR A key 1 -> AR B, AR B key 1 -> AR A: the second visit is marked
// cycle:true and the walk terminates. Call handling 404s, so the trace falls
// back to the IVR and records the error on the node.
func TestRoutingTraceCycleTerminates(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": routingStatic(`{"phone_numbers":[{"id":"pn1","number":"+14155550123",
			"assignee":{"id":"arA","name":"Front Desk","extension_number":2000,"type":"autoReceptionist"}}]}`),
		"/phone/auto_receptionists/arA/ivr": routingStatic(`{"key_actions":[
			{"key":"1","action":6,"target":{"id":"arB","extension_id":"extB","extension_number":"2001","display_name":"Sub Menu"}}]}`),
		"/phone/auto_receptionists/arB/ivr": routingStatic(`{"key_actions":[
			{"key":"1","action":6,"target":{"id":"arA","extension_id":"extA","extension_number":"2000","display_name":"Front Desk"}},
			{"key":"9","action":300,"target":{"id":"arB","extension_id":"extB","extension_number":"2001","display_name":"Sub Menu"}}]}`),
		"/phone/auto_receptionists": routingStatic(`{"auto_receptionists":[
			{"id":"arA","name":"Front Desk","extension_id":"extA","extension_number":2000},
			{"id":"arB","name":"Sub Menu","extension_id":"extB","extension_number":2001}],"next_page_token":""}`),
	})

	out, stderr, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if err != nil {
		t.Fatalf("routing trace: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeTrace(t, out)
	for _, tree := range []*routingNode{r.Business, r.Closed} {
		if tree.Cycle {
			t.Fatalf("root marked cycle: %+v", tree)
		}
		if !strings.Contains(tree.Error, "call handling") {
			t.Fatalf("root error = %q; want the call handling 404 recorded", tree.Error)
		}
		b := routingByKey(t, tree, "1")
		if b.Type != "autoReceptionist" || b.ID != "arB" || b.Cycle {
			t.Fatalf("hop to B = %+v", b)
		}
		back := routingByKey(t, b, "1")
		if back.ID != "arA" || !back.Cycle || len(back.Children) != 0 {
			t.Fatalf("hop back to A = %+v; want cycle:true with no children", back)
		}
		vm := routingByKey(t, b, "9")
		if vm.Type != "voicemail" || vm.Action != "leave_voicemail_auto_receptionist" || vm.Cycle {
			t.Fatalf("B key 9 = %+v; want voicemail leaf", vm)
		}
	}
	// Two IVRs per hours type at most; the cycle never re-fetches.
	if n := fake.countPrefix("/phone/auto_receptionists/arA/ivr"); n != 2 {
		t.Fatalf("arA IVR fetched %d times; want 2 (one per hours type). hits=%v", n, fake.hitList())
	}
	if !strings.Contains(out, `"cycle": true`) && !strings.Contains(out, `"cycle":true`) {
		t.Fatalf("raw JSON lacks cycle:true:\n%s", out)
	}
}

func TestRoutingTraceResolvesExtensionToCallQueue(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/auto_receptionists": routingStatic(`{"auto_receptionists":[{"id":"arMain","name":"Main Line","extension_number":1000,"extension_id":"extMain"}],"next_page_token":""}`),
		"/phone/call_queues": func(q url.Values) (int, any) {
			if q.Get("keyword") != "1001" {
				return http.StatusOK, `{"call_queues":[]}`
			}
			return http.StatusOK, `{"call_queues":[{"id":"cqSales","name":"Sales Queue","extension_number":1001,"extension_id":"extSales","status":"active"}],"next_page_token":""}`
		},
		"/phone/call_queues/cqSales/members":                routingStatic(`{"call_queue_members":[]}`),
		"/phone/call_queues/cqSales/call_handling/settings": routingStatic(`{"call_handling_setting":{"overflow_action":"disconnect"}}`),
	})
	out, _, err := routingRun(t, newNovelRoutingTraceCmd, "--ext", "1001")
	if err != nil {
		t.Fatalf("routing trace --ext: %v", err)
	}
	r := routingDecodeTrace(t, out)
	if r.Resolved.Type != "callQueue" || r.Resolved.ID != "cqSales" {
		t.Fatalf("resolved = %+v; want callQueue cqSales", r.Resolved)
	}
	if len(r.Business.Children) != 1 || r.Business.Children[0].Type != "disconnect" || r.Business.Children[0].Action != "overflow_disconnect" {
		t.Fatalf("business children = %+v; want only overflow_disconnect (no members)", r.Business.Children)
	}
	if fake.countPrefix("/phone/users") != 0 {
		t.Fatalf("scan continued past the matching call queue: %v", fake.hitList())
	}
}

func TestRoutingTraceNotFoundAndInvalidInput(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": routingStatic(`{"phone_numbers":[],"next_page_token":""}`),
	})
	_, _, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if code := routingExitCode(err); code != 3 {
		t.Fatalf("unknown number exit = %d (%v); want 3", code, err)
	}

	before := len(fake.hitList())
	_, _, err = routingRun(t, newNovelRoutingTraceCmd, "__printing_press_invalid__")
	if code := routingExitCode(err); code != 2 {
		t.Fatalf("invalid input exit = %d (%v); want 2", code, err)
	}
	_, _, err = routingRun(t, newNovelRoutingTraceCmd, "+14155550123", "--max-depth", "0")
	if code := routingExitCode(err); code != 2 {
		t.Fatalf("--max-depth 0 exit = %d (%v); want 2", code, err)
	}
	if after := len(fake.hitList()); after != before {
		t.Fatalf("invalid input made %d API calls; want 0", after-before)
	}
}

// --max-pages 1 cuts the queue's member list and the extension index used to
// resolve an overflow target: both are surfaced as truncated, not dropped.
func TestRoutingTraceSurfacesPageTruncation(t *testing.T) {
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": routingStatic(`{"phone_numbers":[{"id":"pn1","number":"+14155550123",
			"assignee":{"id":"arMain","name":"Main Line","extension_number":1000,"type":"autoReceptionist"}}]}`),
		"/phone/auto_receptionists/arMain/call_handling/settings": func(q url.Values) (int, any) {
			if q.Get("hour_type") == "closed_hours" {
				return http.StatusOK, `{"call_handling_setting":{"routing_action":"forward_to_voicemail"}}`
			}
			return http.StatusOK, `{"call_handling_setting":{"routing_action":"forward_to_ivr"}}`
		},
		"/phone/auto_receptionists/arMain/ivr": routingStatic(`{"key_actions":[
			{"key":"1","action":7,"target":{"id":"cqSales","extension_id":"extSales","extension_number":"1001","display_name":"Sales Queue"}}]}`),
		"/phone/call_queues/cqSales/members": func(q url.Values) (int, any) {
			if q.Get("next_page_token") == "m2" {
				return http.StatusOK, `{"call_queue_members":[{"id":"u9","level":"user","name":"Page Two","receive_call":true}],"next_page_token":""}`
			}
			return http.StatusOK, `{"call_queue_members":[{"id":"u1","level":"user","name":"Jane Doe","receive_call":true}],"next_page_token":"m2"}`
		},
		"/phone/call_queues/cqSales/call_handling/settings": routingStatic(`{"call_handling_setting":{"overflow_action":"forward_to_extension",
			"overflow_action_setting":{"forward_to_extension":{"extension_id":"extOverflow","extension_number":1050,"extension_name":"Overflow Queue","extension_type":"callQueue"}}}}`),
		// The overflow queue sits on page 2 of the call queue list.
		"/phone/auto_receptionists": routingStatic(`{"auto_receptionists":[{"id":"arMain","name":"Main Line","extension_id":"extMain","extension_number":1000}],"next_page_token":"a2"}`),
		"/phone/call_queues": func(q url.Values) (int, any) {
			if q.Get("next_page_token") == "q2" {
				return http.StatusOK, `{"call_queues":[{"id":"cqOverflow","name":"Overflow Queue","extension_id":"extOverflow","extension_number":1050}],"next_page_token":""}`
			}
			return http.StatusOK, `{"call_queues":[{"id":"cqSales","name":"Sales Queue","extension_id":"extSales","extension_number":1001}],"next_page_token":"q2"}`
		},
	})

	out, stderr, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123", "--max-pages", "1")
	if err != nil {
		t.Fatalf("routing trace: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeTrace(t, out)
	if !r.Truncated || r.MaxPages != 1 {
		t.Fatalf("top-level truncated=%v max_pages=%d; want true and 1", r.Truncated, r.MaxPages)
	}
	queue := routingByKey(t, r.Business, "1")
	if !queue.Truncated || queue.TruncatedReason != "member_list_max_pages" {
		t.Fatalf("queue = truncated %v reason %q; want member_list_max_pages", queue.Truncated, queue.TruncatedReason)
	}
	if len(queue.Children) != 2 || queue.Children[0].ID != "u1" {
		t.Fatalf("queue children = %+v; want page-1 member u1 + overflow", queue.Children)
	}
	overflow := queue.Children[1]
	if overflow.Action != "overflow_forward_to_extension" || overflow.Type != "callQueue" || overflow.ExtensionID != "extOverflow" {
		t.Fatalf("overflow = %+v", overflow)
	}
	if !overflow.Truncated || overflow.TruncatedReason != "extension_index_max_pages" || overflow.ID != "" || !strings.Contains(overflow.Error, "--max-pages") {
		t.Fatalf("overflow = %+v; want unresolved, truncated extension_index_max_pages, error naming --max-pages", overflow)
	}

	// Same account with room to page: nothing is truncated and the overflow
	// queue resolves to its id.
	out, _, err = routingRun(t, newNovelRoutingTraceCmd, "+14155550123", "--max-pages", "5")
	if err != nil {
		t.Fatalf("routing trace --max-pages 5: %v", err)
	}
	r = routingDecodeTrace(t, out)
	queue = routingByKey(t, r.Business, "1")
	if queue.Truncated || len(queue.Children) != 3 {
		t.Fatalf("queue with full paging = truncated %v, %d children; want false, 3", queue.Truncated, len(queue.Children))
	}
	if ov := queue.Children[2]; ov.ID != "cqOverflow" || ov.Truncated {
		t.Fatalf("overflow with full paging = %+v; want resolved cqOverflow", ov)
	}
}

// An extension missing from a truncated resolution scan is still exit 3, but
// the message says the scan stopped at --max-pages instead of claiming absence.
func TestRoutingTraceNotFoundUnderTruncationSaysSo(t *testing.T) {
	empty := func(key string) routingFakeHandler { return routingStatic(`{"` + key + `":[],"next_page_token":""}`) }
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/auto_receptionists": routingStatic(`{"auto_receptionists":[{"id":"arMain","name":"Main Line","extension_number":1000}],"next_page_token":"a2"}`),
		"/phone/call_queues":        empty("call_queues"),
		"/phone/users":              empty("users"),
		"/phone/common_areas":       empty("common_areas"),
		"/phone/shared_line_groups": empty("shared_line_groups"),
	})
	_, _, err := routingRun(t, newNovelRoutingTraceCmd, "--ext", "1999", "--max-pages", "1")
	if code := routingExitCode(err); code != 3 {
		t.Fatalf("exit = %d (%v); want 3", code, err)
	}
	if !strings.Contains(err.Error(), "--max-pages") || !strings.Contains(err.Error(), "auto_receptionists") {
		t.Fatalf("error = %q; want it to name the truncated list and --max-pages", err)
	}
}

// NoCache: a member added between two runs shows up on the second run in the
// same home (same response cache directory).
func TestRoutingTraceSeesChangeImmediately(t *testing.T) {
	var members atomic.Value
	members.Store(`{"call_queue_members":[]}`)
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": routingStatic(`{"phone_numbers":[{"id":"pn1","number":"+14155550123",
			"assignee":{"id":"cqSales","name":"Sales Queue","extension_number":1001,"type":"callQueue"}}]}`),
		"/phone/call_queues/cqSales/members": func(url.Values) (int, any) { return http.StatusOK, members.Load().(string) },
	})
	out, _, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if n := len(routingDecodeTrace(t, out).Business.Children); n != 0 {
		t.Fatalf("run 1 members = %d; want 0", n)
	}
	members.Store(`{"call_queue_members":[{"id":"u1","level":"user","name":"Jane Doe","receive_call":true}]}`)
	out, _, err = routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if n := len(routingDecodeTrace(t, out).Business.Children); n != 1 {
		t.Fatalf("run 2 members = %d; want 1 (a cached response hid the change)", n)
	}
}

// Stable identifiers are authoritative for IVR targets: a target whose id is
// gone from a complete list is not followed (even though its extension number
// now belongs to another queue), a target whose id and extension_id name
// different queues is not followed, an extension_id-only target resolves by
// extension_id, and only a target with no stable id falls back to the number.
func TestRoutingTraceStableIDsAreAuthoritative(t *testing.T) {
	fake := routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": routingStatic(`{"phone_numbers":[{"id":"pn1","number":"+14155550123",
			"assignee":{"id":"arMain","name":"Main Line","extension_number":1000,"type":"autoReceptionist"}}]}`),
		"/phone/auto_receptionists/arMain/call_handling/settings": routingStatic(`{"call_handling_setting":{"routing_action":"forward_to_ivr"}}`),
		"/phone/auto_receptionists/arMain/ivr": func(q url.Values) (int, any) {
			if q.Get("hours_type") != "business_hours" {
				return http.StatusOK, `{"key_actions":[]}`
			}
			return http.StatusOK, `{"key_actions":[
				{"key":"1","action":7,"target":{"id":"cqGone","extension_id":"extGone","extension_number":"1001","display_name":"Old Sales"}},
				{"key":"2","action":7,"target":{"id":"cqSupport","extension_id":"extSales","extension_number":"1003","display_name":"Support Queue"}},
				{"key":"3","action":7,"target":{"extension_id":"extSales","display_name":"Sales Queue"}},
				{"key":"4","action":7,"target":{"extension_number":"1001"}}]}`
		},
		"/phone/call_queues": routingStatic(`{"call_queues":[
			{"id":"cqSales","name":"Sales Queue","extension_id":"extSales","extension_number":1001},
			{"id":"cqSupport","name":"Support Queue","extension_id":"extSupport","extension_number":1003}],"next_page_token":""}`),
		"/phone/call_queues/cqSales/members":                routingStatic(`{"call_queue_members":[{"id":"u1","level":"user","name":"Jane Doe","receive_call":true}]}`),
		"/phone/call_queues/cqSales/call_handling/settings": routingStatic(`{"call_handling_setting":{"overflow_action":"disconnect"}}`),
	})
	out, stderr, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if err != nil {
		t.Fatalf("routing trace: %v\nstderr=%s", err, stderr)
	}
	r := routingDecodeTrace(t, out)

	gone := routingByKey(t, r.Business, "1")
	if gone.ID != "cqGone" || len(gone.Children) != 0 || !strings.Contains(gone.Error, "not among the account's callQueues") {
		t.Fatalf("key 1 = %+v; want dead cqGone, not followed, not resolved to cqSales by extension 1001", gone)
	}
	conflict := routingByKey(t, r.Business, "2")
	if len(conflict.Children) != 0 || !strings.Contains(conflict.Error, "conflicting target identifiers") || !strings.Contains(conflict.Error, "extSupport") {
		t.Fatalf("key 2 = %+v; want an id/extension_id conflict, not followed", conflict)
	}
	byExtID := routingByKey(t, r.Business, "3")
	if byExtID.ID != "cqSales" || byExtID.Error != "" || len(byExtID.Children) != 2 {
		t.Fatalf("key 3 = %+v; want resolved by extension_id to cqSales and expanded", byExtID)
	}
	byNumber := routingByKey(t, r.Business, "4")
	if byNumber.ID != "cqSales" || byNumber.Name != "Sales Queue" || byNumber.Error != "" {
		t.Fatalf("key 4 = %+v; want number-only target resolved to cqSales", byNumber)
	}
	for _, path := range []string{"/phone/call_queues/cqGone/", "/phone/call_queues/cqSupport/"} {
		if n := fake.countPrefix(path); n != 0 {
			t.Fatalf("%s was fetched %d times; a dead or conflicting target must not be followed", path, n)
		}
	}
	if n := fake.countPrefix("/phone/call_queues?"); n != 1 {
		t.Fatalf("call queue list walked %d times; want once per run", n)
	}
}

// A member list whose cursor repeats leaves an error on the queue node (the
// fetched members are still shown) instead of looking like a complete list.
func TestRoutingTraceRepeatedMemberCursorIsAnError(t *testing.T) {
	routingStartFake(t, map[string]routingFakeHandler{
		"/phone/numbers": routingStatic(`{"phone_numbers":[{"id":"pn1","number":"+14155550123",
			"assignee":{"id":"cqSales","name":"Sales Queue","extension_number":1001,"type":"callQueue"}}]}`),
		"/phone/call_queues/cqSales/members":                routingStatic(`{"call_queue_members":[{"id":"u1","level":"user","name":"Jane Doe","receive_call":true}],"next_page_token":"m1"}`),
		"/phone/call_queues/cqSales/call_handling/settings": routingStatic(`{"call_handling_setting":{"overflow_action":"disconnect"}}`),
	})
	out, _, err := routingRun(t, newNovelRoutingTraceCmd, "+14155550123")
	if err != nil {
		t.Fatalf("routing trace: %v", err)
	}
	q := routingDecodeTrace(t, out).Business
	if !strings.Contains(q.Error, "pagination loop") || len(q.Children) < 2 || q.Children[0].ID != "u1" {
		t.Fatalf("queue = error %q, %d children; want a pagination-loop error with the fetched members kept", q.Error, len(q.Children))
	}
}
