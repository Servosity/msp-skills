// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: tests for ion_mcp_file_flags.go and explicitDBFlag (ion_book.go).

package cli

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"td-synnex-pp-cli/internal/client"
)

// TestPathFlagsNeverReachMCP fails if a command that reads or writes a file
// through a flag does not list that flag in mcp:write-flags.
func TestPathFlagsNeverReachMCP(t *testing.T) {
	root := RootCmd()
	for path, flags := range map[string][]string{
		"teach":                  {"playbook-file", "playbook-notes-file"},
		"teach-playbook":         {"notes-file", "playbook-file"},
		"position trueup":        {"with", "billed", "db"},
		"margin":                 {"db"},
		"subscriptions renewals": {"db"},
	} {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd == root {
			t.Fatalf("%s not found: %v", path, err)
		}
		listed := map[string]bool{}
		for _, n := range strings.Split(cmd.Annotations["mcp:write-flags"], ",") {
			listed[strings.TrimSpace(n)] = true
		}
		for _, f := range flags {
			if !listed[f] {
				t.Errorf("%s --%s is not kept off MCP (mcp:write-flags=%q)", path, f, cmd.Annotations["mcp:write-flags"])
			}
		}
	}
}

// TestExplicitDBFlag pins the check root.go uses to skip the default-store
// auto-refresh when a command names its own store.
func TestExplicitDBFlag(t *testing.T) {
	root := RootCmd()
	cmd, _, err := root.Find([]string{"margin"})
	if err != nil {
		t.Fatal(err)
	}
	if explicitDBFlag(cmd) {
		t.Fatal("--db reported explicit before it was set")
	}
	if err := cmd.Flags().Set("db", "/tmp/book.db"); err != nil {
		t.Fatal(err)
	}
	if !explicitDBFlag(cmd) {
		t.Fatal("--db set but not reported explicit")
	}
}

// TestMarginLeavesUnknownCostsOut fails if a subscription with no reported
// cost is counted at a cost of 0 in margin or margin_pct.
func TestMarginLeavesUnknownCostsOut(t *testing.T) {
	b := &ionBook{Customers: map[string]ionCustomer{"1": {Name: "accounts/9/customers/1", Organization: "A"}}, Subs: []ionSub{
		{ID: "s1", CustomerID: "1", Status: "active", BillingCycle: "monthly", Price: optFloat{V: 100, Set: true}, Cost: optFloat{V: 80, Set: true}},
		{ID: "s2", CustomerID: "1", Status: "active", BillingCycle: "monthly", Price: optFloat{V: 50, Set: true}},
	}}
	rows := bookMargin(b, "customer")
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	r := rows[0]
	if r.Margin != 20 || r.MarginPct == nil || *r.MarginPct != 20 || r.PriceUnknownCost != 50 || r.MonthlyPrice != 150 || r.MonthlyCost != 80 {
		t.Fatalf("unknown cost leaked into margin: %+v pct=%v", r, r.MarginPct)
	}
}

// TestMarginTrendLeavesUnknownCostsOut is the --months twin of
// TestMarginLeavesUnknownCostsOut.
func TestMarginTrendLeavesUnknownCostsOut(t *testing.T) {
	pinNow(t)
	b := &ionBook{Customers: map[string]ionCustomer{"1": {Name: "accounts/9/customers/1", Organization: "A"}}, Subs: []ionSub{
		{ID: "s1", CustomerID: "1", Status: "active", BillingCycle: "monthly", BillingData: []ionBillingRow{
			{BillingMonth: "2026-09", CustomerCost: optFloat{V: 100, Set: true}, SellerCost: optFloat{V: 80, Set: true}},
			{BillingMonth: "2026-09", CustomerCost: optFloat{V: 50, Set: true}},
		}},
	}}
	rows := marginTrend(b, "customer", 1)
	if len(rows) != 1 || rows[0].Margin != 20 || rows[0].CustomerCostCostUnknown != 50 || rows[0].CostComplete {
		t.Fatalf("unknown seller cost leaked into the trend margin: %+v", rows)
	}
}

// TestArgsNameDB: the post-run flag-correction pass must see --db even when
// flag parsing failed (Codex review round 4, P1).
func TestArgsNameDB(t *testing.T) {
	for args, want := range map[string]bool{
		"margin --months nope --db /tmp/x.db": true, "margin --db=/tmp/x.db": true,
		"margin --months 6": false, "margin --select -- --months nope --db /x.db": true, "margin --dbx 1": false,
	} {
		if got := argsNameDB(strings.Fields(args)); got != want {
			t.Errorf("argsNameDB(%q) = %v, want %v", args, got, want)
		}
	}
}

// TestOAuthErrorEchoesOnlyTheCode: error_description is free text a server can
// fill with the token it was sent (Codex review round 4, P1).
func TestOAuthErrorEchoesOnlyTheCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"invalid refresh_token: SENTINEL-RT","message":"SENTINEL-RT"}`))
	}))
	defer srv.Close()
	_, status, err := postIONForm(context.Background(), srv.Client(), srv.URL+"/oauth/token", url.Values{"refresh_token": {"SENTINEL-RT"}}, 0)
	if status != 400 || err == nil || strings.Contains(err.Error(), "SENTINEL") || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

// TestDeliverWebhookRefusesPlainHTTP: --deliver can carry a minted token or
// customer data, so plain http is allowed only to loopback (Codex round 5, P1).
func TestDeliverWebhookRefusesPlainHTTP(t *testing.T) {
	for target, ok := range map[string]bool{
		"webhook:http://example.com/receive": false, "webhook:http://10.0.0.5/x": false,
		"webhook:https://example.com/receive": true, "webhook:http://127.0.0.1:9/x": true, "webhook:http://localhost/x": true,
	} {
		if _, err := ParseDeliverSink(target); (err == nil) != ok {
			t.Errorf("ParseDeliverSink(%q) err=%v, want ok=%v", target, err, ok)
		}
	}
}

// TestDeliverWebhookRefusesRedirectToPlainHTTP: an allowed URL that redirects
// to plain http off loopback must not receive the body (Codex round 6, P1).
func TestDeliverWebhookRefusesRedirectToPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.255.255.1/in", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	if err := deliverWebhook(srv.URL, []byte(`{"access_token":"x"}`), false); err == nil || !strings.Contains(err.Error(), "refusing webhook redirect") {
		t.Fatalf("redirect to plain http was followed: %v", err)
	}
}

// TestSubscriptionUpdateRefusesRedactedIDs: a redacted ID must never be copied
// into an order.
func TestSubscriptionUpdateRefusesRedactedIDs(t *testing.T) {
	s := ionSub{ID: "1", CCPProductID: "p", CCPSkuID: redacted, CCPPlanID: "pl", SubscriptionID: "v", Seats: optFloat{V: 2, Set: true}}
	if _, err := buildSubscriptionUpdate(s, subSetChange{SeatsSet: true, Seats: 3}); err == nil || !strings.Contains(err.Error(), "ccpSkuId") {
		t.Fatalf("redacted SKU ID accepted: %v", err)
	}
}

// TestNormalizeCompanyKeepsEveryScript: true-up joins on the normalized name,
// so two customers whose names differ only in non-ASCII letters must not merge
// (Codex round 9, P1); fullwidth and ASCII spellings of one name still match.
func TestNormalizeCompanyKeepsEveryScript(t *testing.T) {
	if a, b := normalizeCompany("北京 Inc"), normalizeCompany("東京 Inc"); a == b {
		t.Fatalf("distinct customers merged: %q == %q", a, b)
	}
	if a, b := normalizeCompany("किरण Inc"), normalizeCompany("कुरण Inc"); a == b {
		t.Fatalf("names differing in a vowel sign merged: %q", a)
	}
	if a, b := normalizeCompany("Café Müller GmbH"), normalizeCompany("Cafe Muller GmbH"); a == b {
		t.Fatalf("accented and plain names merged: %q", a)
	}
	for _, pair := range [][2]string{{"SilverLeaf, Inc.", "silverleaf inc"}, {"Ｃｏｎｔｏｓｏ Ltd", "Contoso"}} {
		if a, b := normalizeCompany(pair[0]), normalizeCompany(pair[1]); a != b {
			t.Errorf("%q and %q should match: %q vs %q", pair[0], pair[1], a, b)
		}
	}
}

// TestPositionFileRefusesColumnCollisionAndSkipsInactive: two columns mapped to
// one name are refused (the pick was random), and cancelled rows are rejected
// because the TD SYNNEX side counts only active subscriptions (adversarial
// round 10, P2).
func TestPositionFileRefusesColumnCollisionAndSkipsInactive(t *testing.T) {
	if _, _, err := parsePositionFile("x.csv", []byte("company,sku,seats,quantity\nContoso,SKU1,8,11\n"), map[string]string{"company": "customer_name", "sku": "sku_key", "quantity": "seats"}); err == nil {
		t.Fatal("column collision accepted")
	}
	if _, _, err := parsePositionFile("x.csv", []byte("customer_name,sku_key,seats,seats\nAcme,K,5,50\n"), nil); err == nil {
		t.Fatal("duplicate header accepted")
	}
	rows, rejected, err := parsePositionFile("pax8.csv", []byte("company,sku,quantity,status\nContoso,SKU1,4,Cancelled (refunded)\nFabrikam,SKU1,3,Active\nNorthwind,SKU1,2,Pending Cancellation\n"), map[string]string{"company": "customer_name", "sku": "sku_key", "quantity": "seats"})
	if err != nil || len(rows) != 2 || rows[0].CustomerName != "Fabrikam" || len(rejected) != 1 {
		t.Fatalf("rows=%+v rejected=%v err=%v", rows, rejected, err)
	}
}

// TestDryRunBodyFilterIsTheIONRedactor: the client prints a --dry-run body
// only through client.DryRunBodyFilter, which this package sets to the ION
// redactor at init (Codex review round 13, P1).
func TestDryRunBodyFilterIsTheIONRedactor(t *testing.T) {
	const sentinel = "SENTINEL-Hunter2-9f3a-4417"
	if client.DryRunBodyFilter == nil {
		t.Fatal("client.DryRunBodyFilter is not registered")
	}
	out := string(client.DryRunBodyFilter([]byte(`{"orderItems":[{"attributes":[{"name":"adminPassword","value":"` + sentinel + `"}]},{"attributes":[{"name":"tenantDomain","value":"acme.onmicrosoft.com"}]}]}`)))
	if strings.Contains(out, sentinel) || !strings.Contains(out, "acme.onmicrosoft.com") {
		t.Fatalf("dry-run filter: %s", out)
	}
}

// TestOAuthErrorCodeIsAClosedList: a shape rule (lowercase and underscores)
// echoed any "error" value, so a server could return the token it was sent
// as the code (Codex review round 13, P1).
func TestOAuthErrorCodeIsAClosedList(t *testing.T) {
	const token = "testsecrettoken"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"` + token + `","error_description":"` + token + `"}`))
	}))
	defer srv.Close()
	_, status, err := postIONForm(context.Background(), srv.Client(), srv.URL+"/oauth/token", url.Values{"access_token": {token}}, 0)
	if status != 400 || err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if !strings.Contains(err.Error(), "Bad Request") {
		t.Fatalf("unlisted code must fall back to the HTTP status text: %v", err)
	}
}

// TestOAuthTransportErrorNeverEchoesTheServer: net/http puts a malformed
// header line into its error, so a server could echo the token it was sent
// through the transport error (Codex review round 14, P1).
func TestOAuthTransportErrorNeverEchoesTheServer(t *testing.T) {
	const token = "testsecrettoken"
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
			// No colon: net/http rejects the line and quotes it in its error.
			_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nBad token " + token + "\r\nContent-Length: 0\r\n\r\n"))
			conn.Close()
		}
	}()
	_, status, err := postIONForm(context.Background(), &http.Client{}, "http://"+ln.Addr().String()+"/oauth/token", url.Values{"access_token": {token}}, 0)
	if err == nil || status != 0 || strings.Contains(err.Error(), token) {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if !strings.Contains(err.Error(), "transport failure") {
		t.Fatalf("malformed response must be named by kind: %v", err)
	}
	_, _, err = postIONForm(context.Background(), &http.Client{}, "http://127.0.0.1:1/oauth/token", url.Values{"access_token": {token}}, 0)
	if err == nil || !strings.Contains(err.Error(), "connection failed: connection refused") {
		t.Fatalf("refused connection must keep its errno kind: %v", err)
	}
}

// TestAnnualPriceWithoutCostUsesBilledCost: an annual subscription with a
// price but no cost takes the seller cost its billed rows carry instead of
// reporting the cost unknown (adversarial round 12 and 13, P2).
// An annual subscription price without a cost keeps the price and leaves the
// cost unknown: the billed rows' cost is never mixed with the subscription's
// price (adversarial round 15, P1).
func TestAnnualPriceWithoutCostUsesBilledCost(t *testing.T) {
	pinNow(t)
	sub := ionSub{ID: "s1", Status: "active", BillingCycle: "annual", Price: optFloat{V: 1200, Set: true}, BillingData: []ionBillingRow{
		{BillingMonth: "2026-09", CustomerCost: optFloat{V: 1200, Set: true}, SellerCost: optFloat{V: 1080, Set: true}},
	}}
	if m := sub.MonthlyMoney(); m.Price != 100 || !m.PriceKnown || m.CostKnown || m.Basis != "subscription" {
		t.Fatalf("money = %+v", m)
	}
}

// TestZeroAnnualPriceIsAPrice: a price ION sent, zero included, is the
// subscription basis; only a missing price falls through to the billed rows
// (Codex round 15, P1).
func TestZeroAnnualPriceIsAPrice(t *testing.T) {
	pinNow(t)
	var sent, missing ionSub
	billing := `"billingData":[{"billingMonth":"2026-09","customerCost":1200,"sellerCost":900,"currency":"USD"}]`
	if err := json.Unmarshal([]byte(`{"id":"1","subscriptionBillingCycle":"annual","price":0,"currency":"USD",`+billing+`}`), &sent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"id":"1","subscriptionBillingCycle":"annual","currency":"USD",`+billing+`}`), &missing); err != nil {
		t.Fatal(err)
	}
	if m := sent.MonthlyMoney(); m.Price != 0 || !m.PriceKnown || m.Basis != "subscription" || m.CostKnown {
		t.Fatalf("explicit zero price: %+v", m)
	}
	if m := missing.MonthlyMoney(); m.Price != 100 || m.Basis != "billing:2025-10..2026-09" {
		t.Fatalf("missing price: %+v", m)
	}
}
