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
