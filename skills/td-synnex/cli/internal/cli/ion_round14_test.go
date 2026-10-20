// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: regression checks for adversarial round 14 and Codex round 16.

package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/client"
	"td-synnex-pp-cli/internal/cliutil/testenv"
	"td-synnex-pp-cli/internal/config"
)

func activeMonthly(id, cust string) ionSub {
	return ionSub{ID: id, CustomerID: cust, Status: "active", BillingCycle: "monthly", Currency: "USD",
		Seats: optFloat{V: 1, Set: true}, MfgPartNumber: "CFQ7TTC0LDPB:0001:P1M:M"}
}

// A missing price is unknown, never 0, so it cannot make a loss.
func TestMarginUnknownPriceIsNotALoss(t *testing.T) {
	s := activeMonthly("9001", "1")
	s.Cost = optFloat{V: 22, Set: true}
	rows := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{s}}, "customer")
	r := rows[0]
	if r.Margin != 0 || r.NegativeSubs != 0 || r.UnknownPrice != 1 || strings.Join(r.Flags, ",") != "price_incomplete" {
		t.Fatalf("unknown price must not be a loss: %+v", r)
	}
}

// A billed row without a customerCost takes the price ION sent on the
// subscription instead of reading as 0.
func TestMarginBilledRowWithoutPriceUsesSubscriptionPrice(t *testing.T) {
	s := activeMonthly("9002", "1")
	s.Price, s.Cost = optFloat{V: 16, Set: true}, optFloat{V: 12, Set: true}
	s.BillingData = []ionBillingRow{{BillingMonth: nowFunc().Format("2006-01"), SellerCost: optFloat{V: 12, Set: true}}}
	m := s.MonthlyMoney()
	if !m.PriceKnown || m.Price != 16 || m.Cost != 12 || m.Basis != "subscription" {
		t.Fatalf("billed row without a price must take the subscription price: %+v", m)
	}
	// With no subscription price either, the price stays unknown.
	s.Price = optFloat{}
	if m := s.MonthlyMoney(); m.PriceKnown || m.Price != 0 {
		t.Fatalf("price must stay unknown: %+v", m)
	}
	// A partial billed month (a proration) is never paired with the
	// subscription's full-month cost: no loss is invented (adversarial round 15).
	s.Price, s.Cost = optFloat{V: 100, Set: true}, optFloat{}
	s.BillingData = []ionBillingRow{{BillingMonth: nowFunc().Format("2006-01"), CustomerCost: optFloat{V: 50, Set: true}}}
	s.Cost = optFloat{V: 80, Set: true}
	if m := s.MonthlyMoney(); m.Price != 100 || m.Cost != 80 || m.Basis != "subscription" {
		t.Fatalf("billed half must not mix with the subscription half: %+v", m)
	}
	s.Price = optFloat{}
	if m := s.MonthlyMoney(); m.PriceKnown != true || m.Price != 50 || m.CostKnown {
		t.Fatalf("an incomplete billed month with an incomplete subscription stays partly unknown: %+v", m)
	}
}

// An unrecognized billing cycle leaves every amount unknown.
func TestMonthlyMoneyUnknownCycle(t *testing.T) {
	s := activeMonthly("1", "1")
	s.BillingCycle, s.Price, s.Cost = "Every4Years", optFloat{V: 3600, Set: true}, optFloat{V: 3000, Set: true}
	if m := s.MonthlyMoney(); m.PriceKnown || m.CostKnown || m.Basis != "unknown_cycle" {
		t.Fatalf("unknown cycle must leave amounts unknown: %+v", m)
	}
}

// --billed flags supplier seats that have no PSA row at all.
func TestTrueupFlagsSeatsNeverBilled(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Contoso Dental", SKUKey: "A:1", Seats: 3},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "SilverLeaf", SKUKey: "A:1", Seats: 12},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "SilverLeaf", SKUKey: "A:1", Seats: 12},
	}
	flags := map[string]string{}
	for _, r := range trueup(inputs, true, true).Rows {
		flags[r.Customer] = strings.Join(r.Flags, ",")
	}
	if flags["Contoso Dental"] != "not_billed" || flags["SilverLeaf"] != "" {
		t.Fatalf("flags = %v", flags)
	}
	// Without --billed there is no billing file to compare against.
	for _, r := range trueup(inputs[:2], true, false).Rows {
		if len(r.Flags) != 0 {
			t.Fatalf("no not_billed without --billed: %+v", r)
		}
	}
}

// TD SYNNEX rows with no SKU identity or no seat count are listed, never
// merged into one duplicate or summed as 0.
func TestTrueupNeverGroupsMissingIdentityOrSeats(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{
		{ID: "a", CustomerID: "1", Status: "active", Seats: optFloat{V: 5, Set: true}},
		{ID: "b", CustomerID: "1", Status: "active", Seats: optFloat{V: 7, Set: true}},
		{ID: "c", CustomerID: "1", Status: "active", MfgPartNumber: "X:1:P1M:M"},
	}}
	v := trueup(ionPositions(b, false), false, false)
	if len(v.Rows) != 0 || len(v.Unmatched) != 2 {
		t.Fatalf("want 0 rows and 2 unmatched (no sku_key), got %+v / %+v", v.Rows, v.Unmatched)
	}
	// With files, the unknown-seat subscription is a row flagged seats_incomplete, never 0 seats.
	v = trueup(ionPositions(b, false), true, false)
	if len(v.Rows) != 1 || strings.Join(v.Rows[0].Flags, ",") != "seats_incomplete" || len(v.Rows[0].SeatsBySource) != 0 {
		t.Fatalf("unknown seats must be flagged, not summed: %+v", v.Rows)
	}
	if rows := bookMargin(b, "sku"); len(rows) != 3 {
		t.Fatalf("margin --by sku must not group subscriptions without a SKU: %+v", rows)
	}
}

func TestReservedDistributorNames(t *testing.T) {
	for _, n := range []string{"td-synnex", "TD SYNNEX", "TD_Synnex", "td synnex corp"} {
		if reservedDistributor(n, false) == nil {
			t.Errorf("%q must be reserved", n)
		}
	}
	if reservedDistributor("PSA", false) == nil || reservedDistributor("psa", true) != nil || reservedDistributor("pax8", false) != nil {
		t.Error("psa is reserved outside --billed only")
	}
}

// Every default-ignorable character is dropped before judging, and
// typographic apostrophes fold to '.
func TestRedactDropsAllIgnorablesAndFoldsApostrophes(t *testing.T) {
	const s = "QZXsentinelQZX"
	note := func(text string) string {
		b, _ := json.Marshal(map[string]string{"notes": text})
		return string(b)
	}
	attr := func(name string) string {
		b, _ := json.Marshal(map[string]any{"attributes": []map[string]string{{"name": name, "value": s}}})
		return string(b)
	}
	cases := []string{
		attr("tenantAdminPass\U000E0100word"), // VS17
		note("API\U000E01EF key: " + s),       // VS256
		note("pass\u180Fword: " + s),          // Mongolian FVS4
		note("sec\u3164ret: " + s),            // Hangul filler
		note("Code d\u2019acc\u00e8s : " + s), // typographic apostrophe
		attr("Access code"),
		attr("code d'acc\u00e8s"),
		attr("code d\u2019acc\u00e8s"),
		`{"doorCode":"` + s + `"}`,
	}
	for _, in := range cases {
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %q -> %s", in, out)
		}
	}
	// Control: the same characters with no credential word survive.
	if out := string(redactSecretsJSON([]byte(note("Ticket \U000E0100 \u3164 " + s)))); !strings.Contains(out, s) {
		t.Errorf("over-redacted a plain note: %s", out)
	}
}

// Round 15 / Codex round 17 regressions.

// A missing order quantity is unknown, never a zero-seat purchase.
func TestChangesUnknownQuantity(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{{OrderID: "o1", CustomerID: "1", Status: "COMPLETED",
		CreateTime: day(-1), Items: []ionOrderItem{{Action: "CREATE", ResourceID: "r1"}}}}}
	rows := subscriptionChanges(b, testNow, 30*24*time.Hour, "", false)
	if len(rows) != 1 || rows[0].Value != "unknown" || rows[0].Quantity != nil {
		t.Fatalf("missing quantity must be unknown: %+v", rows)
	}
}

// A position file row exported with seats_known false carries no count, and
// a billed row with an unknown count gives no billing verdict.
func TestPositionFileSeatsKnownFalseIsRejected(t *testing.T) {
	rows, bad, err := parsePositionFile("psa.json", []byte(`[{"customer_name":"Acme","sku_key":"K","seats":0,"seats_known":false}]`), nil)
	if err != nil || len(rows) != 1 || rows[0].SeatsKnown || len(bad) != 0 {
		t.Fatalf("rows=%+v bad=%+v err=%v", rows, bad, err)
	}
	rows[0].Distributor = "psa"
	inputs := append([]positionRow{{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 10}}, rows...)
	v := trueup(inputs, true, true)
	if len(v.Rows) != 1 || strings.Join(v.Rows[0].Flags, ",") != "seats_incomplete" || v.Rows[0].BilledSeats != nil {
		t.Fatalf("an unknown billed count must not be not_billed: %+v", v.Rows)
	}
}

// A customer+SKU with an unknown TD SYNNEX seat count gets no billing
// verdict, and trial seats are not "not billed".
func TestTrueupUnknownSeatsAndTrials(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", CustomerID: "1", CustomerName: "Contoso", SKUKey: "A:1"}, // seats unknown
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Contoso", SKUKey: "A:1", Seats: 5},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Trialco", SKUKey: "B:1", Seats: 25, Trial: true},
	}
	flags := map[string]string{}
	for _, r := range trueup(inputs, true, true).Rows {
		flags[r.Customer] = strings.Join(r.Flags, ",")
	}
	if flags["Contoso"] != "multi_source,seats_incomplete" || flags["Trialco"] != "trial" {
		t.Fatalf("flags = %v", flags)
	}
	// Control: the same trial seats, paid, are flagged not_billed.
	inputs[2].Trial = false
	for _, r := range trueup(inputs, true, true).Rows {
		if r.Customer == "Trialco" && strings.Join(r.Flags, ",") != "not_billed" {
			t.Fatalf("paid seats with no PSA row must be not_billed: %+v", r)
		}
	}
}

func TestReservedDistributorStreamOne(t *testing.T) {
	if reservedDistributor("TD SYNNEX StreamOne", false) == nil || reservedDistributor("StreamOne ION", false) == nil {
		t.Fatal("TD SYNNEX StreamOne names are reserved")
	}
}

func TestRedactApostropheVariantsAndPW(t *testing.T) {
	const s = "QZXsentinelQZX"
	for _, in := range []string{
		`{"notes":"Code d` + "‛" + `acc` + "è" + `s : ` + s + `"}`,
		`{"notes":"Code d` + "`" + `acc` + "è" + `s : ` + s + `"}`,
		`{"notes":"Code d` + "´" + `acc` + "è" + `s : ` + s + `"}`,
		`{"attributes":[{"name":"p/w","value":"` + s + `"}]}`,
	} {
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %q -> %s", in, out)
		}
	}
}

// monthly_cost_price_unknown reports the cost inside monthly_cost that margin
// leaves out, so the totals reconcile.
func TestMarginCostOfPriceUnknownReported(t *testing.T) {
	a, b := activeMonthly("1", "1"), activeMonthly("2", "1")
	a.Price, a.Cost = optFloat{V: 100, Set: true}, optFloat{V: 60, Set: true}
	b.Cost = optFloat{V: 50, Set: true}
	r := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{a, b}}, "customer")[0]
	if r.MonthlyCost != 110 || r.CostUnknownPrice != 50 || r.Margin != 40 || r.MonthlyCost-r.CostUnknownPrice != 60 {
		t.Fatalf("%+v", r)
	}
}

// Codex round 18 / adversarial round 16 regressions.

func TestMarginNeverMergesMissingCustomerIDs(t *testing.T) {
	a, b := activeMonthly("1", ""), activeMonthly("2", "")
	a.CustomerName, b.CustomerName = "Acme", "Beta"
	rows := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{a, b}}, "customer")
	if len(rows) != 2 {
		t.Fatalf("two customers without IDs must stay apart: %+v", rows)
	}
}

func TestMarginUnknownSortsLastAndMSRPGap(t *testing.T) {
	known, unknown := activeMonthly("1", "1"), activeMonthly("2", "2")
	known.Price, known.Cost = optFloat{V: 10, Set: true}, optFloat{V: 5, Set: true}
	unknown.Price, unknown.Cost = optFloat{V: 1, Set: true}, optFloat{}
	rows := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{known, unknown}}, "customer")
	if len(rows) != 2 || rows[1].Key != "2" || rows[1].paired != 0 {
		t.Fatalf("a group with no known margin must sort last, not as the worst margin 0: %+v", rows)
	}
}

func TestStuckOrderUnknownTotalAndAge(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{{OrderID: "o1", CustomerID: "1", Status: "ERROR", UpdateTime: day(-1)}}}
	rows := stuckOrders(b, testNow, 0, 48*time.Hour)
	if len(rows) != 1 || rows[0].Total != nil || rows[0].AgeHours != nil {
		t.Fatalf("missing total and createTime must be null: %+v", rows)
	}
}

func TestChangesUnknownQuantityClearsSeatHistory(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	item := func(qty optFloat) []ionOrderItem {
		return []ionOrderItem{{Action: "UPDATE", ResourceID: "r1", Quantity: qty}}
	}
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "o1", Status: "COMPLETED", CreateTime: day(-3), Items: item(q(10))},
		{OrderID: "o2", Status: "COMPLETED", CreateTime: day(-2), Items: item(optFloat{})},
		{OrderID: "o3", Status: "COMPLETED", CreateTime: day(-1), Items: item(q(15))},
	}}
	for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", false) {
		if r.OrderID == "o3" && r.PreviousSeats != nil {
			t.Fatalf("seat count before o3 is unknown, got previous_seats %v", *r.PreviousSeats)
		}
	}
}

func TestTrueupUnknownSeatRowStillCountsAsDuplicate(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Nova", SKUKey: "K1", Seats: 5},
		{Distributor: "td-synnex", CustomerID: "1", CustomerName: "Nova", SKUKey: "K1"},
	}
	v := trueup(inputs, false, false)
	if len(v.Rows) != 1 || strings.Join(v.Rows[0].Flags, ",") != "duplicate_in_td-synnex,seats_incomplete" {
		t.Fatalf("ION-only mode must still report the duplicate: %+v", v.Rows)
	}
}

func TestRedactSlashAndAccentLookAlikes(t *testing.T) {
	const s = "QZXsentinelQZX"
	for _, in := range []string{
		`{"attributes":[{"name":"p` + "∕" + `w","value":"` + s + `"}]}`,
		`{"notes":"Code d` + "ˋ" + `acc` + "è" + `s : ` + s + `"}`,
		`{"notes":"cle` + "\u200D\u0301" + ` API: ` + s + `"}`,      // ZWJ between a letter and its accent
		`{"notes":"code d'acce` + "\u034F\u0300" + `s: ` + s + `"}`, // CGJ between a letter and its accent
		`{"notes":"co` + "\u034F\u0301" + `digo de acceso: ` + s + `"}`,
	} {
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %q -> %s", in, out)
		}
	}
}

// Adversarial round 17 / Codex round 19 regressions.

func TestMarginNeverMergesSubscriptionsWithoutAnyID(t *testing.T) {
	a, b := activeMonthly("", ""), activeMonthly("", "")
	a.CustomerName, b.CustomerName = "Alpha", "Beta"
	a.MfgPartNumber, b.MfgPartNumber = "", ""
	book := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{a, b}}
	if n := len(bookMargin(book, "customer")); n != 2 {
		t.Fatalf("by customer: want 2 groups, got %d", n)
	}
	if n := len(bookMargin(book, "sku")); n != 2 {
		t.Fatalf("by sku: want 2 groups, got %d", n)
	}
}

func TestMarginUnknownMSRPFlagged(t *testing.T) {
	s := activeMonthly("1", "1")
	s.Price, s.Cost = optFloat{V: 100, Set: true}, optFloat{V: 80, Set: true}
	r := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{s}}, "sku")[0]
	if r.UnknownMSRP != 1 || strings.Join(r.Flags, ",") != "msrp_incomplete" || !r.MarginKnown {
		t.Fatalf("missing MSRP must be flagged: %+v", r)
	}
}

func TestChangesZeroQuantityAndUndatedOrdersClearHistory(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	item := func(qty optFloat) []ionOrderItem {
		return []ionOrderItem{{Action: "UPDATE", ResourceID: "r1", Quantity: qty}}
	}
	zero := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "o1", Status: "COMPLETED", CreateTime: day(-3), Items: item(q(5))},
		{OrderID: "o2", Status: "COMPLETED", CreateTime: day(-2), Items: item(q(0))},
		{OrderID: "o3", Status: "COMPLETED", CreateTime: day(-1), Items: item(q(8))},
	}}
	for _, r := range subscriptionChanges(zero, testNow, 30*24*time.Hour, "", false) {
		if r.OrderID == "o3" && r.PreviousSeats != nil {
			t.Fatalf("a zero-quantity order leaves the count unknown, got %v", *r.PreviousSeats)
		}
	}
	undated := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "o1", Status: "COMPLETED", CreateTime: day(-10), Items: item(q(10))},
		{OrderID: "o2", Status: "COMPLETED", Items: item(q(15))}, // no createTime or updateTime
		{OrderID: "o3", Status: "COMPLETED", CreateTime: day(-1), Items: item(q(20))},
	}}
	seen := map[string]subChangeRow{}
	for _, r := range subscriptionChanges(undated, testNow, 30*24*time.Hour, "", false) {
		seen[r.OrderID] = r
	}
	if r, ok := seen["o2"]; !ok || r.When != "" || r.WhenFrom != "unknown" {
		t.Fatalf("an undated order must be listed with an unknown time: %+v", seen)
	}
	if seen["o3"].PreviousSeats != nil {
		t.Fatalf("an undated completed order makes previous_seats unknown: %v", *seen["o3"].PreviousSeats)
	}
}

func TestStuckOpenOrderWithoutCreateTimeIsListed(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "o1", CustomerID: "1", Status: "ON_HOLD"},
		{OrderID: "o2", CustomerID: "1", Status: "ON_HOLD", UpdateTime: day(-3)},
	}}
	reasons := map[string]string{}
	for _, r := range stuckOrders(b, testNow, 24*time.Hour, 0) {
		reasons[r.OrderID] = r.Reason
	}
	if reasons["o1"] != "open, age unknown" || reasons["o2"] != "open past threshold" {
		t.Fatalf("reasons = %v", reasons)
	}
}

func TestBodyReadErrorNamedByKind(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			_, _ = conn.Read(buf)
			// A malformed chunk trailer: net/http quotes the line in its error.
			_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\n{}\r\n0\r\npassword=QZXsentinelQZX\r\n\r\n"))
			conn.Close()
		}
	}()
	t.Setenv("TD_SYNNEX_BASE_URL", "http://"+ln.Addr().String())
	c := client.New(&config.Config{}, 0, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "http://"+ln.Addr().String()+"/accounts/1/customers", nil)
	_, err = c.HTTPClient.Do(req)
	if err == nil || strings.Contains(err.Error(), "QZX") || !strings.Contains(err.Error(), "reading ION response for redaction") {
		t.Fatalf("err=%v", err)
	}
}

// Adversarial round 18 / Codex round 20 regressions.

func TestChangeDateUndated(t *testing.T) {
	if changeDate("") != "unknown" || changeDate("2026-10-01T00:00:00Z") != "2026-10-01" {
		t.Fatal("changeDate")
	}
}

func TestRedactNameRulesRecomposeAccents(t *testing.T) {
	const s = "QZXsentinelQZX"
	for _, name := range []string{
		"cle" + "\u200d\u0301" + " API", "co" + "\u200d\u0301" + "digo de acceso", "contrasen" + "\u200d\u0303" + "a",
		"Kennwo" + "\u200d\u0308" + "rt", "code d" + "\u0301" + "acc" + "\u00e8" + "s",
	} {
		b, _ := json.Marshal(map[string]any{"attributes": []map[string]string{{"name": name, "value": s}}})
		if out := string(redactSecretsJSON(b)); strings.Contains(out, s) {
			t.Errorf("pair leaked: %q -> %s", name, out)
		}
		n, _ := json.Marshal(map[string]string{"notes": name + ": " + s})
		if out := string(redactSecretsJSON(n)); strings.Contains(out, s) {
			t.Errorf("note leaked: %q -> %s", name, out)
		}
	}
}

func TestUnreadableIONNumbersAreUnknown(t *testing.T) {
	var s ionSub
	if err := json.Unmarshal([]byte(`{"id":"1","customerId":"1","subscriptionTotalLicenses":"n/a","price":{"x":1},"cost":"abc","mfgPartNumber":"K:1:P1M:M","subscriptionStatus":"active"}`), &s); err != nil {
		t.Fatalf("an unreadable number must not fail the subscription: %v", err)
	}
	if s.Seats.Set || s.Price.Set || s.Cost.Set {
		t.Fatalf("unreadable numbers must be unknown: %+v", s)
	}
	v := trueup(ionPositions(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{s}}, false), true, false)
	if len(v.Rows) != 1 || strings.Join(v.Rows[0].Flags, ",") != "seats_incomplete" || v.Rows[0].TotalKnown {
		t.Fatalf("trueup: %+v", v.Rows)
	}
}

func TestStuckErrorOrderWithoutErrorTime(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "o1", Status: "ERROR"},
		{OrderID: "o2", Status: "ERROR", CreateTime: day(-30)},
		{OrderID: "o3", Status: "ERROR", UpdateTime: day(-30)},
	}}
	reasons := map[string]string{}
	for _, r := range stuckOrders(b, testNow, 24*time.Hour, 7*24*time.Hour) {
		reasons[r.OrderID] = r.Reason
	}
	if reasons["o1"] != "errored, time unknown" || reasons["o2"] != "errored, time unknown" || reasons["o3"] != "" {
		t.Fatalf("reasons = %v", reasons)
	}
}

func TestTrueupAmbiguousBilledRowBlocksVerdict(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Ghost Co", SKUKey: "G:1", Seats: 3},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Ghost Co", SKUKey: "G:1", Seats: 4},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "Ghost Co", SKUKey: "G:1", Seats: 7},
	}
	v := trueup(inputs, true, true)
	for _, r := range v.Rows {
		if strings.Join(r.Flags, ",") != "match_ambiguous" {
			t.Fatalf("an ambiguous billed row must block not_billed: %+v", r)
		}
	}
	if len(v.Unmatched) != 1 {
		t.Fatalf("the ambiguous PSA row is listed: %+v", v.Unmatched)
	}
}

// Adversarial round 19 / Codex round 21 regressions.

// A field of the wrong JSON type never drops the record.
func TestLenientDecodeKeepsRecords(t *testing.T) {
	var s ionSub
	in := `{"id":8012,"customerId":108536,"subscriptionTotalLicenses":"10","renewalDate":1790000000,` +
		`"subscriptionEndDate":{"seconds":1},"autoRenew":"true","isTrial":"false","subscriptionStatus":"active","price":1e308,` +
		`"billingData":[{"billingMonth":202609,"sellerCost":5},"junk"],"ccpProductInfo":{"skuDisplayName":7}}`
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatalf("a mistyped field must not fail the subscription: %v", err)
	}
	if s.ID != "8012" || s.CustomerID != "108536" || !s.Seats.Set || s.Seats.V != 10 || !s.RenewOn().IsZero() ||
		s.AutoRenewState() != "on" || len(s.BillingData) != 2 || s.BillingData[1].BillingMonth != "" || s.ProductInfo.SkuDisplayName != "7" || s.Price.Set {
		t.Fatalf("decoded %+v", s)
	}
	var o ionOrder
	if err := json.Unmarshal([]byte(`{"orderId":962,"status":"COMPLETED","createTime":1792000000,"orderItems":[{"resourceId":"r1","quantity":5,"attributes":[{"name":"x","value":true}]}]}`), &o); err != nil {
		t.Fatalf("a mistyped order field must not fail the order: %v", err)
	}
	if o.OrderID != "962" || !o.Created().IsZero() || len(o.Items) != 1 || o.Items[0].attr("x") != "true" {
		t.Fatalf("decoded %+v", o)
	}
	if err := json.Unmarshal([]byte(`[1,2]`), &s); err == nil {
		t.Fatal("a non-object is still undecodable")
	}
	// A status, trial flag or owner of an unreadable type would read as a guess,
	// so the record is skipped with a warning instead (adversarial round 20).
	for _, bad := range []string{`{"id":"1","subscriptionStatus":{"code":"CANCELLED"}}`, `{"id":"1","isTrial":"maybe"}`, `{"id":"1","customerId":{"id":"9"}}`} {
		var x ionSub
		if err := json.Unmarshal([]byte(bad), &x); err == nil {
			t.Errorf("must be skipped: %s", bad)
		}
	}
}

// The subscriptions set preview goes through the response redactor, end to
// end through the command (Codex round 21, P1).
func TestSubscriptionSetPreviewRedacted(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("preview sent %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"s1","customerId":"1","subscriptionId":"v1","ccpProductId":"p","ccpSkuId":"k","ccpPlanId":"pl","subscriptionTotalLicenses":"3","subscriptionName":"Keeper Password Manager: Business"}`))
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"subscriptions", "set", "1", "1", "s1", "--friendly-name", "Password: QZXsentinelQZX", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "QZX") || !strings.Contains(out.String(), "preview") {
		t.Fatalf("preview leaked or did not run: %s", out.String())
	}
	// Only the body is redacted: the subscription name survives (adversarial round 20).
	if !strings.Contains(out.String(), "Keeper Password Manager: Business") {
		t.Fatalf("preview blanked the product name: %s", out.String())
	}
}

// The write path decodes strictly: any unreadable field refuses the order.
func TestSubscriptionSetUnreadableOwnerFailsClosed(t *testing.T) {
	testenv.Isolate(t)
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		// A readable owner, but a provider ID of an unreadable type: the lenient
		// read would drop it and still order.
		_, _ = w.Write([]byte(`{"id":"s1","customerId":"1","cloudProviderId":{"id":30},"subscriptionId":"v1","ccpProductId":"p","ccpSkuId":"k","ccpPlanId":"pl","subscriptionTotalLicenses":"3"}`))
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"subscriptions", "set", "1", "1", "s1", "--friendly-name", "Front desk", "--confirm", "--json"})
	if err := cmd.Execute(); err == nil || posts != 0 {
		t.Fatalf("an unreadable field must refuse the order: err=%v posts=%d\n%s", err, posts, out.String())
	}
}

func TestRedactSpacingAccentApostrophes(t *testing.T) {
	const s = "QZXsentinelQZX"
	for _, a := range []string{"\u00B4", "\u1FFD", "\u0384"} {
		in := `{"notes":"code d` + a + `acc` + "\u00e8" + `s: ` + s + `"}`
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %s -> %s", in, out)
		}
	}
}

func TestBilledCycleWithUndatedRowIsIncomplete(t *testing.T) {
	pinNow(t)
	s := ionSub{ID: "1", Status: "active", BillingCycle: "annual", Currency: "USD", BillingData: []ionBillingRow{
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 1200, Set: true}, SellerCost: optFloat{V: 1000, Set: true}},
		{BillingMonth: "", CustomerCost: optFloat{V: -1200, Set: true}, SellerCost: optFloat{V: -1000, Set: true}},
	}}
	if m := s.MonthlyMoney(); m.PriceKnown || m.CostKnown {
		t.Fatalf("an undated billing row leaves the billed totals incomplete: %+v", m)
	}
}

func TestTrueupPSAOnlySKUTotalKnownZero(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "SilverLeaf", SKUKey: "A:1", Seats: 2},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "SilverLeaf", SKUKey: "Z:9", Seats: 3},
	}
	for _, r := range trueup(inputs, true, true).Rows {
		if r.SKUKey == "Z:9" && (!r.TotalKnown || r.TotalSeats != 0 || strings.Join(r.Flags, ",") != "billed_mismatch") {
			t.Fatalf("a PSA-only SKU has a known zero supply: %+v", r)
		}
	}
}

// Codex round 22 / adversarial round 20 regressions.

func TestMarginSyntheticKeysNeverCollide(t *testing.T) {
	a := ionSub{ID: "42", CustomerName: "Alpha", Status: "active", BillingCycle: "monthly", Seats: optFloat{V: 5, Set: true}}
	b := ionSub{SubscriptionID: "42", CustomerName: "Beta", Status: "active", BillingCycle: "monthly", Seats: optFloat{V: 7, Set: true}}
	c := ionSub{ID: "9", CustomerID: "subscription:42", CustomerName: "Collider", Status: "active", BillingCycle: "monthly", Seats: optFloat{V: 3, Set: true}}
	d := ionSub{ID: "10", CustomerID: "s\x000", CustomerName: "Raw", Status: "active", BillingCycle: "monthly", Seats: optFloat{V: 1, Set: true}}
	book := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{a, b, c, d}}
	for _, by := range []string{"customer", "sku"} {
		if n := len(bookMargin(book, by)); n != 4 {
			t.Fatalf("by %s: want 4 groups, got %d", by, n)
		}
	}
}

func TestMarginTrendUndatedRowMarksMonthsIncomplete(t *testing.T) {
	s := activeMonthly("1", "1")
	s.BillingData = []ionBillingRow{
		{BillingMonth: "2026-10", CustomerCost: optFloat{V: 1200, Set: true}, SellerCost: optFloat{V: 1000, Set: true}},
		{BillingMonth: "", CustomerCost: optFloat{V: -1200, Set: true}, SellerCost: optFloat{V: -1000, Set: true}},
	}
	rows := marginTrend(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{s}}, "customer", 1)
	if len(rows) != 1 || rows[0].PriceComplete || rows[0].CostComplete || rows[0].MarginKnown {
		t.Fatalf("an undated row must leave the month incomplete: %+v", rows)
	}
}

// Codex round 23 / adversarial round 21 regressions.

func TestBilledCycleUnknownCurrencyNeverJoins(t *testing.T) {
	s := ionSub{ID: "1", Status: "active", BillingCycle: "monthly", BillingData: []ionBillingRow{
		{BillingMonth: nowFunc().Format("2006-01"), CustomerCost: optFloat{V: 100, Set: true}, SellerCost: optFloat{V: 80, Set: true}},
		{BillingMonth: nowFunc().Format("2006-01"), CustomerCost: optFloat{V: 50, Set: true}, SellerCost: optFloat{V: 40, Set: true}, Currency: "USD"},
	}}
	if m := s.MonthlyMoney(); m.PriceKnown || m.CostKnown || m.Price != 0 {
		t.Fatalf("a row of unknown currency must not join a USD total: %+v", m)
	}
}

func TestExactStatusAndStrictOrderFields(t *testing.T) {
	for _, bad := range []string{`{"id":"1","subscriptionStatus":4}`, `{"id":"1","subscriptionStatus":false}`} {
		var s ionSub
		if json.Unmarshal([]byte(bad), &s) == nil {
			t.Errorf("a status that is not a string must skip the subscription: %s", bad)
		}
	}
	for _, bad := range []string{`{"orderId":"1","status":["ERROR"]}`, `{"orderId":"1","status":7}`, `{"orderId":"1","customerId":{"id":"1"}}`} {
		var o ionOrder
		if json.Unmarshal([]byte(bad), &o) == nil {
			t.Errorf("must skip the order: %s", bad)
		}
	}
	var o ionOrder
	if err := json.Unmarshal([]byte(`{"orderId":"1","customerId":108536,"status":"NEW"}`), &o); err != nil || o.CustomerID != "108536" {
		t.Fatalf("a numeric customer ID still reads as its text: %v %+v", err, o)
	}
}

func TestMistypedListIsOneUnknownElement(t *testing.T) {
	var s ionSub
	if err := json.Unmarshal([]byte(`{"id":"1","subscriptionStatus":"active","billingData":{"rows":[1]}}`), &s); err != nil || len(s.BillingData) != 1 || s.BillingData[0].BillingMonth != "" {
		t.Fatalf("a billingData object must read as one unknown row: %v %+v", err, s.BillingData)
	}
}

func TestChangesItemOfUnknownResourceBlindsCustomer(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "o1", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-3), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r1", Quantity: q(5)}}},
		{OrderID: "o2", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-2), Items: []ionOrderItem{{Action: "UPDATE", Quantity: q(9)}}},
		{OrderID: "o3", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-1), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r1", Quantity: q(12)}}},
	}}
	for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", false) {
		if r.OrderID == "o3" && r.PreviousSeats != nil {
			t.Fatalf("an item of unknown resource makes later previous_seats unknown, got %v", *r.PreviousSeats)
		}
	}
}

func TestMarginCurrencyKeyCannotJoin(t *testing.T) {
	a, b := activeMonthly("1", "108536|USD"), activeMonthly("2", "108536")
	a.Currency, b.Currency = "EUR", "USD|EUR"
	if n := len(bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{a, b}}, "customer")); n != 2 {
		t.Fatalf("want 2 groups, got %d", n)
	}
}

func TestNonFiniteResultsAreUnknown(t *testing.T) {
	s := activeMonthly("1", "1")
	s.Price, s.Cost = optFloat{V: 1e-310, Set: true}, optFloat{V: -5, Set: true}
	r := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{s}}, "customer")[0]
	if r.MarginPct != nil {
		t.Fatalf("an infinite margin_pct must be null: %v", *r.MarginPct)
	}
	v := trueup([]positionRow{{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "A", SKUKey: "K", Seats: 1e-310, MonthlyCost: 5, CostKnown: true}}, true, false)
	if len(v.Rows) == 1 && len(v.Rows[0].UnitCostBySrc) != 0 {
		t.Fatalf("an infinite unit cost must be omitted: %+v", v.Rows[0])
	}
	if _, err := json.Marshal(v); err != nil {
		t.Fatal(err)
	}
}

func TestTrueupBlankSKUBilledRowBlocksVerdict(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "SilverLeaf", SKUKey: "A:1", Seats: 12},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "SilverLeaf", Seats: 12},
	}
	v := trueup(inputs, true, true)
	if len(v.Rows) != 1 || strings.Join(v.Rows[0].Flags, ",") != "match_ambiguous" {
		t.Fatalf("a billed row with no sku_key must block not_billed: %+v", v.Rows)
	}
}

// Codex round 24 / adversarial round 22 regressions.

func TestChangesTiesCreatesAndBlindClearing(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	up := func(res string, v float64) []ionOrderItem {
		return []ionOrderItem{{Action: "UPDATE", ResourceID: res, Quantity: q(v)}}
	}
	prevOf := func(b *ionBook) map[string]*float64 {
		out := map[string]*float64{}
		for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", false) {
			out[r.OrderID] = r.PreviousSeats
		}
		return out
	}
	tie := prevOf(&ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "a", Status: "COMPLETED", CreateTime: day(-5), Items: up("r1", 3)},
		{OrderID: "b", Status: "COMPLETED", CreateTime: day(-3), Items: up("r1", 5)},
		{OrderID: "c", Status: "COMPLETED", CreateTime: day(-3), Items: up("r1", 9)},
		{OrderID: "d", Status: "COMPLETED", CreateTime: day(-1), Items: up("r1", 12)},
	}})
	if tie["b"] != nil || tie["c"] != nil || tie["d"] != nil {
		t.Fatalf("tied orders and the next change have no previous_seats: %v %v %v", tie["b"], tie["c"], tie["d"])
	}
	create := prevOf(&ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "a", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-6), Items: up("r1", 5)},
		{OrderID: "b", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-5), Items: []ionOrderItem{{Action: "CREATE", Quantity: q(3)}}},
		{OrderID: "c", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-4), Items: up("r1", 8)},
	}})
	if create["c"] == nil || *create["c"] != 5 {
		t.Fatalf("a new purchase must not blind the history: %v", create["c"])
	}
	clear := prevOf(&ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "a", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-6), Items: []ionOrderItem{{Action: "UPDATE", Quantity: q(9)}}},
		{OrderID: "b", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-5), Items: up("r1", 5)},
		{OrderID: "c", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-4), Items: up("r1", 8)},
	}})
	if clear["b"] != nil || clear["c"] == nil || *clear["c"] != 5 {
		t.Fatalf("a later known count must clear the blind: b=%v c=%v", clear["b"], clear["c"])
	}
}

func TestUnreadableCurrencyIsItsOwn(t *testing.T) {
	var s ionSub
	in := `{"id":"1","subscriptionStatus":"active","subscriptionBillingCycle":"monthly","currency":"USD","billingData":[` +
		`{"billingMonth":"` + nowFunc().Format("2006-01") + `","customerCost":50,"sellerCost":40,"currency":"USD"},` +
		`{"billingMonth":"` + nowFunc().Format("2006-01") + `","customerCost":100,"sellerCost":80,"currency":{"code":"EUR"}}]}`
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if m := s.MonthlyMoney(); m.PriceKnown || m.CostKnown {
		t.Fatalf("an unreadable currency must not join USD: %+v", m)
	}
}

func TestRedactedIDsAreNoKey(t *testing.T) {
	var o ionOrder
	if err := json.Unmarshal([]byte(`{"orderId":"1","status":"COMPLETED","orderItems":[{"resourceId":"[redacted]","quantity":5}]}`), &o); err != nil || o.Items[0].ResourceID != "" {
		t.Fatalf("a [redacted] resource ID must read as unknown: %v %+v", err, o.Items)
	}
	var s ionSub
	if err := json.Unmarshal([]byte(`{"id":"1","customerId":"[redacted]","subscriptionStatus":"active"}`), &s); err != nil || s.CustomerID != "" {
		t.Fatalf("a [redacted] customer ID must read as unknown: %v %+v", err, s)
	}
}

func TestTrueupBlankSKUAmbiguousAndIONSide(t *testing.T) {
	amb := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 3},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "2", CustomerName: "Acme", SKUKey: "K", Seats: 4},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "Acme", Seats: 10},
	}
	for _, r := range trueup(amb, true, true).Rows {
		if strings.Join(r.Flags, ",") != "match_ambiguous" {
			t.Fatalf("an ambiguous billed row with no sku_key must block not_billed: %+v", r)
		}
	}
	ion := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "SilverLeaf", SKUKey: "A:1", Seats: 12},
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "SilverLeaf", Seats: 5},
		{Distributor: "psa", SeatsKnown: true, CustomerName: "SilverLeaf", SKUKey: "A:1", Seats: 17},
	}
	for _, r := range trueup(ion, true, true).Rows {
		if strings.Join(r.Flags, ",") != "match_ambiguous" {
			t.Fatalf("a TD SYNNEX row with no SKU blocks the customer's billing verdict: %+v", r)
		}
	}
}

func TestFileAmountsBoundedAndPartNumbersReduced(t *testing.T) {
	rows, _, err := parsePositionFile("psa.csv", []byte("customer_name,sku_key,seats\nA,cfq7ttc0lflx:0001:P1Y:Y,5\nB,K,1e308\n"), nil)
	if err != nil || len(rows) != 2 || rows[0].SKUKey != "CFQ7TTC0LFLX:0001" || rows[1].SeatsKnown {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

// Codex round 25 / adversarial round 23 regressions.

func TestChangesBlindByTimeNotWalkOrder(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	up := func(res string, v float64) []ionOrderItem {
		return []ionOrderItem{{Action: "UPDATE", ResourceID: res, Quantity: q(v)}}
	}
	prevOf := func(orders ...ionOrder) map[string]*float64 {
		out := map[string]*float64{}
		for _, r := range subscriptionChanges(&ionBook{Customers: map[string]ionCustomer{}, Orders: orders}, testNow, 30*24*time.Hour, "", false) {
			out[r.OrderID] = r.PreviousSeats
		}
		return out
	}
	// A blind and a count at the same instant: the count does not clear it.
	same := prevOf(
		ionOrder{OrderID: "a", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-3), Items: []ionOrderItem{{Action: "UPDATE", Quantity: q(9)}}},
		ionOrder{OrderID: "b", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-3), Items: up("r1", 5)},
		ionOrder{OrderID: "c", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-1), Items: up("r1", 12)},
	)
	if same["c"] != nil {
		t.Fatalf("a same-instant blind must hold: %v", *same["c"])
	}
	// An undated change of unknown subscription blinds its customer for good.
	undated := prevOf(
		ionOrder{OrderID: "a", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-6), Items: up("r1", 5)},
		ionOrder{OrderID: "u", CustomerID: "1", Status: "COMPLETED", Items: []ionOrderItem{{Action: "UPDATE", Quantity: q(20)}}},
		ionOrder{OrderID: "b", CustomerID: "1", Status: "COMPLETED", CreateTime: day(-4), Items: up("r1", 8)},
	)
	if undated["b"] != nil {
		t.Fatalf("an undated unknown change must blind: %v", *undated["b"])
	}
}

func TestStrictNullsAndCustomerNull(t *testing.T) {
	for _, bad := range []string{`{"id":"1","subscriptionStatus":"active","isTrial":null}`, `{"id":"1","subscriptionStatus":null}`} {
		var s ionSub
		if json.Unmarshal([]byte(bad), &s) == nil {
			t.Errorf("must be skipped: %s", bad)
		}
	}
	var s ionSub
	if err := json.Unmarshal([]byte(`{"id":"1","subscriptionStatus":"active","customerId":null}`), &s); err != nil || s.CustomerID != "" {
		t.Fatalf("a null customer ID is no customer: %v %+v", err, s)
	}
}

func TestFileSKUKeyOnlyReducesMicrosoftPartNumbers(t *testing.T) {
	if fileSKUKey("ACME:BACKUP:STANDARD") == fileSKUKey("ACME:BACKUP:PREMIUM") {
		t.Fatal("two vendor SKUs that share a prefix must stay apart")
	}
	if fileSKUKey("cfq7ttc0lflx:0001:P1Y:Y") != "CFQ7TTC0LFLX:0001" {
		t.Fatal("a Microsoft part number reduces to product:SKU")
	}
}

// Codex round 26 / adversarial round 24 regressions.

func TestIONSKUKeyKeepsVendorPartNumbersWhole(t *testing.T) {
	a := ionSub{MfgPartNumber: "ACME:BACKUP:STANDARD"}
	b := ionSub{MfgPartNumber: "ACME:BACKUP:PREMIUM"}
	if a.SKUKey() == b.SKUKey() || a.SKUKey() != fileSKUKey("acme:backup:standard") {
		t.Fatalf("vendor part numbers: %q %q", a.SKUKey(), b.SKUKey())
	}
	if (ionSub{MfgPartNumber: "CFQ7TTC0LDPB:0001:P1Y:M"}).SKUKey() != "CFQ7TTC0LDPB:0001" {
		t.Fatal("a Microsoft part number reduces to product:SKU")
	}
}

func TestSubscriptionSetRefusesNullStatusOrTrial(t *testing.T) {
	for _, extra := range []string{`"subscriptionStatus":null`, `"isTrial":null`} {
		testenv.Isolate(t)
		posts := 0
		body := `{"id":"s1","customerId":"1","subscriptionId":"v1","ccpProductId":"p","ccpSkuId":"k","ccpPlanId":"pl","subscriptionTotalLicenses":"3",` + extra + `}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				posts++
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
		t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"subscriptions", "set", "1", "1", "s1", "--friendly-name", "Desk", "--confirm", "--json"})
		err := cmd.Execute()
		srv.Close()
		if err == nil || posts != 0 {
			t.Fatalf("%s must refuse the order: err=%v posts=%d\n%s", extra, err, posts, out.String())
		}
	}
}

func TestChangesOrderWithoutCustomerSharesAnyBlind(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "a", CustomerID: "c1", Status: "COMPLETED", CreateTime: day(-10), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "rA", Quantity: q(5)}}},
		{OrderID: "b", CustomerID: "c1", Status: "COMPLETED", CreateTime: day(-8), Items: []ionOrderItem{{Action: "UPDATE", Quantity: q(20)}}},
		{OrderID: "c", Status: "COMPLETED", CreateTime: day(-6), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "rA", Quantity: q(8)}}},
	}}
	for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", false) {
		if r.OrderID == "c" && r.PreviousSeats != nil {
			t.Fatalf("an order of unknown customer is blinded by any customer's unknown change: %v", *r.PreviousSeats)
		}
	}
}

// Adversarial round 25 / Codex round 27 regressions.

func TestSKUKeyLetteredSKUIDAndNoColonPartNumber(t *testing.T) {
	a := ionSub{MfgPartNumber: "CFQ7TTC0Q171:000B:P1M:M"}
	b := ionSub{MfgPartNumber: "CFQ7TTC0Q171:000B:P1Y:M"}
	if a.SKUKey() != "CFQ7TTC0Q171:000B" || b.SKUKey() != a.SKUKey() || fileSKUKey("cfq7ttc0q171:000b:P1Y:Y") != a.SKUKey() {
		t.Fatalf("a lettered Microsoft SKU ID reduces: %q %q", a.SKUKey(), b.SKUKey())
	}
	if k := (ionSub{MfgPartNumber: "ACME-STANDARD", CCPSkuID: "SK1234"}).SKUKey(); k != "ACME-STANDARD" {
		t.Fatalf("a part number without colons is the key: %q", k)
	}
}

func TestFileRedactedSKUIsUnknown(t *testing.T) {
	rows, _, err := parsePositionFile("psa.csv", []byte("customer_name,sku_key,seats\nAcme,[redacted],5\n"), nil)
	if err != nil || len(rows) != 1 || rows[0].SKUKey != "" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	rows[0].Distributor = "psa"
	v := trueup(append([]positionRow{{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Acme", SKUKey: "K", Seats: 5}}, rows...), true, true)
	if len(v.Rows) != 1 || strings.Join(v.Rows[0].Flags, ",") != "match_ambiguous" {
		t.Fatalf("a [redacted] sku_key gives no billing verdict: %+v", v.Rows)
	}
}

func TestChangesFailedOrderComparesWithHistory(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	up := func(v float64) []ionOrderItem {
		return []ionOrderItem{{Action: "UPDATE", ResourceID: "v1", Quantity: q(v)}}
	}
	b := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{{ID: "s1", SubscriptionID: "v1", Seats: q(8)}}, Orders: []ionOrder{
		{OrderID: "a", Status: "COMPLETED", CreateTime: day(-5), Items: up(5)},
		{OrderID: "b", Status: "ERROR", CreateTime: day(-4), Items: up(6)},
		{OrderID: "c", Status: "COMPLETED", CreateTime: day(-3), Items: up(8)},
	}}
	for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", true) {
		if r.OrderID == "b" && (r.PreviousSeats == nil || *r.PreviousSeats != 5) {
			t.Fatalf("a failed order compares with the history, not today's count: %v", r.PreviousSeats)
		}
	}
}

func TestMarginTieBreakIsStable(t *testing.T) {
	a, b := activeMonthly("1", "1"), activeMonthly("2", "1")
	a.Currency, b.Currency = "USD", "EUR"
	for i := 0; i < 20; i++ {
		rows := bookMargin(&ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{a, b}}, "customer")
		if len(rows) != 2 || rows[0].Currency != "EUR" {
			t.Fatalf("rows must order by currency on a tie: %+v", rows)
		}
	}
}

// Codex round 28 / adversarial round 26 regressions.

func TestRedactSIPLoginWithNumericUser(t *testing.T) {
	const s = "QZX987"
	for _, in := range []string{
		`{"attributes":[{"name":"friendlyName","value":"sip:1234:` + s + `@[::1]"}]}`,
		`{"notes":"sips:5551234:` + s + `@pbx"}`,
		`{"notes":"login 1234:` + s + `@[2001:db8::1]"}`,
	} {
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %s -> %s", in, out)
		}
	}
	// Control: a meeting time stays.
	if out := string(redactSecretsJSON([]byte(`{"notes":"Call at 10:30@Contoso"}`))); !strings.Contains(out, "10:30@Contoso") {
		t.Errorf("over-redacted a time: %s", out)
	}
}

func TestChangesTiesWithinOneOrderAndFailedAtCompletedInstant(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	prevOf := func(includeFailed bool, orders ...ionOrder) map[string][]*float64 {
		out := map[string][]*float64{}
		b := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{{ID: "s", SubscriptionID: "r", Seats: q(8)}}, Orders: orders}
		for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", includeFailed) {
			out[r.OrderID] = append(out[r.OrderID], r.PreviousSeats)
		}
		return out
	}
	one := prevOf(false,
		ionOrder{OrderID: "a", Status: "COMPLETED", CreateTime: day(-5), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(5)}}},
		ionOrder{OrderID: "b", Status: "COMPLETED", CreateTime: day(-3), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(8)}, {Action: "UPDATE", ResourceID: "r", Quantity: q(9)}}},
		ionOrder{OrderID: "c", Status: "COMPLETED", CreateTime: day(-1), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(10)}}},
	)
	for _, p := range append(one["b"], one["c"]...) {
		if p != nil {
			t.Fatalf("two items of one subscription in one order are a tie: %v", *p)
		}
	}
	failed := prevOf(true,
		ionOrder{OrderID: "a", Status: "COMPLETED", CreateTime: day(-5), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(5)}}},
		ionOrder{OrderID: "e", Status: "ERROR", CreateTime: day(-3), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(6)}}},
		ionOrder{OrderID: "c", Status: "COMPLETED", CreateTime: day(-3), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(8)}}},
	)
	if failed["e"][0] != nil {
		t.Fatalf("a failed order at a completed order's instant has no previous_seats: %v", *failed["e"][0])
	}
	if failed["c"][0] == nil || *failed["c"][0] != 5 {
		t.Fatalf("a failed order does not tie a completed one: %v", failed["c"][0])
	}
}

func TestSKUKeyPairOutranksNonMicrosoftPartNumber(t *testing.T) {
	s := ionSub{MfgPartNumber: "CFQ7TTC0LDPB", ProviderProductID: "CFQ7TTC0LDPB", ProviderSkuID: "0002"}
	if k := s.SKUKey(); k != "CFQ7TTC0LDPB:0002" {
		t.Fatalf("ION's product/SKU pair outranks a bare part number: %q", k)
	}
}

// Adversarial round 27 / Codex round 29 regressions.

func TestRedactSIPUserWithSlash(t *testing.T) {
	const s = "QZX27b"
	for _, in := range []string{
		`{"attributes":[{"name":"friendlyName","value":"sip:1234/5:` + s + `@pbx"}]}`,
		`{"notes":"voicemail sip:200/1:` + s + `@pbx"}`,
	} {
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %s -> %s", in, out)
		}
	}
	for _, keep := range []string{"Lunch 12:00@[cafe]", "Room 3:2@[B]", "sip:alice@example.com:5060", "sip:1234@pbx"} {
		b, _ := json.Marshal(map[string]string{"notes": keep})
		if out := string(redactSecretsJSON(b)); !strings.Contains(out, keep) {
			t.Errorf("over-redacted %q: %s", keep, out)
		}
	}
}

func TestChangesOpenOrderOnUnsyncedSubscriptionStatesNothing(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	up := func(v float64) []ionOrderItem {
		return []ionOrderItem{{Action: "UPDATE", ResourceID: "unsynced", Quantity: q(v)}}
	}
	b := &ionBook{Customers: map[string]ionCustomer{}, Orders: []ionOrder{
		{OrderID: "a", Status: "COMPLETED", CreateTime: day(-5), Items: up(5)},
		{OrderID: "b", Status: "COMPLETED", CreateTime: day(-3), Items: up(8)},
		{OrderID: "c", Status: "IN_PROGRESS", CreateTime: day(-3), Items: up(9)},
	}}
	for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", false) {
		if r.OrderID == "c" && r.PreviousSeats != nil {
			t.Fatalf("an open order on an unsynced subscription has no count to change from: %v", *r.PreviousSeats)
		}
	}
}

func TestTrueupRedactedIONNameIsNoKey(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "[redacted]", SKUKey: "K", Seats: 5},
		{Distributor: "pax8", SeatsKnown: true, CustomerName: "Redacted", SKUKey: "K", Seats: 7},
	}
	v := trueup(inputs, true, false)
	for _, r := range v.Rows {
		if r.TotalSeats == 12 {
			t.Fatalf("a [redacted] name must not join: %+v", r)
		}
	}
	if len(v.Unmatched) != 1 {
		t.Fatalf("the pax8 row has no customer to join: %+v", v.Unmatched)
	}
}

// Adversarial round 28 / Codex round 30 regressions.

func TestRedactSIPPasswordWithSlashAndBracketedIPv4(t *testing.T) {
	const s = "QZX28"
	for _, in := range []string{
		`{"attributes":[{"name":"friendlyName","value":"sip:1234:aB3/` + s + `@pbx"}]}`,
		`{"notes":"voicemail sip:alice:` + s + `/1@pbx"}`,
		`{"notes":"PBX login 100:` + s + `@[192.168.1.1]:5060"}`,
	} {
		if out := string(redactSecretsJSON([]byte(in))); strings.Contains(out, s) {
			t.Errorf("leaked: %s -> %s", in, out)
		}
	}
	for _, keep := range []string{"sip:1234/5@pbx", "sip:alice/bob@example.com", "Lunch 12:00@[cafe]"} {
		b, _ := json.Marshal(map[string]string{"notes": keep})
		if out := string(redactSecretsJSON(b)); !strings.Contains(out, keep) {
			t.Errorf("over-redacted %q: %s", keep, out)
		}
	}
}

func TestSubscriptionSetRefusesFractionalSeats(t *testing.T) {
	s := ionSub{ID: "s1", CCPProductID: "p", CCPSkuID: "k", CCPPlanID: "pl", SubscriptionID: "v1", Seats: optFloat{V: 1.5, Set: true}}
	if _, err := buildSubscriptionUpdate(s, subSetChange{FriendlyName: "New name"}); err == nil {
		t.Fatal("a fractional count must not be rounded into an order")
	}
	if _, err := buildSubscriptionUpdate(s, subSetChange{FriendlyName: "New name", Seats: 2, SeatsSet: true}); err != nil {
		t.Fatalf("an explicit --seats still orders: %v", err)
	}
}

func TestChangesOwnerlessSyncedSubscriptionAndSharedVendorID(t *testing.T) {
	q := func(v float64) optFloat { return optFloat{V: v, Set: true} }
	b := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{{ID: "s", SubscriptionID: "r", Seats: q(9)}}, Orders: []ionOrder{
		{OrderID: "a", CustomerID: "c1", Status: "COMPLETED", CreateTime: day(-6), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(5)}}},
		{OrderID: "b", CustomerID: "c1", Status: "COMPLETED", CreateTime: day(-5), Items: []ionOrderItem{{Action: "UPDATE", Quantity: q(7)}}},
		{OrderID: "c", CustomerID: "c2", Status: "COMPLETED", CreateTime: day(-4), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "r", Quantity: q(8)}}},
	}}
	for _, r := range subscriptionChanges(b, testNow, 30*24*time.Hour, "", false) {
		if r.OrderID == "c" && r.PreviousSeats != nil {
			t.Fatalf("an ownerless synced subscription is blinded by any customer: %v", *r.PreviousSeats)
		}
	}
	dup := &ionBook{Customers: map[string]ionCustomer{}, Subs: []ionSub{
		{ID: "a", SubscriptionID: "v", Seats: q(10), SubscriptionName: "Dup Active"},
		{ID: "b", SubscriptionID: "v", Seats: q(3), SubscriptionName: "Dup Old"},
	}, Orders: []ionOrder{{OrderID: "o", Status: "IN_PROGRESS", CreateTime: day(-1), Items: []ionOrderItem{{Action: "UPDATE", ResourceID: "v", Quantity: q(12)}}}}}
	for _, r := range subscriptionChanges(dup, testNow, 30*24*time.Hour, "", false) {
		if r.SubscriptionID != "" || r.PreviousSeats != nil {
			t.Fatalf("a vendor ID two subscriptions share matches neither: %+v", r)
		}
	}
}

func TestExplicitDBFlagCountsProfileValue(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("db", "", "")
	if explicitDBFlag(cmd) {
		t.Fatal("no --db is not explicit")
	}
	if err := cmd.Flags().Lookup("db").Value.Set("/tmp/chosen.db"); err != nil {
		t.Fatal(err)
	}
	if !explicitDBFlag(cmd) {
		t.Fatal("a db a profile applied is explicit")
	}
}

// Codex round 31 regressions.

func TestTrueupCustomerIDKeysCannotCollide(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Alpha", SKUKey: "S", Seats: 5},
		{Distributor: "pax8:eu", SeatsKnown: true, CustomerID: "C", CustomerName: "Alpha", SKUKey: "S", Seats: 7},
		{Distributor: "pax8", SeatsKnown: true, CustomerID: "eu:C", CustomerName: "Beta", SKUKey: "S", Seats: 9},
	}
	v := trueup(inputs, true, false)
	for _, r := range v.Rows {
		if r.TotalSeats == 21 {
			t.Fatalf("Beta must not join Alpha through a colon-built key: %+v", r)
		}
	}
	if len(v.Unmatched) != 1 {
		t.Fatalf("Beta has no TD SYNNEX customer: %+v", v.Unmatched)
	}
}

func TestSubscriptionSetProfileSeatsCount(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"s1","customerId":"1","subscriptionId":"v1","ccpProductId":"p","ccpSkuId":"k","ccpPlanId":"pl","subscriptionTotalLicenses":"10"}`))
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	cmd := RootCmd()
	set, _, err := cmd.Find([]string{"subscriptions", "set"})
	if err != nil {
		t.Fatal(err)
	}
	if err := set.Flags().Lookup("seats").Value.Set("12"); err != nil { // as a --profile applies it: Changed stays false
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"subscriptions", "set", "1", "1", "s1", "--renewal", "auto_on", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"quantity": 12`) {
		t.Fatalf("a profile-applied --seats must be ordered: %s", out.String())
	}
}

// Codex round 32 / adversarial round 30 regressions.

func TestTrueupKeysWithNULCannotCollide(t *testing.T) {
	inputs := []positionRow{
		{Distributor: "td-synnex", SeatsKnown: true, CustomerID: "1", CustomerName: "Alpha", SKUKey: "S", Seats: 5},
		{Distributor: "pax8\x00eu", SeatsKnown: true, CustomerID: "C", CustomerName: "Alpha", SKUKey: "S", Seats: 7},
		{Distributor: "pax8", SeatsKnown: true, CustomerID: "eu\x00C", CustomerName: "Beta", SKUKey: "S", Seats: 9},
	}
	v := trueup(inputs, true, false)
	for _, r := range v.Rows {
		if r.TotalSeats == 21 {
			t.Fatalf("Beta must not join Alpha through a NUL-built key: %+v", r)
		}
	}
}

func TestSKUKeyColonPairIsNoKey(t *testing.T) {
	a := ionSub{ProviderProductID: "A:B", ProviderSkuID: "C", CCPSkuID: "k1"}
	b := ionSub{ProviderProductID: "A", ProviderSkuID: "B:C", CCPSkuID: "k2"}
	if a.SKUKey() == b.SKUKey() {
		t.Fatalf("an ambiguous pair must not key: %q", a.SKUKey())
	}
}

func TestSeatsFlagIsDecimalAndProfileZeroRefused(t *testing.T) {
	var n int
	d := &decimalInt{v: &n}
	if err := d.Set("012"); err != nil || n != 12 || !d.set {
		t.Fatalf("012 is twelve seats: %d %v", n, err)
	}
	for _, bad := range []string{"0x0C", "0b1100", "1_2", "twelve"} {
		if (&decimalInt{v: &n}).Set(bad) == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"s1","customerId":"1","subscriptionId":"v1","ccpProductId":"p","ccpSkuId":"k","ccpPlanId":"pl","subscriptionTotalLicenses":"10"}`))
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
	cmd := RootCmd()
	set, _, _ := cmd.Find([]string{"subscriptions", "set"})
	if err := set.Flags().Lookup("seats").Value.Set("0"); err != nil { // a profile's seats=0
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"subscriptions", "set", "1", "1", "s1", "--renewal", "auto_on", "--json"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error()+out.String(), "seats") {
		t.Fatalf("a saved seats=0 must be refused like a typed one: err=%v %s", err, out.String())
	}
}
