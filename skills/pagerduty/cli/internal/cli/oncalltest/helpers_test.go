// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package oncalltest drives the on-call commands through sync and `oncall who`
// end to end. These tests live apart from internal/cli so each package stays
// inside go test's default time limit. Hand-written; a reprint never
// regenerates it.
package oncalltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cli"
	"pagerduty-pp-cli/internal/client"
	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// The fake PagerDuty is a plain-http httptest server, so the client's outbound
// guard (the kit's transport, which refuses a credential over http) is
// replaced here, as internal/cli's own tests do in credkit_seam_test.go.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunSandboxed(m))
}

func pdRef(typ, id, summary string) string {
	return fmt.Sprintf(`{"id":%q,"type":%q,"summary":%q}`, id, typ, summary)
}

func pdOncallJSON(epID, level, schedID, userID, start, end string) string {
	sched := "null"
	if schedID != "" {
		sched = pdRef("schedule_reference", schedID, "Schedule "+schedID)
	}
	q := func(s string) string {
		if s == "" {
			return "null"
		}
		return `"` + s + `"`
	}
	return `{"escalation_policy":` + pdRef("escalation_policy_reference", epID, "Policy "+epID) + `,"escalation_level":` + level +
		`,"schedule":` + sched + `,"user":` + pdRef("user_reference", userID, "User "+userID) + `,"start":` + q(start) + `,"end":` + q(end) + `}`
}

func pdSetBody(f *fakePagerDuty, p, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[p] = body
}

// fakePagerDuty serves the documented list envelope for every path: the
// records in bodies on the first page, an empty list elsewhere and on every
// later page; 500 for the paths in fail; it records the paths it was asked.
type fakePagerDuty struct {
	mu     sync.Mutex
	bodies map[string]string
	fail   map[string]bool
	seen   []string
}

func newFakePagerDuty(t *testing.T) *fakePagerDuty {
	f := &fakePagerDuty{bodies: map[string]string{}, fail: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimSuffix(r.URL.Path, "/")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.seen = append(f.seen, p)
		if f.fail[p] {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"Internal Server Error","code":2000}}`)
			return
		}
		key := strings.ReplaceAll(path.Base(p), "-", "_")
		records := "[]"
		if off := r.URL.Query().Get("offset"); off == "" || off == "0" {
			if b, ok := f.bodies[p]; ok {
				records = b
			}
		}
		fmt.Fprintf(w, `{%q:%s,"limit":100,"offset":0,"more":false,"total":null}`, key, records)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PAGERDUTY_BASE_URL", srv.URL)
	t.Setenv("PAGERDUTY_API_KEY", "syncNovelsUnitKey-3f8a2c71")

	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	ref := func(typ, id, summary string) string {
		return fmt.Sprintf(`{"id":%q,"type":%q,"summary":%q}`, id, typ, summary)
	}
	ep := ref("escalation_policy_reference", "PEP0001", "Checkout EP")
	svc := ref("service_reference", "PSVC001", "Checkout API")
	sched := ref("schedule_reference", "PSCH001", "Checkout Primary")
	team := ref("team_reference", "PTEAM01", "Payments")
	f.bodies["/services"] = `[{"id":"PSVC001","type":"service","name":"Checkout API","summary":"Checkout API","status":"critical",` +
		`"escalation_policy":` + ep + `,"teams":[` + team + `],"incident_urgency_rule":{"type":"constant","urgency":"high"},` +
		`"last_incident_timestamp":"` + ts(-30*time.Hour) + `","created_at":"2025-01-02T10:00:00Z"},` +
		`{"id":"PSVC002","type":"service","name":"Ledger Batch","summary":"Ledger Batch","status":"active",` +
		`"escalation_policy":null,"teams":[],"created_at":"2025-01-02T10:00:00Z"}]`
	f.bodies["/escalation_policies"] = `[{"id":"PEP0001","type":"escalation_policy","name":"Checkout EP","summary":"Checkout EP",` +
		`"num_loops":0,"escalation_rules":[{"id":"PRULE01","escalation_delay_in_minutes":30,"targets":[` + sched + `]}],` +
		`"services":[` + svc + `],"teams":[` + team + `]}]`
	f.bodies["/schedules"] = `[{"id":"PSCH001","type":"schedule","name":"Checkout Primary","summary":"Checkout Primary","time_zone":"UTC",` +
		`"escalation_policies":[` + ep + `],"users":[` + ref("user_reference", "PUSR001", "Ana Ruiz") + `,` +
		ref("user_reference", "PUSR002", "Ben Okafor") + `],"teams":[` + team + `]}]`
	oncall := func(user, level, start, end string) string {
		return `{"escalation_policy":` + ep + `,"escalation_level":` + level + `,"schedule":` + sched + `,"user":` + user +
			`,"start":` + start + `,"end":` + end + `}`
	}
	start, end := `"`+ts(-2*time.Hour)+`"`, `"`+ts(6*time.Hour)+`"`
	f.bodies["/oncalls"] = `[` +
		oncall(ref("user_reference", "PUSR001", "Ana Ruiz"), "1", start, end) + `,` +
		oncall(ref("user_reference", "PUSR002", "Ben Okafor"), "1", start, end) + `,` +
		oncall(ref("user_reference", "PUSR003", "Cleo Park"), "2", "null", "null") + `]`
	f.bodies["/incidents"] = `[{"id":"PINC001","type":"incident","incident_number":1201,"title":"Checkout 500s","summary":"[#1201] Checkout 500s",` +
		`"status":"triggered","urgency":"high","created_at":"` + ts(-30*time.Hour) + `","updated_at":"` + ts(-30*time.Hour) + `",` +
		`"last_status_change_at":"` + ts(-30*time.Hour) + `","service":` + svc + `,"escalation_policy":` + ep + `,"teams":[` + team + `],` +
		`"assignments":[{"at":"` + ts(-30*time.Hour) + `","assignee":` + ref("user_reference", "PUSR001", "Ana Ruiz") + `}],` +
		`"acknowledgements":[],"priority":null},` +
		`{"id":"PINC002","type":"incident","incident_number":1202,"title":"Checkout latency","summary":"[#1202] Checkout latency",` +
		`"status":"resolved","urgency":"high","created_at":"` + ts(-50*time.Hour) + `","updated_at":"` + ts(-49*time.Hour) + `",` +
		`"last_status_change_at":"` + ts(-49*time.Hour) + `","resolved_at":"` + ts(-49*time.Hour) + `","service":` + svc + `,` +
		`"escalation_policy":` + ep + `,"teams":[` + team + `],"assignments":[],"acknowledgements":[],"priority":null}]`
	f.bodies["/log_entries"] = `[{"id":"PLOG001","type":"trigger_log_entry","summary":"Triggered through the API.","created_at":"` + ts(-30*time.Hour) + `",` +
		`"incident":` + ref("incident_reference", "PINC001", "[#1201] Checkout 500s") + `,"service":` + svc + `,` +
		`"channel":{"type":"api"},"agent":` + svc + `},` +
		`{"id":"PLOG002","type":"assign_log_entry","summary":"Assigned to Ana Ruiz.","created_at":"` + ts(-30*time.Hour+time.Minute) + `",` +
		`"incident":` + ref("incident_reference", "PINC001", "[#1201] Checkout 500s") + `,"service":` + svc + `,` +
		`"assignees":[` + ref("user_reference", "PUSR001", "Ana Ruiz") + `]},` +
		`{"id":"PLOG003","type":"acknowledge_log_entry","summary":"Acknowledged by Ana Ruiz.","created_at":"` + ts(-30*time.Hour+4*time.Minute) + `",` +
		`"incident":` + ref("incident_reference", "PINC002", "[#1202] Checkout latency") + `,"service":` + svc + `,` +
		`"agent":` + ref("user_reference", "PUSR001", "Ana Ruiz") + `,"channel":{"type":"website"}}]`
	f.bodies["/change_events"] = `[{"id":"PCHG001","type":"change_event","summary":"Deploy checkout v2.14","timestamp":"` + ts(-31*time.Hour) + `",` +
		`"services":[` + svc + `],"integration":` + ref("inbound_integration_reference", "PINT001", "GitHub") + `,"source":"github","links":[]}]`
	return f
}

func mustRunPagerDuty(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, err := runPagerDuty(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out, errOut)
	}
	return out
}

// runPagerDuty runs the CLI in process, as internal/cli's own tests do.
func runPagerDuty(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := cli.RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// pdOncallWhoResult is the part of `oncall who --json` these tests read.
type pdOncallWhoResult struct {
	Current []struct {
		UserID string `json:"user_id"`
	} `json:"current"`
	Next []struct {
		UserID string `json:"user_id"`
	} `json:"next"`
}

// pdPagedOncalls serves GET /oncalls from a function of its query (the
// documented offset envelope) and an empty list for every other path.
func pdPagedOncalls(t *testing.T) func(func(q map[string]string) (records string, more bool, status int)) {
	t.Helper()
	var mu sync.Mutex
	var answer func(q map[string]string) (string, bool, int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimSuffix(r.URL.Path, "/")
		key := strings.ReplaceAll(path.Base(p), "-", "_")
		if p != "/oncalls" {
			fmt.Fprintf(w, `{%q:[],"limit":100,"offset":0,"more":false,"total":null}`, key)
			return
		}
		q := map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		mu.Lock()
		defer mu.Unlock()
		records, more, status := answer(q)
		if status != 0 {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"error":{"message":"Internal Server Error","code":2000}}`)
			return
		}
		fmt.Fprintf(w, `{"oncalls":%s,"limit":100,"offset":0,"more":%t,"total":null}`, records, more)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PAGERDUTY_BASE_URL", srv.URL)
	t.Setenv("PAGERDUTY_API_KEY", "syncNovelsUnitKey-3f8a2c71")
	return func(f func(q map[string]string) (string, bool, int)) {
		mu.Lock()
		defer mu.Unlock()
		answer = f
	}
}

func pdWhoUserIDs(t *testing.T, args ...string) []string {
	t.Helper()
	out := mustRunPagerDuty(t, append(append([]string{"oncall", "who"}, args...), "--json", "--no-learn")...)
	var res pdOncallWhoResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("oncall who %v: not JSON: %v\n%s", args, err, out)
	}
	ids := []string{}
	for _, e := range append(res.Current, res.Next...) {
		ids = append(ids, e.UserID)
	}
	sort.Strings(ids)
	return ids
}
