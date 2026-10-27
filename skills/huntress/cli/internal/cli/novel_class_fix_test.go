// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the wrong-result classes
// the second Codex review left open on the novel commands (reprint 4.34.0,
// fix B). Each test seeds the concrete input that produced a wrong answer and
// asserts the answer an MSP can act on: the right number, or the number
// withheld with a warning that says why.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"huntress-pp-cli/internal/cliutil/testenv"
	"huntress-pp-cli/internal/store"
)

// timedRow is one stored row with an explicit synced_at (now minus ago).
type timedRow struct {
	rt  string
	obj string
	ago time.Duration
}

// syncMark is one sync_state row: a completed walk watermarked at now minus
// ago, or an unfinished attempt.
type syncMark struct {
	ago      time.Duration
	complete bool
}

// seedTimed builds a store whose rows and sync states carry the given times,
// so a test can say which walk saw which row.
func seedTimed(t *testing.T, rows []timedRow, marks map[string]syncMark) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "data.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UTC()
	for _, r := range rows {
		if _, _, err := st.UpsertBatch(r.rt, []json.RawMessage{json.RawMessage(r.obj)}); err != nil {
			t.Fatalf("seeding %s: %v", r.rt, err)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(r.obj), &obj); err != nil {
			t.Fatal(err)
		}
		if _, err := st.DB().Exec(`UPDATE resources SET synced_at = ? WHERE resource_type = ? AND CAST(json_extract(data,'$.id') AS TEXT) = ?`,
			now.Add(-r.ago).Format(time.RFC3339), r.rt, fmt.Sprint(obj["id"])); err != nil {
			t.Fatal(err)
		}
	}
	for rt, m := range marks {
		if m.complete {
			if err := st.SaveSyncStateAt(rt, "", 0, now.Add(-m.ago)); err != nil {
				t.Fatal(err)
			}
			// A completed flat walk is also recorded as what it enumerated,
			// as sync records it: complete and unfiltered, from its start to
			// now. A reseller partition walk is recorded per account by sync;
			// the marks name no account, so a partition mark stays a sync
			// the walk records do not describe. Asserted through an
			// interface so these tests also build against a store without
			// walk records.
			if rec, ok := any(st).(interface {
				RecordWalkCoverage(resourceType, scope string, started, ended time.Time, complete, filtered, resumed bool) error
			}); ok && !aliasIsPartition(rt) {
				if err := rec.RecordWalkCoverage(rt, "", now.Add(-m.ago), now, true, false, false); err != nil {
					t.Fatal(err)
				}
			}
		} else if err := st.SaveSyncProgress(rt, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func idsOf(rows []any) []string {
	out := []string{}
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			out = append(out, fmt.Sprint(m["id"]))
		}
	}
	sort.Strings(out)
	return out
}

// ---- class 1: alias reconciliation -------------------------------------------

// The review's input: both agent aliases completed; accounts_agents still
// holds agent 1 from five minutes ago, and a later complete flat agents walk
// returned nothing. Whether the flat walk covers agent 1's account cannot be
// established, so the agent may not be counted silently.
func TestAliasReconcileWarnsWhenALaterCompleteSyncDidNotReturnARow(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"accounts_organizations", `{"id":70,"name":"Acme","account_id":7,"parent_id":"7"}`, 5*time.Minute + time.Second},
		{"accounts_agents", `{"id":1,"organization_id":70,"account_id":7,"parent_id":"7","last_callback_at":"` + hoursAgo(1) + `"}`, 5*time.Minute + time.Second},
	}, map[string]syncMark{
		"accounts_organizations": {ago: 5 * time.Minute, complete: true},
		"accounts_agents":        {ago: 5 * time.Minute, complete: true},
		"agents":                 {ago: time.Minute, complete: true},
	})
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if !strings.Contains(errOut, "not returned by the later complete sync of agents") {
		t.Fatalf("fleet-summary counted an agent a later complete agents sync did not return, with no warning; stderr %q, out %v", errOut, sum)
	}
}

// A later complete flat walk that returned other agents of the same
// organization is authoritative for the agent it did not return.
func TestAliasReconcileDropsARowALaterCompleteSyncOfItsScopeDidNotReturn(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"organizations", `{"id":70,"name":"Acme","account_id":7}`, 30 * time.Second},
		{"accounts_agents", `{"id":1,"organization_id":70,"account_id":7,"parent_id":"7"}`, 10 * time.Minute},
		{"agents", `{"id":2,"organization_id":70,"account_id":7}`, 30 * time.Second},
	}, map[string]syncMark{
		"organizations":   {ago: time.Minute, complete: true},
		"accounts_agents": {ago: 10 * time.Minute, complete: true},
		"agents":          {ago: time.Minute, complete: true},
	})
	sum, _ := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "1" {
		t.Fatalf("fleet-summary agents = %v, want 1 (agent 1 was not returned by a later complete agents walk that covered organization 70)", sum["agents"])
	}
}

// The same holds inside one alias: a flat walk that completed after a row
// was written and did not return it, while returning its organization's
// other agents, removed it; with no such evidence the row is kept and flagged.
func TestAliasReconcileTreatsAStaleFlatRowByEvidence(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"organizations", `{"id":70,"name":"Acme"}`, 30 * time.Second},
		{"organizations", `{"id":80,"name":"Beta"}`, 30 * time.Second},
		{"agents", `{"id":1,"organization_id":70}`, 10 * time.Minute},
		{"agents", `{"id":2,"organization_id":70}`, 30 * time.Second},
		{"agents", `{"id":3,"organization_id":80}`, 10 * time.Minute},
	}, map[string]syncMark{
		"organizations": {ago: time.Minute, complete: true},
		"agents":        {ago: time.Minute, complete: true},
	})
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "2" {
		t.Fatalf("fleet-summary agents = %v, want 2 (agent 1 dropped by evidence, agent 3 kept as unverified)", sum["agents"])
	}
	if !strings.Contains(errOut, "not returned by the later complete sync of agents") {
		t.Fatalf("an unverified stale agent printed no warning; stderr %q", errOut)
	}
}

// Guard: a flat row its own latest walk returned is current, and a later
// partition walk that cannot be shown to cover its account does not flag it.
func TestAliasReconcileDoesNotFlagRowsOutsideALaterWalksScope(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"accounts", `{"id":7,"name":"Child"}`, 2 * time.Minute},
		{"agents", `{"id":5,"organization_id":10,"account_id":1}`, 10 * time.Minute},
		{"accounts_agents", `{"id":6,"organization_id":70,"account_id":7,"parent_id":"7"}`, time.Minute},
	}, map[string]syncMark{
		"accounts":        {ago: 3 * time.Minute, complete: true},
		"agents":          {ago: 11 * time.Minute, complete: true},
		"accounts_agents": {ago: time.Minute, complete: true},
	})
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "2" || strings.Contains(errOut, "not returned by the later complete sync") {
		t.Fatalf("fleet-summary agents = %v (want 2) with stderr %q (want no reconciliation warning)", sum["agents"], errOut)
	}
}

// Guard: a reseller partition that walked no account (a standard
// credential has none) says nothing about flat rows, even ones that carry no
// account and come from a flat walk that did not finish.
func TestAliasReconcileIgnoresAPartitionThatWalkedNothing(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"invoices", `{"id":1,"quantity":10,"status":"paid","created_at":"2026-10-01T00:00:00Z"}`, 10 * time.Minute},
	}, map[string]syncMark{
		"invoices":          {complete: false},
		"accounts_invoices": {ago: time.Minute, complete: true},
	})
	_, errOut := runNovelObject(t, "billing-reconcile", "--db", db)
	if strings.Contains(errOut, "not returned by the later complete sync") {
		t.Fatalf("a partition that walked no account flagged a flat invoice; stderr %q", errOut)
	}
}

// ---- class 2: identity across aliases ----------------------------------------

// The review's input: the freshest copy of invoice 2 is a flat copy without
// account fields; its partition copy says account 7. One account, one billing
// period: 100 seats, delta 0.
func TestBillingReconcileKeepsTheAccountAcrossAliases(t *testing.T) {
	testenv.Isolate(t)
	rows := []timedRow{
		{"accounts_invoices", `{"id":1,"quantity":100,"status":"paid","created_at":"2026-09-01T00:00:00Z","parent_id":"7"}`, 10 * time.Minute},
		{"accounts_invoices", `{"id":2,"quantity":100,"status":"paid","created_at":"2026-10-01T00:00:00Z","parent_id":"7"}`, 10 * time.Minute},
		{"invoices", `{"id":2,"quantity":100,"status":"paid","created_at":"2026-10-01T00:00:00Z"}`, 9 * time.Minute},
	}
	for i := 1; i <= 100; i++ {
		rows = append(rows, timedRow{"accounts_agents", fmt.Sprintf(`{"id":%d,"organization_id":70,"account_id":7,"parent_id":"7"}`, i), 10 * time.Minute})
	}
	db := seedTimed(t, rows, map[string]syncMark{
		"accounts_invoices": {ago: 10 * time.Minute, complete: true},
		"invoices":          {ago: 10*time.Minute + time.Second, complete: true},
		"accounts_agents":   {ago: 10 * time.Minute, complete: true},
	})
	got, _ := runNovelObject(t, "billing-reconcile", "--db", db)
	if fmt.Sprint(got["invoiced_seats"]) != "100" || fmt.Sprint(got["delta"]) != "0" {
		t.Fatalf("billing-reconcile = invoiced_seats %v delta %v, want 100 and 0 (invoice 2's account is 7 on its other copy): %v", got["invoiced_seats"], got["delta"], got)
	}
}

// A store that carries account identities, plus an invoice no copy of which
// says its account, cannot be totalled: the invoice is unattributed and the
// total is withheld rather than merged into a guess.
func TestBillingReconcileReportsUnattributedInvoices(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"accounts_invoices": {`{"id":1,"quantity":100,"status":"paid","created_at":"2026-10-01T00:00:00Z","parent_id":"7"}`},
		"invoices":          {`{"id":9,"quantity":50,"status":"paid","created_at":"2026-10-01T00:00:00Z"}`},
		"accounts_agents":   {`{"id":1,"organization_id":70,"parent_id":"7"}`},
	})
	got, errOut := runNovelObject(t, "billing-reconcile", "--db", db)
	if got["invoiced_seats"] != nil || got["delta"] != nil {
		t.Fatalf("billing-reconcile with an unattributed invoice = invoiced_seats %v delta %v, want both withheld (null)", got["invoiced_seats"], got["delta"])
	}
	un, _ := got["unattributed_invoices"].([]any)
	if len(un) != 1 || !strings.Contains(errOut, "unattributed") {
		t.Fatalf("unattributed_invoices = %v, stderr %q; want invoice 9 listed with a warning", got["unattributed_invoices"], errOut)
	}
}

// Copies that disagree about the account are not merged into either one.
func TestBillingReconcileRefusesConflictingAccountIdentity(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"accounts_invoices": {`{"id":1,"quantity":100,"status":"paid","created_at":"2026-10-01T00:00:00Z","parent_id":"7"}`},
		"invoices":          {`{"id":1,"quantity":100,"status":"paid","created_at":"2026-10-01T00:00:00Z","account_id":8}`},
	})
	got, _ := runNovelObject(t, "billing-reconcile", "--db", db)
	un, _ := got["unattributed_invoices"].([]any)
	if got["invoiced_seats"] != nil || len(un) != 1 {
		t.Fatalf("billing-reconcile over copies naming accounts 7 and 8 = %v, want the invoice unattributed and the total withheld", got)
	}
}

// Guard: a standard credential's flat invoices carry no account fields at
// all; they are the credential's own account and total normally.
func TestBillingReconcileFlatOnlyStoreStillTotals(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"invoices": {
			`{"id":1,"quantity":40,"status":"paid","created_at":"2026-09-01T00:00:00Z"}`,
			`{"id":2,"quantity":50,"status":"paid","created_at":"2026-10-01T00:00:00Z"}`,
		},
		"agents": {`{"id":1,"organization_id":70}`},
	})
	got, errOut := runNovelObject(t, "billing-reconcile", "--db", db)
	if fmt.Sprint(got["invoiced_seats"]) != "50" || fmt.Sprint(got["delta"]) != "49" || strings.Contains(errOut, "unattributed") {
		t.Fatalf("billing-reconcile over a flat-only store = %v (stderr %q), want invoiced_seats 50, delta 49, no unattributed warning", got, errOut)
	}
}

// ---- class 3: indicator matching ---------------------------------------------

func blastIDs(t *testing.T, db, indicator string) (incidents, agents []string, out map[string]any, errOut string) {
	t.Helper()
	out, errOut = runNovelObject(t, "blast-radius", "--indicator", indicator, "--db", db)
	inc, _ := out["incident_reports"].([]any)
	ag, _ := out["agents"].([]any)
	return idsOf(inc), idsOf(ag), out, errOut
}

func TestBlastRadiusParsesAddressTokens(t *testing.T) {
	testenv.Isolate(t)
	inc := func(id int, summary string) string {
		return fmt.Sprintf(`{"id":%d,"organization_id":7,"status":"sent","sent_at":"%s","summary":%q}`, id, hoursAgo(float64(id)), summary)
	}
	db := seedStore(t, map[string][]string{
		"accounts_incident_reports": {
			inc(1, "Connected to 10.0.0.1:443"),
			inc(2, "Seen 10.0.0.11 only"),
			inc(3, "Outbound [2001:db8::1]:443"),
			inc(4, "Outbound 2001:db8::10"),
			inc(5, "Expanded 2001:0db8:0:0:0:0:0:1 seen"),
			inc(6, "Loopback [::1]:443"),
			inc(7, "beacon to notevil.com"),
			inc(8, "fetched https://c2.evil.com/payload"),
			inc(9, "resolved evil.com.attacker.net"),
			inc(10, "EVIL.COM:443 contacted"),
			inc(11, "nic 00:11:22:33:44:55:66 seen"),
			inc(12, "nic 00-11-22-33-44-55 seen"),
		},
		"accounts_agents": {
			`{"id":21,"organization_id":7,"hostname":"host10"}`,
			`{"id":22,"organization_id":7,"hostname":"host1.corp.local"}`,
			`{"id":23,"organization_id":7,"hostname":"x","mac_addresses":["00-11-22-33-44-55"]}`,
			`{"id":24,"organization_id":7,"hostname":"WS01.corp.local"}`,
			`{"id":25,"organization_id":7,"hostname":"ws01.corp.local.evil"}`,
		},
	})
	cases := []struct {
		name, indicator   string
		incidents, agents []string
	}{
		{"ipv4 with a port, not a longer address", "10.0.0.1", []string{"1"}, nil},
		{"bracketed ipv6 with a port, any spelling", "2001:db8::1", []string{"3", "5"}, nil},
		{"bracketed ipv6 loopback", "::1", []string{"6"}, nil},
		{"domain: whole name or subdomain, never a longer name", "evil.com", []string{"10", "8"}, nil},
		{"mac: any separator, never a longer address", "00:11:22:33:44:55", []string{"12"}, []string{"23"}},
		{"fqdn hostname: whole name only", "ws01.corp.local", nil, []string{"24"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotInc, gotAg, _, _ := blastIDs(t, db, c.indicator)
			want := func(s []string) string {
				if s == nil {
					s = []string{}
				}
				sort.Strings(s)
				return fmt.Sprint(s)
			}
			if fmt.Sprint(gotInc) != want(c.incidents) || fmt.Sprint(gotAg) != want(c.agents) {
				t.Fatalf("blast-radius %q = incidents %v agents %v, want incidents %v agents %v", c.indicator, gotInc, gotAg, want(c.incidents), want(c.agents))
			}
		})
	}
	t.Run("a single-label fragment inside a longer name is flagged", func(t *testing.T) {
		_, gotAg, out, errOut := blastIDs(t, db, "host1")
		if fmt.Sprint(gotAg) != "[21 22]" {
			t.Fatalf("blast-radius host1 agents = %v, want 21 (fragment, kept) and 22", gotAg)
		}
		if fmt.Sprint(out["fragment_matches"]) != "1" || !strings.Contains(errOut, "inside a longer") {
			t.Fatalf("blast-radius host1 fragment_matches = %v, stderr %q; want 1 with a warning", out["fragment_matches"], errOut)
		}
		for _, r := range out["agents"].([]any) {
			m := r.(map[string]any)
			want := map[string]string{"21": "fragment", "22": "exact"}[fmt.Sprint(m["id"])]
			if m["match"] != want {
				t.Fatalf("agent %v match = %v, want %s", m["id"], m["match"], want)
			}
		}
	})
}

// ---- class 4: collection envelopes -------------------------------------------

// resellerServerWith serves one account (7) whose agents and subscriptions
// responses the test chooses.
func resellerServerWith(t *testing.T, accountsBody, agentsBody, subsBody string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/accounts":
			_, _ = w.Write([]byte(accountsBody))
		case "/v1/reseller/subscriptions":
			_, _ = w.Write([]byte(subsBody))
		case "/v1/accounts/7/agents":
			_, _ = w.Write([]byte(agentsBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUNTRESS_BASE_URL", srv.URL)
	t.Setenv("HUNTRESS_API_KEY", "hk_test_key_0001")
	t.Setenv("HUNTRESS_API_SECRET", "test-secret-value-0001")
}

const (
	oneAccount = `{"accounts":[{"id":7,"name":"Acme","status":"active"}],"pagination":{}}`
	edrMinTwo  = `{"subscriptions":[{"id":1,"account":{"id":7},"product":"edr","status":"active","minimum_usage":2}],"pagination":{}}`
)

func TestResellerRollupRefusesUnreadableAgentEnvelopes(t *testing.T) {
	for name, body := range map[string]string{
		"an error body with no agents array": `{"error":"temporary failure"}`,
		"a null agents array":                `{"agents":null,"pagination":{}}`,
		"an agents object":                   `{"agents":{},"pagination":{}}`,
		"a non-string next_page_token":       `{"agents":[{"id":1}],"pagination":{"next_page_token":5}}`,
		"no pagination metadata":             `{"agents":[{"id":1}]}`,
		"a non-object pagination":            `{"agents":[{"id":1}],"pagination":"done"}`,
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			resellerServerWith(t, oneAccount, body, edrMinTwo)
			got, _ := runNovelObject(t, "reseller-rollup", "--no-cache")
			row := got["accounts"].([]any)[0].(map[string]any)
			fails, _ := got["fetch_failures"].([]any)
			if row["deployed_agents"] != nil || row["usage_vs_minimum_delta"] != nil || len(fails) != 1 {
				t.Fatalf("reseller-rollup over %s = deployed_agents %v delta %v failures %v; want both withheld and one fetch failure", name, row["deployed_agents"], row["usage_vs_minimum_delta"], got["fetch_failures"])
			}
		})
	}
}

func TestResellerRollupRefusesAnUnreadableAccountList(t *testing.T) {
	testenv.Isolate(t)
	resellerServerWith(t, `{"error":"temporary failure"}`, `{"agents":[],"pagination":{}}`, edrMinTwo)
	if out, errOut, err := runCLI(t, "reseller-rollup", "--no-cache", "--json"); err == nil {
		t.Fatalf("reseller-rollup over an account list with no accounts array succeeded: %s (stderr %q)", out, errOut)
	}
}

func TestIncidentRemediationsEnvelopeMustBeReadable(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"accounts_incident_reports": {
		`{"id":1,"organization_id":7,"status":"sent","sent_at":"` + hoursAgo(1) + `","remediations":{"error":"x"}}`,
		`{"id":2,"organization_id":7,"status":"sent","sent_at":"` + hoursAgo(2) + `"}`,
	}})
	for _, id := range []string{"1", "2"} {
		det, _ := runNovelObject(t, "incident-detail", id, "--db", db)
		if det["remediation_count"] != nil || det["remediations_complete"] != false || det["remediations"] != nil {
			t.Fatalf("incident-detail %s over an unreadable remediations field = count %v complete %v items %v; want null, false, null", id, det["remediation_count"], det["remediations_complete"], det["remediations"])
		}
	}
	for _, r := range runNovelJSON(t, "fleet-incidents", "--db", db) {
		if r["remediations_count"] != nil {
			t.Fatalf("fleet-incidents remediations_count for incident %v = %v, want null (unreadable)", r["id"], r["remediations_count"])
		}
	}
}

func TestEscalationOrganizationsMustBeReadable(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"escalations": {
		`{"id":1,"severity":"high","status":"open","created_at":"` + hoursAgo(1) + `","organizations":{"id":7}}`,
	}})
	rows := runNovelJSON(t, "escalation-queue", "--db", db)
	if len(rows) != 1 || rows[0]["organizations"] != nil {
		t.Fatalf("escalation-queue organizations for an unreadable field = %v, want null (not an empty list)", rows)
	}
}

func TestDriftRefusesAnUnreadableSnapshot(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"accounts_agents": {`{"id":1,"organization_id":7}`}})
	snap, err := driftSnapshotPath(db, "agents")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(snap), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snap, []byte(`{"error":"not a snapshot"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runCLI(t, "drift", "--db", db, "--json"); err == nil {
		t.Fatalf("drift over an unreadable prior snapshot succeeded: %s", out)
	}
	if b, _ := os.ReadFile(snap); string(b) != `{"error":"not a snapshot"}` {
		t.Fatalf("drift overwrote the unreadable snapshot: %s", b)
	}
}

// ---- class 5: unreadable inputs never publish as 0 ----------------------------

func TestResellerRollupWithholdsTotalsBuiltOnUnreadableInputs(t *testing.T) {
	agents := `{"agents":[{"id":1},{"id":2}],"pagination":{}}`
	t.Run("an active EDR minimum that is null", func(t *testing.T) {
		testenv.Isolate(t)
		resellerServerWith(t, oneAccount, agents, `{"subscriptions":[{"id":1,"account":{"id":7},"product":"edr","status":"active","minimum_usage":null}],"pagination":{}}`)
		got, errOut := runNovelObject(t, "reseller-rollup", "--no-cache")
		row := got["accounts"].([]any)[0].(map[string]any)
		if row["minimum_usage_total"] != nil || row["usage_vs_minimum_delta"] != nil || !strings.Contains(errOut, "minimum_usage") {
			t.Fatalf("minimum_usage_total %v delta %v stderr %q; want both withheld and a warning naming minimum_usage", row["minimum_usage_total"], row["usage_vs_minimum_delta"], errOut)
		}
	})
	t.Run("a subscription with no readable status", func(t *testing.T) {
		testenv.Isolate(t)
		resellerServerWith(t, oneAccount, agents, `{"subscriptions":[{"id":2,"account":{"id":7},"product":"sat","minimum_usage":40}],"pagination":{}}`)
		got, _ := runNovelObject(t, "reseller-rollup", "--no-cache")
		by, _ := got["accounts"].([]any)[0].(map[string]any)["minimum_usage_by_product"].(map[string]any)
		if v, ok := by["sat"]; !ok || v != nil {
			t.Fatalf("minimum_usage_by_product = %v, want sat present and withheld (null)", by)
		}
	})
	t.Run("an EDR subscription no account can be read for", func(t *testing.T) {
		testenv.Isolate(t)
		resellerServerWith(t, oneAccount, agents, `{"subscriptions":[
			{"id":3,"product":"edr","status":"active","minimum_usage":5},
			{"id":1,"account":{"id":7},"product":"edr","status":"active","minimum_usage":2}],"pagination":{}}`)
		got, errOut := runNovelObject(t, "reseller-rollup", "--no-cache")
		row := got["accounts"].([]any)[0].(map[string]any)
		un, _ := got["unattributed_subscriptions"].([]any)
		if row["minimum_usage_total"] != nil || row["usage_vs_minimum_delta"] != nil || len(un) != 1 || !strings.Contains(errOut, "unattributed") {
			t.Fatalf("total %v delta %v unattributed %v stderr %q; want the EDR figures withheld and subscription 3 reported", row["minimum_usage_total"], row["usage_vs_minimum_delta"], got["unattributed_subscriptions"], errOut)
		}
	})
}

// An unreadable created_at matters only when it must be ordered against
// another invoice of the same account.
func TestBillingReconcileWithholdsAnUnorderableBillingPeriod(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"invoices": {
			`{"id":1,"quantity":40,"status":"paid","created_at":"garbage"}`,
			`{"id":2,"quantity":50,"status":"paid","created_at":"2026-10-01T00:00:00Z"}`,
		},
		"agents": {`{"id":1,"organization_id":70}`},
	})
	got, errOut := runNovelObject(t, "billing-reconcile", "--db", db)
	if got["invoiced_seats"] != nil || got["delta"] != nil || !strings.Contains(errOut, "created_at") {
		t.Fatalf("billing-reconcile with an unorderable invoice = %v (stderr %q), want invoiced_seats and delta withheld with a warning", got, errOut)
	}
	one := seedStore(t, map[string][]string{
		"invoices": {`{"id":1,"quantity":40,"status":"paid"}`},
		"agents":   {`{"id":1,"organization_id":70}`},
	})
	got, _ = runNovelObject(t, "billing-reconcile", "--db", one)
	if fmt.Sprint(got["invoiced_seats"]) != "40" {
		t.Fatalf("billing-reconcile with a single invoice and no created_at = %v, want invoiced_seats 40 (one invoice is the latest)", got)
	}
}

func TestBillingReconcileWithholdsAnUnreadableQuantity(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"invoices": {`{"id":1,"quantity":"lots","status":"paid","created_at":"2026-10-01T00:00:00Z"}`},
		"agents":   {`{"id":1,"organization_id":70}`},
	})
	got, errOut := runNovelObject(t, "billing-reconcile", "--db", db)
	if got["invoiced_seats"] != nil || got["delta"] != nil || !strings.Contains(errOut, "quantity") {
		t.Fatalf("billing-reconcile over an unreadable quantity = %v (stderr %q), want invoiced_seats and delta withheld with a warning", got, errOut)
	}
}

func TestTriageAgeDoesNotBucketAnUnreadableAgeAsNew(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"accounts_incident_reports": {
		`{"id":1,"organization_id":7,"status":"sent","severity":"high","sent_at":"` + hoursAgo(1) + `"}`,
		`{"id":2,"organization_id":7,"status":"sent","severity":"critical","sent_at":"garbage"}`,
	}})
	rows := runNovelJSON(t, "triage-age", "--db", db)
	if fmt.Sprint(rows[0]["total"]) != "1" {
		t.Fatalf("triage-age first bucket = %v, want 1 (an unreadable sent_at is not a new incident)", rows[0])
	}
	last := rows[len(rows)-1]
	if fmt.Sprint(last["total"]) != "1" || !strings.Contains(fmt.Sprint(last["bucket"]), "unknown") || last["sla_breached"] != nil {
		t.Fatalf("triage-age last bucket = %v, want an unknown-age bucket of 1 with sla_breached null", last)
	}
	if prev := rows[len(rows)-2]; prev["sla_breached"] != nil {
		t.Fatalf("triage-age oldest bucket sla_breached = %v, want null while an incident's age is unknown", prev["sla_breached"])
	}
}

func TestFleetSummaryDoesNotHideUnreadableTimestamps(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"organizations": {`{"id":7,"name":"Acme"}`},
		"agents": {
			`{"id":1,"organization_id":7,"last_callback_at":"garbage"}`,
			`{"id":2,"organization_id":7,"last_callback_at":"` + hoursAgo(1) + `"}`,
		},
		"incident-reports": {
			`{"id":1,"organization_id":7,"status":"sent","severity":"critical","sent_at":"garbage"}`,
			`{"id":2,"organization_id":7,"status":"sent","severity":"critical","sent_at":"` + hoursAgo(5) + `"}`,
		},
	})
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if sum["oldest_open_critical_hours"] != nil {
		t.Fatalf("oldest_open_critical_hours = %v, want null (one open critical's age is unreadable)", sum["oldest_open_critical_hours"])
	}
	if fmt.Sprint(sum["stale_agents_unknown"]) != "1" || !strings.Contains(errOut, "last_callback_at") {
		t.Fatalf("stale_agents_unknown = %v stderr %q, want 1 with a warning", sum["stale_agents_unknown"], errOut)
	}
}

func TestMTTRWithholdsAMeanOverUnreadableTimestamps(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"incident-reports": {
		`{"id":1,"organization_id":7,"status":"closed","sent_at":"` + hoursAgo(10) + `","closed_at":"` + hoursAgo(8) + `"}`,
		`{"id":2,"organization_id":7,"status":"closed","sent_at":"garbage","closed_at":"` + hoursAgo(1) + `"}`,
	}})
	rows := runNovelJSON(t, "mttr", "--db", db)
	if len(rows) != 1 || rows[0]["mttr_hours"] != nil || fmt.Sprint(rows[0]["unreadable_timestamps"]) != "1" {
		t.Fatalf("mttr = %v, want one org group with mttr_hours null and unreadable_timestamps 1", rows)
	}
	card, _ := runNovelObject(t, "org-scorecard", "7", "--db", db)
	if card["mttr_hours"] != nil {
		t.Fatalf("org-scorecard mttr_hours = %v, want null", card["mttr_hours"])
	}
}

func TestCoverageGapsCountsUnreadableCallbacks(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"organizations": {`{"id":7,"name":"Acme"}`},
		"agents":        {`{"id":1,"organization_id":7,"last_callback_at":"garbage","defender_status":"enabled","firewall_status":"enabled"}`},
	})
	rows := runNovelJSON(t, "coverage-gaps", "--db", db)
	if len(rows) != 1 || fmt.Sprint(rows[0]["stale_unknown"]) != "1" {
		t.Fatalf("coverage-gaps = %v, want organization 7 listed with stale_unknown 1", rows)
	}
}

func TestWindowedNovelsWarnAboutUnreadableTimestamps(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{"accounts_incident_reports": {
		`{"id":1,"organization_id":7,"status":"sent","severity":"critical","sent_at":"garbage","indicator_types":["ransomware_canaries"]}`,
	}})
	for _, args := range [][]string{{"canary-watch", "--window", "24h"}, {"handoff", "--since", "12h"}} {
		_, errOut, err := runCLI(t, append(args, "--db", db, "--json")...)
		if err != nil || !strings.Contains(errOut, "unreadable") {
			t.Fatalf("%v over an incident with an unreadable sent_at: err %v stderr %q, want a warning", args, err, errOut)
		}
	}
}

// ---- class 6: provenance checks that fail ---------------------------------------

// The review's input: an existing store whose sync_state predates
// last_attempt_complete, opened read-only (no migration).
func TestNovelsWarnWhenSyncCompletenessCannotBeChecked(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"organizations": {`{"id":7,"name":"Acme"}`},
		"agents":        {`{"id":1,"organization_id":7}`},
	})
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(`ALTER TABLE sync_state DROP COLUMN last_attempt_complete`); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	_, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if !strings.Contains(errOut, "could not check whether the last sync") {
		t.Fatalf("fleet-summary over a legacy sync_state printed no provenance warning; stderr %q", errOut)
	}
}
