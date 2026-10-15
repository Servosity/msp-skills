// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package qw

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParsePage(t *testing.T) {
	list := `{"data":[{"type":"DocumentHeaders","id":"G1","attributes":{"DocNo":"1042","GrandTotal":1234.56}}],"meta":{"total":1,"ignored_filters":["Total"]}}`
	p, err := ParsePage([]byte(list))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Data) != 1 || p.Data[0].ID != "G1" {
		t.Fatalf("data = %+v", p.Data)
	}
	if got := NumOr(p.Data[0].Attributes["GrandTotal"], 0); got != 1234.56 {
		t.Fatalf("GrandTotal = %v", got)
	}
	if got := IgnoredFilters(p.Meta); !reflect.DeepEqual(got, []string{"Total"}) {
		t.Fatalf("ignored = %v", got)
	}
	single := `{"data":{"type":"DocumentItems","id":"I1","attributes":null}}`
	p, err = ParsePage([]byte(single))
	if err != nil || len(p.Data) != 1 || p.Data[0].Attributes == nil {
		t.Fatalf("single parse = %+v, %v", p, err)
	}
	if flat := p.Data[0].Flat(); flat["id"] != "I1" {
		t.Fatalf("flat = %v", flat)
	}
	empty, err := ParsePage([]byte(`{"data":[]}`))
	if err != nil || len(empty.Data) != 0 {
		t.Fatalf("empty = %+v %v", empty, err)
	}
	numeric, err := ParsePage([]byte(`{"data":[{"type":"Payments","id":42,"attributes":{"Amount":10}}]}`))
	if err != nil || numeric.Data[0].ID != "42" {
		t.Fatalf("numeric id = %+v %v", numeric, err)
	}
	bare, err := ParsePage([]byte(`[{"RecGUID":"G9","DocNo":"7"}]`))
	if err != nil || len(bare.Data) != 1 || bare.Data[0].ID != "G9" || bare.Data[0].Attributes["DocNo"] != "7" {
		t.Fatalf("bare array = %+v %v", bare, err)
	}
	if _, err := ParsePage([]byte(`{"data":"nope"}`)); err == nil {
		t.Fatal("expected error for scalar data")
	}
	if _, err := ParsePage([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestIgnoredFiltersShapes(t *testing.T) {
	cases := []struct {
		meta map[string]any
		want []string
	}{
		{nil, nil},
		{map[string]any{"ignored_filters": []any{}}, nil},
		{map[string]any{"ignored_filters": []any{map[string]any{"name": "Company", "reason": "unknown column"}}}, []string{"Company (unknown column)"}},
		{map[string]any{"ignored_filters": "Total"}, []string{"Total"}},
	}
	for _, c := range cases {
		if got := IgnoredFilters(c.meta); !reflect.DeepEqual(got, c.want) {
			t.Errorf("IgnoredFilters(%v) = %v, want %v", c.meta, got, c.want)
		}
	}
}

func TestCondAndOr(t *testing.T) {
	c := Cond("DocType", "eq", "QUOTE")
	if c["val"] != "QUOTE" || c["op"] != "eq" {
		t.Fatalf("cond = %v", c)
	}
	if _, has := Cond("ConvertedRef", "is_null", nil)["val"]; has {
		t.Fatal("is_null must not carry val")
	}
	b, _ := json.Marshal(Or(Cond("A", "eq", 1), Cond("B", "eq", 2)))
	if string(b) != `{"or":[{"name":"A","op":"eq","val":1},{"name":"B","op":"eq","val":2}]}` {
		t.Fatalf("or = %s", b)
	}
}

func TestNumAndStr(t *testing.T) {
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{json.Number("12.50"), 12.5, true},
		{"7", 7, true},
		{3.0, 3, true},
		{nil, 0, false},
		{"abc", 0, false},
		{true, 0, false},
	}
	for _, c := range cases {
		got, ok := Num(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Num(%v) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
	if Str(json.Number("1042")) != "1042" || Str(nil) != "" || Str("  x ") != "x" {
		t.Fatal("Str mismatch")
	}
}

func TestLineTypes(t *testing.T) {
	cases := map[any]string{nil: "product", 1.0: "product", 2.0: "comment", 8.0: "group_header", 0.0: "unknown(0)"}
	for in, want := range cases {
		if got := LineTypeName(in); got != want {
			t.Errorf("LineTypeName(%v) = %q want %q", in, got, want)
		}
	}
	if !IsProductLine(map[string]any{}) || !IsProductLine(map[string]any{"LineType": 1.0}) || IsProductLine(map[string]any{"LineType": 2.0}) {
		t.Fatal("IsProductLine mismatch")
	}
}

func TestMarginMath(t *testing.T) {
	if m, ok := MarginPct(100, 82); !ok || m != 18 {
		t.Fatalf("margin = %v %v", m, ok)
	}
	if _, ok := MarginPct(0.01, 1e306); ok {
		t.Fatal("a non-finite margin must not be reported")
	}
	if _, ok := MarginPct(1, 1e306); ok {
		t.Fatal("a margin that overflows when rounded must not be reported")
	}
	if q, ok := EffectiveQty(map[string]any{"QtyBase": 2.0, "QtyMultiplier1": 3.0}); !ok || q != 6 {
		t.Fatalf("EffectiveQty multipliers = %v %v", q, ok)
	}
	if q, ok := EffectiveQty(map[string]any{"QtyBase": 2.0, "QtyTotal": 12.0}); !ok || q != 12 {
		t.Fatalf("EffectiveQty prefers QtyTotal = %v %v", q, ok)
	}
	if _, ok := EffectiveQty(map[string]any{}); ok {
		t.Fatal("no quantity must be unknown")
	}
	for _, inactive := range []map[string]any{{"TemplateType": 1.0}, {"Superceeded": true}, {"Superceeded": 1.0}, {"IsAutoRecoverDocForDocRecGUID": "G"}} {
		if IsActiveDocument(inactive) {
			t.Fatalf("%v must be inactive", inactive)
		}
	}
	if !IsActiveDocument(map[string]any{"TemplateType": 0.0, "Superceeded": false}) || !IsActiveDocument(map[string]any{}) {
		t.Fatal("ordinary documents are active")
	}
	if _, ok := MarginPct(0, 5); ok {
		t.Fatal("zero price must not produce a margin")
	}
	p, err := PriceForMargin(82, 18)
	if err != nil || p != 100 {
		t.Fatalf("PriceForMargin = %v %v", p, err)
	}
	if _, err := PriceForMargin(10, 100); err == nil {
		t.Fatal("100% margin must error")
	}
	if got := PriceForMarkup(80, 25); got != 100 {
		t.Fatalf("markup = %v", got)
	}
}

func TestProductDBAndTableNames(t *testing.T) {
	cases := map[string]string{"Products_AllProducts_Products": "AllProducts", "products_myproducts_products": "myproducts"}
	for in, want := range cases {
		if got, ok := ProductDB(in); !ok || got != want {
			t.Errorf("ProductDB(%q) = %q,%v", in, got, ok)
		}
	}
	for _, bad := range []string{"DocumentHeaders", "Products", "Products__Products"} {
		if _, ok := ProductDB(bad); ok {
			t.Errorf("ProductDB(%q) should not match", bad)
		}
	}
	p := Page{Data: []Resource{{ID: "DocumentHeaders"}, {ID: "x", Attributes: map[string]any{"name": "Products_A_Products"}}, {ID: "documentheaders"}}}
	if got := TableNames(p); !reflect.DeepEqual(got, []string{"DocumentHeaders", "Products_A_Products", "x"}) {
		t.Fatalf("TableNames = %v", got)
	}
}

func TestParseDateAndBuckets(t *testing.T) {
	for _, s := range []string{"2026-09-14T00:00:00", "2026-09-14", "2026-09-14T10:00:00Z"} {
		if d, ok := ParseDate(s); !ok || d.Year() != 2026 || d.Month() != time.September {
			t.Errorf("ParseDate(%q) = %v %v", s, d, ok)
		}
	}
	if _, ok := ParseDate("nope"); ok {
		t.Fatal("bad date parsed")
	}
	if FilterDate(time.Date(2026, 1, 2, 15, 0, 0, 0, time.UTC)) != "2026-01-02T00:00:00" {
		t.Fatal("FilterDate format")
	}
	for days, want := range map[int]string{0: "0-7", 8: "8-30", 45: "31-60", 90: "61-90", 400: "90+"} {
		if got := AgeBucket(days); got != want {
			t.Errorf("AgeBucket(%d) = %s want %s", days, got, want)
		}
	}
}

func TestClassifier(t *testing.T) {
	c := NewClassifier("Won, Ordered", "Dead")
	cases := []struct {
		attrs        map[string]any
		want, signal string
	}{
		{map[string]any{"DocStatus": "won"}, OutcomeWon, "DocStatus"},
		{map[string]any{"DocStatus": "Dead", "QuoteValetStatusCode": 1.0}, OutcomeLost, "DocStatus"},
		{map[string]any{"ConvertedRef": "1041", "QuoteValetStatusCode": 3.0}, OutcomeLost, "QuoteValetStatusCode"},
		{map[string]any{"QuoteValetStatusCode": 2.0}, OutcomeWon, "QuoteValetStatusCode"},
		{map[string]any{"QuoteValetStatusCode": 3.0}, OutcomeLost, "QuoteValetStatusCode"},
		{map[string]any{"QuoteValetStatusCode": 0.0}, OutcomeOpen, "QuoteValetStatusCode"},
		{map[string]any{}, OutcomeOpen, "none"},
	}
	for _, tc := range cases {
		got, sig := c.Classify(tc.attrs)
		if got != tc.want || sig != tc.signal {
			t.Errorf("Classify(%v) = %s/%s want %s/%s", tc.attrs, got, sig, tc.want, tc.signal)
		}
	}
	if QuoteValetLabel(1.0) != "accepted" || QuoteValetLabel(nil) != "" {
		t.Fatal("QuoteValetLabel mismatch")
	}
}

func TestProductIndexMatch(t *testing.T) {
	products := []Product{
		ProductFromAttrs("Products_A_Products", "1", map[string]any{"Manufacturer": "Lenovo", "ManufacturerPartNumber": " 20ues0x100 ", "Cost": 900.0, "Price": 1100.0}),
		ProductFromAttrs("Products_A_Products", "s", map[string]any{"Manufacturer": "Lenovo", "ManufacturerPartNumber": "20UE S0X100", "Cost": 1.0, "Price": 2.0}),
		ProductFromAttrs("Products_B_Products", "2", map[string]any{"Manufacturer": "Lenovo", "ManufacturerPartNumber": "20ues0x100", "Cost": 880.0, "Price": 1080.0}),
		ProductFromAttrs("Products_B_Products", "3", map[string]any{"Manufacturer": "Other", "ManufacturerPartNumber": "20UES0X100", "Cost": 10.0}),
		ProductFromAttrs("Products_A_Products", "4", map[string]any{"VendorPartNumber": "V-77", "Cost": 5.0}),
	}
	idx := NewProductIndex(products)
	p, ok := idx.Match("Lenovo", "20UES0X100", "")
	if !ok || p.ID != "2" || p.DB != "B" {
		t.Fatalf("match = %+v %v (want cheapest same-manufacturer row 2)", p, ok)
	}
	if p.HasList || !products[0].HasCost {
		t.Fatalf("HasList/HasCost tracking: %+v", p)
	}
	if f := ProductFromAttrs("Products_A_Products", "f", map[string]any{"Cost": 90.0, "Price": 120.0, "UnitOfPricingFactor": 0.1}); f.HasCost || f.HasPrice || !f.PricingFactor {
		t.Fatalf("a per-box catalog price must not read as per item: %+v", f)
	}
	if m := ProductFromAttrs("Products_A_Products", "m", map[string]any{"Cost": 80.0, "Price": 100.0, "UnitOfPricingFactor": 1.0, "UnitOfMeasureFactor": 12.0}); !m.HasCost || !m.HasPrice {
		t.Fatalf("a unit of measure factor (shipping weight) must not hide prices: %+v", m)
	}
	if b, _ := json.Marshal(ProductFromAttrs("Products_A_Products", "f", map[string]any{"Cost": 500.0, "UnitOfPricingFactor": 100.0})); !strings.Contains(string(b), `"Cost":null`) || !strings.Contains(string(b), `"pricing_factor":true`) {
		t.Fatalf("unknown catalog amounts must serialize as null: %s", b)
	}
	if zero := ProductFromAttrs("Products_A_Products", "z", map[string]any{"Cost": 0.0, "Price": 0.0}); zero.HasCost || zero.HasPrice {
		t.Fatalf("zero catalog values must read as unknown: %+v", zero)
	}
	zidx := NewProductIndex([]Product{ProductFromAttrs("Products_A_Products", "z", map[string]any{"ManufacturerPartNumber": "Q", "Cost": 0.0}), ProductFromAttrs("Products_B_Products", "r", map[string]any{"ManufacturerPartNumber": "Q", "Cost": 7.0})})
	if p, _ := zidx.Match("", "Q", ""); p.ID != "r" {
		t.Fatalf("a zero-cost row must not win lowest cost: %+v", p)
	}
	if p, ok := idx.Match("", "", "v-77"); !ok || p.ID != "4" {
		t.Fatalf("vendor fallback = %+v %v", p, ok)
	}
	if _, ok := idx.Match("Lenovo", "NOPE", ""); ok {
		t.Fatal("unexpected match")
	}
}

func TestLint(t *testing.T) {
	idx := NewProductIndex([]Product{ProductFromAttrs("Products_A_Products", "p1", map[string]any{"ManufacturerPartNumber": "X1", "Cost": 50.0, "Price": 80.0})})
	header := map[string]any{"SoldToCompany": "Acme Dental", "SubTotal": 500.0}
	lines := []map[string]any{
		{"id": "i1", "LineType": 1.0, "ManufacturerPartNumber": "X1", "UnitPrice": 40.0, "UnitCost": 50.0, "QtyBase": 1.0, "ExtendedPrice": 40.0},
		{"id": "i2", "LineType": 1.0, "ManufacturerPartNumber": "X1", "UnitPrice": 80.0, "UnitCost": 0.0, "QtyBase": 0.0, "ExtendedPrice": 0.0},
		{"id": "i3", "LineType": 2.0, "Description": "comment line"},
		{"id": "i4", "LineType": 1.0, "Description": "Labor", "UnitPrice": 100.0, "UnitCost": 95.0, "QtyBase": 1.0, "ExtendedPrice": 100.0},
	}
	fs := Lint(header, lines, idx, 10)
	rules := map[string]string{}
	for _, f := range fs {
		rules[f.Rule+"/"+f.ItemID] = f.Severity
	}
	want := map[string]string{
		"price-below-cost/i1":    SevError,
		"zero-cost/i2":           SevError,
		"bad-quantity/i2":        SevError,
		"low-margin/i4":          SevWarning,
		"missing-part-number/i4": SevWarning,
		"subtotal-differs/":      SevInfo,
	}
	for k, sev := range want {
		if rules[k] != sev {
			t.Errorf("missing %s (%s); got %v", k, sev, rules)
		}
	}
	for k := range rules {
		if k == "low-margin/i3" || k == "zero-cost/i3" {
			t.Errorf("comment line must not be linted: %s", k)
		}
	}
	clean := Lint(map[string]any{"SoldToCompany": "Acme"}, []map[string]any{{"id": "ok", "ManufacturerPartNumber": "X1", "UnitPrice": 80.0, "UnitCost": 50.0, "QtyBase": 2.0}}, idx, 10)
	if CountSeverity(clean, SevError) != 0 || CountSeverity(clean, SevWarning) != 0 {
		t.Fatalf("clean quote produced findings: %+v", clean)
	}
	zeroQty := Lint(map[string]any{"SoldToCompany": "Acme"}, []map[string]any{{"id": "z", "ManufacturerPartNumber": "X", "QtyBase": 1.0, "QtyMultiplier1": 0.0, "QtyTotal": 0.0, "UnitPrice": 100.0, "UnitCost": 80.0}}, nil, 10)
	if CountSeverity(zeroQty, SevError) != 1 {
		t.Fatalf("a zero effective quantity is an error: %+v", zeroQty)
	}
	if counts, known := TotalsContribution(map[string]any{"LineAttributes": 16.0}); counts || !known {
		t.Fatal("an unselected option never counts")
	}
	if counts, known := TotalsContribution(map[string]any{"LineAttributes": 272.0}); !counts || !known {
		t.Fatal("a selected option (16+256) counts")
	}
	if counts, known := TotalsContribution(map[string]any{"LineAttributes": 128.0}); counts || known {
		t.Fatal("a recurring line's contribution is unknown")
	}
	if counts, known := TotalsContribution(map[string]any{"LineAttributes": 17.0}); counts || !known {
		t.Fatal("Exclude wins over Option")
	}
	if counts, known := TotalsContribution(map[string]any{"LineAttributes": 1.0}); counts || !known {
		t.Fatal("an excluded line never counts")
	}
	if counts, known := TotalsContribution(map[string]any{"LineAttributes": 2.0}); !counts || !known {
		t.Fatal("a hidden-price line still counts")
	}
	edge := Lint(map[string]any{"SoldToCompany": "Acme"}, []map[string]any{{"id": "e", "ManufacturerPartNumber": "Z", "UnitPrice": 100.0, "UnitCost": 90.004, "QtyBase": 1.0}}, nil, 10)
	if CountSeverity(edge, SevWarning) != 1 {
		t.Fatalf("a 9.996%% margin is below 10%%: %+v", edge)
	}
	long := Lint(map[string]any{"SoldToCompany": "This Company Name Is Definitely Longer Than Fifty Characters Inc"}, nil, nil, 10)
	if CountSeverity(long, SevError) != 1 {
		t.Fatalf("long customer: %+v", long)
	}
}

func TestDiffDocs(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	prev := map[string]map[string]any{
		"a": {"id": "a", "DocNo": "1", "DocStatus": "Open", "QuoteValetStatusCode": 0.0, "GrandTotal": 100.0, "SalesRep": "Ann"},
		"b": {"id": "b", "DocNo": "2", "DocStatus": "Open", "GrandTotal": 50.0},
		"c": {"id": "c", "DocNo": "3", "DocStatus": "Open"},
	}
	cur := []map[string]any{
		{"id": "a", "DocNo": "1", "DocStatus": "Won", "QuoteValetStatusCode": 1.0, "GrandTotal": 120.0, "SalesRep": "Bob"},
		{"id": "b", "DocNo": "2", "DocStatus": "Open", "GrandTotal": 50.0},
		{"id": "e", "DocNo": "5", "DocStatus": "Open"},
	}
	kinds := func(evs []ChangeEvent) map[string]bool {
		m := map[string]bool{}
		for _, e := range evs {
			m[e.DocID+":"+e.Kind] = true
		}
		return m
	}
	got := kinds(DiffDocs(prev, cur, true, at, nil))
	want := []string{"a:accepted", "a:status", "a:value", "a:rep", "e:new", "c:removed"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	if got["b:status"] || got["b:value"] || len(got) != len(want) {
		t.Errorf("unexpected events %v", got)
	}
	if partial := kinds(DiffDocs(prev, cur, false, at, nil)); partial["c:removed"] {
		t.Error("removed must not be reported for a partial fetch")
	}
	if v := kinds(DiffDocs(map[string]map[string]any{"v": {"id": "v", "GrandTotal": 100.0}}, []map[string]any{{"id": "v"}}, false, at, nil)); !v["v:value"] {
		t.Error("a total that became null is a value change")
	}
	if cents := kinds(DiffDocs(map[string]map[string]any{"c": {"id": "c", "GrandTotal": 100.004}}, []map[string]any{{"id": "c", "GrandTotal": 100.006}}, false, at, nil)); !cents["c:value"] {
		t.Error("100.00 -> 100.01 is a value change")
	}
	if same := kinds(DiffDocs(map[string]map[string]any{"c": {"id": "c", "GrandTotal": 100.006}}, []map[string]any{{"id": "c", "GrandTotal": 100.014}}, false, at, nil)); same["c:value"] {
		t.Error("100.01 -> 100.01 is not a value change")
	}
	if scoped := kinds(DiffDocs(prev, cur, false, at, func(d map[string]any) bool { return d["id"] != "e" })); scoped["e:new"] {
		t.Error("a document outside the previous mirror's scope is not new")
	}
	if len(DiffDocs(prev, []map[string]any{{"id": "b", "DocStatus": "Open", "GrandTotal": 50.0}}, false, at, nil)) != 0 {
		t.Error("unchanged doc must produce no events")
	}
}
