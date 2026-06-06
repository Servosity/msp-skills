// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.
// Tests for hand-written novel-feature helpers.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUnwrapData(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"envelope", `{"data":[{"a":1},{"a":2}],"totalItems":2}`, 2},
		{"bare array", `[{"a":1}]`, 1},
		{"results key", `{"results":[{"x":1},{"y":2},{"z":3}]}`, 3},
		{"single object", `{"deviceId":5}`, 1},
		{"empty", ``, 0},
		{"empty data", `{"data":[]}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unwrapData(json.RawMessage(tt.in))
			if len(got) != tt.want {
				t.Fatalf("unwrapData(%s) = %d items, want %d", tt.in, len(got), tt.want)
			}
		})
	}
}

func TestFirstFieldAndExtra(t *testing.T) {
	obj := decodeObj([]byte(`{"deviceId":42,"_extra":{"deviceName":"WEB01","customerName":"Acme"}}`))
	if got := asString(firstField(obj, "deviceId", "device_id")); got != "42" {
		t.Errorf("deviceId = %q, want 42", got)
	}
	if got := asString(extraField(obj, "deviceName")); got != "WEB01" {
		t.Errorf("extra deviceName = %q, want WEB01", got)
	}
	if got := asString(extraField(obj, "customerName")); got != "Acme" {
		t.Errorf("extra customerName = %q, want Acme", got)
	}
	if v := firstField(obj, "missing"); v != nil {
		t.Errorf("missing field should be nil, got %v", v)
	}
}

func TestAsInt(t *testing.T) {
	obj := decodeObj([]byte(`{"n":7,"s":"12","f":3.0,"bad":"x"}`))
	if n, ok := asInt(firstField(obj, "n")); !ok || n != 7 {
		t.Errorf("n = %d,%v want 7,true", n, ok)
	}
	if n, ok := asInt(firstField(obj, "s")); !ok || n != 12 {
		t.Errorf("s = %d,%v want 12,true", n, ok)
	}
	if n, ok := asInt(firstField(obj, "f")); !ok || n != 3 {
		t.Errorf("f = %d,%v want 3,true", n, ok)
	}
	if _, ok := asInt(firstField(obj, "bad")); ok {
		t.Errorf("bad should not parse")
	}
}

func TestHostFromBaseURL(t *testing.T) {
	tests := map[string]string{
		"https://acme.ncod.n-able.com/api": "acme.ncod.n-able.com",
		"https://ncod.n-able.com/api":      "ncod.n-able.com",
		"":                                 "",
		"not a url with spaces":            "not a url with spaces",
	}
	for in, want := range tests {
		if got := hostFromBaseURL(in); got != want {
			t.Errorf("hostFromBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestServerLabelForDB(t *testing.T) {
	tests := map[string]string{
		"/path/to/server-a.db": "server-a",
		"data.db":              "data",
		"/x/mirror":            "mirror",
	}
	for in, want := range tests {
		if got := serverLabelForDB(in); got != want {
			t.Errorf("serverLabelForDB(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinPath(t *testing.T) {
	if got := joinPath("srv", "", "Acme", "Site1"); got != "srv > Acme > Site1" {
		t.Errorf("joinPath skip-empty = %q", got)
	}
	if got := joinPath("", "", ""); got != "" {
		t.Errorf("joinPath all-empty = %q, want empty", got)
	}
	if got := joinPath("only"); got != "only" {
		t.Errorf("joinPath single = %q", got)
	}
}

func TestParseDateFlag(t *testing.T) {
	if d, ok := parseDateFlag("2026-06-15"); !ok || d.Year() != 2026 || d.Month() != 6 || d.Day() != 15 {
		t.Errorf("parseDateFlag valid = %v,%v", d, ok)
	}
	if _, ok := parseDateFlag(""); ok {
		t.Errorf("empty should fail")
	}
	if _, ok := parseDateFlag("06/15/2026"); ok {
		t.Errorf("wrong format should fail")
	}
}

func TestParseFlexibleTime(t *testing.T) {
	obj := decodeObj([]byte(`{"rfc":"2026-06-15T10:00:00Z","date":"2026-06-15","epoch":1750000000,"ms":1750000000000,"bad":"nope"}`))
	if _, ok := parseFlexibleTime(firstField(obj, "rfc")); !ok {
		t.Errorf("rfc3339 should parse")
	}
	if _, ok := parseFlexibleTime(firstField(obj, "date")); !ok {
		t.Errorf("date should parse")
	}
	tEpoch, ok := parseFlexibleTime(firstField(obj, "epoch"))
	if !ok || tEpoch.Year() != 2025 {
		t.Errorf("epoch seconds = %v,%v", tEpoch, ok)
	}
	tMs, ok := parseFlexibleTime(firstField(obj, "ms"))
	if !ok || tMs.Year() != 2025 {
		t.Errorf("epoch ms = %v,%v", tMs, ok)
	}
	if _, ok := parseFlexibleTime(firstField(obj, "bad")); ok {
		t.Errorf("bad should not parse")
	}
}

func TestEqualFoldTrim(t *testing.T) {
	if !equalFoldTrim(" Backup Plan ", "backup plan") {
		t.Errorf("should match case-insensitively after trim")
	}
	if equalFoldTrim("a", "b") {
		t.Errorf("should not match different strings")
	}
}

// --- triage aggregation ---

func TestTriageAggregateAndSort(t *testing.T) {
	issues := []string{
		`{"deviceId":1,"serviceName":"DiskSpace","notificationState":5,"_extra":{"customerName":"Acme","deviceName":"WEB01"}}`,
		`{"deviceId":1,"serviceName":"CPU","notificationState":2,"_extra":{"customerName":"Acme","deviceName":"WEB01"}}`,
		`{"deviceId":2,"serviceName":"DiskSpace","notificationState":3,"_extra":{"customerName":"Beta","deviceName":"DB01"}}`,
	}
	groups := map[string]*triageGroup{}
	for _, raw := range issues {
		triageAggregate(groups, "customer", json.RawMessage(raw))
	}
	out := triageSort(groups)
	if len(out) != 2 {
		t.Fatalf("want 2 customer groups, got %d", len(out))
	}
	// Acme has the highest severity (5), so it must sort first.
	if out[0].Group != "Acme" || out[0].Count != 2 || out[0].TopSeverity != 5 {
		t.Errorf("group0 = %+v, want Acme count2 sev5", out[0])
	}
	// Within Acme, the sev-5 issue should be first.
	if triageSeverity(out[0].Issues[0]) != 5 {
		t.Errorf("Acme issues not sorted by severity desc")
	}
	if out[1].Group != "Beta" || out[1].Count != 1 {
		t.Errorf("group1 = %+v, want Beta count1", out[1])
	}
}

func TestTriageAggregateByMonitor(t *testing.T) {
	groups := map[string]*triageGroup{}
	triageAggregate(groups, "monitor", json.RawMessage(`{"serviceName":"DiskSpace","notificationState":4}`))
	triageAggregate(groups, "monitor", json.RawMessage(`{"serviceName":"DiskSpace","notificationState":1}`))
	out := triageSort(groups)
	if len(out) != 1 || out[0].Group != "DiskSpace" || out[0].Count != 2 || out[0].TopSeverity != 4 {
		t.Errorf("monitor group = %+v", out)
	}
}

func TestTriageAggregateUnknownKey(t *testing.T) {
	groups := map[string]*triageGroup{}
	triageAggregate(groups, "customer", json.RawMessage(`{"deviceId":9,"notificationState":1}`))
	out := triageSort(groups)
	if len(out) != 1 || out[0].Group != "(unknown)" {
		t.Errorf("missing customer should group under (unknown), got %+v", out)
	}
}

// --- props value resolution ---

func TestPropsFindValue(t *testing.T) {
	resp := `{"data":[
		{"name":"Backup Plan","value":"Daily"},
		{"propertyName":"AssetTag","value":""},
		{"name":"Tags","values":["a","b"]}
	]}`
	if v, ok := propsFindValue([]byte(resp), "Backup Plan"); !ok || v != "Daily" {
		t.Errorf("Backup Plan = %q,%v want Daily,true", v, ok)
	}
	if v, ok := propsFindValue([]byte(resp), "assettag"); !ok || v != "" {
		t.Errorf("AssetTag present-but-empty = %q,%v want \"\",true", v, ok)
	}
	if v, ok := propsFindValue([]byte(resp), "Tags"); !ok || v != "a, b" {
		t.Errorf("Tags values = %q,%v want 'a, b',true", v, ok)
	}
	if _, ok := propsFindValue([]byte(resp), "DoesNotExist"); ok {
		t.Errorf("absent property should return found=false")
	}
}

// --- maintenance coverage ---

func TestMaintHasCoveringWindow(t *testing.T) {
	before := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	covered := `{"data":[{"startTime":"2026-06-10T02:00:00Z"}]}`
	if !maintHasCoveringWindow([]byte(covered), before) {
		t.Errorf("window before cutoff should be covered")
	}
	after := `{"data":[{"startTime":"2026-07-01T02:00:00Z"}]}`
	if maintHasCoveringWindow([]byte(after), before) {
		t.Errorf("window after cutoff should NOT be covered")
	}
	none := `{"data":[]}`
	if maintHasCoveringWindow([]byte(none), before) {
		t.Errorf("no windows should NOT be covered")
	}
	// Unparseable date but window present -> treated as covered.
	unparseable := `{"data":[{"label":"weekly"}]}`
	if !maintHasCoveringWindow([]byte(unparseable), before) {
		t.Errorf("present window with no parseable date should be treated as covered")
	}
}

// --- guardian OK-error scan ---

func TestGuardianScanOKError(t *testing.T) {
	if msg := guardianScanOKError(json.RawMessage(`{"error":"boom"}`)); msg != "boom" {
		t.Errorf("error field = %q, want boom", msg)
	}
	if msg := guardianScanOKError(json.RawMessage(`{"errorMessage":"bad token"}`)); msg != "bad token" {
		t.Errorf("errorMessage = %q", msg)
	}
	if msg := guardianScanOKError(json.RawMessage(`{"version":"2024.6","name":"server"}`)); msg != "" {
		t.Errorf("clean body should yield no error, got %q", msg)
	}
	// "message" alone is benign.
	if msg := guardianScanOKError(json.RawMessage(`{"message":"hello"}`)); msg != "" {
		t.Errorf("bare message should be benign, got %q", msg)
	}
	// "message" with a failure status is an error.
	if msg := guardianScanOKError(json.RawMessage(`{"message":"nope","status":"error"}`)); msg != "nope" {
		t.Errorf("message+status:error = %q, want nope", msg)
	}
	if msg := guardianScanOKError(json.RawMessage(`{"message":"failed","success":false}`)); msg != "failed" {
		t.Errorf("message+success:false = %q, want failed", msg)
	}
}

func TestFanoutResourceTypeAndName(t *testing.T) {
	dev := decodeObj([]byte(`{"deviceId":1,"longName":"WEB01"}`))
	if got := fanoutResourceType(dev); got != "device" {
		t.Errorf("device type = %q", got)
	}
	if got := fanoutDisplayName(dev); got != "WEB01" {
		t.Errorf("device name = %q", got)
	}
	cust := decodeObj([]byte(`{"customerId":5,"customerName":"Acme"}`))
	if got := fanoutResourceType(cust); got != "customer" {
		t.Errorf("customer type = %q", got)
	}
	if got := fanoutDisplayName(cust); got != "Acme" {
		t.Errorf("customer name = %q", got)
	}
}

func TestFanoutResultMarshal(t *testing.T) {
	r := fanoutResult{Server: "acme", ResourceType: "device", record: map[string]any{"longName": "WEB01", "deviceId": 7}}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["server"] != "acme" || out["resourceType"] != "device" || out["longName"] != "WEB01" {
		t.Errorf("marshaled fanout result = %v", out)
	}
}
