// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// TestNovelRuleTriggersDecideHelpWires smoke-tests that the rule-triggers decide command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRuleTriggersDecideHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"rule-triggers", "decide", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rule-triggers decide --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "decide"} {
		if !strings.Contains(help, want) {
			t.Fatalf("rule-triggers decide --help missing %q in output:\n%s", want, help)
		}
	}
}

func rqtDecisionPaths(calls []rqtCall) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.Method+" "+strings.TrimPrefix(c.Path, "/respondApi/api/v1"))
	}
	return out
}

func rqtSkipReasons(r rtDecideResult) map[string]string {
	out := map[string]string{}
	for _, s := range r.Skipped {
		out[s.TriggerID] = s.Reason
	}
	return out
}

func TestRuleTriggersDecidePreviewMakesNoWrites(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--remarks", "false positive", "--json")
	if err != nil {
		t.Fatalf("decide preview: %v\nstderr: %s", err, stderr)
	}
	res := rqtDecode[rtDecideResult](t, out)
	if res.Mode != "preview" || len(res.Planned) != 1 || res.Planned[0].TriggerID != "RT_B" {
		t.Fatalf("preview plan wrong: %s", out)
	}
	p := res.Planned[0]
	if p.Method != "POST" || p.Path != "/respondApi/api/v1/rule-triggers/RT_B/reject" || p.StatusBefore != "PENDING" ||
		strings.Join(p.WouldFire, ",") != "Expire Account Logins" || p.Trigger.RuleName != "Mass download" {
		t.Fatalf("planned call wrong: %+v", p)
	}
	body, _ := json.Marshal(p.Body)
	if string(body) != `{"remarks":"false positive"}` {
		t.Fatalf("planned body = %s", body)
	}
	if !strings.Contains(res.Note, "--confirm") || len(res.Results) != 0 {
		t.Fatalf("preview note/results wrong: %+v", res)
	}
	if w := api.writes(); len(w) != 0 {
		t.Fatalf("preview made writes: %v", rqtDecisionPaths(w))
	}
	lists := api.callsMatching("GET", "/respondApi/api/v1/rule-triggers")
	if len(lists) == 0 || strings.Join(lists[0].Query["ruleIds[]"], ",") != "RULE_B" || strings.Join(lists[0].Query["statuses[]"], ",") != "PENDING" {
		t.Fatalf("candidate fetch did not filter by rule/status: %+v", lists)
	}
}

func TestRuleTriggersDecideConfirmSelectsNonExpiredAndWritesReceipts(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	dbPath := filepath.Join(t.TempDir(), "receipts.db")
	out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "approve", "--org", "ORG_1", "--confirm", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("decide confirm: %v\nstderr: %s\n%s", err, stderr, out)
	}
	res := rqtDecode[rtDecideResult](t, out)

	posts := rqtDecisionPaths(api.writes())
	want := []string{"POST /rule-triggers/RT_B/approve", "POST /rule-triggers/RT_A2/approve"}
	if strings.Join(posts, "|") != strings.Join(want, "|") {
		t.Fatalf("POSTs = %v, want exactly %v", posts, want)
	}
	for _, c := range api.writes() {
		if c.Body != "" {
			t.Fatalf("approve must send no body, sent %q", c.Body)
		}
	}
	reasons := rqtSkipReasons(res)
	if reasons["RT_A"] != rtSkipExpiredSuper || reasons["RT_D"] != rtSkipDestructive || len(reasons) != 2 {
		t.Fatalf("skipped = %v, want RT_A superseded and RT_D destructive", reasons)
	}
	if res.Mode != "executed" || res.Summary.Succeeded != 2 || res.Summary.Verified != 2 || res.Summary.Failed != 0 {
		t.Fatalf("summary wrong: %+v", res.Summary)
	}
	for _, r := range res.Results {
		if !r.OK || !r.Verified || r.StatusBefore != "PENDING" || r.StatusAfter != "APPROVED" || r.ReceiptID == 0 {
			t.Fatalf("result not verified: %+v", r)
		}
	}
	if gets := api.callsMatching("GET", "/respondApi/api/v1/rule-triggers/RT_"); len(gets) != 2 {
		t.Fatalf("verification reads = %d, want 2", len(gets))
	}

	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open receipts store: %v", err)
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT trigger_id, decision, status_before, status_after, verified, decided_at FROM decision_receipts ORDER BY id`)
	if err != nil {
		t.Fatalf("query receipts: %v", err)
	}
	got := make([]string, 0)
	for rows.Next() {
		var id, decision, before, after, decidedAt string
		var verified int
		if err := rows.Scan(&id, &decision, &before, &after, &verified, &decidedAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, fmt.Sprintf("%s:%s:%s->%s:%d:%s", id, decision, before, after, verified, decidedAt))
	}
	rows.Close()
	wantRows := []string{
		"RT_B:approve:PENDING->APPROVED:1:2026-10-14T12:00:00Z",
		"RT_A2:approve:PENDING->APPROVED:1:2026-10-14T12:00:00Z",
	}
	if strings.Join(got, "|") != strings.Join(wantRows, "|") {
		t.Fatalf("receipts = %v, want %v", got, wantRows)
	}
}

func TestRuleTriggersDecideDestructiveGate(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	dbPath := filepath.Join(t.TempDir(), "receipts.db")

	out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "approve", "--ids", "RT_D", "--confirm", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("blocked approve should exit 0: %v", err)
	}
	res := rqtDecode[rtDecideResult](t, out)
	if len(api.writes()) != 0 || len(res.Planned) != 0 || rqtSkipReasons(res)["RT_D"] != rtSkipDestructive {
		t.Fatalf("Delete User trigger was not blocked: writes=%v result=%s", rqtDecisionPaths(api.writes()), out)
	}
	if !strings.Contains(res.Skipped[0].Detail, "Delete User") {
		t.Fatalf("skip detail should name the destructive action: %+v", res.Skipped[0])
	}

	// Rejecting it is not destructive, so the gate does not apply.
	out, _, err = rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_D", "--json")
	if err != nil {
		t.Fatalf("reject preview: %v", err)
	}
	if res := rqtDecode[rtDecideResult](t, out); len(res.Planned) != 1 {
		t.Fatalf("reject of a destructive-rule trigger should be planned: %s", out)
	}

	out, _, err = rqtRun(t, "rule-triggers", "decide", "--decision", "approve", "--ids", "RT_D", "--allow-destructive", "--confirm", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("allowed approve: %v\n%s", err, out)
	}
	if posts := rqtDecisionPaths(api.writes()); strings.Join(posts, "|") != "POST /rule-triggers/RT_D/approve" {
		t.Fatalf("--allow-destructive POSTs = %v", posts)
	}
}

func TestRuleTriggersDecideExpiredAndIDsOutsidePages(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)

	out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "ignore", "--org", "ORG_2", "--json")
	if err != nil {
		t.Fatalf("ignore preview: %v", err)
	}
	res := rqtDecode[rtDecideResult](t, out)
	if len(res.Planned) != 0 || rqtSkipReasons(res)["RT_E"] != rtSkipExpired12h {
		t.Fatalf("expired RT_E should be skipped (expired_12h): %s", out)
	}

	out, _, err = rqtRun(t, "rule-triggers", "decide", "--decision", "ignore", "--org", "ORG_2", "--include-expired", "--json")
	if err != nil {
		t.Fatalf("ignore --include-expired: %v", err)
	}
	res = rqtDecode[rtDecideResult](t, out)
	if len(res.Planned) != 1 || res.Planned[0].TriggerID != "RT_E" {
		t.Fatalf("--include-expired should plan RT_E: %s", out)
	}
	if body, _ := json.Marshal(res.Planned[0].Body); string(body) != `{"remarks":null}` {
		t.Fatalf("ignore without --remarks must send remarks:null, got %s", body)
	}

	// An id that is not PENDING is read directly and reported, not decided;
	// an id that does not exist is not_found and fails the run (exit 3).
	out, _, err = rqtRun(t, "rule-triggers", "decide", "--decision", "approve", "--ids", "RT_OLD,RT_NOPE", "--json")
	if rqtExit(err) != 3 {
		t.Fatalf("ids preview with a missing id: exit=%d err=%v, want 3", rqtExit(err), err)
	}
	res = rqtDecode[rtDecideResult](t, out)
	reasons := rqtSkipReasons(res)
	if len(res.Planned) != 0 || reasons["RT_OLD"] != rtSkipStatusMismatch || reasons["RT_NOPE"] != rtSkipNotFound {
		t.Fatalf("ids outside the selection wrong: %s", out)
	}
	if len(api.writes()) != 0 {
		t.Fatalf("previews wrote: %v", rqtDecisionPaths(api.writes()))
	}
}

func TestRuleTriggersDecideUsageErrors(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	cases := [][]string{
		{"rule-triggers", "decide", "--decision", "reject"},
		{"rule-triggers", "decide", "--decision", "reject", "--status", "PENDING", "--confirm"},
		{"rule-triggers", "decide", "--rule", "RULE_B"},
		{"rule-triggers", "decide", "--decision", "nuke", "--rule", "RULE_B"},
		{"rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--status", "SOMETIMES"},
	}
	for _, args := range cases {
		_, _, err := rqtRun(t, args...)
		if rqtExit(err) != 2 {
			t.Fatalf("%v: exit = %d (%v), want 2", args, rqtExit(err), err)
		}
	}
	if api.callCount() != 0 {
		t.Fatalf("usage errors reached the API %d times", api.callCount())
	}
}

func TestRuleTriggersDecidePartialFailureKeepsGoing(t *testing.T) {
	api := newRQTAPI(t)
	api.decisionFail["RT_B"] = http.StatusConflict
	rqtEnv(t, api)
	dbPath := filepath.Join(t.TempDir(), "receipts.db")
	out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_B,RT_A2", "--remarks", "fp", "--confirm", "--db", dbPath, "--json")
	if rqtExit(err) == 0 {
		t.Fatalf("a failed decision must exit non-zero")
	}
	res := rqtDecode[rtDecideResult](t, out)
	if posts := rqtDecisionPaths(api.writes()); len(posts) != 2 {
		t.Fatalf("should POST both despite the failure, got %v", posts)
	}
	byID := map[string]rtDecideOutcome{}
	for _, r := range res.Results {
		byID[r.TriggerID] = r
	}
	if b := byID["RT_B"]; b.OK || b.HTTPStatus != http.StatusConflict || b.Error == "" {
		t.Fatalf("RT_B failure not reported: %+v", b)
	}
	if a := byID["RT_A2"]; !a.OK || !a.Verified || a.StatusAfter != "REJECTED" {
		t.Fatalf("RT_A2 should still succeed: %+v", a)
	}
	if res.Summary.Failed != 1 || res.Summary.Succeeded != 1 {
		t.Fatalf("summary = %+v", res.Summary)
	}
}

func TestRuleTriggersDecideHarnessRefusesConfirm(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--confirm", "--json")
	if err != nil {
		t.Fatalf("harness refusal should exit 0: %v", err)
	}
	if !strings.Contains(out, `"refused"`) || len(api.writes()) != 0 {
		t.Fatalf("harness did not refuse: out=%s writes=%v", out, rqtDecisionPaths(api.writes()))
	}
}

func TestRuleTriggersDecideVerificationIsDecisionSpecific(t *testing.T) {
	cases := []struct {
		name         string
		decision     string
		setup        func(a *rqtAPI)
		verified     bool
		statusAfter  string
		verification string
	}{
		{name: "reject that reads back AUTO_IGNORED is not verified", decision: "reject",
			setup: func(a *rqtAPI) { a.decisionStatus["RT_B"] = "AUTO_IGNORED" }, statusAfter: "AUTO_IGNORED", verification: "not an outcome of reject"},
		{name: "reject that reads back REJECTED is verified", decision: "reject",
			setup: func(a *rqtAPI) {}, verified: true, statusAfter: "REJECTED", verification: "status REJECTED"},
		{name: "approve that moves on to remediation is verified", decision: "approve",
			setup: func(a *rqtAPI) { a.decisionStatus["RT_B"] = "REMEDIATION_SUCCESSFUL" }, verified: true, statusAfter: "REMEDIATION_SUCCESSFUL", verification: "status REMEDIATION_SUCCESSFUL"},
		{name: "approve whose status lags is verified by its history entry", decision: "approve",
			setup: func(a *rqtAPI) { a.decisionLag["RT_B"] = true }, verified: true, statusAfter: "PENDING", verification: "history APPROVED"},
		{name: "ignore that reads back REJECTED is not verified", decision: "ignore",
			setup: func(a *rqtAPI) { a.decisionStatus["RT_B"] = "REJECTED" }, statusAfter: "REJECTED", verification: "not an outcome of ignore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			tc.setup(api)
			rqtEnv(t, api)
			dbPath := filepath.Join(t.TempDir(), "receipts.db")
			out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", tc.decision, "--ids", "RT_B", "--confirm", "--db", dbPath, "--json")
			// The fake's POST answer carries the same status the re-read shows,
			// so an unverified decision also has an answer that is no evidence:
			// unknown_outcome, non-zero exit.
			if tc.verified != (err == nil) {
				t.Fatalf("decide exit err=%v, want success=%v\n%s", err, tc.verified, out)
			}
			res := rqtDecode[rtDecideResult](t, out)
			if len(res.Results) != 1 {
				t.Fatalf("results = %s", out)
			}
			got := res.Results[0]
			if !tc.verified && got.Outcome != "unknown_outcome" {
				t.Fatalf("outcome = %q, want unknown_outcome", got.Outcome)
			}
			if got.Verified != tc.verified || got.StatusAfter != tc.statusAfter || !strings.Contains(got.Verification, tc.verification) {
				t.Fatalf("verified=%v status_after=%s verification=%q; want %v %s %q", got.Verified, got.StatusAfter, got.Verification, tc.verified, tc.statusAfter, tc.verification)
			}
			if wantUnverified := map[bool]int{true: 0, false: 1}[tc.verified]; res.Summary.Unverified != wantUnverified {
				t.Fatalf("summary.unverified = %d, want %d", res.Summary.Unverified, wantUnverified)
			}
		})
	}
}

func TestRuleTriggersDecideHelpStatesPreviewDefault(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := rqtRun(t, "rule-triggers", "decide", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"without --confirm, is a preview", "--dry-run only prints a stub"} {
		if !strings.Contains(out, want) {
			t.Fatalf("decide --help missing %q", want)
		}
	}
}

func TestRuleTriggersDecideDirectFetchShapes(t *testing.T) {
	pending := func(id string) map[string]any {
		return map[string]any{"id": id, "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": "ORG_9", "name": "Tailspin"},
			"accounts":     rqtAccount("ACC_X", "xavier@example.com"),
			"rule":         rqtRule("RULE_X", "Unusual mailbox access", "medium", "Alert Only")}
	}
	cases := []struct {
		name    string
		body    any // nil: no override (the fake 404s an unknown id)
		reason  string
		planned bool
	}{
		{name: "null body", body: rqtNull, reason: rtSkipLookupFailed},
		{name: "error-shaped object", body: map[string]any{"message": "backend failure"}, reason: rtSkipLookupFailed},
		{name: "null ruleTrigger", body: map[string]any{"ruleTrigger": nil, "events": []any{}}, reason: rtSkipLookupFailed},
		{name: "another trigger's id", body: map[string]any{"ruleTrigger": pending("RT_OTHER"), "events": []any{}}, reason: rtSkipLookupFailed},
		{name: "bare RuleTrigger", body: pending("RT_X"), planned: true},
		{name: "documented ruleTrigger wrapper", body: map[string]any{"ruleTrigger": pending("RT_X"), "events": []any{}, "lastRuleTriggerTime": rqtMS(-time.Hour)}, planned: true},
		{name: "HTTP 404", body: nil, reason: rtSkipNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			if tc.body != nil {
				api.triggerGet["RT_X"] = tc.body
			}
			rqtEnv(t, api)
			// RT_B comes from the list; RT_X is outside it and read directly.
			out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_B,RT_X", "--json")
			res := rqtDecode[rtDecideResult](t, out)
			planned := map[string]bool{}
			for _, p := range res.Planned {
				planned[p.TriggerID] = true
			}
			if !planned["RT_B"] {
				t.Fatalf("RT_B must still be planned: %s", out)
			}
			if tc.planned {
				if err != nil || !planned["RT_X"] {
					t.Fatalf("a valid trigger must be planned with exit 0: err=%v\n%s", err, out)
				}
				return
			}
			if got := rqtSkipReasons(res)["RT_X"]; got != tc.reason {
				t.Fatalf("RT_X skip reason = %q, want %q\n%s", got, tc.reason, out)
			}
			if tc.reason == rtSkipNotFound {
				if rqtExit(err) != 3 || res.Summary.NotFound != 1 || res.Summary.LookupErrors != 0 {
					t.Fatalf("an HTTP 404 is not_found and fails the run with exit 3: exit=%d summary=%+v", rqtExit(err), res.Summary)
				}
				return
			}
			if rqtExit(err) == 0 || res.Summary.LookupErrors != 1 {
				t.Fatalf("a malformed 200 must fail the run: exit=%d lookup_errors=%d", rqtExit(err), res.Summary.LookupErrors)
			}
		})
	}

	t.Run("confirm decides the good id and still exits non-zero", func(t *testing.T) {
		api := newRQTAPI(t)
		api.triggerGet["RT_X"] = map[string]any{"message": "backend failure"}
		rqtEnv(t, api)
		dbPath := filepath.Join(t.TempDir(), "receipts.db")
		out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_B,RT_X", "--confirm", "--db", dbPath, "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("a failed lookup must exit non-zero:\n%s", out)
		}
		if posts := rqtDecisionPaths(api.writes()); strings.Join(posts, "|") != "POST /rule-triggers/RT_B/reject" {
			t.Fatalf("POSTs = %v, want only RT_B", posts)
		}
	})
}

func TestRuleTriggersDecideUndecodedRowsFailTheRun(t *testing.T) {
	rtB := rqtTriggers()[1]
	t.Run("all rows invalid", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{map[string]any{"ruleTriggers": []any{map[string]any{"foo": 1}, "junk", nil},
			"pagination": map[string]any{"hasNextPage": false}}}
		rqtEnv(t, api)
		out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("undecodable rows must fail the run:\n%s", out)
		}
		res := rqtDecode[rtDecideResult](t, out)
		if res.Summary.Undecoded != 3 || len(res.Planned) != 0 || !strings.Contains(stderr, "could not be decoded") {
			t.Fatalf("undecoded=%d planned=%d stderr=%q", res.Summary.Undecoded, len(res.Planned), stderr)
		}
	})
	t.Run("mixed rows still decide the valid ones", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{map[string]any{"ruleTriggers": []any{rtB, map[string]any{"status": "PENDING"}},
			"pagination": map[string]any{"hasNextPage": false}}}
		rqtEnv(t, api)
		dbPath := filepath.Join(t.TempDir(), "receipts.db")
		out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--confirm", "--db", dbPath, "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("an undecodable row must fail the run even when others succeed:\n%s", out)
		}
		res := rqtDecode[rtDecideResult](t, out)
		if res.Summary.Undecoded != 1 || res.Summary.Succeeded != 1 {
			t.Fatalf("summary = %+v", res.Summary)
		}
		if posts := rqtDecisionPaths(api.writes()); strings.Join(posts, "|") != "POST /rule-triggers/RT_B/reject" {
			t.Fatalf("POSTs = %v, want RT_B decided", posts)
		}
	})
}

// rqtPendingWithRule is a PENDING trigger whose rule is exactly rule.
func rqtPendingWithRule(id string, rule any) map[string]any {
	return map[string]any{"id": id, "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
		"organization": map[string]any{"id": "ORG_9", "name": "Tailspin"},
		"accounts":     rqtAccount("ACC_X", "xavier@example.com"),
		"rule":         rule}
}

func TestRuleTriggersDecideApproveNeedsKnownActions(t *testing.T) {
	ruleNoActions := func(id string) map[string]any {
		return map[string]any{"id": id, "name": "Partial rule", "alertSeverity": "high"}
	}
	cases := []struct {
		name        string
		rule        any
		rules       map[string]any // GET /rules/{id} bodies
		direct      any            // GET /rule-triggers/RT_X override
		wantPost    bool
		wantSkip    string
		wantNonZero bool
	}{
		{name: "rule null", rule: nil, wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "responseActions missing, rule lookup 404", rule: ruleNoActions("RULE_NOACT"), wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "entries missing responseAction", rule: map[string]any{"id": "RULE_PARTIAL", "responseActions": []any{
			map[string]any{"applicationType": "MS"}, map[string]any{"applicationType": "MS", "responseAction": "Block Sign In"}}},
			wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "an action name outside the spec enum", rule: map[string]any{"id": "RULE_ODD", "responseActions": []any{
			map[string]any{"applicationType": "MS", "responseAction": "Wipe Device"}}},
			wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "rule lookup returns an error-shaped body", rule: ruleNoActions("RULE_BROKEN"),
			rules: map[string]any{"RULE_BROKEN": map[string]any{"message": "backend failure"}}, wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "rule lookup is a failure response with an empty action list", rule: ruleNoActions("RULE_F1"),
			rules:    map[string]any{"RULE_F1": map[string]any{"id": "RULE_F1", "success": false, "error": "backend failure", "responseActions": []any{}}},
			wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "rule lookup is a failure response with an apparently safe action list", rule: ruleNoActions("RULE_F2"),
			rules: map[string]any{"RULE_F2": func() map[string]any {
				r := rqtRule("RULE_F2", "Partial rule", "high", "Block Sign In")
				r["ok"], r["code"] = false, 500
				return r
			}()},
			wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "embedded rule carries a failure indicator", rule: func() map[string]any {
			r := rqtRule("RULE_EMB", "Embedded rule", "high", "Block Sign In")
			r["error"] = "rule unavailable"
			return r
		}(), wantSkip: rtSkipSafetyUnknown, wantNonZero: true},
		{name: "rule lookup supplies safe actions", rule: ruleNoActions("RULE_SAFE"),
			rules: map[string]any{"RULE_SAFE": rqtRule("RULE_SAFE", "Partial rule", "high", "Block Sign In")}, wantPost: true},
		{name: "rule lookup supplies Delete User", rule: ruleNoActions("RULE_DEL"),
			rules: map[string]any{"RULE_DEL": rqtRule("RULE_DEL", "Partial rule", "high", "Block Sign In", "Delete User")}, wantSkip: rtSkipDestructive},
		{name: "direct re-read supplies complete actions", rule: ruleNoActions("RULE_REREAD"),
			direct: rqtPendingWithRule("RT_X", rqtRule("RULE_REREAD", "Partial rule", "high", "Expire Account Logins")), wantPost: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.triggers = append(api.triggers, rqtPendingWithRule("RT_X", tc.rule))
			for id, body := range tc.rules {
				api.rules[id] = body
			}
			if tc.direct != nil {
				api.triggerGet["RT_X"] = tc.direct
			}
			rqtEnv(t, api)
			dbPath := filepath.Join(t.TempDir(), "receipts.db")
			out, stderr, err := rqtRun(t, "rule-triggers", "decide", "--decision", "approve", "--ids", "RT_X", "--confirm", "--db", dbPath, "--json")
			res := rqtDecode[rtDecideResult](t, out)
			posts := rqtDecisionPaths(api.writes())
			if tc.wantPost {
				if err != nil || strings.Join(posts, "|") != "POST /rule-triggers/RT_X/approve" || len(res.Results) != 1 || len(res.Planned[0].WouldFire) == 0 {
					t.Fatalf("approval should proceed with known safe actions: err=%v posts=%v\n%s", err, posts, out)
				}
				return
			}
			if len(posts) != 0 {
				t.Fatalf("approve was POSTed without known-safe actions: %v", posts)
			}
			if got := rqtSkipReasons(res)["RT_X"]; got != tc.wantSkip {
				t.Fatalf("skip reason = %q, want %q\n%s", got, tc.wantSkip, out)
			}
			if tc.wantNonZero != (rqtExit(err) != 0) {
				t.Fatalf("exit=%d, want non-zero=%v", rqtExit(err), tc.wantNonZero)
			}
			if tc.wantSkip == rtSkipSafetyUnknown && (res.Summary.ActionSafetyUnknown != 1 || !strings.Contains(stderr, "action_safety_unknown")) {
				t.Fatalf("summary/stderr wrong: %+v %q", res.Summary, stderr)
			}
		})
	}

	t.Run("reject does not need the actions", func(t *testing.T) {
		api := newRQTAPI(t)
		api.triggers = append(api.triggers, rqtPendingWithRule("RT_X", nil))
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_X", "--json")
		if err != nil || len(rqtDecode[rtDecideResult](t, out).Planned) != 1 {
			t.Fatalf("reject should be planned as before: err=%v\n%s", err, out)
		}
		if n := len(api.callsMatching("GET", "/respondApi/api/v1/rules/")) + len(api.callsMatching("GET", "/respondApi/api/v1/rule-triggers/RT_X")); n != 0 {
			t.Fatalf("reject looked up actions %d times", n)
		}
	})
}

func TestRuleTriggersDecideOffContractPostAnswer(t *testing.T) {
	cases := []struct {
		name     string
		body     any // nil: the fake's documented RuleTrigger answer
		applies  bool
		outcome  string
		verified bool
		nonZero  bool
	}{
		{name: "error-shaped 200, trigger unchanged", body: map[string]any{"message": "backend failure"}, outcome: "unknown_outcome", nonZero: true},
		{name: "status ERROR, trigger unchanged", body: map[string]any{"id": "RT_B", "status": "ERROR"}, outcome: "unknown_outcome", nonZero: true},
		{name: "null 200, trigger unchanged", body: rqtNull, outcome: "unknown_outcome", nonZero: true},
		{name: "matching id with a failure message, trigger unchanged", body: map[string]any{"id": "RT_B", "message": "backend failure"}, outcome: "unknown_outcome", nonZero: true},
		{name: "matching id, success:false and an error, trigger unchanged", body: map[string]any{"id": "RT_B", "status": "PENDING", "success": false, "error": "decision failed"}, outcome: "unknown_outcome", nonZero: true},
		{name: "matching id still PENDING (no evidence of the decision), trigger unchanged", body: map[string]any{"id": "RT_B", "status": "PENDING"}, outcome: "unknown_outcome", nonZero: true},
		{name: "error-shaped 200 but the re-read proves it", body: map[string]any{"message": "backend failure"}, applies: true, outcome: "unknown_outcome", verified: true},
		{name: "documented answer", outcome: "accepted", verified: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			if tc.body != nil {
				api.decisionBody["RT_B"] = tc.body
				api.decisionApplies["RT_B"] = tc.applies
			}
			rqtEnv(t, api)
			dbPath := filepath.Join(t.TempDir(), "receipts.db")
			out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_B", "--remarks", "fp", "--confirm", "--db", dbPath, "--json")
			res := rqtDecode[rtDecideResult](t, out)
			if n := len(api.callsMatching("POST", "/respondApi/api/v1/rule-triggers/RT_B/reject")); n != 1 {
				t.Fatalf("decision POSTs = %d, want exactly 1 (never replayed)", n)
			}
			if len(res.Results) != 1 {
				t.Fatalf("results = %s", out)
			}
			got := res.Results[0]
			if got.Outcome != tc.outcome || got.Verified != tc.verified {
				t.Fatalf("outcome=%q verified=%v, want %q %v\n%s", got.Outcome, got.Verified, tc.outcome, tc.verified, out)
			}
			if tc.nonZero != (rqtExit(err) != 0) {
				t.Fatalf("exit=%d, want non-zero=%v", rqtExit(err), tc.nonZero)
			}
			if tc.nonZero && (res.Summary.UnknownOutcomes != 1 || !strings.Contains(got.Error, "not retried")) {
				t.Fatalf("unknown outcome not reported: %+v / %q", res.Summary, got.Error)
			}
		})
	}
}

func TestRuleTriggersDecideVerificationReadWithFailureIndicators(t *testing.T) {
	rejected := func() map[string]any {
		tr := rqtTriggers()[1]
		tr["status"] = "REJECTED"
		return tr
	}
	cases := map[string]any{
		"trigger with success:false and an error": func() map[string]any {
			tr := rejected()
			tr["success"], tr["error"] = false, "decision failed"
			return tr
		}(),
		"wrapper with an envelope status": map[string]any{"ruleTrigger": rejected(), "status": "ERROR"},
		"trigger with ok:false":           func() map[string]any { tr := rejected(); tr["ok"] = false; return tr }(),
	}
	for name, read := range cases {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.decisionBody["RT_B"] = map[string]any{"message": "backend failure"}
			api.triggerGet["RT_B"] = read
			rqtEnv(t, api)
			dbPath := filepath.Join(t.TempDir(), "receipts.db")
			out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_B", "--confirm", "--db", dbPath, "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("a failure-flagged read must not verify the decision:\n%s", out)
			}
			res := rqtDecode[rtDecideResult](t, out)
			if got := res.Results[0]; got.Verified || got.Outcome != "unknown_outcome" || got.Verification != "read_failed" {
				t.Fatalf("result = %+v", got)
			}
			db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("open receipts: %v", err)
			}
			defer db.Close()
			var verified int
			if err := db.DB().QueryRow(`SELECT verified FROM decision_receipts WHERE trigger_id = 'RT_B'`).Scan(&verified); err != nil || verified != 0 {
				t.Fatalf("receipt must be unverified: verified=%d err=%v", verified, err)
			}
		})
	}
}

func TestRuleTriggersBulkRowsWithFailureIndicatorsAreRejected(t *testing.T) {
	flagged := rqtTriggers()[1]
	flagged["error"] = "row unavailable"
	api := newRQTAPI(t)
	api.listResponses = []any{map[string]any{"ruleTriggers": []any{rqtTriggers()[0], flagged}, "pagination": map[string]any{"hasNextPage": false}}}
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "rule-triggers", "queue", "--json")
	env := rqtDecode[rqQueueEnvelope](t, out)
	if rqtExit(err) == 0 || env.Undecoded != 1 || len(env.Items) != 1 || env.Items[0].TriggerID != "RT_A2" {
		t.Fatalf("a row carrying a failure indicator must be rejected: err=%v undecoded=%d %s", err, env.Undecoded, out)
	}
}

// rqtCaseTrigger is a PENDING trigger with explicit id, organization and rule.
func rqtCaseTrigger(id, org, rule string) map[string]any {
	return map[string]any{"id": id, "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
		"organization": map[string]any{"id": org, "name": org},
		"accounts":     rqtAccount("ACC_"+id, id+"@example.com"),
		"rule":         rqtRule(rule, "Case rule", "medium", "Alert Only")}
}

func TestRuleTriggersDecideIdsAreExact(t *testing.T) {
	t.Run("--ids picks only the exact id", func(t *testing.T) {
		api := newRQTAPI(t)
		api.triggers = append(api.triggers, rqtCaseTrigger("RtCase", "ORG_RC", "RULE_RC1"), rqtCaseTrigger("rtcase", "ORG_RC", "RULE_RC2"))
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RtCase", "--confirm",
			"--db", filepath.Join(t.TempDir(), "r.db"), "--json")
		if err != nil {
			t.Fatalf("decide: %v\n%s", err, out)
		}
		if posts := rqtDecisionPaths(api.writes()); strings.Join(posts, "|") != "POST /rule-triggers/RtCase/reject" {
			t.Fatalf("POSTs = %v, want exactly RtCase", posts)
		}
	})
	t.Run("a case variant of a missing id is not a match", func(t *testing.T) {
		api := newRQTAPI(t)
		api.triggers = append(api.triggers, rqtCaseTrigger("rtcase", "ORG_RC", "RULE_RC2"))
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--ids", "RtCase", "--confirm",
			"--db", filepath.Join(t.TempDir(), "r.db"), "--json")
		res := rqtDecode[rtDecideResult](t, out)
		if len(api.writes()) != 0 || rqtSkipReasons(res)["RtCase"] != rtSkipNotFound || rqtExit(err) == 0 {
			t.Fatalf("RtCase must be not_found with nothing sent and a non-zero exit: writes=%v exit=%d\n%s", api.writes(), rqtExit(err), out)
		}
	})
}

func TestRuleTriggersOrgAndRuleFiltersAreExact(t *testing.T) {
	setup := func(t *testing.T) *rqtAPI {
		t.Helper()
		api := newRQTAPI(t)
		upper, lower := rqtCaseTrigger("RT_UPPER", "OrgCase", "RuleCase"), rqtCaseTrigger("RT_LOWER", "orgcase", "rulecase")
		api.triggers = []map[string]any{upper, lower}
		// The fake API ignores the filters here (like a case-insensitive
		// server would), so only the CLI's own matching can separate them.
		api.listResponses = []any{map[string]any{"ruleTriggers": []any{upper, lower}, "pagination": map[string]any{"hasNextPage": false}}}
		rqtEnv(t, api)
		return api
	}
	for _, sel := range [][]string{{"--org", "OrgCase"}, {"--rule", "RuleCase"}} {
		t.Run("decide "+sel[0], func(t *testing.T) {
			api := setup(t)
			args := append([]string{"rule-triggers", "decide", "--decision", "reject", "--confirm", "--db", filepath.Join(t.TempDir(), "r.db"), "--json"}, sel...)
			out, _, err := rqtRun(t, args...)
			if err != nil {
				t.Fatalf("decide: %v\n%s", err, out)
			}
			if posts := rqtDecisionPaths(api.writes()); strings.Join(posts, "|") != "POST /rule-triggers/RT_UPPER/reject" {
				t.Fatalf("%v POSTs = %v, want only RT_UPPER", sel, posts)
			}
		})
		t.Run("queue "+sel[0], func(t *testing.T) {
			setup(t)
			out, _, err := rqtRun(t, append([]string{"rule-triggers", "queue", "--json"}, sel...)...)
			if err != nil {
				t.Fatalf("queue: %v", err)
			}
			if got := strings.Join(rqtQueueIDs(rqtDecode[rqQueueEnvelope](t, out)), ","); got != "RT_UPPER" {
				t.Fatalf("%v queue = %s, want only RT_UPPER", sel, got)
			}
		})
	}
}
