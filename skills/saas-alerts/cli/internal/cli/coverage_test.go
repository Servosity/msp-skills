// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// TestNovelCoverageHelpWires smoke-tests that the coverage command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelCoverageHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"coverage", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("coverage --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "coverage"} {
		if !strings.Contains(help, want) {
			t.Fatalf("coverage --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestCoverageFlagsOrderingAndSummary(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasCoverageResult
	stdout, _ := saasRunJSON(t, &got, "coverage", "--db", db, "--silent", "4h", "--json")
	saasRequireKeys(t, stdout, "synced", "unsynced_resources", "customers", "summary")
	want := []struct {
		id, flags string
	}{
		{"CUST_D", "SILENT,NEVER_SEEN,RESPOND_NOT_CONNECTED"},
		{"CUST_C", "SILENT,RESPOND_NOT_CONNECTED"}, // a silent customer outranks Respond-only gaps
		{"CUST_B", "RESPOND_DISCONNECTED,NO_RULES"},
		{"CUST_A", ""},
	}
	if len(got.Customers) != len(want) {
		t.Fatalf("customers = %d, want 4", len(got.Customers))
	}
	for i, w := range want {
		c := got.Customers[i]
		if c.CustomerID != w.id || strings.Join(c.Flags, ",") != w.flags {
			t.Errorf("customer[%d] = %s %v, want %s %s", i, c.CustomerID, c.Flags, w.id, w.flags)
		}
	}
	if got.Summary != (saasCoverageSummary{Total: 4, Silent: 2, NeverSeen: 1, RespondDisconnected: 1, RespondNotConnected: 2, NoRules: 1}) {
		t.Errorf("summary = %+v", got.Summary)
	}

	d, c, b, a := got.Customers[0], got.Customers[1], got.Customers[2], got.Customers[3]
	if d.LastEventTime != nil || d.EventAge != nil || d.Events7d != 0 || d.Match != "none" {
		t.Errorf("never-seen customer = %+v", d)
	}
	if b.Match != "name" || !b.MatchInferred || b.RespondOrgID != "ORG_B" || b.RespondStatus != "Active" ||
		b.RespondLastError != "Request failed with status code 400" || b.RuleCount == nil || *b.RuleCount != 0 || b.Events7d != 3 {
		t.Errorf("birch (name-matched, disconnected, excluded from the ALL rule) = %+v", b)
	}
	if b.LastEventTime == nil || *b.LastEventTime != "2026-10-14T09:15:00Z" || *b.EventAge != "2h45m" || *b.EventAgeSeconds != 9900 {
		t.Errorf("birch last event = %v / %v", b.LastEventTime, b.EventAge)
	}
	if c.Events7d != 3 || c.MaxPullLagSeconds == nil || *c.MaxPullLagSeconds != 600 || *c.EventAge != "6h" {
		t.Errorf("cedar = %+v", c)
	}
	if a.Match != "id" || a.MatchInferred || a.RuleCount == nil || *a.RuleCount != 2 || a.RespondStatus != "Active" || a.Events7d != 4 ||
		a.MaxPullLagSeconds == nil || *a.MaxPullLagSeconds != 300 || *a.EventAge != "30m" {
		t.Errorf("acme = %+v", a)
	}
}

func TestCoverageSilentThreshold(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasCoverageResult
	saasRunJSON(t, &got, "coverage", "--db", db, "--json") // default 72h
	if got.Summary.Silent != 1 || got.Summary.NeverSeen != 1 {
		t.Fatalf("72h summary = %+v, want only the never-seen customer silent", got.Summary)
	}
	for _, c := range got.Customers {
		if c.CustomerID == "CUST_C" && strings.Contains(strings.Join(c.Flags, ","), "SILENT") {
			t.Errorf("cedar is 6h old and must not be SILENT at 72h: %v", c.Flags)
		}
	}
	if _, _, err := saasRunCLI(t, "coverage", "--db", db, "--silent", "forever"); ExitCode(err) != 2 {
		t.Errorf("bad --silent: err=%v, want exit 2", err)
	}
}

func TestCoverageMissingStoreAndSelect(t *testing.T) {
	saasIsolate(t)
	missing := saasMissingDBPath(t)
	var empty saasCoverageResult
	stdout, stderr := saasRunJSON(t, &empty, "coverage", "--db", missing, "--json")
	if !strings.HasPrefix(stderr, "hint: no local mirror at "+missing) || !strings.Contains(stdout, `"customers": []`) || empty.Summary != (saasCoverageSummary{}) {
		t.Errorf("missing store: stdout=%s stderr=%q", stdout, stderr)
	}

	db := saasSeedFixtureStore(t)
	out, _, err := saasRunCLI(t, "coverage", "--db", db, "--silent", "4h", "--select", "customers.customer_id,customers.flags", "--json")
	if err != nil {
		t.Fatal(err)
	}
	saasRequireKeys(t, out, "customers")
	if strings.Contains(out, "rule_count") || !strings.Contains(out, `"NEVER_SEEN"`) {
		t.Errorf("--select output = %s", out)
	}
}

func TestCoverageRuleCounting(t *testing.T) {
	org := &saasRespondOrg{ID: "ORG_X", RuleIDs: []string{"RULE_UNSYNCED", "RULE_DRAFT"}}
	rules := []map[string]any{
		{"id": "RULE_ALL", "status": "ACTIVE", "organizations": map[string]any{"type": "ALL"}},
		{"id": "RULE_ALL_EXCL", "status": "ACTIVE", "organizations": map[string]any{"type": "ALL", "excludedOrganizations": []any{map[string]any{"id": "ORG_X"}}}},
		{"id": "RULE_SPECIFIC", "status": "ACTIVE", "organizations": map[string]any{"type": "SPECIFIC", "organizations": []any{map[string]any{"id": "ORG_X"}}}},
		{"id": "RULE_SWAPPED", "status": "ACTIVE", "organizations": map[string]any{"type": "SPECIFIC", "organizations": []any{map[string]any{"id": "Org X", "name": "ORG_X"}}}},
		{"id": "RULE_OTHER", "status": "ACTIVE", "organizations": map[string]any{"type": "SPECIFIC", "organizations": []any{map[string]any{"id": "ORG_Y"}}}},
		{"id": "RULE_DRAFT", "status": "DRAFT", "organizations": map[string]any{"type": "ALL"}},
		{"id": "RULE_NOSTATUS", "organizations": map[string]any{"type": "SPECIFIC", "organizations": []any{map[string]any{"id": "ORG_X"}}}},
	}
	// Confirmed: RULE_ALL, RULE_SPECIFIC, RULE_SWAPPED (ACTIVE and applying).
	// Unknown: RULE_UNSYNCED (a ruleIds reference never synced) and
	// RULE_NOSTATUS (applies, but no status). RULE_DRAFT is referenced by
	// ruleIds yet synced as DRAFT, so it counts as neither.
	confirmed, unknown := saasCountOrgRules(org, rules)
	if confirmed != 3 || unknown != 2 {
		t.Fatalf("rule counts = %d confirmed, %d unknown; want 3 and 2", confirmed, unknown)
	}
}

func TestCoverageConnectionState(t *testing.T) {
	broken := &saasRespondOrg{ID: "ORG_X", Connections: []map[string]any{
		{"id": "C1", "status": "Active", "lastConnectedAt": float64(saasFixtureNow.Add(-time.Hour).UnixMilli())},
		{"id": "C2", "status": "Broken", "lastError": map[string]any{"message": "AADSTS50173: grant expired"}},
	}}
	status, lastErr, lastConnected, isBroken := saasRespondConnectionState(broken)
	if status != "Broken" || !isBroken || lastErr != "AADSTS50173: grant expired" || lastConnected == nil || *lastConnected != "2026-10-14T11:00:00Z" {
		t.Fatalf("broken org = %q %q %v %v", status, lastErr, lastConnected, isBroken)
	}
	healthy := &saasRespondOrg{ID: "ORG_Y", Connections: []map[string]any{{"id": "C3", "status": "Upgradable"}, {"id": "C4", "status": "Active"}}}
	if status, _, last, isBroken := saasRespondConnectionState(healthy); status != "Upgradable" || isBroken || last != nil {
		t.Fatalf("healthy org = %q %v %v", status, last, isBroken)
	}

	// An org seen only in organizations-disconnected (spec example spells the
	// key organization") still indexes, by id and by name.
	_, byID, byName := saasIndexRespondOrgs(nil, []map[string]any{
		{"id": "APP_CONNECTION_Z", "status": "Active", "organization\"": map[string]any{"id": "ORG_Z", "name": "Zeta Corp"}},
	})
	if org := byID["ORG_Z"]; org == nil || len(org.disconnected) != 1 || len(byName["zeta corp"]) != 1 {
		t.Fatalf("disconnected-only org not indexed: %+v", byID)
	}
}

// Flag severity, not flag count, orders customers: a silent customer is a
// monitoring blind spot and outranks any mix of Respond gaps.
func TestCoverageSortBySeverityWeightThenAge(t *testing.T) {
	age := func(s int64) *int64 { return &s }
	rows := []saasCoverageCustomer{
		{CustomerID: "RESPOND", CustomerName: "Alpha", Flags: []string{saasCoverageRespondDisconnected, saasCoverageNoRules}, EventAgeSeconds: age(60)},
		{CustomerID: "SILENT", CustomerName: "Zulu", Flags: []string{saasCoverageSilent}, EventAgeSeconds: age(400000)},
		{CustomerID: "CLEAN", CustomerName: "Bravo", Flags: []string{}, EventAgeSeconds: age(10)},
		{CustomerID: "NOTCONN-NEW", CustomerName: "Charlie", Flags: []string{saasCoverageRespondNotConnected}, EventAgeSeconds: age(100)},
		{CustomerID: "NOTCONN-OLD", CustomerName: "Delta", Flags: []string{saasCoverageRespondNotConnected}, EventAgeSeconds: age(9000)},
		{CustomerID: "NEVER", CustomerName: "Echo", Flags: []string{saasCoverageSilent, saasCoverageNeverSeen}},
		{CustomerID: "NORULES", CustomerName: "Foxtrot", Flags: []string{saasCoverageNoRules}, EventAgeSeconds: age(5)},
	}
	saasSortCoverageCustomers(rows)
	var order []string
	for _, r := range rows {
		order = append(order, r.CustomerID)
	}
	want := "NEVER,SILENT,RESPOND,NOTCONN-OLD,NOTCONN-NEW,NORULES,CLEAN"
	if strings.Join(order, ",") != want {
		t.Fatalf("order = %v, want %s", order, want)
	}
}

// A name fallback is only trusted when exactly one Respond organization has
// that name. Two same-name orgs with conflicting protection states must not
// be resolved by an arbitrary pick.
func TestCoverageAmbiguousOrgNameMatch(t *testing.T) {
	saasIsolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []saasM{
		{"id": "CUST_E", "name": "Elm Clinic", "status": "active", "countries": []any{}},
		{"id": "CUST_F", "name": "Fir Labs", "status": "active", "countries": []any{}},
	} {
		if err := db.UpsertCustomers(saasMustJSON(t, c)); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []saasM{
		// Protected: Active connection and a rule.
		{"id": "ORG_E1", "name": "Elm Clinic", "applicationConnections": []any{saasM{"id": "APP_E1", "status": "Active"}}},
		// Same name, opposite state: broken connection, no rules.
		{"id": "ORG_E2", "name": "ELM CLINIC", "applicationConnections": []any{saasM{"id": "APP_E2", "status": "Broken", "lastError": saasM{"message": "grant expired"}}}},
		{"id": "ORG_F", "name": "fir labs", "applicationConnections": []any{saasM{"id": "APP_F", "status": "Active"}}},
	} {
		if err := db.UpsertOrganizations(saasMustJSON(t, o)); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []saasM{
		{"id": "RULE_E", "status": "ACTIVE", "organizations": saasM{"type": "SPECIFIC", "organizations": []any{saasM{"id": "ORG_E1"}}}},
		{"id": "RULE_F", "status": "ACTIVE", "organizations": saasM{"type": "SPECIFIC", "organizations": []any{saasM{"id": "ORG_F"}}}},
	} {
		if err := db.UpsertRules(saasMustJSON(t, r)); err != nil {
			t.Fatal(err)
		}
	}
	events := []json.RawMessage{
		saasMustJSON(t, saasEventDoc("EV-E1", time.Hour, "CUST_E", "Elm Clinic", "ed@example.com", "Microsoft", "low", "login.success", saasLogin, "192.0.2.21", "US")),
		saasMustJSON(t, saasEventDoc("EV-F1", time.Hour, "CUST_F", "Fir Labs", "fay@example.com", "Microsoft", "low", "login.success", saasLogin, "192.0.2.22", "US")),
	}
	if _, failures, err := db.UpsertBatch("events", events); err != nil || failures != 0 {
		t.Fatalf("seed events: failures=%d err=%v", failures, err)
	}
	for _, r := range []string{"customers", "events", "organizations", "rules"} {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	// Decoded into a local shape so the test pins the JSON contract.
	var got struct {
		Customers []struct {
			CustomerID       string   `json:"customer_id"`
			Match            string   `json:"match"`
			MatchInferred    bool     `json:"match_inferred"`
			Candidates       []string `json:"respond_org_candidates"`
			RespondOrgID     string   `json:"respond_org_id"`
			RespondStatus    string   `json:"respond_status"`
			RespondLastError string   `json:"respond_last_error"`
			RuleCount        *int     `json:"rule_count"`
			Flags            []string `json:"flags"`
		} `json:"customers"`
		Summary map[string]int `json:"summary"`
	}
	stdout, _ := saasRunJSON(t, &got, "coverage", "--db", dbPath, "--json")
	if len(got.Customers) != 2 {
		t.Fatalf("customers = %d", len(got.Customers))
	}
	elm, fir := got.Customers[0], got.Customers[1]
	if elm.CustomerID != "CUST_E" {
		t.Fatalf("ambiguous customer must sort first (weighted like RESPOND_NOT_CONNECTED): %s", stdout)
	}
	if elm.Match != "ambiguous" || strings.Join(elm.Candidates, ",") != "ORG_E1,ORG_E2" ||
		elm.RespondOrgID != "" || elm.RespondStatus != "" || elm.RespondLastError != "" || elm.RuleCount != nil ||
		strings.Join(elm.Flags, ",") != "AMBIGUOUS_ORG_MATCH" {
		t.Errorf("ambiguous match = %+v (rule_count %v)", elm, elm.RuleCount)
	}
	if fir.Match != "name" || !fir.MatchInferred || fir.RespondOrgID != "ORG_F" || fir.RuleCount == nil || *fir.RuleCount != 1 || len(fir.Flags) != 0 {
		t.Errorf("unique name match = %+v", fir)
	}
	if got.Summary["ambiguous_org_match"] != 1 || got.Summary["respond_disconnected"] != 0 || got.Summary["no_rules"] != 0 || got.Summary["respond_not_connected"] != 0 {
		t.Errorf("summary = %v", got.Summary)
	}
}

// saasCoverageRow is the JSON contract of one coverage row, decoded locally so
// tests pin field names rather than reuse the command's own struct.
type saasCoverageRow struct {
	CustomerID       string   `json:"customer_id"`
	Match            string   `json:"match"`
	MatchInferred    bool     `json:"match_inferred"`
	Candidates       []string `json:"respond_org_candidates"`
	RespondOrgID     string   `json:"respond_org_id"`
	RespondStatus    string   `json:"respond_status"`
	RespondLastError string   `json:"respond_last_error"`
	RuleCount        *int     `json:"rule_count"`
	UnknownRuleRefs  *int     `json:"unknown_rule_refs"`
	Flags            []string `json:"flags"`
}

type saasCoverageEnvelope struct {
	Synced            bool              `json:"synced"`
	Note              string            `json:"note"`
	UnsyncedResources []string          `json:"unsynced_resources"`
	Customers         []saasCoverageRow `json:"customers"`
	Summary           map[string]int    `json:"summary"`
}

// saasSeedRespondStore seeds customers (one recent event each), Respond
// organizations and rules, and records sync_state for the given resources.
func saasSeedRespondStore(t *testing.T, customers, orgs, rules []saasM, synced ...string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var events []json.RawMessage
	for i, c := range customers {
		if err := db.UpsertCustomers(saasMustJSON(t, c)); err != nil {
			t.Fatal(err)
		}
		id, name := c["id"].(string), c["name"].(string)
		events = append(events, saasMustJSON(t, saasEventDoc("EV-"+id, time.Hour, id, name, fmt.Sprintf("user%d@example.com", i), "Microsoft", "low", "login.success", saasLogin, fmt.Sprintf("192.0.2.%d", 30+i), "US")))
	}
	if _, failures, err := db.UpsertBatch("events", events); err != nil || failures != 0 {
		t.Fatalf("seed events: failures=%d err=%v", failures, err)
	}
	for _, o := range orgs {
		if err := db.UpsertOrganizations(saasMustJSON(t, o)); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range rules {
		if err := db.UpsertRules(saasMustJSON(t, r)); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range synced {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	return dbPath
}

// Only confirmed ACTIVE synced rules protect an org. Unsynced ruleIds and
// rules without a status are unknown: RULES_UNKNOWN, never a silent pass.
func TestCoverageRulesUnknownVersusConfirmed(t *testing.T) {
	saasIsolate(t)
	active := func(id string, orgs ...string) saasM {
		list := []any{}
		for _, o := range orgs {
			list = append(list, saasM{"id": o})
		}
		return saasM{"id": id, "status": "ACTIVE", "organizations": saasM{"type": "SPECIFIC", "organizations": list}}
	}
	noStatus := active("RULE_NOSTATUS", "ORG_R2", "ORG_R3")
	delete(noStatus, "status")
	customers := []saasM{
		{"id": "ORG_R1", "name": "Rowan Health", "status": "active"},
		{"id": "ORG_R2", "name": "Spruce Law", "status": "active"},
		{"id": "ORG_R3", "name": "Tamarack Dental", "status": "active"},
		{"id": "ORG_R4", "name": "Umber Labs", "status": "active"},
	}
	conn := []any{saasM{"id": "APP", "status": "Active"}}
	orgs := []saasM{
		{"id": "ORG_R1", "name": "Rowan Health", "applicationConnections": conn, "ruleIds": []any{"RULE_NOT_SYNCED"}},
		{"id": "ORG_R2", "name": "Spruce Law", "applicationConnections": conn},
		{"id": "ORG_R3", "name": "Tamarack Dental", "applicationConnections": conn},
		{"id": "ORG_R4", "name": "Umber Labs", "applicationConnections": conn},
	}
	rules := []saasM{active("RULE_ACTIVE", "ORG_R3"), noStatus, {"id": "RULE_DRAFT", "status": "DRAFT", "organizations": saasM{"type": "ALL"}}}
	db := saasSeedRespondStore(t, customers, orgs, rules, "customers", "events", "organizations", "organizations-disconnected", "rules")

	var got saasCoverageEnvelope
	saasRunJSON(t, &got, "coverage", "--db", db, "--json")
	byID := map[string]saasCoverageRow{}
	for _, c := range got.Customers {
		byID[c.CustomerID] = c
	}
	check := func(id string, confirmed, unknown int, flags string) {
		t.Helper()
		c := byID[id]
		if c.RuleCount == nil || *c.RuleCount != confirmed || c.UnknownRuleRefs == nil || *c.UnknownRuleRefs != unknown || strings.Join(c.Flags, ",") != flags {
			t.Errorf("%s: rule_count=%v unknown_rule_refs=%v flags=%v, want %d/%d/%q", id, c.RuleCount, c.UnknownRuleRefs, c.Flags, confirmed, unknown, flags)
		}
	}
	check("ORG_R1", 0, 1, "RULES_UNKNOWN") // only an unsynced ruleIds reference
	check("ORG_R2", 0, 1, "RULES_UNKNOWN") // only a status-less rule
	check("ORG_R3", 1, 1, "")              // one confirmed ACTIVE rule protects it
	check("ORG_R4", 0, 0, "NO_RULES")      // DRAFT rules never count
	if got.Summary["rules_unknown"] != 2 || got.Summary["no_rules"] != 1 || !got.Synced || len(got.UnsyncedResources) != 0 {
		t.Errorf("summary=%v synced=%v unsynced=%v", got.Summary, got.Synced, got.UnsyncedResources)
	}

	// Respond resources never synced: say so instead of reporting gaps as fact.
	partial := saasSeedRespondStore(t, customers, orgs, rules, "customers", "events")
	var p saasCoverageEnvelope
	saasRunJSON(t, &p, "coverage", "--db", partial, "--json")
	if p.Synced || strings.Join(p.UnsyncedResources, ",") != "organizations,rules" ||
		!strings.HasPrefix(p.Note, "local store has no synced organizations or rules;") {
		t.Errorf("unsynced Respond: synced=%v unsynced=%v note=%q", p.Synced, p.UnsyncedResources, p.Note)
	}
}

// --agent compacts list rows to keys most rows carry; respond_org_candidates
// appears only on ambiguous rows, so it must be in the keep list to survive.
func TestCoverageAgentKeepsAmbiguousCandidates(t *testing.T) {
	saasIsolate(t)
	customers := []saasM{
		{"id": "CUST_E", "name": "Elm Clinic", "status": "active"},
		{"id": "CUST_F", "name": "Fir Labs", "status": "active"},
		{"id": "CUST_G", "name": "Gum Tree", "status": "active"},
	}
	orgs := []saasM{
		{"id": "ORG_E1", "name": "Elm Clinic", "applicationConnections": []any{saasM{"id": "APP_E1", "status": "Active"}}},
		{"id": "ORG_E2", "name": "elm clinic", "applicationConnections": []any{saasM{"id": "APP_E2", "status": "Broken"}}},
		{"id": "ORG_F", "name": "Fir Labs", "applicationConnections": []any{saasM{"id": "APP_F", "status": "Active"}}},
	}
	rules := []saasM{{"id": "RULE_F", "status": "ACTIVE", "organizations": saasM{"type": "SPECIFIC", "organizations": []any{saasM{"id": "ORG_F"}}}}}
	db := saasSeedRespondStore(t, customers, orgs, rules, "customers", "events", "organizations", "rules")

	var agent struct {
		Results saasCoverageEnvelope `json:"results"`
	}
	saasRunJSON(t, &agent, "coverage", "--db", db, "--agent")
	rows := map[string]saasCoverageRow{}
	for _, c := range agent.Results.Customers {
		rows[c.CustomerID] = c
	}
	if len(rows) != 3 {
		t.Fatalf("--agent customers = %d, want 3", len(rows))
	}
	if elm := rows["CUST_E"]; elm.Match != "ambiguous" || strings.Join(elm.Candidates, ",") != "ORG_E1,ORG_E2" {
		t.Errorf("--agent dropped ambiguous candidates: %+v", elm)
	}
	if fir := rows["CUST_F"]; fir.RuleCount == nil || *fir.RuleCount != 1 || fir.UnknownRuleRefs == nil {
		t.Errorf("--agent row = %+v", fir)
	}
}

func TestCoverageFutureEventDoesNotHideSilence(t *testing.T) {
	saasIsolate(t)
	dbPath := saasSeedFixtureStore(t)
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// A parseable but future-dated event (clock skew, bad source row) for cedar,
	// whose newest real event is 6h old.
	future := saasEventDoc("EV-C9", -48*time.Hour, "CUST_C", "Cedar Clinic", "carl.jones@example.com", "Microsoft", "low", "login.success", saasLogin, "192.0.2.99", "CA")
	if err := db.UpsertEvents(saasMustJSON(t, future)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var got saasCoverageResult
	saasRunJSON(t, &got, "coverage", "--db", dbPath, "--silent", "4h", "--json")
	for _, c := range got.Customers {
		if c.CustomerID != "CUST_C" {
			continue
		}
		if !strings.Contains(strings.Join(c.Flags, ","), "SILENT") || c.EventAge == nil || *c.EventAge != "6h" {
			t.Fatalf("a future event hid cedar's 6h silence: flags=%v age=%v", c.Flags, c.EventAge)
		}
		return
	}
	t.Fatal("cedar missing from coverage")
}
