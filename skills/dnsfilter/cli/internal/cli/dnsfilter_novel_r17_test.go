// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 17: absent, null, mistyped or unparseable data is
// unknown, never empty, false, "not blocked", "allowed" or an all-clear.
// These tests use only symbols that predate the fixes (JSON output is read
// as maps), so each one also runs, and fails, against the unfixed code.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// r17Runner runs the CLI against h and returns stdout, stderr and the error.
func r17Runner(t *testing.T, h http.Handler) func(args ...string) (string, string, error) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Cleanup(func() { humanFriendly = false })
	return func(args ...string) (string, string, error) {
		humanFriendly = false
		cmd := RootCmd()
		cmd.SetArgs(append(args, "--rate-limit", "0"))
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		err := cmd.Execute()
		return out.String(), errOut.String(), err
	}
}

// r17Routes serves GET path -> body (page 1 only; later pages are empty
// lists). A body starting with "!" is a failed read, sent with HTTP 400 (a
// 5xx is retried with backoff, which only slows the test: both are errors).
func r17Routes(t *testing.T, routes map[string]string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Query().Get("page[number]"); p != "" && p != "1" {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not in fixture"}`)
			return
		}
		if strings.HasPrefix(body, "!") {
			w.WriteHeader(http.StatusBadRequest)
			body = body[1:]
		}
		fmt.Fprint(w, body)
	})
}

func r17JSON(t *testing.T, label, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%s: output is not a JSON object: %v\n%s", label, err, out)
	}
	return m
}

// Finding 1: {"data":null} (or a missing collection) on any page was an
// exhausted empty list, so unblocks triage printed "No pending unblock
// requests.".
func TestR17FetchPagesNullCollectionIsNotEmpty(t *testing.T) {
	full := make([]string, dnsfilterPageSize)
	for i := range full {
		full[i] = fmt.Sprintf(`{"id":"u%d","attributes":{"domain":"d%d.example.com","category":{"is_threat":false}}}`, i, i)
	}
	pages := map[string][2]string{
		"null data on page 1":    {`{"data":null}`, ``},
		"no data key on page 1":  {`{"meta":{"total":3}}`, ``},
		"object data on page 1":  {`{"data":{"id":"u1"}}`, ``},
		"null data on page 2":    {`{"data":[` + strings.Join(full, ",") + `]}`, `{"data":null}`},
		"missing data on page 2": {`{"data":[` + strings.Join(full, ",") + `]}`, `{}`},
	}
	for name, bodies := range pages {
		t.Run(name, func(t *testing.T) {
			run := r17Runner(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/unblock_requests" {
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.URL.Query().Get("page[number]") == "2" {
					fmt.Fprint(w, bodies[1])
					return
				}
				fmt.Fprint(w, bodies[0])
			}))
			out, _, err := run("unblocks", "triage", "--max-coverage-checks", "0", "--human-friendly")
			if err == nil || strings.Contains(out, "No pending unblock requests") {
				t.Fatalf("an unreadable queue page read as an empty queue: err=%v\n%s", err, out)
			}
			if out, _, err := run("unblocks", "triage", "--max-coverage-checks", "0", "--json"); err == nil {
				t.Fatalf("--json: an unreadable queue page returned success:\n%s", out)
			}
		})
	}
	// Control: a real empty list is an empty queue.
	run := r17Runner(t, r17Routes(t, map[string]string{"/v2/unblock_requests": `{"data":[]}`}))
	if out, _, err := run("unblocks", "triage", "--human-friendly"); err != nil || !strings.Contains(out, "No pending unblock requests.") {
		t.Fatalf("an empty list must still read as an empty queue: err=%v\n%s", err, out)
	}
}

// Finding 2: allow_list_only, blacklist_domains, blacklist_categories (and
// the rest) as null or absent reached allowed/no_matching_rule because
// boolean() read unknown as false and strs() read it as [].
func TestR17EvaluatePolicyUnknownAttributes(t *testing.T) {
	social := []categoryRef{{ID: "7", Name: "Social"}}
	full := map[string]any{"name": "P", "whitelist_domains": []any{}, "allow_list_only": false, "blacklist_domains": []any{}, "allow_unknown_domains": true, "blacklist_categories": []any{}}
	policy := func(set map[string]any, drop ...string) jaRes {
		attrs := map[string]any{}
		for k, v := range full {
			attrs[k] = v
		}
		for k, v := range set {
			attrs[k] = v
		}
		for _, k := range drop {
			delete(attrs, k)
		}
		raw, _ := json.Marshal(map[string]any{"id": "10", "attributes": attrs})
		return ja(t, string(raw))
	}
	unknown := []struct {
		name string
		p    jaRes
		cats []categoryRef
	}{
		{"allow_list_only null", policy(map[string]any{"allow_list_only": nil}), social},
		{"allow_list_only absent", policy(nil, "allow_list_only"), social},
		{"allow_list_only mistyped", policy(map[string]any{"allow_list_only": "false"}), social},
		{"blacklist_domains null", policy(map[string]any{"blacklist_domains": nil}), social},
		{"blacklist_domains absent", policy(nil, "blacklist_domains"), social},
		{"blacklist_domains with a null entry", policy(map[string]any{"blacklist_domains": []any{nil, "other.example"}}), social},
		{"blacklist_categories null", policy(map[string]any{"blacklist_categories": nil}), social},
		{"blacklist_categories absent", policy(nil, "blacklist_categories"), social},
		{"whitelist_domains null", policy(map[string]any{"whitelist_domains": nil, "allow_list_only": true}), social},
		{"allow_unknown_domains null, uncategorized", policy(map[string]any{"allow_unknown_domains": nil}), []categoryRef{}},
	}
	for _, c := range unknown {
		if got := evaluatePolicy("example.net", c.p, c.cats, true); got.Decision != "unknown" {
			t.Errorf("%s: got %s/%s (%s); an unreadable attribute must give an unknown verdict", c.name, got.Decision, got.Rule, got.Detail)
		}
	}
	// A decision the readable data proves still stands.
	if got := evaluatePolicy("example.net", policy(map[string]any{"whitelist_domains": []any{"example.net"}, "allow_list_only": nil, "blacklist_domains": nil, "allow_unknown_domains": nil, "blacklist_categories": nil}), social, true); got.Decision != "allowed" || got.Rule != "allow_list" {
		t.Errorf("exact allow entry with everything else null = %s/%s, want allowed/allow_list", got.Decision, got.Rule)
	}
	if got := evaluatePolicy("example.net", policy(map[string]any{"blacklist_domains": []any{"example.net"}, "blacklist_categories": nil}), social, true); got.Decision != "blocked" || got.Rule != "block_list" {
		t.Errorf("exact block entry with categories null = %s/%s, want blocked/block_list", got.Decision, got.Rule)
	}
	if got := evaluatePolicy("example.net", policy(nil), social, true); got.Decision != "allowed" || got.Rule != "no_matching_rule" {
		t.Errorf("fully readable policy, nothing matches = %s/%s, want allowed/no_matching_rule", got.Decision, got.Rule)
	}
}

// whyBlockedRoutes is an org with one site on scheduled policy 300.
func whyBlockedRoutes(sched, policies, logs string) map[string]string {
	return map[string]string{
		"/v1/organizations/1":            `{"data":{"id":"1","attributes":{"name":"Acme Dental"}}}`,
		"/v1/policies/all":               policies,
		"/v1/networks/all":               `{"data":[{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"300"}}}}]}`,
		"/v1/scheduled_policies/300":     sched,
		"/v1/domains/user_lookup":        fixtureLookupOK,
		"/v1/traffic_reports/query_logs": logs,
	}
}

// Finding 3: with every deployment unknown (unreadable scheduled policy) and
// the query log failing, the top-level "blocked" was false: an all-clear.
func TestR17WhyBlockedAggregateUnknown(t *testing.T) {
	allowing := `{"data":[{"id":"10","attributes":{"name":"Std","whitelist_domains":[],"allow_list_only":false,"blacklist_domains":[],"blacklist_categories":[],"allow_unknown_domains":true}}]}`
	slots := `{"name":"Always 10","timezone":"UTC","policy_ids":` + scheduleSlots(10, 10) + `}`
	cases := []struct {
		name, sched, logs string
		decision          string
		blocked           any
	}{
		{"schedule unreadable, query log fails", `!{"error":"boom"}`, `!{"error":"boom"}`, "unknown", nil},
		{"schedule unreadable, empty log", `!{"error":"boom"}`, `{"data":{"values":[]}}`, "unknown", nil},
		{"allowed everywhere, query log fails", slots, `!{"error":"boom"}`, "unknown", nil},
		{"allowed everywhere, complete empty log", slots, `{"data":{"values":[]}}`, "not_blocked", false},
		{"unknown deployment, exact hit", `!{"error":"boom"}`, `{"data":{"values":[{"fqdn":"example.com","result":"blocked","policy_id":"10"}]}}`, "blocked", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := r17Runner(t, r17Routes(t, whyBlockedRoutes(c.sched, allowing, c.logs)))
			out, errOut, err := run("why-blocked", "example.com", "--org", "1", "--json")
			if err != nil {
				t.Fatalf("why-blocked: %v\n%s%s", err, out, errOut)
			}
			m := r17JSON(t, c.name, out)
			if m["decision"] != c.decision || m["blocked"] != c.blocked {
				t.Fatalf("decision=%v blocked=%v, want %s/%v\n%s", m["decision"], m["blocked"], c.decision, c.blocked, out)
			}
			human, _, err := run("why-blocked", "example.com", "--org", "1", "--human-friendly")
			if err != nil {
				t.Fatalf("human: %v", err)
			}
			want := "overall: " + strings.ToUpper(strings.ReplaceAll(c.decision, "_", " ")) + "\n"
			if !strings.Contains(human, want) || (c.decision != "not_blocked" && strings.Contains(human, "NOT BLOCKED")) {
				t.Fatalf("human output must say %q:\n%s", want, human)
			}
		})
	}
}

// Finding 4: a usable parent record (categories []) certified the lookup
// while the requested domain's own record had categories.data null, so
// allow --confirm wrote without --force.
func TestR17LookupCategoriesNeedsOwnRecord(t *testing.T) {
	for name, body := range map[string]string{
		"own record null, parent usable": `{"data":[{"id":"d1","attributes":{"host":"cdn.example.com"},"relationships":{"categories":{"data":null}}},{"id":"d2","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[]}}}]}`,
		"only a parent record":           `{"data":[{"id":"d2","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[]}}}]}`,
		"own record, unusable sibling":   `{"data":[{"id":"d1","attributes":{"host":"cdn.example.com"},"relationships":{"categories":{"data":[]}}},{"id":"d2","attributes":{"host":"example.com"},"relationships":{"categories":{"data":null}}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fixtureAPI{lookupBody: body}
			_, err := runFixture(t, f, "allow", "cdn.example.com", "--org", "1", "--note", "T", "--confirm")
			if exitCode(err) != 2 || !strings.Contains(fmt.Sprint(err), "categories (and threat status) are unknown") || len(f.writes) != 0 {
				t.Fatalf("err=%v exit=%d writes=%v; categories are unknown without a usable record for the domain itself", err, exitCode(err), f.writes)
			}
		})
	}
	// Control: the domain's own usable record (plus a usable parent) is known.
	f := &fixtureAPI{lookupBody: `{"data":[{"id":"d1","attributes":{"host":"cdn.example.com"},"relationships":{"categories":{"data":[]}}},{"id":"d2","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[]}}}]}`}
	if out, err := runFixture(t, f, "allow", "cdn.example.com", "--org", "1", "--note", "T"); err != nil || strings.Contains(out, "categorization is unavailable") {
		t.Fatalf("a usable own record was treated as unknown: err=%v\n%s", err, out)
	}
}

// Finding 5: the assignment fallback (no query-log hit) wrote to every
// editable assigned policy, including one that already allows the domain.
func TestR17AllowAssignmentTargetsOnlyBlockingPolicies(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK, allowlisted: map[string]bool{"10": true}}
	base := f.handler(t)
	over := map[string]string{
		"/v1/networks/all": `{"data":[
			{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}}}},
			{"id":"51","attributes":{"name":"Annex"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"11"}}}},
			{"id":"52","attributes":{"name":"Lab"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"12"}}}}]}`,
		"/v1/policies/all": `{"data":[
			{"id":"10","attributes":{"name":"Blocks","can_edit":true,"is_global_policy":false,"whitelist_domains":[],"allow_list_only":false,"blacklist_domains":["example.com"],"blacklist_categories":[],"allow_unknown_domains":true}},
			{"id":"11","attributes":{"name":"Allows","can_edit":true,"is_global_policy":false,"whitelist_domains":[],"allow_list_only":false,"blacklist_domains":[],"blacklist_categories":[],"allow_unknown_domains":true}},
			{"id":"12","attributes":{"name":"Unknown","can_edit":true,"is_global_policy":false,"whitelist_domains":[],"allow_list_only":null,"blacklist_domains":[],"blacklist_categories":[],"allow_unknown_domains":true}}]}`,
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := over[r.URL.Path]; ok && r.Method == http.MethodGet {
			if p := r.URL.Query().Get("page[number]"); p != "" && p != "1" {
				fmt.Fprint(w, `{"data":[]}`)
				return
			}
			fmt.Fprint(w, body)
			return
		}
		base.ServeHTTP(w, r)
	})
	run := r17Runner(t, h)
	out, errOut, err := run("allow", "example.com", "--org", "1", "--note", "T", "--confirm", "--json")
	if err != nil {
		t.Fatalf("allow: %v\n%s%s", err, out, errOut)
	}
	m := r17JSON(t, "allow", out)
	actions := map[string]string{}
	targets, _ := m["targets"].([]any)
	for _, it := range targets {
		tg, _ := it.(map[string]any)
		id, _ := tg["policy_id"].(string)
		actions[id], _ = tg["action"].(string)
	}
	f.mu.Lock()
	writes := append([]string(nil), f.writes...)
	f.mu.Unlock()
	if m["attribution"] != "assignment" || actions["10"] != "add" || actions["11"] != "noop" || actions["12"] != "review" {
		t.Fatalf("actions=%v attribution=%v; want 10 add (blocks), 11 noop (allows), 12 review (unknown)\n%s", actions, m["attribution"], out)
	}
	if strings.Join(writes, ",") != "/v1/policies/10/add_whitelist_domain" {
		t.Fatalf("writes = %v; only the policy that blocks the domain may be written", writes)
	}
}

// Finding 6: whitelist_domains and allow_unknown_domains null produced a
// clean audit with no note.
func TestR17PolicyAuditUnknownAttributesNotClean(t *testing.T) {
	cats := []jaRes{ja(t, `{"id":"60","attributes":{"name":"Malware","security":true}}`)}
	nullAttrs := ja(t, `{"id":"10","attributes":{"name":"Nulls","whitelist_domains":null,"allow_unknown_domains":null,"allow_list_only":false,"blacklist_categories":[60]}}`)
	found := ""
	for _, f := range auditPolicies([]jaRes{nullAttrs}, cats, nil, nil) {
		if f.PolicyID == "10" && f.Kind == "policy_attributes_unknown" {
			found = f.Detail
		}
	}
	if !strings.Contains(found, "whitelist_domains") || !strings.Contains(found, "allow_unknown_domains") {
		t.Fatalf("a policy with null whitelist_domains and allow_unknown_domains audited clean (finding %q)", found)
	}
	clean := ja(t, `{"id":"11","attributes":{"name":"Clean","whitelist_domains":[],"allow_unknown_domains":false,"allow_list_only":false,"blacklist_categories":[60]}}`)
	if got := auditPolicies([]jaRes{clean}, cats, nil, nil); len(got) != 0 {
		t.Fatalf("a fully readable compliant policy produced findings %+v", got)
	}
	// End to end, the note says it is not an all-clear.
	run := r17Runner(t, r17Routes(t, map[string]string{
		"/v1/categories/all": `{"data":[{"id":"60","attributes":{"name":"Malware","security":true}}]}`,
		"/v1/policies/all":   `{"data":[{"id":"10","attributes":{"name":"Nulls","is_global_policy":false,"whitelist_domains":null,"allow_unknown_domains":null,"allow_list_only":false,"blacklist_categories":[60]}}]}`,
	}))
	out, _, err := run("policy-audit", "--json")
	if err != nil {
		t.Fatalf("policy-audit: %v", err)
	}
	if m := r17JSON(t, "policy-audit", out); !strings.Contains(fmt.Sprint(m["note"]), "not an all-clear") || len(m["findings"].([]any)) == 0 {
		t.Fatalf("unknown attributes left a clean audit:\n%s", out)
	}
}

// Finding 7: lookup coverage compared counts, so a record for a host nobody
// asked about stood in for a requested domain that returned nothing.
func TestR17PolicyAuditLookupCoverageByMembership(t *testing.T) {
	run := r17Runner(t, r17Routes(t, map[string]string{
		"/v1/categories/all":      `{"data":[{"id":"60","attributes":{"name":"Malware","security":true}}]}`,
		"/v1/policies/all":        `{"data":[{"id":"10","attributes":{"name":"Std","is_global_policy":false,"whitelist_domains":["a.example","b.example"],"allow_unknown_domains":false,"allow_list_only":false,"blacklist_categories":[60]}}]}`,
		"/v1/domains/bulk_lookup": `{"data":[{"id":"1","attributes":{"host":"a.example"},"relationships":{"categories":{"data":[]}}},{"id":"3","attributes":{"host":"c.example"},"relationships":{"categories":{"data":[]}}}]}`,
	}))
	out, _, err := run("policy-audit", "--json")
	if err != nil {
		t.Fatalf("policy-audit: %v", err)
	}
	m := r17JSON(t, "policy-audit", out)
	if m["allowlist_domains_categorized"] != float64(1) || !strings.Contains(fmt.Sprint(m["note"]), "1 allow-list domains returned no usable lookup record") {
		t.Fatalf("b.example was never looked up, yet coverage looked complete:\n%s", out)
	}
}

// Finding 8: a policy whose whitelist_domains is null and blacklist_domains
// omitted counted toward "No synced policy lists <domain> or a parent domain."
func TestR17ListsWhereUnreadablePolicyListsNotAllClear(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db, map[string][]string{
		"organizations": {mirrorOrg},
		"networks":      {mirrorSite},
		"user-agents":   {mirrorAgent()},
		"policies": {mirrorPolicy,
			`{"id":"12","attributes":{"name":"Broken","is_global_policy":false,"whitelist_domains":null},"relationships":{"organization":{"data":{"id":"1"}}}}`},
	}, map[string]string{"organizations": "complete", "networks": "complete", "user-agents": "complete", "policies": "complete", "scheduled-policies": "complete"})
	human, _ := run("lists", "where", "nothing.test", "--db", db, "--human-friendly")
	if strings.Contains(human, "No synced policy lists") || !strings.Contains(human, "policy ids 12") {
		t.Fatalf("an unreadable policy list counted toward the all-clear:\n%s", human)
	}
	out, _ := run("lists", "where", "nothing.test", "--db", db, "--json")
	if out == "[]\n" || !strings.Contains(out, `"unreadable_policies"`) {
		t.Fatalf("--json returned a clean empty list over an unreadable policy:\n%s", out)
	}
}

// Finding 9: coverage read from /v2/unblock_requests/A was stored under A
// even when the response was request B's.
func TestR17UnblocksCoverageMustBeTheRequestedRecord(t *testing.T) {
	for _, c := range []struct {
		returnedID string
		allowed    bool
		hint       string
	}{{"B", false, "review"}, {"A", true, "resolve-already-allowed"}} {
		run := r17Runner(t, r17Routes(t, map[string]string{
			"/v2/unblock_requests":   `{"data":[{"id":"A","attributes":{"domain":"example.com","category":{"is_threat":false}}}]}`,
			"/v2/unblock_requests/A": `{"data":{"id":"` + c.returnedID + `","attributes":{"already_covered":{"covered":true,"list":"policy"}}}}`,
		}))
		out, _, err := run("unblocks", "triage", "--json")
		if err != nil {
			t.Fatalf("triage: %v", err)
		}
		groups, _ := r17JSON(t, "triage", out)["groups"].([]any)
		g, _ := groups[0].(map[string]any)
		if g["already_allowed"] != c.allowed || g["hint"] != c.hint {
			t.Fatalf("response id %s: already_allowed=%v hint=%v, want %v/%s", c.returnedID, g["already_allowed"], g["hint"], c.allowed, c.hint)
		}
	}
}

// Finding 10: strs() dropped invalid slots before the length check, so a
// 673-entry policy_ids with a leading null passed as 672 with every later
// slot shifted by one.
func TestR17ScheduledPolicyRawSlotsValidated(t *testing.T) {
	at, _ := time.Parse(time.RFC3339, "2026-10-12T09:30:00Z") // Monday slot 38 in UTC
	sched := func(ids []any) *jaRes {
		raw, _ := json.Marshal(map[string]any{"id": "300", "attributes": map[string]any{"name": "S", "timezone": "UTC", "policy_ids": ids}})
		r := ja(t, string(raw))
		return &r
	}
	slots := func(n int, fill any) []any {
		ids := make([]any, n)
		for i := range ids {
			ids[i] = fill
		}
		return ids
	}
	shifted := append([]any{nil}, slots(672, "10")...)
	shifted[39] = "11" // raw slot 38 is "10"; with the null dropped, "11" lands on slot 38
	nullElsewhere := slots(672, "10")
	nullElsewhere[5] = nil
	boolSlot := slots(672, "10")
	boolSlot[5] = true
	fraction := slots(672, float64(10))
	fraction[5] = 10.5
	for name, sp := range map[string]*jaRes{"673 entries, leading null": sched(shifted), "null in another slot": sched(nullElsewhere), "boolean slot": sched(boolSlot), "fractional id": sched(fraction)} {
		if id, _, ok := scheduledPolicyNow(sp, at); ok {
			t.Errorf("%s resolved to %q; an invalid raw list must leave the slot unresolved", name, id)
		}
	}
	zeroElsewhere := slots(672, float64(10))
	zeroElsewhere[5] = float64(0)
	if id, _, ok := scheduledPolicyNow(sched(zeroElsewhere), at); !ok || id != "10" {
		t.Errorf("672 numeric ids with a 0 in another slot = %q, %v; want 10, true", id, ok)
	}
}

// Finding 11: --org dropped clients whose org membership could not be
// resolved (site missing from the mirror), which could read as an all-clear.
func TestR17StaleOrgMembershipUnknownReported(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	networks := []jaRes{
		ja(t, `{"id":"50","relationships":{"organization":{"data":{"id":"1"}}}}`),
		ja(t, `{"id":"51","relationships":{"organization":{"data":{"id":"2"}}}}`),
		ja(t, `{"id":"52","attributes":{"name":"no org link"}}`),
		ja(t, `{"id":"53","relationships":{"organization":{"data":{"id":"7"}}}}`),
	}
	orgs := []jaRes{ja(t, `{"id":"1","attributes":{"name":"Acme Dental"}}`), ja(t, `{"id":"2","attributes":{"name":"Bolt Legal"}}`)}
	client := func(id, rels string) jaRes {
		return ja(t, `{"id":"`+id+`","attributes":{"hostname":"PC-`+id+`","agent_state":"protected","status":"active","last_sync":"2026-10-14T11:00:00Z"},"relationships":{`+rels+`}}`)
	}
	agents := []jaRes{
		client("in", `"network":{"data":{"id":"50"}}`),
		client("out", `"network":{"data":{"id":"51"}}`),
		client("missing-site", `"network":{"data":{"id":"99"}}`),
		client("no-site", ``),
		client("unlinked-site", `"network":{"data":{"id":"52"}}`),
		client("missing-org", `"network":{"data":{"id":"53"}}`),
	}
	rows, scanned := computeStale(agents, networks, orgs, "acme", 7*24*time.Hour, now)
	unknown := map[string]bool{}
	for _, r := range rows {
		for _, a := range r.Agents {
			if a.Reason == "membership_unknown" {
				unknown[a.ID] = true
			}
			if a.ID == "out" || a.ID == "in" {
				t.Errorf("healthy client %s with known membership was reported: %+v", a.ID, a)
			}
		}
	}
	if len(unknown) != 4 || !unknown["missing-site"] || !unknown["no-site"] || !unknown["unlinked-site"] || !unknown["missing-org"] || scanned != 5 {
		t.Fatalf("membership_unknown = %v scanned=%d rows=%+v; every client --org cannot place must be reported", unknown, scanned, rows)
	}
	// Without --org nothing changes: the healthy fleet is clean.
	if rows, _ := computeStale(agents, networks, orgs, "", 7*24*time.Hour, now); len(rows) != 0 {
		t.Fatalf("no --org: rows = %+v, want none (all healthy)", rows)
	}
}

func TestR17DriftOrgMembershipUnknownReported(t *testing.T) {
	_, releases, networks, orgs := driftFixtures(t)
	agents := []jaRes{
		ja(t, `{"id":"a1","attributes":{"hostname":"CURRENT","agent_type":"windows","agent_version":"3.1.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`),
		ja(t, `{"id":"a2","attributes":{"hostname":"LOST","agent_type":"windows","agent_version":"3.0.4","status":"active"},"relationships":{"network":{"data":{"id":"99"}}}}`),
	}
	v := computeDrift(agents, releases, networks, orgs, driftOptions{Org: "acme"})
	raw, _ := json.Marshal(v)
	var m struct {
		Orgs []map[string]any `json:"orgs"`
	}
	_ = json.Unmarshal(raw, &m)
	if len(m.Orgs) != 1 || m.Orgs[0]["membership_unknown"] != float64(1) || m.Orgs[0]["behind_latest"] != float64(1) {
		t.Fatalf("drift --org dropped a client whose site is missing: orgs=%v", m.Orgs)
	}
}

// Finding 14: "999unknown" compared by its leading digits (999) and read as
// ahead of every release.
func TestR17DriftIncomparableVersionIsUnknown(t *testing.T) {
	_, releases, networks, orgs := driftFixtures(t)
	for _, ver := range []string{"999unknown", "3.x", "3..1", "v", "3.1.0-", "3.1.0+"} {
		agents := []jaRes{ja(t, `{"id":"a1","attributes":{"hostname":"ODD","agent_type":"windows","agent_version":"`+ver+`","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`)}
		v := computeDrift(agents, releases, networks, orgs, driftOptions{BadVersion: "3.1.0"})
		if len(v.Orgs) != 1 || v.Orgs[0].VersionUnknown != 1 || v.Orgs[0].OnBadVersion != 0 {
			t.Errorf("version %q: orgs = %+v; an incomparable version is version_unknown, never current", ver, v.Orgs)
		}
	}
	// An in-channel release with an incomparable version may be the latest.
	odd := append(append([]jaRes{}, releases...), ja(t, `{"agent_type":"windows","agent_version":"4.x-build","release_channels":[{"value":"stable"}]}`))
	agents := []jaRes{ja(t, `{"id":"a1","attributes":{"hostname":"PC","agent_type":"windows","agent_version":"3.1.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`)}
	if v := computeDrift(agents, odd, networks, orgs, driftOptions{}); v.Latest["windows"] != "" || len(v.Orgs) != 1 || v.Orgs[0].UnknownLatest != 1 || !strings.Contains(v.Note, "cannot be compared") {
		t.Errorf("an unparseable release version left latest=%q orgs=%+v note=%q", v.Latest["windows"], v.Orgs, v.Note)
	}
}

// Finding 15: a schedule with one usable id and 671 nulls (or a 0 slot)
// counted as resolved.
func TestR17ListsWhereScheduleNeedsEverySlot(t *testing.T) {
	policies := []jaRes{ja(t, `{"id":"10","attributes":{"name":"Std","whitelist_domains":["example.com"],"blacklist_domains":[]}}`)}
	networks := []jaRes{ja(t, `{"id":"50","relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"7"}}}}`)}
	ids := func(first string, rest string) string {
		s := make([]string, 672)
		for i := range s {
			s[i] = rest
		}
		s[0] = first
		return "[" + strings.Join(s, ",") + "]"
	}
	for name, slotJSON := range map[string]string{
		"one id, 671 nulls": ids("10", "null"),
		"a 0 slot":          ids("0", "10"),
		"671 entries":       "[" + strings.TrimSuffix(strings.Repeat("10,", 671), ",") + "]",
	} {
		sched := []jaRes{ja(t, `{"id":"7","attributes":{"name":"S","timezone":"UTC","policy_ids":`+slotJSON+`}}`)}
		if _, unresolved := computeListsWhere("example.com", policies, nil, networks, nil, sched); unresolved["7"] != 1 {
			t.Errorf("%s: unresolved = %v; any unusable slot leaves the schedule unresolved", name, unresolved)
		}
	}
	sched := []jaRes{ja(t, `{"id":"7","attributes":{"name":"S","timezone":"UTC","policy_ids":`+scheduleSlots(10, 10)+`}}`)}
	if rows, unresolved := computeListsWhere("example.com", policies, nil, networks, nil, sched); len(unresolved) != 0 || len(rows) != 1 || rows[0].Sites != 1 {
		t.Errorf("a fully resolved schedule: rows=%+v unresolved=%v", rows, unresolved)
	}
}

// exportServer serves /v1/policies/all: page 1 is a full page, page 2 fails
// unless ok. It records each include_global_policies value it saw.
type exportServer struct {
	mu     sync.Mutex
	ok     bool
	global []string
}

func (s *exportServer) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/policies/all" {
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.global = append(s.global, r.URL.Query().Get("include_global_policies"))
		switch r.URL.Query().Get("page[number]") {
		case "1":
			items := make([]string, dnsfilterPageSize)
			for i := range items {
				items[i] = fmt.Sprintf(`{"id":"%d","attributes":{"name":"P%d"}}`, i+1, i+1)
			}
			fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(items, ","))
		case "2":
			if !s.ok {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"page 2 failed"}`)
				return
			}
			fmt.Fprint(w, `{"data":[{"id":"900","attributes":{"name":"Last"}}]}`)
		default:
			fmt.Fprint(w, `{"data":[]}`)
		}
	})
}

// Finding 12: openExportOutput truncated an existing --output file before
// the first request succeeded, so a failed export destroyed the backup.
func TestR17ExportFailureKeepsPreviousOutput(t *testing.T) {
	s := &exportServer{}
	run := r17Runner(t, s.handler(t))
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.jsonl")
	if err := os.WriteFile(path, []byte("previous backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("export", "policies", "--output", path, "--no-cache"); err == nil {
		t.Fatal("export with a failing page 2 succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "previous backup\n" {
		t.Fatalf("a failed export changed the existing file: %q (%v)", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("a failed export left files behind: %v", entries)
	}
	s.mu.Lock()
	s.ok = true
	s.mu.Unlock()
	if _, errOut, err := run("export", "policies", "--output", path, "--no-cache"); err != nil {
		t.Fatalf("export: %v\n%s", err, errOut)
	}
	got, _ = os.ReadFile(path)
	if lines := strings.Count(string(got), "\n"); lines != dnsfilterPageSize+1 || strings.Contains(string(got), "previous backup") {
		t.Fatalf("a successful export did not replace the file: %d lines", lines)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("a successful export left a temp file behind: %v", entries)
	}
}

// Finding 13: export policies never sent include_global_policies=true, so
// global (MSP-shared) policies were silently left out.
func TestR17ExportPoliciesIncludesGlobal(t *testing.T) {
	s := &exportServer{ok: true}
	run := r17Runner(t, s.handler(t))
	if _, errOut, err := run("export", "policies", "--no-cache"); err != nil {
		t.Fatalf("export: %v\n%s", err, errOut)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.global) == 0 {
		t.Fatal("export never listed policies")
	}
	for i, v := range s.global {
		if v != "true" {
			t.Fatalf("request %d sent include_global_policies=%q, want true on every page", i+1, v)
		}
	}
}

// ---- siblings of the round-17 findings (same absent-is-unknown class) ----

// planAllowTargets read an absent or null whitelist_domains as [] and planned
// "add: domain not on this policy's allow list".
func TestR17PlanAllowSkipsUnreadableAllowList(t *testing.T) {
	policies := map[string]jaRes{
		"10": ja(t, `{"id":"10","attributes":{"name":"Null list","is_global_policy":false,"can_edit":true,"whitelist_domains":null}}`),
		"11": ja(t, `{"id":"11","attributes":{"name":"No list","is_global_policy":false,"can_edit":true}}`),
		"12": ja(t, `{"id":"12","attributes":{"name":"Readable","is_global_policy":false,"can_edit":true,"whitelist_domains":[]}}`),
	}
	got := map[string]string{}
	for _, tg := range planAllowTargets("example.com", policies, map[string][]string{"10": {"site A"}, "11": {"site B"}, "12": {"site C"}}, false) {
		got[tg.PolicyID] = tg.Action
	}
	if got["10"] != "skip" || got["11"] != "skip" || got["12"] != "add" {
		t.Fatalf("actions = %v; an unreadable allow list must never be planned as missing the domain", got)
	}
}

// groupUnblockRequests read a missing is_threat as "not a threat" and a
// non-boolean covered as "not covered" (but checked).
func TestR17TriageUnknownThreatAndCoverage(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	reqs := []jaRes{
		ja(t, `{"id":"u1","attributes":{"domain":"maybe-evil.example","category":{"is_threat":null}}}`),
		ja(t, `{"id":"u2","attributes":{"domain":"no-category.example"}}`),
		ja(t, `{"id":"u3","attributes":{"domain":"odd-coverage.example","category":{"is_threat":false}}}`),
	}
	coverage := map[string]map[string]any{"u1": {"covered": true, "list": "policy"}, "u2": {"covered": true, "list": "policy"}, "u3": {"covered": "yes"}}
	by := map[string]triageGroup{}
	for _, g := range groupUnblockRequests(reqs, coverage, now) {
		by[g.Domain] = g
	}
	for _, d := range []string{"maybe-evil.example", "no-category.example"} {
		if g := by[d]; g.Hint != "review" {
			t.Errorf("%s: hint %q; an unknown threat status must keep the group at review, never resolve-already-allowed", d, g.Hint)
		}
	}
	if g := by["odd-coverage.example"]; g.Unchecked != 1 || g.Hint != "review" {
		t.Errorf("odd-coverage.example: unchecked=%d hint=%q; a non-boolean covered is unchecked", g.Unchecked, g.Hint)
	}
}

// A scheduled-policy read answering for another schedule was used as this one.
func TestR17WhyBlockedScheduleMustBeTheRequestedOne(t *testing.T) {
	allowing := `{"data":[{"id":"10","attributes":{"name":"Std","whitelist_domains":[],"allow_list_only":false,"blacklist_domains":[],"blacklist_categories":[],"allow_unknown_domains":true}}]}`
	for _, c := range []struct{ id, decision string }{{`"id":"301",`, "unknown"}, {`"id":"300",`, "not_blocked"}, {``, "not_blocked"}} {
		sched := `{` + c.id + `"name":"Always 10","timezone":"UTC","policy_ids":` + scheduleSlots(10, 10) + `}`
		run := r17Runner(t, r17Routes(t, whyBlockedRoutes(sched, allowing, `{"data":{"values":[]}}`)))
		out, _, err := run("why-blocked", "example.com", "--org", "1", "--json")
		if err != nil {
			t.Fatalf("why-blocked: %v", err)
		}
		m := r17JSON(t, "why-blocked", out)
		deps, _ := m["deployments"].([]any)
		d, _ := deps[0].(map[string]any)
		v, _ := d["verdict"].(map[string]any)
		if want := map[string]string{"unknown": "unknown", "not_blocked": "allowed"}[c.decision]; v["decision"] != want {
			t.Fatalf("schedule response %q: deployment verdict %v, want %s\n%s", c.id, v["decision"], want, out)
		}
	}
}

// A site that names no organization may belong to the org; it is not
// evaluated, so the aggregate cannot be not_blocked.
func TestR17WhyBlockedUnlinkedSiteIsAGap(t *testing.T) {
	routes := whyBlockedRoutes(`{"name":"Always 10","timezone":"UTC","policy_ids":`+scheduleSlots(10, 10)+`}`,
		`{"data":[{"id":"10","attributes":{"name":"Std","whitelist_domains":[],"allow_list_only":false,"blacklist_domains":[],"blacklist_categories":[],"allow_unknown_domains":true}}]}`,
		`{"data":{"values":[]}}`)
	routes["/v1/networks/all"] = `{"data":[{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"300"}}}},{"id":"51","attributes":{"name":"Orphan"},"relationships":{"policy":{"data":{"id":"66"}}}}]}`
	run := r17Runner(t, r17Routes(t, routes))
	out, _, err := run("why-blocked", "example.com", "--org", "1", "--json")
	if err != nil {
		t.Fatalf("why-blocked: %v", err)
	}
	if m := r17JSON(t, "why-blocked", out); m["decision"] != "unknown" || m["blocked"] != nil {
		t.Fatalf("an unlinked site was ignored: decision=%v blocked=%v\n%s", m["decision"], m["blocked"], out)
	}
}

// The audit's de-duplication collapsed every policy without an id into one.
func TestR17PolicyAuditKeepsIdlessPolicies(t *testing.T) {
	p := `{"attributes":{"name":"%s","is_global_policy":false,"whitelist_domains":[],"allow_unknown_domains":false,"allow_list_only":false,"blacklist_categories":[60]}}`
	run := r17Runner(t, r17Routes(t, map[string]string{
		"/v1/categories/all": `{"data":[{"id":"60","attributes":{"name":"Malware","security":true}}]}`,
		"/v1/policies/all":   `{"data":[` + fmt.Sprintf(p, "A") + `,` + fmt.Sprintf(p, "B") + `]}`,
	}))
	out, _, err := run("policy-audit", "--json")
	if err != nil {
		t.Fatalf("policy-audit: %v", err)
	}
	if m := r17JSON(t, "policy-audit", out); m["policies_scanned"] != float64(2) {
		t.Fatalf("two id-less policies scanned as %v:\n%s", m["policies_scanned"], out)
	}
}

// A --baseline policy with absent flags compared as false on both sides.
func TestR17PolicyAuditBaselineUnknownFlags(t *testing.T) {
	cats := []jaRes{ja(t, `{"id":"60","attributes":{"name":"Malware","security":true}}`)}
	p := ja(t, `{"id":"10","attributes":{`+auditFullAttrs+`,"name":"P","blacklist_categories":[60]}}`)
	baseline := ja(t, `{"id":"20","attributes":{"name":"Base","blacklist_categories":[60]}}`)
	found := false
	for _, f := range auditPolicies([]jaRes{p}, cats, nil, &baseline) {
		if f.PolicyID == "20" && f.Kind == "baseline_attributes_unknown" && strings.Contains(f.Detail, "google_safesearch") {
			found = true
		}
	}
	if !found {
		t.Fatal("a --baseline with unreadable flags reported no drift and no gap")
	}
}

// A roaming client inheriting from a site the mirror lacks was silently
// left out of every impact count.
func TestR17ListsWhereUnplacedClientNotAllClear(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	lost := `{"id":"a9","attributes":{"hostname":"LOST","status":"active"},"relationships":{"network":{"data":{"id":"99"}}}}`
	seedMirror(t, db, map[string][]string{
		"organizations": {mirrorOrg}, "networks": {mirrorSite}, "user-agents": {mirrorAgent(), lost}, "policies": {mirrorPolicy},
	}, map[string]string{"organizations": "complete", "networks": "complete", "user-agents": "complete", "policies": "complete", "scheduled-policies": "complete"})
	out, _ := run("lists", "where", "example.com", "--db", db, "--json")
	if !strings.Contains(out, `"unplaced_roaming_clients": 1`) {
		t.Fatalf("a client on a missing site left the impact counts looking complete:\n%s", out)
	}
}

// --bad-version accepted a version compareSemver cannot order.
func TestR17DriftRejectsIncomparableBadVersion(t *testing.T) {
	_, err := runFixture(t, &fixtureAPI{}, "agents", "drift", "--bad-version", "3.1.x")
	if exitCode(err) != 2 {
		t.Fatalf("--bad-version 3.1.x: exit %d (%v), want 2", exitCode(err), err)
	}
}
