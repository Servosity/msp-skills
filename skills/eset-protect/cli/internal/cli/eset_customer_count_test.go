// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestMSPOwnDevicesAreNeverCountedAsACustomer (adversarial round 5, P2): the
// "(unassigned)" bucket (the MSP's own machines, or anything under the hidden
// scope root) was counted as a proven customer, so a hash on Acme, Beta and
// the MSP's jump box read customers_known 3 and thresholds_proven true for
// --min-customers 3: "proven" with two real customers. customers_known and
// the thresholds count only COMPANY groups (and the one "(tenant)" of a
// single-tenant account); "(unassigned)" stays listed. exposure
// totals.customers follows the same rule.
func TestMSPOwnDevicesAreNeverCountedAsACustomer(t *testing.T) {
	testenv.Isolate(t)
	ago := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	const shared, mspOnly = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	msp := fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {
			`{"uuid":"msp","displayName":"Servosity MSP","parentGroupUuid":"hidden-root","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"acme","displayName":"Acme","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"beta","displayName":"Beta Co","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
		},
		"devices": {
			`{"uuid":"d1","displayName":"ACME-01","parentGroupUuid":"acme"}`,
			`{"uuid":"d3","displayName":"BETA-03","parentGroupUuid":"beta"}`,
			`{"uuid":"d6","displayName":"MSP-JUMP-06","parentGroupUuid":"msp"}`,
		},
		"detections": {
			detectionsSpreadTestDetection("det-1", shared, "Trojan", "a.exe", "d1", "SEVERITY_LEVEL_HIGH", ago),
			detectionsSpreadTestDetection("det-3", shared, "Trojan", "a.exe", "d3", "SEVERITY_LEVEL_HIGH", ago),
			detectionsSpreadTestDetection("det-6", shared, "Trojan", "a.exe", "d6", "SEVERITY_LEVEL_HIGH", ago),
			detectionsSpreadTestDetection("det-7", mspOnly, "Tool", "t.exe", "d6", "SEVERITY_LEVEL_LOW", ago),
		},
	})
	group := func(v detectionsSpreadView, sha string) *detectionsSpreadGroup {
		for i := range v.Groups {
			if v.Groups[i].SHA1 == sha {
				return &v.Groups[i]
			}
		}
		return nil
	}
	// The reviewer's command: three customers are not proven, nor possible.
	if v := detectionsSpreadTestRun(t, msp, "--since", "7d", "--min-customers", "3"); len(v.Groups) != 0 || v.UnattributedDetections != 0 {
		t.Fatalf("--min-customers 3 listed a hash seen at two customers and the MSP: %+v", v.Groups)
	}
	v := detectionsSpreadTestRun(t, msp, "--since", "7d", "--min-customers", "2")
	g := group(v, shared)
	if g == nil || g.CustomersKnown != 2 || g.CustomersPossible != 2 || !g.ThresholdsProven || strings.Join(g.Customers, "|") != "(unassigned)|Acme|Beta Co" || g.Devices != 3 {
		t.Fatalf("--min-customers 2: %+v", g)
	}
	if group(v, mspOnly) != nil {
		t.Fatalf("--min-customers 2 listed a hash seen only on the MSP's own device")
	}
	// The default lists every hash, the MSP-only one included, with 0
	// customers and "(unassigned)" named.
	v = detectionsSpreadTestRun(t, msp)
	if m := group(v, mspOnly); m == nil || m.CustomersKnown != 0 || strings.Join(m.Customers, "|") != "(unassigned)" || !m.ThresholdsProven {
		t.Fatalf("default run, MSP-only hash: %+v", m)
	}
	if v := detectionsSpreadTestRun(t, msp, "--min-customers", "1"); group(v, mspOnly) != nil {
		t.Fatalf("--min-customers 1 listed a hash seen at no customer")
	}
	out, err := fleetHealthTestRun(t, "detections", "spread", "--db", msp, "--human-friendly")
	if err != nil || !strings.Contains(string(out), "2 +MSP") || !strings.Contains(string(out), "0 +MSP") {
		t.Fatalf("table must show the MSP hit beside the customer count: %v\n%s", err, out)
	}

	// exposure: three affected devices, two customers, the MSP's own row
	// listed but not counted.
	scope := newExposureScope([]esetGroup{
		{UUID: "msp", Name: "Servosity MSP", ParentUUID: "hidden-root", EntityType: esetMSPEntityType},
		{UUID: "acme", Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "beta", Name: "Beta Co", ParentUUID: "msp", EntityType: esetCompanyEntityType},
	}, []esetDevice{{UUID: "d1", ParentGroupUUID: "acme"}, {UUID: "d3", ParentGroupUUID: "beta"}, {UUID: "d6", ParentGroupUUID: "msp"}})
	vulns := exposureDecodeFixture(t,
		exposureVulnJSON("d1", "acme", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
		exposureVulnJSON("d3", "beta", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
		exposureVulnJSON("d6", "msp", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""),
	)
	rows, totals := exposureBuild(exposureQuery{CVEs: []string{"CVE-2024-1111"}}, vulns, scope, 0)
	if len(rows) != 3 || totals != (exposureTotals{AffectedDevices: 3, Customers: 2}) {
		t.Fatalf("exposure: want 3 rows, 3 devices across 2 customers: %+v %+v", rows, totals)
	}

	// An unknown-owner device may be the MSP's own: one alone proves no
	// customer.
	unknown := fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {
			`{"uuid":"msp","displayName":"Servosity MSP","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"acme","displayName":"Acme","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
		},
		"devices":    {`{"uuid":"d9","displayName":"NEW-09","parentGroupUuid":"made-after-sync"}`},
		"detections": {detectionsSpreadTestDetection("det-9", shared, "Trojan", "a.exe", "d9", "SEVERITY_LEVEL_HIGH", ago)},
	})
	v = detectionsSpreadTestRun(t, unknown, "--min-customers", "1")
	if g := group(v, shared); g == nil || g.CustomersKnown != 0 || g.CustomersPossible != 1 || g.ThresholdsProven || v.GroupsUnproven != 1 {
		t.Fatalf("one unknown-owner device must not prove a customer: %+v", g)
	}

	// Single-tenant account: the one "(tenant)" is the customer.
	tenant := fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {`{"uuid":"all","displayName":"All"}`, `{"uuid":"office","displayName":"Office","parentGroupUuid":"all"}`},
		"devices":       {`{"uuid":"t1","displayName":"PC-1","parentGroupUuid":"office"}`, `{"uuid":"t2","displayName":"PC-2","parentGroupUuid":"all"}`},
		"detections": {
			detectionsSpreadTestDetection("det-t1", shared, "Trojan", "a.exe", "t1", "SEVERITY_LEVEL_HIGH", ago),
			detectionsSpreadTestDetection("det-t2", shared, "Trojan", "a.exe", "t2", "SEVERITY_LEVEL_HIGH", ago),
		},
	})
	v = detectionsSpreadTestRun(t, tenant, "--min-customers", "1")
	if g := group(v, shared); g == nil || g.CustomersKnown != 1 || strings.Join(g.Customers, "|") != esetCustomerTenant || !g.ThresholdsProven {
		t.Fatalf("single tenant: %+v", g)
	}
	if v := detectionsSpreadTestRun(t, tenant, "--min-customers", "2"); len(v.Groups) != 0 {
		t.Fatalf("a single tenant reached --min-customers 2: %+v", v.Groups)
	}
}
