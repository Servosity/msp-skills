// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCustomerUUIDSeparatesBucketsFromCustomers (adversarial round 6, P2):
// fleet stale device rows and devices profile's device.customer carried a
// bare customer name, so a real COMPANY named "(parent group not visible)"
// read the same as the owner-unknown bucket. Both now carry customer_uuid:
// the COMPANY group's uuid, null for a bucket (handfixes.json:
// customer-uuid-beside-customer-name).
func TestCustomerUUIDSeparatesBucketsFromCustomers(t *testing.T) {
	const (
		kiosk = "d3000000-0000-4000-8000-000000000003"
		lf    = "d8000000-0000-4000-8000-000000000008"
	)
	old := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": {
			{"uuid": "acme", "displayName": "Acme", "parentGroupUuid": "hidden-msp", "linkedEntityType": esetCompanyEntityType},
			{"uuid": "lookalike", "displayName": esetCustomerParentHidden, "parentGroupUuid": "hidden-msp", "linkedEntityType": esetCompanyEntityType},
			{"uuid": "lost", "displayName": "Lost & found", "parentGroupUuid": "hidden-all"},
		},
		"devices": {
			{"uuid": kiosk, "displayName": "KIOSK-03", "parentGroupUuid": "lookalike", "lastSyncTime": old},
			{"uuid": lf, "displayName": "LF-08", "parentGroupUuid": "lost", "lastSyncTime": old},
		},
	})
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices/" + kiosk:
			_, _ = w.Write([]byte(`{"device":{"uuid":"` + kiosk + `","displayName":"KIOSK-03","parentGroupUuid":"lookalike","lastSyncTime":"` + old + `"}}`))
		case "/v1/devices/" + lf:
			_, _ = w.Write([]byte(`{"device":{"uuid":"` + lf + `","displayName":"LF-08","parentGroupUuid":"lost","lastSyncTime":"` + old + `"}}`))
		default:
			_, _ = w.Write([]byte(`{"detections":[],"vulnerabilities":[],"devices":[],"scanDetails":[],"patchingDetails":[],"nextPageToken":""}`))
		}
	})
	want := map[string]any{kiosk: "lookalike", lf: nil}

	out, stderr, err := reportVulnsRunCLI(t, srv, "fleet", "stale", "--days", "14", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet stale: %v\n%s\n%s", err, out, stderr)
	}
	var stale struct {
		Devices []map[string]any `json:"devices"`
	}
	fleetHealthTestDecode(t, []byte(out), &stale)
	if len(stale.Devices) != 2 {
		t.Fatalf("fleet stale: want both devices: %s", out)
	}
	for _, row := range stale.Devices {
		uuid, _ := row["device_uuid"].(string)
		got, present := row["customer_uuid"]
		if !present || got != want[uuid] {
			t.Fatalf("fleet stale %s: customer %v customer_uuid %v (present %v), want %v", row["device"], row["customer"], got, present, want[uuid])
		}
	}

	for dev, wantUUID := range want {
		out, stderr, err := reportVulnsRunCLI(t, srv, "devices", "profile", dev, "--db", db, "--json")
		if err != nil && !strings.Contains(out, `"device"`) {
			t.Fatalf("devices profile %s: %v\n%s\n%s", dev, err, out, stderr)
		}
		var prof struct {
			Device map[string]any `json:"device"`
		}
		fleetHealthTestDecode(t, []byte(out), &prof)
		got, present := prof.Device["customer_uuid"]
		if prof.Device["customer"] != esetCustomerParentHidden || !present || got != wantUUID {
			raw, _ := json.Marshal(prof.Device)
			t.Fatalf("devices profile %s: customer_uuid %v (present %v), want %v: %s", dev, got, present, wantUUID, raw)
		}
	}
}
