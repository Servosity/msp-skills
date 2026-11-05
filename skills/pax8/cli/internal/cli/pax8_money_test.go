// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// spend and mrr print money an MSP acts on (handfixes.json novel-money-shapes).
// Every expected figure below is worked by hand from the fixture beside it.

func moneyID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func runNovelText(t *testing.T, dbPath string, args ...string) string {
	t.Helper()
	root := RootCmd()
	var buf, errb bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&errb)
	root.SetArgs(append(args, "--db", dbPath))
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, errb.String())
	}
	return buf.String()
}

func invoiceRow(n int, company, status string, total float64, currency string) string {
	s := fmt.Sprintf(`{"id":"%s","total":%v`, moneyID(n), total)
	if company != "" {
		s += `,"companyId":"` + company + `"`
	}
	if status != "" {
		s += `,"status":"` + status + `"`
	}
	if currency != "" {
		s += `,"currencyCode":"` + currency + `"`
	}
	return s + "}"
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// spendRowFor finds the one row of a company in a currency.
func spendRowFor(t *testing.T, rows []map[string]any, company, currency string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, r := range rows {
		cur, _ := r["currency"].(string)
		if r["companyId"] == company && cur == currency {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d rows for company %s in %q, want 1: %+v", len(found), company, currency, rows)
	}
	return found[0]
}

func wantFields(t *testing.T, label string, got map[string]any, want map[string]any) {
	t.Helper()
	for k, v := range want {
		g, present := got[k]
		if v == nil && !present {
			t.Errorf("%s: %q is absent, want it present and null: %+v", label, k, got)
			continue
		}
		if f, ok := v.(int); ok {
			v = float64(f)
		}
		if gf, ok := g.(float64); ok {
			if vf, ok := v.(float64); ok && math.Abs(gf-vf) < 1e-9 {
				continue
			}
		}
		if g != v {
			t.Errorf("%s: %q = %v, want %v: %+v", label, k, g, v, got)
		}
	}
}

// The /invoices status enum: Unpaid, Paid and Carried were billed; Void, Nothing
// Due and Credited are not owed; anything else is reported and never summed.
func TestSpendCountsOnlyWhatIsOwed(t *testing.T) {
	path, db := novelStore(t)
	const acme, beta, gamma, delta = "c0000000-0000-4000-8000-000000000001", "c0000000-0000-4000-8000-000000000002", "c0000000-0000-4000-8000-000000000003", "c0000000-0000-4000-8000-000000000004"
	seedAsSync(t, db, "invoices", "", "",
		invoiceRow(1, acme, "Carried", 100, "USD"),
		invoiceRow(2, acme, "Unpaid", 200, "USD"),
		invoiceRow(3, acme, "Paid", 50, "USD"),
		invoiceRow(4, beta, "Void", 150, "USD"),
		invoiceRow(5, gamma, "Nothing Due", 20, "USD"),
		invoiceRow(6, gamma, "Credited", 30, "USD"),
		invoiceRow(7, delta, "Refunded", 70, "USD"),
		invoiceRow(8, delta, "", 5, "USD"))
	var rows []map[string]any
	runNovelJSON(t, path, &rows, "spend")
	// 100 + 200 + 50: three invoices.
	wantFields(t, "acme", spendRowFor(t, rows, acme, "USD"), map[string]any{"invoiceCount": 3, "total": 350.0})
	wantFields(t, "beta", spendRowFor(t, rows, beta, "USD"), map[string]any{"invoiceCount": 0, "total": 0.0, "notOwedInvoices": 1})
	wantFields(t, "gamma", spendRowFor(t, rows, gamma, "USD"), map[string]any{"invoiceCount": 0, "total": 0.0, "notOwedInvoices": 2})
	// 70 + 5 stays visible, beside the total and never in it.
	wantFields(t, "delta", spendRowFor(t, rows, delta, "USD"), map[string]any{"invoiceCount": 0, "total": 0.0, "unclassifiedInvoices": 2, "unclassifiedTotal": 75.0})

	text := runNovelText(t, path, "spend")
	for _, want := range []string{"350.00 USD", "1 not owed", "2 not owed", "2 with a status Pax8 does not document, 75.00 USD"} {
		if !strings.Contains(text, want) {
			t.Errorf("spend text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "150.00") {
		t.Errorf("spend text shows the voided 150:\n%s", text)
	}
}

// The statuses are read over their documented spelling, not byte for byte.
func TestSpendReadsAStatusWhateverItsCase(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "invoices", "", "",
		invoiceRow(1, cid1, " void ", 150, "USD"),
		invoiceRow(2, cid1, "NOTHING DUE", 20, "USD"),
		invoiceRow(3, cid1, "paid", 7, "USD"))
	var rows []map[string]any
	runNovelJSON(t, path, &rows, "spend")
	wantFields(t, "row", spendRowFor(t, rows, cid1, "USD"), map[string]any{"invoiceCount": 1, "total": 7.0, "notOwedInvoices": 2})
}

func TestSpendShowsEveryCompanyByDefault(t *testing.T) {
	path, db := novelStore(t)
	var invoices []string
	for i := 1; i <= 30; i++ {
		invoices = append(invoices, invoiceRow(i, fmt.Sprintf("c0000000-0000-4000-8000-%012d", i), "Paid", float64(i), "USD"))
	}
	seedAsSync(t, db, "invoices", "", "", invoices...)
	var rows []map[string]any
	runNovelJSON(t, path, &rows, "spend")
	if len(rows) != 30 {
		t.Fatalf("spend --json: %d rows, want every one of 30", len(rows))
	}
	if text := runNovelText(t, path, "spend"); strings.Count(text, " USD") != 30 {
		t.Fatalf("spend: %d rows, want 30:\n%s", strings.Count(text, " USD"), text)
	}
	var limited []map[string]any
	runNovelJSON(t, path, &limited, "spend", "--limit", "5")
	if len(limited) != 5 || limited[0]["total"] != 30.0 {
		t.Fatalf("spend --limit 5 = %+v, want the top 5", limited)
	}
	var all []map[string]any
	runNovelJSON(t, path, &all, "spend", "--limit", "0")
	if len(all) != 30 {
		t.Fatalf("spend --limit 0: %d rows, want 30", len(all))
	}
}

func TestSpendNeverAddsCurrencies(t *testing.T) {
	path, db := novelStore(t)
	const other = "c0000000-0000-4000-8000-000000000002"
	seedAsSync(t, db, "invoices", "", "",
		invoiceRow(1, cid1, "Paid", 100, "USD"),
		invoiceRow(2, cid1, "Paid", 20, "usd"),
		invoiceRow(3, cid1, "Paid", 300, "CAD"),
		invoiceRow(4, cid1, "Paid", 9, ""),
		invoiceRow(5, other, "Paid", 40, "USD"))
	var rows []map[string]any
	runNovelJSON(t, path, &rows, "spend")
	if len(rows) != 4 {
		t.Fatalf("spend = %d rows, want 4: %+v", len(rows), rows)
	}
	wantFields(t, "usd", spendRowFor(t, rows, cid1, "USD"), map[string]any{"invoiceCount": 2, "total": 120.0})
	wantFields(t, "cad", spendRowFor(t, rows, cid1, "CAD"), map[string]any{"invoiceCount": 1, "total": 300.0})
	wantFields(t, "none", spendRowFor(t, rows, cid1, ""), map[string]any{"invoiceCount": 1, "total": 9.0})
	wantFields(t, "other", spendRowFor(t, rows, other, "USD"), map[string]any{"invoiceCount": 1, "total": 40.0})
	text := runNovelText(t, path, "spend")
	for _, want := range []string{"120.00 USD", "300.00 CAD", "40.00 USD"} {
		if !strings.Contains(text, want) {
			t.Errorf("spend text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "429") || strings.Contains(text, "420") {
		t.Errorf("spend text added currencies:\n%s", text)
	}
}

func TestSpendSingleCurrencyKeepsItsShape(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "companies", "", "", `{"id":"`+cid1+`","name":"Acme"}`)
	seedAsSync(t, db, "invoices", "", "", invoiceRow(1, cid1, "Paid", 100, "USD"))
	var rows []map[string]any
	runNovelJSON(t, path, &rows, "spend")
	want := []map[string]any{{"companyId": cid1, "companyName": "Acme", "invoiceCount": 1.0, "total": 100.0, "currency": "USD"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("spend --json = %+v, want %+v", rows, want)
	}
}

// examples/invoices-paged carries no companyId: the row says so.
func TestSpendNamesARowWithNoCompany(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "invoices", "", "", invoiceRow(1, "", "Paid", 100, "USD"))
	if text := runNovelText(t, path, "spend"); !strings.Contains(text, "(no company)\t1\t100.00 USD") {
		t.Fatalf("spend:\n%s", text)
	}
}

func subscriptionRow(id, term string, qty, price float64, cost any, currency string) string {
	s := fmt.Sprintf(`{"id":"%s","companyId":"%s","productId":"%s","status":"Active","billingTerm":"%s","quantity":%v,"price":%v`, id, cid1, pid1, term, qty, price)
	switch c := cost.(type) {
	case nil:
	case string:
		s += `,"partnerCost":` + c
	default:
		s += fmt.Sprintf(`,"partnerCost":%v`, c)
	}
	if currency != "" {
		s += `,"currencyCode":"` + currency + `"`
	}
	return s + "}"
}

type mrrJSON struct {
	Totals    map[string]any   `json:"totals"`
	ByProduct []map[string]any `json:"byProduct"`
}

// examples/microsoft-office-365-e3-subscription verbatim: no partnerCost.
func TestMRRHasNoMarginWithoutAPartnerCost(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "", `{"id":"05c587df-9e7d-4ef9-8218-0262198e36c9","companyId":"fe7a2d5f-4a14-43ff-940e-a65cda66474a","productId":"e91bba5e-a9a3-483f-b036-f80165d410e3","quantity":5,"startDate":"2021-01-01T08:30:00Z","createdDate":"2020-12-01T08:30:00Z","billingStart":"2021-01-01","status":"Active","price":32.01,"currencyCode":"USD","billingTerm":"Monthly"}`)
	var got mrrJSON
	runNovelJSON(t, path, &got, "mrr")
	// 32.01 x 5.
	wantFields(t, "totals", got.Totals, map[string]any{"mrr": 160.05, "margin": nil, "activeSubscriptions": 1, "subscriptionsWithoutCost": 1, "currency": "USD"})
	if len(got.ByProduct) != 1 {
		t.Fatalf("byProduct = %+v", got.ByProduct)
	}
	wantFields(t, "product", got.ByProduct[0], map[string]any{"mrr": 160.05, "margin": nil, "subscriptionsWithoutCost": 1})

	text := runNovelText(t, path, "mrr")
	if !strings.Contains(text, "MRR:    160.05 USD") || !strings.Contains(text, "Margin: unknown") || !strings.Contains(text, "1 subscription(s) carry no partner cost") {
		t.Errorf("mrr text:\n%s", text)
	}
	if strings.Contains(text, "Margin: 160.05") || strings.Contains(text, "\t160.05\t160.05") {
		t.Errorf("mrr text reports the MRR as a margin:\n%s", text)
	}
}

func TestMRRMarginOnlyOverRowsThatCarryTheCost(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		subscriptionRow(moneyID(1), "Monthly", 2, 10, 6, "USD"),     // MRR 20, margin 8
		subscriptionRow(moneyID(2), "Monthly", 1, 10, nil, "USD"),   // MRR 10, no cost
		subscriptionRow(moneyID(3), "Monthly", 1, 5, 0, "USD"),      // MRR 5, margin 5: a cost of 0 is a cost
		subscriptionRow(moneyID(4), "Monthly", 1, 3, "null", "USD")) // MRR 3, no cost
	var got mrrJSON
	runNovelJSON(t, path, &got, "mrr")
	wantFields(t, "totals", got.Totals, map[string]any{"mrr": 38.0, "margin": 13.0, "activeSubscriptions": 4, "subscriptionsWithoutCost": 2})
	wantFields(t, "product", got.ByProduct[0], map[string]any{"mrr": 38.0, "margin": 13.0, "subscriptionsWithoutCost": 2})
	text := runNovelText(t, path, "mrr")
	if !strings.Contains(text, "Margin: 13.00 USD") || !strings.Contains(text, "2 subscription(s) carry no partner cost") {
		t.Errorf("mrr text:\n%s", text)
	}
}

func TestMRRTermRowHasNoMarginWithoutAPartnerCost(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		subscriptionRow(moneyID(1), "Annual", 1, 1200, nil, "USD"),
		subscriptionRow(moneyID(2), "2-Year", 1, 2000, 1500, "USD"),
		subscriptionRow(moneyID(3), "2-Year", 1, 100, nil, "USD"))
	var got mrrJSON
	runNovelJSON(t, path, &got, "mrr")
	terms, _ := got.Totals["otherCadence"].([]any)
	if len(terms) != 2 {
		t.Fatalf("otherCadence = %+v", got.Totals["otherCadence"])
	}
	wantFields(t, "2-Year", terms[0].(map[string]any), map[string]any{"billingTerm": "2-Year", "amount": 2100.0, "margin": 500.0, "subscriptions": 2, "subscriptionsWithoutCost": 1})
	wantFields(t, "Annual", terms[1].(map[string]any), map[string]any{"billingTerm": "Annual", "amount": 1200.0, "margin": nil, "subscriptions": 1, "subscriptionsWithoutCost": 1})
	text := runNovelText(t, path, "mrr")
	if strings.Contains(text, "margin 1200.00") {
		t.Errorf("mrr text reports the amount as a margin:\n%s", text)
	}
	if !strings.Contains(text, "1 Annual subscription(s), 1200.00 USD per term (margin unknown") {
		t.Errorf("mrr text:\n%s", text)
	}
}

func TestMRRNeverAddsCurrencies(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		subscriptionRow(moneyID(1), "Monthly", 1, 100, 60, "USD"),
		subscriptionRow(moneyID(2), "Monthly", 1, 100, 60, "CAD"),
		subscriptionRow(moneyID(3), "Monthly", 1, 7, nil, "cad"),
		subscriptionRow(moneyID(4), "Annual", 1, 1200, 1000, "EUR"))
	var got mrrJSON
	runNovelJSON(t, path, &got, "mrr")
	// No single sum exists, so the flat figures say so.
	wantFields(t, "totals", got.Totals, map[string]any{"mrr": nil, "margin": nil, "activeSubscriptions": 3, "subscriptionsWithoutCost": 1})
	if cur, present := got.Totals["currency"]; present {
		t.Errorf("totals name one currency, %v", cur)
	}
	by, _ := got.Totals["byCurrency"].([]any)
	if len(by) != 3 {
		t.Fatalf("byCurrency = %+v", got.Totals["byCurrency"])
	}
	wantFields(t, "CAD", by[0].(map[string]any), map[string]any{"currency": "CAD", "mrr": 107.0, "margin": 40.0, "activeSubscriptions": 2, "subscriptionsWithoutCost": 1})
	wantFields(t, "EUR", by[1].(map[string]any), map[string]any{"currency": "EUR", "mrr": 0.0, "activeSubscriptions": 0})
	wantFields(t, "USD", by[2].(map[string]any), map[string]any{"currency": "USD", "mrr": 100.0, "margin": 40.0, "activeSubscriptions": 1, "subscriptionsWithoutCost": 0})
	// One product, sold in two currencies: one row each.
	if len(got.ByProduct) != 2 {
		t.Fatalf("byProduct = %+v", got.ByProduct)
	}
	wantFields(t, "product CAD", got.ByProduct[0], map[string]any{"currency": "CAD", "mrr": 107.0, "margin": 40.0})
	wantFields(t, "product USD", got.ByProduct[1], map[string]any{"currency": "USD", "mrr": 100.0, "margin": 40.0})
	terms, _ := got.Totals["otherCadence"].([]any)
	if len(terms) != 1 {
		t.Fatalf("otherCadence = %+v", got.Totals["otherCadence"])
	}
	wantFields(t, "Annual", terms[0].(map[string]any), map[string]any{"currency": "EUR", "amount": 1200.0, "margin": 200.0})

	text := runNovelText(t, path, "mrr")
	for _, want := range []string{"MRR:    107.00 CAD", "MRR:    100.00 USD", "Margin: 40.00 USD", "1200.00 EUR per term (margin 200.00)"} {
		if !strings.Contains(text, want) {
			t.Errorf("mrr text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "207") {
		t.Errorf("mrr text added currencies:\n%s", text)
	}
}

func TestMRRSingleCurrencyKeepsItsShape(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		subscriptionRow(moneyID(1), "Monthly", 2, 10, 6, "USD"),
		subscriptionRow(moneyID(2), "Annual", 1, 1200, 1000, "USD"))
	var got mrrJSON
	runNovelJSON(t, path, &got, "mrr")
	if want := []string{"activeSubscriptions", "currency", "margin", "mrr", "nonRecurringSubscriptions", "otherCadence", "subscriptionsWithoutCost", "unclassifiedSubscriptions"}; !reflect.DeepEqual(keysOf(got.Totals), want) {
		t.Errorf("totals keys = %v, want %v", keysOf(got.Totals), want)
	}
	wantFields(t, "totals", got.Totals, map[string]any{"mrr": 20.0, "margin": 8.0, "currency": "USD", "activeSubscriptions": 1, "subscriptionsWithoutCost": 0})
	if want := []string{"margin", "mrr", "productId", "productName", "quantity"}; !reflect.DeepEqual(keysOf(got.ByProduct[0]), want) {
		t.Errorf("byProduct keys = %v, want %v", keysOf(got.ByProduct[0]), want)
	}
	terms, _ := got.Totals["otherCadence"].([]any)
	if want := []string{"amount", "billingTerm", "margin", "subscriptions"}; len(terms) != 1 || !reflect.DeepEqual(keysOf(terms[0].(map[string]any)), want) {
		t.Errorf("otherCadence = %+v, want keys %v", terms, want)
	}
	text := runNovelText(t, path, "mrr")
	if !strings.Contains(text, "MRR:    20.00 USD\nMargin: 8.00 USD\nActive subscriptions: 1\n") {
		t.Errorf("mrr text changed:\n%s", text)
	}
}

// A product average never mixes currencies: USD 100 next to JPY 15000 is no
// overage for either, and a real one inside one currency is still flagged.
func TestOverageNeverAveragesAcrossCurrencies(t *testing.T) {
	row := func(id, product string, charges float64, currency string) string {
		return fmt.Sprintf(`{"id":%q,"productId":%q,"companyId":"c","currentCharges":%v,"currencyCode":%q}`, id, product, charges, currency)
	}
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", moneyID(90),
		row(moneyID(1), "same-product", 100, "USD"),
		row(moneyID(2), "same-product", 15000, "JPY"),
		row(moneyID(3), "other-product", 10, "USD"),
		row(moneyID(4), "other-product", 10, "USD"),
		row(moneyID(5), "other-product", 40, "USD"))
	var flagged []overageRow
	runNovelJSON(t, path, &flagged, "overage")
	if len(flagged) != 1 || flagged[0].UsageSummaryID != moneyID(5) {
		t.Fatalf("overage = %+v, want only the USD 40 row", flagged)
	}
}
