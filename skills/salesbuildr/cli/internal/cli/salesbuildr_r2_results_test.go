// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// documented-zero-read-as-missing, the mirror case (absent read as zero),
// closed for the class with gnumOK presence: ProductResponseDto does not
// require price or cost, so a catalog product without a cost is unknown, not
// a cost of 0; pricing drift reported every book cost as drift against it.

import "testing"

func TestR2PricingDriftComparesOnlyNumbersBothSidesCarry(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"product", "p1", `{"id":"p1","name":"No cost on file","price":1000}`},
		{"product", "p2", `{"id":"p2","name":"Nothing on file"}`},
		{"product", "p3", `{"id":"p3","name":"Drifted","price":100,"cost":50}`},
		{"pricing-book", "pb1", `{"id":"pb1","name":"Book","products":[{"productId":"p1","price":1000,"cost":900},{"productId":"p2","price":10,"cost":5},{"productId":"p3","price":90,"cost":50}]}`},
	})
	report := runNovel(t, newNovelPricingDriftCmd, "--db", dbPath)
	rows := report["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (only p3 drifted; p1 has no catalog cost, p2 nothing): %#v", len(rows), rows)
	}
	row := rows[0].(map[string]any)
	if row["productId"] != "p3" || row["priceDelta"].(float64) != -10 {
		t.Fatalf("row = %#v, want p3 with priceDelta -10", row)
	}
	if n, _ := report["incomparable"].(float64); n != 1 {
		t.Fatalf("incomparable = %v, want 1 (p2): %#v", report["incomparable"], report)
	}
}

func TestR2WhitespaceKeepsAnUnknownPriceUnknown(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"company", "c1", `{"id":"c1","name":"Acme"}`},
		{"product", "p1", `{"id":"p1","name":"No price on file"}`},
		{"product", "p2", `{"id":"p2","name":"Free tier","price":0}`},
		{"product", "p3", `{"id":"p3","name":"Priced","price":100}`},
	})
	report := runNovel(t, newNovelCompanyWhitespaceCmd, "c1", "--db", dbPath)
	rows := report["whitespace"].([]any)
	if len(rows) != 3 {
		t.Fatalf("whitespace = %d rows, want 3: %#v", len(rows), rows)
	}
	var order []string
	for _, r := range rows {
		order = append(order, r.(map[string]any)["id"].(string))
	}
	if order[0] != "p3" || order[1] != "p2" || order[2] != "p1" {
		t.Fatalf("order = %v, want p3, p2, p1 (an explicit 0 is a price; no price sorts last)", order)
	}
	if price, ok := rows[2].(map[string]any)["price"]; !ok || price != nil {
		t.Fatalf("p1 price = %#v (present %v), want null: no price on file is unknown, not 0", price, ok)
	}
	if price := rows[1].(map[string]any)["price"]; price != float64(0) {
		t.Fatalf("p2 price = %#v, want 0: an explicit 0 stays a value", price)
	}
}
