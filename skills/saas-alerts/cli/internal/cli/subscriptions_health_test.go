// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
)

// TestNovelSubscriptionsHealthHelpWires smoke-tests that the subscriptions health command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelSubscriptionsHealthHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"subscriptions", "health", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("subscriptions health --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "health"} {
		if !strings.Contains(help, want) {
			t.Fatalf("subscriptions health --help missing %q in output:\n%s", want, help)
		}
	}
}

func rqtWebhookFixtures(api *rqtAPI) {
	customers := make([]any, 0, 51)
	for i := 0; i < 51; i++ {
		customers = append(customers, fmt.Sprintf("CUST_%02d", i))
	}
	api.subs = []any{
		map[string]any{"channelId": "CH_S1", "url": "https://hooks.example.com/saas/feed", "partnerId": "P1", "enabled": true,
			"alertStatuses": []any{"critical"}, "created": "2026-10-01 10:00:00.000"},
		map[string]any{"channelId": "CH_S2", "url": "https://other.example.net/x", "partnerId": "P1", "enabled": true,
			"eventTypes": []any{nil}, "created": "2026-10-01 10:00:00.000"},
		map[string]any{"channelId": "CH_S3", "url": "https://hooks.example.com/saas/three", "partnerId": "P1", "enabled": true,
			"alertStatuses": []any{"low"}, "eventTypes": []any{"login.success"}, "customerIds": customers, "created": "2026-10-01 10:00:00.000"},
	}
	api.disabledSubs = []any{
		map[string]any{"channelId": "CH_S4", "url": "https://hooks.example.com/saas/four", "partnerId": "P1", "enabled": false,
			"alertStatuses": []any{"critical", "medium"}, "customerIds": []any{"C1"}, "skipSuppressed": true,
			"disabledTime": "2026-10-13 21:00:00.000", "failedAttempts": []any{map[string]any{"message": "HTTP 502 from receiver", "time": 1}},
			"created": "2026-10-01 10:00:00.000"},
		map[string]any{"channelId": "CH_S5", "url": "https://hooks.example.com/saas/five", "partnerId": "P1", "enabled": false,
			"created": "2026-10-01 10:00:00.000"},
		map[string]any{"channelId": "CH_S6", "url": "https://hooks.example.com/saas/six", "partnerId": "P1", "enabled": false,
			"eventTypes": []any{"login.success"}, "created": "2026-10-01 10:00:00.000"},
		map[string]any{"channelId": "CH_S7", "url": "https://hooks.example.com/saas/seven", "partnerId": "P1", "enabled": false,
			"alertStatuses": []any{"critical"}, "created": "2026-10-01 10:00:00.000"},
		map[string]any{"channelId": "CH_S8", "url": "https://evil.example.org/hook", "partnerId": "P1", "enabled": false,
			"alertStatuses": []any{"medium"}, "created": "2026-10-01 10:00:00.000"},
	}
	api.domains = []any{map[string]any{"id": "D1", "domain": "hooks.example.com/saas", "partnerId": "P1", "created": "2026-10-01 10:00:00.000"}}
	api.queue = []any{
		map[string]any{"id": "Q1", "partnerId": "P1", "customerId": "C1", "time": "2026-10-14 10:00:00.000", "unsentChannelIds": []any{"CH_S4"}},
		map[string]any{"id": "Q2", "partnerId": "P1", "customerId": "C1", "time": "2026-10-14 10:05:00.000", "unsentChannelIds": []any{"CH_S4", "CH_UNKNOWN"}},
	}
}

func rqtRowsByChannel(res shResult) map[string]shRow {
	out := map[string]shRow{}
	for _, r := range res.Subscriptions {
		out[r.ChannelID] = r
	}
	return out
}

func TestSubscriptionsHealthLintFlags(t *testing.T) {
	api := newRQTAPI(t)
	rqtWebhookFixtures(api)
	rqtEnv(t, api)
	out, stderr, err := rqtRun(t, "subscriptions", "health", "--json")
	if err != nil {
		t.Fatalf("subscriptions health: %v\nstderr: %s", err, stderr)
	}
	res := rqtDecode[shResult](t, out)
	want := map[string]string{
		"CH_S1": "",
		"CH_S2": "FILTERS_EMPTY,UNVERIFIED_DOMAIN",
		"CH_S3": "FILTERS_BOTH,CUSTOMER_IDS_OVER_50",
		"CH_S4": "DISABLED,QUEUE_BACKLOG",
		"CH_S5": "FILTERS_EMPTY,DISABLED",
		"CH_S6": "DISABLED",
		"CH_S7": "DISABLED",
		"CH_S8": "DISABLED,UNVERIFIED_DOMAIN",
	}
	rows := rqtRowsByChannel(res)
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d: %s", len(rows), len(want), out)
	}
	for ch, flags := range want {
		if got := strings.Join(rows[ch].Flags, ","); got != flags {
			t.Errorf("%s flags = %q, want %q", ch, got, flags)
		}
	}
	s4 := rows["CH_S4"]
	if s4.Enabled || *s4.UnsentQueueDepth != 2 || !*s4.DomainVerified || s4.Domain != "hooks.example.com/saas" ||
		s4.Filters.AlertStatuses != 2 || s4.Filters.CustomerIDs != 1 || s4.FailedAttempts != 1 || s4.LastFailureLength != len("HTTP 502 from receiver") ||
		s4.DisabledTime != "2026-10-13 21:00:00.000" {
		t.Fatalf("CH_S4 row wrong: %+v", s4)
	}
	if rows["CH_S2"].Filters.EventTypes != 0 {
		t.Fatalf("a [null] eventTypes list must count as empty: %+v", rows["CH_S2"].Filters)
	}
	if rows["CH_S3"].Filters.CustomerIDs != 51 || *rows["CH_S1"].UnsentQueueDepth != 0 {
		t.Fatalf("counts wrong: S3=%+v S1 depth=%d", rows["CH_S3"].Filters, *rows["CH_S1"].UnsentQueueDepth)
	}
	sum := res.Summary
	if sum.Total != 8 || sum.Enabled != 3 || sum.Disabled != 5 || sum.Backlog != 1 || sum.UnsentPacks != 2 || sum.Flagged != 7 {
		t.Fatalf("summary = %+v", sum)
	}
	if res.Subscriptions[0].Enabled {
		t.Fatalf("disabled subscriptions should sort first: %+v", res.Subscriptions[0])
	}
	if res.Plan != nil || res.Executed != nil || res.QueueSource != "queue" {
		t.Fatalf("plain health must not plan: plan=%v executed=%v queue_source=%s", res.Plan, res.Executed, res.QueueSource)
	}
	if len(api.writes()) != 0 {
		t.Fatalf("health wrote: %+v", api.writes())
	}
}

func TestSubscriptionsHealthReenablePreviewAndConfirm(t *testing.T) {
	api := newRQTAPI(t)
	rqtWebhookFixtures(api)
	rqtEnv(t, api)

	out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--json")
	if err != nil {
		t.Fatalf("health --reenable: %v", err)
	}
	if w := api.writes(); len(w) != 0 {
		t.Fatalf("--reenable without --confirm wrote: %+v", w)
	}
	res := rqtDecode[shResult](t, out)
	if res.Plan == nil || res.Executed != nil || !strings.Contains(res.Note, "--confirm") {
		t.Fatalf("preview plan missing: %s", out)
	}
	plan := map[string]shPlanItem{}
	for _, p := range *res.Plan {
		plan[p.ChannelID] = p
	}
	if len(plan) != 5 {
		t.Fatalf("plan should cover the 5 disabled subscriptions, got %d", len(plan))
	}
	if p := plan["CH_S5"]; p.Planned || !strings.HasPrefix(p.Reason, "FILTERS_EMPTY") || len(p.Requests) != 0 {
		t.Fatalf("CH_S5 must not be planned (FILTERS_EMPTY): %+v", p)
	}
	if p := plan["CH_S8"]; p.Planned || !strings.HasPrefix(p.Reason, "UNVERIFIED_DOMAIN") {
		t.Fatalf("CH_S8 must not be planned (UNVERIFIED_DOMAIN): %+v", p)
	}
	s4 := plan["CH_S4"]
	if !s4.Planned || len(s4.Requests) != 2 {
		t.Fatalf("CH_S4 plan wrong: %+v", s4)
	}
	patch, _ := json.Marshal(s4.Requests[0].Body)
	if s4.Requests[0].Method != "PATCH" || s4.Requests[0].Path != "/outgoingWebhookApi/api/v1/subscriptions/CH_S4" ||
		string(patch) != `{"alertStatuses":["critical","medium"],"customerIds":["C1"],"enabled":true,"skipSuppressed":true}` {
		t.Fatalf("CH_S4 PATCH wrong: %s %s %s", s4.Requests[0].Method, s4.Requests[0].Path, patch)
	}
	test, _ := json.Marshal(s4.Requests[1].Body)
	if s4.Requests[1].Path != "/outgoingWebhookApi/api/v1/misc/sendTestEvent" || string(test) != `{"alertStatus":"critical","partnerId":"P1"}` {
		t.Fatalf("CH_S4 test event wrong: %s", test)
	}
	s6 := plan["CH_S6"]
	if p6, _ := json.Marshal(s6.Requests[0].Body); string(p6) != `{"enabled":true,"eventTypes":["login.success"]}` {
		t.Fatalf("CH_S6 PATCH = %s", p6)
	}
	if t6, _ := json.Marshal(s6.Requests[1].Body); string(t6) != `{"jointType":"login.success","partnerId":"P1"}` {
		t.Fatalf("CH_S6 test event = %s", t6)
	}
	if s7 := plan["CH_S7"]; !s7.Planned || len(s7.Requests) != 1 || !strings.Contains(s7.Reason, "CH_S4") {
		t.Fatalf("CH_S7 should reuse CH_S4's identical test event: %+v", s7)
	}

	out, _, err = rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
	if err != nil {
		t.Fatalf("health --reenable --confirm: %v\n%s", err, out)
	}
	got := make([]string, 0)
	for _, c := range api.writes() {
		got = append(got, c.Method+" "+strings.TrimPrefix(c.Path, "/outgoingWebhookApi/api/v1"))
	}
	// Every re-enable PATCH first, then one test event per distinct filter.
	want := []string{
		"PATCH /subscriptions/CH_S4", "PATCH /subscriptions/CH_S6", "PATCH /subscriptions/CH_S7",
		"POST /misc/sendTestEvent", "POST /misc/sendTestEvent",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("writes = %v, want %v", got, want)
	}
	res = rqtDecode[shResult](t, out)
	if res.Executed == nil || len(*res.Executed) != 5 {
		t.Fatalf("executed = %v", res.Executed)
	}
	for _, e := range *res.Executed {
		if !e.OK {
			t.Fatalf("execution failed: %+v", e)
		}
	}

	_, _, err = rqtRun(t, "subscriptions", "health", "--confirm")
	if rqtExit(err) != 2 {
		t.Fatalf("--confirm without --reenable: exit=%d err=%v", rqtExit(err), err)
	}
}

func TestSubscriptionsHealthPerChannelQueueAndDomainOutage(t *testing.T) {
	api := newRQTAPI(t)
	rqtWebhookFixtures(api)
	// Queue items without unsentChannelIds force the per-channel endpoint.
	api.queue = []any{map[string]any{"id": "Q1", "partnerId": "P1", "time": "2026-10-14 10:00:00.000"}}
	api.channelQueue["CH_S4"] = []any{map[string]any{"id": "Q1"}, map[string]any{"id": "Q2"}, map[string]any{"id": "Q3"}}
	api.domainsCode = http.StatusForbidden
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--json")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	res := rqtDecode[shResult](t, out)
	rows := rqtRowsByChannel(res)
	if res.QueueSource != "queue/channels" || *rows["CH_S4"].UnsentQueueDepth != 3 || *rows["CH_S1"].UnsentQueueDepth != 0 {
		t.Fatalf("per-channel depths wrong: source=%s S4=%v", res.QueueSource, rows["CH_S4"].UnsentQueueDepth)
	}
	if n := len(api.callsMatching("GET", "/outgoingWebhookApi/api/v1/queue/channels/")); n != 8 {
		t.Fatalf("per-channel queue calls = %d, want 8", n)
	}
	if rows["CH_S8"].DomainVerified != nil || rows["CH_S8"].hasFlag(shFlagUnverified) {
		t.Fatalf("with /domains unavailable, verification must be unknown, not failed: %+v", rows["CH_S8"])
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "domains unavailable") {
		t.Fatalf("domain outage warning missing: %v", res.Warnings)
	}
	for _, p := range *res.Plan {
		if p.ChannelID == "CH_S4" && (p.Planned || !strings.Contains(p.Reason, "could not be read")) {
			t.Fatalf("re-enable must wait for domain verification: %+v", p)
		}
	}
}

func TestSubscriptionsHealthEmptyAccount(t *testing.T) {
	api := newRQTAPI(t)
	api.subs, api.disabledSubs, api.domains, api.queue = []any{}, []any{}, []any{}, []any{}
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "subscriptions", "health", "--json")
	if err != nil {
		t.Fatalf("health on empty account: %v", err)
	}
	res := rqtDecode[shResult](t, out)
	if res.Subscriptions == nil || len(res.Subscriptions) != 0 || res.Summary.Total != 0 {
		t.Fatalf("empty account should return an empty list: %s", out)
	}
}

func rqtTestEventResults(res shResult) []shExecuted {
	out := make([]shExecuted, 0)
	for _, e := range *res.Executed {
		if e.Path == "/outgoingWebhookApi/api/v1/misc/sendTestEvent" {
			out = append(out, e)
		}
	}
	return out
}

func TestSubscriptionsHealthTestEventResponseShapes(t *testing.T) {
	cases := []struct {
		name      string
		resp      any
		ok        bool
		errorText string
		channel   string
	}{
		{name: "true", resp: true, ok: true},
		{name: "false", resp: false, errorText: "answered false"},
		{name: "plain string", resp: "Test event sent", ok: true},
		{name: "error string", resp: "Error: receiver unreachable", errorText: "error string of 27 characters"},
		{name: "empty error list", resp: []any{}, ok: true},
		{name: "IWhError list", resp: []any{map[string]any{"channelId": "CH_S4", "message": "receiver returned 502"}},
			errorText: "delivery failed for channel(s) CH_S4", channel: "CH_S4"},
		{name: "unexpected object", resp: map[string]any{"weird": 1}, errorText: "unexpected test event response"},
		{name: "Not Authorized", resp: "Not Authorized", errorText: "error string"},
		{name: "Access denied", resp: "Access denied", errorText: "error string"},
		{name: "Forbidden", resp: "Forbidden", errorText: "error string"},
		{name: "invalid", resp: "invalid", errorText: "error string"},
		{name: "rejected", resp: "rejected", errorText: "error string"},
		{name: "denied", resp: "denied", errorText: "error string"},
		{name: "not sent", resp: "Test event not sent", errorText: "error string"},
		{name: "unrecognized string", resp: "queued for later", errorText: "outcome unknown"},
		{name: "negated without a failure word", resp: "no subscribers sent", errorText: "outcome unknown"},
		{name: "wasn't sent", resp: "Test event wasn't sent", errorText: "outcome unknown"},
		{name: "couldn't be delivered", resp: "Test event couldn't be delivered", errorText: "outcome unknown"},
		{name: "Success: false", resp: "Success: false", errorText: "outcome unknown"},
		{name: "0 events sent", resp: "0 events sent", errorText: "outcome unknown"},
		{name: "true string", resp: "true", ok: true},
		{name: "test event delivered", resp: "Test event delivered", ok: true},
		{name: "sent", resp: "sent", ok: true},
		{name: "Delivered.", resp: "Delivered.", ok: true},
		{name: "OK", resp: "OK", ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtWebhookFixtures(api)
			api.testEventResp = tc.resp
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
			if tc.ok != (err == nil) {
				t.Fatalf("exit err=%v, want ok=%v\n%s", err, tc.ok, out)
			}
			events := rqtTestEventResults(rqtDecode[shResult](t, out))
			if len(events) != 2 {
				t.Fatalf("test events = %d, want 2", len(events))
			}
			for _, e := range events {
				if e.OK != tc.ok || (tc.errorText != "" && !strings.Contains(e.Error, tc.errorText)) {
					t.Fatalf("test event result = %+v, want ok=%v error~%q", e, tc.ok, tc.errorText)
				}
				if tc.channel != "" && (len(e.DeliveryErrors) != 1 || e.DeliveryErrors[0].ChannelID != tc.channel) {
					t.Fatalf("delivery error not attributed to %s: %+v", tc.channel, e.DeliveryErrors)
				}
			}
		})
	}
}

func TestSubscriptionsHealthFailedPatchStillSendsSharedTestEvent(t *testing.T) {
	api := newRQTAPI(t)
	rqtWebhookFixtures(api)
	api.patchFail["CH_S4"] = http.StatusBadRequest
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
	if rqtExit(err) == 0 {
		t.Fatalf("a failed PATCH must exit non-zero:\n%s", out)
	}
	got := make([]string, 0)
	for _, c := range api.writes() {
		got = append(got, c.Method+" "+strings.TrimPrefix(c.Path, "/outgoingWebhookApi/api/v1"))
	}
	want := "PATCH /subscriptions/CH_S4|PATCH /subscriptions/CH_S6|PATCH /subscriptions/CH_S7|POST /misc/sendTestEvent|POST /misc/sendTestEvent"
	if strings.Join(got, "|") != want {
		t.Fatalf("writes = %v, want %s", got, want)
	}
	events := rqtTestEventResults(rqtDecode[shResult](t, out))
	channels := make([]string, 0)
	for _, e := range events {
		channels = append(channels, strings.Join(e.Channels, "+"))
	}
	// CH_S4's PATCH failed, so the critical test event goes out for CH_S7 only.
	if strings.Join(channels, ",") != "CH_S6,CH_S7" && strings.Join(channels, ",") != "CH_S7,CH_S6" {
		t.Fatalf("test event channels = %v, want CH_S7 (critical) and CH_S6 (login.success)", channels)
	}
	for _, c := range api.callsMatching("POST", "/outgoingWebhookApi/api/v1/misc/sendTestEvent") {
		if strings.Contains(c.Body, "critical") {
			return
		}
	}
	t.Fatalf("the critical test event was dropped with CH_S4's failed PATCH")
}

func TestSubscriptionsHealthHelpStatesReenableRules(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := rqtRun(t, "subscriptions", "health", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"Read-only by default", "--reenable --confirm", "verified domain"} {
		if !strings.Contains(out, want) {
			t.Fatalf("subscriptions health --help missing %q", want)
		}
	}
}

func TestSubscriptionsHealthNullListIsAnError(t *testing.T) {
	api := newRQTAPI(t)
	rqtWebhookFixtures(api)
	api.disabledSubs = rqtNull
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "subscriptions", "health", "--json")
	if err == nil || !strings.Contains(err.Error(), "expected an array, got null") {
		t.Fatalf("a null disabled-subscriptions body must fail, got err=%v out=%s", err, out)
	}
	if strings.Contains(out, `"subscriptions"`) {
		t.Fatalf("a failed read must not print a health report: %s", out)
	}
}

func TestSubscriptionsHealthNullCollectionsAreUnknown(t *testing.T) {
	t.Run("null /queue leaves every depth unknown", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.queue = rqtNull
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--json")
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		res := rqtDecode[shResult](t, out)
		for _, r := range res.Subscriptions {
			if r.UnsentQueueDepth != nil || !r.hasFlag(shFlagQueueUnknown) || r.hasFlag(shFlagBacklog) {
				t.Fatalf("%s: a null queue must be unknown, not empty: depth=%v flags=%v", r.ChannelID, r.UnsentQueueDepth, r.Flags)
			}
		}
		if res.Summary.QueueUnknown != 8 || res.Summary.Backlog != 0 || len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "queue unavailable") {
			t.Fatalf("summary/warnings wrong: %+v %v", res.Summary, res.Warnings)
		}
	})

	t.Run("null /queue/channels/{id} leaves that channel unknown", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.queue = []any{map[string]any{"id": "Q1", "partnerId": "P1", "time": "2026-10-14 10:00:00.000"}}
		api.channelQueue["CH_S4"] = rqtNull
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--json")
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		res := rqtDecode[shResult](t, out)
		rows := rqtRowsByChannel(res)
		if s4 := rows["CH_S4"]; s4.UnsentQueueDepth != nil || !s4.hasFlag(shFlagQueueUnknown) {
			t.Fatalf("CH_S4 null queue must be unknown: depth=%v flags=%v", s4.UnsentQueueDepth, s4.Flags)
		}
		if s1 := rows["CH_S1"]; s1.UnsentQueueDepth == nil || *s1.UnsentQueueDepth != 0 || s1.hasFlag(shFlagQueueUnknown) {
			t.Fatalf("CH_S1 empty queue ([]) must read as 0: %+v", s1)
		}
		if res.Summary.QueueUnknown != 1 || !strings.Contains(strings.Join(res.Warnings, " "), "channel CH_S4 is not an array") {
			t.Fatalf("summary/warnings wrong: %+v %v", res.Summary, res.Warnings)
		}
	})

	t.Run("null /domains leaves verification unknown and blocks re-enable", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.domains = rqtNull
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
		if err != nil {
			t.Fatalf("health: %v\n%s", err, out)
		}
		res := rqtDecode[shResult](t, out)
		for _, r := range res.Subscriptions {
			if r.DomainVerified != nil || !r.hasFlag(shFlagDomainUnknown) || r.hasFlag(shFlagUnverified) {
				t.Fatalf("%s: a null domain list must be unknown, not unverified: verified=%v flags=%v", r.ChannelID, r.DomainVerified, r.Flags)
			}
		}
		for _, p := range *res.Plan {
			if p.Planned {
				t.Fatalf("re-enable planned with unknown domain verification: %+v", p)
			}
			if p.ChannelID == "CH_S4" && !strings.HasPrefix(p.Reason, "DOMAIN_UNKNOWN") {
				t.Fatalf("CH_S4 reason = %q", p.Reason)
			}
		}
		if w := api.writes(); len(w) != 0 || res.Summary.DomainUnknown != 8 {
			t.Fatalf("unknown domains must send nothing: writes=%v summary=%+v", w, res.Summary)
		}
	})
}

func TestSubscriptionsHealthInvalidMembersMakeCollectionsIncomplete(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(a *rqtAPI)
		collection string
		valid      int
		flag       string // a flag every row must then carry, if any
	}{
		{name: "null member in subscriptions", collection: "subscriptions", valid: 3,
			mutate: func(a *rqtAPI) { a.subs = append(a.subs.([]any), nil) }},
		{name: "error-shaped member in subscriptions", collection: "subscriptions", valid: 3,
			mutate: func(a *rqtAPI) { a.subs = append(a.subs.([]any), map[string]any{"message": "backend failure"}) }},
		{name: "null member in disabled subscriptions", collection: "subscriptions/disabled", valid: 5,
			mutate: func(a *rqtAPI) { a.disabledSubs = append(a.disabledSubs.([]any), nil) }},
		{name: "error-shaped member in disabled subscriptions", collection: "subscriptions/disabled", valid: 5,
			mutate: func(a *rqtAPI) {
				a.disabledSubs = append(a.disabledSubs.([]any), map[string]any{"message": "backend failure"})
			}},
		{name: "null member in domains", collection: "domains", valid: 1, flag: shFlagDomainUnknown,
			mutate: func(a *rqtAPI) { a.domains = append(a.domains.([]any), nil) }},
		{name: "null member in queue", collection: "queue", valid: 2, flag: shFlagQueueUnknown,
			mutate: func(a *rqtAPI) { a.queue = append(a.queue.([]any), nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtWebhookFixtures(api)
			tc.mutate(api)
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "subscriptions", "health", "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("an invalid member must fail the run:\n%s", out)
			}
			res := rqtDecode[shResult](t, out)
			if len(res.CollectionIncomplete) != 1 {
				t.Fatalf("collection_incomplete = %+v", res.CollectionIncomplete)
			}
			inc := res.CollectionIncomplete[0]
			if inc.Collection != tc.collection || inc.Valid != tc.valid || inc.Invalid != 1 {
				t.Fatalf("collection_incomplete = %+v, want %s valid=%d invalid=1", inc, tc.collection, tc.valid)
			}
			if len(res.Subscriptions) == 0 {
				t.Fatalf("valid members must still be reported, not a healthy empty result")
			}
			if tc.flag != "" {
				for _, r := range res.Subscriptions {
					if !r.hasFlag(tc.flag) {
						t.Fatalf("%s: want %s with an incomplete %s list: %v", r.ChannelID, tc.flag, tc.collection, r.Flags)
					}
				}
			}
		})
	}

	t.Run("only invalid members is never a healthy empty result", func(t *testing.T) {
		api := newRQTAPI(t)
		api.subs, api.disabledSubs = []any{nil}, []any{map[string]any{"message": "backend failure"}}
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("must fail:\n%s", out)
		}
		if res := rqtDecode[shResult](t, out); len(res.CollectionIncomplete) != 2 || res.Summary.Total != 0 {
			t.Fatalf("result = %s", out)
		}
	})
}

func TestSubscriptionsHealthReenablePatchAnswers(t *testing.T) {
	cases := []struct {
		name     string
		body     any // nil: the fake's real answer (the updated subscription)
		applies  bool
		verified bool
		reads    bool // a verification re-read is expected
	}{
		{name: "enabled:false, unchanged", body: map[string]any{"channelId": "CH_S4", "enabled": false}, reads: true},
		{name: "malformed answer, unchanged", body: map[string]any{"message": "update rejected"}, reads: true},
		{name: "contradictory answer (enabled:true with an error), unchanged", body: map[string]any{"channelId": "CH_S4", "enabled": true, "error": "not saved"}, reads: true},
		{name: "another channel's answer, unchanged", body: map[string]any{"channelId": "CH_OTHER", "enabled": true}, reads: true},
		{name: "malformed answer but the change applied", body: map[string]any{"message": "update queued"}, applies: true, verified: true, reads: true},
		{name: "documented answer", verified: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtWebhookFixtures(api)
			if tc.body != nil {
				api.patchBody["CH_S4"] = tc.body
				api.patchApplies["CH_S4"] = tc.applies
			}
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
			res := rqtDecode[shResult](t, out)
			if tc.verified != (err == nil) {
				t.Fatalf("exit err=%v, want success=%v\n%s", err, tc.verified, out)
			}
			if n := len(api.callsMatching("PATCH", "/outgoingWebhookApi/api/v1/subscriptions/CH_S4")); n != 1 {
				t.Fatalf("CH_S4 PATCHes = %d, want exactly 1 (never replayed)", n)
			}
			var s4 shExecuted
			criticalChannels := ""
			for _, e := range *res.Executed {
				if e.Method == "PATCH" && e.ChannelID == "CH_S4" {
					s4 = e
				}
				if e.Method == "POST" && strings.Contains(strings.Join(e.Channels, ","), "CH_S7") {
					criticalChannels = strings.Join(e.Channels, ",")
				}
			}
			wantOutcome := map[bool]string{true: "reenabled", false: "reenable_unverified"}[tc.verified]
			if s4.Outcome != wantOutcome || s4.OK != tc.verified {
				t.Fatalf("CH_S4 PATCH = %+v, want outcome %s", s4, wantOutcome)
			}
			// CH_S4 shares its test event with CH_S7: it is sent for CH_S7
			// either way, and for CH_S4 only once its re-enable is established.
			wantChannels := map[bool]string{true: "CH_S4,CH_S7", false: "CH_S7"}[tc.verified]
			if criticalChannels != wantChannels {
				t.Fatalf("critical test event channels = %q, want %q", criticalChannels, wantChannels)
			}
			reads := len(api.callsMatching("GET", "/outgoingWebhookApi/api/v1/subscriptions/disabled"))
			if tc.reads != (reads > 1) {
				t.Fatalf("disabled-list reads = %d, want a verification re-read: %v", reads, tc.reads)
			}
		})
	}
}

func TestSubscriptionsHealthReenableVerificationReadIncomplete(t *testing.T) {
	api := newRQTAPI(t)
	rqtWebhookFixtures(api)
	// The PATCH answer does not prove anything, so the lists are re-read. By
	// then the disabled list holds an unreadable CH_S4 entry while the active
	// list claims CH_S4 is enabled.
	api.patchBody["CH_S4"] = map[string]any{"message": "update queued"}
	api.onPatch = func(a *rqtAPI, ch string) {
		if ch != "CH_S4" {
			return
		}
		kept := make([]any, 0)
		for _, item := range a.disabledSubs.([]any) {
			if sub, _ := item.(map[string]any); sub["channelId"] != "CH_S4" {
				kept = append(kept, item)
			}
		}
		a.disabledSubs = append(kept, map[string]any{"channelId": "CH_S4", "enabled": "false"})
		a.subs = append(a.subs.([]any), map[string]any{"channelId": "CH_S4", "url": "https://hooks.example.com/saas/four",
			"partnerId": "P1", "enabled": true, "alertStatuses": []any{"critical", "medium"}})
	}
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
	if rqtExit(err) == 0 {
		t.Fatalf("an incomplete verification read must fail the run:\n%s", out)
	}
	res := rqtDecode[shResult](t, out)
	for _, e := range *res.Executed {
		if e.Method == "PATCH" && e.ChannelID == "CH_S4" && (e.OK || e.Outcome != "reenable_unverified" || e.Verification != "read_failed") {
			t.Fatalf("CH_S4 re-enable must stay unverified: %+v", e)
		}
		if e.Method == "POST" && strings.Contains(strings.Join(e.Channels, ","), "CH_S4") {
			t.Fatalf("CH_S4 must not be credited with a test event: %+v", e)
		}
	}
}

func TestSubscriptionsHealthFailureFlaggedEvidenceProvesNothing(t *testing.T) {
	run := func(t *testing.T, api *rqtAPI) {
		t.Helper()
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("failure-flagged evidence must not prove a re-enable:\n%s", out)
		}
		res := rqtDecode[shResult](t, out)
		for _, e := range *res.Executed {
			if e.Method == "PATCH" && e.ChannelID == "CH_S4" && (e.OK || e.Outcome != "reenable_unverified") {
				t.Fatalf("CH_S4 must stay reenable_unverified: %+v", e)
			}
			if e.Method == "POST" && strings.Contains(strings.Join(e.Channels, ","), "CH_S4") {
				t.Fatalf("CH_S4 must not be credited with a test event: %+v", e)
			}
		}
	}
	t.Run("PATCH answer with ok:false and status ERROR", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.patchBody["CH_S4"] = map[string]any{"channelId": "CH_S4", "enabled": true, "ok": false, "status": "ERROR"}
		run(t, api)
	})
	t.Run("verification row with success:false", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.patchBody["CH_S4"] = map[string]any{"channelId": "CH_S4"} // proves nothing: no enabled:true
		api.onPatch = func(a *rqtAPI, ch string) {
			if ch != "CH_S4" {
				return
			}
			kept := make([]any, 0)
			for _, item := range a.disabledSubs.([]any) {
				if sub, _ := item.(map[string]any); sub["channelId"] != "CH_S4" {
					kept = append(kept, item)
				}
			}
			a.disabledSubs = kept
			a.subs = append(a.subs.([]any), map[string]any{"channelId": "CH_S4", "url": "https://hooks.example.com/saas/four",
				"partnerId": "P1", "enabled": true, "success": false, "alertStatuses": []any{"critical", "medium"}})
		}
		run(t, api)
	})
}

func TestSubscriptionsHealthFailureFlaggedDomainRowsLeaveVerificationUnknown(t *testing.T) {
	rows := map[string]map[string]any{
		"success:false":            {"id": "D1", "domain": "hooks.example.com/saas", "partnerId": "P1", "success": false},
		"ok:false with an error":   {"id": "D1", "domain": "hooks.example.com/saas", "partnerId": "P1", "ok": false, "error": "verification unavailable"},
		"an envelope-style status": {"id": "D1", "domain": "hooks.example.com/saas", "partnerId": "P1", "status": "ERROR"},
	}
	for name, row := range rows {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtWebhookFixtures(api)
			api.domains = []any{row}
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "subscriptions", "health", "--reenable", "--confirm", "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("an incomplete domain list must fail the run:\n%s", out)
			}
			res := rqtDecode[shResult](t, out)
			for _, r := range res.Subscriptions {
				if r.DomainVerified != nil || !r.hasFlag(shFlagDomainUnknown) {
					t.Fatalf("%s: verification must be unknown: %+v", r.ChannelID, r)
				}
			}
			for _, p := range *res.Plan {
				if p.Planned {
					t.Fatalf("no re-enable may be planned on unknown verification: %+v", p)
				}
			}
			if w := api.writes(); len(w) != 0 {
				t.Fatalf("writes = %+v, want none", w)
			}
		})
	}
}

func TestSubscriptionsHealthFailureFlaggedQueueMembersLeaveDepthUnknown(t *testing.T) {
	t.Run("allTagged path", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.queue = []any{map[string]any{"unsentChannelIds": []any{}, "success": false, "error": "backend failure"}}
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("an incomplete queue must fail the run:\n%s", out)
		}
		res := rqtDecode[shResult](t, out)
		for _, r := range res.Subscriptions {
			if r.UnsentQueueDepth != nil || !r.hasFlag(shFlagQueueUnknown) {
				t.Fatalf("%s: depth must be unknown, never 0: depth=%v flags=%v", r.ChannelID, r.UnsentQueueDepth, r.Flags)
			}
		}
		if len(res.CollectionIncomplete) != 1 || res.CollectionIncomplete[0].Collection != "queue" || res.CollectionIncomplete[0].Invalid != 1 {
			t.Fatalf("collection_incomplete = %+v", res.CollectionIncomplete)
		}
	})
	t.Run("per-channel path", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.queue = []any{map[string]any{"id": "Q1", "partnerId": "P1", "time": "2026-10-14 10:00:00.000"}} // untagged: per-channel reads
		api.channelQueue["CH_S4"] = []any{map[string]any{"id": "PACK_1", "success": false, "error": "backend failure"}}
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "subscriptions", "health", "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("an incomplete channel queue must fail the run:\n%s", out)
		}
		res := rqtDecode[shResult](t, out)
		rows := rqtRowsByChannel(res)
		if s4 := rows["CH_S4"]; s4.UnsentQueueDepth != nil || !s4.hasFlag(shFlagQueueUnknown) || s4.hasFlag(shFlagBacklog) {
			t.Fatalf("CH_S4 depth must be unknown: depth=%v flags=%v", s4.UnsentQueueDepth, s4.Flags)
		}
		if s1 := rows["CH_S1"]; s1.UnsentQueueDepth == nil || *s1.UnsentQueueDepth != 0 {
			t.Fatalf("CH_S1 (empty queue) should still read 0: %+v", s1)
		}
		if len(res.CollectionIncomplete) != 1 || res.CollectionIncomplete[0].Collection != "queue/channels/CH_S4" {
			t.Fatalf("collection_incomplete = %+v", res.CollectionIncomplete)
		}
	})
}
