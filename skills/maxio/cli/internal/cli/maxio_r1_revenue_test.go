// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex r1 (maxio 4.34.2 reprint) wrong-result classes in the revenue
// commands. Every fixture is a response shape spec.json documents
// (Subscription / SubscriptionState, Invoice / InvoiceStatus, MRR, Movement /
// MovementLineItem / MRRMovement), stored the way sync stores it.

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maxio-pp-cli/internal/cliutil/testenv"
	"maxio-pp-cli/internal/revenue"
	"maxio-pp-cli/internal/store"
)

const r1Snap = "2026-10-29T00:00:00Z"

type r1Fixture struct {
	t    *testing.T
	path string
	st   *store.Store
}

func newR1Fixture(t *testing.T) *r1Fixture {
	t.Helper()
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "r1.db")
	st, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := revenue.EnsureSchema(st.DB()); err != nil {
		t.Fatal(err)
	}
	f := &r1Fixture{t: t, path: path, st: st}
	t.Cleanup(func() { _ = st.Close() })
	return f
}

func (f *r1Fixture) put(resourceType, id, data string) {
	f.t.Helper()
	if !json.Valid([]byte(data)) {
		f.t.Fatalf("fixture %s %s is not JSON", resourceType, id)
	}
	if err := f.st.Upsert(resourceType, id, json.RawMessage(data)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *r1Fixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.st.DB().Exec(q, args...); err != nil {
		f.t.Fatalf("%s: %v", q, err)
	}
}

func (f *r1Fixture) subMRR(subID string, cents int64) {
	f.exec(`INSERT INTO rev_sub_mrr_snapshots (snapshot_at, subscription_id, mrr_cents, plan_cents, usage_cents) VALUES (?,?,?,?,0)`,
		r1Snap, subID, cents, cents)
}

// run executes one command against the fixture store and decodes its JSON.
func (f *r1Fixture) run(args ...string) map[string]any {
	f.t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(append([]string{}, args...), "--db", f.path, "--json"))
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	if err := cmd.Execute(); err != nil {
		f.t.Fatalf("%v: %v\nstderr: %s", args, err, errb.String())
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		f.t.Fatalf("%v: output is not one JSON object: %v\n%s", args, err, out.String())
	}
	return v
}

func num(v any) float64 {
	n, _ := v.(float64)
	return n
}

// Class current-states-narrowed-to-active: a subscription in a documented
// live or problem state (past_due, soft_failure, unpaid, trialing) is a
// current customer to every command that reads current customers; triage's
// past-due scoring was unreachable.
func TestCurrentCustomersIncludeProblemStates(t *testing.T) {
	f := newR1Fixture(t)
	ends := time.Now().AddDate(0, 0, 10).UTC().Format(time.RFC3339)
	f.put("customers-json", "1", `{"id":1,"organization":"Delinquent Co"}`)
	f.put("customers-json", "2", `{"id":2,"organization":"Paying Co"}`)
	f.put("subscriptions-json", "7", `{"id":7,"state":"past_due","customer":{"id":1},"current_period_ends_at":"`+ends+`","created_at":"2026-01-05T00:00:00Z"}`)
	f.put("subscriptions-json", "8", `{"id":8,"state":"active","customer":{"id":2},"current_period_ends_at":"`+ends+`","created_at":"2026-01-05T00:00:00Z"}`)
	f.subMRR("7", 5000)
	f.subMRR("8", 5000)
	f.exec(`INSERT INTO rev_site_snapshots (snapshot_at, at_time, mrr_cents, currency, active_subs, total_subs) VALUES (?,?,?,?,?,?)`,
		r1Snap, r1Snap, 10000, "USD", 2, 2)

	tri := f.run("triage")
	if b, _ := json.Marshal(tri); !strings.Contains(string(b), "Delinquent Co") || !strings.Contains(string(b), "past-due/unpaid subscription") {
		t.Errorf("triage omits the past-due customer: %s", b)
	}
	if c := f.run("concentration"); num(c["total_mrr_cents"]) != 10000 || num(c["total_customers"]) != 2 {
		t.Errorf("concentration total %v over %v customers, want 10000 over 2", c["total_mrr_cents"], c["total_customers"])
	}
	if r := f.run("renewals"); num(r["count"]) != 2 {
		t.Errorf("renewals counts %v, want 2 (the past-due subscription renews too)", r["count"])
	}
	if m := f.run("mrr", "client"); num(m["total_mrr_cents"]) != 10000 {
		t.Errorf("mrr client total %v, want 10000", m["total_mrr_cents"])
	}
	if c := f.run("cohort"); func() bool {
		rows, _ := c["cohorts"].([]any)
		return len(rows) != 1 || num(rows[0].(map[string]any)["still_active"]) != 2
	}() {
		t.Errorf("cohort does not count the past-due subscription as still current: %v", c["cohorts"])
	}
}

// Class missing-invoice-detail-read-as-zero: GET /invoices.json returns
// totals only unless line_items=true (spec.json, its line_items parameter
// defaults to false); an invoice without line items is not zero revenue.
func TestInvoiceWithoutLineItemsIsNotZeroRevenue(t *testing.T) {
	f := newR1Fixture(t)
	f.put("invoices-json", "inv_a", `{"uid":"inv_a","status":"paid","issue_date":"2026-10-01","total_amount":"100.00"}`)
	v := f.run("revenue-mix", "--month", "2026-10")
	if num(v["total_cents"]) != 10000 || num(v["unclassified_cents"]) != 10000 || num(v["invoices_without_line_items"]) != 1 {
		t.Errorf("revenue-mix total %v cents (unclassified %v over %v invoices), want 10000 unclassified over 1 (the paid invoice)",
			v["total_cents"], v["unclassified_cents"], v["invoices_without_line_items"])
	}
	// With line items the kinds split it; nothing is unclassified.
	f.put("invoices-json", "inv_b", `{"uid":"inv_b","status":"open","issue_date":"2026-11-02","total_amount":"30.00","line_items":[{"kind":"baseline","total_amount":"20.00"},{"kind":"metered_component","total_amount":"10.00"}]}`)
	v = f.run("revenue-mix", "--month", "2026-11")
	if num(v["total_cents"]) != 3000 || num(v["recurring_cents"]) != 2000 || num(v["usage_cents"]) != 1000 || num(v["unclassified_cents"]) != 0 {
		t.Errorf("revenue-mix with line items: %v", v)
	}
	if got := syncResourceParamDefaults("invoices-json")["line_items"]; got != "true" {
		t.Errorf("the default sync asks /invoices.json for line_items=%q, want true", got)
	}
}

// Class unbilled-invoice-status-counted: only issued invoices are billed
// revenue; InvoiceStatus documents draft, voided and canceled.
func TestUnbilledInvoiceStatusesAreNotRevenue(t *testing.T) {
	f := newR1Fixture(t)
	day := time.Now().AddDate(0, 0, -5).UTC().Format("2006-01-02")
	f.put("customers-json", "1", `{"id":1,"organization":"Acme"}`)
	f.put("subscriptions-json", "10", `{"id":10,"state":"active","customer":{"id":1}}`)
	f.subMRR("10", 10000)
	f.put("invoices-json", "a", `{"uid":"a","subscription_id":10,"status":"paid","issue_date":"`+day+`","total_amount":"100.00"}`)
	f.put("invoices-json", "b", `{"uid":"b","subscription_id":10,"status":"voided","issue_date":"`+day+`","total_amount":"1000.00"}`)
	f.put("invoices-json", "c", `{"uid":"c","subscription_id":10,"status":"draft","issue_date":"`+day+`","total_amount":"500.00"}`)
	f.put("invoices-json", "d", `{"uid":"d","subscription_id":10,"status":"canceled","issue_date":"`+day+`","total_amount":"700.00"}`)
	v := f.run("reconcile", "--since", "1m")
	totals, _ := v["totals"].(map[string]any)
	if num(totals["invoiced_cents"]) != 10000 {
		t.Errorf("reconcile invoiced %v cents, want 10000 (only the paid invoice)", totals["invoiced_cents"])
	}
	m := f.run("revenue-mix", "--month", day[:7])
	if num(m["total_cents"]) != 10000 {
		t.Errorf("revenue-mix total %v cents, want 10000", m["total_cents"])
	}
}

// Class currency-dropped: /mrr.json documents currency; a EUR site's local
// snapshot is EUR, never "USD" or a dollar sign.
func TestSnapshotCurrencyIsKept(t *testing.T) {
	f := newR1Fixture(t)
	f.exec(`INSERT INTO rev_site_snapshots (snapshot_at, at_time, mrr_cents, currency, active_subs, total_subs) VALUES (?,?,?,?,?,?)`,
		r1Snap, r1Snap, 10000, "EUR", 1, 1)
	v := f.run("mrr", "now", "--data-source", "local")
	if v["currency"] != "EUR" || strings.Contains(v["mrr"].(string), "$") {
		t.Errorf("mrr now local: currency %v, mrr %v; want EUR without a dollar sign", v["currency"], v["mrr"])
	}
	// Every revenue command renders in the site's currency.
	f.put("customers-json", "1", `{"id":1,"organization":"Acme"}`)
	f.put("subscriptions-json", "10", `{"id":10,"state":"active","customer":{"id":1}}`)
	f.subMRR("10", 10000)
	m := f.run("mrr", "client")
	rows, _ := m["customers"].([]any)
	if m["currency"] != "EUR" || len(rows) != 1 || rows[0].(map[string]any)["mrr"] != "100.00 EUR" {
		t.Errorf("mrr client: currency %v, customers %v; want EUR amounts", m["currency"], m["customers"])
	}
	for in, want := range map[string]string{"USD": "$1,234.56", "EUR": "1,234.56 EUR", "": "1,234.56"} {
		if got := revenue.Money(123456, in); got != want {
			t.Errorf("Money(123456, %q) = %q, want %q", in, got, want)
		}
	}
}

// Class movement-category-ignored: a line item's MRRMovement carries its own
// category (spec.json example: new_business); acquisition is not expansion.
func TestLineItemMovementCategoryIsRead(t *testing.T) {
	f := newR1Fixture(t)
	li := `[{"product_id":306386,"component_id":0,"price_point_id":3856987,"name":"Cached Queries","mrr":2173,"mrr_movements":[{"amount":2173,"category":"new_business","subscriber_delta":0,"lead_delta":0}],"quantity":1,"prev_quantity":0,"recurring":true}]`
	f.exec(`INSERT INTO rev_mrr_movements (mvmt_key, ts, amount_cents, category, bucket, plan_cents, usage_cents, description, line_items) VALUES (?,?,?,?,?,?,?,?,?)`,
		"k1", "2014-12-03T13:59:46-05:00", 2173, "new_business", revenue.BucketNew, 2173, 0, "Awesome Company signed up", li)
	v := f.run("usage-drivers")
	for _, d := range v["drivers"].([]any) {
		if m := d.(map[string]any); num(m["expansion_cents"]) != 0 || num(m["contraction_cents"]) != 0 {
			t.Errorf("a new_business line item counted as expansion/contraction: %v", m)
		}
	}
	// An expansion line item is still a driver.
	exp := `[{"name":"Seats","mrr":500,"mrr_movements":[{"amount":500,"category":"expansion"}]},{"name":"Seats","mrr":-200,"mrr_movements":[{"amount":-200,"category":"contraction"}]}]`
	f.exec(`INSERT INTO rev_mrr_movements (mvmt_key, ts, amount_cents, category, bucket, plan_cents, usage_cents, description, line_items) VALUES (?,?,?,?,?,?,?,?,?)`,
		"k2", "2015-01-03T13:59:46-05:00", 300, "expansion", revenue.BucketExpansion, 300, 0, "upgrade", exp)
	v = f.run("usage-drivers")
	ds, _ := v["drivers"].([]any)
	if len(ds) != 1 || num(ds[0].(map[string]any)["expansion_cents"]) != 500 || num(ds[0].(map[string]any)["contraction_cents"]) != -200 {
		t.Errorf("usage-drivers drivers %v, want Seats +500/-200 only", ds)
	}
}

// Class subscription-counted-as-logo: a customer is churned only when no
// current subscription remains, and a cohort's logo retention counts
// customers, not subscriptions.
func TestLogosAreCustomersNotSubscriptions(t *testing.T) {
	f := newR1Fixture(t)
	f.put("customers-json", "1", `{"id":1,"organization":"Retained Co"}`)
	f.put("customers-json", "2", `{"id":2,"organization":"Gone Co"}`)
	f.put("subscriptions-json", "10", `{"id":10,"state":"canceled","customer":{"id":1},"canceled_at":"2026-10-10T00:00:00Z","cancellation_method":"merchant_api","created_at":"2026-09-01T00:00:00Z","activated_at":"2026-09-01T00:00:00Z"}`)
	f.put("subscriptions-json", "11", `{"id":11,"state":"active","customer":{"id":1},"created_at":"2026-09-01T00:00:00Z","activated_at":"2026-09-01T00:00:00Z"}`)
	f.put("subscriptions-json", "12", `{"id":12,"state":"canceled","customer":{"id":2},"canceled_at":"2026-10-12T00:00:00Z","cancellation_method":"dunning","created_at":"2026-09-01T00:00:00Z","activated_at":"2026-09-01T00:00:00Z"}`)
	v := f.run("churn", "--since", "2026-10-01")
	logos := 0.0
	for _, p := range v["periods"].([]any) {
		logos += num(p.(map[string]any)["churned_logos"])
	}
	if logos != 1 {
		t.Errorf("churn reports %v churned logos, want 1 (customer 1 is still subscribed)", logos)
	}
	if rs, _ := v["reasons"].([]any); len(rs) != 1 || rs[0].(map[string]any)["reason"] != "dunning" {
		t.Errorf("churn reasons %v, want one: dunning (the churned customer's)", v["reasons"])
	}
	c := f.run("cohort")
	rows, _ := c["cohorts"].([]any)
	if len(rows) != 1 || num(rows[0].(map[string]any)["logo_retention_pct"]) != 50 {
		t.Errorf("cohort logo retention %v, want 50 (one of two customers retained)", c["cohorts"])
	}
}

// Class currency-dropped, invoice side: Invoice.currency (spec.json, ISO 4217
// per invoice) on a multi-currency site; an amount in another currency is
// never added to the site-currency total.
func TestInvoicesInAnotherCurrencyAreNotSummed(t *testing.T) {
	f := newR1Fixture(t)
	day := time.Now().AddDate(0, 0, -5).UTC().Format("2006-01-02")
	f.exec(`INSERT INTO rev_site_snapshots (snapshot_at, at_time, mrr_cents, currency, active_subs, total_subs) VALUES (?,?,?,?,?,?)`,
		r1Snap, r1Snap, 10000, "USD", 1, 1)
	f.put("customers-json", "1", `{"id":1,"organization":"Acme"}`)
	f.put("subscriptions-json", "10", `{"id":10,"state":"active","customer":{"id":1}}`)
	f.subMRR("10", 10000)
	f.put("invoices-json", "a", `{"uid":"a","subscription_id":10,"status":"paid","currency":"USD","issue_date":"`+day+`","total_amount":"100.00","line_items":[{"kind":"baseline","total_amount":"100.00"}]}`)
	f.put("invoices-json", "b", `{"uid":"b","subscription_id":10,"status":"paid","currency":"EUR","issue_date":"`+day+`","total_amount":"1000.00","line_items":[{"kind":"baseline","total_amount":"1000.00"}]}`)
	v := f.run("reconcile", "--since", "1m")
	totals, _ := v["totals"].(map[string]any)
	if num(totals["invoiced_cents"]) != 10000 || num(v["other_currency_invoices"]) != 1 {
		t.Errorf("reconcile invoiced %v cents (other-currency invoices %v), want 10000 USD and 1 EUR invoice set aside", totals["invoiced_cents"], v["other_currency_invoices"])
	}
	m := f.run("revenue-mix", "--month", day[:7])
	if num(m["total_cents"]) != 10000 || num(m["other_currency_invoices"]) != 1 {
		t.Errorf("revenue-mix total %v cents (other-currency invoices %v), want 10000 and 1", m["total_cents"], m["other_currency_invoices"])
	}
}

// runStderr runs one command against the fixture store and returns its stderr.
func (f *r1Fixture) runStderr(args ...string) string {
	f.t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(append([]string{}, args...), "--db", f.path, "--json"))
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	if err := cmd.Execute(); err != nil {
		f.t.Fatalf("%v: %v\nstderr: %s", args, err, errb.String())
	}
	return errb.String()
}

// Class incomplete-backfill-read-as-complete: a store whose first movement page
// failed holds no movements at all; churn and new-customers then report $0
// unless they say the history is incomplete.
func TestColdFailedBackfillWarnsEveryMovementReader(t *testing.T) {
	for _, c := range []string{"churn", "new-customers"} {
		f := newR1Fixture(t)
		f.exec(`CREATE TABLE IF NOT EXISTS rev_sync_state (key TEXT PRIMARY KEY, value TEXT)`)
		f.exec(`INSERT OR REPLACE INTO rev_sync_state (key, value) VALUES ('movements_backfill','partial')`)
		if got := f.runStderr(c, "--since", "2026-10-01"); !strings.Contains(got, "movement history") {
			t.Errorf("%s read a failed cold backfill as complete; stderr %q", c, got)
		}
		f.exec(`UPDATE rev_sync_state SET value='complete' WHERE key='movements_backfill'`)
		if got := f.runStderr(c, "--since", "2026-10-01"); strings.Contains(got, "movement history") {
			t.Errorf("%s warned over a complete history; stderr %q", c, got)
		}
	}
}

// Class consolidated-invoice-double-counted: "Parent" invoices have no lines of
// their own but totals that aggregate the member segments (spec.json
// InvoiceConsolidationLevel), so a parent with a stored child is not billed a
// second time. A parent whose children are not stored is the only record.
func TestConsolidatedParentIsNotCountedOnTopOfItsChild(t *testing.T) {
	parent := `{"uid":"inv_p","status":"paid","issue_date":"2026-10-01","currency":"USD","total_amount":"100.00","consolidation_level":"parent","subscription_id":1}`
	child := `{"uid":"inv_c","status":"paid","issue_date":"2026-10-01","currency":"USD","total_amount":"100.00","consolidation_level":"child","parent_invoice_uid":"inv_p","subscription_id":1,"line_items":[{"kind":"baseline","total_amount":"100.00"}]}`

	f := newR1Fixture(t)
	f.put("invoices-json", "inv_p", parent)
	v := f.run("revenue-mix", "--month", "2026-10")
	if num(v["total_cents"]) != 10000 {
		t.Errorf("a parent with no stored child: revenue-mix total %v cents, want 10000", v["total_cents"])
	}
	f.put("invoices-json", "inv_c", child)
	v = f.run("revenue-mix", "--month", "2026-10")
	if num(v["total_cents"]) != 10000 || num(v["unclassified_cents"]) != 0 {
		t.Errorf("revenue-mix total %v cents (unclassified %v), want 10000 and 0 (the child carries the parent's lines)", v["total_cents"], v["unclassified_cents"])
	}

	f.put("customers-json", "9", `{"id":9,"organization":"Acme"}`)
	f.put("subscriptions-json", "1", `{"id":1,"state":"active","customer":{"id":9}}`)
	f.subMRR("1", 10000)
	v = f.run("reconcile", "--since", "2026-01-01")
	rows, _ := v["customers"].([]any)
	if len(rows) != 1 {
		t.Fatalf("reconcile rows: %v", v)
	}
	if got := num(rows[0].(map[string]any)["invoiced_cents"]); got != 10000 {
		t.Errorf("reconcile invoiced %v cents, want 10000 (parent + child is one bill)", got)
	}
}
