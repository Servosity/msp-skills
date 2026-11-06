// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
	"dropsuite-pp-cli/internal/cliutil/testenv"
	"dropsuite-pp-cli/internal/config"
)

var testNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func TestGradeObject(t *testing.T) {
	stale, crit := 48*time.Hour, 7*24*time.Hour
	cases := []struct {
		name  string
		o     objectRec
		grade string
	}{
		{"fresh completed", objectRec{Status: "Completed", LastBackup: testNow.Add(-2 * time.Hour)}, "ok"},
		{"connection failed even when fresh", objectRec{Status: "Connection Failed", LastBackup: testNow.Add(-time.Hour)}, "crit"},
		{"stale beyond warn", objectRec{Status: "Completed", LastBackup: testNow.Add(-72 * time.Hour)}, "warn"},
		{"stale beyond crit", objectRec{Status: "Completed", LastBackup: testNow.Add(-8 * 24 * time.Hour)}, "crit"},
		{"running and fresh is ok", objectRec{Status: "Running", LastBackup: testNow.Add(-time.Hour)}, "ok"},
		{"preparing and stale warns", objectRec{Status: "Preparing Backup", LastBackup: testNow.Add(-60 * time.Hour)}, "warn"},
		{"never backed up, recent add", objectRec{Status: "Connecting", AddedOn: testNow.Add(-time.Hour)}, "warn"},
		{"never backed up, old add", objectRec{AddedOn: testNow.Add(-30 * 24 * time.Hour)}, "crit"},
		{"vendor errors on fresh backup", objectRec{Status: "Completed", LastBackup: testNow.Add(-time.Hour), ErrText: `{"host":"x"}`}, "warn"},
		{"deactivated is skipped", objectRec{Status: "Connection Failed", Deactivated: true}, "skip"},
	}
	for _, tc := range cases {
		got, reason := gradeObject(tc.o, testNow, stale, crit)
		if got != tc.grade {
			t.Errorf("%s: grade = %s (%s), want %s", tc.name, got, reason, tc.grade)
		}
	}
}

func TestRedactSecretFields(t *testing.T) {
	in := `[{"id":"1-2","authentication_token":"tok-a","reseller_token":"tok-r","user":{"authentication_token":"tok-b"},"list":[{"reseller_token":"tok-c"}],"email":"a<b@example.com","seats_used":12345678901234}]`
	out := string(redactSecretFields([]byte(in)))
	for _, secret := range []string{"tok-a", "tok-r", "tok-b", "tok-c"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret %q survived redaction: %s", secret, out)
		}
	}
	if strings.Count(out, redactedValue) != 4 {
		t.Fatalf("want 4 redactions, got %s", out)
	}
	if !strings.Contains(out, `"a<b@example.com"`) || !strings.Contains(out, "12345678901234") {
		t.Fatalf("non-secret content changed: %s", out)
	}
	// Bodies without secrets are returned byte-identical.
	plain := `{"email":"x@example.com"}`
	if got := string(redactSecretFields([]byte(plain))); got != plain {
		t.Fatalf("untouched body changed: %s", got)
	}
	// Empty and null token values stay as they are.
	empty := `{"authentication_token":"","reseller_token":null}`
	if got := string(redactSecretFields([]byte(empty))); strings.Contains(got, redactedValue) {
		t.Fatalf("empty values were redacted: %s", got)
	}
	// A body that does not parse still gets a textual scrub.
	broken := `{"authentication_token": "tok-z", "x": `
	if got := string(redactSecretFields([]byte(broken))); strings.Contains(got, "tok-z") {
		t.Fatalf("unparseable body leaked: %s", got)
	}
	// Duplicate keys and JSON-escaped key names cannot carry the secret through.
	dup := `{"authentication_token":"tok-dup","authentication_token":""}`
	if got := string(redactSecretFields([]byte(dup))); strings.Contains(got, "tok-dup") {
		t.Fatalf("duplicate-key body leaked: %s", got)
	}
	esc := `{"authentication` + "\\u005f" + `token":"tok-esc"}`
	if got := string(redactSecretFields([]byte(esc))); strings.Contains(got, "tok-esc") {
		t.Fatalf("escaped-key body leaked: %s", got)
	}
	escBroken := `{"reseller` + "\\u005f" + `token":"tok-esc2", `
	if got := string(redactSecretFields([]byte(escBroken))); strings.Contains(got, "tok-esc2") {
		t.Fatalf("escaped-key unparseable body leaked: %s", got)
	}
	// A BOM or XSSI prefix (stripped by the client before it parses) must not
	// push the body onto the textual path, and any \uXXXX-escaped key counts.
	bom := "\xEF\xBB\xBF" + `{"authentic` + "\\u0061" + `tion_token":"tok-bom"}`
	if got := string(redactSecretFields([]byte(bom))); strings.Contains(got, "tok-bom") {
		t.Fatalf("BOM-prefixed escaped-key body leaked: %s", got)
	}
	xssiBroken := ")]}'\n" + `{"authentic` + "\\u0061" + `tion_token":"tok-xssi",`
	if got := string(redactSecretFields([]byte(xssiBroken))); strings.Contains(got, "tok-xssi") {
		t.Fatalf("XSSI-prefixed unparseable body leaked: %s", got)
	}
	// The request's own credential echoed in an error is masked too.
	echo := `{"error":"bad token org-secret-123 for org"}`
	if got := string(redactSecretFields([]byte(echo), "org-secret-123", "short")); strings.Contains(got, "org-secret-123") {
		t.Fatalf("echoed request credential leaked: %s", got)
	}
}

func TestMatchOrgAndSelect(t *testing.T) {
	orgs := []dsOrg{
		{ID: "101-2", OrganizationID: "501", Email: "Owner@Alpha.example", ExternalID: "PSA-1"},
		{ID: "102-2", OrganizationID: "502", Email: "owner@beta.example"},
		{ID: "103-2", Email: "owner@gamma.example", CustomerDeactivated: true},
	}
	for _, ref := range []string{"101-2", "501", "PSA-1", "owner@alpha.example"} {
		o, err := matchOrg(orgs, ref)
		if err != nil || o.ID != "101-2" {
			t.Errorf("matchOrg(%q) = %v, %v; want 101-2", ref, o.ID, err)
		}
	}
	if _, err := matchOrg(orgs, "nope"); err == nil || ExitCode(err) != 3 {
		t.Errorf("unknown org: err=%v code=%d, want not-found exit 3", err, ExitCode(err))
	}
	dup := append(orgs, dsOrg{ID: "104-2", Email: "owner@beta.example"})
	if _, err := matchOrg(dup, "owner@beta.example"); err == nil || ExitCode(err) != 2 {
		t.Errorf("ambiguous org: err=%v, want usage exit 2", err)
	}
	// An exact id wins even when another organization's alias equals it.
	idClash := append(orgs, dsOrg{ID: "105-2", ExternalID: "102-2"})
	if o, err := matchOrg(idClash, "102-2"); err != nil || o.ID != "102-2" {
		t.Errorf("matchOrg(id) with a clashing alias = %v, %v; want 102-2", o.ID, err)
	}
	picked, skipped, err := selectOrgs(orgs, "", false)
	if err != nil || len(picked) != 2 || len(skipped) != 1 || skipped[0].OrgID != "103-2" {
		t.Errorf("selectOrgs default = %d picked, %v skipped, %v", len(picked), skipped, err)
	}
	picked, _, _ = selectOrgs(orgs, "", true)
	if len(picked) != 3 {
		t.Errorf("selectOrgs includeInactive = %d, want 3", len(picked))
	}
}

func TestDecodeList(t *testing.T) {
	cases := []struct {
		body string
		n    int
		next int
	}{
		{`[{"id":1},{"id":2}]`, 2, 0},
		{`{"pagination":{"current_page":1,"next_page":2},"data":[{"id":1}]}`, 1, 2},
		{`{"result_set":[],"pagination":{"total_entries":0},"api_version":2}`, 0, 0},
	}
	for _, tc := range cases {
		items, next, err := decodeList(json.RawMessage(tc.body))
		if err != nil || len(items) != tc.n || next != tc.next {
			t.Errorf("decodeList(%s) = %d items next %d err %v; want %d/%d", tc.body, len(items), next, err, tc.n, tc.next)
		}
	}
}

func TestCoveragePct(t *testing.T) {
	if coveragePct(0, 0) != nil {
		t.Fatal("0/0 must be nil, not 100%")
	}
	if v := coveragePct(2, 2); v == nil || *v != 50 {
		t.Fatalf("2/(2+2) = %v, want 50", v)
	}
	if v := coveragePct(1, 2); v == nil || *v != 33.3 {
		t.Fatalf("1/3 = %v, want 33.3", v)
	}
}

func TestInvoiceReconcile(t *testing.T) {
	csv := "\ufeffExternal_ID,Qty\nPSA-1,9\nPSA-2,5\nPSA-2,1\nPSA-X,4\n"
	keyCol, lines, err := parseInvoiceCSV(strings.NewReader(csv))
	if err != nil || keyCol != "external_id" || len(lines) != 4 {
		t.Fatalf("parse = %q %d %v", keyCol, len(lines), err)
	}
	agg, order := aggregateInvoice(lines, keyCol)
	if agg["PSA-2"].Quantity != 6 || len(agg["PSA-2"].Lines) != 2 || len(order) != 3 {
		t.Fatalf("aggregate = %+v %v", agg["PSA-2"], order)
	}
	if _, _, err := parseInvoiceCSV(strings.NewReader("name,amount\na,1\n")); err == nil {
		t.Fatal("header without key/quantity columns must fail")
	}
	if _, _, err := parseInvoiceCSV(strings.NewReader("id,quantity\na,lots\n")); err == nil {
		t.Fatal("non-numeric quantity must fail")
	}

	req := int64(12)
	paid := int64(2)
	inv := 9.0
	row := billingRow{SeatsUsed: 10, SeatsAvailable: 8, RequiredSeats: &req, PaidSharedMailboxes: &paid, Invoiced: &inv}
	reconcileRow(&row, "seats_used", true)
	if row.Billable == nil || *row.Billable != 12 || row.Delta == nil || *row.Delta != -3 {
		t.Fatalf("billable/delta = %v/%v", row.Billable, row.Delta)
	}
	want := map[string]bool{"seats_used_exceeds_available": true, "required_exceeds_available": true, "under_billed": true}
	for _, f := range row.Flags {
		delete(want, f)
	}
	if len(want) != 0 {
		t.Fatalf("missing flags %v in %v", want, row.Flags)
	}
	clean := billingRow{SeatsUsed: 5, SeatsAvailable: 10}
	reconcileRow(&clean, "seats_used", false)
	if len(clean.Flags) != 1 || clean.Flags[0] != "not_on_invoice" {
		t.Fatalf("clean row flags = %v, want only not_on_invoice", clean.Flags)
	}
	// A missing counter (failed detail read) is unknown, never zero.
	missing := billingRow{SeatsUsed: 5, SeatsAvailable: 10, Invoiced: &inv}
	reconcileRow(&missing, "active_seats", false)
	if missing.Billable != nil || missing.Delta != nil || len(missing.Flags) != 1 || missing.Flags[0] != "billable_unknown" {
		t.Fatalf("missing counter: billable=%v delta=%v flags=%v", missing.Billable, missing.Delta, missing.Flags)
	}
	if _, _, err := parseInvoiceCSV(strings.NewReader("id,quantity\nx\n")); err == nil || ExitCode(err) != 2 {
		t.Fatalf("short row must be a usage error, got %v", err)
	}
	if _, _, err := parseInvoiceCSV(strings.NewReader("id,quantity\n,4\n")); err == nil {
		t.Fatal("blank key must be rejected")
	}
}

func TestHygieneForOrg(t *testing.T) {
	ago := func(d time.Duration) string { return testNow.Add(-d).Format(time.RFC3339) }
	deact := []map[string]any{
		{"email": "old@example.com", "deactivated_since": ago(60 * 24 * time.Hour), "storage": json.Number("10")},
		{"email": "recent@example.com", "deactivated_since": ago(2 * 24 * time.Hour), "storage": json.Number("10")},
		{"email": "empty@example.com", "deactivated_since": ago(60 * 24 * time.Hour), "storage": json.Number("0")},
	}
	deleted := []map[string]any{{"email": "gone@example.com", "flg_deleted": true, "storage": json.Number("7")}}
	invites := []map[string]any{
		{"email": "exp@example.com", "expire": "Expired", "last_sent_at": testNow.Format("2006/01/02 15:04")},
		{"email": "pending@example.com", "expire": "", "last_sent_at": testNow.Add(-24 * time.Hour).Format("2006/01/02 15:04")},
	}
	rows, sum := hygieneForOrg(deact, deleted, invites, dsOrg{ID: "1-2"}, testNow, 30*24*time.Hour)
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Email] = true
	}
	for _, want := range []string{"old@example.com", "gone@example.com", "exp@example.com"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, not := range []string{"recent@example.com", "empty@example.com", "pending@example.com"} {
		if got[not] {
			t.Errorf("%s should not be reported", not)
		}
	}
	if sum.StorageHeld != 17 {
		t.Errorf("storage held = %d, want 17", sum.StorageHeld)
	}
}

func i64(v int64) *int64 { return &v }

func TestDriftSnapshots(t *testing.T) {
	t0, t1 := testNow.Add(-24*time.Hour), testNow
	before := []snapshotRow{
		{OrgID: "1", Kind: "mailbox", Object: "a", Grade: "ok", Status: "Completed", LastBackup: "2026-10-14T00:00:00Z", Items: i64(100), TakenAt: t0},
		{OrgID: "1", Kind: "mailbox", Object: "gone", Grade: "ok", TakenAt: t0},
		{OrgID: "1", Kind: "sharepoint", Object: "*", Grade: "unknown", TakenAt: t0},
	}
	after := []snapshotRow{
		{OrgID: "1", Kind: "mailbox", Object: "a", Grade: "warn", Status: "Connecting", LastBackup: "2026-10-14T00:00:00Z", Items: i64(90), TakenAt: t1},
		{OrgID: "1", Kind: "mailbox", Object: "new", Grade: "ok", TakenAt: t1},
	}
	changes, _ := diffSnapshots(before, after)
	want := map[string]bool{"grade_changed": false, "status_changed": false, "backup_not_advancing": false, "items_dropped": false, "added": false, "removed": false}
	for _, c := range changes {
		if c.Object == "*" {
			t.Fatalf("kind-level unknown rows must not diff: %+v", c)
		}
		want[c.Change] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("expected change %s in %+v", k, changes)
		}
	}
	if got, _ := diffSnapshots(after, after); len(got) != 0 {
		t.Errorf("identical snapshots must produce no drift, got %+v", got)
	}
	// A count the API stopped returning (null) is unknown, not a drop to 0.
	nullAfter := []snapshotRow{{OrgID: "1", Kind: "mailbox", Object: "a", Grade: "ok", Status: "Completed", LastBackup: "2026-10-15T00:00:00Z", TakenAt: t1}}
	for _, c := range func() []driftChange { c, _ := diffSnapshots(before[:1], nullAfter); return c }() {
		if c.Change == "items_dropped" {
			t.Fatalf("a null count must not read as dropped: %+v", c)
		}
	}
	if (&locateObject{Deleted: true}).protected() || !(&locateObject{}).protected() {
		t.Fatal("a deleted mailbox is not protected")
	}
	// A scoped run (only org 1 mailbox read) must not report org 2 or other
	// workloads as removed, nor newly read pairs as added.
	full := []snapshotRow{
		{OrgID: "1", Kind: "mailbox", Object: scopeMarker, Grade: "scope", TakenAt: t0},
		{OrgID: "1", Kind: "mailbox", Object: "a", Grade: "ok", TakenAt: t0},
		{OrgID: "2", Kind: "mailbox", Object: scopeMarker, Grade: "scope", TakenAt: t0},
		{OrgID: "2", Kind: "mailbox", Object: "b", Grade: "ok", TakenAt: t0},
	}
	scoped := []snapshotRow{
		{OrgID: "1", Kind: "mailbox", Object: scopeMarker, Grade: "scope", TakenAt: t1},
		{OrgID: "1", Kind: "mailbox", Object: "a", Grade: "ok", TakenAt: t1},
		{OrgID: "1", Kind: "onedrive", Object: scopeMarker, Grade: "scope", TakenAt: t1},
		{OrgID: "1", Kind: "onedrive", Object: "d", Grade: "ok", TakenAt: t1},
	}
	got, notCompared := diffSnapshots(full, scoped)
	if len(got) != 0 || notCompared != 2 {
		t.Errorf("scoped run: changes=%+v notCompared=%d, want none and 2", got, notCompared)
	}
	runs := []snapshotRun{{RunID: "a", TakenAt: testNow.Add(-10 * 24 * time.Hour)}, {RunID: "b", TakenAt: testNow.Add(-3 * 24 * time.Hour)}, {RunID: "c", TakenAt: testNow}}
	from, to, ok := pickDriftRuns(runs, 7*24*time.Hour)
	if !ok || from.RunID != "a" || to.RunID != "c" {
		t.Errorf("pickDriftRuns 7d = %s..%s", from.RunID, to.RunID)
	}
	from, _, _ = pickDriftRuns(runs, 24*time.Hour)
	if from.RunID != "b" {
		t.Errorf("pickDriftRuns 1d from = %s, want b", from.RunID)
	}
	if _, _, ok := pickDriftRuns(runs[:1], time.Hour); ok {
		t.Error("one run must not produce a drift pair")
	}
}

func TestFreshnessAndLedger(t *testing.T) {
	runs := []snapshotRun{{RunID: "r1", TakenAt: testNow.Add(-26 * time.Hour)}, {RunID: "r2", TakenAt: testNow.Add(-25 * time.Hour)}, {RunID: "r3", TakenAt: testNow}}
	rows := map[string][]snapshotRow{
		"r1": {{Object: "a", Grade: "crit"}},
		"r2": {{Object: "a", Grade: "ok"}, {Object: "b", Grade: "warn"}},
		"r3": {{Object: "a", Grade: "ok"}, {Object: "*", Grade: "unknown"}},
	}
	for _, k := range allHealthKinds {
		rows["r2"] = append(rows["r2"], snapshotRow{Kind: k, Object: scopeMarker, Grade: "scope"})
	}
	f := freshnessByDay(runs, rows)
	if len(f) != 2 || f[0].RunID != "r2" || f[0].Objects != 2 || f[0].OKPct == nil || *f[0].OKPct != 50 || !f[0].Complete {
		t.Fatalf("freshnessByDay day 1 = %+v", f)
	}
	if f[1].Complete || f[1].OKPct != nil || f[1].Unknown != 1 {
		t.Fatalf("a day with an unreadable workload must have no ok_pct: %+v", f[1])
	}
	// health --kinds mailbox: the other workloads were never checked.
	scoped := freshnessByDay(runs[2:], map[string][]snapshotRow{"r3": {{Kind: kindMailbox, Object: "a", Grade: "ok"}, {Kind: kindMailbox, Object: scopeMarker, Grade: "scope"}}})
	if len(scoped) != 1 || scoped[0].Complete || scoped[0].OKPct != nil || len(scoped[0].NotChecked) != len(allHealthKinds)-1 {
		t.Fatalf("a mailbox-only snapshot must not read as complete: %+v", scoped)
	}
	orgs := []knownOrg{{ID: "1-2", Email: "owner@example.com"}, {ID: "2-2", ExternalID: "PSA-9"}}
	tests := []restoreTest{
		{OrgRef: "OWNER@example.com", Result: "pass", TestedAt: testNow.Add(-10 * 24 * time.Hour).Format(time.RFC3339)},
		{OrgRef: "1-2", Result: "fail", TestedAt: testNow.Format(time.RFC3339)},
		{OrgRef: "2-2", Result: "partial", TestedAt: testNow.Format(time.RFC3339)},
	}
	if !orgs[1].matches("PSA-9") || orgs[1].matches("PSA") {
		t.Fatal("knownOrg must match its external_id exactly")
	}
	// An alias two organizations share credits neither; an id still resolves.
	shared := []knownOrg{{ID: "5-2", Email: "it@msp.example"}, {ID: "6-2", Email: "it@msp.example"}}
	if _, n := resolveKnownOrg(shared, "IT@msp.example"); n != 2 {
		t.Fatalf("shared email resolved to %d organizations, want 2", n)
	}
	if o, n := resolveKnownOrg(shared, "6-2"); n != 1 || o.ID != "6-2" {
		t.Fatal("an organization id must resolve uniquely")
	}
	if got := lastPassByOrg(shared, []restoreTest{{OrgRef: "it@msp.example", Result: "pass", TestedAt: testNow.Format(time.RFC3339)}}); len(got) != 0 {
		t.Fatalf("a pass under a shared alias must not count: %v", got)
	}
	// testsFor keeps only rows whose reference resolves to the organization
	// alone: not an alias another one shares (inactive included), not an
	// alias equal to another organization's id, and external ids compare
	// with case.
	mixed := []knownOrg{{ID: "7-2", Email: "a@x.example", ExternalID: "8-2"}, {ID: "8-2"}, {ID: "9-2", Email: "a@x.example", Inactive: true},
		{ID: "10-2", ExternalID: "PSA-X"}, {ID: "11-2", ExternalID: "psa-x"}}
	ledger := []restoreTest{{ID: 1, OrgRef: "7-2"}, {ID: 2, OrgRef: "a@x.example"}, {ID: 3, OrgRef: "8-2"}, {ID: 4, OrgRef: "psa-x"}, {ID: 5, OrgRef: "PSA-X"}}
	if got := testsFor(mixed, mixed[0], ledger); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("testsFor(7-2) = %+v, want only the id row", got)
	}
	if got := testsFor(mixed, mixed[3], ledger); len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("testsFor(PSA-X) = %+v, want only the exact-case external id row", got)
	}
	// Teams with one display name but different ids stay distinct; the label is not the identity.
	a, b := map[string]any{"id": 1}, map[string]any{"id": 2}
	if stableID(a, "d1", "x/Sales") == stableID(b, "d1", "x/Sales") || stableID(map[string]any{}, "d1", "x/Sales") != "x/Sales" {
		t.Fatal("stableID must key on the vendor id and fall back to the label")
	}
	if _, err := parseKinds(","); err == nil {
		t.Fatal("--kinds naming no workload must be a usage error")
	}
	last := lastPassByOrg(orgs, tests)
	if _, ok := last["1-2"]; !ok {
		t.Fatal("a pass recorded under the owner email must count for the organization")
	}
	if _, ok := last["2-2"]; ok {
		t.Fatal("a partial result is not a pass")
	}
}

func TestSmallHelpers(t *testing.T) {
	for path, want := range map[string]bool{"/accounts": true, "/users": false, "/users/1-2": false, "/users/1-2/tenants": true, "/users/sub_users": true, "/users/generate_support_sso_link": false, "/status": false, "/countries": false, "/sharepoints/domains/5/sites": true} {
		if got, _ := orgScopedPath(path); got != want {
			t.Errorf("orgScopedPath(%s) = %t, want %t", path, got, want)
		}
	}
	if _, id := orgScopedPath("/users/26561-14/tenants/670/accounts"); id != "26561-14" {
		t.Errorf("path org id = %q, want 26561-14", id)
	}
	if fmtDuration(90*24*time.Hour) != "90d" || fmtDuration(36*time.Hour) != "36h0m0s" {
		t.Errorf("fmtDuration = %s / %s", fmtDuration(90*24*time.Hour), fmtDuration(36*time.Hour))
	}
	if emailDomain("Jane@Example.COM") != "example.com" || emailDomain("nope") != "" || emailDomain("x@") != "" {
		t.Error("emailDomain")
	}
	if k, err := parseKinds("Mailbox, shared-drive"); err != nil || len(k) != 2 || k[1] != kindSharedDrive {
		t.Errorf("parseKinds = %v %v", k, err)
	}
	if _, err := parseKinds("fax"); err == nil {
		t.Error("unknown kind must fail")
	}
	m := map[string]any{"a": json.Number("12"), "b": "34", "c": true, "t": "2026/05/22 07:43"}
	if num(m, "a") != 12 || num(m, "b") != 34 || !boolv(m, "c") {
		t.Error("num/boolv")
	}
	if ts, ok := timeField(m, "t"); !ok || ts.Day() != 22 {
		t.Error("timeField invite format")
	}
}

// h2Server serves the handler over HTTP/2 TLS and returns a client pointed at it.
func h2Server(t *testing.T, h http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	cfg := &config.Config{BaseURL: srv.URL + "/api", AccessToken: "admin"}
	c := client.New(cfg, 10*time.Second, 0)
	c.NoCache = true
	c.HTTPClient.Transport = srv.Client().Transport // trusts the test certificate
	installRedaction(c)
	return c
}

func TestFetchListPagingAndRepeatGuard(t *testing.T) {
	testenv.Isolate(t)
	calls := 0
	c := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/paged":
			n := 25
			if page == "2" {
				n = 3
			}
			items := make([]map[string]any, n)
			for i := range items {
				items[i] = map[string]any{"id": page + "-" + fmt.Sprint(i)}
			}
			_ = json.NewEncoder(w).Encode(items)
		case "/api/ignores-page":
			items := make([]map[string]any, 25)
			for i := range items {
				items[i] = map[string]any{"id": i}
			}
			_ = json.NewEncoder(w).Encode(items)
		case "/api/env-skip":
			// page 1 is empty but points forward; page 2 holds the row.
			next, data := 2, []any{}
			if page == "2" {
				next, data = 0, []any{map[string]any{"id": "y"}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"next_page": next}, "data": data})
		case "/api/env-back":
			// page 1 points forward, page 2 points back to page 1.
			next := 2
			if page == "2" {
				next = 1
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"next_page": next}, "data": []any{map[string]any{"id": page}}})
		case "/api/env-repeat":
			// next_page stays 2 and page 2 repeats page 1.
			_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"next_page": 2}, "data": []any{map[string]any{"id": "x"}}})
		case "/api/env":
			next := 0
			if page == "1" {
				next = 2
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]any{"next_page": next}, "data": []any{map[string]any{"id": page}}})
		}
	})
	ctx := context.Background()
	items, err := fetchList(ctx, c, "/paged", nil, pagedByPage, 10)
	if err != nil || len(items) != 28 {
		t.Fatalf("paged: %d items, %v; want 28", len(items), err)
	}
	calls = 0
	items, err = fetchList(ctx, c, "/ignores-page", nil, pagedByPage, 10)
	var repeated *truncatedError
	if !errors.As(err, &repeated) || !repeated.Repeated || len(items) != 25 || calls != 2 {
		t.Fatalf("repeat guard: %d items after %d calls, %v; want 25 after 2 and a repeated-page error", len(items), calls, err)
	}
	items, err = fetchList(ctx, c, "/env", nil, pagedEnvelope, 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("envelope: %d items, %v; want 2", len(items), err)
	}
	items, err = fetchList(ctx, c, "/env-repeat", nil, pagedEnvelope, 10)
	if !errors.As(err, &repeated) || !repeated.Repeated || len(items) != 1 {
		t.Fatalf("envelope repeat: %d items, %v; want 1 and a repeated-page error", len(items), err)
	}
	items, err = fetchList(ctx, c, "/env-skip", nil, pagedEnvelope, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("envelope empty page with a forward pointer: %d items, %v; want 1", len(items), err)
	}
	items, err = fetchList(ctx, c, "/env-back", nil, pagedEnvelope, 10)
	if !errors.As(err, &repeated) || len(items) != 2 {
		t.Fatalf("envelope backward pointer: %d items, %v; want 2 and a cycle error", len(items), err)
	}
	// Running out of --max-pages with pages left is reported, not silent.
	items, err = fetchList(ctx, c, "/paged", nil, pagedByPage, 1)
	var trunc *truncatedError
	if !errors.As(err, &trunc) || len(items) != 25 {
		t.Fatalf("truncation: %d items, err %v; want 25 and a truncatedError", len(items), err)
	}
}

func TestRedactingTransportAndReveal(t *testing.T) {
	testenv.Isolate(t)
	c := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1-2","authentication_token":"real-org-token"}]`))
	})
	ctx := context.Background()
	data, err := c.Get(ctx, "/users", nil)
	if err != nil || strings.Contains(string(data), "real-org-token") {
		t.Fatalf("default read leaked or failed: %s %v", data, err)
	}
	orgs, err := fetchOrgs(ctx, c)
	if err != nil || len(orgs) != 1 || orgs[0].token != "real-org-token" {
		t.Fatalf("fetchOrgs must see the real token in memory: %+v %v", orgs, err)
	}
}

func TestRedactionScrubsErrorBodiesEvenWithReveal(t *testing.T) {
	testenv.Isolate(t)
	c := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"boom","authentication_token":"leaky-token"}`))
	})
	_, err := c.Get(withSecrets(context.Background()), "/users", nil)
	if err == nil || strings.Contains(err.Error(), "leaky-token") {
		t.Fatalf("error body leaked a token: %v", err)
	}
}

func TestEchoBackstop(t *testing.T) {
	testenv.Isolate(t)
	const cred = "org-secret-123"
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Debug", "token=org%2Dsecret-123")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad token org\u002dsecret-123`))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	c := client.New(&config.Config{BaseURL: srv.URL + "/api", AccessToken: cred}, 10*time.Second, 0)
	c.NoCache = true
	c.HTTPClient.Transport = srv.Client().Transport
	installRedaction(c)
	_, err := c.Get(context.Background(), "/accounts", nil)
	if err == nil || strings.Contains(err.Error(), "secret-123") {
		t.Fatalf("escaped credential echo must be withheld: %v", err)
	}
	if !echoesCredential("x%6Frg-secret-123", []string{cred}) || echoesCredential("nothing here", []string{cred}) {
		t.Fatal("echoesCredential must see URL-escaped echoes and only those")
	}
	if !echoesCredential("org%2Dsecret-123 (100% complete)", []string{cred}) {
		t.Fatal("a stray '%' must not switch off percent decoding")
	}
	if !mentionsSecretField(`{\"authentication_token\"\n:\"x\"}`) {
		t.Fatal("a JSON-escaped newline between a credential key and its separator must still match")
	}
	if _, _, err := decodeList(json.RawMessage(`{"data":null,"pagination":{"next_page":2}}`)); err == nil {
		t.Fatal("an envelope without its array must be an error, not a record")
	}
	if !echoesCredential("<title>org&#45;secret&#x2d;123</title>", []string{"org-secret-123"}) {
		t.Fatal("HTML entities must decode in the backstop")
	}
	if !echoesCredential(`org%5Cu002dsecret-123`, []string{cred}) {
		t.Fatal("layered URL-then-JSON encoding must decode")
	}
	// An unparseable body naming a credential field is withheld whole, even
	// when its value is unterminated.
	if got := string(redactSecretFields([]byte(`{"error":"x","authentication_token":"other-org-secret`))); strings.Contains(got, "other-org-secret") {
		t.Fatalf("unterminated secret field leaked: %s", got)
	}
	// A revealed (withSecrets) success that is not JSON, and an unsupported
	// encoding named after the credential, are scrubbed like any other body.
	srv2 := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/enc" {
			w.Header().Set("Content-Encoding", cred)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<title>" + cred + "</title>"))
	}))
	srv2.EnableHTTP2 = true
	srv2.StartTLS()
	t.Cleanup(srv2.Close)
	c2 := client.New(&config.Config{BaseURL: srv2.URL + "/api", AccessToken: cred}, 10*time.Second, 0)
	c2.NoCache = true
	c2.HTTPClient.Transport = srv2.Client().Transport
	installRedaction(c2)
	t.Setenv(cliutil.VerifyEnvVar, "1") // no retries: each attempt is scrubbed the same way
	for _, p := range []string{"/html", "/enc"} {
		data, err := c2.Get(withSecrets(context.Background()), p, nil)
		if strings.Contains(string(data), "secret-123") || (err != nil && strings.Contains(err.Error(), "secret-123")) {
			t.Fatalf("%s leaked: %s %v", p, data, err)
		}
		// A withheld success is an error, not an empty result.
		if err == nil {
			t.Fatalf("%s: a withheld response must fail, got %s", p, data)
		}
	}
	if got := string(redactSecretFields([]byte(`{"authentication_token":["x-org-token"],"reseller_token":{"v":"y-token"}}`))); strings.Contains(got, "x-org-token") || strings.Contains(got, "y-token") {
		t.Fatalf("non-string secret values leaked: %s", got)
	}
	// The default base URL is a template url.Parse rejects; routing still
	// needs its path to tell /api/users (admin) from organization paths.
	for base, want := range map[string]string{"https://{host}/api": "/api", "http://127.0.0.1:1/api/": "/api", "https://x.example": ""} {
		if got := basePathOf(base); got != want {
			t.Errorf("basePathOf(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestDefaultOrgRouting(t *testing.T) {
	testenv.Isolate(t)
	orgTokens = orgTokenDirectory{}
	t.Cleanup(func() { orgTokens = orgTokenDirectory{} })
	t.Setenv(defaultOrgEnv, "owner@alpha.example")
	seen := map[string]string{}
	usersReseller := ""
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("X-Access-Token")
		if r.URL.Path == "/api/users" {
			usersReseller = r.Header.Get("X-Reseller-Token")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/users":
			_, _ = w.Write([]byte(`[{"id":"101-2","email":"owner@alpha.example","authentication_token":"org1-token"},{"id":"102-2","email":"owner@beta.example","authentication_token":"org2-token"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	cfg := &config.Config{BaseURL: srv.URL + "/api", AccessToken: "admin"}
	c := client.New(cfg, 10*time.Second, 0)
	c.HTTPClient.Transport = srv.Client().Transport
	installRedaction(c)
	installOrgRouting(c)
	ctx := context.Background()
	for _, p := range []string{"/accounts", "/users/101-2", "/users/101-2/tenants", "/users/102-2/tenants", "/countries"} {
		if _, err := c.Get(ctx, p, nil); err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
	}
	// A client with other credentials gets its own token set.
	c2 := client.New(&config.Config{BaseURL: srv.URL + "/api", AccessToken: "admin", DropsuiteResellerToken: "other-reseller"}, 10*time.Second, 0)
	c2.HTTPClient.Transport = srv.Client().Transport
	installRedaction(c2)
	installOrgRouting(c2)
	if _, err := c2.Get(ctx, "/accounts", nil); err != nil {
		t.Fatalf("second client: %v", err)
	}
	// A reseller token overridden through config headers is a credential
	// pair of its own, and its directory loads with that header.
	c3 := client.New(&config.Config{BaseURL: srv.URL + "/api", AccessToken: "admin", Headers: map[string]string{"X-Reseller-Token": "hdr-reseller"}}, 10*time.Second, 0)
	c3.HTTPClient.Transport = srv.Client().Transport
	installRedaction(c3)
	installOrgRouting(c3)
	if _, err := c3.Get(ctx, "/accounts", nil); err != nil {
		t.Fatalf("third client: %v", err)
	}
	if len(orgTokens.sets) != 3 || usersReseller != "hdr-reseller" {
		t.Fatalf("token sets = %d (want one per credential pair), last /users reseller header %q", len(orgTokens.sets), usersReseller)
	}
	if !c.NoCache || !c2.NoCache {
		t.Fatal("routed clients must not use the response cache (its key omits the swapped and reseller tokens)")
	}
	// The path's organization wins over DROPSUITE_ORG; admin routes keep the reseller token.
	want := map[string]string{"/api/accounts": "org1-token", "/api/users/101-2": "admin", "/api/users/101-2/tenants": "org1-token", "/api/users/102-2/tenants": "org2-token", "/api/countries": "admin"}
	for p, tok := range want {
		if seen[p] != tok {
			t.Errorf("%s sent token %q, want %q", p, seen[p], tok)
		}
	}
}

func TestLegacySnapshotMigration(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := openFleetDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `CREATE TABLE ds_health_snapshots (run_id TEXT, taken_at TEXT, org_id TEXT, org_email TEXT, kind TEXT, object TEXT,
		status TEXT, last_backup TEXT, items INTEGER, storage INTEGER, grade TEXT, reason TEXT);
		INSERT INTO ds_health_snapshots VALUES ('r1', '2026-01-01T00:00:00Z', '1-2', 'o@x.example', 'teams', 'x/Sales', '', '', 0, 0, 'ok', '')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	ro, err := openFleetDBReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	rows, err := loadSnapshotRows(ctx, ro.DB(), "r1", "")
	if err != nil || len(rows) != 1 || rows[0].ObjectID != "x/Sales" || hasLegacySnapshots(ctx, ro.DB()) {
		t.Fatalf("legacy rows not migrated: %+v %v", rows, err)
	}
}

type headerProbe struct{ got http.Header }

func (h *headerProbe) RoundTrip(r *http.Request) (*http.Response, error) {
	h.got = r.Header.Clone()
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}, Request: r}, nil
}

func TestCredentialPinningAndHeaderOverride(t *testing.T) {
	probe := &headerProbe{}
	pin := pinCredentials{next: probe, origin: "http://127.0.0.1:1/api", access: "acc-token", reseller: "res-token"}
	for url, want := range map[string]bool{
		"http://127.0.0.1:1/api/users":  true,
		"http://127.0.0.1:10/api/users": false, // host prefix, different port
		"https://elsewhere.example/x":   false,
	} {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("X-Access-Token", "stale")
		if _, err := pin.RoundTrip(req); err != nil {
			t.Fatal(err)
		}
		if got := probe.got.Get("X-Access-Token") == "acc-token" && probe.got.Get("X-Reseller-Token") == "res-token"; got != want {
			t.Errorf("%s: pinned=%v, want %v (headers %v)", url, got, want, probe.got)
		}
		if !want && (probe.got.Get("X-Access-Token") != "" || probe.got.Get("X-Reseller-Token") != "") {
			t.Errorf("%s: credentials sent off origin", url)
		}
	}
	if got := effectiveAccessToken(&config.Config{AccessToken: "field", Headers: map[string]string{"x-ACCESS-token": "hdr"}}); got != "hdr" {
		t.Fatalf("effectiveAccessToken = %q, want the configured header", got)
	}
	// A same-origin redirect re-stamps the credential field; it is still the
	// reseller's own token for routing.
	rt := orgRoutingTransport{adminToken: "hdr", owner: client.New(&config.Config{AccessToken: "field"}, time.Second, 0)}
	if !rt.isAdminToken("hdr") || !rt.isAdminToken("field") || rt.isAdminToken("org-token") || rt.isAdminToken("") {
		t.Fatal("isAdminToken must accept the effective header and the credential field only")
	}
	// Serialized credentials inside string values, and camelCase keys, are
	// masked; prose about a token is kept.
	body := `{"error":"{\"authentication_token\":\"other-org-x\"}","authenticationToken":"camel-x","msg":"Invalid reseller token"}`
	got := string(redactSecretFields([]byte(body)))
	if strings.Contains(got, "other-org-x") || strings.Contains(got, "camel-x") || !strings.Contains(got, "Invalid reseller token") {
		t.Fatalf("redaction = %s", got)
	}
	cfg := &config.Config{Headers: map[string]string{"x-access-token": "admin", "X-Other": "keep"}}
	dropAccessTokenHeader(cfg)
	if _, ok := cfg.Headers["x-access-token"]; ok || cfg.Headers["X-Other"] != "keep" {
		t.Fatalf("dropAccessTokenHeader = %v", cfg.Headers)
	}
	// Two runs that both failed to read an organization report it as unread.
	failed := []snapshotRow{{OrgID: "1-2", Kind: "*", Object: "*", Grade: "unknown"}}
	if got := unreadScopes(failed, failed); len(got) != 1 || got[0] != "1-2/*" {
		t.Fatalf("unreadScopes = %v, want [1-2/*]", got)
	}
}

func TestLayeredDecodingAndOrgGuards(t *testing.T) {
	testenv.Isolate(t)
	// Double-serialized escape: one JSON layer at a time keeps backslash parity.
	if !mentionsSecretField(`authentication\\u005ftoken=plaintext-secret`) {
		t.Fatal("a doubly escaped underscore must decode")
	}
	// Nine URL layers decode; past the budget the text counts as a credential.
	nine := "authentication%" + strings.Repeat("25", 8) + "5Ftoken=plain"
	deep := "authentication%" + strings.Repeat("25", maxDecodeRounds+5) + "5Ftoken=plain"
	if !mentionsSecretField(nine) || !mentionsSecretField(deep) || mentionsSecretField("Invalid reseller token") {
		t.Fatal("layered decoding must reach a fixpoint and fail closed past the budget")
	}
	if _, _, err := decodeList(json.RawMessage(`[null]`)); err == nil {
		t.Fatal("a non-object list element must be an error")
	}
	for _, body := range []string{"", "null", "  "} {
		if _, _, err := decodeList(json.RawMessage(body)); err == nil {
			t.Fatalf("an empty or null body %q must not read as an empty inventory", body)
		}
	}
	if got := contentEncoding(http.Header{"Content-Encoding": {"gzip", "deflate"}}); got != "multiple" {
		t.Fatalf("stacked encodings = %q, want multiple (rejected)", got)
	}
	if got := contentEncoding(http.Header{"Content-Encoding": {"identity, gzip"}}); got != "gzip" {
		t.Fatalf("contentEncoding = %q, want gzip", got)
	}
	if _, _, err := decodeList(json.RawMessage(`{}`)); err == nil {
		t.Fatal("an inventory route answering a lone object must be an error")
	}
	if m, err := singleObject(json.RawMessage(`{"id":103,"email":"x@example.com"}`)); err != nil || str(m, "id") != "103" {
		t.Fatalf("singleObject = %v, %v", m, err)
	}
	if _, err := ssoLinkFrom(json.RawMessage(`{"other":"x"}`)); err == nil {
		t.Fatal("a sign-in response without redirect_url must be an error")
	}
	// A client holding organization A's token never follows a redirect to B.
	c := client.New(&config.Config{BaseURL: "http://x.example/api"}, time.Second, 0)
	guardOrgRedirects(c, "A-2")
	orig, _ := http.NewRequest(http.MethodGet, "http://x.example/api/users/A-2/tenants", nil)
	toB, _ := http.NewRequest(http.MethodGet, "http://x.example/api/users/B-2/tenants", nil)
	toBDetail, _ := http.NewRequest(http.MethodGet, "http://x.example/api/users/B-2", nil)
	samePath, _ := http.NewRequest(http.MethodGet, "http://x.example/api/users/A-2/tenants?page=2", nil)
	if c.HTTPClient.CheckRedirect(toB, []*http.Request{orig}) == nil || c.HTTPClient.CheckRedirect(toBDetail, []*http.Request{orig}) == nil {
		t.Fatal("redirect to another path must be refused")
	}
	if err := c.HTTPClient.CheckRedirect(samePath, []*http.Request{orig}); err != nil {
		t.Fatalf("a same-path redirect is allowed: %v", err)
	}
	// An HTML form field carrying another organization's token, inside a
	// JSON string or as a whole non-JSON body.
	form := `<input name="authentication_token" value="other-org-secret">`
	if got := string(redactSecretFields([]byte(form))); strings.Contains(got, "other-org-secret") {
		t.Fatalf("HTML credential field leaked: %s", got)
	}
	if got := string(redactSecretFields([]byte(`{"html":"<input name=\"authentication_token\" value=\"other-org-secret\">"}`))); strings.Contains(got, "other-org-secret") {
		t.Fatalf("HTML credential field in a JSON string leaked: %s", got)
	}
	// A key that is itself escaped (double-serialized) is still a credential key.
	if got := string(redactSecretFields([]byte(`{"authentic\\u0061tion_token":"other-org-secret"}`))); strings.Contains(got, "other-org-secret") {
		t.Fatalf("escaped secret key leaked: %s", got)
	}
	// restore-test due sees every organization any health run has seen.
	ctx := context.Background()
	db, err := openFleetDB(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = saveHealthSnapshot(ctx, db.DB(), "r1", testNow.Add(-time.Hour), []healthRow{{OrgID: "A-2", Kind: "mailbox", Object: "a", Grade: "ok"}, {OrgID: "B-2", Kind: "mailbox", Object: "b", Grade: "ok"}})
	_ = saveHealthSnapshot(ctx, db.DB(), "r2", testNow, []healthRow{{OrgID: "A-2", Kind: "mailbox", Object: "a", Grade: "ok"}})
	orgs, err := knownOrgs(ctx, db.DB())
	if err != nil || len(orgs) != 2 {
		t.Fatalf("knownOrgs = %+v, %v; want A-2 and B-2", orgs, err)
	}
	// An owner email change keeps the old email as an alias.
	_ = saveHealthSnapshot(ctx, db.DB(), "r3", testNow.Add(time.Hour), []healthRow{{OrgID: "A-2", OrgEmail: "new@a.example", Kind: "mailbox", Object: "a", Grade: "ok"}})
	_ = saveHealthSnapshot(ctx, db.DB(), "r4", testNow.Add(-2*time.Hour), []healthRow{{OrgID: "A-2", OrgEmail: "old@a.example", Kind: "mailbox", Object: "a", Grade: "ok"}})
	orgs, _ = knownOrgs(ctx, db.DB())
	var a knownOrg
	for _, o := range orgs {
		if o.ID == "A-2" {
			a = o
		}
	}
	if a.Email != "new@a.example" || !a.matches("old@a.example") {
		t.Fatalf("A-2 = %+v; want new@ primary and old@ still matching", a)
	}
	// Evidence merges live identities with that history.
	merged := mergeKnown([]knownOrg{{ID: "A-2", Email: "live@a.example"}}, orgs)
	if len(merged) != 2 || !merged[0].matches("old@a.example") || !merged[0].matches("new@a.example") || merged[0].Email != "live@a.example" {
		t.Fatalf("mergeKnown = %+v", merged)
	}
}

// Two health runs taken minutes apart see the same last_backup on every
// healthy object; drift must not call that a stall. An unchanged backup that
// is over a day old at the later run still is one.
func TestDriftBackupNotAdvancingNeedsAge(t *testing.T) {
	t0, t1 := testNow.Add(-10*time.Minute), testNow
	recent := formatTime(testNow.Add(-3 * time.Hour))
	old := formatTime(testNow.Add(-30 * time.Hour))
	before := []snapshotRow{
		{OrgID: "1", Kind: "mailbox", Object: "fresh", Grade: "ok", LastBackup: recent, TakenAt: t0},
		{OrgID: "1", Kind: "mailbox", Object: "stuck", Grade: "ok", LastBackup: old, TakenAt: t0},
	}
	after := []snapshotRow{
		{OrgID: "1", Kind: "mailbox", Object: "fresh", Grade: "ok", LastBackup: recent, TakenAt: t1},
		{OrgID: "1", Kind: "mailbox", Object: "stuck", Grade: "ok", LastBackup: old, TakenAt: t1},
	}
	changes, _ := diffSnapshots(before, after)
	var stalled []string
	for _, c := range changes {
		if c.Change == "backup_not_advancing" {
			stalled = append(stalled, c.Object)
		}
	}
	if len(stalled) != 1 || stalled[0] != "stuck" {
		t.Fatalf("backup_not_advancing = %v, want only stuck", stalled)
	}
	if !backupStalled("not a timestamp", testNow) {
		t.Fatal("an unparseable last_backup must keep the equality signal")
	}
}

// --invoice is reachable from the MCP server, so a file that is not a PSA
// export must not have its first row echoed back in the error.
func TestInvoiceHeaderErrorDoesNotEchoFile(t *testing.T) {
	_, _, err := parseInvoiceCSV(strings.NewReader("API_SECRET=hunter2-do-not-echo\n"))
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("err = %v, want a usage error", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("header error echoed file contents: %v", err)
	}
}

// A failed organization listing exits with the documented codes (4 auth,
// 5 API) and names both reseller tokens, instead of a bare exit 1.
func TestFetchOrgsClassifiesFailures(t *testing.T) {
	testenv.Isolate(t)
	for _, tc := range []struct {
		status int
		code   int
		want   string
	}{
		{401, 4, "DROPSUITE_RESELLER_TOKEN"},
		{403, 4, "DROPSUITE_ACCESS_TOKEN"},
		{404, 5, "DROPSUITE_HOST"},
	} {
		c := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"errors":{"user":["Unauthorized"]}}`))
		})
		_, err := fetchOrgs(context.Background(), c)
		if err == nil || ExitCode(err) != tc.code || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("HTTP %d: exit %d err %v; want exit %d mentioning %s", tc.status, ExitCode(err), err, tc.code, tc.want)
		}
	}
}

// When DROPSUITE_ORG routing cannot list organizations, the error reaches a
// generated command unclassified, so its classifyAPIError still prints the
// agent JSON envelope (a *cliError would make it return before printing).
func TestRoutingOrgListFailureStaysUnclassified(t *testing.T) {
	testenv.Isolate(t)
	orgTokens = orgTokenDirectory{}
	t.Cleanup(func() { orgTokens = orgTokenDirectory{} })
	t.Setenv(defaultOrgEnv, "owner@alpha.example")
	t.Setenv(cliutil.VerifyEnvVar, "1") // no retries
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"errors":{"user":["Unauthorized"]}}`))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	// A token that appears in no message, so credential masking cannot
	// rewrite the error and hide the chain this test inspects.
	c := client.New(&config.Config{BaseURL: srv.URL + "/api", AccessToken: "acc-7f3e9q"}, 10*time.Second, 0)
	c.HTTPClient.Transport = srv.Client().Transport
	installRedaction(c)
	installOrgRouting(c)
	_, err := c.Get(context.Background(), "/accounts", nil)
	var typed *cliError
	if err == nil || errors.As(err, &typed) || !strings.Contains(err.Error(), "listing organizations") {
		t.Fatalf("routing error = %v (typed=%v); want an unclassified organization-list failure", err, typed != nil)
	}
	var buf strings.Builder
	classified := classifyAPIError(&buf, err, &rootFlags{asJSON: true})
	if ExitCode(classified) != 4 || !strings.Contains(buf.String(), `"code"`) {
		t.Fatalf("generated classification: exit %d, envelope %q", ExitCode(classified), buf.String())
	}
}
