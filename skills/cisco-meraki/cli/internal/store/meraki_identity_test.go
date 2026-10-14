// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"testing"
)

// TestMerakiIdentityFallbackKeysDocumentedRows guards the hand-fix
// meraki-sync-identity-fallback with rows shaped like the Dashboard API's own
// documented examples. Before it, none of these had an extractable ID, so sync
// stored 0 rows for them and exited 1 on any org that had the data.
func TestMerakiIdentityFallbackKeysDocumentedRows(t *testing.T) {
	cases := []struct {
		resource, row, want string
	}{
		{"organizations_appliance_vpn_statuses", `{"networkId":"N_12345678","networkName":"SF","deviceSerial":"Q234-ABCD-1234","deviceStatus":"online"}`, "N_12345678+Q234-ABCD-1234"},
		{"uplinks", `{"networkId":"N_24329156","serial":"Q234-ABCD-5678","model":"MX68C","lastReportedAt":"2018-02-11T00:00:00Z","uplinks":[]}`, "N_24329156+Q234-ABCD-5678"},
		{"organizations_wireless_devices_packet_loss_by_device", `{"device":{"serial":"Q234-ABCD-5678","name":"AP"},"network":{"id":"N_24329156"},"downstream":{"total":1}}`, "N_24329156+Q234-ABCD-5678"},
		{"organizations_wireless_clients_usage_by_network_by_ssid", `{"network":{"id":"N_123456"},"ssid":{"number":0,"name":"Guest"},"usage":{"total":1}}`, "N_123456+0"},
		{"organizations_sensor_gateways_connections_latest", `{"lastReportedAt":"2026-10-13T19:29:05Z","rssi":-24,"sensor":{"serial":"Q234-ABCD-0001"},"gateway":{"serial":"Q234-ABCD-0004"},"network":{"id":"N_1"}}`, "N_1+Q234-ABCD-0001+Q234-ABCD-0004"},
		{"organizations_sensor_readings_history", `{"ts":"2021-10-18T23:54:48.000000Z","serial":"Q234-ABCD-5678","metric":"temperature","network":{"id":"N_2"}}`, "N_2+Q234-ABCD-5678+temperature+2021-10-18T23:54:48.000000Z"},
		{"organizations_wireless_rf_profiles_assignments_by_device", `{"network":{"id":"1234"},"name":"Device_name","serial":"Q234-ABCD-0001"}`, "1234+Q234-ABCD-0001"},
	}
	for _, tc := range cases {
		var obj map[string]any
		if err := json.Unmarshal([]byte(tc.row), &obj); err != nil {
			t.Fatal(err)
		}
		if got := ExtractResourceID(tc.resource, obj); got != tc.want {
			t.Fatalf("%s: ID = %q, want %q", tc.resource, got, tc.want)
		}
	}
	// A row with nothing but a timestamp has no identity and must still fail.
	if got := merakiIdentityFallback(map[string]any{"ts": "2026-10-12T01:40:29Z", "value": 3.0}, false); got != "" {
		t.Fatalf("timestamp-only row got ID %q", got)
	}
}

// TestMerakiSameNameRowsStayDistinct guards the hand-fix meraki-row-keys-not-
// display-names: the generator keyed ~30 resources on "name", so two switches
// both called "Core Switch", or "Data" VLANs in two networks, collapsed into one
// mirror row with no warning. Distinct devices, rules and VLANs must keep
// distinct IDs even when their display names match.
func TestMerakiSameNameRowsStayDistinct(t *testing.T) {
	pairs := []struct {
		resource, a, b string
	}{
		{"organizations_switch_ports_statuses_by_switch", `{"name":"Core Switch","serial":"Q555-5555-5555","network":{"id":"N_1"}}`, `{"name":"Core Switch","serial":"Q555-5555-9999","network":{"id":"N_2"}}`},
		{"organizations_policies_global_firewall_rulesets_rules", `{"name":"Block","rulesetId":"rs1","ruleId":"r1"}`, `{"name":"Block","rulesetId":"rs1","ruleId":"r2"}`},
		{"organizations_policies_global_group_policies_appliance_vlans_assignments_by_vlan", `{"name":"Data","vlanId":10,"network":{"id":"N_1"}}`, `{"name":"Data","vlanId":10,"network":{"id":"N_2"}}`},
		{"organizations_appliance_security_intrusion_rule_groups", `{"name":"Malware","groupId":"g1","policy":{"id":"p1"}}`, `{"name":"Malware","groupId":"g2","policy":{"id":"p1"}}`},
		{"organizations_floor_plans_auto_locate_statuses", `{"name":"Floor 1","floorPlanId":"g_1","network":{"id":"N_1"}}`, `{"name":"Floor 1","floorPlanId":"g_2","network":{"id":"N_2"}}`},
	}
	for _, p := range pairs {
		var a, b map[string]any
		if err := json.Unmarshal([]byte(p.a), &a); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(p.b), &b); err != nil {
			t.Fatal(err)
		}
		ida, idb := ExtractResourceID(p.resource, a), ExtractResourceID(p.resource, b)
		if ida == "" || idb == "" || ida == idb {
			t.Fatalf("%s: same-name rows got IDs %q and %q, want two distinct IDs", p.resource, ida, idb)
		}
	}
}

// TestMerakiRowKeysAreOwnIdentity extends meraki-row-keys-not-display-names to
// the generator's other non-identity picks: intrusion rules keyed on the Snort
// generator id (gid is 1 for nearly every rule), policy assignments keyed on
// the foreign policyId, and keys that carry a mutable name or URL (a rename
// left a ghost row). Rows that differ only in their own id stay distinct;
// a row whose name or URL changed keeps its id.
func TestMerakiRowKeysAreOwnIdentity(t *testing.T) {
	decode := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	distinct := []struct{ resource, a, b string }{
		{"organizations_appliance_security_intrusion_rules", `{"gid":1,"sid":57556,"ruleId":"r-A"}`, `{"gid":1,"sid":58556,"ruleId":"r-B"}`},
		{"organizations_policies_global_group_policies_appliance_vlans_assignments", `{"policyId":"p1","assignmentId":"a1","interfaceId":"i1"}`, `{"policyId":"p1","assignmentId":"a2","interfaceId":"i2"}`},
		{"organizations_policies_global_group_policies_firewall_rulesets_assignments", `{"policyId":"p1","assignmentId":"a1","rulesetId":"rs1"}`, `{"policyId":"p1","assignmentId":"a2","rulesetId":"rs2"}`},
		{"organizations_policies_global_group_policies_adaptive_policy_groups_assignments", `{"policyId":"p1","assignmentId":"a1","adaptivePolicyGroupId":"g1"}`, `{"policyId":"p1","assignmentId":"a2","adaptivePolicyGroupId":"g2"}`},
	}
	for _, p := range distinct {
		ida, idb := ExtractResourceID(p.resource, decode(p.a)), ExtractResourceID(p.resource, decode(p.b))
		if ida == "" || idb == "" || ida == idb {
			t.Fatalf("%s: distinct rows got IDs %q and %q, want two distinct IDs", p.resource, ida, idb)
		}
	}
	stable := []struct{ resource, a, b string }{
		{"integrations", `{"networkId":"N_1","name":"Test network"}`, `{"networkId":"N_1","name":"Test network RENAMED"}`},
		{"organizations_campus_gateway_connections", `{"serial":"Q1","url":"https://n1.meraki.com/Old/n/x/manage/access_points/Q1/summary"}`, `{"serial":"Q1","url":"https://n1.meraki.com/New/n/x/manage/access_points/Q1/summary"}`},
		{"organizations_campus_gateway_clusters_networks_overviews", `{"networkId":"N_1","url":"old"}`, `{"networkId":"N_1","url":"new"}`},
		{"organizations_campus_gateway_clusters_ssids", `{"ssidId":"s1","url":"old"}`, `{"ssidId":"s1","url":"new"}`},
		{"organizations_sensor_gateways_connections_latest", `{"lastReportedAt":"2026-10-01T00:00:00Z","rssi":-24,"network":{"id":"N_1"},"sensor":{"serial":"S1"},"gateway":{"serial":"G1"}}`, `{"lastReportedAt":"2026-10-02T00:00:00Z","rssi":-30,"network":{"id":"N_1"},"sensor":{"serial":"S1"},"gateway":{"serial":"G1"}}`},
	}
	for _, p := range stable {
		ida, idb := ExtractResourceID(p.resource, decode(p.a)), ExtractResourceID(p.resource, decode(p.b))
		if ida == "" || ida != idb {
			t.Fatalf("%s: the same row after a rename got IDs %q and %q, want one stable ID", p.resource, ida, idb)
		}
	}
}
