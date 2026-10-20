// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: behavior tests for the StreamOne ION novel-command logic.

package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cliutil"
	"td-synnex-pp-cli/internal/config"
)

var testNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func day(n int) string { return testNow.AddDate(0, 0, n).Format(time.RFC3339) }

func boolp(b bool) *bool { return &b }

func testBook() *ionBook {
	return &ionBook{
		Customers: map[string]ionCustomer{
			"1": {Name: "accounts/9/customers/1", Organization: "SilverLeaf"},
			"2": {Name: "accounts/9/customers/2", Organization: "NovaSphere, Inc."},
		},
		Subs: []ionSub{
			{ID: "s1", CustomerID: "1", SubscriptionID: "v1", SubscriptionName: "M365 BS", Seats: optFloat{V: 10, Set: true}, Status: "active",
				RenewalDate: day(10), AutoRenew: boolp(true), BillingCycle: "monthly", Price: optFloat{V: 125, Set: true}, Cost: optFloat{V: 110, Set: true}, MSRP: optFloat{V: 125, Set: true},
				MfgPartNumber: "CFQ7TTC0LDPB:0001:P1Y:M", Currency: "USD", CCPProductID: "p", CCPSkuID: "k", CCPPlanID: "pl"},
			{ID: "s2", CustomerID: "1", SubscriptionID: "v2", SubscriptionName: "M365 BS", Seats: optFloat{V: 2, Set: true}, Status: "active",
				RenewalDate: day(200), AutoRenew: boolp(true), BillingCycle: "monthly", Price: optFloat{V: 25, Set: true}, Cost: optFloat{V: 22, Set: true},
				MfgPartNumber: "CFQ7TTC0LDPB:0001:P1M:M", Currency: "USD"},
			{ID: "s3", CustomerID: "2", SubscriptionID: "v3", SubscriptionName: "E3", Seats: optFloat{V: 5, Set: true}, Status: "active",
				RenewalDate: day(20), AutoRenew: boolp(false), BillingCycle: "annual", Price: optFloat{V: 1200, Set: true}, Cost: optFloat{V: 1080, Set: true}, MSRP: optFloat{V: 1440, Set: true},
				ProviderProductID: "CFQ7TTC0LFLX", ProviderSkuID: "0001", Currency: "USD"},
			{ID: "s4", CustomerID: "2", SubscriptionID: "v4", SubscriptionName: "Old", Seats: optFloat{V: 1, Set: true}, Status: "cancelled",
				RenewalDate: day(5), Price: optFloat{V: 10, Set: true}, Cost: optFloat{V: 8, Set: true}, Currency: "USD"},
			{ID: "s5", CustomerID: "2", SubscriptionID: "v5", SubscriptionName: "Forever", Seats: optFloat{V: 1, Set: true}, Status: "active",
				EndDate: "9999-12-31T00:00:00Z", Currency: "USD"},
		},
		Orders: []ionOrder{
			{OrderID: "o1", CustomerID: "2", Status: "ON_HOLD", CreateTime: day(-3), Items: []ionOrderItem{{Action: "CREATE", Quantity: optFloat{V: 3, Set: true}, SkuName: "Mailbox"}}},
			{OrderID: "o2", CustomerID: "1", Status: "ERROR", CreateTime: day(-2), UpdateTime: day(-2),
				Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "V2", Quantity: optFloat{V: 2, Set: true},
					Attributes: []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{{"operations", "updateSubscription"}, {"statusUpdate", "suspended"}}}}},
			{OrderID: "o3", CustomerID: "1", Status: "IN_PROGRESS", CreateTime: testNow.Add(-2 * time.Hour).Format(time.RFC3339)},
			{OrderID: "o4", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-1),
				Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "v1", Quantity: optFloat{V: 12, Set: true},
					Attributes: []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{{"operations", "updateSubscription"}}}}},
			{OrderID: "o5", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-90), Items: []ionOrderItem{{Action: "CREATE", Quantity: optFloat{V: 1, Set: true}}}},
		},
	}
}

func TestFlexFloatAcceptsStringsAndNumbers(t *testing.T) {
	var v struct {
		A flexFloat `json:"a"`
		B flexFloat `json:"b"`
		C flexFloat `json:"c"`
		D flexFloat `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"a":"12","b":3.5,"c":null,"d":""}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 12 || v.B != 3.5 || v.C != 0 || v.D != 0 {
		t.Fatalf("got %+v", v)
	}
}

func TestMonthlyFactor(t *testing.T) {
	cases := []struct {
		cycle string
		want  float64
	}{
		{"monthly", 1}, {"annual", 1.0 / 12}, {"P1Y", 1.0 / 12}, {"P3Y", 1.0 / 36},
		{"Quarterly", 1.0 / 3}, {"Semi-Annual", 1.0 / 6}, {"Triennially", 1.0 / 36}, {"BIENNIAL", 1.0 / 24},
	}
	for _, c := range cases {
		if got, ok := monthlyFactor(c.cycle); !ok || got != c.want {
			t.Errorf("monthlyFactor(%q)=%v,%v want %v", c.cycle, got, ok, c.want)
		}
	}
	// An unrecognized or missing cycle is unknown, never monthly (adversarial
	// round 14, P2: a triennial charge was reported per month). The term is
	// never consulted: it is the commitment length, not the billing period.
	for _, c := range []string{"", "Every4Years", "weird", "PAY_MONTHLY"} {
		if got, ok := monthlyFactor(c); ok {
			t.Errorf("monthlyFactor(%q)=%v,true want not ok", c, got)
		}
	}
}

func TestSKUKeyPrefersPartNumberThenProviderPair(t *testing.T) {
	b := testBook()
	if got := b.Subs[0].SKUKey(); got != "CFQ7TTC0LDPB:0001" {
		t.Errorf("part-number key = %q", got)
	}
	if got := b.Subs[2].SKUKey(); got != "CFQ7TTC0LFLX:0001" {
		t.Errorf("provider-pair key = %q", got)
	}
	if got := (ionSub{CCPSkuID: "abc"}).SKUKey(); got != "ABC" {
		t.Errorf("fallback key = %q", got)
	}
}

func pinNow(t *testing.T) {
	t.Helper()
	prev := nowFunc
	nowFunc = func() time.Time { return testNow }
	t.Cleanup(func() { nowFunc = prev })
}

func TestMonthlyMoneyPrefersRecentBillingMonthNormalizedByCycle(t *testing.T) {
	pinNow(t)
	monthly := ionSub{Price: optFloat{V: 999, Set: true}, Cost: optFloat{V: 999, Set: true}, BillingCycle: "monthly", Currency: "USD", BillingData: []ionBillingRow{
		{BillingMonth: "2026-08", CustomerCost: optFloat{V: 90, Set: true}, SellerCost: optFloat{V: 80, Set: true}},
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 100, Set: true}, SellerCost: optFloat{V: 85, Set: true}, MSRP: optFloat{V: 120, Set: true}},
	}}
	if m := monthly.MonthlyMoney(); m.Price != 100 || m.Cost != 85 || m.MSRP != 120 || m.Basis != "billing:2026-09" || m.Currency != "USD" {
		t.Fatalf("monthly billing basis got %+v", m)
	}
	annual := ionSub{BillingCycle: "annual", BillingData: []ionBillingRow{{BillingMonth: "2026-01", CustomerCost: optFloat{V: 1200, Set: true}, SellerCost: optFloat{V: 1080, Set: true}}}}
	if m := annual.MonthlyMoney(); m.Price != 100 || m.Cost != 90 || m.Basis != "billing:2025-02..2026-01" {
		t.Fatalf("annual charge billed in January must count 1/12 per month, got %+v", m)
	}
	// A later credit or proration in the same term adjusts the annual charge
	// instead of replacing it (adversarial round 10, P1: a one-seat credit made
	// a profitable annual E3 read as negative margin).
	credit := ionSub{BillingCycle: "annual", BillingData: []ionBillingRow{
		{BillingMonth: "2026-01", CustomerCost: optFloat{V: 10800, Set: true}, SellerCost: optFloat{V: 9000, Set: true}},
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: -432, Set: true}, SellerCost: optFloat{V: -360, Set: true}},
	}}
	if m := credit.MonthlyMoney(); m.Price != 864 || m.Cost != 720 || m.Basis != "billing:2025-10..2026-09" {
		t.Fatalf("annual charge plus later credit got %+v", m)
	}
	// With the subscription's own price and cost, annual billing uses them:
	// billed rows across a renewal (a monthly-to-annual switch, a co-termed
	// first term, a prior-term proration) are not one term's run-rate
	// (adversarial round 11, P1).
	switched := ionSub{BillingCycle: "annual", Price: optFloat{V: 10800, Set: true}, Cost: optFloat{V: 9000, Set: true}, BillingData: []ionBillingRow{
		{BillingMonth: "2026-08", CustomerCost: optFloat{V: 900, Set: true}, SellerCost: optFloat{V: 750, Set: true}},
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 10800, Set: true}, SellerCost: optFloat{V: 9000, Set: true}},
	}}
	if m := switched.MonthlyMoney(); m.Price != 900 || m.Cost != 750 || m.Basis != "subscription" {
		t.Fatalf("annual subscription with its own price got %+v", m)
	}
	stale := ionSub{Price: optFloat{V: 30, Set: true}, Cost: optFloat{V: 20, Set: true}, BillingCycle: "monthly", BillingData: []ionBillingRow{{BillingMonth: "2026-03", CustomerCost: optFloat{V: 500, Set: true}, SellerCost: optFloat{V: 400, Set: true}}}}
	if m := stale.MonthlyMoney(); m.Price != 30 || m.Cost != 20 || m.Basis != "subscription" {
		t.Fatalf("a billing month older than one cycle must fall back to subscription fields, got %+v", m)
	}
	noBilling := ionSub{Price: optFloat{V: 1200, Set: true}, Cost: optFloat{V: 1000, Set: true}, BillingCycle: "annual"}
	if m := noBilling.MonthlyMoney(); m.Price != 100 || m.Cost != 1000.0/12 || m.Basis != "subscription" {
		t.Fatalf("subscription basis got %+v", m)
	}
	// ION returns unpadded months; 2026-9 must beat 2026-08, not sort after 2026-10 as a string would.
	unpadded := ionSub{BillingCycle: "monthly", BillingData: []ionBillingRow{
		{BillingMonth: "2026-08", CustomerCost: optFloat{V: 1, Set: true}}, {BillingMonth: "2026-9", CustomerCost: optFloat{V: 7, Set: true}, SellerCost: optFloat{V: 5, Set: true}},
	}}
	if m := unpadded.MonthlyMoney(); m.Basis != "billing:2026-09" || m.Price != 7 {
		t.Fatalf("unpadded month got %+v", m)
	}
	// Billing rows carry their own currency, which wins over the subscription currency.
	hrk := ionSub{Currency: "USD", BillingCycle: "monthly", BillingData: []ionBillingRow{{BillingMonth: "2026-09", CustomerCost: optFloat{V: 50, Set: true}, Currency: "HRK"}}}
	if m := hrk.MonthlyMoney(); m.Currency != "HRK" || m.Price != 50 {
		t.Fatalf("billing currency got %+v", m)
	}
	// Mixed currencies in one month are never summed; fall back to the subscription fields.
	mixed := ionSub{Price: optFloat{V: 9, Set: true}, Currency: "USD", BillingCycle: "monthly", BillingData: []ionBillingRow{
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 50, Set: true}, Currency: "HRK"}, {BillingMonth: "2026-09", CustomerCost: optFloat{V: 5, Set: true}, Currency: "EUR"},
	}}
	if m := mixed.MonthlyMoney(); m.Basis != "subscription" || m.Currency != "USD" || m.Price != 9 {
		t.Fatalf("mixed currency got %+v", m)
	}
}

func TestFlexFloatRejectsNonNumbers(t *testing.T) {
	var v struct {
		N flexFloat `json:"n"`
	}
	for _, bad := range []string{`{"n":"n/a"}`, `{"n":"NaN"}`, `{"n":"Inf"}`} {
		if err := json.Unmarshal([]byte(bad), &v); err == nil {
			t.Fatalf("%s decoded without error", bad)
		}
	}
	for _, ok := range []string{`{"n":""}`, `{"n":null}`, `{"n":"12"}`, `{"n":3.5}`} {
		if err := json.Unmarshal([]byte(ok), &v); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
}

func TestRenewalsWithinWindowSkipsInactiveAndOpenEnded(t *testing.T) {
	rows := renewalsWithin(testBook(), testNow, 30*24*time.Hour, "", "")
	if len(rows) != 2 || rows[0].SubscriptionID != "s1" || rows[1].SubscriptionID != "s3" {
		t.Fatalf("got %+v", rows)
	}
	if rows[0].DaysLeft != 10 || rows[1].AutoRenew != "off" {
		t.Fatalf("days/auto-renew wrong: %+v", rows)
	}
	off := renewalsWithin(testBook(), testNow, 30*24*time.Hour, "off", "")
	if len(off) != 1 || off[0].SubscriptionID != "s3" {
		t.Fatalf("auto-renew off filter got %+v", off)
	}
	if got := renewalsWithin(testBook(), testNow, 24*time.Hour, "", ""); len(got) != 0 {
		t.Fatalf("1-day window must be empty, got %+v", got)
	}
	if got := renewalsWithin(testBook(), testNow, 30*24*time.Hour, "", "2"); len(got) != 1 || got[0].SubscriptionID != "s3" {
		t.Fatalf("customer filter got %+v", got)
	}
}

func TestBookMarginByCustomerAndFlags(t *testing.T) {
	rows := bookMargin(testBook(), "customer")
	if len(rows) != 2 {
		t.Fatalf("want 2 customers, got %+v", rows)
	}
	byKey := map[string]marginRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	silver := byKey["1"]
	if silver.MonthlyPrice != 150 || silver.MonthlyCost != 132 || silver.Margin != 18 || silver.Seats != 12 {
		t.Fatalf("silverleaf rollup wrong: %+v", silver)
	}
	nova := byKey["2"] // annual 1200/12=100 price, 1080/12=90 cost, msrp 120 -> below_msrp; cancelled s4 excluded
	if nova.MonthlyPrice != 100 || nova.MonthlyCost != 90 || nova.Subscriptions != 2 {
		t.Fatalf("nova rollup wrong: %+v", nova)
	}
	// s5 ("Forever") reports no billing cycle, price or cost, so the group is
	// flagged price_incomplete and cost_incomplete.
	if strings.Join(nova.Flags, ",") != "below_msrp,price_incomplete,cost_incomplete" || nova.UnknownCost != 1 || nova.UnknownPrice != 1 {
		t.Fatalf("nova flags = %v (unknown-cost subs %d)", nova.Flags, nova.UnknownCost)
	}
	b := testBook()
	b.Subs = append(b.Subs, ionSub{ID: "s6", CustomerID: "1", Seats: optFloat{V: 1, Set: true}, Status: "active", BillingCycle: "monthly", Price: optFloat{V: 5, Set: true}, Cost: optFloat{V: 9, Set: true}, Currency: "USD"})
	for _, r := range bookMargin(b, "customer") {
		// s2 and s6 send no MSRP, so the group is also msrp_incomplete.
		if r.Key == "1" && (r.NegativeSubs != 1 || r.Margin != 14 || strings.Join(r.Flags, ",") != "msrp_incomplete,has_negative_subs" || r.UnknownMSRP != 2) {
			t.Fatalf("a loss-making subscription inside a profitable customer must surface: %+v", r)
		}
	}
	sku := bookMargin(testBook(), "sku")
	for _, r := range sku {
		if r.Key == "CFQ7TTC0LDPB:0001" && r.Subscriptions != 2 {
			t.Fatalf("sku grouping should merge s1+s2: %+v", r)
		}
	}
}

func TestStuckOrders(t *testing.T) {
	rows := stuckOrders(testBook(), testNow, 24*time.Hour, 7*24*time.Hour)
	ids := map[string]stuckOrderRow{}
	for _, r := range rows {
		ids[r.OrderID] = r
	}
	if len(rows) != 2 || !ids["o1"].Cancellable || ids["o2"].Cancellable || ids["o2"].Reason != "errored recently" {
		t.Fatalf("got %+v", rows)
	}
	if _, ok := ids["o3"]; ok {
		t.Fatalf("2h-old order must be under a 24h threshold")
	}
	if got := stuckOrders(testBook(), testNow, 24*time.Hour, time.Hour); len(got) != 1 {
		t.Fatalf("1h error window should drop o2, got %+v", got)
	}
}

func TestSubscriptionChangesDecodeAndJoin(t *testing.T) {
	rows := subscriptionChanges(testBook(), testNow, 7*24*time.Hour, "", false)
	if len(rows) != 2 {
		t.Fatalf("want 2 changes (o4 seats, o1 new); the ERROR order o2 is excluded by default, got %+v", rows)
	}
	if rows[0].OrderID != "o4" || rows[0].Change != "seats" || rows[0].Value != "12" || rows[0].SubscriptionID != "s1" || !rows[0].Applied {
		t.Fatalf("newest change wrong: %+v", rows[0])
	}
	if rows[1].Change != "new" || rows[1].Applied {
		t.Fatalf("ON_HOLD order must not count as applied: %+v", rows[1])
	}
	all := subscriptionChanges(testBook(), testNow, 7*24*time.Hour, "", true)
	if len(all) != 3 || all[1].Change != "status" || all[1].Value != "suspended" || all[1].SubscriptionID != "s2" || all[1].Applied {
		t.Fatalf("--include-failed must add the ERROR order, joined case-insensitively: %+v", all)
	}
	if got := subscriptionChanges(testBook(), testNow, 7*24*time.Hour, "2", false); len(got) != 1 || got[0].Change != "new" {
		t.Fatalf("customer filter got %+v", got)
	}
}

func TestDecodeChangesCombos(t *testing.T) {
	it := ionOrderItem{Action: "UPDATE", SkuID: "SKU2", PlanID: "P2", Quantity: optFloat{V: 4, Set: true}}
	it.Attributes = append(it.Attributes, struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}{"operations", "changeSubscription"})
	got := decodeChanges(it)
	if len(got) != 1 || got[0][0] != "tier" || got[0][1] != "SKU2 / P2" {
		t.Fatalf("tier decode got %v", got)
	}
}

func TestNormalizeCompany(t *testing.T) {
	cases := map[string]string{"SilverLeaf, Inc.": "silverleaf", "NovaSphere Inc": "novasphere", "Contoso  Dental Ltd": "contoso dental", "Inc": "inc"}
	for in, want := range cases {
		if got := normalizeCompany(in); got != want {
			t.Errorf("normalizeCompany(%q)=%q want %q", in, got, want)
		}
	}
}

func TestParsePositionFileCSVAndJSONWithMapping(t *testing.T) {
	csvRows, _, err := parsePositionFile("pax8.csv", []byte("company,sku,qty\n\"A, Inc\",cfq:0001,3\n"), map[string]string{"company": "customer_name", "sku": "sku_key", "qty": "seats"})
	if err != nil || len(csvRows) != 1 {
		t.Fatalf("csv parse: %v %+v", err, csvRows)
	}
	if r := csvRows[0]; r.Distributor != "pax8" || r.CustomerName != "A, Inc" || r.SKUKey != "CFQ:0001" || r.Seats != 3 {
		t.Fatalf("csv row wrong: %+v", r)
	}
	jsonRows, _, err := parsePositionFile("x.json", []byte(`[{"distributor":"Sherweb","customer_name":"B","sku_key":"k","seats":2}]`), nil)
	if err != nil || len(jsonRows) != 1 || jsonRows[0].Distributor != "sherweb" || jsonRows[0].Seats != 2 {
		t.Fatalf("json parse: %v %+v", err, jsonRows)
	}
	if _, _, err := parsePositionFile("bad.json", []byte(`[{`), nil); err == nil {
		t.Fatal("bad JSON must error")
	}
}

func TestTrueupJoinsFlagsAndUnmatched(t *testing.T) {
	ion := ionPositions(testBook(), false)
	inputs := append(ion,
		positionRow{Distributor: "pax8", SeatsKnown: true, CustomerName: "SilverLeaf Inc", SKUKey: "CFQ7TTC0LDPB:0001", Seats: 4, MonthlyCost: 40, CostKnown: true},
		positionRow{Distributor: "pax8", SeatsKnown: true, CustomerName: "Nobody", SKUKey: "X:1", Seats: 1},
		positionRow{Distributor: "pax8", SeatsKnown: true, CustomerName: "NovaSphere", SKUKey: "", Seats: 1},
		positionRow{Distributor: "psa", SeatsKnown: true, CustomerName: "silverleaf", SKUKey: "CFQ7TTC0LDPB:0001", Seats: 16},
	)
	v := trueup(inputs, true, false)
	var silver *trueupRow
	for i := range v.Rows {
		if v.Rows[i].SKUKey == "CFQ7TTC0LDPB:0001" {
			silver = &v.Rows[i]
		}
	}
	if silver == nil || silver.TotalSeats != 16 || silver.BilledSeats == nil || *silver.BilledSeats != 16 || silver.MatchedOn != "name" {
		t.Fatalf("silverleaf row wrong: %+v", silver)
	}
	if silver.UnitCostBySrc["td-synnex"] != 11 || silver.UnitCostBySrc["pax8"] != 10 {
		t.Fatalf("unit cost must be cost-weighted per source: %+v", silver.UnitCostBySrc)
	}
	want := map[string]bool{"duplicate_in_td-synnex": true, "multi_source": true}
	if len(silver.Flags) != 2 || !want[silver.Flags[0]] || !want[silver.Flags[1]] {
		t.Fatalf("flags = %v (billed equals total, so no billed_mismatch)", silver.Flags)
	}
	// s5 has no SKU identity on the TD SYNNEX side, so it is listed too.
	if len(v.Unmatched) != 3 {
		t.Fatalf("want 3 unmatched (unknown customer, missing sku twice), got %+v", v.Unmatched)
	}
	ionOnly := trueup(ion, false, false)
	if len(ionOnly.Rows) != 1 || ionOnly.Rows[0].Flags[0] != "duplicate_in_td-synnex" {
		t.Fatalf("ION-only mode should report only the duplicate SKU: %+v", ionOnly.Rows)
	}
}

func TestTrueupNeverMergesTwoCustomersOfOneSourceByName(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Contoso", SKUKey: "A:1", Seats: 1},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Contoso", SKUKey: "A:1", Seats: 1},
	}
	v := trueup(inputs, false, false)
	if len(v.Rows) != 0 {
		t.Fatalf("two distinct customers must not become a duplicate: %+v", v.Rows)
	}
}

func TestBuildSubscriptionUpdate(t *testing.T) {
	s := testBook().Subs[0]
	body, err := buildSubscriptionUpdate(s, subSetChange{Seats: 12, SeatsSet: true, Renewal: "cancel_at_renewal"})
	if err != nil {
		t.Fatal(err)
	}
	item := body["orderItems"].([]any)[0].(map[string]any)
	if item["action"] != "UPDATE" || item["quantity"] != 12 || item["resourceId"] != "v1" || item["skuId"] != "k" {
		t.Fatalf("item wrong: %+v", item)
	}
	attrs := item["attributes"].([]map[string]string)
	if len(attrs) != 2 || attrs[0]["value"] != "updateSubscription" || attrs[1]["value"] != "cancel_at_renewal" {
		t.Fatalf("attrs wrong: %+v", attrs)
	}
	up, err := buildSubscriptionUpdate(s, subSetChange{UpgradeSKU: "k2"})
	if err != nil {
		t.Fatal(err)
	}
	upItem := up["orderItems"].([]any)[0].(map[string]any)
	if upItem["skuId"] != "k2" || upItem["quantity"] != 10 || upItem["attributes"].([]map[string]string)[0]["value"] != "changeSubscription" {
		t.Fatalf("upgrade item wrong: %+v", upItem)
	}
	for _, bad := range []subSetChange{{}, {Status: "suspended", SeatsSet: true, Seats: 2}, {Status: "paused"}, {Renewal: "maybe"}, {SeatsSet: true, Seats: 0}} {
		if _, err := buildSubscriptionUpdate(s, bad); err == nil {
			t.Errorf("expected error for %+v", bad)
		}
	}
	if _, err := buildSubscriptionUpdate(ionSub{ID: "x", Seats: optFloat{V: 1, Set: true}}, subSetChange{SeatsSet: true, Seats: 2}); err == nil {
		t.Error("missing product/sku/plan/resource IDs must error")
	}
}

func TestIONOAuthURL(t *testing.T) {
	cases := map[string]string{
		"https://ion.tdsynnex.com/api/v3/accounts/{accountId}": "https://ion.tdsynnex.com/oauth/token",
		"http://127.0.0.1:9/api/v3/accounts/{accountId}":       "http://127.0.0.1:9/oauth/token",
		"": "https://ion.tdsynnex.com/oauth/token",
	}
	for in, want := range cases {
		if got := ionOAuthURL(in, "token"); got != want {
			t.Errorf("ionOAuthURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSubscriptionsSetHonoursAccountDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	flags := &rootFlags{}
	root := newRootCmd(flags)
	set, _, err := root.Find([]string{"subscriptions", "set"})
	if err != nil || set.Name() != "set" {
		t.Fatalf("subscriptions set not registered: %v", err)
	}
	if set.Annotations["td:account-wrapped"] != "true" {
		t.Fatal("subscriptions set does not fill <accountId> from TD_SYNNEX_ACCOUNT_ID")
	}
	if n := strings.Count(set.Long, "TD_SYNNEX_ACCOUNT_ID"); n != 1 {
		t.Fatalf("account hint appears %d times in Long", n)
	}
	got := withDefaultAccountID(flags, []string{"108536", "1608355"}, 3)
	if len(got) != 3 || got[0] != "10505" {
		t.Fatalf("withDefaultAccountID got %v", got)
	}
}

func TestSetTokenKeepsStoredRefreshToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "")
	flags := &rootFlags{}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("", "", "access-1", "refresh-1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("access-2\n"))
	cmd.SetOut(io.Discard)
	if err := ionSetTokenRunE(flags)(cmd, nil); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "access-2" || cfg.RefreshToken != "refresh-1" {
		t.Fatalf("after set-token: access=%q refresh kept=%v", cfg.AccessToken, cfg.RefreshToken == "refresh-1")
	}
}

// TestAuthKeepsRefreshTokenShadowedByConfig: token fields in config.toml make
// Load skip credentials.toml. set-token must keep the refresh token stored
// there and mint must use it (Codex round 3, P1; adversarial round 4, P2), and
// both must refuse a refresh_token in config.toml rather than save a possibly
// spent one over the newer stored one (Codex round 4, P1), or a stored file
// with unsafe permissions (Codex round 4, P1).
func TestAuthKeepsRefreshTokenShadowedByConfig(t *testing.T) {
	setup := func(t *testing.T, cfgBody string, credMode os.FileMode) (string, *cobra.Command) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "")
		t.Setenv("TD_SYNNEX_REFRESH_TOKEN", "")
		creds, err := cliutil.CredentialsFilePath()
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		for path, body := range map[string]string{creds: "refresh_token = \"refresh-R1\"\n", cfg.Path: cfgBody} {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(creds, credMode); err != nil {
			t.Fatal(err)
		}
		cmd := &cobra.Command{}
		cmd.SetIn(strings.NewReader("access-2\n"))
		cmd.SetOut(io.Discard)
		return creds, cmd
	}
	t.Run("legacy access token: kept", func(t *testing.T) {
		creds, cmd := setup(t, "access_token = \"legacy\"\n", 0o600)
		if err := ionSetTokenRunE(&rootFlags{})(cmd, nil); err != nil {
			t.Fatal(err)
		}
		if got, err := tomlRefreshToken(creds, true); err != nil || got != "refresh-R1" {
			t.Fatalf("set-token dropped the stored refresh token: %q %v", got, err)
		}
	})
	for name, tc := range map[string]struct {
		cfg  string
		mode os.FileMode
		want string
	}{
		"config refresh_token": {"access_token = \"legacy\"\nrefresh_token = \"refresh-R0\"\n", 0o600, "may already be spent"},
		"unsafe credentials":   {"access_token = \"legacy\"\n", 0o644, "refusing to replace"},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.mode&0o077 != 0 && runtime.GOOS == "windows" {
				t.Skip("Windows has no mode bits: credential safety there is an ACL check (creds_perms_eval tests)")
			}
			creds, cmd := setup(t, tc.cfg, tc.mode)
			before, _ := os.ReadFile(creds)
			if err := ionSetTokenRunE(&rootFlags{})(cmd, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("set-token: err=%v", err)
			}
			if err := runIONMint(cmd, &rootFlags{}, "", false, 0, false, creds); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("auth mint: err=%v", err)
			}
			if after, _ := os.ReadFile(creds); string(after) != string(before) {
				t.Fatalf("credentials.toml changed: %q", after)
			}
		})
	}
}

func TestSubscriptionUpdateRefusesZeroQuantity(t *testing.T) {
	s := ionSub{ID: "1", CCPProductID: "p", CCPSkuID: "s", CCPPlanID: "pl", SubscriptionID: "v", Seats: optFloat{V: 0, Set: true}}
	if _, err := buildSubscriptionUpdate(s, subSetChange{Renewal: "auto_on"}); err == nil {
		t.Fatal("renewal-only order on a 0-seat read must be refused")
	}
	if _, err := buildSubscriptionUpdate(s, subSetChange{Renewal: "auto_on", Seats: 3, SeatsSet: true}); err != nil {
		t.Fatalf("explicit --seats must still work: %v", err)
	}
}

func TestDecodeChangesKeepsSeatsInCombinedUpdate(t *testing.T) {
	var it ionOrderItem
	if err := json.Unmarshal([]byte(`{"action":"UPDATE","quantity":12,"attributes":[{"name":"operations","value":"updateSubscription"},{"name":"renewalSetting","value":"auto_on"}]}`), &it); err != nil {
		t.Fatal(err)
	}
	got := decodeChanges(it)
	if len(got) != 2 || got[0] != [2]string{"seats", "12"} || got[1] != [2]string{"renewal", "auto_on"} {
		t.Fatalf("combined seats+renewal order decoded as %v", got)
	}
	var suspend ionOrderItem
	_ = json.Unmarshal([]byte(`{"action":"UPDATE","quantity":12,"attributes":[{"name":"operations","value":"updateSubscription"},{"name":"statusUpdate","value":"suspended"}]}`), &suspend)
	if got := decodeChanges(suspend); len(got) != 1 || got[0][0] != "status" {
		t.Fatalf("status order decoded as %v", got)
	}
}

func TestRenewalsIncludeDateOnlyRenewalToday(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{
		{ID: "today", CustomerID: "1", Status: "active", RenewalDate: testNow.Format("2006-01-02")},
	}}
	if rows := renewalsWithin(b, testNow, 7*24*time.Hour, "", ""); len(rows) != 1 || rows[0].DaysLeft != 0 {
		t.Fatalf("renewal due today got %+v", rows)
	}
}

func TestTrueupNeverChainsTwoIONCustomers(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 5},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Beta", SKUKey: "K", Seats: 3},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Acme", TenantID: "T", SKUKey: "K", Seats: 1},
		{Distributor: "sherweb", SeatsKnown: true, CustomerName: "Beta", TenantID: "T", SKUKey: "K", Seats: 1},
	}
	view := trueup(inputs, true, false)
	for _, r := range view.Rows {
		if r.SeatsBySource["td-synnex"] > 5 {
			t.Fatalf("two TD SYNNEX customers merged through external rows: %+v", r)
		}
	}
}

func TestParsePositionFileNullsBOMThousandsAndBadSeats(t *testing.T) {
	rows, _, err := parsePositionFile("pax8.json", []byte(`[{"customer_name":"A","tenant_id":null,"customer_domain":null,"customer_id":1234567,"sku_key":"k","seats":2}]`), nil)
	if err != nil || len(rows) != 1 || rows[0].TenantID != "" || rows[0].CustomerDomain != "" || rows[0].CustomerID != "1234567" {
		t.Fatalf("JSON nulls/numbers: %v %+v", err, rows)
	}
	data := []byte("\xef\xbb\xbfcustomer_name,sku_key,seats\nA,k,\"1,000\"\nB,k,n/a\nC,k,\n")
	rows, bad, err := parsePositionFile("pax8.csv", data, nil)
	if err != nil || len(rows) != 3 || len(bad) != 0 || rows[0].CustomerName != "A" || rows[0].Seats != 1000 || !rows[0].SeatsKnown {
		t.Fatalf("BOM/thousands: %v %+v %+v", err, rows, bad)
	}
	// An unreadable or missing seat count is unknown, never 0, and the row is kept.
	if rows[1].SeatsKnown || rows[1].seatsProblem != `invalid seats "n/a"` || rows[2].SeatsKnown || rows[2].seatsProblem != "missing seats" {
		t.Fatalf("bad seat counts must be unknown with a reason, got %+v", rows)
	}
}

func TestCheckPositionFileRefusesNonExports(t *testing.T) {
	dir := t.TempDir()
	ok := dir + "/pax8.csv"
	if err := os.WriteFile(ok, []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkPositionFile(ok); err != nil {
		t.Fatalf("a plain export must pass: %v", err)
	}
	for _, name := range []string{"id_rsa", ".env.csv", "notes.txt"} {
		p := dir + "/" + name
		_ = os.WriteFile(p, []byte("a\n"), 0o600)
		if err := checkPositionFile(p); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	hidden := dir + "/.secrets"
	_ = os.MkdirAll(hidden, 0o700)
	_ = os.WriteFile(hidden+"/k.csv", []byte("a\n"), 0o600)
	if err := checkPositionFile(hidden + "/k.csv"); err == nil {
		t.Fatal("a file in a hidden directory must be refused")
	}
}

// Guards the resource_paths.go and tail.go hand-fixes: a regenerate that drops
// them leaves these empty, and the build and other tests stay green.
func TestResourcePathHandfixesPresent(t *testing.T) {
	if len(resourceReadPaths) == 0 || len(resourceDetailPaths) == 0 || len(resourceWritePaths) == 0 || len(resourceReadConfigs) == 0 {
		t.Fatal("resource path maps are empty: re-apply fix_resource_paths.py")
	}
	if len(tailKnownResources()) == 0 {
		t.Fatal("tailKnownResources is empty: re-apply fix_tail.py")
	}
	got, err := resourceDetailPath("customers", "108536")
	if err != nil || got != "/accounts/{accountId}/customers/108536" {
		t.Fatalf("resourceDetailPath substituted the wrong placeholder: %q %v", got, err)
	}
	if c := resourceReadConfigFor("subscriptions"); c.paginationType != "offset" || c.limitParam != "pagination.limit" {
		t.Fatalf("subscriptions read config lacks offset paging: %+v", c)
	}
	if !resourceSupportsPagination("subscriptions") {
		t.Fatal("sync.go subscriptions pagination hand-fix missing: re-apply fix_sync.py")
	}
}

func TestRedactSecretsJSON(t *testing.T) {
	in := `{"orderItems":[{"attributes":[{"name":"password","value":"Hunter2"},{"name":"domainName","value":"contoso.onmicrosoft.com"}]}],"userCredentials":{"userName":"admin","password":"P@ss"},"name":"accounts/1/customers/2"}`
	out := string(redactSecretsJSON(json.RawMessage(in)))
	if strings.Contains(out, "Hunter2") || strings.Contains(out, "P@ss") {
		t.Fatalf("secret survived redaction: %s", out)
	}
	if !strings.Contains(out, "contoso.onmicrosoft.com") || !strings.Contains(out, "accounts/1/customers/2") || !strings.Contains(out, `"userName":"admin"`) {
		t.Fatalf("redaction removed non-secret data: %s", out)
	}
}

func TestTrueupNeverJoinsTwoTenantsThroughAName(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 5},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Acme", TenantID: "tenant-a", SKUKey: "K", Seats: 4},
		{Distributor: "sherweb", SeatsKnown: true, CustomerName: "Acme", TenantID: "tenant-b", SKUKey: "K", Seats: 7},
	}
	view := trueup(inputs, true, false)
	for _, r := range view.Rows {
		if r.TotalSeats > 9 {
			t.Fatalf("rows for two tenants merged into one position: %+v", r)
		}
	}
	found := false
	for _, u := range view.Unmatched {
		if u["distributor"] == "sherweb" && strings.HasPrefix(u["reason"].(string), "ambiguous") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the conflicting sherweb row must be listed as ambiguous, got %+v", view.Unmatched)
	}
}

func TestTrueupAmbiguousExternalRowIsUnmatched(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 5},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Beta", SKUKey: "K", Seats: 3},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Acme", CustomerDomain: "shared.example", SKUKey: "K", Seats: 1},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Beta", CustomerDomain: "shared.example", SKUKey: "K", Seats: 2},
	}
	view := trueup(inputs, true, false)
	for _, r := range view.Rows {
		if r.Customer == "Acme" && r.SeatsBySource["pax8"] > 1 {
			t.Fatalf("Beta's pax8 row was attributed to Acme: %+v", r)
		}
	}
}

func TestChangesDropUnchangedSeatsInCombinedOrders(t *testing.T) {
	mk := func(id string, day int, qty float64, attrs string) ionOrder {
		var it ionOrderItem
		_ = json.Unmarshal([]byte(`{"action":"UPDATE","resourceId":"v1","quantity":`+strconv.FormatFloat(qty, 'f', -1, 64)+`,"attributes":[{"name":"operations","value":"updateSubscription"}`+attrs+`]}`), &it)
		return ionOrder{OrderID: id, CustomerID: "1", Status: "COMPLETED", CreateTime: testNow.AddDate(0, 0, day).Format(time.RFC3339), Items: []ionOrderItem{it}}
	}
	b := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{{ID: "s1", SubscriptionID: "v1", Seats: optFloat{V: 12, Set: true}}}, Orders: []ionOrder{
		mk("o1", -20, 10, ""),
		mk("o2", -10, 10, `,{"name":"renewalSetting","value":"auto_on"}`),
		mk("o3", -5, 12, `,{"name":"renewalSetting","value":"cancel_at_renewal"}`),
	}}
	rows := subscriptionChanges(b, testNow, 30*24*time.Hour, "", false)
	got := map[string][]string{}
	for _, r := range rows {
		v := r.Change + "=" + r.Value
		if r.PreviousSeats != nil {
			v += " was " + strconv.FormatFloat(*r.PreviousSeats, 'f', -1, 64)
		}
		got[r.OrderID] = append(got[r.OrderID], v)
	}
	if strings.Join(got["o2"], ",") != "renewal=auto_on" {
		t.Fatalf("o2 resent 10 seats with a renewal change; want no seats row, got %v", got["o2"])
	}
	if strings.Join(got["o3"], ",") != "seats=12 was 10,renewal=cancel_at_renewal" {
		t.Fatalf("o3 changed 10 to 12; got %v", got["o3"])
	}
	if strings.Join(got["o1"], ",") != "seats=10" {
		t.Fatalf("a seats-only order keeps its row; got %v", got["o1"])
	}
}

func TestSavedTokenIsNotShadowedAndEndpointNotPersisted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	// A legacy file-stored synnex_access_token used to outrank every saved token.
	if err := os.WriteFile(cfg.Path, []byte("synnex_access_token = \"stale\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pre, _ := config.Load(""); pre.AuthHeader() != "Bearer stale" {
		t.Fatalf("fixture did not load the stale file token: %q", pre.AuthHeader())
	}
	t.Setenv("TD_SYNNEX_BASE_URL", "http://127.0.0.1:9/api/v3")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "777")
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("fresh\n"))
	cmd.SetOut(io.Discard)
	if err := ionSetTokenRunE(&rootFlags{})(cmd, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TD_SYNNEX_BASE_URL", "")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "")
	cfg, err = config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "Bearer fresh" {
		t.Fatalf("a file-stored token still shadows the saved one: %q", got)
	}
	if cfg.BaseURL != "https://ion.tdsynnex.com/api/v3" || cfg.TemplateVars["accountId"] != "" {
		t.Fatalf("this run's env endpoint was persisted: base_url=%q accountId=%q", cfg.BaseURL, cfg.TemplateVars["accountId"])
	}
}

func TestTokenHostRequiresHTTPSForION(t *testing.T) {
	t.Setenv("TD_SYNNEX_BASE_URL", "http://ion.tdsynnex.com/api/v3")
	if err := ionTokenHostAllowed("http://ion.tdsynnex.com/oauth/token", "cfg"); err == nil {
		t.Fatal("plain http to ion.tdsynnex.com must be refused even with TD_SYNNEX_BASE_URL set")
	}
	if err := ionTokenHostAllowed("https://ion.tdsynnex.com/oauth/token", "cfg"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TD_SYNNEX_BASE_URL", "http://127.0.0.1:9/api/v3")
	if err := ionTokenHostAllowed("http://127.0.0.1:9/oauth/token", "cfg"); err != nil {
		t.Fatalf("an explicit test server must stay allowed: %v", err)
	}
}

func TestTrueupDuplicateIONNamesMakeExternalRowAmbiguous(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 5},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Acme", SKUKey: "K", Seats: 3},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Acme", SKUKey: "K", Seats: 4},
	}
	view := trueup(inputs, true, false)
	for _, r := range view.Rows {
		if r.SeatsBySource["pax8"] > 0 {
			t.Fatalf("pax8 row silently joined one of two same-named customers: %+v", r)
		}
	}
	if len(view.Unmatched) != 1 || !strings.HasPrefix(view.Unmatched[0]["reason"].(string), "ambiguous") {
		t.Fatalf("want one ambiguous unmatched row, got %+v", view.Unmatched)
	}
}

func TestTrueupAmbiguousRowDoesNotSteerLaterRows(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 5},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Beta", SKUKey: "K", Seats: 3},
		{Distributor: "sherweb", SeatsKnown: true, CustomerName: "Acme", TenantID: "T", SKUKey: "K", Seats: 1},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Beta", CustomerID: "PB", TenantID: "T", SKUKey: "K", Seats: 2},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Acme", CustomerID: "PA", TenantID: "T", SKUKey: "K", Seats: 6},
	}
	view := trueup(inputs, true, false)
	var acme *trueupRow
	for i := range view.Rows {
		if view.Rows[i].Customer == "Acme" {
			acme = &view.Rows[i]
		}
	}
	if acme == nil || acme.SeatsBySource["pax8"] != 6 {
		t.Fatalf("the valid pax8 Acme row must join Acme despite the earlier ambiguous Beta row: %+v", acme)
	}
}

func TestTrueupDomainFormsJoinAndFlag(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme Inc", SKUKey: "K", Seats: 5},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Acme", CustomerDomain: "https://www.Acme.com/", SKUKey: "K", Seats: 1},
		{Distributor: "sherweb", SeatsKnown: true, CustomerName: "Acme Inc.", CustomerDomain: "acme.onmicrosoft.com", SKUKey: "K", Seats: 2},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "ACME", CustomerDomain: "acme-corp.com", SKUKey: "K", Seats: 8},
	}
	view := trueup(inputs, true, false)
	if len(view.Unmatched) != 0 || len(view.Rows) != 1 {
		t.Fatalf("domain spellings must not split one customer: rows=%+v unmatched=%+v", view.Rows, view.Unmatched)
	}
	r := view.Rows[0]
	if r.TotalSeats != 8 || r.BilledSeats == nil || *r.BilledSeats != 8 {
		t.Fatalf("seats wrong: %+v", r)
	}
	flagged := false
	for _, f := range r.Flags {
		if f == "domain_mismatch" {
			flagged = true
		}
	}
	if !flagged {
		t.Fatalf("two custom domains (acme.com, acme-corp.com) must be flagged: %v", r.Flags)
	}
}

func TestTrueupUnitCostCountsFreeSeats(t *testing.T) {
	rows, _, err := parsePositionFile("pax8.csv", []byte("customer_name,sku_key,seats,monthly_cost\nA,K,10,100\nA,K,5,0\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := append([]positionRow{{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "A", SKUKey: "K", Seats: 15}}, rows...)
	view := trueup(inputs, true, false)
	if len(view.Rows) != 1 || view.Rows[0].UnitCostBySrc["pax8"] != 6.67 {
		t.Fatalf("10 seats at 100 plus 5 free seats is 6.67 per seat: %+v", view.Rows)
	}
}

func TestRedactSecretsJSONKeepsNonSecretNames(t *testing.T) {
	in := `{"compassId":"c1","nextPageToken":"p2","adminPassword":"x1","clientSecret":"x2","access_token":"x3","attributes":[{"name":"tenantPassword","value":"x4"}]}`
	out := string(redactSecretsJSON(json.RawMessage(in)))
	for _, keep := range []string{`"compassId":"c1"`, `"nextPageToken":"p2"`} {
		if !strings.Contains(out, keep) {
			t.Fatalf("over-redacted %s: %s", keep, out)
		}
	}
	for _, gone := range []string{"x1", "x2", "x3", "x4"} {
		if strings.Contains(out, `"`+gone+`"`) {
			t.Fatalf("secret %s survived: %s", gone, out)
		}
	}
}

func TestStoreScopeKeepsProductionKeyAndSplitsOtherHosts(t *testing.T) {
	prod := &config.Config{BaseURL: "https://ion.tdsynnex.com/api/v3", TemplateVars: map[string]string{"accountId": "10505"}}
	test := &config.Config{BaseURL: "http://127.0.0.1:38917/api/v3", TemplateVars: map[string]string{"accountId": "10505"}}
	if got := config.StoreScopeIdentity(prod); got != "ion-account:10505" {
		t.Fatalf("production key changed: %q", got)
	}
	if got := config.StoreScopeIdentity(test); got != `ion-account@"127.0.0.1:38917":"10505"` {
		t.Fatalf("test host key: %q", got)
	}
	// An account ID shaped like a test key never reaches the test host's store.
	odd := &config.Config{BaseURL: "https://ion.tdsynnex.com/api/v3", TemplateVars: map[string]string{"accountId": `127.0.0.1:38917:10505`}}
	if config.StoreScopeIdentity(odd) == config.StoreScopeIdentity(test) {
		t.Fatal("a production account ID collided with a test host key")
	}
}

func TestUnitCostIgnoresMissingIONCost(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{"1": {Name: "accounts/9/customers/1", Organization: "A"}}, Subs: []ionSub{
		{ID: "s1", CustomerID: "1", CCPSkuID: "K", Seats: optFloat{V: 10, Set: true}, Status: "active", BillingCycle: "monthly", Cost: optFloat{V: 100, Set: true}},
		{ID: "s2", CustomerID: "1", CCPSkuID: "K", Seats: optFloat{V: 5, Set: true}, Status: "active", BillingCycle: "monthly"}, // ION sent no cost
	}}
	view := trueup(ionPositions(b, false), true, false)
	if len(view.Rows) != 1 || view.Rows[0].UnitCostBySrc["td-synnex"] != 10 {
		t.Fatalf("missing ION costs must not count as free seats: %+v", view.Rows)
	}
}

func TestParsePositionFileKeepsLargeNumericIDs(t *testing.T) {
	rows, _, err := parsePositionFile("pax8.json", []byte(`[{"customer_name":"A","customer_id":9007199254740993,"sku_key":"K","seats":1}]`), nil)
	if err != nil || len(rows) != 1 || rows[0].CustomerID != "9007199254740993" {
		t.Fatalf("large numeric ID lost digits: %v %+v", err, rows)
	}
}

func TestStaleFileTokenDroppedEvenWithEnvToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Dir(cfg.Path), 0o700)
	if err := os.WriteFile(cfg.Path, []byte("synnex_access_token = \"stale\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "from-env")
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("fresh\n"))
	cmd.SetOut(io.Discard)
	if err := ionSetTokenRunE(&rootFlags{})(cmd, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "")
	cfg, err = config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "Bearer fresh" {
		t.Fatalf("the stale file token came back once the env token was unset: %q", got)
	}
}

func TestCostCompletenessInMoneyAndMargin(t *testing.T) {
	pinNow(t)
	partial := ionSub{BillingCycle: "monthly", BillingData: []ionBillingRow{
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 120, Set: true}, SellerCost: optFloat{V: 100, Set: true}},
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 30, Set: true}}, // no sellerCost
	}}
	if m := partial.MonthlyMoney(); m.CostKnown {
		t.Fatalf("a month with a row lacking sellerCost must not count as a known cost: %+v", m)
	}
	b := &ionBook{Customers: map[string]ionCustomer{"1": {Name: "accounts/9/customers/1", Organization: "A"}}, Subs: []ionSub{
		{ID: "s1", CustomerID: "1", Status: "active", BillingCycle: "monthly", Price: optFloat{V: 100, Set: true}},
	}}
	rows := bookMargin(b, "customer")
	if len(rows) != 1 || rows[0].UnknownCost != 1 || !strings.Contains(strings.Join(rows[0].Flags, ","), "cost_incomplete") {
		t.Fatalf("a subscription without a reported cost must be flagged: %+v", rows)
	}
}

func TestParsePositionFileRejectsTrailingJSON(t *testing.T) {
	if _, _, err := parsePositionFile("x.json", []byte(`[{"customer_name":"A","sku_key":"K","seats":1}][{"customer_name":"B"}]`), nil); err == nil {
		t.Fatal("a second concatenated array must be an error, not silently dropped")
	}
}

func TestCostCompletenessReachesExportAndCustomerView(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{"1": {Name: "accounts/9/customers/1", Organization: "A"}}, Subs: []ionSub{
		{ID: "s1", CustomerID: "1", CCPSkuID: "K", Seats: optFloat{V: 1, Set: true}, Status: "active", BillingCycle: "monthly", Price: optFloat{V: 100, Set: true}},
	}}
	rows := ionPositions(b, false)
	if len(rows) != 1 || rows[0].CostKnown {
		t.Fatalf("export must say the cost is unknown: %+v", rows)
	}
	v := customerView(b, b.Customers["1"], testNow)
	if len(v.Subscriptions) != 1 || v.Subscriptions[0].CostKnown || len(v.Totals) != 1 || v.Totals[0].CostComplete {
		t.Fatalf("customer view must carry cost completeness: %+v", v)
	}
}

func TestOrderItemLabelPrefersSyncedSubscription(t *testing.T) {
	sub := ionSub{SubscriptionName: "Microsoft 365 Business Standard"}
	update := ionOrderItem{Action: "UPDATE", ResourceID: "v-aaa", SkuID: "USCFQ7TTC0LDPB0001"}
	if got := orderItemLabel(update, sub, true); got != "Microsoft 365 Business Standard" {
		t.Fatalf("matched UPDATE item should take the subscription label, got %q", got)
	}
	for _, c := range []struct {
		it   ionOrderItem
		want string
	}{
		{ionOrderItem{SkuName: "Teams Essentials", SkuID: "k"}, "Teams Essentials"},
		{ionOrderItem{SkuID: "USCFQ7TTC0LDPB0001", ResourceID: "v-aaa"}, "USCFQ7TTC0LDPB0001"},
		{ionOrderItem{ResourceID: "v-aaa"}, "v-aaa"},
		{ionOrderItem{}, "unnamed item"},
	} {
		if got := orderItemLabel(c.it, ionSub{}, false); got != c.want {
			t.Fatalf("unmatched %+v: got %q, want %q", c.it, got, c.want)
		}
	}
	b := testBook()
	b.Orders = append(b.Orders, ionOrder{OrderID: "o9", CustomerID: "1", Status: "NEW", CreateTime: day(-3),
		Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "v1", SkuID: "USCFQ7TTC0LDPB0001", Quantity: optFloat{V: 26, Set: true}}}})
	found := false
	for _, r := range stuckOrders(b, testNow, 24*time.Hour, 0) {
		if r.OrderID == "o9" {
			found = true
			if len(r.Items) != 1 || r.Items[0] != "M365 BS x26 (update)" {
				t.Fatalf("stuck UPDATE item should be named from subscription v1, got %v", r.Items)
			}
		}
	}
	if !found {
		t.Fatal("o9 (NEW, 3 days old) should be stuck")
	}
}

func TestPrintIONJSONCarriesFreshnessInAgentMeta(t *testing.T) {
	var out strings.Builder
	flags := &rootFlags{agent: true, asJSON: true,
		freshnessMeta: cliutil.FreshnessMeta{Decision: "no-store", Ran: true, Reason: "refresh_failed"}}
	if err := printIONJSON(&out, []stuckOrderRow{}, flags); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Meta struct {
			Source    string `json:"source"`
			Freshness struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
			} `json:"freshness"`
		} `json:"meta"`
		Results []any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if env.Meta.Source != "local" || env.Meta.Freshness.Reason != "refresh_failed" || env.Meta.Freshness.Decision != "no-store" || env.Results == nil {
		t.Fatalf("an empty result must say the refresh failed: %s", out.String())
	}
}

func TestFreshnessMetaDoesNotCertifyAnotherDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	flags := &rootFlags{freshnessMeta: map[string]any{"decision": "fresh"}}
	cmd := &cobra.Command{}
	cmd.SetErr(io.Discard)
	book, closeDB, err := openIONBook(cmd, flags, filepath.Join(home, "missing.db"), "subscriptions")
	defer closeDB()
	if err != nil || book != nil {
		t.Fatalf("missing --db store: book=%v err=%v", book, err)
	}
	m, _ := flags.freshnessMeta.(map[string]any)
	if m["decision"] != "no-store" {
		t.Fatalf("a missing --db store must report no-store, got %v", flags.freshnessMeta)
	}
}
