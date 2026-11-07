// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `report vulnerabilities`: row parsing, the per-customer
// rollup, and end-to-end runs against a local ESET Connect stand-in.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// Realistic group tree: an MSP with two COMPANY customers, one with a site.
const (
	reportVulnsTestMSP   = "0b6f6a7e-1f3a-4c55-9b0e-2d1c4e5f6a70"
	reportVulnsTestAcme  = "1c7a8b9d-2e4f-4a6b-8c0d-3e5f7a9b1c2d"
	reportVulnsTestHQ    = "2d8b9cae-3f50-4b7c-9d1e-4f6a8b0c2d3e"
	reportVulnsTestPlumb = "3e9cadbf-4061-4c8d-ae2f-5a7b9c1d3e4f"
	reportVulnsTestDev1  = "a1b2c3d4-0001-4e5f-8a9b-0c1d2e3f4a51"
	reportVulnsTestDev2  = "a1b2c3d4-0002-4e5f-8a9b-0c1d2e3f4a52"
	reportVulnsTestDev3  = "a1b2c3d4-0003-4e5f-8a9b-0c1d2e3f4a53"
)

func reportVulnsTestGroups() []esetGroup {
	return []esetGroup{
		{UUID: reportVulnsTestMSP, Name: "Servosity MSP", EntityType: "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{UUID: reportVulnsTestAcme, Name: "Acme Dental", ParentUUID: reportVulnsTestMSP, EntityType: esetCompanyEntityType},
		{UUID: reportVulnsTestHQ, Name: "HQ", ParentUUID: reportVulnsTestAcme, EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{UUID: reportVulnsTestPlumb, Name: "Acme Plumbing", ParentUUID: reportVulnsTestMSP, EntityType: esetCompanyEntityType},
	}
}

// reportVulnsTestRows: CVE-2024-3400 on three devices across two customers
// (two Acme Dental devices in different groups, one Acme Plumbing device),
// plus an OS CVE and a package CVE. riskScore arrives both as a protobuf
// int64 string and as a number.
var reportVulnsTestRows = []string{
	`{"deviceUuid":"` + reportVulnsTestDev1 + `","deviceGroupUuid":"` + reportVulnsTestHQ + `","availablePatchUuids":["p-1"],
	  "applicationVulnerability":{"application":{"displayName":"Mozilla Firefox","version":{"name":"115.0.2"}},
	  "cveNumber":"CVE-2024-3400","riskScore":"92","severity":"SEVERITY_LEVEL_HIGH","patchAvailable":true,"firstDetectTime":"2026-09-01T10:00:00Z"}}`,
	`{"deviceUuid":"` + reportVulnsTestDev2 + `","deviceGroupUuid":"` + reportVulnsTestAcme + `",
	  "applicationVulnerability":{"application":{"displayName":"Mozilla Firefox","version":{"major":"114","minor":"0","patch":"1"}},
	  "cveNumber":"CVE-2024-3400","riskScore":88,"severity":"SEVERITY_LEVEL_HIGH","patchAvailable":false}}`,
	`{"deviceUuid":"` + reportVulnsTestDev3 + `","deviceGroupUuid":"` + reportVulnsTestPlumb + `",
	  "applicationVulnerability":{"application":{"displayName":"Mozilla Firefox"},
	  "cveNumber":"CVE-2024-3400","riskScore":"75","severity":"SEVERITY_LEVEL_MEDIUM"}}`,
	`{"deviceUuid":"` + reportVulnsTestDev1 + `","deviceGroupUuid":"` + reportVulnsTestHQ + `",
	  "osVulnerability":{"osFamilyId":"1","cveNumber":"CVE-2025-21333","riskScore":"64","severity":"SEVERITY_LEVEL_MEDIUM","patchAvailable":true}}`,
	`{"deviceUuid":"` + reportVulnsTestDev3 + `","deviceGroupUuid":"` + reportVulnsTestPlumb + `",
	  "packageVulnerability":{"package":{"name":"openssl","versionName":"3.0.2-0ubuntu1.10"},
	  "cveNumber":"CVE-2023-0286","riskScore":"81","severity":"SEVERITY_LEVEL_HIGH","patchAvailable":true}}`,
}

func reportVulnsTestParsed(t *testing.T) []reportVulnsRow {
	t.Helper()
	rows := make([]reportVulnsRow, 0, len(reportVulnsTestRows))
	for _, s := range reportVulnsTestRows {
		r, err := reportVulnsParse(json.RawMessage(s))
		if err != nil {
			t.Fatalf("fixture did not parse: %v\n%s", err, s)
		}
		rows = append(rows, r)
	}
	return rows
}

func TestReportVulnsParseVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want reportVulnsRow
	}{
		{
			name: "application with string risk score and named version",
			raw:  reportVulnsTestRows[0],
			want: reportVulnsRow{DeviceUUID: reportVulnsTestDev1, DeviceGroupUUID: reportVulnsTestHQ, Scope: "application", CVE: "CVE-2024-3400",
				RiskScore: 92, Severity: "HIGH", PatchAvailable: true, Target: "Mozilla Firefox 115.0.2", FirstDetectTime: "2026-09-01T10:00:00Z"},
		},
		{
			name: "application with numeric version parts",
			raw:  reportVulnsTestRows[1],
			want: reportVulnsRow{DeviceUUID: reportVulnsTestDev2, DeviceGroupUUID: reportVulnsTestAcme, Scope: "application", CVE: "CVE-2024-3400",
				RiskScore: 88, Severity: "HIGH", Target: "Mozilla Firefox 114.0.1"},
		},
		{
			name: "os vulnerability keeps the family id and no target",
			raw:  reportVulnsTestRows[3],
			want: reportVulnsRow{DeviceUUID: reportVulnsTestDev1, DeviceGroupUUID: reportVulnsTestHQ, Scope: "os", CVE: "CVE-2025-21333",
				RiskScore: 64, Severity: "MEDIUM", PatchAvailable: true, OSFamilyID: 1},
		},
		{
			name: "package falls back to the package manager name",
			raw:  reportVulnsTestRows[4],
			want: reportVulnsRow{DeviceUUID: reportVulnsTestDev3, DeviceGroupUUID: reportVulnsTestPlumb, Scope: "package", CVE: "CVE-2023-0286",
				RiskScore: 81, Severity: "HIGH", PatchAvailable: true, Target: "openssl 3.0.2-0ubuntu1.10"},
		},
		{
			name: "an empty variant object is still that variant",
			raw:  `{"deviceUuid":"d","deviceGroupUuid":"g","osVulnerability":{},"applicationVulnerability":null}`,
			want: reportVulnsRow{DeviceUUID: "d", DeviceGroupUUID: "g", Scope: "os", Severity: "UNSPECIFIED"},
		},
		{
			name: "missing severity and patch flag, but a patch uuid is listed",
			raw:  `{"deviceUuid":"d","availablePatchUuids":["p"],"osVulnerability":{"cveNumber":" CVE-2024-1 "}}`,
			want: reportVulnsRow{DeviceUUID: "d", Scope: "os", CVE: "CVE-2024-1", Severity: "UNSPECIFIED", PatchAvailable: true},
		},
		{
			name: "unknown severity enum passes through",
			raw:  `{"deviceUuid":"d","packageVulnerability":{"cveNumber":"CVE-2024-2","severity":"SEVERITY_LEVEL_CRITICAL","riskScore":"77"}}`,
			want: reportVulnsRow{DeviceUUID: "d", Scope: "package", CVE: "CVE-2024-2", RiskScore: 77, Severity: "CRITICAL"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reportVulnsParse(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
	// Rows that must be rejected rather than read as zeros: a non-object row
	// (null and scalars decode "successfully" into an empty row), a row without
	// exactly one vulnerability variant, and wrongly typed fields. riskScore
	// "oops" used to decode as risk 0.
	for name, raw := range map[string]string{
		"non-object":            `[1,2]`,
		"null row":              `null`,
		"number row":            `42`,
		"string row":            `"CVE-2024-3400"`,
		"no variant at all":     `{"deviceUuid":"d","deviceGroupUuid":"g","availablePatchUuids":["p-1"]}`,
		"only null variants":    `{"deviceUuid":"d","applicationVulnerability":null,"osVulnerability":null,"packageVulnerability":null}`,
		"two variants":          `{"deviceUuid":"d","osVulnerability":{"cveNumber":"CVE-2024-1"},"packageVulnerability":{"cveNumber":"CVE-2024-2"}}`,
		"variant not an object": `{"deviceUuid":"d","osVulnerability":"CVE-2024-1"}`,
		"garbage risk score":    `{"deviceUuid":"d","packageVulnerability":{"cveNumber":"CVE-2024-2","severity":"SEVERITY_LEVEL_CRITICAL","riskScore":"oops"}}`,
		"fractional risk":       `{"deviceUuid":"d","osVulnerability":{"cveNumber":"CVE-2024-3","riskScore":64.5}}`,
		"patch flag as string":  `{"deviceUuid":"d","applicationVulnerability":{"cveNumber":"CVE-2024-4","patchAvailable":"yes"}}`,
		"patch uuids as string": `{"deviceUuid":"d","availablePatchUuids":"p-1","osVulnerability":{"cveNumber":"CVE-2024-5"}}`,
	} {
		if _, err := reportVulnsParse(json.RawMessage(raw)); err == nil {
			t.Errorf("%s: the row must be rejected", name)
		}
	}
}

func TestReportVulnsRollupAcrossCustomers(t *testing.T) {
	tree := newEsetTree(reportVulnsTestGroups())
	got := reportVulnsRollup(reportVulnsTestParsed(t), tree.customerOf, 10)
	if len(got) != 2 {
		t.Fatalf("want 2 customers, got %d: %+v", len(got), got)
	}
	acme, plumb := got[0], got[1]
	if acme.Customer != "Acme Dental" || plumb.Customer != "Acme Plumbing" {
		t.Fatalf("worst-first order wrong: %q then %q", acme.Customer, plumb.Customer)
	}
	if acme.CustomerUUID != reportVulnsTestAcme {
		t.Fatalf("customer uuid = %q", acme.CustomerUUID)
	}
	if acme.VulnerableDevices != 2 || acme.Vulnerabilities != 3 || acme.MaxRiskScore != 92 || acme.PatchAvailable != 2 {
		t.Fatalf("acme totals wrong: %+v", acme)
	}
	if acme.BySeverity["HIGH"] != 2 || acme.BySeverity["MEDIUM"] != 1 {
		t.Fatalf("acme by_severity = %v", acme.BySeverity)
	}
	if len(acme.TopCVEs) != 2 {
		t.Fatalf("acme top cves = %+v", acme.TopCVEs)
	}
	first := acme.TopCVEs[0]
	if first.CVE != "CVE-2024-3400" || first.AffectedDevices != 2 || first.MaxRiskScore != 92 || first.Severity != "HIGH" || !first.PatchAvailable {
		t.Fatalf("acme worst cve = %+v", first)
	}
	if plumb.VulnerableDevices != 1 || plumb.Vulnerabilities != 2 || plumb.MaxRiskScore != 81 || plumb.PatchAvailable != 1 {
		t.Fatalf("plumbing totals wrong: %+v", plumb)
	}
	// Plumbing: CVE-2023-0286 (81) outranks CVE-2024-3400 (75 there, unpatched).
	if plumb.TopCVEs[0].CVE != "CVE-2023-0286" || plumb.TopCVEs[1].CVE != "CVE-2024-3400" || plumb.TopCVEs[1].PatchAvailable {
		t.Fatalf("plumbing top cves = %+v", plumb.TopCVEs)
	}
	// The same CVE spans both customers: 2 + 1 = 3 devices in total.
	total := 0
	for _, c := range got {
		for _, cv := range c.TopCVEs {
			if cv.CVE == "CVE-2024-3400" {
				total += cv.AffectedDevices
			}
		}
	}
	if total != 3 {
		t.Fatalf("CVE-2024-3400 should cover 3 devices across customers, got %d", total)
	}

	capped := reportVulnsRollup(reportVulnsTestParsed(t), tree.customerOf, 1)
	for _, c := range capped {
		if len(c.TopCVEs) != 1 {
			t.Fatalf("--top 1 not applied for %s: %+v", c.Customer, c.TopCVEs)
		}
	}
}

func TestReportVulnsRollupEmptyAndEdgeCases(t *testing.T) {
	tree := newEsetTree(reportVulnsTestGroups())
	empty := reportVulnsRollup(nil, tree.customerOf, 10)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty input must give an empty, non-nil slice: %#v", empty)
	}
	if b, _ := json.Marshal(empty); string(b) != "[]" {
		t.Fatalf("empty rollup JSON = %s", b)
	}

	// No COMPANY groups: everything rolls into one (tenant) row; rows without
	// a CVE id count toward totals but never appear in top_cves; a device in
	// a group outside any customer lands in (unassigned) under an MSP tree.
	rows := []reportVulnsRow{
		{DeviceUUID: "d1", DeviceGroupUUID: "g1", CVE: "CVE-2024-1", RiskScore: 40, Severity: "LOW"},
		{DeviceUUID: "d2", DeviceGroupUUID: "g2", CVE: "", RiskScore: 99, Severity: "HIGH"},
	}
	tenant := reportVulnsRollup(rows, newEsetTree(nil).customerOf, 10)
	if len(tenant) != 1 || tenant[0].Customer != esetCustomerTenant || tenant[0].CustomerUUID != "" {
		t.Fatalf("tenant rollup = %+v", tenant)
	}
	if tenant[0].Vulnerabilities != 2 || tenant[0].VulnerableDevices != 2 || tenant[0].MaxRiskScore != 99 {
		t.Fatalf("tenant totals = %+v", tenant[0])
	}
	if len(tenant[0].TopCVEs) != 1 || tenant[0].TopCVEs[0].CVE != "CVE-2024-1" {
		t.Fatalf("a row without a CVE id must not enter top_cves: %+v", tenant[0].TopCVEs)
	}
	unassigned := reportVulnsRollup([]reportVulnsRow{{DeviceUUID: "d9", DeviceGroupUUID: reportVulnsTestMSP, CVE: "CVE-2024-9", Severity: "UNSPECIFIED"}}, tree.customerOf, 10)
	if len(unassigned) != 1 || unassigned[0].Customer != esetCustomerNone {
		t.Fatalf("a synced group outside every customer should roll into %s: %+v", esetCustomerNone, unassigned)
	}
	// A group the tree does not hold is owner-unknown, never (unassigned).
	unknown := reportVulnsRollup([]reportVulnsRow{{DeviceUUID: "d9", DeviceGroupUUID: "nowhere", CVE: "CVE-2024-9", Severity: "UNSPECIFIED"}}, tree.customerOf, 10)
	if len(unknown) != 1 || unknown[0].Customer != esetCustomerUnknownGroup {
		t.Fatalf("an unknown group should roll into %s: %+v", esetCustomerUnknownGroup, unknown)
	}
}

func TestReportVulnsBuildViewNotes(t *testing.T) {
	items := make([]json.RawMessage, 0)
	for _, s := range reportVulnsTestRows {
		items = append(items, json.RawMessage(s))
	}
	items = append(items, json.RawMessage(`"garbage"`),
		json.RawMessage(`{"deviceUuid":"`+reportVulnsTestDev1+`","deviceGroupUuid":"`+reportVulnsTestHQ+`","applicationVulnerability":{"cveNumber":"CVE-2024-9999","riskScore":"oops"}}`))
	tree := newEsetTree(reportVulnsTestGroups())
	var rej esetRejects
	v := reportVulnsBuildView(esetPageResult{Items: items, Pages: 3, Truncated: true}, tree, nil, 10, 3, nil, &rej)
	if v.ScannedVulnerabilities != 7 || v.Pages != 3 || !v.Truncated || v.RejectedRows != 2 || rej.N != 2 || v.OtherCustomerRows != 0 {
		t.Fatalf("view counters = %+v", v)
	}
	if !strings.Contains(v.Note, "--max-scan-pages 3") || !strings.Contains(v.Note, "2 row(s) could not be decoded") {
		t.Fatalf("truncated view must say how to get the rest and name the rejects: %q", v.Note)
	}
	// The rejected riskScore "oops" row never reaches a customer's totals.
	for _, c := range v.Customers {
		for _, cv := range c.TopCVEs {
			if cv.CVE == "CVE-2024-9999" {
				t.Fatalf("a rejected row leaked into %s: %+v", c.Customer, cv)
			}
		}
		if c.Customer == "Acme Dental" && c.Vulnerabilities != 3 {
			t.Fatalf("acme vulnerabilities = %d, want 3", c.Vulnerabilities)
		}
	}
	clean := reportVulnsBuildView(esetPageResult{Items: []json.RawMessage{}, Pages: 1}, tree, nil, 10, 20, nil, nil)
	if clean.Note != "" || clean.Customers == nil || clean.RejectedRows != 0 {
		t.Fatalf("clean empty view = %+v", clean)
	}
	if b, _ := json.Marshal(clean); !strings.Contains(string(b), `"rejected_rows":0`) || !strings.Contains(string(b), `"other_customer_rows":0`) {
		t.Fatalf("rejected_rows and other_customer_rows must always be present: %s", b)
	}
}

// reportVulnsTestNestedGroups: Acme Dental (COMPANY) has a site, and a second
// COMPANY, Acme Kids Clinic, nested under it with its own site.
const (
	reportVulnsTestKids     = "4fadbec0-5172-4d9e-bf30-6b8cad2e4f50"
	reportVulnsTestKidsSite = "50becfd1-6283-4eaf-8041-7c9dbe3f5061"
	reportVulnsTestDev4     = "a1b2c3d4-0004-4e5f-8a9b-0c1d2e3f4a54"
)

func reportVulnsTestNestedGroups() []esetGroup {
	return append(reportVulnsTestGroups(),
		esetGroup{UUID: reportVulnsTestKids, Name: "Acme Kids Clinic", ParentUUID: reportVulnsTestAcme, EntityType: esetCompanyEntityType},
		esetGroup{UUID: reportVulnsTestKidsSite, Name: "Main St", ParentUUID: reportVulnsTestKids, EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
	)
}

// TestReportVulnsNestedCompanyAttribution: ESET's deviceGroupUuid filter is
// recursive, so querying Acme Dental also returns rows of Acme Kids Clinic
// nested under it. Those rows belong to Acme Kids Clinic, not Acme Dental.
func TestReportVulnsNestedCompanyAttribution(t *testing.T) {
	tree := newEsetTree(reportVulnsTestNestedGroups())
	acme := esetGroup{UUID: reportVulnsTestAcme, Name: "Acme Dental", EntityType: esetCompanyEntityType}
	kidsRow := `{"deviceUuid":"` + reportVulnsTestDev4 + `","deviceGroupUuid":"` + reportVulnsTestKidsSite + `",
	  "applicationVulnerability":{"cveNumber":"CVE-2024-3400","riskScore":"99","severity":"SEVERITY_LEVEL_HIGH","patchAvailable":true}}`
	unknownRow := `{"deviceUuid":"a1b2c3d4-0005-4e5f-8a9b-0c1d2e3f4a55","deviceGroupUuid":"60cfd0e2-7394-4fb0-9152-8daecf406172",
	  "osVulnerability":{"cveNumber":"CVE-2025-21333","riskScore":"50","severity":"SEVERITY_LEVEL_MEDIUM"}}`
	items := []json.RawMessage{json.RawMessage(reportVulnsTestRows[0]), json.RawMessage(reportVulnsTestRows[1]), json.RawMessage(kidsRow), json.RawMessage(unknownRow)}

	v := reportVulnsBuildView(esetPageResult{Items: items, Pages: 1}, tree, &acme, 10, 20, nil, nil)
	if len(v.Customers) != 1 || v.Customers[0].CustomerUUID != reportVulnsTestAcme {
		t.Fatalf("--customer Acme Dental must report only Acme Dental: %+v", v.Customers)
	}
	a := v.Customers[0]
	// Rows 0 and 1 are Acme's. The nested clinic's row is left out, and so is
	// the row from a group created after the last sync: the recursive scope
	// returned it for Acme, but that does not prove Acme is its nearest COMPANY.
	if a.Vulnerabilities != 2 || a.VulnerableDevices != 2 || a.MaxRiskScore != 92 || a.BySeverity["MEDIUM"] != 0 {
		t.Fatalf("acme totals = %+v (neither the nested clinic's risk 99 nor the unknown group's MEDIUM row may count)", a)
	}
	for _, cv := range a.TopCVEs {
		if cv.CVE == "CVE-2025-21333" {
			t.Fatalf("the unknown group's CVE leaked into Acme: %+v", a.TopCVEs)
		}
	}
	// The nested clinic's row is another customer's; the unknown group's row
	// may be Acme's, so it is unattributed, not another customer's.
	if v.OtherCustomerRows != 1 || !strings.Contains(v.Note, "1 row(s) belong to other customers") || v.UnattributedRows != 1 ||
		!reflect.DeepEqual(v.UnattributedDeviceUUIDs, []string{"a1b2c3d4-0005-4e5f-8a9b-0c1d2e3f4a55"}) || !strings.Contains(v.Note, "1 vulnerability row(s) could not be tied to Acme Dental") {
		t.Fatalf("other_customer_rows = %d unattributed_rows = %d %v, note = %q", v.OtherCustomerRows, v.UnattributedRows, v.UnattributedDeviceUUIDs, v.Note)
	}

	// Without --customer the nested clinic is its own customer and the
	// unknown group gets its own UUID-less row (not (unassigned)).
	all := reportVulnsBuildView(esetPageResult{Items: items, Pages: 1}, tree, nil, 10, 20, nil, nil)
	names := map[string]int{}
	for _, c := range all.Customers {
		names[c.Customer] = c.Vulnerabilities
		if c.Customer == esetCustomerUnknownGroup && (c.CustomerUUID != "" || c.MaxRiskScore != 50 || c.VulnerableDevices != 1) {
			t.Fatalf("unknown-group row = %+v", c)
		}
	}
	want := map[string]int{"Acme Kids Clinic": 1, "Acme Dental": 2, esetCustomerUnknownGroup: 1}
	if !reflect.DeepEqual(names, want) || all.OtherCustomerRows != 0 || all.UnattributedRows != 1 {
		t.Fatalf("unfiltered attribution = %v (other %d), want %v", names, all.OtherCustomerRows, want)
	}

	// --customer the nested clinic itself keeps its row.
	kids := esetGroup{UUID: reportVulnsTestKids, Name: "Acme Kids Clinic", EntityType: esetCompanyEntityType}
	only := reportVulnsBuildView(esetPageResult{Items: []json.RawMessage{json.RawMessage(kidsRow)}, Pages: 1}, tree, &kids, 10, 20, nil, nil)
	if len(only.Customers) != 1 || only.Customers[0].Customer != "Acme Kids Clinic" || only.OtherCustomerRows != 0 {
		t.Fatalf("--customer clinic = %+v", only)
	}

	// No synced tree: every group is unknown, so every row is the target's.
	bare := esetGroup{UUID: reportVulnsTestAcme, Name: reportVulnsTestAcme}
	noTree := reportVulnsBuildView(esetPageResult{Items: items, Pages: 1}, newEsetTree(nil), &bare, 10, 20, nil, nil)
	if len(noTree.Customers) != 1 || noTree.Customers[0].Vulnerabilities != 4 || noTree.OtherCustomerRows != 0 {
		t.Fatalf("no-tree --customer = %+v", noTree)
	}
}

// reportVulnsTestServer is a stand-in for every ESET Connect host. The client
// rebases absolute service URLs onto ESET_PROTECT_BASE_URL, so one server
// answers device-management, incident-management, vulnerability-management
// and patch-management paths.
type reportVulnsTestServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*url.URL
}

func (s *reportVulnsTestServer) seen() []*url.URL {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*url.URL(nil), s.requests...)
}

func reportVulnsNewServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *reportVulnsTestServer {
	t.Helper()
	s := &reportVulnsTestServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		u := *r.URL
		s.requests = append(s.requests, &u)
		s.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// reportVulnsRunCLI runs the real command tree in an isolated home against
// srv (nil means no server is expected to be called).
func reportVulnsRunCLI(t *testing.T, srv *reportVulnsTestServer, args ...string) (string, string, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv(cliutil.DogfoodEnvVar, "")
	if srv != nil {
		t.Setenv("ESET_PROTECT_BASE_URL", srv.URL)
	} else {
		t.Setenv("ESET_PROTECT_BASE_URL", "http://127.0.0.1:1")
	}
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if srv != nil {
		// The client paces requests by default; the stand-in server has no limit.
		args = append(append([]string(nil), args...), "--rate-limit", "0")
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// reportVulnsSeedDB writes groups (and optional devices) into a fresh mirror.
func reportVulnsSeedDB(t *testing.T, groups []esetGroup, devices []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eset.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, g := range groups {
		b, _ := json.Marshal(g)
		if err := db.Upsert("device-groups", g.UUID, b); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range devices {
		var probe struct {
			UUID string `json:"uuid"`
		}
		if err := json.Unmarshal([]byte(d), &probe); err != nil || probe.UUID == "" {
			t.Fatalf("bad device fixture %s", d)
		}
		if err := db.Upsert("devices", probe.UUID, json.RawMessage(d)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func reportVulnsTestVulnHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/device-vulnerabilities" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("pageToken") == "" {
		_, _ = w.Write([]byte(`{"vulnerabilities":[` + strings.Join(reportVulnsTestRows[:3], ",") + `],"nextPageToken":"page-2"}`))
		return
	}
	_, _ = w.Write([]byte(`{"vulnerabilities":[` + strings.Join(reportVulnsTestRows[3:], ",") + `],"nextPageToken":""}`))
}

func TestReportVulnsCommandRollsUpPerCustomer(t *testing.T) {
	srv := reportVulnsNewServer(t, reportVulnsTestVulnHandler)
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), nil)
	out, stderr, err := reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr)
	}
	var v reportVulnsView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not the view JSON: %v\n%s", err, out)
	}
	if v.Pages != 2 || v.Truncated || v.ScannedVulnerabilities != 5 || v.Note != "" {
		t.Fatalf("paging = %+v", v)
	}
	if len(v.Customers) != 2 || v.Customers[0].Customer != "Acme Dental" || v.Customers[0].VulnerableDevices != 2 {
		t.Fatalf("customers = %+v", v.Customers)
	}
	reqs := srv.seen()
	if len(reqs) != 2 || reqs[0].Query().Get("pageSize") != "1000" || reqs[1].Query().Get("pageToken") != "page-2" {
		t.Fatalf("requests = %v", reqs)
	}
	if reqs[0].Query().Get("deviceGroupUuid") != "" {
		t.Fatal("no --customer must mean no deviceGroupUuid filter")
	}
}

func TestReportVulnsCommandCustomerFilterAndPageCap(t *testing.T) {
	srv := reportVulnsNewServer(t, reportVulnsTestVulnHandler)
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), nil)
	out, stderr, err := reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--customer", "acme dental", "--max-scan-pages", "1", "--top", "1", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr)
	}
	var v reportVulnsView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	reqs := srv.seen()
	if len(reqs) != 1 || reqs[0].Query().Get("deviceGroupUuid") != reportVulnsTestAcme {
		t.Fatalf("--customer must send the COMPANY group uuid once: %v", reqs)
	}
	if !v.Truncated || v.Pages != 1 || !strings.Contains(v.Note, "--max-scan-pages") {
		t.Fatalf("page cap not reported: %+v", v)
	}
	// Under --customer a row is attributed to its own nearest COMPANY: the
	// Acme Plumbing row (the stand-in ignores the filter, as a nested COMPANY
	// would come back from ESET) is left out and counted, not merged into
	// Acme Dental.
	if len(v.Customers) != 1 || v.Customers[0].Customer != "Acme Dental" || v.Customers[0].Vulnerabilities != 2 || len(v.Customers[0].TopCVEs) != 1 {
		t.Fatalf("customers = %+v", v.Customers)
	}
	if v.OtherCustomerRows != 1 || v.RejectedRows != 0 {
		t.Fatalf("other_customer_rows = %d rejected = %d", v.OtherCustomerRows, v.RejectedRows)
	}
}

// TestReportVulnsCommandRejectsMalformedRows: a row with a wrongly typed
// field used to vanish (or read as risk 0) with truncated=false and no
// signal. The command must count it and warn once on stderr.
func TestReportVulnsCommandRejectsMalformedRows(t *testing.T) {
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vulnerabilities":[` + reportVulnsTestRows[0] + `,
		  {"deviceUuid":"` + reportVulnsTestDev2 + `","deviceGroupUuid":"` + reportVulnsTestAcme + `","applicationVulnerability":{"cveNumber":"CVE-2024-1111","riskScore":"oops"}},
		  {"deviceUuid":"` + reportVulnsTestDev3 + `","deviceGroupUuid":"` + reportVulnsTestPlumb + `","osVulnerability":{"cveNumber":"CVE-2024-2222","riskScore":{"value":5}}}],"nextPageToken":""}`))
	})
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), nil)
	out, stderr, err := reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, stderr)
	}
	var v reportVulnsView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if v.Truncated || v.ScannedVulnerabilities != 3 || v.RejectedRows != 2 || len(v.Customers) != 1 || v.Customers[0].Vulnerabilities != 1 {
		t.Fatalf("view = %+v", v)
	}
	if strings.Count(stderr, "could not be decoded") != 1 || !strings.Contains(stderr, "warning: 2 row(s)") || !strings.Contains(stderr, "vulnerability row 1") {
		t.Fatalf("stderr = %q", stderr)
	}
}

// TestReportVulnsCommandRejectsNullScalarAndVariantlessRows: json.Unmarshal
// accepts null into a struct, so "vulnerabilities":[null] used to add a fake
// risk-0 vulnerability with rejected_rows 0. Null, scalar and variant-less
// rows must be counted in rejected_rows and kept out of every total.
func TestReportVulnsCommandRejectsNullScalarAndVariantlessRows(t *testing.T) {
	variantless := `{"deviceUuid":"` + reportVulnsTestDev2 + `","deviceGroupUuid":"` + reportVulnsTestAcme + `","availablePatchUuids":["p-2"]}`
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vulnerabilities":[` + reportVulnsTestRows[0] + `,null,7,"CVE-2024-3400",` + variantless + `],"nextPageToken":""}`))
	})
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), nil)
	out, stderr, err := reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, stderr)
	}
	var v reportVulnsView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if v.ScannedVulnerabilities != 5 || v.RejectedRows != 4 || !strings.Contains(v.Note, "4 row(s) could not be decoded") {
		t.Fatalf("view = %+v", v)
	}
	if len(v.Customers) != 1 || v.Customers[0].Customer != "Acme Dental" || v.Customers[0].Vulnerabilities != 1 ||
		v.Customers[0].VulnerableDevices != 1 || v.Customers[0].BySeverity["UNSPECIFIED"] != 0 || v.Customers[0].PatchAvailable != 1 {
		t.Fatalf("only the one good row may count: %+v", v.Customers)
	}
	if !strings.Contains(stderr, "warning: 4 row(s) could not be decoded") || !strings.Contains(stderr, "vulnerability row 1: not a JSON object") {
		t.Fatalf("stderr = %q", stderr)
	}

	// A page of nothing but null is not "no vulnerabilities".
	nullOnly := reportVulnsBuildView(esetPageResult{Items: []json.RawMessage{json.RawMessage(`null`)}, Pages: 1}, newEsetTree(reportVulnsTestGroups()), nil, 10, 20, nil, nil)
	if nullOnly.RejectedRows != 1 || len(nullOnly.Customers) != 0 || nullOnly.ScannedVulnerabilities != 1 {
		t.Fatalf("[null] view = %+v", nullOnly)
	}
}

func TestReportVulnsCommandWithoutMirror(t *testing.T) {
	srv := reportVulnsNewServer(t, reportVulnsTestVulnHandler)
	missing := filepath.Join(t.TempDir(), "absent.db")
	out, stderr, err := reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--db", missing, "--json")
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "no local mirror") {
		t.Fatalf("missing mirror hint absent from stderr: %q", stderr)
	}
	var v reportVulnsView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if len(v.Customers) != 1 || v.Customers[0].Customer != esetCustomerTenant || v.Customers[0].VulnerableDevices != 3 {
		t.Fatalf("no-mirror rollup = %+v", v.Customers)
	}
	if !strings.Contains(v.Note, "sync --resources device-groups") {
		t.Fatalf("note should say how to get per-customer rows: %q", v.Note)
	}

	// A customer NAME cannot be resolved without the tree: usage error, no API call.
	before := len(srv.seen())
	_, _, err = reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--customer", "Acme Dental", "--db", missing, "--json")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("want usage error (exit 2), got %v", err)
	}
	if len(srv.seen()) != before {
		t.Fatal("an unresolvable --customer must not reach the API")
	}
	// A group UUID is sent as-is.
	out, _, err = reportVulnsRunCLI(t, srv, "report", "vulnerabilities", "--customer", reportVulnsTestPlumb, "--db", missing, "--json")
	if err != nil {
		t.Fatal(err)
	}
	reqs := srv.seen()
	if reqs[len(reqs)-1].Query().Get("deviceGroupUuid") != reportVulnsTestPlumb {
		t.Fatalf("UUID --customer not sent: %v", reqs)
	}
	v = reportVulnsView{}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	// With no tree nothing can be attributed: every row ESET returned for the
	// group is kept under the target (as exposure's unsynced bucket keeps
	// them), never dropped as "not in the synced tree".
	if len(v.Customers) != 1 || v.Customers[0].Customer != reportVulnsTestPlumb || v.Customers[0].CustomerUUID != reportVulnsTestPlumb ||
		v.Customers[0].Vulnerabilities != 5 || v.OtherCustomerRows != 0 || !strings.Contains(v.Note, "nested customers") {
		t.Fatalf("customer should be labelled by its uuid and keep every row: %s", out)
	}
}

func TestReportVulnsCommandDryRunAndFlags(t *testing.T) {
	out, _, err := reportVulnsRunCLI(t, nil, "report", "vulnerabilities", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"dry_run":true`) || !strings.Contains(out, "report vulnerabilities") {
		t.Fatalf("dry-run output = %s", out)
	}
	if _, _, err := reportVulnsRunCLI(t, nil, "report", "vulnerabilities", "--top", "0", "--json"); err == nil || ExitCode(err) != 2 {
		t.Fatalf("--top 0 should be a usage error, got %v", err)
	}
	help, _, err := reportVulnsRunCLI(t, nil, "report", "vulnerabilities", "--help")
	if err != nil || !strings.Contains(help, "Do NOT use it to look up one CVE; use 'exposure' instead") {
		t.Fatalf("help missing the manifest guidance: %v\n%s", err, help)
	}
}
