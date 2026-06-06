// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet remediate worklist grouping.

package cli

import (
	"encoding/json"
	"testing"
)

func vulnEnt(cid, id, cve, severity string, raw map[string]any) fleetEntity {
	e := fleetEntity{CID: cid, Kind: kindVuln, ID: id, Name: cve, Severity: severity}
	if raw != nil {
		b, _ := json.Marshal(raw)
		e.Raw = b
	}
	return e
}

func TestRemediationWorklistGroupsByAction(t *testing.T) {
	patch := map[string]any{
		"remediation": map[string]any{"entities": []any{map[string]any{"action": "Update Chrome to 126"}}},
		"host_info":   map[string]any{"hostname": "ws-01"},
	}
	patchOtherHost := map[string]any{
		"remediation": map[string]any{"entities": []any{map[string]any{"action": "Update Chrome to 126"}}},
		"host_info":   map[string]any{"hostname": "ws-02"},
	}
	rebootFix := map[string]any{
		"remediation": map[string]any{"ids": []string{"reboot-required"}},
		"host_info":   map[string]any{"hostname": "srv-09"},
	}
	ents := []fleetEntity{
		vulnEnt("cid-a", "v1", "CVE-2026-1111", "critical", patch),
		vulnEnt("cid-b", "v2", "CVE-2026-1111", "critical", patchOtherHost),
		vulnEnt("cid-a", "v3", "CVE-2026-2222", "high", rebootFix),
	}
	view := remediationWorklist(ents, "")
	if view.TotalVulns != 3 {
		t.Fatalf("total_vulns = %d, want 3", view.TotalVulns)
	}
	if len(view.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(view.Groups))
	}
	top := view.Groups[0] // sorted by host count desc
	if top.Action != "Update Chrome to 126" {
		t.Fatalf("top action = %q, want Update Chrome to 126", top.Action)
	}
	if top.Hosts != 2 || top.CIDs != 2 || top.Vulns != 2 {
		t.Errorf("top group hosts/cids/vulns = %d/%d/%d, want 2/2/2", top.Hosts, top.CIDs, top.Vulns)
	}
	if top.Severities["critical"] != 2 {
		t.Errorf("top severities = %v, want critical:2", top.Severities)
	}
	if len(top.SampleCVEs) != 1 || top.SampleCVEs[0] != "CVE-2026-1111" {
		t.Errorf("sample_cves = %v, want deduped [CVE-2026-1111]", top.SampleCVEs)
	}
}

func TestRemediationWorklistSeverityFilter(t *testing.T) {
	raw := map[string]any{
		"remediation": map[string]any{"ids": []string{"patch-os"}},
		"host_info":   map[string]any{"hostname": "ws-01"},
	}
	ents := []fleetEntity{
		vulnEnt("cid-a", "v1", "CVE-2026-1111", "critical", raw),
		vulnEnt("cid-a", "v2", "CVE-2026-2222", "low", raw),
	}
	view := remediationWorklist(ents, "critical")
	if view.TotalVulns != 1 {
		t.Fatalf("severity filter leaked: total_vulns = %d, want 1", view.TotalVulns)
	}
	if len(view.Groups) != 1 || view.Groups[0].Vulns != 1 {
		t.Fatalf("groups = %+v, want one group with one vuln", view.Groups)
	}
}

func TestRemediationWorklistNoFacetDataNote(t *testing.T) {
	// Vulns synced without remediation facets (e.g. by the prior CLI version).
	ents := []fleetEntity{
		vulnEnt("cid-a", "v1", "CVE-2026-1111", "critical", map[string]any{"cve": map[string]any{"id": "CVE-2026-1111"}}),
	}
	view := remediationWorklist(ents, "")
	if view.VulnsWithoutRemediation != 1 {
		t.Fatalf("vulns_without_remediation = %d, want 1", view.VulnsWithoutRemediation)
	}
	if len(view.Groups) != 0 {
		t.Fatalf("groups = %v, want none", view.Groups)
	}
	if view.Note == "" {
		t.Error("expected an honest no-facet-data note, got none")
	}
}

func TestRemediationWorklistEmptyStoreNote(t *testing.T) {
	view := remediationWorklist(nil, "")
	if view.Note == "" {
		t.Error("expected an honest empty-store note, got none")
	}
	if len(view.Groups) != 0 || view.TotalVulns != 0 {
		t.Errorf("expected empty view, got %+v", view)
	}
}
