// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the local-store and live
// novel commands' correctness classes (reprint 4.34.0 review). Each test seeds
// the exact store or API shape that produced a wrong answer and asserts the
// answer an MSP can act on.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"huntress-pp-cli/internal/cliutil/testenv"
	"huntress-pp-cli/internal/store"
)

// runCLI runs the root command and returns stdout, stderr and the error.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// runNovelObject runs a command whose JSON output is one object.
func runNovelObject(t *testing.T, args ...string) (map[string]any, string) {
	t.Helper()
	out, errOut, err := runCLI(t, append(args, "--json")...)
	if err != nil {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errOut)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("%v: output is not a JSON object: %v\n%s", args, err, out)
	}
	return obj, errOut
}

// seedStore opens a fresh store and upserts rows under each resource type.
func seedStore(t *testing.T, rows map[string][]string) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "data.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for rt, items := range rows {
		raw := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			raw = append(raw, json.RawMessage(it))
		}
		if _, _, err := st.UpsertBatch(rt, raw); err != nil {
			t.Fatalf("seeding %s: %v", rt, err)
		}
		if err := st.SaveSyncState(rt, "", len(items)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func hoursAgo(h float64) string {
	return time.Now().UTC().Add(-time.Duration(h * float64(time.Hour))).Format(time.RFC3339)
}

// Class: the local-store novels read every resource type sync writes an
// entity under. A standard (non-reseller) credential syncs the flat
// /v1/agents, /v1/organizations and /v1/incident_reports collections, so a
// command that reads only the reseller fan-out partitions answers "nothing".
func TestNovelsReadFlatSyncedResourceTypes(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"organizations":    {`{"id":7,"name":"Acme"}`},
		"agents":           {`{"id":1,"organization_id":7,"hostname":"ACME-01","last_callback_at":"` + hoursAgo(24*30) + `"}`},
		"incident-reports": {`{"id":11,"organization_id":7,"agent_id":1,"severity":"critical","status":"sent","sent_at":"` + hoursAgo(5) + `","indicator_types":["ransomware_canaries"]}`},
		// The same agent also arrives through a reseller partition: it must
		// be counted once.
		"accounts_agents": {`{"id":1,"organization_id":7,"hostname":"ACME-01","last_callback_at":"` + hoursAgo(24*30) + `"}`},
	})
	rows := runNovelJSON(t, "fleet-incidents", "--db", db)
	if len(rows) != 1 || rows[0]["organization_name"] != "Acme" {
		t.Fatalf("fleet-incidents over flat resource types = %v, want the one sent incident with its org name", rows)
	}
	sum, _ := runNovelObject(t, "fleet-summary", "--db", db)
	for k, want := range map[string]string{"organizations": "1", "agents": "1", "open_criticals": "1", "stale_agents": "1"} {
		if fmt.Sprint(sum[k]) != want {
			t.Fatalf("fleet-summary[%s] = %v, want %s (flat types read, duplicates counted once): %v", k, sum[k], want, sum)
		}
	}
	if rows := runNovelJSON(t, "canary-watch", "--db", db, "--window", "24h"); len(rows) != 1 {
		t.Fatalf("canary-watch over flat incident-reports = %v, want 1", rows)
	}
}

// Class: an aggregate built from a sync walk that did not finish says so.
func TestNovelsWarnWhenTheSyncWalkWasIncomplete(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"accounts_organizations": {`{"id":7,"name":"Acme"}`},
		"accounts_agents":        {`{"id":1,"organization_id":7}`},
	})
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSyncProgress("accounts_agents", "next-2", 1); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	_, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if !strings.Contains(errOut, "warning:") || !strings.Contains(errOut, "agents") {
		t.Fatalf("fleet-summary over an unfinished agents walk printed no partial-data warning; stderr: %q", errOut)
	}
}

// Class: remediations are a {total_count, has_more, items} envelope in the
// API; a decoder that only accepts an array reports zero.
func TestRemediationEnvelopeIsCounted(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"accounts_incident_reports": {
			`{"id":1,"organization_id":7,"status":"sent","severity":"high","sent_at":"` + hoursAgo(2) + `","remediations":{"total_count":1,"has_more":false,"items":[{"id":9,"status":"pending"}]}}`,
			`{"id":2,"organization_id":7,"status":"sent","severity":"high","sent_at":"` + hoursAgo(3) + `","remediations":{"total_count":12,"has_more":true,"items":[{"id":1},{"id":2}]}}`,
		},
	})
	det, _ := runNovelObject(t, "incident-detail", "1", "--db", db)
	if fmt.Sprint(det["remediation_count"]) != "1" {
		t.Fatalf("incident-detail remediation_count = %v, want 1: %v", det["remediation_count"], det)
	}
	if items, _ := det["remediations"].([]any); len(items) != 1 {
		t.Fatalf("incident-detail remediations = %v, want the one envelope item", det["remediations"])
	}
	det2, _ := runNovelObject(t, "incident-detail", "2", "--db", db)
	if fmt.Sprint(det2["remediation_count"]) != "12" || det2["remediations_complete"] != false {
		t.Fatalf("incident-detail for a truncated envelope = %v, want remediation_count 12 and remediations_complete false", det2)
	}
	rows := runNovelJSON(t, "fleet-incidents", "--db", db, "--sort", "age")
	got := map[string]string{}
	for _, r := range rows {
		got[fmt.Sprint(r["id"])] = fmt.Sprint(r["remediations_count"])
	}
	if got["1"] != "1" || got["2"] != "12" {
		t.Fatalf("fleet-incidents remediations_count = %v, want 1 and 12", got)
	}
}

// Class: billing reconciles one billing period, not the invoice history.
func TestBillingReconcileUsesTheLatestBillingPeriod(t *testing.T) {
	testenv.Isolate(t)
	agents := make([]string, 0, 100)
	for i := 1; i <= 100; i++ {
		agents = append(agents, fmt.Sprintf(`{"id":%d,"organization_id":7}`, i))
	}
	db := seedStore(t, map[string][]string{
		"accounts_invoices": {
			`{"id":1,"quantity":100,"status":"paid","created_at":"2026-08-01T00:00:00Z"}`,
			`{"id":2,"quantity":100,"status":"paid","created_at":"2026-09-01T00:00:00Z"}`,
		},
		"accounts_agents": agents,
	})
	got, _ := runNovelObject(t, "billing-reconcile", "--db", db)
	if fmt.Sprint(got["invoiced_seats"]) != "100" || fmt.Sprint(got["delta"]) != "0" {
		t.Fatalf("billing-reconcile = %v, want invoiced_seats 100 and delta 0 (two monthly invoices are not 200 seats)", got)
	}
}

// Class: drift identifies an agent the same way on both sides of a run.
func TestDriftKeepsLargeIDsStable(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"accounts_agents": {`{"id":1000000,"organization_id":7,"last_callback_at":"2026-10-01T00:00:00Z"}`},
	})
	_, _ = runNovelObject(t, "drift", "--db", db)
	second, _ := runNovelObject(t, "drift", "--db", db)
	if fmt.Sprint(second["added_count"]) != "0" || fmt.Sprint(second["removed_count"]) != "0" {
		t.Fatalf("second drift over an unchanged store = %v, want no adds or removes", second)
	}
}

// Class: a drift snapshot belongs to the database it was taken from.
func TestDriftSnapshotIsPerDatabase(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	mk := func(name, row string) string {
		p := filepath.Join(dir, name)
		st, err := store.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := st.UpsertBatch("accounts_agents", []json.RawMessage{json.RawMessage(row)}); err != nil {
			t.Fatal(err)
		}
		_ = st.SaveSyncState("accounts_agents", "", 1)
		_ = st.Close()
		return p
	}
	a := mk("a.db", `{"id":1,"organization_id":7}`)
	b := mk("b.db", `{"id":2,"organization_id":8}`)
	_, _ = runNovelObject(t, "drift", "--db", a)
	got, _ := runNovelObject(t, "drift", "--db", b)
	if got["had_prior"] != false || fmt.Sprint(got["removed_count"]) != "0" {
		t.Fatalf("first drift of b.db = %v, want had_prior false and nothing removed (a.db's snapshot is not b.db's)", got)
	}
}

// Class: a capped collection reports its true total and that it was cut.
func TestBlastRadiusReportsTruncation(t *testing.T) {
	testenv.Isolate(t)
	agents := make([]string, 0, 201)
	for i := 1; i <= 201; i++ {
		agents = append(agents, fmt.Sprintf(`{"id":%d,"organization_id":7,"hostname":"H%d","external_ip":"203.0.113.7"}`, i, i))
	}
	db := seedStore(t, map[string][]string{"accounts_agents": agents})
	got, errOut := runNovelObject(t, "blast-radius", "--indicator", "203.0.113.7", "--db", db)
	if fmt.Sprint(got["match_count"]) != "201" || got["truncated"] != true {
		t.Fatalf("blast-radius over 201 matching agents = match_count %v truncated %v, want 201 and true", got["match_count"], got["truncated"])
	}
	if !strings.Contains(errOut, "warning:") {
		t.Fatalf("blast-radius truncation printed no warning; stderr %q", errOut)
	}
}

// Class: an indicator is matched literally; LIKE metacharacters in it are
// not wildcards, and an IP matches only that IP.
func TestBlastRadiusMatchesIndicatorsLiterally(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"accounts_agents": {
		`{"id":1,"organization_id":7,"hostname":"srv_1"}`,
		`{"id":2,"organization_id":7,"hostname":"srvX1"}`,
		`{"id":3,"organization_id":7,"hostname":"a","external_ip":"10.0.0.1"}`,
		`{"id":4,"organization_id":7,"hostname":"b","external_ip":"10.0.0.15"}`,
	}})
	got, _ := runNovelObject(t, "blast-radius", "--indicator", "srv_1", "--db", db)
	if fmt.Sprint(got["match_count"]) != "1" {
		t.Fatalf("blast-radius srv_1 = %v, want only the srv_1 host (underscore is literal)", got["agents"])
	}
	got, _ = runNovelObject(t, "blast-radius", "--indicator", "10.0.0.1", "--db", db)
	if fmt.Sprint(got["match_count"]) != "1" {
		t.Fatalf("blast-radius 10.0.0.1 = %v, want only the 10.0.0.1 agent, not 10.0.0.15", got["agents"])
	}
}

// Class: a window or scope the CLI cannot read is refused, never widened or
// substituted.
func TestUnreadableWindowsAndScopesAreRefused(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"organizations": {`{"id":7,"name":"Acme"}`}})
	for _, args := range [][]string{
		{"signal-digest", "--since", "30m"},
		{"signal-digest", "--since", "inf"},
		{"canary-watch", "--window", "lastweek"},
		{"handoff", "--since", "garbage"},
		{"mttr", "--since", "5x"},
		{"fleet-incidents", "--org", "-1"},
		{"signal-digest", "--org", "abc"},
		{"escalation-queue", "--org", "7x"},
		{"incident-detail", "12x"},
		{"org-scorecard", "7x"},
		{"stale-agents", "--days", "-1"},
		{"triage-age", "--buckets", "4,x,72"},
	} {
		if _, _, err := runCLI(t, append(args, "--db", db, "--json")...); err == nil {
			t.Errorf("%v was accepted; unreadable input must be refused", args)
		}
	}
	// The other direction: well-formed input still runs.
	for _, args := range [][]string{
		{"signal-digest", "--since", "12h", "--org", "7"},
		{"signal-digest", "--since", "all"},
		{"canary-watch", "--window", "all"},
		{"canary-watch", "--window", "2w"},
		{"handoff", "--since", "1.5d"},
		{"mttr", "--since", "30d", "--group-by", "severity"},
		{"fleet-incidents", "--org", "7"},
		{"escalation-queue", "--org", "7"},
		{"org-scorecard", "7"},
		{"org-scorecard", "--org", "7"},
		{"stale-agents", "--days", "0"},
		{"triage-age", "--buckets", "72,4,24"},
	} {
		if _, errOut, err := runCLI(t, append(args, "--db", db, "--json")...); err != nil {
			t.Errorf("%v was refused: %v (%s)", args, err, errOut)
		}
	}
}

// reseller fixture: account 7 has two agents across two pages; subscriptions
// are configurable per test.
func resellerServer(t *testing.T, subsPages []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		tok := r.URL.Query().Get("page_token")
		switch r.URL.Path {
		case "/v1/accounts":
			_, _ = w.Write([]byte(`{"accounts":[{"id":7,"name":"Acme","status":"active"}],"pagination":{}}`))
		case "/v1/reseller/subscriptions":
			i := 0
			if tok != "" {
				fmt.Sscanf(tok, "p%d", &i)
			}
			_, _ = w.Write([]byte(subsPages[i]))
		case "/v1/accounts/7/agents":
			if tok == "" {
				_, _ = w.Write([]byte(`{"agents":[{"id":1}],"pagination":{"next_page_token":"second"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"agents":[{"id":2}],"pagination":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUNTRESS_BASE_URL", srv.URL)
	t.Setenv("HUNTRESS_API_KEY", "hk_test_key_0001")
	t.Setenv("HUNTRESS_API_SECRET", "test-secret-value-0001")
	return srv
}

func resellerRow(t *testing.T, args ...string) map[string]any {
	t.Helper()
	got, _ := runNovelObject(t, append([]string{"reseller-rollup", "--no-cache"}, args...)...)
	accts, _ := got["accounts"].([]any)
	if len(accts) != 1 {
		t.Fatalf("reseller-rollup accounts = %v", got)
	}
	return accts[0].(map[string]any)
}

// Class: a collection's size is counted from the collection, never from the
// length of one page.
func TestResellerRollupCountsEveryAgentPage(t *testing.T) {
	testenv.Isolate(t)
	resellerServer(t, []string{`{"subscriptions":[{"id":1,"account":{"id":7},"product":"edr","status":"active","minimum_usage":2}],"pagination":{}}`})
	row := resellerRow(t)
	if fmt.Sprint(row["deployed_agents"]) != "2" {
		t.Fatalf("deployed_agents = %v, want 2 (two pages of one agent)", row["deployed_agents"])
	}
}

// Class: a derived figure is withheld when a collection it depends on was
// cut short.
func TestResellerRollupWithholdsDeltasOverTruncatedSubscriptions(t *testing.T) {
	testenv.Isolate(t)
	resellerServer(t, []string{
		`{"subscriptions":[{"id":1,"account":{"id":99},"product":"edr","status":"active","minimum_usage":5}],"pagination":{"next_page_token":"p1"}}`,
		`{"subscriptions":[{"id":2,"account":{"id":7},"product":"edr","status":"active","minimum_usage":2}],"pagination":{}}`,
	})
	row := resellerRow(t, "--max-scan-pages", "1")
	if d, ok := row["usage_vs_minimum_delta"]; ok && d != nil {
		t.Fatalf("usage_vs_minimum_delta = %v over a truncated subscription list, want it withheld: %v", d, row)
	}
}

// Class: commitments are compared only with the deployment metric they
// measure (EDR minimum vs deployed agents), never summed across products.
func TestResellerRollupComparesLikeProducts(t *testing.T) {
	testenv.Isolate(t)
	resellerServer(t, []string{`{"subscriptions":[
		{"id":1,"account":{"id":7},"product":"edr","status":"active","minimum_usage":2},
		{"id":2,"account":{"id":7},"product":"sat","status":"active","minimum_usage":40}],"pagination":{}}`})
	row := resellerRow(t)
	if fmt.Sprint(row["usage_vs_minimum_delta"]) != "0" {
		t.Fatalf("usage_vs_minimum_delta = %v, want 0 (2 agents vs EDR minimum 2; SAT seats are not agents): %v", row["usage_vs_minimum_delta"], row)
	}
}

// The live roll-up is labelled live in the agent envelope.
func TestResellerRollupMetaSourceIsLive(t *testing.T) {
	testenv.Isolate(t)
	resellerServer(t, []string{`{"subscriptions":[],"pagination":{}}`})
	got, _ := runNovelObject(t, "reseller-rollup", "--no-cache", "--agent")
	meta, _ := got["meta"].(map[string]any)
	if meta["source"] != "live" {
		t.Fatalf("reseller-rollup --agent meta = %v, want source live", got["meta"])
	}
}

// Regression guard for the address-boundary rule: an IPv4 address followed by
// a port, or wrapped in punctuation, is still that address.
func TestBlastRadiusMatchesAnAddressWithAPort(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"accounts_incident_reports": {
		`{"id":1,"organization_id":7,"status":"sent","sent_at":"` + hoursAgo(1) + `","summary":"Connected to 10.0.0.1:443"}`,
		`{"id":2,"organization_id":7,"status":"sent","sent_at":"` + hoursAgo(2) + `","summary":"Beacon to (10.0.0.1), blocked."}`,
		`{"id":3,"organization_id":7,"status":"sent","sent_at":"` + hoursAgo(3) + `","summary":"Seen 10.0.0.15 and 110.0.0.1 and 10.0.0.1.5"}`,
	}})
	got, _ := runNovelObject(t, "blast-radius", "--indicator", "10.0.0.1", "--db", db)
	if fmt.Sprint(got["incident_reports_total"]) != "2" {
		t.Fatalf("blast-radius 10.0.0.1 incidents = %v, want incidents 1 and 2 (address with a port or punctuation) and not 3", got["incident_reports"])
	}
}
