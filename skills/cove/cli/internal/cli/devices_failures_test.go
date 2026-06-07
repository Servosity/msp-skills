// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDevicesFailuresFiltersAndDecodes(t *testing.T) {
	now := time.Now().Unix()
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 1, "PartnerId": 10, "Settings": settingsList(map[string]string{
			"I1": "ok-host", "I8": "Acme", "D9F00": "5", "D9F15": fmt.Sprint(now),
		})},
		{"AccountId": 2, "PartnerId": 10, "Settings": settingsList(map[string]string{
			"I1": "bad-host", "I8": "Acme", "I10": "Server Backup", "D9F00": "2", "D9F06": "4", "D9F15": fmt.Sprint(now),
		})},
		{"AccountId": 3, "PartnerId": 11, "Settings": settingsList(map[string]string{
			"I1": "never-host", "I8": "Globex", "D9F00": "7",
		})},
	})
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "devices", "failures", "--json")
	if err != nil {
		t.Fatalf("devices failures: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			AccountID  int64  `json:"account_id"`
			DeviceName string `json:"device_name"`
			Customer   string `json:"customer"`
			StatusName string `json:"status_name"`
			Errors     int64  `json:"errors"`
		} `json:"items"`
		TotalFailures  int `json:"total_failures"`
		ScannedDevices int `json:"scanned_devices"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if view.TotalFailures != 2 || len(view.Items) != 2 {
		t.Fatalf("expected exactly the failed + never-run devices, got %+v", view)
	}
	// Healthy device (status 5) must be absent.
	for _, it := range view.Items {
		if it.DeviceName == "ok-host" {
			t.Fatal("completed device leaked into the failure sweep")
		}
	}
	// Status decode: 2 → Failed with error count carried through.
	if view.Items[0].StatusName != "Failed" || view.Items[0].Errors != 4 {
		t.Fatalf("expected decoded Failed with 4 errors first (sorted Acme<Globex), got %+v", view.Items[0])
	}
	if view.Items[1].StatusName != "NotStarted" || view.Items[1].Customer != "Globex" {
		t.Fatalf("expected NotStarted Globex device second, got %+v", view.Items[1])
	}
	if view.ScannedDevices != 3 {
		t.Fatalf("expected scanned_devices=3, got %d", view.ScannedDevices)
	}
}

func TestDevicesFailuresExcludesNeverRunWhenAsked(t *testing.T) {
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 3, "PartnerId": 11, "Settings": settingsList(map[string]string{
			"I1": "never-host", "I8": "Globex", "D9F00": "7",
		})},
	})
	fleetTestEnv(t, srv)
	out, err := runCoveCmd(t, "devices", "failures", "--include-never-run=false", "--json")
	if err != nil {
		t.Fatalf("devices failures: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"total_failures": 0`) {
		t.Fatalf("expected zero failures with never-run excluded:\n%s", out)
	}
	if !strings.Contains(out, "no failed last sessions") {
		t.Fatalf("expected honest zero-result note:\n%s", out)
	}
}

func TestDevicesFailuresDryRunShortCircuits(t *testing.T) {
	// No server, no creds: --dry-run must exit clean before any IO.
	t.Setenv("COVE_USERNAME", "")
	t.Setenv("COVE_PASSWORD", "")
	out, err := runCoveCmd(t, "devices", "failures", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run should not error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "would sweep") {
		t.Fatalf("dry-run should describe the would-be action:\n%s", out)
	}
}
