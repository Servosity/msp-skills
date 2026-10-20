// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// TestNovelInvestigateHelpWires smoke-tests that the investigate command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelInvestigateHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"investigate", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("investigate --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "investigate"} {
		if !strings.Contains(help, want) {
			t.Fatalf("investigate --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestInvestigateAccountFullPicture(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasInvestigateResult
	stdout, _ := saasRunJSON(t, &got, "investigate", "jane.doe@example.com", "--db", db, "--json")
	saasRequireKeys(t, stdout, "email", "window_start", "window_end", "synced", "unsynced_resources", "ambiguous_id_matches", "events", "ips", "country_hops", "user", "rule_triggers", "suppressions", "devices")
	if got.WindowStart != "2026-10-07T12:00:00Z" {
		t.Errorf("default --since 7d window_start = %s", got.WindowStart)
	}
	// EV-A1..A4 are inside 7d; EV-A5 (34d) is not.
	if got.Events.Total != 4 {
		t.Fatalf("events.total = %d, want 4", got.Events.Total)
	}
	if got.Events.ByProduct["Microsoft"] != 3 || got.Events.ByProduct["Google Workspace"] != 1 || len(got.Events.ByProduct) != 2 {
		t.Errorf("by_product = %v", got.Events.ByProduct)
	}
	if got.Events.BySeverity["critical"] != 2 || got.Events.BySeverity["medium"] != 1 || got.Events.BySeverity["low"] != 1 {
		t.Errorf("by_severity = %v", got.Events.BySeverity)
	}
	var recent []string
	for _, ev := range got.Events.Recent {
		recent = append(recent, ev.EventID)
	}
	if strings.Join(recent, ",") != "EV-A2,EV-A1,EV-A3,EV-A4" {
		t.Errorf("recent = %v, want newest first", recent)
	}
	var ips []string
	for _, ip := range got.IPs {
		ips = append(ips, ip.IP)
	}
	if strings.Join(ips, ",") != "198.51.100.7,185.220.101.4,192.0.2.10" || got.IPs[0].Count != 2 {
		t.Errorf("ips = %v", ips)
	}
	tor := got.IPs[1]
	if tor.ASN != "AS205100" || tor.ASNName != "F3 Netze e.V." || strings.Join(tor.Countries, ",") != "DE" {
		t.Errorf("tor ip = %+v", tor)
	}
	if threat, ok := tor.Threat.(map[string]any); !ok || threat["is_tor"] != true {
		t.Errorf("tor threat = %#v", tor.Threat)
	}
	// GB (3d ago) -> DE is far outside --hop-window 2h; DE -> US is 1h.
	if len(got.CountryHops) != 1 {
		t.Fatalf("country_hops = %+v, want exactly DE->US", got.CountryHops)
	}
	hop := got.CountryHops[0]
	if hop.From != "DE" || hop.To != "US" || hop.GapSeconds != 3600 || hop.Gap != "1h" || hop.FromIP != "185.220.101.4" || hop.ToIP != "198.51.100.7" ||
		hop.FromTime != "2026-10-14T10:00:00Z" || hop.ToTime != "2026-10-14T11:00:00Z" {
		t.Errorf("hop = %+v", hop)
	}
	u := got.User
	if u == nil {
		t.Fatal("user = null, want the synced Microsoft record")
	}
	if u.ID != "U-JANE-MS" || u.CustomerID != "CUST_A" || u.CustomerName != "Acme Dental" || u.Product != "Microsoft" || u.Records != 2 ||
		strings.Join(u.Products, ",") != "Dropbox,Microsoft" || strings.Join(u.Licenses, ",") != "SKU-E3" ||
		u.AccountEnabled == nil || !*u.AccountEnabled || u.IsServiceAccount != nil || u.LastEvent == nil || u.LastEvent.Country != "US" {
		t.Errorf("user = %+v", u)
	}
	if len(got.RuleTriggers) != 1 || got.RuleTriggers[0].ID != "RULE_TRIGGER_ID_1" || got.RuleTriggers[0].RuleName != "Brute force lockout" ||
		got.RuleTriggers[0].Status != "PENDING" || got.RuleTriggers[0].TriggeredAt != "2026-10-14T11:40:00Z" {
		t.Errorf("rule_triggers = %+v", got.RuleTriggers)
	}
	if len(got.Suppressions) != 1 || got.Suppressions[0].ID != "SUPP_ID_1" || got.Suppressions[0].Match != "users" {
		t.Errorf("suppressions = %+v", got.Suppressions)
	}
	// The device is synced under both devices and devices-mapped: one row.
	if len(got.Devices) != 1 || got.Devices[0].DeviceName != "ACME-LT-042" || got.Devices[0].MappedBy != "tech@msp.example" ||
		got.Devices[0].Confidence == nil || *got.Devices[0].Confidence != 0.9 {
		t.Errorf("devices = %+v", got.Devices)
	}
}

func TestInvestigateCaseInsensitiveAndWindows(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)

	var upper saasInvestigateResult
	saasRunJSON(t, &upper, "investigate", "JANE.DOE@EXAMPLE.COM", "--db", db, "--json")
	if upper.Events.Total != 4 || upper.User == nil || len(upper.RuleTriggers) != 1 || len(upper.Suppressions) != 1 || len(upper.Devices) != 1 {
		t.Fatalf("uppercase email lost matches: total %d user %v triggers %d", upper.Events.Total, upper.User, len(upper.RuleTriggers))
	}

	var tight saasInvestigateResult
	saasRunJSON(t, &tight, "investigate", "jane.doe@example.com", "--db", db, "--hop-window", "30m", "--json")
	if len(tight.CountryHops) != 0 {
		t.Errorf("--hop-window 30m hops = %+v, want none", tight.CountryHops)
	}

	var wide saasInvestigateResult
	saasRunJSON(t, &wide, "investigate", "jane.doe@example.com", "--db", db, "--since", "5w", "--json")
	if wide.Events.Total != 5 || wide.Events.ByProduct["Microsoft"] != 4 {
		t.Errorf("--since 5w total = %d by_product %v, want 5 incl. EV-A5", wide.Events.Total, wide.Events.ByProduct)
	}

	var bob saasInvestigateResult
	saasRunJSON(t, &bob, "investigate", "bob.smith@example.com", "--db", db, "--json")
	// Bob's empty-eventId event is stored twice and must count once.
	if bob.Events.Total != 3 || bob.User == nil || *bob.User.AccountEnabled || len(bob.RuleTriggers) != 1 || bob.RuleTriggers[0].ID != "RULE_TRIGGER_ID_2" {
		t.Fatalf("bob = total %d user %+v triggers %+v", bob.Events.Total, bob.User, bob.RuleTriggers)
	}
	if len(bob.Suppressions) != 0 || len(bob.Devices) != 0 || len(bob.CountryHops) != 0 {
		t.Errorf("bob leaked jane's rows: supp %v dev %v hops %v", bob.Suppressions, bob.Devices, bob.CountryHops)
	}
}

func TestInvestigateUnknownEmailReturnsEmptySections(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasInvestigateResult
	stdout, _ := saasRunJSON(t, &got, "investigate", "nobody@example.com", "--db", db, "--json")
	if got.Email != "nobody@example.com" || got.Events.Total != 0 || got.User != nil {
		t.Fatalf("unknown email = %+v", got)
	}
	for _, want := range []string{`"recent": []`, `"ips": []`, `"country_hops": []`, `"user": null`, `"rule_triggers": []`, `"suppressions": []`, `"devices": []`, `"by_product": {}`, `"critical": 0`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("unknown email output missing %s:\n%s", want, stdout)
		}
	}
}

func TestInvestigateInputErrorsAndMissingStore(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)

	stdout, _, err := saasRunCLI(t, "investigate")
	if err != nil || !strings.Contains(stdout, "Usage:") {
		t.Errorf("bare investigate: err=%v, want help text; got %q", err, stdout)
	}
	if _, _, err := saasRunCLI(t, "investigate", "--db", db); ExitCode(err) != 2 {
		t.Errorf("missing email with flags: err=%v, want exit 2", err)
	}
	if _, _, err := saasRunCLI(t, "investigate", "a@example.com", "b@example.com", "--db", db); ExitCode(err) != 2 {
		t.Errorf("two emails: err=%v, want exit 2", err)
	}
	if _, _, err := saasRunCLI(t, "investigate", "a@example.com", "--hop-window", "soon", "--db", db); ExitCode(err) != 2 {
		t.Errorf("bad --hop-window: err=%v, want exit 2", err)
	}

	missing := saasMissingDBPath(t)
	var got saasInvestigateResult
	out, stderr := saasRunJSON(t, &got, "investigate", "jane.doe@example.com", "--db", missing, "--json")
	if !strings.HasPrefix(stderr, "hint: no local mirror at "+missing) || got.Email != "jane.doe@example.com" || got.User != nil || !strings.Contains(out, `"devices": []`) {
		t.Errorf("missing store: stdout=%s stderr=%q", out, stderr)
	}
}

func TestInvestigateSelect(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	stdout, _, err := saasRunCLI(t, "investigate", "jane.doe@example.com", "--db", db, "--select", "country_hops,user.records", "--json")
	if err != nil {
		t.Fatal(err)
	}
	saasRequireKeys(t, stdout, "country_hops", "user")
	if !strings.Contains(stdout, `"records": 2`) || strings.Contains(stdout, "licenses") || !strings.Contains(stdout, `"to": "US"`) {
		t.Errorf("--select output = %s", stdout)
	}
}

func TestInvestigateSuppressionMatching(t *testing.T) {
	docs := []map[string]any{
		{"id": "S-USERS", "scope": "users", "users": []any{"Jane.Doe@example.com"}},
		{"id": "S-FILTER", "scope": "customer", "filters": map[string]any{"jointType": "x", "user.name": []any{"other@example.com", "jane.doe@example.com"}}},
		{"id": "S-INFERRED", "scope": "customer", "filters": map[string]any{"jointType": "x", "account.email": "jane.doe@example.com"}},
		{"id": "S-AUDIT-ONLY", "scope": "customer", "filters": map[string]any{"jointType": "x"}, "lastUpdatedBy": "jane.doe@example.com"},
		{"id": "S-OTHER", "scope": "users", "users": []any{"bob.smith@example.com"}},
	}
	got := saasInvestigateMatchSuppressions(docs, "jane.doe@example.com")
	var pairs []string
	for _, s := range got {
		pairs = append(pairs, s.ID+":"+s.Match)
	}
	if strings.Join(pairs, ",") != "S-FILTER:filter,S-INFERRED:inferred,S-USERS:users" {
		t.Fatalf("suppression matches = %v", pairs)
	}
}

// The Reports example puts a display name in user.name; the account email
// lives in the top-level userEmail (or the deprecated email) field.
func TestInvestigateMatchesUserEmailFields(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	ev := func(id, userName, field, value string, ago time.Duration) json.RawMessage {
		d := saasEventDoc(id, ago, "CUST_C", "Cedar Clinic", userName, "Microsoft", "medium", "outside.own.location", saasOutside, "192.0.2.150", "US")
		d["user"] = saasM{"id": "uid-charles", "name": userName}
		if field != "" {
			d[field] = value
		}
		return saasMustJSON(t, d)
	}
	items := []json.RawMessage{
		ev("EV-CH1", "Charles Buck", "userEmail", "charles@example.com", time.Hour),
		ev("EV-CH2", "Charles Buck", "email", "Charles@Example.com", 2*time.Hour),
		ev("EV-CH3", "charles@example.com", "", "", 3*time.Hour),
		ev("EV-CH4", "Charles Buck", "", "", 4*time.Hour),                                  // display name only: not attributable
		ev("EV-CH5", "Charles Buck", "userEmail", "someone.else@example.com", 5*time.Hour), // a different account
	}
	if _, failures, err := rw.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	rw.Close()

	for _, email := range []string{"charles@example.com", "CHARLES@EXAMPLE.COM"} {
		var got saasInvestigateResult
		saasRunJSON(t, &got, "investigate", email, "--db", db, "--json")
		var ids []string
		for _, r := range got.Events.Recent {
			ids = append(ids, r.EventID)
		}
		if got.Events.Total != 3 || strings.Join(ids, ",") != "EV-CH1,EV-CH2,EV-CH3" {
			t.Errorf("investigate %s: total %d recent %v, want EV-CH1,EV-CH2,EV-CH3", email, got.Events.Total, ids)
		}
	}
	var jane saasInvestigateResult
	saasRunJSON(t, &jane, "investigate", "jane.doe@example.com", "--db", db, "--json")
	if jane.Events.Total != 4 {
		t.Errorf("jane total = %d, want 4 (unchanged)", jane.Events.Total)
	}
}

// Shaped like the Reports example: the event carries only user {id, name}
// with a display name and no email field. A synced user row maps that id to
// the email, so investigate resolves the id first and finds the event.
func TestInvestigateResolvesAccountIDsFromSyncedUsers(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	const charlesID = "14a1fec5-9cc7-4e52-a89b-a12312fefc86"
	if err := rw.UpsertUsers(saasMustJSON(t, saasM{
		"id": charlesID, "name": "Charles Buck", "email": "charles@example.com", "isBillable": true, "accountEnabled": true,
		"product": saasM{"id": "prod-Microsoft", "name": "Microsoft"}, "time": "2026-10-01 10:00:00", "customerConnectionId": "cc-9",
		"parent_id": "CUST_C", "customers_id": "CUST_C",
	})); err != nil {
		t.Fatal(err)
	}
	byNestedID := saasEventDoc("EV-CB1", time.Hour, "CUST_C", "Cedar Clinic", "Charles Buck", "Microsoft", "critical", "login.failure.brute", saasBrute, "192.0.2.160", "US")
	byNestedID["user"] = saasM{"id": charlesID, "name": "Charles Buck"}
	byTopLevelID := saasEventDoc("EV-CB2", 2*time.Hour, "CUST_C", "Cedar Clinic", "Charles Buck", "Microsoft", "low", "login.success", saasLogin, "192.0.2.161", "US")
	byTopLevelID["user"] = saasM{"name": "Charles Buck"}
	byTopLevelID["UserId"] = charlesID
	otherAccount := saasEventDoc("EV-CB3", 3*time.Hour, "CUST_C", "Cedar Clinic", "Charles Buck", "Microsoft", "low", "login.success", saasLogin, "192.0.2.162", "US")
	otherAccount["user"] = saasM{"id": "another-account-id", "name": "Charles Buck"}
	if _, failures, err := rw.UpsertBatch("events", []json.RawMessage{saasMustJSON(t, byNestedID), saasMustJSON(t, byTopLevelID), saasMustJSON(t, otherAccount)}); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	rw.Close()

	var got saasInvestigateResult
	saasRunJSON(t, &got, "investigate", "Charles@Example.com", "--db", db, "--json")
	var ids []string
	for _, r := range got.Events.Recent {
		ids = append(ids, r.EventID)
	}
	if got.Events.Total != 2 || strings.Join(ids, ",") != "EV-CB1,EV-CB2" {
		t.Fatalf("total %d recent %v, want EV-CB1,EV-CB2 (same display name, other account id excluded)", got.Events.Total, ids)
	}
	if got.User == nil || got.User.ID != charlesID {
		t.Errorf("user = %+v", got.User)
	}
}

// Account ids are only unique within a tenant and product: an event is
// attributed by id only when its customer is the synced user's customer and
// its product agrees with the user's product, and never when the event names
// a different account by email.
func TestInvestigateIDAttributionIsTenantAndProductScoped(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	const shared = "acct-shared-7"
	user := func(email, cust, prodID, prodName string) json.RawMessage {
		return saasMustJSON(t, saasM{"id": shared, "name": "Dana Display", "email": email, "isBillable": true, "accountEnabled": true,
			"product": saasM{"id": prodID, "name": prodName}, "time": "2026-10-01 10:00:00", "customerConnectionId": "cc-" + cust,
			"parent_id": cust, "customers_id": cust})
	}
	// The same raw id belongs to Dana at Acme (Microsoft) and to someone else at Birch.
	for _, u := range []json.RawMessage{user("dana@example.com", "CUST_A", "prod-Microsoft", "Microsoft"), user("other@example.com", "CUST_B", "prod-Microsoft", "Microsoft")} {
		if err := rw.UpsertUsers(u); err != nil {
			t.Fatal(err)
		}
	}
	ev := func(id, cust, custName, product string, ago time.Duration, extra saasM) json.RawMessage {
		d := saasEventDoc(id, ago, cust, custName, "Dana Display", product, "medium", "outside.own.location", saasOutside, "192.0.2.170", "US")
		d["user"] = saasM{"id": shared, "name": "Dana Display"}
		for k, v := range extra {
			d[k] = v
		}
		return saasMustJSON(t, d)
	}
	items := []json.RawMessage{
		ev("EV-DANA", "CUST_A", "Acme Dental", "Microsoft", time.Hour, nil),                                                   // Dana: tenant and product match
		ev("EV-BIRCH", "CUST_B", "Birch Legal", "Microsoft", 2*time.Hour, nil),                                                // same id, other tenant
		ev("EV-GWS", "CUST_A", "Acme Dental", "Google Workspace", 3*time.Hour, nil),                                           // same id, other product
		ev("EV-OTHERMAIL", "CUST_A", "Acme Dental", "Microsoft", 4*time.Hour, saasM{"userEmail": "someone.else@example.com"}), // explicit other account
	}
	if _, failures, err := rw.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	rw.Close()

	recentIDs := func(email string) string {
		var got saasInvestigateResult
		saasRunJSON(t, &got, "investigate", email, "--db", db, "--json")
		var ids []string
		for _, r := range got.Events.Recent {
			ids = append(ids, r.EventID)
		}
		return strings.Join(ids, ",")
	}
	if got := recentIDs("dana@example.com"); got != "EV-DANA" {
		t.Errorf("dana@example.com events = %q, want only EV-DANA", got)
	}
	if got := recentIDs("other@example.com"); got != "EV-BIRCH" {
		t.Errorf("other@example.com events = %q, want only EV-BIRCH", got)
	}
	if got := recentIDs("someone.else@example.com"); got != "EV-OTHERMAIL" {
		t.Errorf("someone.else@example.com events = %q, want only EV-OTHERMAIL", got)
	}
}

// ID-only attribution needs POSITIVE product agreement. An email-less event
// with the account's id in the account's tenant but no comparable product
// (missing product, or an id on one side and only a name on the other) is
// not attributed; it is counted as ambiguous_id_matches instead.
func TestInvestigateIDAttributionNeedsProductAgreement(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	const acct = "acct-erin-3"
	// The synced user row carries a product id only.
	if err := rw.UpsertUsers(saasMustJSON(t, saasM{"id": acct, "name": "Erin Display", "email": "erin@example.com", "isBillable": true, "accountEnabled": true,
		"product": saasM{"id": "prod-Microsoft"}, "time": "2026-10-01 10:00:00", "customerConnectionId": "cc-erin",
		"parent_id": "CUST_A", "customers_id": "CUST_A"})); err != nil {
		t.Fatal(err)
	}
	ev := func(id string, ago time.Duration, product any) json.RawMessage {
		d := saasEventDoc(id, ago, "CUST_A", "Acme Dental", "Erin Display", "Microsoft", "medium", "outside.own.location", saasOutside, "192.0.2.190", "US")
		d["user"] = saasM{"id": acct, "name": "Erin Display"}
		if product == nil {
			delete(d, "product")
		} else {
			d["product"] = product
		}
		return saasMustJSON(t, d)
	}
	items := []json.RawMessage{
		ev("EV-ERIN", time.Hour, saasM{"id": "prod-Microsoft", "name": "Microsoft"}),  // ids agree: attributed
		ev("EV-NOPROD", 2*time.Hour, nil),                                             // no product: ambiguous
		ev("EV-NAMEONLY", 3*time.Hour, saasM{"name": "Microsoft"}),                    // id vs name only: ambiguous
		ev("EV-OTHERPROD", 4*time.Hour, saasM{"id": "prod-Google", "name": "Google"}), // ids disagree: excluded, not ambiguous
	}
	if _, failures, err := rw.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	rw.Close()

	var got struct {
		Events struct {
			Total  int `json:"total"`
			Recent []struct {
				EventID string `json:"event_id"`
			} `json:"recent"`
		} `json:"events"`
		AmbiguousIDMatches *int   `json:"ambiguous_id_matches"`
		AmbiguousIDNote    string `json:"ambiguous_id_note"`
	}
	saasRunJSON(t, &got, "investigate", "erin@example.com", "--db", db, "--json")
	var ids []string
	for _, r := range got.Events.Recent {
		ids = append(ids, r.EventID)
	}
	if strings.Join(ids, ",") != "EV-ERIN" {
		t.Errorf("attributed events = %v, want only EV-ERIN", ids)
	}
	if got.AmbiguousIDMatches == nil || *got.AmbiguousIDMatches != 2 || !strings.Contains(got.AmbiguousIDNote, "product") {
		t.Errorf("ambiguous_id_matches=%v note=%q, want 2 with a product note", got.AmbiguousIDMatches, got.AmbiguousIDNote)
	}

	// No ambiguity: the field is present and zero, with no note.
	var jane struct {
		AmbiguousIDMatches *int    `json:"ambiguous_id_matches"`
		AmbiguousIDNote    *string `json:"ambiguous_id_note"`
	}
	saasRunJSON(t, &jane, "investigate", "jane.doe@example.com", "--db", db, "--json")
	if jane.AmbiguousIDMatches == nil || *jane.AmbiguousIDMatches != 0 || jane.AmbiguousIDNote != nil {
		t.Errorf("jane ambiguous=%v note=%v, want 0 and no note", jane.AmbiguousIDMatches, jane.AmbiguousIDNote)
	}
}

// users is stamped by the customers sync as a dependent. A completed
// customers sync with zero customers never stamps users, so investigate must
// not demand a users sync that can never complete; with customers present,
// an unstamped users stays unsynced.
func TestInvestigateUsersDependentSyncState(t *testing.T) {
	saasIsolate(t)
	run := func(withCustomer bool) (bool, []string) {
		dbPath := filepath.Join(t.TempDir(), "data.db")
		db, err := store.OpenWithContext(context.Background(), dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if withCustomer {
			// An _id-only customer is stored but skipped by the id-keyed
			// customer loader; it still makes users a real, unstamped gap.
			if err := db.UpsertCustomers(saasMustJSON(t, saasM{"_id": "CUST_A", "name": "Acme Dental", "status": "active"})); err != nil {
				t.Fatal(err)
			}
		}
		for _, r := range []string{"customers", "events", "rule-triggers", "suppressions", "devices"} {
			if err := db.SaveSyncState(r, "", 0); err != nil {
				t.Fatal(err)
			}
		}
		db.Close()
		var got struct {
			Synced            bool     `json:"synced"`
			UnsyncedResources []string `json:"unsynced_resources"`
		}
		saasRunJSON(t, &got, "investigate", "jane.doe@example.com", "--db", dbPath, "--json")
		return got.Synced, got.UnsyncedResources
	}
	if synced, unsynced := run(false); !synced || len(unsynced) != 0 {
		t.Errorf("zero customers: synced=%v unsynced=%v, want true and []", synced, unsynced)
	}
	if synced, unsynced := run(true); synced || strings.Join(unsynced, ",") != "users" {
		t.Errorf("customers present, users unstamped: synced=%v unsynced=%v, want false and [users]", synced, unsynced)
	}
}
