// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package warranty

import (
	"strings"
	"testing"
)

func TestDetectVendor(t *testing.T) {
	cases := []struct {
		serial, want string
		ok           bool
	}{
		{"1abc234", Dell, true},
		{"PF1ABCDE", Lenovo, true},
		{"PF1ABCDE.21CB", Lenovo, true},
		{"C02XG0FDH7JY", Apple, true},
		{"123456789012", "", false}, // 12 digits: Microsoft Surface shape, not Apple
		{"5CG1234ABC", "", false},   // 10: Apple 2021+ or HP, ambiguous
		{"ABC", "", false},
		{"AB-1234", "", false},
	}
	for _, c := range cases {
		got, ok := DetectVendor(c.serial)
		if got != c.want || ok != c.ok {
			t.Errorf("DetectVendor(%q) = %q,%v want %q,%v", c.serial, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeVendor(t *testing.T) {
	for in, want := range map[string]string{"Dell Inc.": Dell, "LENOVO": Lenovo, "Apple Inc.": Apple, "apple": Apple} {
		if got, ok := NormalizeVendor(in); !ok || got != want {
			t.Errorf("NormalizeVendor(%q) = %q,%v", in, got, ok)
		}
	}
	if _, ok := NormalizeVendor("HP"); ok {
		t.Error("HP must not map to a vendor")
	}
}

func TestDay(t *testing.T) {
	for in, want := range map[string]string{
		"2026-03-14T04:59:59.999Z":  "2026-03-14",
		"2026-01-19T00:00:00":       "2026-01-19",
		"2026-02-02T00:00:00Z":      "2026-02-02",
		"2026-02-02T23:30:00-05:00": "2026-02-03",
		"03/04/2026":                "2026-03-04",
		"":                          "",
		"not a date":                "",
		"2026-13-45":                "",
	} {
		if got := Day(in); got != want {
			t.Errorf("Day(%q) = %q want %q", in, got, want)
		}
	}
}

const dellFixture = `[
 {"id":1,"serviceTag":"1ABC234","shipDate":"2023-03-14T05:00:00Z","productLineDescription":"LATITUDE 5430","invalid":false,"duplicated":false,
  "entitlements":[
   {"itemNumber":"997-8367","startDate":"2023-03-14T05:00:00Z","endDate":"2026-03-14T04:59:59.999Z","entitlementType":"INITIAL","serviceLevelCode":"ND","serviceLevelDescription":"ProSupport Plus for PCs","serviceLevelGroup":5},
   {"itemNumber":"997-0000","startDate":"2023-03-14T05:00:00Z","endDate":"2028-03-14T04:59:59.999Z","entitlementType":"INITIAL","serviceLevelCode":"D","serviceLevelDescription":"Dell Digital Delivery","serviceLevelGroup":8}]},
 {"id":2,"serviceTag":"0000000","invalid":true,"entitlements":[]}
]`

func TestParseDell(t *testing.T) {
	recs, err := ParseDell([]byte(dellFixture), []string{"1abc234", "0000000", "9ZZZ999"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("records = %d", len(recs))
	}
	r := recs[0]
	if r.Result != ResultOK || r.Model != "LATITUDE 5430" || r.ShipDate != "2023-03-14" || len(r.Lines) != 2 {
		t.Fatalf("record 0 = %+v", r)
	}
	sum := Summarize(r.Lines, "")
	if sum.End != "2026-03-14" {
		t.Errorf("coverage end = %q: the non-hardware D line (2028) must not set it", sum.End)
	}
	if recs[1].Result != ResultInvalid || recs[2].Result != ResultNotFound {
		t.Errorf("invalid/not-found = %q/%q", recs[1].Result, recs[2].Result)
	}
	if _, err := ParseDell([]byte(`{"fault":"x"}`), []string{"1ABC234"}); err == nil {
		t.Error("an object body must be an error, not zero records")
	}
}

func TestParseLenovo(t *testing.T) {
	one := `{"Serial":"PF1ABCDE","Product":"21CBCTO1WW","InWarranty":true,"Purchased":"2023-02-01T00:00:00","Shipped":"2023-01-20T00:00:00",
 "UpgradeUrl":"https://pcsupport.lenovo.com/x",
 "Warranty":[{"ID":"5WS0E97124","Name":"3 Year Premier Support","Type":"UPGRADE","Start":"2023-01-20T00:00:00","End":"2026-01-19T00:00:00"},
             {"ID":"1EZ","Name":"Base Warranty","Type":"BASE","Start":"2023-01-20T00:00:00","End":"2024-01-19T00:00:00"}],
 "Contract":[{"Contract":"4400123456","SLA":"NBD","Status":"Cancelled","Start":"2023-01-20T00:00:00","End":"2029-01-19T00:00:00"}]}`
	recs, err := ParseLenovo([]byte(one), []string{"pf1abcde"})
	if err != nil || len(recs) != 1 {
		t.Fatalf("recs=%v err=%v", recs, err)
	}
	r := recs[0]
	if r.Result != ResultOK || r.Model != "21CBCTO1WW" || r.PurchaseDate != "2023-02-01" || len(r.Lines) != 3 {
		t.Fatalf("record = %+v", r)
	}
	if got := Summarize(r.Lines, "").End; got != "2026-01-19" {
		t.Errorf("coverage end = %q: a cancelled contract must not extend coverage", got)
	}
	list := `[{"Serial":"AAAA1111","ErrorCode":100,"ErrorMessage":"Warranty data is not found"},
	         {"Serial":"BBBB2222","ErrorCode":101,"ErrorMessage":"multiple records"},
	         {"Serial":"CCCC3333","ErrorCode":7,"ErrorMessage":"boom"}]`
	recs, err = ParseLenovo([]byte(list), []string{"AAAA1111", "BBBB2222", "CCCC3333", "DDDD4444"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ResultNotFound, ResultAmbiguous, ResultError, ResultNotFound}
	for i, w := range want {
		if recs[i].Result != w {
			t.Errorf("serial %d result = %q want %q", i, recs[i].Result, w)
		}
	}
	if !strings.Contains(recs[2].Message, "7") {
		t.Errorf("error message = %q", recs[2].Message)
	}
}

func TestParseAppleCoverage(t *testing.T) {
	body := `{"data":[
 {"type":"appleCareCoverage","id":"C02XG0FDH7JY","attributes":{"status":"ACTIVE","paymentType":"NONE","description":"Limited Warranty","startDateTime":"2025-02-02T00:00:00Z","endDateTime":"2026-02-02T00:00:00Z","isRenewable":false,"isCanceled":false}},
 {"type":"appleCareCoverage","id":"0000000001","attributes":{"status":"ACTIVE","paymentType":"SUBSCRIPTION","description":"AppleCare+","startDateTime":"2025-04-17T00:00:00Z","endDateTime":"2027-04-17T00:00:00Z","isRenewable":true,"isCanceled":true}}],
 "links":{"self":"x"}}`
	r, err := ParseAppleCoverage("c02xg0fdh7jy", []byte(body))
	if err != nil || len(r.Lines) != 2 {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if got := Summarize(r.Lines, "").End; got != "2026-02-02" {
		t.Errorf("coverage end = %q: a canceled AppleCare+ must not extend coverage", got)
	}
	abe := `{"data":[{"id":"abe-1","attributes":{"status":"ACTIVE","paymentType":"ABE_SUBSCRIPTION","description":"AppleCare+ for Business","startDateTime":"2025-04-17T00:00:00Z","endDateTime":null}}]}`
	r, _ = ParseAppleCoverage("X", []byte(abe))
	sum := Summarize(r.Lines, "")
	if !sum.Open || sum.End != "" {
		t.Errorf("AppleCare+ for Business summary = %+v", sum)
	}
	if st, _ := Status(sum, "2026-10-28", 90); st != "covered" {
		t.Errorf("open coverage status = %q", st)
	}
	if _, err := ParseAppleCoverage("X", []byte(`{"errors":[]}`)); err == nil {
		t.Error("a body with no data array must be an error")
	}
}

func TestStatusAndEvents(t *testing.T) {
	sum := Summary{End: "2026-12-01", Fingerprint: "b"}
	if st, d := Status(sum, "2026-10-28", 90); st != "expiring" || *d != 34 {
		t.Errorf("status = %q %v", st, d)
	}
	if st, _ := Status(sum, "2026-12-02", 90); st != "expired" {
		t.Errorf("status = %q", st)
	}
	if st, _ := Status(Summary{}, "2026-10-28", 90); st != "unknown" {
		t.Errorf("empty status = %q", st)
	}
	evs := Events("1ABC234", Dell, Prior{Fingerprint: "a", End: "2026-06-01", Checked: true}, sum, "2026-10-28", 90)
	if len(evs) != 2 || evs[0].Type != EventChanged || evs[1].Type != EventExpiring {
		t.Fatalf("events = %+v", evs)
	}
	again := Events("1ABC234", Dell, Prior{Fingerprint: "b", End: "2026-12-01", Checked: true}, sum, "2026-10-29", 90)
	if len(again) != 1 || again[0].EventID != evs[1].EventID {
		t.Errorf("the expiring event id must be stable across lookups: %+v", again)
	}
	if first := Events("X", Dell, Prior{}, sum, "2026-10-28", 90); len(first) != 1 || first[0].Type != EventExpiring {
		t.Errorf("a first lookup is not a change: %+v", first)
	}
	if none := Events("X", Dell, Prior{Checked: true, Fingerprint: "b"}, Summary{End: "2030-01-01", Fingerprint: "b"}, "2026-10-28", 90); len(none) != 0 {
		t.Errorf("unchanged, far-off coverage raised %+v", none)
	}
}

func TestParseImport(t *testing.T) {
	csvText := "\ufeffDevice Name,Serial Number,Manufacturer,Model,Site\n" +
		"WS-01,1abc234,Dell Inc.,Latitude 5430,Acme Dental\n" +
		"WS-02,pf1abcde,,ThinkPad T14,Acme Dental\n" +
		"WS-03,5CG1234ABC,HP,EliteBook,Acme Dental\n" +
		"WS-04,,Dell Inc.,x,Acme Dental\n" +
		"WS-05,1ABC234,Dell Inc.,dup,Acme Dental\n" +
		"WS-06,bad serial!,Dell,x,Acme\n"
	rows, skips, err := ParseImport(strings.NewReader(csvText), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || len(skips) != 3 {
		t.Fatalf("rows=%+v skips=%+v", rows, skips)
	}
	if rows[0].Serial != "1ABC234" || rows[0].Vendor != Dell || rows[0].VendorSource != SourceDeclared || rows[0].Customer != "Acme Dental" || rows[0].Hostname != "WS-01" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].Vendor != Lenovo || rows[1].VendorSource != SourceDetected {
		t.Errorf("row 1 = %+v", rows[1])
	}
	if rows[2].Vendor != "" {
		t.Errorf("HP row must keep no vendor: %+v", rows[2])
	}
	jsonText := `{"devices":[{"serialNumber":"C02XG0FDH7JY","manufacturer":"Apple Inc.","client":"Beta LLC"}]}`
	rows, _, err = ParseImport(strings.NewReader(jsonText), "", "Override Co")
	if err != nil || len(rows) != 1 || rows[0].Vendor != Apple || rows[0].Customer != "Override Co" {
		t.Fatalf("json rows=%+v err=%v", rows, err)
	}
	if _, _, err := ParseImport(strings.NewReader("name,model\nx,y\n"), "csv", ""); err == nil {
		t.Error("a CSV with no serial column must be an error")
	}
}

// Apple documents startDateTime as when coverage commenced, and AppleCare+ for
// Business can have no end. Input: lines judged before they start. Nothing covers
// the device yet; after the start the same lines do.
func TestCoverageNotBeforeItsStart(t *testing.T) {
	open := []Line{{LineID: "a", Kind: KindAppleCare, Start: "2025-04-17", Hardware: true, Active: true}}
	if s := Summarize(open, "2024-01-01"); s.Open || s.End != "" {
		t.Errorf("open-ended coverage counted before its start: %+v", s)
	}
	if st, _ := Status(Summarize(open, "2024-01-01"), "2024-01-01", 90); st == "covered" {
		t.Errorf("status before the start is covered")
	}
	if s := Summarize(open, "2025-05-01"); !s.Open {
		t.Errorf("open-ended coverage not counted after its start: %+v", s)
	}
	dated := []Line{{LineID: "d", Start: "2027-01-01", End: "2030-01-01", Hardware: true, Active: true}}
	if s := Summarize(dated, "2026-06-01"); s.End != "" {
		t.Errorf("a line starting later set the coverage end: %+v", s)
	}
	if s := Summarize(dated, "2027-06-01"); s.End != "2030-01-01" {
		t.Errorf("a started line did not set the coverage end: %+v", s)
	}
}

// Dell sells an EXTENDED line that starts the day INITIAL ends, and Lenovo an
// upgrade that starts the day after base warranty ends. Input: back-to-back lines
// judged before the second one starts. The device is covered to the last end;
// only a gap before a future line leaves it ending at the earlier date.
func TestBackToBackCoverageChains(t *testing.T) {
	cases := []struct {
		name  string
		lines []Line
		want  Summary
	}{
		{"dell same day", []Line{
			{LineID: "i", Start: "2025-11-20", End: "2027-01-15", Hardware: true, Active: true},
			{LineID: "e", Start: "2027-01-15", End: "2029-01-15", Hardware: true, Active: true}}, Summary{End: "2029-01-15"}},
		{"lenovo next day", []Line{
			{LineID: "b", Start: "2024-01-01", End: "2026-12-01", Hardware: true, Active: true},
			{LineID: "u", Start: "2026-12-02", End: "2028-12-01", Hardware: true, Active: true},
			{LineID: "v", Start: "2028-12-02", End: "2029-06-01", Hardware: true, Active: true}}, Summary{End: "2029-06-01"}},
		{"applecare next day", []Line{
			{LineID: "l", Start: "2024-01-01", End: "2026-12-01", Hardware: true, Active: true},
			{LineID: "a", Kind: KindAppleCare, Start: "2026-12-02", Hardware: true, Active: true}}, Summary{End: "2026-12-01", Open: true}},
		{"gap", []Line{
			{LineID: "b", Start: "2024-01-01", End: "2026-12-01", Hardware: true, Active: true},
			{LineID: "g", Start: "2027-03-01", End: "2029-01-01", Hardware: true, Active: true}}, Summary{End: "2026-12-01"}},
		{"inactive future", []Line{
			{LineID: "b", Start: "2024-01-01", End: "2026-12-01", Hardware: true, Active: true},
			{LineID: "x", Start: "2026-12-02", End: "2029-01-01", Hardware: true}}, Summary{End: "2026-12-01"}},
	}
	for _, c := range cases {
		s := Summarize(c.lines, "2026-11-04")
		if s.End != c.want.End || s.Open != c.want.Open {
			t.Errorf("%s: got end %q open %v, want end %q open %v", c.name, s.End, s.Open, c.want.End, c.want.Open)
		}
	}
}
