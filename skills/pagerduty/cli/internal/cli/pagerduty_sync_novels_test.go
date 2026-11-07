// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pagerduty handfix sync-populates-novel-tables: the store-backed novels read
// what a real `sync` writes. In the 4.24 library and the published tree the
// default sync filled none of the tables the novels query (the record schemas
// are allOf compositions, so the press saw no `id`), and the README's "sync
// once, then offline" answered "nothing synced" everywhere. This test syncs
// from a fake PagerDuty REST API with the documented list envelope
// ({<plural>:[...],limit,offset,more,total}; developer.pagerduty.com,
// Pagination) and runs each novel on the result. On-call entries have no id
// and a permanent on-call has null start and end (List all of the on-calls):
// two people on call at one level over the same span, and a permanent
// on-call, must all survive into the store.

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cliutil/testenv"

	_ "modernc.org/sqlite"
)

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

func (f *fakePagerDuty) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func runPagerDuty(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func mustRunPagerDuty(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, err := runPagerDuty(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out, errOut)
	}
	return out
}

func countRows(t *testing.T, db, table string) int {
	t.Helper()
	h, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM "` + table + `"`).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestNovelsReadWhatSyncWrites(t *testing.T) {
	testenv.Isolate(t)
	newFakePagerDuty(t)
	if _, errOut, err := runPagerDuty(t, "sync", "--no-learn"); err != nil {
		t.Fatalf("sync: %v\n%s", err, errOut)
	}
	db := pdStorePath()
	for table, want := range map[string]int{"services": 2, "escalation_policies": 1, "schedules": 1, "oncalls": 3,
		"incidents": 2, "log_entries": 3, "change_events": 1} {
		if n := countRows(t, db, table); n != want {
			t.Errorf("%s has %d rows after sync, want %d", table, n, want)
		}
	}

	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"oncall", "who"}, []string{"PUSR001", "PUSR002", "PUSR003"}},
		{[]string{"oncall", "hours"}, []string{"PUSR001", "PUSR002"}},
		{[]string{"pulse"}, []string{"Checkout API"}},
		{[]string{"insights", "noisy"}, []string{"Checkout API"}},
		{[]string{"insights", "mttr"}, []string{"Checkout API"}},
		{[]string{"insights", "responders"}, []string{"Ana Ruiz"}},
		{[]string{"insights", "stale"}, []string{"PINC001"}},
		{[]string{"incidents", "timeline", "PINC001"}, []string{`"type": "trigger"`, `"by": "Checkout API"`}},
		{[]string{"incidents", "changes", "PINC001"}, []string{"Deploy checkout v2.14"}},
		{[]string{"audit", "coverage"}, []string{"Ledger Batch"}},
		{[]string{"audit", "schedule-gaps"}, []string{"PSCH001"}},
	} {
		out := mustRunPagerDuty(t, append(c.args, "--json", "--no-learn")...)
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v after a real sync: missing %q in\n%s", c.args, w, out)
			}
		}
	}
}

// The on-call key is stable across syncs (a re-sync updates rows, never
// duplicates them) and differs for each person, level and span.
func TestOncallKeyIdentity(t *testing.T) {
	entry := func(user, level, start string) map[string]any {
		var s any = start
		if start == "" {
			s = nil
		}
		return map[string]any{"escalation_policy": map[string]any{"id": "PEP0001"}, "escalation_level": level,
			"schedule": map[string]any{"id": "PSCH001"}, "user": map[string]any{"id": user}, "start": s, "end": nil}
	}
	a := oncallKey(entry("PUSR001", "1", "2026-10-29T00:00:00Z"))
	if a != oncallKey(entry("PUSR001", "1", "2026-10-29T00:00:00Z")) {
		t.Fatal("the same entry got two keys")
	}
	for _, other := range []map[string]any{entry("PUSR002", "1", "2026-10-29T00:00:00Z"), entry("PUSR001", "2", "2026-10-29T00:00:00Z"),
		entry("PUSR001", "1", "2026-10-30T00:00:00Z"), entry("PUSR001", "1", "")} {
		if oncallKey(other) == a {
			t.Fatalf("%v shares a key with another entry", other)
		}
	}
	// A body that is not the documented envelope passes through unchanged.
	for _, b := range []string{`[]`, `{"oncalls":"x"}`, `not json`, `{"oncalls":[]} {}`} {
		if got := string(stampOncallBody([]byte(b))); got != b {
			t.Errorf("stampOncallBody(%q) = %q, want it unchanged", b, got)
		}
	}
	// An entry that already carries a key keeps it.
	if got := string(stampOncallBody([]byte(`{"oncalls":[{"pp_oncall_key":"k1"}]}`))); got != `{"oncalls":[{"pp_oncall_key":"k1"}]}` {
		t.Errorf("a keyed entry was rewritten: %s", got)
	}
}
