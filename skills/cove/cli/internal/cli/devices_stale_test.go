// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDevicesStaleRanksWorstFirst(t *testing.T) {
	now := time.Now()
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 1, "PartnerId": 10, "Settings": settingsList(map[string]string{
			"I1": "fresh-host", "I8": "Acme", "D9F09": fmt.Sprint(now.Add(-2 * time.Hour).Unix()), "D9F00": "5",
		})},
		{"AccountId": 2, "PartnerId": 10, "Settings": settingsList(map[string]string{
			"I1": "stale-host", "I8": "Acme", "D9F09": fmt.Sprint(now.Add(-5 * 24 * time.Hour).Unix()), "D9F00": "5",
		})},
		{"AccountId": 3, "PartnerId": 11, "Settings": settingsList(map[string]string{
			"I1": "never-host", "I8": "Globex", "D9F00": "7",
		})},
	})
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "devices", "stale", "--days", "3", "--json")
	if err != nil {
		t.Fatalf("devices stale: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			DeviceName     string  `json:"device_name"`
			DaysStale      float64 `json:"days_stale"`
			NeverSucceeded bool    `json:"never_succeeded"`
		} `json:"items"`
		TotalStale int `json:"total_stale"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if view.TotalStale != 2 {
		t.Fatalf("expected stale-host + never-host only, got %+v", view)
	}
	// Never-succeeded ranks first, then the 5-day gap.
	if !view.Items[0].NeverSucceeded || view.Items[0].DeviceName != "never-host" {
		t.Fatalf("expected never-succeeded device first, got %+v", view.Items[0])
	}
	if view.Items[1].DeviceName != "stale-host" || view.Items[1].DaysStale < 4.5 {
		t.Fatalf("expected ~5 days stale for stale-host, got %+v", view.Items[1])
	}
	// The 2-hour-fresh device must be absent (negative filter check).
	if strings.Contains(out, "fresh-host") {
		t.Fatalf("fresh device leaked into stale list:\n%s", out)
	}
}

func TestDevicesStaleRejectsNonPositiveDays(t *testing.T) {
	srv := mockFleet(t, nil)
	fleetTestEnv(t, srv)
	out, err := runCoveCmd(t, "devices", "stale", "--days", "0", "--json")
	if err == nil {
		t.Fatalf("expected usage error for --days 0, got:\n%s", out)
	}
}
