// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// saasFixtureNow is the pinned clock every local-reader test runs at.
var saasFixtureNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func saasPinNow(t *testing.T) {
	t.Helper()
	prev := saasLocalNow
	saasLocalNow = func() time.Time { return saasFixtureNow }
	t.Cleanup(func() { saasLocalNow = prev })
}

func saasAgo(d time.Duration) string { return saasFixtureNow.Add(-d).Format(time.RFC3339) }

func saasAgoMillis(d time.Duration) int64 { return saasFixtureNow.Add(-d).UnixMilli() }

// saasRunCLI executes the real root command and returns stdout, stderr, err.
func saasRunCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := RootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func saasMustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

type saasM = map[string]any

func saasEventDoc(eventID string, ago time.Duration, custID, custName, user, product, severity, jointType, jointDesc, ip, country string) saasM {
	return saasM{
		"time":        saasAgo(ago),
		"eventId":     eventID,
		"user":        saasM{"id": "uid-" + user, "name": user},
		"customer":    saasM{"id": custID, "name": custName},
		"partner":     saasM{"id": "coThCxxb3wqZ6gnqYL57", "name": "Example MSP"},
		"product":     saasM{"id": "prod-" + product, "name": product},
		"alertStatus": severity,
		"jointType":   jointType,
		"jointDesc":   jointDesc,
		"ip":          ip,
		"location":    saasM{"country": country, "region": "", "city": "", "ipInfo": saasM{"asn": nil, "threat": nil}},
	}
}

const (
	saasBrute   = "IAM Event - Brute Force Attack"
	saasOutside = "IAM Event - User Location - Outside approved location"
	saasShare   = "File Sharing - Shared Externally"
	saasLogin   = "IAM Event - Login Success"
)

// saasSeedFixtureStore writes a store that looks like real sync output:
// 4 customers (CUST_D never sends events), events across A/B/C including an
// empty-eventId duplicate pair, a shared IP (198.51.100.7 at A and B), an IP
// whitelisted by B (203.0.113.50), a DE->US hop for jane, and events outside
// the 24h and 7d windows; plus users, 2 rule triggers, 1 suppression, 1
// device (in both devices and devices-mapped), 2 Respond organizations (B's
// connection still reads Active but it is listed in organizations-disconnected,
// as in the published example) and 2 rules. Every seeded resource is
// recorded in sync_state, as a real sync would.
func saasSeedFixtureStore(t *testing.T) string {
	t.Helper()
	return saasSeedFixture(t, true)
}

// saasSyncedFixtureResources are the resources the fixture marks as synced.
var saasSyncedFixtureResources = []string{"customers", "events", "users", "rule-triggers", "suppressions", "devices", "devices-mapped", "organizations", "organizations-disconnected", "rules"}

// saasSeedFixture seeds the fixture rows; markSynced=false leaves sync_state
// empty (rows present but never recorded as synced).
func saasSeedFixture(t *testing.T, markSynced bool) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	customers := []saasM{
		{"id": "CUST_A", "name": "Acme Dental", "email": "it@acme.example", "status": "active", "partnerId": "coThCxxb3wqZ6gnqYL57",
			"createTime": "2021-03-29 14:00:40", "products": []any{},
			"countries": []any{saasM{"code": "US", "label": "United States", "phone": "1"}}, "ipRanges": []any{}, "asnList": []any{}, "inheritanceWl": false},
		{"id": "CUST_B", "name": "Birch Legal", "email": "it@birch.example", "status": "active", "partnerId": "coThCxxb3wqZ6gnqYL57",
			"createTime": "2021-03-29 14:00:40", "products": []any{}, "countries": []any{},
			"ipRanges": []any{saasM{"from": "203.0.113.0", "to": "203.0.113.255"}}, "asnList": []any{}},
		{"id": "CUST_C", "name": "Cedar Clinic", "email": "it@cedar.example", "status": "active", "partnerId": "coThCxxb3wqZ6gnqYL57",
			"createTime": "2021-03-29 14:00:40", "products": []any{}, "countries": []any{}},
		{"id": "CUST_D", "name": "Dune Partners", "email": "it@dune.example", "status": "active", "partnerId": "coThCxxb3wqZ6gnqYL57",
			"createTime": "2021-03-29 14:00:40", "products": []any{}, "countries": []any{}},
	}
	for _, c := range customers {
		if err := db.UpsertCustomers(saasMustJSON(t, c)); err != nil {
			t.Fatalf("seed customer: %v", err)
		}
	}

	jane, bob, carl := "jane.doe@example.com", "bob.smith@example.com", "carl.jones@example.com"
	a1 := saasEventDoc("EV-A1", 1*time.Hour, "CUST_A", "Acme Dental", jane, "Microsoft", "critical", "login.failure.brute", saasBrute, "198.51.100.7", "US")
	a1["pullTime"] = saasAgo(55 * time.Minute) // lag 300s
	a1["psaTicket"] = []any{saasM{"ticketNumber": "T-1001", "psaTicketId": "psa-1", "time": saasAgo(50 * time.Minute), "type": "autotask"}}
	a1["location"].(saasM)["ipInfo"] = saasM{"asn": saasM{"asn": "AS64500", "name": "Example Transit", "type": "isp", "route": "198.51.100.0/24", "domain": "transit.example"}, "threat": nil}
	a2 := saasEventDoc("EV-A2", 30*time.Minute, "CUST_A", "Acme Dental", jane, "Microsoft", "critical", "login.failure.brute", saasBrute, "198.51.100.7", "US")
	a2["pullTime"] = saasAgo(28 * time.Minute) // lag 120s
	a2["location"].(saasM)["ipInfo"] = a1["location"].(saasM)["ipInfo"]
	a3 := saasEventDoc("EV-A3", 2*time.Hour, "CUST_A", "Acme Dental", jane, "Microsoft", "medium", "outside.own.location", saasOutside, "185.220.101.4", "DE")
	a3["location"].(saasM)["ipInfo"] = saasM{"asn": saasM{"asn": "AS205100", "name": "F3 Netze e.V.", "type": "hosting", "route": "185.220.101.0/24", "domain": "f3netze.de"},
		"threat": saasM{"is_tor": true, "is_threat": true, "is_bogon": false, "is_known_abuser": true, "is_known_attacker": false, "is_anonymous": true, "is_proxy": false}}
	a4 := saasEventDoc("EV-A4", 3*24*time.Hour+3*time.Hour, "CUST_A", "Acme Dental", jane, "Google Workspace", "low", "login.success", saasLogin, "192.0.2.10", "GB")
	a5 := saasEventDoc("EV-A5", 34*24*time.Hour, "CUST_A", "Acme Dental", jane, "Microsoft", "critical", "login.failure.brute", saasBrute, "192.0.2.77", "FR")
	b2 := saasEventDoc("EV-B2", 4*time.Hour, "CUST_B", "Birch Legal", bob, "Microsoft", "medium", "outside.own.location", saasOutside, "203.0.113.50", "US")
	b3 := saasEventDoc("EV-B3", 5*time.Hour, "CUST_B", "Birch Legal", bob, "Microsoft", "critical", "login.failure.brute", saasBrute, "203.0.113.50", "US")
	c1 := saasEventDoc("EV-C1", 6*time.Hour, "CUST_C", "Cedar Clinic", carl, "Microsoft", "medium", "outside.own.location", saasOutside, "203.0.113.50", "US")
	c1["pullTime"] = saasAgo(6*time.Hour - 10*time.Minute) // lag 600s
	c2 := saasEventDoc("EV-C2", 7*time.Hour, "CUST_C", "Cedar Clinic", carl, "Microsoft", "low", "login.success", saasLogin, "192.0.2.99", "CA")
	c3 := saasEventDoc("EV-C3", 2*24*time.Hour, "CUST_C", "Cedar Clinic", carl, "Microsoft", "critical", "login.failure.brute", saasBrute, "192.0.2.99", "CA")
	for _, ev := range []saasM{a1, a2, a3, a4, a5, b2, b3, c1, c2, c3} {
		if err := db.UpsertEvents(saasMustJSON(t, ev)); err != nil {
			t.Fatalf("seed event %v: %v", ev["eventId"], err)
		}
	}
	// The same Dropbox-style event twice with an empty eventId (stored under
	// two storage ids, as an older binary or a second write path would). It
	// must count once.
	dup := saasEventDoc("", 2*time.Hour+45*time.Minute, "CUST_B", "Birch Legal", bob, "Dropbox", "low", "file.share.external", saasShare, "198.51.100.7", "US")
	dup["location"].(saasM)["ipInfo"] = saasM{"asn": saasM{"asn": "AS64500", "name": "Example Transit"}, "threat": saasM{"is_threat": true, "is_known_attacker": true}}
	dupAlt := saasM{}
	for k, v := range dup {
		dupAlt[k] = v
	}
	if err := db.Upsert("events", "dup-copy-1", saasMustJSON(t, dup)); err != nil {
		t.Fatalf("seed dup: %v", err)
	}
	if err := db.Upsert("events", "dup-copy-2", saasMustJSON(t, dupAlt)); err != nil {
		t.Fatalf("seed dup alt: %v", err)
	}

	// users: dependent rows carry parent_id/customers_id as sync injects them.
	users := []saasM{
		{"id": "U-JANE-MS", "name": "Jane Doe", "email": jane, "isBillable": true, "accountEnabled": true, "isLicensed": true,
			"product": saasM{"id": "prod-ms", "name": "Microsoft"}, "time": "2026-10-01 10:00:00", "customerConnectionId": "cc-1",
			"assignedLicenses": []any{saasM{"skuId": "SKU-E3", "disabledPlans": []any{}}},
			"lastEvent":        saasM{"time": saasAgo(30 * time.Minute), "ip": "198.51.100.7", "jointDesc": saasBrute, "location": saasM{"country": "US"}},
			"parent_id":        "CUST_A", "customers_id": "CUST_A"},
		{"id": "U-JANE-DBX", "name": "Jane Doe", "email": "Jane.Doe@Example.com", "isBillable": false, "accountEnabled": true,
			"product": saasM{"id": "prod-dbx", "name": "Dropbox"}, "time": "2026-09-01 10:00:00", "customerConnectionId": "cc-2",
			"lastEvent": saasM{"time": saasAgo(20 * 24 * time.Hour), "ip": "192.0.2.10", "location": saasM{"country": "GB"}},
			"parent_id": "CUST_A", "customers_id": "CUST_A"},
		{"id": "U-BOB", "name": "Bob Smith", "email": bob, "isBillable": true, "accountEnabled": false,
			"product": saasM{"id": "prod-ms", "name": "Microsoft"}, "time": "2026-10-01 10:00:00", "customerConnectionId": "cc-3",
			"parent_id": "CUST_B", "customers_id": "CUST_B"},
	}
	for _, u := range users {
		if err := db.UpsertUsers(saasMustJSON(t, u)); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}

	triggers := []saasM{
		{"id": "RULE_TRIGGER_ID_1", "status": "PENDING", "triggeredAt": saasAgoMillis(20 * time.Minute), "partnerId": "PARTNER_ID_12345",
			"accounts":     []any{saasM{"id": "ACCOUNT_ID_1", "email": jane, "fullName": "Jane Doe", "product": saasM{"type": "MS"}}},
			"organization": saasM{"id": "CUST_A", "name": "Acme Dental"},
			"rule":         saasM{"id": "RULE_ID_12345", "name": "Brute force lockout", "status": "ACTIVE"}},
		{"id": "RULE_TRIGGER_ID_2", "status": "APPROVED", "triggeredAt": saasAgoMillis(3 * time.Hour), "partnerId": "PARTNER_ID_12345",
			"accounts":     []any{saasM{"id": "ACCOUNT_ID_2", "email": bob, "fullName": "Bob Smith"}},
			"organization": saasM{"id": "ORG_B", "name": "Birch Legal"},
			"rule":         saasM{"id": "RULE_ID_2345", "name": "Impossible travel", "status": "ACTIVE"}},
	}
	for _, rt := range triggers {
		if err := db.UpsertRuleTriggers(saasMustJSON(t, rt)); err != nil {
			t.Fatalf("seed rule trigger: %v", err)
		}
	}

	supp := saasM{"id": "SUPP_ID_1", "partnerId": "PARTNER_ID_12345", "scope": "users", "users": []any{jane},
		"customerId": "CUST_A", "customerName": "Acme Dental", "status": "Active", "reason": "Travelling for conference",
		"filters":   saasM{"jointType": "outside.own.location"},
		"rules":     saasM{"showInAnalysis": false, "createPSATicket": false, "processOAL": false, "processInRespond": false, "processInIOC": false},
		"startDate": "2026-10-10T00:00:00.000Z", "endDate": "2026-10-20T00:00:00.000Z",
		"lastUpdatedBy": "tech@msp.example", "createdAt": "2026-10-09T00:00:00.000Z"}
	if err := db.Upsert("suppressions", "SUPP_ID_1", saasMustJSON(t, supp)); err != nil {
		t.Fatalf("seed suppression: %v", err)
	}

	device := saasM{"_id": "DEVICE_ID_1", "deviceName": "ACME-LT-042", "deviceType": "Laptop", "deviceOS": "Windows 11 Pro",
		"customer": saasM{"id": "CUST_A", "name": "Acme Dental"}, "product": saasM{"id": "0", "name": "NinjaOne"},
		"customerConnectionId": "0", "lastActiveTime": saasAgo(time.Hour), "IPv4History": []any{saasM{"ip": "198.51.100.7"}},
		"mappedAccounts": []any{saasM{"email": jane, "mappedBy": "tech@msp.example", "confidence": 0.9}}}
	for _, rt := range []string{"devices", "devices-mapped"} {
		if err := db.Upsert(rt, "DEVICE_ID_1", saasMustJSON(t, device)); err != nil {
			t.Fatalf("seed %s: %v", rt, err)
		}
	}

	orgs := []saasM{
		{"id": "CUST_A", "name": "Acme Dental", "partnerId": "PARTNER_ID_12345", "ruleIds": []any{},
			"applicationConnections": []any{saasM{"applicationType": "MS", "id": "APP_CONNECTION_A", "partnerId": "PARTNER_ID_12345", "status": "Active",
				"organization": saasM{"id": "CUST_A", "name": "Acme Dental"}}}},
		{"id": "ORG_B", "name": "birch legal", "partnerId": "PARTNER_ID_12345", "ruleIds": []any{},
			"applicationConnections": []any{saasM{"applicationType": "MS", "id": "APP_CONNECTION_B", "partnerId": "PARTNER_ID_12345", "status": "Active",
				"organization": saasM{"id": "ORG_B", "name": "birch legal"}, "lastError": saasM{"message": "Request failed with status code 400"}}}},
	}
	for _, o := range orgs {
		if err := db.UpsertOrganizations(saasMustJSON(t, o)); err != nil {
			t.Fatalf("seed org: %v", err)
		}
	}
	disc := saasM{"id": "APP_CONNECTION_B", "applicationType": "MS", "partnerId": "PARTNER_ID_12345", "status": "Active",
		"organization": saasM{"id": "ORG_B", "name": "birch legal"}, "lastError": saasM{"message": "Request failed with status code 400"}}
	if err := db.Upsert("organizations-disconnected", "APP_CONNECTION_B", saasMustJSON(t, disc)); err != nil {
		t.Fatalf("seed disconnected org: %v", err)
	}

	rules := []saasM{
		{"id": "RULE_ID_12345", "name": "Brute force lockout", "status": "ACTIVE", "alertSeverity": "critical",
			"organizations": saasM{"type": "SPECIFIC", "organizations": []any{saasM{"id": "CUST_A", "name": "Acme Dental"}}}},
		{"id": "RULE_ID_2345", "name": "Impossible travel", "status": "ACTIVE", "alertSeverity": "medium",
			"organizations": saasM{"type": "ALL", "organizations": []any{}, "excludedOrganizations": []any{saasM{"id": "ORG_B", "name": "birch legal"}}}},
	}
	for _, r := range rules {
		if err := db.UpsertRules(saasMustJSON(t, r)); err != nil {
			t.Fatalf("seed rule: %v", err)
		}
	}
	if markSynced {
		for _, r := range saasSyncedFixtureResources {
			if err := db.SaveSyncState(r, "", 1); err != nil {
				t.Fatalf("seed sync_state %s: %v", r, err)
			}
		}
	}
	return dbPath
}

// ---- shared helper tests ----

func TestSaasParseTimeFormats(t *testing.T) {
	want := time.Date(2026, 10, 14, 9, 15, 0, 0, time.UTC)
	for _, in := range []any{
		"2026-10-14T09:15:00Z",
		"2026-10-14T09:15:00.000+00:00",
		"2026-10-14T11:15:00+02:00",
		"2026-10-14 09:15:00",
		"2026-10-14T09:15:00",
		float64(want.UnixMilli()),
		float64(want.Unix()),
		strconv.FormatInt(want.UnixMilli(), 10),
		map[string]any{"seconds": float64(want.Unix()), "nanoseconds": float64(0)},
		map[string]any{"_seconds": float64(want.Unix()), "_nanoseconds": float64(0)},
	} {
		got, ok := saasParseTime(in)
		if !ok || !got.Equal(want) {
			t.Errorf("saasParseTime(%#v) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, bad := range []any{nil, "", "not a time", map[string]any{"x": 1.0}, true} {
		if _, ok := saasParseTime(bad); ok {
			t.Errorf("saasParseTime(%#v) unexpectedly parsed", bad)
		}
	}
}

func TestSaasEventDedupeKey(t *testing.T) {
	// No usable id: identity is a hash of the whole document, so only an
	// exact copy collapses (storage keeps a re-spelled time apart too).
	base := saasM{"time": "2026-10-14T09:15:00Z", "eventId": "", "user": saasM{"name": "bob@example.com"}, "customer": saasM{"id": "CUST_B"}, "jointType": "file.share.external"}
	copyOf := saasM{"time": "2026-10-14T09:15:00Z", "eventId": "", "user": saasM{"name": "bob@example.com"}, "customer": saasM{"id": "CUST_B"}, "jointType": "file.share.external"}
	respelled := saasM{"time": "2026-10-14T09:15:00.000+00:00", "eventId": "", "user": saasM{"name": "bob@example.com"}, "customer": saasM{"id": "CUST_B"}, "jointType": "file.share.external"}
	e1, ok1 := saasDecodeEvent(base)
	e2, ok2 := saasDecodeEvent(copyOf)
	if !ok1 || !ok2 {
		t.Fatal("decode failed")
	}
	if e1.Key != e2.Key || !strings.HasPrefix(e1.Key, "h|") {
		t.Fatalf("exact copies must share a content key: %q vs %q", e1.Key, e2.Key)
	}
	if e3, _ := saasDecodeEvent(respelled); e3.Key == e1.Key {
		t.Fatal("a different document must not share a content key")
	}
	other := saasM{"time": "2026-10-14T09:15:00Z", "eventId": "", "user": saasM{"name": "bob@example.com"}, "customer": saasM{"id": "CUST_C"}, "jointType": "file.share.external"}
	if e4, _ := saasDecodeEvent(other); e4.Key == e1.Key {
		t.Fatal("different customer must not share a dedupe key")
	}
	if _, ok := saasDecodeEvent(saasM{"eventId": "EV-10"}); ok {
		t.Fatal("event without time must be skipped")
	}
}

func TestSaasCustomerWhitelists(t *testing.T) {
	c := saasDecodeCustomer(saasM{
		"id": "C1", "name": "One",
		"ipRanges":  []any{saasM{"from": "203.0.113.0", "to": "203.0.113.255"}, saasM{"from": "192.0.2.10"}, saasM{"from": "2001:db8::/32"}},
		"asnList":   []any{saasM{"name": "Google LLC", "country": "US"}},
		"countries": []any{saasM{"code": "US", "label": "United States", "phone": "1"}, "ca"},
	})
	if !c.HasWhitelistFields {
		t.Fatal("whitelist fields not detected")
	}
	for ip, want := range map[string]bool{
		"203.0.113.50": true, "203.0.113.255": true, "203.0.114.1": false, "192.0.2.10": true, "192.0.2.11": false,
		"2001:db8::1": true, "2001:db9::1": false, "::ffff:203.0.113.7": true, "garbage": false,
	} {
		if got := c.whitelistsIP(ip); got != want {
			t.Errorf("whitelistsIP(%s) = %v, want %v", ip, got, want)
		}
	}
	if !c.whitelistsASN("AS15169", "google llc") || c.whitelistsASN("AS1", "Other") {
		t.Error("ASN name whitelist mismatch")
	}
	if !c.whitelistsCountry("us") || !c.whitelistsCountry("United States") || !c.whitelistsCountry("CA") || c.whitelistsCountry("DE") {
		t.Error("country whitelist mismatch")
	}
	bare := saasDecodeCustomer(saasM{"id": "C2", "name": "Two"})
	if bare.HasWhitelistFields || bare.whitelistsIP("203.0.113.50") {
		t.Error("customer without whitelist fields must exclude nothing")
	}
}

func TestSaasLoadLocalEventsWindowAndDedupe(t *testing.T) {
	testenv.Isolate(t)
	dbPath := saasSeedFixtureStore(t)
	db, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	all, err := saasLoadLocalEvents(context.Background(), db, saasEventQuery{})
	if err != nil {
		t.Fatal(err)
	}
	// 10 eventId rows + 2 copies of one empty-eventId event = 11 distinct.
	if len(all) != 11 {
		t.Fatalf("unbounded load = %d events, want 11", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Time.Before(all[i-1].Time) {
			t.Fatal("events not oldest-first")
		}
	}
	day, err := saasLoadLocalEvents(context.Background(), db, saasEventQuery{Start: saasFixtureNow.Add(-24 * time.Hour), End: saasFixtureNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(day) != 8 {
		t.Fatalf("24h window = %d events, want 8", len(day))
	}
	jane, err := saasLoadLocalEvents(context.Background(), db, saasEventQuery{Start: saasFixtureNow.Add(-7 * 24 * time.Hour), End: saasFixtureNow, UserEmail: "JANE.DOE@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jane) != 4 {
		t.Fatalf("jane 7d = %d events, want 4 (EV-A5 is 34d old)", len(jane))
	}
}

func TestSaasLoadLocalEventsMissingTable(t *testing.T) {
	testenv.Isolate(t)
	// A file that exists but was never migrated: reads see no tables.
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DROP TABLE resources`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	ro, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	events, err := saasLoadLocalEvents(context.Background(), ro, saasEventQuery{})
	if err != nil || len(events) != 0 {
		t.Fatalf("missing table: events=%d err=%v, want 0/nil", len(events), err)
	}
}

func TestSaasFormatAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		40 * time.Second:             "40s",
		7 * time.Minute:              "7m",
		time.Hour:                    "1h",
		5*time.Hour + 12*time.Minute: "5h12m",
		72 * time.Hour:               "3d",
		3*24*time.Hour + 4*time.Hour: "3d4h",
		-time.Minute:                 "0s",
	} {
		if got := saasFormatAge(d); got != want {
			t.Errorf("saasFormatAge(%v) = %q, want %q", d, got, want)
		}
	}
}

// saasIsolate sandboxes a command test: throwaway home, pinned clock, and
// the root auto-refresh hook disabled so no test reaches the network.
func saasIsolate(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("SAAS_ALERTS_NO_AUTO_REFRESH", "1")
	saasPinNow(t)
}

// saasRunJSON runs the CLI, requires exit 0, and decodes stdout into v.
func saasRunJSON(t *testing.T, v any, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := saasRunCLI(t, args...)
	if err != nil {
		t.Fatalf("%v: %v (stderr=%q)", args, err, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), v); err != nil {
		t.Fatalf("%v: decode stdout: %v\n%s", args, err, stdout)
	}
	return stdout, stderr
}

// saasRequireKeys asserts a JSON object carries exactly the expected keys.
func saasRequireKeys(t *testing.T, raw string, want ...string) {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("decode object: %v\n%s", err, raw)
	}
	if len(obj) != len(want) {
		t.Fatalf("keys = %v, want %v", saasObjKeys(obj), want)
	}
	for _, k := range want {
		if _, ok := obj[k]; !ok {
			t.Fatalf("missing key %q in %v", k, saasObjKeys(obj))
		}
	}
}

func saasObjKeys(obj map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

func saasMissingDBPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "nope", "data.db")
}

func TestSaasHumanRenderers(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	render := func(fn func(*bytes.Buffer) error) string {
		var out bytes.Buffer
		if err := fn(&out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	var ct saasCrossTenantResult
	saasRunJSON(t, &ct, "cross-tenant", "--db", db, "--by", "asn", "--json")
	text := render(func(out *bytes.Buffer) error {
		cmd := newNovelCrossTenantCmd(&rootFlags{})
		cmd.SetOut(out)
		return saasRenderCrossTenantHuman(cmd, ct)
	})
	for _, want := range []string{"AS64500 (Example Transit)", "Acme Dental, Birch Legal"} {
		if !strings.Contains(text, want) {
			t.Errorf("cross-tenant human output missing %q:\n%s", want, text)
		}
	}

	var inv saasInvestigateResult
	saasRunJSON(t, &inv, "investigate", "jane.doe@example.com", "--db", db, "--json")
	text = render(func(out *bytes.Buffer) error {
		cmd := newNovelInvestigateCmd(&rootFlags{})
		cmd.SetOut(out)
		return saasRenderInvestigateHuman(cmd, inv)
	})
	for _, want := range []string{"DE -> US in 1h", "Brute force lockout", "ACME-LT-042", "(users match)"} {
		if !strings.Contains(text, want) {
			t.Errorf("investigate human output missing %q:\n%s", want, text)
		}
	}

	var cov saasCoverageResult
	saasRunJSON(t, &cov, "coverage", "--db", db, "--silent", "4h", "--json")
	text = render(func(out *bytes.Buffer) error {
		cmd := newNovelCoverageCmd(&rootFlags{})
		cmd.SetOut(out)
		return saasRenderCoverageHuman(cmd, cov)
	})
	for _, want := range []string{"4 customers: 2 silent (1 never seen)", "SILENT,NEVER_SEEN,RESPOND_NOT_CONNECTED", "(name match)"} {
		if !strings.Contains(text, want) {
			t.Errorf("coverage human output missing %q:\n%s", want, text)
		}
	}
}

// eventId is the external product's raw id (the spec example is
// "logins_2021-03-24T07:36:46Z"), so it only identifies an event within one
// customer, product and user; _id (the ES document id) wins when present.
func TestSaasDedupeScopesEventIDAndPrefersDocID(t *testing.T) {
	raw := "logins_2021-03-24T07:36:46Z"
	doc := func(docID, cust, prodID, prodName, user, at string) saasM {
		d := saasM{"time": at, "eventId": raw, "user": saasM{"name": user},
			"customer": saasM{"id": cust, "name": cust}, "product": saasM{"id": prodID, "name": prodName}, "jointType": "login.success"}
		if docID != "" {
			d["_id"] = docID
		}
		return d
	}
	key := func(d saasM) string {
		ev, ok := saasDecodeEvent(d)
		if !ok {
			t.Fatalf("decode %v", d)
		}
		return ev.Key
	}
	at := "2026-10-14T09:00:00Z"
	base := key(doc("", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", at))
	for name, other := range map[string]saasM{
		"other customer":            doc("", "CUST_B", "prod-ms", "Microsoft", "jane.doe@example.com", at),
		"other product (by id)":     doc("", "CUST_A", "prod-gws", "Google Workspace", "jane.doe@example.com", at),
		"other product (name only)": doc("", "CUST_A", "", "Dropbox", "jane.doe@example.com", at),
		"other user":                doc("", "CUST_A", "prod-ms", "Microsoft", "bob.smith@example.com", at),
	} {
		if key(other) == base {
			t.Errorf("%s: equal eventId must not collapse distinct events (key %q)", name, base)
		}
	}
	if key(doc("", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", "2026-10-14T09:00:05Z")) != base {
		t.Error("same eventId + customer + product + user is one event")
	}
	// An email-shaped user name is an account id compared lowercased, as
	// storage does, so a differently cased address is the same account.
	if key(doc("", "CUST_A", "prod-ms", "Microsoft", "JANE.DOE@example.com", at)) != base {
		t.Error("email account ids are case-insensitive in storage identity")
	}
	// Same ES document stored twice, even with a drifted eventId, collapses;
	// different _id values with the same eventId stay distinct.
	withDoc := doc("es-doc-1", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", at)
	drift := doc("es-doc-1", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", at)
	drift["eventId"] = ""
	if key(withDoc) != key(drift) {
		t.Error("same _id must collapse")
	}
	if key(withDoc) == key(doc("es-doc-2", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", at)) {
		t.Error("different _id must stay distinct")
	}

	// End to end through the store: rows stored under distinct storage ids.
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]saasM{
		"r1": doc("", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", at),
		"r2": doc("", "CUST_B", "prod-ms", "Microsoft", "bob.smith@example.com", at),                              // same eventId, other customer
		"r3": doc("", "CUST_A", "prod-gws", "Google Workspace", "jane.doe@example.com", at),                       // same eventId, other product
		"r4": doc("", "CUST_A", "prod-ms", "Microsoft", "jane.doe@example.com", at),                               // true duplicate of r1
		"r5": doc("es-doc-9", "CUST_C", "prod-ms", "Microsoft", "carl.jones@example.com", at),                     // ES doc ...
		"r6": doc("es-doc-9", "CUST_C", "prod-ms", "Microsoft", "carl.jones@example.com", "2026-10-14T09:00:01Z"), // ... stored twice
	}
	for id, d := range rows {
		if err := db.Upsert("events", id, saasMustJSON(t, d)); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	ro, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	events, err := saasLoadLocalEvents(context.Background(), ro, saasEventQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("loaded %d events, want 4 (r1=r4 and r5=r6 collapse; r2, r3 kept)", len(events))
	}
}

// A store that sync never recorded must not read as all-clear: every reader
// reports synced=false plus a note naming the sync to run.
func TestSaasUnsyncedStoreIsNotAllClear(t *testing.T) {
	saasIsolate(t)
	emptyPath := filepath.Join(t.TempDir(), "data.db")
	empty, err := store.OpenWithContext(context.Background(), emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	empty.Close()
	rowsNoState := saasSeedFixture(t, false)

	const (
		triageNote   = "local store has no synced events; run: saas-alerts-cli sync --resources customers,events --since 7d"
		investNote   = "local store has no synced events, users, rule-triggers, suppressions or devices; run: saas-alerts-cli sync --resources customers,events,rule-triggers,suppressions,devices --since 7d"
		coverageNote = "local store has no synced customers, events, organizations or rules; run: saas-alerts-cli sync --resources customers,events,organizations,organizations-disconnected,rules --since 7d"
	)
	type syncedEnvelope struct {
		Synced bool   `json:"synced"`
		Note   string `json:"note"`
	}
	for _, dbPath := range []string{emptyPath, rowsNoState} {
		for _, tc := range []struct {
			args []string
			note string
		}{
			{[]string{"triage"}, triageNote},
			{[]string{"cross-tenant"}, triageNote},
			{[]string{"investigate", "jane.doe@example.com"}, investNote},
			{[]string{"coverage"}, coverageNote},
		} {
			var got syncedEnvelope
			_, stderr := saasRunJSON(t, &got, append(tc.args, "--db", dbPath, "--json")...)
			if got.Synced || got.Note != tc.note {
				t.Errorf("%v on %s: synced=%v note=%q, want false/%q", tc.args, filepath.Base(filepath.Dir(dbPath)), got.Synced, got.Note, tc.note)
			}
			if !strings.Contains(stderr, "hint: local store has not been synced yet") {
				t.Errorf("%v: stderr = %q", tc.args, stderr)
			}
		}
	}

	// Events synced but customers not: coverage still refuses to look clean.
	partial := saasSeedFixture(t, false)
	rw, err := store.OpenWithContext(context.Background(), partial)
	if err != nil {
		t.Fatal(err)
	}
	if err := rw.SaveSyncState("events", "", 1); err != nil {
		t.Fatal(err)
	}
	rw.Close()
	var cov syncedEnvelope
	saasRunJSON(t, &cov, "coverage", "--db", partial, "--json")
	if cov.Synced || !strings.HasPrefix(cov.Note, "local store has no synced customers, organizations or rules;") {
		t.Errorf("coverage with unsynced customers: synced=%v note=%q", cov.Synced, cov.Note)
	}
	var tri syncedEnvelope
	saasRunJSON(t, &tri, "triage", "--db", partial, "--json")
	if !tri.Synced || tri.Note != "" {
		t.Errorf("triage with synced events: synced=%v note=%q", tri.Synced, tri.Note)
	}
	// Events synced but the joined resources not: investigate's empty user,
	// trigger, suppression and device sections must not read as all-clear.
	var inv struct {
		Synced            bool     `json:"synced"`
		Note              string   `json:"note"`
		UnsyncedResources []string `json:"unsynced_resources"`
	}
	saasRunJSON(t, &inv, "investigate", "jane.doe@example.com", "--db", partial, "--json")
	if inv.Synced || strings.Join(inv.UnsyncedResources, ",") != "users,rule-triggers,suppressions,devices" ||
		!strings.HasPrefix(inv.Note, "local store has no synced users, rule-triggers, suppressions or devices;") {
		t.Errorf("investigate with only events synced: synced=%v unsynced=%v note=%q", inv.Synced, inv.UnsyncedResources, inv.Note)
	}
	var full struct {
		Synced            bool      `json:"synced"`
		UnsyncedResources *[]string `json:"unsynced_resources"`
	}
	saasRunJSON(t, &full, "investigate", "jane.doe@example.com", "--db", saasSeedFixtureStore(t), "--json")
	if !full.Synced || full.UnsyncedResources == nil || len(*full.UnsyncedResources) != 0 {
		t.Errorf("investigate on a fully synced store: synced=%v unsynced=%v, want true and []", full.Synced, full.UnsyncedResources)
	}
}

// The missing-mirror hint is one stderr line starting "hint:" so the MCP
// shell-out filter (which keeps only hint:/warning: lines) passes it through,
// and the empty envelope also says synced=false.
func TestSaasMissingMirrorHintSurvivesMCPFilter(t *testing.T) {
	saasIsolate(t)
	for _, args := range [][]string{{"triage"}, {"cross-tenant"}, {"investigate", "jane.doe@example.com"}, {"coverage"}} {
		missing := saasMissingDBPath(t)
		var got struct {
			Synced *bool  `json:"synced"`
			Note   string `json:"note"`
		}
		_, stderr := saasRunJSON(t, &got, append(args, "--db", missing, "--json")...)
		lines := strings.Split(strings.TrimSpace(stderr), "\n")
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "hint: no local mirror at "+missing+"; run: saas-alerts-cli sync --resources ") {
			t.Errorf("%v stderr = %q, want one hint: line", args, stderr)
		}
		if got.Synced == nil || *got.Synced || !strings.HasPrefix(got.Note, "local store has no synced ") {
			t.Errorf("%v envelope synced=%v note=%q", args, got.Synced, got.Note)
		}
	}
}

// Long text states the local-store dependency and the sync to run.
func TestSaasLongTextNamesLocalStoreAndSync(t *testing.T) {
	flags := &rootFlags{}
	for _, tc := range []struct {
		cmd  *cobra.Command
		sync string
	}{
		{newNovelTriageCmd(flags), "sync --resources customers,events'"},
		{newNovelCrossTenantCmd(flags), "sync --resources customers,events'"},
		{newNovelInvestigateCmd(flags), "sync --resources customers,events,rule-triggers,suppressions,devices'"},
		{newNovelCoverageCmd(flags), "sync --resources customers,events,organizations,organizations-disconnected,rules'"},
	} {
		if !strings.Contains(tc.cmd.Long, "Reads the local store only") || !strings.Contains(tc.cmd.Long, tc.sync) {
			t.Errorf("%s Long missing local-store/sync guidance:\n%s", tc.cmd.Name(), tc.cmd.Long)
		}
	}
}

// The reader must never collapse events that storage keeps apart: it uses
// the same identity order as internal/store/saas_events_id.go (_id; else
// eventId scoped by customer id + product + user id-else-name; else a hash of
// the whole event). Events go in through the same batch upsert sync uses;
// legacy rows (stored under other ids by an older binary) carry exact copies.
func TestSaasReaderIdentityMatchesStorage(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ev := func(s string) json.RawMessage { return json.RawMessage(s) }
	sameNameUser1 := `{"eventId":"logins_2026-10-14T07:36:46Z","customer":{"id":"C1","name":"Acme Dental"},"product":{"id":"prod-ms","name":"Microsoft"},"user":{"id":"u-1","name":"Charles Buck"},"time":"2026-10-14T07:36:46Z","alertStatus":"low","jointType":"login.success"}`
	sameNameUser2 := `{"eventId":"logins_2026-10-14T07:36:46Z","customer":{"id":"C1","name":"Acme Dental"},"product":{"id":"prod-ms","name":"Microsoft"},"user":{"id":"u-2","name":"Charles Buck"},"time":"2026-10-14T07:36:46Z","alertStatus":"low","jointType":"login.success"}`
	noIDMicrosoft := `{"eventId":"","customer":{"id":"C1","name":"Acme Dental"},"product":{"name":"Microsoft"},"user":{"name":"jane.doe@example.com"},"time":"2026-10-14T08:00:00Z","alertStatus":"medium","jointType":"file.share.external"}`
	noIDDropbox := `{"eventId":"","customer":{"id":"C1","name":"Acme Dental"},"product":{"name":"Dropbox"},"user":{"name":"jane.doe@example.com"},"time":"2026-10-14T08:00:00Z","alertStatus":"medium","jointType":"file.share.external"}`
	items := []json.RawMessage{
		ev(sameNameUser1), ev(sameNameUser2), ev(noIDMicrosoft), ev(noIDDropbox),
		ev(sameNameUser1), ev(noIDDropbox), // re-deliveries in the same batch
	}
	if _, failures, err := db.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	var stored int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'events'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 4 {
		t.Fatalf("storage kept %d events, want 4", stored)
	}
	// Exact copies under other storage ids (an older binary keyed on bare eventId).
	if err := db.Upsert("events", "logins_2026-10-14T07:36:46Z", ev(sameNameUser1)); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("events", "legacy-no-id", ev(noIDMicrosoft)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	ro, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	events, err := saasLoadLocalEvents(context.Background(), ro, saasEventQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		var keys []string
		for _, e := range events {
			keys = append(keys, e.UserID+"/"+e.ProductName)
		}
		t.Fatalf("reader returned %d events %v, want 4: same display name with different user.id and empty eventId with different products stay distinct; exact copies collapse", len(events), keys)
	}
}

// Round-5 identity: the eventId scope needs customer id, product AND a stable
// account id (user.id, UserId, userEmail, email, or an email-shaped user.name).
// A display name is not an account. Events go through UpsertBatch (storage
// identity), then exact copies are added under legacy storage ids; the reader
// must keep exactly what storage keeps and collapse only true re-deliveries.
func TestSaasReaderIdentityStableAccountScope(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	const head = `"eventId":"logins_2026-10-14T07:36:46Z","customer":{"id":"C1"},"product":{"id":"prod-ms","name":"Microsoft"},"time":"2026-10-14T07:36:46Z","jointType":"login.success"`
	docs := map[string]string{
		// Same display name, different top-level UserId: two accounts.
		"userId1": `{` + head + `,"user":{"name":"Charles Buck"},"UserId":"u-100"}`,
		"userId2": `{` + head + `,"user":{"name":"Charles Buck"},"UserId":"u-200"}`,
		// Same display name, different userEmail: two accounts.
		"email1": `{` + head + `,"user":{"name":"Charles Buck"},"userEmail":"charles@example.com"}`,
		"email2": `{` + head + `,"user":{"name":"Charles Buck"},"userEmail":"chuck@example.com"}`,
		// Display name only: no stable account, so content hash; different
		// content (ip) stays distinct.
		"name1": `{` + head + `,"user":{"name":"Charles Buck"},"ip":"192.0.2.1"}`,
		"name2": `{` + head + `,"user":{"name":"Charles Buck"},"ip":"192.0.2.2"}`,
		// Absent nested ids and no product: content hash.
		"bare1": `{"eventId":"logins_2026-10-14T07:36:46Z","user":{"name":"dana@example.com"},"time":"2026-10-14T07:36:47Z","jointType":"login.success"}`,
		"bare2": `{"eventId":"logins_2026-10-14T07:36:46Z","user":{"name":"dana@example.com"},"time":"2026-10-14T07:36:48Z","jointType":"login.success"}`,
	}
	var items []json.RawMessage
	for _, k := range []string{"userId1", "userId2", "email1", "email2", "name1", "name2", "bare1", "bare2"} {
		items = append(items, json.RawMessage(docs[k]))
	}
	// Re-deliveries of every kind in the same batch.
	items = append(items, json.RawMessage(docs["userId1"]), json.RawMessage(docs["email2"]), json.RawMessage(docs["name1"]), json.RawMessage(docs["bare2"]))
	if _, failures, err := db.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	var stored int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'events'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 8 {
		t.Fatalf("storage kept %d events, want 8", stored)
	}
	// Exact copies under other storage ids (older binary keyed differently).
	for i, k := range []string{"userId2", "email1", "name2", "bare1"} {
		if err := db.Upsert("events", fmt.Sprintf("legacy-%d", i), json.RawMessage(docs[k])); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	ro, err := store.OpenReadOnlyContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	events, err := saasLoadLocalEvents(context.Background(), ro, saasEventQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 8 {
		t.Fatalf("reader returned %d events, want the 8 storage keeps (copies collapse, distinct accounts and contents stay apart)", len(events))
	}
}
