// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// fakePager serves a PagedDTO collection like JSM: honours start/limit, caps
// the page at forceLimit, and (like the customers endpoint) can lie with
// isLastPage=true on every page.
type fakePager struct {
	total      int
	forceLimit int
	alwaysLast bool
	calls      int
	dupShift   int // repeat this many items from the previous page (queue drift)
}

func (f *fakePager) GetWithHeadersNoCache(_ context.Context, _ string, p map[string]string, h map[string]string) (json.RawMessage, error) {
	if h["X-ExperimentalApi"] != "opt-in" {
		return nil, fmt.Errorf("412: X-ExperimentalApi header missing")
	}
	f.calls++
	start, _ := strconv.Atoi(p["start"])
	limit, _ := strconv.Atoi(p["limit"])
	if f.forceLimit > 0 && limit > f.forceLimit {
		limit = f.forceLimit
	}
	from := start - f.dupShift
	if from < 0 {
		from = 0
	}
	vals := []map[string]string{}
	for i := from; i < start+limit && i < f.total; i++ {
		vals = append(vals, map[string]string{"key": fmt.Sprintf("K-%d", i)})
	}
	body := map[string]any{"start": start, "limit": limit, "size": len(vals), "isLastPage": f.alwaysLast || start+limit >= f.total, "values": vals}
	return json.Marshal(body)
}

func TestJSMWalkPagesPastLyingIsLastPage(t *testing.T) {
	f := &fakePager{total: 120, forceLimit: 50, alwaysLast: true}
	items, truncated, err := jsmWalk(context.Background(), f, "/servicedesk/1/customer", nil, 0, jsonKey("key"))
	if err != nil || truncated {
		t.Fatalf("err=%v truncated=%v", err, truncated)
	}
	if len(items) != 120 {
		t.Fatalf("got %d items, want 120 (walker trusted isLastPage or stopped on the capped page)", len(items))
	}
	if f.calls != 3 {
		t.Fatalf("calls = %d, want 3 (50+50+20, stop on short page)", f.calls)
	}
}

func TestJSMWalkDedupesDrift(t *testing.T) {
	f := &fakePager{total: 120, dupShift: 2}
	items, _, err := jsmWalk(context.Background(), f, "/q", nil, 0, jsonKey("key"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 120 {
		t.Fatalf("got %d, want 120 unique items despite overlapping pages", len(items))
	}
}

func TestJSMWalkMaxPagesTruncates(t *testing.T) {
	f := &fakePager{total: 500}
	items, truncated, err := jsmWalk(context.Background(), f, "/q", nil, 2, nil)
	if err != nil || !truncated || len(items) != 100 {
		t.Fatalf("items=%d truncated=%v err=%v, want 100 true nil", len(items), truncated, err)
	}
}

func TestJSMWalkEmpty(t *testing.T) {
	f := &fakePager{total: 0}
	items, truncated, err := jsmWalk(context.Background(), f, "/q", nil, 0, nil)
	if err != nil || truncated || len(items) != 0 || items == nil {
		t.Fatalf("empty walk: items=%v truncated=%v err=%v", items, truncated, err)
	}
}

func cyc(remaining int64, breached, paused bool) *jsmSLACycle {
	return &jsmSLACycle{Breached: breached, Paused: paused, RemainingTime: jsmDuration{Millis: remaining}}
}

func TestMostUrgent(t *testing.T) {
	slas := []jsmSLA{
		{Name: "TTR", OngoingCycle: cyc(5000, false, false)},
		{Name: "TTFR", OngoingCycle: cyc(-10, true, false)},
		{Name: "Paused", OngoingCycle: cyc(-99999, true, true)},
		{Name: "Done"},
	}
	r, ok := mostUrgent(slas)
	if !ok || r.Metric != "TTFR" || !r.Breached {
		t.Fatalf("mostUrgent = %+v, want running breached TTFR over paused", r)
	}
	if _, ok := mostUrgent([]jsmSLA{{Name: "Done"}}); ok {
		t.Fatal("no ongoing cycle must report not found")
	}
	r, ok = mostUrgent([]jsmSLA{{Name: "P", OngoingCycle: cyc(10, false, true)}})
	if !ok || !r.Paused {
		t.Fatalf("paused-only should still be reported (caller filters): %+v", r)
	}
}

func req(key, desk, rtype, reporter, cat string, slas ...jsmSLA) localRequest {
	r := localRequest{IssueKey: key, ServiceDeskID: flexString(desk), RequestTypeID: flexString(rtype)}
	r.Reporter.AccountID = reporter
	r.CurrentStatus.Status = "Open"
	r.CurrentStatus.StatusCategory = cat
	r.SLA.Values = slas
	return r
}

func done(stopMS, elapsed, goal int64, breached bool) jsmSLA {
	return jsmSLA{Name: "TTFR", CompletedCycles: []jsmSLACycle{{Breached: breached, StopTime: &jsmDate{EpochMillis: stopMS}, ElapsedTime: jsmDuration{Millis: elapsed}, GoalDuration: jsmDuration{Millis: goal}}}}
}

func TestSLARollup(t *testing.T) {
	reqs := []localRequest{
		req("A-1", "1", "10", "u1", "DONE", done(1000, 10, 60, false)),
		req("A-2", "1", "11", "u1", "DONE", done(1000, 90, 60, true)),
		req("A-3", "2", "10", "u2", "NEW", done(1000, 30, 60, false)),
		req("A-4", "2", "10", "u2", "NEW", done(10, 99, 60, true)), // before cutoff
	}
	g := slaRollup(reqs, 500, "desk", map[string]string{"1": "Desk One"})
	if len(g) != 2 || g[0].Group != "1" || g[0].BreachRate != 0.5 || g[0].Name != "Desk One" || g[0].MedianElapsedMS != 50 {
		t.Fatalf("desk rollup = %+v", g)
	}
	if g[1].Cycles != 1 || g[1].Breached != 0 {
		t.Fatalf("cutoff not applied: %+v", g[1])
	}
	byType := slaRollup(reqs, 500, "requesttype", nil)
	if len(byType) != 2 || byType[0].Group != "11" || byType[0].BreachRate != 1 {
		t.Fatalf("requesttype rollup = %+v", byType)
	}
	if len(slaRollup(nil, 0, "metric", nil)) != 0 {
		t.Fatal("empty input must give empty rollup")
	}
}

func TestClientRollup(t *testing.T) {
	breach := jsmSLA{Name: "TTR", OngoingCycle: cyc(-1, true, false)}
	r1 := req("A-1", "1", "10", "u1", "NEW", breach)
	r1.CreatedDate.EpochMillis = 1000
	r2 := req("A-2", "1", "11", "u1", "INDETERMINATE")
	r2.CreatedDate.EpochMillis = 500
	r3 := req("A-3", "1", "10", "u1", "DONE")
	r4 := req("A-4", "1", "10", "stranger", "NEW", breach)
	p := clientRollup("9", "Acme", map[string]bool{"u1": true}, []localRequest{r1, r2, r3, r4}, map[string]string{"10": "Incident"}, 500+7_200_000)
	if p.Requests != 3 || p.Open != 2 || p.OngoingBreached != 1 || p.OldestOpen != "A-2" || p.OldestOpenAgeH != 2 {
		t.Fatalf("client rollup = %+v", p)
	}
	if p.ByRequestType["Incident"] != 1 || p.ByRequestType["11"] != 1 {
		t.Fatalf("request type names = %+v", p.ByRequestType)
	}
	if len(p.BreachedKeys) != 1 || p.BreachedKeys[0] != "A-1" {
		t.Fatalf("breached keys = %v (stranger's ticket must not count)", p.BreachedKeys)
	}
}

func row(line int, kv ...string) csvRow {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return csvRow{Line: line, Fields: m}
}

func TestReconcileDiff(t *testing.T) {
	objs := []reconcileObject{
		{ObjectKey: "IT-1", Label: "srv-01", Attrs: map[string]string{"Name": "srv-01", "Serial": "SN1"}},
		{ObjectKey: "IT-2", Label: "srv-02", Attrs: map[string]string{"Name": "SRV-02", "Serial": "SN2"}},
		{ObjectKey: "IT-3", Label: "srv-03", Attrs: map[string]string{"Name": "srv-03"}},
	}
	rows := []csvRow{
		row(2, "name", "srv-01", "serial", "sn1"),
		row(3, "name", "srv-02", "serial", "SNX"),
		row(4, "name", "laptop-9", "serial", "SN9"),
	}
	v := reconcileDiff("Server", objs, rows, "Name", "Name", []string{"Serial"}, false)
	if v.Matched != 2 || len(v.MissingInAssets) != 1 || v.MissingInAssets[0].Key != "laptop-9" || v.MissingInAssets[0].Line != 4 {
		t.Fatalf("matching = %+v", v)
	}
	if len(v.MissingInInventory) != 1 || v.MissingInInventory[0].ObjectKey != "IT-3" {
		t.Fatalf("missing in inventory = %+v", v.MissingInInventory)
	}
	if len(v.Mismatches) != 1 || v.Mismatches[0].ObjectKey != "IT-2" || v.Mismatches[0].Inventory != "SNX" {
		t.Fatalf("mismatches = %+v (case-insensitive equal values must not mismatch)", v.Mismatches)
	}
}

func TestReconcileDiffKeepsDuplicatesAndBlanks(t *testing.T) {
	objs := []reconcileObject{
		{ObjectKey: "IT-1", Attrs: map[string]string{"Name": "srv-01", "OS": "Win11"}},
		{ObjectKey: "IT-9", Attrs: map[string]string{"Name": "srv-01", "OS": "Win10"}},
		{ObjectKey: "IT-5", Attrs: map[string]string{"Name": ""}},
	}
	rows := []csvRow{row(2, "Name", "srv-01", "OS", "Win11"), row(3, "Name", "srv-01", "OS", "Win11"), row(4, "Name", "", "OS", "x")}
	v := reconcileDiff("Laptop", objs, rows, "Name", "Name", []string{"OS"}, false)
	if len(v.DuplicateAssetsKeys) != 1 || len(v.DuplicateAssetsKeys[0].ObjectKeys) != 2 {
		t.Fatalf("duplicate assets keys = %+v", v.DuplicateAssetsKeys)
	}
	if len(v.DuplicateInventoryKeys) != 1 || len(v.DuplicateInventoryKeys[0].Lines) != 2 {
		t.Fatalf("duplicate inventory keys = %+v", v.DuplicateInventoryKeys)
	}
	if len(v.AssetsWithoutKey) != 1 || v.AssetsWithoutKey[0] != "IT-5" || len(v.InventoryLinesWithoutKey) != 1 || v.InventoryLinesWithoutKey[0] != 4 {
		t.Fatalf("blank keys = %v / %v", v.AssetsWithoutKey, v.InventoryLinesWithoutKey)
	}
	// IT-9 (Win10) differs from both inventory rows: the second object is not silently ignored.
	if len(v.Mismatches) != 2 || v.Mismatches[0].ObjectKey != "IT-9" {
		t.Fatalf("mismatches = %+v", v.Mismatches)
	}
}

func TestReconcileDiffTruncatedIsUnverified(t *testing.T) {
	v := reconcileDiff("Laptop", nil, []csvRow{row(2, "Name", "srv-77")}, "Name", "Name", nil, true)
	if len(v.MissingInAssets) != 0 || len(v.UnverifiedInventory) != 1 {
		t.Fatalf("truncated scan must not assert missing: %+v", v)
	}
}

func TestReadCSVRows(t *testing.T) {
	h, rows, err := readCSVRows(strings.NewReader("\ufeffName,Serial\nsrv-01, SN1 \n\"srv,02\",SN2\n"), ".csv")
	if err != nil || len(h) != 2 || h[0] != "Name" || len(rows) != 2 || rows[0].Fields["Serial"] != "SN1" || rows[1].Fields["Name"] != "srv,02" || rows[1].Line != 3 {
		t.Fatalf("h=%v rows=%+v err=%v", h, rows, err)
	}
	if _, _, err := readCSVRows(strings.NewReader("Name,Serial\nsrv-01\n"), ".csv"); err == nil {
		t.Fatal("short row must be rejected")
	}
	if _, _, err := readCSVRows(strings.NewReader("Name,name\nx,y\n"), ".csv"); err == nil {
		t.Fatal("duplicate header must be rejected")
	}
	if _, _, err := readCSVRows(strings.NewReader(""), ".csv"); err == nil {
		t.Fatal("empty file must be rejected")
	}
	if h, rows, err := readCSVRows(strings.NewReader("Name\tSerial\nsrv-01\tSN1\n"), ".tsv"); err != nil || len(h) != 2 || rows[0].Fields["Serial"] != "SN1" {
		t.Fatalf("tsv: h=%v rows=%v err=%v", h, rows, err)
	}
	if h, rows, err := readCSVRows(strings.NewReader("Name\tModel\nsrv-01\t24\" Monitor\n"), ".txt"); err != nil || len(h) != 2 || rows[0].Fields["Model"] != "24\" Monitor" {
		t.Fatalf("txt sniffed as tab with a bare quote: h=%v rows=%v err=%v", h, rows, err)
	}
	if h, _, err := readCSVRows(strings.NewReader("Name,Serial\nsrv-01,SN1\n"), ".txt"); err != nil || len(h) != 2 {
		t.Fatalf("txt sniffed as comma: h=%v err=%v", h, err)
	}
}

func TestAllFailed(t *testing.T) {
	boom := fmt.Errorf("401")
	if allFailed(nil, 0) != nil || allFailed([]fetchFailure{{ID: "a", err: boom}}, 2) != nil {
		t.Fatal("partial or empty failure must not be fatal")
	}
	if err := allFailed([]fetchFailure{{ID: "a", err: boom}, {ID: "b", err: boom}}, 2); err != boom {
		t.Fatalf("all failed = %v, want the underlying error", err)
	}
}

func TestCmdbLookupAQL(t *testing.T) {
	cases := map[string]string{
		"ITAM-12":       `Key = "ITAM-12" OR Name = "ITAM-12"`,
		"srv-01":        `Name = "srv-01"`,
		`weird "name"`:  `Name = "weird \"name\""`,
		"laptop-99":     `Name = "laptop-99"`,
		"ABC-":          `Name = "ABC-"`,
		"SD3-1":         `Key = "SD3-1" OR Name = "SD3-1"`,
		"SRV-01":        `Key = "SRV-01" OR Name = "SRV-01"`,
		"mixedCase-123": `Name = "mixedCase-123"`,
	}
	for in, want := range cases {
		if got := cmdbLookupAQL(in); got != want {
			t.Errorf("cmdbLookupAQL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFlattenObject(t *testing.T) {
	o := assetsObject{ObjectKey: "IT-1", Label: "srv-01", Attributes: []assetsAttribute{
		{ObjectTypeAttributeID: "100", ObjectAttributeValues: []assetsAttrValue{{DisplayValue: "srv-01"}}},
		{ObjectTypeAttributeID: "101", ObjectAttributeValues: []assetsAttrValue{{DisplayValue: "a"}, {Value: 7.0}}},
		{ObjectTypeAttributeID: "999"},
	}}
	got := flattenObject(o, map[string]string{"100": "Name", "101": "Tags"})
	if got["Name"] != "srv-01" || got["Tags"] != "a, 7" || got["Key"] != "IT-1" || got["Label"] != "srv-01" {
		t.Fatalf("flatten = %v", got)
	}
	if _, ok := got["attr:999"]; !ok {
		t.Fatalf("unnamed attribute must keep a stable key: %v", got)
	}
}

func TestFlexString(t *testing.T) {
	var v struct {
		A flexString `json:"a"`
		B flexString `json:"b"`
		C flexString `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":"x","b":42,"c":null}`), &v); err != nil || v.A != "x" || v.B != "42" || v.C != "" {
		t.Fatalf("flexString = %+v err=%v", v, err)
	}
}
