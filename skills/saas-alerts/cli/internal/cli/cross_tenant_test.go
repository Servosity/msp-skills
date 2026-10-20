// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// TestNovelCrossTenantHelpWires smoke-tests that the cross-tenant command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelCrossTenantHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"cross-tenant", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cross-tenant --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "cross-tenant"} {
		if !strings.Contains(help, want) {
			t.Fatalf("cross-tenant --help missing %q in output:\n%s", want, help)
		}
	}
}

func saasSourceKeys(rows []saasCrossTenantRow) []string {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	return keys
}

func saasCustomerIDs(cs []saasCrossTenantCustomer) string {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ",")
}

func TestCrossTenantByIPExcludesWhitelistedSources(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasCrossTenantResult
	stdout, _ := saasRunJSON(t, &got, "cross-tenant", "--db", db, "--json")
	saasRequireKeys(t, stdout, "window_start", "window_end", "by", "min_customers", "synced", "scanned_events", "whitelist_exclusions", "total_sources", "sources")
	if got.By != "ip" || got.MinCustomers != 2 || got.ScannedEvents != 8 {
		t.Errorf("envelope = by %s min %d scanned %d", got.By, got.MinCustomers, got.ScannedEvents)
	}
	// 203.0.113.50 is inside Birch Legal's 203.0.113.0-255 whitelist: its two
	// Birch events drop, leaving Cedar alone, below --min-customers 2.
	if got.WhitelistExclusions != 2 || got.WhitelistNote != "" {
		t.Errorf("whitelist_exclusions = %d note %q, want 2 and no note", got.WhitelistExclusions, got.WhitelistNote)
	}
	if strings.Join(saasSourceKeys(got.Sources), ",") != "198.51.100.7" {
		t.Fatalf("sources = %v, want only 198.51.100.7", saasSourceKeys(got.Sources))
	}
	src := got.Sources[0]
	if saasCustomerIDs(src.Customers) != "CUST_A,CUST_B" || src.Customers[0].Name != "Acme Dental" {
		t.Errorf("customers = %+v", src.Customers)
	}
	if src.EventCount != 3 || src.MaxSeverity != "critical" {
		t.Errorf("event_count %d max_severity %s, want 3 critical (deduped share counts once)", src.EventCount, src.MaxSeverity)
	}
	if strings.Join(src.Users, ",") != "bob.smith@example.com,jane.doe@example.com" {
		t.Errorf("users = %v", src.Users)
	}
	if strings.Join(src.JointDescs, "|") != saasShare+"|"+saasBrute {
		t.Errorf("joint_descs = %v", src.JointDescs)
	}
	threat, ok := src.Threat.(map[string]any)
	if !ok || threat["is_known_attacker"] != true {
		t.Errorf("threat = %#v, want ipInfo.threat with is_known_attacker", src.Threat)
	}
}

func TestCrossTenantIncludeWhitelistedAndOtherDimensions(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)

	var incl saasCrossTenantResult
	saasRunJSON(t, &incl, "cross-tenant", "--db", db, "--include-whitelisted", "--json")
	if strings.Join(saasSourceKeys(incl.Sources), ",") != "198.51.100.7,203.0.113.50" || incl.WhitelistExclusions != 0 {
		t.Fatalf("--include-whitelisted sources = %v exclusions %d", saasSourceKeys(incl.Sources), incl.WhitelistExclusions)
	}
	if wl := incl.Sources[1]; saasCustomerIDs(wl.Customers) != "CUST_B,CUST_C" || wl.EventCount != 3 {
		t.Errorf("203.0.113.50 = %s x%d, want CUST_B,CUST_C x3", saasCustomerIDs(wl.Customers), wl.EventCount)
	}

	// Acme whitelists country US, so its two US events drop; US still hits
	// Birch and Cedar. DE (Acme only) and CA (Cedar only) stay below 2.
	var country saasCrossTenantResult
	saasRunJSON(t, &country, "cross-tenant", "--db", db, "--by", "country", "--json")
	if strings.Join(saasSourceKeys(country.Sources), ",") != "US" || country.WhitelistExclusions != 2 {
		t.Fatalf("--by country = %v exclusions %d", saasSourceKeys(country.Sources), country.WhitelistExclusions)
	}
	if us := country.Sources[0]; saasCustomerIDs(us.Customers) != "CUST_B,CUST_C" || us.EventCount != 4 {
		t.Errorf("US = %s x%d, want CUST_B,CUST_C x4", saasCustomerIDs(us.Customers), us.EventCount)
	}

	var asn saasCrossTenantResult
	saasRunJSON(t, &asn, "cross-tenant", "--db", db, "--by", "asn", "--json")
	if len(asn.Sources) != 1 || asn.Sources[0].Key != "AS64500" || asn.Sources[0].Label != "Example Transit" || saasCustomerIDs(asn.Sources[0].Customers) != "CUST_A,CUST_B" {
		t.Fatalf("--by asn = %+v", asn.Sources)
	}

	var three saasCrossTenantResult
	saasRunJSON(t, &three, "cross-tenant", "--db", db, "--min-customers", "3", "--include-whitelisted", "--json")
	if len(three.Sources) != 0 || three.TotalSources != 0 {
		t.Fatalf("--min-customers 3 = %v, want none", saasSourceKeys(three.Sources))
	}

	var one saasCrossTenantResult
	saasRunJSON(t, &one, "cross-tenant", "--db", db, "--min-customers", "1", "--limit", "2", "--json")
	if len(one.Sources) != 2 || one.TotalSources != 4 || one.Sources[0].Key != "198.51.100.7" {
		t.Fatalf("--min-customers 1 --limit 2 = %v of %d", saasSourceKeys(one.Sources), one.TotalSources)
	}
}

func TestCrossTenantWithoutSyncedCustomersSkipsExclusion(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rw.DB().Exec(`DELETE FROM resources WHERE resource_type = 'customers'`); err != nil {
		t.Fatal(err)
	}
	rw.Close()
	var got saasCrossTenantResult
	saasRunJSON(t, &got, "cross-tenant", "--db", db, "--json")
	if got.WhitelistExclusions != 0 || !strings.Contains(got.WhitelistNote, "no synced customers") {
		t.Errorf("exclusions %d note %q", got.WhitelistExclusions, got.WhitelistNote)
	}
	if strings.Join(saasSourceKeys(got.Sources), ",") != "198.51.100.7,203.0.113.50" {
		t.Errorf("sources = %v", saasSourceKeys(got.Sources))
	}
}

func TestCrossTenantUsageErrorsAndMissingStore(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	for _, args := range [][]string{
		{"cross-tenant", "--db", db, "--by", "user"},
		{"cross-tenant", "--db", db, "--min-customers", "0"},
		{"cross-tenant", "--db", db, "--since", "-1h"},
	} {
		if _, _, err := saasRunCLI(t, args...); ExitCode(err) != 2 {
			t.Errorf("%v: err=%v, want exit 2", args, err)
		}
	}
	missing := saasMissingDBPath(t)
	var got saasCrossTenantResult
	stdout, stderr := saasRunJSON(t, &got, "cross-tenant", "--db", missing, "--by", "asn", "--json")
	if !strings.HasPrefix(stderr, "hint: no local mirror at "+missing) || got.By != "asn" || !strings.Contains(stdout, `"sources": []`) {
		t.Errorf("missing store: stdout=%s stderr=%q", stdout, stderr)
	}
}

func TestCrossTenantSelect(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	stdout, _, err := saasRunCLI(t, "cross-tenant", "--db", db, "--select", "whitelist_exclusions,sources.key", "--json")
	if err != nil {
		t.Fatal(err)
	}
	saasRequireKeys(t, stdout, "whitelist_exclusions", "sources")
	if !strings.Contains(stdout, `"key": "198.51.100.7"`) || strings.Contains(stdout, "event_count") {
		t.Errorf("--select output = %s", stdout)
	}
}

// The help text must not overclaim the whitelist behavior.
func TestCrossTenantHelpStatesWhitelistScope(t *testing.T) {
	cmd := newNovelCrossTenantCmd(&rootFlags{})
	for _, want := range []string{
		"own customer whitelist for the chosen --by dimension",
		"match by ASN name",
		"Partner-inherited whitelists are not applied",
	} {
		if !strings.Contains(cmd.Long, want) {
			t.Errorf("Long missing %q:\n%s", want, cmd.Long)
		}
	}
	if usage := cmd.Flags().Lookup("include-whitelisted").Usage; !strings.Contains(usage, "own whitelist for the --by dimension") || !strings.Contains(usage, "partner-inherited") {
		t.Errorf("--include-whitelisted usage = %q", usage)
	}
	if strings.Contains(cmd.Short, "each customer's whitelists") {
		t.Errorf("Short still overclaims: %q", cmd.Short)
	}
}

// At equal breadth, the worst severity outranks raw volume: a benign
// high-volume source must not push a critical one down (or out of --limit).
func TestCrossTenantRanksSeverityBeforeVolume(t *testing.T) {
	ev := func(asn, name, cust, sev string) saasLocalEvent {
		return saasLocalEvent{ASN: asn, ASNName: name, CustomerID: cust, CustomerName: cust, Severity: sev}
	}
	var events []saasLocalEvent
	for i := 0; i < 5; i++ {
		events = append(events, ev("AS15169", "GOOGLE", "CUST_1", "medium"), ev("AS15169", "GOOGLE", "CUST_2", "medium"))
	}
	events = append(events,
		ev("AS64500", "Example Transit", "CUST_1", "critical"),
		ev("AS64500", "Example Transit", "CUST_2", "low"),
		// Broader but low severity: breadth still ranks first.
		ev("AS64501", "Wide Low", "CUST_1", "low"), ev("AS64501", "Wide Low", "CUST_2", "low"), ev("AS64501", "Wide Low", "CUST_3", "low"),
	)
	result := saasCrossTenantResult{By: "asn", MinCustomers: 2}
	saasBuildCrossTenant(&result, events, nil, true, 2)
	if got := strings.Join(saasSourceKeys(result.Sources), ","); got != "AS64501,AS64500" {
		t.Fatalf("sources = %s, want AS64501 (3 customers) then AS64500 (critical) ahead of the 10-event medium AS15169", got)
	}
	if result.TotalSources != 3 || result.Sources[1].MaxSeverity != "critical" {
		t.Fatalf("total=%d second=%+v", result.TotalSources, result.Sources[1])
	}
}
